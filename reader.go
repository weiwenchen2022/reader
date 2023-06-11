// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// package reader provides Reader.
// It implements the io.Reader, io.ReaderAt, io.WriterTo, io.Seeker,
// io.ByteScanner, and io.RuneScanner interfaces by reading from
// a byte slice or a string.
package reader

import (
	"errors"
	"io"
	"unicode/utf8"
)

// A Reader implements the io.Reader, io.ReaderAt, io.WriterTo, io.Seeker,
// io.ByteScanner, and io.RuneScanner interfaces by reading from
// a byte slice or a string.
// A Reader is read-only and supports seeking.
// The zero value for Reader operates like a Reader of an empty slice or an empty string.
type Reader[S ~[]byte | ~string] struct {
	s        S
	off      int64  // read at s[off]
	lastRead readOp // last read operation, so that Unread* can work correctly.
}

// The readOp constants describe the last action performed on
// the reader, so that UnreadRune and UnreadByte can check for
// invalid usage. opReadRuneX constants are chosen such that
// converted to int they correspond to the rune size that was read.
type readOp int8

// Don't use iota for these, as the values need to correspond with the
// names and comments, which is easier to see when being explicit.
const (
	opRead      readOp = -1 // Any other read operation.
	opInvalid   readOp = 0  // Non-read operation.
	opReadRune1 readOp = 1  // Read rune of size 1.
	opReadRune2 readOp = 2  // Read rune of size 2.
	opReadRune3 readOp = 3  // Read rune of size 3.
	opReadRune4 readOp = 4  // Read rune of size 4.
)

// Len returns the number of bytes of the unread portion of the
// slice or string.
func (r *Reader[S]) Len() int {
	if r.off >= int64(len(r.s)) {
		return 0
	}
	return int(int64(len(r.s)) - r.off)
}

// Size returns the original length of the underlying byte slice or string.
// Size is the number of bytes available for reading via ReadAt.
// The returned value is always the same and is not affected
// by any method calls except Reset.
func (r *Reader[S]) Size() int64 { return int64(len(r.s)) }

// Read implements the io.Reader interface.
func (r *Reader[S]) Read(p []byte) (n int, err error) {
	if r.off >= int64(len(r.s)) {
		return 0, io.EOF
	}

	r.lastRead = opInvalid
	n = copy(p, r.s[r.off:])
	r.off += int64(n)
	if n > 0 {
		r.lastRead = opRead
	}
	return n, nil
}

// ReadAt implements the io.ReaderAt interface.
func (r *Reader[S]) ReadAt(p []byte, off int64) (n int, err error) {
	// cannot modify state - see io.ReaderAt
	if off < 0 {
		return 0, errors.New("reader.Reader.ReadAt: negative offset")
	}

	if off >= int64(len(r.s)) {
		return 0, io.EOF
	}

	n = copy(p, r.s[off:])
	if n < len(p) {
		err = io.EOF
	}
	return n, err
}

// ReadByte implements the io.ByteReader interface.
func (r *Reader[S]) ReadByte() (byte, error) {
	r.lastRead = opInvalid
	if r.off >= int64(len(r.s)) {
		return 0, io.EOF
	}

	c := r.s[r.off]
	r.off++
	r.lastRead = opRead
	return c, nil
}

// UnreadByte complements ReadByte in implementing the io.ByteScanner interface.
func (r *Reader[S]) UnreadByte() error {
	if r.off <= 0 {
		return errors.New("reader.Reader.UnreadByte: at beginning of slice or string")
	}

	r.lastRead = opInvalid
	r.off--
	return nil
}

// ReadRune implements the io.RuneReader interface.
func (r *Reader[S]) ReadRune() (ch rune, size int, err error) {
	if r.off >= int64(len(r.s)) {
		r.lastRead = opInvalid
		return 0, 0, io.EOF
	}

	if c := r.s[r.off]; c < utf8.RuneSelf {
		r.off++
		r.lastRead = opReadRune1
		return rune(c), 1, nil
	}

	ch, size = utf8.DecodeRune([]byte(r.s[r.off:]))
	r.off += int64(size)
	r.lastRead = readOp(size)
	return ch, size, nil
}

// UnreadRune complements ReadRune in implementing the io.RuneScanner interface.
func (r *Reader[S]) UnreadRune() error {
	switch r.lastRead {
	default:
		return errors.New("reader.Reader.UnreadRune: previous operation was not ReadRune")
	case opReadRune1, opReadRune2, opReadRune3, opReadRune4:
	}

	r.off -= int64(r.lastRead)
	r.lastRead = opInvalid
	return nil
}

// Seek implements the io.Seeker interface.
func (r *Reader[S]) Seek(offset int64, whence int) (int64, error) {
	r.lastRead = opInvalid
	switch whence {
	default:
		return 0, errors.New("reader.Reader.Seek: invalid whence")
	case io.SeekStart:
	case io.SeekCurrent:
		offset += r.off
	case io.SeekEnd:
		offset += int64(len(r.s))
	}

	if offset < 0 {
		return 0, errors.New("reader.Reader.Seek: negative position")
	}

	r.off = offset
	return offset, nil
}

// WriteTo implements the io.WriterTo interface.
func (r *Reader[S]) WriteTo(w io.Writer) (n int64, err error) {
	r.lastRead = opInvalid
	if r.off >= int64(len(r.s)) {
		return 0, nil
	}

	s := r.s[r.off:]
	m, err := w.Write([]byte(s))
	if m > len(s) {
		panic("reader.Reader.WriteTo: invalid Write count")
	}

	r.off += int64(m)
	n = int64(m)
	if len(s) != m && err == nil {
		err = io.ErrShortWrite
	}
	return n, err
}

// Reset resets the Reader to be reading from s.
func (r *Reader[S]) Reset(s S) { *r = Reader[S]{s: s} }

// New returns a new Reader reading from s.
func New[S ~[]byte | ~string](s S) *Reader[S] { return &Reader[S]{s: s} }
