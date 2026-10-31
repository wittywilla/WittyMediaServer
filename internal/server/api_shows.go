package server

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// tvSource is the active TV metadata service (TheTVDB with a key, else TVmaze).
func (s *Server) tvSource(r *http.Request) library.SeriesSource {
	ctx := r.Context()
	return library.TVSource(s.DB.GetSetting(ctx, "tvdbKey", ""), s.DB.GetSetting(ctx, "tvdbPin", ""), s.DB.GetSetting(ctx, "tvmaze", "1") == "1")
}

// animeQuery sets how a library lists anime: an anime library also shows
// the anime of the TV libraries; TV libraries leave anime out when the admin
// sends anime to the anime library only (and one exists).
func (s *Server) animeQuery(ctx context.Context, q *library.ShowQuery) {
	if q.LibraryID == 0 {
		return
	}
	lib, err := s.Store.Library(ctx, q.LibraryID)
	if err != nil {
		return
	}
	switch lib.Type {
	case "anime":
		q.AnimeOf = true
	case "shows":
		if s.DB.GetSetting(ctx, "animeIn", "both") == "anime" {
			libs, _ := s.Store.Libraries(ctx)
			for _, l := range libs {
				if l.Type == "anime" {
					q.HideAnime = true
					break
				}
			}
		}
	}
}

func (s *Server) handleShows(w http.ResponseWriter, r *http.Request) {
	q := library.ShowQuery{
		LibraryID: int64(qInt(r, "library", 0)),
		Search:    r.URL.Query().Get("search"),
		Genre:     r.URL.Query().Get("genre"),
		Network:   r.URL.Query().Get("network"),
		Sort:      r.URL.Query().Get("sort"),
		Desc:      qBool(r, "desc"),
		Limit:     qInt(r, "limit", 0),
		UserID:    userFrom(r).ID,
		Access:    s.access(r),
	}
	s.animeQuery(r.Context(), &q)
	shows, err := s.Store.Shows(r.Context(), q)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"shows": s.showsForViewer(r, shows...), "total": len(shows)})
}

// handleNetworks lists TV networks with their shows and genres:
// GET /api/shows/networks?library=N
func (s *Server) handleNetworks(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.Networks(r.Context(), int64(qInt(r, "library", 0)), s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, n)
}

func (s *Server) handleShowGenres(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.ShowGenres(r.Context(), int64(qInt(r, "library", 0)), s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, g)
}

