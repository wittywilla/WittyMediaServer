package server

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/themes"
)

// GET /api/themes (public: the login page is themed too)
func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	writeJSON(w, 200, map[string]any{
		"themes":    s.Themes.List(),
		"default":   s.DB.GetSetting(ctx, "theme", "default"),
		"brandName": s.DB.GetSetting(ctx, "brandName", ""),
	})
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
	// Theme files are inert: no scripts even if an SVG carries one.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
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
