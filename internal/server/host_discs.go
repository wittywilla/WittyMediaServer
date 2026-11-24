// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"net/http"
	"strconv"
	"time"

	wittycd "github.com/wittywilla/wittymediaserver/internal/cd"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/pace"
)

// The host API's calls for the disc modules (module API 5, M5). A disc's page, its ranking, the
// choice of film, the names of its titles, and finding what an audio CD is are WittyDVD's,
// WittyBluray's and WittyCD's own routes and SDUI sources now; what they show still rests on what
// only this server keeps and checks:
//
//   - the disc columns of its item (the scan's ranking, an administrator's choice of film,
//     whether it is encrypted), which the server's own disc sessions play from;
//   - the disc engines' answers, read in a helper of their own and checked here (the
//     scramble and AACS guards, the answer checks), never taken on a module's word;
//   - asking MusicBrainz about a CD, and fetching its cover, which the scan does too: the
//     process that reads an image never has a network.
//
// So a module asks here, on behalf of the person whose request it serves, and is answered with
// that person's access. Each module asks only about its own kind of disc.

// discModuleOf is the module whose discs an item is: wittydvd, wittybluray, or "".
func discModuleOf(it *library.Item) string {
	switch {
	case isDVD(it):
		return wittyDVDID
	case isBluray(it):
		return wittyBlurayID
	}
	return ""
}

const (
	wittyDVDID    = "wittydvd"
	wittyBlurayID = "wittybluray"
	wittyCDID     = "wittycd"
)

// hostAsUser is r as the person a host call is for: the server's own checks (visibleItem,
// viewerError, access) then read them as for a request of theirs.
func (s *Server) hostAsUser(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	u, _, err := s.onBehalf(r)
	if err != nil || u == nil {
		writeJSON(w, 403, map[string]string{"error": "say who is asking"})
		return nil, false
	}
	return r.WithContext(context.WithValue(r.Context(), userKey, u)), true
}

// hostDiscItem is the disc a module asks about, if the person may see it and it is the module's.
func (s *Server) hostDiscItem(w http.ResponseWriter, r *http.Request, module string) *library.Item {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no such disc"})
		return nil
	}
	it, err := s.visibleItem(r, id)
	if err != nil || discModuleOf(it) != module {
		writeJSON(w, 404, map[string]string{"error": "no such disc"})
		return nil
	}
	return it
}

// hostDisc is what a disc module is told about one of its discs.
type hostDisc struct {
	ID         int64   `json:"id"`
	Kind       string  `json:"kind"` // dvd or bluray
	Title      string  `json:"title"`
	Label      string  `json:"label"`
	RuntimeSec float64 `json:"runtimeSec"`
	Scrambled  bool    `json:"scrambled"`
	// Main is what Play plays: the chosen title, or else the engine's; EngineMain the engine's
	// own pick. Both are -1 when there is none.
	Main       int                 `json:"main"`
	EngineMain int                 `json:"engineMain"`
	Chosen     *library.DiscChoice `json:"chosen"`
	ChosenBy   string              `json:"chosenBy,omitempty"` // for administrators
	// Rank is the kept ranking, without the feature's cells; FeatureSec is how long the film
	// plays when it runs through several titles (the ranking's main title), else 0.
	Rank       *library.DiscRank `json:"rank"`
	FeatureSec float64           `json:"featureSec,omitempty"`
	RankError  string            `json:"rankError,omitempty"`
	Pending    bool              `json:"pending"`
	// NoPlay: the person's screen time is up, so nothing offers to play.
	NoPlay bool `json:"noPlay"`
	// Page is the disc's page, with ?page=1: the film, the other titles worth watching and the
	// menus to jump into; PageError says why there is none ("encrypted", or what went wrong).
	Page      *hostDiscPage       `json:"page,omitempty"`
	PageError string              `json:"pageError,omitempty"`
	Second    *secondOpinionState `json:"secondOpinion,omitempty"`
}

type hostDiscPage struct {
	Main   *discTitle  `json:"main"`
	Extras []discTitle `json:"extras"`
	Menus  []discMenu  `json:"menus"`
	Hidden int         `json:"hidden"` // -1 without a ranking
}

