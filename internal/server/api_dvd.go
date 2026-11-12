// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
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
		Mode    string `json:"mode"` // menu | title
		Menu    string `json:"menu"` // with mode=menu: open this menu directly (root, chapter, audio, subpicture, angle, title)
		MenuVTS int    `json:"menuVts"`
		Angle   int    `json:"angle"` // remembered camera angle
		TTN     int    `json:"ttn"`
		Chapter int    `json:"chapter"`
		// Episode, with mode=title: play this episode of a TV disc, by its place in
		// viewing order (1-based), as the disc's analysis lists them.
		Episode int `json:"episode"`
		// Exact: ttn is meant as it stands, even 0, which is a real playlist on a Blu-ray.
		// Without it, 0 means the disc's film.
		Exact            bool   `json:"exact"`
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
		Owner: userFrom(r).ID,
		Wide:  true, Quality: q, MaxHeight: maxHeight, AudioCodec: s.audioCodec(r), LowPower: s.DB.GetSetting(r.Context(), "lowPower", "0") == "1",
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
	// With no title named, Play plays the disc's film: the title an admin chose, or else
	// the engine's ranked pick. A Blu-ray's playlists start at 0, so there "none named" is
	// only told from "playlist 0" by what the disc's film is.
	ttn := b.TTN
	if ttn == 0 && it.Disc != nil && !b.Exact {
		ttn = it.Disc.MainTitle
	}
	var episode *library.DiscEpisode
	if b.Mode == "title" && b.Episode > 0 {
		if episode = discEpisode(it.Disc, b.Episode); episode == nil {
			writeErr(w, 400, "This disc has no such episode.")
			return
		}
		ttn = episode.TTN
	}
	if it.Disc != nil && it.Disc.Rank != nil && !isBluray(it) {
		// The film as the engine lays it out, for when a cell of it plays: by Play
		// here, or by the disc's own menu.
		if f := it.Disc.FeaturePlays(it.Disc.Rank.Main); f != nil {
			prefs.Feature = f.Engine()
		}
	}
	if isBluray(it) {
		sess, err := s.BD.Open(it.ID, it.Path, prefs)
		if writeModuleMissing(w, err) {
			return
		}
		if err != nil {
			writeErr(w, 500, s.viewerError(r, err))
			return
		}
		var p *dvdsession.Presentation
		switch {
		case episode != nil && episode.Part:
			p = sess.PlayItem(r.Context(), ttn, episode.Item)
		case b.Mode == "title":
			p = sess.PlayTitle(r.Context(), ttn, b.Chapter)
		case b.Menu != "":
			p = sess.OpenMenu(r.Context(), b.Menu, b.MenuVTS)
		default:
			p = sess.Start(r.Context())
		}
		s.logDiscOpened(r, it, sess.ID, "bluray", b.Mode, p)
		writeJSON(w, 200, map[string]any{"sessionId": sess.ID, "presentation": p, "stereo3d": sess.Stereo3D()})
		return
	}
	sess, err := s.DVD.Open(it.ID, it.Path, prefs)
	if writeModuleMissing(w, err) {
		return
	}
	if errors.Is(err, dvdsession.ErrScrambled) {
		writeErr(w, 403, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	var p *dvdsession.Presentation
	if b.Mode == "title" {
		if ttn == 0 {
			ttn = 1
		}
		if episode != nil && episode.Part {
			p = sess.PlayFrom(r.Context(), ttn, episode.Chapter, episode.StartSec)
		} else {
			p = sess.PlayTitle(r.Context(), ttn, b.Chapter)
		}
	} else if b.Menu != "" {
		p = sess.OpenMenu(r.Context(), b.Menu, b.MenuVTS)
	} else {
		p = sess.Start(r.Context())
	}
	s.logDiscOpened(r, it, sess.ID, "dvd", b.Mode, p)
	writeJSON(w, 200, map[string]any{"sessionId": sess.ID, "presentation": p})
}

// logDiscOpened ties the request that opened a disc to its session. A disc's
// path to the screen is the same whatever is on it, so its decision is "disc".
func (s *Server) logDiscOpened(r *http.Request, it *library.Item, session, kind, mode string, p *dvdsession.Presentation) {
	shows := ""
	if p != nil {
		shows = p.Kind
	}
	s.Log.InfoContext(r.Context(), "disc session opened", "cat", "playback", "session", session, "item", it.ID, "title", it.Title, "mode", "disc", "format", kind, "open", mode, "shows", shows)
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
		if sess := s.bdSession(r, sid); sess != nil {
			return sess
		}
		writeErr(w, 404, "disc session expired; open it again")
		return nil
	}
	sess := s.DVD.Get(sid)
	if sess == nil || !s.discAllowed(r, sess.ItemID) {
		writeErr(w, 404, "disc session expired; open it again")
		return nil
	}
	return sess
}

// discAllowed: as for a stream, someone with limits holding a session's name is checked
// against what it plays, every time.
func (s *Server) discAllowed(r *http.Request, itemID int64) bool {
	a := s.access(r)
	return a == nil || s.itemAllowed(r.Context(), a, itemID)
}

func (s *Server) bdSession(r *http.Request, sid string) *bdsession.Session {
	sess := s.BD.Get(sid)
	if sess == nil || !s.discAllowed(r, sess.ItemID) {
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
		if s.bdSession(r, sid) != nil {
			s.BD.CloseSession(sid)
		}
	} else if sess := s.DVD.Get(sid); sess != nil && s.discAllowed(r, sess.ItemID) {
		s.DVD.CloseSession(sid)
	}
	s.Log.InfoContext(r.Context(), "disc session closed", "cat", "playback", "session", sid)
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
	sess := s.bdSession(r, sid)
	if sess == nil {
		writeErr(w, 404, "disc session expired; open it again")
		return
	}
	since, _ := strconv.Atoi(r.URL.Query().Get("since"))
	writeJSON(w, 200, sess.PollBDJ(r.Context(), since))
}

// handleBDJFrame serves the applications' graphics.
func (s *Server) handleBDJFrame(w http.ResponseWriter, r *http.Request) {
	sess := s.bdSession(r, r.PathValue("sid"))
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

// handleBDJStatus says whether this machine can run BD-J, for the settings page. Trying Java means
// running it, so that waits until BD-J is on. Which Java comes from the config file, never from a
// setting: a web account can't choose what the server runs.
func (s *Server) handleBDJStatus(w http.ResponseWriter, r *http.Request) {
	on := s.DB.GetSetting(r.Context(), "bdjEnabled", "0") == "1"
	out := map[string]any{"enabled": on, "configured": s.Cfg.JavaPath != ""}
	if !on {
		if where, err := bdsession.FindJava(s.Cfg.JavaPath); err == nil {
			out["path"] = where
		} else {
			out["error"] = err.Error()
		}
		writeJSON(w, 200, out)
		return
	}
	version, where, err := bdsession.JavaStatus(r.Context(), s.Cfg.CacheDir, s.Cfg.JavaPath)
	switch {
	case errors.Is(err, bdsession.ErrJavaCantRun):
		// What a program printed stays in the log.
		out["error"], out["path"] = "This Java can't run Blu-ray menus. The log says why.", where
		s.Log.WarnContext(r.Context(), "BD-J is on, but this Java can't run it", "cat", "discs", "err", err)
	case err != nil:
		out["error"] = err.Error()
		s.Log.WarnContext(r.Context(), "BD-J is on, but no Java was found", "cat", "discs", "err", err)
	default:
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
