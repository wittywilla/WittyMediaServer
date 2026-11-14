// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// What a playlist from the internet names stays on the internet: WittyTuner says so of a channel
// it opens (Opening.Home), and the server's gate holds FFmpeg to it. Which addresses such a
// playlist may name at all, and where its guide comes from, are WittyTuner's to check.

// internetOnly stands in for the internet-only client. It serves a few made-up public hosts, and
// counts anything else as an attempt to leave the internet, as the real client refuses those.
type internetOnly struct {
	hosts map[string]string
	other atomic.Int64
}

func (f *internetOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	body, ok := f.hosts[r.URL.Host+r.URL.Path]
	if !ok {
		f.other.Add(1)
		return nil, fmt.Errorf("refusing to connect to %s", r.URL.Host)
	}
	return &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
}

// internetTuner is a channel of a playlist from the internet, which WittyTuner opened: the first
// address in the playlist.
func internetTuner(t *testing.T, playlist string, extra map[string]string) (*Manager, *internetOnly, *fakeDirectory, *Channel) {
	t.Helper()
	f := &internetOnly{hosts: map[string]string{"playlist.example/list.m3u": playlist}}
	for k, v := range extra {
		f.hosts[k] = v
	}
	m, d := testManager(t)
	m.internet = &http.Client{Transport: f}
	d.home, d.client = false, &http.Client{Transport: f}
	addr := ""
	for l := range strings.Lines(playlist) {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			addr = l
			break
		}
	}
	c := d.add(1, Channel{Number: "1", Name: "Live", Driver: "iptv"}, addr)
	return m, f, d, c
}

// The stream itself: a playlist, its variants and its segments, all through that same client, and
// a segment on the home network is refused.
func TestInternetStreamThroughTheGate(t *testing.T) {
	m, f, _, c := internetTuner(t, "#EXTM3U\n#EXTINF:-1,Live\nhttps://stream.example/live.m3u8\n", map[string]string{
		"stream.example/live.m3u8": "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nhttp://192.168.1.1/admin.ts\n#EXTINF:6,\nseg.ts\n",
		"stream.example/seg.ts":    strings.Repeat("\x47", 188*4),
	})
	o, err := m.Open(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Release()
	code, body := gateGet(t, o.gate.url(o.gate.entry))
	if code != 200 || strings.Contains(body, "192.168") {
		t.Fatalf("%d %s", code, body)
	}
	var codes []int
	for l := range strings.Lines(body) {
		if strings.HasPrefix(l, "http://") {
			c, _ := gateGet(t, strings.TrimSpace(l))
			codes = append(codes, c)
		}
	}
	if fmt.Sprint(codes) != "[502 200]" {
		t.Errorf("%v", codes)
	}
	if f.other.Load() != 1 {
		t.Errorf("%d asked for elsewhere", f.other.Load())
	}
}

// A DASH manifest names its own addresses, and FFmpeg goes to them by itself, past the gate.
func TestInternetPlaylistCantUseDASH(t *testing.T) {
	m, _, d, c := internetTuner(t, "#EXTM3U\n#EXTINF:-1,Live\nhttps://stream.example/live.mpd\n", map[string]string{
		"stream.example/live.mpd": `<?xml version="1.0"?><MPD xmlns="urn:mpeg:dash:schema:mpd:2011"><Period><BaseURL>http://192.168.1.1/admin</BaseURL></Period></MPD>`,
	})
	o, err := m.Open(context.Background(), c.ID)
	if err == nil {
		o.Release()
		t.Fatal("it opened")
	}
	if strings.Contains(err.Error(), "192.168") {
		t.Errorf("%v", err)
	}
	// The tuner is free again.
	if n := d.used(1); n != 0 {
		t.Errorf("%d in use", n)
	}
}

// At home, the same manifest is the owner's business.
func TestDASHAtHomeStillOpens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".m3u") {
			fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1,Live\nhttp://%s/live.mpd\n", r.Host)
			return
		}
		fmt.Fprint(w, `<?xml version="1.0"?><MPD xmlns="urn:mpeg:dash:schema:mpd:2011"><Period></Period></MPD>`)
	}))
	defer srv.Close()
	m, d := testManager(t)
	c := d.add(1, Channel{Number: "1", Name: "Live", Driver: "iptv"}, srv.URL+"/live.mpd")
	o, err := m.Open(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	o.Release()
}
