package dvdsession

import (
	"bytes"
	"fmt"
	"image"
	"strconv"
	"strings"

	"github.com/wittywilla/wittydvd"
)

// SubEvent is one subtitle image on the title timeline.
type SubEvent struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
	W      int     `json:"w"`
	H      int     `json:"h"`
	Forced bool    `json:"forced,omitempty"`
	URL    string  `json:"url"`
}

type subPic struct {
	ev  SubEvent
	png []byte
}

const maxSubPics = 3000

// Subtitles returns the sub-picture events of logical stream n that overlap
// [from, to] seconds of a title, reading only the cells in that window.
func (s *Session) Subtitles(ttn, n int, from, to float64) ([]SubEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts := s.title(ttn)
	if ts == nil || n < 0 || n > 31 {
		return nil, fmt.Errorf("no such title or stream")
	}
	v := s.opened.Disc.VTS[ts.VTS]
	if v == nil {
		return nil, fmt.Errorf("title set %d missing", ts.VTS)
	}
	sp := wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: ts.VTS}
	out := []SubEvent{}
	for _, c := range ts.Cells {
		cellEnd := c.Start + c.Cell.Duration
		if cellEnd < from || c.Start > to {
			continue
		}
		var pgc *wittydvd.PGC
		if c.PGCN >= 1 && c.PGCN <= len(v.PGCs) {
			pgc = v.PGCs[c.PGCN-1].PGC
		}
		phys := n
		if pgc != nil && pgc.SubpControl[n] != nil {
			sc := pgc.SubpControl[n]
			switch {
			case ts.Video.Wide && s.prefs.Wide:
				phys = sc.Wide
			case ts.Video.Wide:
				phys = sc.Letterbox
			default:
				phys = sc.Narrow
			}
		}
		cs, err := s.opened.CellSubpictures(sp, c.Cell)
		if err != nil {
			return nil, err
		}
		var units []int
		for i, u := range cs.Units {
			if u.Stream == phys && u.PTS >= 0 {
				units = append(units, i)
			}
		}
		for k, i := range units {
			key := fmt.Sprintf("%d-%d-%d", ts.VTS, c.Cell.FirstSector, i)
			pics, ok := s.subPics[key]
			if !ok {
				var next float64 = -1
				if k+1 < len(units) {
					next = c.Start + float64(cs.Units[units[k+1]].PTS-int64(cs.BasePTM))/90000
				}
				pics = s.renderSub(cs.Units[i], c.Start, cs.BasePTM, cellEnd, next, pgc)
				if len(s.subPics) >= maxSubPics {
					s.subPics = map[string][]*subPic{}
				}
				s.subPics[key] = pics
			}
			for f, sp := range pics {
				if sp.png == nil || sp.ev.End < from || sp.ev.Start > to {
					continue
				}
				ev := sp.ev
				ev.URL = s.url("sub", fmt.Sprintf("%s-%d.png", key, f))
				out = append(out, ev)
			}
		}
	}
	return out, nil
}

// renderSub renders every picture a sub-picture unit shows. Most units show
// one; units with several control sequences change colour, blink or move
// (see wittydvd.DecodeSPUFrames). Pictures that draw nothing (units that
// clear the screen) are left out.
func (s *Session) renderSub(u wittydvd.SPUUnit, cellStart float64, base uint32, cellEnd, next float64, pgc *wittydvd.PGC) []*subPic {
	frames, err := wittydvd.DecodeSPUFrames(u.Bytes)
	if err != nil {
		return nil
	}
	var pal [16]uint32
	if pgc != nil {
		pal = pgc.Palette
	}
	at := cellStart + float64(u.PTS-int64(base))/90000
	var out []*subPic
	for _, spu := range frames {
		img := spu.Render(pal, nil).Crop()
		if img.Width == 0 || img.Height == 0 {
			continue
		}
		ev := SubEvent{Start: at + spu.StartSec, X: img.X, Y: img.Y, W: img.Width, H: img.Height, Forced: spu.Forced}
		switch {
		case spu.StopSec > spu.StartSec:
			ev.End = at + spu.StopSec
		case next > ev.Start:
			ev.End = next
		default:
			ev.End = ev.Start + 5
		}
		if ev.End > cellEnd {
			ev.End = cellEnd
		}
		if ev.Start < cellStart {
			ev.Start = cellStart
		}
		nrgba := image.NewNRGBA(image.Rect(0, 0, img.Width, img.Height))
		copy(nrgba.Pix, img.Pix)
		var buf bytes.Buffer
		_ = pngEnc.Encode(&buf, nrgba)
		out = append(out, &subPic{ev: ev, png: buf.Bytes()})
	}
	return out
}

// SubPNG serves a rendered subtitle image ("<unit key>-<picture>.png").
func (s *Session) SubPNG(name string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name = strings.TrimSuffix(name, ".png")
	cut := strings.LastIndexByte(name, '-')
	if cut < 0 {
		return nil, false
	}
	f, err := strconv.Atoi(name[cut+1:])
	pics := s.subPics[name[:cut]]
	if err != nil || f < 0 || f >= len(pics) || pics[f].png == nil {
		return nil, false
	}
	return pics[f].png, true
}
