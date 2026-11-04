package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// WittyPhotos: the gallery's JSON API and its pictures. See docs/PHOTOS.md.

// libraryOfType loads a library of one of the given types that the account may open.
func (s *Server) libraryOfType(r *http.Request, id int64, types ...string) (*library.Library, error) {
	lib, err := s.Store.Library(r.Context(), id)
	if err != nil {
		return nil, library.ErrNotFound
	}
	if a := s.access(r); a != nil && len(a.Libraries) > 0 {
		allowed := false
		for _, x := range a.Libraries {
			allowed = allowed || x == id
		}
		if !allowed {
			return nil, library.ErrNotFound
		}
	}
	for _, t := range types {
		if lib.Type == t {
			return lib, nil
		}
	}
	return nil, library.ErrNotFound
}

// canPost: comments, uploads and links are for grown-up accounts. Kids can like
// what they may see; a Guest, being anyone, can only look.
func canPost(u *auth.User) bool {
	return u != nil && (u.IsAdmin || u.Kind == auth.KindAdult || u.Kind == "")
}
func canReact(u *auth.User) bool { return u != nil && (u.IsAdmin || u.Kind != auth.KindGuest) }

// What one account can keep. Far more than a person writes, and each row is a place to store
// things on someone else's disk. An administrator has no limit.
var (
	maxCommentsPerAccount    = 5000
	maxCollectionsPerAccount = 200
	maxAlbumsPerAccount      = 200
)

// underLimit answers 409 when an account already has as many of something as it may.
func (s *Server) underLimit(w http.ResponseWriter, r *http.Request, u *auth.User, count string, limit int, what string) bool {
	if u.IsAdmin {
		return true
	}
	var n int
	if err := s.DB.QueryRowContext(r.Context(), count, u.ID).Scan(&n); err != nil {
		writeErr(w, 500, "could not check that")
		return false
	}
	if n >= limit {
		writeErr(w, 409, fmt.Sprintf("You have %d %s, which is as many as one account can have. Remove some first.", n, what))
		return false
	}
	return true
}

func (s *Server) photoQuery(r *http.Request) (library.PhotoQuery, error) {
	q := r.URL.Query()
	pq := library.PhotoQuery{LibraryID: int64(qInt(r, "library", 0)), UserID: userFrom(r).ID, Access: s.access(r), YM: qInt(r, "ym", 0),
		Favorites: qBool(r, "favorites"), Search: strings.TrimSpace(q.Get("search")), Camera: q.Get("camera"), Limit: qInt(r, "limit", 0), Cursor: q.Get("cursor")}
	if _, err := s.libraryOfType(r, pq.LibraryID, "photos"); err != nil {
		return pq, err
	}
	if k := q.Get("kind"); k == "photo" || k == "video" {
		pq.Kind = k
	}
	if q.Has("folder") {
		f := q.Get("folder")
		pq.Folder = &f
	}
	if v := q.Get("album"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return pq, library.ErrNotFound
		}
		a, err := s.Store.PhotoAlbumByID(r.Context(), id, userFrom(r).ID, userFrom(r).IsAdmin)
		if err != nil || a.LibraryID != pq.LibraryID {
			return pq, library.ErrNotFound
		}
		pq.Album = id
	}
	if v := q.Get("bbox"); v != "" {
		b, err := parseBBox(v)
		if err != nil {
			return pq, err
		}
		pq.BBox = b
	}
	return pq, nil
}

// parseBBox reads "south,west,north,east" in degrees. West greater than east
// is a box across the date line. A box at least the world wide is no limit on
// longitude, which a map zoomed right out asks for.
func parseBBox(v string) (*[4]float64, error) {
	bad := library.ErrBadInput("bbox is south,west,north,east in degrees")
	parts := strings.Split(v, ",")
	if len(parts) != 4 {
		return nil, bad
	}
	var b [4]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, bad
		}
		b[i] = f
	}
	if b[0] < -90 || b[2] > 90 || b[0] > b[2] || b[1] < -180 || b[1] > 180 || b[3] < -180 || b[3] > 180 {
		return nil, bad
	}
	return &b, nil
}

func writeLibErr(w http.ResponseWriter, err error) {
	var bad library.ErrBadInput
	switch {
	case errors.Is(err, library.ErrNotFound):
		writeErr(w, 404, "not found")
	case errors.As(err, &bad):
		writeErr(w, 400, bad.Error())
	default:
		writeErr(w, 500, err.Error())
	}
}

