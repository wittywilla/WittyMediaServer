package sdui

import (
	"errors"
	"strings"
	"testing"
)

// minimalDoc returns the smallest valid document.
func minimalDoc() Document {
	return Document{
		Version: SchemaVersion,
		Screen:  "test",
		Root:    Node{Type: Screen, ID: "root"},
	}
}

func mustInvalid(t *testing.T, doc Document, wantPath, wantMsg string) {
	t.Helper()
	err := Validate(&doc)
	if err == nil {
		t.Fatalf("expected validation error containing %q at %q, got nil", wantMsg, wantPath)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	for _, is := range ve.Issues {
		if (wantPath == "" || is.Path == wantPath) && strings.Contains(is.Message, wantMsg) {
			return
		}
	}
	t.Fatalf("no issue at %q containing %q; got:\n%v", wantPath, wantMsg, err)
}

func TestValidate_Minimal(t *testing.T) {
	d := minimalDoc()
	if err := Validate(&d); err != nil {
		t.Fatalf("minimal doc should validate: %v", err)
	}
	if err := Validate(nil); err == nil {
		t.Fatal("nil doc should fail")
	}
}

func TestValidate_Rules(t *testing.T) {
	deep := func(n int) Node {
		root := Node{Type: Screen, ID: "root"}
		cur := &root
		for i := 0; i < n; i++ {
			cur.Children = []Node{{Type: Stack, ID: "s" + itoa(i)}}
			cur = &cur.Children[0]
		}
		return root
	}
	wide := func(n int) Node {
		root := Node{Type: Screen, ID: "root"}
		for i := 0; i < n; i++ {
			root.Children = append(root.Children, Node{Type: Spacer, ID: "sp" + itoa(i)})
		}
		return root
	}

	cases := []struct {
		name    string
		mutate  func(*Document)
		path    string
		message string
	}{
		{"bad version", func(d *Document) { d.Version = 2 }, "/version", "unsupported schema version"},
		{"empty screen", func(d *Document) { d.Screen = " " }, "/screen", "must not be empty"},
		{"root not screen", func(d *Document) { d.Root.Type = Stack }, "/root/type", "root must be"},
		{"empty id", func(d *Document) { d.Root.ID = "" }, "/root/id", "must not be empty"},
		{"long id", func(d *Document) { d.Root.ID = strings.Repeat("a", 129) }, "/root/id", "longer than"},
		{"bad id chars", func(d *Document) { d.Root.ID = "root/x" }, "/root/id", "outside"},
		{"duplicate id", func(d *Document) {
			d.Root.Children = []Node{{Type: Spacer, ID: "a"}, {Type: Spacer, ID: "a"}}
		}, "/root/children/1/id", "duplicate id"},
		{"unknown type", func(d *Document) {
			d.Root.Children = []Node{{Type: "Widget", ID: "w"}}
		}, "/root/children/0/type", "unknown component type"},
		{"nested screen", func(d *Document) {
			d.Root.Children = []Node{{Type: Screen, ID: "s2"}}
		}, "/root/children/0/type", "only appear at the root"},
		{"leaf with children", func(d *Document) {
			d.Root.Children = []Node{{Type: Text, ID: "t", Props: map[string]any{"text": "x"},
				Children: []Node{{Type: Spacer, ID: "sp"}}}}
		}, "/root/children/0/children", "does not allow children"},
		{"disallowed child", func(d *Document) {
			d.Root.Children = []Node{{Type: Tabs, ID: "tabs", Children: []Node{{Type: Spacer, ID: "sp"}}}}
		}, "/root/children/0/children/0/type", "not an allowed child"},
		{"missing required prop", func(d *Document) {
			d.Root.Children = []Node{{Type: Text, ID: "t"}}
		}, "/root/children/0/props/text", "requires prop"},
		{"too deep", func(d *Document) { d.Root = deep(MaxDepth + 1) }, "", "nesting depth"},
		{"too many nodes", func(d *Document) { d.Root = wide(MaxNodes) }, "/root", "limit is"},
		{"props not serialisable", func(d *Document) {
			d.Root.Props = map[string]any{"fn": func() {}}
		}, "/root/props", "not JSON-serialisable"},
		{"unresolved action", func(d *Document) { d.Root.Action = "nope" }, "/root/action", "not defined"},
		{"unknown action type", func(d *Document) {
			d.Actions = map[string]Action{"x": {Type: "jump", Target: "a"}}
		}, "/actions/x/type", "unknown action type"},
		{"action missing target", func(d *Document) {
			d.Actions = map[string]Action{"x": {Type: Navigate}}
		}, "/actions/x/target", "requires a target"},
		{"bad action name", func(d *Document) {
			d.Actions = map[string]Action{"bad name": {Type: Back}}
		}, "/actions/bad name", "invalid action name"},
		{"action params not serialisable", func(d *Document) {
			d.Actions = map[string]Action{"x": {Type: Back, Params: map[string]any{"c": make(chan int)}}}
		}, "/actions/x/params", "not JSON-serialisable"},
		{"bad colour literal", func(d *Document) {
			d.Root.Style = &Style{Background: "red"}
		}, "/root/style/background", "not a hex"},
		{"bad direction", func(d *Document) { d.Root.Style = &Style{Direction: "diagonal"} }, "/root/style/direction", "row"},
		{"negative gap", func(d *Document) { d.Root.Style = &Style{Gap: -1} }, "/root/style/gap", "negative"},
		{"negative columns", func(d *Document) { d.Root.Style = &Style{Columns: -1} }, "/root/style/columns", "negative"},
		{"negative radius", func(d *Document) { d.Root.Style = &Style{Radius: -1} }, "/root/style/radius", "negative"},
		{"negative elevation", func(d *Document) { d.Root.Style = &Style{Elevation: -1} }, "/root/style/elevation", "negative"},
		{"bad opacity", func(d *Document) { d.Root.Style = &Style{Opacity: 1.5} }, "/root/style/opacity", "between"},
		{"bad aspect", func(d *Document) { d.Root.Style = &Style{AspectRatio: "wide"} }, "/root/style/aspectRatio", "16/9"},
		{"bad style module", func(d *Document) { d.Root.Style = &Style{Module: "Witty Play"} }, "/root/style/module", "invalid module"},
		{"bad style token", func(d *Document) {
			d.Root.Style = &Style{Tokens: map[string]string{"accent": "blue"}}
		}, "/root/style/tokens/accent", "not a hex"},
		{"bad token name", func(d *Document) {
			d.Root.Style = &Style{Tokens: map[string]string{"1bad": "#fff"}}
		}, "/root/style/tokens/1bad", "invalid token name"},
		{"bad binding path", func(d *Document) {
			d.Root.Bindings = map[string]Binding{"title": {Path: "user.name"}}
		}, "/root/bindings/title/path", "must start with $"},
		{"bad binding prop", func(d *Document) {
			d.Root.Bindings = map[string]Binding{"bad prop": {Path: "$.x"}}
		}, "/root/bindings/bad prop", "invalid prop name"},
		{"bad binding format", func(d *Document) {
			d.Root.Bindings = map[string]Binding{"title": {Path: "$.x", Format: "shout"}}
		}, "/root/bindings/title/format", "unknown format"},
		{"bad binding default", func(d *Document) {
			d.Root.Bindings = map[string]Binding{"title": {Path: "$.x", Default: func() {}}}
		}, "/root/bindings/title/default", "not JSON-serialisable"},
		{"condition min>max width", func(d *Document) {
			d.Root.Visibility = &Condition{MinWidth: 900, MaxWidth: 600}
		}, "/root/visibility", "minWidth 900 exceeds"},
		{"condition min>max height", func(d *Document) {
			d.Root.Visibility = &Condition{MinHeight: 900, MaxHeight: 600}
		}, "/root/visibility", "minHeight 900 exceeds"},
		{"condition negative", func(d *Document) {
			d.Root.Visibility = &Condition{MinWidth: -1}
		}, "/root/visibility", "negative"},
		{"condition nested bad module", func(d *Document) {
			d.Root.Visibility = &Condition{All: []Condition{{Module: "BAD"}}}
		}, "/root/visibility/all/0/module", "invalid module"},
		{"condition any/not nested", func(d *Document) {
			d.Root.Visibility = &Condition{Any: []Condition{{Not: &Condition{MinWidth: 5, MaxWidth: 1}}}}
		}, "/root/visibility/any/0/not", "exceeds"},
		{"theme empty id", func(d *Document) { d.Theme = &ThemeRef{} }, "/theme/id", "must not be empty"},
		{"theme bad module", func(d *Document) {
			d.Theme = &ThemeRef{ID: "default", ModuleOverrides: map[string]map[string]string{"Bad": {"accent": "#fff"}}}
		}, "/theme/moduleOverrides/Bad", "invalid module id"},
		{"theme bad token", func(d *Document) {
			d.Theme = &ThemeRef{ID: "default", ModuleOverrides: map[string]map[string]string{"wittyplay": {"-x": "#fff"}}}
		}, "/theme/moduleOverrides/wittyplay/-x", "invalid token name"},
		{"theme bad colour", func(d *Document) {
			d.Theme = &ThemeRef{ID: "default", ModuleOverrides: map[string]map[string]string{"wittyplay": {"accent": "green"}}}
		}, "/theme/moduleOverrides/wittyplay/accent", "not a hex"},
		{"negative ttl", func(d *Document) { d.Meta.TTLSeconds = -5 }, "/meta/ttlSeconds", "negative"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := minimalDoc()
			tc.mutate(&d)
			mustInvalid(t, d, tc.path, tc.message)
		})
	}
}

