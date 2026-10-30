package wittybluray

import (
	"errors"
	"fmt"
	"image"
)

// Interactive graphics: a Blu-ray menu is an IG stream of pages, each a set
// of buttons with pictures for their normal, selected and activated states,
// the buttons they lead to with the arrow keys, and the commands they run.
// Menu loads a stream from a clip; the player draws a page and runs a
// button's commands through the navigator.

// Menu is one IG stream.
type Menu struct {
	Width, Height int
	// Popup menus (UIModel 1) stay hidden until the user's Popup key; always-on
	// menus (0) show at once.
	Popup bool
	// Timeouts, in seconds (0 = none): the whole composition, and how long a
	// selection waits before the default activated button runs.
	Timeout, SelectionTimeout, UserTimeout float64
	Pages                                  []Page
	palettes                               map[int]Palette
	objects                                map[int]*Object
}

// Page is one menu page.
type Page struct {
	ID               int
	UOMask           uint64
	DefaultSelected  int // button id, or NoButton
	DefaultActivated int // a button that runs when the selection times out, or NoButton
	Palette          int
	BOGs             []BOG
}

// NoButton means "none" in button references.
const NoButton = 0xffff

// BOG is a button overlap group: buttons that share a place on screen, of
// which one (the valid one) is shown at a time.
type BOG struct {
	DefaultValid int
	Buttons      []Button
}

// Button is one menu button.
type Button struct {
	ID             int
	NumericSelect  int // the number key that selects it, or NoButton
	AutoAction     bool
	X, Y           int
	Up, Down       int // neighbours by button id, NoButton when none
	Left, Right    int
	Normal         StateImages
	Selected       StateImages
	Activated      StateImages
	SelectedSound  int
	ActivatedSound int
	Commands       []Command
}

// StateImages is the range of pictures of one button state (an animation
// from Start to End; the player shows the first).
type StateImages struct {
	Start, End int // object ids, NoButton when none
	Repeat     bool
}

// ButtonState picks which picture of a button to draw.
type ButtonState int

const (
	ButtonNormal ButtonState = iota
	ButtonSelected
	ButtonActivated
)