// GET /api/photos/months?library=&favorites=&album=...
func (s *Server) handlePhotoMonths(w http.ResponseWriter, r *http.Request) {
	pq, err := s.photoQuery(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	m, err := s.Store.PhotoMonths(r.Context(), pq)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, m)
}

// GET /api/photos/list?library=&ym=&album=&folder=&favorites=&kind=&search=&bbox=&limit=&cursor=
func (s *Server) handlePhotoList(w http.ResponseWriter, r *http.Request) {
	pq, err := s.photoQuery(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	items, next, err := s.Store.Photos(r.Context(), pq)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"photos": items, "nextCursor": next})
}

// GET /api/photos/folders?library=
func (s *Server) handlePhotoFolders(w http.ResponseWriter, r *http.Request) {
	pq, err := s.photoQuery(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	f, err := s.Store.PhotoFolders(r.Context(), pq)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, f)
}

// GET /api/photos/map?library=&zoom=&bbox=&fit=1 (and the list's filters): groups
// of pictures for the part of the map in view. fit=1 adds the box around every
// located picture, for the first view; without a zoom it's only that box.
func (s *Server) handlePhotoMap(w http.ResponseWriter, r *http.Request) {
	pq, err := s.photoQuery(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	c, cut := []library.PhotoCluster{}, false
	if !qBool(r, "fit") || r.URL.Query().Has("zoom") { // fit alone is the first view's question: just the box
		if c, cut, err = s.Store.PhotoClusters(r.Context(), pq, qInt(r, "zoom", 0)); err != nil {
			writeLibErr(w, err)
			return
		}
	}
	tiles, credit := s.onlineMap(r.Context())
	out := map[string]any{"clusters": c, "truncated": cut, "tiles": tiles, "attribution": credit}
	if qBool(r, "fit") {
		b, err := s.Store.PhotoBounds(r.Context(), pq)
		if err != nil {
			writeLibErr(w, err)
			return
		}
		out["bounds"] = b
	}
	writeJSON(w, 200, out)
}

// onlineMap is the online map's address and the credit its provider asks for,
// or nothing while the online map is off. mapOnline unset (servers from before
// the switch) follows whether an address was given, as it did then.
func (s *Server) onlineMap(ctx context.Context) (tiles, credit string) {
	tiles = s.DB.GetSetting(ctx, "mapTiles", "")
	if tiles == "" || s.DB.GetSetting(ctx, "mapOnline", "") == "0" || !validMapTiles(tiles) {
		return "", ""
	}
	return tiles, s.DB.GetSetting(ctx, "mapAttribution", "")
}

// GET /api/photos/ai: the place for the local AI. It doesn't exist yet; when it
// does, people and places will come from it, on this machine and nowhere else.
func (s *Server) handlePhotoAI(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"available": false, "people": []string{}, "places": []string{},
		"note": "Reserved for the planned local AI. Nothing here looks at your photos yet, and nothing is ever sent elsewhere."})
}

// GET /api/photos/{id}
func (s *Server) handlePhotoDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	d, err := s.Store.PhotoDetail(r.Context(), userFrom(r).ID, it)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, d)
}

// GET /api/items/{id}/image?w=: a picture to look at. Without w, a browser-safe
// original goes as it is, and anything else as a large JPEG. With w, a JPEG of
// that size. Native players can rely on JPEG for every w.
func (s *Server) handleItemImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil || (it.Kind != "photo" && it.Kind != "video") {
		writeErr(w, 404, "not found")
		return
	}
	width := qInt(r, "w", 0)
	if it.Kind == "photo" && strings.HasPrefix(mediatypes.Ext(it.Path), ".svg") {
		s.serveSVG(w, r, it)
		return
	}
	if width <= 0 && it.Kind == "photo" && library.WebSafeImage(it.Path) {
		s.serveOriginal(w, r, it, false)
		return
	}
	if width <= 0 {
		width = 2560
	}
	s.serveThumb(w, r, it, width)
}

