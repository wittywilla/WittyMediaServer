// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

// The module store's pages (admins only): what every store offers, Update All and installs with
// what they're built on, other stores, and a module's log. docs/MODULES.md has the rules.

func (s *Server) moduleStoreRoutes(m *http.ServeMux) {
	m.Handle("GET /api/modulestore", s.requireAdmin(s.handleStoreView))
	m.Handle("POST /api/modulestore/check", s.requireAdmin(s.handleStoreCheck))
	m.Handle("GET /api/modulestore/item/{store}/{id}", s.requireAdmin(s.handleStoreItem))
	m.Handle("POST /api/modulestore/install", s.requireAdmin(s.handleStoreInstall))
	m.Handle("POST /api/modulestore/update-all", s.requireAdmin(s.handleStoreUpdateAll))
	m.Handle("GET /api/modulestore/run", s.requireAdmin(s.handleStoreRun))
	m.Handle("POST /api/modulestore/run/undo", s.requireAdmin(s.handleStoreRunUndo))
	m.Handle("DELETE /api/modulestore/run", s.requireAdmin(s.handleStoreRunDismiss))
	m.Handle("GET /api/modulestore/modules/{id}/log", s.requireAdmin(s.handleStoreModuleLog))
	m.Handle("GET /api/modulestore/media/{store}/{sha}", s.requireAdmin(s.handleStoreMedia))
	m.Handle("POST /api/modulestore/stores/preview", s.requireAdmin(s.handleStorePreview))
	m.Handle("POST /api/modulestore/stores", s.requireAdmin(s.handleStoreAdd))
	m.Handle("DELETE /api/modulestore/stores/{id}", s.requireAdmin(s.handleStoreRemove))
	m.Handle("PUT /api/modulestore/stores/{id}/enabled", s.requireAdmin(s.handleStoreEnabled))
	m.Handle("POST /api/modulestore/stores/{id}/check", s.requireAdmin(s.handleStoreCheckOne))
	m.Handle("POST /api/modulestore/stores/{id}/fork/{module}", s.requireAdmin(s.handleStoreFork))
}

func (s *Server) noStore(w http.ResponseWriter) bool {
	if s.ModuleStores == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return true
	}
	return false
}

// writeOlderServer answers a refusal of a version made for an older server: 409, code
// "older-server", with each such version named in older (module, name, version, madeFor, server,
// warning), so a page can ask about each before sending confirmOlder. plan, when there is one, is
// what the install would do.
func writeOlderServer(w http.ResponseWriter, err error, plan []modmgr.RunItem, more ...*modmgr.ErrOlderServer) bool {
	var one *modmgr.ErrOlderServer
	if err != nil && !errors.As(err, &one) {
		return false
	}
	older := more
	if one != nil && len(older) == 0 {
		older = []*modmgr.ErrOlderServer{one}
	}
	if len(older) == 0 {
		return false
	}
	out := map[string]any{"error": older[0].Error(), "code": "older-server", "warning": older[0].Warning, "older": older}
	if plan != nil {
		out["plan"] = plan
	}
	writeJSON(w, 409, out)
	return true
}

// writeStoreErr answers with the plain words for what went wrong, and the kind of thing it was.
func writeStoreErr(w http.ResponseWriter, name string, err error) {
	if writeOlderServer(w, err, nil) {
		return
	}
	var need *modmgr.ErrNeedsConfirm
	if errors.As(err, &need) {
		writeJSON(w, 409, map[string]any{"error": need.Warning, "code": "confirm", "warning": need.Warning})
		return
	}
	var ov *modmgr.ErrNeedsOverride
	if errors.As(err, &ov) {
		writeJSON(w, 409, map[string]any{"error": ov.Error(), "code": "needs-override", "needsOverride": ov})
		return
	}
	var bad *modmgr.ErrBadOverride
	if errors.As(err, &bad) {
		writeJSON(w, 400, map[string]any{"error": bad.Reason, "code": "bad-override", "needsOverride": bad.Needs})
		return
	}
	var key *modmgr.NeedKey
	if errors.As(err, &key) {
		f := modmgr.Failure{Kind: modmgr.FailSignature, Title: "This store needs a key", Message: err.Error(), Try: []string{"Ask whoever runs the store for their public key, and paste it in."}}
		writeJSON(w, 422, map[string]any{"error": err.Error(), "code": "need-key", "failure": f})
		return
	}
	status := 409
	if errors.Is(err, modmgr.ErrUnknown) || errors.Is(err, modmgr.ErrUnknownStore) {
		status = 404
	}
	writeJSON(w, status, map[string]any{"error": err.Error(), "failure": modmgr.ExplainError(name, err)})
}

