// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bdsession

import (
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
)

func TestOpeningADiscOnlyEvictsYourOwnFirst(t *testing.T) {
	now := time.Now()
	session := func(id string, owner int64, age time.Duration) *Session {
		return &Session{ID: id, prefs: dvdsession.Prefs{Owner: owner}, last: now.Add(-age)}
	}
	all := map[string]*Session{}
	for i := range maxSessionsPerAccount {
		s := session(string(rune('a'+i)), 7, time.Duration(i)*time.Minute)
		all[s.ID] = s
	}
	all["other"] = session("other", 8, time.Hour)
	if got := whoMakesWay(all, 7); got == nil || got.ID != "d" {
		t.Errorf("account 7 opening another: %+v", got)
	}
	if got := whoMakesWay(all, 8); got != nil {
		t.Errorf("account 8 opening another: %+v", got)
	}
}
