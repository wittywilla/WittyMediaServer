// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"strings"
	"testing"
)

// Any library can be made read-only from Settings, Libraries, games among them (#398).
func TestAGamesLibraryCanBeReadOnly(t *testing.T) {
	h, admin, kid, s := tvServer(t)
	lib, err := s.Store.CreateLibrary(context.Background(), "Games", "games", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/libraries/" + itoa(lib.ID) + "/read-only"
	if w := tvCall(h, "PUT", path, kid, `{"readOnly":true}`); w.Code != 403 {
		t.Errorf("someone who isn't an administrator: %d", w.Code)
	}
	if w := tvCall(h, "PUT", path, admin, `{"readOnly":true}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got, _ := s.Store.Library(context.Background(), lib.ID); got == nil || !got.ReadOnly {
		t.Fatal("not read-only after saying so")
	}
	if !strings.Contains(tvCall(h, "GET", "/api/libraries", admin, "").Body.String(), `"readOnly":true`) {
		t.Error("the list of libraries doesn't say so")
	}
	if w := tvCall(h, "PUT", path, admin, `{"readOnly":false}`); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if got, _ := s.Store.Library(context.Background(), lib.ID); got.ReadOnly {
		t.Error("still read-only")
	}
	if w := tvCall(h, "PUT", "/api/libraries/99999/read-only", admin, `{"readOnly":true}`); w.Code != 404 {
		t.Errorf("a library that doesn't exist: %d", w.Code)
	}
}
