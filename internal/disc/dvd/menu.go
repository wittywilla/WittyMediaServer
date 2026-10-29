package dvd

import (
	"errors"
)

// SectorReader reads sectors of a VOB address space (menu VOB or the
// concatenated title VOBs of a title set).
type SectorReader interface {
	ReadSectors(first uint32, count int) ([]byte, error)
}

// MenuButton is a button in frame coordinates with its highlight artwork.
type MenuButton struct {
	N       int     `json:"n"`
	X0      int     `json:"x0"`
	Y0      int     `json:"y0"`
	X1      int     `json:"x1"`
	Y1      int     `json:"y1"`
	Up      int     `json:"up"`
	Down    int     `json:"down"`
	Left    int     `json:"left"`
	Right   int     `json:"right"`
	Auto    bool    `json:"auto"`
	Cmd     Command `json:"-"`
	CmdText string  `json:"cmd"`
	Select  *RGBA   `json:"-"`
	Action  *RGBA   `json:"-"`
	Inert   bool    `json:"inert,omitempty"` // no visible highlight and no command
}

// MenuPicture is everything a client needs to draw and operate a menu.
type MenuPicture struct {
	FrameW, FrameH int
	Aspect         float64 // 4/3 or 16/9
	Buttons        []MenuButton
	DefaultButton  int
	Normal         *RGBA // sub-picture as shown with nothing selected
	ButtonsFromSec float64
	HasHighlights  bool
	Scrambled      bool
	NoButtons      bool
}

// MenuScanOptions tune ScanMenu.
type MenuScanOptions struct {
	Palette     [16]uint32
	Video       VideoAttr
	SubpControl *SubpControl // from the PGC, picks the SPU stream per display mode
	Wide        bool         // viewer wants 16:9 presentation
	NavButton   int          // SPRM8 highlight to prefer as default
	MaxHops     int
	ReadAhead   int // sectors to read once buttons are found
}

var ErrNoNav = errors.New("dvd: no NAV pack at cell start")

