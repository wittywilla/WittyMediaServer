// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	h("GET "+b+"/libraries", s.hostLibraries)
	h("GET "+b+"/may", s.hostMay)
	h("GET "+b+"/settings", s.hostSettings)
	h("GET "+b+"/secrets/{name}", s.hostSecret)
	h("POST "+b+"/images", s.hostImage)
	h("POST "+b+"/sessions", s.hostSessionStart)
	h("DELETE "+b+"/sessions/{id}", s.hostSessionEnd)
	h("GET "+b+"/events", s.hostEventsWait)
	h("POST "+b+"/tasks/{name}", s.hostTask)
	h("GET "+b+"/legacy/{table}", s.hostLegacy)
	h("GET "+b+"/legacy-files/{dir...}", s.hostLegacyFiles)
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
	writeJSON(w, 200, host.Hello{Module: module, Server: s.Version, Offers: modmgr.Offers()})
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
	writeJSON(w, 200, moduleItem(it))
}

func (s *Server) hostItems(w http.ResponseWriter, r *http.Request, module string) {
	_, a, err := s.onBehalf(r)
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
	var out []*service.Item
	for _, l := range libs {
		if !s.servesKind(module, l.Type) || (want != 0 && l.ID != want) || (kind != "" && l.Type != kind) {
			continue
		}
		iq := library.ItemQuery{LibraryID: l.ID, Kind: q.Get("kind"), Search: q.Get("search"), Console: q.Get("facet.console"),
			Access: a, Brief: true, NoTotal: true, Limit: offset + limit}
		items, _, err := s.Store.Items(r.Context(), iq)
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		for _, it := range items {
			m := moduleItem(it)
			m.Files = nil // a list doesn't carry files: ask for the item
			out = append(out, m)
		}
	}
	if offset >= len(out) {
		out = nil
	} else {
		out = out[offset:min(len(out), offset+limit)]
	}
	writeJSON(w, 200, map[string]any{"items": out})
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

// hostCounts counts games by console in the module's games libraries, as the person asking may
// see them (everything, with nobody named). Only the console facet is counted today.
func (s *Server) hostCounts(w http.ResponseWriter, r *http.Request, module string) {
	_, a, err := s.onBehalf(r)
	if err != nil {
		writeJSON(w, 403, map[string]string{"error": err.Error()})
		return
	}
	kind, by := r.URL.Query().Get("libraryKind"), r.URL.Query().Get("by")
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
		if l.Type != kind {
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

// ---------------------------------------------------------------- live sessions

// moduleSession is something live a module reported: a game being played.
type moduleSession struct {
	host.Session
	Module  string
	Started time.Time
}

type moduleSessions struct {
	mu   sync.Mutex
	next int
	m    map[string]*moduleSession
}

// List is the live sessions modules reported, for metrics and "playing now".
func (ms *moduleSessions) List() []moduleSession {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	out := make([]moduleSession, 0, len(ms.m))
	for _, v := range ms.m {
		out = append(out, *v)
	}
	return out
}

func (s *Server) hostSessionStart(w http.ResponseWriter, r *http.Request, module string) {
	var in host.Session
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil || in.User <= 0 || in.Kind == "" {
		writeJSON(w, 400, map[string]string{"error": "a session needs a kind and a person"})
		return
	}
	ms := &s.modSessions
	ms.mu.Lock()
	if ms.m == nil {
		ms.m = map[string]*moduleSession{}
	}
	ms.next++
	in.ID = module + "-" + strconv.Itoa(ms.next)
	ms.m[in.ID] = &moduleSession{Session: in, Module: module, Started: time.Now()}
	ms.mu.Unlock()
	writeJSON(w, 200, map[string]string{"id": in.ID})
}

func (s *Server) hostSessionEnd(w http.ResponseWriter, r *http.Request, module string) {
	ms := &s.modSessions
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if v := ms.m[r.PathValue("id")]; v == nil || v.Module != module {
		writeJSON(w, 404, map[string]string{"error": "no such session"})
		return
	}
	delete(ms.m, r.PathValue("id"))
	w.WriteHeader(204)
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

// ---------------------------------------------------------------- legacy data

// legacyImports are the server's own data a module of API 5 may take over, by the name its
// integration.json gives (data.import): which module, and which tables. Only these tables, and
// only for that module, can be read.
var legacyImports = map[string]struct {
	module string
	tables []string
	files  []string // folders under the data folder whose files it may read, once
}{
	"server-games-v1": {"wittyplay", []string{"wittyplay_controls", "wittyplay_profiles", "wittyplay_backgrounds", "wittyplay_settings",
		"wittyplay_game_meta", "wittyplay_core_options", "wittyplay_bezels", "wittyplay_patches", "custom_cores", "custom_core_versions",
		"custom_consoles", "game_core_choice", "game_core_plays", "game_core_audit", "wittyplay_presence", "wittyplay_kids_friends",
		"game_pictures", "game_meta", "game_meta_sources", "game_files", "game_review", "game_decisions", "game_discs"},
		[]string{"gaming/backgrounds"}},
	"server-couch-v1": {"wittycouch", []string{"wittycouch_hosts", "wittyrigs_rigs", "wittyrigs_apps"}, nil},
}

func (s *Server) hostLegacy(w http.ResponseWriter, r *http.Request, module string) {
	table := r.PathValue("table")
	in := s.hostMods().Integration(module)
	allowed := false
	if in != nil && in.Data != nil {
		if imp, ok := legacyImports[in.Data.Import]; ok && imp.module == module && slices.Contains(imp.tables, table) {
			allowed = true
		}
	}
	if !allowed {
		writeJSON(w, 403, map[string]string{"error": "not data this module takes over"})
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit := min(max(qIntOf(r.URL.Query().Get("limit"), 500), 1), 2000)
	rows, next, err := s.legacyRows(r.Context(), table, after, limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"rows": rows, "next": next})
}

// legacyRows reads a page of a table by its rowid, each row as a JSON object of its columns.
// table is one of legacyImports' names, never anything a caller typed in freely.
func (s *Server) legacyRows(ctx context.Context, table string, after int64, limit int) ([]json.RawMessage, int64, error) {
	rs, err := s.DB.QueryContext(ctx, fmt.Sprintf(`SELECT rowid AS _rowid, * FROM %q WHERE rowid > ? ORDER BY rowid LIMIT ?`, table), after, limit)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, after, nil
		}
		return nil, after, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, after, err
	}
	var out []json.RawMessage
	next := after
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, after, err
		}
		row := map[string]any{}
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			if c == "_rowid" {
				if n, ok := vals[i].(int64); ok {
					next = n
				}
				continue
			}
			row[c] = vals[i]
		}
		raw, _ := json.Marshal(row)
		out = append(out, raw)
	}
	return out, next, rs.Err()
}

