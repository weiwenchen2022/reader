package reader_test

import (
	"fmt"

	"github.com/weiwenchen2022/reader"
)

func ExampleReader_Len() {
	fmt.Println(reader.New([]byte("Hi!")).Len())
	fmt.Println(reader.New([]byte("こんにちは!")).Len())
	// Output:
	// 3
	// 16
}
