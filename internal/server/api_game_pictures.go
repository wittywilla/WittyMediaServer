// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
	"github.com/wittywilla/wittymediaserver/internal/wittyplayfile"
)

// How WittyPlay pictures games (WittyPlay Settings › Game pictures): box art fitted or filled, or
// drawn in a cartridge or case. A person's own choices live in their /me/ui (wittyplayPictures); the
// server's are here: its default and per-console looks, and per-game looks an administrator chose or
// a .wittyplay file beside the game gives. The file format is docs/specs/wittyplay-file.md.
//
// Brand logos: WittyPlay can put real console logos on its cartridges and cases and use them as the
// console's mark. Nothing is bundled; an administrator turns it on and confirms a warning first, and
// the logos then come through the server's logo service (downloaded once, kept in the image cache,
// served by this server, so a browser never contacts the source), with their source and licence kept.

func (s *Server) gamePictureRoutes(m routes) {
	a := s.requireUser
	m.Handle("GET /api/games/pictures", a(s.handleGamePictures))
	m.Handle("PUT /api/games/pictures", s.requireAdmin(s.handleGamePicturesPut))
	m.Handle("GET /api/games/pictures/file", s.requireAdmin(s.handleGamePicturesFile))
	m.Handle("POST /api/games/pictures/check", a(s.handleGamePicturesCheck))
	m.Handle("PUT /api/games/{id}/picture", s.requireAdmin(s.handleGamePicturePut))
	m.Handle("GET /api/games/brand-logos", a(s.handleBrandLogos))
	m.Handle("PUT /api/games/brand-logos", s.requireAdmin(s.handleBrandLogosPut))
	m.Handle("DELETE /api/games/brand-logos", s.requireAdmin(s.handleBrandLogosDelete))
}

// readWittyplay reads a .wittyplay file sent as a request's body, refusing anything over the limit.
func readWittyplay(r *http.Request) (*wittyplayfile.Doc, error) {
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, wittyplayfile.MaxSize+1))
	if err != nil {
		return nil, err
	}
	return wittyplayfile.Parse(b)
}

func wittyplayErr(w http.ResponseWriter, err error) {
	code := http.StatusBadRequest
	if errors.Is(err, wittyplayfile.ErrTooBig) {
		code = http.StatusRequestEntityTooLarge
	}
	writeErr(w, code, err.Error())
}

// GET /api/games/pictures: the server's own document (default and consoles), and the games it has a
// look for that this account may see, by id. The case styles this server draws come too.
func (s *Server) handleGamePictures(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := s.Store.GamePictures(ctx)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	games := map[string]wittyplayfile.Look{}
	limited := s.accessOf(ctx, userFrom(r)) != nil
	for id, l := range all {
		if limited {
			if _, ok := s.itemFor(ctx, userFrom(r), id); !ok {
				continue
			}
		}
		games[strconv.FormatInt(id, 10)] = l
	}
	writeJSON(w, 200, map[string]any{"server": s.Store.ServerPictures(ctx), "games": games, "caseStyles": wittyplayfile.CaseStyles})
}

// PUT /api/games/pictures: a .wittyplay document for the whole server. Its default and consoles become
// the server's; each of its games is applied to the games here it matches, as an administrator's choice.
func (s *Server) handleGamePicturesPut(w http.ResponseWriter, r *http.Request) {
	d, err := readWittyplay(r)
	if err != nil {
		wittyplayErr(w, err)
		return
	}
	ctx := r.Context()
	if err := s.Store.SetServerPictures(ctx, d); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	applied, unmatched := 0, 0
	for _, g := range d.Games {
		ids, err := s.Store.GameMatch(ctx, g.Match, 50)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		if len(ids) == 0 {
			unmatched++
		}
		for _, id := range ids {
			was, _ := s.Store.GamePictureFrom(ctx, id, library.PictureByAdmin)
			if err := s.Store.SetGamePicture(ctx, id, library.PictureByAdmin, g.Look.Over(was)); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			applied++
		}
	}
	s.Log.InfoContext(ctx, "game pictures saved", "cat", "games", "by", userFrom(r).Name, "games", applied, "unmatched", unmatched)
	writeJSON(w, 200, map[string]any{"server": s.Store.ServerPictures(ctx), "applied": applied, "unmatched": unmatched})
}

