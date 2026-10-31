package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// logServer is a test server that really logs.
func logServer(t *testing.T) (*testServer, string) {
	t.Helper()
	ts := newTestServer(t)
	l, err := logging.New(filepath.Join(t.TempDir(), "logs"), slog.LevelInfo, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	guard, _, err := logging.StartCrashGuard(filepath.Join(t.TempDir(), "crash"), l.Dir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(guard.Close)
	ts.Logs, ts.Crashes = l, guard
	ts.Log = slog.New(l.Handler())
	old := slog.Default()
	slog.SetDefault(ts.Log)
	t.Cleanup(func() { slog.SetDefault(old) })
	_, admin := ts.user(t, "admin", "adult", true, "")
	return ts, admin
}

func logEntries(t *testing.T, ts *testServer, admin, query string) []logging.Record {
	t.Helper()
	w := ts.do(t, "GET", "/api/logs/entries?"+query, admin, nil)
	if w.Code != 200 {
		t.Fatalf("entries: %d %s", w.Code, w.Body)
	}
	var page logging.Page
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	return page.Entries
}

func logFiles(t *testing.T, ts *testServer) string {
	t.Helper()
	ts.Logs.Flush()
	var b strings.Builder
	for _, f := range ts.Logs.Files() {
		data, err := os.ReadFile(filepath.Join(ts.Logs.Dir(), f.Name))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
	}
	return b.String()
}

func attr(rec logging.Record, k string) string {
	for _, a := range rec.Attrs {
		if a.K == k {
			return a.V
		}
	}
	return ""
}

// A request that fails is on record with its ID, its answer and no token, even
// when nobody asked for web requests to be logged.
func TestFailedRequestsAreRecorded(t *testing.T) {
	ts, admin := logServer(t)
	const secret = "PLANTEDSECRETVALUE"
	ts.mux.HandleFunc("GET /api/test-fails", func(w http.ResponseWriter, r *http.Request) {
		ts.Log.WarnContext(r.Context(), "something inside the request")
		writeErr(w, 500, "the database went away")
	})
	w := ts.do(t, "GET", "/api/test-fails?token="+secret+"&q=word", "", nil)
	id := w.Header().Get("X-Request-Id")
	if w.Code != 500 || id == "" {
		t.Fatalf("%d, id %q", w.Code, id)
	}
	got := logEntries(t, ts, admin, "session="+id)
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	var rec logging.Record
	for _, e := range got {
		if e.Msg == "http" {
			rec = e
		}
	}
	if rec.Level != "error" || rec.Cat != "http" || attr(rec, "s") != "500" || !strings.Contains(attr(rec, "err"), "the database went away") {
		t.Errorf("%+v", rec)
	}
	if text := logFiles(t, ts); strings.Contains(text, secret) {
		t.Errorf("a token reached the log:\n%s", text)
	}
	// A request that went well costs nothing and leaves nothing.
	before := len(logEntries(t, ts, admin, "cat=http"))
	if w := ts.do(t, "GET", "/api/health", "", nil); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if after := len(logEntries(t, ts, admin, "cat=http")); after != before {
		t.Errorf("%d http records, was %d", after, before)
	}
}

// A panic in a handler answers 500, keeps the server up, and leaves a stack tied
// to the request and a crash report.
func TestPanicInHandler(t *testing.T) {
	ts, admin := logServer(t)
	ts.mux.HandleFunc("GET /api/test-panic", func(http.ResponseWriter, *http.Request) { panic("handler exploded") })
	w := ts.do(t, "GET", "/api/test-panic", "", nil)
	if w.Code != 500 {
		t.Fatalf("got %d", w.Code)
	}
	id := w.Header().Get("X-Request-Id")
	if !strings.Contains(w.Body.String(), id) {
		t.Errorf("the answer should give the request ID: %s", w.Body)
	}
	var found *logging.Record
	entries := logEntries(t, ts, admin, "level=error")
	for i := range entries {
		if entries[i].Level == "crash" {
			found = &entries[i]
		}
	}
	if found == nil || !strings.Contains(attr(*found, "stack"), "goroutine") || !strings.Contains(attr(*found, "panic"), "handler exploded") {
		t.Fatalf("%+v", entries)
	}
	if list := ts.Crashes.List(); len(list) != 1 || list[0].Kind != "panic" {
		t.Errorf("%+v", list)
	}
	// Still working. A client that went away is no crash.
	if w := ts.do(t, "GET", "/api/health", "", nil); w.Code != 200 {
		t.Error(w.Code)
	}
}

// A stream hands the connection to a sendfile-capable writer; the logger must
// not stand in its way.
func TestStatusWriterKeepsFastPaths(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, code: 200}
	if _, ok := any(sw).(io.ReaderFrom); !ok {
		t.Error("no ReadFrom: files would be copied the slow way")
	}
	if _, ok := any(sw).(http.Flusher); !ok {
		t.Error("no Flush")
	}
	if _, ok := any(sw).(http.Hijacker); !ok {
		t.Error("no Hijack: game streams would break")
	}
	if http.NewResponseController(sw).Flush() != nil {
		t.Error("the response controller can't find its way through")
	}
	if _, err := sw.ReadFrom(strings.NewReader("abc")); err != nil || rec.Body.String() != "abc" {
		t.Errorf("%v %q", err, rec.Body)
	}
}

func TestClientReports(t *testing.T) {
	ts, admin := logServer(t)
	_, adult := ts.user(t, "adult", "adult", false, "")
	const secret = "PLANTEDSECRETVALUE"
	if w := ts.do(t, "POST", "/api/logs/client", "", map[string]any{"message": "x"}); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/logs/entries", adult, nil); w.Code != 403 {
		t.Errorf("reading logs as a non-admin: %d", w.Code)
	}
	evil := "line one\ntime=2026-01-01T00:00:00Z level=ERROR msg=forged\x1b[2J"
	w := ts.do(t, "POST", "/api/logs/client", adult, map[string]any{
		"app": "web", "version": "1.2.3", "device": "phone",
		"events": []map[string]any{
			{"level": "error", "kind": "player", "message": "MEDIA_ERR_DECODE http://h/s?st=" + secret, "session": "stream-1", "request": "r-abc", "page": "/watch?token=" + secret, "stack": "at x\nBearer " + secret},
			{"level": "error", "kind": "js", "message": evil, "session": "not valid\nsession"},
			{"level": "info", "kind": "js", "message": "only at debug"},
		},
	})
	if w.Code != 204 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got := logEntries(t, ts, admin, "cat=clients")
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].Session != "stream-1" || attr(got[0], "request") != "r-abc" || attr(got[0], "kind") != "player" || attr(got[0], "user") != "adult" || got[0].Explain == nil {
		t.Errorf("%+v", got[0])
	}
	if got[1].Session != "" {
		t.Errorf("an invalid session was kept: %+v", got[1])
	}
	text := logFiles(t, ts)
	if strings.Contains(text, secret) {
		t.Errorf("a secret from a client reached the log:\n%s", text)
	}
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if !strings.HasPrefix(line, "time=") || strings.Contains(line, "\x1b") {
			t.Errorf("forged or unsafe line: %q", line)
		}
	}
	// The older one-at-a-time form still works.
	if w := ts.do(t, "POST", "/api/logs/client", adult, map[string]any{"level": "error", "message": "old form", "app": "web"}); w.Code != 204 {
		t.Errorf("%d", w.Code)
	}
	if got := logEntries(t, ts, admin, "q=old+form"); len(got) != 1 {
		t.Errorf("%+v", got)
	}

	// Too big.
	big, _ := json.Marshal(map[string]any{"message": strings.Repeat("x", clientBodyLimit*2)})
	if w := ts.do(t, "POST", "/api/logs/client", adult, big); w.Code != 413 {
		t.Errorf("too big: %d", w.Code)
	}
	// Too many: refused with a count, and it doesn't reach the disk.
	events := make([]map[string]any, clientMaxEvents)
	for i := range events {
		events[i] = map[string]any{"level": "error", "message": "flood"}
	}
	limited := 0
	for i := 0; i < clientPerUser; i++ {
		if w := ts.do(t, "POST", "/api/logs/client", adult, map[string]any{"events": events}); w.Code == 429 {
			limited++
		}
	}
	if limited == 0 {
		t.Error("never rate limited")
	}
	if n := len(logEntries(t, ts, admin, "q=flood&limit=1000")); n > clientPerUser {
		t.Errorf("%d kept", n)
	}

	// Off means off.
	if w := ts.do(t, "PUT", "/api/logs", admin, map[string]any{"levels": map[string]string{"clients": "off"}}); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	_, other := ts.user(t, "other", "adult", false, "")
	if w := ts.do(t, "POST", "/api/logs/client", other, map[string]any{"message": "after turning it off"}); w.Code != 204 {
		t.Errorf("%d", w.Code)
	}
	if got := logEntries(t, ts, admin, "q=after+turning"); len(got) != 0 {
		t.Errorf("%+v", got)
	}
}

