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
