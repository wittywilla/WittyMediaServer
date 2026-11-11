package sdui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func sp(id string) Node { return Node{Type: Spacer, ID: id} }

func docWith(children ...Node) Document {
	d := minimalDoc()
	d.Root.Children = children
	return d
}

func assertRoundTrip(t *testing.T, name string, old, new Document) []Patch {
	t.Helper()
	patches := Diff(old, new)
	got, err := Apply(old, patches)
	if err != nil {
		t.Fatalf("%s: Apply: %v\npatches: %+v", name, err, patches)
	}
	cg, _ := Canonical(got)
	cn, _ := Canonical(new)
	if !bytes.Equal(cg, cn) {
		t.Fatalf("%s: round-trip mismatch\n got: %s\nwant: %s\npatches: %+v", name, cg, cn, patches)
	}
	// Idempotence: diffing equal docs yields nothing.
	if again := Diff(got, new); len(again) != 0 {
		t.Fatalf("%s: diff after apply should be empty, got %+v", name, again)
	}
	return patches
}

func TestDiff_RoundTrip(t *testing.T) {
	home := HomeTemplate(HomeData{})
	homeMore := HomeTemplate(HomeData{Shelves: []ShelfData{{ID: "extra", Title: "Extra", Module: "wittymusic",
		Items: []Item{{ID: "1", Title: "One"}, {ID: "2", Title: "Two", Progress: 0.3}}}}})
	homeRendered, _ := Render(home, RenderContext{Role: "admin", Modules: []string{"wittyplay"}, Features: []string{"history"}})

	cases := []struct {
		name     string
		old, new Document
		ops      []PatchOp
	}{
		{"identical", docWith(sp("a"), sp("b")), docWith(sp("a"), sp("b")), nil},
		{"add end", docWith(sp("a")), docWith(sp("a"), sp("b")), []PatchOp{PatchAdd}},
		{"add front", docWith(sp("b")), docWith(sp("a"), sp("b")), []PatchOp{PatchAdd}},
		{"add middle", docWith(sp("a"), sp("c")), docWith(sp("a"), sp("b"), sp("c")), []PatchOp{PatchAdd}},
		{"remove", docWith(sp("a"), sp("b"), sp("c")), docWith(sp("a"), sp("c")), []PatchOp{PatchRemove}},
		{"remove all", docWith(sp("a"), sp("b")), docWith(), []PatchOp{PatchRemove, PatchRemove}},
		{"reorder swap", docWith(sp("a"), sp("b")), docWith(sp("b"), sp("a")), []PatchOp{PatchMove}},
		{"reorder reverse", docWith(sp("a"), sp("b"), sp("c"), sp("d")), docWith(sp("d"), sp("c"), sp("b"), sp("a")), nil},
		{"reorder rotate", docWith(sp("a"), sp("b"), sp("c")), docWith(sp("c"), sp("a"), sp("b")), []PatchOp{PatchMove}},
		{"replace props", docWith(Node{Type: Text, ID: "t", Props: map[string]any{"text": "a"}}),
			docWith(Node{Type: Text, ID: "t", Props: map[string]any{"text": "b"}}), []PatchOp{PatchReplace}},
		{"replace type", docWith(sp("x")), docWith(Node{Type: Divider, ID: "x"}), []PatchOp{PatchReplace}},
		{"replace style", docWith(Node{Type: Stack, ID: "s", Style: &Style{Gap: 1}}),
			docWith(Node{Type: Stack, ID: "s", Style: &Style{Gap: 2}}), []PatchOp{PatchReplace}},
		{"replace keeps children", docWith(Node{Type: Stack, ID: "s", Style: &Style{Gap: 1}, Children: []Node{sp("c")}}),
			docWith(Node{Type: Stack, ID: "s", Style: &Style{Gap: 2}, Children: []Node{sp("c")}}), []PatchOp{PatchReplace}},
		{"nested add", docWith(Node{Type: Stack, ID: "s", Children: []Node{sp("c")}}),
			docWith(Node{Type: Stack, ID: "s", Children: []Node{sp("c"), sp("d")}}), []PatchOp{PatchAdd}},
		{"nested remove and reorder", docWith(Node{Type: Stack, ID: "s", Children: []Node{sp("a"), sp("b"), sp("c")}}),
			docWith(Node{Type: Stack, ID: "s", Children: []Node{sp("c"), sp("a")}}), nil},
		{"mixed", docWith(sp("a"), sp("b"), sp("c")), docWith(sp("d"), sp("c"), Node{Type: Divider, ID: "a"}), nil},
		{"root replaced", docWith(sp("a")), func() Document { d := docWith(sp("a")); d.Root.ID = "other"; return d }(), []PatchOp{PatchReplace}},
		{"root props", minimalDoc(), func() Document { d := minimalDoc(); d.Root.Props = map[string]any{"title": "x"}; return d }(), []PatchOp{PatchReplace}},
		{"screen", minimalDoc(), func() Document { d := minimalDoc(); d.Screen = "other"; return d }(), []PatchOp{PatchReplace}},
		{"theme set", minimalDoc(), func() Document { d := minimalDoc(); d.Theme = &ThemeRef{ID: "x"}; return d }(), []PatchOp{PatchReplace}},
		{"theme cleared", func() Document { d := minimalDoc(); d.Theme = &ThemeRef{ID: "x"}; return d }(), minimalDoc(), []PatchOp{PatchReplace}},
		{"actions", minimalDoc(), func() Document {
			d := minimalDoc()
			d.Actions = map[string]Action{"go": {Type: Navigate, Target: "/"}}
			return d
		}(), []PatchOp{PatchReplace}},
		{"actions cleared", func() Document {
			d := minimalDoc()
			d.Actions = map[string]Action{"go": {Type: Navigate, Target: "/"}}
			return d
		}(), minimalDoc(), []PatchOp{PatchReplace}},
		{"meta", minimalDoc(), func() Document { d := minimalDoc(); d.Meta.Title = "T"; return d }(), []PatchOp{PatchReplace}},
		{"meta volatile ignored", minimalDoc(), func() Document {
			d := minimalDoc()
			d.Meta.GeneratedAt = time.Now()
			d.Meta.ETag = "x"
			return d
		}(), nil},
		{"template grow", home, homeMore, nil},
		{"template shrink", homeMore, home, nil},
		{"authored to rendered", home, homeRendered, nil},
		{"rendered to authored", homeRendered, home, nil},
		{"home to settings", home, SettingsTemplate(SettingsData{}), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			patches := assertRoundTrip(t, tc.name, tc.old, tc.new)
			if tc.ops != nil {
				if len(patches) != len(tc.ops) {
					t.Fatalf("expected ops %v, got %+v", tc.ops, patches)
				}
				for i, op := range tc.ops {
					if patches[i].Op != op {
						t.Errorf("patch %d: op %s, want %s", i, patches[i].Op, op)
					}
				}
			}
			// Deterministic.
			again := Diff(tc.old, tc.new)
			if !jsonEqual(again, patches) {
				t.Error("Diff is not deterministic")
			}
		})
	}
}

