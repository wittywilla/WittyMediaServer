package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
)

func (s *Server) handleDVDOpen(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.Store.Item(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "item not found")
		return
	}
	if it.Kind != "dvd-iso" && it.Kind != "dvd-folder" {
		writeErr(w, 400, "not a DVD")
		return
	}
	var b struct {
		Mode             string `json:"mode"` // menu | title
		TTN              int    `json:"ttn"`
		Chapter          int    `json:"chapter"`
		Wide             *bool  `json:"wide"`
		MenuLanguage     string `json:"menuLanguage"`
		AudioLanguage    string `json:"audioLanguage"`
		SubtitleLanguage string `json:"subtitleLanguage"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	hw, maxHeight := s.prefs(r)
	prefs := dvdsession.Prefs{
		Wide: true, HW: hw, MaxHeight: maxHeight, AudioCodec: s.audioCodec(r),
		MenuLanguage:     firstNonEmpty(b.MenuLanguage, s.DB.GetSetting(r.Context(), "menuLanguage", "en")),
		AudioLanguage:    firstNonEmpty(b.AudioLanguage, s.DB.GetSetting(r.Context(), "audioLanguage", "")),
		SubtitleLanguage: firstNonEmpty(b.SubtitleLanguage, s.DB.GetSetting(r.Context(), "subtitleLanguage", "")),
	}
	if b.Wide != nil {
		prefs.Wide = *b.Wide
	}
	sess, err := s.DVD.Open(it.ID, it.Path, prefs)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var p *dvdsession.Presentation
	if b.Mode == "title" {
		ttn := b.TTN
		if ttn == 0 && it.Disc != nil {
			ttn = it.Disc.MainTitle
		}
		if ttn == 0 {
			ttn = 1
		}
		p = sess.PlayTitle(r.Context(), ttn, b.Chapter)
	} else {
		p = sess.Start(r.Context())
	}
	writeJSON(w, 200, map[string]any{"sessionId": sess.ID, "presentation": p})
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func (s *Server) dvdSession(w http.ResponseWriter, r *http.Request) *dvdsession.Session {
	sess := s.DVD.Get(r.PathValue("sid"))
	if sess == nil {
		writeErr(w, 404, "DVD session expired; open it again")
		return nil
	}
	return sess
}

func (s *Server) handleDVDGet(w http.ResponseWriter, r *http.Request) {
	sess := s.dvdSession(w, r)
	if sess == nil {
		return
	}
	writeJSON(w, 200, map[string]any{"sessionId": sess.ID, "presentation": sess.Current()})
}

func (s *Server) handleDVDAction(w http.ResponseWriter, r *http.Request) {
	sess := s.dvdSession(w, r)
	if sess == nil {
		return
	}
	var a dvdsession.Action
	if err := readJSON(r, &a); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p := sess.Do(r.Context(), a)
	writeJSON(w, 200, map[string]any{"sessionId": sess.ID, "presentation": p})
}

func (s *Server) handleDVDClose(w http.ResponseWriter, r *http.Request) {
	s.DVD.CloseSession(r.PathValue("sid"))
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) handleDVDOverlay(w http.ResponseWriter, r *http.Request) {
	sess := s.dvdSession(w, r)
	if sess == nil {
		return
	}
	serial, _ := strconv.Atoi(r.PathValue("serial"))
	png, ok := sess.OverlayPNG(serial)
	if !ok {
		writeErr(w, 404, "no overlay")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(png)
}

func (s *Server) handleDVDHighlight(w http.ResponseWriter, r *http.Request) {
	sess := s.dvdSession(w, r)
	if sess == nil {
		return
	}
	serial, _ := strconv.Atoi(r.PathValue("serial"))
	n, _ := strconv.Atoi(strings.TrimSuffix(r.PathValue("n"), ".png"))
	png, ok := sess.HighlightPNG(serial, n)
	if !ok {
		writeErr(w, 404, "no highlight")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(png)
}

func (s *Server) handleDVDStill(w http.ResponseWriter, r *http.Request) {
	sess := s.dvdSession(w, r)
	if sess == nil {
		return
	}
	p, ok := sess.StillPath(r.PathValue("name"))
	if !ok {
		writeErr(w, 404, "no still")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, p)
}
