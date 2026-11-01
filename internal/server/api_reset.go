package server

import (
	"context"
	"net/http"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/reset"
)

func (s *Server) resetFolders() reset.Folders {
	return reset.Folders{Data: s.Cfg.DataDir, Others: []string{s.Cfg.CacheDir, s.Cfg.TranscodeDir, s.Cfg.ImagesDir},
		Program: config.ProgramDir(), Config: s.Cfg.ConfigFile}
}

func (s *Server) resetCheck(ctx context.Context) ([]string, error) {
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, l := range libs {
		paths = append(paths, l.Paths...)
	}
	return reset.Check(s.resetFolders(), paths)
}

// resetView is what the reset page shows.
type resetView struct {
	DataDir    string         `json:"dataDir"`
	Outside    []string       `json:"outside"`
	Blocked    string         `json:"blocked,omitempty"` // why this install can't be reset as it's set up
	Copies     []reset.Copy   `json:"copies"`
	Pending    *reset.Request `json:"pending,omitempty"`
	Last       *reset.Result  `json:"last,omitempty"`
	CanRestart bool           `json:"canRestart"`
	TwoFactor  bool           `json:"twoFactor"`
}

// GET /api/reset (admin)
func (s *Server) handleResetView(w http.ResponseWriter, r *http.Request) {
	v := resetView{DataDir: s.Cfg.DataDir, Outside: []string{}, Copies: reset.Copies(s.Cfg.DataDir),
		Pending: reset.Pending(s.Cfg.DataDir), Last: reset.Last(s.Cfg.DataDir), CanRestart: s.Restart != nil, TwoFactor: userFrom(r).TwoFactor}
	if out, err := s.resetCheck(r.Context()); err != nil {
		v.Blocked = err.Error()
	} else if out != nil {
		v.Outside = out
	}
	writeJSON(w, 200, v)
}

type resetBody struct {
	Password string `json:"password"`
	Code     string `json:"code"`
	Confirm  string `json:"confirm"`
	Delete   bool   `json:"delete"` // nothing is kept, earlier copies included
	Copy     string `json:"copy"`   // restore: the copy to bring back
}

// resetAllowed asks for the password and any second step again. It answers the request
// itself when they are refused.
func (s *Server) resetAllowed(w http.ResponseWriter, r *http.Request, b resetBody) bool {
	u := userFrom(r)
	if !s.checkOwnPassword(w, r, u, b.Password) {
		return false
	}
	if u.TwoFactor && !s.Auth.CheckTwoFactor(r.Context(), u, b.Code) {
		s.signInFailed(r, u.Name)
		writeErr(w, 403, "that code is not right")
		return false
	}
	return true
}

// resetAndRestart writes the request, answers, and restarts, which is when it happens.
func (s *Server) resetAndRestart(w http.ResponseWriter, r *http.Request, req reset.Request) {
	req.At = time.Now()
	out, err := s.resetCheck(r.Context())
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	req.Outside = out
	if err := reset.Ask(s.Cfg.DataDir, req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.WarnContext(r.Context(), "the server was asked to reset", "cat", "server", "mode", req.Mode, "by", userFrom(r).Name)
	writeJSON(w, 202, map[string]any{"restarting": s.Restart != nil})
	if s.Restart != nil {
		time.AfterFunc(300*time.Millisecond, s.Restart) // so the answer arrives
	}
}

// POST /api/reset (admin) {password, code, confirm: "RESET", delete}
func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	var b resetBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Confirm != "RESET" {
		writeErr(w, 400, "type RESET to confirm")
		return
	}
	if !s.resetAllowed(w, r, b) {
		return
	}
	mode := reset.ModeKeepCopy
	if b.Delete {
		mode = reset.ModeDelete
	}
	s.resetAndRestart(w, r, reset.Request{Mode: mode})
}

// POST /api/reset/restore (admin) {password, code, copy}
func (s *Server) handleResetRestore(w http.ResponseWriter, r *http.Request) {
	var b resetBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.resetAllowed(w, r, b) {
		return
	}
	s.resetAndRestart(w, r, reset.Request{Mode: reset.ModeRestore, Copy: b.Copy})
}

// DELETE /api/reset/request (admin): takes back a reset waiting for the next start.
func (s *Server) handleResetCancel(w http.ResponseWriter, r *http.Request) {
	found, err := reset.Cancel(s.Cfg.DataDir)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if found {
		s.Log.InfoContext(r.Context(), "a waiting reset was cancelled", "cat", "server", "by", userFrom(r).Name)
	}
	writeJSON(w, 200, map[string]any{"cancelled": found})
}

// POST /api/reset/copies/delete (admin) {password, code, copy}
func (s *Server) handleResetCopyDelete(w http.ResponseWriter, r *http.Request) {
	var b resetBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.resetAllowed(w, r, b) {
		return
	}
	if err := reset.DeleteCopy(s.Cfg.DataDir, b.Copy); err != nil {
		code := 500
		if err.Error() == "no such copy" {
			code = 404
		}
		writeErr(w, code, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "a copy from an earlier reset was deleted", "cat", "server", "copy", b.Copy, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"ok": true})
}
