// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package connectsteps

import (
	"strings"
	"testing"
)

func TestTheListHoldsTogether(t *testing.T) {
	want := []string{Read, Found, Secure, SignIn}
	steps := Steps()
	if len(steps) != len(want) {
		t.Fatalf("steps: %v", steps)
	}
	for i, s := range steps {
		if s.ID != want[i] || s.Title == "" || s.Failed == "" {
			t.Errorf("step %d: %+v", i, s)
		}
	}
	seen := map[string]bool{}
	reasons := map[string]string{}
	for _, c := range Codes() {
		if seen[c.Code] {
			t.Errorf("%s twice", c.Code)
		}
		seen[c.Code] = true
		if c.Code != "unknown" && StepIndex(c.Step) < 0 {
			t.Errorf("%s: no step %q", c.Code, c.Step)
		}
		if c.Say == "" || c.Next == "" || !strings.HasSuffix(c.Say, ".") || !strings.HasSuffix(c.Next, ".") {
			t.Errorf("%s: say and next are whole sentences: %q %q", c.Code, c.Say, c.Next)
		}
		for _, m := range c.Match {
			if m != strings.ToLower(m) {
				t.Errorf("%s: match %q isn't lower case", c.Code, m)
			}
		}
		for _, r := range c.Reasons {
			if other, ok := reasons[r]; ok {
				t.Errorf("reason %q is both %s and %s", r, other, c.Code)
			}
			reasons[r] = c.Code
		}
	}
	if !seen["unknown"] {
		t.Error("no unknown")
	}
}

// The words the connector and the server really say, and what each is.
func TestClassify(t *testing.T) {
	cases := []struct{ reason, text, step, code, wantStep string }{
		{"expired", "", Found, "expired", SignIn},
		{"used", "That invite was used", "", "used", SignIn},
		{"too-many-tries", "Too many tries. Wait a minute and try again.", "", "too-many-tries", SignIn},
		{"off", "This server takes no invites.", "", "off", SignIn},
		{"wrong", "That code isn't right.", "", "wrong", SignIn},
		{"unreachable", "", Found, "not-found", Found},
		{"", "not an invite link", Read, "invite-bad", Read},
		{"", "this device has no connection key", Read, "no-key", Read},
		{"", "the server wasn't found in the public address book: context deadline exceeded", Found, "not-found", Found},
		{"", "the server didn't answer in time", Found, "timeout", Found},
		{"", "no server took the code in time", Found, "timeout", Found},
		{"", "the server was found, but a direct connection through both routers couldn't be made, and no community relay is available to carry it", Secure, "no-direct-path", Secure},
		{"", "the server refused the connection (is this device still on its list?): stream reset", Secure, "not-listed", SignIn},
		{"", "the relay said no (full)", Found, "relay-full", Found},
		{"", "the relay said no (key)", Found, "relay-refused", Found},
		{"", "the relay couldn't be reached: connection refused", Found, "relay-unreachable", Found},
		{"", "joining by a code needs the address book, which is turned off on this server", Found, "dht-off", Found},
		{"", "dial tcp: lookup example: no such host", Found, "offline", Found},
		{"", "something new and odd", Secure, "unknown", Secure},
		{"made-up", "", SignIn, "unknown", SignIn},
	}
	for _, c := range cases {
		f := Classify(c.reason, c.text, c.step)
		if f.Code != c.code || f.Step != c.wantStep || f.Say == "" || f.Next == "" {
			t.Errorf("Classify(%q, %q, %q) = %+v, want %s at %s", c.reason, c.text, c.step, f, c.code, c.wantStep)
		}
	}
	if f := Of("expired"); f.Step != SignIn || f.Say == "" {
		t.Errorf("Of: %+v", f)
	}
	if f := Of("nope"); f.Code != "unknown" {
		t.Errorf("Of(nope): %+v", f)
	}
}
