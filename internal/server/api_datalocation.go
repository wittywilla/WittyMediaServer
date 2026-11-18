// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/datahome"
	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Where the server keeps its settings and data: the setup step that asks, and Settings → Server,
// which can move it later. The move itself happens at the next start (datahome.Apply).

type dataLocationView struct {
	DataDir string `json:"dataDir"`
	From    string `json:"from"` // config.DataFrom*
	// Movable says it can be moved from here; Why says why not.
	Movable bool   `json:"movable"`
	Why     string `json:"why,omitempty"`
	// Ask says first-time setup asks where to keep it: a desktop install that hasn't been told.
	Ask     bool              `json:"ask"`
	Options []datahome.Option `json:"options"`
	// Local is a folder on this computer that no sync program keeps, to suggest instead of a synced one.
	Local      string `json:"local"`
	Synced     string `json:"synced,omitempty"` // the data folder now is synced, by this
	Files      int    `json:"files"`
	Bytes      int64  `json:"bytes"`
	Sep        string `json:"sep"`
	CanRestart bool   `json:"canRestart"`
	// Temporary video and caches set apart from the data folder, which a move leaves where they are.
	Apart   []string          `json:"apart,omitempty"`
	Pending *datahome.Request `json:"pending,omitempty"`
	Last    *datahome.Result  `json:"last,omitempty"`
	Old     *datahome.Moved   `json:"old,omitempty"`
}

// askDataLocation: first-time setup on a desktop asks, while the server is new and nothing chose its folder.
func (s *Server) askDataLocation(ctx context.Context) bool {
	if s.Cfg.DataFrom != config.DataFromDefault || desktop.Headless() {
		return false
	}
	if s.DB.GetSetting(ctx, "setupDone", "") != "0" {
		return false
	}
	libs, _ := s.Store.Libraries(ctx)
	return len(libs) == 0
}

// GET /api/data-location (admin)
func (s *Server) handleDataLocation(w http.ResponseWriter, r *http.Request) {
	data := s.Cfg.DataDir
	files, size := datahome.Footprint(data)
	v := dataLocationView{DataDir: data, From: s.Cfg.DataFrom, Ask: s.askDataLocation(r.Context()), Files: files, Bytes: size,
		Sep: string(filepath.Separator), CanRestart: s.Restart != nil, Local: config.LegacyDataDir(), Synced: config.Synced(data),
		Pending: datahome.Pending(data), Last: datahome.Last(data), Old: datahome.OldCopy(data)}
	v.Movable, v.Why = datahome.Movable(s.Cfg.DataFrom)
	if v.Movable {
		v.Options = datahome.Options(data, size)
	} else {
		v.Options = []datahome.Option{}
	}
	for _, d := range []string{s.Cfg.TranscodeDir, s.Cfg.CacheDir} {
		if d != "" && !datahome.Same(filepath.Dir(d), data) {
			v.Apart = append(v.Apart, d)
		}
	}
	writeJSON(w, 200, v)
}

type dataLocationBody struct {
	Choice string `json:"choice"` // documents, install or custom
	Path   string `json:"path"`   // custom only
}

// target is the folder a choice means, or a sentence saying why it can't be had.
func (s *Server) dataTarget(r *http.Request, b dataLocationBody) (string, string) {
	switch b.Choice {
	case config.ChoiceDocuments, config.ChoiceInstall:
		for _, o := range datahome.Options(s.Cfg.DataDir, 0) {
			if o.Choice == b.Choice {
				if !o.Available {
					return "", o.Why
				}
				return o.Path, ""
			}
		}
	case config.ChoiceCustom:
		p := strings.TrimSpace(b.Path)
		if p == "" || !filepath.IsAbs(p) {
			return "", "Type the folder's full path."
		}
		if err := s.checkAdminPaths(r, []string{p}, nil); err != nil {
			return "", err.Error()
		}
		return filepath.Clean(p), ""
	}
	return "", "Choose Documents, the install folder or a folder of your own."
}

