package server

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

// A stolen session looking around this machine's own network through "Look at this store".
func TestPreviewOnThisMachineAsksForThePassword(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	dir := filepath.Join(t.TempDir(), "modules")
	m := modmgr.New(modmgr.Config{Dir: dir, Server: "0.6.0-alpha", Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(m.Close)
	ts.Modules = m
	ts.ModuleStores = modmgr.NewStores(modmgr.StoresConfig{Manager: m, Dir: dir, Transport: library.SafeClient.Transport, Log: slog.New(slog.DiscardHandler)})

	var asked atomic.Int32
	inside := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked.Add(1); w.WriteHeader(401) }))
	defer inside.Close()
	body := []byte(`{"url":"` + inside.URL + `/catalog.json","private":true}`)
	w := ts.do(t, "POST", "/api/modulestore/stores/preview", admin, body)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "password-needed") {
		t.Errorf("a look on this machine with no password: %d %s", w.Code, w.Body)
	}
	if asked.Load() != 0 {
		t.Error("it fetched from this machine before asking for the password")
	}
	right := []byte(`{"url":"` + inside.URL + `/catalog.json","private":true,"password":"correct-horse-1"}`)
	if w := ts.do(t, "POST", "/api/modulestore/stores/preview", admin, right); w.Code == 403 {
		t.Errorf("with the password: %d %s", w.Code, w.Body)
	}
	if asked.Load() == 0 {
		t.Error("the right password should let an admin look at a store of their own")
	}
}

// Every way of writing this machine's address, through the transport the server really uses, with
// a store that was never allowed a private address. None may connect.
func TestAPublicStoreCantReachThisMachine(t *testing.T) {
	var asked atomic.Int32
	inside := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { asked.Add(1) }))
	defer inside.Close()
	port := inside.URL[strings.LastIndexByte(inside.URL, ':'):]

	dir := filepath.Join(t.TempDir(), "modules")
	m := modmgr.New(modmgr.Config{Dir: dir, Server: "0.6.0-alpha", Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(m.Close)
	s := modmgr.NewStores(modmgr.StoresConfig{Manager: m, Dir: dir, Transport: library.SafeClient.Transport, Log: slog.New(slog.DiscardHandler)})
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	key := base64.StdEncoding.EncodeToString(pub)
	for _, host := range []string{"127.0.0.1", "127.1", "0177.0.0.1", "0x7f.0.0.1", "0x7f.1", "2130706433", "017700000001", "[::1]", "[::ffff:127.0.0.1]", "[::ffff:7f00:1]", "[0:0:0:0:0:ffff:7f00:1]", "[64:ff9b::7f00:1]", "[2002:7f00:1::]",
		"localhost", "LOCALHOST", "localhost.", "foo.localhost", "ip6-localhost", "0.0.0.0", "[::]", "169.254.169.254", "[fe80::1]", "[fe80::1%25lo]", "100.64.0.1", "10.0.0.1", "192.168.0.1"} {
		if _, err := s.Preview(context.Background(), "https://"+host+port+"/catalog.json", key, false); err == nil {
			t.Errorf("%s: it answered", host)
		}
	}
	if asked.Load() != 0 {
		t.Errorf("%d requests reached this machine", asked.Load())
	}
}
