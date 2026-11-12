// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/discovery"
	"github.com/wittywilla/wittymediaserver/internal/identity"
	"github.com/wittywilla/wittymediaserver/internal/playback"
	"github.com/wittywilla/wittymediaserver/internal/remote"
)

// The API is versioned by its path: /api/v1/... is the contract clients are
// written against (docs/CLIENT-API.md, docs/openapi.json), and /api/... is the
// same thing without the promise, used by the web UI that ships with each
// server. Within v1 changes only add things; removing or changing anything
// means v2, with v1 kept going alongside it.

const (
	apiVersion = 1
	// apiRevision counts additions within the version: a client that needs
	// something new can check for it, or look for a feature name below.
	//
	// 15 is everything 0.8.0-alpha adds over 0.7 (revision 10), counted once: server-driven
	// pages with their data, form factors, diffs and Top Shelf, the catalogue, navigation,
	// each profile's own arrangement and the editor's routes (sdui-pages, sdui-catalog,
	// sdui-navigation, sdui-personal, sdui-editor); the theme engine; secure
	// pairing; canvas, stems, skip markers, intro detection and where-to-watch; WittyRemote
	// and watch parties; profile switching, parental controls and screen time; standalone
	// emulators. And what makes them one thing:
	//   - the profile switches "syncplay" and "remoteControl" in limits and in /me/limits,
	//     with canJoin in GET /syncplay and canControl in GET /remote-control
	//     (parental-feature-switches);
	//   - server-driven pages leave out what a profile is kept from, set the flag
	//     screenTimeUp, and with screen time over offer nothing to play and say so in the
	//     document's screenTime; the player is the app's own, and its routes refuse with
	//     code "screen-time" (sdui-limits);
	//   - remote control refuses with "screen-time" and "screen-time-there"; a watch
	//     party's view has timeUp and the member state "time-up";
	//   - the streams of remote control and watch parties end the moment their sign-in
	//     does, with "bye" or "closed" and a reason: "signed-out" or "feature-off"
	//     (session-end-streams).
	apiRevision = 15
	apiPrefix   = "/api/v1/"
)

var otherAPIVersion = regexp.MustCompile(`^/api/v[0-9]+(/|$)`)

