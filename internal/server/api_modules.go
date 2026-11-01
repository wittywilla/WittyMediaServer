package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wittywilla/wittymodule"
	"io"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

// Modules (admins only): the version of each in use and why, what is available,
// installing it and going back. See internal/modules and docs/MODULES.md.

// GET /api/modules
func (s *Server) handleModules(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	writeJSON(w, 200, s.Modules.Report())
}

// POST /api/modules/check: ask GitHub now, whatever the automatic setting says,
// because an admin asked (answers younger than a minute are reused, so the button
// can't be used to flood GitHub).
func (s *Server) handleModulesCheck(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	writeJSON(w, 200, s.Modules.Check(r.Context(), time.Minute))
}

// POST /api/modules/{id}/install: download, verify and install a version, with no
// restart: the newest the last check found, or the one named in the body. A choice with
// a warning (older, withdrawn as broken, taken out of use before) is answered 409 with
// the warning until the body says confirm.
func (s *Server) handleModuleInstall(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	var b struct {
		Version string `json:"version"`
		Confirm bool   `json:"confirm"`
		moduleSecrets
	}
	if r.ContentLength != 0 && r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil && !errors.Is(err, io.EOF) {
			writeErr(w, 400, "unreadable request")
			return
		}
	}
	// Going back can bring back a hole that was fixed. The warning comes first, and needs nothing.
	id := r.PathValue("id")
	if b.Confirm && b.Version != "" && s.Modules.OlderThanInUse(id, b.Version) {
		if !s.moduleGate(w, r, b.moduleSecrets, id) {
			return
		}
		s.auditModule(r, "module-older", id, "to version "+b.Version)
	}
	var err error
	if b.Version == "" {
		err = s.Modules.Install(r.PathValue("id"))
	} else {
		err = s.Modules.InstallVersion(r.PathValue("id"), b.Version, b.Confirm)
	}
	if err != nil {
		s.Log.WarnContext(r.Context(), "module update refused", "cat", "modules", "module", r.PathValue("id"), "err", err)
		writeModuleErr(w, err)
		return
	}
	s.Log.InfoContext(r.Context(), "module update started", "cat", "modules", "module", r.PathValue("id"), "version", b.Version, "user", userFrom(r).Name)
	writeJSON(w, 202, s.Modules.Report())
}

// GET /api/modules/{id}/versions: every version known, explained, without going
// online. POST /api/modules/check is what asks GitHub.
func (s *Server) handleModuleVersions(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	l, err := s.Modules.Versions(r.PathValue("id"))
	if err != nil {
		writeModuleErr(w, err)
		return
	}
	writeJSON(w, 200, l)
}

// POST /api/modules/{id}/pin: keep the version in use; automatic updates leave it alone.
func (s *Server) handleModulePin(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module pinned", 200, func(id string) error { return s.Modules.Pin(id) })
}

// DELETE /api/modules/{id}/pin: let automatic updates move it again.
func (s *Server) handleModuleUnpin(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module unpinned", 200, func(id string) error { return s.Modules.Unpin(id) })
}

