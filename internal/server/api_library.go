package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func (s *Server) handleLibraries(w http.ResponseWriter, r *http.Request) {
	libs, err := s.visibleLibraries(r.Context(), s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, libs)
}

type libraryBody struct {
	Name  string   `json:"name"`
	Type  string   `json:"type"`
	Paths []string `json:"paths"`
}

func (b *libraryBody) validate() error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return errors.New("name is required")
	}
	switch b.Type {
	case "movies", "discs", "shows", "anime", "videos", "music", "audiobooks", "podcasts", "books", "photos", "shorts", "channels", "musicvideos", "radio":
	case "":
		b.Type = "movies"
	default:
		return errors.New("unknown library type")
	}
	var clean []string
	for _, p := range b.Paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			return errors.New("folder not found: " + p)
		}
		clean = append(clean, filepath.Clean(p))
	}
	// An anime library may have no folders of its own: it then shows the
	// anime found in the TV libraries.
	if len(clean) == 0 && b.Type != "anime" && b.Type != "podcasts" && b.Type != "radio" {
		return errors.New("at least one folder is required")
	}
	b.Paths = clean
	return nil
}

func (s *Server) handleCreateLibrary(w http.ResponseWriter, r *http.Request) {
	var b libraryBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := b.validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	lib, err := s.Store.CreateLibrary(r.Context(), b.Name, b.Type, b.Paths)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Scanner.Start(lib.ID)
	writeJSON(w, 201, lib)
}

func (s *Server) handleUpdateLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b libraryBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := b.validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	lib, err := s.Store.UpdateLibrary(r.Context(), id, b.Name, b.Type, b.Paths)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Scanner.Start(lib.ID) // folders may have been added
	writeJSON(w, 200, lib)
}

func (s *Server) handleDeleteLibrary(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	s.Scanner.Cancel(id)
	if err := s.Store.DeleteLibrary(r.Context(), id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.Store.Library(r.Context(), id); err != nil {
		writeErr(w, 404, "library not found")
		return
	}
	started := s.Scanner.Start(id)
	writeJSON(w, 202, map[string]any{"started": started})
}

func (s *Server) handleScanProgress(w http.ResponseWriter, r *http.Request) {
	id, _ := pathInt(r, "id")
	out := s.Scanner.Snapshot(id)
	writeJSON(w, 200, map[string]any{"scanning": out != nil, "progress": out})
}

func (s *Server) handleGenres(w http.ResponseWriter, r *http.Request) {
	g, err := s.Store.Genres(r.Context(), int64(qInt(r, "library", 0)))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, g)
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	q := library.ItemQuery{
		LibraryID: int64(qInt(r, "library", 0)),
		Search:    r.URL.Query().Get("search"),
		Sort:      r.URL.Query().Get("sort"),
		Desc:      qBool(r, "desc"),
		Limit:     qInt(r, "limit", 0),
		Offset:    qInt(r, "offset", 0),
		Genre:     r.URL.Query().Get("genre"),
		Kind:      r.URL.Query().Get("kind"),
		Access:    s.access(r),
		// noEpisodes=1: movies and discs only (the Shuffle widget)
		NoEpisodes: qBool(r, "noEpisodes"),
		Watchable:  qBool(r, "watchable"),
		// discContent=movie|tv: discs holding films or TV episodes
		DiscContent: r.URL.Query().Get("discContent"),
	}
	items, total, err := s.Store.Items(r.Context(), q)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.Store.AttachProgress(r.Context(), userFrom(r).ID, items)
	writeJSON(w, 200, map[string]any{"items": items, "total": total})
}

func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "item not found")
		return
	}
	_ = s.Store.AttachProgress(r.Context(), userFrom(r).ID, []*library.Item{it})
	it.NextID = s.Store.NextEpisode(r.Context(), it)
	if next := s.nextForCollection(r, it); next != nil {
		it.NextID = next // playing a list through
	}
	if it.IntroEnd == nil && it.ShowID != nil {
		if sh, err := s.Store.Show(r.Context(), *it.ShowID, 0); err == nil {
			it.IntroStart, it.IntroEnd = sh.IntroStart, sh.IntroEnd
		}
	}
	writeJSON(w, 200, it)
}

// handleRecommend suggests what to watch next once a season or a movie ends.
func (s *Server) handleRecommend(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "item not found")
		return
	}
	rec, show, err := s.Store.Recommend(r.Context(), userFrom(r).ID, it, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"item": rec, "show": show})
}

