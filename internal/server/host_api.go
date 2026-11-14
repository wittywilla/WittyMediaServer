// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/host"
	"github.com/wittywilla/wittymodule/service"
)

// The host API (module API 5, docs/MODULES-API5.md): how a module's service asks this server for
// what only the server has. It listens on this machine only, on an address of its own that is
// never the server's public one, and each service is given a secret of its own when it starts
// (modules.Manager.SetHost). Every call made on someone's behalf is answered with that
// person's access and no more, and a module only ever sees libraries of the kinds it serves.

// hostModules is what the host API asks the module manager; tests stand in for it.
type hostModules interface {
	HostModule(token string) (string, bool)
	Libraries(id string) []string
	Integration(id string) *wittymodule.Integration
	Name(id string) string
	ModuleReport(id string, working bool, reason string) error
}

func (s *Server) hostMods() hostModules {
	if s.hostModsForTest != nil {
		return s.hostModsForTest
	}
	return s.Modules
}

// HostAPI is the host API's handler, for StartHostAPI and tests.
func (s *Server) HostAPI() http.Handler {
	mux := http.NewServeMux()
	h := func(pattern string, f func(w http.ResponseWriter, r *http.Request, module string)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Witty-Host", "1") // a refusal of the host API's, not an unknown call
			module, ok := s.hostMods().HostModule(r.Header.Get(host.HeaderToken))
			if !ok {
				writeJSON(w, 403, map[string]string{"error": "not a module's call"})
				return
			}
			f(w, r, module)
		})
	}
	b := host.Base
	h("GET "+b+"/hello", s.hostHello)
	h("GET "+b+"/items/{id}", s.hostItem)
	h("GET "+b+"/items", s.hostItems)
	h("GET "+b+"/items/counts", s.hostCounts)
	h("GET "+b+"/items/facets", s.hostFacets)
	h("GET "+b+"/libraries", s.hostLibraries)
	h("GET "+b+"/may", s.hostMay)
	h("GET "+b+"/settings", s.hostSettings)
	h("GET "+b+"/secrets/{name}", s.hostSecret)
	h("POST "+b+"/images", s.hostImage)
	h("POST "+b+"/sessions", s.hostSessionStart)
	h("DELETE "+b+"/sessions/{id}", s.hostSessionEnd)
	h("GET "+b+"/users", s.hostUsers)
	h("GET "+b+"/tools", s.hostTools)
	h("POST "+b+"/report", s.hostReport)
	h("GET "+b+"/events", s.hostEventsWait)
	h("POST "+b+"/tasks/{name}", s.hostTask)
	h("POST "+b+"/audit", s.hostAudit)
	h("GET "+b+"/audit", s.hostAuditLog)
	h("GET "+b+"/setup", s.hostSetupState)
	h("POST "+b+"/setup/{id}", s.hostSetupDone)
	return mux
}

// StartHostAPI listens for the host API on this machine, and tells the module manager where.
func (s *Server) StartHostAPI() error {
	if s.Modules == nil {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s.HostAPI(), ReadHeaderTimeout: 10 * time.Second}
	logging.Go("host api", func() { _ = srv.Serve(ln) }, "cat", "modules")
	s.Modules.SetHost("http://" + ln.Addr().String())
	s.hostEvents().publish("server.started", nil)
	return nil
}

// onBehalf is the person a host call is for, and their access; nil for a call on nobody's.
func (s *Server) onBehalf(r *http.Request) (*auth.User, *library.Access, error) {
	v := r.Header.Get(host.HeaderOnUser)
	if v == "" {
		return nil, nil, nil
	}
	id, err := strconv.ParseInt(v, 10, 64)
	if err != nil || id <= 0 {
		return nil, nil, errors.New("not a person")
	}
	u, err := s.Auth.User(r.Context(), id)
	if err != nil || u == nil {
		return nil, nil, errors.New("no such person")
	}
	return u, s.accessOf(r.Context(), u), nil
}

