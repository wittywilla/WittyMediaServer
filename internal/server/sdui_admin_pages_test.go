// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
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
	if n := findNode(&doc.Root, "all"); n == nil || len(n.Children) != 1 || n.Children[0].Action == nil || n.Children[0].Action.Op != "library.scan" {
		t.Fatalf("libraries for an administrator: %+v", n)
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
