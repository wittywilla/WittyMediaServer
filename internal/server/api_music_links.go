// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/music"
)

// Songs tied to their music videos. The link itself and its matching are the music
// framework's (internal/music); these are its routes, and what keeps it up to date.

func (s *Server) musicLinks() *music.Links { return &music.Links{DB: s.DB.DB} }

// MusicLinkSide is the song or the video at one end of a link.
type MusicLinkSide struct {
	ID         int64   `json:"id"`
	LibraryID  int64   `json:"libraryId"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist"`
	Album      string  `json:"album,omitempty"`
	RuntimeSec float64 `json:"runtimeSec"`
	UpdatedAt  int64   `json:"updatedAt"`
}

// MusicLinkView is a link with what's at both ends.
type MusicLinkView struct {
	SongID  int64          `json:"songId"`
	VideoID int64          `json:"videoId"`
	State   string         `json:"state"` // tied | suggested
	How     string         `json:"how"`   // isrc | recording | name | close | hand
	SyncMS  *int64         `json:"syncMs,omitempty"`
	By      string         `json:"by,omitempty"`
	Song    *MusicLinkSide `json:"song"`
	Video   *MusicLinkSide `json:"video"`
}

// MusicLinks is everything one song or video is tied to, and (for admins) what it might be.
type MusicLinks struct {
	ItemID    int64           `json:"itemId"`
	Role      string          `json:"role"` // song | video
	Tied      []MusicLinkView `json:"tied"`
	Suggested []MusicLinkView `json:"suggested"`
	CanEdit   bool            `json:"canEdit"`
}

func linkSide(it *library.Item) *MusicLinkSide {
	side := &MusicLinkSide{ID: it.ID, LibraryID: it.LibraryID, Title: it.Title, Album: it.Album, RuntimeSec: it.RuntimeSec, UpdatedAt: it.UpdatedAt}
	switch {
	case it.Kind == "video":
		side.Artist, side.Album = it.Studio, ""
	case it.Artist != "":
		side.Artist = it.Artist
	default:
		side.Artist = it.AlbumArtist
	}
	return side
}