func TestDiff_PathFormat(t *testing.T) {
	ps := Diff(docWith(sp("a")), docWith(sp("a"), sp("b")))
	if len(ps) != 1 || ps[0].Path != "#root/children/1" || ps[0].Node == nil || ps[0].Node.ID != "b" {
		t.Fatalf("unexpected add patch: %+v", ps)
	}
	ps = Diff(docWith(sp("a"), sp("b")), docWith(sp("b"), sp("a")))
	if len(ps) != 1 || ps[0].Op != PatchMove || ps[0].From != "#b" || ps[0].Path != "#root/children/0" {
		t.Fatalf("unexpected move patch: %+v", ps)
	}
	ps = Diff(docWith(sp("a")), docWith())
	if len(ps) != 1 || ps[0].Path != "#a" {
		t.Fatalf("unexpected remove patch: %+v", ps)
	}
	ps = Diff(docWith(Node{Type: Stack, ID: "s", Children: []Node{sp("c")}}),
		docWith(Node{Type: Stack, ID: "s", Style: &Style{Gap: 3}, Children: []Node{sp("c")}}))
	if len(ps) != 1 || ps[0].Path != "#s" || ps[0].Node.Children != nil {
		t.Fatalf("replace should carry node without children: %+v", ps)
	}
}

func TestApply_Errors(t *testing.T) {
	base := docWith(Node{Type: Stack, ID: "s", Children: []Node{sp("a"), sp("b")}}, sp("z"))
	n := sp("new")
	cases := []struct {
		name string
		p    Patch
		msg  string
	}{
		{"bad path", Patch{Op: PatchAdd, Path: "s/children/0", Node: &n}, "must start with #"},
		{"empty path", Patch{Op: PatchRemove, Path: "#"}, "must start with #"},
		{"bad segment", Patch{Op: PatchAdd, Path: "#s/kids/0", Node: &n}, "expected /children/"},
		{"bad index", Patch{Op: PatchAdd, Path: "#s/children/x", Node: &n}, "bad child index"},
		{"add without index", Patch{Op: PatchAdd, Path: "#s", Node: &n}, "add needs"},
		{"add without node", Patch{Op: PatchAdd, Path: "#s/children/0"}, "needs a node"},
		{"add unknown parent", Patch{Op: PatchAdd, Path: "#nope/children/0", Node: &n}, "unknown parent"},
		{"add out of range", Patch{Op: PatchAdd, Path: "#s/children/5", Node: &n}, "out of range"},
		{"remove with index", Patch{Op: PatchRemove, Path: "#s/children/0"}, "takes a #id"},
		{"remove root", Patch{Op: PatchRemove, Path: "#root"}, "cannot remove the root"},
		{"remove unknown", Patch{Op: PatchRemove, Path: "#nope"}, "unknown node"},
		{"replace with index", Patch{Op: PatchReplace, Path: "#s/children/0", Node: &n}, "takes a #id"},
		{"replace without node", Patch{Op: PatchReplace, Path: "#s"}, "needs a node"},
		{"replace unknown", Patch{Op: PatchReplace, Path: "#nope", Node: &n}, "unknown node"},
		{"move without index", Patch{Op: PatchMove, Path: "#s", From: "#a"}, "move needs"},
		{"move bad from", Patch{Op: PatchMove, Path: "#s/children/0", From: "#a/children/0"}, "needs a #id from"},
		{"move unknown parent", Patch{Op: PatchMove, Path: "#nope/children/0", From: "#a"}, "unknown parent"},
		{"move unknown from", Patch{Op: PatchMove, Path: "#s/children/0", From: "#nope"}, "unknown node"},
		{"move across parents", Patch{Op: PatchMove, Path: "#s/children/0", From: "#z"}, "within one parent"},
		{"move out of range", Patch{Op: PatchMove, Path: "#s/children/9", From: "#a"}, "out of range"},
		{"unknown op", Patch{Op: "swap", Path: "#s"}, "unknown op"},
		{"doc path non-replace", Patch{Op: PatchAdd, Path: "/screen", Value: "x"}, "only support replace"},
		{"doc unknown path", Patch{Op: PatchReplace, Path: "/nope", Value: "x"}, "unknown document path"},
		{"root without node", Patch{Op: PatchReplace, Path: "/root"}, "needs a node"},
		{"screen bad value", Patch{Op: PatchReplace, Path: "/screen", Value: 42}, "cannot unmarshal"},
		{"theme bad value", Patch{Op: PatchReplace, Path: "/theme", Value: "x"}, "cannot unmarshal"},
		{"meta bad value", Patch{Op: PatchReplace, Path: "/meta", Value: "x"}, "cannot unmarshal"},
		{"actions bad value", Patch{Op: PatchReplace, Path: "/actions", Value: "x"}, "cannot unmarshal"},
		{"value not serialisable", Patch{Op: PatchReplace, Path: "/screen", Value: func() {}}, "unsupported type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Apply(base, []Patch{tc.p})
			if err == nil || !strings.Contains(err.Error(), tc.msg) {
				t.Fatalf("expected error containing %q, got %v", tc.msg, err)
			}
		})
	}
	// Input is never modified, even on failure.
	if len(base.Root.Children[0].Children) != 2 {
		t.Error("Apply mutated input")
	}
}

