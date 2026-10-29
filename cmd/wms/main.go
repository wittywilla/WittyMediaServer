// Command wms is the WittyMediaServer binary. With no arguments it starts the
// server; subcommands expose disc tooling used during development and support.
package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "disc":
			os.Exit(cmdDisc(os.Args[2:]))
		case "dvd":
			os.Exit(cmdDVD(os.Args[2:]))
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

var version = "dev"

func usage() {
	fmt.Println(`WittyMediaServer

usage:
  wms [serve] [flags]        start the server (default)
  wms disc ls <image>        list files inside an ISO/UDF image
  wms dvd info <image>       dump DVD-Video structure (titles, menus, chapters)
  wms dvd walk <image>       run first-play and print the menu the viewer would see
  wms dvd render <image> <out-dir>
                             write menu overlay/highlight PNGs for the first menu
  wms version`)
}
