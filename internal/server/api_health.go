// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/diagnose"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Settings, Library health: what in the libraries needs a look, with a fix for each where
// there is one (internal/library/health.go). Administrators only. The fixes that already
// had a route (a scan, leaving a title unmatched, fetching its details again) use theirs;
// the ones here are what was missing: copies kept as versions, a gone file forgotten, a
// file read again, and setting a row aside as fine.

const (
	healthSetAsideSetting = "libraryHealthSetAside"
	healthSetAsideMax     = 5000
	healthCheckTime       = 3 * time.Minute
	healthCheckFiles      = 100000
)

type healthState struct {
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    int
	total   int
	last    *library.SidecarCheck
	err     string
}

func (s *Server) healthRoutes(m *http.ServeMux) {
	m.Handle("GET /api/health/library", s.requireAdmin(s.handleHealthView))
	m.Handle("POST /api/health/library/check", s.requireAdmin(s.handleHealthCheck))
	m.Handle("DELETE /api/health/library/check", s.requireAdmin(s.handleHealthCheckStop))
	m.Handle("POST /api/health/library/fix", s.requireAdmin(s.handleHealthFix))
}

type healthCheckView struct {
	Running bool   `json:"running"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	At      int64  `json:"at,omitempty"`
	Stopped bool   `json:"stopped,omitempty"` // the last one ran out of time before the end
	Error   string `json:"error,omitempty"`
}

type healthView struct {
	*library.HealthReport
	Check healthCheckView `json:"check"`
}

func (s *Server) healthSetAside(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	var keys []string
	if v := s.DB.GetSetting(ctx, healthSetAsideSetting, ""); v != "" {
		if err := json.Unmarshal([]byte(v), &keys); err != nil {
			s.Log.Warn("the rows set aside on Library health can't be read", "cat", "server", "err", err)
		}
	}
	for _, k := range keys {
		out[k] = true
	}
	return out
}

func (s *Server) saveSetAside(ctx context.Context, set map[string]bool) error {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	if len(keys) > healthSetAsideMax {
		return fmt.Errorf("at most %d things can be set aside: bring some back first", healthSetAsideMax)
	}
	js, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	return s.DB.SetSetting(ctx, healthSetAsideSetting, string(js))
}

func (s *Server) healthCheckView() healthCheckView {
	st := &s.health
	st.mu.Lock()
	defer st.mu.Unlock()
	v := healthCheckView{Running: st.running, Done: st.done, Total: st.total, Error: st.err}
	if st.last != nil {
		v.At, v.Stopped = st.last.At.Unix(), st.last.Stopped
	}
	return v
}

// healthReport works out the page, or one section of it.
func (s *Server) healthReport(ctx context.Context, lib int64, section string, limit int) (*library.HealthReport, error) {
	s.health.mu.Lock()
	last := s.health.last
	s.health.mu.Unlock()
	return s.Scanner.Health(ctx, library.HealthOptions{Library: lib, Only: section, Limit: limit, SetAside: s.healthSetAside(ctx), Sidecars: last})
}

// GET /api/health/library?library=&section=&limit=
func (s *Server) handleHealthView(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lib, _ := strconv.ParseInt(q.Get("library"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	section := q.Get("section")
	if section != "" {
		known := false
		for _, id := range library.HealthSections {
			known = known || id == section
		}
		if !known {
			writeErr(w, 400, "there's no such section")
			return
		}
	}
	check := s.healthCheckView() // first: the report is then never older than what it says about the check
	rep, err := s.healthReport(r.Context(), lib, section, limit)
	if errors.Is(err, library.ErrNotFound) {
		writeErr(w, 404, "there's no such library")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, healthView{HealthReport: rep, Check: check})
}

// POST /api/health/library/check: read the NFO and metadata files beside the media, in
// the background. It shows in Activity, and the page asks how it's going.
func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	st := &s.health
	st.mu.Lock()
	if st.running {
		st.mu.Unlock()
		writeJSON(w, 202, s.healthCheckView())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	st.running, st.cancel, st.done, st.total, st.err = true, cancel, 0, 0, ""
	st.mu.Unlock()
	var total int
	_ = s.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM items WHERE missing = 0`).Scan(&total)
	st.mu.Lock()
	st.total = total
	st.mu.Unlock()
	t := s.Tasks.Start(tasks.Spec{Type: tasks.TypeDiagnose, Title: "Checking NFO files", Unit: "titles", By: byUser(userFrom(r)), Cancel: cancel, AdminOnly: true})
	t.Progress(0, int64(total))
	logging.Go("library health check", func() {
		defer cancel()
		res, err := s.Store.CheckSidecars(ctx, library.SidecarLimits{Files: healthCheckFiles, Deadline: time.Now().Add(healthCheckTime)}, diagnose.SidecarProblem, func(done int) {
			st.mu.Lock()
			st.done = done
			st.mu.Unlock()
			t.Progress(int64(done), int64(total))
		})
		st.mu.Lock()
		st.running, st.cancel = false, nil
		if err != nil {
			if errors.Is(err, context.Canceled) {
				st.err = "The check was stopped."
			} else {
				st.err = err.Error()
			}
		} else {
			st.last, st.done = res, total
		}
		st.mu.Unlock()
		if err == nil {
			msg := fmt.Sprintf("%d files can't be read", len(res.Rows))
			switch len(res.Rows) {
			case 0:
				msg = "Every NFO file can be read"
			case 1:
				msg = "1 file can't be read"
			}
			if res.Stopped {
				msg += " (it stopped early, to keep from running too long)"
			}
			t.Result(msg)
		}
		t.Finish(err)
	}, "cat", "library")
	writeJSON(w, 202, s.healthCheckView())
}

