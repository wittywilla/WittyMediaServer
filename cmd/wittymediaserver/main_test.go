// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"strings"
	"testing"
)

// A misspelt command must not start a server: in a container, that's a second
// server on the same data.
func TestUnknownCommandDoesntServe(t *testing.T) {
	for _, word := range []string{"hardwar", "helth", "start"} {
		if got := run([]string{word}); got != 2 {
			t.Errorf("%s: exit %d", word, got)
		}
	}
}

func TestPUIDMismatch(t *testing.T) {
	if msg := puidMismatch("", "", 1000, 1000); msg != "" {
		t.Errorf("nothing set: %q", msg)
	}
	if msg := puidMismatch("1000", "1000", 1000, 1000); msg != "" {
		t.Errorf("matching: %q", msg)
	}
	if msg := puidMismatch("1026", "100", 1000, 1000); !strings.Contains(msg, `user: "1026:100"`) {
		t.Errorf("both: %q", msg)
	}
	if msg := puidMismatch("", "100", 1026, 1000); !strings.Contains(msg, `user: "1026:100"`) {
		t.Errorf("only PGID: %q", msg)
	}
}
