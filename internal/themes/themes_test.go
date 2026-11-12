// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstallListAndUpload(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	if err := m.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	list := m.List()
	if len(list) != 15 || list[0].ID != "default" {
		t.Fatalf("built-in themes: %d, first %q", len(list), list[0].ID)
	}
	css, ct, err := m.File("midnight", "theme.css")
	if err != nil || !strings.HasPrefix(ct, "text/css") || !strings.Contains(string(css), "--accent: #") {
		t.Fatalf("midnight css: %v %s", err, ct)
	}
	// A server still set to a theme that's gone gets the look that replaced it.
	if old, _, err := m.File("light-blue", "theme.css"); err != nil || !strings.Contains(string(old), "color-scheme: light") {
		t.Fatalf("retired theme: %v", err)
	}
	good := zipOf(t, map[string]string{
		"theme.json":    `{"id":"my-cinema","name":"My Cinema","scheme":"dark"}`,
		"theme.css":     `:root{--accent:#ff0}body{background:url(assets/bg.jpg)}`,
		"assets/bg.jpg": "jpeg",
		"README.md":     "hi",
	})
	meta, err := m.Add(good, "upload.zip")
	if err != nil || meta.ID != "my-cinema" {
		t.Fatalf("upload: %v %+v", err, meta)
	}
	if b, ct, err := m.File("my-cinema", "assets/bg.jpg"); err != nil || ct != "image/jpeg" || string(b) != "jpeg" {
		t.Fatalf("asset: %v %s", err, ct)
	}
	if _, _, err := m.File("my-cinema", "../default.zip"); err == nil {
		t.Fatal("path traversal served a file")
	}
	for name, files := range map[string]map[string]string{
		"no css":     {"theme.json": `{"name":"x"}`},
		"script":     {"theme.json": `{"name":"x"}`, "theme.css": "", "evil.js": "alert(1)"},
		"traversal":  {"theme.json": `{"name":"x"}`, "theme.css": "", "../x.css": ""},
		"builtin id": {"theme.json": `{"id":"default","name":"x"}`, "theme.css": ""},
	} {
		if _, err := m.Add(zipOf(t, files), "x.zip"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := m.Remove("default"); err == nil {
		t.Fatal("removed a built-in theme")
	}
	if err := m.Remove("my-cinema"); err != nil {
		t.Fatal(err)
	}
}

func TestRetiredBuiltinsGoAndOwnThemesStay(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	write := func(id, meta string) {
		if err := writeAtomic(filepath.Join(m.Dir, id+".zip"), zipOf(t, map[string]string{"theme.json": meta, "theme.css": ":root{}"})); err != nil {
			t.Fatal(err)
		}
	}
	write("dark-red", `{"name":"Dark Red","scheme":"dark","builtin":true}`)
	write("dark-blue", `{"name":"My own blue","scheme":"dark"}`)
	if err := m.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, th := range m.List() {
		seen[th.ID] = th.Name
	}
	if _, ok := seen["dark-red"]; ok {
		t.Fatal("the retired built-in is still listed")
	}
	if seen["dark-blue"] != "My own blue" {
		t.Fatal("someone's own theme was removed")
	}
}

func TestAddCustom(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	colors := map[string]string{}
	for _, k := range customVars {
		colors[k] = "#336699"
	}
	meta, err := m.AddCustom(Custom{Name: "  My   Look! ", Scheme: "dark", Colors: colors, Radius: 12})
	if err != nil || meta.ID != "custom-my-look" || meta.Name != "My Look!" {
		t.Fatalf("%v %+v", err, meta)
	}
	css, _, err := m.File(meta.ID, "theme.css")
	if err != nil || !strings.Contains(string(css), "--accent: #336699;") || !strings.Contains(string(css), "--radius: 12px;") || !strings.Contains(string(css), "--bg-rgb: 51 102 153;") {
		t.Fatalf("%v %s", err, css)
	}
	for name, c := range map[string]Custom{
		"no name": {Scheme: "dark", Colors: colors},
		"scheme":  {Name: "x", Scheme: "blue", Colors: colors},
		"corners": {Name: "x", Scheme: "dark", Colors: colors, Radius: 99},
		"missing": {Name: "x", Scheme: "dark", Colors: map[string]string{"bg": "#000000"}},
		"injection": {Name: "x", Scheme: "dark", Colors: func() map[string]string {
			c := map[string]string{}
			for k, v := range colors {
				c[k] = v
			}
			c["bg"] = "#000;}body{display:none"
			return c
		}()},
	} {
		if _, err := m.AddCustom(c); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// One gives exactly what List gives for that theme, without reading the others.
func TestOneIsWhatListShows(t *testing.T) {
	m := &Manager{Dir: t.TempDir()}
	if err := m.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	list := m.List()
	if len(list) < 2 {
		t.Fatalf("too few themes: %d", len(list))
	}
	for _, want := range list {
		got, ok := m.One(strings.ToUpper(want.ID))
		if !ok || !reflect.DeepEqual(*got, want) {
			t.Errorf("%s: %+v, want %+v", want.ID, got, want)
		}
	}
	for _, id := range []string{"no-such-theme", "../default", ""} {
		if _, ok := m.One(id); ok {
			t.Errorf("%q found", id)
		}
	}
}