func (s *Server) hostHello(w http.ResponseWriter, r *http.Request, module string) {
	machine := map[string]bool{
		// The config file allows custom cores, and the machine's lock doesn't keep them off.
		"gamingCustomCores": s.Cfg != nil && s.Cfg.GamingCustomCores && os.Getenv("WMS_GAMING_NO_CUSTOM_CORES") != "1",
		// The built-in test console is offered: a developer's server, or WMS_GAMING_TESTCORE=1.
		"gamingTestCore": s.testingGames(),
	}
	writeJSON(w, 200, host.Hello{Module: module, Server: s.Version, Offers: modmgr.Offers(), Machine: machine})
}

// servesKind says whether a module serves libraries of a type.
func (s *Server) servesKind(module, kind string) bool {
	return slices.Contains(s.hostMods().Libraries(module), kind)
}

func (s *Server) hostItem(w http.ResponseWriter, r *http.Request, module string) {
	u, a, err := s.onBehalf(r)
	if err != nil {
		writeJSON(w, 403, map[string]string{"error": err.Error()})
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no such item"})
		return
	}
	it, err := s.Store.Item(r.Context(), id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no such item"})
		return
	}
	lib, err := s.Store.Library(r.Context(), it.LibraryID)
	if err != nil || !s.servesKind(module, lib.Type) {
		writeJSON(w, 404, map[string]string{"error": "no such item"}) // not one of its kinds of library
		return
	}
	if u != nil && a != nil {
		var sh *library.Show
		if it.ShowID != nil {
			sh, _ = s.Store.ShowFacts(r.Context(), *it.ShowID)
		}
		if !a.AllowsItem(it, sh) {
			writeJSON(w, 403, map[string]string{"error": "not for this person"})
			return
		}
	}
	writeJSON(w, 200, s.moduleItemFull(r.Context(), it))
}

// hostItemSorts are the orders a module may list items in.
var hostItemSorts = []string{"", "added", "rating", "oldest", "title"}

// hostItems lists the items of the module's kinds of library a person may see. With full=1
// each is the server's own brief item, as GET /api/v1/items gives it (poster, rating, genres
// and the rest); otherwise the module's shape of it, without its files.
func (s *Server) hostItems(w http.ResponseWriter, r *http.Request, module string) {
	u, a, err := s.onBehalf(r)
	if err != nil {
		writeJSON(w, 403, map[string]string{"error": err.Error()})
		return
	}
	q := r.URL.Query()
	libs, err := s.Store.Libraries(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	want, _ := strconv.ParseInt(q.Get("library"), 10, 64)
	kind := q.Get("libraryKind")
	limit := min(max(qIntOf(q.Get("limit"), 100), 1), 500)
	offset := max(qIntOf(q.Get("offset"), 0), 0)
	sort := q.Get("sort")
	if !slices.Contains(hostItemSorts, sort) {
		writeJSON(w, 400, map[string]string{"error": "items sort by added, rating, oldest or title"})
		return
	}
	var ids []int64
	if v := q.Get("ids"); v != "" {
		for _, p := range strings.Split(v, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
			if err != nil || id <= 0 || len(ids) >= 500 {
				writeJSON(w, 400, map[string]string{"error": "ids are up to 500 item numbers, with commas between"})
				return
			}
			ids = append(ids, id)
		}
	}
	minRating, _ := strconv.ParseFloat(q.Get("minRating"), 64)
	full := q.Get("full") == "1"
	var found []*library.Item
	for _, l := range libs {
		if !s.servesKind(module, l.Type) || (want != 0 && l.ID != want) || (kind != "" && l.Type != kind) {
			continue
		}
		iq := library.ItemQuery{LibraryID: l.ID, Kind: q.Get("kind"), Search: q.Get("search"), Console: q.Get("facet.console"),
			Sort: sort, Desc: q.Get("desc") == "1", MinRating: minRating, Genre: q.Get("genre"), Series: q.Get("series"), IDs: ids,
			Access: a, Brief: true, NoTotal: true, Limit: offset + limit}
		items, _, err := s.Store.Items(r.Context(), iq)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		found = append(found, items...)
	}
	if offset >= len(found) {
		found = nil
	} else {
		found = found[offset:min(len(found), offset+limit)]
	}
	if full {
		if u != nil {
			_ = s.Store.AttachProgress(r.Context(), u.ID, found)
		}
		if u == nil || !u.IsAdmin {
			for _, it := range found {
				it.Path = ""
				it.Poster, it.Backdrop = artForViewer(it.Poster), artForViewer(it.Backdrop)
			}
		}
		if found == nil {
			found = []*library.Item{}
		}
		writeJSON(w, 200, map[string]any{"items": found})
		return
	}
	var out []*service.Item
	for _, it := range found {
		m := moduleItem(it)
		m.Files = nil // a list doesn't carry files: ask for the item
		out = append(out, m)
	}
	writeJSON(w, 200, map[string]any{"items": out})
}

// hostFacets is what a games library of the module's holds, as the person asking may see it:
// the same counts GET /api/v1/games/facets gives.
func (s *Server) hostFacets(w http.ResponseWriter, r *http.Request, module string) {
	_, a, err := s.onBehalf(r)
	if err != nil {
		writeJSON(w, 403, map[string]string{"error": err.Error()})
		return
	}
	id, _ := strconv.ParseInt(r.URL.Query().Get("library"), 10, 64)
	lib, err := s.Store.Library(r.Context(), id)
	if err != nil || lib.Type != "games" || !s.servesKind(module, lib.Type) {
		writeJSON(w, 404, map[string]string{"error": "no such library"})
		return
	}
	f, err := s.Store.GameFacets(r.Context(), lib.ID, "", a)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, f)
}