// GET /api/modulestore
func (s *Server) handleStoreView(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	v := s.ModuleStores.View()
	if kinds := r.URL.Query().Get("moduleKind"); kinds != "" {
		items := v.Items[:0]
		for _, it := range v.Items {
			if it.MatchesKinds(kinds) {
				items = append(items, it)
			}
		}
		v.Items = items
	}
	writeJSON(w, 200, v)
}

// POST /api/modulestore/check: look at every store now, because an admin asked. Answers younger
// than a minute are reused for this project's catalogue, as for the older button.
func (s *Server) handleStoreCheck(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	s.Modules.Check(r.Context(), time.Minute)
	s.ModuleStores.CheckAll(r.Context())
	writeJSON(w, 200, s.ModuleStores.View())
}

// GET /api/modulestore/item/{store}/{id}: a module's page.
func (s *Server) handleStoreItem(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	d, err := s.ModuleStores.Detail(r.PathValue("store"), r.PathValue("id"))
	if err != nil {
		writeStoreErr(w, "That module", err)
		return
	}
	writeJSON(w, 200, d)
}

// POST /api/modulestore/install {"id": "...", "confirm": true}: install a module and what it's
// built on. If it would bring anything else, it says what until the body says confirm. A version
// made for an older server in the plan is refused (409, "older-server", naming each) until the
// body says "confirmOlder": true. Progress is in the run and the module's status.
func (s *Server) handleStoreInstall(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	var b struct {
		ID           string `json:"id"`
		Confirm      bool   `json:"confirm"`
		ConfirmOlder bool   `json:"confirmOlder"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&b); err != nil || b.ID == "" {
		writeErr(w, 400, "say which module: {\"id\": \"...\"}")
		return
	}
	plan, err := s.ModuleStores.InstallPlan(b.ID)
	if err != nil {
		writeStoreErr(w, "That module", err)
		return
	}
	if len(plan) > 1 && !b.Confirm {
		writeJSON(w, 409, map[string]any{"error": "it needs other modules", "code": "plan", "plan": plan})
		return
	}
	if older := s.ModuleStores.OlderInPlan(plan); len(older) > 0 && !b.ConfirmOlder {
		s.Log.InfoContext(r.Context(), "module install asks about a version made for an older server", "cat", "modules-store", "module", b.ID, "version", older[0].Version, "madeFor", older[0].MadeFor)
		writeOlderServer(w, nil, plan, older...)
		return
	}
	run, err := s.ModuleStores.InstallWithNeedsOlder(b.ID, userFrom(r).Name, b.ConfirmOlder)
	if err != nil {
		writeStoreErr(w, "That module", err)
		return
	}
	if b.ConfirmOlder {
		for _, o := range s.ModuleStores.OlderInPlan(plan) {
			s.auditOlderServer(r, o.Module, o.Version)
		}
	}
	s.Log.InfoContext(r.Context(), "module install started", "cat", "modules-store", "module", b.ID, "by", userFrom(r).Name)
	writeJSON(w, 202, run)
}

// POST /api/modulestore/update-all
func (s *Server) handleStoreUpdateAll(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	run, err := s.ModuleStores.UpdateAll(userFrom(r).Name)
	if err != nil {
		writeStoreErr(w, "Update All", err)
		return
	}
	writeJSON(w, 202, run)
}

// GET /api/modulestore/run: the run going on, or the summary of the last.
func (s *Server) handleStoreRun(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	writeJSON(w, 200, map[string]any{"run": s.ModuleStores.CurrentRun()})
}

// POST /api/modulestore/run/undo {"password": "...", "code": "..."}: take the last Update All back.
// Going back can bring back a hole that was fixed, so it asks what going back one module asks.
func (s *Server) handleStoreRunUndo(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	var b moduleSecrets
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "unreadable request")
		return
	}
	if !s.moduleGate(w, r, b, "update-all") {
		return
	}
	run, err := s.ModuleStores.UndoRun(userFrom(r).Name)
	if err != nil {
		writeStoreErr(w, "Update All", err)
		return
	}
	s.auditModule(r, "module-undo", "update-all", "took the last Update All back")
	writeJSON(w, 200, run)
}

// DELETE /api/modulestore/run: put the last run's summary away.
func (s *Server) handleStoreRunDismiss(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	s.ModuleStores.DismissRun()
	w.WriteHeader(204)
}

// moduleLogLines is how much of a module's log the page shows.
const moduleLogLines = 200

// GET /api/modulestore/modules/{id}/log: what this module has logged lately, and what happened
// to it, as lines a person can read and copy. Records are redacted when they're written.
func (s *Server) handleStoreModuleLog(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	id := r.PathValue("id")
	var status *modmgr.Status
	for _, st := range s.Modules.Report().Modules {
		if st.ID == id {
			st := st
			status = &st
			break
		}
	}
	if status == nil {
		writeErr(w, 404, "there is no such module")
		return
	}
	type line struct {
		At    int64  `json:"at"`
		Level string `json:"level"`
		Text  string `json:"text"`
	}
	lines := []line{}
	if s.Logs != nil {
		page, err := s.Logs.Query(logging.Filter{MinLevel: slog.LevelDebug, Module: id, Limit: moduleLogLines})
		if err != nil {
			s.Log.WarnContext(r.Context(), "reading a module's log", "cat", "modules", "err", err)
		}
		for _, rec := range page.Entries {
			text := rec.Msg
			for _, a := range rec.Attrs {
				text += " " + a.K + "=" + a.V
			}
			lines = append(lines, line{rec.Time.UnixMilli(), rec.Level, text})
		}
	}
	for _, ev := range status.Events {
		text := ev.Kind
		if ev.Version != "" {
			text += " " + ev.Version
		}
		if ev.Detail != "" {
			text += ": " + ev.Detail
		}
		lines = append(lines, line{ev.At, "event", text})
	}
	sort.SliceStable(lines, func(a, b int) bool { return lines[a].At < lines[b].At })
	var text strings.Builder
	for _, l := range lines {
		text.WriteString(time.UnixMilli(l.At).UTC().Format("2006-01-02 15:04:05"))
		text.WriteString(" ")
		text.WriteString(l.Level)
		text.WriteString(" ")
		text.WriteString(l.Text)
		text.WriteString("\n")
	}
	out := map[string]any{"id": id, "name": status.Name, "lines": lines, "text": text.String()}
	writeJSON(w, 200, out)
}

// GET /api/modulestore/media/{store}/{sha}: a picture a catalogue lists, from this server. The
// address carries its hash, so it never changes. The page asks with a header: this route takes no
// token in its address, as only an admin calls it, and an admin's sign-in has no place in a log.
func (s *Server) handleStoreMedia(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	path, typ, err := s.ModuleStores.Media(r.Context(), r.PathValue("store"), r.PathValue("sha"))
	if err != nil {
		s.Log.DebugContext(r.Context(), "a store's picture wasn't served", "cat", "modules-store", "store", r.PathValue("store"), "err", err)
		writeErr(w, 404, "no such picture")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, 404, "no such picture")
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	_, _ = io.Copy(w, f)
}

type storeBody struct {
	URL     string `json:"url"`
	Key     string `json:"key"`
	Private bool   `json:"private"`
	Agreed  bool   `json:"agreed"`
	moduleSecrets
}

// POST /api/modulestore/stores/preview: fetch a store and say what adding it would mean, with the
// warning. Nothing is kept. An address on the admin's own network asks for the password first.
func (s *Server) handleStorePreview(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	var b storeBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b); err != nil {
		writeErr(w, 400, "unreadable request")
		return
	}
	// From here a stolen session could otherwise look around this machine and its network, one
	// address at a time, by what comes back.
	if b.Private && !s.moduleGate(w, r, b.moduleSecrets, "store") {
		return
	}
	p, err := s.ModuleStores.Preview(r.Context(), b.URL, b.Key, b.Private)
	if err != nil {
		writeStoreErr(w, "That store", err)
		return
	}
	writeJSON(w, 200, p)
}

// POST /api/modulestore/stores: add a store, pinning the key in the body. Refused unless the body
// says the warning was agreed to, and with the password: from then on, whoever holds that key can
// offer code for this server to run.
func (s *Server) handleStoreAdd(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	var b storeBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&b); err != nil {
		writeErr(w, 400, "unreadable request")
		return
	}
	if !b.Agreed {
		writeErr(w, 400, "the warning has to be agreed to first")
		return
	}
	if !s.moduleGate(w, r, b.moduleSecrets, "store") {
		return
	}
	info, err := s.ModuleStores.Add(r.Context(), b.URL, b.Key, b.Private, userFrom(r).Name)
	if err != nil {
		writeStoreErr(w, "That store", err)
		return
	}
	s.auditModule(r, "store-added", "store", b.URL)
	writeJSON(w, 201, info)
}

// DELETE /api/modulestore/stores/{id}: forget a store. Anything using it goes back to the official
// version first.
func (s *Server) handleStoreRemove(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	if err := s.ModuleStores.Remove(r.PathValue("id"), userFrom(r).Name); err != nil {
		writeStoreErr(w, "That store", err)
		return
	}
	s.auditModule(r, "store-removed", "store", r.PathValue("id"))
	w.WriteHeader(204)
}

// PUT /api/modulestore/stores/{id}/enabled {"on": false}
func (s *Server) handleStoreEnabled(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	var b struct {
		On *bool `json:"on"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&b); err != nil || b.On == nil {
		writeErr(w, 400, "say whether it's on: {\"on\": true}")
		return
	}
	if err := s.ModuleStores.SetEnabled(r.PathValue("id"), *b.On, userFrom(r).Name); err != nil {
		writeStoreErr(w, "That store", err)
		return
	}
	w.WriteHeader(204)
}

