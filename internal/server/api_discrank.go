// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/pace"
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

// discAnalysisView is what GET /api/items/{id}/disc/analysis answers.
type discAnalysisView struct {
	Kind string `json:"kind"` // dvd or bluray
	// Ranked: the engine in use gave a ranking. False with an engine from before
	// rankings: the main title is then the older rule's pick, with no reasons.
	Ranked bool `json:"ranked"`
	// Pending: the ranking is being worked out in the background, on a server set to go
	// easy on its machine. Ask again in a few seconds.
	Pending bool `json:"pending"`
	// Main is what Play plays: the chosen title, or else the engine's. EngineMain is the
	// engine's own pick. Both are -1 when there is none.
	Main       int                 `json:"main"`
	EngineMain int                 `json:"engineMain"`
	Chosen     *library.DiscChoice `json:"chosen"`
	// ChosenBy is the name of the account that chose, for admins.
	ChosenBy  string            `json:"chosenBy,omitempty"`
	Rank      *library.DiscRank `json:"rank"`
	CanChoose bool              `json:"canChoose"`
	// RankError says why the ranking couldn't be worked out lately. It is asked again after
	// a while, or at once by an admin's "Rank again".
	RankError string `json:"rankError,omitempty"`
	Protected bool   `json:"protected"`
	// SecondOpinion says whether a libbluray can be asked: Blu-rays, and admins, only.
	SecondOpinion *secondOpinionState `json:"secondOpinion,omitempty"`
}

type secondOpinionState struct {
	Enabled   bool   `json:"enabled"`   // the setting is on
	Available bool   `json:"available"` // the module found a libbluray on this machine
	Version   string `json:"version,omitempty"`
	Reason    string `json:"reason,omitempty"` // why not, when not available
}

// ranking keeps one background analysis going at a time.
var ranking sync.Mutex

func (s *Server) discItem(w http.ResponseWriter, r *http.Request) *library.Item {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	it, err := s.visibleItem(r, id)
	if err != nil || !isDiscKind(it) {
		writeErr(w, 404, "disc not found")
		return nil
	}
	return it
}

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
	return s.Store.SetDiscRank(ctx, it, rank)
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

func (s *Server) discAnalysis(r *http.Request, it *library.Item, pending bool) discAnalysisView {
	u := userFrom(r)
	v := discAnalysisView{Kind: "dvd", Main: -1, EngineMain: -1, Pending: pending, CanChoose: u.IsAdmin}
	if isBluray(it) {
		v.Kind = "bluray"
	}
	if d := it.Disc; d != nil {
		v.Protected = d.Scrambled
		v.Rank, v.Chosen, v.Ranked = d.Rank, d.Chosen, d.Rank != nil
		if len(d.Titles) > 0 || d.Rank != nil {
			v.Main, v.EngineMain = d.MainTitle, d.EngineMain()
		}
		if d.Chosen != nil && u.IsAdmin {
			_ = s.DB.QueryRowContext(r.Context(), `SELECT name FROM users WHERE id = ?`, d.Chosen.By).Scan(&v.ChosenBy)
		}
	}
	if isBluray(it) && u.IsAdmin {
		v.SecondOpinion = s.secondOpinionState(r.Context())
	}
	return v
}

func (s *Server) secondOpinionState(ctx context.Context) *secondOpinionState {
	st := &secondOpinionState{Enabled: s.DB.GetSetting(ctx, settingSecondOpinion, "0") == "1"}
	if bdsession.Capability == nil {
		st.Reason = "WittyBluray isn't on this server."
		return st
	}
	c, err := bdsession.Capability()
	if err != nil {
		st.Reason = "WittyBluray couldn't say: " + err.Error()
		return st
	}
	st.Available, st.Version, st.Reason = c.Libbluray.Available, c.Libbluray.Version, c.Libbluray.Reason
	return st
}

// GET /api/items/{id}/disc/analysis
func (s *Server) handleDiscAnalysis(w http.ResponseWriter, r *http.Request) {
	it := s.discItem(w, r)
	if it == nil {
		return
	}
	pending := false
	failed := s.rankFailedLately(it)
	if failed == "" && s.rankStale(it) {
		if s.DB.GetSetting(r.Context(), "lowPower", "0") == "1" {
			// Nobody waits for it on a machine that is being spared: it is worked out
			// behind, one disc at a time, and the page asks again.
			pending = true
			if ranking.TryLock() {
				go func(it library.Item) {
					defer ranking.Unlock()
					ctx, cancel := context.WithTimeout(pace.Background(context.WithoutCancel(r.Context())), 5*time.Minute)
					defer cancel()
					pace.Yield(ctx)
					if err := s.rankDisc(ctx, &it, false); err != nil {
						s.Log.WarnContext(ctx, "a disc couldn't be ranked", "cat", "discs", "item", it.ID, "err", err)
					}
				}(*it)
			}
		} else if err := s.rankDisc(r.Context(), it, false); err != nil {
			if writeModuleMissing(w, err) {
				return
			}
			// The page still has what the scan found.
			s.Log.WarnContext(r.Context(), "a disc couldn't be ranked", "cat", "discs", "item", it.ID, "err", err)
			failed = err.Error()
		}
	}
	v := s.discAnalysis(r, it, pending)
	if failed != "" {
		v.RankError = s.viewerError(r, errors.New(failed))
	}
	writeJSON(w, 200, v)
}

