// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// What only an administrator may see stays that way through a collection.
func TestCollectionsDontGiveAwayAddresses(t *testing.T) {
	ts, lib, st := radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("audio"))
	})
	it := addStation(t, ts, lib, "Private", st.URL+"/stream?key=SECRETKEY")
	w := ts.do("POST", "/api/collections", ts.adult, map[string]any{"name": "Mine"})
	var c struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &c); w.Code >= 300 || err != nil {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	path := fmt.Sprintf("/api/collections/%d", c.ID)
	if w := ts.do("POST", path+"/items", ts.adult, map[string]any{"items": []int64{it.ID}}); w.Code >= 300 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := ts.do("GET", path, ts.adult, nil)
	if got.Code != 200 || !strings.Contains(got.Body.String(), "Private") {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	if strings.Contains(got.Body.String(), "SECRETKEY") || strings.Contains(got.Body.String(), st.URL) {
		t.Errorf("the station's address came through: %s", got.Body)
	}
}

// A station's address can hold a private key: listeners never get it, and nor does the log.
func TestRadioPlaylistsDontGiveAwayAddresses(t *testing.T) {
	var ts *galleryServer
	var lib *library.Library
	var st *httptest.Server
	ts, lib, st = radioFixture(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/live.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			fmt.Fprint(w, "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nSECRETPATH/seg1.aac?key=SECRETKEY\n")
		default:
			w.Header().Set("Content-Type", "audio/aac")
			_, _ = w.Write([]byte("segment"))
		}
	})
	it := addStation(t, ts, lib, "Live", st.URL+"/live.m3u8")
	w := ts.do("GET", fmt.Sprintf("/api/radio/%d/stream", it.ID), ts.adult, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	body := w.Body.String()
	m := regexp.MustCompile(`hls\?u=([A-Za-z0-9_-]+)`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no part in %s", body)
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "SECRET") {
			t.Errorf("the address shows: %s", line)
		}
	}
	if strings.Contains(m[1], "aHR0c") { // the start of "http" in base64
		t.Errorf("that's only encoded: %s", m[1])
	}
	seg := ts.do("GET", fmt.Sprintf("/api/radio/%d/hls?u=%s", it.ID, m[1]), ts.adult, nil)
	if seg.Code != 200 || seg.Body.String() != "segment" {
		t.Errorf("the part doesn't play: %d %s", seg.Code, seg.Body)
	}
	// It means nothing for another station, and can't be altered.
	other := addStation(t, ts, lib, "Other", st.URL+"/other")
	if w := ts.do("GET", fmt.Sprintf("/api/radio/%d/hls?u=%s", other.ID, m[1]), ts.adult, nil); w.Code != 403 {
		t.Errorf("another station: %d", w.Code)
	}
	if w := ts.do("GET", fmt.Sprintf("/api/radio/%d/hls?u=A%s", it.ID, m[1]), ts.adult, nil); w.Code != 403 {
		t.Errorf("altered: %d", w.Code)
	}
	if got := redactQuery("u=" + m[1] + "&st=abc"); strings.Contains(got, m[1]) || strings.Contains(got, "abc") {
		t.Errorf("the log keeps it: %s", got)
	}
}

func TestStationErrorsDontQuoteTheAddress(t *testing.T) {
	_, err := library.OpenStation(context.Background(), "http://127.0.0.1:1/SECRETPATH?key=SECRETKEY")
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("%v", err)
	}
}