// hostLibraries lists the libraries of the module's kinds, with their folders: for its own work
// (a BIOS sweep), not on anyone's behalf.
func (s *Server) hostLibraries(w http.ResponseWriter, r *http.Request, module string) {
	libs, err := s.Store.Libraries(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := []host.Library{}
	for _, l := range libs {
		if s.servesKind(module, l.Type) {
			out = append(out, host.Library{ID: l.ID, Name: l.Name, Kind: l.Type, Console: l.Console, Paths: l.Paths})
		}
	}
	writeJSON(w, 200, map[string]any{"libraries": out})
}

// hostCounts counts games by console in the module's games libraries (or one of them, library=),
// as the person asking may see them (everything, with nobody named). Only the console facet is
// counted today.
func (s *Server) hostCounts(w http.ResponseWriter, r *http.Request, module string) {
	_, a, err := s.onBehalf(r)
	if err != nil {
		writeJSON(w, 403, map[string]string{"error": err.Error()})
		return
	}
	kind, by := r.URL.Query().Get("libraryKind"), r.URL.Query().Get("by")
	want, _ := strconv.ParseInt(r.URL.Query().Get("library"), 10, 64)
	if want != 0 {
		// One library: of its kind, which has to be one the module serves.
		lib, err := s.Store.Library(r.Context(), want)
		if err != nil || !s.servesKind(module, lib.Type) || (kind != "" && kind != lib.Type) {
			writeJSON(w, 404, map[string]string{"error": "no such library"})
			return
		}
		kind = lib.Type
	}
	if by != "console" || kind != "games" || !s.servesKind(module, kind) {
		writeJSON(w, 400, map[string]string{"error": "only games, by console, can be counted"})
		return
	}
	libs, err := s.Store.Libraries(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	out := map[string]int{}
	for _, l := range libs {
		if l.Type != kind || (want != 0 && l.ID != want) {
			continue
		}
		counts, err := s.Store.GameCounts(r.Context(), l.ID, a)
		if err != nil {
			continue
		}
		for id, n := range counts {
			out[id] += n
		}
	}
	writeJSON(w, 200, map[string]any{"counts": out})
}

func qIntOf(v string, def int) int {
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}
	return def
}

func (s *Server) hostMay(w http.ResponseWriter, r *http.Request, module string) {
	u, a, err := s.onBehalf(r)
	if err != nil || u == nil {
		writeJSON(w, 403, map[string]string{"error": "say who is asking"})
		return
	}
	f := r.URL.Query().Get("feature")
	may := a == nil || a.Can(f)
	if may && a != nil && !a.Time.Empty() {
		p := s.parentalInit()
		may = a.Time.Check(s.clock(), p.tracker.Usage(r.Context(), u.ID)).Allowed
	}
	writeJSON(w, 200, map[string]bool{"may": may})
}

func (s *Server) hostSettings(w http.ResponseWriter, r *http.Request, module string) {
	u, _, err := s.onBehalf(r)
	if err != nil {
		writeJSON(w, 403, map[string]string{"error": err.Error()})
		return
	}
	if u == nil {
		// Everyone's: the server's own values, with defaults, and no one's own.
		u = &auth.User{}
	}
	r = r.WithContext(context.WithValue(r.Context(), userKey, u))
	out := s.moduleSettingValues(r, module)
	if out == nil {
		out = map[string]any{}
	}
	writeJSON(w, 200, out)
}

// secretKey is where an administrator's secret for a module is kept.
func secretKey(module, name string) string { return "module." + module + ".secret." + name }

func (s *Server) hostSecret(w http.ResponseWriter, r *http.Request, module string) {
	name := r.PathValue("name")
	if !validSecretName(name) {
		writeJSON(w, 404, map[string]string{"error": "no such secret"})
		return
	}
	writeJSON(w, 200, map[string]string{"value": s.DB.GetSetting(r.Context(), secretKey(module, name), "")})
}

func validSecretName(n string) bool {
	if n == "" || len(n) > 64 {
		return false
	}
	for _, c := range n {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

// maxHostImage bounds a picture a module keeps.
const maxHostImage = 8 << 20

// hostImage keeps a picture for a module in the image folder, named by its content, and says
// where pages can show it (/api/module-images/<module>/<name>).
func (s *Server) hostImage(w http.ResponseWriter, r *http.Request, module string) {
	b, err := io.ReadAll(io.LimitReader(r.Body, maxHostImage+1))
	if err != nil || len(b) == 0 || len(b) > maxHostImage {
		writeJSON(w, 400, map[string]string{"error": "a picture of up to 8 MB"})
		return
	}
	ext := ""
	switch ct := http.DetectContentType(b); ct {
	case "image/png":
		ext = ".png"
	case "image/jpeg":
		ext = ".jpg"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	default:
		writeJSON(w, 400, map[string]string{"error": "not a picture this server shows (" + ct + ")"})
		return
	}
	sum := sha256.Sum256(b)
	name := hex.EncodeToString(sum[:16]) + ext
	dir := filepath.Join(s.Cfg.ImagesDir, "modules", module)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]string{"url": "/api/module-images/" + module + "/" + name})
}

// handleModuleImage serves a picture a module kept.
func (s *Server) handleModuleImage(w http.ResponseWriter, r *http.Request) {
	module, name := r.PathValue("module"), r.PathValue("name")
	if !validSecretName(module) || !validSecretName(name) || strings.Count(name, ".") != 1 {
		writeErr(w, 404, "not found")
		return
	}
	path := filepath.Join(s.Cfg.ImagesDir, "modules", module, name)
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable") // named by what it holds
	http.ServeFile(w, r, path)
}

// ---------------------------------------------------------------- events

// hostEventLog keeps the last events for modules to read, waiting for new ones.
type hostEventLog struct {
	mu     sync.Mutex
	seq    int64
	events []host.Event
	wake   chan struct{}
}

const hostEventsKept = 512

func (s *Server) hostEvents() *hostEventLog {
	s.hostEvMu.Lock()
	defer s.hostEvMu.Unlock()
	if s.hostEv == nil {
		s.hostEv = &hostEventLog{wake: make(chan struct{})}
	}
	return s.hostEv
}

// publish tells every module something happened.
func (l *hostEventLog) publish(typ string, data any) {
	raw, _ := json.Marshal(data)
	l.mu.Lock()
	l.seq++
	l.events = append(l.events, host.Event{Seq: l.seq, Type: typ, Data: raw})
	if len(l.events) > hostEventsKept {
		l.events = l.events[len(l.events)-hostEventsKept:]
	}
	close(l.wake)
	l.wake = make(chan struct{})
	l.mu.Unlock()
}

func (l *hostEventLog) after(seq int64) ([]host.Event, int64, chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []host.Event
	for _, e := range l.events {
		if e.Seq > seq {
			out = append(out, e)
		}
	}
	return out, l.seq, l.wake
}

func (s *Server) hostEventsWait(w http.ResponseWriter, r *http.Request, module string) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	wait, _ := time.ParseDuration(r.URL.Query().Get("wait"))
	wait = min(max(wait, 0), 55*time.Second)
	l := s.hostEvents()
	evs, next, wake := l.after(after)
	if len(evs) == 0 && wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-wake:
		case <-t.C:
		case <-r.Context().Done():
		}
		evs, next, _ = l.after(after)
	}
	writeJSON(w, 200, map[string]any{"events": evs, "next": next})
}

