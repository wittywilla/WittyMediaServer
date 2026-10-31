package server

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Podcasts: subscriptions are managed by admins; everyone with access to
// the library listens. Episodes are audio items (see library/podcasts.go).

// GET /api/podcasts?library=
func (s *Server) handlePodcasts(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Store.Podcasts(r.Context(), int64(qInt(r, "library", 0)))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if u := userFrom(r); !u.IsAdmin {
		a := s.access(r)
		out := ps[:0]
		for _, p := range ps {
			if a != nil && !a.AllowsLibrary(p.LibraryID) {
				continue
			}
			p.URL = "" // a private feed's address is a password
			out = append(out, p)
		}
		ps = out
	}
	writeJSON(w, 200, ps)
}

// POST /api/podcasts {library, url} (admin)
func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Library int64  `json:"library"`
		URL     string `json:"url"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	lib, err := s.Store.Library(r.Context(), b.Library)
	if err != nil || lib.Type != "podcasts" {
		writeErr(w, 400, "choose a podcasts library")
		return
	}
	p, err := s.Scanner.Subscribe(r.Context(), lib.ID, b.URL)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 201, p)
}

func (s *Server) podcastFrom(w http.ResponseWriter, r *http.Request) *library.Podcast {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	p, err := s.Store.Podcast(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "no such podcast")
		return nil
	}
	return p
}

// PUT /api/podcasts/{id} {autoDownload} (admin)
func (s *Server) handleUpdatePodcast(w http.ResponseWriter, r *http.Request) {
	p := s.podcastFrom(w, r)
	if p == nil {
		return
	}
	var b struct {
		AutoDownload *int `json:"autoDownload"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.AutoDownload != nil {
		n := min(max(*b.AutoDownload, 0), 50)
		if _, err := s.DB.ExecContext(r.Context(), `UPDATE podcasts SET auto_download = ? WHERE id = ?`, n, p.ID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		p.AutoDownload = n
	}
	writeJSON(w, 200, p)
}

// POST /api/podcasts/{id}/refresh (admin)
func (s *Server) handleRefreshPodcast(w http.ResponseWriter, r *http.Request) {
	p := s.podcastFrom(w, r)
	if p == nil {
		return
	}
	if err := s.Scanner.RefreshPodcast(r.Context(), p); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, p)
}

// DELETE /api/podcasts/{id} (admin): downloaded episodes stay.
func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	p := s.podcastFrom(w, r)
	if p == nil {
		return
	}
	if err := s.Store.Unsubscribe(r.Context(), p.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/items/{id}/download (admin): keep a podcast episode on disk.
func (s *Server) handleDownloadEpisode(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.Store.Item(r.Context(), id)
	if err != nil || it.Kind != "audio" {
		writeErr(w, 404, "not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if err := s.Scanner.Download(ctx, it); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, it)
}

// GET /api/podcasts/search?q= (admin): Apple's free podcast directory.
func (s *Server) handlePodcastSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		writeJSON(w, 200, []any{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", "https://itunes.apple.com/search?media=podcast&entity=podcast&limit=15&term="+url.QueryEscape(q), nil)
	res, err := library.SafeClient.Do(req)
	if err != nil {
		writeErr(w, 502, "the podcast directory did not answer")
		return
	}
	defer res.Body.Close()
	var out struct {
		Results []struct {
			Name   string `json:"collectionName"`
			Artist string `json:"artistName"`
			Feed   string `json:"feedUrl"`
			Image  string `json:"artworkUrl600"`
			Genre  string `json:"primaryGenreName"`
		} `json:"results"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&out); err != nil {
		writeErr(w, 502, "unexpected answer from the podcast directory")
		return
	}
	hits := []map[string]string{}
	for _, x := range out.Results {
		if x.Feed != "" {
			hits = append(hits, map[string]string{"title": x.Name, "author": x.Artist, "feed": x.Feed, "image": x.Image, "genre": x.Genre})
		}
	}
	writeJSON(w, 200, hits)
}

// proxyEpisode streams an episode that was not downloaded, through the
// server (private, works for old http:// feeds on https pages), passing
// Range so seeking works.
func (s *Server) proxyEpisode(w http.ResponseWriter, r *http.Request, it *library.Item) {
	req, err := http.NewRequestWithContext(r.Context(), "GET", it.Path, nil)
	if err != nil {
		writeErr(w, 400, "bad episode address")
		return
	}
	req.Header.Set("User-Agent", "WittyMediaServer/0.4 (podcasts)")
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}
	res, err := library.SafeClient.Do(req)
	if err != nil {
		if r.Context().Err() == nil {
			s.Log.WarnContext(r.Context(), "could not fetch a podcast episode", "cat", "playback", "item", it.ID, "url", it.Path, "err", err)
		}
		writeErr(w, 502, "the podcast server did not answer")
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 && res.StatusCode != 416 {
		s.Log.WarnContext(r.Context(), "could not fetch a podcast episode", "cat", "playback", "item", it.ID, "url", it.Path, "status", res.StatusCode)
		writeErr(w, 502, "the podcast server answered "+res.Status)
		return
	}
	for _, h := range []string{"Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag"} {
		if v := res.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("Content-Type", episodeType(res.Header.Get("Content-Type")))
	w.WriteHeader(res.StatusCode)
	// Too late to tell the listener, and usually it's they who went.
	if _, err := io.Copy(w, res.Body); err != nil && r.Context().Err() == nil {
		s.Log.DebugContext(r.Context(), "a podcast episode stopped part-way", "cat", "playback", "item", it.ID, "err", err)
	}
}

// episodeType keeps the type a podcast server gave only when it is audio or
// video: the address could serve anything, and this stream comes from our
// own site, so a web page from a feed must never be sent as one.
func episodeType(ct string) string {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return "application/octet-stream"
	}
	if strings.HasPrefix(mt, "audio/") || strings.HasPrefix(mt, "video/") || mt == "application/ogg" {
		return mt
	}
	return "application/octet-stream"
}
