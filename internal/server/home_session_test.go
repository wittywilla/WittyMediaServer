// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"testing"
)

// A password-free profile works at home. Its sign-in used to keep working from the internet for 90
// days, for a visitor who tapped Guest once on the Wi-Fi, or a page that took the token.
func TestPasswordFreeSignInStaysHome(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	putRemote(t, h, tok, map[string]any{"enabled": true})
	u, err := s.Auth.CreateProfile(context.Background(), "Visitor", "", "guest", false, "")
	if err != nil {
		t.Fatal(err)
	}
	r := call(h, "POST", "/api/login", lan, map[string]string{"name": "Visitor"})
	var in struct {
		Token string `json:"token"`
	}
	r.json(t, &in)
	if in.Token == "" {
		t.Fatalf("%d %s", r.code, r.body)
	}
	bearer := "Authorization: Bearer " + in.Token
	if r := call(h, "GET", "/api/me", public, nil, "TLS: yes", bearer); r.code != 401 {
		t.Errorf("from the internet: %d %s", r.code, r.body)
	}
	if r := call(h, "GET", "/api/me", lan, nil, bearer); r.code != 200 {
		t.Errorf("at home: %d %s", r.code, r.body)
	}

	// A password makes an ordinary sign-in, only for sign-ins made with it.
	if err := s.Auth.SetPassword(context.Background(), u.ID, "1234"); err != nil {
		t.Fatal(err)
	}
	r = call(h, "POST", "/api/login", public, map[string]string{"name": "Visitor", "password": "1234"}, "TLS: yes")
	in.Token = ""
	r.json(t, &in)
	if in.Token == "" {
		t.Fatalf("with a PIN: %d %s", r.code, r.body)
	}
	if r := call(h, "GET", "/api/me", public, nil, "TLS: yes", "Authorization: Bearer "+in.Token); r.code != 200 {
		t.Errorf("a PIN works away from home: %d %s", r.code, r.body)
	}

	// Taking the password away keeps what was signed in at home from then on.
	if err := s.Auth.ClearPassword(context.Background(), u.ID); err != nil {
		t.Fatal(err)
	}
	if r := call(h, "GET", "/api/me", public, nil, "TLS: yes", "Authorization: Bearer "+in.Token); r.code != 401 {
		t.Errorf("after the password went: %d %s", r.code, r.body)
	}
}
