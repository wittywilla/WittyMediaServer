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
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/identity"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/portmap"
	"github.com/wittywilla/wittymediaserver/internal/remote"
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
	Modules *modmgr.Manager // nil: modules can't be updated here
	// Discovery announces the server on the home network; nil when it can't
	// be reached from other devices.
	Discovery interface{ Refresh() }
	HTTPS     *remote.HTTPS   // the HTTPS port; nil until StartRemote
	Mapper    *portmap.Keeper // automatic port mapping; nil until StartRemote
	// Channels and Live TV (see api_channels.go, api_tuners.go).
	Channels *channels.Store
	TV       *livetv.Manager
	silent   sync.Map // tuner channels whose sound can't be decoded here

	mux        *http.ServeMux
	logins     loginLimiter
	loginSlots chan struct{} // password checks are slow on purpose: only so many at once
	clientLogs clientLogLimiter
	probes     struct {
		sync.Mutex
		m map[string]*ffmpeg.Probe
	}
	stream    streamTokens
	pair      pairings
	subtitles subtitleFiles
	idOnce    sync.Once
	id        *identity.Identity
	idErr     error
	netState  atomic.Pointer[netState]
	applyMu   sync.Mutex // one change of the remote access settings at a time
	reach     reachTest
	outside   outsideSeen
	routerMu  sync.Mutex
	router    *portmap.Probe // the latest "check my router"

	setupCode     string
	setupCodeOnce sync.Once
	audioSlots    chan struct{} // audio transcodes running at once
	gameOnce      sync.Once
	gameSvc       *gameService
}

type ctxKey int

const (
	userKey ctxKey = iota + 1
	tokenKey
	clientKey
)

func userFrom(r *http.Request) *auth.User {
	u, _ := r.Context().Value(userKey).(*auth.User)
	return u
}

