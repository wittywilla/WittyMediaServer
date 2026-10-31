package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
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
	}
	if r.ContentLength != 0 && r.Body != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil && !errors.Is(err, io.EOF) {
			writeErr(w, 400, "unreadable request")
			return
		}
	}
	var err error
	if b.Version == "" {
		err = s.Modules.Install(r.PathValue("id"))
	} else {
		err = s.Modules.InstallVersion(r.PathValue("id"), b.Version, b.Confirm)
	}
	if err != nil {
		writeModuleErr(w, err)
		return
	}
	s.Log.Info("module version change started", "module", r.PathValue("id"), "version", b.Version, "by", userFrom(r).Name)
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

// POST /api/modules/{id}/builtin: use the built-in copy, without blaming the version in use.
func (s *Server) handleModuleUseBuiltin(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module switched to its built-in copy", 200, func(id string) error { return s.Modules.UseBuiltin(id) })
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
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&b); err != nil {
		writeErr(w, 400, "unreadable request")
		return
	}
	if err := s.Modules.SetSource(r.PathValue("id"), b.Repo, b.Key, userFrom(r).Name, b.Confirm); err != nil {
		writeModuleErr(w, err)
		return
	}
	s.Log.Warn("a module now comes from another publisher", "module", r.PathValue("id"), "repo", b.Repo, "by", userFrom(r).Name)
	writeJSON(w, 200, s.Modules.Report())
}

// DELETE /api/modules/{id}/source: back to the module's own repository.
func (s *Server) handleModuleSourceClear(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module back to its own repository", 200, func(id string) error { return s.Modules.ClearSource(id) })
}

// POST /api/modules/{id}/add: put a module on this server. Works straight away, offline.
func (s *Server) handleModuleAdd(w http.ResponseWriter, r *http.Request) {
	s.moduleAction(w, r, "module added", 200, func(id string) error { return s.Modules.AddModule(id) })
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
	writeJSON(w, 200, map[string]any{"bundles": s.Modules.WebBundles()})
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
// Three reports within a day, at most one a minute, take that version out of use for
// the built-in copy. Anyone signed in can send them, and the worst that can cost is the
// newer version of a reader until an admin chooses it again, so each is logged with who
// said so.
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
	s.Log.Warn("a browser reported a module failing", "module", r.PathValue("id"), "version", b.Version, "reason", b.Reason, "by", userFrom(r).Name)
	if err := s.Modules.ReportBrowserFailure(r.PathValue("id"), b.Version, "in a browser: "+b.Reason); err != nil {
		writeModuleErr(w, err)
		return
	}
	w.WriteHeader(204)
}

// POST /api/modules/{id}/rollback: go back to the previous version, or the
// built-in copy.
func (s *Server) handleModuleRollback(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	if err := s.Modules.Rollback(r.PathValue("id")); err != nil {
		writeModuleErr(w, err)
		return
	}
	s.Log.Info("module rolled back", "module", r.PathValue("id"), "by", userFrom(r).Name)
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
