package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/metrics"
)

func TestMetricsRoles(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "grownup", "adult", false, "")
	_, kid := ts.user(t, "kiddo", "kids", false, "")

	// Nobody else by default, and the catalogue just says so.
	var cat MetricsCatalog
	w := ts.do(t, "GET", "/api/system/metrics", adult, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &cat); err != nil || cat.Access.Visible || len(cat.Sections) != 0 {
		t.Fatalf("%s", w.Body)
	}
	for _, path := range []string{"/now", "/stream", "/report"} {
		if w := ts.do(t, "GET", "/api/system/metrics"+path, adult, nil); w.Code != 403 {
			t.Errorf("%s by default: %d", path, w.Code)
		}
	}
	if w := ts.do(t, "GET", "/api/system/metrics/history?range=1h", adult, nil); w.Code != 403 {
		t.Errorf("history: %d", w.Code)
	}

	// Administrators get the machine.
	w = ts.do(t, "GET", "/api/system/metrics", admin, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &cat); err != nil || !cat.Access.Full || len(cat.Series) == 0 {
		t.Fatalf("%s", w.Body)
	}
	w = ts.do(t, "GET", "/api/system/metrics/now", admin, nil)
	var now MetricsNow
	if err := json.Unmarshal(w.Body.Bytes(), &now); err != nil || now.Process == nil || now.Memory == nil || now.Server == nil || len(now.Volumes) == 0 {
		t.Fatalf("%s", w.Body)
	}

	// Allowed: only what's theirs, and no machine, no paths.
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"statsViewers": "adults"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = ts.do(t, "GET", "/api/system/metrics/now", adult, nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &now); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{`"cpu"`, `"memory"`, `"volumes"`, `"net"`, `"process"`, `"modules"`, ts.Cfg.DataDir} {
		if strings.Contains(w.Body.String(), leak) {
			t.Errorf("an adult could read %s: %s", leak, w.Body)
		}
	}
	if w := ts.do(t, "GET", "/api/system/metrics/history?range=1h", adult, nil); w.Code != 403 {
		t.Errorf("the machine's history stays with admins: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/system/metrics/now", kid, nil); w.Code != 403 {
		t.Errorf("adults only: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/settings", admin, map[string]string{"statsViewers": "dogs"}); w.Code != 400 {
		t.Errorf("a bad value: %d", w.Code)
	}
}

func TestMetricsHistoryChecksWhatsAsked(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	if w := ts.do(t, "GET", "/api/system/metrics/history?range=forever", admin, nil); w.Code != 400 {
		t.Errorf("bad range: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/system/metrics/history?range=1h&series=cpu,../../etc", admin, nil); w.Code != 400 {
		t.Errorf("bad series: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/system/metrics/history?range=1h&series=cpu", admin, nil); w.Code != 200 {
		t.Errorf("good: %d %s", w.Code, w.Body)
	}
	if err := (metricStore{ts.Server}).SaveHour(context.Background(), time.Now().Add(-48*time.Hour).Truncate(time.Hour).Unix(), metrics.CPU, 12, 40); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "GET", "/api/system/metrics/history?range=7d&series=cpu", admin, nil)
	var h metrics.History
	if err := json.Unmarshal(w.Body.Bytes(), &h); err != nil || len(h.Times) != 1 || h.Series[metrics.CPU][0] != 12 || h.Peak[metrics.CPU][0] != 40 {
		t.Fatalf("%s", w.Body)
	}
}

func TestMetricsReport(t *testing.T) {
	ts := newTestServer(t)
	admin, adminTok := ts.user(t, "boss", "adult", true, "")
	ctx := context.Background()
	open, _ := ts.Store.CreateLibrary(ctx, "Open", "movies", []string{t.TempDir()})
	shut, _ := ts.Store.CreateLibrary(ctx, "Shut", "movies", []string{t.TempDir()})
	viewer, viewerTok := ts.user(t, "viewer", "adult", false, fmt.Sprintf(`{"maxAge":-1,"allowUnrated":true,"libraries":[%d]}`, open.ID))
	add := func(lib int64, title string, year int, codec string, h, size int64) int64 {
		res, err := ts.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, year, video_codec, height, size_bytes, added_at, updated_at, missing) VALUES (?, 'video', ?, ?, ?, ?, ?, ?, ?, 1, 1, 0)`,
			lib, "/secret/"+title+fmt.Sprint(lib, size), title, title, year, codec, h, size)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	a := add(open.ID, "A Made Up Film", 2001, "h264", 1080, 1000)
	add(open.ID, "A Made Up Film", 2001, "hevc", 2160, 3000)
	b := add(shut.ID, "Secret Picture", 2002, "h264", 720, 500)
	for _, row := range [][3]int64{{admin.ID, a, 100}, {viewer.ID, a, 50}, {admin.ID, b, 900}} {
		if _, err := ts.DB.ExecContext(ctx, `INSERT INTO play_log(user_id, item_id, day, seconds) VALUES (?, ?, 1, ?)`, row[0], row[1], row[2]); err != nil {
			t.Fatal(err)
		}
	}
	var rep MetricsReport
	w := ts.do(t, "GET", "/api/system/metrics/report", adminTok, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatalf("%v %s", err, w.Body)
	}
	if len(rep.Libraries) != 2 || rep.Duplicates == nil || rep.Duplicates.Groups != 1 || rep.Duplicates.WastedBytes != 1000 || rep.Savings == nil || rep.Savings.Items != 2 {
		t.Fatalf("%s", w.Body)
	}
	if rep.MostWatched[0].Title != "Secret Picture" || rep.Database == nil {
		t.Fatalf("%s", w.Body)
	}
	got := map[string]int64{}
	for _, r := range rep.Resolutions {
		got[r.Name] = r.Items
	}
	if got["4K"] != 1 || got["1080p"] != 1 || got["720p"] != 1 {
		t.Errorf("resolutions %+v", rep.Resolutions)
	}

	// Someone limited sees their library and their own record, and nothing that says more.
	if w := ts.do(t, "PUT", "/api/settings", adminTok, map[string]string{"statsViewers": "adults"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = ts.do(t, "GET", "/api/system/metrics/report", viewerTok, nil)
	body := w.Body.String()
	for _, leak := range []string{"Secret Picture", "Shut", "/secret/", `"database"`, `"duplicates"`, `"bandwidth"`, `"caches"`} {
		if strings.Contains(body, leak) {
			t.Errorf("%s could be read: %s", leak, body)
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.MostWatched) != 1 || rep.MostWatched[0].Seconds != 50 || len(rep.Libraries) != 1 {
		t.Fatalf("%s", body)
	}
}

func TestStreamsAreTrackedAndCounted(t *testing.T) {
	ts := newTestServer(t)
	admin, adminTok := ts.user(t, "boss", "adult", true, "")
	_, viewerTok := ts.user(t, "viewer", "adult", false, "")
	dir := t.TempDir()
	file := filepath.Join(dir, "A Made Up Film.mp4")
	if err := os.WriteFile(file, make([]byte, 64<<10), 0o644); err != nil {
		t.Fatal(err)
	}
	lib, _ := ts.Store.CreateLibrary(context.Background(), "Films", "movies", []string{dir})
	res, err := ts.DB.ExecContext(context.Background(), `INSERT INTO items(library_id, kind, path, title, sort_title, size_bytes, added_at, updated_at, missing) VALUES (?, 'video', ?, 'A Made Up Film', 'a', ?, 1, 1, 0)`, lib.ID, file, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/file", id), adminTok, nil); w.Code != 200 || w.Body.Len() != 64<<10 {
		t.Fatalf("file: %d %d", w.Code, w.Body.Len())
	}
	var now MetricsNow
	w := ts.do(t, "GET", "/api/system/metrics/now?sections=streams", adminTok, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &now); err != nil {
		t.Fatal(err)
	}
	if len(now.Streams) != 1 || now.Streams[0].Bytes != 64<<10 || now.Streams[0].UserID != admin.ID || now.Streams[0].Decision != "direct" {
		t.Fatalf("%s", w.Body)
	}
	if strings.Contains(w.Body.String(), dir) {
		t.Errorf("a path in the streams: %s", w.Body)
	}
	if w := ts.do(t, "PUT", "/api/settings", adminTok, map[string]string{"statsViewers": "adults"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	w = ts.do(t, "GET", "/api/system/metrics/now?sections=streams", viewerTok, nil)
	if strings.Contains(w.Body.String(), "Made Up") || strings.Contains(w.Body.String(), "boss") {
		t.Errorf("someone else's stream: %s", w.Body)
	}
}

func TestTwoPeopleSharingAStreamAreEachCounted(t *testing.T) {
	ts := newTestServer(t)
	a, aTok := ts.user(t, "alex", "adult", true, "")
	b, bTok := ts.user(t, "bea", "adult", false, "")
	item := &library.Item{ID: 7, Title: "A Made Up Film"}
	serve := func(tok string, u *auth.User) {
		r := httptest.NewRequest("GET", "/api/hls/v-shared/seg/1", nil)
		r = r.WithContext(context.WithValue(context.WithValue(r.Context(), userKey, u), tokenKey, tok))
		ts.streams.note(r, item, "transcode", []string{"why"}, "v-shared")
		w, done := ts.streams.served(httptest.NewRecorder(), r, 0, "v-shared")
		_, _ = w.Write(make([]byte, 100))
		done()
	}
	serve(aTok, a)
	serve(bTok, b)
	serve(aTok, a)
	got := map[int64]int64{}
	for _, v := range ts.streams.streams(0) {
		got[v.UserID] = v.Bytes
	}
	if len(got) != 2 || got[a.ID] != 200 || got[b.ID] != 100 {
		t.Fatalf("%+v", got)
	}
}

func TestEndingStreamsLetsAServerStop(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	srv := httptest.NewServer(ts.h)
	defer srv.Close()
	for _, path := range []string{"/api/system/tasks/stream", "/api/system/metrics/stream"} {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+admin)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		ended := make(chan struct{})
		go func() { _, _ = io.Copy(io.Discard, res.Body); close(ended) }()
		ts.EndStreams()
		ts.EndStreams() // twice is once
		select {
		case <-ended:
		case <-time.After(5 * time.Second):
			t.Fatalf("%s kept going", path)
		}
	}
}

func TestMetricsStreamSamplesOnlyWhileOpen(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "boss", "adult", true, "")
	srv := httptest.NewServer(ts.h)
	defer srv.Close()
	if ts.Metrics.Watchers() != 0 {
		t.Fatal("nobody is watching")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/system/metrics/stream?sections=cpu,process", nil)
	req.Header.Set("Authorization", "Bearer "+admin)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	r := bufio.NewReader(res.Body)
	samples := 0
	for samples < 2 {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "event: sample") {
			samples++
		}
	}
	if ts.Metrics.Watchers() != 1 {
		t.Fatal("the stream should keep the sampler going")
	}
	cancel()
	res.Body.Close()
	waitFor(t, func() bool { return ts.Metrics.Watchers() == 0 })
}
