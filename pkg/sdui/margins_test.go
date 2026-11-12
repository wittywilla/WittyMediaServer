// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"strconv"
	"strings"
	"testing"
)

// Side margins: the safe area on a television, the m spacing elsewhere; the content width
// the conformance kit checks, the grid's columns and what the reference says all agree.
func TestSideMarginsAgreeEverywhere(t *testing.T) {
	cases := []struct {
		ff, class   string
		window      int
		margin      int
		contentWide int
	}{
		{FormTV, "", 0, 90, 1740},
		{FormTV, "4k", 0, 90, 1740},
		{FormPhone, "", 390, 16, 358},
		{FormTablet, "", 1024, 16, 992},
		{FormTablet, "", 820, 16, 788},
		{FormDesktop, "", 1440, 16, 1408},
		{FormWeb, "", 1280, 16, 1248},
	}
	for _, tc := range cases {
		l := ProfileFor(tc.ff, tc.class)
		if tc.window > 0 {
			l = l.WithViewport(Viewport{Width: tc.window, Height: 800})
		}
		if got := l.SideMargin(); got != tc.margin {
			t.Errorf("%s %s: margin %d, want %d", tc.ff, tc.class, got, tc.margin)
		}
		if got := l.ContentWidth(); got != tc.contentWide {
			t.Errorf("%s %s at %d: content %d, want %d", tc.ff, tc.class, tc.window, got, tc.contentWide)
		}
	}
	page := componentSpec("Page")
	if page == nil {
		t.Fatal("no Page in the catalogue")
	}
	m := strconv.Itoa(ProfileFor(FormTablet, "").SideMargin())
	if !strings.Contains(page.Renders["tablet"], m+"-point side margins") {
		t.Errorf("the reference says the tablet's margins are %q; the profile has %s", page.Renders["tablet"], m)
	}
}

func componentSpec(name string) *ComponentSpec {
	for i, c := range Catalog().Components {
		if c.Type == name {
			return &Catalog().Components[i]
		}
	}
	return nil
}
