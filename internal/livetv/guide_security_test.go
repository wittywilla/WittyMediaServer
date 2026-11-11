// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

const guideXML = `<?xml version="1.0"?><tv><channel id="a"><display-name>A</display-name></channel></tv>`

func guideSources(t *testing.T, m *Manager) []string {
	t.Helper()
	list, err := m.Sources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range list {
		out = append(out, s.URL)
	}
	return out
}

// A playlist's guide line can't name a file on this machine or a network share, whoever hosts the
// playlist: they aren't web addresses, and no playlist gets to choose what the server opens.
func TestPlaylistCantNameAFileAsItsGuide(t *testing.T) {
	file := filepath.Join(t.TempDir(), "private.xml")
	if err := os.WriteFile(file, []byte(guideXML), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, place := range []string{file, `\\attacker.example\share\g.xml`, "//attacker.example/share/g.xml", "file://" + file, "ftp://attacker.example/g.xml"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, "#EXTM3U url-tvg=\"%s\"\n#EXTINF:-1,Live\nhttp://stream.example/a.ts\n", place)
		}))
		m := testManager(t)
		if _, err := m.Create(context.Background(), &Tuner{Driver: "iptv", Name: "Live", Options: map[string]string{"playlist": srv.URL + "/l.m3u"}}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		if got := guideSources(t, m); len(got) != 0 {
			t.Errorf("%s became a guide source: %v", place, got)
		}
		if ids, _ := m.GuideIDs(context.Background()); len(ids) != 0 {
			t.Errorf("%s was read: %v", place, ids)
		}
		srv.Close()
	}
}

// One saved by an earlier version is refused when it comes up for fetching.
func TestOldPlaylistGuideFileIsRefused(t *testing.T) {
	file := filepath.Join(t.TempDir(), "private.xml")
	if err := os.WriteFile(file, []byte(guideXML), 0o600); err != nil {
		t.Fatal(err)
	}
	m := testManager(t)
	ctx := context.Background()
	tn, err := m.Create(ctx, &Tuner{Driver: "stream", Name: "One", Options: map[string]string{"url": "http://stream.example/a.ts"}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := m.DB.Exec(`INSERT INTO epg_sources(name, url, tuner_id) VALUES('old', ?, ?)`, file, tn.ID)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if err := m.FetchSource(ctx, id); err == nil {
		t.Error("the file was read")
	}
	if ids, _ := m.GuideIDs(ctx); len(ids) != 0 {
		t.Errorf("%v", ids)
	}
}

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

func internetTuner(t *testing.T, playlist string, extra map[string]string) (*Manager, *internetOnly, *Tuner) {
	t.Helper()
	f := &internetOnly{hosts: map[string]string{"playlist.example/list.m3u": playlist}}
	for k, v := range extra {
		f.hosts[k] = v
	}
	old := engine
	t.Cleanup(func() { SetEngine(old) })
	SetEngine(&listEngine{playlist: playlist})
	m := bareManager(t)
	m.internet = &http.Client{Transport: f}
	tn, err := m.Create(context.Background(), &Tuner{Driver: "iptv", Name: "Elsewhere", Options: map[string]string{"playlist": "https://playlist.example/list.m3u"}})
	if err != nil {
		t.Fatal(err)
	}
	return m, f, tn
}

// listEngine is WittyTuner's helper as far as the server can see it: what a playlist names, and a
// stream for FFmpeg. A DASH manifest comes back checked and with nothing to look at again.
type listEngine struct {
	fakeEngine
	playlist string
}

func (e *listEngine) New(context.Context, string, tuner.Config, tuner.Env) (tuner.Source, error) {
	return &listSource{playlist: e.playlist}, nil
}

type listSource struct{ playlist string }

func (s *listSource) Info() tuner.Info {
	info := tuner.Info{Driver: "iptv"}
	if _, after, ok := strings.Cut(s.playlist, `url-tvg="`); ok {
		info.GuideURL, _, _ = strings.Cut(after, `"`)
	}
	return info
}

func (s *listSource) Channels(context.Context) ([]tuner.Channel, error) {
	var out []tuner.Channel
	for l := range strings.Lines(s.playlist) {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, tuner.Channel{ID: l, Name: "Live", Locator: l})
		}
	}
	return out, nil
}

func (s *listSource) Scan(ctx context.Context, _ tuner.ScanOptions, _ func(tuner.Progress)) ([]tuner.Channel, error) {
	return s.Channels(ctx)
}

func (s *listSource) Open(_ context.Context, ch tuner.Channel) (*tuner.Stream, error) {
	return &tuner.Stream{Input: []string{"-i", ch.Locator}, Verified: strings.HasSuffix(ch.Locator, ".mpd")}, nil
}

func (s *listSource) Close() error { return nil }

