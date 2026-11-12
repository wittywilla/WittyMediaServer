// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package brandname

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type caseFile struct {
	Cases []struct {
		Name      string `json:"name"`
		Colouring string `json:"colouring"`
		Want      string `json:"want"`
	} `json:"cases"`
	Valid   []string `json:"valid"`
	Invalid []string `json:"invalid"`
}

func readCases(t *testing.T) caseFile {
	t.Helper()
	b, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var c caseFile
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// marked writes parts as the cases do: coloured parts in [brackets].
func marked(parts []Part) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Accent {
			b.WriteString("[" + p.Text + "]")
		} else {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func TestParts(t *testing.T) {
	for _, c := range readCases(t).Cases {
		parts := Parts(c.Name, c.Colouring)
		if got := marked(parts); got != c.Want {
			t.Errorf("Parts(%q, %q) = %q, want %q", c.Name, c.Colouring, got, c.Want)
		}
		var whole strings.Builder
		for i, p := range parts {
			whole.WriteString(p.Text)
			if p.Text == "" || i > 0 && parts[i-1].Accent == p.Accent {
				t.Errorf("Parts(%q, %q): part %d is empty or should have joined the one before", c.Name, c.Colouring, i)
			}
		}
		if whole.String() != c.Name {
			t.Errorf("Parts(%q, %q) don't join back into the name: %q", c.Name, c.Colouring, whole.String())
		}
	}
}

func TestValid(t *testing.T) {
	c := readCases(t)
	for _, v := range c.Valid {
		if !Valid(v) {
			t.Errorf("Valid(%q) = false", v)
		}
	}
	for _, v := range c.Invalid {
		if Valid(v) {
			t.Errorf("Valid(%q) = true", v)
		}
	}
}

func FuzzParts(f *testing.F) {
	f.Add("WittyMediaServer", "auto")
	f.Add("éḾ", "pick:0-3")
	f.Fuzz(func(t *testing.T, name, colouring string) {
		var whole strings.Builder
		for _, p := range Parts(name, colouring) {
			whole.WriteString(p.Text)
		}
		if strings.ToValidUTF8(name, "�") != whole.String() && name == strings.ToValidUTF8(name, "�") {
			t.Fatalf("parts of %q join into %q", name, whole.String())
		}
	})
}

func TestSpansColourPartsAndMarkShowsLetters(t *testing.T) {
	spans, err := ParseSpans(`[{"from":0,"to":5,"color":{"kind":"static","hex":"#b4491f"},"bold":true},{"from":5,"to":10,"color":{"kind":"gradient","stops":["#0a6aa8","#3aa84a"],"angle":90}}]`, 16)
	if err != nil {
		t.Fatal(err)
	}
	parts := PartsFromSpans("WittyMediaServer", spans)
	if len(parts) != 3 || parts[0].Text != "Witty" || parts[0].Color.Kind != KindStatic || !parts[0].Bold || parts[1].Color.Kind != KindGradient || parts[2].Text != "Server" || parts[2].Color.Kind != KindFG {
		t.Errorf("parts: %+v", parts)
	}
	for _, bad := range []string{`[{"from":3,"to":2,"color":{"kind":"fg"}}]`, `[{"from":0,"to":20,"color":{"kind":"fg"}}]`, `[{"from":0,"to":3,"color":{"kind":"static","hex":"red"}}]`,
		`[{"from":0,"to":3,"color":{"kind":"gradient","stops":["#000000"]}}]`, `[{"from":0,"to":3,"color":{"kind":"neon"}}]`, `[{"from":0,"to":3,"colour":{}}]`} {
		if _, err := ParseSpans(bad, 16); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	for in, want := range map[string]string{"": "W", `{"kind":"initials"}`: "WMS", `{"kind":"text","text":"WM"}`: "WM"} {
		m, err := ParseMark(in)
		if err != nil || m.Shown("WittyMediaServer") != want {
			t.Errorf("mark %q: %+v %v, shown %q want %q", in, m, err, m.Shown("WittyMediaServer"), want)
		}
	}
	if _, err := ParseMark(`{"kind":"text","text":"abc"}`); err == nil {
		t.Error("three characters of text")
	}
	for _, p := range Parts("WittyMediaServer", Auto) {
		if p.Color == nil || (p.Accent != (p.Color.Kind == KindAccent)) {
			t.Errorf("the quick modes carry their colour: %+v", p)
		}
	}
}
