package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Collections and smart playlists, radio stations, Year in review and the home
// page's recommendations. See docs/COLLECTIONS.md, docs/RADIO.md, docs/REVIEW.md.

// GET /api/collections
func (s *Server) handleCollections(w http.ResponseWriter, r *http.Request) {
	c, err := s.Store.Collections(r.Context(), userFrom(r).ID, s.access(r))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, c)
}

type collectionBody struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Kind        string                  `json:"kind"`
	Shared      bool                    `json:"shared"`
	Rule        *library.CollectionRule `json:"rule"`
}

func (b *collectionBody) clean() error {
	b.Name, b.Description = strings.TrimSpace(b.Name), strings.TrimSpace(b.Description)
	if b.Name == "" || len(b.Name) > 200 {
		return library.ErrBadInput("a collection needs a name of up to 200 characters")
	}
	if len(b.Description) > 4000 {
		return library.ErrBadInput("the description is too long")
	}
	return nil
}

// POST /api/collections
func (s *Server) handleCreateCollection(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !canPost(u) {
		writeErr(w, 403, "this profile can't make collections")
		return
	}
	var b collectionBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := b.clean(); err != nil {
		writeLibErr(w, err)
		return
	}
	c := &library.Collection{OwnerID: u.ID, Owner: u.Name, Name: b.Name, Description: b.Description, Kind: b.Kind, Shared: b.Shared, Rule: b.Rule}
	if err := s.Store.SaveCollection(r.Context(), c); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 201, c)
}

