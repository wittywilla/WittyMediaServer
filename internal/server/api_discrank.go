// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Which title of a disc is the film, and why. The disc engines rank a disc's titles
// (WittyDVD) or playlists (WittyBluray) and explain their pick; the scan keeps that with the
// item, and these routes show it, let an admin choose another title for good, and ask for
// the ranking again.
//
// Who may do what: anyone who may see the disc may read its analysis. Choosing the film is
// a fact about the disc for everyone who watches it, like the names of its titles, so only
// an admin may set or clear the choice, and a choice beats the engine everywhere: Play, the
// disc's page, its length in lists. Anyone may still play any other title once, from the
// page. Asking a libbluray on this machine for a second opinion is an admin's action too,
// and only when the setting for it is on.

// settingSecondOpinion turns on asking a libbluray the machine's owner installed.
const settingSecondOpinion = "blurayLibbluray"

type secondOpinionState struct {
	Enabled   bool   `json:"enabled"`   // the setting is on
	Available bool   `json:"available"` // the module found a libbluray on this machine
	Version   string `json:"version,omitempty"`
	Reason    string `json:"reason,omitempty"` // why not, when not available
}

// ranking keeps one background analysis going at a time.
var ranking sync.Mutex

// engineOf names the module version in use for a disc's kind, as a kept ranking names it.
func (s *Server) engineOf(it *library.Item) string {
	id := "wittydvd"
	if isBluray(it) {
		id = "wittybluray"
	}
	if v := moduleVersion(s, id); v != "" {
		return id + " " + v
	}
	return ""
}

// moduleVersion is the version of a module in use, or "". Tests stand in for it.
var moduleVersion = func(s *Server, id string) string {
	if s.Modules == nil {
		return ""
	}
	return s.Modules.VersionInUse(id)
}

// A disc whose ranking failed isn't asked about again for a while: every visit to its page
// would otherwise start another helper on it, and a disc that makes a helper hang would
// hold one for the whole time limit each time. A new version of its module, or an admin's
// "Rank again", asks at once.
const (
	rankRetryAfter  = 15 * time.Minute
	maxRankFailures = 1024
)

type rankFailureKey struct {
	s  *Server
	id int64
}

type rankFailure struct {
	engine string
	at     time.Time
	err    string
}

var rankFailures = struct {
	sync.Mutex
	m map[rankFailureKey]rankFailure
}{m: map[rankFailureKey]rankFailure{}}

// rankFailed remembers that ranking an item failed with the engine in use.
func (s *Server) rankFailed(it *library.Item, err error) {
	rankFailures.Lock()
	defer rankFailures.Unlock()
	if len(rankFailures.m) >= maxRankFailures {
		// Bounded: what is past its time goes first, and if that frees nothing, all of it.
		for k, f := range rankFailures.m {
			if time.Since(f.at) > rankRetryAfter {
				delete(rankFailures.m, k)
			}
		}
		if len(rankFailures.m) >= maxRankFailures {
			clear(rankFailures.m)
		}
	}
	rankFailures.m[rankFailureKey{s, it.ID}] = rankFailure{engine: s.engineOf(it), at: time.Now(), err: err.Error()}
}

// rankFailedLately is the error of a recent failed ranking of the item with the engine in
// use, or "".
func (s *Server) rankFailedLately(it *library.Item) string {
	rankFailures.Lock()
	defer rankFailures.Unlock()
	f, ok := rankFailures.m[rankFailureKey{s, it.ID}]
	if !ok {
		return ""
	}
	if time.Since(f.at) > rankRetryAfter || f.engine != s.engineOf(it) {
		delete(rankFailures.m, rankFailureKey{s, it.ID})
		return ""
	}
	return f.err
}

func (s *Server) rankSucceeded(it *library.Item) {
	rankFailures.Lock()
	delete(rankFailures.m, rankFailureKey{s, it.ID})
	rankFailures.Unlock()
}

