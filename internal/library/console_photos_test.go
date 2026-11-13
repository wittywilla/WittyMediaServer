// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeConsoleWiki answers as Wikidata and Commons would, with nothing on the network: entities by id, and Commons' facts for any file.
func fakeConsoleWiki(t *testing.T, entities map[string]any, license string, asked *[]string) *WikimediaLogos {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		*asked = append(*asked, q.Get("action")+":"+q.Get("ids")+q.Get("search")+q.Get("titles"))
		switch q.Get("action") {
		case "wbsearchentities":
			json.NewEncoder(w).Encode(map[string]any{"search": []map[string]string{{"id": "Q900"}}})
		case "wbgetentities":
			json.NewEncoder(w).Encode(map[string]any{"entities": entities})
		case "query":
			json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": map[string]any{"1": map[string]any{"title": q.Get("titles"), "imageinfo": []map[string]any{{
				"thumburl": "https://upload.wikimedia.org/console.jpg", "descriptionurl": "https://commons.wikimedia.org/wiki/File:Console.jpg",
				"extmetadata": map[string]any{"LicenseShortName": map[string]string{"value": license}, "Artist": map[string]string{"value": "<a href='x'>Evan Amos</a>"}},
			}}}}}})
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return &WikimediaLogos{Client: srv.Client(), WikidataAPI: srv.URL + "/wd", CommonsAPI: srv.URL + "/c"}
}

func consoleEntity(label, desc, image string) map[string]any {
	return map[string]any{"labels": map[string]any{"en": map[string]string{"value": label}}, "descriptions": map[string]any{"en": map[string]string{"value": desc}},
		"claims": map[string]any{"P18": []map[string]any{{"rank": "normal", "mainsnak": map[string]any{"datavalue": map[string]any{"value": image}}}}}}
}

func TestConsolePhotoFromItsEntity(t *testing.T) {
	var asked []string
	w := fakeConsoleWiki(t, map[string]any{"Q183259": consoleEntity("Super Nintendo Entertainment System", "home video game console", "SNES-Mod1-Console-Set.jpg")}, "Public domain", &asked)
	c, err := w.ConsolePhoto(context.Background(), "snes", "Super Nintendo Entertainment System")
	if err != nil {
		t.Fatal(err)
	}
	if c.ImageURL != "https://upload.wikimedia.org/console.jpg" || c.License != "Public domain" || c.Credit != "Evan Amos" || c.PageURL == "" {
		t.Errorf("got %+v", c)
	}
	// The table's entity is asked for directly: no search.
	for _, a := range asked {
		if a[:len("wbsearch")] == "wbsearch" {
			t.Errorf("searched although the entity is known: %v", asked)
		}
	}
}

func TestConsolePhotoRefusesWhatIsNotAConsoleOrNotFree(t *testing.T) {
	var asked []string
	// An entity that isn't a console (a wrong id, or a search's namesake) is never used.
	w := fakeConsoleWiki(t, map[string]any{"Q183259": consoleEntity("Super Nintendo", "1993 album", "Cover.jpg")}, "Public domain", &asked)
	if _, err := w.ConsolePhoto(context.Background(), "snes", "Super Nintendo Entertainment System"); !errors.Is(err, ErrNoPhoto) {
		t.Errorf("an album's picture was taken: %v", err)
	}
	// A licence that doesn't allow this use: no picture.
	w = fakeConsoleWiki(t, map[string]any{"Q183259": consoleEntity("Super Nintendo Entertainment System", "video game console", "SNES.jpg")}, "Fair use", &asked)
	if _, err := w.ConsolePhoto(context.Background(), "snes", "Super Nintendo Entertainment System"); !errors.Is(err, ErrNoPhoto) {
		t.Errorf("a non-free picture was taken: %v", err)
	}
}

func TestConsolePhotoBySearchNeedsTheExactName(t *testing.T) {
	var asked []string
	w := fakeConsoleWiki(t, map[string]any{"Q900": consoleEntity("Zeta Pocket", "handheld game console", "Zeta.jpg")}, "CC BY-SA 4.0", &asked)
	if _, err := w.ConsolePhoto(context.Background(), "custom-zeta", "Zeta Pocket"); err != nil {
		t.Errorf("the console of that name was not found: %v", err)
	}
	if _, err := w.ConsolePhoto(context.Background(), "custom-other", "Omega Pocket"); !errors.Is(err, ErrNoPhoto) {
		t.Errorf("another name's console was taken: %v", err)
	}
	if ConsoleWikidataID("snes") == "" || ConsoleWikidataID("nothing") != "" {
		t.Error("the table's ids")
	}
}
