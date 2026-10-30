package server

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Disc sessions: DVDs (WittyDVD) and Blu-rays (WittyBluray) present the
// same way, so the player, the remote and these endpoints serve both. A
// session id starting with "bd" belongs to the Blu-ray manager.

// discSession is what the endpoints need from either kind of session.
type discSession interface {
	Current() *dvdsession.Presentation
	Do(ctx context.Context, a dvdsession.Action) *dvdsession.Presentation
	OverlayPNG(serial int) ([]byte, bool)
	HighlightPNG(serial, n int) ([]byte, bool)
	StillPath(name string) (string, bool)
	Subtitles(ttn, n int, from, to float64) ([]dvdsession.SubEvent, error)
	SubPNG(name string) ([]byte, bool)
}

func isDVD(it *library.Item) bool      { return it.Kind == "dvd-iso" || it.Kind == "dvd-folder" }
func isBluray(it *library.Item) bool   { return it.Kind == "bluray-iso" || it.Kind == "bluray-folder" }
func isDiscKind(it *library.Item) bool { return isDVD(it) || isBluray(it) }

func (s *Server) handleDVDOpen(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "item not found")
		return
	}
	if !isDiscKind(it) {
		writeErr(w, 400, "not a disc")
		return
	}
	var b struct {
		Mode             string `json:"mode"` // menu | title
		Menu             string `json:"menu"` // with mode=menu: open this menu directly (root, chapter, audio, subpicture, angle, title)
		MenuVTS          int    `json:"menuVts"`
		Angle            int    `json:"angle"` // remembered camera angle
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
	q, maxHeight := s.quality(r)
	prefs := dvdsession.Prefs{
		Wide: true, Quality: q, MaxHeight: maxHeight, AudioCodec: s.audioCodec(r), LowPower: s.DB.GetSetting(r.Context(), "lowPower", "0") == "1",
		MenuLanguage:     firstNonEmpty(b.MenuLanguage, s.DB.GetSetting(r.Context(), "menuLanguage", "en")),
		AudioLanguage:    firstNonEmpty(b.AudioLanguage, s.DB.GetSetting(r.Context(), "audioLanguage", "")),
		SubtitleLanguage: firstNonEmpty(b.SubtitleLanguage, s.DB.GetSetting(r.Context(), "subtitleLanguage", "")),
		Angle:            b.Angle,
		Tag:              s.capTag(r),
		DVDRegion:        s.intSetting(r.Context(), "dvdRegion"),
		BDRegion:         s.intSetting(r.Context(), "blurayRegion"),
		DVDParental:      s.intSetting(r.Context(), "dvdParental"),
		BDAge:            s.intSetting(r.Context(), "blurayAge"),
		Country:          s.DB.GetSetting(r.Context(), "discCountry", ""),
	}
	if b.Wide != nil {
		prefs.Wide = *b.Wide
	}
	ttn := b.TTN
	if ttn == 0 && it.Disc != nil {
		ttn = it.Disc.MainTitle
	}
	if isBluray(it) {
		sess, err := s.BD.Open(it.ID, it.Path, prefs)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		var p *dvdsession.Presentation
		switch {
		case b.Mode == "title":
			p = sess.PlayTitle(r.Context(), ttn, b.Chapter)
		case b.Menu != "":
			p = sess.OpenMenu(r.Context(), b.Menu, b.MenuVTS)
		default:
			p = sess.Start(r.Context())
		}
		writeJSON(w, 200, map[string]any{"sessionId": sess.ID, "presentation": p, "stereo3d": sess.Stereo3D()})
		return
	}
	sess, err := s.DVD.Open(it.ID, it.Path, prefs)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var p *dvdsession.Presentation
	if b.Mode == "title" {
		if ttn == 0 {
			ttn = 1
		}
		p = sess.PlayTitle(r.Context(), ttn, b.Chapter)
	} else if b.Menu != "" {
		p = sess.OpenMenu(r.Context(), b.Menu, b.MenuVTS)
	} else {
		p = sess.Start(r.Context())
	}
	writeJSON(w, 200, map[string]any{"sessionId": sess.ID, "presentation": p})
}

