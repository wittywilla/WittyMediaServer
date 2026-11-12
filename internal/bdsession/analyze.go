// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bdsession

import (
	"context"
	"log/slog"

	"github.com/wittywilla/wittymediaserver/internal/bluray"
)

// Analysis is what the WittyBluray in use makes of a disc beyond its structure: which
// playlist is the film and why, and where the copy's structures disagree. It is worked out
// in a helper of its own, apart from any session's, so an analysis that fails or runs long
// costs no viewer their disc.
type Analysis struct {
	// Ranked is nil when the WittyBluray in use is from before the ranking: callers then
	// use the engine's MainPlaylist, which is that version's own older rule.
	Ranked *bluray.Ranked
	// Consistency is nil when the engine didn't say.
	Consistency *bluray.Consistency
	// Playlists are the disc's playlist numbers, as the helper listed them.
	Playlists []int
	// Engine is the version of WittyBluray that gave the answer.
	Engine string
	// Protected: the server's own look found encrypted streams. The playlists and clip
	// information of a disc are never encrypted and are still read; nothing of it plays,
	// and no second opinion is asked about it.
	Protected bool
	// SecondOpinion: a libbluray on this machine was asked to compare.
	SecondOpinion bool
}

// Analyzer analyses a disc through the WittyBluray version in use. The server sets it as it
// starts. second asks for the comparison with a libbluray the machine's owner installed,
// which only the server's setting may turn on.
var Analyzer func(path string, second bool, log *slog.Logger) (*Analysis, error)

// Analyze asks the WittyBluray in use about a disc. The answer is remembered for as long as
// the disc on disk stays the same.
func Analyze(path string, second bool, log *slog.Logger) (*Analysis, error) {
	if Analyzer == nil {
		return nil, ErrNoEngine
	}
	return Analyzer(path, second, log)
}

// Capability says what optional help the WittyBluray in use has on this machine. The server
// sets it as it starts.
var Capability func() (bluray.Capabilities, error)

// Chapters are a playlist's chapters on its playing time: from the engine when it answers
// "chapters", and otherwise read from the playlist's marks as before.
func Chapters(d DiscEngine, pl *bluray.Playlist) []bluray.Chapter {
	if pl == nil {
		return nil
	}
	if c, ok := d.(interface {
		Chapters(pl *bluray.Playlist) []bluray.Chapter
	}); ok {
		return c.Chapters(pl)
	}
	return bluray.MarkChapters(pl)
}

// PlayItem plays a playlist from the start of one of its play items: a stretch of a
// playlist that plays a TV disc's episodes one after another.
func (s *Session) PlayItem(ctx context.Context, ttn, item int) *Presentation {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disc.Encrypted {
		return s.stop(encryptedRefusal, true)
	}
	pl := s.disc.Playlist(ttn)
	if pl == nil {
		return s.stop("the disc has no such playlist", false)
	}
	if item < 0 || item >= len(pl.Items) {
		item = 0
	}
	if len(pl.Items) == 0 {
		return s.stop("the disc has nothing to play", false)
	}
	s.direct = pl.Number + 1
	return s.show(ctx, s.nav.PlayPlaylist(pl, item, pl.Items[item].In))
}
