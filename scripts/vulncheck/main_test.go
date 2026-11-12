// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"strings"
	"testing"
)

const config = `{"config":{"protocol_version":"v1.0.0","scanner_name":"govulncheck"}}`

func stream(parts ...string) string { return config + "\n" + strings.Join(parts, "\n") }

func reached(id, module, version, fixed string) string {
	f := `{"finding":{"osv":"` + id + `","fixed_version":"` + fixed + `","trace":[{"module":"` + module + `","version":"` + version + `","package":"` + module + `/x","function":"Vuln"},` +
		`{"module":"github.com/wittywilla/wittymediaserver","package":"github.com/wittywilla/wittymediaserver/internal/p2p","function":"Start","position":{"filename":"internal/p2p/node.go","line":12}}]}}`
	return f
}

func imported(id, module string) string {
	return `{"finding":{"osv":"` + id + `","trace":[{"module":"` + module + `","version":"v1.0.0","package":"` + module + `/x"}]}}`
}

// What the check lets through, and what it doesn't: an accepted advisory only in its own module
// and only while nothing fixes it; everything else this code reaches fails the build.
func TestOnlyTheAcceptedAdvisoryIsLetThrough(t *testing.T) {
	const dht = "github.com/libp2p/go-libp2p-kad-dht"
	osv := `{"osv":{"id":"GO-2024-3218","summary":"Content censorship"}}`
	for name, c := range map[string]struct {
		in       string
		problems int
		accepted bool
	}{
		"the DHT advisory, no fix":        {stream(osv, reached("GO-2024-3218", dht, "v0.43.0", "")), 0, true},
		"reached twice, counted once":     {stream(reached("GO-2024-3218", dht, "v0.43.0", ""), reached("GO-2024-3218", dht, "v0.43.0", "")), 0, true},
		"the same advisory, a fix exists": {stream(reached("GO-2024-3218", dht, "v0.43.0", "v0.44.0")), 1, false},
		"the same ID in another module":   {stream(reached("GO-2024-3218", "github.com/someone/else", "v1.0.0", "")), 1, false},
		"anything else reached":           {stream(reached("GO-2026-6859", "stdlib", "v1.27.3", "v1.27.4")), 1, false},
		"only imported, not called":       {stream(imported("GO-2026-6859", "stdlib")), 0, false},
		"nothing at all":                  {stream(), 0, false},
		"accepted beside one that fails":  {stream(reached("GO-2024-3218", dht, "v0.43.0", ""), reached("GO-2026-5932", "golang.org/x/crypto", "v0.58.0", "v0.58.1")), 1, true},
	} {
		problems, ok, err := check(strings.NewReader(c.in))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(problems) != c.problems || (len(ok) == 1) != c.accepted {
			t.Errorf("%s: %d problems %+v, accepted %v", name, len(problems), problems, ok)
		}
	}
}

// Anything but govulncheck's JSON is an error, never a pass.
func TestOtherInputIsAnError(t *testing.T) {
	for name, in := range map[string]string{
		"its text output": "=== Symbol Results ===\nNo vulnerabilities found.\n",
		"empty":           "",
		"JSON, not its":   `{"something":"else"}`,
	} {
		if _, _, err := check(strings.NewReader(in)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// The account of a finding names where this repository's code reaches it.
func TestTheTraceSaysWhere(t *testing.T) {
	problems, _, _ := check(strings.NewReader(stream(reached("GO-2026-6859", "stdlib", "v1.27.3", "v1.27.4"))))
	if len(problems) != 1 || !strings.Contains(problems[0].via, "internal/p2p/node.go:12") {
		t.Fatalf("%+v", problems)
	}
}
