// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// bigLineup adds two IPTV tuners with n channels between them, in groups, and
// hours of guide data for every one of them, the way a large provider's playlist and
// XMLTV file would leave the database.
func bigLineup(t testing.TB, s *Server, n, hours int) {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for id, name := range []string{"Provider A", "Provider B"} {
		if _, err := tx.Exec(`INSERT INTO tuners(id, driver, name, options, created_at) VALUES(?, 'iptv', ?, '{}', 1)`, id+1, name); err != nil {
			t.Fatal(err)
		}
	}
	groups := []string{"News", "Sports", "Kids", "Movies", "Music", "Documentary", "Entertainment", "Regional"}
	start := time.Now().Truncate(30*time.Minute).Unix() - 3600
	for i := 1; i <= n; i++ {
		epg := fmt.Sprintf("ch%05d.test", i)
		if _, err := tx.Exec(`INSERT INTO tuner_channels(id, tuner_id, source_id, number, name, grp, epg_id) VALUES(?,?,?,?,?,?,?)`,
			i, 1+i%2, fmt.Sprint("s", i), fmt.Sprint(i), fmt.Sprintf("Channel %05d", i), groups[i%len(groups)], epg); err != nil {
			t.Fatal(err)
		}
		for h := 0; h < hours*2; h++ {
			at := start + int64(h)*1800
			if _, err := tx.Exec(`INSERT INTO epg_programmes(channel, start, stop, title) VALUES(?,?,?,?)`, epg, at, at+1800, fmt.Sprintf("Programme %d on %d", h, i)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// A provider with ten thousand channels must stay quick to page through. The times are
// logged (go test -v -run Scale) and held to a bound loose enough for a busy machine.
func TestGuideScale(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a ten-thousand-channel lineup")
	}
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	bigLineup(t, ts.Server, 10000, 8)
	from := time.Now().Truncate(30 * time.Minute).Unix()
	cases := []struct{ name, url string }{
		{"page of 100 with 8 hours", fmt.Sprintf("/api/guide?from=%d&hours=8&offset=5000&limit=100", from)},
		{"page of 25", fmt.Sprintf("/api/guide?from=%d&hours=8&offset=0&limit=25", from)},
		{"every number", "/api/guide?list=1&limit=20000"},
		{"every number, light", "/api/guide?list=1&lite=1&limit=20000"},
		{"a group, page 3 of 50", fmt.Sprintf("/api/guide?from=%d&hours=8&set=grp:News&offset=100&limit=50", from)},
		{"dial 9876", fmt.Sprintf("/api/guide?from=%d&hours=8&locate=9876&limit=75", from)},
		{"search a name", fmt.Sprintf("/api/guide?from=%d&hours=8&q=channel%%2009&limit=100", from)},
		{"the set picker", "/api/tv-sets"},
	}
	for _, c := range cases {
		ts.do(t, "GET", c.url, tok, nil) // the first time warms the database's cache
		began := time.Now()
		w := ts.do(t, "GET", c.url, tok, nil)
		took := time.Since(began)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
		var g struct {
			Total    int               `json:"total"`
			Channels []json.RawMessage `json:"channels"`
			Sets     []json.RawMessage `json:"sets"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
			t.Fatal(err)
		}
		t.Logf("%-28s %5d rows of %5d, %7.1f KB, %v", c.name, len(g.Channels)+len(g.Sets), g.Total, float64(w.Body.Len())/1024, took)
		if g.Total == 0 && len(g.Sets) == 0 {
			t.Fatalf("%s: nothing came back", c.name)
		}
		if strings.Contains(c.url, "lite=1") && w.Body.Len() > 1500*1024 {
			t.Errorf("the light list is %d KB", w.Body.Len()/1024)
		}
		if took > 3*time.Second {
			t.Errorf("%s took %v", c.name, took)
		}
	}
}
