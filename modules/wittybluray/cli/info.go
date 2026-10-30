// Package cli holds the wittybluray commands, shared by the standalone
// tool and WittyMediaServer's `wms bluray ...`.
package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/wittywilla/wittybluray"
)

// Bluray runs the bluray subcommands: info, playlist.
func Bluray(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: bluray info|playlist <image-or-folder> [args]")
		return 2
	}
	d, err := wittybluray.Open(args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer d.Close()
	switch args[0] {
	case "info":
		return info(d)
	case "walk":
		var presses []int
		for _, a := range args[2:] {
			var n int
			fmt.Sscanf(a, "%d", &n)
			presses = append(presses, n)
		}
		return walk(d, presses)
	case "render":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: bluray render <image-or-folder> <out-dir>")
			return 2
		}
		return render(d, args[2])
	case "playlist":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: bluray playlist <image-or-folder> <number>")
			return 2
		}
		var n int
		fmt.Sscanf(args[2], "%d", &n)
		pl := d.Playlist(n)
		if pl == nil {
			fmt.Fprintln(os.Stderr, "no such playlist")
			return 1
		}
		printPlaylist(d, pl, true)
		return 0
	}
	fmt.Fprintln(os.Stderr, "unknown bluray subcommand", args[0])
	return 2
}

func hms(sec float64) string {
	s := int(sec + 0.5)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

func objName(o wittybluray.IndexObject) string {
	switch o.Kind {
	case wittybluray.ObjectHDMV:
		kind := "movie"
		if o.Interactive {
			kind = "interactive"
		}
		return fmt.Sprintf("HDMV object %d (%s)", o.MovieObject, kind)
	case wittybluray.ObjectBDJ:
		return fmt.Sprintf("BD-J %q (not supported)", o.BDJName)
	}
	return "none"
}

func info(d *wittybluray.Disc) int {
	ix := d.Index
	fmt.Printf("label=%q source=%s index=%s video-format=%d frame-rate=%d\n", d.Label, d.Source.Describe(), ix.Version, ix.VideoFormat, ix.FrameRate)
	flags := []string{}
	if d.Encrypted {
		flags = append(flags, "ENCRYPTED (AACS): playback refused")
	}
	if d.Stereo3D {
		flags = append(flags, "3D")
	}
	if d.BDJ {
		flags = append(flags, "uses BD-J (Java) menus")
	}
	if len(flags) > 0 {
		fmt.Println("flags:", strings.Join(flags, ", "))
	}
	fmt.Println("first play:", objName(ix.FirstPlay))
	fmt.Println("top menu:  ", objName(ix.TopMenu))
	for _, t := range ix.Titles {
		fmt.Printf("title %d: %s access=%d\n", t.Number, objName(t.Object), t.Access)
	}
	fmt.Printf("movie objects: %d\n", len(d.MovieObjects))
	for i, m := range d.MovieObjects {
		fmt.Printf("  object %d: %d commands", i, len(m.Commands))
		if m.ResumeIntention {
			fmt.Print(" resume")
		}
		if m.MenuCallMask {
			fmt.Print(" menu-call-masked")
		}
		if m.TitleSearchMask {
			fmt.Print(" title-search-masked")
		}
		fmt.Println()
		for pc, c := range m.Commands {
			fmt.Printf("    %3d: %s\n", pc, wittybluray.Decode(c))
		}
	}
	nums := make([]int, 0, len(d.Playlists))
	for n := range d.Playlists {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	main := d.MainPlaylist()
	fmt.Printf("playlists: %d\n", len(nums))
	for _, n := range nums {
		pl := d.Playlists[n]
		mark := ""
		if pl == main {
			mark = "  <- main feature"
		}
		fmt.Printf("  %05d: %s, %d items, %d chapters%s\n", n, hms(d.PlaylistDuration(pl)), len(pl.Items), countChapters(pl), mark)
		printPlaylist(d, pl, false)
	}
	fmt.Printf("clips: %d\n", len(d.Clips))
	names := make([]string, 0, len(d.Clips))
	for n := range d.Clips {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		c := d.Clips[n]
		fmt.Printf("  %s: %s, %d packets (%.1f MB), app-type=%d", n, hms(c.Duration()), c.Packets, float64(c.Size())/1e6, c.ApplicationType)
		for _, m := range c.EPMaps {
			fmt.Printf(", %d entry points for PID %#x", len(m.Entries), m.PID)
		}
		fmt.Println()
		for _, p := range c.Programs {
			for _, s := range p.Streams {
				fmt.Printf("     PID %#x %s", s.PID, (wittybluray.Stream{Coding: s.Coding}).CodingName())
				if s.Lang != "" {
					fmt.Printf(" [%s]", s.Lang)
				}
				fmt.Println()
			}
		}
	}
	return 0
}

func countChapters(pl *wittybluray.Playlist) int {
	n := 0
	for _, m := range pl.Marks {
		if m.Type == 1 {
			n++
		}
	}
	return n
}

func printPlaylist(d *wittybluray.Disc, pl *wittybluray.Playlist, full bool) {
	for i, it := range pl.Items {
		fmt.Printf("      item %d: clip %s %s..%s (%s)", i, it.Clip, hms(float64(it.In)/45000), hms(float64(it.Out)/45000), hms(it.Duration()))
		if len(it.Angles) > 1 {
			fmt.Printf(" %d angles", len(it.Angles))
		}
		if it.StillMode != 0 {
			fmt.Printf(" still=%d/%ds", it.StillMode, it.StillTime)
		}
		fmt.Printf(" cc=%d", it.ConnectionCondition)
		fmt.Println()
		if !full && i > 0 {
			if i == 1 {
				fmt.Printf("      ... %d more items\n", len(pl.Items)-1)
			}
			if i == 1 {
				break
			}
		}
		for _, grp := range [][]wittybluray.Stream{it.Video, it.Audio, it.PG, it.IG, it.SecAudio, it.SecVideo} {
			for _, s := range grp {
				where := ""
				if s.EntryType != 1 {
					where = fmt.Sprintf(" via sub-path %d", s.SubPath)
				}
				lang := ""
				if s.Lang != "" {
					lang = " [" + s.Lang + "]"
				}
				fmt.Printf("        %-9s PID %#x %s%s%s\n", s.Kind, s.PID, s.CodingName(), lang, where)
			}
		}
	}
	for i, sp := range pl.SubPaths {
		fmt.Printf("      sub-path %d: type %d, %d items", i, sp.Type, len(sp.Items))
		for _, it := range sp.Items {
			fmt.Printf(" [clip %s %s..%s]", it.Clip, hms(float64(it.In)/45000), hms(float64(it.Out)/45000))
		}
		fmt.Println()
	}
	if full {
		for i, m := range pl.Marks {
			fmt.Printf("      mark %d: type %d item %d at %s\n", i, m.Type, m.Item, hms(float64(m.Time)/45000))
		}
	}
}