// POST /api/data-location/check (admin) {choice, path}: what choosing it would mean, before it's chosen.
func (s *Server) handleDataLocationCheck(w http.ResponseWriter, r *http.Request) {
	var b dataLocationBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	to, why := s.dataTarget(r, b)
	out := map[string]any{"path": to, "problem": why}
	if to != "" {
		_, size := datahome.Footprint(s.Cfg.DataDir)
		out["free"], out["synced"], out["current"] = datahome.FreeBytes(to), config.Synced(to), datahome.Same(to, s.Cfg.DataDir)
		if !datahome.Same(to, s.Cfg.DataDir) {
			out["problem"] = datahome.CheckTarget(s.Cfg.DataDir, to, size)
		}
	}
	writeJSON(w, 200, out)
}

// POST /api/data-location (admin) {choice, path}: keep the data there from the next start. The
// server restarts to move it. In first-time setup the old folder is cleared once the copy is
// checked; otherwise it's kept until someone deletes it.
func (s *Server) handleDataLocationSet(w http.ResponseWriter, r *http.Request) {
	if ok, why := datahome.Movable(s.Cfg.DataFrom); !ok {
		writeErr(w, 409, why)
		return
	}
	var b dataLocationBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	to, why := s.dataTarget(r, b)
	if why != "" {
		writeErr(w, 400, why)
		return
	}
	ctx := r.Context()
	by := userFrom(r).Name
	// Already there: only the choice needs remembering, so setup doesn't ask again.
	if datahome.Same(to, s.Cfg.DataDir) {
		if s.Cfg.DataFrom != config.DataFromPointer {
			if err := config.WritePointer(config.Pointer{DataDir: s.Cfg.DataDir, Choice: b.Choice}); err != nil {
				writeErr(w, 500, "the choice couldn't be saved: "+err.Error())
				return
			}
			s.Cfg.DataFrom = config.DataFromPointer
		}
		s.Log.InfoContext(ctx, "the data folder stays where it is", "cat", "server", "dir", to, "by", by)
		writeJSON(w, 200, map[string]any{"restarting": false, "dataDir": to})
		return
	}
	if s.Restart == nil {
		writeErr(w, 409, "This server can't restart itself, and the data moves while it starts. Stop it, move the folder, and start it with -data.")
		return
	}
	_, size := datahome.Footprint(s.Cfg.DataDir)
	if p := datahome.CheckTarget(s.Cfg.DataDir, to, size); p != "" {
		writeErr(w, 400, p)
		return
	}
	req := datahome.Request{To: to, Choice: b.Choice, KeepOld: !s.askDataLocation(ctx), By: by, At: time.Now().UTC()}
	if err := datahome.Ask(s.Cfg.DataDir, req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.WarnContext(ctx, "the data folder is moving at the next start", "cat", "server", "from", s.Cfg.DataDir, "to", to, "keepOld", req.KeepOld, "by", by)
	writeJSON(w, 202, map[string]any{"restarting": true, "dataDir": to})
	logging.NoteStop(logging.StopRestartSetting, "moving the data folder", by)
	time.AfterFunc(300*time.Millisecond, s.Restart) // so the answer arrives
}

// DELETE /api/data-location/request (admin): takes back a move waiting for the next start.
func (s *Server) handleDataLocationCancel(w http.ResponseWriter, r *http.Request) {
	found, err := datahome.Cancel(s.Cfg.DataDir)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"cancelled": found})
}

// POST /api/data-location/old/delete (admin) {password, code}: deletes the copy a move kept, once someone is happy
// with the new one. Only what was copied out of it goes.
func (s *Server) handleDataLocationDeleteOld(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.confirmOwner(w, r, b.Password, b.Code) {
		return
	}
	m, err := datahome.DeleteOld(s.Cfg.DataDir)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "the old copy of the data folder was deleted", "cat", "server", "dir", m.From, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"deleted": m.From})
}

// GET /api/data-location/progress (admin): the server is back. While a move runs, the server's
// port answers this with how far it is instead (cmd/wittymediaserver/moving.go).
func (s *Server) handleDataLocationProgress(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"moving": false, "dataDir": s.Cfg.DataDir, "last": datahome.Last(s.Cfg.DataDir)})
}
