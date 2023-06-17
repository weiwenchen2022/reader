// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package reader_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"

	. "github.com/weiwenchen2022/reader"
)

const N = 10000       // make this bigger for a larger (and slower) test
var testString string // test data for write tests
var testBytes []byte  // test data; same as testString but as a slice.

func init() {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	testBytes = make([]byte, N)
	for i := 0; i < N; i++ {
		testBytes[i] = 'a' + byte(r.Intn(26))
	}
	testString = string(testBytes)
}

type readerInterface interface {
	Size() int64
	Len() int

	io.Reader
	io.ReaderAt

	io.WriterTo

	io.Seeker

	io.ByteScanner
	io.RuneScanner
}

func testReader[S ~[]byte | ~string](t *testing.T, s S, testFn func(t *testing.T, r readerInterface)) {
	t.Helper()

	var r readerInterface
	r = New([]byte(s))
	t.Run(fmt.Sprintf("%T", r), func(t *testing.T) {
		testFn(t, r)
	})

	r = New(string(s))
	t.Run(fmt.Sprintf("%T", r), func(t *testing.T) {
		testFn(t, r)
	})
}

func TestReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		off     int64
		whence  int
		n       int
		want    string
		wantpos int64
		readerr error
		seekerr string
	}{
		{off: 0, whence: io.SeekStart, n: 20, want: "0123456789"},
		{off: 1, whence: io.SeekStart, n: 1, want: "1"},
		{off: 1, whence: io.SeekCurrent, wantpos: 3, n: 2, want: "34"},
		{off: -1, whence: io.SeekStart, seekerr: "reader.Reader.Seek: negative position"},
		{off: 1 << 33, whence: io.SeekStart, wantpos: 1 << 33, readerr: io.EOF},
		{off: 1, whence: io.SeekCurrent, wantpos: 1<<33 + 1, readerr: io.EOF},
		{whence: io.SeekStart, n: 5, want: "01234"},
		{whence: io.SeekCurrent, n: 5, want: "56789"},
		{off: -1, whence: io.SeekEnd, n: 1, wantpos: 9, want: "9"},
	}

	testReader(t, "0123456789", func(t *testing.T, r readerInterface) {
		for i, tt := range tests {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				pos, err := r.Seek(tt.off, tt.whence)
				if tt.seekerr != "" && err == nil {
					t.Fatalf("want seek error %q", tt.seekerr)
				}
				if err != nil && tt.seekerr != err.Error() {
					t.Fatalf("seek error = %q; want %q", err, tt.seekerr)
				}
				if tt.wantpos != 0 && tt.wantpos != pos {
					t.Errorf("pos = %d, want %d", pos, tt.wantpos)
				}
				buf := make([]byte, tt.n)
				n, err := r.Read(buf)
				if tt.readerr != err {
					t.Fatalf("read = %v; want %v", err, tt.readerr)
				}
				if got := string(buf[:n]); tt.want != got {
					t.Errorf("got %q; want %q", got, tt.want)
				}
			})
		}
	})
}

func TestReadAfterBigSeek(t *testing.T) {
	t.Parallel()

	testReader(t, "0123456789", func(t *testing.T, r readerInterface) {
		if _, err := r.Seek(1<<31+5, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		if n, err := r.Read(make([]byte, 10)); n != 0 || io.EOF != err {
			t.Errorf("Read = %d, %v; want 0, EOF", n, err)
		}
	})
}

func TestReaderAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		off     int64
		n       int
		want    string
		wanterr any
	}{
		{0, 10, "0123456789", nil},
		{1, 10, "123456789", io.EOF},
		{1, 9, "123456789", nil},
		{11, 10, "", io.EOF},
		{0, 0, "", nil},
		{-1, 0, "", "reader.Reader.ReadAt: negative offset"},
	}

	testReader(t, "0123456789", func(t *testing.T, r readerInterface) {
		for i, tt := range tests {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				b := make([]byte, tt.n)
				n, err := r.ReadAt(b, tt.off)
				if tt.want != string(b[:n]) {
					t.Errorf("got %q; want %q", b[:n], tt.want)
				}
				if fmt.Sprint(tt.wanterr) != fmt.Sprint(err) {
					t.Errorf("got error = %q; want %q", err, tt.wanterr)
				}
			})
		}
	})
}

func TestReaderAtConcurrent(t *testing.T) {
	// Test for the race detector, to verify ReadAt doesn't mutate
	// any state.
	testReader(t, "0123456789", func(t *testing.T, r readerInterface) {
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(i int) {
				var buf [1]byte
				_, _ = r.ReadAt(buf[:], int64(i))
				wg.Done()
			}(i)
		}
		wg.Wait()
	})
}