// tokenFrom is the token this request was signed in with.
func tokenFrom(r *http.Request) string {
	t, _ := r.Context().Value(tokenKey).(string)
	return t
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
	m.HandleFunc("GET /api/capabilities", s.handleCapabilities)
	m.HandleFunc("POST /api/identity", s.handleIdentity)
	m.HandleFunc("GET /api/themes/{id}/tokens.json", s.handleThemeTokens)
	m.HandleFunc("POST /api/pair/start", s.handlePairStart)
	m.HandleFunc("POST /api/pair/poll", s.handlePairPoll)
	m.HandleFunc("POST /api/pair/redeem", s.handlePairRedeem)

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
	m.Handle("POST /api/pair/info", a(s.handlePairInfo))
	m.Handle("POST /api/pair/approve", a(s.handlePairApprove))
	m.Handle("POST /api/pair/deny", a(s.handlePairDeny))
	m.Handle("POST /api/pair/invite", a(s.handlePairInvite))
	m.Handle("GET /api/playback/profiles", a(s.handlePlaybackProfiles))
	m.Handle("POST /api/items/{id}/playback", a(s.handlePlayback))
	m.Handle("POST /api/items/{id}/stream-token", a(s.handleItemStreamToken))
	m.Handle("GET /api/items/{id}/subtitles/{file}", a(s.handleItemSubtitle))
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
	m.Handle("GET /api/dvd/{sid}/menu/{serial}/effect/{n}", a(s.handleDVDEffect))
	m.Handle("GET /api/dvd/{sid}/sound/{n}", a(s.handleDVDSound))
	m.Handle("GET /api/dvd/{sid}/bdj", a(s.handleBDJPoll))
	m.Handle("GET /api/dvd/{sid}/bdj/frame/{n}", a(s.handleBDJFrame))
	m.Handle("GET /api/bdj/status", s.requireAdmin(s.handleBDJStatus))
	m.Handle("GET /api/dvd/{sid}/still/{name}", a(s.handleDVDStill))
	m.Handle("GET /api/dvd/{sid}/subs", a(s.handleDVDSubs))
	m.Handle("GET /api/dvd/{sid}/sub/{name}", a(s.handleDVDSub))
	m.Handle("GET /api/hls/{id}/master.m3u8", a(s.handleHLSMaster))
	m.Handle("GET /api/hls/{id}/subs/{file}", a(s.handleHLSSubtitle))
	m.Handle("GET /api/hls/{id}/playlist.m3u8", a(s.handleHLSPlaylist))
	m.Handle("GET /api/hls/{id}/seg/{n}", a(s.handleHLSSegment))
	m.Handle("DELETE /api/hls/{id}", a(s.handleHLSStop))
	m.Handle("GET /api/settings", s.requireAdmin(s.handleGetSettings))
	m.Handle("PUT /api/settings", s.requireAdmin(s.handlePutSettings))
	m.Handle("GET /api/about", a(s.handleAbout))
	m.Handle("GET /api/licenses", a(s.handleLicenses))
	m.Handle("GET /api/licenses/{id}", a(s.handleLicenseText))
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
	m.Handle("GET /api/modules", s.requireAdmin(s.handleModules))
	m.Handle("POST /api/modules/check", s.requireAdmin(s.handleModulesCheck))
	m.Handle("POST /api/modules/{id}/install", s.requireAdmin(s.handleModuleInstall))
	m.Handle("POST /api/modules/{id}/rollback", s.requireAdmin(s.handleModuleRollback))
	s.remoteRoutes(m)

	// WittyPhotos
	m.Handle("GET /api/photos/months", a(s.handlePhotoMonths))
	m.Handle("GET /api/photos/list", a(s.handlePhotoList))
	m.Handle("GET /api/photos/folders", a(s.handlePhotoFolders))
	m.Handle("GET /api/photos/map", a(s.handlePhotoMap))
	m.Handle("GET /api/photos/ai", a(s.handlePhotoAI))
	m.Handle("GET /api/photos/albums", a(s.handlePhotoAlbums))
	m.Handle("POST /api/photos/albums", a(s.handleCreateAlbum))
	m.Handle("PUT /api/photos/albums/{id}", a(s.handleUpdateAlbum))
	m.Handle("DELETE /api/photos/albums/{id}", a(s.handleDeleteAlbum))
	m.Handle("POST /api/photos/albums/{id}/items", a(s.handleAlbumAdd))
	m.Handle("DELETE /api/photos/albums/{id}/items", a(s.handleAlbumRemove))
	m.Handle("GET /api/photos/{id}", a(s.handlePhotoDetail))
	m.Handle("GET /api/items/{id}/image", a(s.handleItemImage))
	m.Handle("GET /api/items/{id}/original", a(s.handleOriginal))
	m.Handle("GET /api/items/{id}/motion", a(s.handleMotion))
	m.Handle("POST /api/libraries/{id}/upload", a(s.handleUpload))

	// WittyShorts and WittyChannels
	m.Handle("GET /api/creators", a(s.handleCreators))
	m.Handle("GET /api/creators/{id}", a(s.handleCreator))
	m.Handle("PUT /api/creators/{id}", s.requireAdmin(s.handleUpdateCreator))
	m.Handle("GET /api/creators/{id}/art/{kind}", a(s.handleCreatorArt))
	m.Handle("PUT /api/creators/{id}/subscribe", a(s.handleSubscribe2))
	m.Handle("GET /api/videos", a(s.handleVideos))
	m.Handle("GET /api/videos/feed", a(s.handleVideoFeed))
	m.Handle("GET /api/videos/tags", a(s.handleVideoTags))
	m.Handle("GET /api/videos/{id}", a(s.handleVideo))
	m.Handle("PUT /api/videos/{id}", s.requireAdmin(s.handleEditVideo))
	m.Handle("GET /api/videos/{id}/related", a(s.handleVideoRelated))

	// On any item: likes, comments and views
	m.Handle("PUT /api/items/{id}/reaction", a(s.handleReaction))
	m.Handle("POST /api/items/{id}/view", a(s.handleView))
	m.Handle("GET /api/items/{id}/comments", a(s.handleComments))
	m.Handle("POST /api/items/{id}/comments", a(s.handleAddComment))
	m.Handle("PUT /api/comments/{cid}", a(s.handleEditComment))
	m.Handle("DELETE /api/comments/{cid}", a(s.handleDeleteComment))

	// Collections and smart playlists
	m.Handle("GET /api/collections", a(s.handleCollections))
	m.Handle("POST /api/collections", a(s.handleCreateCollection))
	m.Handle("POST /api/collections/preview", a(s.handlePreviewRule))
	m.Handle("GET /api/collections/holding", a(s.handleCollectionsHolding))
	m.Handle("POST /api/collections/watch-later/items", a(s.handleWatchLaterAdd))
	m.Handle("GET /api/collections/{id}", a(s.handleCollection))
	m.Handle("PUT /api/collections/{id}", a(s.handleUpdateCollection))
	m.Handle("DELETE /api/collections/{id}", a(s.handleDeleteCollection))
	m.Handle("POST /api/collections/{id}/items", a(s.handleCollectionAdd))
	m.Handle("DELETE /api/collections/{id}/items", a(s.handleCollectionRemove))

	// Internet radio
	m.Handle("GET /api/radio", a(s.handleRadioList))
	m.Handle("POST /api/radio", s.requireAdmin(s.handleRadioAdd))
	m.Handle("POST /api/radio/import", s.requireAdmin(s.handleRadioImport))
	m.Handle("DELETE /api/radio/{id}", s.requireAdmin(s.handleRadioRemove))
	m.Handle("GET /api/radio/{id}/stream", a(s.handleRadioStream))

	// Year in review and recommendations, from this account's own record
	m.Handle("GET /api/me/review", a(s.handleReview))
	m.Handle("DELETE /api/me/history", a(s.handleClearHistory))
	m.Handle("GET /api/me/recommended", a(s.handleRecommended))

	// Links that share with someone without an account
	m.Handle("GET /api/shares", a(s.handleShares))
	m.Handle("POST /api/shares", a(s.handleCreateShare))
	m.Handle("DELETE /api/shares/{id}", a(s.handleDeleteShare))
	m.HandleFunc("GET /api/share/{token}", s.handleShareOpen)
	m.HandleFunc("GET /api/share/{token}/image/{id}", s.handleShareImage)
	m.HandleFunc("GET /api/share/{token}/original/{id}", s.handleShareOriginal)
	m.HandleFunc("GET /api/share/{token}/play/{id}", s.handleSharePlay)
	m.HandleFunc("GET /api/share/{token}/file/{id}", s.handleShareFile)
	m.HandleFunc("GET /api/share/{token}/hls/{id}/playlist.m3u8", s.handleShareHLSPlaylist)
	m.HandleFunc("GET /api/share/{token}/hls/{id}/seg/{n}", s.handleShareHLSSegment)

	s.channelRoutes(m)
	s.gameRoutes(m)

	// Web UI (SPA)
	m.Handle("/", s.spa())

	s.audioSlots = make(chan struct{}, 2*max(s.Cfg.MaxTranscode, 1))
	s.loginSlots = make(chan struct{}, loginConcurrency())
	s.reloadNet(context.Background())
	return s.identify(s.limits(s.logging(s.headers(s.gate(s.versioned(s.cors(s.csrf(m))))))))
}