// POST /api/collections/preview {rule}: what a rule would find.
func (s *Server) handlePreviewRule(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Rule library.CollectionRule `json:"rule"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	items, count, err := s.Store.PreviewRule(r.Context(), &b.Rule, userFrom(r).ID, s.access(r))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "count": count})
}

func (s *Server) visibleCollection(r *http.Request) (*library.Collection, error) {
	id, ok := pathInt(r, "id")
	if !ok {
		return nil, library.ErrNotFound
	}
	u := userFrom(r)
	return s.Store.CollectionByID(r.Context(), id, u.ID, u.IsAdmin)
}

func (s *Server) ownCollection(r *http.Request) (*library.Collection, error) {
	c, err := s.visibleCollection(r)
	if err != nil {
		return nil, err
	}
	u := userFrom(r)
	if c.OwnerID != u.ID && !u.IsAdmin {
		return nil, library.ErrNotFound
	}
	return c, nil
}

// GET /api/collections/{id}?limit=&offset=
func (s *Server) handleCollection(w http.ResponseWriter, r *http.Request) {
	c, err := s.visibleCollection(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	u := userFrom(r)
	limit, offset := qInt(r, "limit", 100), qInt(r, "offset", 0)
	items, shows, err := s.Store.CollectionItems(r.Context(), c, u.ID, s.access(r), limit, offset)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"collection": c, "items": items, "shows": shows, "more": len(items) == limit})
}

// PUT /api/collections/{id}
func (s *Server) handleUpdateCollection(w http.ResponseWriter, r *http.Request) {
	c, err := s.ownCollection(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	var b collectionBody
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := b.clean(); err != nil {
		writeLibErr(w, err)
		return
	}
	c.Name, c.Description, c.Shared = b.Name, b.Description, b.Shared
	if c.Kind == "smart" && b.Rule != nil {
		c.Rule = b.Rule
	}
	if c.System != "" {
		c.Shared = false // Watch later stays personal
	}
	if err := s.Store.SaveCollection(r.Context(), c); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, c)
}

// DELETE /api/collections/{id}: what it holds stays.
func (s *Server) handleDeleteCollection(w http.ResponseWriter, r *http.Request) {
	c, err := s.ownCollection(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	if c.System != "" {
		writeErr(w, 400, "this list can't be deleted")
		return
	}
	if err := s.Store.DeleteCollection(r.Context(), c.ID); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

type collectionItemsBody struct {
	Items []int64 `json:"items"`
	Shows []int64 `json:"shows"`
}

func (b *collectionItemsBody) valid() bool {
	n := len(b.Items) + len(b.Shows)
	return n > 0 && n <= maxAlbumBatch
}

// POST /api/collections/{id}/items {items, shows}
func (s *Server) handleCollectionAdd(w http.ResponseWriter, r *http.Request) {
	c, err := s.ownCollection(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	s.addToCollection(w, r, c)
}

// POST /api/collections/watch-later/items {items, shows}
func (s *Server) handleWatchLaterAdd(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !canReact(u) {
		writeErr(w, 403, "this profile can't keep lists")
		return
	}
	c, err := s.Store.WatchLater(r.Context(), u.ID)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	s.addToCollection(w, r, c)
}

func (s *Server) addToCollection(w http.ResponseWriter, r *http.Request, c *library.Collection) {
	if c.Kind == "smart" {
		writeErr(w, 400, "a smart playlist fills itself")
		return
	}
	var b collectionItemsBody
	if err := readJSON(r, &b); err != nil || !b.valid() {
		writeErr(w, 400, "items and shows must list 1 to 2000 things")
		return
	}
	items := make([]int64, 0, len(b.Items))
	for _, id := range b.Items {
		if _, err := s.visibleItem(r, id); err == nil {
			items = append(items, id)
		}
	}
	shows := make([]int64, 0, len(b.Shows))
	for _, id := range b.Shows {
		if _, err := s.visibleShow(r, id); err == nil {
			shows = append(shows, id)
		}
	}
	if err := s.Store.AddToCollection(r.Context(), c.ID, items, shows); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"added": len(items) + len(shows), "collectionId": c.ID})
}

// DELETE /api/collections/{id}/items {items, shows}
func (s *Server) handleCollectionRemove(w http.ResponseWriter, r *http.Request) {
	c, err := s.ownCollection(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	var b collectionItemsBody
	if err := readJSON(r, &b); err != nil || !b.valid() {
		writeErr(w, 400, "items and shows must list 1 to 2000 things")
		return
	}
	if err := s.Store.RemoveFromCollection(r.Context(), c.ID, b.Items, b.Shows); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/collections/holding?item=&show=: which of this account's lists hold it.
func (s *Server) handleCollectionsHolding(w http.ResponseWriter, r *http.Request) {
	ids, err := s.Store.CollectionsHolding(r.Context(), userFrom(r).ID, int64(qInt(r, "item", 0)), int64(qInt(r, "show", 0)))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, ids)
}

// nextForCollection answers "what's next" when a list is being played through.
func (s *Server) nextForCollection(r *http.Request, it *library.Item) *int64 {
	id := int64(qInt(r, "collection", 0))
	if id == 0 {
		return nil
	}
	u := userFrom(r)
	c, err := s.Store.CollectionByID(r.Context(), id, u.ID, u.IsAdmin)
	if err != nil {
		return nil
	}
	return s.Store.NextInCollection(r.Context(), c, u.ID, it.ID, s.access(r))
}

// ---------------------------------------------------------------------------
// Radio
// ---------------------------------------------------------------------------

// GET /api/radio?library=
func (s *Server) handleRadioList(w http.ResponseWriter, r *http.Request) {
	libID := int64(qInt(r, "library", 0))
	if _, err := s.libraryOfType(r, libID, "radio"); err != nil {
		writeLibErr(w, err)
		return
	}
	items, _, err := s.Store.Items(r.Context(), library.ItemQuery{LibraryID: libID, Kind: "radio", Sort: "title", Access: s.access(r), Limit: 5000})
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, items)
}

// POST /api/radio {libraryId, name, url, genre} (admin)
func (s *Server) handleRadioAdd(w http.ResponseWriter, r *http.Request) {
	var b struct {
		LibraryID int64 `json:"libraryId"`
		library.RadioStation
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := s.libraryOfType(r, b.LibraryID, "radio"); err != nil {
		writeLibErr(w, err)
		return
	}
	it, err := s.Store.AddRadioStation(r.Context(), b.LibraryID, b.RadioStation)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 201, it)
}

// POST /api/radio/import {libraryId, playlist} (admin): an M3U or PLS playlist's text.
func (s *Server) handleRadioImport(w http.ResponseWriter, r *http.Request) {
	var b struct {
		LibraryID int64  `json:"libraryId"`
		Playlist  string `json:"playlist"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := s.libraryOfType(r, b.LibraryID, "radio"); err != nil {
		writeLibErr(w, err)
		return
	}
	added := 0
	for _, st := range library.ParsePlaylist(b.Playlist) {
		if _, err := s.Store.AddRadioStation(r.Context(), b.LibraryID, st); err == nil {
			added++
		}
	}
	writeJSON(w, 200, map[string]any{"added": added})
}

