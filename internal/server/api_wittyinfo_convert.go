// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"net/http"
	"strconv"
	"strings"
	"sync"
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
//	GET  /api/wittyinfo/convert/report?task=ID          what a finished one did, counted by kind

func (s *Server) wittyinfoConvertRoutes(m *http.ServeMux) {
	m.Handle("GET /api/wittyinfo/convert/counts", s.requireAdmin(s.handleConvertCounts))
	m.Handle("POST /api/wittyinfo/convert", s.requireAdmin(s.handleConvert))
	m.Handle("GET /api/wittyinfo/convert/report", s.requireAdmin(s.handleConvertReport))
}

// GET /api/wittyinfo/convert/report?task=ID: the counts behind a finished conversion's result, so
// a page can say it in its own language and words (films and episodes, shows, albums, songs).
func (s *Server) handleConvertReport(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("task")
	j := &s.convertJobs
	j.mu.Lock()
	rep, ok := j.reports[id]
	j.mu.Unlock()
	if !ok {
		writeErr(w, 404, "no report for that task")
		return
	}
	writeJSON(w, 200, rep)
}

// convertResult is a finished conversion's result in a sentence, for Tasks and the log, counting
// what the format holds: films and episodes, and shows (.wittyinfo); albums, and songs' lyrics
// (.wittyaudio).
func convertResult(format string, rep library.ConvertReport) string {
	plural := func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	}
	var parts []string
	if format == "" || format == "wittyinfo" {
		if rep.Titles > 0 || rep.Shows == 0 {
			parts = append(parts, fmt.Sprintf("%d of %d %s converted", rep.TitlesConverted, rep.Titles, plural(rep.Titles, "film or episode", "films and episodes")))
		}
		if rep.Shows > 0 {
			parts = append(parts, fmt.Sprintf("%d of %d %s", rep.ShowsConverted, rep.Shows, plural(rep.Shows, "show", "shows")))
		}
	}
	if format == "" || format == "wittyaudio" {
		if rep.Albums > 0 || rep.Songs == 0 || format == "wittyaudio" {
			parts = append(parts, fmt.Sprintf("%d of %d %s converted", rep.AlbumsConverted, rep.Albums, plural(rep.Albums, "album", "albums")))
		}
		if rep.SongsConverted > 0 || format == "wittyaudio" {
			parts = append(parts, fmt.Sprintf("lyrics for %d of %d %s", rep.SongsConverted, rep.Songs, plural(rep.Songs, "song", "songs")))
		}
	}
	if rep.Retired > 0 {
		parts = append(parts, fmt.Sprintf("%d old %s renamed to .bak", rep.Retired, plural(rep.Retired, "file", "files")))
	}
	if rep.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d couldn't be (see the log)", rep.Failed))
	}
	return strings.Join(parts, "; ") + "."
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
		case "movies", "shows", "anime", "videos", "discs", "music", "audiobooks":
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
	// Format: "wittyinfo" (films and shows) or "wittyaudio" (songs); left out, both. Each
	// format runs as a task of its own, so a page can show each one's progress.
	Format string `json:"format"`
}

// POST /api/wittyinfo/convert
func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	var b convertBody
	if err := readJSON(r, &b); err != nil || (b.Direction != "gather" && b.Direction != "spread") || (b.Format != "" && b.Format != "wittyinfo" && b.Format != "wittyaudio") {
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
	// One run of each format at a time.
	key := b.Format
	if key == "" {
		key = "all"
	}
	j := &s.convertJobs
	j.mu.Lock()
	if j.running[key] || j.running["all"] || (key == "all" && len(j.running) > 0) {
		j.mu.Unlock()
		writeErr(w, 409, "A conversion is already running. Wait for it to finish, or stop it in Tasks.")
		return
	}
	if j.running == nil {
		j.running = map[string]bool{}
	}
	j.running[key] = true
	j.mu.Unlock()
	done := func() { j.mu.Lock(); delete(j.running, key); j.mu.Unlock() }

	u := userFrom(r)
	author := u.DisplayName
	if author == "" {
		author = u.Name
	}
	o := library.ConvertOptions{Subtitles: b.Subtitles, Retire: b.Retire, Beside: s.saveInfoBeside(r), Author: author, Tool: "WittyMediaServer", ToolVersion: s.Version, Format: b.Format}
	titles := map[string]string{
		"gather/":           i18n.T("Gathering NFO files, pictures and lyrics into .wittyinfo and .wittyaudio files"),
		"gather/wittyinfo":  i18n.T("Gathering NFO files and pictures into .wittyinfo files"),
		"gather/wittyaudio": i18n.T("Gathering lyrics, covers and album details into .wittyaudio files"),
		"spread/":           i18n.T("Writing NFO files, pictures and lyrics from .wittyinfo and .wittyaudio files"),
		"spread/wittyinfo":  i18n.T("Writing NFO files and pictures from .wittyinfo files"),
		"spread/wittyaudio": i18n.T("Writing lyrics, covers and album details from .wittyaudio files"),
	}
	title := titles[b.Direction+"/"+b.Format]
	stopped, stop := context.WithCancel(context.Background())
	unit := "titles"
	if b.Format == "wittyaudio" {
		unit = "songs"
	}
	task := s.Tasks.Start(tasks.Spec{Type: tasks.TypeMetadata, Title: title, Unit: unit, By: byUser(u), Cancel: stop, AdminOnly: true})
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
		task.Item(i18n.T("Waiting for the libraries to be scanned"), "")
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
		j.keep(task.ID(), rep)
		task.Result(convertResult(b.Format, rep))
		log.Info("converted", "format", b.Format, "titles", rep.Titles, "shows", rep.Shows, "songs", rep.Songs, "albums", rep.Albums, "converted", rep.Converted, "failed", rep.Failed, "retired", rep.Retired, "errors", strings.Join(rep.Errors, " | "))
		task.Finish(nil)
	}, "cat", "library", "session", sid)
	if !started {
		done()
		stop()
		task.Finish(context.Canceled)
		writeErr(w, 503, "the server is stopping")
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "task": task.ID(), "libraries": libs, "format": b.Format})
}

// convertJobs: the formats being converted now, and the reports of the last few that finished.
type convertJobs struct {
	mu      sync.Mutex
	running map[string]bool
	reports map[string]library.ConvertReport
	order   []string
}

// keep remembers a finished conversion's report by its task, the last 20 of them.
func (j *convertJobs) keep(task string, rep library.ConvertReport) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.reports == nil {
		j.reports = map[string]library.ConvertReport{}
	}
	rep.Errors = nil // for the log, not for pages
	j.reports[task] = rep
	j.order = append(j.order, task)
	for len(j.order) > 20 {
		delete(j.reports, j.order[0])
		j.order = j.order[1:]
	}
}
