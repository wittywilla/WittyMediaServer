// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyconnect

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// As everywhere in the peer-to-peer tests: both ends are on this machine and
// know only each other.

var loopback = []string{"/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/tcp/0"}

type list struct {
	mu sync.Mutex
	ok map[string]bool
}

func (l *list) Allowed(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.ok[id]
}
func (l *list) Seen(string, string) {}

func seed(b byte) []byte { return bytes.Repeat([]byte{b}, p2p.SeedSize) }

func serverHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "host=%s secret=%q cookies=%q xff=%q origin=%q", r.Host, r.Header.Get(SecretHeader), r.Header.Get("Cookie"), r.Header.Get("X-Forwarded-For"), r.Header.Get("Origin"))
	})
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := range 2 {
			fmt.Fprintf(w, "data: %d\n\n", i)
			w.(http.Flusher).Flush()
			time.Sleep(300 * time.Millisecond)
		}
	})
	mux.HandleFunc("/socket", func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		line, _ := buf.ReadString('\n')
		_, _ = io.WriteString(conn, "echo "+line)
	})
	mux.HandleFunc("/video", func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "v.bin", time.Time{}, bytes.NewReader(bytes.Repeat([]byte("abcdefgh"), 1<<16)))
	})
	return mux
}

func start(t *testing.T) (*Connector, *p2p.Node, *list) {
	t.Helper()
	devs := &list{ok: map[string]bool{}}
	node, err := p2p.StartNode(p2p.NodeConfig{Seed: seed(1), Devices: devs, DevicesOnly: true, Handler: serverHandler(), Local: true, Listen: loopback})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(node.Close)
	key := p2p.EncodeSeed(seed(2))
	id, _ := DeviceID(key)
	devs.ok[id] = true
	c, err := Start(Config{Key: key, ServerID: node.ID(), Hints: node.Addrs(), NoDHT: true, NoRelay: true,
		Tune: func(cc *p2p.ClientConfig) { cc.Local, cc.Listen = true, loopback }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c, node, devs
}

func fetch(t *testing.T, c *Connector, path string, hdr ...string) (int, string, http.Header) {
	t.Helper()
	req, _ := http.NewRequest("GET", c.URL()+path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "Host" {
			req.Host = hdr[i+1]
		} else {
			req.Header.Add(hdr[i], hdr[i+1])
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b), res.Header
}

func TestTheLocalAddressNeedsTheSecret(t *testing.T) {
	c, _, _ := start(t)
	if !strings.HasPrefix(c.URL(), "http://127.0.0.1:") || len(c.Secret()) < 30 {
		t.Fatalf("address %s, secret of %d characters", c.URL(), len(c.Secret()))
	}
	// Another program on this computer: it can reach the port, and that is all.
	for _, hdr := range [][]string{nil, {SecretHeader, "guess"}, {"Cookie", SecretCookie + "=guess"}, {SecretHeader, ""}} {
		if code, body, _ := fetch(t, c, "/echo", hdr...); code != 403 || strings.Contains(body, "host=") {
			t.Fatalf("with %v: %d %q", hdr, code, body)
		}
	}
	// A web page elsewhere, pointing a name of its own at 127.0.0.1.
	if code, _, _ := fetch(t, c, "/echo", SecretHeader, c.Secret(), "Host", "evil.example:"+fmt.Sprint(c.Port())); code != 403 {
		t.Fatalf("another host name was accepted: %d", code)
	}

	// With the secret, by header or by cookie, the request reaches the server:
	// without the secret in it, without any forwarded header, with the app's
	// other cookies and its own Host.
	code, body, _ := fetch(t, c, "/echo", SecretHeader, c.Secret(), "X-Forwarded-For", "127.0.0.1", "Cookie", "session=abc")
	if code != 200 || !strings.Contains(body, "host=127.0.0.1:") || !strings.Contains(body, `secret=""`) || !strings.Contains(body, `cookies="session=abc"`) || !strings.Contains(body, `xff=""`) {
		t.Fatalf("by header: %d %s", code, body)
	}
	code, body, _ = fetch(t, c, "/echo", "Cookie", "session=abc; "+SecretCookie+"="+c.Secret()+"; theme=dark")
	if code != 200 || strings.Contains(body, c.Secret()) || !strings.Contains(body, "session=abc") || !strings.Contains(body, "theme=dark") {
		t.Fatalf("by cookie: %d %s", code, body)
	}
}

func TestStreamsRangesAndSocketsPassThrough(t *testing.T) {
	c, _, _ := start(t)
	// A range of a video.
	code, body, hdr := fetch(t, c, "/video", SecretHeader, c.Secret(), "Range", "bytes=8-23")
	if code != 206 || body != "abcdefghabcdefgh" || hdr.Get("Content-Range") != "bytes 8-23/524288" {
		t.Fatalf("range: %d %q %q", code, body, hdr.Get("Content-Range"))
	}
	// Server-sent events arrive one by one.
	req, _ := http.NewRequest("GET", c.URL()+"/events", nil)
	req.Header.Set(SecretHeader, c.Secret())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	first, _ := bufio.NewReader(res.Body).ReadString('\n')
	if first != "data: 0\n" || time.Since(began) > 250*time.Millisecond {
		t.Fatalf("the first event %q came after %v: it was held back", first, time.Since(began))
	}
	res.Body.Close()
	// An upgraded connection, as a WebSocket is.
	conn, err := net.Dial("tcp", strings.TrimPrefix(c.URL(), "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(conn, "GET /socket HTTP/1.1\r\nHost: 127.0.0.1:%d\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nCookie: %s=%s\r\n\r\n", c.Port(), SecretCookie, c.Secret())
	br := bufio.NewReader(conn)
	if status, _ := br.ReadString('\n'); !strings.Contains(status, "101") {
		t.Fatalf("upgrade answered %q", status)
	}
	for {
		if l, err := br.ReadString('\n'); err != nil || l == "\r\n" {
			break
		}
	}
	fmt.Fprint(conn, "hello\n")
	if l, _ := br.ReadString('\n'); l != "echo hello\n" {
		t.Fatalf("through the socket: %q", l)
	}
}

func TestErrorsAreSaidPlainly(t *testing.T) {
	c, node, devs := start(t)
	if code, _, _ := fetch(t, c, "/echo", SecretHeader, c.Secret()); code != 200 {
		t.Fatalf("%d", code)
	}
	// Taken off the server's list: the app is told, as a page or as JSON.
	devs.mu.Lock()
	devs.ok = map[string]bool{}
	devs.mu.Unlock()
	node.Kick(c.DeviceID())
	code, body, hdr := fetch(t, c, "/echo", SecretHeader, c.Secret(), "Accept", "text/html")
	if code != 502 || !strings.Contains(body, "<h1>") || !strings.Contains(hdr.Get("Content-Type"), "text/html") || hdr.Get("X-Witty-Connect-Error") != "1" {
		t.Fatalf("as a page: %d %s", code, body)
	}
	code, body, hdr = fetch(t, c, "/echo", SecretHeader, c.Secret())
	if code != 502 || !strings.HasPrefix(body, `{"error":"`) || hdr.Get("Content-Type") != "application/json" {
		t.Fatalf("as JSON: %d %s", code, body)
	}
	// Closed, the port is closed.
	c.Close()
	if _, err := net.DialTimeout("tcp", strings.TrimPrefix(c.URL(), "http://"), time.Second); err == nil {
		t.Fatal("the local address still answers after Close")
	}
}

func TestKeys(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	id, err := DeviceID(key)
	if err != nil || !strings.HasPrefix(id, "12D3KooW") {
		t.Fatalf("%q %v", id, err)
	}
	if _, err := DeviceID("not a key"); err == nil {
		t.Fatal("rubbish was read as a key")
	}
	if _, err := Start(Config{Key: "rubbish", ServerID: id}); err == nil {
		t.Fatal("started without a key")
	}
	if _, err := Start(Config{Key: key, ServerID: id}); err == nil {
		t.Fatal("a device can't be its own server")
	}
	_ = context.Background()
}
