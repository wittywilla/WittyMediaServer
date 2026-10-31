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

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittytuner"
)

func tsData(n int, scrambled bool) []byte {
	p := bytes.Repeat([]byte{0x47}, 188)
	p[1], p[2], p[3] = 0x01, 0x00, 0x10
	if scrambled {
		p[3] |= 0x80
	}
	return bytes.Repeat(p, n)
}

func testManager(t *testing.T) *Manager {
	t.Helper()
	d, err := db.Open(t.TempDir())
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

func addIPTV(t *testing.T, m *Manager, srv *httptest.Server) (*Tuner, []*Channel) {
	t.Helper()
	ctx := context.Background()
	tn, err := m.Create(ctx, &Tuner{Driver: "iptv", Name: "List", Options: map[string]string{"playlist": srv.URL + "/list.m3u", "guide": srv.URL + "/guide.xml"}})
	if err != nil {
		t.Fatal(err)
	}
	if tn.Channels != 4 || tn.LastError != "" {
		t.Fatalf("%+v", tn)
	}
	ch, err := m.Channels(ctx, tn.ID)
	if err != nil {
		t.Fatal(err)
	}
	return tn, ch
}

func TestChannelsSurviveRescan(t *testing.T) {
	m := testManager(t)
	srv := iptvServer(t)
	tn, ch := addIPTV(t, m, srv)
	ctx := context.Background()
	if ch[0].Name != "News" || ch[0].Number != "5" || ch[0].Group != "News" || ch[0].EPGID != "news.test" {
		t.Fatalf("%+v", ch[0])
	}
	if ch[2].Encrypted == "" {
		t.Fatal("a DRM playlist entry must be marked")
	}
	name, num, off := "My News", "105", false
	if _, err := m.EditChannel(ctx, ch[0].ID, ChannelEdit{Name: &name, Number: &num, Enabled: &off}); err != nil {
		t.Fatal(err)
	}
	if err := m.Refresh(ctx, tn.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Channel(ctx, ch[0].ID)
	if got.Name != "My News" || got.Number != "105" || got.Enabled {
		t.Fatalf("a refresh must keep the owner's choices: %+v", got)
	}
	bad := "abc"
	if _, err := m.EditChannel(ctx, ch[0].ID, ChannelEdit{Number: &bad}); err == nil {
		t.Fatal("bad number")
	}
}

func TestOpenEncryptedAndLimits(t *testing.T) {
	m := testManager(t)
	srv := iptvServer(t)
	tn, ch := addIPTV(t, m, srv)
	ctx := context.Background()
	_, err := m.Open(ctx, ch[2].ID)
	var e *wittytuner.EncryptedError
	if !errors.As(err, &e) {
		t.Fatalf("encrypted: %v", err)
	}
	if _, err := m.Update(ctx, tn.ID, "List", nil, true, 1); err != nil {
		t.Fatal(err)
	}
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
	m.mu.Lock()
	n := m.inUse[tn.ID]
	m.mu.Unlock()
	if n != 0 {
		t.Fatalf("in use: %d", n)
	}
}

func TestPasswordsNotSentBack(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	tn, err := m.Create(ctx, &Tuner{Driver: "backend", Name: "B", Options: map[string]string{"protocol": "http-m3u", "address": "http://127.0.0.1:1", "username": "u", "password": "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if pub := PublicOptions(tn); pub["password"] != Masked {
		t.Fatalf("%+v", pub)
	}
	opts := PublicOptions(tn)
	got, err := m.Update(ctx, tn.ID, "B2", opts, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Options["password"] != "secret" {
		t.Fatalf("a masked password must keep the stored one: %v", got.Options["password"])
	}
}

func TestInvalidTuner(t *testing.T) {
	m := testManager(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, &Tuner{Driver: "nope", Name: "x"}); err == nil {
		t.Fatal("unknown driver")
	}
	if _, err := m.Create(ctx, &Tuner{Driver: "iptv", Name: ""}); err == nil {
		t.Fatal("no name")
	}
	if _, err := m.Create(ctx, &Tuner{Driver: "command", Name: "x", Options: map[string]string{"command": "/bin/sh"}}); err == nil {
		t.Fatal("custom commands are off by default")
	}
}

func TestXMLTVGuide(t *testing.T) {
	m := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, m, srv)
	ctx := context.Background()
	list, _ := m.Sources(ctx)
	if len(list) != 1 {
		t.Fatalf("the tuner's guide should become a source: %+v", list)
	}
	if err := m.FetchSource(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
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
	// A channel with no id matches by name.
	locked, _ := m.Channel(ctx, ch[2].ID)
	if locked.EPGID != "byname" {
		t.Fatalf("name match: %+v", locked)
	}
	ids, _ := m.GuideIDs(ctx)
	if len(ids) != 3 {
		t.Fatalf("%+v", ids)
	}
	if err := m.DeleteSource(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
}

func TestBroadcastDataFillsTheGuide(t *testing.T) {
	m := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, m, srv)
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
	m := testManager(t)
	srv := iptvServer(t)
	tn, ch := addIPTV(t, m, srv)
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	finished := make(chan string, 1)
	m.AfterRecording = func(p string) { finished <- p }
	ctx := context.Background()
	now := time.Now().Unix()
	r, err := m.Schedule(ctx, ch[0].ID, 1, `News: "Late" / Edition`, "Part 1", now, now+5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Schedule(ctx, ch[0].ID, 1, "News", "", now, now+1); err == nil {
		t.Fatal("a second recording of the same airing should be refused")
	}
	m.startDue(ctx)
	select {
	case p := <-finished:
		st, err := os.Stat(p)
		if err != nil || st.Size() != 500*188 {
			t.Fatalf("%v %v", err, st)
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
	if got.Status != "done" || got.SizeBytes != 500*188 {
		t.Fatalf("%+v", got)
	}
	m.mu.Lock()
	n := m.inUse[tn.ID]
	m.mu.Unlock()
	if n != 0 {
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
	m := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, m, srv)
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
	// This test runs it, not the scheduler.
	if _, err := m.DB.Exec(`UPDATE recordings SET status = 'recording' WHERE id = ?`, r.ID); err != nil {
		t.Fatal(err)
	}
	m.record(full)
	got, _ := m.recording(ctx, r.ID)
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
	m := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, m, srv)
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
	cases := map[string]string{`a/b\c:d`: "a b c d", "..": "Recording", "  x  y ": "x y", `CON?`: "CON"}
	for in, want := range cases {
		if got := safeFileName(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
	if !strings.HasPrefix(safeFileName(strings.Repeat("é", 200)), "é") || len(safeFileName(strings.Repeat("é", 200))) > 120 {
		t.Error("long names are cut")
	}
}