// ---------------------------------------------------------------- tasks

func (s *Server) hostTask(w http.ResponseWriter, r *http.Request, module string) {
	name := r.PathValue("name")
	if !validSecretName(name) || s.Tasks == nil {
		writeJSON(w, 404, map[string]string{"error": "no such task"})
		return
	}
	var in struct {
		Title string `json:"title"`
		Done  int64  `json:"done"`
		Total int64  `json:"total"`
		Error string `json:"error"`
		End   bool   `json:"end"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "not a task's report"})
		return
	}
	key := module + "/" + name
	s.modTasksMu.Lock()
	if s.modTasks == nil {
		s.modTasks = map[string]*tasks.Handle{}
	}
	t := s.modTasks[key]
	if t == nil {
		title := in.Title
		if title == "" {
			title = s.hostMods().Name(module) + ": " + name
		}
		t = s.Tasks.Start(tasks.Spec{Type: "module", Title: title, Source: module, Key: key, AdminOnly: true})
		s.modTasks[key] = t
	}
	end := in.End || in.Error != "" || (in.Total > 0 && in.Done >= in.Total)
	if end {
		delete(s.modTasks, key)
	}
	s.modTasksMu.Unlock()
	t.Progress(in.Done, in.Total)
	if end {
		if in.Error != "" {
			t.Finish(errors.New(in.Error))
		} else {
			t.Finish(nil)
		}
	}
	w.WriteHeader(204)
}

// hostAudit writes a row of the server's audit log for a module (custom cores, overrides and the
// rest share it): who did it (the person a call is for), what, to what. Kept by the server, so a
// module can't take its own rows out.
func (s *Server) hostAudit(w http.ResponseWriter, r *http.Request, module string) {
	u, _, err := s.onBehalf(r)
	if err != nil {
		writeJSON(w, 403, map[string]string{"error": err.Error()})
		return
	}
	var in struct {
		Action string `json:"action"`
		Core   string `json:"core"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&in); err != nil || in.Action == "" || len(in.Action) > 64 || len(in.Core) > 128 {
		writeJSON(w, 400, map[string]string{"error": "an audit row needs an action"})
		return
	}
	s.writeAudit(r.Context(), u, module+" "+in.Action, module, in.Action, in.Core, in.Detail)
	w.WriteHeader(204)
}

// hostAuditLog is the module's own rows of the audit log, newest first: limit=, at most 500.
func (s *Server) hostAuditLog(w http.ResponseWriter, r *http.Request, module string) {
	limit := min(max(qIntOf(r.URL.Query().Get("limit"), 100), 1), 500)
	writeJSON(w, 200, map[string]any{"rows": s.coreAudit(r.Context(), module, limit)})
}