// LoadMenu reads the IG stream with the PID from a clip. For in-mux menus
// (the IG stream inside the title's clip) `limit` packets are scanned (0 =
// the whole clip); menu clips are small and read whole.
func (d *Disc) LoadMenu(clip string, pid uint16, limit uint32) (*Menu, error) {
	c := d.Clip(clip)
	if c == nil {
		return nil, fmt.Errorf("clip %s: no clip information", clip)
	}
	m := &Menu{palettes: map[int]Palette{}, objects: map[int]*Object{}}
	ods := newODSAssembler()
	var ics []byte
	var icsDone bool
	hd := true
	var segs []segment
	err := d.ReadPID(clip, pid, 0, limit, func(payload []byte, pts uint64) bool {
		segs = splitSegments(payload, pts, segs[:0])
		for _, s := range segs {
			switch s.typ {
			case segPDS:
				id, p := parsePDS(s.data, hd)
				m.palettes[id] = p
			case segODS:
				ods.feed(s.data)
			case segICS:
				// 5 bytes of video descriptor, 3 of composition descriptor,
				// 1 sequence descriptor, then the interactive composition,
				// which may continue in further ICS segments.
				if icsDone || len(s.data) < 9 {
					continue
				}
				m.Width = int(s.data[0])<<8 | int(s.data[1])
				m.Height = int(s.data[2])<<8 | int(s.data[3])
				hd = m.Height >= 720
				flags := s.data[8]
				if flags&0x80 != 0 {
					ics = ics[:0]
				}
				ics = append(ics, s.data[9:]...)
				if flags&0x40 != 0 {
					icsDone = true
				}
			case segEnd:
				// The first display set (composition, palettes, pictures) is
				// a whole menu; later sets repeat it for players joining late.
				if icsDone {
					return false
				}
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if !icsDone {
		return nil, errors.New("no interactive composition in the stream")
	}
	m.objects = ods.complete
	if err := m.parseIC(ics); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Menu) parseIC(b []byte) error {
	r := &rd{b: b}
	o := 3 // skip the length
	flags := r.u8(o)
	o++
	m.Popup = flags&0x40 != 0
	if flags&0x80 == 0 { // in-mux: timeouts in 90 kHz
		m.Timeout = float64(r.u64(o)&0x1ffffffff) / 90000
		m.SelectionTimeout = float64(r.u64(o+5)&0x1ffffffff) / 90000
		o += 10
	}
	m.UserTimeout = float64(uint32(r.u8(o))<<16|uint32(r.u8(o+1))<<8|uint32(r.u8(o+2))) / 90000
	o += 3
	n := int(r.u8(o))
	o++
	for i := 0; i < n && r.err == nil; i++ {
		p := Page{ID: int(r.u8(o)), UOMask: r.u64(o + 2)}
		o += 10
		o = skipEffects(r, o) // in effects
		o = skipEffects(r, o) // out effects
		o++                   // animation frame rate code
		p.DefaultSelected = int(r.u16(o))
		p.DefaultActivated = int(r.u16(o + 2))
		p.Palette = int(r.u8(o + 4))
		nb := int(r.u8(o + 5))
		o += 6
		for j := 0; j < nb && r.err == nil; j++ {
			g := BOG{DefaultValid: int(r.u16(o))}
			nbt := int(r.u8(o + 2))
			o += 3
			for k := 0; k < nbt && r.err == nil; k++ {
				bt := Button{ID: int(r.u16(o)), NumericSelect: int(r.u16(o + 2)), AutoAction: r.u8(o+4)&0x80 != 0, X: int(r.u16(o + 5)), Y: int(r.u16(o + 7)),
					Up: int(r.u16(o + 9)), Down: int(r.u16(o + 11)), Left: int(r.u16(o + 13)), Right: int(r.u16(o + 15))}
				bt.Normal = StateImages{Start: int(r.u16(o + 17)), End: int(r.u16(o + 19)), Repeat: r.u8(o+21)&0x80 != 0}
				bt.SelectedSound = int(r.u8(o + 22))
				bt.Selected = StateImages{Start: int(r.u16(o + 23)), End: int(r.u16(o + 25)), Repeat: r.u8(o+27)&0x80 != 0}
				bt.ActivatedSound = int(r.u8(o + 28))
				bt.Activated = StateImages{Start: int(r.u16(o + 29)), End: int(r.u16(o + 31))}
				nc := int(r.u16(o + 33))
				o += 35
				if nc > 10000 {
					return fmt.Errorf("button %d has %d commands", bt.ID, nc)
				}
				for c := 0; c < nc && r.err == nil; c++ {
					bt.Commands = append(bt.Commands, Command{Op: r.u32(o), Dst: r.u32(o + 4), Src: r.u32(o + 8)})
					o += 12
				}
				g.Buttons = append(g.Buttons, bt)
			}
			p.BOGs = append(p.BOGs, g)
		}
		m.Pages = append(m.Pages, p)
	}
	if r.err != nil {
		return fmt.Errorf("interactive composition: %w", r.err)
	}
	return nil
}

// skipEffects steps over an effect sequence (page transitions, which the
// player does not animate).
func skipEffects(r *rd, o int) int {
	nw := int(r.u8(o))
	o += 1 + nw*9
	ne := int(r.u8(o))
	o++
	for i := 0; i < ne && r.err == nil; i++ {
		no := int(r.u8(o + 4))
		o += 5
		for j := 0; j < no && r.err == nil; j++ {
			if r.u8(o+4)&0x80 != 0 {
				o += 16
			} else {
				o += 8
			}
		}
	}
	return o
}

// Page returns a page by id.
func (m *Menu) Page(id int) *Page {
	for i := range m.Pages {
		if m.Pages[i].ID == id {
			return &m.Pages[i]
		}
	}
	return nil
}

// Button finds a button on a page.
func (p *Page) Button(id int) *Button {
	for i := range p.BOGs {
		for j := range p.BOGs[i].Buttons {
			if p.BOGs[i].Buttons[j].ID == id {
				return &p.BOGs[i].Buttons[j]
			}
		}
	}
	return nil
}

// Visible lists the buttons shown on the page: each group's valid button,
// as the group defaults say, overridden by `valid` (button ids enabled by
// ENABLE_BUTTON) and `hidden` (disabled ones).
func (p *Page) Visible(enabled, disabled map[int]bool) []*Button {
	var out []*Button
	for i := range p.BOGs {
		g := &p.BOGs[i]
		pick := -1
		for j := range g.Buttons {
			b := &g.Buttons[j]
			if enabled[b.ID] && !disabled[b.ID] {
				pick = j
				break
			}
		}
		if pick < 0 {
			for j := range g.Buttons {
				if g.Buttons[j].ID == g.DefaultValid && !disabled[g.Buttons[j].ID] {
					pick = j
				}
			}
		}
		if pick >= 0 {
			out = append(out, &g.Buttons[pick])
		}
	}
	return out
}

// Rect is a button's place on screen, from its picture.
func (m *Menu) Rect(b *Button) image.Rectangle {
	for _, st := range []StateImages{b.Normal, b.Selected, b.Activated} {
		if o := m.objects[st.Start]; o != nil && st.Start != NoButton {
			return image.Rect(b.X, b.Y, b.X+o.Width, b.Y+o.Height)
		}
	}
	return image.Rect(b.X, b.Y, b.X, b.Y)
}

// Draw paints one button in a state into img (the menu's full frame).
func (m *Menu) Draw(img *image.RGBA, page *Page, b *Button, state ButtonState) bool {
	st := b.Normal
	switch state {
	case ButtonSelected:
		if b.Selected.Start != NoButton {
			st = b.Selected
		}
	case ButtonActivated:
		if b.Activated.Start != NoButton {
			st = b.Activated
		} else if b.Selected.Start != NoButton {
			st = b.Selected
		}
	}
	o := m.objects[st.Start]
	if o == nil || st.Start == NoButton {
		return false
	}
	p, ok := m.palettes[page.Palette]
	if !ok {
		return false
	}
	draw(img, o, &p, b.X, b.Y, 0, 0, 0, 0)
	return true
}

// Render draws a whole page with one button selected. Buttons are those
// Visible returns; the result is transparent where there are no buttons.
func (m *Menu) Render(page *Page, buttons []*Button, selected int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, m.Width, m.Height))
	for _, b := range buttons {
		state := ButtonNormal
		if b.ID == selected {
			state = ButtonSelected
		}
		m.Draw(img, page, b, state)
	}
	return img
}

// Objects and Palettes count what the stream carried (for diagnostics).
func (m *Menu) Objects() int  { return len(m.objects) }
func (m *Menu) Palettes() int { return len(m.palettes) }
