// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
)

var fromTheHomeNetwork = []string{"RemoteAddr", "192.168.1.20:1", "Host", "192.168.1.5:9797"}

// A path naming another machine makes the server sign in there with its own account, on Windows
// the machine's. That's for the owner to decide, not for whoever has an administrator's session.
func TestAdminPathsRefuseOtherMachines(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	for _, share := range []string{`\\attacker.example\share\x`, `//attacker.example/share/x`, `\\?\UNC\attacker.example\share`, `\\.\pipe\x`, `\/attacker.example/share`} {
		for _, c := range []struct {
			what, method, path string
			body               any
		}{
			{"library", "POST", "/api/libraries", map[string]any{"name": "L", "type": "movies", "paths": []string{share}}},
			{"certificate", "PUT", "/api/remote", map[string]any{"httpsMode": "cert", "certFile": share, "keyFile": "/a/key.pem"}},
			{"key", "PUT", "/api/remote", map[string]any{"httpsMode": "cert", "certFile": "/a/cert.pem", "keyFile": share}},
			{"log folder", "PUT", "/api/logs", map[string]any{"dir": share}},
			{"recordings", "PUT", "/api/tv/settings", map[string]any{"dvrFolder": share}},
			{"browser", "GET", "/api/fs?path=" + strings.ReplaceAll(strings.ReplaceAll(share, `\`, "%5C"), "?", "%3F"), nil},
		} {
			w := ts.do(t, c.method, c.path, tok, c.body, fromTheHomeNetwork...)
			if w.Code < 400 || !strings.Contains(w.Body.String(), "another machine") {
				t.Errorf("%s %s: %d %s", c.what, share, w.Code, w.Body)
			}
		}
	}
	if got, _ := ts.Store.Libraries(context.Background()); len(got) != 0 {
		t.Errorf("%+v", got)
	}
}

// First-time setup lays out the ways forward from a share, so it has to be able to tell this
// refusal from any other, for a folder typed in and for one being browsed to.
func TestAnotherMachineSaysSoWithACode(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	share := `\\nas.example\media`
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/libraries", map[string]any{"name": "L", "type": "movies", "paths": []string{share}}},
		{"GET", "/api/fs?path=" + strings.ReplaceAll(share, `\`, "%5C"), nil},
	} {
		w := ts.do(t, c.method, c.path, tok, c.body, fromTheHomeNetwork...)
		var got struct {
			Code  string `json:"code"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Code != "other-machine" {
			t.Errorf("%s %s: %d %s", c.method, c.path, w.Code, w.Body)
		}
		for _, way := range []string{"own desktop", "drive letter", "allowNetworkPaths"} {
			if !strings.Contains(got.Error, way) {
				t.Errorf("%s: no mention of %q: %s", c.path, way, got.Error)
			}
		}
	}
	// Any other refusal has no such code, so the page doesn't offer the wrong help.
	w := ts.do(t, "POST", "/api/libraries", tok, map[string]any{"name": "L", "type": "movies", "paths": []string{"relative/folder"}}, fromTheHomeNetwork...)
	if w.Code < 400 || strings.Contains(w.Body.String(), "other-machine") {
		t.Errorf("an ordinary refusal: %d %s", w.Code, w.Body)
	}
}

// Someone at the server itself may choose a share, as may a server whose owner said so. A library
// already on a share can still be edited.
func TestNetworkPathsWhenTheOwnerSaysSo(t *testing.T) {
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	share := `//attacker.example/share/x`
	body := map[string]any{"name": "L", "type": "movies", "paths": []string{share}}
	// Refused for not existing here, not for being a share.
	if w := ts.do(t, "POST", "/api/libraries", tok, body); strings.Contains(w.Body.String(), "another machine") {
		t.Errorf("at the server itself: %d %s", w.Code, w.Body)
	}
	ts.Cfg.AllowNetworkPaths = true
	if w := ts.do(t, "POST", "/api/libraries", tok, body, fromTheHomeNetwork...); strings.Contains(w.Body.String(), "another machine") {
		t.Errorf("with the setting: %d %s", w.Code, w.Body)
	}
	ts.Cfg.AllowNetworkPaths = false
	r := httptest.NewRequest("PUT", "/api/libraries/1", nil)
	r.RemoteAddr, r.Host = "192.168.1.20:1", "192.168.1.5:9797"
	if err := ts.checkAdminPaths(r, []string{share}, []string{share}); err != nil {
		t.Errorf("a share it already has: %v", err)
	}
	if err := ts.checkAdminPaths(r, []string{share}, nil); err == nil {
		t.Error("a new share")
	}
}

// A stream's address can hold a login, and what a connection answered says what's on the network.
// Neither is for the page.
func TestTunerErrorsHoldNoAddress(t *testing.T) {
	ts := newTestServer(t)
	_, adult := ts.user(t, "viewer", "adult", false, "")
	const secret = "USERNAME/PASSWORD"
	c := tunerChannel(t, livetv.Channel{Number: "1", Name: "One", Enabled: true}, "http://127.0.0.1:1/live/"+secret+"/1.ts")
	key := "tun-" + strconv.FormatInt(c.ID, 10)
	w := ts.do(t, "GET", "/api/tv/"+key+"/tune", adult, nil)
	if w.Code < 400 || strings.Contains(w.Body.String(), "PASSWORD") || strings.Contains(w.Body.String(), "127.0.0.1") || strings.Contains(w.Body.String(), "refused") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}
