// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mobile

import (
	"encoding/json"
	"strings"
	"testing"
)

// What the bindings give Kotlin and Swift, short of a connection (the package
// underneath tests those): keys, invites, and a session that fails cleanly.
func TestKeysInvitesAndBadConfigs(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	id, err := DeviceID(key)
	if err != nil || !strings.HasPrefix(id, "12D3KooW") {
		t.Fatalf("%q %v", id, err)
	}
	link := "wittymedia://p2p/" + id + "?s=abcdefghijklmnopqrstuv&n=Home&a=%2Fip4%2F203.0.113.4%2Ftcp%2F4100"
	raw, err := ParseInvite(link)
	if err != nil {
		t.Fatal(err)
	}
	var in struct {
		ServerID, Secret, Name string
		Addrs                  []string
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil || in.ServerID != id || in.Secret != "abcdefghijklmnopqrstuv" || in.Name != "Home" || len(in.Addrs) != 1 {
		t.Fatalf("%s %v", raw, err)
	}
	if _, err := ParseInvite("https://example.com"); err == nil {
		t.Fatal("any link was read as an invite")
	}
	for _, bad := range []string{"", "{", `{"key":"x","serverId":"y"}`, `{"key":"` + key + `","serverId":"` + id + `"}`} {
		if s, err := Start(bad, nil); err == nil {
			s.Close()
			t.Fatalf("started from %q", bad)
		}
	}
	// A closed session answers, and does nothing.
	s := &Session{}
	s.Close()
	s.NetworkChanged()
	if s.Port() != 0 || s.Secret() != "" || s.KnownAddrs() != "[]" || !strings.Contains(s.State(), "offline") {
		t.Fatal("a closed session should say it is closed")
	}
	if _, err := s.Enrol("x", "y", "z"); err == nil {
		t.Fatal("enrolled on a closed session")
	}
}
