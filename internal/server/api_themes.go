// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/brandname"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// brand is the server's own name ("" for the default), how it is coloured,
// and the name to show split ready to draw, for /status and /themes.
func (s *Server) brand(ctx context.Context) (name, colouring string, parts []brandname.Part) {
	name = s.DB.GetSetting(ctx, "brandName", "")
	colouring = s.DB.GetSetting(ctx, "brandColouring", brandname.Auto)
	shown := firstNonEmpty(name, brandname.Default)
	// Spans the editor saved win over the quick modes; ones that no longer fit the name
	// (it was renamed) are ignored.
	if spans, err := brandname.ParseSpans(s.DB.GetSetting(ctx, "brandSpans", ""), len([]rune(shown))); err == nil && len(spans) > 0 {
		return name, colouring, brandname.PartsFromSpans(shown, spans)
	}
	return name, colouring, brandname.Parts(shown, colouring)
}

// brandMark is the badge the rail, the tab bar's header and the sign-in draw: what it holds
// and, for a logo, where it is. A logo that isn't on the server any more is the letter.
func (s *Server) brandMark(ctx context.Context) brandname.Mark {
	m, err := brandname.ParseMark(s.DB.GetSetting(ctx, "brandMark", ""))
	if err != nil {
		return brandname.Mark{Kind: brandname.MarkLetter}
	}
	if m.Kind == brandname.MarkImage {
		if v := s.DB.GetSetting(ctx, "brandLogo", ""); v != "" {
			m.ImageURL = "/api/v1/branding/logo?v=" + v
		} else {
			m = brandname.Mark{Kind: brandname.MarkLetter}
		}
	}
	return m
}

// GET /api/themes (public: the login page is themed too)
func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name, colouring, parts := s.brand(ctx)
	// A theme a profile imported is offered to that profile only.
	var uid int64
	if u, _ := s.signedIn(r); u != nil {
		uid = u.ID
	}
	list := []themes.Meta{}
	for _, m := range s.Themes.List() {
		if s.themeVisibleTo(m.ID, uid) {
			list = append(list, m)
		}
	}
	writeJSON(w, 200, map[string]any{
		"themes":         list,
		"default":        s.serverTheme(ctx),
		"brandName":      name,
		"brandColouring": colouring,
		"brandParts":     parts,
	})
}

// serverTheme is the theme new viewers start with. One that isn't here (saved by another build, or
// its file removed by hand) would leave the app on its built-in colours, so they get the default.
func (s *Server) serverTheme(ctx context.Context) string {
	id := s.DB.GetSetting(ctx, "theme", "default")
	if id == "auto" || s.Themes == nil || s.Themes.Has(id) {
		return id
	}
	return "default"
}

