package cli

import (
	"fmt"
	"os"

	"github.com/wittywilla/wittybluray"
)

// menu prints a playlist's menu (its IG stream): every page, its buttons
// with their neighbours and pictures, and every command, for working out
// why a disc's menu does something unexpected.
func menu(d *wittybluray.Disc, playlist int) int {
	pl := d.Playlist(playlist)
	if pl == nil || len(pl.Items) == 0 {
		fmt.Fprintln(os.Stderr, "no such playlist")
		return 1
	}
	it := pl.Items[0]
	if len(it.IG) == 0 {
		fmt.Println("the playlist has no menu stream")
		return 0
	}
	ig := it.IG[0]
	clip, limit := it.Clip, uint32(200000)
	if ig.EntryType != 1 && ig.SubPath < len(pl.SubPaths) && len(pl.SubPaths[ig.SubPath].Items) > 0 {
		clip, limit = pl.SubPaths[ig.SubPath].Items[0].Clip, 0
	}
	m, err := d.LoadMenu(clip, ig.PID, limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	kind := "always on"
	if m.Popup {
		kind = "pop-up"
	}
	fmt.Printf("menu of playlist %05d: IG PID %#x in clip %s, %dx%d, %s, %d pages, %d pictures, %d palettes\n", playlist, ig.PID, clip, m.Width, m.Height, kind, len(m.Pages), m.Objects(), m.Palettes())
	if m.Timeout > 0 || m.SelectionTimeout > 0 || m.UserTimeout > 0 {
		fmt.Printf("  timeouts: composition %.1fs, selection %.1fs, user %.1fs\n", m.Timeout, m.SelectionTimeout, m.UserTimeout)
	}
	for pi := range m.Pages {
		p := &m.Pages[pi]
		fmt.Printf("page %d: default selected %d, default activated %d, palette %d, animation rate %d, %d button groups\n", p.ID, p.DefaultSelected, p.DefaultActivated, p.Palette, p.AnimationRate, len(p.BOGs))
		for gi, g := range p.BOGs {
			for _, b := range g.Buttons {
				r := m.Rect(&b)
				fmt.Printf("  group %d button %d at (%d,%d) %dx%d up=%d down=%d left=%d right=%d", gi, b.ID, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), nb(b.Up), nb(b.Down), nb(b.Left), nb(b.Right))
				if b.ID == g.DefaultValid {
					fmt.Print(" [shown]")
				}
				if b.AutoAction {
					fmt.Print(" [auto]")
				}
				if n := b.Selected.Frames(); n > 1 {
					fmt.Printf(" [%d-frame highlight]", n)
				}
				fmt.Println()
				for i, c := range b.Commands {
					fmt.Printf("      %2d: %s\n", i, wittybluray.Decode(c))
				}
			}
		}
	}
	return 0
}

func nb(id int) int {
	if id == wittybluray.NoButton {
		return -1
	}
	return id
}
