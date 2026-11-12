// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/sduistore"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// What an administrator edits the server-driven UI with: the whole configuration, one
// page or the navigation at a time, a draft with a preview, presets, import and export,
// the history. Every route here is for administrators only. docs/SDUI.md, "The editor's
// contract", describes them.
//
// A change goes to the published configuration, where it needs If-Match with the ETag it
// was made against (428 without, 412 when someone saved in between), or with
// ?target=draft to the draft, which nobody sees until it is published.

func (s *Server) sduiAdminRoutes(m *http.ServeMux) {
	ad := s.requireAdmin
	m.Handle("GET /api/sdui/config", ad(s.handleSDUIConfig))
	m.Handle("PUT /api/sdui/config", ad(s.handleSDUIConfigPut))
	m.Handle("GET /api/sdui/config/history", ad(s.handleSDUIHistory))
	m.Handle("GET /api/sdui/config/diff", ad(s.handleSDUIConfigDiff))
	m.Handle("POST /api/sdui/config/rollback/{rev}", ad(s.handleSDUIRollback))
	m.Handle("GET /api/sdui/config/pages", ad(s.handleSDUIConfigPages))
	m.Handle("GET /api/sdui/config/pages/{page}", ad(s.handleSDUIConfigPage))
	m.Handle("PUT /api/sdui/config/pages/{page}", ad(s.handleSDUIConfigPagePut))
	m.Handle("DELETE /api/sdui/config/pages/{page}", ad(s.handleSDUIConfigPageDelete))
	m.Handle("GET /api/sdui/config/navigation", ad(s.handleSDUIConfigNav))
	m.Handle("PUT /api/sdui/config/navigation", ad(s.handleSDUIConfigNavPut))
	m.Handle("DELETE /api/sdui/config/navigation", ad(s.handleSDUIConfigNavDelete))
	m.Handle("POST /api/sdui/validate", ad(s.handleSDUIValidate))
	m.Handle("GET /api/sdui/presets", ad(s.handleSDUIPresets))
	m.Handle("POST /api/sdui/presets/{id}/apply", ad(s.handleSDUIPresetApply))
	m.Handle("GET /api/sdui/draft", ad(s.handleSDUIDraft))
	m.Handle("PUT /api/sdui/draft", ad(s.handleSDUIDraftPut))
	m.Handle("DELETE /api/sdui/draft", ad(s.handleSDUIDraftDelete))
	m.Handle("POST /api/sdui/draft/publish", ad(s.handleSDUIDraftPublish))
	m.Handle("POST /api/sdui/draft/preview", ad(s.handleSDUIPreview))
	m.Handle("GET /api/sdui/export", ad(s.handleSDUIExport))
	m.Handle("POST /api/sdui/import", ad(s.handleSDUIImport))
}

// writeUIStoreErr answers for an error from the store, and logs why a change to the UI
// configuration was refused. action is a fixed word for what was tried ("publish", "page
// saved", ...).
func (s *Server) writeUIStoreErr(w http.ResponseWriter, r *http.Request, action string, err error) {
	var inv *sduistore.InvalidError
	var mc *sduistore.MergeConflictError
	ctx := r.Context()
	switch {
	case errors.As(err, &mc):
		s.Log.InfoContext(ctx, "UI configuration change refused: both sides changed the same items", "cat", "sdui", "action", action, "by", adminName(r), "conflicts", len(mc.Items), "baseRevision", mc.Base, "currentRevision", mc.Current)
		writeJSON(w, 409, map[string]any{"error": mc.Error() + ": reload the draft and the published configuration, then publish again preferring one side (prefer=draft or prefer=published) or change the draft",
			"conflicts": mc.Items, "baseRevision": mc.Base, "currentRevision": mc.Current})
	case errors.Is(err, errNoSuchPage):
		writeErr(w, 404, err.Error())
	case errors.Is(err, sduistore.ErrConflict):
		s.Log.InfoContext(ctx, "UI configuration change refused: it changed since it was read", "cat", "sdui", "action", action, "by", adminName(r))
		writeErr(w, 412, err.Error()+": read it again and redo the change")
	case errors.Is(err, sduistore.ErrNoRevision):
		writeErr(w, 404, err.Error())
	case errors.As(err, &inv):
		s.Log.WarnContext(ctx, "UI configuration change refused as invalid", "cat", "sdui", "action", action, "by", adminName(r), "issues", len(inv.Issues))
		writeJSON(w, 400, map[string]any{"error": inv.Error(), "issues": inv.Issues})
	default:
		s.Log.ErrorContext(ctx, "could not save the UI configuration", "cat", "sdui", "action", action, "by", adminName(r), "err", err)
		writeErr(w, 500, err.Error())
	}
}