func TestApply_DocLevelFromDecodedJSON(t *testing.T) {
	// Values arriving over the wire are maps, not typed structs.
	base := minimalDoc()
	patches := []Patch{
		{Op: PatchReplace, Path: "/theme", Value: map[string]any{"id": "oled", "moduleOverrides": map[string]any{"wittytv": map[string]any{"accent": "#fff"}}}},
		{Op: PatchReplace, Path: "/actions", Value: map[string]any{"go": map[string]any{"type": "navigate", "target": "/x"}}},
		{Op: PatchReplace, Path: "/meta", Value: map[string]any{"title": "T", "ttlSeconds": 5}},
		{Op: PatchReplace, Path: "/screen", Value: "renamed"},
		{Op: PatchReplace, Path: "/root", Node: &Node{Type: Screen, ID: "r2"}},
		{Op: PatchReplace, Path: "/theme", Value: nil},
	}
	out, err := Apply(base, patches)
	if err != nil {
		t.Fatal(err)
	}
	if out.Theme != nil || out.Actions["go"].Target != "/x" || out.Meta.Title != "T" || out.Meta.TTLSeconds != 5 ||
		out.Screen != "renamed" || out.Root.ID != "r2" {
		t.Errorf("doc-level patches not applied: %+v", out)
	}
}