// ---------------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------------

// Limits on what a client can make the server hold on to while it sends a
// request: uploads (a theme, a logo) are the largest bodies there are.
const (
	maxRequestBody  = 32 << 20
	requestBodyTime = 2 * time.Minute
)

func (s *Server) limits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
			_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(requestBodyTime))
		}
		next.ServeHTTP(w, r)
	})
}

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
		s.Log.Info("http", "cat", "http", "from", ip, "m", r.Method, "p", redactPath(r.URL.Path), "q", redactQuery(r.URL.RawQuery), "s", sw.code, "ms", time.Since(start).Milliseconds())
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(c int)           { w.code = c; w.ResponseWriter.WriteHeader(c) }
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// redactQuery hides session tokens (media URLs carry one) so logs, which
// people attach to bug reports, never hold a working login.
func redactQuery(q string) string {
	// "%" too: %74oken=... decodes to the same parameter when signing in.
	if !strings.Contains(q, "token") && !strings.Contains(q, "st=") && !strings.Contains(q, "%") {
		return q
	}
	v, err := url.ParseQuery(q)
	if err != nil {
		return "(unreadable)"
	}
	for _, k := range []string{"token", "st"} {
		if v.Has(k) {
			v.Set(k, "REDACTED")
		}
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
			// Nothing the API sends is a page: whatever a file, feed or stream
			// turns out to hold, opening its address can't run it on this site.
			// Handlers with their own needs replace this.
			h.Set("Content-Security-Policy", apiCSP)
		} else {
			h.Set("Content-Security-Policy", appCSP)
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
		}
		if r.TLS != nil && r.TLS.ServerName != "" {
			h.Set("Strict-Transport-Security", hsts)
		}
		next.ServeHTTP(w, r)
	})
}

