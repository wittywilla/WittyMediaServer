// Package cli is the WittyCD command line, used by the standalone wittycd tool
// and by `wms cd`.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/wittywilla/wittycd"
)

// CD runs the cd subcommands: info, check.
func CD(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cd info [-json] <image> | cd check [-json] <image>")
		return 2
	}
	cmd := args[0]
	asJSON := false
	rest := args[1:]
	if len(rest) > 1 && rest[0] == "-json" {
		asJSON = true
		rest = rest[1:]
	}
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: cd info [-json] <image> | cd check [-json] <image>")
		return 2
	}
	d, err := wittycd.Open(rest[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "wittycd:", err)
		return 1
	}
	defer d.Close()
	switch cmd {
	case "info":
		if asJSON {
			return printJSON(d.Facts())
		}
		info(d)
		return 0
	case "check":
		r := d.Check(nil)
		if asJSON {
			if printJSON(r) != 0 {
				return 1
			}
		} else {
			printReport(r)
		}
		if r.Worst == wittycd.SeverityProblem {
			return 1
		}
		return 0
	}
	fmt.Fprintln(os.Stderr, "unknown cd command:", cmd)
	return 2
}

func printJSON(v any) int {
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "  ")
	if err := e.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "wittycd:", err)
		return 1
	}
	return 0
}

func msf(sectors int) string {
	if sectors < 0 {
		return "-" + msf(-sectors)
	}
	return fmt.Sprintf("%02d:%02d:%02d", sectors/wittycd.FramesPerSecond/60, sectors/wittycd.FramesPerSecond%60, sectors%wittycd.FramesPerSecond)
}

func info(d *wittycd.Disc) {
	f := d.Facts()
	fmt.Printf("image: %s (%s)\n", d.Path, f.Format)
	if d.Text.Title != "" || d.Text.Performer != "" {
		fmt.Printf("disc: %s", d.Text.Title)
		if d.Text.Performer != "" {
			fmt.Printf(" by %s", d.Text.Performer)
		}
		fmt.Println()
	}
	if d.Date != "" || d.Genre != "" {
		fmt.Printf("date %s, genre %s\n", d.Date, d.Genre)
	}
	if d.Catalog != "" {
		fmt.Println("barcode:", d.Catalog)
	}
	fmt.Printf("%d tracks (%d audio), %d session(s), playing time %s\n", len(d.Tracks), len(d.AudioTracks()), d.Sessions, msf(int(d.DurationSec()*wittycd.FramesPerSecond)))
	fmt.Printf("disc ID %s, CDDB ID %s\n", f.DiscID, f.CDDBID)
	fmt.Printf("table of contents: %s\n", f.TOC)
	if d.HiddenTrack {
		fmt.Println("hidden track: audio in track 1's pregap")
	}
	if d.HasGraphics {
		fmt.Println("CD+G graphics: yes")
	}
	fmt.Println()
	fmt.Println(" tr  type   pregap    start     length    flags           title")
	for i := range d.Tracks {
		t := &d.Tracks[i]
		var fl []string
		if t.PreEmphasis {
			fl = append(fl, "PRE")
		}
		if t.CopyAllowed {
			fl = append(fl, "DCP")
		}
		if t.FourChannel {
			fl = append(fl, "4CH")
		}
		kind := "audio"
		if !t.Audio {
			kind = "data"
		}
		name := t.Text.Title
		if t.Text.Performer != "" {
			name += " / " + t.Text.Performer
		}
		fmt.Printf(" %2d  %-5s  %-8s  %-8s  %-8s  %-14s  %s\n", t.Number, kind, msf(t.Start-t.Pregap), msf(t.Start), msf(t.End-t.Start), strings.Join(fl, ","), name)
		for _, x := range t.Indexes {
			fmt.Printf("     index %02d at %s\n", x.Number, msf(x.LBA))
		}
		if t.ISRC != "" {
			fmt.Printf("     ISRC %s\n", t.ISRC)
		}
	}
	if len(d.Warnings) > 0 {
		fmt.Println()
		for _, w := range d.Warnings {
			fmt.Println("note:", w.Message)
		}
	}
}

func printReport(r *wittycd.Report) {
	for _, f := range r.Findings {
		fmt.Printf("%-8s %s\n", f.Severity, f.Text)
	}
	fmt.Printf("checked %d sectors in %d audio tracks: %s\n", r.Sectors, r.AudioTracks, r.Summary)
}
