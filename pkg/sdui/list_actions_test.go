// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "testing"

// A List's row carries its item's own buttons as a card does (a friend's Invite, a room's Join): at
// most four, each with a label and an action, a glyph only from the shared pad map.
func TestListRowsCarryTheirItemsActions(t *testing.T) {
	r := newRenderer(&Page{}, &RenderContext{})
	invite := &Action{Type: ActCommand, Op: "wittyplay.friend.invite", Item: "2"}
	it := &Item{ID: "profile-2", Kind: "person", Title: "alex", Actions: []ItemAction{
		{ID: "invite", Label: "Invite", Glyph: "x", Action: invite},
		{ID: "odd", Label: "Odd glyph", Glyph: "z", Action: invite},
		{ID: "none", Label: "No action"},
		{ID: "3", Label: "Three", Action: invite}, {ID: "4", Label: "Four", Action: invite}, {ID: "5", Label: "Five", Action: invite},
	}}
	n := r.listItem("friends", it)
	acts, ok := n.Props["actions"].([]ItemAction)
	if !ok || len(acts) != 4 || acts[0].ID != "invite" || acts[0].Glyph != "x" || acts[0].Action.Op != "wittyplay.friend.invite" || acts[1].Glyph != "" || acts[2].ID != "3" {
		t.Fatalf("a row's actions: %#v", n.Props["actions"])
	}
	if n.Action != nil {
		t.Errorf("a row with only buttons opens something: %+v", n.Action)
	}
	if n := r.listItem("friends", &Item{ID: "profile-3", Kind: "person", Title: "sam"}); n.Props["actions"] != nil {
		t.Errorf("a row with no buttons has actions: %#v", n.Props)
	}
}
