// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// The administrators' pages: an administrator gets the libraries, a form to add one and a title's match
// page; anyone else a notice and no rows.
func TestSDUIAdminPages(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "movies", root)
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "a.mkv"), Title: "Unknown Film"})
	_, admin := ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "bob", "adult", false, "")

	doc := sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/manage-libraries?formFactor=web", admin, nil))
	if n := findNode(&doc.Root, "all"); n == nil || len(n.Children) != 1 || n.Children[0].Action == nil || n.Children[0].Action.To != fmt.Sprintf("/manage/library/%d", lib) || n.Children[0].Action.As != "layer" {
		t.Fatalf("libraries for an administrator: %+v", n)
	}
	// A library's scan choices: Scan for changes, and Full rescan, which asks first.
	one := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/manage-library?formFactor=web&item=%d", lib), admin, nil))
	n := findNode(&one.Root, "choices")
	if n == nil || len(n.Children) < 2 {
		t.Fatalf("scan choices: %+v", n)
	}
	changes, full := n.Children[len(n.Children)-2], n.Children[len(n.Children)-1]
	if changes.Action == nil || changes.Action.Op != "library.scan" || changes.Action.Confirm != "" {
		t.Errorf("scan for changes: %+v", changes.Action)
	}
	if full.Action == nil || full.Action.Op != "library.rescan" || full.Action.Confirm == "" {
		t.Errorf("full rescan: %+v", full.Action)
	}
	one = sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/manage-library?formFactor=web&item=%d", lib), adult, nil))
	if findNode(&one.Root, "choices") != nil || findNode(&one.Root, "not") == nil {
		t.Errorf("a profile that isn't an administrator sees the scan choices: %+v", one.Root)
	}
	if f := findNode(&doc.Root, "add"); f == nil || f.Action == nil || f.Action.Op != "library.create" {
		t.Errorf("the add form: %+v", f)
	}
	doc = sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/manage-libraries?formFactor=web", adult, nil))
	if findNode(&doc.Root, "all") != nil || findNode(&doc.Root, "not") == nil {
		t.Errorf("a profile that isn't an administrator sees the libraries: %+v", doc.Root)
	}
	doc = sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/v1/sdui/pages/match?formFactor=web&item=%d", it.ID), admin, nil))
	if n := findNode(&doc.Root, "candidates"); n == nil || len(n.Children) == 0 || n.Children[len(n.Children)-1].Action.Op != "meta.unmatch" {
		t.Errorf("match page: %+v", n)
	}
}

// The You page's Libraries row opens /manage/libraries on every client: the route is the page
// manage-libraries, and on every form factor (a television's too) an administrator gets its rows, each
// opening that library's scan choices as a layer.
func TestManageLibrariesOnEveryFormFactor(t *testing.T) {
	routes := map[string]sdui.RouteSpec{}
	for _, r := range sdui.Catalog().Routes {
		routes[r.Pattern] = r
	}
	if r := routes["/manage/libraries"]; r.Page != "manage-libraries" || r.Native != "" {
		t.Fatalf("/manage/libraries: %+v", r)
	}
	if r := routes["/manage/library/{item}"]; r.Page != "manage-library" || r.Params["item"] != "{item}" {
		t.Fatalf("/manage/library/{item}: %+v", r)
	}
	ts := newTestServer(t)
	lib := ts.library(t, "movies", t.TempDir())
	_, admin := ts.user(t, "boss", "adult", true, "")
	for _, ff := range []string{"phone", "tablet", "tv", "desktop", "web"} {
		doc := sduiDoc(t, ts.do(t, "GET", "/api/v1/sdui/pages/manage-libraries?formFactor="+ff, admin, nil))
		n := findNode(&doc.Root, "all")
		if n == nil || len(n.Children) != 1 || n.Children[0].Action == nil || n.Children[0].Action.To != fmt.Sprintf("/manage/library/%d", lib) || n.Children[0].Action.As != "layer" {
			t.Errorf("%s: libraries for an administrator: %+v", ff, n)
		}
	}
}