// DELETE /api/radio/{id} (admin)
func (s *Server) handleRadioRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.Store.RemoveRadioStation(r.Context(), id); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// radioTypes: what a station may send. A playlist file is unwrapped by the
// client's import, not played; anything else isn't audio.
func radioAudioType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "application/ogg") || strings.HasPrefix(ct, "video/mp2t") || ct == "application/octet-stream" || ct == ""
}

// GET /api/radio/{id}/stream: relays a station, so listeners on any device get it
// from this server with one sign-in, whatever the station's address looks like. It
// reaches the internet only (never this network), only while someone listens,
// and the file type is checked, so a station's address can't serve up a web page.
func (s *Server) handleRadioStream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "radio" {
		writeErr(w, 404, "not found")
		return
	}
	if !library.ValidStreamURL(it.Path) {
		writeErr(w, 404, "not found")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", it.Path, nil)
	if err != nil {
		writeErr(w, 400, "that station's address can't be used")
		return
	}
	req.Header.Set("User-Agent", "WittyMediaServer")
	res, err := library.SafeClient.Do(req)
	if err != nil {
		s.Log.Info("radio", "cat", "playback", "item", it.ID, "err", err)
		writeErr(w, 502, "the station can't be reached")
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		writeErr(w, 502, "the station answered "+res.Status)
		return
	}
	ct := res.Header.Get("Content-Type")
	if !radioAudioType(ct) {
		writeErr(w, 502, "that address isn't an audio stream; if it's a playlist, import it instead")
		return
	}
	if ct == "" || ct == "application/octet-stream" {
		ct = "audio/mpeg"
	}
	s.Store.TouchRadio(context.WithoutCancel(r.Context()), it.ID)
	h := w.Header()
	h.Set("Content-Type", ct)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	started := time.Now()
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if rerr != nil {
			if !errors.Is(rerr, io.EOF) && r.Context().Err() == nil {
				s.Log.Info("radio stream ended", "cat", "playback", "item", it.ID, "err", rerr)
			}
			break
		}
	}
	_ = s.Store.RecordView(context.WithoutCancel(r.Context()), userFrom(r).ID, it.ID, library.ViewEvent{Seconds: time.Since(started).Seconds()})
}

// ---------------------------------------------------------------------------
// Year in review, and recommendations
// ---------------------------------------------------------------------------

// GET /api/me/review?year=
func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	year := qInt(r, "year", time.Now().Year())
	if year < 2000 || year > 3000 {
		writeErr(w, 400, "bad year")
		return
	}
	rv, err := s.Store.ReviewOfYear(r.Context(), userFrom(r).ID, year)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, rv)
}

// DELETE /api/me/history: forgets what this account watched and listened to.
func (s *Server) handleClearHistory(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.ClearWatchHistory(r.Context(), userFrom(r).ID); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/me/recommended?limit=: films and shows for this account, from its own
// record on this server.
func (s *Server) handleRecommended(w http.ResponseWriter, r *http.Request) {
	limit := qInt(r, "limit", 20)
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	u := userFrom(r)
	items, shows, because, err := s.Store.RecommendForYou(r.Context(), u.ID, s.access(r), library.FeedSeed(u.ID), limit)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "shows": shows, "because": because})
}