// GET /host/v1/discs/{id}?page=1&rank=1
// rank=1 asks for the ranking first when the kept one isn't the engine in use's, as a disc's
// analysis does: on a server set to go easy, in the background, and the answer is pending.
func (s *Server) hostDiscGet(w http.ResponseWriter, r *http.Request, module string) {
	r, ok := s.hostAsUser(w, r)
	if !ok {
		return
	}
	it := s.hostDiscItem(w, r, module)
	if it == nil {
		return
	}
	q := r.URL.Query()
	pending, failed := false, ""
	if q.Get("rank") == "1" {
		pending, failed = s.rankIfStale(r, it)
		if it2, err := s.Store.Item(r.Context(), it.ID); err == nil {
			it = it2
		}
	}
	v := s.hostDiscOf(r, it)
	v.Pending = pending
	if failed != "" {
		v.RankError = s.viewerError(r, errors.New(failed))
	}
	if q.Get("page") == "1" {
		p, err := s.discPage(r.Context(), it)
		switch {
		case errors.Is(err, errDiscEncrypted):
			v.PageError = "encrypted"
		case err != nil:
			if writeModuleMissing(w, err) {
				return
			}
			v.PageError = s.viewerError(r, err)
		default:
			pg := &hostDiscPage{Main: p.Main, Extras: p.Extras, Menus: p.Menus, Hidden: p.Hidden}
			if pg.Extras == nil {
				pg.Extras = []discTitle{}
			}
			if pg.Menus == nil {
				pg.Menus = []discMenu{}
			}
			v.Page = pg
		}
	}
	writeJSON(w, 200, v)
}

// rankIfStale works the ranking out when the kept one is missing or another engine's.
func (s *Server) rankIfStale(r *http.Request, it *library.Item) (pending bool, failed string) {
	failed = s.rankFailedLately(it)
	if failed != "" || !s.rankStale(it) {
		return false, failed
	}
	if s.DB.GetSetting(r.Context(), "lowPower", "0") == "1" {
		// Nobody waits for it on a machine that is being spared: it is worked out behind,
		// one disc at a time, and the page asks again.
		if ranking.TryLock() {
			item, req := *it, logging.RequestFrom(r.Context())
			started := s.background("disc ranking", func(bg context.Context) {
				defer ranking.Unlock()
				ctx, cancel := context.WithTimeout(pace.Background(logging.WithRequest(bg, req)), 5*time.Minute)
				defer cancel()
				pace.Yield(ctx)
				if err := s.rankDisc(ctx, &item, false); err != nil && ctx.Err() == nil {
					s.Log.WarnContext(ctx, "a disc couldn't be ranked", "cat", "discs", "item", item.ID, "err", err)
				}
			}, "cat", "discs", "item", item.ID)
			if !started {
				ranking.Unlock()
			}
		}
		return true, ""
	}
	if err := s.rankDisc(r.Context(), it, false); err != nil {
		// The page still has what the scan found.
		s.Log.WarnContext(r.Context(), "a disc couldn't be ranked", "cat", "discs", "item", it.ID, "err", err)
		return false, err.Error()
	}
	return false, ""
}

// hostDiscOf is what a module is told about a disc, for the person r is from.
func (s *Server) hostDiscOf(r *http.Request, it *library.Item) hostDisc {
	u := userFrom(r)
	v := hostDisc{ID: it.ID, Kind: "dvd", Title: it.Title, RuntimeSec: it.RuntimeSec, Main: -1, EngineMain: -1}
	if isBluray(it) {
		v.Kind = "bluray"
	}
	if d := it.Disc; d != nil {
		v.Label, v.Scrambled, v.Chosen = d.Label, d.Scrambled, d.Chosen
		if len(d.Titles) > 0 || d.Rank != nil {
			v.Main, v.EngineMain = d.MainTitle, d.EngineMain()
		}
		if d.Rank != nil {
			rk := *d.Rank
			if rk.Feature != nil {
				f := *rk.Feature
				f.Segments = nil // the server's to play; nothing a page shows
				rk.Feature = &f
			}
			v.Rank = &rk
			if f := d.FeaturePlays(d.Rank.Main); f != nil {
				v.FeatureSec = f.DurationSec
			}
		}
		if d.Chosen != nil && u.IsAdmin {
			_ = s.DB.QueryRowContext(r.Context(), `SELECT name FROM users WHERE id = ?`, d.Chosen.By).Scan(&v.ChosenBy)
		}
	}
	if a := s.access(r); a != nil && !a.Time.Empty() {
		p := s.parentalInit()
		v.NoPlay = !a.Time.Check(s.clock(), p.tracker.Usage(r.Context(), u.ID)).Allowed
	}
	if isBluray(it) && u.IsAdmin {
		v.Second = s.secondOpinionState(r.Context())
	}
	return v
}

