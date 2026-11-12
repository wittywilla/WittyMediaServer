// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// The owner's karaoke switch decides who may ask for an approximate instrumental, as it
// does for karaoke's own requests: anyone with "auto", administrators alone with "admin"
// or "never". This server has no FFmpeg, so a request that is let through answers 503.
func TestApproximateFollowsTheKaraokeSwitch(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	song := f.url("/api/media/%d/stems", f.song)
	cases := []struct {
		mode, who string
		code      int
		reason    string // what GET /stems says of the approximate way; "" when available
	}{
		{"auto", "adult", 503, "no-ffmpeg"},
		{"auto", "admin", 503, "no-ffmpeg"},
		{"admin", "adult", 403, "admin-only"},
		{"admin", "admin", 503, "no-ffmpeg"},
		{"never", "adult", 403, "off"},
		{"never", "admin", 503, "no-ffmpeg"},
	}
	for _, tc := range cases {
		t.Run(tc.mode+"/"+tc.who, func(t *testing.T) {
			if w := ts.do(t, "PUT", "/api/settings", f.admin, map[string]any{"karaokeAuto": tc.mode}); w.Code != 200 {
				t.Fatalf("setting: %d %s", w.Code, w.Body)
			}
			tok := map[string]string{"adult": f.adult, "admin": f.admin}[tc.who]
			w := ts.do(t, "POST", song+"/approximate", tok, nil)
			if w.Code != tc.code {
				t.Errorf("approximate: %d %s", w.Code, w.Body)
			}
			if tc.code == 403 && decode[map[string]any](t, w)["reason"] != tc.reason {
				t.Errorf("the refusal's reason: %s", w.Body)
			}
			got := decode[stemsAnswer](t, ts.do(t, "GET", song, tok, nil))
			if got.Separation.Approximate.Available || got.Separation.Approximate.Reason != tc.reason {
				t.Errorf("the listing: %+v", got.Separation.Approximate)
			}
			if ts.mediaJobTask(f.url("stems-approx:%d", f.song)) != "" {
				t.Error("something was queued")
			}
		})
	}
	// With "auto", an account's asking is bounded per hour like karaoke's own.
	if w := ts.do(t, "PUT", "/api/settings", f.admin, map[string]any{"karaokeAuto": "auto"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	ts.Tools = &ffmpeg.Tools{FFmpeg: "/nonexistent/ffmpeg"}
	adultID := ts.userID(t, f.adult)
	for ts.karaoke.allow("approx", adultID, karaokeApproxPerHour) {
	}
	if w := ts.do(t, "POST", song+"/approximate", f.adult, nil); w.Code != 429 || decode[map[string]any](t, w)["reason"] != "rate-limited" {
		t.Errorf("over the hourly limit: %d %s", w.Code, w.Body)
	}
	if ts.mediaJobTask(f.url("stems-approx:%d", f.song)) != "" {
		t.Error("something was queued over the limit")
	}
}