func (s *Server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.Store.Item(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "item not found")
		return
	}
	var b struct {
		Title    *string `json:"title"`
		Year     *int    `json:"year"`
		Overview *string `json:"overview"`
		Tagline  *string `json:"tagline"`
		// ContentRating, e.g. "PG-13" or "TV-Y7"; sets what Kids profiles see.
		ContentRating *string   `json:"contentRating"`
		Genres        *[]string `json:"genres"`
		Poster        *string   `json:"poster"`
		Backdrop      *string   `json:"backdrop"`
		Refresh       bool      `json:"refresh"`
		PlayMode      *string   `json:"playMode"` // DVDs: menu | title | ""
		InsertFX      *string   `json:"insertFx"` // DVDs: on | off | ""
		// DiscContent: what a disc holds, "movie" or "tv"; "" works it out.
		DiscContent *string `json:"discContent"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Title != nil && strings.TrimSpace(*b.Title) != "" {
		it.Title = strings.TrimSpace(*b.Title)
		it.SortTitle = library.SortTitle(it.Title)
	}
	if b.Year != nil {
		it.Year = b.Year
	}
	if b.Overview != nil {
		it.Overview = *b.Overview
	}
	if b.Tagline != nil {
		it.Tagline = *b.Tagline
	}
	if b.ContentRating != nil {
		it.ContentRating = strings.TrimSpace(*b.ContentRating)
	}
	if b.Genres != nil {
		it.Genres = *b.Genres
	}
	if b.Poster != nil {
		it.Poster = *b.Poster
	}
	if b.Backdrop != nil {
		it.Backdrop = *b.Backdrop
	}
	if b.Refresh {
		// The owner's own agents first; they fill only what is empty.
		if lib, err := s.Store.Library(r.Context(), it.LibraryID); err == nil && it.ShowID == nil {
			if strings.HasPrefix(it.Provider, "agent:") {
				it.Provider, it.ProviderID, it.Overview, it.Genres = "", "", "", nil
			}
			s.Scanner.ApplyAgents(r.Context(), it, lib.Type)
		}
		if strings.HasPrefix(it.Provider, "agent:") {
			// An agent answered: it takes the place of the built-in ones.
		} else if key := s.DB.GetSetting(r.Context(), "tmdbKey", ""); key != "" {
			t := &library.TMDB{Key: key}
			it.Provider, it.ProviderID = "", ""
			it.Overview, it.Poster, it.Backdrop = "", "", ""
			it.Genres = nil
			if err := t.Enrich(r.Context(), it); err != nil {
				writeErr(w, 502, "TMDB: "+err.Error())
				return
			}
		} else if it.ShowID == nil && s.DB.GetSetting(r.Context(), "wikidata", "1") == "1" {
			// No TMDB key: facts and the rating from Wikidata (posters stay).
			if it.Provider == "wikidata" || it.Provider == "" {
				it.Provider, it.ProviderID, it.Overview, it.Genres, it.ContentRating = "", "", "", nil, ""
			}
			if err := (&library.Wikidata{}).Enrich(r.Context(), it); err != nil {
				writeErr(w, 502, "No match on Wikidata: "+err.Error())
				return
			}
		}
	}
	if b.PlayMode != nil || b.InsertFX != nil {
		pm, fx := it.PlayMode, it.InsertFX
		if b.PlayMode != nil && (*b.PlayMode == "" || *b.PlayMode == "menu" || *b.PlayMode == "title") {
			pm = *b.PlayMode
		}
		if b.InsertFX != nil && (*b.InsertFX == "" || *b.InsertFX == "on" || *b.InsertFX == "off") {
			fx = *b.InsertFX
		}
		if err := s.Store.SetItemPrefs(r.Context(), it.ID, pm, fx); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		it.PlayMode, it.InsertFX = pm, fx
	}
	if b.DiscContent != nil && it.Disc != nil {
		switch v := *b.DiscContent; v {
		case "movie", "tv":
			it.Disc.Content, it.Disc.ContentManual = v, true
		case "":
			lib, _ := s.Store.Library(r.Context(), it.LibraryID)
			typ := ""
			if lib != nil {
				typ = lib.Type
			}
			it.Disc.Content, it.Disc.ContentManual = library.GuessDiscContent(typ, it.Title, it.Disc.Label), false
		}
	}
	metaEdit := b.Title != nil || b.Year != nil || b.Overview != nil || b.Tagline != nil || b.Genres != nil || b.Poster != nil || b.Backdrop != nil || b.Refresh || b.ContentRating != nil
	gallery := false
	if lib, err := s.Store.Library(r.Context(), it.LibraryID); err == nil {
		gallery = library.IsGalleryLibrary(lib.Type) // their edits stay in the database
	}
	if metaEdit && !gallery && s.saveMeta(r) {
		if err := s.Scanner.SaveItemMetadata(r.Context(), it); err != nil {
			s.Log.Warn("save metadata", "path", it.Path, "err", err)
		}
	}
	if err := s.Store.UpdateItem(r.Context(), it); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, it)
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(r)
	acc := s.access(r)
	cont, _ := s.Store.ContinueWatching(ctx, u.ID, 20)
	cont = s.filterItems(ctx, acc, cont)
	libs, err := s.visibleLibraries(ctx, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	type hub struct {
		Key     string          `json:"key"`
		Title   string          `json:"title"`
		Library *int64          `json:"libraryId,omitempty"`
		Items   []*library.Item `json:"items"`
		Shows   []*library.Show `json:"shows,omitempty"`
	}
	hubs := []hub{}
	if len(cont) > 0 {
		// One row per kind of media, each worded for what you do with it.
		var watch, read, listen []*library.Item
		for _, it := range cont {
			switch it.Kind {
			case "book":
				read = append(read, it)
			case "audio":
				listen = append(listen, it)
			default:
				watch = append(watch, it)
			}
		}
		for _, h := range []struct {
			key, title string
			items      []*library.Item
		}{{"continue", "Continue watching", watch}, {"continue-reading", "Continue reading", read}, {"continue-listening", "Continue listening", listen}} {
			if len(h.items) > 0 {
				hubs = append(hubs, hub{Key: h.key, Title: h.title, Items: h.items})
			}
		}
	}
	if next := s.nextUp(r, libs); len(next) > 0 {
		hubs = append(hubs, hub{Key: "nextup", Title: "Next up", Items: next})
	}
	for _, l := range libs {
		if l.Type == "shows" || l.Type == "anime" {
			sq := library.ShowQuery{LibraryID: l.ID, Sort: "added", Desc: true, Limit: 20, UserID: u.ID, Access: acc}
			s.animeQuery(ctx, &sq)
			shows, err := s.Store.Shows(ctx, sq)
			if err != nil || len(shows) == 0 {
				continue
			}
			id := l.ID
			hubs = append(hubs, hub{Key: "recent-" + itoa(l.ID), Title: "Recently added in " + l.Name, Library: &id, Items: []*library.Item{}, Shows: shows})
			continue
		}
		items, _, err := s.Store.Items(ctx, library.ItemQuery{LibraryID: l.ID, Sort: "added", Desc: true, Limit: 20, Access: acc})
		if err != nil || len(items) == 0 {
			continue
		}
		_ = s.Store.AttachProgress(ctx, u.ID, items)
		id := l.ID
		hubs = append(hubs, hub{Key: "recent-" + itoa(l.ID), Title: "Recently added in " + l.Name, Library: &id, Items: items})
	}
	// A "featured" pick for the hero: a random highly rated / recent item.
	var featured []*library.Item
	all, _, _ := s.Store.Items(ctx, library.ItemQuery{Sort: "added", Desc: true, Limit: 40, NoEpisodes: true, NoGallery: true, Access: acc})
	for _, it := range all {
		if it.Backdrop != "" {
			featured = append(featured, it)
		}
		if len(featured) >= 8 {
			break
		}
	}
	if featured == nil {
		featured = all
		if len(featured) > 8 {
			featured = featured[:8]
		}
	}
	_ = s.Store.AttachProgress(ctx, u.ID, featured)
	writeJSON(w, 200, map[string]any{"hubs": hubs, "featured": featured, "libraries": libs})
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// nextUp offers the next episode of each show the user has started.
func (s *Server) nextUp(r *http.Request, libs []library.Library) []*library.Item {
	ctx, uid, acc := r.Context(), userFrom(r).ID, s.access(r)
	var out []*library.Item
	seen := map[int64]bool{} // anime can be listed by a TV and an anime library
	for _, l := range libs {
		if l.Type != "shows" && l.Type != "anime" {
			continue
		}
		shows, err := s.Store.Shows(ctx, library.ShowQuery{LibraryID: l.ID, UserID: uid, Access: acc})
		if err != nil {
			continue
		}
		for _, sh := range shows {
			if seen[sh.ID] {
				continue
			}
			seen[sh.ID] = true
			if sh.Unwatched == 0 || sh.Unwatched == sh.EpisodeCount {
				continue // finished, or never started
			}
			eps, err := s.Store.Episodes(ctx, sh.ID, acc)
			if err != nil {
				continue
			}
			_ = s.Store.AttachProgress(ctx, uid, eps)
			if n := library.NextUp(eps); n != nil && (n.Progress == nil || n.Progress.PositionSec <= 60) {
				out = append(out, n)
			}
			if len(out) >= 20 {
				return out
			}
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Artwork
// ---------------------------------------------------------------------------

func (s *Server) handlePoster(w http.ResponseWriter, r *http.Request)   { s.serveArt(w, r, false) }
func (s *Server) handleBackdrop(w http.ResponseWriter, r *http.Request) { s.serveArt(w, r, true) }

func (s *Server) serveArt(w http.ResponseWriter, r *http.Request, backdrop bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "item not found")
		return
	}
	src := it.Poster
	if backdrop {
		src = it.Backdrop
		if src == "" {
			src = it.Poster
		}
	}
	if src == library.LazyPoster {
		width := qInt(r, "w", 480)
		if backdrop {
			width = 1920
		}
		s.serveThumb(w, r, it, width)
		return
	}
	s.serveArtSrc(w, r, src)
}

// serveArtSrc serves artwork from a cache name, a remote URL (cached) or a
// path. Artwork references come from places the owner doesn't control
// (NFO files, podcast feeds, online catalogues), so only pictures are ever
// served: never another kind of file from this machine, never an SVG or a
// page that could run scripts on this site.
func (s *Server) serveArtSrc(w http.ResponseWriter, r *http.Request, src string) {
	if src == "" {
		writeErr(w, 404, "no artwork")
		return
	}
	var p string
	switch {
	case strings.HasPrefix(src, "cache:"):
		name := strings.TrimPrefix(src, "cache:")
		if name == "" || name != filepath.Base(name) {
			writeErr(w, 404, "no artwork")
			return
		}
		p = filepath.Join(s.Cfg.ImagesDir, name)
	case strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://"):
		var err error
		if p, err = s.cachedRemote(r.Context(), src); err != nil {
			writeErr(w, 502, err.Error())
			return
		}
	default:
		// Pictures next to the media (poster.jpg, an NFO's thumb): inside a
		// library folder only.
		if !s.inLibraries(r.Context(), src) {
			writeErr(w, 404, "no artwork")
			return
		}
		p = filepath.Clean(src)
	}
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, 404, "no artwork")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	ct := imageType(f, p)
	if err != nil || st.IsDir() || ct == "" {
		writeErr(w, 404, "no artwork")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// inLibraries: the path is inside one of the libraries' folders.
func (s *Server) inLibraries(ctx context.Context, path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return false
	}
	clean := filepath.Clean(path)
	for _, l := range libs {
		for _, p := range l.Paths {
			if rel, err := filepath.Rel(p, clean); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
				return true
			}
		}
	}
	return false
}

// imageType is the type of a raster picture, from its first bytes (AVIF by
// its extension), or "" for anything else.
func imageType(f io.ReadSeeker, name string) string {
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	switch ct := http.DetectContentType(head[:n]); ct {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp":
		return ct
	}
	if strings.EqualFold(filepath.Ext(name), ".avif") && n > 12 && string(head[4:8]) == "ftyp" {
		return "image/avif"
	}
	return ""
}

// cachedRemote downloads a remote image once into the cache, from the
// internet only (a feed or catalogue must not reach devices on this network).
func (s *Server) cachedRemote(ctx context.Context, u string) (string, error) {
	h := sha1.Sum([]byte(u))
	p := filepath.Join(s.Cfg.ImagesDir, "r-"+hex.EncodeToString(h[:10])+filepath.Ext(strings.Split(u, "?")[0]))
	if st, err := os.Stat(p); err == nil && st.Size() > 0 {
		return p, nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	res, err := library.SafeClient.Do(req)
	if err != nil {
		return "", errors.New("artwork fetch failed")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New("artwork fetch failed: " + res.Status)
	}
	tmp := p + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, io.LimitReader(res.Body, 20<<20)); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()
	return p, os.Rename(tmp, p)
}

// ---------------------------------------------------------------------------
// Progress
// ---------------------------------------------------------------------------

func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var p library.Progress
	if err := readJSON(r, &p); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	s.logWatched(r, id, p)
	if err := s.Store.SetProgress(r.Context(), userFrom(r).ID, id, p); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleClearProgress(w http.ResponseWriter, r *http.Request) {
	id, _ := pathInt(r, "id")
	_ = s.Store.ClearProgress(r.Context(), userFrom(r).ID, id)
	writeJSON(w, 200, map[string]any{"ok": true})
}
