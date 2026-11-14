// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import "testing"

func TestItemTitlesForDropsRepeats(t *testing.T) {
	if s := itemTitlesFor("", "mega game", "Mega Game"); s != "" {
		t.Errorf("%q", s)
	}
	if s := itemTitlesFor("メガゲーム", "Mega Game", "Mega Game"); s != `{"native":"メガゲーム"}` {
		t.Errorf("%q", s)
	}
	if readItemTitles(`{}`, "x") != nil || readItemTitles(`nope`, "x") != nil || readItemTitles("", "x") != nil {
		t.Error("nothing known should be nil")
	}
}
