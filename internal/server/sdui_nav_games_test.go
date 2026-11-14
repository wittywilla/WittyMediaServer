// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

func TestGamesBecomeWittyPlay(t *testing.T) {
	libs := []sdui.NavLibrary{{ID: 3, Name: "Anime", Kind: "anime"}, {ID: 7, Name: "Games", Kind: "games"}}
	items := []sdui.NavEntry{{ID: "home"}, {ID: "lib-3", Label: "Anime", Placement: "primary"}, {ID: "lib-7", Label: "Games", Placement: "primary"},
		{ID: "settings"}, {ID: wittyPlayNavID, Label: "Games", Placement: "more"}}
	got, _ := gamesBecomeWittyPlay(items, nil, libs)
	ids := ""
	for _, e := range got {
		ids += e.ID + ","
		if e.ID == wittyPlayNavID && e.Placement != "primary" {
			t.Errorf("WittyPlay took placement %q, want the library's primary", e.Placement)
		}
	}
	if ids != "home,lib-3,"+wittyPlayNavID+",settings," {
		t.Fatalf("got %s", ids)
	}
	// Then the label rule has nothing left to drop.
	got, _ = dropModuleEntriesLibrariesCover(got, nil, nil)
	if len(got) != 4 {
		t.Fatalf("dedupe dropped WittyPlay: %d entries", len(got))
	}
	// WittyPlay on a phone's bar, the games library in More: Games goes on the bar.
	phone := []sdui.NavEntry{{ID: "home"}, {ID: "lib-7", Label: "Games", Placement: "more"}, {ID: wittyPlayNavID, Label: "Games", Placement: "primary"}}
	if got, _ := gamesBecomeWittyPlay(phone, nil, libs); len(got) != 2 || got[1].Placement != "primary" {
		t.Errorf("WittyPlay's own primary: %+v", got)
	}
	// A games entry an administrator hid stays hidden.
	hid := []sdui.NavEntry{{ID: "home"}, {ID: "lib-7", Label: "Games", Placement: "hidden"}, {ID: wittyPlayNavID, Label: "Games", Placement: "primary"}}
	if got, _ := gamesBecomeWittyPlay(hid, nil, libs); got[1].Placement != "hidden" {
		t.Errorf("a hidden games entry: %+v", got)
	}
	// Without WittyPlay, nothing changes.
	plain := items[:4]
	if got, _ := gamesBecomeWittyPlay(plain, nil, libs); len(got) != 4 {
		t.Fatalf("changed a menu without WittyPlay: %d", len(got))
	}
}
