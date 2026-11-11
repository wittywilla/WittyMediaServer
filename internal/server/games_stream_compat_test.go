// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymodule/service"
)

// oldWittyPlay stands in for a WittyPlay from before version 2 of the stream: it answers what
// version 1 needs, and has no Session.Connect and no /stream route, as 0.3.4 hasn't.
func oldWittyPlay(t *testing.T, owner int64) {
	t.Helper()
	answer := func(w http.ResponseWriter, rets ...any) {
		var raw []json.RawMessage
		for _, r := range rets {
			b, _ := json.Marshal(r)
			raw = append(raw, b)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rets": raw})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /call/{name}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("name") {
		case "stream.Manager.Get":
			var b struct {
				Args []json.RawMessage `json:"args"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			if len(b.Args) == 1 && string(b.Args[0]) == `"0123abcd"` {
				answer(w, map[string]string{"id": "0123abcd"})
			} else {
				answer(w, nil)
			}
		case "stream.Session.UserID":
			answer(w, owner)
		case "stream.Session.ID":
			answer(w, "0123abcd")
		case "stream.Manager.Modes":
			// As 0.3.4 says them: no "stream".
			answer(w, []map[string]any{{"id": "webrtc", "video": []string{"h264"}, "audio": "opus", "input": "datachannel"}, {"id": "ws-h264", "video": []string{"h264"}, "audio": "pcm", "input": "websocket"}})
		case "stream.Session.Answer":
			answer(w, "v=0 an answer")
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"err": "this version has no \"" + r.PathValue("name") + "\"", "code": "unknown-method"})
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("an older WittyPlay was sent %s %s, which it has no route for", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base, _ := url.Parse(srv.URL)
	conn := &service.Conn{Base: base, Client: srv.Client()}
	dial, proxy := service.Dial, service.Proxy
	service.Dial = func(string) (*service.Conn, error) { return conn, nil }
	service.Proxy = func(_ string, w http.ResponseWriter, r *http.Request, path string) error {
		r.URL.Path = path
		mux.ServeHTTP(w, r)
		return nil
	}
	t.Cleanup(func() { service.Dial, service.Proxy = dial, proxy })
}

// A server whose WittyPlay is older than version 2 of the stream: version 1 plays as it always
// has, version 2 isn't offered, and a client that asks for it anyway is told what to update.
func TestAnOlderWittyPlayKeepsVersion1AndSaysWhatVersion2Needs(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &library.Store{DB: d}
	s := &Server{Cfg: &config.Config{DataDir: t.TempDir(), Dev: true}, DB: d, Log: log, Store: store, Auth: &auth.Service{DB: d},
		Tools: &ffmpeg.Tools{FFmpeg: "ffmpeg", Encoders: map[string]bool{}}, Scanner: library.NewScanner(store, nil, t.TempDir(), func() string { return "" }, log)}
	u, err := s.Auth.CreateUser(ctx, "admin", "a long enough password", true)
	if err != nil {
		t.Fatal(err)
	}
	tok, _, err := s.Auth.Login(ctx, "admin", "a long enough password", "test", false)
	if err != nil {
		t.Fatal(err)
	}
	oldWittyPlay(t, u.ID)
	s.games() // as starting a game would
	t.Cleanup(s.StopGames)
	h := s.Handler()
	do := func(method, path string, body any, header ...string) *httptest.ResponseRecorder {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, rd)
		req.RemoteAddr = "192.168.1.50:4000"
		req.Header.Set("Authorization", "Bearer "+tok)
		for i := 0; i+1 < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}

	// Version 1, unchanged.
	if w := do("POST", "/api/play/0123abcd/webrtc", map[string]any{"sdp": "v=0 an offer"}); w.Code != 200 || !strings.Contains(w.Body.String(), "an answer") {
		t.Fatalf("version 1's offer: %d %s", w.Code, w.Body.String())
	}
	// Version 2 isn't offered.
	if w := do("GET", "/api/games/modes?client=native", nil); w.Code != 200 || strings.Contains(w.Body.String(), `"stream"`) {
		t.Fatalf("modes: %d %s", w.Code, w.Body.String())
	}
	// Asked for anyway, it says what's needed: not a failure, and not "no such game".
	for name, w := range map[string]*httptest.ResponseRecorder{
		"the offer": do("POST", "/api/play/0123abcd/stream", map[string]any{"hello": map[string]any{"v": 2}, "sdp": "v=0 an offer"}),
		"the WebSocket": do("GET", "/api/play/0123abcd/stream", nil,
			"Connection", "Upgrade", "Upgrade", "websocket", "Sec-WebSocket-Version", "13", "Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ=="),
	} {
		var b struct{ Code, Error string }
		_ = json.Unmarshal(w.Body.Bytes(), &b)
		if w.Code != 501 || b.Code != "needs-update" || !strings.Contains(b.Error, "WittyPlay") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
	// A wrong hello is the client's mistake whichever WittyPlay is in.
	if w := do("POST", "/api/play/0123abcd/stream", map[string]any{"hello": map[string]any{"v": 1}, "sdp": "x"}); w.Code != 400 || !strings.Contains(w.Body.String(), `"bad-hello"`) {
		t.Errorf("a version 1 hello: %d %s", w.Code, w.Body.String())
	}
	// Someone else's game is still nobody's business.
	if w := do("POST", "/api/play/ffff0000/stream", map[string]any{"hello": map[string]any{"v": 2}, "sdp": "x"}); w.Code != 404 {
		t.Errorf("an unknown game: %d %s", w.Code, w.Body.String())
	}
}
