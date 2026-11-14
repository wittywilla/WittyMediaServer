// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Libraries a module holds (module API 5, host API): a games library whose games come from
// somewhere other than files, such as a gaming PC's (WittyRigs). They stay the server's own
// libraries, so every access rule applies to their games, but nothing scans them: the module
// adds, renames and removes them, and the games in them, here. Such a library has the console
// "rig" (library.RigConsole), and only a module that serves games libraries may make one.

// heldPrograms is what the host API asks the module manager about a module's programs.
type heldPrograms interface {
	Requires(id string) []string
	ProgramCommand(id string) (path string, args []string, version string, ok bool)
}

func (s *Server) heldLibrary(w http.ResponseWriter, r *http.Request, module string) *library.Library {
	if !s.servesKind(module, "games") {
		writeJSON(w, 403, map[string]string{"error": "only a module serving games libraries holds one"})
		return nil
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no such library"})
		return nil
	}
	lib, err := s.Store.Library(r.Context(), id)
	if err != nil || !lib.IsRig() {
		writeJSON(w, 404, map[string]string{"error": "no such library"})
		return nil
	}
	return lib
}

type heldLibraryView struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Console string `json:"console"`
}

func heldName(r *http.Request) (string, bool) {
	var b struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &b); err != nil {
		return "", false
	}
	name := strings.TrimSpace(b.Name)
	return name, name != "" && len(name) <= 100
}

// POST /host/v1/libraries {name}
func (s *Server) hostLibraryAdd(w http.ResponseWriter, r *http.Request, module string) {
	if !s.servesKind(module, "games") {
		writeJSON(w, 403, map[string]string{"error": "only a module serving games libraries holds one"})
		return
	}
	name, ok := heldName(r)
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "a name of up to 100 characters"})
		return
	}
	lib, err := s.Store.CreateRigLibrary(r.Context(), name)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.Log.Info("a module added a library", "cat", "modules", "module", module, "library", lib.ID)
	writeJSON(w, 201, heldLibraryView{ID: lib.ID, Name: lib.Name, Type: lib.Type, Console: lib.Console})
}

// PATCH /host/v1/libraries/{id} {name}
func (s *Server) hostLibraryRename(w http.ResponseWriter, r *http.Request, module string) {
	lib := s.heldLibrary(w, r, module)
	if lib == nil {
		return
	}
	name, ok := heldName(r)
	if !ok {
		writeJSON(w, 400, map[string]string{"error": "a name of up to 100 characters"})
		return
	}
	if _, err := s.Store.UpdateLibrary(r.Context(), lib.ID, name, "games", nil); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, heldLibraryView{ID: lib.ID, Name: name, Type: lib.Type, Console: lib.Console})
}