func adminName(r *http.Request) string {
	if u := userFrom(r); u != nil {
		return u.Name
	}
	return ""
}

// errNoSuchPage: a page an edit names isn't in the configuration, built in or a module's.
var errNoSuchPage = errors.New("no such page")

// sduiTarget is which configuration an edit is for.
func sduiTarget(r *http.Request) (draft bool, err error) {
	switch r.URL.Query().Get("target") {
	case "", "published":
		return false, nil
	case "draft":
		return true, nil
	}
	return false, errors.New("target: published or draft")
}

// sduiBase is the configuration an editor is looking at: the draft when it asks for it
// (the published one while there is no draft yet).
func (s *Server) sduiBase(r *http.Request, st *sduistore.Store) (*sduistore.Config, bool, error) {
	draft, err := sduiTarget(r)
	if err != nil {
		return nil, false, err
	}
	if draft {
		if d := st.Draft(); d != nil {
			return d, true, nil
		}
	}
	return st.Current(), draft, nil
}

// sduiMutate applies fn to the published configuration (If-Match required) or to the draft.
// action is fixed text for the log; attrs say what it was done to (e.g. "page", id).
func (s *Server) sduiMutate(w http.ResponseWriter, r *http.Request, action string, fn func(c *sduistore.Config) error, attrs ...any) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	draft, err := sduiTarget(r)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ifMatch := strings.TrimSpace(r.Header.Get("If-Match"))
	var saved *sduistore.Config
	if draft {
		base := st.Draft()
		if base == nil {
			base = st.Current()
		}
		next := base.Clone()
		if err := fn(next); err != nil {
			if errors.Is(err, errNoSuchPage) {
				s.writeUIStoreErr(w, r, action, err)
				return
			}
			writeErr(w, 400, err.Error())
			return
		}
		saved, err = st.SaveDraft(next, ifMatch, adminName(r))
	} else {
		if ifMatch == "" {
			writeErr(w, 428, "If-Match: send the ETag of the configuration this change was made to")
			return
		}
		saved, err = st.Update(ifMatch, adminName(r), fn)
	}
	if err != nil {
		var inv *sduistore.InvalidError
		if !errors.As(err, &inv) && !errors.Is(err, sduistore.ErrConflict) && !errors.Is(err, sduistore.ErrNoRevision) && !errors.Is(err, errNoSuchPage) {
			writeErr(w, 400, err.Error())
			return
		}
		s.writeUIStoreErr(w, r, action, err)
		return
	}
	where := "published"
	if draft {
		where = "draft"
	}
	s.Log.InfoContext(r.Context(), "UI configuration changed", append([]any{"cat", "sdui", "action", action, "by", adminName(r), "target", where, "revision", saved.Revision}, attrs...)...)
	writeJSONTagged(w, 200, saved.ETag, saved)
}

func sduiBody(r *http.Request, limit int64) ([]byte, error) {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("too large: at most " + strconv.FormatInt(limit, 10) + " bytes")
	}
	return b, nil
}

// GET /api/sdui/config: all of ui_config.json.
func (s *Server) handleSDUIConfig(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	c := st.Current()
	if notModified(w, r, c.ETag) {
		return
	}
	writeJSONTagged(w, 200, c.ETag, c)
}

