// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"encoding/json"
)

// navDefault: how the owner has arranged the main menu for anyone who hasn't
// arranged their own. The app reads and writes it as JSON; this only makes sure a
// value it can't use never gets stored.
func init() {
	settingKeys = append(settingKeys, "navDefault")
	settingChecks["navDefault"] = validNavDefault
}

const (
	navDefaultMaxBytes = 8 << 10
	navMaxItems        = 500
	navMaxID           = 64
)

func validNavDefault(v string) bool {
	if len(v) > navDefaultMaxBytes {
		return false
	}
	var p struct {
		Order    []string          `json:"order"`
		Hidden   []string          `json:"hidden"`
		Grouped  bool              `json:"grouped"`
		Groups   map[string]string `json:"groups"`
		Overflow string            `json:"overflow"`
		Collapse string            `json:"collapse"`
		Style    string            `json:"style"`
	}
	d := json.NewDecoder(bytes.NewReader([]byte(v)))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil || d.More() {
		return false
	}
	short := func(ids []string) bool {
		if len(ids) > navMaxItems {
			return false
		}
		for _, id := range ids {
			if len(id) > navMaxID {
				return false
			}
		}
		return true
	}
	if !short(p.Order) || !short(p.Hidden) || len(p.Groups) > navMaxItems {
		return false
	}
	for id, g := range p.Groups {
		if len(id) > navMaxID || len(g) > navMaxID {
			return false
		}
	}
	return oneOf("", "more", "scroll")(p.Overflow) && oneOf("", "rail", "hide")(p.Collapse) && oneOf("", "side", "top", "both")(p.Style)
}