// GET /api/games/pictures/file: the server's choices as a .wittyplay file to download: its default,
// its consoles, and every game an administrator chose for, matched by file name, title and console
// (and this server's id) so another server can find them.
func (s *Server) handleGamePicturesFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d := s.Store.ServerPictures(ctx)
	d.Generator = "WittyMediaServer " + s.Version
	rows, err := s.DB.QueryContext(ctx, `SELECT g.item_id, g.look, i.path, COALESCE(i.title, ''), COALESCE(i.console, '') FROM game_pictures g JOIN items i ON i.id = g.item_id
		WHERE g.source = ? ORDER BY i.sort_title LIMIT ?`, library.PictureByAdmin, wittyplayfile.MaxGames)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var raw, path, title, console string
		if err := rows.Scan(&id, &raw, &path, &title, &console); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		g := wittyplayfile.Game{Match: wittyplayfile.Match{Item: id, File: baseName(path), Title: title, Console: console}}
		if err := g.Look.UnmarshalJSON([]byte(raw)); err != nil {
			continue
		}
		if len(g.Match.File) > wittyplayfile.MaxName {
			g.Match.File = ""
		}
		if len(g.Match.Title) > wittyplayfile.MaxName {
			g.Match.Title = ""
		}
		d.Games = append(d.Games, g)
	}
	if err := rows.Err(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	b, err := d.Encode()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", wittyplayfile.MediaType)
	w.Header().Set("Content-Disposition", `attachment; filename="server-game-pictures`+wittyplayfile.Ext+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

// baseName is a path's last part, whichever separator it was written with.
func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// POST /api/games/pictures/check: reads a .wittyplay file for someone about to import it into their
// own choices. It says whether the file is valid and, for each of its games, which of this account's
// games it matches.
func (s *Server) handleGamePicturesCheck(w http.ResponseWriter, r *http.Request) {
	d, err := readWittyplay(r)
	if err != nil {
		wittyplayErr(w, err)
		return
	}
	ctx := r.Context()
	u := userFrom(r)
	limited := s.accessOf(ctx, u) != nil
	matches := make([][]int64, len(d.Games))
	for i, g := range d.Games {
		ids, err := s.Store.GameMatch(ctx, g.Match, 50)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		matches[i] = []int64{}
		for _, id := range ids {
			if limited {
				if _, ok := s.itemFor(ctx, u, id); !ok {
					continue
				}
			}
			matches[i] = append(matches[i], id)
		}
	}
	writeJSON(w, 200, map[string]any{"doc": d, "matches": matches})
}

// PUT /api/games/{id}/picture {picture, case, label}: an administrator's look for one game, for
// everyone on this server; an empty one takes it away.
func (s *Server) handleGamePicturePut(w http.ResponseWriter, r *http.Request) {
	it, ok := s.gameItem(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	var l wittyplayfile.Look
	if err := readJSON(r, &l); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	probe := wittyplayfile.Doc{Version: wittyplayfile.Version, Default: &l}
	if err := probe.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	if err := s.Store.SetGamePicture(ctx, it.ID, library.PictureByAdmin, l); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	got, err := s.Store.GamePictureFrom(ctx, it.ID, library.PictureByAdmin)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.DebugContext(ctx, "game picture saved", "cat", "games", "item", it.ID, "by", userFrom(r).Name)
	writeJSON(w, 200, got)
}

// ---- brand logos ----

// brandLogoNames are the names the logo service looks console logos up by: every console this
// server knows, and the ones it has traits for.
func brandLogoConsoles() map[string]string {
	out := map[string]string{}
	for _, c := range wittyplay.Consoles() {
		if c.Hidden {
			continue
		}
		if n := library.ConsoleLogoName(c.ID); n != "" {
			out[c.ID] = n
		} else {
			out[c.ID] = c.Name
		}
	}
	for id := range consoleTraitsTable {
		if _, ok := out[id]; !ok {
			if n := library.ConsoleLogoName(id); n != "" {
				out[id] = n
			}
		}
	}
	return out
}

type brandLogo struct {
	ID         int64  `json:"id"`
	Version    int    `json:"version"`
	Opaque     bool   `json:"opaque"`
	Name       string `json:"name"`
	Source     string `json:"source,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
	License    string `json:"license,omitempty"`
	LicenseURL string `json:"licenseUrl,omitempty"`
	Credit     string `json:"credit,omitempty"`
}

// GET /api/games/brand-logos: whether WittyPlay shows console logos, and, when it does, each console's
// logo this server has (by console id) with where it came from. Off, the list is empty: WittyPlay draws
// its own generic shapes and the console's name.
func (s *Server) handleBrandLogos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	on := s.Store.ConsoleLogosOn(ctx)
	logos := map[string]brandLogo{}
	pending := 0
	if on {
		for id, name := range brandLogoConsoles() {
			g, err := s.Store.LogoFor(ctx, library.LogoConsole, name)
			if err != nil {
				writeErr(w, 500, err.Error())
				return
			}
			if g == nil {
				continue
			}
			if g.Status == library.LogoPending {
				pending++
			}
			if !g.Has {
				continue
			}
			logos[id] = brandLogo{ID: g.ID, Version: g.Version, Opaque: g.Opaque, Name: g.Name, Source: g.Source, SourceURL: g.SourceURL, License: g.License, LicenseURL: g.LicenseURL, Credit: g.Credit}
		}
	}
	out := map[string]any{"enabled": on, "logos": logos, "warning": brandLogoWarning}
	if userFrom(r).IsAdmin {
		out["appWide"] = s.Store.LogoConsent(ctx)
		out["pending"] = pending
		out["agreedAt"] = s.DB.GetSetting(ctx, library.ConsoleLogosSetting+"At", "")
	}
	writeJSON(w, 200, out)
}

// brandLogoWarning is what an administrator confirms before any logo is downloaded. The settings
// page shows these words; the server keeps its own copy so every client shows the same.
var brandLogoWarning = []string{
	"Console and game studio logos are trademarks of their owners.",
	"They are downloaded from Wikimedia Commons for personal, non-commercial display on this server only: console logos by themselves, a game studio's only when you choose one.",
	"This server and WittyPlay are not endorsed by, sponsored by or affiliated with any of the owners.",
	"You are responsible for how they are used here.",
	"Each logo's source and licence is recorded and shown in the credits. You can remove every downloaded logo at any time.",
}

// PUT /api/games/brand-logos {enabled, accepted}: turning them on needs accepted, the administrator's
// confirmation of the warning. Turning them off stops downloads (unless the app-wide logo agreement
// stands) and stops showing them; what was downloaded stays until removed.
func (s *Server) handleBrandLogosPut(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Enabled  bool `json:"enabled"`
		Accepted bool `json:"accepted"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if b.Enabled && !b.Accepted {
		writeErr(w, 400, "Confirm the warning about trademarks first.")
		return
	}
	ctx := r.Context()
	var names []string
	if b.Enabled {
		for _, n := range brandLogoConsoles() {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	if err := s.logos().SetConsoleLogos(ctx, b.Enabled, names); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.InfoContext(ctx, "WittyPlay console logos", "cat", "games", "on", b.Enabled, "by", userFrom(r).Name)
	s.handleBrandLogos(w, r)
}

// DELETE /api/games/brand-logos: removes every downloaded console and game studio logo, files and all,
// and turns them off. A logo an administrator uploaded is theirs, and stays.
func (s *Server) handleBrandLogosDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.logos().SetConsoleLogos(ctx, false, nil); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	n := 0
	for _, kind := range library.BrandLogoKinds {
		k, err := s.logos().ForgetDownloadedKind(ctx, kind)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		n += k
	}
	s.Log.InfoContext(ctx, "WittyPlay console logos removed", "cat", "games", "removed", n, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]any{"removed": n, "enabled": false})
}
