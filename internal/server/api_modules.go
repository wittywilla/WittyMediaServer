package server

import (
	"errors"
	"net/http"
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

// POST /api/modules/{id}/install: download, verify and install the version the
// last check found. It needs no restart.
func (s *Server) handleModuleInstall(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
	}
	if err := s.Modules.Install(r.PathValue("id")); err != nil {
		s.Log.WarnContext(r.Context(), "module update refused", "cat", "modules", "module", r.PathValue("id"), "err", err)
		writeModuleErr(w, err)
		return
	}
	s.Log.InfoContext(r.Context(), "module update started", "cat", "modules", "module", r.PathValue("id"), "user", userFrom(r).Name)
	writeJSON(w, 202, s.Modules.Report())
}

// POST /api/modules/{id}/rollback: go back to the previous version, or the
// built-in copy.
func (s *Server) handleModuleRollback(w http.ResponseWriter, r *http.Request) {
	if s.Modules == nil {
		writeErr(w, 404, "modules can't be updated in this build")
		return
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
	writeErr(w, 409, err.Error())
}
