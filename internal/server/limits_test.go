package server

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// What a stranger can make the server hold on to is small, whatever the route. A valid
// request padded past the limit is refused: looking at the answer alone can't tell that
// from a bad one, but these would all go through if they were read to the end.
func TestPublicRoutesTakeOnlySmallBodies(t *testing.T) {
	ts := newTestServer(t)
	ts.user(t, "owner", "adult", true, "")
	nonce := base64.StdEncoding.EncodeToString(make([]byte, 16))
	padded := func(field string) string { return "{" + strings.Repeat(" ", maxCredentialBytes+1024) + field + "}" }
	for _, c := range []struct{ path, field string }{
		{"/api/identity", `"nonce":"` + nonce + `"`},
		{"/api/v1/identity", `"nonce":"` + nonce + `"`},
		{"/api/pair/start", `"device":"tv"`},
		{"/api/v1/pair/start", `"device":"tv"`},
		{"/api/pair/poll", `"deviceSecret":"` + strings.Repeat("a", 64) + `"`},
		{"/api/pair/redeem", `"code":"1"`},
		{"/api/login", `"name":"owner","password":"correct-horse-1"`},
	} {
		over := ts.do(t, "POST", c.path, "", []byte(padded(c.field)))
		if over.Code != 400 && over.Code != 413 {
			t.Errorf("%s with a body over the limit: %d %s", c.path, over.Code, over.Body)
		}
		if ok := ts.do(t, "POST", c.path, "", []byte("{"+c.field+"}")); ok.Body.String() == over.Body.String() {
			t.Errorf("%s reads the same whether it fits or not: %d %s", c.path, ok.Code, ok.Body)
		}
	}
	if !publicBody("/api/setup") || !publicBody("/api/logout") || publicBody("/api/users") || publicBody("/api/identity/x") {
		t.Error("the list of public bodies is wrong")
	}
}

// A new public route is held to the small limit by being listed.
func TestEveryPublicWriteHasTheSmallLimit(t *testing.T) {
	for route := range publicRoutes {
		method, path, _ := strings.Cut(route, " ")
		if method != "GET" && (!publicBody(path) || !publicBody(strings.Replace(path, "/api/", "/api/v1/", 1))) {
			t.Errorf("%s takes a body from anyone and isn't held to the small limit", route)
		}
	}
}

// A stranger who starts a request and stops sending gives up their connection soon.
func TestPublicBodyHasAShortDeadline(t *testing.T) {
	saved := publicBodyTime
	publicBodyTime = 200 * time.Millisecond
	defer func() { publicBodyTime = saved }()
	ts := newTestServer(t)
	srv := httptest.NewServer(ts.h)
	defer srv.Close()
	c, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "POST /api/identity HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Type: application/json\r\nContent-Length: 1000\r\n\r\n{")
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	n, _ := c.Read(buf)
	if n == 0 {
		t.Fatal("the connection is still being held")
	}
	if !bytes.Contains(buf[:n], []byte("400")) {
		t.Errorf("%s", buf[:n])
	}
}

// The deadline is for the body to arrive. A request still at work past it isn't cancelled: this
// is what ended a slow core install after exactly two minutes.
func TestLongRequestOutlivesTheBodyDeadline(t *testing.T) {
	saved := publicBodyTime
	publicBodyTime = 100 * time.Millisecond
	defer func() { publicBodyTime = saved }()
	var s Server
	srv := httptest.NewServer(s.limits(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
			w.WriteHeader(http.StatusTeapot)
		case <-time.After(400 * time.Millisecond):
		}
	})))
	defer srv.Close()
	for name, body := range map[string]io.Reader{"no body": nil, "a body": strings.NewReader(`{}`)} {
		res, err := http.Post(srv.URL+"/api/logout", "application/json", body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: the request was cancelled at the deadline", name)
		}
	}
}

// Pictures are read whole into memory, so no more than a few at once. The answer comes
// before any of the body is read.
func TestPictureUploadsTakeASlotFirst(t *testing.T) {
	g := newGameServer(t)
	for range cap(importSlots) {
		importSlots <- struct{}{}
	}
	defer func() {
		for range cap(importSlots) {
			<-importSlots
		}
	}()
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/game-bezels?name=a.png"},
		{"PUT", "/api/game-background/image"},
	} {
		r := httptest.NewRequest(c.method, c.path, &failingBody{t: t})
		r.RemoteAddr = "192.168.1.50:4000"
		r.Header.Set("Authorization", "Bearer "+g.other)
		w := httptest.NewRecorder()
		g.h.ServeHTTP(w, r)
		if w.Code != 503 {
			t.Errorf("%s: %d %s", c.path, w.Code, w.Body)
		}
	}
}

type failingBody struct{ t *testing.T }

func (f *failingBody) Read([]byte) (int, error) {
	f.t.Error("the body was read before a slot was taken")
	return 0, fmt.Errorf("stop")
}

// Ordinary JSON has an ordinary limit, for anyone signed in.
func TestJSONBodiesAreLimited(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	padded := func(n int) []byte { return []byte("{" + strings.Repeat(" ", n) + `"rule":{}}`) }
	if w := ts.do(t, "POST", "/api/collections/preview", tok, padded(2<<20)); w.Code != 400 {
		t.Errorf("past the limit: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", "/api/collections/preview", tok, padded(512<<10)); w.Code == 400 {
		t.Errorf("within it: %d %s", w.Code, w.Body)
	}
	// The one route where a long body is the point, for an administrator.
	r := httptest.NewRequest("POST", "/", strings.NewReader("{"+strings.Repeat(" ", 2<<20)+`"playlist":"x"}`))
	var b struct {
		Playlist string `json:"playlist"`
	}
	if err := readJSONUpTo(r, &b, maxPlaylistBody); err != nil || b.Playlist != "x" {
		t.Errorf("%v %+v", err, b)
	}
}
