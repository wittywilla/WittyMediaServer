// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net"
	"slices"
	"testing"
)

// First-run setup's last page shows where the server is (#536). It once showed nothing at all when the server listened
// on this computer only; the home-network address is there as a guess even then.
func TestHomeURLs(t *testing.T) {
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("172.20.0.1"), net.ParseIP("192.168.1.20"), net.ParseIP("8.8.4.4"), net.ParseIP("fd00::1")}
	main := net.ParseIP("192.168.1.20")
	for _, c := range []struct {
		name, listen string
		main         net.IP
		urls         []string
		guess        string
		local        bool
	}{
		{"every address, the main one first", ":9797", main, []string{"http://192.168.1.20:9797", "http://172.20.0.1:9797"}, "http://192.168.1.20:9797", false},
		{"no main one known", "0.0.0.0:9797", nil, []string{"http://172.20.0.1:9797", "http://192.168.1.20:9797"}, "http://172.20.0.1:9797", false},
		{"loopback only: no address answers, but the guess is there", "127.0.0.1:9797", main, []string{}, "http://192.168.1.20:9797", true},
		{"a name for loopback", "localhost:9797", main, []string{}, "http://192.168.1.20:9797", true},
		{"one home address", "172.20.0.1:9797", main, []string{"http://172.20.0.1:9797"}, "http://172.20.0.1:9797", false},
	} {
		urls, guess, local := homeURLs(c.listen, "http", "9797", ips, c.main)
		if !slices.Equal(urls, c.urls) || guess != c.guess || local != c.local {
			t.Errorf("%s: got %v %q %v, want %v %q %v", c.name, urls, guess, local, c.urls, c.guess, c.local)
		}
	}
	if urls, guess, _ := homeURLs(":9797", "https", "", []net.IP{net.ParseIP("10.0.0.5")}, nil); !slices.Equal(urls, []string{"https://10.0.0.5"}) || guess != "https://10.0.0.5" {
		t.Errorf("no port: %v %q", urls, guess)
	}
	if urls, guess, _ := homeURLs(":9797", "http", "9797", []net.IP{net.ParseIP("8.8.4.4")}, nil); len(urls) != 0 || guess != "" {
		t.Errorf("a public address is never offered: %v %q", urls, guess)
	}
}