func TestEmptyReaderConcurrent(t *testing.T) {
	t.Parallel()

	// Test for the race detector, to verify a Read that doesn't yield any bytes
	// is okay to use from multiple goroutines. This was our historic behavior.
	// See golang.org/issue/7856
	testReader(t, "", func(t *testing.T, r readerInterface) {
		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(2)
			go func() {
				var buf [1]byte
				_, _ = r.Read(buf[:])
				wg.Done()
			}()
			go func() {
				_, _ = r.Read(nil)
				wg.Done()
			}()
		}
		wg.Wait()
	})
}

func TestReaderWriteTo(t *testing.T) {
	t.Parallel()

	testReader(t, "", func(t *testing.T, r readerInterface) {
		for i := 0; i < 30; i += 3 {
			var l int
			if i > 0 {
				l = len(testString) / i
			}
			s := testString[:l]

			switch r.(type) {
			case *Reader[[]byte]:
				r = New(testBytes[:l])
			case *Reader[string]:
				r = New(testString[:l])
			default:
				t.Fatalf("unknown reader %T", r)
			}

			var b bytes.Buffer
			n, err := r.WriteTo(&b)
			if expect := int64(len(s)); expect != n {
				t.Errorf("got %d; want %d", n, expect)
			}
			if err != nil {
				t.Errorf("for length %d: got error = %v; want nil", l, err)
			}
			if s != b.String() {
				t.Errorf("got string %q; want %q", b.String(), s)
			}
			if r.Len() != 0 {
				t.Errorf("reader contains %d bytes; want 0", r.Len())
			}
		}
	})

	testReader(t, "", func(t *testing.T, r readerInterface) {
		const str = "0123456789"
		for i := 0; i <= len(str); i++ {
			s := str[i:]
			switch r.(type) {
			case *Reader[[]byte]:
				r = New([]byte(s))
			case *Reader[string]:
				r = New(s)
			default:
				t.Fatalf("unknown reader: %T", r)
			}

			var b bytes.Buffer
			n, err := r.WriteTo(&b)
			if expect := int64(len(s)); expect != n {
				t.Errorf("got %d; want %d", n, expect)
			}
			if err != nil {
				t.Errorf("for length %d: got error = %v; want nil", len(s), err)
			}
			if s != b.String() {
				t.Errorf("got string %q; want %q", b.String(), s)
			}
			if r.Len() != 0 {
				t.Errorf("reader contains %d bytes; want 0", r.Len())
			}
		}
	})
}

func TestReaderLen(t *testing.T) {
	t.Parallel()

	const data = "hello world"
	testReader(t, data, func(t *testing.T, r readerInterface) {
		if got, want := r.Len(), len(data); want != got {
			t.Errorf("r.Len(): got %d, want %d", got, want)
		}
		if n, err := r.Read(make([]byte, 10)); n != 10 || err != nil {
			t.Errorf("Read failed: read %d %v", n, err)
		}
		if got, want := r.Len(), 1; got != want {
			t.Errorf("r.Len(): got %d, want %d", got, want)
		}
		if n, err := r.Read(make([]byte, 1)); n != 1 || err != nil {
			t.Errorf("Read failed: read %d %v; want 1, nil", n, err)
		}
		if got := r.Len(); got != 0 {
			t.Errorf("r.Len(): got %d, want 0", got)
		}
	})
}

var unreadRuneErrorTests = []struct {
	name string
	f    func(readerInterface)
}{
	{"Read", func(r readerInterface) { _, _ = r.Read([]byte{0}) }},
	{"ReadByte", func(r readerInterface) { _, _ = r.ReadByte() }},
	{"UnreadRune", func(r readerInterface) { _ = r.UnreadRune() }},
	{"Seek", func(r readerInterface) { _, _ = r.Seek(0, io.SeekCurrent) }},
	{"WriteTo", func(r readerInterface) { _, _ = r.WriteTo(&bytes.Buffer{}) }},
}

func TestUnreadRuneError(t *testing.T) {
	t.Parallel()

	testReader(t, "", func(t *testing.T, r readerInterface) {
		for _, tt := range unreadRuneErrorTests {
			switch r.(type) {
			case *Reader[[]byte]:
				r = New([]byte("0123456789"))
			case *Reader[string]:
				r = New("0123456789")
			default:
				t.Fatalf("unknown reader %T", r)
			}

			if _, _, err := r.ReadRune(); err != nil {
				// should not happen
				t.Fatal(err)
			}
			tt.f(r)
			if err := r.UnreadRune(); err == nil {
				t.Errorf("Unreading after %s: expected error", tt.name)
			}
		}
	})
}