// PUT /api/sdui/config: replace ui_config.json. If-Match must be the ETag it was read with.
func (s *Server) handleSDUIConfigPut(w http.ResponseWriter, r *http.Request) {
	b, err := sduiBody(r, sduistore.MaxBytes)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c, err := sduistore.Parse(b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.sduiMutate(w, r, "saved", func(next *sduistore.Config) error {
		*next = *c.Clone()
		return nil
	})
}

// GET /api/sdui/config/history: the revisions kept, newest first.
func (s *Server) handleSDUIHistory(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	h, err := st.History()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	c := st.Current()
	writeJSON(w, 200, map[string]any{"current": c.Revision, "etag": c.ETag, "keep": sduistore.MaxHistory, "revisions": h})
}

// sduiChange is one difference between two configurations.
type sduiChange struct {
	// Path is /pages/<id>, /navigation, /theme or /personalisation.
	Path string `json:"path"`
	// Change is added, removed or changed.
	Change string `json:"change"`
	// For a changed page: the sections added, removed and changed, by ID, and whether
	// the order of the sections both have differs.
	Added     []string `json:"added,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Changed   []string `json:"changed,omitempty"`
	Reordered bool     `json:"reordered,omitempty"`
	// Other names what else about the page differs (title, variants, ...).
	Other []string `json:"other,omitempty"`
}

func sameJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// configChanges lists what differs between two configurations, page by page and, inside
// a changed page, section by section.
func configChanges(from, to *sduistore.Config) []sduiChange {
	out := []sduiChange{}
	ids := map[string]bool{}
	for id := range from.Pages {
		ids[id] = true
	}
	for id := range to.Pages {
		ids[id] = true
	}
	names := make([]string, 0, len(ids))
	for id := range ids {
		names = append(names, id)
	}
	sort.Strings(names)
	for _, id := range names {
		a, inA := from.Pages[id]
		b, inB := to.Pages[id]
		switch {
		case !inA:
			out = append(out, sduiChange{Path: "/pages/" + id, Change: "added"})
		case !inB:
			out = append(out, sduiChange{Path: "/pages/" + id, Change: "removed"})
		case !sameJSON(a, b):
			ch := sduiChange{Path: "/pages/" + id, Change: "changed"}
			before := map[string]sdui.Section{}
			var orderA, orderB []string
			for _, sec := range a.Sections {
				before[sec.ID] = sec
			}
			after := map[string]bool{}
			for _, sec := range b.Sections {
				after[sec.ID] = true
				old, had := before[sec.ID]
				switch {
				case !had:
					ch.Added = append(ch.Added, sec.ID)
				case !sameJSON(old, sec):
					ch.Changed = append(ch.Changed, sec.ID)
				}
				if had {
					orderB = append(orderB, sec.ID)
				}
			}
			for _, sec := range a.Sections {
				if !after[sec.ID] {
					ch.Removed = append(ch.Removed, sec.ID)
				} else {
					orderA = append(orderA, sec.ID)
				}
			}
			ch.Reordered = strings.Join(orderA, " ") != strings.Join(orderB, " ")
			a.Sections, b.Sections = nil, nil
			for name, pair := range map[string][2]any{"title": {a.Title, b.Title}, "icon": {a.Icon, b.Icon}, "kind": {a.Kind, b.Kind}, "match": {a.Match, b.Match},
				"scope": {a.Scope, b.Scope}, "variants": {a.Variants, b.Variants}, "personal": {a.Personal, b.Personal}, "description": {a.Description, b.Description}, "ttlSeconds": {a.TTLSeconds, b.TTLSeconds}} {
				if !sameJSON(pair[0], pair[1]) {
					ch.Other = append(ch.Other, name)
				}
			}
			sort.Strings(ch.Other)
			out = append(out, ch)
		}
	}
	for name, pair := range map[string][2]any{"/navigation": {from.Navigation, to.Navigation}, "/theme": {from.Theme, to.Theme}, "/personalisation": {from.Personalisation, to.Personalisation}} {
		if !sameJSON(pair[0], pair[1]) {
			out = append(out, sduiChange{Path: name, Change: "changed"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// sduiRevision reads "current", "draft" or a kept revision's number.
func (s *Server) sduiRevision(st *sduistore.Store, name string) (*sduistore.Config, error) {
	switch name {
	case "", "current":
		return st.Current(), nil
	case "draft":
		if d := st.Draft(); d != nil {
			return d, nil
		}
		return nil, sduistore.ErrNoRevision
	}
	n, err := strconv.ParseInt(name, 10, 64)
	if err != nil || n < 1 {
		return nil, errors.New("a revision is a number, current or draft")
	}
	return st.RevisionConfig(n)
}

// sduiDiffRevision is the revision a diff names for c: its own for a published revision,
// and for the draft (whose own number only counts its saves) the published revision it
// was begun from, with draft true.
func sduiDiffRevision(st *sduistore.Store, c *sduistore.Config) (int64, bool) {
	if d := st.Draft(); d != nil && c == d {
		return d.BaseRevision, true
	}
	return c.Revision, false
}

// GET /api/sdui/config/diff?from=&to=: what differs between two revisions (a number,
// "current" or "draft"), page by page and section by section. "from" and "to" are the
// revisions; for the draft, the published revision it was begun from, with
// fromDraft/toDraft true.
func (s *Server) handleSDUIConfigDiff(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	q := r.URL.Query()
	from, err := s.sduiRevision(st, q.Get("from"))
	if err == nil {
		var to *sduistore.Config
		if to, err = s.sduiRevision(st, firstNonEmpty(q.Get("to"), "current")); err == nil {
			fromRev, fromDraft := sduiDiffRevision(st, from)
			toRev, toDraft := sduiDiffRevision(st, to)
			writeJSON(w, 200, map[string]any{"from": fromRev, "to": toRev, "fromDraft": fromDraft, "toDraft": toDraft, "changes": configChanges(from, to)})
			return
		}
	}
	if errors.Is(err, sduistore.ErrNoRevision) {
		writeErr(w, 404, err.Error())
		return
	}
	writeErr(w, 400, err.Error())
}

// POST /api/sdui/config/rollback/{rev}: make a kept revision current again, as a new one.
func (s *Server) handleSDUIRollback(w http.ResponseWriter, r *http.Request) {
	rev, ok := pathInt(r, "rev")
	if !ok || rev < 1 {
		writeErr(w, 400, "rev: a revision number")
		return
	}
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	saved, err := st.Rollback(rev, strings.TrimSpace(r.Header.Get("If-Match")), adminName(r))
	if err != nil {
		s.Log.WarnContext(r.Context(), "could not roll back the UI configuration", "cat", "sdui", "by", adminName(r), "to", rev, "err", err)
		s.writeUIStoreErr(w, r, "rollback", err)
		return
	}
	logging.Audit(r.Context(), s.Log, "UI configuration rolled back", "cat", "sdui", "by", adminName(r), "to", rev, "revision", saved.Revision)
	writeJSONTagged(w, 200, saved.ETag, saved)
}

// sduiPageEntry is one row of the editor's list of pages.
type sduiPageEntry struct {
	ID          string      `json:"id"`
	Kind        string      `json:"kind"`
	Title       string      `json:"title"`
	Icon        string      `json:"icon,omitempty"`
	Description string      `json:"description,omitempty"`
	Match       *sdui.Match `json:"match,omitempty"`
	// Source is "builtin" (as shipped), "module" (a module's, as it ships), "changed" (a
	// built-in or module page the configuration replaces; deleting it resets it) or
	// "custom" (the configuration's own).
	Source string `json:"source"`
	// Module is the module whose page it is, or which the page it replaces came from.
	Module   string `json:"module,omitempty"`
	Sections int    `json:"sections"`
	Variants int    `json:"variants,omitempty"`
}

func pageSource(cfg *sduistore.Config, id string, set *sdui.ModuleSet) string {
	_, builtin := sdui.StockPage(id)
	_, module := set.Page(id)
	builtin = builtin || module
	_, mine := cfg.Pages[id]
	if module && !mine {
		return "module"
	}
	switch {
	case mine && builtin:
		return "changed"
	case mine:
		return "custom"
	}
	return "builtin"
}

// GET /api/sdui/config/pages[?target=draft]: every page in force, built-in ones included.
func (s *Server) handleSDUIConfigPages(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cfg, draft, err := s.sduiBase(r, st)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out := []sduiPageEntry{}
	set := s.moduleSet(nil)
	for _, p := range effectivePages(cfg, set) {
		out = append(out, sduiPageEntry{ID: p.ID, Kind: p.Kind, Title: p.Title, Icon: p.Icon, Description: p.Description, Match: p.Match, Source: pageSource(cfg, p.ID, set), Module: set.PageModule(p.ID),
			Sections: len(p.Sections), Variants: len(p.Variants)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, 200, map[string]any{"etag": cfg.ETag, "draft": draft && st.Draft() != nil, "pages": out})
}

// GET /api/sdui/config/pages/{page}[?target=draft]: one page as authored, where it comes
// from, the built-in page it would reset to, and what the validator says of it.
func (s *Server) handleSDUIConfigPage(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cfg, _, err := s.sduiBase(r, st)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id := r.PathValue("page")
	set := s.moduleSet(nil)
	p, ok := effectivePage(cfg, id, set)
	if !ok {
		writeErr(w, 404, "no such page")
		return
	}
	out := map[string]any{"etag": cfg.ETag, "page": p, "source": pageSource(cfg, id, set), "issues": issuesOrEmpty(sdui.ValidatePageWith(&p, set))}
	if b, ok := sdui.StockPage(id); ok {
		out["builtin"] = set.WithInserts(b)
	} else if b, ok := set.Page(id); ok {
		out["builtin"] = b
	}
	writeJSON(w, 200, out)
}

func issuesOrEmpty(i []sdui.Issue) []sdui.Issue {
	if i == nil {
		return []sdui.Issue{}
	}
	return i
}

// PUT /api/sdui/config/pages/{page}[?target=draft]: save one page.
func (s *Server) handleSDUIConfigPagePut(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("page")
	b, err := sduiBody(r, sdui.MaxPageBytes)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p, err := sdui.DecodePage(b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if p.ID == "" {
		p.ID = id
	}
	if p.ID != id {
		writeErr(w, 400, "the page calls itself "+strconv.Quote(p.ID)+" but is being saved as "+strconv.Quote(id))
		return
	}
	s.sduiMutate(w, r, "page saved", func(c *sduistore.Config) error {
		if c.Pages == nil {
			c.Pages = map[string]sdui.Page{}
		}
		c.Pages[id] = p
		return nil
	}, "page", id)
}

// DELETE /api/sdui/config/pages/{page}[?target=draft]: remove the configuration's page: a
// built-in page goes back to how it ships ("reset to default"), a page of the
// administrator's own is gone. 404 for a page that is in none of them, whichever the target.
func (s *Server) handleSDUIConfigPageDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("page")
	set := s.moduleSet(nil)
	s.sduiMutate(w, r, "page reset", func(c *sduistore.Config) error {
		if _, ok := c.Pages[id]; !ok {
			_, builtin := sdui.StockPage(id)
			_, module := set.Page(id)
			if builtin || module {
				return nil // already as shipped
			}
			return errNoSuchPage
		}
		delete(c.Pages, id)
		return nil
	}, "page", id)
}

// GET /api/sdui/config/navigation[?target=draft]: the navigation as authored, and the
// built-in one.
func (s *Server) handleSDUIConfigNav(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cfg, _, err := s.sduiBase(r, st)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	nav, source := sdui.StockNavigation(), "builtin"
	if cfg.Navigation != nil {
		nav, source = *cfg.Navigation, "changed"
	}
	writeJSON(w, 200, map[string]any{"etag": cfg.ETag, "navigation": nav, "source": source, "builtin": sdui.StockNavigation()})
}

// PUT /api/sdui/config/navigation[?target=draft]: save the navigation.
func (s *Server) handleSDUIConfigNavPut(w http.ResponseWriter, r *http.Request) {
	b, err := sduiBody(r, sdui.MaxPageBytes)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	nav, err := sdui.DecodeNavigation(b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	set := s.moduleSet(nil)
	s.sduiMutate(w, r, "navigation saved", func(c *sduistore.Config) error {
		// Every page it opens is there now: the configuration's, a module's in use, or a
		// built-in one. (The file itself only warns, so a module turned off later doesn't
		// make it bad.)
		var errs []sdui.Issue
		for _, is := range sdui.ValidateNavigation(&nav, func(id string) bool { _, ok := effectivePage(c, id, set); return ok }) {
			if is.Severity == "error" {
				is.Path = "/navigation" + is.Path
				errs = append(errs, is)
			}
		}
		if len(errs) > 0 {
			return &sduistore.InvalidError{Issues: errs}
		}
		c.Navigation = &nav
		return nil
	})
}

// DELETE /api/sdui/config/navigation[?target=draft]: back to the built-in navigation.
func (s *Server) handleSDUIConfigNavDelete(w http.ResponseWriter, r *http.Request) {
	s.sduiMutate(w, r, "navigation reset", func(c *sduistore.Config) error {
		c.Navigation = nil
		return nil
	})
}

// POST /api/sdui/validate[?target=draft]: check a page, a navigation or a whole
// configuration without saving it. The body is {"page": ...}, {"navigation": ...} or
// {"config": ...}. A navigation may name the pages of the target's configuration.
func (s *Server) handleSDUIValidate(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	b, err := sduiBody(r, sduistore.MaxBytes)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var body struct {
		Page       json.RawMessage `json:"page"`
		Navigation json.RawMessage `json:"navigation"`
		Config     json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		writeErr(w, 400, "the body isn't JSON: "+err.Error())
		return
	}
	var issues []sdui.Issue
	decodeIssue := func(err error) {
		issues = append(issues, sdui.Issue{Path: "", Severity: "error", Message: err.Error(), Fix: "Check the spelling of every property against the catalogue: unknown properties are refused."})
	}
	switch {
	case body.Page != nil:
		if p, err := sdui.DecodePage(body.Page); err != nil {
			decodeIssue(err)
		} else {
			issues = sdui.ValidatePageWith(&p, s.moduleSet(nil))
		}
	case body.Navigation != nil:
		if n, err := sdui.DecodeNavigation(body.Navigation); err != nil {
			decodeIssue(err)
		} else {
			// The pages it may name are those of the configuration it is for: the
			// draft's with ?target=draft (the published one's while there is none).
			cfg, _, err := s.sduiBase(r, st)
			if err != nil {
				writeErr(w, 400, err.Error())
				return
			}
			set := s.moduleSet(nil)
			issues = sdui.ValidateNavigation(&n, func(id string) bool { _, ok := effectivePage(cfg, id, set); return ok })
		}
	case body.Config != nil:
		if c, err := sduistore.Parse(body.Config); err != nil {
			decodeIssue(err)
		} else {
			c.Version = sduistore.FormatVersion
			issues = st.Check(c)
		}
	default:
		writeErr(w, 400, "send {\"page\": ...}, {\"navigation\": ...} or {\"config\": ...}")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": !sdui.HasErrors(issues), "issues": issuesOrEmpty(issues)})
}

// GET /api/sdui/presets: the gallery of ready-made pages and sections.
func (s *Server) handleSDUIPresets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"presets": append(sdui.Presets(), s.moduleSet(nil).Presets()...)})
}

// POST /api/sdui/presets/{id}/apply[?target=draft]: put a page preset in the configuration,
// under the preset's own page ID.
func (s *Server) handleSDUIPresetApply(w http.ResponseWriter, r *http.Request) {
	p, ok := sdui.PresetByID(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "no such preset")
		return
	}
	if p.Page == nil {
		writeErr(w, 400, "that preset is one section: add it to a page in the editor")
		return
	}
	page := sdui.ClonePage(*p.Page)
	s.sduiMutate(w, r, "preset applied", func(c *sduistore.Config) error {
		if c.Pages == nil {
			c.Pages = map[string]sdui.Page{}
		}
		c.Pages[page.ID] = page
		return nil
	}, "preset", p.ID)
}

// GET /api/sdui/draft: the unpublished draft, 404 when there is none.
func (s *Server) handleSDUIDraft(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	d := st.Draft()
	if d == nil {
		writeErr(w, 404, "there is no draft")
		return
	}
	writeJSONTagged(w, 200, d.ETag, d)
}

// PUT /api/sdui/draft: keep a whole configuration as the draft. If-Match, when sent, is
// the draft's ETag (the published configuration's when there is no draft yet).
func (s *Server) handleSDUIDraftPut(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	b, err := sduiBody(r, sduistore.MaxBytes)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	c, err := sduistore.Parse(b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	saved, err := st.SaveDraft(c, strings.TrimSpace(r.Header.Get("If-Match")), adminName(r))
	if err != nil {
		s.writeUIStoreErr(w, r, "draft saved", err)
		return
	}
	s.Log.InfoContext(r.Context(), "UI draft saved", "cat", "sdui", "by", adminName(r), "revision", saved.Revision)
	writeJSONTagged(w, 200, saved.ETag, saved)
}

// DELETE /api/sdui/draft: forget the draft and every preview of it.
func (s *Server) handleSDUIDraftDelete(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := st.DiscardDraft(); err != nil {
		s.Log.ErrorContext(r.Context(), "could not discard the UI draft", "cat", "sdui", "by", adminName(r), "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "UI draft discarded", "cat", "sdui", "by", adminName(r))
	s.sdui.mu.Lock()
	s.sdui.preview = nil
	s.sdui.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/sdui/draft/publish[?prefer=draft|published]: apply what the draft changed to
// the configuration everyone gets. What was published since the draft was begun stays;
// 409 lists the items both changed, unless prefer says which side wins for them.
func (s *Server) handleSDUIDraftPublish(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	prefer := r.URL.Query().Get("prefer")
	if prefer != "" && prefer != "draft" && prefer != "published" {
		writeErr(w, 400, "prefer: draft or published")
		return
	}
	saved, err := st.PublishPreferring(strings.TrimSpace(r.Header.Get("If-Match")), adminName(r), prefer)
	if errors.Is(err, sduistore.ErrNoRevision) {
		writeErr(w, 404, "there is no draft to publish")
		return
	}
	if err != nil {
		s.writeUIStoreErr(w, r, "publish", err)
		return
	}
	s.sdui.mu.Lock()
	s.sdui.preview = nil
	s.sdui.mu.Unlock()
	logging.Audit(r.Context(), s.Log, "UI draft published", "cat", "sdui", "by", adminName(r), "revision", saved.Revision, "prefer", prefer)
	writeJSONTagged(w, 200, saved.ETag, saved)
}

// POST /api/sdui/draft/preview: a token that shows the draft to this administrator alone,
// on any device they are signed in on, for two hours. Sent as preview=<token> (or
// X-WMS-SDUI-Preview) with the page and navigation routes.
func (s *Server) handleSDUIPreview(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if st.Draft() == nil {
		writeErr(w, 404, "there is no draft to preview")
		return
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	tok := hex.EncodeToString(raw[:])
	now := time.Now()
	s.sdui.mu.Lock()
	if s.sdui.preview == nil {
		s.sdui.preview = map[string]previewToken{}
	}
	for k, v := range s.sdui.preview {
		if now.After(v.expires) {
			delete(s.sdui.preview, k)
		}
	}
	for len(s.sdui.preview) >= maxPreviewTokens { // the one that ends soonest makes room
		oldest, at := "", time.Time{}
		for k, v := range s.sdui.preview {
			if oldest == "" || v.expires.Before(at) {
				oldest, at = k, v.expires
			}
		}
		delete(s.sdui.preview, oldest)
	}
	s.sdui.preview[tok] = previewToken{uid: userFrom(r).ID, expires: now.Add(previewTokenFor)}
	s.sdui.mu.Unlock()
	writeJSON(w, 200, map[string]any{"token": tok, "expiresAt": now.Add(previewTokenFor).UTC().Format(time.RFC3339), "param": "preview", "header": "X-WMS-SDUI-Preview"})
}

// sduiExport is a page or a whole configuration as a file.
type sduiExport struct {
	// Format is "wittymedia-ui"; FormatVersion the configuration format's.
	Format        string `json:"format"`
	FormatVersion int    `json:"formatVersion"`
	// Kind is "page" or "config".
	Kind       string            `json:"kind"`
	ExportedAt string            `json:"exportedAt,omitempty"`
	Page       *sdui.Page        `json:"page,omitempty"`
	Config     *sduistore.Config `json:"config,omitempty"`
}

const sduiExportFormat = "wittymedia-ui"

// GET /api/sdui/export[?page=<id>][&target=draft]: one page, or the whole configuration,
// as a file to keep or to import into another server.
func (s *Server) handleSDUIExport(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cfg, _, err := s.sduiBase(r, st)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	out := sduiExport{Format: sduiExportFormat, FormatVersion: sduistore.FormatVersion, ExportedAt: time.Now().UTC().Format(time.RFC3339)}
	name := "wittymedia-ui.json"
	if id := r.URL.Query().Get("page"); id != "" {
		p, ok := effectivePage(cfg, id, s.moduleSet(nil))
		if !ok {
			writeErr(w, 404, "no such page")
			return
		}
		out.Kind, out.Page, name = "page", &p, "wittymedia-page-"+p.ID+".json"
	} else {
		c := cfg.Clone()
		c.Revision, c.ETag, c.UpdatedBy, c.BaseRevision = 0, "", "", 0
		out.Kind, out.Config = "config", c
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	writeJSON(w, 200, out)
}

// POST /api/sdui/import[?target=draft|published][&dryRun=1][&theme=1]: bring in a file made
// by export: a page (saved under its own ID) or a whole configuration (its pages and
// navigation replace the target's; its theme colours too, with theme=1). Into the draft
// unless target=published, which needs If-Match. dryRun only says what the validator
// thinks.
func (s *Server) handleSDUIImport(w http.ResponseWriter, r *http.Request) {
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	b, err := sduiBody(r, sduistore.MaxBytes)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var env struct {
		Format string          `json:"format"`
		Kind   string          `json:"kind"`
		Page   json.RawMessage `json:"page"`
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		writeErr(w, 400, "that file isn't JSON: "+err.Error())
		return
	}
	if env.Format != sduiExportFormat {
		writeErr(w, 400, "that isn't a file exported from a WittyMediaServer's interface settings (its format should be \""+sduiExportFormat+"\")")
		return
	}
	var page *sdui.Page
	var cfg *sduistore.Config
	switch {
	case env.Page != nil:
		p, err := sdui.DecodePage(env.Page)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		page = &p
	case env.Config != nil:
		if cfg, err = sduistore.Parse(env.Config); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	default:
		writeErr(w, 400, "the file holds neither a page nor a configuration")
		return
	}
	withTheme := qBool(r, "theme")
	apply := func(c *sduistore.Config) error {
		if page != nil {
			if c.Pages == nil {
				c.Pages = map[string]sdui.Page{}
			}
			c.Pages[page.ID] = *page
			return nil
		}
		in := cfg.Clone()
		c.Pages, c.Navigation, c.Personalisation = in.Pages, in.Navigation, in.Personalisation
		if withTheme {
			c.Theme = in.Theme
		}
		return nil
	}
	if qBool(r, "dryRun") {
		draft, _ := sduiTarget(r)
		base := st.Current()
		if d := st.Draft(); d != nil && (draft || r.URL.Query().Get("target") == "") {
			base = d
		}
		next := base.Clone()
		_ = apply(next)
		next.Version = sduistore.FormatVersion
		issues := st.Check(next)
		writeJSON(w, 200, map[string]any{"ok": !sdui.HasErrors(issues), "issues": issuesOrEmpty(issues), "changes": configChanges(base, next)})
		return
	}
	// Into the draft unless told otherwise: an imported file is looked at before it is live.
	if r.URL.Query().Get("target") == "" {
		q := r.URL.Query()
		q.Set("target", "draft")
		r.URL.RawQuery = q.Encode()
	}
	s.sduiMutate(w, r, "imported", apply)
}