// versioned serves /api/v1/... as /api/..., and answers plainly for versions
// this server doesn't have.
func (s *Server) versioned(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("X-WMS-API-Version", "1")
		if strings.HasPrefix(p, apiPrefix) {
			u := *r.URL
			u.Path = "/api/" + strings.TrimPrefix(p, apiPrefix)
			if u.RawPath != "" {
				u.RawPath = "/api/" + strings.TrimPrefix(u.RawPath, apiPrefix)
			}
			r2 := *r
			r2.URL = &u
			next.ServeHTTP(w, &r2)
			return
		}
		if otherAPIVersion.MatchString(p) {
			writeJSON(w, 404, map[string]any{"error": "this server doesn't have that API version", "apiVersions": []int{apiVersion}})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// serverFeatures names what a client can rely on being here, so an app can
// hide what an older server lacks without knowing its version numbers. A new
// area adds its name here along with its routes.
var serverFeatures = []string{
	"profiles", "pairing", "pair-invite", "pair-totp", "pair-qr", "server-identity", "lan-discovery", "stream-tokens",
	"direct-play", "hls", "hls-remux", "hls-fmp4", "transcode", "playback-profiles", "subtitles-webvtt", "subtitles-burn-in",
	"dvd", "dvd-menus", "bluray", "bluray-menus", "audio-cd",
	"shows", "next-up", "recommendations", "skip-intro",
	"music", "audiobooks", "podcasts", "books", "annotations", "radio", "radio-directory",
	"themes", "theme-tokens", "modules", "game-metadata", "system-tasks", "system-metrics", "module-pages",
	"game-controller-profiles", "game-settings", "custom-game-cores", "game-rigs", "library-transcoding",
	"channels", "live-tv", "live-tv-stream-tokens", "tv-favourites", "tv-reminders", "recordings",
	"browse", "saved-views", "metadata-editing", "logos", "streaming-listings",
	"music-home", "music-radio", "lyrics", "min-client-versions", "robust-streaming",
	"sdui", "sdui-diff", "theme-engine", "theme-module-overrides",
	"sdui-pages", "sdui-catalog", "sdui-navigation", "sdui-personal", "sdui-editor",
	"canvas", "karaoke-stems", "skip-markers", "where-to-watch-xref", "module-identity",
	"remote-protocol", "syncplay",
	"sdui-data", "sdui-form-factors", "sdui-topshelf", "pair-admin-confirm", "pair-session-info", "theme-variants",
	"canvas-generate", "stems-approximate", "stems-separate", "intro-detection", "karaoke-auto",
	"profile-switching", "profile-avatars", "parental-controls", "screen-time",
	"standalone-emulators",
	// What 0.8.0's parts do together: see apiRevision 15 above.
	"parental-feature-switches", "sdui-limits", "session-end-streams",
	"p2p",
	// A device away from home shows a code, and someone signed in types it in: POST /api/v1/p2p/join.
	"p2p-join",
	// Modules carry their own SDUI (docs/SDUI-MODULES.md): their pages, rows, menu entries,
	// sources and colours are merged in; their pictures and settings have routes of their own.
	"sdui-modules", "module-settings",
	// Layout options: document.layout.shell, the navigation's shell and layouts, /sdui/layout.
	"sdui-layouts",
}

// minClientVersions names the oldest version of each app this server still works with, by
// the kind of app. An app older than this says it needs updating instead of failing in odd
// ways. Only raised when the API itself leaves an app behind, which within v1 it shouldn't.
var minClientVersions = map[string]string{"android": "0.1.0", "apple": "0.1.0", "desktop": "0.1.0"}

// GET /api/capabilities (public): what this server is and can do, the first
// thing an app asks. Nothing here is private, apart from which version runs, which as in
// /api/status is nobody's business on the internet until they sign in.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	version := s.Version
	if clientFrom(r).Where == remote.Internet && s.resolveUser(r) == nil {
		version = ""
	}
	profiles := []string{}
	for _, p := range playback.Profiles() {
		profiles = append(profiles, p.ID)
	}
	id, err := s.Identity()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{
		"server":            "WittyMediaServer",
		"serverId":          id.ID,
		"serverName":        s.ServerName(ctx),
		"publicKey":         id.PublicKey(),
		"fingerprint":       id.Fingerprint(),
		"version":           version,
		"apiVersion":        apiVersion,
		"apiRevision":       apiRevision,
		"apiVersions":       []int{apiVersion},
		"apiBase":           strings.TrimSuffix(apiPrefix, "/"),
		"setupRequired":     s.Auth.NoAccounts(ctx),
		"features":          serverFeatures,
		"minClientVersions": minClientVersions,
		"auth": map[string]any{
			"password":         true,
			"pairing":          true,
			"streamTokenParam": "st",
			"tokenHeader":      "Authorization: Bearer",
		},
		"playback": map[string]any{
			"profiles": profiles,
			// The rules every client shares, so none has to guess them.
			"rules": map[string]any{
				"resumeMinSec":          playback.ResumeMinSec,
				"watchedFraction":       0.92,
				"watchNextBeforeEndSec": 30,
				"nextEpisodeCountdown":  8,
				"reportProgressSec":     10,
				"reportProgressMinSec":  5,
			},
			"directPlay": s.DB.GetSetting(ctx, "directPlay", "1") == "1",
			// remoteStreaming is the server's streaming mode for viewers away from home.
			// A device can choose its own with streaming in /playback.
			"remoteStreaming": s.DB.GetSetting(ctx, remoteStreamingKey, playback.StreamStandard),
			"streamingModes":  []string{playback.StreamStandard, playback.StreamRobust},
		},
		"libraryTypes": []string{"movies", "discs", "shows", "anime", "videos", "music", "audiobooks", "podcasts", "books", "radio", "games", "photos"},
		"itemKinds":    []string{"video", "dvd-iso", "dvd-folder", "bluray-iso", "bluray-folder", "cd-image", "audio", "book", "radio", "game", "photo"},
	})
}

// Identity is this server's ID and key, made the first time they're needed.
func (s *Server) Identity() (*identity.Identity, error) {
	s.idOnce.Do(func() { s.id, s.idErr = identity.Load(context.Background(), s.DB) })
	return s.id, s.idErr
}

// ServerName is what the server calls itself to other devices. Never the
// computer's own name, which is often a person's.
func (s *Server) ServerName(ctx context.Context) string {
	return firstNonEmpty(s.DB.GetSetting(ctx, "brandName", ""), s.DB.GetSetting(ctx, "serverName", ""), "WittyMediaServer")
}

// DiscoveryInfo is all that's announced on the home network. tls stays false
// until the server itself speaks HTTPS.
func (s *Server) DiscoveryInfo(port int) discovery.Info {
	info := discovery.Info{Name: s.ServerName(context.Background()), Version: s.Version, API: apiVersion, Port: port}
	if id, err := s.Identity(); err == nil {
		info.ID = id.ID
	}
	return info
}

// POST /api/identity {nonce} (public): proves this is the server an app pinned.
// The ID and public key are no secret, so only a signature over a fresh challenge
// tells it from a look-alike. An app does this before sending its token.
func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Nonce string `json:"nonce"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	if err := identity.CheckNonce(b.Nonce); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id, err := s.Identity()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"serverId": id.ID, "publicKey": id.PublicKey(), "fingerprint": id.Fingerprint(), "algorithm": "ed25519", "signature": id.Sign(b.Nonce)})
}

// GET /api/playback/profiles: the built-in client profiles, for apps and
// tools to read; POST /api/items/{id}/playback names one.
func (s *Server) handlePlaybackProfiles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"profiles": playback.Profiles()})
}
