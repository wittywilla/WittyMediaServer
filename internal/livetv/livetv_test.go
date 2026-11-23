// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db/dbtest"
	wittytuner "github.com/wittywilla/wittymediaserver/internal/tuner"
)

func tsData(n int, scrambled bool) []byte {
	p := bytes.Repeat([]byte{0x47}, 188)
	p[1], p[2], p[3] = 0x01, 0x00, 0x10
	if scrambled {
		p[3] |= 0x80
	}
	return bytes.Repeat(p, n)
}

func bareManager(t *testing.T) *Manager {
	t.Helper()
	d, err := dbtest.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := New(d, nil, slog.Default())
	t.Cleanup(func() { m.Close(); d.Close() })
	return m
}

func playlist(srv *httptest.Server) string {
	return fmt.Sprintf(`#EXTM3U
#EXTINF:-1 tvg-id="news.test" tvg-chno="5" group-title="News",News
%[1]s/news.ts
#EXTINF:-1 tvg-id="film.test" tvg-chno="6",Films
%[1]s/film.ts
#EXTPROP:inputstream.adaptive.license_key=abc
#EXTINF:-1 tvg-chno="7",Locked
%[1]s/locked.mpd
#EXTINF:-1 tvg-chno="8",Scrambled
%[1]s/scrambled.ts
`, srv.URL)
}

func iptvServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/list.m3u", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, playlist(srv)) })
	mux.HandleFunc("/news.ts", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsData(500, false)) })
	mux.HandleFunc("/film.ts", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsData(500, false)) })
	mux.HandleFunc("/scrambled.ts", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsData(2000, true)) })
	mux.HandleFunc("/guide.xml", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now().Add(-10 * time.Minute).UTC().Format("20060102150405 -0700")
		stop := time.Now().Add(50 * time.Minute).UTC().Format("20060102150405 -0700")
		next := time.Now().Add(110 * time.Minute).UTC().Format("20060102150405 -0700")
		fmt.Fprintf(w, `<tv><channel id="news.test"><display-name>News</display-name></channel>
			<channel id="other"><display-name>Not here</display-name></channel>
			<channel id="byname"><display-name>Locked</display-name></channel>
			<programme start="%s" stop="%s" channel="news.test"><title>Morning</title><desc>D</desc></programme>
			<programme start="%s" stop="%s" channel="news.test"><title>Noon</title></programme>
			<programme start="%s" stop="%s" channel="other"><title>Skipped</title></programme>
			<programme start="%s" stop="%s" channel="byname"><title>By Name</title></programme>
			</tv>`, start, stop, stop, next, start, stop, start, stop)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// addIPTV puts the playlist's channels on a tuner of the fake WittyTuner, as it would read them.
func addIPTV(t *testing.T, f *fakeDirectory, srv *httptest.Server) (int64, []*Channel) {
	t.Helper()
	const tn = 1
	ch := []*Channel{
		f.add(tn, Channel{Number: "5", Name: "News", Group: "News", EPGID: "news.test", Driver: "iptv"}, srv.URL+"/news.ts"),
		f.add(tn, Channel{Number: "6", Name: "Films", EPGID: "film.test", Driver: "iptv"}, srv.URL+"/film.ts"),
		f.add(tn, Channel{Number: "7", Name: "Locked", Encrypted: "its playlist entry names a DRM licence", Driver: "iptv"}, srv.URL+"/locked.mpd"),
		f.add(tn, Channel{Number: "8", Name: "Scrambled", Driver: "iptv"}, srv.URL+"/scrambled.ts"),
	}
	return tn, ch
}

func TestOpenEncryptedAndLimits(t *testing.T) {
	m, f := testManager(t)
	srv := iptvServer(t)
	tn, ch := addIPTV(t, f, srv)
	ctx := context.Background()
	_, err := m.Open(ctx, ch[2].ID)
	var e *wittytuner.EncryptedError
	if !errors.As(err, &e) {
		t.Fatalf("encrypted: %v", err)
	}
	f.limit[tn] = 1
	a, err := m.Open(ctx, ch[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Open(ctx, ch[1].ID); !errors.Is(err, ErrBusy) {
		t.Fatalf("over the limit: %v", err)
	}
	a.Release()
	a.Release() // twice is harmless
	b, err := m.Open(ctx, ch[1].ID)
	if err != nil {
		t.Fatalf("released: %v", err)
	}
	b.Release()
	if n := f.used(tn); n != 0 {
		t.Fatalf("in use: %d", n)
	}
}

// A guide WittyTuner hands over keeps only the programmes of channels on this server, and a
// channel with no guide id is matched by its name, for WittyTuner to keep.
func TestXMLTVGuide(t *testing.T) {
	m, f := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, f, srv)
	ctx := context.Background()
	res, err := http.Get(srv.URL + "/guide.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	n, matched, err := m.Ingest(ctx, 3, res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || matched[ch[2].ID] != "byname" || len(matched) != 1 {
		t.Fatalf("%d programmes, matched %v", n, matched)
	}
	f.change(m, ch[2].ID, func(c *Channel) { c.EPGID = matched[ch[2].ID] })
	now := time.Now()
	g, err := m.Guide(ctx, ch[0], now, now.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(g) != 2 || g[0].Title != "Morning" || g[0].Desc != "D" || g[1].Title != "Noon" {
		t.Fatalf("%+v", g)
	}
	var skipped int
	_ = m.DB.QueryRow(`SELECT COUNT(*) FROM epg_programmes WHERE channel = 'other'`).Scan(&skipped)
	if skipped != 0 {
		t.Fatal("programmes of channels not on this server must not be stored")
	}
	ids, _ := m.GuideIDs(ctx)
	if len(ids) != 3 {
		t.Fatalf("%+v", ids)
	}
	if err := m.DropSource(ctx, 3); err != nil {
		t.Fatal(err)
	}
	ids, _ = m.GuideIDs(ctx)
	var left int
	_ = m.DB.QueryRow(`SELECT COUNT(*) FROM epg_programmes WHERE source LIKE 'xmltv:3%'`).Scan(&left)
	if len(ids) != 0 || left != 0 {
		t.Fatalf("a removed source's guide stays: %d ids, %d programmes", len(ids), left)
	}
}

func TestBroadcastDataFillsTheGuide(t *testing.T) {
	m, f := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, f, srv)
	ctx := context.Background()
	c := ch[1]
	key := broadcastKey(c)
	start := time.Now().Add(-5 * time.Minute).Unix()
	if _, err := m.DB.Exec(`INSERT INTO epg_programmes(channel, start, stop, title, source) VALUES(?,?,?,?, 'broadcast')`, key, start, start+1800, "From the air"); err != nil {
		t.Fatal(err)
	}
	g, err := m.Guide(ctx, c, time.Now(), time.Now().Add(time.Hour))
	if err != nil || len(g) != 1 || g[0].Title != "From the air" {
		t.Fatalf("%v %+v", err, g)
	}
}

func TestRecordingRawStream(t *testing.T) {
	// The test stream ends after 500 packets, long before the recording does: it's opened again
	// and added to the same file until the recording's end, as an IPTV stream that drops is.
	defer func(a int64, f, x time.Duration) { padAfter, recordingRetryFirst, recordingRetryMax = a, f, x }(padAfter, recordingRetryFirst, recordingRetryMax)
	padAfter, recordingRetryFirst, recordingRetryMax = 0, 50*time.Millisecond, 50*time.Millisecond
	m, f := testManager(t)
	srv := iptvServer(t)
	tn, ch := addIPTV(t, f, srv)
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	finished := make(chan string, 1)
	m.AfterRecording = func(p string) { finished <- p }
	ctx := context.Background()
	now := time.Now().Unix()
	// The same airing twice is refused. Done on an airing hours away, so the scheduler (which
	// Schedule wakes) can't start and finish the first one before the second is asked for.
	later := now + 7200
	if _, err := m.Schedule(ctx, ch[0].ID, 1, "News", "", later, later+60); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Schedule(ctx, ch[0].ID, 1, "News", "", later, later+30); err == nil {
		t.Fatal("a second recording of the same airing should be refused")
	}
	r, err := m.Schedule(ctx, ch[0].ID, 1, `News: "Late" / Edition`, "Part 1", now, now+2)
	if err != nil {
		t.Fatal(err)
	}
	m.startDue(ctx)
	select {
	case p := <-finished:
		st, err := os.Stat(p)
		if err != nil || st.Size() < 2*500*188 || st.Size()%(500*188) != 0 {
			t.Fatalf("the stream wasn't opened again and added to the file: %v %v", err, st)
		}
		rel, err := filepath.Rel(folder, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			t.Fatalf("recording outside its folder: %s", p)
		}
		if strings.ContainsAny(filepath.Base(p), `"/:`) {
			t.Fatalf("unsafe name: %s", p)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the recording did not finish")
	}
	got, _ := m.recording(ctx, r.ID)
	if got.Status != "done" || got.SizeBytes < 2*500*188 || !strings.Contains(got.Error, "dropped") {
		t.Fatalf("%+v", got)
	}
	if n := f.used(tn); n != 0 {
		t.Fatalf("the tuner should be free: %d", n)
	}
	if err := m.CancelRecording(ctx, r.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(got.Path); !os.IsNotExist(err) {
		t.Fatal("the file should be gone")
	}
}

func TestRecordingRefusesEncrypted(t *testing.T) {
	m, f := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, f, srv)
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	ctx := context.Background()
	now := time.Now().Unix()
	if _, err := m.Schedule(ctx, ch[2].ID, 1, "x", "", now, now+60); err == nil {
		t.Fatal("a marked channel must be refused when scheduling")
	}
	if _, err := m.AddRule(ctx, ch[2].ID, 1, "x"); err == nil {
		t.Fatal("and for series")
	}
	// Scrambled only shows once it is read: the file must not stay.
	r, err := m.Schedule(ctx, ch[3].ID, 1, "Secret", "", now, now+60)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := m.recording(ctx, r.ID)
	// Schedule woke the scheduler, and this airing is due now: whichever of the two claims it
	// first, as startDue does, runs it. Both running it was the flaky failure, the second one
	// writing a file after the first had cleaned up.
	res, err := m.DB.Exec(`UPDATE recordings SET status = 'recording' WHERE id = ? AND status = 'scheduled'`, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		m.record(full)
	}
	var got *Recording
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		got, _ = m.recording(ctx, r.ID)
		if got.Status != "scheduled" && got.Status != "recording" || time.Now().After(end) {
			break
		}
	}
	if got.Status != "failed" || !strings.Contains(got.Error, "does not decrypt") {
		t.Fatalf("%+v", got)
	}
	var found []string
	_ = filepath.Walk(folder, func(p string, i os.FileInfo, _ error) error {
		if i != nil && !i.IsDir() {
			found = append(found, p)
		}
		return nil
	})
	if len(found) != 0 {
		t.Fatalf("an encrypted recording left files: %v", found)
	}
}

func TestSeriesRule(t *testing.T) {
	m, f := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, f, srv)
	ctx := context.Background()
	future := time.Now().Add(2 * time.Hour).Unix()
	key := ch[0].GuideKey()
	for i := int64(0); i < 2; i++ {
		s := future + i*7200
		if _, err := m.DB.Exec(`INSERT INTO epg_programmes(channel, start, stop, title) VALUES(?,?,?,?)`, key, s, s+1800, "Weekly"); err != nil {
			t.Fatal(err)
		}
	}
	rule, err := m.AddRule(ctx, ch[0].ID, 1, "weekly")
	if err != nil {
		t.Fatal(err)
	}
	m.applyRules(ctx)
	m.applyRules(ctx) // twice must not double up
	list, _ := m.Recordings(ctx)
	if len(list) != 2 || list[0].Status != "scheduled" {
		t.Fatalf("%+v", list)
	}
	if err := m.DeleteRule(ctx, rule.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = m.Recordings(ctx)
	if len(list) != 0 {
		t.Fatalf("scheduled recordings of a removed rule should go: %+v", list)
	}
}

func TestSafeFileName(t *testing.T) {
	cases := map[string]string{`a/b\c:d`: "a b c d", "..": "Recording", "  x  y ": "x y", `CON?`: "Recording CON"}
	for in, want := range cases {
		if got := safeFileName(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
	if !strings.HasPrefix(safeFileName(strings.Repeat("é", 200)), "é") || len(safeFileName(strings.Repeat("é", 200))) > 120 {
		t.Error("long names are cut")
	}
}

func TestFFmpegFailuresInWords(t *testing.T) {
	for in, want := range map[string]string{
		"exit status 0xcbcfcb08:  [in#0] HTTP error 404 Not Found\r\nError opening input: Server returned 404 Not Found": "isn't there any more",
		"exit status 1: Server returned 403 Forbidden (access denied)":                                                   "refused this server",
		"exit status 1: Connection refused":                                                                              "could not be reached",
		"exit status 1: something new":                                                                                   "stopped and couldn't be started again",
	} {
		if got := PublicError(errors.New(in)); !strings.Contains(got, want) || strings.Contains(got, "exit status") {
			t.Errorf("%q: %q", in, got)
		}
	}
	if got := PublicError(errors.New("all of this device's tuners are in use")); got != "all of this device's tuners are in use" {
		t.Errorf("a plain error changed: %q", got)
	}
}
