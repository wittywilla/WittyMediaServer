package cli

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittydvd"
)

// DVD runs the dvd subcommands: info, walk, crawl, pgc, render.
func DVD(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: dvd info|walk|crawl|pgc|render <image> [args]")
		return 2
	}
	o, err := wittydvd.Open(args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer o.Close()
	switch args[0] {
	case "info":
		return dvdInfo(o)
	case "walk":
		return dvdWalk(o, args[2:])
	case "crawl":
		limit := 300
		if len(args) > 2 {
			fmt.Sscan(args[2], &limit)
		}
		return dvdCrawl(o, limit)
	case "render":
		if len(args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: dvd render <image> <out-dir>")
			return 2
		}
		return dvdRender(o, args[2])
	case "pgc":
		if len(args) < 5 {
			fmt.Fprintln(os.Stderr, "usage: dvd pgc <image> fp|vmgm|vtsm|vts <vts> <pgcn>")
			return 2
		}
		var vts, pgcn int
		fmt.Sscan(args[3], &vts)
		fmt.Sscan(args[4], &pgcn)
		return dvdDumpPGC(o, args[2], vts, pgcn)
	}
	fmt.Fprintln(os.Stderr, "unknown dvd subcommand", args[0])
	return 2
}

func hms(sec float64) string {
	d := time.Duration(sec * float64(time.Second))
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

func dvdInfo(o *wittydvd.Opened) int {
	d := o.Disc
	fmt.Printf("label=%q format=%s provider=%q region-mask=%08b titlesets=%d\n", o.Label, o.Format, d.VMG.Provider, d.VMG.RegionMask, d.VMG.NumTitleSets)
	if d.VMG.FirstPlay != nil {
		fmt.Printf("first-play: %d pre / %d cells / %d post commands\n", len(d.VMG.FirstPlay.Pre), len(d.VMG.FirstPlay.Cells), len(d.VMG.FirstPlay.Post))
		for i, c := range d.VMG.FirstPlay.Pre {
			fmt.Printf("   pre %2d: %s\n", i+1, wittydvd.Decode(c))
		}
	}
	fmt.Printf("VMGM menus: %d language units\n", len(d.VMG.Menus))
	for _, lu := range d.VMG.Menus {
		for i, e := range lu.PGCs {
			fmt.Printf("   [%s] pgc %d entry=%v type=%d cells=%d dur=%s pre=%d post=%d\n", lu.Language, i+1, e.Entry, e.MenuType, len(e.PGC.Cells), hms(e.PGC.Duration), len(e.PGC.Pre), len(e.PGC.Post))
		}
	}
	for _, t := range d.Titles() {
		fmt.Printf("title %2d: vts %d/%d  %s  chapters=%d angles=%d video=%s%s audio=%d subs=%d cells=%d\n",
			t.Number, t.VTS, t.VTSTitle, hms(t.Duration), len(t.Chapters), t.Angles, t.Video.Standard, map[bool]string{true: " 16:9", false: " 4:3"}[t.Video.Wide], len(t.Audio), len(t.Subp), len(t.Cells))
	}
	for n := 1; n <= 99; n++ {
		v := d.VTS[n]
		if v == nil {
			continue
		}
		fmt.Printf("VTS %02d: %d title pgcs, %d menu LUs, audio:", n, len(v.PGCs), len(v.Menus))
		for _, a := range v.Audio {
			fmt.Printf(" %s/%s/%dch", a.Language, a.Codec, a.Channels)
		}
		fmt.Printf(" subs:")
		for _, s := range v.Subp {
			fmt.Printf(" %s", s.Language)
		}
		fmt.Println()
		for _, lu := range v.Menus {
			for i, e := range lu.PGCs {
				fmt.Printf("   menu [%s] pgc %d entry=%v type=%d cells=%d dur=%s\n", lu.Language, i+1, e.Entry, e.MenuType, len(e.PGC.Cells), hms(e.PGC.Duration))
			}
		}
	}
	if m := d.MainTitle(); m != nil {
		fmt.Printf("main title: %d (%s)\n", m.Number, hms(m.Duration))
	}
	return 0
}

func describeStep(s *wittydvd.Step) string {
	still := fmt.Sprint(s.StillSec)
	if math.IsInf(s.StillSec, 1) {
		still = "inf"
	}
	out := fmt.Sprintf("%s %s vts=%d pgc=%d cell=%d sectors=%d..%d still=%s loop=%v btn=%d dur=%.1fs", s.Kind, s.DomainName, s.VTS, s.PGCN, s.CellN, s.FirstSector, s.LastSector, still, s.Loop, s.DefaultButton, s.CellDuration)
	if s.Reason != "" {
		out += " reason=" + s.Reason
	}
	if s.Title != nil {
		out += fmt.Sprintf(" title=%d chapter=%d", s.Title.TTN, s.Title.PTTN)
	}
	return out
}

func menuFor(o *wittydvd.Opened, nav *wittydvd.Nav, s *wittydvd.Step) (*wittydvd.MenuPicture, error) {
	sp := wittydvd.SpaceFor(s.Domain, s.VTS)
	rd, err := o.Reader(sp)
	if err != nil {
		return nil, err
	}
	var video wittydvd.VideoAttr
	var subc *wittydvd.SubpControl
	pgc := pgcOfStep(o, nav, s)
	switch s.Domain {
	case wittydvd.DomainVTSM:
		video = o.Disc.VTS[s.VTS].MenuVideo
	case wittydvd.DomainVTS:
		video = o.Disc.VTS[s.VTS].Video
	default:
		video = o.Disc.VMG.MenuVideo
	}
	if pgc != nil {
		subc = pgc.SubpControl[0]
	}
	return wittydvd.ScanMenu(rd, s.FirstSector, s.LastSector, wittydvd.MenuScanOptions{Palette: s.Palette, Video: video, SubpControl: subc, NavButton: s.DefaultButton})
}

func pgcOfStep(o *wittydvd.Opened, nav *wittydvd.Nav, s *wittydvd.Step) *wittydvd.PGC {
	lang := nav.Registers().SPRM[wittydvd.SprmMenuLang]
	pick := func(units []wittydvd.LangUnit) []wittydvd.PGCEntry {
		if len(units) == 0 {
			return nil
		}
		for _, u := range units {
			if u.Language != "" && uint16(u.Language[0])<<8|uint16(u.Language[1]) == lang {
				return u.PGCs
			}
		}
		return units[0].PGCs
	}
	var list []wittydvd.PGCEntry
	switch s.Domain {
	case wittydvd.DomainFP:
		return o.Disc.VMG.FirstPlay
	case wittydvd.DomainVMGM:
		list = pick(o.Disc.VMG.Menus)
	case wittydvd.DomainVTSM:
		list = pick(o.Disc.VTS[s.VTS].Menus)
	case wittydvd.DomainVTS:
		list = o.Disc.VTS[s.VTS].PGCs
	}
	if s.PGCN >= 1 && s.PGCN <= len(list) {
		return list[s.PGCN-1].PGC
	}
	return nil
}

// dvdWalk runs first play, auto-advancing through cells until a menu with
// buttons (or a title) appears, then optionally presses buttons given on the
// command line, e.g. "walk SHREK.ISO 1 2".
func dvdWalk(o *wittydvd.Opened, presses []string) int {
	nav := wittydvd.NewNav(o.Disc, wittydvd.NavOptions{})
	s := nav.Start()
	for hop := 0; hop < 50; hop++ {
		fmt.Println("step:", describeStep(s))
		if s.Kind == wittydvd.StepStop {
			return 0
		}
		pic, err := menuFor(o, nav, s)
		if err != nil {
			fmt.Println("   menu scan:", err)
		} else if pic.Scrambled {
			fmt.Println("   CSS-scrambled: cannot play")
			return 1
		} else if len(pic.Buttons) > 0 {
			fmt.Printf("   %dx%d aspect=%.2f buttons=%d default=%d highlights=%v buttonsFrom=%.1fs overlay=%v\n", pic.FrameW, pic.FrameH, pic.Aspect, len(pic.Buttons), pic.DefaultButton, pic.HasHighlights, pic.ButtonsFromSec, pic.Normal != nil)
			for _, b := range pic.Buttons {
				fmt.Printf("     btn %2d (%3d,%3d)-(%3d,%3d) u%d d%d l%d r%d auto=%v lit=%v  %s\n", b.N, b.X0, b.Y0, b.X1, b.Y1, b.Up, b.Down, b.Left, b.Right, b.Auto, b.Select.Opaque(), b.CmdText)
			}
			if len(presses) == 0 {
				return 0
			}
			var n int
			fmt.Sscan(presses[0], &n)
			presses = presses[1:]
			var cmd *wittydvd.Command
			for _, b := range pic.Buttons {
				if b.N == n {
					c := b.Cmd
					cmd = &c
				}
			}
			fmt.Printf("press button %d\n", n)
			s = nav.PressButton(n, cmd)
			continue
		}
		if s.Kind == wittydvd.StepTitle {
			if s.Title != nil && s.Title.Duration > 120 {
				fmt.Println("   playing the feature; stopping walk")
				return 0
			}
			fmt.Println("   short title: watching it through")
			// Let every remaining cell of this title finish.
			for i := 0; i < 64 && s.Kind == wittydvd.StepTitle; i++ {
				s = nav.CellEnded()
				if s.Kind != wittydvd.StepTitle {
					break
				}
			}
			continue
		}
		if math.IsInf(s.StillSec, 1) {
			fmt.Println("   infinite still without buttons; stopping walk")
			return 0
		}
		s = nav.CellEnded()
	}
	return 0
}

func writePNG(path string, r *wittydvd.RGBA, w, h int) error {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	if r != nil {
		for y := 0; y < r.Height; y++ {
			for x := 0; x < r.Width; x++ {
				i := (y*r.Width + x) * 4
				img.SetNRGBA(r.X+x, r.Y+y, color.NRGBA{r.Pix[i], r.Pix[i+1], r.Pix[i+2], r.Pix[i+3]})
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func dvdRender(o *wittydvd.Opened, outDir string) int {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	nav := wittydvd.NewNav(o.Disc, wittydvd.NavOptions{})
	s := nav.Start()
	for hop := 0; hop < 50 && s.Kind != wittydvd.StepStop; hop++ {
		pic, err := menuFor(o, nav, s)
		if err == nil && len(pic.Buttons) > 0 {
			name := fmt.Sprintf("%s-vts%d-pgc%d-cell%d", s.DomainName, s.VTS, s.PGCN, s.CellN)
			if pic.Normal != nil {
				_ = writePNG(filepath.Join(outDir, name+"-normal.png"), pic.Normal, pic.FrameW, pic.FrameH)
			}
			for _, b := range pic.Buttons {
				if b.Select.Opaque() {
					_ = writePNG(filepath.Join(outDir, fmt.Sprintf("%s-btn%02d.png", name, b.N)), b.Select, pic.FrameW, pic.FrameH)
				}
			}
			fmt.Println("wrote", name, "with", len(pic.Buttons), "buttons to", outDir)
			// Also dump the first 2 MB of the cell as raw MPEG-PS so a frame can be grabbed with ffmpeg.
			if rd, err := o.Reader(wittydvd.SpaceFor(s.Domain, s.VTS)); err == nil {
				n := int(s.LastSector-s.FirstSector) + 1
				if n > 1024 {
					n = 1024
				}
				if buf, err := rd.ReadSectors(s.FirstSector, n); err == nil {
					_ = os.WriteFile(filepath.Join(outDir, name+".vob"), buf, 0o644)
				}
			}
			return 0
		}
		if s.Kind == wittydvd.StepTitle {
			for i := 0; i < 64 && s.Kind == wittydvd.StepTitle; i++ {
				s = nav.CellEnded()
			}
			continue
		}
		if math.IsInf(s.StillSec, 1) {
			break
		}
		s = nav.CellEnded()
	}
	fmt.Println("no menu with buttons reached:", strings.TrimSpace(describeStep(s)))
	return 1
}