func (s *Server) intSetting(ctx context.Context, key string) int {
	n, _ := strconv.Atoi(s.DB.GetSetting(ctx, key, ""))
	return n
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func (s *Server) dvdSession(w http.ResponseWriter, r *http.Request) discSession {
	sid := r.PathValue("sid")
	if strings.HasPrefix(sid, "bd") {
		if sess := s.BD.Get(sid); sess != nil {
			return sess
		}
		writeErr(w, 404, "disc session expired; open it again")
		return nil
	}
	sess := s.DVD.Get(sid)
	if sess == nil {
		writeErr(w, 404, "disc session expired; open it again")
		return nil
	}
	return sess
}

func (s *Server) handleDVDGet(w http.ResponseWriter, r *http.Request) {
	sess := s.dvdSession(w, r)
	if sess == nil {
		return
	}
	writeJSON(w, 200, map[string]any{"sessionId": r.PathValue("sid"), "presentation": sess.Current()})
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
	writeJSON(w, 200, map[string]any{"sessionId": r.PathValue("sid"), "presentation": p})
}

func (s *Server) handleDVDClose(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	if strings.HasPrefix(sid, "bd") {
		s.BD.CloseSession(sid)
	} else {
		s.DVD.CloseSession(sid)
	}
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
	var png []byte
	var ok bool
	if f, _ := strconv.Atoi(r.URL.Query().Get("f")); f > 0 {
		if fs, can := sess.(interface {
			HighlightFramePNG(serial, n, frame int) ([]byte, bool)
		}); can {
			png, ok = fs.HighlightFramePNG(serial, n, f)
		}
	} else {
		png, ok = sess.HighlightPNG(serial, n)
	}
	if !ok {
		writeErr(w, 404, "no highlight")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(png)
}

// handleDVDEffect serves one frame of a menu page's transition.
func (s *Server) handleDVDEffect(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.dvdSession(w, r).(interface {
		EffectPNG(serial, n int) ([]byte, bool)
	})
	if !ok {
		return
	}
	serial, _ := strconv.Atoi(r.PathValue("serial"))
	n, _ := strconv.Atoi(strings.TrimSuffix(r.PathValue("n"), ".png"))
	png, found := sess.EffectPNG(serial, n)
	if !found {
		writeErr(w, 404, "no effect")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(png)
}

// handleBDJPoll waits for a disc's BD-J applications to change something.
func (s *Server) handleBDJPoll(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("sid")
	sess := s.BD.Get(sid)
	if sess == nil {
		writeErr(w, 404, "disc session expired; open it again")
		return
	}
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))
	writeJSON(w, 200, sess.PollBDJ(r.Context(), since))
}

// handleBDJFrame serves the applications' graphics.
func (s *Server) handleBDJFrame(w http.ResponseWriter, r *http.Request) {
	sess := s.BD.Get(r.PathValue("sid"))
	if sess == nil {
		writeErr(w, 404, "disc session expired; open it again")
		return
	}
	png, ok := sess.BDJFrame()
	if !ok {
		writeErr(w, 404, "no frame")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(png)
}

// handleBDJStatus says whether this machine can run BD-J, for the settings page.
func (s *Server) handleBDJStatus(w http.ResponseWriter, r *http.Request) {
	path := s.DB.GetSetting(r.Context(), "javaPath", "")
	out := map[string]any{"enabled": s.DB.GetSetting(r.Context(), "bdjEnabled", "0") == "1"}
	version, where, err := bdsession.JavaStatus(r.Context(), s.Cfg.CacheDir, path)
	if err != nil {
		out["error"] = err.Error()
	} else {
		out["version"], out["path"] = version, where
	}
	writeJSON(w, 200, out)
}

// handleDVDSound serves a button sound as WAV.
func (s *Server) handleDVDSound(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.dvdSession(w, r).(interface {
		SoundWAV(id int) ([]byte, bool)
	})
	if !ok {
		return
	}
	n, _ := strconv.Atoi(strings.TrimSuffix(r.PathValue("n"), ".wav"))
	wav, found := sess.SoundWAV(n)
	if !found {
		writeErr(w, 404, "no sound")
		return
	}
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(wav)
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
