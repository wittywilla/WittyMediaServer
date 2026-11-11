// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package stream

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wittywilla/wittymodule/service"
)

// A game that can't start says whose fault that is: the helper's own (it went away, or
// answered nonsense), or the game's, its core's or its console's (anything the helper said).
// Only the first may count against the module's version. The helper here is a stand-in that
// answers the way a real one does over the wire.
func TestStartSaysWhenTheHelperItselfFailed(t *testing.T) {
	for _, c := range []struct {
		name   string
		answer func(w http.ResponseWriter)
		helper bool
		is     error
	}{
		{"the runner ended", func(w http.ResponseWriter) {
			_, _ = w.Write([]byte(`{"err":"the game runner ended by itself: signal: segmentation fault","code":"stream.ErrRunnerDied"}`))
		}, false, ErrRunnerDied},
		{"a core refused", func(w http.ResponseWriter) {
			_, _ = w.Write([]byte(`{"err":"the core could not load this game"}`))
		}, false, nil},
		{"no graphics card", func(w http.ResponseWriter) {
			_, _ = w.Write([]byte(`{"err":"this server has no usable graphics card for this game: hardware-rendered games run on Windows and Linux servers"}`))
		}, false, nil},
		{"the helper went away mid-call", func(w http.ResponseWriter) {
			conn, _, err := http.NewResponseController(w).Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}, true, nil},
		{"the helper answered nonsense", func(w http.ResponseWriter) {
			_, _ = w.Write([]byte(`not an answer`))
		}, true, nil},
		{"the helper's service failed", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusInternalServerError)
		}, true, nil},
		{"the helper answered without a game", func(w http.ResponseWriter) {
			_, _ = w.Write([]byte(`{"rets":[null]}`))
		}, true, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/call/stream.Manager.Start") {
					http.NotFound(w, r)
					return
				}
				c.answer(w)
			}))
			defer ts.Close()
			useService(t, ts.URL)
			_, err := (&Manager{}).Start(Options{Console: "n64"})
			if err == nil {
				t.Fatal("it started")
			}
			if got := errors.Is(err, ErrHelper); got != c.helper {
				t.Errorf("helper's fault = %v, want %v: %v", got, c.helper, err)
			}
			if c.is != nil && !errors.Is(err, c.is) {
				t.Errorf("lost what the helper said: %v", err)
			}
		})
	}
}

// A helper that can't be reached at all was counted by whatever tried to start it, so
// that error isn't marked a second time.
func TestStartDoesNotMarkAHelperThatNeverStarted(t *testing.T) {
	refused := errors.New("the helper could not start")
	old := service.Dial
	service.Dial = func(string) (*service.Conn, error) { return nil, refused }
	t.Cleanup(func() { service.Dial = old })
	_, err := (&Manager{}).Start(Options{})
	if !errors.Is(err, refused) || errors.Is(err, ErrHelper) {
		t.Fatalf("%v", err)
	}
}

func useService(t *testing.T, base string) {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	if host, _, _ := net.SplitHostPort(u.Host); host != "127.0.0.1" {
		t.Fatalf("not on this machine: %s", base)
	}
	old := service.Dial
	service.Dial = func(string) (*service.Conn, error) {
		return &service.Conn{Base: u, Token: "test", Client: http.DefaultClient}, nil
	}
	t.Cleanup(func() { service.Dial = old })
}
