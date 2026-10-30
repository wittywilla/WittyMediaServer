package bdsession

import (
	"fmt"
	"image/color"
	"strings"

	"github.com/wittywilla/wittybluray"
)

type subPic struct {
	ev  SubEvent
	png []byte
}

const maxSubPics = 3000

// Subtitles returns the subtitle events of stream n (0-based, from the
// title's subtitle list) that overlap [from, to] seconds of playlist ttn,
// reading only that window of the clips.
func (s *Session) Subtitles(ttn, n int, from, to float64) ([]SubEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pl := s.disc.Playlist(ttn)
	if pl == nil {
		return nil, fmt.Errorf("no such playlist")
	}
	laid := s.playlist
	if laid == nil || laid.pl != pl {
		laid = s.layout(pl, -1)
	}
	out := []SubEvent{}
	for _, pi := range laid.items {
		if pi.start+pi.duration < from || pi.start > to {
			continue
		}
		it := &pl.Items[pi.item]
		if n < 0 || n >= len(it.PG) {
			continue
		}
		st := it.PG[n]
		if st.Coding == 0x92 {
			out = append(out, s.textEvents(pl, pi, &st, from, to)...)
			continue
		}
		// A stream kept in a sub-path has its own clip and its own clock: its
		// IN time meets the main path at the sub play item's sync time.
		clip, stc := pi.clip, it.STCID
		clipBase := pi.start - float64(pi.startPTS)/45000 // playlist seconds at the clip's time 0
		if st.EntryType != 1 {
			sp := subPlayItem(pl, &st, pi.item)
			if sp == nil {
				continue
			}
			clip, stc = pl.SubClip(st), sp.STCID
			clipBase = pi.start + (float64(sp.SyncStart)-float64(pi.startPTS)-float64(sp.In))/45000
		}
		fromPTS := uint32(0)
		if t := from - clipBase; t > 0 {
			fromPTS = uint32(t * 45000)
		}
		end := to
		if pi.start+pi.duration < end {
			end = pi.start + pi.duration
		}
		toPTS := uint32(0)
		if t := end - clipBase; t > 0 {
			toPTS = uint32(t * 45000)
		}
		pid := st.PID
		key := fmt.Sprintf("%s-%x-%d-%d", clip, pid, fromPTS/45000/60, pi.item) // one window per clip, stream and minute
		win, err := s.disc.SubtitlesIn(clip, pid, stc, fromPTS, toPTS)
		if err != nil {
			return nil, err
		}
		for k, sub := range win.Subtitles {
			start := clipBase + float64(sub.Start)/45000
			stop := start + 5
			if sub.End > sub.Start {
				stop = clipBase + float64(sub.End)/45000
			} else if k+1 < len(win.Subtitles) {
				stop = clipBase + float64(win.Subtitles[k+1].Start)/45000
			}
			if stop > pi.start+pi.duration {
				stop = pi.start + pi.duration
			}
			if stop < from || start > to {
				continue
			}
			id := fmt.Sprintf("%s-%d-%d", key, sub.Start, k)
			sp, ok := s.subPics[id]
			if !ok {
				sp = &subPic{ev: SubEvent{Start: start, End: stop, X: sub.X, Y: sub.Y, W: sub.Image.Rect.Dx(), H: sub.Image.Rect.Dy(), Forced: sub.Forced}, png: encode(sub.Image)}
				if len(s.subPics) >= maxSubPics {
					s.subPics = map[string]*subPic{}
				}
				s.subPics[id] = sp
			}
			ev := sp.ev
			ev.URL = s.url("sub", id+".png")
			out = append(out, ev)
		}
	}
	return out, nil
}

// subPlayItem finds the sub play item that goes with a main-path item.
func subPlayItem(pl *wittybluray.Playlist, st *wittybluray.Stream, item int) *wittybluray.SubPlayItem {
	if st.SubPath >= len(pl.SubPaths) || len(pl.SubPaths[st.SubPath].Items) == 0 {
		return nil
	}
	items := pl.SubPaths[st.SubPath].Items
	for i := range items {
		if items[i].SyncItem == item {
			return &items[i]
		}
	}
	return &items[0]
}

// textEvents lists the dialogs of a text subtitle stream in a time window.
func (s *Session) textEvents(pl *wittybluray.Playlist, pi playedItem, st *wittybluray.Stream, from, to float64) []SubEvent {
	sp := subPlayItem(pl, st, pi.item)
	if sp == nil {
		return nil
	}
	clip := pl.SubClip(*st)
	key := fmt.Sprintf("%s-%x", clip, st.PID)
	cues, ok := s.textCues[key]
	if !ok {
		cues, _ = s.disc.TextSubtitles(clip, st.PID, st.CharCode)
		if s.textCues == nil {
			s.textCues = map[string][]wittybluray.TextCue{}
		}
		s.textCues[key] = cues
	}
	base := pi.start + (float64(sp.SyncStart)-float64(pi.startPTS)-float64(sp.In))/45000
	var out []SubEvent
	for i := range cues {
		c := &cues[i]
		start, stop := base+float64(c.Start)/45000, base+float64(c.End)/45000
		if stop < from || start > to || start > pi.start+pi.duration || c.Text == "" {
			continue
		}
		y := c.Style
		out = append(out, SubEvent{Start: start, End: stop, X: y.X, Y: y.Y, W: y.W, H: y.H, Forced: c.Forced, Text: c.Text,
			Color: cssColor(y.Color), Outline: cssColor(y.OutlineColor), Size: y.FontSize, HAlign: y.HAlign, VAlign: y.VAlign, Bold: y.Bold, Italic: y.Italic})
	}
	return out
}

func cssColor(c color.RGBA) string {
	if c.A == 0 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// SubPNG serves a rendered subtitle picture.
func (s *Session) SubPNG(name string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sp, ok := s.subPics[strings.TrimSuffix(name, ".png")]
	if !ok || sp.png == nil {
		return nil, false
	}
	return sp.png, true
}

// Stereo3D says whether the disc carries 3D content (the player then offers
// 3D viewing modes).
func (s *Session) Stereo3D() bool { return s.disc.Stereo3D }

var _ = wittybluray.NoButton
