// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"testing"
)

// FuzzUnmarshal feeds arbitrary bytes through Unmarshal and, for anything
// that decodes, through Validate, Render, Canonical, Marshal and a
// Diff/Apply round trip. None of these may panic.
func FuzzUnmarshal(f *testing.F) {
	for _, doc := range DefaultScreens() {
		b, err := Marshal(doc)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte(`{"version":1,"screen":"x","root":{"type":"Screen","id":"r"}}`))
	f.Add([]byte(`{"version":1,"screen":"x","root":{"type":"Screen","id":"r","children":[{"type":"Text","id":"t","bindings":{"text":{"path":"$.a[0].b","format":"duration"}}}]},"actions":{"go":{"type":"navigate","target":"/"}}}`))
	f.Add([]byte(`{"version":1,"screen":"x","root":{"type":"Screen","id":"r","visibility":{"any":[{"minWidth":1,"maxWidth":2}],"not":{"role":"a"}}}}`))
	f.Add([]byte(`{`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		doc, err := Unmarshal(data)
		if err != nil {
			return
		}
		_ = Validate(&doc)
		ctx := RenderContext{
			Platform: "web", Role: "admin",
			Features: []string{"history"}, Modules: []string{"wittyplay"},
			Flags:    map[string]bool{"beta": true},
			Viewport: Viewport{Width: 1000, Height: 600},
			Data:     map[string]any{"a": []any{map[string]any{"b": float64(5)}}},
		}
		rendered, rerr := Render(doc, ctx)
		if _, err := Canonical(doc); err != nil {
			return // unserialisable values cannot come from JSON, but be safe
		}
		_ = ETag(doc)
		if _, err := Marshal(doc); err != nil {
			t.Fatalf("Marshal failed after successful Unmarshal: %v", err)
		}
		if rerr == nil {
			patches := Diff(doc, rendered)
			if _, err := Apply(doc, patches); err != nil && Validate(&doc) == nil {
				// Apply may legitimately fail on documents with duplicate IDs;
				// for valid documents it must succeed.
				t.Fatalf("Apply failed on valid doc: %v", err)
			}
		}
	})
}

// FuzzBindingPath checks the path parser never panics.
func FuzzBindingPath(f *testing.F) {
	for _, s := range []string{"$", "$.a", "$.a[0].b", "$[1]", "", "a", "$.", "$[", "$[x]"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		segs, err := ParseBindingPath(s)
		if err == nil {
			Lookup(map[string]any{"a": []any{map[string]any{"b": 1}}}, segs)
		}
	})
}

// FuzzValidateColor checks the colour validator never panics.
func FuzzValidateColor(f *testing.F) {
	for _, s := range []string{"#fff", "token:accent", "rgb(1,2,3)", "hsla(", "", "x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { _ = ValidateColor(s) })
}