// POST /api/modulestore/stores/{id}/check: look at one store now. A check that fails answers
// normally: the problem is on the store, which is what the page shows.
func (s *Server) handleStoreCheckOne(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	id := r.PathValue("id")
	if err := s.ModuleStores.Refresh(r.Context(), id); errors.Is(err, modmgr.ErrUnknownStore) {
		writeStoreErr(w, "That store", err)
		return
	}
	for _, info := range s.ModuleStores.List() {
		if info.ID == id {
			writeJSON(w, 200, info)
			return
		}
	}
	writeErr(w, 404, "there is no such store")
}

// POST /api/modulestore/stores/{id}/fork/{module} {"confirm": true, "password": "..."}: use a
// store's version of a module this server knows. Answered 409 with the warning until the body says
// confirm, and that asks for the password.
func (s *Server) handleStoreFork(w http.ResponseWriter, r *http.Request) {
	if s.noStore(w) {
		return
	}
	var b struct {
		Confirm     bool          `json:"confirm"`
		Fingerprint string        `json:"fingerprint"` // of the key the admin was shown
		Override    *overrideBody `json:"override"`    // the weak-sandbox go-ahead, on a Mac
		moduleSecrets
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "unreadable request")
		return
	}
	if b.Confirm && !s.moduleGate(w, r, b.moduleSecrets, r.PathValue("module")) {
		return
	}
	if err := s.ModuleStores.UseFork(r.PathValue("id"), r.PathValue("module"), userFrom(r).Name, b.Confirm, b.Fingerprint, b.Override.accept(userFrom(r).Name)); err != nil {
		if b.Override != nil {
			s.auditOverride(r, "module override install failed", r.PathValue("module"), "store", r.PathValue("id"), "overrides", b.Override.kinds(), "err", err)
		}
		writeStoreErr(w, "That module", err)
		return
	}
	s.Log.WarnContext(r.Context(), "a module now comes from another store", "cat", "modules-store", "module", r.PathValue("module"), "store", r.PathValue("id"), "by", userFrom(r).Name, "overrides", b.Override.kinds())
	s.auditModule(r, "module-source", r.PathValue("module"), "now from store "+r.PathValue("id")+b.Override.words())
	w.WriteHeader(202)
}
