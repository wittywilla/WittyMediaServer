// Command wittycd inspects audio CD images with the WittyCD engine: tracks,
// indexes, CD-TEXT, disc IDs, and a read of the whole image to find damage. It
// reads nothing but the image and writes nothing, and its output holds no audio,
// so it is safe to attach to a bug report.
package main

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittycd/cli"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "info", "check":
			os.Exit(cli.CD(os.Args[1:]))
		case "version", "--version", "-v":
			fmt.Println("wittycd", version)
			return
		}
	}
	fmt.Fprint(os.Stderr, `wittycd `+version+`: inspect audio CD images

  wittycd info  [-json] <image>   tracks, indexes, CD-TEXT, flags and disc ID
  wittycd check [-json] <image>   read the whole image and report what is wrong with it

<image> is a .cue, .toc, .ccd, .nrg, .chd, or a bare .iso, .img or .bin of CD audio.
`)
	os.Exit(2)
}