func TestWhoMayLeadHome(t *testing.T) {
	for addr, want := range map[string]bool{
		"/srv/list.m3u":                    true,
		`C:\\lists\\list.m3u`:              true,
		"http://192.168.1.2/list.m3u":      true,
		"http://10.0.0.2/list.m3u":         true,
		"http://[fd00::2]/list.m3u":        true,
		"http://100.101.102.103/list.m3u":  true,
		"http://localhost/list.m3u":        true,
		"http://127.0.0.1:8000/list.m3u":   true,
		"http://mediabox/list.m3u":         true,
		"http://nas.local/list.m3u":        true,
		"http://nas.lan:8080/list.m3u":     true,
		"http://nas.home.arpa/list.m3u":    true,
		"http://NAS.LOCAL./list.m3u":       true,
		"https://iptv.example.com/l.m3u":   false,
		"https://8.8.8.8/list.m3u":         false,
		"https://nas.local.example.com/l":  false,
		"https://localhost.example.com/l":  false,
		"https://127.0.0.1.nip.io/list":    false,
		"https://192.168.1.2.example.com/": false,
		"https://[2001:4860::1]/list.m3u":  false,
	} {
		tn := Tuner{Driver: "iptv", Options: map[string]string{"playlist": addr}}
		if got := mayLeadHome(&tn); got != want {
			t.Errorf("%s: %v", addr, got)
		}
	}
	for _, d := range []string{"network", "stream", "dvb", "satip", "backend"} {
		if !mayLeadHome(&Tuner{Driver: d}) {
			t.Errorf("%s has the address the owner typed", d)
		}
	}
}

// A playlist from the internet names only web addresses. FFmpeg makes its own connections for the
// rest, so those would reach whatever they name, and udp://@:5000 would even open a port here.
func TestInternetPlaylistNamesOnlyWebAddresses(t *testing.T) {
	for _, addr := range []string{"rtsp://192.168.1.1:554/x", "rtmp://2130706433/", "rtsp://2130706433:554/x", "udp://@:5000", "rtp://239.0.0.1:5004", "srt://192.168.1.1:9000"} {
		m, _, tn := internetTuner(t, "#EXTM3U\n#EXTINF:-1,Live\n"+addr+"\n", nil)
		ch, err := m.Channels(context.Background(), tn.ID)
		if err != nil || len(ch) != 1 {
			t.Fatalf("%s: %v %v", addr, err, ch)
		}
		m.Tools = nil
		o, err := m.Open(context.Background(), ch[0].ID)
		if err == nil {
			o.Release()
			t.Errorf("%s was opened", addr)
		} else if !strings.Contains(err.Error(), "web address") {
			t.Errorf("%s: %v", addr, err)
		}
	}
}

// The stream itself: a playlist, its variants and its segments, all through that same client, and
// a segment on the home network is refused.
func TestInternetStreamThroughTheGate(t *testing.T) {
	m, f, tn := internetTuner(t, "#EXTM3U\n#EXTINF:-1,Live\nhttps://stream.example/live.m3u8\n", map[string]string{
		"stream.example/live.m3u8": "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6,\nhttp://192.168.1.1/admin.ts\n#EXTINF:6,\nseg.ts\n",
		"stream.example/seg.ts":    strings.Repeat("\x47", 188*4),
	})
	ch, err := m.Channels(context.Background(), tn.ID)
	if err != nil || len(ch) != 1 {
		t.Fatalf("%v %v", err, ch)
	}
	o, err := m.Open(context.Background(), ch[0].ID)
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

// A guide line in a playlist from the internet is fetched by the same client.
func TestInternetPlaylistGuideStaysOnTheInternet(t *testing.T) {
	var hit atomic.Int64
	home := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit.Add(1) }))
	defer home.Close()
	m, _, tn := internetTuner(t, "#EXTM3U url-tvg=\""+home.URL+"/cgi-bin/reboot\"\n#EXTINF:-1,Live\nhttp://stream.example/a.ts\n", nil)
	m.internet = nil // the real one, now the playlist has been read
	var id int64
	if err := m.DB.QueryRow(`SELECT id FROM epg_sources WHERE tuner_id = ?`, tn.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := m.FetchSource(context.Background(), id); err == nil {
		t.Error("it reached a home address")
	}
	if hit.Load() != 0 {
		t.Errorf("reached it %d times", hit.Load())
	}
}

// What the owner typed goes on working, on the home network too.
func TestOwnersGuideOnTheHomeNetworkWorks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, guideXML) }))
	defer srv.Close()
	m := testManager(t)
	ctx := context.Background()
	s, err := m.AddSource(ctx, "Mine", srv.URL+"/g.xml")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.FetchSource(ctx, s.ID); err != nil {
		t.Errorf("%v", err)
	}
}

// A DASH manifest names its own addresses, and FFmpeg goes to them by itself, past the gate.
func TestInternetPlaylistCantUseDASH(t *testing.T) {
	m, _, tn := internetTuner(t, "#EXTM3U\n#EXTINF:-1,Live\nhttps://stream.example/live.mpd\n", map[string]string{
		"stream.example/live.mpd": `<?xml version="1.0"?><MPD xmlns="urn:mpeg:dash:schema:mpd:2011"><Period><BaseURL>http://192.168.1.1/admin</BaseURL></Period></MPD>`,
	})
	ch, err := m.Channels(context.Background(), tn.ID)
	if err != nil || len(ch) != 1 {
		t.Fatalf("%v %v", err, ch)
	}
	o, err := m.Open(context.Background(), ch[0].ID)
	if err == nil {
		o.Release()
		t.Fatal("it opened")
	}
	if strings.Contains(err.Error(), "192.168") {
		t.Errorf("%v", err)
	}
	// The tuner is free again.
	if m.inUse[tn.ID] != 0 {
		t.Errorf("%d in use", m.inUse[tn.ID])
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
	m := testManager(t)
	tn, err := m.Create(context.Background(), &Tuner{Driver: "iptv", Name: "Home", Options: map[string]string{"playlist": srv.URL + "/l.m3u"}})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := m.Channels(context.Background(), tn.ID)
	if err != nil || len(ch) != 1 {
		t.Fatalf("%v %v", err, ch)
	}
	o, err := m.Open(context.Background(), ch[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	o.Release()
}
