// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"strings"
	"sync"
)

// How much of a screen a television offers (docs/TV-MODE.md). A remote and a sofa suit browsing,
// playing and games; editors with many fields, drag handles or typing do not, so on a television
// those show a card that says to use the desktop app or a web browser, with the page's address and
// what is set now. Whatever is changed there is kept on the server and so applies on the television.
const (
	TVFull        = "full"        // everything works with a remote or a controller
	TVView        = "view"        // the values are shown, nothing can be changed
	TVDesktopOnly = "desktopOnly" // a card in its place: change this on a computer
)

// TVLevelNames are the levels in order, for a field's choices.
var TVLevelNames = []string{TVFull, TVView, TVDesktopOnly}

// ValidTVLevel says whether s is a level, or empty (which means full).
func ValidTVLevel(s string) bool {
	return s == "" || s == TVFull || s == TVView || s == TVDesktopOnly
}

// TVScreen is one screen's level, found by its address. Path is a pattern: ":name" is one part of the
// address, and "*" at the end is the rest of it, or nothing.
type TVScreen struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Level   string `json:"level"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"` // what GET /api/sdui/tv-summary/{summary} says is set now
}

// TVPart is a part of a screen that is otherwise usable on a television (the theme maker in
// Appearance), found by its id.
type TVPart struct {
	ID    string `json:"id"`
	Level string `json:"level"`
	Title string `json:"title"`
}

// TVLevelTable is data/tvlevels.json, which the catalogue carries as tvLevels.
type TVLevelTable struct {
	Levels  []string   `json:"levels"`
	Default string     `json:"default"`
	Note    string     `json:"note"`
	Screens []TVScreen `json:"screens"`
	Parts   []TVPart   `json:"parts"`
}

var tvTable = sync.OnceValue(func() *TVLevelTable {
	var t TVLevelTable
	if err := json.Unmarshal(dataFile("tvlevels.json"), &t); err != nil {
		panic("sdui: data/tvlevels.json: " + err.Error())
	}
	return &t
})

// TVLevels is the table.
func TVLevels() *TVLevelTable { return tvTable() }

// TVScreenFor is the screen an address is, or nil for one that is full on a television. The first
// match wins, so the table lists a narrower address before a wider one.
func TVScreenFor(path string) *TVScreen {
	path = strings.SplitN(path, "?", 2)[0]
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	t := tvTable()
	for i := range t.Screens {
		if TVPathMatches(t.Screens[i].Path, path) {
			return &t.Screens[i]
		}
	}
	return nil
}

// TVPathMatches says whether an address fits a screen's pattern.
func TVPathMatches(pattern, path string) bool {
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	ap := strings.Split(strings.Trim(path, "/"), "/")
	if len(ap) == 1 && ap[0] == "" {
		ap = nil
	}
	for i, seg := range pp {
		if seg == "*" && i == len(pp)-1 {
			return true
		}
		if i >= len(ap) {
			return false
		}
		if strings.HasPrefix(seg, ":") {
			if ap[i] == "" {
				return false
			}
			continue
		}
		if seg != ap[i] {
			return false
		}
	}
	return len(ap) == len(pp)
}

// TVPartLevel is a part's level, full when the table doesn't name it.
func TVPartLevel(id string) string {
	for _, p := range tvTable().Parts {
		if p.ID == id {
			return p.Level
		}
	}
	return TVFull
}