func TestCanonicalAndETag(t *testing.T) {
	a := minimalDoc()
	a.Root.Props = map[string]any{"z": 1, "a": "x", "m": map[string]any{"q": true, "b": nil}}
	a.Meta.GeneratedAt = time.Now()
	a.Meta.ETag = "stale"
	b := a
	b.Root.Props = map[string]any{"m": map[string]any{"b": nil, "q": true}, "a": "x", "z": 1}
	b.Meta = Meta{}
	ca, err := Canonical(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, _ := Canonical(b)
	if !bytes.Equal(ca, cb) {
		t.Errorf("canonical should ignore key order and volatile meta:\n%s\n%s", ca, cb)
	}
	if bytes.Contains(ca, []byte(" ")) || bytes.Contains(ca, []byte("\n")) {
		t.Error("canonical should have no whitespace")
	}
	if !bytes.HasPrefix(ca, []byte(`{"root":{"id":"root","props":{"a":"x","m":{"b":null,"q":true},"z":1}`)) {
		t.Errorf("keys not sorted: %s", ca)
	}
	et := ETag(a)
	if !strings.HasPrefix(et, `W/"sha256-`) || !strings.HasSuffix(et, `"`) || len(et) != len(`W/"sha256-"`)+32 {
		t.Errorf("etag format: %q", et)
	}
	if ETag(a) != ETag(b) {
		t.Error("etag should match for canonical-equal docs")
	}
	c := a
	c.Screen = "other"
	if ETag(c) == ETag(a) {
		t.Error("etag should differ for different docs")
	}
	bad := minimalDoc()
	bad.Root.Props = map[string]any{"f": func() {}}
	if _, err := Canonical(bad); err == nil {
		t.Error("canonical should fail on unserialisable props")
	}
	if ETag(bad) != "" {
		t.Error("etag of unserialisable doc should be empty")
	}
	// Numbers survive canonicalisation unchanged.
	n := minimalDoc()
	n.Root.Props = map[string]any{"big": 12345678901234567890.0, "small": 0.1}
	cn, _ := Canonical(n)
	if !bytes.Contains(cn, []byte("0.1")) {
		t.Errorf("number mangled: %s", cn)
	}
}
