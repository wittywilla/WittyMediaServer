// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "testing"

// A book is read, not played: its title header says Read, Continue reading or Read again.
func TestBookButtonsAreAboutReading(t *testing.T) {
	label := func(acts []any, id string) string {
		for _, a := range acts {
			if m, ok := a.(map[string]any); ok && m["id"] == id {
				return m["label"].(string)
			}
		}
		return ""
	}
	cases := []struct {
		it          Item
		play, again string
	}{
		{Item{ID: "1", Kind: "book", PlayID: "1", Can: []string{"play"}}, "Read", ""},
		{Item{ID: "1", Kind: "book", PlayID: "1", Can: []string{"play"}, ResumeSec: 40}, "Continue reading", "Read from the start"},
		{Item{ID: "1", Kind: "book", PlayID: "1", Can: []string{"play"}, Watched: true}, "Read again", ""},
		{Item{ID: "2", Kind: "movie", PlayID: "2", Can: []string{"play"}, ResumeSec: 40}, "Resume", "Play from the start"},
	}
	for _, c := range cases {
		acts := headerActions(&c.it, []string{"play", "restart"})
		if got := label(acts, "play"); got != c.play {
			t.Errorf("%s resume %v watched %v: play button says %q, want %q (%#v)", c.it.Kind, c.it.ResumeSec, c.it.Watched, got, c.play, acts)
		}
		if got := label(acts, "restart"); got != c.again {
			t.Errorf("%s resume %v: restart says %q, want %q", c.it.Kind, c.it.ResumeSec, got, c.again)
		}
	}
}