// hostDiscAdmin is a disc only an administrator may change.
func (s *Server) hostDiscAdmin(w http.ResponseWriter, r *http.Request, module string) (*http.Request, *library.Item) {
	r, ok := s.hostAsUser(w, r)
	if !ok {
		return nil, nil
	}
	if !userFrom(r).IsAdmin {
		writeJSON(w, 403, map[string]string{"error": "Only an administrator can do that."})
		return nil, nil
	}
	it := s.hostDiscItem(w, r, module)
	if it == nil {
		return nil, nil
	}
	return r, it
}

// POST /host/v1/discs/{id}/rank {"secondOpinion": bool}
// Works the ranking out again, for an administrator. With secondOpinion, a libbluray on this
// machine is asked to compare, which the server's setting has to allow.
func (s *Server) hostDiscRank(w http.ResponseWriter, r *http.Request, module string) {
	r, it := s.hostDiscAdmin(w, r, module)
	if it == nil {
		return
	}
	var b struct {
		SecondOpinion bool `json:"secondOpinion"`
	}
	if err := readJSON(r, &b); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if b.SecondOpinion {
		switch {
		case !isBluray(it):
			writeJSON(w, 400, map[string]string{"error": "Only a Blu-ray can have a second opinion."})
			return
		case s.DB.GetSetting(r.Context(), settingSecondOpinion, "0") != "1":
			writeJSON(w, 403, map[string]string{"error": "Asking libbluray is turned off. An admin can turn it on in Settings, DVDs and Blu-rays."})
			return
		case it.Disc != nil && it.Disc.Scrambled:
			writeJSON(w, 403, map[string]string{"error": "This disc is encrypted, and only this server's own reader looks at an encrypted disc."})
			return
		}
		if st := s.secondOpinionState(r.Context()); !st.Available {
			writeJSON(w, 409, map[string]string{"error": firstNonEmpty(st.Reason, i18n.T("No libbluray was found on this machine."))})
			return
		}
	}
	if err := s.rankDisc(r.Context(), it, b.SecondOpinion); err != nil {
		if writeModuleMissing(w, err) {
			return
		}
		writeJSON(w, 500, map[string]string{"error": s.viewerError(r, err)})
		return
	}
	s.Log.InfoContext(r.Context(), "a disc was ranked again", "cat", "discs", "item", it.ID, "secondOpinion", b.SecondOpinion, "module", module)
	s.hostDiscAnswer(w, r, it.ID)
}

// PUT /host/v1/discs/{id}/choice {"ttn": n}: n is the disc's film for everyone; -1 forgets the
// choice, and the engine's pick is the film again. Administrators only.
func (s *Server) hostDiscChoose(w http.ResponseWriter, r *http.Request, module string) {
	r, it := s.hostDiscAdmin(w, r, module)
	if it == nil {
		return
	}
	var b struct {
		TTN *int `json:"ttn"`
	}
	if err := readJSON(r, &b); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	switch {
	case b.TTN == nil || *b.TTN < -1:
		writeJSON(w, 400, map[string]string{"error": "Say which title: ttn, or -1 for the engine's pick."})
		return
	case *b.TTN >= 0 && (it.Disc == nil || !it.Disc.HasTitle(*b.TTN)):
		writeJSON(w, 400, map[string]string{"error": i18n.F("This disc has no title {value} to play.", "value", *b.TTN)})
		return
	}
	if err := s.Store.ChooseDiscMain(r.Context(), it, *b.TTN, userFrom(r).ID); err != nil {
		writeJSON(w, 500, map[string]string{"error": s.viewerError(r, err)})
		return
	}
	discPageForget(it.ID)
	s.Log.InfoContext(r.Context(), "a disc's film was chosen", "cat", "discs", "item", it.ID, "ttn", *b.TTN, "module", module)
	s.hostDiscAnswer(w, r, it.ID)
}