func (s *Server) serveThumb(w http.ResponseWriter, r *http.Request, it *library.Item, width int) {
	p, err := s.Scanner.Thumb(r.Context(), it, width)
	if errors.Is(err, library.ErrNotFound) {
		writeErr(w, 404, "not found")
		return
	}
	if err != nil {
		if r.Context().Err() == nil {
			s.Log.WarnContext(r.Context(), "could not make a thumbnail", "cat", "library", "item", it.ID, "file", it.Path, "err", err)
		}
		writeErr(w, 415, "this picture could not be read")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cache-Control", "private, max-age=2592000")
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// originalType is the type a photo or clip is served with, by its name only, from the
// registry: a file never gets to say it is a web page.
func originalType(name string) string { return mediatypes.MIME(name) }

// serveSVG sends a picture that can hold a program: sanitised, and sandboxed besides.
// A vector picture needs no smaller copies.
func (s *Server) serveSVG(w http.ResponseWriter, r *http.Request, it *library.Item) {
	b, err := library.SVGFile(it.Path)
	if err != nil {
		writeErr(w, 415, "this picture could not be read")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	h.Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(b)
}

func (s *Server) serveOriginal(w http.ResponseWriter, r *http.Request, it *library.Item, download bool) {
	f, st, err := s.Store.OpenLibraryFile(r.Context(), it.Path)
	if err != nil {
		writeErr(w, 404, "file missing")
		return
	}
	defer f.Close()
	h := w.Header()
	h.Set("Content-Type", originalType(it.Path))
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, max-age=3600")
	if download {
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(it.Path)}))
	}
	http.ServeContent(s.capWriter(w, r), r, "", st.ModTime(), f)
}

// GET /api/items/{id}/original: the file itself, to download.
func (s *Server) handleOriginal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil || (it.Kind != "photo" && it.Kind != "video") {
		writeErr(w, 404, "not found")
		return
	}
	s.serveOriginal(w, r, it, true)
}

// GET /api/items/{id}/motion: the clip of a live photo.
func (s *Server) handleMotion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	p := s.Store.PhotoMotion(r.Context(), it.ID)
	if strings.HasPrefix(p, library.EmbeddedMotion) {
		s.serveEmbeddedMotion(w, r, it, p)
		return
	}
	if p == "" || !s.inLibraries(r.Context(), p) {
		writeErr(w, 404, "no motion")
		return
	}
	clip := *it
	clip.Path = p
	s.serveOriginal(w, r, &clip, false)
}

// serveEmbeddedMotion sends the clip stored after a picture, in the picture's own file.
func (s *Server) serveEmbeddedMotion(w http.ResponseWriter, r *http.Request, it *library.Item, motion string) {
	off, length, ok := library.ParseEmbeddedMotion(motion)
	if !ok || !s.inLibraries(r.Context(), it.Path) {
		writeErr(w, 404, "no motion")
		return
	}
	f, err := os.Open(it.Path)
	if err != nil {
		writeErr(w, 404, "file missing")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || off+length > st.Size() {
		writeErr(w, 404, "no motion")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "video/mp4")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, "", st.ModTime(), io.NewSectionReader(f, off, length))
}

// ---------------------------------------------------------------------------
// Albums
// ---------------------------------------------------------------------------

// GET /api/photos/albums?library=
func (s *Server) handlePhotoAlbums(w http.ResponseWriter, r *http.Request) {
	libID := int64(qInt(r, "library", 0))
	if _, err := s.libraryOfType(r, libID, "photos"); err != nil {
		writeLibErr(w, err)
		return
	}
	a, err := s.Store.PhotoAlbums(r.Context(), libID, userFrom(r).ID, s.access(r))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, a)
}

