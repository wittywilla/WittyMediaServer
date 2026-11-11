// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package logging

import (
	"strings"
	"testing"
	"time"
)

// Log lines hold whatever clients and files said, and support bundles are read back through these.
func FuzzLogText(f *testing.F) {
	for _, s := range []string{
		`time=2026-01-01T00:00:00Z level=INFO cat=http msg="a \"b\"" x=1`, "token=abc&password=def", "https://u:p@h/live/a/b/1.ts", `{"recoveryCodes":["a","b"]}`,
		strings.Repeat("%74", 100) + "=x", strings.Repeat("a=", 2000), "\x1b[31m\n\r‮",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		start := time.Now()
		ParseLine(s)
		out := Redact(s)
		if d := time.Since(start); d > 2*time.Second {
			t.Fatalf("took %v on %d bytes", d, len(s))
		}
		// Something that has been hidden stays hidden.
		if again := Redact(out); strings.Count(again, hidden) < strings.Count(out, hidden) {
			t.Fatalf("%q then %q", out, again)
		}
	})
}
