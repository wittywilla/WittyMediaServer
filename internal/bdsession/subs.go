package bdsession

import (
	"fmt"
	"strings"

	"github.com/wittywilla/wittybluray"
)

type subPic struct {
	ev  SubEvent
	png []byte
}

const maxSubPics = 3000

// Subtitles returns the subtitle pictures of PG stream n (0-based, from
// the title's subtitle list) that overlap [from, to] seconds of playlist
// ttn, reading only that window of the clips.
func (s *Session) Subtitles(ttn, n int, from, to float64) ([]SubEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pl := s.disc.Playlist(ttn)
	if pl == nil {
		return nil, fmt.Errorf("no such playlist")
	}
	laid := s.playlist
	if laid == nil || laid.pl != pl {
		laid = s.layout(pl)
	}
	out := []SubEvent{}
	for i, pi := range laid.items {
		if pi.start+pi.duration < from || pi.start > to {
			continue
		}
		it := pl.Items[i]
		if n < 0 || n >= len(it.PG) || it.PG[n].EntryType != 1 {
			continue
		}
		pid := it.PG[n].PID
		fromPTS := pi.startPTS
		if from > pi.start {
			fromPTS += uint32((from - pi.start) * 45000)
		}
		toPTS := pi.startPTS + uint32(pi.duration*45000)
		if to < pi.start+pi.duration {
			toPTS = pi.startPTS + uint32((to-pi.start)*45000)
		}
		key := fmt.Sprintf("%s-%x-%d-%d", pi.clip, pid, fromPTS/45000/60, i) // one window per clip, stream and minute
		win, err := s.disc.Subtitles(pi.clip, pid, fromPTS, toPTS)
		if err != nil {
			return nil, err
		}
		for k, sub := range win.Subtitles {
			start := pi.start + float64(int64(sub.Start)-int64(pi.startPTS))/45000
			end := start + 5
			if sub.End > sub.Start {
				end = pi.start + float64(int64(sub.End)-int64(pi.startPTS))/45000
			} else if k+1 < len(win.Subtitles) {
				end = pi.start + float64(int64(win.Subtitles[k+1].Start)-int64(pi.startPTS))/45000
			}
			if end > pi.start+pi.duration {
				end = pi.start + pi.duration
			}
			if end < from || start > to {
				continue
			}
			id := fmt.Sprintf("%s-%d", key, sub.Start)
			sp, ok := s.subPics[id]
			if !ok {
				sp = &subPic{ev: SubEvent{Start: start, End: end, X: sub.X, Y: sub.Y, W: sub.Image.Rect.Dx(), H: sub.Image.Rect.Dy(), Forced: sub.Forced}, png: encode(sub.Image)}
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