// POST /api/themes/custom: a theme from the app's theme maker.
func (s *Server) handleThemeCustom(w http.ResponseWriter, r *http.Request) {
	var c themes.Custom
	if err := readJSON(r, &c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	meta, err := s.Themes.AddCustom(c)
	if err != nil {
		s.Log.WarnContext(r.Context(), "custom theme refused", "cat", "sdui", "by", adminName(r), "err", err)
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "custom theme saved", "cat", "sdui", "by", adminName(r), "theme", meta.ID)
	writeJSON(w, 201, meta)
}

// GET /api/themes/{id}/{file...}: theme.css and the images/fonts it uses.
func (s *Server) handleThemeFile(w http.ResponseWriter, r *http.Request) {
	b, ct, err := s.Themes.File(r.PathValue("id"), r.PathValue("file"))
	if err != nil {
		writeErr(w, 404, "no such theme file")
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Theme files are inert: no scripts even if an SVG carries one, and no form in it
	// that could send what's typed elsewhere, which default-src doesn't cover.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// GET /api/themes/{id}/tokens.json: the theme's colours, lengths and fonts as
// data, for clients that can't apply CSS (public, like the themes themselves).
func (s *Server) handleThemeTokens(w http.ResponseWriter, r *http.Request) {
	t, err := s.Themes.Tokens(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "no such theme")
		return
	}
	writeJSON(w, 200, t)
}

// GET /api/themes/{id}/download: the theme ZIP, as a template for your own.
func (s *Server) handleThemeDownload(w http.ResponseWriter, r *http.Request) {
	p, err := s.Themes.ZipPath(r.PathValue("id"))
	if err != nil {
		writeErr(w, 404, "no such theme")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ToLower(r.PathValue("id"))+`.zip"`)
	http.ServeFile(w, r, p)
}

// POST /api/themes: upload a theme ZIP (request body = the ZIP).
func (s *Server) handleThemeUpload(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 26<<20))
	if err != nil {
		s.Log.WarnContext(r.Context(), "theme upload could not be read", "cat", "sdui", "by", adminName(r), "err", err)
		writeErr(w, 400, err.Error())
		return
	}
	meta, err := s.Themes.Add(data, r.URL.Query().Get("name"))
	if err != nil {
		s.Log.WarnContext(r.Context(), "theme upload refused", "cat", "sdui", "by", adminName(r), "bytes", len(data), "err", err)
		writeErr(w, 400, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "theme uploaded", "cat", "sdui", "by", adminName(r), "theme", meta.ID, "version", meta.Version)
	writeJSON(w, 201, meta)
}

// DELETE /api/themes/{id}
func (s *Server) handleThemeDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Themes.Remove(id); err != nil {
		code := 400
		if errors.Is(err, themes.ErrNotFound) {
			code = 404
		}
		s.Log.InfoContext(r.Context(), "theme not deleted", "cat", "sdui", "by", adminName(r), "theme", id, "status", code, "err", err)
		writeErr(w, code, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "theme deleted", "cat", "sdui", "by", adminName(r), "theme", id)
	if s.DB.GetSetting(r.Context(), "theme", "default") == strings.ToLower(id) {
		if err := s.DB.SetSetting(r.Context(), "theme", "default"); err != nil {
			s.Log.WarnContext(r.Context(), "could not put the server theme back to the default", "cat", "sdui", "err", err)
		} else {
			s.Log.InfoContext(r.Context(), "server theme back to the default: its theme was deleted", "cat", "sdui", "theme", id)
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// A profile's own imported themes (the experience policy's profileThemes): installed like any
// theme, but offered only to the profile that imported them. Who owns a theme is a small file
// beside it, <id>.owner, holding the account's ID.

// maxProfileThemes bounds the themes one profile may import.
const maxProfileThemes = 5

func (s *Server) themeOwnerPath(id string) string {
	return filepath.Join(s.Themes.Dir, id+".owner")
}

// themeOwner is the account that imported a theme, 0 for a theme the server owns.
func (s *Server) themeOwner(id string) int64 {
	b, err := os.ReadFile(s.themeOwnerPath(id))
	if err != nil {
		return 0
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return n
}

// themeVisibleTo says whether an account may be offered a theme.
func (s *Server) themeVisibleTo(id string, uid int64) bool {
	o := s.themeOwner(id)
	return o == 0 || o == uid
}

// POST /api/themes/mine: a theme ZIP imported by the signed-in profile, when the
// administrator lets profiles import their own (administrators always may; theirs is the
// server's, as POST /api/themes makes it).
func (s *Server) handleThemeImportMine(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	st, err := s.uiStore()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !u.IsAdmin && !(st.Current().Experiences != nil && st.Current().Experiences.ProfileThemes) {
		writeErr(w, 403, "This server doesn't let profiles import themes. An administrator can allow it.")
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 26<<20))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	id, err := themes.IDOf(data, r.URL.Query().Get("name"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !u.IsAdmin {
		if o := s.themeOwner(id); (o != 0 && o != u.ID) || (o == 0 && s.Themes.Has(id)) {
			writeErr(w, 409, "A theme with that id is already here. Change the id in theme.json.")
			return
		}
		count := 0
		if files, _ := filepath.Glob(filepath.Join(s.Themes.Dir, "*.owner")); len(files) > 0 {
			for _, f := range files {
				if s.themeOwner(strings.TrimSuffix(filepath.Base(f), ".owner")) == u.ID {
					count++
				}
			}
		}
		if count >= maxProfileThemes && s.themeOwner(id) != u.ID {
			writeErr(w, 409, "You have imported "+strconv.Itoa(maxProfileThemes)+" themes already. Remove one first.")
			return
		}
	}
	meta, err := s.Themes.Add(data, r.URL.Query().Get("name"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !u.IsAdmin {
		_ = os.WriteFile(s.themeOwnerPath(meta.ID), []byte(strconv.FormatInt(u.ID, 10)), 0o644)
	}
	s.Log.InfoContext(r.Context(), "theme imported", "cat", "sdui", "by", adminName(r), "theme", meta.ID, "own", !u.IsAdmin)
	writeJSON(w, 201, meta)
}