// hostLegacyFiles lists (a folder) or sends (a file) what the server kept on disk for a module
// that takes it over: only under the folders its import names, and nothing outside them.
func (s *Server) hostLegacyFiles(w http.ResponseWriter, r *http.Request, module string) {
	in := s.hostMods().Integration(module)
	rel := filepath.Clean(filepath.FromSlash(r.PathValue("dir")))
	ok := false
	if in != nil && in.Data != nil && !strings.Contains(r.PathValue("dir"), "..") {
		if imp, found := legacyImports[in.Data.Import]; found && imp.module == module {
			for _, d := range imp.files {
				d = filepath.FromSlash(d)
				if rel == d || strings.HasPrefix(rel, d+string(filepath.Separator)) {
					ok = true
				}
			}
		}
	}
	if !ok {
		writeJSON(w, 403, map[string]string{"error": "not files this module takes over"})
		return
	}
	full := filepath.Join(s.Cfg.DataDir, rel)
	st, err := os.Lstat(full)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "not there"})
		return
	}
	if st.IsDir() {
		ents, _ := os.ReadDir(full)
		names := []string{}
		for _, e := range ents {
			if e.Type().IsRegular() {
				names = append(names, e.Name())
			}
		}
		writeJSON(w, 200, map[string]any{"files": names})
		return
	}
	if !st.Mode().IsRegular() {
		writeJSON(w, 404, map[string]string{"error": "not there"})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, full)
}
