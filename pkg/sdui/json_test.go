// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	doc := HomeTemplate(HomeData{RecentlyAdded: sampleItems()})
	b, err := Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasSuffix(b, []byte("\n")) || bytes.Contains(b, []byte(`\u003c`)) {
		t.Error("Marshal should be compact and not HTML-escape")
	}
	back, err := Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	c1, _ := Canonical(doc)
	c2, _ := Canonical(back)
	if !bytes.Equal(c1, c2) {
		t.Error("round trip changed the document")
	}
	pretty, err := MarshalIndent(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pretty, []byte("\n  ")) {
		t.Error("MarshalIndent should indent")
	}
	if _, err := Unmarshal(pretty); err != nil {
		t.Errorf("indented output should decode: %v", err)
	}
}

func TestUnmarshal_Strict(t *testing.T) {
	cases := []struct {
		name string
		in   string
		msg  string
	}{
		{"unknown top-level field", `{"version":1,"screen":"x","root":{"type":"Screen","id":"r"},"bogus":1}`, "unknown field"},
		{"unknown node field", `{"version":1,"screen":"x","root":{"type":"Screen","id":"r","colour":"red"}}`, "unknown field"},
		{"unknown style field", `{"version":1,"screen":"x","root":{"type":"Screen","id":"r","style":{"zIndex":1}}}`, "unknown field"},
		{"wrong version", `{"version":2,"screen":"x","root":{"type":"Screen","id":"r"}}`, "unsupported schema version"},
		{"missing version", `{"screen":"x","root":{"type":"Screen","id":"r"}}`, "unsupported schema version"},
		{"trailing data", `{"version":1,"screen":"x","root":{"type":"Screen","id":"r"}} {}`, "trailing data"},
		{"not json", `{`, "decode"},
		{"empty", ``, "decode"},
		{"wrong type", `{"version":"1"}`, "decode"},
		{"array", `[]`, "decode"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Unmarshal([]byte(tc.in))
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("expected error containing %q, got %v", tc.msg, err)
			}
		})
	}
	_, err := Unmarshal([]byte(`{"version":3,"screen":"x","root":{"type":"Screen","id":"r"}}`))
	if !errors.Is(err, ErrVersion) {
		t.Errorf("expected ErrVersion, got %v", err)
	}
}

func TestUnmarshal_SizeCap(t *testing.T) {
	big := make([]byte, MaxDocumentBytes+1)
	_, err := Unmarshal(big)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
	// Exactly at the cap is allowed (even though it is not valid JSON here,
	// the size check must not trip).
	atCap := []byte(`{"version":1,"screen":"x","root":{"type":"Screen","id":"r"},"meta":{"description":"`)
	pad := MaxDocumentBytes - len(atCap) - len(`"}}`)
	atCap = append(atCap, bytes.Repeat([]byte("a"), pad)...)
	atCap = append(atCap, []byte(`"}}`)...)
	if len(atCap) != MaxDocumentBytes {
		t.Fatalf("test construction wrong: %d", len(atCap))
	}
	if _, err := Unmarshal(atCap); err != nil {
		t.Errorf("document exactly at cap should decode: %v", err)
	}
}

func TestMarshal_Error(t *testing.T) {
	doc := minimalDoc()
	doc.Root.Props = map[string]any{"f": func() {}}
	if _, err := Marshal(doc); err == nil {
		t.Error("expected marshal error")
	}
	if _, err := MarshalIndent(doc); err == nil {
		t.Error("expected marshal error")
	}
}
