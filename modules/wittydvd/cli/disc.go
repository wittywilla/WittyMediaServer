package cli

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittydvd/isofs"
)

// Disc runs the disc subcommands: ls.
func Disc(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: disc ls <image.iso>")
		return 2
	}
	switch args[0] {
	case "ls":
		d, err := isofs.Open(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer d.Close()
		fmt.Printf("format=%s label=%q size=%d\n", d.Format, d.Label, d.Size())
		for _, f := range d.Files() {
			if f.IsDir {
				fmt.Printf("%12s  %s/\n", "", f.Path)
				continue
			}
			fmt.Printf("%12d  %s", f.Size, f.Path)
			if len(f.Extents) != 1 {
				fmt.Printf("  (%d extents)", len(f.Extents))
			} else {
				fmt.Printf("  @sector %d", f.Extents[0].Offset/isofs.Sector)
			}
			fmt.Println()
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, "unknown disc subcommand", args[0])
	return 2
}
