// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package dvdsession

import (
	"testing"
	"time"
)

func TestOpeningADiscOnlyEvictsYourOwnFirst(t *testing.T) {
	now := time.Now()
	session := func(id string, owner int64, age time.Duration) *Session {
		return &Session{ID: id, prefs: Prefs{Owner: owner}, used: now.Add(-age)}
	}
	all := map[string]*Session{}
	for i := range 4 {
		s := session(string(rune('a'+i)), 7, time.Duration(i)*time.Minute)
		all[s.ID] = s
	}
	other := session("other", 8, time.Hour)
	all[other.ID] = other
	// Account 7 is at its limit, and goes first: its own oldest, not someone else's older one.
	if got := whoMakesWay(all, 7); got == nil || got.ID != "d" {
		t.Errorf("account 7 opening another: %+v", got)
	}
	// Someone with room, while the server has room: nobody goes.
	if got := whoMakesWay(all, 8); got != nil {
		t.Errorf("account 8 opening another: %+v", got)
	}
	for i := range maxSessions - len(all) {
		s := session(string(rune('m'+i)), int64(100+i), time.Minute)
		all[s.ID] = s
	}
	// The server is full: whoever has left theirs longest.
	if got := whoMakesWay(all, 8); got == nil || got.ID != "other" {
		t.Errorf("a full server: %+v", got)
	}
}
