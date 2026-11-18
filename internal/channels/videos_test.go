// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package channels

import (
	"context"
	"fmt"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// The editor's video lists come a page at a time, from the libraries asked for.
func TestVideoPages(t *testing.T) {
	s, ls := testStore(t)
	ctx := context.Background()
	ads, _ := ls.CreateLibrary(ctx, "Ads", library.CommercialsType, []string{"/ads"})
	idents, _ := ls.CreateLibrary(ctx, "Idents", library.CommercialsType, []string{"/id"})
	other, _ := ls.CreateLibrary(ctx, "Clips", "videos", []string{"/c"})
	add := func(path, title string, libID int64) int64 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, added_at, updated_at) VALUES(?, 'video', ?, ?, ?, 15, 1, 1)`, libID, path, title, title)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	for i := range 60 {
		add(fmt.Sprintf("/ads/%02d.mp4", i), fmt.Sprintf("Ad %02d", i), ads.ID)
	}
	ident := add("/id/a.mp4", "Station ident", idents.ID)
	chosen := add("/c/x.mp4", "Sting 100% loud", other.ID)
	add("/c/y.mp4", "Not a bumper", other.ID)
	if err := s.AddBumper(ctx, chosen, false); err != nil {
		t.Fatal(err)
	}

	page := func(q VideoQuery) VideoPage {
		t.Helper()
		p, err := s.Videos(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := page(VideoQuery{Libraries: []int64{ads.ID}, Limit: 25, Offset: 50}); p.Total != 60 || len(p.Items) != 10 || p.Items[0].Title != "Ad 50" {
		t.Errorf("the last page of the ads: %d of %d, first %+v", len(p.Items), p.Total, p.Items)
	}
	if p := page(VideoQuery{Libraries: []int64{ads.ID}}); len(p.Items) != 50 {
		t.Errorf("a page holds 50 unless asked: %d", len(p.Items))
	}
	if p := page(VideoQuery{Libraries: []int64{ads.ID}, Limit: 5000}); len(p.Items) > maxVideoPage {
		t.Errorf("a page is never more than %d: %d", maxVideoPage, len(p.Items))
	}
	if p := page(VideoQuery{Libraries: []int64{idents.ID}, Own: true}); p.Total != 2 || p.Items[0].ItemID != ident || p.Items[1].ItemID != chosen || !p.Items[1].Chosen {
		t.Errorf("a library and the bumper list: %+v", p)
	}
	if p := page(VideoQuery{Own: true, Search: "100%"}); p.Total != 1 || p.Items[0].ItemID != chosen {
		t.Errorf("search: %+v", p)
	}
	if p := page(VideoQuery{Libraries: []int64{ads.ID}, Search: "%"}); p.Total != 60 {
		t.Errorf("a %% in a search is not a wildcard of its own: %d", p.Total)
	}
	// Chosen videos, wherever they are, and only those.
	if p := page(VideoQuery{IDs: []int64{ident, chosen}}); p.Total != 2 {
		t.Errorf("chosen videos: %+v", p)
	}
	if p := page(VideoQuery{Libraries: []int64{ads.ID}, IDs: []int64{ident}}); p.Total != 0 {
		t.Errorf("ids within a library: %+v", p)
	}
	if p := page(VideoQuery{}); p.Total != 0 || p.Items == nil {
		t.Errorf("nothing asked for should list nothing, as an empty list: %+v", p)
	}
}