// POST /api/items/{id}/disc/analysis  {"secondOpinion":true}
// Works the ranking out again, for an admin. With secondOpinion, a libbluray on this
// machine is asked to compare, which the setting has to allow.
func (s *Server) handleDiscAnalyse(w http.ResponseWriter, r *http.Request) {
	it := s.discItem(w, r)
	if it == nil {
		return
	}
	var b struct {
		SecondOpinion bool `json:"secondOpinion"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.SecondOpinion {
		switch {
		case !isBluray(it):
			writeErr(w, 400, "Only a Blu-ray can have a second opinion.")
			return
		case s.DB.GetSetting(r.Context(), settingSecondOpinion, "0") != "1":
			writeErr(w, 403, "Asking libbluray is turned off. An admin can turn it on in Settings, Playback.")
			return
		case it.Disc != nil && it.Disc.Scrambled:
			writeErr(w, 403, "This disc is encrypted, and only this server's own reader looks at an encrypted disc.")
			return
		}
		if st := s.secondOpinionState(r.Context()); !st.Available {
			writeErr(w, 409, firstNonEmpty(st.Reason, "No libbluray was found on this machine."))
			return
		}
	}
	if err := s.rankDisc(r.Context(), it, b.SecondOpinion); err != nil {
		if writeModuleMissing(w, err) {
			return
		}
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	s.Log.InfoContext(r.Context(), "a disc was ranked again", "cat", "discs", "item", it.ID, "secondOpinion", b.SecondOpinion)
	writeJSON(w, 200, s.discAnalysis(r, it, false))
}

// PUT /api/items/{id}/disc/choice  {"ttn":3}
// Remembers a title as this disc's film, for everyone. Admins only.
func (s *Server) handleDiscChoose(w http.ResponseWriter, r *http.Request) {
	it := s.discItem(w, r)
	if it == nil {
		return
	}
	var b struct {
		TTN *int `json:"ttn"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.TTN == nil || *b.TTN < 0 {
		writeErr(w, 400, "Say which title: ttn.")
		return
	}
	if it.Disc == nil || !it.Disc.HasTitle(*b.TTN) {
		writeErr(w, 400, fmt.Sprintf("This disc has no title %d to play.", *b.TTN))
		return
	}
	if err := s.Store.ChooseDiscMain(r.Context(), it, *b.TTN, userFrom(r).ID); err != nil {
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	s.Log.InfoContext(r.Context(), "a disc's film was chosen", "cat", "discs", "item", it.ID, "ttn", *b.TTN)
	writeJSON(w, 200, s.discAnalysis(r, it, false))
}

// DELETE /api/items/{id}/disc/choice
// Forgets the choice: the engine's pick is the film again. Admins only.
func (s *Server) handleDiscUnchoose(w http.ResponseWriter, r *http.Request) {
	it := s.discItem(w, r)
	if it == nil {
		return
	}
	if err := s.Store.ChooseDiscMain(r.Context(), it, -1, userFrom(r).ID); err != nil {
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	s.Log.InfoContext(r.Context(), "a disc's chosen film was cleared", "cat", "discs", "item", it.ID)
	writeJSON(w, 200, s.discAnalysis(r, it, false))
}

// GET /api/bluray/capabilities
// What optional help WittyBluray has on this machine, for the settings page. Admins only:
// it names a library's path.
func (s *Server) handleBlurayCapabilities(w http.ResponseWriter, r *http.Request) {
	st := s.secondOpinionState(r.Context())
	out := map[string]any{"libbluray": st}
	if bdsession.Capability != nil {
		if c, err := bdsession.Capability(); err == nil {
			out["libblurayPath"] = c.Libbluray.Path
		}
	}
	writeJSON(w, 200, out)
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

// discWhy is the short form of a ranking for the disc page: one sentence and how sure.
func discWhy(d *library.DiscInfo) map[string]any {
	if d == nil || d.Rank == nil {
		return nil
	}
	rk := d.Rank
	why := strings.TrimSpace(rk.Why)
	out := map[string]any{"why": why, "confidence": rk.Confidence, "level": rk.Level, "engineMain": rk.Main, "decoys": rk.Decoys}
	if d.Chosen != nil {
		out["chosen"] = d.Chosen.TTN
	}
	return out
}
