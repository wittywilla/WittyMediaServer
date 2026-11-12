// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// A whole show marked watched or unwatched. The server-driven UI names a show "show-<n>"
// and its commands (watched.set, watched.clear) send that ID to /api/items/{id}/progress,
// as they do a film's or an episode's number, so the same template works for every title.

// showRef reads a path's {id} of the form show-<n>.
func showRef(r *http.Request) (int64, bool) {
	rest, ok := strings.CutPrefix(r.PathValue("id"), "show-")
	if !ok {
		return 0, false
	}
	n, err := strconv.ParseInt(rest, 10, 64)
	return n, err == nil && n > 0
}

// handleShowWatched marks every episode of a show this profile may see watched (watched
// true: POST with {"watched": true}) or forgets its progress in them (DELETE). A show
// the profile may not see is 404, whether or not it exists.
func (s *Server) handleShowWatched(w http.ResponseWriter, r *http.Request, id int64, watched bool) {
	if _, err := s.visibleShow(r, id); err != nil {
		writeErr(w, 404, "show not found")
		return
	}
	if watched {
		var p library.Progress
		if err := readJSON(r, &p); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		if !p.Watched {
			writeErr(w, 400, `a show takes {"watched": true}; DELETE forgets the progress in it`)
			return
		}
	}
	eps, err := s.Store.Episodes(r.Context(), id, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	uid := userFrom(r).ID
	for _, ep := range eps {
		if err := r.Context().Err(); err != nil {
			return
		}
		if watched {
			err = s.Store.SetProgress(r.Context(), uid, ep.ID, library.Progress{Watched: true, DurationSec: ep.RuntimeSec, PositionSec: ep.RuntimeSec})
		} else {
			err = s.Store.ClearProgress(r.Context(), uid, ep.ID)
		}
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "episodes": len(eps)})
}
