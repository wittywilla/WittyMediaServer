// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// The settings menu every client draws (data/settingsmenu.json, docs/SETTINGS-MENU.md): the Android
// phone app's You page, its groups, rows, order and words, so the web, its TV mode, the desktop app,
// Android phone and TV, and the Apple apps all show the same list. A client shows a row when each of
// its conditions holds and it can open it: its own screen (Native) or the server's page (Path). What a
// television may change there comes from the TV table, by the row's Path or Part.

// SettingsMenuRow is one row.
type SettingsMenuRow struct {
	ID    string            `json:"id"`
	Key   string            `json:"key"`   // the clients' word for it (strings.en.json)
	Title string            `json:"title"` // in English: the web's words
	Icon  map[string]string `json:"icon"`  // android (Material Symbols), apple (SF Symbols), web (icons.tsx)
	Tint  string            `json:"tint"`  // the icon tile's colour, or "danger"
	// Native names a client's own screen for it; Path is the server's page, opened where a client has
	// no screen of its own, and the address a television's QR code leads to. SDUI marks a page the
	// clients draw from the server's SDUI pages.
	Native string   `json:"native,omitempty"`
	Path   string   `json:"path,omitempty"`
	SDUI   bool     `json:"sdui,omitempty"`
	Part   string   `json:"part,omitempty"` // a part in the TV table, for a client's own editor
	When   []string `json:"when,omitempty"`
	// Value is what the row shows on its right; Summary the TV summary (GET /api/sdui/tv-summary/{id})
	// a television shows on a row it can't change, so the row still says what is set.
	Value   string `json:"value,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// SettingsMenuGroup is a heading and its rows.
type SettingsMenuGroup struct {
	ID    string            `json:"id"`
	Key   string            `json:"key"`
	Title string            `json:"title"`
	Rows  []SettingsMenuRow `json:"rows"`
}

// SettingsMenu is the whole file.
type SettingsMenu struct {
	Note       string              `json:"note"`
	Conditions []string            `json:"conditions"`
	Values     []string            `json:"values"`
	Groups     []SettingsMenuGroup `json:"groups"`
}

var settingsMenu = sync.OnceValue(func() *SettingsMenu {
	var m SettingsMenu
	if err := json.Unmarshal(dataFile("settingsmenu.json"), &m); err != nil {
		panic("sdui: data/settingsmenu.json: " + err.Error())
	}
	return &m
})

// SettingsMenuData is the menu.
func SettingsMenuData() *SettingsMenu { return settingsMenu() }

// Problems lists what is wrong with the menu, in words; none means every client can draw it.
func (m *SettingsMenu) Problems() []string {
	var out []string
	conds := map[string]bool{}
	for _, c := range m.Conditions {
		conds[c] = true
	}
	values := map[string]bool{}
	for _, v := range m.Values {
		values[v] = true
	}
	parts := map[string]bool{}
	for _, p := range TVLevels().Parts {
		parts[p.ID] = true
	}
	ids := map[string]bool{}
	for _, g := range m.Groups {
		if g.ID == "" || g.Key == "" || g.Title == "" || len(g.Rows) == 0 {
			out = append(out, fmt.Sprintf("group %q needs an id, a key, a title and rows", g.ID))
		}
		for _, r := range g.Rows {
			at := g.ID + "/" + r.ID
			if r.ID == "" || r.Key == "" || r.Title == "" {
				out = append(out, at+": needs an id, a key and a title")
			}
			if ids[r.Key] {
				out = append(out, at+": key "+r.Key+" used twice")
			}
			ids[r.Key] = true
			for _, k := range []string{"android", "apple", "web"} {
				if r.Icon[k] == "" {
					out = append(out, at+": no "+k+" icon")
				}
			}
			if r.Tint != "danger" && (len(r.Tint) != 7 || r.Tint[0] != '#') {
				out = append(out, at+": tint "+r.Tint+" is not #rrggbb or danger")
			}
			if r.Native == "" && r.Path == "" {
				out = append(out, at+": opens nothing (no native screen, no path)")
			}
			if r.Path != "" && !strings.HasPrefix(r.Path, "/") {
				out = append(out, at+": path "+r.Path+" doesn't start with /")
			}
			if r.Part != "" && !parts[r.Part] {
				out = append(out, at+": part "+r.Part+" isn't in the TV table")
			}
			for _, c := range r.When {
				if !conds[c] {
					out = append(out, at+": condition "+c+" isn't listed")
				}
			}
			if r.Value != "" && !values[r.Value] {
				out = append(out, at+": value "+r.Value+" isn't listed")
			}
		}
	}
	return out
}

// SettingsRowLevel is what a television may do with a row: its part's level, else its page's.
func SettingsRowLevel(r SettingsMenuRow) string {
	if r.Part != "" {
		return TVPartLevel(r.Part)
	}
	if r.Path == "" {
		return TVFull
	}
	path := strings.SplitN(r.Path, "#", 2)[0]
	if s := TVScreenFor(path); s != nil {
		return s.Level
	}
	return TVFull
}