// ScanMenu finds the highlight information for the cell [first,last] by
// hopping NAV pack to NAV pack, then reads the surrounding VOBUs for the
// sub-picture that carries the button artwork.
func ScanMenu(rd SectorReader, first, last uint32, o MenuScanOptions) (*MenuPicture, error) {
	if o.MaxHops == 0 {
		o.MaxHops = 20000
	}
	if o.ReadAhead == 0 {
		o.ReadAhead = 3072
	}
	pic := &MenuPicture{Aspect: 4.0 / 3.0}
	pic.FrameW, pic.FrameH = o.Video.Size()
	if o.Video.Wide {
		pic.Aspect = 16.0 / 9.0
	}

	var (
		startPTM  uint32
		havePTM   bool
		hliSector uint32
		hliPCI    *PCI
		prevVOBUs []uint32
	)
	sector := first
	for hop := 0; hop < o.MaxHops && sector <= last; hop++ {
		buf, err := rd.ReadSectors(sector, 1)
		if err != nil || len(buf) < Sector {
			return nil, err
		}
		if PackScrambled(buf) {
			pic.Scrambled = true
			return pic, nil
		}
		sc := ScanSectors(buf, sector)
		if len(sc.Navs) == 0 {
			if hop == 0 {
				return nil, ErrNoNav
			}
			break
		}
		nav := sc.Navs[0]
		if nav.PCI != nil && !havePTM {
			startPTM, havePTM = nav.PCI.VOBUStartPTM, true
		}
		if nav.PCI != nil && (nav.PCI.HLStatus == 1 || nav.PCI.HLStatus == 3) && len(nav.PCI.Buttons) > 0 {
			hliSector, hliPCI = sector, nav.PCI
			break
		}
		prevVOBUs = append(prevVOBUs, sector)
		if len(prevVOBUs) > 3 {
			prevVOBUs = prevVOBUs[1:]
		}
		if nav.DSI == nil || nav.DSI.VOBUEnd == 0 {
			break
		}
		sector += nav.DSI.VOBUEnd + 1
	}
	if hliPCI == nil {
		pic.NoButtons = true
		return pic, nil
	}
	if havePTM && hliPCI.HLStartPTM > startPTM {
		pic.ButtonsFromSec = float64(hliPCI.HLStartPTM-startPTM) / 90000
	}

	// Read from a couple of VOBUs before the highlight VOBU so we catch the
	// sub-picture unit that was sent ahead of its display time.
	readFrom := hliSector
	if len(prevVOBUs) > 0 {
		readFrom = prevVOBUs[0]
	}
	count := int(hliSector-readFrom) + o.ReadAhead
	if readFrom+uint32(count)-1 > last {
		count = int(last - readFrom + 1)
	}
	buf, err := rd.ReadSectors(readFrom, count)
	if err != nil {
		return nil, err
	}
	sc := ScanSectors(buf, readFrom)
	if sc.Scrambled {
		pic.Scrambled = true
		return pic, nil
	}

	stream := 0
	if o.SubpControl != nil {
		if o.Wide && o.Video.Wide {
			stream = o.SubpControl.Wide
		} else if o.Video.Wide {
			stream = o.SubpControl.Letterbox
		} else {
			stream = o.SubpControl.Narrow
		}
	}
	var spu *SPU
	var best *SPUUnit
	for i := range sc.SPUs {
		u := &sc.SPUs[i]
		if u.Stream != stream {
			continue
		}
		// Prefer the latest unit whose PTS is at or before the highlight start.
		if u.PTS >= 0 && uint32(u.PTS) <= hliPCI.HLStartPTM {
			best = u
		} else if best == nil {
			best = u
		}
	}
	if best == nil && len(sc.SPUs) > 0 {
		best = &sc.SPUs[0]
	}
	if best != nil {
		if d, err := DecodeSPU(best.Bytes); err == nil {
			spu = d
		}
	}

	pic.DefaultButton = hliPCI.ForcedSelect
	if o.NavButton >= 1 && o.NavButton <= len(hliPCI.Buttons) {
		pic.DefaultButton = o.NavButton
	}
	if pic.DefaultButton == 0 && len(hliPCI.Buttons) > 0 {
		pic.DefaultButton = hliPCI.Buttons[0].N
	}
	// Button group for the viewer's display mode.
	group := 1
	if len(hliPCI.Groups) > 1 {
		want := "normal"
		if o.Video.Wide && !o.Wide {
			want = "letterbox"
		}
		for _, g := range hliPCI.Groups {
			if g.Mode == want {
				group = g.N
			}
		}
	}
	for _, b := range hliPCI.Buttons {
		if b.Group != group {
			continue
		}
		mb := MenuButton{
			N: b.N, X0: b.X0, Y0: b.Y0, X1: b.X1, Y1: b.Y1,
			Up: b.Up, Down: b.Down, Left: b.Left, Right: b.Right, Auto: b.Auto, Cmd: b.Cmd,
			CmdText: Decode(b.Cmd).String(),
		}
		if spu != nil {
			sel := b.Select
			mb.Select = spu.RenderRect(o.Palette, &sel, b.X0, b.Y0, b.X1, b.Y1)
			act := b.Action
			mb.Action = spu.RenderRect(o.Palette, &act, b.X0, b.Y0, b.X1, b.Y1)
			if mb.Select.Opaque() {
				pic.HasHighlights = true
			}
		}
		empty := Decode(b.Cmd)
		if !mb.Select.Opaque() && (empty.Invalid != "" || empty.String() == "NOP") {
			mb.Inert = true
		}
		pic.Buttons = append(pic.Buttons, mb)
	}
	if spu != nil {
		normal := spu.Render(o.Palette, nil)
		if normal.Opaque() {
			pic.Normal = normal
		}
	}
	return pic, nil
}
