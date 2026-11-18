// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Converting libraries between NFO files and pictures and .wittyinfo files (first-run setup's
// Posters and details, and Settings, Metadata). See internal/library/wittyinfo_convert.go.
//
//	GET  /api/wittyinfo/convert/counts?libraries=1,2   what there is to convert
//	POST /api/wittyinfo/convert                         start it as a task

func (s *Server) wittyinfoConvertRoutes(m *http.ServeMux) {
	m.Handle("GET /api/wittyinfo/convert/counts", s.requireAdmin(s.handleConvertCounts))
	m.Handle("POST /api/wittyinfo/convert", s.requireAdmin(s.handleConvert))
}

// convertLibraries are the libraries asked for, or every one with films or shows.
func (s *Server) convertLibraries(ctx context.Context, asked []int64) ([]int64, error) {
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return nil, err
	}
	want := map[int64]bool{}
	for _, id := range asked {
		want[id] = true
	}
	var out []int64
	for _, l := range libs {
		switch l.Type {
		case "movies", "shows", "anime", "videos", "discs":
		default:
			continue
		}
		if len(asked) == 0 || want[l.ID] {
			out = append(out, l.ID)
		}
	}
	return out, nil
}

func idList(v string) ([]int64, error) {
	var out []int64
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil || n <= 0 {
			return nil, errors.New("libraries: numbers separated by commas")
		}
		out = append(out, n)
	}
	if len(out) > 1000 {
		return nil, errors.New("too many libraries")
	}
	return out, nil
}

// GET /api/wittyinfo/convert/counts
func (s *Server) handleConvertCounts(w http.ResponseWriter, r *http.Request) {
	asked, err := idList(r.URL.Query().Get("libraries"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	libs, err := s.convertLibraries(r.Context(), asked)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	c, err := s.Store.CountConvertible(ctx, libs)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"counts": c, "canWrite": s.saveInfoBeside(r), "saveMetadata": s.saveMeta(r)})
}

// convertBody is the body of POST /api/wittyinfo/convert.
type convertBody struct {
	// Direction: "gather" puts NFO files, pictures and skips in .wittyinfo files; "spread"
	// writes them back out of the .wittyinfo files.
	Direction string  `json:"direction"`
	Libraries []int64 `json:"libraries"`
	Subtitles bool    `json:"subtitles"`
	Retire    bool    `json:"retire"`
}

// POST /api/wittyinfo/convert
func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	var b convertBody
	if err := readJSON(r, &b); err != nil || (b.Direction != "gather" && b.Direction != "spread") {
		writeErr(w, 400, `expected {"direction": "gather" or "spread", "libraries": [...], "subtitles", "retire"}`)
		return
	}
	if len(b.Libraries) > 1000 {
		writeErr(w, 400, "too many libraries")
		return
	}
	if s.Scanner == nil {
		writeErr(w, 503, "the library isn't ready")
		return
	}
	libs, err := s.convertLibraries(r.Context(), b.Libraries)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(libs) == 0 {
		writeErr(w, 409, "There is no library of films or shows to convert.")
		return
	}
	if b.Direction == "spread" && !s.saveMeta(r) {
		writeErr(w, 409, "Writing NFO files and pictures beside your media is off. Turn on “Save information and artwork next to your media” first.")
		return
	}
	j := &s.convertJob
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		writeErr(w, 409, "A conversion is already running. Wait for it to finish, or stop it in Tasks.")
		return
	}
	j.running = true
	j.mu.Unlock()
	done := func() { j.mu.Lock(); j.running = false; j.mu.Unlock() }

	u := userFrom(r)
	author := u.DisplayName
	if author == "" {
		author = u.Name
	}
	o := library.ConvertOptions{Subtitles: b.Subtitles, Retire: b.Retire, Beside: s.saveInfoBeside(r), Author: author, Tool: "WittyMediaServer", ToolVersion: s.Version}
	title := "Gathering NFO files and pictures into .wittyinfo files"
	if b.Direction == "spread" {
		title = "Writing NFO files and pictures from .wittyinfo files"
	}
	stopped, stop := context.WithCancel(context.Background())
	task := s.Tasks.Start(tasks.Spec{Type: tasks.TypeMetadata, Title: title, Unit: "titles", By: byUser(u), Cancel: stop, AdminOnly: true})
	sid := logging.NewID("convert")
	log := s.Log.With("cat", "library", "session", sid, "direction", b.Direction)
	started := s.background("wittyinfo conversion", func(bg context.Context) {
		defer done()
		defer stop()
		ctx, c := context.WithCancel(pace.Background(logging.WithSession(bg, sid)))
		defer c()
		defer context.AfterFunc(stopped, c)()
		// A library still being scanned has titles yet to come: wait for it (setup starts this
		// while the first scans run).
		task.Item("Waiting for the libraries to be scanned", "")
		for waited := 0; waited < 6*3600; waited += 5 {
			busy := false
			if all, err := s.Store.Libraries(ctx); err == nil {
				for _, l := range all {
					for _, id := range libs {
						busy = busy || (l.ID == id && l.Scanning)
					}
				}
			}
			if !busy {
				break
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
			}
			if ctx.Err() != nil {
				break
			}
		}
		progress := func(n, total int, what string) {
			task.Item(what, "")
			task.Progress(int64(n), int64(total))
		}
		var rep library.ConvertReport
		var err error
		if b.Direction == "gather" {
			rep, err = s.Scanner.GatherInfo(ctx, libs, o, progress)
		} else {
			rep, err = s.Scanner.SpreadInfo(ctx, libs, o, progress)
		}
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Error("converting failed", "err", err)
			}
			task.Finish(err)
			return
		}
		msg := fmt.Sprintf("%d of %d titles and shows converted", rep.Converted, rep.Titles+rep.Shows)
		if rep.Retired > 0 {
			msg += fmt.Sprintf("; %d old files renamed to .bak", rep.Retired)
		}
		if rep.Failed > 0 {
			msg += fmt.Sprintf("; %d couldn't be (see the log)", rep.Failed)
		}
		task.Result(msg + ".")
		log.Info("converted", "titles", rep.Titles, "shows", rep.Shows, "converted", rep.Converted, "failed", rep.Failed, "retired", rep.Retired, "errors", strings.Join(rep.Errors, " | "))
		task.Finish(nil)
	}, "cat", "library", "session", sid)
	if !started {
		done()
		stop()
		task.Finish(context.Canceled)
		writeErr(w, 503, "the server is stopping")
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "task": task.ID(), "libraries": libs})
}