// rankDisc asks the item's engine for its ranking and keeps it with the item. second asks a
// libbluray too. The engines remember their answers for as long as the disc is the same.
func (s *Server) rankDisc(ctx context.Context, it *library.Item, second bool) error {
	err := s.rankDiscOnce(ctx, it, second)
	if err != nil && ctx.Err() == nil {
		s.rankFailed(it, err)
	} else if err == nil {
		s.rankSucceeded(it)
	}
	return err
}

func (s *Server) rankDiscOnce(ctx context.Context, it *library.Item, second bool) error {
	var rank *library.DiscRank
	if isBluray(it) {
		an, err := bdsession.Analyze(it.Path, second, s.Log)
		if err != nil {
			return err
		}
		rank = library.BlurayRank(an)
	} else {
		an, err := dvdsession.Analyze(it.Path)
		if err != nil {
			return err
		}
		rank = library.DVDRank(an)
		// A feature that this server can't lay out as one title plays by the disc's
		// own commands: its cell list is of no use here.
		if rank != nil && rank.Feature != nil && rank.Feature.Assembled && !it.Disc.Scrambled {
			if o, err := dvdsession.Open(it.Path); err == nil {
				if _, _, ok := o.Disc.FeatureTitle(an.Ranking.Feature, 1); !ok {
					rank.Feature.Segments = nil
				}
				_ = o.Close()
			} else {
				// The feature's cells stay as the engine gave them; playing it will say more.
				s.Log.DebugContext(ctx, "a disc couldn't be opened to lay out its feature", "cat", "discs", "item", it.ID, "err", err)
			}
		}
	}
	if rank == nil {
		return nil // an engine from before rankings
	}
	if it.Disc == nil {
		return nil
	}
	if it.Disc.Scrambled {
		rank.Protected = true
		if rank.Feature != nil {
			rank.Feature.Segments = nil
		}
	}
	if err := s.Store.SetDiscRank(ctx, it, rank); err != nil {
		return err
	}
	s.Log.InfoContext(ctx, "a disc was ranked", "cat", "discs", "item", it.ID, "engine", rank.Engine, "main", rank.Main,
		"confidence", rank.Confidence, "level", rank.Level, "protected", rank.Protected, "secondOpinion", second)
	return nil
}

// rankStale says the kept ranking isn't the engine in use's: there is none, or another
// version gave it.
func (s *Server) rankStale(it *library.Item) bool {
	if it.Disc == nil {
		return false
	}
	engine := s.engineOf(it)
	if engine == "" {
		return false // no engine to ask
	}
	return it.Disc.Rank == nil || it.Disc.Rank.Engine != engine
}

func (s *Server) secondOpinionState(ctx context.Context) *secondOpinionState {
	st := &secondOpinionState{Enabled: s.DB.GetSetting(ctx, settingSecondOpinion, "0") == "1"}
	if bdsession.Capability == nil {
		st.Reason = "WittyBluray isn't on this server."
		return st
	}
	c, err := bdsession.Capability()
	if err != nil {
		st.Reason = i18n.F("WittyBluray couldn't say: {error}", "error", err.Error())
		return st
	}
	st.Available, st.Version, st.Reason = c.Libbluray.Available, c.Libbluray.Version, c.Libbluray.Reason
	return st
}

// discEpisode finds an episode of the disc by its place in viewing order.
func discEpisode(d *library.DiscInfo, index int) *library.DiscEpisode {
	if d == nil || d.Rank == nil {
		return nil
	}
	for i := range d.Rank.Episodes {
		if d.Rank.Episodes[i].Index == index {
			return &d.Rank.Episodes[i]
		}
	}
	return nil
}

// libblurayState is whether a libbluray can give a second opinion, and where it is, for
// WittyBluray's settings.
func (s *Server) libblurayState(ctx context.Context) map[string]any {
	out := map[string]any{"libbluray": s.secondOpinionState(ctx)}
	if bdsession.Capability != nil {
		if c, err := bdsession.Capability(); err == nil {
			out["libblurayPath"] = c.Libbluray.Path
		}
	}
	return out
}
