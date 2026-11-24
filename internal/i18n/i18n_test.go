// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package i18n

import (
	"errors"
	"testing"
)

func TestF(t *testing.T) {
	cases := []struct {
		in   string
		args []any
		want string
	}{
		{"Plain", nil, "Plain"},
		{"Hello, {name}", []any{"name", "Ana"}, "Hello, Ana"},
		{"{a} and {b}", []any{"b", 2, "a", 1}, "1 and 2"},
		{"Kept {unknown}", []any{"x", 1}, "Kept {unknown}"},
		{"Broken {", []any{"x", 1}, "Broken {"},
		{"Why: {why}", []any{"why", errors.New("disk full")}, "Why: disk full"},
		{"Nil: {why}", []any{"why", error(nil)}, "Nil: <nil>"}, // as fmt writes it, so a Sprintf made into F says the same
	}
	for _, c := range cases {
		if got := F(c.in, c.args...); got != c.want {
			t.Errorf("F(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if T("Same") != "Same" {
		t.Error("T changed its text")
	}
	if Error("Can't open {name}", "name", "a.mkv").Error() != "Can't open a.mkv" {
		t.Error("Error didn't fill its names")
	}
}

func TestN(t *testing.T) {
	if got := N("{count} album", "{count} albums", 1); got != "1 album" {
		t.Errorf("one: %q", got)
	}
	if got := N("{count} album", "{count} albums", 3); got != "3 albums" {
		t.Errorf("more: %q", got)
	}
	if got := N("{count} song by {name}", "{count} songs by {name}", 2, "name", "Ana"); got != "2 songs by Ana" {
		t.Errorf("with a name: %q", got)
	}
}
