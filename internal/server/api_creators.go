package server

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// WittyShorts and WittyChannels: creators, videos, the For You feed, likes and
// comments. Likes, comments and views work on any item an account may see.
// See docs/SHORTS.md and docs/CHANNELS.md.

var creatorLibraries = []string{"shorts", "channels", "musicvideos"}

// GET /api/creators?library=&search=&subscribed=
func (s *Server) handleCreators(w http.ResponseWriter, r *http.Request) {
	q := library.CreatorQuery{LibraryID: int64(qInt(r, "library", 0)), UserID: userFrom(r).ID, Access: s.access(r), Search: strings.TrimSpace(r.URL.Query().Get("search")), Subscribed: qBool(r, "subscribed")}
	if q.LibraryID != 0 {
		if _, err := s.libraryOfType(r, q.LibraryID, creatorLibraries...); err != nil {
			writeLibErr(w, err)
			return
		}
	}
	c, err := s.Store.Creators(r.Context(), q)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, c)
}

// visibleCreator: a creator with something the account may see. Otherwise the
// account can't learn that the creator exists.
func (s *Server) visibleCreator(r *http.Request) (*library.Creator, error) {
	id, ok := pathInt(r, "id")
	if !ok {
		return nil, library.ErrNotFound
	}
	c, err := s.Store.Creators(r.Context(), library.CreatorQuery{ID: id, UserID: userFrom(r).ID, Access: s.access(r)})
	if err != nil {
		return nil, err
	}
	if len(c) == 0 {
		return nil, library.ErrNotFound
	}
	return &c[0], nil
}

// GET /api/creators/{id}
func (s *Server) handleCreator(w http.ResponseWriter, r *http.Request) {
	c, err := s.visibleCreator(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	lists, err := s.Store.CreatorPlaylists(r.Context(), c.ID, s.access(r))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"creator": c, "playlists": lists})
}

// PUT /api/creators/{id} (admin)
func (s *Server) handleUpdateCreator(w http.ResponseWriter, r *http.Request) {
	c, err := s.visibleCreator(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	var b struct {
		Name        *string `json:"name"`
		Handle      *string `json:"handle"`
		Description *string `json:"description"`
		Platform    *string `json:"platform"`
		URL         *string `json:"url"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	name, handle, desc, platform, url := c.Name, c.Handle, c.Description, c.Platform, c.URL
	for _, f := range []struct {
		from *string
		to   *string
		max  int
	}{{b.Name, &name, 200}, {b.Handle, &handle, 200}, {b.Description, &desc, 8000}, {b.Platform, &platform, 80}, {b.URL, &url, 2000}} {
		if f.from != nil {
			*f.to = strings.TrimSpace(*f.from)
			if len(*f.to) > f.max {
				writeErr(w, 400, "that text is too long")
				return
			}
		}
	}
	if name == "" {
		writeErr(w, 400, "a creator needs a name")
		return
	}
	if err := s.Store.UpdateCreator(r.Context(), c.ID, name, handle, desc, platform, url); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/creators/{id}/art/{kind}: avatar or banner.
func (s *Server) handleCreatorArt(w http.ResponseWriter, r *http.Request) {
	c, err := s.visibleCreator(r)
	if err != nil {
		writeErr(w, 404, "no artwork")
		return
	}
	src, err := s.Store.CreatorArt(r.Context(), c.ID, r.PathValue("kind"))
	if err != nil {
		writeErr(w, 404, "no artwork")
		return
	}
	s.serveArtSrc(w, r, src)
}

// PUT /api/creators/{id}/subscribe {on}
func (s *Server) handleSubscribe2(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !canReact(u) {
		writeErr(w, 403, "this profile can't subscribe")
		return
	}
	c, err := s.visibleCreator(r)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	var b struct {
		On bool `json:"on"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Store.Subscribe(r.Context(), u.ID, c.ID, b.On); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"subscribed": b.On})
}

// GET /api/videos?library=&creator=&playlist=&tag=&platform=&search=&shape=vertical|wide
// &subscribed=&liked=&history=&sort=latest|oldest|popular|title&limit=&offset=
func (s *Server) handleVideos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	vq := library.VideoQuery{LibraryID: int64(qInt(r, "library", 0)), UserID: userFrom(r).ID, Access: s.access(r), CreatorID: int64(qInt(r, "creator", 0)),
		Tag: q.Get("tag"), Platform: q.Get("platform"), Search: strings.TrimSpace(q.Get("search")), Subscribed: qBool(r, "subscribed"), Liked: qBool(r, "liked"),
		History: qBool(r, "history"), Sort: q.Get("sort"), Limit: qInt(r, "limit", 0), Offset: qInt(r, "offset", 0)}
	if vq.LibraryID != 0 {
		if _, err := s.libraryOfType(r, vq.LibraryID, creatorLibraries...); err != nil {
			writeLibErr(w, err)
			return
		}
	}
	switch q.Get("length") {
	case "long":
		v := true
		vq.Long = &v
	case "short":
		v := false
		vq.Long = &v
	}
	if q.Has("playlist") {
		p := q.Get("playlist")
		vq.Playlist = &p
	}
	switch q.Get("shape") {
	case "vertical":
		v := true
		vq.Vertical = &v
	case "wide":
		v := false
		vq.Vertical = &v
	}
	items, more, err := s.Store.Videos(r.Context(), vq)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"videos": items, "more": more})
}

