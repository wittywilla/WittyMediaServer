package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The disc page for a Blu-ray: its playlists as the feature and extras,
// with chapters, and the top menu as a shortcut. "ttn" is the playlist
// number, as it is for the player.

func (s *Server) handleDiscBluray(w http.ResponseWriter, r *http.Request, it *library.Item) {
	d, err := wittybluray.Open(it.Path)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer d.Close()
	labels := s.Store.DiscLabels(r.Context(), it.ID)
	view := func(pl *wittybluray.Playlist) discTitle {
		t := discTitle{TTN: pl.Number, Label: labels.Titles[strconv.Itoa(pl.Number)], DurationSec: d.PlaylistDuration(pl), Angles: 1, Wide: true,
			Chapters: []discChapter{}, Audio: []string{}, Subtitles: []string{}}
		names := labels.Chapters[strconv.Itoa(pl.Number)]
		var t0 float64
		starts := map[int]float64{}
		for i, pi := range pl.Items {
			starts[i] = t0
			t0 += pi.Duration()
		}
		n := 0
		for _, m := range pl.Marks {
			if m.Type != 1 || m.Item >= len(pl.Items) {
				continue
			}
			n++
			at := starts[m.Item] + float64(int64(m.Time)-int64(pl.Items[m.Item].In))/45000
			t.Chapters = append(t.Chapters, discChapter{N: n, Start: at, Label: names[strconv.Itoa(n)]})
		}
		if len(pl.Items) > 0 {
			it0 := pl.Items[0]
			t.Angles = len(it0.Angles)
			for _, a := range it0.Audio {
				t.Audio = append(t.Audio, strings.TrimSpace(a.Lang+" "+a.CodingName()))
			}
			for _, sub := range it0.PG {
				t.Subtitles = append(t.Subtitles, sub.Lang)
			}
		}
		return t
	}
	out := map[string]any{"main": nil, "extras": []discTitle{}, "menus": []discMenu{}}
	main := d.MainPlaylist()
	if main != nil {
		out["main"] = view(main)
	}
	var extras []discTitle
	for n, pl := range d.Playlists {
		if pl == main || d.PlaylistDuration(pl) < 10 || len(pl.Items) == 0 || len(pl.Items[0].Video) == 0 {
			continue
		}
		if len(pl.Items[0].IG) > 0 && d.PlaylistDuration(pl) < 600 {
			continue // menu loops
		}
		_ = n
		extras = append(extras, view(pl))
	}
	sort.SliceStable(extras, func(i, j int) bool { return extras[i].DurationSec > extras[j].DurationSec })
	if extras != nil {
		out["extras"] = extras
	}
	if d.Index.TopMenu.Kind == wittybluray.ObjectHDMV {
		out["menus"] = []discMenu{{Kind: "title", Label: "Top menu"}}
	}
	writeJSON(w, 200, out)
}

// makeBlurayThumb grabs a frame of a playlist (chapter n, 0 = the start).
func (s *Server) makeBlurayThumb(ctx context.Context, it *library.Item, ttn, n int, out string) error {
	d, err := wittybluray.Open(it.Path)
	if err != nil {
		return err
	}
	defer d.Close()
	pl := d.Playlist(ttn)
	if pl == nil {
		pl = d.MainPlaylist()
	}
	if pl == nil || len(pl.Items) == 0 {
		return errNoTitle
	}
	// A few seconds into the chapter skips fades from black.
	item, at := 0, pl.Items[0].In+uint32(minf(90, d.PlaylistDuration(pl)*0.1)*45000)
	chapters := 0
	for _, m := range pl.Marks {
		if m.Type != 1 {
			continue
		}
		chapters++
		if chapters == n {
			item, at = m.Item, m.Time+6*45000
		}
	}
	if n < 0 || n > chapters {
		return errNoTitle // only real chapters: every other number would be one more picture on disk
	}
	if item >= len(pl.Items) {
		item = 0
	}
	pi := pl.Items[item]
	if at >= pi.Out {
		at = pi.In + (pi.Out-pi.In)/2
	}
	c := d.Clip(pi.Clip)
	if c == nil {
		return fmt.Errorf("clip missing")
	}
	e, ok := c.SPNAt(c.VideoPID(), at)
	if !ok {
		return fmt.Errorf("no entry point")
	}
	ext, err := d.ClipExtents(pi.Clip)
	if err != nil {
		return err
	}
	// Read from the entry point to a few seconds after it.
	from := int64(e.SPN) * 192
	to := from + int64(c.TSRate/8)*20
	if to <= from {
		to = from + 64<<20
	}
	var cut []wittydvd.FileExtent
	var off int64
	for _, x := range ext {
		a, b := off, off+x.Length
		off = b
		lo, hi := max64(a, from), min64(b, to)
		if hi > lo {
			cut = append(cut, wittydvd.FileExtent{File: x.Path, Offset: x.Offset + (lo - a), Length: hi - lo})
		}
	}
	if len(cut) == 0 {
		return fmt.Errorf("cannot read the chapter")
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
	list, err := ffmpeg.WriteConcat(filepath.Join(s.Cfg.CacheDir, "lists"), strings.TrimSuffix(filepath.Base(out), ".jpg"), []ffmpeg.Segment{{Extents: cut}}, int(c.VideoPID()))
	if err != nil {
		return err
	}
	tctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	offSec := float64(int64(at)-int64(e.PTS)) / 45000
	if err := s.Tools.Still(tctx, ffmpeg.ConcatInputArgs(list), offSec, 360, out); err != nil {
		return err
	}
	if st, err := os.Stat(out); (err != nil || st.Size() == 0) && offSec > 0 {
		if err := s.Tools.Still(tctx, ffmpeg.ConcatInputArgs(list), 0, 360, out); err != nil {
			return err
		}
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return errNoPicture
	}
	return nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