// DELETE /api/health/library/check: stop it.
func (s *Server) handleHealthCheckStop(w http.ResponseWriter, _ *http.Request) {
	s.health.mu.Lock()
	if s.health.cancel != nil {
		s.health.cancel()
	}
	s.health.mu.Unlock()
	writeJSON(w, 200, s.healthCheckView())
}

// POST /api/health/library/fix {action, key, target, id, ids}
//
//	versions {ids}: keep the copies as versions of the first (the better copy)
//	forget {target, id}: a title whose file has gone for good, with its history
//	probe {id}: read a video file's details again
//	setaside {key}: it's fine as it is; bringback {key} or {} for every one
func (s *Server) handleHealthFix(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Action string  `json:"action"`
		Key    string  `json:"key"`
		Target string  `json:"target"`
		ID     int64   `json:"id"`
		IDs    []int64 `json:"ids"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	who := userFrom(r).Name
	switch b.Action {
	case "versions":
		if err := s.Store.MakeVersions(ctx, b.IDs); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		s.Log.InfoContext(ctx, "copies were kept as versions of one film", "cat", "library", "ids", fmt.Sprint(b.IDs), "by", who)
		writeJSON(w, 200, map[string]any{"result": fmt.Sprintf("%d copies are now versions of one film.", len(b.IDs))})
	case "forget":
		if b.Target != library.TargetItem && b.Target != library.TargetShow {
			writeErr(w, 400, "choose a title")
			return
		}
		err := s.Store.ForgetGone(ctx, b.Target, b.ID)
		switch {
		case errors.Is(err, library.ErrNotFound):
			writeErr(w, 404, "there's no such title")
			return
		case errors.Is(err, library.ErrStillThere):
			writeErr(w, 409, "Its file is there, or its drive isn't connected, so nothing was forgotten. Scan the library instead.")
			return
		case err != nil:
			writeErr(w, 500, err.Error())
			return
		}
		s.Log.InfoContext(ctx, "a title whose file had gone was forgotten", "cat", "library", "target", b.Target, "id", b.ID, "by", who)
		writeJSON(w, 200, map[string]any{"result": "Forgotten."})
	case "probe":
		it, err := s.Scanner.ReadAgain(ctx, b.ID)
		if errors.Is(err, library.ErrNotFound) {
			writeErr(w, 404, "there's no such title")
			return
		}
		if err != nil {
			writeErr(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"result": "It can be read now: " + strings.ToUpper(it.Container) + "."})
	case "setaside", "bringback":
		set := s.healthSetAside(ctx)
		switch {
		case b.Action == "setaside" && (b.Key == "" || len(b.Key) > 2048):
			writeErr(w, 400, "choose something to set aside")
			return
		case b.Action == "setaside":
			set[b.Key] = true
		case b.Key == "":
			set = map[string]bool{}
		default:
			delete(set, b.Key)
		}
		if err := s.saveSetAside(ctx, set); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"setAside": len(set)})
	default:
		writeErr(w, 400, "choose versions, forget, probe, setaside or bringback")
	}
}

// healthSummary is the counts, for TV mode's card and anything short.
func (s *Server) healthSummary(ctx context.Context) []string {
	rep, err := s.healthReport(ctx, 0, "", 1)
	if err != nil {
		return nil
	}
	words := map[string][2]string{
		library.HealthMissing:    {"file has gone", "files have gone"},
		library.HealthUnmatched:  {"title has no match", "titles have no match"},
		library.HealthDuplicates: {"title may be a duplicate", "titles may be duplicates"},
		library.HealthNames:      {"file has an odd name", "files have odd names"},
		library.HealthNFO:        {"NFO file can't be read", "NFO files can't be read"},
		library.HealthArtwork:    {"title has no artwork", "titles have no artwork"},
		library.HealthUnreadable: {"file can't be read", "files can't be read"},
	}
	var out []string
	for _, sec := range rep.Sections {
		if sec.Count > 0 {
			out = append(out, countOf(sec.Count, words[sec.ID][0], words[sec.ID][1]))
		}
	}
	if len(out) == 0 {
		out = append(out, "Nothing needs a look")
	}
	return out
}

func countOf(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
