// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package conformance

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

var update = flag.Bool("update", false, "rewrite fixtures/*.json from the cases")

// encode writes a fixture: compact, one line, so that sixty of them are a megabyte and a
// half rather than five. `jq . file` or `python3 -m json.tool file` prints one to read.
func encode(t *testing.T, v any) []byte {
	return encodeIndent(t, v, "")
}

func encodeIndent(t *testing.T, v any, indent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	if err := enc.Encode(v); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// build makes the fixture of a case: the input, and what the reference
// implementation answers for it.
func build(t *testing.T, k kase) Fixture {
	t.Helper()
	// The input as JSON holds it, so the fixture and the run can't differ.
	var in Input
	if err := json.Unmarshal(encode(t, k.in), &in); err != nil {
		t.Fatalf("%s: %v", k.id, err)
	}
	if issues := sdui.ValidatePageWith(&in.Page, sdui.NewModuleSet(in.ModuleUI)); sdui.HasErrors(issues) {
		t.Fatalf("%s: its page doesn't validate: %v", k.id, issues)
	}
	exp, err := Run(in)
	if err != nil {
		t.Fatalf("%s: %v", k.id, err)
	}
	for _, m := range k.more {
		ans, err := RunSection(in, m.Section, m.Cursor, m.Season)
		if err != nil {
			t.Fatalf("%s: section %s at %d: %v", k.id, m.Section, m.Cursor, err)
		}
		m.Answer = ans
		exp.More = append(exp.More, m)
	}
	return Fixture{Kit: KitVersion, Schema: sdui.SchemaVersion, Catalog: sdui.CatalogRevision, ID: k.id, Title: k.title, Checks: k.checks, Input: in, Expected: exp}
}

// The fixtures in the repository are what the cases give. Run with -update
// after a deliberate change to the contract, and read the diff.
func TestFixturesAreCurrent(t *testing.T) {
	seen := map[string]bool{}
	type entry struct {
		ID         string `json:"id"`
		File       string `json:"file"`
		Title      string `json:"title"`
		FormFactor string `json:"formFactor"`
	}
	index := struct {
		Kit     int     `json:"kit"`
		Schema  int     `json:"schemaVersion"`
		Catalog int     `json:"catalogRevision"`
		Cases   []entry `json:"cases"`
	}{Kit: KitVersion, Schema: sdui.SchemaVersion, Catalog: sdui.CatalogRevision}
	for _, k := range cases() {
		if seen[k.id] {
			t.Fatalf("two cases are called %s", k.id)
		}
		seen[k.id] = true
		want := encode(t, build(t, k))
		name := k.id + ".json"
		index.Cases = append(index.Cases, entry{ID: k.id, File: name, Title: k.title, FormFactor: k.in.Request.FormFactor})
		path := filepath.Join("fixtures", name)
		if *update {
			if err := os.WriteFile(path, want, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v (run go test ./pkg/sdui/conformance -update)", k.id, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is not what the reference implementation gives now. If the contract changed on purpose, run go test ./pkg/sdui/conformance -update, read the diff, and tell the renderer authors.", path)
		}
	}
	if len(index.Cases) < 40 {
		t.Errorf("the kit has %d cases; it promises at least 40", len(index.Cases))
	}
	want := encodeIndent(t, index, " ")
	path := filepath.Join("fixtures", "index.json")
	if *update {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		// A fixture no case makes any more would be checked by nobody.
		files, _ := filepath.Glob(filepath.Join("fixtures", "*.json"))
		for _, f := range files {
			if id := strings.TrimSuffix(filepath.Base(f), ".json"); id != "index" && !seen[id] {
				_ = os.Remove(f)
			}
		}
		return
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
		t.Errorf("fixtures/index.json is out of date (run with -update)")
	}
}

// What a renderer's test suite does, done here in Go: read each fixture as
// plain JSON, and check both halves of it from the file alone.
func TestFixturesFromDisk(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("fixtures", "*.json"))
	if err != nil || len(files) < 2 {
		t.Fatalf("no fixtures: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		if filepath.Base(f) == "index.json" {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var fx Fixture
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&fx); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		t.Run(fx.ID, func(t *testing.T) {
			// The server's half: the input renders to the expected document.
			exp, err := Run(fx.Input)
			if err != nil {
				t.Fatal(err)
			}
			if exp.NotFound != fx.Expected.NotFound {
				t.Fatalf("notFound: got %v, want %v", exp.NotFound, fx.Expected.NotFound)
			}
			if fx.Expected.NotFound {
				return
			}
			if got, want := encode(t, exp.Document), encode(t, fx.Expected.Document); !bytes.Equal(got, want) {
				t.Errorf("the rendered document differs from expected.document")
			}
			// The renderer's half: the expected document normalises to the
			// expected layout.
			layout := Normalise(*fx.Expected.Document)
			if got, want := encode(t, layout), encode(t, fx.Expected.Layout); !bytes.Equal(got, want) {
				t.Errorf("Normalise(expected.document) differs from expected.layout")
			}
			for _, m := range fx.Expected.More {
				ans, err := RunSection(fx.Input, m.Section, m.Cursor, m.Season)
				if err != nil {
					t.Fatal(err)
				}
				if got, want := encode(t, ans), encode(t, m.Answer); !bytes.Equal(got, want) {
					t.Errorf("section %s at cursor %d differs", m.Section, m.Cursor)
				}
			}
			checkDocument(t, fx.Expected.Document)
		})
	}
}

