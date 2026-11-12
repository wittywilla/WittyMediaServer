// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bdremote

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/bluray"
	"github.com/wittywilla/wittymediaserver/internal/disckey"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
)

// The playlist ranking, chapters and cross-checks of the WittyBluray in use. An analysis has
// a helper of its own, which ends with the answer; a session's helper only answers
// "chapters", for the playlists that session plays.

const (
	// analyzeLimit is how long one call of an analysis may take. A ranking of two thousand
	// playlists takes a fraction of a second on a desktop; a disc on a slow share reads its
	// playlists and clip information first. A helper quiet for longer is stopped, which
	// counts against its version like any hang.
	analyzeLimit = 2 * time.Minute
	// capabilityLife is how long what a helper said it can do is believed: someone may
	// install or remove a library while the server runs.
	capabilityLife = 5 * time.Minute
)

// unknownMethod: a working helper from before a call existed.
func unknownMethod(err error) bool {
	var ce *wittymodule.CallError
	return errors.As(err, &ce) && ce.Code == "unknown-method"
}

// refused: a working helper saying no, about this disc or this call.
func refused(err error) bool {
	var ce *wittymodule.CallError
	return errors.As(err, &ce)
}

var (
	analyses  = disckey.Cache[*bdsession.Analysis]{Max: 24}
	analysing sync.Mutex

	capMu   sync.Mutex
	capAt   time.Time
	capFor  string
	capLast bluray.Capabilities
)

// useAnalysis wires the analysis calls to the manager's version of WittyBluray.
func useAnalysis(m *modules.Manager) {
	bdsession.Analyzer = func(path string, second bool, log *slog.Logger) (*bdsession.Analysis, error) {
		a := m.Attach(moduleID, analyzeLimit)
		if a == nil {
			return nil, m.Missing(moduleID)
		}
		version := a.Version()
		protected := bdsession.Scrambled(path)
		if protected {
			second = false // nothing but this project's own reader looks at a protected disc
		}
		key, keyed := disckey.Of(path, fmt.Sprintf("%s|%t", version, second))
		cached := func() *bdsession.Analysis {
			if keyed {
				if got, ok := analyses.Get(key); ok {
					return got
				}
			}
			return nil
		}
		if got := cached(); got != nil {
			a.Close() // nothing was started
			return got, nil
		}
		analysing.Lock()
		defer analysing.Unlock()
		if got := cached(); got != nil {
			a.Close()
			return got, nil
		}
		got, err := analyze(a, path, second, log, func(reason string) { _ = m.ReportFailure(moduleID, version, reason) })
		if err != nil {
			return nil, err
		}
		got.Engine, got.Protected = version, protected
		if keyed {
			analyses.Put(key, got)
		}
		return got, nil
	}
	bdsession.Capability = func() (bluray.Capabilities, error) {
		a := m.Attach(moduleID, javaProbeLimit)
		if a == nil {
			return bluray.Capabilities{}, m.Missing(moduleID)
		}
		defer a.Close()
		capMu.Lock()
		defer capMu.Unlock()
		if capFor == a.Version() && time.Since(capAt) < capabilityLife {
			return capLast, nil
		}
		c, err := capabilities(a)
		if err != nil {
			return c, err
		}
		capFor, capAt, capLast = a.Version(), time.Now(), c
		return c, nil
	}
}

// capabilities asks a helper what optional help it has. One from before the call has none.
func capabilities(h process) (bluray.Capabilities, error) {
	var c bluray.Capabilities
	_, err := h.Call("capabilities", bluray.Params{}, &c)
	if unknownMethod(err) {
		return bluray.Capabilities{Libbluray: bluray.Cap{Reason: "This version of WittyBluray can't ask libbluray. Update WittyBluray."}}, nil
	}
	if err != nil {
		return bluray.Capabilities{}, err
	}
	c.Libbluray.Tidy()
	return c, nil
}

// analyze asks one helper, and closes it. A disc the server found encrypted is still read
// for its structure: playlists and clip information never are encrypted, reading them
// decrypts nothing, and nothing of such a disc plays whatever the analysis says.
func analyze(h process, path string, second bool, log *slog.Logger, report func(string)) (*bdsession.Analysis, error) {
	defer h.Close()
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	e, err := newEngine(h, path, bluray.NavOptions{}, log, report)
	if err != nil {
		return nil, err
	}
	var info bluray.Opened
	if _, err := h.Call("open", bluray.Params{Path: e.path}, &info); err != nil {
		return nil, err
	}
	if len(info.Playlists) > maxPlaylists {
		err := &badAnswer{fmt.Errorf("a disc of %d playlists", len(info.Playlists))}
		report(err.Error())
		return nil, err
	}
	known := make(map[int]bool, len(info.Playlists))
	for _, n := range info.Playlists {
		known[n] = n >= 0 && n <= 99999
	}
	out := &bdsession.Analysis{Playlists: info.Playlists, SecondOpinion: second}
	var ranked bluray.Ranked
	_, err = h.Call("ranking", bluray.Params{Shown: second, Limit: bluray.RankingLimit}, &ranked)
	switch {
	case unknownMethod(err):
		return out, nil // a WittyBluray from before the ranking: its "main" still answers
	case refused(err):
		log.Info("bluray: the playlist ranking was refused", "cat", "discs", "path", path, "err", err)
		return out, nil
	case err != nil:
		log.Warn("bluray: the playlist ranking failed", "cat", "discs", "path", path, "err", err)
		return nil, err
	}
	if err := ranked.Check(func(n int) bool { return known[n] }); err != nil {
		report("it sent a bad answer: " + err.Error())
		log.Warn("bluray: the playlist ranking was refused by the server", "cat", "discs", "path", path, "err", err)
		return nil, &badAnswer{err}
	}
	if !second {
		// What it was not asked for is not taken from it.
		ranked.Ranking.SecondOpinion, ranked.Note = nil, ""
	}
	out.Ranked = &ranked
	var c bluray.Consistency
	if _, err := h.Call("consistency", bluray.Params{}, &c); err == nil {
		c.Tidy()
		out.Consistency = &c
	} else if !refused(err) {
		// The helper went while cross-checking. The ranking it gave stands.
		log.Warn("bluray: the cross-checks failed", "cat", "discs", "path", path, "err", err)
	}
	return out, nil
}

// maxPlaylists is more playlists than a disc can have: their names are five digits.
const maxPlaylists = 100000

// Chapters are a playlist's chapters on its playing time, from the engine. A helper from
// before the call is remembered as such, and the marks are read here as they were before.
func (e *engine) Chapters(pl *bluray.Playlist) []bluray.Chapter {
	if pl == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if got, ok := e.chapters[pl.Number]; ok {
		return got
	}
	if e.h == nil || e.noChapters {
		return bluray.MarkChapters(pl)
	}
	var list bluray.ChapterList
	_, err := e.h.Call("chapters", bluray.Params{N: pl.Number}, &list)
	switch {
	case unknownMethod(err):
		e.noChapters = true
		return bluray.MarkChapters(pl)
	case refused(err):
		return bluray.MarkChapters(pl)
	case err != nil:
		e.fail(err)
		return nil
	}
	if err := list.Check(len(pl.Items)); err != nil || list.Playlist != pl.Number {
		if err == nil {
			err = errors.New("the chapters are another playlist's")
		}
		e.fail(&badAnswer{err})
		return nil
	}
	if e.chapters == nil {
		e.chapters = map[int][]bluray.Chapter{}
	}
	e.chapters[pl.Number] = list.Chapters
	return list.Chapters
}