// PUT /api/modules/{id}/autoupdate: whether this module may update by itself while the
// server's setting is on. {"on": false} leaves it to be updated by hand.
func (s *Server) handleModuleAutoUpdate(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	var b struct {
		On *bool `json:"on"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&b); err != nil || b.On == nil {
		writeErr(w, 400, "say whether it may update by itself: {\"on\": true}")
		return
	}
	if err := s.Modules.SetAutoUpdate(r.PathValue("id"), *b.On); err != nil {
		writeModuleErr(w, err)
		return
	}
	s.Log.Info("module automatic updates changed", "module", r.PathValue("id"), "on", *b.On, "by", userFrom(r).Name)
	writeJSON(w, 200, s.Modules.Report())
}

// POST /api/modules/{id}/latest: end a pin and move to the newest version that fits.
func (s *Server) handleModuleLatest(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module returning to the latest", 202, func(id string) error { return s.Modules.ToLatest(id) })
}

// moduleSecrets are what a change to which code runs asks for.
type moduleSecrets struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

// moduleGate asks for the signed-in person's password, and code if they use one, before a stolen
// session could change what runs on this server. It answers the request itself when refused. With
// nothing given, the answer says what to ask for.
func (s *Server) moduleGate(w http.ResponseWriter, r *http.Request, b moduleSecrets, id string) bool {
	if b.Password == "" {
		writeJSON(w, 403, map[string]any{"error": "This asks for your password.", "code": "password-needed"})
		return false
	}
	if s.confirmOwner(w, r, b.Password, b.Code) {
		return true
	}
	s.auditModule(r, "password-refused", id, r.Method+" "+r.URL.Path)
	return false
}

func (s *Server) moduleGateFromBody(w http.ResponseWriter, r *http.Request) bool {
	var b moduleSecrets
	if r.ContentLength != 0 && r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil && !errors.Is(err, io.EOF) {
			writeErr(w, 400, "unreadable request")
			return false
		}
	}
	return s.moduleGate(w, r, b, r.PathValue("id"))
}

// auditModule writes a change to which code runs down beside custom cores', as both are the same
// kind of thing, and in the log.
func (s *Server) auditModule(r *http.Request, action, id, detail string) {
	s.writeAudit(r.Context(), userFrom(r), "module", action, "module "+id, detail)
}

func (s *Server) moduleAction(w http.ResponseWriter, r *http.Request, what string, code int, do func(id string) error) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	if err := do(r.PathValue("id")); err != nil {
		writeModuleErr(w, err)
		return
	}
	s.Log.Info(what, "module", r.PathValue("id"), "by", userFrom(r).Name)
	writeJSON(w, code, s.Modules.Report())
}

// PUT /api/modules/{id}/source: take this module's releases from someone's fork, signed
// with the key given. Answered 409 with the warning until the body says confirm.
func (s *Server) handleModuleSource(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	var b struct {
		Repo    string `json:"repo"`
		Key     string `json:"key"`
		Confirm bool   `json:"confirm"`
		moduleSecrets
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil {
		writeErr(w, 400, "unreadable request")
		return
	}
	if why := modmgr.ForkRefusal(); why != "" {
		s.auditModule(r, "fork-refused", r.PathValue("id"), b.Repo)
		writeErr(w, 403, why)
		return
	}
	if b.Confirm && !s.moduleGate(w, r, b.moduleSecrets, r.PathValue("id")) {
		return
	}
	if err := s.Modules.SetSource(r.PathValue("id"), b.Repo, b.Key, userFrom(r).Name, b.Confirm); err != nil {
		writeModuleErr(w, err)
		return
	}
	s.Log.Warn("a module now comes from another publisher", "module", r.PathValue("id"), "repo", b.Repo, "by", userFrom(r).Name)
	s.auditModule(r, "module-source", r.PathValue("id"), "now from "+b.Repo)
	writeJSON(w, 200, s.Modules.Report())
}

// DELETE /api/modules/{id}/source: back to the module's own repository.
func (s *Server) handleModuleSourceClear(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module back to its own repository", 200, func(id string) error { return s.Modules.ClearSource(id) })
}

// POST /api/modules/{id}/add: put a module on this server. Works straight away, offline.
func (s *Server) handleModuleAdd(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module added", 202, func(id string) error { return s.Modules.AddModule(id) })
}

// POST /api/modules/{id}/remove: take a module off this server. Refused, with their
// names, while libraries need it. No data is touched.
func (s *Server) handleModuleRemove(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module removed", 200, func(id string) error {
		libs, err := s.librariesByType(r.Context())
		if err != nil {
			return err
		}
		return s.Modules.RemoveModule(id, libs)
	})
}

// PUT /api/modules/installed: make the set of modules on this server what someone chose,
// as first-run setup does. {"installed": ["wittyvideo", ...]}. Refused whole if libraries
// need something left out.
func (s *Server) handleModulesInstalled(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	var b struct {
		Installed []string `json:"installed"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&b); err != nil || b.Installed == nil {
		writeErr(w, 400, "say which modules: {\"installed\": [...]}")
		return
	}
	libs, err := s.librariesByType(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.Modules.SetInstalled(b.Installed, libs); err != nil {
		writeModuleErr(w, err)
		return
	}
	s.Log.Info("modules chosen", "installed", b.Installed, "by", userFrom(r).Name)
	writeJSON(w, 200, s.Modules.Report())
}

// GET /api/modules/libraries: every kind of library a module provides, and whether it
// can be made here. Anyone signed in may ask: it decides what the app shows.
func (s *Server) handleModulesLibraries(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeJSON(w, 200, map[string]any{"libraries": []modmgr.LibraryType{}})
		return
	}
	writeJSON(w, 200, map[string]any{"libraries": s.Modules.LibraryTypes()})
}

