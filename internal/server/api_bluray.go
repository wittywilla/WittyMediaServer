// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/bluray"
	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The disc page for a Blu-ray: its playlists as the feature and extras,
// with chapters, and the top menu as a shortcut. "ttn" is the playlist
// number, as it is for the player.

func (s *Server) handleDiscBluray(w http.ResponseWriter, r *http.Request, it *library.Item) {
	od, err := bdsession.Open(it.Path, s.Log)
	if writeModuleMissing(w, err) {
		return
	}
	if err != nil {
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	if od.Disc == nil {
		writeErr(w, 403, "This disc is encrypted, and encrypted discs aren't supported.")
		return
	}
	d := od.Disc
	defer d.Close()
	labels := s.Store.DiscLabels(r.Context(), it.ID)
	view := func(pl *bluray.Playlist) discTitle {
		t := discTitle{TTN: pl.Number, Label: labels.Titles[strconv.Itoa(pl.Number)], DurationSec: d.PlaylistDuration(pl), Angles: 1, Wide: true,
			Chapters: []discChapter{}, Audio: []string{}, Subtitles: []string{}}
		names := labels.Chapters[strconv.Itoa(pl.Number)]
		// Chapters as the engine maps them onto the playlist's playing time.
		for i, ch := range bdsession.Chapters(d, pl) {
			t.Chapters = append(t.Chapters, discChapter{N: i + 1, Start: ch.Start, Label: names[strconv.Itoa(i+1)]})
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
	// The film is the playlist an admin chose, or the engine's ranked pick.
	main := d.MainPlaylist()
	if it.Disc != nil && (it.Disc.Chosen != nil || it.Disc.Rank != nil) && it.Disc.MainTitle >= 0 {
		if pl := d.Playlist(it.Disc.MainTitle); pl != nil && len(pl.Items) > 0 {
			main = pl
		}
	}
	if main != nil {
		out["main"] = view(main)
	}
	// With a ranking, the extras are what the engine sent as worth watching. A disc with
	// a thousand decoy playlists then lists none of them, and asks for none of them.
	playlists := od.Playlists
	if rk := rankOf(it); rk != nil {
		playlists = playlists[:0:0]
		for _, c := range rk.Candidates {
			if c.Class != "duplicate" && c.Class != "stub" {
				playlists = append(playlists, c.TTN)
			}
		}
		out["hidden"] = rk.Hidden + rk.Decoys
	}
	addRank(out, it)
	var extras []discTitle
	for _, n := range playlists {
		pl := d.Playlist(n)
		if pl == nil || pl == main || d.PlaylistDuration(pl) < 10 || len(pl.Items) == 0 || len(pl.Items[0].Video) == 0 {
			continue
		}
		if len(pl.Items[0].IG) > 0 && d.PlaylistDuration(pl) < 600 {
			continue // menu loops
		}
		extras = append(extras, view(pl))
	}
	sort.SliceStable(extras, func(i, j int) bool { return extras[i].DurationSec > extras[j].DurationSec })
	if extras != nil {
		out["extras"] = extras
	}
	if od.Index != nil && od.Index.TopMenu.Kind != bluray.ObjectNone {
		out["menus"] = []discMenu{{Kind: "title", Label: "Top menu"}}
	}
	writeJSON(w, 200, out)
}

// makeBlurayThumb grabs a frame of a playlist (chapter n, 0 = the start).
func (s *Server) makeBlurayThumb(ctx context.Context, it *library.Item, ttn, n int, out string) error {
	od, err := bdsession.Open(it.Path, s.Log)
	if err != nil {
		return err
	}
	if od.Disc == nil {
		return errNoPicture
	}
	d := od.Disc
	defer d.Close()
	pl := d.Playlist(ttn)
	if pl == nil && ttn == 0 {
		pl = d.MainPlaylist()
	}
	if pl == nil || len(pl.Items) == 0 {
		return errNoPicture
	}
	chapters := bdsession.Chapters(d, pl)
	// Only chapters that exist get a file: the address is the viewer's to choose, and
	// every other number would be one more picture on disk.
	if n < 0 || n > len(chapters) {
		return errNoPicture
	}
	// A few seconds into the chapter skips fades from black.
	item, at := 0, pl.Items[0].In+uint32(min(90, d.PlaylistDuration(pl)*0.1)*45000)
	if n >= 1 {
		item, at = chapters[n-1].Item, chapters[n-1].ClipTime+6*45000
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
	// A clip with no entry-point map (a recording, a damaged disc) is read
	// from its start.
	e, ok := c.SPNAt(c.VideoPID(), at)
	if !ok {
		e = bluray.EPEntry{PTS: pi.In}
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
	var cut []dvd.FileExtent
	var off int64
	for _, x := range ext {
		a, b := off, off+x.Length
		off = b
		lo, hi := max(a, from), min(b, to)
		if hi > lo {
			cut = append(cut, dvd.FileExtent{File: x.Path, Offset: x.Offset + (lo - a), Length: hi - lo})
		}
	}
	if len(cut) == 0 {
		return fmt.Errorf("cannot read the chapter")
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755) // a failure shows when FFmpeg can't write the picture
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
