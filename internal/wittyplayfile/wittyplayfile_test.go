// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyplayfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

const example = `{
  "wittyplay": 1,
  "name": "Living room",
  "default": {"picture": "case", "label": "boxart"},
  "consoles": {"snes": {"case": "cart-super"}, "ps1": {"picture": "fitted"}},
  "games": [
    {"match": {"file": "Made Up Quest (USA).sfc"}, "case": "generic-box", "label": "title"},
    {"match": {"title": "Another Game", "console": "ps1"}, "picture": "filled"}
  ]
}`

func TestParseExample(t *testing.T) {
	d, err := Parse([]byte(example))
	if err != nil {
		t.Fatal(err)
	}
	if d.Version != 1 || d.Name != "Living room" || d.Default.Picture != "case" || d.Consoles["snes"].Case != "cart-super" || len(d.Games) != 2 {
		t.Fatalf("read wrongly: %+v", d)
	}
	got := d.For(0, "snes", "made up quest (usa).SFC", "Made Up Quest")
	if got.Picture != "case" || got.Case != "generic-box" || got.Label != "title" {
		t.Fatalf("game look = %+v", got)
	}
	if got := d.For(0, "snes", "Other.sfc", "Other"); got.Case != "cart-super" || got.Picture != "case" {
		t.Fatalf("console look = %+v", got)
	}
	if got := d.For(0, "ps1", "x.cue", "Another Game"); got.Picture != "filled" {
		t.Fatalf("title match = %+v", got)
	}
	// The title alone, on the wrong console, is another game.
	if got := d.For(0, "n64", "x.z64", "Another Game"); got.Picture != "case" {
		t.Fatalf("console must agree: %+v", got)
	}
}

// What a reader doesn't know comes out of a write as it went in, at every level.
func TestUnknownFieldsKept(t *testing.T) {
	in := `{"wittyplay":1,"future":{"a":[1,2]},"default":{"picture":"fitted","sparkle":true},
	  "consoles":{"nes":{"case":"cart-nes","glow":"blue"}},
	  "games":[{"match":{"item":7,"crc":"abcd1234"},"label":"logo","note":"kept"}]}`
	d, err := Parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	out, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"future"`, `"sparkle": true`, `"glow": "blue"`, `"crc": "abcd1234"`, `"note": "kept"`} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("lost %s in\n%s", want, out)
		}
	}
	if !bytes.HasPrefix(out, []byte("{\n  \"wittyplay\": 1,")) {
		t.Errorf("the version comes first:\n%s", out)
	}
	again, err := Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	out2, _ := again.Encode()
	if !bytes.Equal(out, out2) {
		t.Errorf("a second round trip changed it:\n%s\n%s", out, out2)
	}
}

func TestRefused(t *testing.T) {
	for name, in := range map[string]string{
		"not json":         `wittyplay: 1`,
		"array":            `[1]`,
		"no version":       `{"default":{}}`,
		"version zero":     `{"wittyplay":0}`,
		"version string":   `{"wittyplay":"1"}`,
		"version fraction": `{"wittyplay":1.5}`,
		"bad picture":      `{"wittyplay":1,"default":{"picture":"stretched"}}`,
		"bad label":        `{"wittyplay":1,"default":{"label":"photo"}}`,
		"bad case":         `{"wittyplay":1,"default":{"case":"../../etc"}}`,
		"bad console":      `{"wittyplay":1,"consoles":{"Not A Console":{}}}`,
		"empty match":      `{"wittyplay":1,"games":[{"match":{},"picture":"case"}]}`,
		"no match":         `{"wittyplay":1,"games":[{"picture":"case"}]}`,
		"file with folder": `{"wittyplay":1,"games":[{"match":{"file":"../x.sfc"}}]}`,
		"negative item":    `{"wittyplay":1,"games":[{"match":{"item":-3}}]}`,
		"wrong type":       `{"wittyplay":1,"consoles":[]}`,
		"long name":        `{"wittyplay":1,"name":"` + strings.Repeat("x", MaxName+1) + `"}`,
	} {
		if _, err := Parse([]byte(in)); err == nil {
			t.Errorf("%s: accepted %s", name, in)
		}
	}
	if _, err := Parse([]byte(`{"wittyplay":2}`)); !errors.Is(err, ErrNewer) {
		t.Errorf("a newer version: %v", err)
	}
	big := `{"wittyplay":1,"pad":"` + strings.Repeat("x", MaxSize) + `"}`
	if _, err := Parse([]byte(big)); !errors.Is(err, ErrTooBig) {
		t.Errorf("too big: %v", err)
	}
	if _, err := Read(strings.NewReader(big)); !errors.Is(err, ErrTooBig) {
		t.Errorf("too big, read: %v", err)
	}
	var many strings.Builder
	many.WriteString(`{"wittyplay":1,"games":[`)
	for i := 0; i <= MaxGames; i++ {
		if i > 0 {
			many.WriteByte(',')
		}
		many.WriteString(`{"match":{"item":1}}`)
	}
	many.WriteString(`]}`)
	if _, err := Parse([]byte(many.String())); err == nil {
		t.Error("too many games accepted")
	}
}

// A case style a newer program draws is kept and refused by no one; this one draws the console's own.
func TestUnknownCaseKept(t *testing.T) {
	d, err := Parse([]byte(`{"wittyplay":1,"default":{"case":"cart-hologram"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if d.Default.Case != "cart-hologram" || KnownCase("cart-hologram") || !KnownCase("cart-nes") {
		t.Fatal("unknown case style")
	}
}

// A byte-order mark, as some editors write, is fine.
func TestBOM(t *testing.T) {
	if _, err := Parse([]byte("\xef\xbb\xbf{\"wittyplay\":1}")); err != nil {
		t.Fatal(err)
	}
}

// The spec's example and its schema's case list say what this package says.
func TestSpecAgrees(t *testing.T) {
	schema, err := os.ReadFile("../../docs/specs/wittyplay.schema.json")
	if err != nil {
		t.Skip("no spec here")
	}
	var s struct {
		Defs map[string]struct {
			Properties map[string]struct {
				Enum     []string `json:"enum"`
				Examples []string `json:"examples"`
			} `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		t.Fatal(err)
	}
	look := s.Defs["look"].Properties
	if strings.Join(look["picture"].Enum, ",") != "fitted,filled,case" || strings.Join(look["label"].Enum, ",") != "boxart,logo,title" {
		t.Errorf("schema enums differ: %+v", look)
	}
	if strings.Join(look["case"].Examples, ",") != "auto,"+strings.Join(CaseStyles, ",") {
		t.Errorf("schema's case styles differ from CaseStyles: %v", look["case"].Examples)
	}
	spec, err := os.ReadFile("../../docs/specs/wittyplay-file.md")
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(spec, []byte("```json\n"))
	end := bytes.Index(spec[start+8:], []byte("```"))
	if start < 0 || end < 0 {
		t.Fatal("the spec has no example")
	}
	if _, err := Parse(spec[start+8 : start+8+end]); err != nil {
		t.Errorf("the spec's example doesn't read: %v", err)
	}
}