// checkDocument holds every expected document to the rules of the contract
// that don't depend on the case: unique IDs, focus numbered from 1 without
// gaps with exactly one default, pictures from the device's buckets, and no
// component outside the catalogue.
func checkDocument(t *testing.T, doc *sdui.Document) {
	t.Helper()
	ids := map[string]bool{}
	var orders []int
	defaults := 0
	var walk func(n *sdui.Node)
	walk = func(n *sdui.Node) {
		if ids[n.ID] {
			t.Errorf("the ID %q is used twice", n.ID)
		}
		ids[n.ID] = true
		if len(n.ID) > sdui.MaxNodeIDLength {
			t.Errorf("the ID %q is too long", n.ID)
		}
		if _, ok := sdui.Component(n.Type); !ok {
			t.Errorf("%s: %q is not in the catalogue", n.ID, n.Type)
		}
		if n.Focus != nil {
			orders = append(orders, n.Focus.Order)
			if n.Focus.Default {
				defaults++
			}
		}
		if im, _ := n.Props["image"].(map[string]any); im != nil {
			w, _ := im["width"].(float64)
			ok := false
			for _, b := range doc.Layout.ImageWidths {
				ok = ok || int(w) == b
			}
			if !ok {
				t.Errorf("%s asks for a picture %v wide, which is not one of the widths", n.ID, w)
			}
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&doc.Root)
	sort.Ints(orders)
	for i, o := range orders {
		if o != i+1 {
			t.Errorf("focus order has a gap or a repeat at %d", o)
			break
		}
	}
	if len(orders) > 0 && defaults != 1 {
		t.Errorf("%d nodes are marked as where focus starts; want 1", defaults)
	}
	// A television and a headset always show the key legend, so every page has a Dock holding it, whatever
	// the page is (a not-found page has no document to check).
	if ff := doc.Layout.FormFactor; ff == sdui.FormTV || ff == sdui.FormHeadset {
		legend := false
		var find func(n *sdui.Node, inDock bool)
		find = func(n *sdui.Node, inDock bool) {
			legend = legend || (inDock && n.Type == "KeyLegend")
			for i := range n.Children {
				find(&n.Children[i], inDock || n.Type == "Dock")
			}
		}
		find(&doc.Root, false)
		if !legend {
			t.Errorf("a %s page has no Dock with the key legend", ff)
		}
	}
	if doc.Meta.ETag != sdui.ETag(*doc) {
		t.Errorf("meta.etag is not the document's ETag")
	}
}

// A renderer's own normaliser reports differences; so does this, to show the
// comparison is strict.
func TestNormaliseNoticesAChange(t *testing.T) {
	k := cases()[0]
	fx := build(t, k)
	before := encode(t, Normalise(*fx.Expected.Document))
	doc := roundTrip(*fx.Expected.Document)
	doc.Root.Children[0].Props["title"] = "Something else"
	if bytes.Equal(before, encode(t, Normalise(doc))) {
		t.Fatal("a changed title made no difference to the layout")
	}
}

func TestPlainText(t *testing.T) {
	for in, want := range map[string]string{
		"## Friday\nPick **one** each. The [full list](/collections/12) is longer.": "Friday Pick one each. The full list is longer.",
		"plain":                 "plain",
		"`code` and _emphasis_": "code and emphasis",
	} {
		if got := PlainText(in); got != want {
			t.Errorf("PlainText(%q) = %q, want %q", in, got, want)
		}
	}
}

// card-style-table.json (beside the fixtures, not in them) is the catalogue's card styles as a renderer that keeps a table of its
// own must have them: a renderer's test compares its table with this one (README, "Requirement").
func TestCardStyleTableIsCurrent(t *testing.T) {
	type style struct {
		Style         string   `json:"style"`
		Shape         string   `json:"shape"`
		Aspect        string   `json:"aspect"`
		Image         []string `json:"image"`
		Shows         []string `json:"shows"`
		TitleLines    int      `json:"titleLines"`
		SubtitleLines int      `json:"subtitleLines"`
	}
	table := struct {
		Kit     int     `json:"kit"`
		Catalog int     `json:"catalogRevision"`
		Styles  []style `json:"cardStyles"`
	}{Kit: KitVersion, Catalog: sdui.CatalogRevision}
	for _, c := range sdui.Catalog().CardStyles {
		table.Styles = append(table.Styles, style{c.Style, c.Shape, c.Aspect, c.Image, c.Shows, c.TitleLines, c.SubtitleLines})
	}
	want := encodeIndent(t, table, " ")
	path := "card-style-table.json"
	if *update {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
		t.Errorf("%s is not the catalogue's card styles (run go test ./pkg/sdui/conformance -update): %v", path, err)
	}
}