// hostDiscAnswer answers with the disc as it is now.
func (s *Server) hostDiscAnswer(w http.ResponseWriter, r *http.Request, id int64) {
	it, err := s.Store.Item(r.Context(), id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no such disc"})
		return
	}
	writeJSON(w, 200, s.hostDiscOf(r, it))
}

// GET /host/v1/discs/libbluray: whether a libbluray can give a second opinion, and where it is
// (it names a path on this machine, so for an administrator only). WittyBluray's settings show it.
func (s *Server) hostLibbluray(w http.ResponseWriter, r *http.Request, module string) {
	r, ok := s.hostAsUser(w, r)
	if !ok {
		return
	}
	if module != wittyBlurayID || !userFrom(r).IsAdmin {
		writeJSON(w, 403, map[string]string{"error": "Only an administrator can see that."})
		return
	}
	writeJSON(w, 200, s.libblurayState(r.Context()))
}

// ---------------------------------------------------------------- audio CDs

// hostCDItem is an audio CD the person may see, open, for WittyCD.
func (s *Server) hostCDItem(w http.ResponseWriter, r *http.Request, module string) (*http.Request, *library.Item, *wittycd.Disc, func()) {
	r, ok := s.hostAsUser(w, r)
	if !ok {
		return nil, nil, nil, nil
	}
	if module != wittyCDID || !userFrom(r).IsAdmin {
		writeJSON(w, 403, map[string]string{"error": "Only an administrator can do that."})
		return nil, nil, nil, nil
	}
	it, d, release, ok := s.cdItem(w, r)
	if !ok {
		return nil, nil, nil, nil
	}
	return r, it, d, release
}

// GET /host/v1/cds/{id}/matches: what an audio CD could be. Only the disc's table of contents
// is sent to MusicBrainz.
func (s *Server) hostCDMatches(w http.ResponseWriter, r *http.Request, module string) {
	r, it, d, release := s.hostCDItem(w, r, module)
	if it == nil {
		return
	}
	defer release()
	ms, err := s.Scanner.CDLookup(r.Context(), d)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	if ms == nil {
		ms = []library.CDMatch{}
	}
	writeJSON(w, 200, map[string]any{"itemId": it.ID, "discId": d.TOC().MusicBrainzID(), "matches": ms, "attribution": library.MusicBrainzAttribution})
}

// PUT /host/v1/cds/{id}/release {"releaseId": "..."}: use this release's names and cover;
// "" goes back to the disc's own names. Answers the disc as GET /api/cd/{id} does.
func (s *Server) hostCDRelease(w http.ResponseWriter, r *http.Request, module string) {
	r, it, d, release := s.hostCDItem(w, r, module)
	if it == nil {
		return
	}
	defer release()
	var b struct {
		ReleaseID string `json:"releaseId"`
	}
	if err := readJSON(r, &b); err != nil {
		writeJSON(w, 400, map[string]string{"error": "expected a JSON body"})
		return
	}
	lib, err := s.Store.Library(r.Context(), it.LibraryID)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if b.ReleaseID == "" {
		err = s.Scanner.ForgetCDMatch(r.Context(), lib, it, d)
	} else {
		var m *library.CDMatch
		if m, err = s.Scanner.CDRelease(r.Context(), d, b.ReleaseID); err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		err = s.Scanner.ApplyCDMatch(r.Context(), lib, it, d, m)
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.Log.InfoContext(r.Context(), "a CD was identified", "cat", "library", "item", it.ID, "release", b.ReleaseID, "module", module)
	v, err := s.cdViewOf(r, it, d)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, v)
}