func (s *Server) handleShow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	ctx, uid := r.Context(), userFrom(r).ID
	sh, err := s.visibleShow(r, id)
	if err != nil {
		writeErr(w, 404, "show not found")
		return
	}
	seasons, err := s.Store.Seasons(ctx, id, uid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	eps, err := s.Store.Episodes(ctx, id, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.Store.AttachProgress(ctx, uid, eps)
	var next *int64
	if n := library.NextUp(eps); n != nil {
		next = &n.ID
	}
	s.showsForViewer(r, sh)
	if !userFrom(r).IsAdmin {
		for i := range seasons {
			seasons[i].Poster = artForViewer(seasons[i].Poster)
		}
	}
	writeJSON(w, 200, map[string]any{"show": sh, "seasons": seasons, "episodes": s.forViewer(r, eps...), "nextUpId": next})
}

func (s *Server) handleUpdateShow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	ctx := r.Context()
	var b struct {
		Title    *string   `json:"title"`
		Year     *int      `json:"year"`
		Overview *string   `json:"overview"`
		Genres   *[]string `json:"genres"`
		Poster   *string   `json:"poster"`
		Backdrop *string   `json:"backdrop"`
		TVDBID   string    `json:"tvdbId"`
		// ContentRating applies to every episode without its own rating.
		ContentRating *string `json:"contentRating"`
		Network       *string `json:"network"`
		Refresh       bool    `json:"refresh"`
		// Intro range in seconds; a negative end clears it.
		IntroStart *float64 `json:"introStart"`
		IntroEnd   *float64 `json:"introEnd"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Refresh || b.TVDBID != "" {
		if _, err := s.Scanner.RefreshShow(ctx, id, strings.TrimSpace(b.TVDBID)); err != nil {
			writeErr(w, 502, err.Error())
			return
		}
	}
	sh, err := s.Store.Show(ctx, id, userFrom(r).ID)
	if err != nil {
		writeErr(w, 404, "show not found")
		return
	}
	if b.Title != nil && strings.TrimSpace(*b.Title) != "" {
		sh.Title = strings.TrimSpace(*b.Title)
		sh.SortTitle = library.SortTitle(sh.Title)
	}
	if b.Year != nil {
		sh.Year = b.Year
	}
	if b.Network != nil {
		sh.Network = strings.TrimSpace(*b.Network)
	}
	if b.ContentRating != nil {
		sh.ContentRating = strings.TrimSpace(*b.ContentRating)
	}
	if b.Overview != nil {
		sh.Overview = *b.Overview
	}
	if b.Genres != nil {
		sh.Genres = *b.Genres
	}
	if b.Poster != nil {
		sh.Poster = *b.Poster
	}
	if b.Backdrop != nil {
		sh.Backdrop = *b.Backdrop
	}
	if b.IntroEnd != nil {
		if *b.IntroEnd <= 0 {
			sh.IntroStart, sh.IntroEnd = nil, nil
		} else {
			start := 0.0
			if b.IntroStart != nil && *b.IntroStart >= 0 && *b.IntroStart < *b.IntroEnd {
				start = *b.IntroStart
			}
			sh.IntroStart, sh.IntroEnd = &start, b.IntroEnd
		}
	}
	metaEdit := b.Title != nil || b.Year != nil || b.Overview != nil || b.Genres != nil || b.Poster != nil || b.Backdrop != nil || b.Refresh || b.TVDBID != ""
	if metaEdit && s.saveMeta(r) {
		if err := s.Scanner.SaveShowMetadata(ctx, sh); err != nil {
			s.Log.WarnContext(ctx, "could not save metadata beside the media", "cat", "library", "show", sh.Title, "err", err)
		}
	}
	if err := s.Store.UpsertShow(ctx, sh); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, sh)
}

func (s *Server) handleShowArt(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	sh, err := s.Store.ShowFacts(r.Context(), id)
	if err != nil || !s.access(r).AllowsShow(sh) {
		writeErr(w, 404, "show not found")
		return
	}
	src := sh.Poster
	switch r.PathValue("kind") {
	case "backdrop":
		if sh.Backdrop != "" {
			src = sh.Backdrop
		}
	case "season":
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if p := s.Store.SeasonPoster(r.Context(), id, n); p != "" {
			src = p
		} else if r.URL.Query().Get("strict") == "1" {
			src = ""
		}
	}
	if src == "" {
		// No artwork yet (no TheTVDB key, no local files): use an episode still.
		src = s.Store.EpisodePoster(r.Context(), id)
	}
	s.serveArtSrc(w, r, src)
}

// artCandidate is one choice in the artwork picker.
type artCandidate struct {
	URL    string `json:"url"`    // value to store
	Source string `json:"source"` // local | tmdb | tvdb | current
}

// localImages lists image files in a folder.
func localImages(dir string) []artCandidate {
	var out []artCandidate
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || (ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp") {
			continue
		}
		out = append(out, artCandidate{URL: filepath.Join(dir, e.Name()), Source: "local"})
	}
	return out
}

// handleArtwork lists poster/backdrop candidates for an item or a show:
// GET /api/items/{id}/artwork?kind=poster|backdrop, /api/shows/{id}/artwork?kind=...
func (s *Server) handleArtwork(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	ctx := r.Context()
	backdrop := r.URL.Query().Get("kind") == "backdrop"
	var out []artCandidate
	seen := map[string]bool{}
	add := func(c ...artCandidate) {
		for _, x := range c {
			if x.URL != "" && !seen[x.URL] {
				seen[x.URL] = true
				out = append(out, x)
			}
		}
	}
	var warn string
	if strings.HasPrefix(r.URL.Path, "/api/shows/") {
		sh, err := s.Store.Show(ctx, id, 0)
		if err != nil {
			writeErr(w, 404, "show not found")
			return
		}
		cur := sh.Poster
		if backdrop {
			cur = sh.Backdrop
		}
		add(artCandidate{URL: cur, Source: "current"})
		add(localImages(sh.Path)...)
		if t := s.tvSource(r); t != nil {
			var meta *library.SeriesMeta
			var err error
			if sh.Provider == t.Name() && sh.ProviderID != "" {
				meta, err = t.Series(ctx, sh.ProviderID)
			} else {
				meta, err = t.FindSeries(ctx, sh.Title, sh.Year)
			}
			if err != nil {
				warn = err.Error()
			} else {
				list := meta.Posters
				if backdrop {
					list = meta.Backdrops
				}
				for _, u := range list {
					add(artCandidate{URL: u, Source: t.Name()})
				}
			}
		} else {
			warn = "Online TV artwork is off. Add a TheTVDB key or turn on TVmaze in Settings."
		}
	} else {
		it, err := s.Store.Item(ctx, id)
		if err != nil {
			writeErr(w, 404, "item not found")
			return
		}
		cur := it.Poster
		if backdrop {
			cur = it.Backdrop
		}
		add(artCandidate{URL: cur, Source: "current"})
		dir := filepath.Dir(it.Path)
		if it.Kind == "dvd-folder" {
			dir = it.Path
		}
		add(localImages(dir)...)
		if key := s.DB.GetSetting(ctx, "tmdbKey", ""); key != "" && it.ShowID == nil {
			t := &library.TMDB{Key: key}
			posters, backdrops, err := t.Images(ctx, it)
			if err != nil {
				warn = err.Error()
			}
			list := posters
			if backdrop {
				list = backdrops
			}
			for i, u := range list {
				if i >= 40 {
					break
				}
				add(artCandidate{URL: u, Source: "tmdb"})
			}
		} else if it.ShowID == nil {
			warn = "Add a TMDB API key in Settings for online artwork."
		}
	}
	if out == nil {
		out = []artCandidate{}
	}
	writeJSON(w, 200, map[string]any{"candidates": out, "warning": warn})
}

// handleArtPreview serves a candidate image (local path or remote URL) for
// the picker. Local paths must be images inside a library folder.
func (s *Server) handleArtPreview(w http.ResponseWriter, r *http.Request) {
	src := r.URL.Query().Get("src")
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "cache:") {
		s.serveArtSrc(w, r, src)
		return
	}
	ext := strings.ToLower(filepath.Ext(src))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".tbn" {
		writeErr(w, 400, "not an image")
		return
	}
	if !s.inLibraries(r.Context(), src) {
		writeErr(w, 403, "outside the libraries")
		return
	}
	s.serveArtSrc(w, r, filepath.Clean(src))
}

// handleShowView sets one show's episode layout (cards | list | "" = default).
func (s *Server) handleShowView(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		View string `json:"view"`
	}
	if err := readJSON(r, &b); err != nil || (b.View != "" && b.View != "cards" && b.View != "list") {
		writeErr(w, 400, "view must be cards, list or empty")
		return
	}
	if err := s.Store.SetShowView(r.Context(), id, b.View); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"view": b.View})
}

// handlePrefs returns the server-wide playback defaults every viewer needs.
func (s *Server) handlePrefs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, 200, map[string]any{
		"dvdStart":    s.DB.GetSetting(ctx, "dvdStart", "menu"),
		"insertFx":    s.DB.GetSetting(ctx, "insertFx", "1") == "1",
		"insertSound": s.DB.GetSetting(ctx, "insertSound", "1") == "1",
		"episodeView": s.DB.GetSetting(ctx, "episodeView", "cards"),
		"tvdb":        s.DB.GetSetting(ctx, "tvdbKey", "") != "",
		"tmdb":        s.DB.GetSetting(ctx, "tmdbKey", "") != "",
		"maxSpeed":    maxSpeed(s.DB.GetSetting(ctx, "maxSpeed", "4")),
		"clientLogs":  s.Logs != nil && s.Logs.Accepts("clients", slog.LevelWarn),
		"shareLinks":  s.DB.GetSetting(ctx, "shareLinks", "1") == "1",
		"uploads":     s.DB.GetSetting(ctx, "uploads", "1") == "1",
	})
}

// maxSpeed is the fastest playback speed viewers may pick: 4x unless an
// admin raised it (browsers top out at 16x).
func maxSpeed(v string) float64 {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 1 {
		return 4
	}
	return math.Min(f, 16)
}

func (s *Server) saveMeta(r *http.Request) bool {
	return s.DB.GetSetting(r.Context(), "saveMetadata", "1") == "1"
}

// DVD subtitles (client-side rendering).

const maxSubtitleWindow = 600 // seconds

func (s *Server) handleDVDSubs(w http.ResponseWriter, r *http.Request) {
	sess := s.dvdSession(w, r)
	if sess == nil {
		return
	}
	q := r.URL.Query()
	from, err := strconv.ParseFloat(q.Get("from"), 64)
	if err != nil || math.IsNaN(from) || math.IsInf(from, 0) {
		from = 0
	}
	to, err := strconv.ParseFloat(q.Get("to"), 64)
	if err != nil || math.IsNaN(to) || to <= from {
		to = from + 120
	}
	// Subtitles are read off the disc: a window the size of a whole film would
	// keep a drive busy for each request.
	to = math.Min(to, from+maxSubtitleWindow)
	evs, err := sess.Subtitles(qInt(r, "ttn", 0), qInt(r, "stream", 0), from, to)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"events": evs, "from": from, "to": to})
}

func (s *Server) handleDVDSub(w http.ResponseWriter, r *http.Request) {
	sess := s.dvdSession(w, r)
	if sess == nil {
		return
	}
	png, ok := sess.SubPNG(r.PathValue("name"))
	if !ok {
		writeErr(w, 404, "no subtitle image")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(png)
}
