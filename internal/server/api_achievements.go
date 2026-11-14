// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/library"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// A game's achievements at RetroAchievements, to show with the game. Nothing is earned
// or tracked here yet: every achievement is shown greyed out. The game is found by a
// checksum of its file, taken the first time someone opens it and kept, never during a
// scan. The file is only read: games live on drives that may be read-only.
//
// Badges come through the server, so a browser never contacts RetroAchievements and
// the key never leaves the server.

// Achievement answer states.
const (
	achOK          = "ok"
	achNoKey       = "nokey"
	achUnsupported = "unsupported"
	achNotFound    = "notfound"
	achError       = "error"
)

// AchievementsAnswer is GET /api/items/{id}/achievements.
type AchievementsAnswer struct {
	State        string            `json:"state"`
	Game         *AchievementsGame `json:"game"`
	Achievements []AchievementView `json:"achievements"`
	Message      string            `json:"message"`
}

// AchievementsGame is the game as RetroAchievements knows it.
type AchievementsGame struct {
	ID     int    `json:"id"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Points int    `json:"points"`
}

// AchievementView is one achievement. Badge is this server's address for its
// greyed-out picture, "" if it has none.
type AchievementView struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Points      int    `json:"points"`
	Badge       string `json:"badge"`
}

// raHashSlots: checksums taken at once. Each reads a whole file, or unpacks one.
var raHashSlots = make(chan struct{}, 2)

// discExts are files of disc games, whose checksums aren't taken here.
var discExts = map[string]bool{".cue": true, ".m3u": true, ".gdi": true, ".chd": true, ".ccd": true, ".iso": true, ".toc": true,
	".cdi": true, ".pbp": true, ".cso": true, ".img": true, ".mds": true, ".mdf": true, ".nrg": true}

func achAnswer(state, msg string) AchievementsAnswer {
	return AchievementsAnswer{State: state, Achievements: []AchievementView{}, Message: msg}
}

// raGameFor finds the game an item is at RetroAchievements. The answer that isn't a
// game is a whole reply.
func (s *Server) raGameFor(ctx context.Context, it *library.Item) (*gamemeta.RetroAchievements, *gamemeta.RAGame, *AchievementsAnswer) {
	consoles := gamemeta.RAConsoles(it.Console)
	if len(consoles) == 0 || discExts[strings.ToLower(filepath.Ext(it.Path))] {
		a := achAnswer(achUnsupported, "Achievements can't be looked up for this console's games yet.")
		return nil, nil, &a
	}
	ra, ready := s.GameMeta.Achievements(ctx)
	if ra == nil || !ready {
		a := achAnswer(achNoKey, "To see this game's achievements, an administrator can add a RetroAchievements Web API key in Settings, Metadata agents.")
		return nil, nil, &a
	}
	hash, err := s.raHash(ctx, ra, it)
	if errors.Is(err, gamemeta.ErrRANotCovered) {
		a := achAnswer(achUnsupported, "Achievements can't be looked up for this game's file.")
		return nil, nil, &a
	}
	if err != nil {
		s.Log.Info("achievements: no checksum", "cat", "games", "item", it.ID, "err", err)
		a := achAnswer(achError, "This game's file couldn't be read to find its achievements.")
		return nil, nil, &a
	}
	id, _, err := ra.FindGame(ctx, consoles, hash)
	if err == nil {
		var g *gamemeta.RAGame
		if g, err = ra.Game(ctx, id); err == nil {
			return ra, g, nil
		}
	}
	a := raErrorAnswer(err)
	if a.State == achError {
		s.Log.Info("achievements", "cat", "games", "item", it.ID, "err", err)
	}
	return nil, nil, &a
}

func raErrorAnswer(err error) AchievementsAnswer {
	var rl *gamemeta.RateLimited
	switch {
	case errors.Is(err, gamemeta.ErrNotFound):
		return achAnswer(achNotFound, "RetroAchievements doesn't know this exact copy of the game, or has no achievements for it.")
	case errors.Is(err, gamemeta.ErrNotSet):
		return achAnswer(achNoKey, "To see this game's achievements, an administrator can add a RetroAchievements Web API key in Settings, Metadata agents.")
	case errors.Is(err, gamemeta.ErrKey):
		return achAnswer(achError, "RetroAchievements didn't accept the Web API key. An administrator can check it in Settings, Metadata agents.")
	case errors.As(err, &rl):
		return achAnswer(achError, "RetroAchievements asked for a pause. Try again in a few minutes.")
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return achAnswer(achError, "RetroAchievements didn't answer in time. Try again in a moment.")
	}
	return achAnswer(achError, "RetroAchievements couldn't be asked just now. Try again later.")
}

// raKept is a file's checksum, kept with what the file was when it was taken.
type raKept struct {
	Path  string `json:"path"`
	Entry string `json:"entry,omitempty"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
	Hash  string `json:"hash"`
}

