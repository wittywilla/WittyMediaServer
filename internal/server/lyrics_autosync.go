// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/lyrics"
	"github.com/wittywilla/wittymediaserver/internal/pace"
)

// Timed lyrics set by themselves (internal/lyrics AutoSync): the first time a song's timed lines
// are asked for, the server listens to the song once, in the background, and keeps how far the
// lines should move. That becomes the song's own timing; anyone who moves the words themselves
// keeps their own. New lyrics for the song are listened for again.

// lyricsAutoVersion is raised when the way of listening changes, so older answers are made again.
const lyricsAutoVersion = 1

type lyricsAuto struct {
	V        int     `json:"v"`
	Key      string  `json:"key"` // the lines it was worked out for
	OffsetMS int64   `json:"offsetMs"`
	Moved    bool    `json:"moved"` // false: nothing stood out, the lines stay where they are
	Sure     float64 `json:"sure"`
	At       int64   `json:"at"`
}

var (
	lyricsAutoMu   sync.Mutex
	lyricsAutoBusy = map[string]bool{}
	lyricsAutoOne  = make(chan struct{}, 1) // one song listened to at a time
)

func (s *Server) lyricsAutoPath(id int64) string {
	return filepath.Join(s.Cfg.CacheDir, "lyrics", "timing-"+strconv.FormatInt(id, 10)+".json")
}

func lyricsLinesKey(starts []int64) string {
	h := sha256.New()
	for _, t := range starts {
		h.Write([]byte(strconv.FormatInt(t, 10) + ","))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// lyricsAutoOffset is how far these lines of it should move by themselves, and what is known:
// "set" (moved), "kept" (listened, nothing stood out), "working" (being listened to now), or ""
// (not for these lyrics: too few lines, no FFmpeg, a song this server can't read).
func (s *Server) lyricsAutoOffset(it *library.Item, lines []lyrics.Line) (int64, string) {
	starts := lyrics.LineStarts(lines, 400)
	if len(starts) < 4 || it.Path == "" || s.Tools == nil || s.Tools.FFmpeg == "" {
		return 0, ""
	}
	// The song's own voice, when it has been separated from the music (karaoke), is far clearer
	// to listen to than the middle of the mix; a separation made later is listened to again.
	song, from := it.Path, "song"
	for _, st := range s.stemsOf(context.Background(), it) {
		if st.Name == "vocals" && !st.Approximate && !st.Multitrack && st.Path != "" {
			song, from = st.Path, "voice"
			break
		}
	}
	key := lyricsLinesKey(starts) + "-" + from
	path := s.lyricsAutoPath(it.ID)
	if b, err := os.ReadFile(path); err == nil {
		var a lyricsAuto
		if json.Unmarshal(b, &a) == nil && a.V == lyricsAutoVersion && a.Key == key {
			if a.Moved {
				return a.OffsetMS, "set"
			}
			return 0, "kept"
		}
	}
	lyricsAutoMu.Lock()
	defer lyricsAutoMu.Unlock()
	if !lyricsAutoBusy[path] {
		lyricsAutoBusy[path] = true
		go s.listenForLyrics(it.ID, song, starts, key, path)
	}
	return 0, "working"
}

func (s *Server) listenForLyrics(id int64, song string, starts []int64, key, path string) {
	defer func() { lyricsAutoMu.Lock(); delete(lyricsAutoBusy, path); lyricsAutoMu.Unlock() }()
	lyricsAutoOne <- struct{}{}
	defer func() { <-lyricsAutoOne }()
	ctx, cancel := context.WithTimeout(pace.Background(context.Background()), 2*time.Minute)
	defer cancel()
	started := time.Now()
	pcm, _, err := ffmpeg.Run(ctx, s.Tools.FFmpeg, lyrics.AutoSyncArgs(song), "for", "lyrics timing")
	if err != nil {
		// Not kept: a song that can't be read now may be readable later.
		s.Log.Info("lyrics timing: the song couldn't be listened to", "cat", "lyrics", "item", id, "err", err)
		return
	}
	ms, sure, moved := lyrics.BestOffset(lyrics.VoiceCurve(pcm, lyrics.AutoSyncRate), starts)
	a := lyricsAuto{V: lyricsAutoVersion, Key: key, OffsetMS: ms, Moved: moved && ms != 0, Sure: sure, At: time.Now().Unix()}
	if !a.Moved {
		a.OffsetMS = 0
	}
	b, _ := json.Marshal(a)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
	s.Log.Info("lyrics timing set by listening", "cat", "lyrics", "item", id, slog.Int64("offsetMs", a.OffsetMS), "moved", a.Moved, "sure", sure, "took", time.Since(started).Round(time.Millisecond))
}

// lyricsAutoCached is where listening last put a song's lines, 0 when it moved them nowhere.
func (s *Server) lyricsAutoCached(id int64) int64 {
	var a lyricsAuto
	if b, err := os.ReadFile(s.lyricsAutoPath(id)); err == nil && json.Unmarshal(b, &a) == nil && a.V == lyricsAutoVersion && a.Moved {
		return a.OffsetMS
	}
	return 0
}