func TestValidate_GoodCases(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Document)
	}{
		{"required prop via binding", func(d *Document) {
			d.Root.Children = []Node{{Type: Text, ID: "t", Bindings: map[string]Binding{"text": {Path: "$.x"}}}}
		}},
		{"actions resolve", func(d *Document) {
			d.Actions = map[string]Action{"go": {Type: Navigate, Target: "/x"}, "back": {Type: Back}, "close": {Type: CloseModal}}
			d.Root.Action = "go"
		}},
		{"styles ok", func(d *Document) {
			d.Root.Style = &Style{Direction: "row", Gap: 8, Columns: 3, Radius: 4, Elevation: 1, Opacity: 0.5,
				AspectRatio: "2/3", Background: "token:bg", Foreground: "#fff", Accent: "rgba(1,2,3,.5)",
				Border: "hsl(120deg 50% 50%)", Module: "wittyplay", Tokens: map[string]string{"accent": "#abc"}}
		}},
		{"deep but allowed", func(d *Document) {
			root := Node{Type: Screen, ID: "root"}
			cur := &root
			for i := 0; i < MaxDepth-1; i++ {
				cur.Children = []Node{{Type: Stack, ID: "s" + itoa(i)}}
				cur = &cur.Children[0]
			}
			d.Root = root
		}},
		{"condition ok", func(d *Document) {
			d.Root.Visibility = &Condition{MinWidth: 100, MaxWidth: 200, MinHeight: 10, MaxHeight: 20, Module: "wittytv",
				All: []Condition{{Feature: "x"}}, Any: []Condition{{Role: "admin"}}, Not: &Condition{Platform: "tv"}}
		}},
		{"theme ok", func(d *Document) {
			d.Theme = &ThemeRef{ID: "midnight", ModuleOverrides: map[string]map[string]string{"wittyplay": {"accent": "#3ad29f"}}}
		}},
		{"id charset", func(d *Document) { d.Root.ID = "A-z_0.9:x" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := minimalDoc()
			tc.mutate(&d)
			if err := Validate(&d); err != nil {
				t.Fatalf("expected valid: %v", err)
			}
		})
	}
}

