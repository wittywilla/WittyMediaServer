package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/wittywilla/wittydisc/isofs"
)

// Disc runs the disc subcommands: ls, cat.
func Disc(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: disc ls <image.iso>\n       disc cat <image.iso> <path-in-image>   (the file's bytes on stdout)")
		return 2
	}
	switch args[0] {
	case "cat":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: disc cat <image.iso> <path-in-image>")
			return 2
		}
		d, err := isofs.Open(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer d.Close()
		f, ok := d.Find(args[2])
		if !ok || f.IsDir {
			fmt.Fprintln(os.Stderr, "not a file in this image:", args[2])
			return 1
		}
		if _, err := io.Copy(os.Stdout, io.NewSectionReader(d.Reader(f), 0, f.Size)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
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
