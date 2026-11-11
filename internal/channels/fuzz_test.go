// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"encoding/json"
	"testing"
	"time"
)

// A channel's definition comes in a request, and is read back whenever its schedule is worked out.
func FuzzDefinition(f *testing.F) {
	for _, s := range []string{
		`{"name":"a","mode":"simple","rules":{"clips":[{"id":"a","item":1}]},"order":{"mode":"sequential"},"filler":{},"alignMin":0}`,
		`{"mode":"blocks","blocks":[{"start":"25:99","rules":{}}],"alignMin":-1}`, `{"bumpers":{"every":-5}}`, `{}`, `[]`, `null`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var d Definition
		if json.Unmarshal([]byte(s), &d) != nil {
			return
		}
		start := time.Now()
		_ = d.Validate()
		if el := time.Since(start); el > 5*time.Second {
			t.Fatalf("took %v", el)
		}
	})
}
