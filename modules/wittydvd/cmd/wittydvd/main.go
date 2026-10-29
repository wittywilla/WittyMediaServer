// Command wittydvd inspects DVD-Video images and VIDEO_TS folders with the
// WittyDVD engine: titles, menus and languages, the disc's program, a
// simulated run through its menus, and the menu artwork. It reads nothing
// but the disc and writes nothing but what you ask for, so its output is
// safe to attach to a bug report.
package main

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittydvd/cli"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "disc":
			os.Exit(cli.Disc(os.Args[2:]))
		case "info", "walk", "crawl", "pgc", "render":
			os.Exit(cli.DVD(os.Args[1:]))
		case "version", "--version", "-v":
			fmt.Println("wittydvd", version)
			return
		}
	}
	fmt.Fprint(os.Stderr, `wittydvd `+version+`: inspect DVD-Video images and VIDEO_TS folders

  wittydvd info    <image>              titles, menus, languages, first-play program
  wittydvd walk    <image> [button...]  simulate inserting the disc and pressing buttons
  wittydvd crawl   <image>              press every button of every menu; report dead ends
  wittydvd pgc     <image> <domain> <vts> <pgcn>   the commands of one program chain
  wittydvd render  <image> <out-dir>    save the first menu's highlight images
  wittydvd disc ls <image>              list the files inside a disc image

<image> is an .iso/.img file or a VIDEO_TS folder. Scrambled (CSS) discs are refused.
`)
	os.Exit(2)
}
