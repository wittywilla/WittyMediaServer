package dvdsession

import (
	"bytes"
	"fmt"
	"image"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/wittydvd"
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
	ev  SubEvent // URL empty when the unit draws nothing (a "clear" unit)
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
			sp, ok := s.subPics[key]
			if !ok {
				var next float64 = -1
				if k+1 < len(units) {
					next = c.Start + float64(cs.Units[units[k+1]].PTS-int64(cs.BasePTM))/90000
				}
				sp = s.renderSub(cs.Units[i], c.Start, cs.BasePTM, cellEnd, next, pgc)
				if len(s.subPics) >= maxSubPics {
					s.subPics = map[string]*subPic{}
				}
				s.subPics[key] = sp
			}
			if sp.ev.URL == "" && sp.png == nil {
				continue
			}
			if sp.ev.End < from || sp.ev.Start > to {
				continue
			}
			ev := sp.ev
			ev.URL = s.url("sub", key+".png")
			out = append(out, ev)
		}
	}
	return out, nil
}

func (s *Session) renderSub(u wittydvd.SPUUnit, cellStart float64, base uint32, cellEnd, next float64, pgc *wittydvd.PGC) *subPic {
	spu, err := wittydvd.DecodeSPU(u.Bytes)
	if err != nil {
		return &subPic{}
	}
	var pal [16]uint32
	if pgc != nil {
		pal = pgc.Palette
	}
	img := spu.Render(pal, nil).Crop()
	if img.Width == 0 || img.Height == 0 {
		return &subPic{} // clears the screen; nothing to draw
	}
	at := cellStart + float64(u.PTS-int64(base))/90000
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
	return &subPic{ev: ev, png: buf.Bytes()}
}

// SubPNG serves a rendered subtitle image.
func (s *Session) SubPNG(name string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sp, ok := s.subPics[strings.TrimSuffix(name, ".png")]
	if !ok || sp.png == nil {
		return nil, false
	}
	return sp.png, true
}
