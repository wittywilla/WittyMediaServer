// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymodule"
)

// The host API's side of scan hooks (host API 2): what a module that scans a kind of library
// found out later about one of its items, and a rescan it asks for. Only libraries of a kind
// the calling module scans, and their items, are its to touch.

// maxItemFields bounds what a module sets on an item at once.
const maxItemFields = 256 << 10

// PATCH /host/v1/items/{id} wittymodule.ItemFields: sets an item's generic fields. A field a
// person typed stays; a picture must be one PutImage kept for this module.
func (s *Server) hostSetItem(w http.ResponseWriter, r *http.Request, module string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, 404, map[string]string{"error": "no such item"})
		return
	}
	var f wittymodule.ItemFields
	if err := json.NewDecoder(io.LimitReader(r.Body, maxItemFields)).Decode(&f); err != nil {
		writeJSON(w, 400, map[string]string{"error": "not an item's fields"})
		return
	}
	if f.ID != 0 && f.ID != id {
		writeJSON(w, 400, map[string]string{"error": "the fields are for another item"})
		return
	}
	f.ID = id
	it, err := s.Store.Item(r.Context(), id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no such item"})
		return
	}
	lib, err := s.Store.Library(r.Context(), it.LibraryID)
	if err != nil || !s.moduleScans(module, lib.Type) {
		writeJSON(w, 404, map[string]string{"error": "no such item"}) // not in a library it scans
		return
	}
	if err := s.Scanner.ApplyItemFields(r.Context(), module, f); err != nil {
		if errors.Is(err, library.ErrModuleArt) {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(204)
}

// POST /host/v1/libraries/{id}/scan: scans a library of a kind the module scans, as soon as it
// can (after a person decided what a file is). A held library has nothing to scan.
func (s *Server) hostRescan(w http.ResponseWriter, r *http.Request, module string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no such library"})
		return
	}
	lib, err := s.Store.Library(r.Context(), id)
	if err != nil || !s.moduleScans(module, lib.Type) {
		writeJSON(w, 404, map[string]string{"error": "no such library"})
		return
	}
	if lib.IsRig() {
		writeJSON(w, 409, map[string]string{"error": "a held library's items come from its module, not from a scan"})
		return
	}
	started := s.Scanner.StartBy(lib.ID, library.ScanChanges, tasks.By{Name: s.hostMods().Name(module)})
	writeJSON(w, 202, map[string]bool{"started": started})
}
