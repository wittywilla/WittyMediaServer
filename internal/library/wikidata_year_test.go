// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// "Joy Ride (2023)": the name search finds the 2001 film only; the full text with the year
// finds the 2023 one, which is the match.
func TestEnrichLooksAgainWithTheYearWhenNoFilmOfThatYearIsNamed(t *testing.T) {
	film := func(id string, year int) string {
		return fmt.Sprintf(`"%s":{"id":"%s","descriptions":{"en":{"value":"%d film"}},"claims":{"P31":[{"mainsnak":{"datavalue":{"value":{"id":"Q11424"}}}}],"P577":[{"mainsnak":{"datavalue":{"value":{"time":"+%d-01-01T00:00:00Z"}}}}]}}`, id, id, year, year)
	}
	var searched []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("action") {
		case "wbsearchentities":
			w.Write([]byte(`{"search":[{"id":"Q2001","description":"2001 film by John Dahl"}]}`))
		case "query":
			searched = append(searched, q.Get("srsearch"))
			if strings.Contains(q.Get("srsearch"), "2023") {
				w.Write([]byte(`{"query":{"search":[{"title":"Q2023"}]}}`))
			} else {
				w.Write([]byte(`{"query":{"search":[]}}`))
			}
		case "wbgetentities":
			var parts []string
			for _, id := range strings.Split(q.Get("ids"), "|") {
				switch id {
				case "Q2001":
					parts = append(parts, film(id, 2001))
				case "Q2023":
					parts = append(parts, film(id, 2023))
				}
			}
			w.Write([]byte(`{"entities":{` + strings.Join(parts, ",") + `}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	w := &Wikidata{Client: &http.Client{Transport: wikipediaTo{srv.URL}}, API: srv.URL + "/wd", CommonsAPI: srv.URL + "/c"}
	year := 2023
	it := &Item{Title: "Joy Ride", Year: &year, Overview: "kept", Poster: "kept"}
	if err := w.Enrich(context.Background(), it); err != nil {
		t.Fatalf("%v (full-text searches: %q)", err, searched)
	}
	if it.ProviderID != "Q2023" {
		t.Errorf("matched %q", it.ProviderID)
	}
	// A file of a year no film has stays unmatched.
	year = 1950
	other := &Item{Title: "Joy Ride", Year: &year, Overview: "kept", Poster: "kept"}
	if err := w.Enrich(context.Background(), other); err != ErrNotFound {
		t.Errorf("1950: %v, matched %q", err, other.ProviderID)
	}
}
