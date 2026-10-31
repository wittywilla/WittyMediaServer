// Command fakecore writes one of the test cores to a folder, for trying the custom
// core pages by hand without any third-party code:
//
//	go run ./testcore/fakecore/cmd/fakecore <folder> [variant] [name]
//
// Variants are good, crash-init, crash-open, hang-init, hang-open, missing, wrong-api,
// hostile-names and foreign (a library for some other kind of computer).
package main

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittyplay/testcore/fakecore"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fakecore <folder> [variant] [name]")
		os.Exit(2)
	}
	variant, name := "good", "fake_libretro"
	if len(os.Args) > 2 {
		variant = os.Args[2]
	}
	if len(os.Args) > 3 {
		name = os.Args[3]
	}
	var path string
	var err error
	if variant == "foreign" {
		path, err = fakecore.Foreign(os.Args[1], name)
	} else {
		path, err = fakecore.Build(os.Args[1], name, fakecore.Variant(variant))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(path)
}