// PUT /api/modules/libraries/{type}/label: what this server calls a kind of library.
// {"label": "Blu-rays and DVDs", "taken": {"movies": "Movies"}} gives it a name; an empty
// label goes back to the usual one. taken is the names the app shows for the other kinds.
func (s *Server) handleModuleLibraryLabel(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	var b struct {
		Label string            `json:"label"`
		Taken map[string]string `json:"taken"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&b); err != nil {
		writeErr(w, 400, "unreadable request")
		return
	}
	if err := s.Modules.SetLibraryLabel(r.PathValue("type"), b.Label, b.Taken); err != nil {
		writeModuleErr(w, err)
		return
	}
	s.Log.Info("a kind of library was named", "type", r.PathValue("type"), "label", b.Label, "by", userFrom(r).Name)
	writeJSON(w, 200, s.Modules.Report())
}

// librariesByType names the libraries there are, by type.
func (s *Server) librariesByType(ctx context.Context) (map[string][]string, error) {
	ls, err := s.Store.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, l := range ls {
		out[l.Type] = append(out[l.Type], l.Name)
	}
	return out, nil
}

// libraryAllowed answers for a kind of library whose module isn't installed, and says
// whether to go on. The answer names what to add, so the app can link to it.
func (s *Server) libraryAllowed(w http.ResponseWriter, libType string) bool {
	if s.Modules == nil {
		return true
	}
	err := s.Modules.LibraryAllowed(libType)
	var missing *modmgr.ErrModuleMissing
	if errors.As(err, &missing) {
		writeJSON(w, 409, map[string]any{"error": err.Error(), "code": "module-missing", "modules": missing.Modules})
		return false
	}
	return true
}

// GET /api/modules/web: the bundles in use instead of the copies in the app. Usually
// none, and asked for only when a reader opens.
func (s *Server) handleModulesWeb(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeJSON(w, 200, map[string]any{"bundles": []modmgr.WebBundle{}})
		return
	}
	bundles := s.Modules.WebBundles()
	// A page for admins isn't offered to anyone else. It keeps nothing safe by itself: what it
	// calls checks for itself.
	if !userFrom(r).IsAdmin {
		for i := range bundles {
			bundles[i].Pages = slices.DeleteFunc(slices.Clone(bundles[i].Pages), func(p wittymodule.Page) bool { return p.Admin })
		}
	}
	writeJSON(w, 200, map[string]any{"bundles": bundles})
}

// bundleTypes are the only kinds of file a bundle can serve (wittymodule pack refuses
// others too), each with the type a browser insists on for it.
var bundleTypes = map[string]string{".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
	".wasm": "application/wasm", ".json": "application/json", ".svg": "image/svg+xml", ".png": "image/png", ".woff2": "font/woff2", ".map": "application/json"}

// GET /api/modules/{id}/web/{version}/{path...}: a file of the bundle in use. Only names
// in its signed list exist here. The address carries the version, so a file never
// changes under it. The default policy for /api/ stays, so opening one directly can't
// run it; the policy that governs it as a script is the page's, which allows only this
// site's own.
func (s *Server) handleModuleWebFile(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "no such file")
		return
	}
	id, ver, rel := r.PathValue("id"), r.PathValue("version"), r.PathValue("path")
	typ := bundleTypes[strings.ToLower(path.Ext(rel))]
	if typ == "" {
		writeErr(w, 404, "no such file")
		return
	}
	// An app that has its own copy of the page reaches this from another origin. A bundle is
	// no secret, and what fetches it still has to be signed in.
	w.Header().Set("Access-Control-Allow-Origin", "*")
	file, hash, err := s.Modules.WebFile(id, ver, rel)
	if err != nil {
		writeErr(w, 404, "no such file")
		return
	}
	h := w.Header()
	h.Set("Content-Type", typ)
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("ETag", `"`+hash+`"`)
	h.Set("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		if gz, gzHash, err := s.Modules.WebFile(id, ver, rel+".gz"); err == nil {
			file = gz
			h.Set("Content-Encoding", "gzip")
			h.Set("ETag", `"`+gzHash+`"`)
		}
	}
	http.ServeFile(w, r, file)
}

// POST /api/modules/{id}/failed: a browser saying a bundle wouldn't load or crashed.
// Three reports from an admin within a day, at most one a minute, take that version out
// of use. Anyone else's report goes in the module's log for an admin
// to see, and each is logged with who said so.
func (s *Server) handleModuleFailed(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	var b struct {
		Version string `json:"version"`
		Reason  string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil {
		writeErr(w, 400, "unreadable request")
		return
	}
	if !s.clientLogs.allow(userFrom(r).ID) { // each writes the module state to disk
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(429)
		return
	}
	s.Log.Warn("a browser reported a module failing", "module", r.PathValue("id"), "version", b.Version, "reason", b.Reason, "by", userFrom(r).Name)
	if err := s.Modules.ReportBrowserFailure(r.PathValue("id"), b.Version, "in a browser: "+b.Reason, userFrom(r).IsAdmin); err != nil {
		writeModuleErr(w, err)
		return
	}
	w.WriteHeader(204)
}

// POST /api/modules/{id}/rollback: go back to the previous version, if any.
func (s *Server) handleModuleRollback(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	if s.Modules.RunningDownload(r.PathValue("id")) {
		if !s.moduleGateFromBody(w, r) {
			return
		}
		s.auditModule(r, "module-rollback", r.PathValue("id"), "")
	}
	if err := s.Modules.Rollback(r.PathValue("id")); err != nil {
		s.Log.WarnContext(r.Context(), "module rollback failed", "cat", "modules", "module", r.PathValue("id"), "err", err)
		writeModuleErr(w, err)
		return
	}
	s.Log.InfoContext(r.Context(), "module rolled back", "cat", "modules", "module", r.PathValue("id"), "user", userFrom(r).Name)
	writeJSON(w, 200, s.Modules.Report())
}

func writeModuleErr(w http.ResponseWriter, err error) {
	if errors.Is(err, modmgr.ErrUnknown) {
		writeErr(w, 404, err.Error())
		return
	}
	var using *modmgr.ErrInUse
	if errors.As(err, &using) {
		writeJSON(w, 409, map[string]any{"error": err.Error(), "code": "module-in-use", "libraries": using.Libraries})
		return
	}
	var need *modmgr.ErrNeedsConfirm
	if errors.As(err, &need) {
		writeJSON(w, 409, map[string]any{"error": need.Warning, "code": "confirm", "warning": need.Warning})
		return
	}
	writeErr(w, 409, err.Error())
}

// writeModuleMissing answers a request a missing module could have answered, and says whether
// that's what happened. 424 means the request was fine and depends on something that isn't
// here: the page explains itself and offers to install it, rather than showing an error.
func writeModuleMissing(w http.ResponseWriter, err error) bool {
	var missing *modmgr.ErrMissing
	if !errors.As(err, &missing) {
		return false
	}
	writeJSON(w, http.StatusFailedDependency, map[string]any{"error": missing.Error(), "module": missing.ID, "moduleName": missing.Name})
	return true
}

// POST /api/modules/{id}/enable and /disable: turn a module on or off. Nothing is downloaded
// or lost, and no restart is needed.
func (s *Server) handleModuleEnable(w http.ResponseWriter, r *http.Request) {
	s.setModuleEnabled(w, r, true)
}

func (s *Server) handleModuleDisable(w http.ResponseWriter, r *http.Request) {
	s.setModuleEnabled(w, r, false)
}

func (s *Server) setModuleEnabled(w http.ResponseWriter, r *http.Request, on bool) {
	what := "module turned off"
	if on {
		what = "module turned on"
	}
	s.moduleAction(w, r, what, 200, func(id string) error {
		libs, err := s.librariesByType(r.Context())
		if err != nil {
			return err
		}
		return s.Modules.SetEnabled(id, on, libs)
	})
}

// routes is where a server's routes go: the mux, or the mux behind a check.
type routes interface {
	Handle(pattern string, h http.Handler)
	HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))
}

// moduleRoutes are routes that only mean anything with a module, which say so without it.
type moduleRoutes struct {
	s  *Server
	id string
	m  *http.ServeMux
}

// needing has what's added to it answer that a module is missing while it is.
func (s *Server) needing(id string, m *http.ServeMux) routes { return moduleRoutes{s, id, m} }

func (g moduleRoutes) Handle(pattern string, h http.Handler) {
	// Who isn't signed in learns nothing about what's on this server.
	missing := g.s.requireUser(func(w http.ResponseWriter, _ *http.Request) {
		writeModuleMissing(w, g.s.Modules.Missing(g.id))
	})
	g.m.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.s.Modules != nil && !g.s.Modules.Installed(g.id) {
			missing.ServeHTTP(w, r)
			return
		}
		h.ServeHTTP(w, r)
	}))
}

func (g moduleRoutes) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request)) {
	g.Handle(pattern, http.HandlerFunc(h))
}

// GET /api/modules/{id}/icon: a module's picture from the catalogue, fetched and kept here so no
// page ever asks anyone else for it.
func (s *Server) handleModuleIcon(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "no picture")
		return
	}
	p, err := s.Modules.IconFile(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "no picture")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, p)
}

// GET /api/modules/{id}/notices: the licenses of the libraries a module's programs are built
// from, as plain text. Each module carries its own, since it's passed on by itself.
func (s *Server) handleModuleNotices(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "no notices")
		return
	}
	b, ok := s.Modules.Notices(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "no notices")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}

// A module's page keeps a little per person, such as how they arranged it. It's kept in the settings
// table, and the key says whose it is, so a module can't read anyone else's or any setting.
const (
	maxModuleStoreValue = 64 << 10
	maxModuleStoreKeys  = 32
)

var moduleStoreKey = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func moduleStoreName(userID int64, id, key string) string {
	return fmt.Sprintf("modulestore.%d.%s.%s", userID, id, key)
}

// moduleStoreTarget checks the address. It writes the answer itself when it says no.
func (s *Server) moduleStoreTarget(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, key := r.PathValue("id"), r.PathValue("key")
	if s.Modules == nil || !moduleStoreKey.MatchString(key) {
		writeErr(w, 404, "not found")
		return "", false
	}
	if !s.Modules.Installed(id) {
		writeModuleMissing(w, s.Modules.Missing(id))
		return "", false
	}
	return moduleStoreName(userFrom(r).ID, id, key), true
}

// GET /api/module-store/{id}/{key}: what this person's page saved, or null.
func (s *Server) handleModuleStoreGet(w http.ResponseWriter, r *http.Request) {
	name, ok := s.moduleStoreTarget(w, r)
	if !ok {
		return
	}
	v := s.DB.GetSetting(r.Context(), name, "null")
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, v)
}

// PUT /api/module-store/{id}/{key}: saves any JSON, up to a limit.
func (s *Server) handleModuleStorePut(w http.ResponseWriter, r *http.Request) {
	name, ok := s.moduleStoreTarget(w, r)
	if !ok {
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxModuleStoreValue))
	if err != nil || !json.Valid(b) {
		writeErr(w, 400, "expected JSON of at most 64 KB")
		return
	}
	prefix := name[:strings.LastIndexByte(name, '.')+1]
	var n int
	if err := s.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM settings WHERE key >= ? AND key < ? AND key <> ?`, prefix, prefix+"~", name).Scan(&n); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if n >= maxModuleStoreKeys {
		writeErr(w, 409, "this module has saved as much as it may")
		return
	}
	if err := s.DB.SetSetting(r.Context(), name, string(b)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
