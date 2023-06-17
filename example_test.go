package reader_test

import (
	"fmt"

	"github.com/weiwenchen2022/reader"
)

func ExampleReader_Len() {
	fmt.Println(reader.New([]byte("Hi!")).Len(), reader.New("Hi!").Len())
	fmt.Println(reader.New([]byte("こんにちは!")).Len(), reader.New("こんにちは!").Len())
	// Output:
	// 3 3
	// 16 16
}