// hsts: a browser that used HTTPS here keeps using it. Only a week: an owner who
// turns HTTPS off again should not be locked out of their own server for long.
const hsts = "max-age=604800"

// appCSP: the web UI's own scripts only; artwork and podcast images may come
// from the internet, media and workers from blobs (HLS, PDF).
const appCSP = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; " +
	"media-src 'self' blob:; connect-src 'self'; worker-src 'self' blob:; font-src 'self' data:; frame-src 'self'; " +
	"object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'self'"

// apiCSP: a sandbox with no scripts, for everything under /api/.
const apiCSP = "default-src 'none'; frame-ancestors 'self'; sandbox"

// CORS: other apps (the mobile app, tools) may call the API with a bearer
// token, and the mobile app's own origins may also sign in. Cookies are never
// honoured across origins, and a web page elsewhere that carries no token gets
// no answer it can read, so it can't list the profiles of a server it finds on
// the network of whoever is looking at it.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && corsAllowed(r, o) {
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

// appOrigins are where the mobile app's pages come from. They are only ever
// the app itself: nothing on the internet can have these origins.
var appOrigins = map[string]bool{"capacitor://localhost": true, "ionic://localhost": true, "https://localhost": true}

func corsAllowed(r *http.Request, origin string) bool {
	if appOrigins[origin] || r.Header.Get("Authorization") != "" || r.Header.Get("X-WMS-Token") != "" {
		return true
	}
	if r.Method == http.MethodOptions {
		h := strings.ToLower(r.Header.Get("Access-Control-Request-Headers"))
		return strings.Contains(h, "authorization") || strings.Contains(h, "x-wms-token")
	}
	return false
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
		if r.Header.Get("Authorization") == "" && r.Header.Get("X-WMS-Token") == "" && !appOrigins[r.Header.Get("Origin")] && s.crossOrigin(r) {
			writeErr(w, 403, "cross-site request refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// crossOrigin: the browser says the request comes from another origin. The
// page's own address is what the browser asked for, what a trusted proxy says
// it asked for, or the server's outside address.
func (s *Server) crossOrigin(r *http.Request) bool {
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
	if strings.EqualFold(u.Host, r.Host) || strings.EqualFold(u.Host, clientFrom(r).ForwardedHost) {
		return false
	}
	if st := s.netState.Load(); st != nil && st.publicHost != "" && strings.EqualFold(u.Host, st.publicHost) {
		return false
	}
	return true
}

func (s *Server) resolveUser(r *http.Request) *auth.User {
	u, _ := s.signedIn(r)
	return u
}

// signedIn finds the account and the token that signed this request in.
func (s *Server) signedIn(r *http.Request) (*auth.User, string) {
	// The first token that belongs to a session here wins (a stale cookie
	// must not hide a valid token).
	for i, tok := range auth.Tokens(r) {
		if i >= 4 {
			break
		}
		var u *auth.User
		var err error
		if auth.IsStreamToken(tok) {
			u, err = s.Auth.ResolveStream(r.Context(), tok, r)
		} else {
			u, err = s.Auth.Resolve(r.Context(), tok)
		}
		if err == nil {
			return u, tok
		}
	}
	return nil, ""
}

func (s *Server) requireUser(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, tok := s.signedIn(r)
		if u == nil {
			u = s.resolveStream(r) // media addresses only; see stream_tokens.go
		}
		if u == nil {
			writeErr(w, 401, "authentication required")
			return
		}
		s.touchSession(r, tok)
		ctx := context.WithValue(context.WithValue(r.Context(), userKey, u), tokenKey, tok)
		h(w, r.WithContext(ctx))
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