// Levels change live and are kept for next time.
func TestLogLevelsLive(t *testing.T) {
	ts, admin := logServer(t)
	w := ts.do(t, "PUT", "/api/logs", admin, map[string]any{"levels": map[string]string{"transcode": "debug", "dvd": "info"}})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var v logsView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Levels["transcode"] != "debug" || v.Levels["discs"] != "info" || len(v.Categories) < 11 {
		t.Errorf("%+v", v)
	}
	ts.Log.Debug("live change works", "cat", "transcode")
	if got := logEntries(t, ts, admin, "q=live+change"); len(got) != 1 {
		t.Errorf("%+v", got)
	}
	saved := ts.DB.GetSetting(context.Background(), "logLevels", "")
	if !strings.Contains(saved, `"transcode":"debug"`) {
		t.Errorf("not saved: %s", saved)
	}
	if w := ts.do(t, "PUT", "/api/logs", admin, map[string]any{"levels": map[string]string{"tv": "loud"}}); w.Code != 400 {
		t.Errorf("a bad level: %d", w.Code)
	}
	// The older form: a list of what's on.
	if w := ts.do(t, "PUT", "/api/logs", admin, map[string]any{"on": []string{"dvd", "signin"}}); w.Code != 200 {
		t.Error(w.Code)
	}
	if l := ts.Logs.Levels(); l["discs"] != "debug" || l["auth"] != "debug" || l["transcode"] != "info" {
		t.Errorf("%+v", l)
	}
}

