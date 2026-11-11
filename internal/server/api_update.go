// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"os"
	"time"
)

// Updates (admins only): what is installed, what is available, and
// installing it. See internal/update.

// GET /api/update
func (s *Server) handleUpdate(w http.ResponseWriter, r *http.Request) {
	if s.Update == nil {
		writeErr(w, 404, "updates are not available in this build")
		return
	}
	writeJSON(w, 200, s.Update.Status())
}

// POST /api/update/check: ask GitHub now (answers younger than a minute are
// reused, so the button can't be used to flood GitHub).
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.Update == nil {
		writeErr(w, 404, "updates are not available in this build")
		return
	}
	writeJSON(w, 200, s.Update.Check(r.Context(), time.Minute)) // a failed check is logged by the updater
}

// POST /api/update/apply: download, verify and install the newest release,
// then restart.
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if s.Update == nil {
		writeErr(w, 404, "updates are not available in this build")
		return
	}
	if err := s.Update.Apply(); err != nil {
		s.Log.WarnContext(r.Context(), "update refused", "cat", "update", "err", err)
		writeErr(w, 409, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "update asked for", "cat", "update", "user", userFrom(r).Name)
	writeJSON(w, 202, s.Update.Status())
}

// POST /api/update/installer: download the installer for this system and check it against the
// release's signed checksums, for a copy that can't replace its own files.
func (s *Server) handleUpdateInstaller(w http.ResponseWriter, r *http.Request) {
	if s.Update == nil {
		writeErr(w, 404, "updates are not available in this build")
		return
	}
	if err := s.Update.FetchInstaller(); err != nil {
		s.Log.WarnContext(r.Context(), "installer download refused", "cat", "update", "err", err)
		writeErr(w, 409, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "installer download asked for", "cat", "update", "user", userFrom(r).Name)
	writeJSON(w, 202, s.Update.Status())
}

// GET /api/update/installer: the checked installer, to save on the device the admin is using.
func (s *Server) handleUpdateInstallerFile(w http.ResponseWriter, r *http.Request) {
	if s.Update == nil {
		writeErr(w, 404, "updates are not available in this build")
		return
	}
	path, name, ok := s.Update.InstallerFile()
	if !ok {
		writeErr(w, 404, "no installer has been downloaded and checked yet")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, 404, "no installer has been downloaded and checked yet")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		writeErr(w, 404, "no installer has been downloaded and checked yet")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, fi.ModTime(), f)
}

// GET /api/health: the server answers. The tray app on the same computer also learns of a newer
// version here, for its menu; nobody else is told.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{"ok": true, "instance": s.Instance}
	if s.Update != nil && onThisMachine(r) {
		if v := s.Update.Available(); v != "" {
			body["update"] = v
		}
	}
	writeJSON(w, 200, body)
}
