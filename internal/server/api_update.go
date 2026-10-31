package server

import (
	"net/http"
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
