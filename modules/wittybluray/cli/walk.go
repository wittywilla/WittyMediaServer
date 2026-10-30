package cli

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/wittywilla/wittybluray"
)

// walk simulates inserting the disc and, optionally, pressing menu buttons
// by number; it prints every step the navigator produces. render writes
// the first menu's pages as PNG files.

func describe(st *wittybluray.Step) string {
	switch st.Kind {
	case wittybluray.StepPlay:
		s := fmt.Sprintf("play playlist %05d from item %d", st.Playlist.Number, st.Item)
		if st.Time > 0 {
			s += fmt.Sprintf(" at %s", hms(float64(st.Time)/45000))
		}
		if st.Menu {
			s += fmt.Sprintf(" with menu (IG PID %#x", st.IGPID)
			if st.IGClip != "" {
				s += " in clip " + st.IGClip
			}
			s += ")"
		}
		if st.Title == 0xffff {
			s += " [first play]"
		} else {
			s += fmt.Sprintf(" [title %d]", st.Title)
		}
		return s
	case wittybluray.StepStay:
		return "stay"
	}
	return string(st.Kind) + ": " + st.Reason
}

// menuOf loads the menu of a play step, if it has one.
func menuOf(d *wittybluray.Disc, st *wittybluray.Step) (*wittybluray.Menu, error) {
	if !st.Menu {
		return nil, nil
	}
	clip := st.IGClip
	limit := uint32(0)
	if clip == "" {
		clip = st.Playlist.Items[st.Item].Clip
		limit = 200000 // in-mux menus come at the start of the clip
	}
	return d.LoadMenu(clip, st.IGPID, limit)
}

func walk(d *wittybluray.Disc, presses []int) int {
	nav := wittybluray.NewNav(d, wittybluray.NavOptions{})
	st := nav.Start()
	for hop := 0; hop < 40; hop++ {
		fmt.Printf("%2d: %s\n", hop, describe(st))
		if st.Kind != wittybluray.StepPlay {
			return 0
		}
		menu, err := menuOf(d, st)
		if err != nil {
			fmt.Printf("    menu: %v\n", err)
		}
		if menu == nil {
			st = nav.PlaylistEnded()
			continue
		}
		page := &menu.Pages[0]
		buttons := page.Visible(nil, nil)
		fmt.Printf("    menu: %dx%d, %d pages, %d pictures; page %d has %d buttons, default %d\n", menu.Width, menu.Height, len(menu.Pages), menu.Objects(), page.ID, len(buttons), page.DefaultSelected)
		for _, b := range buttons {
			r := menu.Rect(b)
			fmt.Printf("      button %d at (%d,%d) %dx%d: %d commands", b.ID, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), len(b.Commands))
			for i, c := range b.Commands {
				if i < 4 {
					fmt.Printf("  %s;", wittybluray.Decode(c))
				}
			}
			fmt.Println()
		}
		if len(presses) == 0 {
			return 0
		}
		want := presses[0]
		presses = presses[1:]
		var pressed *wittybluray.Button
		for _, b := range buttons {
			if b.ID == want {
				pressed = b
			}
		}
		if pressed == nil {
			fmt.Printf("    no button %d on this page\n", want)
			return 1
		}
		fmt.Printf("    press button %d\n", want)
		st = nav.PressButton(pressed.Commands)
		if st.Kind == wittybluray.StepStay {
			if fx := st.Effects; fx != nil && fx.ButtonPage != nil {
				fmt.Printf("    -> page %d button %d\n", fx.ButtonPage.Page, fx.ButtonPage.Button)
			}
			return 0
		}
	}
	return 0
}

func render(d *wittybluray.Disc, outDir string) int {
	nav := wittybluray.NewNav(d, wittybluray.NavOptions{})
	st := nav.Start()
	for hop := 0; hop < 20 && st.Kind == wittybluray.StepPlay && !st.Menu; hop++ {
		st = nav.PlaylistEnded()
	}
	if st.Kind != wittybluray.StepPlay || !st.Menu {
		fmt.Fprintln(os.Stderr, "the disc never reaches a menu:", describe(st))
		return 1
	}
	menu, err := menuOf(d, st)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	for i := range menu.Pages {
		page := &menu.Pages[i]
		buttons := page.Visible(nil, nil)
		for _, sel := range []int{page.DefaultSelected, wittybluray.NoButton} {
			img := menu.Render(page, buttons, sel)
			name := fmt.Sprintf("page%d-normal.png", page.ID)
			if sel != wittybluray.NoButton {
				name = fmt.Sprintf("page%d-selected%d.png", page.ID, sel)
			}
			f, err := os.Create(filepath.Join(outDir, name))
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			png.Encode(f, img)
			f.Close()
			fmt.Println("wrote", name)
		}
	}
	return 0
}
