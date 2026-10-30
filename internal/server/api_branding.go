package server

import (
	"bytes"
	"image"
	_ "image/jpeg" // decoders for checking uploaded logos
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// A custom logo replaces the app icon beside the server's name: in the
// corner, on the sign-in page and in setup. It is stored in the data folder
// as branding/logo.png or logo.jpg; the "brandLogo" setting holds its
// version, so pages ask for it only when there is one.

const maxLogoBytes = 2 << 20

var logoTypes = map[string]string{"png": "image/png", "jpeg": "image/jpeg"}

func (s *Server) logoPath(format string) string {
	ext := ".png"
	if format == "jpeg" {
		ext = ".jpg"
	}
	return filepath.Join(s.Cfg.DataDir, "branding", "logo"+ext)
}

// GET /api/branding/logo (public: the sign-in page shows it)
func (s *Server) handleLogo(w http.ResponseWriter, r *http.Request) {
	for format, ctype := range logoTypes {
		b, err := os.ReadFile(s.logoPath(format))
		if err != nil {
			continue
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "public, max-age=86400") // the address carries ?v=
		_, _ = w.Write(b)
		return
	}
	writeErr(w, 404, "no custom logo")
}

// PUT /api/branding/logo (admin): the image itself as the body.
func (s *Server) handleSetLogo(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLogoBytes))
	if err != nil {
		writeErr(w, 413, "the logo must be 2 MB or smaller")
		return
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || logoTypes[format] == "" {
		writeErr(w, 400, "use a PNG or JPEG image")
		return
	}
	if cfg.Width < 16 || cfg.Height < 16 || cfg.Width > 4096 || cfg.Height > 4096 {
		writeErr(w, 400, "the logo must be between 16 and 4096 pixels on each side")
		return
	}
	if err := os.MkdirAll(filepath.Join(s.Cfg.DataDir, "branding"), 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	dst := s.logoPath(format)
	tmp := dst + ".part"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		writeErr(w, 500, err.Error())
		return
	}
	for f := range logoTypes { // only one logo at a time
		if f != format {
			os.Remove(s.logoPath(f))
		}
	}
	v := strconv.FormatInt(time.Now().Unix(), 10)
	if err := s.DB.SetSetting(r.Context(), "brandLogo", v); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"brandLogo": v})
}

// DELETE /api/branding/logo (admin): back to the app icon.
func (s *Server) handleDeleteLogo(w http.ResponseWriter, r *http.Request) {
	for f := range logoTypes {
		os.Remove(s.logoPath(f))
	}
	if err := s.DB.SetSetting(r.Context(), "brandLogo", ""); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"brandLogo": ""})
}