// raArchives unpacks an archived game long enough to take its checksum, in the server's
// cache. WittyPlay keeps its own unpacked games for playing them.
func (s *Server) raArchives() *gamearchive.Cache {
	s.raArchiveOnce.Do(func() {
		s.raArchive = &gamearchive.Cache{Dir: filepath.Join(s.Cfg.CacheDir, "ra-unpacked"), Limit: func() int64 { return raUnpackLimit }}
	})
	return s.raArchive
}

// raUnpackLimit is the room the checksums' unpacked games may take: one large disc.
const raUnpackLimit = 8 << 30

// raHash is the checksum RetroAchievements knows this game's file by. It's taken once
// and kept under the server's cache, until the file changes.
func (s *Server) raHash(ctx context.Context, ra *gamemeta.RetroAchievements, it *library.Item) (string, error) {
	if gamemeta.RAByName(it.Console) {
		return gamemeta.RAHashName(it.Path), nil
	}
	st, err := os.Stat(it.Path)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", gamemeta.ErrRANotCovered
	}
	entry := ""
	archive := gamearchive.IsArchive(filepath.Base(it.Path))
	if archive {
		entry = s.Store.GameEntry(ctx, it.ID)
	}
	p := filepath.Join(ra.Dir, "hashes", strconv.FormatInt(it.ID, 10)+".json")
	var kept raKept
	if b, err := os.ReadFile(p); err == nil && json.Unmarshal(b, &kept) == nil &&
		kept.Path == it.Path && kept.Entry == entry && kept.Size == st.Size() && kept.MTime == st.ModTime().UnixNano() && kept.Hash != "" {
		return kept.Hash, nil
	}
	select {
	case raHashSlots <- struct{}{}:
		defer func() { <-raHashSlots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	rom := it.Path
	if archive {
		console := wittyplay.ByID(it.Console)
		if console == nil {
			return "", fmt.Errorf("console %q isn't available to unpack the game", it.Console)
		}
		var cleanup func()
		rom, cleanup, err = s.raArchives().AcquireEntry(ctx, it.Path, console, entry)
		if err != nil {
			return "", err
		}
		if cleanup != nil {
			defer cleanup()
		}
		if discExts[strings.ToLower(filepath.Ext(rom))] {
			return "", gamemeta.ErrRANotCovered
		}
	}
	f, err := os.Open(rom)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fst, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !fst.Mode().IsRegular() || fst.Size() > gamemeta.RAMaxSize {
		return "", gamemeta.ErrRANotCovered
	}
	hash, err := gamemeta.RAHash(it.Console, f, fst.Size())
	if err != nil {
		return "", err
	}
	kept = raKept{Path: it.Path, Entry: entry, Size: st.Size(), MTime: st.ModTime().UnixNano(), Hash: hash}
	if b, err := json.Marshal(kept); err == nil {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			tmp, err := os.CreateTemp(filepath.Dir(p), ".hash-*")
			if err == nil {
				_, werr := tmp.Write(b)
				if tmp.Close() == nil && werr == nil {
					_ = os.Rename(tmp.Name(), p)
				}
				_ = os.Remove(tmp.Name())
			}
		}
	}
	return hash, nil
}

// GET /api/items/{id}/achievements
func (s *Server) handleItemAchievements(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second) // a console's list is a few megabytes
	defer cancel()
	_, g, answer := s.raGameFor(ctx, it)
	if answer != nil {
		writeJSON(w, 200, answer)
		return
	}
	out := achAnswer(achOK, "")
	out.Game = &AchievementsGame{ID: g.ID, Title: g.Title, URL: g.URL(), Points: g.Points}
	for _, a := range g.Achievements {
		v := AchievementView{ID: a.ID, Title: a.Title, Description: a.Description, Points: a.Points}
		if a.Badge != "" {
			v.Badge = fmt.Sprintf("/api/items/%d/achievements/badge/%s", it.ID, a.Badge)
		}
		out.Achievements = append(out.Achievements, v)
	}
	writeJSON(w, 200, out)
}

// GET /api/items/{id}/achievements/badge/{badge}: one of the game's achievement
// pictures, greyed out. Only the game's own pictures are fetched.
func (s *Server) handleItemAchievementBadge(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	badge := r.PathValue("badge")
	if !gamemeta.ValidBadge(badge) {
		writeErr(w, 400, "bad badge name")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	ra, g, answer := s.raGameFor(ctx, it)
	if answer != nil || !g.HasBadge(badge) {
		writeErr(w, 404, "no such badge")
		return
	}
	b, err := ra.Badge(ctx, badge)
	if err != nil {
		writeErr(w, 404, "no such badge")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=604800")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	_, _ = w.Write(b)
}
