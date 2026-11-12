// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"encoding/json"
	"strings"
	"testing"
)

// fillCommand builds a command's body the way docs/SDUI.md tells a client to: the item,
// JSON-escaped, in place of {item}, which always stands inside a string.
func fillCommand(body, item string) string {
	q, _ := json.Marshal(item)
	return strings.ReplaceAll(body, `"{item}"`, string(q))
}

// Every command's body is JSON for every kind of item ID the server sends.
func TestCommandBodiesAreJSONForEveryItem(t *testing.T) {
	items := []string{"5", "show-5", "album-3-41", "collection-2", `odd"id\with`}
	for _, cmd := range Catalog().Commands {
		if cmd.Body == "" {
			continue
		}
		if strings.Count(cmd.Body, "{item}") != strings.Count(cmd.Body, `"{item}"`) {
			t.Errorf("%s: {item} must stand inside a JSON string in %s", cmd.Op, cmd.Body)
		}
		for _, it := range items {
			var v any
			if err := json.Unmarshal([]byte(fillCommand(cmd.Body, it)), &v); err != nil {
				t.Errorf("%s with %q: %v", cmd.Op, it, err)
			}
		}
	}
}