func TestValidate_Aggregates(t *testing.T) {
	d := minimalDoc()
	d.Version = 9
	d.Screen = ""
	d.Root.ID = ""
	err := Validate(&d)
	var ve *ValidationError
	if !errors.As(err, &ve) || len(ve.Issues) != 3 {
		t.Fatalf("expected 3 aggregated issues, got %v", err)
	}
	if !strings.Contains(err.Error(), "3 validation issue(s)") {
		t.Errorf("Error() should mention the count: %q", err.Error())
	}
	var empty *ValidationError
	if empty.Error() == "" {
		t.Error("nil ValidationError should still have a message")
	}
	if (&ValidationError{}).Error() == "" {
		t.Error("empty ValidationError should have a message")
	}
}

func TestValidateColor(t *testing.T) {
	good := []string{"#fff", "#ffff", "#a1b2c3", "#a1b2c3dd", "token:accent", "token:bg_2", "token:accent-text",
		"rgb(1,2,3)", "rgba(1, 2, 3, 0.5)", "hsl(120 50% 50%)", "hsla(120deg, 50%, 50%, .5)", "RGB(0 0 0 / 50%)"}
	for _, c := range good {
		if err := ValidateColor(c); err != nil {
			t.Errorf("ValidateColor(%q) = %v, want nil", c, err)
		}
	}
	bad := []string{"", "red", "#ff", "#fffff", "#ggg", "token:", "token:1x", "var(--accent)", "url(x)",
		"rgb(1,2,3", "rgb()", "rgb(1;2;3)", "hsl(expression(1))", "#12345", "transparent"}
	for _, c := range bad {
		if err := ValidateColor(c); err == nil {
			t.Errorf("ValidateColor(%q) = nil, want error", c)
		}
	}
}

