// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"regexp"
	"slices"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/playback"
)

type streamingView struct {
	Prefs   streamPrefs     `json:"prefs"`
	Quality qualityView     `json:"quality"`
	Profile playback.Limits `json:"profile"`
}

// A profile saves its own defaults; an administrator's limits for it win over them and over
// what the player asks for, and the menu is told what it may offer.
func TestStreamQualityPrefsAndLimits(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	viewer, tok := ts.user(t, "viewer", "adult", false, "")

	w := ts.do(t, "PUT", "/api/me/streaming", tok, map[string]string{"home": "b4000-h720", "away": "nonsense"})
	if w.Code != 200 {
		t.Fatalf("saving defaults: %d %s", w.Code, w.Body)
	}
	got := decode[streamingView](t, ts.do(t, "GET", "/api/me/streaming", tok, nil))
	if got.Prefs.Home != "h720-b4000" || got.Prefs.Away != "" {
		t.Errorf("saved defaults: %+v", got.Prefs)
	}
	if got.Quality.Choice != "h720-b4000" || got.Quality.Away {
		t.Errorf("at home the home default applies: %+v", got.Quality)
	}
	// The player's own choice (the header) wins over the saved default.
	got = decode[streamingView](t, ts.do(t, "GET", "/api/me/streaming", tok, nil, qualityHeader, "original"))
	if got.Quality.Choice != "original" || len(got.Quality.Presets) != len(playback.Presets) {
		t.Errorf("Original from the player: %+v", got.Quality)
	}

	limits := fmt.Sprintf("/api/users/%d/stream-limits", viewer.ID)
	if w := ts.do(t, "PUT", limits, tok, map[string]int{"maxHeight": 4320}); w.Code != 403 {
		t.Errorf("a profile can't lift its own limits: %d", w.Code)
	}
	if w := ts.do(t, "PUT", limits, admin, map[string]int{"maxHeight": 480, "maxKbps": 1500}); w.Code != 200 {
		t.Fatalf("an administrator sets limits: %d %s", w.Code, w.Body)
	}
	got = decode[streamingView](t, ts.do(t, "GET", "/api/me/streaming", tok, nil, qualityHeader, "original"))
	if got.Quality.Choice != "h480-b1500" || got.Profile.MaxHeight != 480 {
		t.Errorf("Original is held to the limits: %+v", got)
	}
	var names []string
	for _, p := range got.Quality.Presets {
		names = append(names, p.Name)
		if p.Kbps > 1500 {
			t.Errorf("%s offered at %d kbps, over the 1500 allowed", p.Name, p.Kbps)
		}
	}
	if !slices.Equal(names, []string{"480p", "360p"}) {
		t.Errorf("only sizes within the limit are offered: %v", names)
	}
	// What streams are made to, for every kind of stream (files, discs, channels).
	r := httptest.NewRequest("POST", "/api/items/1/playback", nil)
	r.RemoteAddr = "127.0.0.1:50000"
	r.Header.Set(qualityHeader, "h1080-b8000")
	r = r.WithContext(context.WithValue(r.Context(), userKey, viewer))
	q, h := ts.quality(r)
	if cap := ffmpeg.ParseRate(q.CapRate); h != 480 || cap <= 0 || cap > 1500000 {
		t.Errorf("a 1080p choice under a 480p, 1.5 Mbps limit: height %d, cap %q", h, q.CapRate)
	}
	// Stream IDs name the choice only in characters they allow.
	if tag := ts.streamTag(r); !regexp.MustCompile(`^-q[0-9a-z-]+$`).MatchString(tag) {
		t.Errorf("stream tag %q", tag)
	}
}
