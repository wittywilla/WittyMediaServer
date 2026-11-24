// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
)

// A profile whose TV limit is 7 may watch broadcast and IPTV channels, and tries every way
// the API offers to see a programme rated above that: tuning in, the live playlist, a part of
// the stream by name, catch-up from the programme's start, catch-up from an allowed programme
// that runs on into it, and catch-up's own playlist and parts. Each is refused. What is
// allowed on the same channels isn't, and an account with no limits is never asked.
func TestLiveTVProgrammeRatingsCantBeGotRound(t *testing.T) {
	f, _, _ := programmeFixture(t)
	ts := f.ts
	now := time.Now().Truncate(time.Minute).Unix()
	tunerChannel(t, livetv.Channel{ID: 3, Number: "6", Name: "Late Night", EPGID: "late.test", Enabled: true, CatchupDays: 7}, "http://provider.invalid/3.ts")
	tunerChannel(t, livetv.Channel{ID: 4, Number: "7", Name: "No Guide Ratings", EPGID: "plain.test", Enabled: true}, "http://provider.invalid/4.ts")
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO epg_programmes(channel, start, stop, title, rating) VALUES('late.test', ?, ?, 'Morning Cartoons', 'TV-Y')`, []any{now - 7200, now - 3600}},
		{`INSERT INTO epg_programmes(channel, start, stop, title, rating) VALUES('late.test', ?, ?, 'Grim Thriller', 'TV-MA')`, []any{now - 3600, now + 3600}},
		{`INSERT INTO epg_programmes(channel, start, stop, title) VALUES('plain.test', ?, ?, 'Talk Show')`, []any{now - 600, now + 600}},
	} {
		if _, err := ts.DB.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	_, kid := ts.user(t, "tvkid", "kids", false, `{"maxAge":7,"liveTv":true,"libraryChannels":true,"tunerChannels":true}`)
	_, kidUnrated := ts.user(t, "tvkid2", "kids", false, `{"maxAge":7,"liveTv":true,"libraryChannels":true,"tunerChannels":true,"kinds":{"tv":{"maxAge":7,"allowUnrated":true}}}`)

	refusedForRating := func(code int, body []byte) bool {
		var b struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(body, &b)
		return code == 403 && b.Code == "programme-rating"
	}
	cases := []struct {
		name, token, path string
		refused           bool
	}{
		{"tune into a TV-MA programme", kid, "/api/tv/tun-3/tune", true},
		{"its live playlist directly", kid, "/api/tv/tun-3/live/playlist.m3u8", true},
		{"a part of its stream by name", kid, "/api/tv/tun-3/live/seg/seg-00001.ts", true},
		{"catch-up from its start", kid, fmt.Sprintf("/api/tv/tun-3/catchup?start=%d", now-3600), true},
		{"catch-up from a gap in the guide asked to run on into it", kid, fmt.Sprintf("/api/tv/tun-3/catchup?start=%d&end=%d", now-9000, now-60), true},
		{"catch-up's playlist", kid, fmt.Sprintf("/api/tv/tun-3/catchup/%d/playlist.m3u8", now-3600), true},
		{"catch-up's parts", kid, fmt.Sprintf("/api/tv/tun-3/catchup/%d/seg/seg-00001.ts", now-3600), true},
		{"an unrated programme, unrated not allowed", kid, "/api/tv/tun-4/tune", true},
		{"its playlist", kid, "/api/tv/tun-4/live/playlist.m3u8", true},
		// Allowed: these get past the rating, whatever the made-up provider then answers.
		{"a G programme", kid, "/api/tv/tun-1/tune", false},
		{"catch-up of the allowed programme alone", kid, fmt.Sprintf("/api/tv/tun-3/catchup?start=%d", now-7200), false},
		// Asked to run on, it is cut at the programme's end instead (handleCatchup).
		{"catch-up of the allowed programme asked to run on", kid, fmt.Sprintf("/api/tv/tun-3/catchup?start=%d&end=%d", now-7200, now-60), false},
		{"an unrated programme, unrated allowed", kidUnrated, "/api/tv/tun-4/tune", false},
		{"no limits", f.adult, "/api/tv/tun-3/tune", false},
		{"no limits, playlist", f.adult, "/api/tv/tun-3/live/playlist.m3u8", false},
	}
	for _, c := range cases {
		w := ts.do(t, "GET", c.path, c.token, nil)
		if got := refusedForRating(w.Code, w.Body.Bytes()); got != c.refused {
			t.Errorf("%s (%s): %d %s, refused for its rating = %v, want %v", c.name, c.path, w.Code, w.Body.String(), got, c.refused)
		}
	}

	// The refusal is in the profile's report, for whoever manages it.
	var n int
	if err := ts.DB.QueryRow(`SELECT COUNT(*) FROM profile_events WHERE kind = 'blocked' AND title = 'Grim Thriller'`).Scan(&n); err != nil || n == 0 {
		t.Errorf("the refusal wasn't written down: %d %v", n, err)
	}
}

// When the programme changes to one the profile may not see, the stream it was allowed stops
// at the next request: what was decided isn't kept past the programme's end.
func TestLiveTVVerdictEndsWithTheProgramme(t *testing.T) {
	f, _, _ := programmeFixture(t)
	ts := f.ts
	now := time.Now().Truncate(time.Minute).Unix()
	tunerChannel(t, livetv.Channel{ID: 5, Number: "8", Name: "Evening", EPGID: "evening.test", Enabled: true}, "http://provider.invalid/5.ts")
	if _, err := ts.DB.Exec(`INSERT INTO epg_programmes(channel, start, stop, title, rating) VALUES('evening.test', ?, ?, 'Bedtime Story', 'TV-Y'), ('evening.test', ?, ?, 'Night News', 'TV-14')`,
		now-600, now+600, now+600, now+4200); err != nil {
		t.Fatal(err)
	}
	u, _ := ts.user(t, "evekid", "kids", false, `{"maxAge":7,"liveTv":true,"libraryChannels":true,"tunerChannels":true}`)
	r := httptest.NewRequest("GET", "/api/tv/tun-5/live/playlist.m3u8", nil)
	r = r.WithContext(context.WithValue(r.Context(), userKey, u))
	if ok, _, err := ts.Server.liveAllowed(r, "tun", 5, time.Unix(now, 0)); err != nil || !ok {
		t.Fatalf("the bedtime story: %v %v", ok, err)
	}
	if ok, _, err := ts.Server.liveAllowed(r, "tun", 5, time.Unix(now+601, 0)); err != nil || ok {
		t.Fatalf("the news after it, asked with the story's answer still kept: %v %v", ok, err)
	}
}
