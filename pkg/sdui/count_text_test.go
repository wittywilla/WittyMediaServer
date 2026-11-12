// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "testing"

// A FilterBar's count comes in words every app can show as they are: one title is a title.
func TestHowManyTitlesInWords(t *testing.T) {
	for n, want := range map[int]string{0: "0 titles", 1: "1 title", 2: "2 titles", 1200: "1200 titles"} {
		if got := titlesText(n); got != want {
			t.Errorf("%d: %q, want %q", n, got, want)
		}
	}
}
