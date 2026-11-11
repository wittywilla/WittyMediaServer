// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/brandname"
	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// brand is the server's own name ("" for the default), how it is coloured,
// and the name to show split ready to draw, for /status and /themes.
func (s *Server) brand(ctx context.Context) (name, colouring string, parts []brandname.Part) {
	name = s.DB.GetSetting(ctx, "brandName", "")
	colouring = s.DB.GetSetting(ctx, "brandColouring", brandname.Auto)
	return name, colouring, brandname.Parts(firstNonEmpty(name, brandname.Default), colouring)
}

// GET /api/themes (public: the login page is themed too)
func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name, colouring, parts := s.brand(ctx)
	writeJSON(w, 200, map[string]any{
		"themes":         s.Themes.List(),
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
		writeErr(w, 400, err.Error())
		return
	}
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
		writeErr(w, 400, err.Error())
		return
	}
	meta, err := s.Themes.Add(data, r.URL.Query().Get("name"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
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
		writeErr(w, code, err.Error())
		return
	}
	if s.DB.GetSetting(r.Context(), "theme", "default") == strings.ToLower(id) {
		_ = s.DB.SetSetting(r.Context(), "theme", "default")
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
