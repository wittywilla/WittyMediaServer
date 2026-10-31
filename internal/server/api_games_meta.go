package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// What's known about games, and where it comes from. The work happens in the
// background (library.GameMeta); these routes show it and steer it. See docs/METADATA.md.

func init() {
	settingKeys = append(settingKeys, library.GameSourceKeys...)
	secretSettings = append(secretSettings, library.GameSourceKeys...)
}

func (s *Server) gameMetaRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/games/facets", a(s.handleGameFacets))
	m.Handle("GET /api/games/{id}/metadata", a(s.handleGameMetadata))
	m.Handle("GET /api/games/{id}/art/{n}", a(s.handleGameArt))
	m.Handle("GET /api/games/{id}/metadata/search", s.requireAdmin(s.handleGameSearch))
	m.Handle("PUT /api/games/{id}/metadata/match", s.requireAdmin(s.handleGameMatch))
	m.Handle("DELETE /api/games/{id}/metadata/match", s.requireAdmin(s.handleGameUnmatch))
	m.Handle("POST /api/games/{id}/metadata/refresh", s.requireAdmin(s.handleGameRefresh))
	m.Handle("GET /api/gamemeta/status", s.requireAdmin(s.handleGameMetaStatus))
	m.Handle("POST /api/gamemeta/refetch", s.requireAdmin(s.handleGameMetaRefetch))
	m.Handle("GET /api/gamemeta/sources", s.requireAdmin(s.handleGameSources))
	m.Handle("PUT /api/gamemeta/sources", s.requireAdmin(s.handlePutGameSources))
	m.Handle("POST /api/gamemeta/sources/{id}/test", s.requireAdmin(s.handleGameSourceTest))
}

// visibleGame loads a game the signed-in account may see, answering for itself if not.
func (s *Server) visibleGame(w http.ResponseWriter, r *http.Request) *library.Item {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "game" {
		writeErr(w, 404, "game not found")
		return nil
	}
	return it
}

// GET /api/games/{id}/metadata
func (s *Server) handleGameMetadata(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	d, err := s.Store.GameDetails(r.Context(), it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !userFrom(r).IsAdmin {
		d.CRCs, d.Serial = "", ""
	}
	writeJSON(w, 200, d)
}

// GET /api/games/{id}/art/{n}: one of a game's screenshots, fetched by the server the
// first time anyone looks, so a browser never talks to the source.
func (s *Server) handleGameArt(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		writeErr(w, 400, "bad picture number")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	cache, err := s.GameMeta.GameArtPicture(ctx, it.ID, n)
	if err != nil {
		writeErr(w, 404, "no such picture")
		return
	}
	s.serveArtSrc(w, r, cache)
}

// GET /api/games/facets?library=&console=
func (s *Server) handleGameFacets(w http.ResponseWriter, r *http.Request) {
	id := int64(qInt(r, "library", 0))
	if !s.access(r).AllowsLibrary(id) {
		writeErr(w, 404, "library not found")
		return
	}
	f, err := s.Store.GameFacets(r.Context(), id, r.URL.Query().Get("console"), s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, f)
}

// GET /api/games/{id}/metadata/search?q=: games this might be, from every source in use.
func (s *Server) handleGameSearch(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = it.Title
	}
	if len(q) > 200 {
		q = q[:200]
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	results, failed := s.GameMeta.Search(ctx, it.Console, q)
	if failed == nil {
		failed = []string{}
	}
	writeJSON(w, 200, map[string]any{"query": q, "results": results, "failed": failed})
}

// PUT /api/games/{id}/metadata/match {source, id, lock}: this is the right game. With
// no source, only changes whether the match is locked.
func (s *Server) handleGameMatch(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	var b struct {
		Source string `json:"source"`
		ID     string `json:"id"`
		Lock   *bool  `json:"lock"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	lock := b.Lock == nil || *b.Lock
	var err error
	if b.Source == "" {
		if b.Lock == nil {
			writeErr(w, 400, "say which game this is, or whether to lock the match")
			return
		}
		err = s.GameMeta.SetLocked(ctx, it, lock)
	} else {
		err = s.GameMeta.Choose(ctx, it, b.Source, b.ID, lock)
	}
	if err != nil {
		writeErr(w, sourceStatus(err), sourceMessage(err))
		return
	}
	d, err := s.Store.GameDetails(ctx, it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, d)
}

// DELETE /api/games/{id}/metadata/match: this match is wrong, and there's no right one.
func (s *Server) handleGameUnmatch(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	if err := s.GameMeta.ClearMatch(r.Context(), it); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/games/{id}/metadata/refresh: ask the sources about this game again.
func (s *Server) handleGameRefresh(w http.ResponseWriter, r *http.Request) {
	it := s.visibleGame(w, r)
	if it == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.GameMeta.Refresh(ctx, it); err != nil {
		writeErr(w, sourceStatus(err), sourceMessage(err))
		return
	}
	writeJSON(w, 202, map[string]any{"started": true})
}

// sourceStatus and sourceMessage turn what a source said into an answer. The errors
// here never hold addresses or keys: gamemeta.Client strips them.
func sourceStatus(err error) int {
	switch {
	case errors.Is(err, library.ErrNotFound), errors.Is(err, gamemeta.ErrNotFound):
		return 404
	case errors.Is(err, gamemeta.ErrKey), errors.Is(err, gamemeta.ErrNotSet):
		return 409
	}
	return 502
}

func sourceMessage(err error) string {
	switch {
	case errors.Is(err, library.ErrNotFound), errors.Is(err, gamemeta.ErrNotFound):
		return "That source doesn't know this game."
	case errors.Is(err, gamemeta.ErrKey):
		return "The source didn't accept its key. Check it in Settings, Metadata agents."
	case errors.Is(err, gamemeta.ErrNotSet):
		return "That source needs a key. Add one in Settings, Metadata agents."
	}
	return err.Error()
}

// GET /api/gamemeta/status
func (s *Server) handleGameMetaStatus(w http.ResponseWriter, r *http.Request) {
	st := s.GameMeta.Status(r.Context())
	for _, k := range library.GameSourceKeys {
		if k != "ssDevId" && k != "ssDevPassword" && s.DB.GetSetting(r.Context(), k, "") != "" {
			st.KeysSet = true
		}
	}
	writeJSON(w, 200, st)
}

// POST /api/gamemeta/refetch: forget what sources said about every game and ask again.
func (s *Server) handleGameMetaRefetch(w http.ResponseWriter, r *http.Request) {
	if err := s.GameMeta.RefetchAll(r.Context()); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 202, map[string]any{"started": true})
}

// GET /api/gamemeta/sources
func (s *Server) handleGameSources(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"sources": s.GameMeta.SourceViews(r.Context())})
}

// PUT /api/gamemeta/sources {sources: [{id, on}]}: their order is the order they're believed in.
func (s *Server) handlePutGameSources(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Sources []library.GameSourceSetting `json:"sources"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.GameMeta.SetSourceOrder(r.Context(), b.Sources); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"sources": s.GameMeta.SourceViews(r.Context())})
}

// POST /api/gamemeta/sources/{id}/test {fields: {setting: value}}: does this work? A
// field left out, or sent back masked, uses what's saved. Nothing is saved here.
func (s *Server) handleGameSourceTest(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Fields map[string]string `json:"fields"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	msg, err := s.GameMeta.TestSource(ctx, r.PathValue("id"), b.Fields)
	if errors.Is(err, library.ErrNotFound) {
		writeErr(w, 404, "no such source")
		return
	}
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "message": msg})
}
