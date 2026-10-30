package themes

import (
	"io/fs"
	"testing"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

func TestNormalizeColor(t *testing.T) {
	for in, want := range map[string]string{
		"#0f1115":                  "#0F1115",
		"#abc":                     "#AABBCC",
		"#abcd":                    "#AABBCCDD",
		"#11223344":                "#11223344",
		"rgba(0, 0, 0, 0.6)":       "#00000099",
		"rgb(10 20 30)":            "#0A141E",
		"rgb(10 20 30 / 50%)":      "#0A141E80",
		"transparent":              "#00000000",
		"white":                    "#FFFFFF",
		"linear-gradient(red,red)": "",
		"#12":                      "",
		"rgb(300,0,0)":             "",
	} {
		got, ok := NormalizeColor(in)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v, want %q", in, got, ok, want)
		}
	}
}

func TestParseTokens(t *testing.T) {
	tk := parseTokens(`/* a comment with --fake: 1; */
:root {
  color-scheme: dark;
  --bg: #000; /* page */
  --accent: rgb(1, 2, 3);
  --brand: var(--accent);
  --accent-rgb: 1 2 3;
  --radius: 10px;
  --big: 1.5rem;
  --shadow: 0 1px 2px rgba(0,0,0,.5);
  --font: "Inter", system-ui, 'Segoe UI';
}`)
	if tk.Colors["bg"] != "#000000" || tk.Colors["accent"] != "#010203" || tk.Colors["brand"] != "#010203" {
		t.Errorf("colours: %v", tk.Colors)
	}
	if _, ok := tk.Colors["accent-rgb"]; ok {
		t.Error("the browser-only triple should be left out")
	}
	if tk.Lengths["radius"] != 10 || tk.Lengths["big"] != 24 {
		t.Errorf("lengths: %v", tk.Lengths)
	}
	if tk.Other["shadow"] == "" {
		t.Errorf("other: %v", tk.Other)
	}
	if len(tk.Fonts) != 3 || tk.Fonts[0] != "Inter" || tk.Fonts[2] != "Segoe UI" {
		t.Errorf("fonts: %v", tk.Fonts)
	}
	if tk.WebOnly {
		t.Error("variables alone are not web-only")
	}
	if !parseTokens(`:root{--bg:#000} .card{border:0}`).WebOnly {
		t.Error("own selectors are web-only")
	}
}

// A native app must be able to rely on these in every theme the server ships.
func TestBuiltinThemesAreReadableByNativeApps(t *testing.T) {
	dirs, err := fs.ReadDir(builtin.Builtin, ".")
	if err != nil {
		t.Fatal(err)
	}
	need := []string{"bg", "bg-2", "bg-3", "bg-4", "border", "fg", "fg-2", "fg-3", "accent", "accent-2", "accent-fg", "accent-text", "danger", "ok", "overlay"}
	n := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		n++
		css, err := fs.ReadFile(builtin.Builtin, d.Name()+"/theme.css")
		if err != nil {
			t.Fatal(err)
		}
		tk := parseTokens(string(css))
		for _, name := range need {
			if tk.Colors[name] == "" {
				t.Errorf("%s: no colour --%s", d.Name(), name)
			}
		}
		if tk.Lengths["radius"] == 0 {
			t.Errorf("%s: no --radius", d.Name())
		}
		if len(tk.Fonts) == 0 {
			t.Errorf("%s: no --font", d.Name())
		}
	}
	if n < 20 {
		t.Errorf("only %d built-in themes found", n)
	}
}
