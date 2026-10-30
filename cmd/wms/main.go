// Command wms is the WittyMediaServer binary. With no arguments it starts the
// server; subcommands expose disc tooling used during development and support.
package main

import (
	"fmt"
	"os"

	bdcli "github.com/wittywilla/wittybluray/cli"
	"github.com/wittywilla/wittydvd/cli"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "disc":
			os.Exit(cli.Disc(os.Args[2:]))
		case "dvd":
			os.Exit(cli.DVD(os.Args[2:]))
		case "bluray":
			os.Exit(bdcli.Bluray(os.Args[2:]))
		case "config":
			os.Exit(cmdConfig(os.Args[2:]))
		case "serve":
			os.Exit(cmdServe(os.Args[2:]))
		case "version", "--version", "-v":
			fmt.Println("wms", version)
			return
		case "help", "-h", "--help":
			usage()
			return
		}
	}
	os.Exit(cmdServe(os.Args[1:]))
}

// version is stamped by the release scripts (-X main.version=...); this is
// the default for source builds.
var version = "0.2.0-alpha"

func usage() {
	fmt.Println(`WittyMediaServer

usage:
  wms [serve] [flags]        start the server (default); -config <file> uses a wms.json
  wms config path            show the config file and folders in use
  wms config init [--portable]
                             write a starter wms.json (next to the program with --portable)
  wms disc ls <image>        list files inside an ISO/UDF image
  wms dvd info <image>       dump DVD-Video structure (titles, menus, chapters)
  wms dvd walk <image> [n…]  run first-play, print the menu the viewer would see, press buttons
  wms dvd crawl <image>      press every button of every menu and report dead ends
  wms dvd pgc <image> fp|vmgm|vtsm|vts <vts> <pgcn>
                             disassemble one program chain
  wms dvd render <image> <out-dir>
  wms bluray info <disc>     Blu-ray titles, program, playlists, clips and streams
  wms bluray walk <disc> [button…]  simulate inserting a Blu-ray and pressing menu buttons
  wms bluray render <disc> <out-dir>  save a Blu-ray menu's pages as PNG files
                             write menu overlay/highlight PNGs for the first menu
  wms version`)
}