// GET /api/videos/feed?library=&seed=&limit=&offset=: For You. The first page
// answers with the seed to send for the next ones, so a session's order holds.
func (s *Server) handleVideoFeed(w http.ResponseWriter, r *http.Request) {
	libID := int64(qInt(r, "library", 0))
	if _, err := s.libraryOfType(r, libID, creatorLibraries...); err != nil {
		writeLibErr(w, err)
		return
	}
	seed, err := strconv.ParseInt(r.URL.Query().Get("seed"), 10, 64)
	if err != nil {
		seed = rand.Int64N(1 << 52)
	}
	vq := library.VideoQuery{LibraryID: libID, UserID: userFrom(r).ID, Access: s.access(r), Limit: qInt(r, "limit", 0), Offset: qInt(r, "offset", 0)}
	switch r.URL.Query().Get("shape") {
	case "vertical":
		v := true
		vq.Vertical = &v
	case "wide":
		v := false
		vq.Vertical = &v
	}
	items, more, err := s.Store.ForYou(r.Context(), vq, seed)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"videos": items, "more": more, "seed": seed})
}

// GET /api/videos/tags?library=
func (s *Server) handleVideoTags(w http.ResponseWriter, r *http.Request) {
	libID := int64(qInt(r, "library", 0))
	if _, err := s.libraryOfType(r, libID, creatorLibraries...); err != nil {
		writeLibErr(w, err)
		return
	}
	t, err := s.Store.VideoTags(r.Context(), libID, s.access(r), 30)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, t)
}

// GET /api/videos/{id}
func (s *Server) handleVideo(w http.ResponseWriter, r *http.Request) {
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
	d, err := s.Store.VideoByID(r.Context(), userFrom(r).ID, it)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, d)
}

// GET /api/videos/{id}/related?limit=
func (s *Server) handleVideoRelated(w http.ResponseWriter, r *http.Request) {
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
	limit := qInt(r, "limit", 20)
	if limit <= 0 || limit > 60 {
		limit = 20
	}
	rel, err := s.Store.RelatedVideos(r.Context(), library.VideoQuery{UserID: userFrom(r).ID, Access: s.access(r)}, it, limit)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, rel)
}

// PUT /api/videos/{id} (admin): custom metadata. Title, description, genres and
// rating go through PUT /api/items/{id}, as for every other item.
func (s *Server) handleEditVideo(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.Store.Item(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	var e library.VideoEdit
	if err := readJSON(r, &e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Store.EditVideo(r.Context(), it, e); err != nil {
		writeLibErr(w, err)
		return
	}
	d, err := s.Store.VideoByID(r.Context(), userFrom(r).ID, it)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, d)
}

// ---------------------------------------------------------------------------
// On any item: likes, comments, views
// ---------------------------------------------------------------------------

// PUT /api/items/{id}/reaction {value: 1|0|-1}
func (s *Server) handleReaction(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !canReact(u) {
		writeErr(w, 403, "this profile can't like things")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.visibleItem(r, id); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Value int `json:"value"`
	}
	if err := readJSON(r, &b); err != nil || b.Value < -1 || b.Value > 1 {
		writeErr(w, 400, "value must be 1, 0 or -1")
		return
	}
	if err := s.Store.SetReaction(r.Context(), u.ID, id, b.Value); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"value": b.Value})
}

// POST /api/items/{id}/view {seconds, completed, skipped}
func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.visibleItem(r, id); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	var e library.ViewEvent
	if err := readJSON(r, &e); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.Store.RecordView(r.Context(), userFrom(r).ID, id, e); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// GET /api/items/{id}/comments?limit=&offset=
func (s *Server) handleComments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.visibleItem(r, id); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	c, total, err := s.Store.Comments(r.Context(), id, qInt(r, "limit", 0), qInt(r, "offset", 0))
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"comments": c, "total": total})
}

// POST /api/items/{id}/comments {body, parentId}
func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !canPost(u) {
		writeErr(w, 403, "this profile can't comment")
		return
	}
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.visibleItem(r, id); err != nil {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Body     string `json:"body"`
		ParentID *int64 `json:"parentId"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body := library.CleanComment(b.Body)
	if body == "" {
		writeErr(w, 400, "a comment needs text, up to 2000 characters")
		return
	}
	c, err := s.Store.AddComment(r.Context(), u.ID, id, b.ParentID, body)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 201, c)
}

// ownComment: its writer or an administrator may change it.
func (s *Server) ownComment(r *http.Request) (int64, bool) {
	id, ok := pathInt(r, "cid")
	if !ok {
		return 0, false
	}
	owner, item, err := s.Store.CommentOwner(r.Context(), id)
	if err != nil {
		return 0, false
	}
	u := userFrom(r)
	if u.IsAdmin {
		return id, true
	}
	if owner != u.ID {
		return 0, false
	}
	_, err = s.visibleItem(r, item)
	return id, err == nil
}

// PUT /api/comments/{cid} {body}
func (s *Server) handleEditComment(w http.ResponseWriter, r *http.Request) {
	id, ok := s.ownComment(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	var b struct {
		Body string `json:"body"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	body := library.CleanComment(b.Body)
	if body == "" {
		writeErr(w, 400, "a comment needs text, up to 2000 characters")
		return
	}
	if err := s.Store.EditComment(r.Context(), id, body); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// DELETE /api/comments/{cid}
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	id, ok := s.ownComment(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	if err := s.Store.DeleteComment(r.Context(), id); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