type albumBody struct {
	LibraryID   int64  `json:"libraryId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Shared      bool   `json:"shared"`
	CoverID     *int64 `json:"coverId"`
}

func (b *albumBody) clean() error {
	b.Name, b.Description = strings.TrimSpace(b.Name), strings.TrimSpace(b.Description)
	if b.Name == "" || len(b.Name) > 200 {
		return library.ErrBadInput("an album needs a name of up to 200 characters")
	}
	if len(b.Description) > 4000 {
		return library.ErrBadInput("the description is too long")
	}
	return nil
}

// POST /api/photos/albums
func (s *Server) handleCreateAlbum(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !canPost(u) {
		writeErr(w, 403, "this profile can't make albums")
		return
	}
	if !s.underLimit(w, r, u, `SELECT COUNT(*) FROM photo_albums WHERE owner_id = ?`, maxAlbumsPerAccount, "albums") {
		return
	}
	var b albumBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := b.clean(); err != nil {
		writeLibErr(w, err)
		return
	}
	if _, err := s.libraryOfType(r, b.LibraryID, "photos"); err != nil {
		writeLibErr(w, err)
		return
	}
	a, err := s.Store.CreatePhotoAlbum(r.Context(), b.LibraryID, u.ID, b.Name, b.Description, b.Shared)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 201, a)
}

// ownAlbum loads an album the account may change.
func (s *Server) ownAlbum(r *http.Request) (*library.PhotoAlbum, error) {
	id, ok := pathInt(r, "id")
	if !ok {
		return nil, library.ErrNotFound
	}
	u := userFrom(r)
	a, err := s.Store.PhotoAlbumByID(r.Context(), id, u.ID, u.IsAdmin)
	if err != nil {
		return nil, err
	}
	if a.OwnerID != u.ID && !u.IsAdmin {
		return nil, library.ErrNotFound
	}
	return a, nil
}

// PUT /api/photos/albums/{id}
func (s *Server) handleUpdateAlbum(w http.ResponseWriter, r *http.Request) {
	a, err := s.ownAlbum(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	var b albumBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := b.clean(); err != nil {
		writeLibErr(w, err)
		return
	}
	if b.CoverID != nil && *b.CoverID != 0 {
		if it, err := s.visibleItem(r, *b.CoverID); err != nil || it.LibraryID != a.LibraryID {
			writeErr(w, 400, "that picture can't be the cover")
			return
		}
	}
	if err := s.Store.UpdatePhotoAlbum(r.Context(), a.ID, b.Name, b.Description, b.Shared, b.CoverID); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/photos/albums/{id}: the pictures stay.
func (s *Server) handleDeleteAlbum(w http.ResponseWriter, r *http.Request) {
	a, err := s.ownAlbum(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	if err := s.Store.DeletePhotoAlbum(r.Context(), a.ID); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

const maxAlbumBatch = 2000

// POST /api/photos/albums/{id}/items {ids}
func (s *Server) handleAlbumAdd(w http.ResponseWriter, r *http.Request) {
	a, err := s.ownAlbum(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	var b struct {
		IDs []int64 `json:"ids"`
	}
	if err := readJSON(r, &b); err != nil || len(b.IDs) == 0 || len(b.IDs) > maxAlbumBatch {
		writeErr(w, 400, "ids must list 1 to 2000 pictures")
		return
	}
	allowed := make([]int64, 0, len(b.IDs))
	for _, id := range b.IDs {
		if _, err := s.visibleItem(r, id); err == nil {
			allowed = append(allowed, id)
		}
	}
	n, err := s.Store.AddToPhotoAlbum(r.Context(), a, allowed)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"added": n})
}

// DELETE /api/photos/albums/{id}/items {ids}
func (s *Server) handleAlbumRemove(w http.ResponseWriter, r *http.Request) {
	a, err := s.ownAlbum(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	var b struct {
		IDs []int64 `json:"ids"`
	}
	if err := readJSON(r, &b); err != nil || len(b.IDs) == 0 || len(b.IDs) > maxAlbumBatch {
		writeErr(w, 400, "ids must list 1 to 2000 pictures")
		return
	}
	if err := s.Store.RemoveFromPhotoAlbum(r.Context(), a.ID, b.IDs); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Uploads
// ---------------------------------------------------------------------------

// uploadMaxBytes is the largest single request: settings can lower or raise it.
func (s *Server) uploadMaxBytes(r *http.Request) int64 {
	mb, err := strconv.ParseInt(s.DB.GetSetting(r.Context(), "uploadMaxMb", "4096"), 10, 64)
	if err != nil || mb <= 0 {
		mb = 4096
	}
	return mb << 20
}

// uploadFreeFloor is the room uploads leave on a disk: the server's database may share it, and
// stops when it's full.
var uploadFreeFloor int64 = 2 << 30

func (s *Server) uploadDiskFull(lib *library.Library) bool {
	free := sysinfo.FreeBytes(lib.Paths[0])
	return free >= 0 && free <= uploadFreeFloor
}

// uploadRoom is how much more this account can send to this library now: what's left of its
// total across the server's galleries (uploadAccountMb, 0 for none, and none for an
// administrator), within what the disk can spare.
func (s *Server) uploadRoom(r *http.Request, u *auth.User, lib *library.Library, folder string) int64 {
	room := int64(math.MaxInt64)
	if free := sysinfo.FreeBytes(lib.Paths[0]); free >= 0 {
		room = free - uploadFreeFloor
	}
	mb, err := strconv.ParseInt(s.DB.GetSetting(r.Context(), "uploadAccountMb", settingDefaults["uploadAccountMb"]), 10, 64)
	if err != nil || mb <= 0 || u.IsAdmin {
		return room
	}
	used := int64(0)
	libs, _ := s.Store.Libraries(r.Context())
	for _, l := range libs {
		if (l.Type != "photos" && l.Type != "shorts") || len(l.Paths) == 0 {
			continue
		}
		_ = filepath.WalkDir(filepath.Join(l.Paths[0], "Uploads", folder), func(_ string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				if info, err := d.Info(); err == nil {
					used += info.Size()
				}
			}
			return nil
		})
	}
	return min(room, mb<<20-used)
}

// POST /api/libraries/{id}/upload (multipart, field "file", any number of files)
// Files go to <library folder>/Uploads/<profile>/<yyyy-mm>/ and never replace
// anything that is already there.
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !canPost(u) {
		writeErr(w, 403, "this profile can't upload")
		return
	}
	if s.DB.GetSetting(r.Context(), "uploads", "1") != "1" {
		writeErr(w, 403, "uploads are turned off on this server")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	lib, err := s.libraryOfType(r, id, "photos", "shorts")
	if err != nil {
		writeLibErr(w, err)
		return
	}
	if len(lib.Paths) == 0 {
		writeErr(w, 400, "this library has no folder to save to")
		return
	}
	if lib.ReadOnly {
		writeErr(w, 403, library.ErrReadOnly.Error())
		return
	}
	folder := library.SafeFileName(u.Name)
	if folder == "" {
		folder = fmt.Sprintf("profile-%d", u.ID)
	}
	room := s.uploadRoom(r, u, lib, folder)
	if room <= 0 {
		if s.uploadDiskFull(lib) {
			writeErr(w, 507, "The disk this library is on is nearly full.")
			return
		}
		writeErr(w, 413, "You've used all the upload room this server gives one account.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, min(s.uploadMaxBytes(r), room))
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, 400, "expected a multipart upload")
		return
	}
	dest := filepath.Join(lib.Paths[0], "Uploads", folder, time.Now().Format("2006-01"))
	type result struct {
		Name string `json:"name"`
		ID   int64  `json:"id,omitempty"`
		Err  string `json:"error,omitempty"`
	}
	results := []result{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				writeJSON(w, 413, map[string]any{"error": "this upload is larger than the server allows", "files": results})
				return
			}
			writeJSON(w, 400, map[string]any{"error": "the upload stopped early", "files": results})
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			part.Close()
			continue
		}
		res := result{Name: part.FileName()}
		item, err := s.saveUpload(r, lib, dest, part)
		part.Close()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				res.Err = "larger than the server allows"
				results = append(results, res)
				writeJSON(w, 413, map[string]any{"error": "this upload is larger than the server allows", "files": results})
				return
			}
			var bad library.ErrBadInput
			if errors.As(err, &bad) {
				res.Err = bad.Error()
			} else {
				s.Log.WarnContext(r.Context(), "could not save an upload", "cat", "library", "library", lib.ID, "file", res.Name, "err", err)
				res.Err = "could not be saved"
			}
		} else {
			res.ID = item.ID
		}
		results = append(results, res)
	}
	writeJSON(w, 200, map[string]any{"files": results})
}

// saveUpload writes one file: named safely, checked by its first bytes as well
// as its name, created so it can't overwrite anything, and removed if it fails.
func (s *Server) saveUpload(r *http.Request, lib *library.Library, dest string, src io.Reader) (*library.Item, error) {
	part, _ := src.(interface{ FileName() string })
	name := library.SafeFileName(part.FileName())
	if name == "" || (!library.IsPhotoFile(name) && !library.IsVideoFile(name)) {
		return nil, library.ErrBadInput("not a picture or video this library takes")
	}
	if lib.Type == "shorts" && !library.IsVideoFile(name) {
		return nil, library.ErrBadInput("this library takes videos")
	}
	head := make([]byte, 512)
	n, err := io.ReadFull(src, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if !library.SniffMedia(head[:n]) {
		return nil, library.ErrBadInput("the file's contents are not a picture or video")
	}
	f, path, err := library.CreateInLibrary(lib.Paths[0], dest, name)
	if err != nil {
		return nil, err
	}
	_, err = f.Write(head[:n])
	if err == nil {
		_, err = io.Copy(f, src)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	it, err := s.Scanner.IndexUploaded(r.Context(), lib, path)
	if err != nil {
		os.Remove(path)
		return nil, library.ErrBadInput("the file could not be read as a picture or video")
	}
	return it, nil
}
