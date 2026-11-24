// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// Taking a device off the list is "this device is out": its connections
// close, and the sign-ins it used from away end too, so its token is no use
// however else it reaches the server. Sign-ins the account has elsewhere stay.
func TestP2PRevokingADeviceEndsItsSignIns(t *testing.T) {
	for _, devicesOnly := range []bool{true, false} {
		t.Run(map[bool]string{true: "devices only", false: "anyone reaches the sign-in page"}[devicesOnly], func(t *testing.T) {
			ts, admin := p2pServer(t, nil)
			if !devicesOnly {
				if w := ts.do(t, "PUT", "/api/p2p/settings", admin, map[string]any{"enabled": true, "devicesOnly": false, "accept": true}); w.Code != 200 {
					t.Fatalf("%d %s", w.Code, w.Body)
				}
			}
			u, onLaptop := ts.user(t, "kate", "adult", false, "")
			atHome, _, err := ts.Auth.IssueSession(context.Background(), u.ID, "TV at home")
			if err != nil {
				t.Fatal(err)
			}
			app := p2pApp(t, ts, 2)
			enrol(t, ts, onLaptop, app, "Kate's laptop")
			if code, body, err := over(t, app, "GET", "/api/sessions", onLaptop, nil); err != nil || code != 200 {
				t.Fatalf("signed in from away: %d %s %v", code, body, err)
			}

			if w := ts.do(t, "DELETE", "/api/p2p/devices/"+app.ID(), onLaptop, nil); w.Code != 200 {
				t.Fatalf("removing it: %d %s", w.Code, w.Body)
			}
			if w := ts.do(t, "GET", "/api/sessions", onLaptop, nil); w.Code != 401 {
				t.Fatalf("the revoked device's sign-in still works at home: %d", w.Code)
			}
			if code, _, err := over(t, app, "GET", "/api/sessions", onLaptop, nil); err == nil && code != 401 {
				t.Fatalf("the revoked device's sign-in still works from away: %d", code)
			}
			if w := ts.do(t, "GET", "/api/sessions", atHome, nil); w.Code != 200 {
				t.Fatalf("a sign-in the device never used was ended too: %d", w.Code)
			}
		})
	}
}

// A sign-in used from two devices (its token was copied) ends when either is revoked.
func TestP2PSessionsRememberEachDeviceTheyWereUsedFrom(t *testing.T) {
	ts := newTestServer(t)
	u, _ := ts.user(t, "kate", "adult", false, "")
	tok, _, err := ts.Auth.IssueSession(context.Background(), u.ID, "laptop")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	a, _ := p2p.IDFromSeed(p2pSeedFor(2))
	b, _ := p2p.IDFromSeed(p2pSeedFor(3))
	ts.Auth.MarkP2PDevice(ctx, tok, a)
	ts.Auth.MarkP2PDevice(ctx, tok, a)
	ts.Auth.MarkP2PDevice(ctx, tok, b)
	if n, err := ts.Auth.EndP2PDevice(ctx, "12D3KooW"); err != nil || n != 0 {
		t.Fatalf("a prefix of an ID ended %d sign-ins: %v", n, err)
	}
	if n, err := ts.Auth.EndP2PDevice(ctx, b); err != nil || n != 1 {
		t.Fatalf("revoking the second device ended %d: %v", n, err)
	}
	if _, err := ts.Auth.Resolve(ctx, tok); err == nil {
		t.Fatal("the sign-in outlived a device it was used from")
	}
	for _, bad := range []string{"", "a,b"} {
		ts.Auth.MarkP2PDevice(ctx, tok, bad)
		if n, _ := ts.Auth.EndP2PDevice(ctx, bad); n != 0 {
			t.Fatalf("%q ended %d", bad, n)
		}
	}
}
