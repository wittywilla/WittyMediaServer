// Package server wires the HTTP API, streaming endpoints and the web UI.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/themes"
	"github.com/wittywilla/wittymediaserver/internal/update"
)

type Server struct {
	Cfg     *config.Config
	DB      *db.DB
	Log     *slog.Logger
	Tools   *ffmpeg.Tools
	Store   *library.Store
	Scanner *library.Scanner
	Auth    *auth.Service
	HLS     *hls.Manager
	DVD     *dvdsession.Manager
	BD      *bdsession.Manager
	Themes  *themes.Manager
	Web     fs.FS
	Version string
	Logs    *logging.Logger
	Update  *update.Updater // nil: this build can't update itself

	mux        *http.ServeMux
	logins     loginLimiter
	clientLogs clientLogLimiter
	probes     struct {
		sync.Mutex
		m map[string]*ffmpeg.Probe
	}
}

type ctxKey int

const userKey ctxKey = 1

func userFrom(r *http.Request) *auth.User {
	u, _ := r.Context().Value(userKey).(*auth.User)
	return u
}

// Handler builds the root handler.
func (s *Server) Handler() http.Handler {
	s.mux = http.NewServeMux()
	m := s.mux

	// Public
	m.HandleFunc("GET /api/status", s.handleStatus)
	m.HandleFunc("POST /api/setup", s.handleSetup)
	m.HandleFunc("POST /api/login", s.handleLogin)
	m.HandleFunc("POST /api/logout", s.handleLogout)
	m.HandleFunc("GET /api/profiles", s.handleProfiles)
	m.HandleFunc("GET /api/themes", s.handleThemes)
	m.HandleFunc("GET /api/themes/{id}/download", s.handleThemeDownload)
	m.HandleFunc("GET /api/themes/{id}/{file...}", s.handleThemeFile)
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]any{"ok": true}) })

	// Authenticated
	a := s.requireUser
	m.Handle("GET /api/me", a(s.handleMe))
	m.Handle("GET /api/me/ui", a(s.handleGetUI))
	m.Handle("PUT /api/me/ui", a(s.handlePutUI))
	m.Handle("GET /api/home", a(s.handleHome))
	m.Handle("GET /api/libraries", a(s.handleLibraries))
	m.Handle("POST /api/libraries", s.requireAdmin(s.handleCreateLibrary))
	m.Handle("PUT /api/libraries/{id}", s.requireAdmin(s.handleUpdateLibrary))
	m.Handle("DELETE /api/libraries/{id}", s.requireAdmin(s.handleDeleteLibrary))
	m.Handle("POST /api/libraries/{id}/scan", s.requireAdmin(s.handleScan))
	m.Handle("GET /api/libraries/{id}/scan", a(s.handleScanProgress))
	m.Handle("GET /api/genres", a(s.handleGenres))
	m.Handle("GET /api/items", a(s.handleItems))
	m.Handle("GET /api/items/{id}", a(s.handleItem))
	m.Handle("PUT /api/items/{id}", s.requireAdmin(s.handleUpdateItem))
	m.Handle("GET /api/items/{id}/poster", a(s.handlePoster))
	m.Handle("GET /api/items/{id}/artwork", s.requireAdmin(s.handleArtwork))
	m.Handle("GET /api/items/{id}/recommend", a(s.handleRecommend))
	m.Handle("GET /api/items/{id}/disc", a(s.handleDisc))
	m.Handle("PUT /api/items/{id}/disc/labels", s.requireAdmin(s.handleDiscLabels))
	m.Handle("GET /api/items/{id}/disc/thumb/{ttn}/{n}", a(s.handleDiscThumb))
	m.Handle("GET /api/artwork/preview", s.requireAdmin(s.handleArtPreview))
	m.Handle("GET /api/prefs", a(s.handlePrefs))
	m.Handle("POST /api/themes", s.requireAdmin(s.handleThemeUpload))
	m.Handle("DELETE /api/themes/{id}", s.requireAdmin(s.handleThemeDelete))
	m.Handle("GET /api/shows", a(s.handleShows))
	m.Handle("GET /api/shows/genres", a(s.handleShowGenres))
	m.Handle("GET /api/shows/networks", a(s.handleNetworks))
	m.Handle("GET /api/shows/{id}", a(s.handleShow))
	m.Handle("PUT /api/shows/{id}", s.requireAdmin(s.handleUpdateShow))
	m.Handle("PUT /api/shows/{id}/view", s.requireAdmin(s.handleShowView))
	m.Handle("GET /api/shows/{id}/artwork", s.requireAdmin(s.handleArtwork))
	m.Handle("GET /api/shows/{id}/art/{kind}", a(s.handleShowArt))
	m.Handle("GET /api/items/{id}/backdrop", a(s.handleBackdrop))
	m.Handle("POST /api/items/{id}/progress", a(s.handleProgress))
	m.Handle("DELETE /api/items/{id}/progress", a(s.handleClearProgress))
	m.Handle("GET /api/items/{id}/play", a(s.handlePlayInfo))
	m.Handle("GET /api/items/{id}/file", a(s.handleFile))
	m.Handle("GET /api/items/{id}/stream", a(s.handleAudioStream))
	m.Handle("GET /api/items/{id}/book", a(s.handleBookInfo))
	m.Handle("GET /api/items/{id}/page/{n}", a(s.handleComicPage))
	m.Handle("GET /api/items/{id}/epub/{file...}", a(s.handleEPUBFile))
	m.Handle("GET /api/items/{id}/annotations", a(s.handleAnnotations))
	m.Handle("POST /api/items/{id}/annotations", a(s.handleAddAnnotation))
	m.Handle("PUT /api/annotations/{aid}", a(s.handleUpdateAnnotation))
	m.Handle("DELETE /api/annotations/{aid}", a(s.handleDeleteAnnotation))
	m.Handle("GET /api/items/{id}/search", a(s.handleBookSearch))
	m.Handle("GET /api/dictionary", a(s.handleDictionary))
	m.Handle("GET /api/music/albums", a(s.handleAlbums))
	m.Handle("GET /api/music/artists", a(s.handleArtists))
	m.Handle("GET /api/music/tracks", a(s.handleTracks))
	m.Handle("GET /api/podcasts", a(s.handlePodcasts))
	m.Handle("GET /api/podcasts/search", s.requireAdmin(s.handlePodcastSearch))
	m.Handle("POST /api/podcasts", s.requireAdmin(s.handleSubscribe))
	m.Handle("PUT /api/podcasts/{id}", s.requireAdmin(s.handleUpdatePodcast))
	m.Handle("POST /api/podcasts/{id}/refresh", s.requireAdmin(s.handleRefreshPodcast))
	m.Handle("DELETE /api/podcasts/{id}", s.requireAdmin(s.handleUnsubscribe))
	m.Handle("POST /api/items/{id}/download", s.requireAdmin(s.handleDownloadEpisode))
	m.Handle("POST /api/items/{id}/dvd", a(s.handleDVDOpen))
	m.Handle("GET /api/dvd/{sid}", a(s.handleDVDGet))
	m.Handle("POST /api/dvd/{sid}/action", a(s.handleDVDAction))
	m.Handle("DELETE /api/dvd/{sid}", a(s.handleDVDClose))
	m.Handle("GET /api/dvd/{sid}/menu/{serial}/overlay.png", a(s.handleDVDOverlay))
	m.Handle("GET /api/dvd/{sid}/menu/{serial}/highlight/{n}", a(s.handleDVDHighlight))
	m.Handle("GET /api/dvd/{sid}/still/{name}", a(s.handleDVDStill))
	m.Handle("GET /api/dvd/{sid}/subs", a(s.handleDVDSubs))
	m.Handle("GET /api/dvd/{sid}/sub/{name}", a(s.handleDVDSub))
	m.Handle("GET /api/hls/{id}/playlist.m3u8", a(s.handleHLSPlaylist))
	m.Handle("GET /api/hls/{id}/seg/{n}", a(s.handleHLSSegment))
	m.Handle("DELETE /api/hls/{id}", a(s.handleHLSStop))
	m.Handle("GET /api/settings", s.requireAdmin(s.handleGetSettings))
	m.Handle("PUT /api/settings", s.requireAdmin(s.handlePutSettings))
	m.Handle("GET /api/about", a(s.handleAbout))
	m.Handle("GET /api/branding/logo", http.HandlerFunc(s.handleLogo))
	m.Handle("PUT /api/branding/logo", s.requireAdmin(s.handleSetLogo))
	m.Handle("DELETE /api/branding/logo", s.requireAdmin(s.handleDeleteLogo))
	m.Handle("GET /api/system", s.requireAdmin(s.handleSystem))
	m.Handle("GET /api/users", a(s.handleUsers))       // filtered by who may manage whom
	m.Handle("POST /api/users", a(s.handleCreateUser)) // adults may add Kids profiles
	m.Handle("DELETE /api/users/{id}", a(s.handleDeleteUser))
	m.Handle("PUT /api/users/{id}/access", a(s.handleUserAccess))
	m.Handle("PUT /api/users/{id}/password", a(s.handlePassword))
	m.Handle("GET /api/fs", s.requireAdmin(s.handleFS))
	m.Handle("POST /api/fs/pick", s.requireAdmin(s.handlePickFolder))
	m.Handle("POST /api/metadata/test", s.requireAdmin(s.handleMetadataTest))
	m.Handle("GET /api/agents", s.requireAdmin(s.handleAgents))
	m.Handle("PUT /api/agents", s.requireAdmin(s.handlePutAgents))
	m.Handle("POST /api/agents/test", s.requireAdmin(s.handleTestAgent))
	m.Handle("GET /api/logs", s.requireAdmin(s.handleLogs))
	m.Handle("PUT /api/logs", s.requireAdmin(s.handlePutLogs))
	m.Handle("GET /api/logs/files/{name}", s.requireAdmin(s.handleLogFile))
	m.Handle("POST /api/logs/open", s.requireAdmin(s.handleOpenLogs))
	m.Handle("POST /api/logs/client", a(s.handleClientLog))
	m.Handle("GET /api/update", s.requireAdmin(s.handleUpdate))
	m.Handle("POST /api/update/check", s.requireAdmin(s.handleUpdateCheck))
	m.Handle("POST /api/update/apply", s.requireAdmin(s.handleUpdateApply))

	// Web UI (SPA)
	m.Handle("/", s.spa())

	return s.logging(s.headers(s.cors(s.csrf(m))))
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only measured when the owner turned on request logging.
		if s.Logs == nil || !s.Logs.On("http") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: 200}
		next.ServeHTTP(sw, r)
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		ip := ""
		if p := clientIP(r); p != nil {
			ip = p.String()
		}
		s.Log.Info("http", "cat", "http", "from", ip, "m", r.Method, "p", r.URL.Path, "q", redactQuery(r.URL.RawQuery), "s", sw.code, "ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int) { w.code = c; w.ResponseWriter.WriteHeader(c) }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// redactQuery hides session tokens (media URLs carry one) so logs, which
// people attach to bug reports, never hold a working login.
func redactQuery(q string) string {
	// "%" too: %74oken=... decodes to the same parameter when signing in.
	if !strings.Contains(q, "token") && !strings.Contains(q, "%") {
		return q
	}
	v, err := url.ParseQuery(q)
	if err != nil {
		return "(unreadable)"
	}
	if v.Has("token") {
		v.Set("token", "REDACTED")
	}
	return v.Encode()
}

// headers sets the browser protections every response gets: no MIME
// sniffing, no framing by other sites (clickjacking), no full URLs in
// Referer headers, and for the app itself a policy that only runs its own
// scripts.
func (s *Server) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Content-Security-Policy", apiCSP)
		} else {
			h.Set("Content-Security-Policy", appCSP)
		}
		next.ServeHTTP(w, r)
	})
}

