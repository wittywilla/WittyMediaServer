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
	return d.SubtitlesIn(clip, pid, -1, from, to)
}

// SubtitlesIn is Subtitles within one time base of the clip (-1 = all). The
// read starts a little before `from`, since a picture is stored ahead of the
// time it shows.
func (d *Disc) SubtitlesIn(clip string, pid uint16, stc int, from, to uint32) (*SubtitleWindow, error) {
	c := d.Clip(clip)
	if c == nil {
		return nil, fmt.Errorf("clip %s: no clip information", clip)
	}
	start, stop := uint32(0), uint32(0)
	lead := from
	if lead > 45000*5 {
		lead -= 45000 * 5
	} else {
		lead = 0
	}
	if e, ok := c.EntryAt(c.VideoPID(), stc, lead); ok {
		start = e.SPN
	}
	if f, t, ok := c.STCRange(stc); ok {
		if start < f {
			start = f
		}
		stop = t
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
	matrix := matrix709
	var segs []segment
	var last *Subtitle
	openFrom := 0
	limit := to
	if limit > 0 {
		limit += 45000 * 30 // read on past the end so the last subtitle's end is known
	}
	err := d.ReadPID(clip, pid, start, stop, func(payload []byte, pts90 uint64) bool {
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
				matrix = matrixFor(w.Height)
				if s.data[7]&0x80 != 0 {
					// An epoch starts: nothing from the last one can be used
					// again, so it need not be kept.
					ods.complete = map[int]*Object{}
					palettes = map[int]Palette{}
				}
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
				id, p := parsePDS(s.data, matrix)
				palettes[id] = p
			case segODS:
				ods.feed(s.data)
			case segEnd:
				if cur == nil {
					continue
				}
				if last != nil {
					for i := openFrom; i < len(w.Subtitles); i++ {
						if w.Subtitles[i].End == 0 {
							w.Subtitles[i].End = cur.pts
						}
					}
				}
				if len(cur.objects) > 0 && cur.pts >= from {
					// Forced and ordinary text in one set are separate pictures:
					// with subtitles off, only the forced one shows.
					first := len(w.Subtitles)
					for _, forced := range []bool{true, false} {
						var part []compObject
						for _, o := range cur.objects {
							if o.forced == forced {
								part = append(part, o)
							}
						}
						if sub := compose(part, windows, palettes[cur.palette], ods.complete, w.Width, w.Height); sub != nil {
							sub.Start = cur.pts
							w.Subtitles = append(w.Subtitles, *sub)
						}
					}
					openFrom = first
					last = nil
					if len(w.Subtitles) > first {
						last = &w.Subtitles[first]
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

// compose draws a display set's objects into one picture covering them,
// each cut to its window: authors wipe text on by growing a window.
func compose(objs []compObject, windows []Window, p Palette, pics map[int]*Object, fw, fh int) *Subtitle {
	type placed struct {
		o compObject
		r image.Rectangle
	}
	var list []placed
	bounds := image.Rectangle{}
	forced := false
	for _, o := range objs {
		pic := pics[o.object]
		if pic == nil {
			continue
		}
		if o.cw <= 0 || o.ch <= 0 {
			o.cx, o.cy, o.cw, o.ch = 0, 0, pic.Width, pic.Height
		}
		r := image.Rect(o.x, o.y, o.x+o.cw, o.y+o.ch)
		for _, w := range windows {
			if w.ID == o.window {
				r = r.Intersect(image.Rect(w.X, w.Y, w.X+w.W, w.Y+w.H))
				break
			}
		}
		if fw > 0 && fh > 0 {
			r = r.Intersect(image.Rect(0, 0, fw, fh))
		}
		if r.Empty() {
			continue
		}
		// Keep the picture in register when the window cuts its top or left.
		o.cx, o.cy = o.cx+r.Min.X-o.x, o.cy+r.Min.Y-o.y
		o.x, o.y, o.cw, o.ch = r.Min.X, r.Min.Y, r.Dx(), r.Dy()
		list = append(list, placed{o, r})
		bounds = bounds.Union(r)
		forced = forced || o.forced
	}
	if bounds.Empty() {
		return nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for _, pl := range list {
		o := pl.o
		draw(img, pics[o.object], &p, o.x-bounds.Min.X, o.y-bounds.Min.Y, o.cx, o.cy, o.cw, o.ch)
	}
	return &Subtitle{X: bounds.Min.X, Y: bounds.Min.Y, Image: img, Forced: forced}
}
