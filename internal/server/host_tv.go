// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
)

// The host API's part for WittyTuner (module API 5): the guide stays the server's. WittyTuner
// fetches each of its guide sources and hands the guide over as it arrives; the server keeps the
// programmes of the channels it has, and says which channels without a guide id of their own it
// matched by name, for WittyTuner to keep. Only WittyTuner may call these.

const tunerModule = "wittytuner"

// maxGuide is how much one guide may be: a guide of a whole country, and more.
const maxGuide = 2 << 30

func (s *Server) tvHost(w http.ResponseWriter, r *http.Request, module string) (int64, bool) {
	if module != tunerModule {
		writeJSON(w, 403, map[string]string{"error": "only WittyTuner keeps a guide here"})
		return 0, false
	}
	if s.TV == nil {
		writeJSON(w, 503, map[string]string{"error": "live TV isn't running"})
		return 0, false
	}
	if r.PathValue("source") == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(r.PathValue("source"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, 400, map[string]string{"error": "a guide source is a number"})
		return 0, false
	}
	return id, true
}

// POST /host/v1/tv/guide/{source}: an XMLTV guide WittyTuner fetched from one of its sources.
// The answer is {programmes, matched: {channel id: guide id}}.
func (s *Server) hostTVGuide(w http.ResponseWriter, r *http.Request, module string) {
	id, ok := s.tvHost(w, r, module)
	if !ok {
		return
	}
	n, matched, err := s.TV.Ingest(r.Context(), id, http.MaxBytesReader(w, r.Body, maxGuide))
	if err != nil {
		s.Log.WarnContext(r.Context(), "a guide from WittyTuner couldn't be read", "cat", "tv", "source", id, "err", err)
		status := 400
		if errors.Is(err, livetv.ErrNoEngine) {
			status = 503
		}
		writeJSON(w, status, map[string]string{"error": livetv.PublicError(err)})
		return
	}
	out := make(map[string]string, len(matched))
	for cid, key := range matched {
		out[strconv.FormatInt(cid, 10)] = key
	}
	s.Log.InfoContext(r.Context(), "guide kept", "cat", "tv", "source", id, "programmes", n, "matched", len(out))
	writeJSON(w, 200, map[string]any{"programmes": n, "matched": out})
}

// DELETE /host/v1/tv/guide/{source}: a source WittyTuner removed; what it gave goes.
func (s *Server) hostTVGuideDrop(w http.ResponseWriter, r *http.Request, module string) {
	id, ok := s.tvHost(w, r, module)
	if !ok {
		return
	}
	if err := s.TV.DropSource(r.Context(), id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// GET /host/v1/tv/guide/ids: the guide's channel ids and names, for matching channels by hand.
func (s *Server) hostTVGuideIDs(w http.ResponseWriter, r *http.Request, module string) {
	if _, ok := s.tvHost(w, r, module); !ok {
		return
	}
	list, err := s.TV.GuideIDs(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, list)
}
