// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSeveralPicturesARole(t *testing.T) {
	x := New(ScopeItem)
	if _, err := x.SetImage("poster", ".png", pngBytes(t, 4, 6)); err != nil {
		t.Fatal(err)
	}
	second, err := x.AddImage("poster", ".png", pngBytes(t, 5, 6))
	if err != nil {
		t.Fatal(err)
	}
	third, _ := x.AddImage("poster", ".png", pngBytes(t, 6, 6))
	if got := x.Artwork["poster"]; len(got) != 3 || got[0] != "images/poster.png" || second != "images/poster-2.png" || third != "images/poster-3.png" {
		t.Fatalf("posters: %v", got)
	}
	// Replacing the one shown keeps the others.
	if _, err := x.SetImage("poster", ".jpg", pngBytes(t, 7, 6)); err == nil {
		t.Fatal("a PNG named .jpg was taken")
	}
	if _, err := x.SetImage("poster", ".png", pngBytes(t, 7, 6)); err != nil || len(x.Artwork["poster"]) != 3 {
		t.Fatalf("replacing the first: %v %v", err, x.Artwork["poster"])
	}
	if !x.ShowFirst("poster", third) || x.Artwork.First("poster") != third {
		t.Fatalf("show first: %v", x.Artwork["poster"])
	}
	if !x.RemoveImage("poster", second) || len(x.Artwork["poster"]) != 2 || x.Files[second] != nil {
		t.Fatalf("remove one: %v", x.Artwork["poster"])
	}

	// One picture is written as a name, several as a list, and both read back.
	x.AddImage("backdrop", ".png", pngBytes(t, 8, 4))
	b, err := x.Encode()
	if err != nil {
		t.Fatal(err)
	}
	y, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := json.Marshal(y.Artwork)
	if !strings.Contains(string(m), `"backdrop":"images/backdrop`) || !strings.Contains(string(m), `"poster":["`) {
		t.Fatalf("written as %s", m)
	}
	if len(y.Artwork["poster"]) != 2 || len(y.Files) != 3 {
		t.Fatalf("read back: %v, %d files", y.Artwork, len(y.Files))
	}

	if !x.RemoveImage("poster", "") || x.Artwork["poster"] != nil {
		t.Fatalf("remove all: %v", x.Artwork)
	}
}

func TestPictureListsChecked(t *testing.T) {
	img := pngBytes(t, 4, 4)
	for name, art := range map[string]string{
		"empty list":   `{"poster": []}`,
		"named twice":  `{"poster": ["images/a.png", "images/a.png"]}`,
		"missing":      `{"poster": ["images/a.png", "images/gone.png"]}`,
		"not a string": `{"poster": [3]}`,
		"outside":      `{"poster": ["../a.png"]}`,
	} {
		manifest := `{"format":"wittyinfo","version":1,"artwork":` + art + `}`
		b := zipOf(t, map[string][]byte{"manifest.json": []byte(manifest), "images/a.png": img})
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
	many := make([]string, MaxPicturesPerRole+1)
	files := map[string][]byte{}
	for i := range many {
		many[i] = "images/p" + strings.Repeat("x", i+1) + ".png"
		files[many[i]] = img
	}
	list, _ := json.Marshal(many)
	files["manifest.json"] = []byte(`{"format":"wittyinfo","version":1,"artwork":{"poster":` + string(list) + `}}`)
	if _, err := Parse(zipOf(t, files)); err == nil {
		t.Error("too many pictures for a role were taken")
	}
	x := New(ScopeItem)
	for i := 0; i < MaxPicturesPerRole; i++ {
		if _, err := x.AddImage("backdrop", ".png", img); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := x.AddImage("backdrop", ".png", img); err == nil {
		t.Error("AddImage went past the most a role holds")
	}
	if err := x.ChangeImages("sticker", ImageChange{Picture: img, Ext: ".png"}); err == nil {
		t.Error("an unknown role was taken")
	}
	if err := x.ChangeImages("backdrop", ImageChange{First: "images/none.png"}); err == nil {
		t.Error("showing a picture it doesn't have first was taken")
	}
}
