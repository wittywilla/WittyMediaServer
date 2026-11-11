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
	apiRevision = 14
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
	"canvas", "karaoke-stems", "skip-markers", "where-to-watch-xref", "module-identity",
	"sdui-data", "sdui-form-factors", "sdui-topshelf", "pair-admin-confirm", "pair-session-info", "theme-variants",
	"canvas-generate", "stems-approximate", "stems-separate", "intro-detection",
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