// DELETE /host/v1/libraries/{id}
func (s *Server) hostLibraryRemove(w http.ResponseWriter, r *http.Request, module string) {
	lib := s.heldLibrary(w, r, module)
	if lib == nil {
		return
	}
	s.removeHeldArt(r, lib.ID)
	if err := s.Store.DeleteLibrary(r.Context(), lib.ID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.Log.Info("a module removed a library", "cat", "modules", "module", module, "library", lib.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// PUT /host/v1/libraries/{id}/items {path, title, tagline}: adds a game, or updates the one at
// that path. A new one is asked about by the game metadata sources, as a scan's are.
func (s *Server) hostHeldItemSave(w http.ResponseWriter, r *http.Request, module string) {
	lib := s.heldLibrary(w, r, module)
	if lib == nil {
		return
	}
	var b struct {
		Path    string `json:"path"`
		Title   string `json:"title"`
		Tagline string `json:"tagline"`
	}
	if err := readJSON(r, &b); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	b.Title = strings.TrimSpace(b.Title)
	// A held game's path names nothing on this machine: never a file a module could have read.
	if !strings.HasPrefix(b.Path, "rig://") || len(b.Path) > 200 || b.Title == "" || len(b.Title) > 300 || len(b.Tagline) > 300 {
		writeJSON(w, 400, map[string]string{"error": "a path of rig://…, a title and a short tagline"})
		return
	}
	ctx := r.Context()
	var had int64
	_ = s.DB.QueryRowContext(ctx, `SELECT id FROM items WHERE library_id = ? AND path = ?`, lib.ID, b.Path).Scan(&had)
	id, err := s.Store.SaveRigGame(ctx, lib.ID, b.Path, b.Title, b.Tagline, nil)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	fresh := had == 0
	if fresh && s.GameMeta != nil {
		logging.Go("held game details", func() {
			if err := s.Store.EnrichRigGame(context.Background(), id); err == nil {
				s.GameMeta.Kick()
			}
		}, "cat", "games")
	}
	writeJSON(w, 200, map[string]any{"id": id, "new": fresh})
}

// heldItem is a game in a held library, for the module holding it.
func (s *Server) heldItem(w http.ResponseWriter, r *http.Request, module string) *library.Item {
	if !s.servesKind(module, "games") {
		writeJSON(w, 403, map[string]string{"error": "only a module serving games libraries holds one"})
		return nil
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "no such item"})
		return nil
	}
	it, err := s.Store.Item(r.Context(), id)
	if err != nil || it.Console != library.RigConsole {
		writeJSON(w, 404, map[string]string{"error": "no such item"})
		return nil
	}
	if lib, err := s.Store.Library(r.Context(), it.LibraryID); err != nil || !lib.IsRig() {
		writeJSON(w, 404, map[string]string{"error": "no such item"})
		return nil
	}
	return it
}

// DELETE /host/v1/items/{id}
func (s *Server) hostHeldItemRemove(w http.ResponseWriter, r *http.Request, module string) {
	it := s.heldItem(w, r, module)
	if it == nil {
		return
	}
	s.removeItemArt(it.ID)
	if err := s.Store.RemoveRigGame(r.Context(), it.ID); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// heldArt is where a held game's picture from its PC is kept, in the image folder.
func heldArt(item int64) string { return fmt.Sprintf("rig-i%d-", item) }

func (s *Server) removeItemArt(item int64) {
	matches, _ := filepath.Glob(filepath.Join(s.Cfg.ImagesDir, heldArt(item)+"*"))
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

func (s *Server) removeHeldArt(r *http.Request, library int64) {
	rows, err := s.DB.QueryContext(r.Context(), `SELECT id FROM items WHERE library_id = ?`, library)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		s.removeItemArt(id)
	}
}

// PUT /host/v1/items/{id}/poster (the picture), DELETE to drop the PC's own. A picture someone
// chose by hand is never replaced.
func (s *Server) hostHeldPoster(w http.ResponseWriter, r *http.Request, module string) {
	it := s.heldItem(w, r, module)
	if it == nil {
		return
	}
	ctx := r.Context()
	if r.Method == http.MethodDelete {
		if _, err := s.Store.SetRigPoster(ctx, it.ID, ""); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if it.Poster == "" || strings.HasPrefix(it.Poster, library.RigArtPrefix) {
			s.removeItemArt(it.ID)
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxHostImage+1))
	if err != nil || len(b) == 0 || len(b) > maxHostImage {
		writeJSON(w, 400, map[string]string{"error": "a picture of up to 8 MB"})
		return
	}
	ext := pictureExt(b)
	if ext == "" {
		writeJSON(w, 400, map[string]string{"error": "not a picture this server shows"})
		return
	}
	sum := sha256.Sum256(b)
	name := heldArt(it.ID) + hex.EncodeToString(sum[:8]) + ext
	if err := os.MkdirAll(s.Cfg.ImagesDir, 0o755); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if err := os.WriteFile(filepath.Join(s.Cfg.ImagesDir, name), b, 0o644); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	changed, err := s.Store.SetRigPoster(ctx, it.ID, "cache:"+name)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if changed {
		// The PC's picture before this one isn't anyone's any more.
		if old, ok := strings.CutPrefix(it.Poster, "cache:"); ok && old != name && strings.HasPrefix(old, heldArt(it.ID)) {
			_ = os.Remove(filepath.Join(s.Cfg.ImagesDir, old))
		}
	} else if it.Poster != "cache:"+name {
		_ = os.Remove(filepath.Join(s.Cfg.ImagesDir, name)) // a picture chosen by hand stays
	}
	writeJSON(w, 200, map[string]bool{"ok": true, "set": changed})
}

// pictureExt is a picture's extension by what it holds, or "" for something else.
func pictureExt(b []byte) string {
	switch http.DetectContentType(b) {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	}
	return ""
}

// GET /host/v1/programs/{id}: how to run a program module this module is built on (module.json
// requires, or what those require), such as WittyCouch Host for a stream in the page. 404 when
// it isn't installed and turned on here.
func (s *Server) hostProgram(w http.ResponseWriter, r *http.Request, module string) {
	p, ok := s.hostMods().(heldPrograms)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "no such program here"})
		return
	}
	id := r.PathValue("id")
	built := p.Requires(module)
	for _, req := range slices.Clone(built) {
		built = append(built, p.Requires(req)...)
	}
	if !slices.Contains(built, id) {
		writeJSON(w, 403, map[string]string{"error": "a module only runs the programs of modules it is built on"})
		return
	}
	path, args, version, ok := p.ProgramCommand(id)
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "that program isn't installed here"})
		return
	}
	if args == nil {
		args = []string{}
	}
	writeJSON(w, 200, map[string]any{"path": path, "args": args, "version": version})
}
