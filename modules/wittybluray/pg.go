package wittybluray

import (
	"fmt"
	"image"
)

// Presentation graphics: Blu-ray subtitles are pictures. A display set
// (PCS, windows, palette, objects, END) shows one or more pictures at
// positions on screen from its time until the next set, and a set with no
// pictures clears them. Subtitles reads a window of a clip and returns the
// pictures with their times, for a player to lay over the video.

// Subtitle is one subtitle picture and when it shows.
type Subtitle struct {
	Start, End uint32 // 45 kHz, in the clip's time base; End 0 = until the next
	X, Y       int    // on the video frame
	Image      *image.NRGBA
	Forced     bool // the disc wants it shown even with subtitles off (a translation of on-screen text)
}

// SubtitleWindow is what Subtitles returns: the pictures and the frame size
// they are placed on.
type SubtitleWindow struct {
	Width, Height int
	Subtitles     []Subtitle
}

// Subtitles decodes the PG stream `pid` of a clip between two times (45
// kHz; to = 0 means the end). The clip is read from the entry point at or
// before `from`, so a long movie is never scanned whole.
func (d *Disc) Subtitles(clip string, pid uint16, from, to uint32) (*SubtitleWindow, error) {
	c := d.Clip(clip)
	if c == nil {
		return nil, fmt.Errorf("clip %s: no clip information", clip)
	}
	start := uint32(0)
	if e, ok := c.SPNAt(c.VideoPID(), from); ok {
		start = e.SPN
	}
	w := &SubtitleWindow{}
	palettes := map[int]Palette{}
	ods := newODSAssembler()
	var windows []Window
	type pending struct {
		pts     uint32
		objects []compObject
		palette int
	}
	var cur *pending
	hd := true
	var segs []segment
	var last *Subtitle
	limit := to
	if limit > 0 {
		limit += 45000 * 30 // read on past the end so the last subtitle's end is known
	}
	err := d.ReadPID(clip, pid, start, 0, func(payload []byte, pts90 uint64) bool {
		pts := uint32(pts90 / 2)
		if limit > 0 && pts > limit && cur == nil {
			return false
		}
		segs = splitSegments(payload, pts90, segs[:0])
		for _, s := range segs {
			switch s.typ {
			case segPCS:
				if len(s.data) < 11 {
					continue
				}
				w.Width = int(s.data[0])<<8 | int(s.data[1])
				w.Height = int(s.data[2])<<8 | int(s.data[3])
				hd = w.Height >= 720
				p := &pending{pts: pts, palette: int(s.data[9])}
				n := int(s.data[10])
				for i, off := 0, 11; i < n && off < len(s.data); i++ {
					o, used := parseCompObject(s.data[off:])
					p.objects = append(p.objects, o)
					off += used
				}
				cur = p
			case segWDS:
				windows = parseWindows(s.data)
			case segPDS:
				id, p := parsePDS(s.data, hd)
				palettes[id] = p
			case segODS:
				ods.feed(s.data)
			case segEnd:
				if cur == nil {
					continue
				}
				if last != nil && last.End == 0 {
					last.End = cur.pts
				}
				if len(cur.objects) > 0 && cur.pts >= from {
					if sub := compose(cur.objects, windows, palettes[cur.palette], ods.complete, w.Width, w.Height); sub != nil {
						sub.Start = cur.pts
						w.Subtitles = append(w.Subtitles, *sub)
						last = &w.Subtitles[len(w.Subtitles)-1]
					}
				} else if len(cur.objects) == 0 {
					last = nil
				}
				done := to > 0 && cur.pts > to
				cur = nil
				if done {
					return false
				}
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	return w, nil
}

// compose draws a display set's objects into one picture covering them.
func compose(objs []compObject, windows []Window, p Palette, pics map[int]*Object, fw, fh int) *Subtitle {
	bounds := image.Rectangle{}
	forced := false
	for _, o := range objs {
		pic := pics[o.object]
		if pic == nil {
			continue
		}
		w, h := pic.Width, pic.Height
		if o.cw > 0 {
			w, h = o.cw, o.ch
		}
		r := image.Rect(o.x, o.y, o.x+w, o.y+h)
		if bounds.Empty() {
			bounds = r
		} else {
			bounds = bounds.Union(r)
		}
		forced = forced || o.forced
	}
	if bounds.Empty() {
		return nil
	}
	if fw > 0 && fh > 0 {
		bounds = bounds.Intersect(image.Rect(0, 0, fw, fh))
		if bounds.Empty() {
			return nil
		}
	}
	img := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for _, o := range objs {
		pic := pics[o.object]
		if pic == nil {
			continue
		}
		draw(img, pic, &p, o.x-bounds.Min.X, o.y-bounds.Min.Y, o.cx, o.cy, o.cw, o.ch)
	}
	return &Subtitle{X: bounds.Min.X, Y: bounds.Min.Y, Image: img, Forced: forced}
}
