// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"slices"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// Every row says enough for a client to lay out its games and build its game menu.
func TestEveryConsoleRowIsComplete(t *testing.T) {
	eras := []string{eraFirst, era8Bit, era16Bit, eraEarly3D, eraDisc, eraPocket}
	looks := []string{lookClean, lookScanlines, lookLCD}
	for id, row := range consoleTraitsTable {
		if !slices.Contains(eras, row.Era) {
			t.Errorf("%s: era %q", id, row.Era)
		}
		if row.Media != mediaCart && row.Media != mediaPocket && row.Media != mediaDisc {
			t.Errorf("%s: media %q", id, row.Media)
		}
		if row.Launched < 1970 || row.Ended < row.Launched {
			t.Errorf("%s: years %d to %d", id, row.Launched, row.Ended)
		}
		c := row.Caps
		if c.Input.Ports < 1 || c.Screens.Count < 1 || c.Picture.Aspect == "" || !slices.Contains(looks, c.Picture.Look) {
			t.Errorf("%s: %+v", id, c)
		}
		if c.Input.Multitap != 0 && c.Input.Multitap <= c.Input.Ports {
			t.Errorf("%s: a multitap with no more players than ports", id)
		}
		if c.Screens.Count == 2 {
			if c.Screens.Touch == 0 || len(c.Screens.Options) == 0 {
				t.Errorf("%s: two screens with no touch screen or no way to lay them out", id)
			}
			for core, o := range c.Screens.Options {
				for _, l := range c.Screens.Layouts {
					if _, ok := o.Values[l]; !ok || o.Layout == "" {
						t.Errorf("%s/%s: no value for layout %s", id, core, l)
					}
				}
			}
		}
	}
}

// The consoles the user chose: 8-bit, 16-bit and the handhelds whose states are small enough to
// keep 20 seconds of in the memory a server allows by default. Not the 3D and disc consoles.
func TestRewindIsOnByDefaultForTheOlderConsoles(t *testing.T) {
	for _, id := range []string{"nes", "sms", "snes", "md", "pce", "gb", "gba", "gg", "a2600", "a7800"} {
		if !rewindByDefault(id) {
			t.Errorf("%s should rewind by default", id)
		}
	}
	for _, id := range []string{"n64", "ps1", "ps2", "dc", "nds", "3ds", "psp", "made-up"} {
		if rewindByDefault(id) {
			t.Errorf("%s shouldn't rewind by default", id)
		}
	}
}

// The default sits in the everyone-on-this-console layer when nobody has chosen, and an
// administrator's choice for everyone or for the console always wins.
func TestTheRewindDefaultGivesWayToAnAdministrator(t *testing.T) {
	server, console := &wittyplay.Layer{}, &wittyplay.Layer{}
	withRewindDefault("nes", server, console)
	if console.Values[rewindKey] != true {
		t.Fatalf("not on by default: %v", console.Values)
	}
	server, console = &wittyplay.Layer{Values: map[string]any{rewindKey: false}}, &wittyplay.Layer{}
	withRewindDefault("nes", server, console)
	if _, set := console.Values[rewindKey]; set {
		t.Error("an administrator turned it off for everyone, and the default came back")
	}
	server, console = &wittyplay.Layer{}, &wittyplay.Layer{Values: map[string]any{rewindKey: false}}
	withRewindDefault("nes", server, console)
	if console.Values[rewindKey] != false {
		t.Error("an administrator turned it off for NES, and the default came back")
	}
	server, console = &wittyplay.Layer{}, &wittyplay.Layer{}
	withRewindDefault("ps2", server, console)
	if len(console.Values) != 0 {
		t.Errorf("a disc console got the default: %v", console.Values)
	}
}

// With the module here, every console it knows has a row.
func TestEveryConsoleTheModuleKnowsHasARow(t *testing.T) {
	modtest.Need(t, "wittyplay")
	for _, c := range wittyplay.All() {
		if c.Hidden || c.ID == wittyplay.TestConsole || traitsFor(c) != nil {
			continue
		}
		t.Errorf("no row for %s (%s)", c.ID, c.Name)
	}
}

// Every console with a row has a case and a cartridge (or disc) to draw, of a known kind and shape.
func TestEveryConsoleHasACaseAndACartridge(t *testing.T) {
	cases := []string{"box", "clamshell", "jewel", "keep"}
	carts := []string{"cart", "card", "disc"}
	for id, row := range consoleTraitsTable {
		l, ok := shelfLooks[id]
		if !ok {
			t.Errorf("%s has no case or cartridge", id)
			continue
		}
		if !slices.Contains(cases, l.Case.Kind) || !slices.Contains(carts, l.Cart.Kind) {
			t.Errorf("%s: %+v", id, l)
		}
		for _, sh := range []ShelfShape{l.Case, l.Cart} {
			if sh.Aspect < 0.4 || sh.Aspect > 2 || sh.Height <= 0 || sh.Height > 1 {
				t.Errorf("%s: shape %+v", id, sh)
			}
		}
		if (row.Media == mediaDisc) != (l.Cart.Kind == "disc") && id != "psp" {
			t.Errorf("%s: a disc console's games are discs, and only theirs", id)
		}
	}
}
