package base8_test

import (
	"fmt"
	"os"

	"github.com/pgavlin/base8"
)

func ExampleEncoding_EncodeToString() {
	data := []byte("any + old & data")
	str := base8.EncodeToString(data)
	fmt.Println(str)
	// Output:
	// 3026717110025440336661441002304031060564302=====
}

func ExampleEncoding_DecodeString() {
	str := "3466755531220144302721411007355135064040000201413346204073735677"
	data, err := base8.DecodeString(str)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("%q\n", data)
	// Output:
	// "some data with \x00 and \ufeff"
}

func ExampleNewEncoder() {
	input := []byte("foo\x00bar")
	encoder := base8.NewEncoder(os.Stdout)
	encoder.Write(input)
	// Must close the encoder when finished to flush any partial blocks.
	// If you comment out the following line, the last partial block "r"
	// won't be encoded.
	encoder.Close()
	// Output:
	// 3146755700061141344=====
}