func TestParseBindingPath(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"$", "", true},
		{"$.user.name", ".user.name", true},
		{"$.items[0].title", ".items[0].title", true},
		{"$[2]", "[2]", true},
		{"$.a-b.c_d", ".a-b.c_d", true},
		{"", "", false},
		{"user.name", "", false},
		{"$.", "", false},
		{"$..a", "", false},
		{"$.1a", "", false},
		{"$[", "", false},
		{"$[a]", "", false},
		{"$[1", "", false},
		{"$.a b", "", false},
		{"$['a']", "", false},
	}
	for _, tc := range cases {
		segs, err := ParseBindingPath(tc.in)
		if (err == nil) != tc.ok {
			t.Errorf("ParseBindingPath(%q) err=%v, want ok=%v", tc.in, err, tc.ok)
			continue
		}
		if !tc.ok {
			continue
		}
		var sb strings.Builder
		for _, s := range segs {
			sb.WriteString(s.String())
		}
		if sb.String() != tc.want {
			t.Errorf("ParseBindingPath(%q) = %q, want %q", tc.in, sb.String(), tc.want)
		}
	}
}

func TestCatalog(t *testing.T) {
	cat := Catalog()
	if len(cat) != 23 {
		t.Fatalf("expected 23 component specs, got %d", len(cat))
	}
	seen := map[ComponentType]bool{}
	for _, s := range cat {
		if seen[s.Type] {
			t.Errorf("duplicate catalog entry %s", s.Type)
		}
		seen[s.Type] = true
		if s.Description == "" {
			t.Errorf("%s has no description", s.Type)
		}
		if !s.AllowsChildren && s.AllowedChildren != nil {
			t.Errorf("%s is a leaf but lists allowed children", s.Type)
		}
		if _, ok := Spec(s.Type); !ok {
			t.Errorf("Spec(%s) not found", s.Type)
		}
	}
	// Mutating the copy must not affect the package catalogue.
	cat[0].RequiredProps = append(cat[0].RequiredProps, "zzz")
	if s, _ := Spec(Screen); len(s.RequiredProps) != 0 {
		t.Error("Catalog() did not return a copy")
	}
	if _, ok := Spec("Nope"); ok {
		t.Error("Spec of unknown type should be false")
	}
}

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{digits[i%10]}, b...)
		i /= 10
	}
	return string(b)
}
