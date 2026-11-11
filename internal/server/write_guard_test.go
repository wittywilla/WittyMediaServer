// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
)

func withWriteStall(t *testing.T, d time.Duration) {
	t.Helper()
	saved := writeStall
	writeStall = d
	t.Cleanup(func() { writeStall = saved })
}

// bigFileServer serves one large file the way the web app's files are served, and says when a
// connection is closed.
func bigFileServer(t *testing.T, size int) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	ts := newTestServer(t)
	ts.Web = fstest.MapFS{"index.html": {Data: []byte("<html></html>")}, "assets/big.bin": {Data: make([]byte, size)}}
	srv := httptest.NewUnstartedServer(ts.Handler())
	closed := make(chan struct{}, 16)
	srv.Config.ConnState = func(_ net.Conn, st http.ConnState) {
		if st == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, closed
}

// A client that asks for a file and stops reading is let go. It held a connection for good.
func TestStalledClientIsLetGo(t *testing.T) {
	withWriteStall(t, 300*time.Millisecond)
	srv, closed := bigFileServer(t, 64<<20)
	c, err := net.DialTCP("tcp", nil, srv.Listener.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadBuffer(4096)
	fmt.Fprintf(c, "GET /assets/big.bin HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection is still held by a client that isn't reading")
	}
}

// What counts is whether the client is reading, not how long the whole thing takes.
func TestSlowClientGetsItAll(t *testing.T) {
	withWriteStall(t, time.Second)
	const size = 32 << 20
	srv, _ := bigFileServer(t, size)
	res, err := http.Get(srv.URL + "/assets/big.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	start, got := time.Now(), 0
	buf := make([]byte, 1<<20)
	for {
		n, err := io.ReadFull(res.Body, buf)
		got += n
		if err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got != size {
		t.Errorf("%d of %d bytes", got, size)
	}
	if took := time.Since(start); took < 1200*time.Millisecond {
		t.Errorf("this took %v, so it says nothing about a download longer than the limit", took)
	}
}

// A socket lasts as long as it lasts.
func TestWebSocketOutlivesTheWriteLimit(t *testing.T) {
	withWriteStall(t, 100*time.Millisecond)
	var s Server
	h := s.limits(s.logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer c.CloseNow()
		for {
			typ, b, err := c.Read(r.Context())
			if err != nil {
				return
			}
			if err := c.Write(r.Context(), typ, b); err != nil {
				t.Error(err)
				return
			}
		}
	})))
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	time.Sleep(400 * time.Millisecond)
	if err := c.Write(ctx, websocket.MessageText, []byte("still here")); err != nil {
		t.Fatal(err)
	}
	if _, b, err := c.Read(ctx); err != nil || string(b) != "still here" {
		t.Fatalf("%q %v", b, err)
	}
}

// A handler that thinks for longer than the limit before answering still gets its answer out.
func TestSlowHandlerStillAnswers(t *testing.T) {
	withWriteStall(t, 100*time.Millisecond)
	var s Server
	for name, respond := range map[string]func(http.ResponseWriter){
		"with a body":  func(w http.ResponseWriter) { _, _ = w.Write([]byte("done")) },
		"with a code":  func(w http.ResponseWriter) { w.WriteHeader(204) },
		"with nothing": func(http.ResponseWriter) {},
	} {
		srv := httptest.NewServer(s.limits(s.logging(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(300 * time.Millisecond)
			respond(w)
		}))))
		res, err := http.Get(srv.URL)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		} else {
			res.Body.Close()
		}
		srv.Close()
	}
}

func TestMediaRoutesMayPauseForLonger(t *testing.T) {
	for path, want := range map[string]time.Duration{
		"/api/items/5/file":                   mediaStall,
		"/api/v1/items/5/file":                mediaStall,
		"/api/items/5/stream":                 mediaStall,
		"/api/items/5/original":               mediaStall,
		"/api/items/5/motion":                 mediaStall,
		"/api/share/abc/file/5":               mediaStall,
		"/api/share/abc/original/5":           mediaStall,
		"/api/cd/5/audio":                     mediaStall,
		"/api/radio/5/stream":                 mediaStall,
		"/api/items/5":                        writeStall,
		"/api/items/5/poster":                 writeStall,
		"/api/items/x/file":                   writeStall,
		"/assets/big.js":                      writeStall,
		"/api/logs/bundle":                    writeStall,
		"/api/hls/x/seg/1":                    writeStall,
		"/api/items/5/file/../../../settings": writeStall,
	} {
		if got := stallLimit(path); got != want {
			t.Errorf("%s: %v, want %v", path, got, want)
		}
	}
}

// recordingWriter sees how the guard hands data on.
type recordingWriter struct {
	httptest.ResponseRecorder
	biggest int
	sources []io.Reader
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	w.biggest = max(w.biggest, len(b))
	return w.ResponseRecorder.Write(b)
}

func (w *recordingWriter) ReadFrom(r io.Reader) (int64, error) {
	w.sources = append(w.sources, r)
	return io.Copy(writerOnly{&w.ResponseRecorder}, r)
}

type writerOnly struct{ io.Writer }

// A big write goes in pieces, so one slow one doesn't count against all of it.
func TestGuardSplitsLargeWrites(t *testing.T) {
	rw := &recordingWriter{ResponseRecorder: *httptest.NewRecorder()}
	g := newWriteGuard(rw, "/x")
	data := make([]byte, 3*writeChunk+5)
	n, err := g.Write(data)
	if err != nil || n != len(data) || rw.Body.Len() != len(data) {
		t.Fatalf("%d %v %d", n, err, rw.Body.Len())
	}
	if rw.biggest > writeChunk {
		t.Errorf("a write of %d", rw.biggest)
	}
}

// A file reaches the system's copy still as a file, so sending it costs what it did.
func TestGuardKeepsTheSystemsOwnCopy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f")
	data := []byte(strings.Repeat("x", 3*writeChunk+5))
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rw := &recordingWriter{ResponseRecorder: *httptest.NewRecorder()}
	g := newWriteGuard(rw, "/x")
	src := &io.LimitedReader{R: f, N: int64(len(data) - 2)}
	n, err := g.ReadFrom(src)
	if err != nil || n != int64(len(data)-2) || rw.Body.Len() != len(data)-2 || src.N != 0 {
		t.Fatalf("%d %v %d left %d", n, err, rw.Body.Len(), src.N)
	}
	if len(rw.sources) < 3 {
		t.Errorf("sent in %d pieces", len(rw.sources))
	}
	for _, s := range rw.sources {
		lr, ok := s.(*io.LimitedReader)
		if !ok || lr.R != io.Reader(f) {
			t.Errorf("handed on as %T, which the system can't send by itself", s)
		}
	}
	// Anything else that isn't limited comes out whole too.
	rw = &recordingWriter{ResponseRecorder: *httptest.NewRecorder()}
	g = newWriteGuard(rw, "/x")
	if n, err := g.ReadFrom(strings.NewReader(string(data))); err != nil || n != int64(len(data)) || rw.Body.Len() != len(data) {
		t.Fatalf("%d %v %d", n, err, rw.Body.Len())
	}
}
