// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"net/http"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
)

// Looking after a long channel list: many channels at once, likely duplicates, and checking
// that channels still work. Administrators only, like the rest of a tuner's settings.

func (s *Server) tvAdminRoutes(m *http.ServeMux) {
	ad := s.requireAdmin
	m.Handle("POST /api/tuner-channels/bulk", ad(s.handleBulkTunerChannels))
	m.Handle("GET /api/tuners/duplicates", ad(s.handleTunerDuplicates))
	m.Handle("POST /api/tuners/{id}/health", ad(s.handleStartHealth))
	m.Handle("GET /api/tuners/{id}/health", ad(s.handleChannelHealth))
	m.Handle("DELETE /api/tuners/{id}/health", ad(s.handleStopHealth))
}

// POST /api/tuner-channels/bulk {"ids":[…],"action":…,"start":"100","into":42}
func (s *Server) handleBulkTunerChannels(w http.ResponseWriter, r *http.Request) {
	if s.TV == nil {
		writeErr(w, 404, livetv.ErrNoEngine.Error())
		return
	}
	var body struct {
		IDs    []int64 `json:"ids"`
		Action string  `json:"action"`
		Start  string  `json:"start"`
		Into   int64   `json:"into"`
	}
	if err := readJSONUpTo(r, &body, 1<<20); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(body.IDs) == 0 || len(body.IDs) > livetv.MaxBulk {
		writeErr(w, 400, "choose between 1 and 20000 channels")
		return
	}
	ctx := r.Context()
	var err error
	switch body.Action {
	case "enable", "disable":
		err = s.TV.SetEnabled(ctx, body.IDs, body.Action == "enable")
	case "renumber":
		err = s.TV.Renumber(ctx, body.IDs, body.Start)
	case "merge":
		err = s.TV.Merge(ctx, body.Into, body.IDs)
	case "unmerge":
		err = s.TV.Unmerge(ctx, body.IDs)
	case "check":
		// A few channels, checked now; a whole tuner goes through /health.
		if len(body.IDs) > 50 {
			writeErr(w, 400, "check up to 50 channels at once, or check the whole tuner")
			return
		}
		results := map[int64]string{}
		for _, id := range body.IDs {
			if cerr := s.TV.CheckChannel(ctx, id); cerr != nil {
				results[id] = livetv.PublicError(cerr)
			} else {
				results[id] = ""
			}
			if ctx.Err() != nil {
				break
			}
		}
		writeJSON(w, 200, map[string]any{"ok": true, "results": results})
		return
	default:
		writeErr(w, 400, "action is enable, disable, renumber, merge, unmerge or check")
		return
	}
	if err != nil {
		if errors.Is(err, livetv.ErrNotFound) {
			writeErr(w, 404, "channel not found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/tuners/duplicates: channels across every tuner that look like the same station.
func (s *Server) handleTunerDuplicates(w http.ResponseWriter, r *http.Request) {
	if s.TV == nil {
		writeJSON(w, 200, map[string]any{"groups": []livetv.DupGroup{}})
		return
	}
	groups, err := s.TV.Duplicates(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if groups == nil {
		groups = []livetv.DupGroup{}
	}
	writeJSON(w, 200, map[string]any{"groups": groups})
}

func (s *Server) healthTuner(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := pathInt(r, "id")
	if !ok || s.TV == nil {
		writeErr(w, 404, "tuner not found")
		return 0, false
	}
	if _, err := s.TV.Tuner(r.Context(), id); err != nil {
		writeErr(w, 404, "tuner not found")
		return 0, false
	}
	return id, true
}

// POST /api/tuners/{id}/health: check every enabled channel of a tuner, in the background.
func (s *Server) handleStartHealth(w http.ResponseWriter, r *http.Request) {
	id, ok := s.healthTuner(w, r)
	if !ok {
		return
	}
	if err := s.TV.StartHealth(id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 202, s.TV.Health(id))
}

// GET /api/tuners/{id}/health: how that check is going.
func (s *Server) handleChannelHealth(w http.ResponseWriter, r *http.Request) {
	id, ok := s.healthTuner(w, r)
	if !ok {
		return
	}
	st := s.TV.Health(id)
	if st == nil {
		st = &livetv.HealthState{}
	}
	writeJSON(w, 200, st)
}

// DELETE /api/tuners/{id}/health: stop it.
func (s *Server) handleStopHealth(w http.ResponseWriter, r *http.Request) {
	id, ok := s.healthTuner(w, r)
	if !ok {
		return
	}
	s.TV.StopHealth(id)
	writeJSON(w, 200, map[string]any{"ok": true})
}