// The summary and bundle hold no secrets, wherever they are planted, and the
// bundle is admin's alone.
func TestSummaryAndBundleHoldNoSecrets(t *testing.T) {
	ts, admin := logServer(t)
	const key, pin = "PLANTEDTMDBKEYVALUE", "PLANTEDPINVALUE"
	ctx := context.Background()
	for k, v := range map[string]string{"tmdbKey": key, "tvdbKey": key, "tvdbPin": pin} {
		if err := ts.DB.SetSetting(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	ts.LogStartup(ctx)
	ts.Log.Warn("a metadata lookup failed", "err", "Get https://example.com/x/"+key+"/y: timeout")
	for _, plain := range []bool{true, false} {
		q := "?days=1"
		if !plain {
			q += "&redact=1"
		}
		w := ts.do(t, "GET", "/api/logs/bundle"+q, admin, nil)
		if w.Code != 200 || w.Header().Get("Content-Type") != "application/zip" {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, f := range zr.File {
			names[f.Name] = true
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			if bytes.Contains(b, []byte(key)) || bytes.Contains(b, []byte(pin)) {
				t.Errorf("%s holds a planted secret:\n%s", f.Name, b)
			}
		}
		if !names["summary.json"] || !names["README.txt"] {
			t.Errorf("%v", names)
		}
	}
	w := ts.do(t, "GET", "/api/logs/summary", admin, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), key) || !strings.Contains(w.Body.String(), `"secretsSet":["tmdbKey","tvdbKey","tvdbPin"]`) {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if text := logFiles(t, ts); strings.Contains(text, key) || strings.Contains(text, pin) || !strings.Contains(text, "server started") {
		t.Errorf("startup summary:\n%s", text)
	}
	if w := ts.do(t, "GET", "/api/logs/bundle/preview?days=1&redact=1", admin, nil); w.Code != 200 || strings.Contains(w.Body.String(), key) {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

func TestCrashRoutes(t *testing.T) {
	ts, admin := logServer(t)
	ts.mux.HandleFunc("GET /api/test-panic", func(http.ResponseWriter, *http.Request) { panic("boom") })
	ts.do(t, "GET", "/api/test-panic", "", nil)
	var list struct{ Crashes []logging.Crash }
	if err := json.Unmarshal(ts.do(t, "GET", "/api/logs/crashes", admin, nil).Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Crashes) != 1 || list.Crashes[0].Seen {
		t.Fatalf("%+v", list)
	}
	id := list.Crashes[0].ID
	if w := ts.do(t, "GET", "/api/logs/crashes/"+id, admin, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "boom") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/logs/crashes/"+id, "", nil); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	for _, bad := range []string{"..%2F..%2Fwms.db", "crash-x-fatal", "pending-1"} {
		if w := ts.do(t, "GET", "/api/logs/crashes/"+bad, admin, nil); w.Code != 404 {
			t.Errorf("%s: %d", bad, w.Code)
		}
	}
	if w := ts.do(t, "POST", "/api/logs/crashes/"+id+"/seen", admin, nil); w.Code != 200 {
		t.Error(w.Code)
	}
	if w := ts.do(t, "DELETE", "/api/logs/crashes/"+id, admin, nil); w.Code != 200 {
		t.Error(w.Code)
	}
	if ts.Crashes.List() != nil {
		t.Errorf("%+v", ts.Crashes.List())
	}
}

// What a request pays for being tied together and watched, and what direct play
// pays: compare with BenchmarkRequestBare.
func BenchmarkRequestLogged(b *testing.B) {
	l, err := logging.New(b.TempDir(), slog.LevelInfo, nil)
	if err != nil {
		b.Fatal(err)
	}
	defer l.Close()
	s := &Server{Logs: l, Log: slog.New(l.Handler())}
	h := s.logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	r := httptest.NewRequest("GET", "/api/items/5", nil)
	w := httptest.NewRecorder()
	b.ReportAllocs()
	for b.Loop() {
		h.ServeHTTP(w, r)
	}
}

func BenchmarkRequestBare(b *testing.B) {
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	r := httptest.NewRequest("GET", "/api/items/5", nil)
	w := httptest.NewRecorder()
	b.ReportAllocs()
	for b.Loop() {
		h.ServeHTTP(w, r)
	}
}

// A module that arrives later is noticed, and each has its own controls, live.
func TestModuleLogging(t *testing.T) {
	ts, admin := logServer(t)
	view := func() map[string]LogModule {
		w := ts.do(t, "GET", "/api/logs", admin, nil)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var v logsView
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		out := map[string]LogModule{}
		for _, m := range v.Modules {
			out[m.ID] = m
		}
		return out
	}
	mods := view()
	dvd, ok := mods["wittydvd"]
	if !ok || dvd.New || len(dvd.Topics) == 0 || dvd.Level != "" {
		t.Fatalf("%+v", mods)
	}
	if strings.Contains(logFiles(t, ts), "a new module was found") {
		t.Error("the very first look is the starting point, not news")
	}

	// A new one arrives: it's noticed, listed first, and said so in the log.
	lm := &ts.logMods
	lm.mu.Lock()
	delete(lm.state, "wittydvd")
	lm.mu.Unlock()
	mods = view()
	if !mods["wittydvd"].New {
		t.Fatalf("not noticed: %+v", mods["wittydvd"])
	}
	found := logEntries(t, ts, admin, "module=wittydvd&cat=modules")
	if len(found) != 1 || !strings.Contains(found[0].Msg, "a new module was found") {
		t.Errorf("%+v", found)
	}
	if ts.DB.GetSetting(context.Background(), logModulesKey, "") == "" {
		t.Error("not kept")
	}

	// A level of its own, and an option turned on, take effect at once.
	debug, off := "debug", "off"
	if w := ts.do(t, "PUT", "/api/logs", admin, map[string]any{"modules": map[string]moduleLogChange{"wittybluray": {Level: &debug}, "wittydvd": {Topics: map[string]bool{"nav": true, "made-up": true}}}, "modulesSeen": true}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	mods = view()
	if mods["wittydvd"].New || mods["wittybluray"].Level != "debug" {
		t.Errorf("%+v", mods)
	}
	on := map[string]bool{}
	for _, tp := range mods["wittydvd"].Topics {
		on[tp.ID] = tp.On
	}
	if !on["nav"] || on["menus"] || on["made-up"] {
		t.Errorf("%+v", on)
	}
	ts.Log.Debug("blu-ray detail", "cat", "discs", "module", "wittybluray")
	ts.Log.Debug("dvd navigation", "cat", "discs", "module", "wittydvd", "topic", "nav")
	ts.Log.Debug("dvd menus stay quiet", "cat", "discs", "module", "wittydvd", "topic", "menus")
	ts.Log.Debug("the area stays quiet", "cat", "discs")
	text := logFiles(t, ts)
	for _, want := range []string{"blu-ray detail", "dvd navigation"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, not := range []string{"dvd menus stay quiet", "the area stays quiet"} {
		if strings.Contains(text, not) {
			t.Errorf("%q should be quiet", not)
		}
	}
	// Bad requests change nothing.
	for _, body := range []map[string]any{
		{"modules": map[string]moduleLogChange{"nonesuch": {Level: &debug}}},
		{"modules": map[string]moduleLogChange{"wittydvd": {Level: &[]string{"loud"}[0]}, "wittybluray": {Level: &off}}},
	} {
		if w := ts.do(t, "PUT", "/api/logs", admin, body); w.Code != 400 {
			t.Errorf("%v: %d", body, w.Code)
		}
	}
	if view()["wittybluray"].Level != "debug" {
		t.Error("a refused change was half applied")
	}
	// Kept across a restart.
	ts.logMods = logModules{}
	ts.Logs.SetModules(nil)
	ts.syncLogModules(context.Background())
	ts.Log.Debug("still detailed after a restart", "cat", "discs", "module", "wittybluray")
	if !strings.Contains(logFiles(t, ts), "still detailed after a restart") {
		t.Error("forgotten")
	}
	// Non-admins can't change anything.
	_, adult := ts.user(t, "someone", "adult", false, "")
	if w := ts.do(t, "PUT", "/api/logs", adult, map[string]any{"modules": map[string]moduleLogChange{"wittydvd": {Level: &debug}}}); w.Code != 403 {
		t.Errorf("%d", w.Code)
	}
}

func TestRecordEverythingRoute(t *testing.T) {
	ts, admin := logServer(t)
	view := func(body map[string]any) (int, logsView) {
		w := ts.do(t, "PUT", "/api/logs", admin, body)
		var v logsView
		_ = json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v
	}
	code, v := view(map[string]any{"recordMinutes": 30})
	if code != 200 || v.RecordingUntil == nil {
		t.Fatalf("%d %+v", code, v.RecordingUntil)
	}
	ts.Log.Debug("caught", "cat", "playback")
	if got := logEntries(t, ts, admin, "q=caught"); len(got) != 1 {
		t.Errorf("%+v", got)
	}
	if v.Levels["playback"] != "warn" {
		t.Errorf("settings changed: %v", v.Levels)
	}
	code, v = view(map[string]any{"recordMinutes": 0})
	if code != 200 || v.RecordingUntil != nil {
		t.Errorf("%d %+v", code, v.RecordingUntil)
	}
	if code, _ := view(map[string]any{"recordMinutes": 100000}); code != 400 {
		t.Errorf("%d", code)
	}
	_, adult := ts.user(t, "someone", "adult", false, "")
	if w := ts.do(t, "PUT", "/api/logs", adult, map[string]any{"recordMinutes": 30}); w.Code != 403 {
		t.Errorf("%d", w.Code)
	}
}

func TestSessionFromPath(t *testing.T) {
	for p, want := range map[string]string{
		"/api/hls/abc123/seg/5":   "abc123",
		"/api/hls/abc123":         "abc123",
		"/api/dvd/sid9/menu/1/x":  "sid9",
		"/api/items/5/play":       "",
		"/api/share/token/hls/1":  "",
		"/api/hls/bad%0Aname/seg": "",
	} {
		if got := sessionFromPath(p); got != want {
			t.Errorf("%s: %q, want %q", p, got, want)
		}
	}
}
