// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// A library page can be filtered by streaming service: the filter bar offers the services
// that list its titles, and service= (a page's own name for the library's platform filter,
// since platform= is the device's) narrows the grid to what one of them streams.
func TestSDUILibraryFiltersByStreamingService(t *testing.T) {
	ts, admin, adult, _ := sduiAccounts(t)
	ctx := context.Background()
	lib := ts.library(t, "movies", t.TempDir())
	var ids []int64
	for _, title := range []string{"On Both", "Only Here", "Nowhere"} {
		ids = append(ids, ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Title: title, Path: "/m/" + title + ".mkv"}).ID)
	}
	_ = ts.DB.SetSetting(ctx, "listingsConsent", library.ListingsVersion)
	_ = ts.DB.SetSetting(ctx, "listingsRegion", "US")
	for _, row := range []struct {
		id        int64
		key, name string
	}{{ids[0], "streamy", "Streamy"}, {ids[0], "flicks", "Flicks"}, {ids[1], "streamy", "Streamy"}} {
		if _, err := ts.DB.ExecContext(ctx, `INSERT INTO listings(target, target_id, region, platform_key, platform, offer) VALUES ('item', ?, 'US', ?, ?, 'flatrate')`, row.id, row.key, row.name); err != nil {
			t.Fatal(err)
		}
	}
	var cfg struct{ ETag string }
	_ = json.Unmarshal(ts.do(t, "GET", "/api/sdui/config", admin, nil).Body.Bytes(), &cfg)
	if w := ts.do(t, "PUT", "/api/sdui/config/pages/films-by-service", admin, filmsPage(lib), "If-Match", cfg.ETag); w.Code != 200 {
		t.Fatalf("saving the page: %d %s", w.Code, w.Body)
	}

	doc := sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/library?library="+itoa(lib)+"&formFactor=desktop", adult, nil))
	bar, grid := findNode(&doc.Root, "filters"), findNode(&doc.Root, "all")
	if bar == nil || grid == nil {
		t.Fatalf("no filter bar or grid: %+v", doc.Root.Children)
	}
	b, _ := json.Marshal(bar.Props)
	if !strings.Contains(string(b), `"service"`) || !strings.Contains(string(b), `"Streamy"`) || !strings.Contains(string(b), `"Flicks"`) {
		t.Fatalf("the filter bar doesn't offer the services: %s", b)
	}
	if len(grid.Children) != 3 {
		t.Fatalf("unfiltered: %d titles", len(grid.Children))
	}
	doc = sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/library?library="+itoa(lib)+"&formFactor=desktop&platform=web&service=streamy", adult, nil))
	grid = findNode(&doc.Root, "all")
	if grid == nil || len(grid.Children) != 2 {
		t.Fatalf("filtered by a service: %+v", grid)
	}
	if doc.Page.Params["service"] != "streamy" {
		t.Errorf("the page's params don't carry the service: %v", doc.Page.Params)
	}
	// With listings off, the filter isn't offered. (Another form factor: the same request
	// a moment ago is answered from the render the server just made.)
	_ = ts.DB.SetSetting(ctx, "listingsConsent", "")
	doc = sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/library?library="+itoa(lib)+"&formFactor=tablet", adult, nil))
	b, _ = json.Marshal(findNode(&doc.Root, "filters").Props)
	if strings.Contains(string(b), `"Streamy"`) {
		t.Errorf("services offered with listings off: %s", b)
	}
}

// filmsPage is a library page for films with a filter bar that offers the services.
func filmsPage(lib int64) sdui.Page {
	return sdui.Page{ID: "films-by-service", Kind: "library", Title: "{library}", Match: &sdui.Match{Library: lib}, Sections: []sdui.Section{
		{ID: "filters", Component: "FilterBar", Props: map[string]any{"target": "all", "fields": []any{"genre", "service"}}},
		{ID: "all", Component: "Grid", Source: sdui.Source{"type": "browse", "library": "$library", "filterable": true}},
	}}
}