// appCSP: the web UI's own scripts only; artwork and podcast images may come
// from the internet, media and workers from blobs (HLS, PDF).
const appCSP = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; " +
	"media-src 'self' blob: https:; connect-src 'self' https:; worker-src 'self' blob:; font-src 'self' data:; frame-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'self'"

// apiCSP: nothing the API answers may run as a page on this site, whatever a
// file or a remote server claims it is. Answers that have to be shown as a
// document (book chapters, pictures, theme files) set their own policy.
const apiCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; media-src 'self'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'"

// CORS: other apps (the mobile app, tools) may call the API with a bearer
// token. Cookies are never honoured across origins, so a page elsewhere on
// the same host or network cannot act as whoever is signed in here.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" {
			w.Header().Set("Access-Control-Allow-Origin", o)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-WMS-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// csrf refuses state-changing requests signed in by cookie that come from
// another origin (a form or script on another page). Requests with the token
// in a header cannot be forged that way, so apps and the mobile client pass.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") == "" && r.Header.Get("X-WMS-Token") == "" && crossOrigin(r) {
			writeErr(w, 403, "cross-site request refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// crossOrigin: the browser says the request comes from another origin.
func crossOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return false
	case "same-site", "cross-site":
		return true
	}
	o := r.Header.Get("Origin")
	if o == "" || o == "null" {
		return o == "null"
	}
	u, err := url.Parse(o)
	if err != nil {
		return true
	}
	return !strings.EqualFold(u.Host, r.Host) && !strings.EqualFold(u.Host, r.Header.Get("X-Forwarded-Host"))
}

func (s *Server) resolveUser(r *http.Request) *auth.User {
	// The first token that belongs to a session here wins (a stale cookie
	// must not hide a valid token).
	for i, tok := range auth.Tokens(r) {
		if i >= 4 {
			break
		}
		if u, err := s.Auth.Resolve(r.Context(), tok); err == nil {
			return u
		}
	}
	return nil
}

func (s *Server) requireUser(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := s.resolveUser(r)
		if u == nil {
			writeErr(w, 401, "authentication required")
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func (s *Server) requireAdmin(h http.HandlerFunc) http.Handler {
	return s.requireUser(func(w http.ResponseWriter, r *http.Request) {
		if !userFrom(r).IsAdmin {
			writeErr(w, 403, "admin only")
			return
		}
		h(w, r)
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func pathInt(r *http.Request, name string) (int64, bool) {
	n, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return n, err == nil
}

func qInt(r *http.Request, name string, def int) int {
	if v := r.URL.Query().Get(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func qBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}