// linkViews puts the items into links, and leaves out any link to something this person
// can't see.
func (s *Server) linkViews(r *http.Request, links []music.Link) ([]MusicLinkView, error) {
	out := []MusicLinkView{}
	if len(links) == 0 {
		return out, nil
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, l := range links {
		for _, id := range []int64{l.SongID, l.VideoID} {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	items, _, err := s.Store.Items(r.Context(), library.ItemQuery{IDs: ids, Access: s.access(r), Brief: true, NoTotal: true, Limit: len(ids)})
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*library.Item, len(items))
	for _, it := range items {
		if !it.Missing {
			byID[it.ID] = it
		}
	}
	for _, l := range links {
		song, video := byID[l.SongID], byID[l.VideoID]
		if song == nil || video == nil {
			continue
		}
		out = append(out, MusicLinkView{SongID: l.SongID, VideoID: l.VideoID, State: l.State, How: l.How, SyncMS: l.SyncMS, By: l.By, Song: linkSide(song), Video: linkSide(video)})
	}
	return out, nil
}

// linkRole says whether an item can be in a link, and as which end.
func (s *Server) linkRole(ctx context.Context, it *library.Item) string {
	lib, err := s.Store.Library(ctx, it.LibraryID)
	if err != nil {
		return ""
	}
	switch {
	case lib.Type == "music" && it.Kind == "audio":
		return "song"
	case lib.Type == "musicvideos" && it.Kind == "video":
		return "video"
	}
	return ""
}

// GET /api/music/links/{id}: what a song or a music video is tied to. Admins also get
// what it might be tied to.
func (s *Server) handleMusicLinks(w http.ResponseWriter, r *http.Request) {
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
	u := userFrom(r)
	out := MusicLinks{ItemID: it.ID, Role: s.linkRole(r.Context(), it), Tied: []MusicLinkView{}, Suggested: []MusicLinkView{}, CanEdit: u.IsAdmin}
	if out.Role == "" {
		writeJSON(w, 200, out)
		return
	}
	links, err := s.musicLinks().Of(r.Context(), it.ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !u.IsAdmin {
		tied := links[:0]
		for _, l := range links {
			if l.State == "tied" {
				tied = append(tied, l)
			}
		}
		links = tied
	}
	views, err := s.linkViews(r, links)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, v := range views {
		if v.State == "tied" {
			out.Tied = append(out.Tied, v)
		} else {
			out.Suggested = append(out.Suggested, v)
		}
	}
	writeJSON(w, 200, out)
}

// GET /api/music/links/{id}/candidates?q= (admin): the other side to choose from by hand.
func (s *Server) handleMusicLinkCandidates(w http.ResponseWriter, r *http.Request) {
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
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 200 {
		writeErr(w, 400, "that search is too long")
		return
	}
	if q == "" {
		q = it.Title
	}
	ids, err := s.musicLinks().Candidates(r.Context(), it.ID, q, qInt(r, "limit", 30))
	if errors.Is(err, music.ErrNotPair) {
		writeErr(w, 400, "only songs and music videos can be tied")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := []MusicLinkSide{}
	if len(ids) > 0 {
		items, _, err := s.Store.Items(r.Context(), library.ItemQuery{IDs: ids, Access: s.access(r), Brief: true, NoTotal: true, Limit: len(ids), Sort: "title"})
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		for _, c := range items {
			out = append(out, *linkSide(c))
		}
	}
	writeJSON(w, 200, out)
}

// GET /api/music/links (admin): suggestions waiting for someone to look at them.
func (s *Server) handleMusicLinkSuggestions(w http.ResponseWriter, r *http.Request) {
	links, err := s.musicLinks().Suggested(r.Context(), qInt(r, "limit", 100))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	views, err := s.linkViews(r, links)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, views)
}

// The longest intro a video may have before its song starts, or the song before its video.
const maxSyncMS = 10 * 60 * 1000

// PUT /api/music/links (admin) {songId, videoId, syncMs, noSync}: ties a song to a video
// for good. syncMs is where the song starts in the video; noSync says they don't line up;
// with neither, what was there stays.
func (s *Server) handleMusicLinkTie(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SongID  int64  `json:"songId"`
		VideoID int64  `json:"videoId"`
		SyncMS  *int64 `json:"syncMs"`
		NoSync  bool   `json:"noSync"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.SyncMS != nil && (*in.SyncMS < -maxSyncMS || *in.SyncMS > maxSyncMS) {
		writeErr(w, 400, "that's more than ten minutes out")
		return
	}
	if !s.bothVisible(r, in.SongID, in.VideoID) {
		writeErr(w, 404, "not found")
		return
	}
	keep := in.SyncMS == nil && !in.NoSync
	err := s.musicLinks().Tie(r.Context(), in.SongID, in.VideoID, in.SyncMS, keep, userFrom(r).Name)
	if errors.Is(err, music.ErrNotPair) {
		writeErr(w, 400, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	l, _ := s.musicLinks().Get(r.Context(), in.SongID, in.VideoID)
	writeJSON(w, 200, l)
}

// DELETE /api/music/links?song=&video= (admin): unties them, and no match ties them again.
func (s *Server) handleMusicLinkUntie(w http.ResponseWriter, r *http.Request) {
	song, video := int64(qInt(r, "song", 0)), int64(qInt(r, "video", 0))
	if !s.bothVisible(r, song, video) {
		writeErr(w, 404, "not found")
		return
	}
	err := s.musicLinks().Untie(r.Context(), song, video, userFrom(r).Name)
	if errors.Is(err, music.ErrNotPair) {
		writeErr(w, 400, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *Server) bothVisible(r *http.Request, ids ...int64) bool {
	for _, id := range ids {
		if id <= 0 {
			return false
		}
		if _, err := s.visibleItem(r, id); err != nil {
			return false
		}
	}
	return true
}

// POST /api/music/links/match (admin): matches every song and video again now.
func (s *Server) handleMusicLinkMatch(w http.ResponseWriter, r *http.Request) {
	res, err := s.rematchMusic(r.Context(), nil)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, res)
}

// One round of matching at a time, and scans finishing while one waits share the next.
var (
	musicMatching    sync.Mutex
	musicMatchQueued atomic.Bool
)

// rematchMusic runs a round; started is called once it's this round's turn.
func (s *Server) rematchMusic(ctx context.Context, started func()) (music.Result, error) {
	musicMatching.Lock()
	defer musicMatching.Unlock()
	if started != nil {
		started()
	}
	res, err := s.musicLinks().Rematch(ctx)
	if err == nil {
		_ = s.DB.SetSetting(ctx, "music.linksMatched", "1")
		s.Log.InfoContext(ctx, "songs and music videos matched", "cat", "library", "tied", res.Tied, "suggested", res.Suggested)
	}
	return res, err
}

// kickMusicMatch matches in the background, after a scan of music or music videos.
func (s *Server) kickMusicMatch(libraryID int64) {
	if libraryID != 0 {
		lib, err := s.Store.Library(context.Background(), libraryID)
		if err != nil || (lib.Type != "music" && lib.Type != "musicvideos") {
			return
		}
	}
	if !musicMatchQueued.CompareAndSwap(false, true) {
		return // the round already waiting will see this scan too
	}
	logging.Go("music video matching", func() {
		if _, err := s.rematchMusic(context.Background(), func() { musicMatchQueued.Store(false) }); err != nil {
			s.Log.Warn("couldn't match songs with music videos", "cat", "library", "err", err)
		}
	}, "cat", "library")
}

// MusicVideoCard is a music video in a music library's Music videos, with the song it's
// tied to, when this person may see that song.
type MusicVideoCard struct {
	library.VideoCard
	Song   *MusicLinkSide `json:"song,omitempty"`
	SyncMS *int64         `json:"syncMs,omitempty"`
}

// GET /api/music/videos?search=&sort=&limit=&offset=: the videos of every music video
// library this person can see, by artist (or newest first with sort=latest).
func (s *Server) handleMusicVideos(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sort := "artist"
	if q.Get("sort") == "latest" {
		sort = "latest"
	}
	search := strings.TrimSpace(q.Get("search"))
	if len(search) > 200 {
		writeErr(w, 400, "that search is too long")
		return
	}
	cards, more, err := s.Store.Videos(r.Context(), library.VideoQuery{LibraryType: "musicvideos", UserID: userFrom(r).ID, Access: s.access(r),
		Search: search, Sort: sort, Limit: qInt(r, "limit", 60), Offset: max(0, qInt(r, "offset", 0))})
	if err != nil {
		writeLibErr(w, err)
		return
	}
	ids := make([]int64, len(cards))
	for i, c := range cards {
		ids[i] = c.ID
	}
	links, err := s.musicLinks().Tied(r.Context(), ids)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	views, err := s.linkViews(r, links) // only songs this person may see
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	byVideo := make(map[int64]MusicLinkView, len(views))
	for _, v := range views {
		if _, ok := byVideo[v.VideoID]; !ok {
			byVideo[v.VideoID] = v
		}
	}
	out := make([]MusicVideoCard, len(cards))
	for i, c := range cards {
		out[i].VideoCard = c
		if v, ok := byVideo[c.ID]; ok {
			out[i].Song, out[i].SyncMS = v.Song, v.SyncMS
		}
	}
	writeJSON(w, 200, map[string]any{"videos": out, "more": more})
}
