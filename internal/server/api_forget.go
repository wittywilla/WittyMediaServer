// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Removing downloaded details, so the owner can see matching and downloading happen again
// from a clean start (library.ForgetFiles and library.ForgetDetails say what goes and what
// stays). Administrators only.
//
//	POST /api/libraries/{id}/forget-metadata   one library
//	POST /api/libraries/forget-metadata        every library
//
// Body: {"files": bool, "details": bool, "rescan": bool}; files and details are on when left
// out, rescan is off. The answer is a library.ForgetResult plus "rescanning": the libraries
// whose scan was started.

type forgetBody struct {
	Files   *bool `json:"files"`
	Details *bool `json:"details"`
	Rescan  bool  `json:"rescan"`
}

func (s *Server) handleForgetMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	lib, err := s.Store.Library(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "library not found")
		return
	}
	s.forgetMetadata(w, r, []library.Library{*lib}, false)
}

func (s *Server) handleForgetAllMetadata(w http.ResponseWriter, r *http.Request) {
	libs, err := s.Store.Libraries(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.forgetMetadata(w, r, libs, true)
}

func (s *Server) forgetMetadata(w http.ResponseWriter, r *http.Request, libs []library.Library, every bool) {
	var b forgetBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	opt := library.ForgetOptions{Files: b.Files == nil || *b.Files, Details: b.Details == nil || *b.Details, ImagesDir: s.Cfg.ImagesDir}
	if !opt.Files && !opt.Details {
		writeErr(w, 400, "choose the files beside the media, the details in the server, or both")
		return
	}
	// A scan writes what this takes away: none may be going on in these libraries, and none
	// may start until this is done.
	for _, l := range libs {
		if s.Scanner.Running(l.ID) {
			writeErr(w, 409, l.Name+" is being scanned. Try again once the scan has finished, or stop it first.")
			return
		}
	}
	hold, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	release, err := s.Scanner.HoldScans(hold)
	cancel()
	if err != nil {
		writeErr(w, 409, "Another library is being scanned. Try again once the scan has finished.")
		return
	}
	defer release()

	u := userFrom(r)
	title := "Removing downloaded details from every library"
	spec := tasks.Spec{Type: tasks.TypeMetadata, Title: title, Unit: "libraries", By: byUser(u), AdminOnly: true}
	if !every && len(libs) == 1 {
		spec.Title, spec.LibraryID, spec.LibraryName = "Removing downloaded details from "+libs[0].Name, libs[0].ID, libs[0].Name
	}
	task := s.Tasks.Start(spec)
	ctx := context.WithoutCancel(r.Context()) // half done would be worse than done
	res := &library.ForgetResult{NotReset: []string{}}
	var errs []error
	for i, l := range libs {
		task.Item(l.Name, "")
		task.Progress(int64(i), int64(len(libs)))
		if opt.Files {
			if err := s.Store.ForgetFiles(ctx, &l, opt, res); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", l.Name, err))
			}
		}
		if opt.Details {
			if err := s.Store.ForgetDetails(ctx, &l, opt, res); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", l.Name, err))
			}
		}
		res.Libraries++
	}
	if opt.Details && every {
		if err := s.Store.ForgetEverywhereExtras(ctx); err != nil {
			errs = append(errs, err)
		}
		n, err := s.logos().ForgetDownloaded(ctx)
		res.Logos = n
		if err != nil {
			errs = append(errs, err)
		}
	}
	task.Progress(int64(len(libs)), int64(len(libs)))
	task.Item("", "")
	err = errors.Join(errs...)
	task.Result(forgetSummary(res, opt))
	task.Finish(err)
	names := make([]string, 0, len(libs))
	for _, l := range libs {
		names = append(names, l.Name)
	}
	logging.Audit(ctx, s.Log, "downloaded details removed", "cat", "library", "by", adminName(r), "libraries", strings.Join(names, ", "),
		"files", opt.Files, "details", opt.Details, "removed", res.Removed, "restored", res.Restored, "skipped", res.Skipped,
		"titles", res.Titles, "shows", res.Shows, "cached", res.Cached, "rescan", b.Rescan, "err", err)
	release()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	rescanning := []int64{}
	if b.Rescan {
		for _, l := range libs {
			if s.Scanner.StartBy(l.ID, byUser(u)) {
				rescanning = append(rescanning, l.ID)
			}
		}
	}
	writeJSON(w, 200, struct {
		*library.ForgetResult
		Rescanning []int64 `json:"rescanning"`
	}{res, rescanning})
}

// forgetSummary is the line the activity panel shows when it's done.
func forgetSummary(res *library.ForgetResult, opt library.ForgetOptions) string {
	var parts []string
	if opt.Files {
		parts = append(parts, fmt.Sprintf("%d files removed, %d NFOs put back, %d left as they were", res.Removed, res.Restored, res.Skipped))
	}
	if opt.Details {
		parts = append(parts, fmt.Sprintf("%d titles and %d shows reset", res.Titles, res.Shows))
	}
	return strings.Join(parts, "; ")
}
