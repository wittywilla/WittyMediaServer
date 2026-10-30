// Command wittybluray inspects Blu-ray images and BDMV folders with the
// WittyBluray engine. It reads nothing but the disc, and its output holds no
// video or artwork, so it is safe to attach to a bug report.
package main

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittybluray/cli"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "info", "playlist", "walk", "render":
			os.Exit(cli.Bluray(os.Args[1:]))
		case "version", "--version", "-v":
			fmt.Println("wittybluray", version)
			return
		}
	}
	fmt.Fprint(os.Stderr, `wittybluray `+version+`: inspect Blu-ray images and BDMV folders

  wittybluray info      <disc>            titles, the disc's program, playlists, clips and streams
  wittybluray playlist  <disc> <number>   every item, stream and chapter of one playlist
  wittybluray walk      <disc> [button...] simulate inserting the disc, then pressing menu buttons by id
  wittybluray render    <disc> <out-dir>  save the first menu's pages as PNG files

<disc> is an .iso/.img image, or a folder that contains BDMV. Encrypted (AACS) discs are refused.
`)
	os.Exit(2)
}