func TestReaderDoubleUnreadRune(t *testing.T) {
	t.Parallel()

	testReader(t, "groucho", func(t *testing.T, r readerInterface) {
		if _, _, err := r.ReadRune(); err != nil {
			// should not happen
			t.Fatal(err)
		}
		if err := r.UnreadRune(); err != nil {
			// should not happen
			t.Fatal(err)
		}
		if err := r.UnreadRune(); err == nil {
			t.Fatal("UnreadRune: expected error, got nil")
		}
	})
}

// verify that copying from an empty reader always has the same results,
// regardless of the presence of a WriteTo method.
func TestReaderCopyNothing(t *testing.T) {
	t.Parallel()

	type nErr struct {
		n   int64
		err error
	}
	type justReader struct {
		io.Reader
	}
	type justWriter struct {
		io.Writer
	}
	discard := justWriter{io.Discard} // hide ReadFrom

	testReader(t, "", func(t *testing.T, r readerInterface) {
		var with, withOut nErr

		switch r.(type) {
		case *Reader[[]byte]:
			r = New([]byte(nil))
		case *Reader[string]:
			r = New("")
		default:
			t.Fatalf("unknown reader %T", r)
		}
		with.n, with.err = io.Copy(discard, r)

		switch r.(type) {
		case *Reader[[]byte]:
			r = New([]byte(nil))
		case *Reader[string]:
			r = New("")
		default:
			t.Fatalf("unknown reader %T", r)
		}
		withOut.n, withOut.err = io.Copy(discard, justReader{r})
		if with != withOut {
			t.Errorf("behavior differs: with: %#v; without: %#v", with, withOut)
		}
	})
}

// tests that Len is affected by reads, but Size is not.
func TestReaderLenSize(t *testing.T) {
	t.Parallel()

	testReader(t, "abc", func(t *testing.T, r readerInterface) {
		_, _ = io.CopyN(io.Discard, r, 1)
		if r.Len() != 2 {
			t.Errorf("Len = %d; want 2", r.Len())
		}
		if r.Size() != 3 {
			t.Errorf("Size = %d; want 3", r.Size())
		}
	})
}

func TestReaderReset(t *testing.T) {
	t.Parallel()

	testReader(t, "", func(t *testing.T, r readerInterface) {
		switch r.(type) {
		case *Reader[[]byte]:
			r = New([]byte("世界"))
		case *Reader[string]:
			r = New("世界")
		default:
			t.Fatalf("unknown reader %T", r)
		}
		if _, _, err := r.ReadRune(); err != nil {
			t.Errorf("ReadRune: unexpected error: %v", err)
		}

		const want = "abcdef"
		switch r := r.(type) {
		case *Reader[[]byte]:
			r.Reset([]byte(want))
		case *Reader[string]:
			r.Reset(want)
		default:
			t.Fatalf("unknown reader %T", r)
		}
		if err := r.UnreadRune(); err == nil {
			t.Errorf("UnreadRune: expected error, got nil")
		}
		buf, err := io.ReadAll(r)
		if err != nil {
			t.Errorf("ReadAll: unexpected error: %v", err)
		}
		if want != string(buf) {
			t.Errorf("ReadAll: got %q, want %q", buf, want)
		}
	})
}

func TestReaderZero(t *testing.T) {
	t.Parallel()

	testReader(t, "", func(t *testing.T, r readerInterface) {
		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if l := r.Len(); l != 0 {
			t.Errorf("Len: got %d, want 0", l)
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if n, err := r.Read(nil); n != 0 || io.EOF != err {
			t.Errorf("Read: got %d, %v; want 0, io.EOF", n, err)
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if n, err := r.ReadAt(nil, 11); n != 0 || io.EOF != err {
			t.Errorf("ReadAt: got %d, %v; want 0, io.EOF", n, err)
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if c, err := r.ReadByte(); c != 0 || io.EOF != err {
			t.Errorf("ReadByte: got %d, %v; want 0, io.EOF", c, err)
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if ch, size, err := r.ReadRune(); ch != 0 || size != 0 || io.EOF != err {
			t.Errorf("ReadRune: got %d, %d, %v; want 0, 0, io.EOF", ch, size, err)
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if offset, err := r.Seek(11, io.SeekStart); offset != 11 || err != nil {
			t.Errorf("Seek: got %d, %v; want 11, nil", offset, err)
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if s := r.Size(); s != 0 {
			t.Errorf("Size: got %d, want 0", s)
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if r.UnreadByte() == nil {
			t.Errorf("UnreadByte: got nil, want error")
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if r.UnreadRune() == nil {
			t.Errorf("UnreadRune: got nil, want error")
		}

		r = reflect.New(reflect.TypeOf(r).Elem()).Interface().(readerInterface)
		if n, err := r.WriteTo(io.Discard); n != 0 || err != nil {
			t.Errorf("WriteTo: got %d, %v; want 0, nil", n, err)
		}
	})
}
