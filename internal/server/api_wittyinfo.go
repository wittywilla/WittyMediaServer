// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/lyrics"
	"github.com/wittywilla/wittymediaserver/internal/playback"
	"github.com/wittywilla/wittymediaserver/pkg/langtag"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

// .wittyinfo files (docs/formats/wittyinfo.md) through the API: read, edit, import, export,
// pictures. A target is an item (/api/wittyinfo/item/{id}), a show (/api/wittyinfo/show/{id})
// or a season of one (/api/wittyinfo/show/{id}?season=N).
//
// An edit is written beside the media when the administrator allows it (saveMetadata and
// saveWittyinfo both on) and kept by the server otherwise, ready to Export.

func (s *Server) wittyinfoRoutes(m *http.ServeMux) {
	ad := s.requireAdmin
	m.Handle("GET /api/wittyinfo/{target}/{id}", ad(s.handleInfoGet))
	m.Handle("PUT /api/wittyinfo/{target}/{id}", ad(s.handleInfoPut))
	m.Handle("DELETE /api/wittyinfo/{target}/{id}", ad(s.handleInfoDelete))
	m.Handle("GET /api/wittyinfo/{target}/{id}/export", ad(s.handleInfoExport))
	m.Handle("POST /api/wittyinfo/{target}/{id}/import", ad(s.handleInfoImport))
	m.Handle("POST /api/wittyinfo/{target}/{id}/use-anyway", ad(s.handleInfoForce))
	m.Handle("PUT /api/wittyinfo/{target}/{id}/images/{role}", ad(s.handleInfoImagePut))
	m.Handle("DELETE /api/wittyinfo/{target}/{id}/images/{role}", ad(s.handleInfoImageDelete))
	m.Handle("PUT /api/wittyinfo/{target}/{id}/subtitles/{name}", ad(s.handleInfoSubtitlePut))
	m.Handle("DELETE /api/wittyinfo/{target}/{id}/subtitles/{name}", ad(s.handleInfoSubtitleDelete))
	m.Handle("GET /api/wittyinfo/{target}/{id}/files/{name...}", s.requireUser(s.handleInfoFile))
}

// infoUploadPath: an import or a picture is bigger than the usual body limit.
func infoUploadPath(p string) bool {
	if rest, ok := strings.CutPrefix(p, apiPrefix); ok {
		p = "/api/" + rest
	}
	rest, ok := strings.CutPrefix(p, "/api/wittyinfo/")
	return ok && (strings.HasSuffix(rest, "/import") || strings.Contains(rest, "/images/") || strings.Contains(rest, "/subtitles/"))
}

// saveInfoBeside: may an edit be written beside the media?
func (s *Server) saveInfoBeside(r *http.Request) bool {
	return s.saveMeta(r) && s.DB.GetSetting(r.Context(), "saveWittyinfo", "1") == "1"
}

// infoTarget reads the target of a request, if the account may see it.
func (s *Server) infoTarget(w http.ResponseWriter, r *http.Request) (library.InfoTarget, *library.Item, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return library.InfoTarget{}, nil, false
	}
	switch r.PathValue("target") {
	case "item":
		it, err := s.visibleItem(r, id)
		if err != nil {
			writeErr(w, 404, "not found")
			return library.InfoTarget{}, nil, false
		}
		if it.Kind != "video" && !library.IsDiscKind(it.Kind) && !library.IsSong(it) {
			writeErr(w, 400, "a .wittyinfo is for films and episodes, and a .wittyaudio for songs")
			return library.InfoTarget{}, nil, false
		}
		return library.InfoTarget{Kind: library.InfoItem, ID: id}, it, true
	case "album":
		// /api/wittyinfo/album/{a song's id}: that song's album.wittyaudio.
		it, err := s.visibleItem(r, id)
		if err != nil || !library.IsSong(it) {
			writeErr(w, 404, "not found")
			return library.InfoTarget{}, nil, false
		}
		return library.InfoTarget{Kind: library.InfoAlbum, ID: s.Store.AlbumKey(r.Context(), it)}, nil, true
	case "show":
		if _, err := s.visibleShow(r, id); err != nil {
			writeErr(w, 404, "not found")
			return library.InfoTarget{}, nil, false
		}
		if q := r.URL.Query().Get("season"); q != "" {
			n, err := strconv.Atoi(q)
			if err != nil || n < 0 || n > 9999 {
				writeErr(w, 400, "bad season")
				return library.InfoTarget{}, nil, false
			}
			return library.InfoTarget{Kind: library.InfoSeason, ID: id, Season: n}, nil, true
		}
		return library.InfoTarget{Kind: library.InfoShow, ID: id}, nil, true
	}
	writeErr(w, 404, "not found")
	return library.InfoTarget{}, nil, false
}

// infoFile is a member of the archive, as the editor lists it.
type infoFile struct {
	Name string `json:"name"`
	Size int    `json:"size"`
	URL  string `json:"url,omitempty"`
}

// infoAnswer is what GET /api/wittyinfo/... answers.
type infoAnswer struct {
	Kind   string `json:"kind"`
	ID     int64  `json:"id"`
	Season *int   `json:"season,omitempty"`
	// Info: the manifest in use, null when there is none.
	Info *wittyinfo.Info `json:"info"`
	// Origin: "stored" (kept by the server), "file" (beside the media) or "".
	Origin    string              `json:"origin"`
	Path      string              `json:"path,omitempty"`
	FileFound bool                `json:"fileFound"`
	FileError string              `json:"fileError,omitempty"`
	Problems  []wittyinfo.Problem `json:"problems,omitempty"`
	Match     *wittyinfo.Match    `json:"match,omitempty"`
	Force     bool                `json:"force"`
	Files     []infoFile          `json:"files"`
	// Version: the newest version of the format this server writes. Newer: the file in use
	// follows a later one; what this server doesn't know of it is kept as it is.
	Version int  `json:"version"`
	Newer   bool `json:"newer,omitempty"`
	// CanWrite: an edit is written beside the media; otherwise the server keeps it.
	CanWrite bool `json:"canWrite"`
	// Identity: which video a new file is for (items only).
	Identity *wittyinfo.Identity `json:"identity,omitempty"`
	// Markers: what is in force for the item now, each part with where it came from, so the
	// editor can show what a .wittyinfo overrides.
	Markers any `json:"markers,omitempty"`
	// Defaults: the season's and the show's files, for an episode.
	Defaults map[string]infoSummary `json:"defaults,omitempty"`
	// Chapters: the file's own chapters, to copy into scenes.
	Chapters []chapter `json:"chapters,omitempty"`
	Duration float64   `json:"durationSec,omitempty"`
}

// infoSummary says briefly what a season's or a show's file holds.
type infoSummary struct {
	Origin   string `json:"origin"`
	Segments int    `json:"segments"`
	Error    string `json:"error,omitempty"`
}

func summaryOf(l library.LoadedInfo) infoSummary {
	out := infoSummary{Origin: l.Origin}
	if l.Info != nil {
		out.Segments = len(l.Info.Segments)
	}
	if l.FileErr != nil {
		out.Error = l.FileErr.Error()
	}
	return out
}

func (s *Server) infoAnswerFor(r *http.Request, t library.InfoTarget, it *library.Item) infoAnswer {
	ctx := r.Context()
	l := s.Store.LoadInfo(ctx, t)
	out := infoAnswer{Kind: string(t.Kind), ID: t.ID, Info: l.Info, Origin: l.Origin, Path: l.Path, FileFound: l.FileFound, Force: l.Force, Files: []infoFile{}}
	if t.Kind == library.InfoSeason {
		n := t.Season
		out.Season = &n
		out.Kind = "show"
	}
	out.CanWrite = s.saveInfoBeside(r) && l.Path != ""
	out.Version = wittyinfo.Version
	out.Newer = l.Info != nil && l.Info.Newer()
	if l.FileErr != nil {
		out.FileError = l.FileErr.Error()
		var e *wittyinfo.Error
		if errors.As(l.FileErr, &e) {
			out.Problems = e.Problems
		}
	}
	if l.Info != nil {
		base := apiPrefix + "wittyinfo/" + r.PathValue("target") + "/" + strconv.FormatInt(t.ID, 10) + "/files/"
		for _, name := range sortedNames(l.Info.Files) {
			f := infoFile{Name: name, Size: len(l.Info.Files[name])}
			if strings.HasPrefix(name, "images/") {
				f.URL = base + name + seasonQuery(t)
			}
			out.Files = append(out.Files, f)
		}
	}
	if it != nil {
		m, infos := s.mediaMarkersAndInfo(ctx, it)
		out.Markers = m
		if l.Info != nil {
			mt := infos.Match
			out.Match = &mt
		}
		if l.Info == nil || l.Info.Identity == nil {
			out.Identity = s.Store.IdentityOf(ctx, it)
		}
		out.Duration = it.RuntimeSec
		if it.ShowID != nil {
			out.Defaults = map[string]infoSummary{"show": summaryOf(infos.Show)}
			if it.Season != nil {
				out.Defaults["season"] = summaryOf(infos.Season)
			}
		}
		out.Chapters = s.fileChapters(r, it)
	} else {
		// A show's or a season's defaults: the longest of its episodes, so every time fits.
		q, args := `SELECT IFNULL(MAX(runtime_sec), 0) FROM items WHERE show_id = ?`, []any{t.ID}
		if t.Kind == library.InfoSeason {
			q, args = q+` AND season = ?`, append(args, t.Season)
		}
		_ = s.DB.QueryRowContext(ctx, q, args...).Scan(&out.Duration)
	}
	return out
}

func seasonQuery(t library.InfoTarget) string {
	if t.Kind == library.InfoSeason {
		return "?season=" + strconv.Itoa(t.Season)
	}
	return ""
}

func sortedNames(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// fileChapters are the chapters in the video file itself.
func (s *Server) fileChapters(r *http.Request, it *library.Item) []chapter {
	if it.Kind != "video" || s.Tools == nil || !s.Store.LibraryFile(r.Context(), it.Path) {
		return nil
	}
	pr, err := s.Tools.ProbeFile(r.Context(), it.Path)
	if err != nil || pr == nil {
		return nil
	}
	var out []chapter
	for _, c := range pr.Chapters {
		if at, err := strconv.ParseFloat(c.Start, 64); err == nil {
			out = append(out, chapter{Start: at, Title: c.Tags["title"]})
		}
	}
	return out
}

// GET /api/wittyinfo/{target}/{id}
func (s *Server) handleInfoGet(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

// infoEdit is the body of PUT /api/wittyinfo/{target}/{id}: the manifest. The pictures and
// other members already in the file are kept; they change through /images.
type infoEdit struct {
	Info json.RawMessage `json:"info"`
}

// saveInfo checks, stamps and keeps a file for a target: beside the media when allowed,
// else in the server. A broken file that was beside the media is kept as .bak first.
func (s *Server) saveInfo(w http.ResponseWriter, r *http.Request, t library.InfoTarget, it *library.Item, x *wittyinfo.Info) bool {
	ctx := r.Context()
	x.Scope = t.Scope()
	if it != nil && library.IsSong(it) {
		x.Format, x.Scope = wittyinfo.FormatAudio, wittyinfo.ScopeSong
	} else if t.Kind == library.InfoAlbum {
		x.Format = wittyinfo.FormatAudio
	} else {
		x.Format = wittyinfo.Format
	}
	if t.Kind == library.InfoSeason {
		n := t.Season
		x.Season = &n
	} else {
		x.Season = nil
	}
	u := userFrom(r)
	author := u.DisplayName
	if author == "" {
		author = u.Name
	}
	x.Stamp(author, "WittyMediaServer", s.Version, time.Now())
	body, err := x.Encode()
	if err != nil {
		writeInfoErr(w, err)
		return false
	}
	l := s.Store.LoadInfo(ctx, t)
	if s.saveInfoBeside(r) && l.Path != "" {
		if l.FileFound && l.FileErr != nil {
			_ = s.Store.RetireInfoFile(ctx, l.Path)
		}
		if err := s.Store.WriteInfoFile(ctx, l.Path, body); err != nil {
			// The disk said no (read-only, full): keep it here instead, so nothing is lost.
			s.Log.WarnContext(ctx, "could not write a .wittyinfo beside the media; kept by the server instead", "cat", "library", "file", l.Path, "err", err)
			if err := s.Store.SetStoredInfo(ctx, t, body); err != nil {
				writeErr(w, 500, err.Error())
				return false
			}
		} else if err := s.Store.SetStoredInfo(ctx, t, nil); err != nil {
			writeErr(w, 500, err.Error())
			return false
		}
	} else if err := s.Store.SetStoredInfo(ctx, t, body); err != nil {
		writeErr(w, 500, err.Error())
		return false
	}
	if it != nil && s.Scanner != nil {
		if err := s.Scanner.ApplyInfoNow(ctx, it.ID); err != nil {
			s.Log.WarnContext(ctx, "a .wittyinfo's details could not be taken in", "cat", "library", "item", it.ID, "err", err)
		}
	}
	return true
}

func writeInfoErr(w http.ResponseWriter, err error) {
	var e *wittyinfo.Error
	if errors.As(err, &e) {
		writeJSON(w, 400, map[string]any{"error": e.Error(), "problems": e.Problems})
		return
	}
	writeErr(w, 400, err.Error())
}

// current is the target's file to change: the one in use, or a new one.
func (s *Server) currentInfo(r *http.Request, t library.InfoTarget, it *library.Item) *wittyinfo.Info {
	if l := s.Store.LoadInfo(r.Context(), t); l.Info != nil {
		return l.Info.Clone()
	}
	x := wittyinfo.New(t.Scope())
	if t.Kind == library.InfoAlbum {
		x = wittyinfo.NewAudio(wittyinfo.ScopeAlbum)
	}
	if it != nil {
		x = library.NewFor(it)
		x.Identity = s.Store.IdentityOf(r.Context(), it)
	}
	return x
}

// PUT /api/wittyinfo/{target}/{id}
func (s *Server) handleInfoPut(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	var b infoEdit
	if err := readJSONUpTo(r, &b, wittyinfo.MaxManifest+1024); err != nil || len(b.Info) == 0 {
		writeErr(w, 400, "expected {\"info\": the manifest}")
		return
	}
	x, err := wittyinfo.Parse(b.Info)
	if err != nil {
		// The pictures it names are checked against the ones kept below.
		var e *wittyinfo.Error
		if !errors.As(err, &e) || !onlyMissingMembers(e) {
			writeInfoErr(w, err)
			return
		}
		x = &wittyinfo.Info{}
		if err := json.Unmarshal(b.Info, x); err != nil {
			writeInfoErr(w, err)
			return
		}
	}
	cur := s.currentInfo(r, t, it)
	x.Files = cur.Files
	// A file says which cut it was made for: one made here without saying is for this one.
	if it != nil && x.Identity == nil {
		x.Identity = cur.Identity
		if x.Identity == nil {
			x.Identity = s.Store.IdentityOf(r.Context(), it)
		}
	}
	if !s.saveInfo(w, r, t, it, x) {
		return
	}
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

// onlyMissingMembers: the only problems are names of pictures or subtitles not sent along.
func onlyMissingMembers(e *wittyinfo.Error) bool {
	for _, p := range e.Problems {
		if !strings.Contains(p.What, "which isn't in the .wittyinfo") {
			return false
		}
	}
	return true
}

// DELETE /api/wittyinfo/{target}/{id}: forget the server's copy; with ?file=1 also move the
// file beside the media out of the way (to .wittyinfo.bak), when writing there is allowed.
func (s *Server) handleInfoDelete(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if err := s.Store.SetStoredInfo(ctx, t, nil); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if r.URL.Query().Get("file") == "1" {
		l := s.Store.LoadInfo(ctx, t)
		if l.FileFound {
			if !s.saveInfoBeside(r) {
				writeErr(w, 409, "Writing .wittyinfo files beside the media is off, so the file there stays. Turn it on in Settings, or remove the file by hand.")
				return
			}
			if err := s.Store.RetireInfoFile(ctx, l.Path); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
	}
	_ = s.Store.SetInfoForce(ctx, t, false)
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

// GET /api/wittyinfo/{target}/{id}/export: the file in use, as a .wittyinfo to save.
func (s *Server) handleInfoExport(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	l := s.Store.LoadInfo(r.Context(), t)
	if l.Info == nil {
		writeErr(w, 404, "There is no .wittyinfo for this yet.")
		return
	}
	body, err := l.Info.Clone().Encode()
	if err != nil {
		writeInfoErr(w, err)
		return
	}
	name := "info" + wittyinfo.Ext
	switch {
	case l.Path != "":
		name = filepath.Base(l.Path)
	case it != nil:
		name = safeFileName(it.Title) + wittyinfo.Ext
	}
	w.Header().Set("Content-Type", "application/vnd.wittyinfo+zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func safeFileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if s == "" {
		return "info"
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}

// POST /api/wittyinfo/{target}/{id}/import: a .wittyinfo (archive or bare manifest) as the
// body replaces the one in use. One made for another cut is kept, and says so.
func (s *Server) handleInfoImport(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, wittyinfo.MaxFile+1))
	if err != nil {
		writeErr(w, 413, "A .wittyinfo can be 64 MB at most.")
		return
	}
	x, err := wittyinfo.Parse(b)
	if err != nil {
		writeInfoErr(w, err)
		return
	}
	want := t.Scope()
	if it != nil && library.IsSong(it) {
		want = wittyinfo.ScopeSong
	}
	if x.ScopeOr() != want {
		writeErr(w, 400, "This file is for a "+x.ScopeOr()+", not a "+want+".")
		return
	}
	if !s.saveInfo(w, r, t, it, x) {
		return
	}
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

// POST /api/wittyinfo/item/{id}/use-anyway {"use": true}: use a file made for another cut.
func (s *Server) handleInfoForce(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	var b struct {
		Use bool `json:"use"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected {\"use\": true or false}")
		return
	}
	if err := s.Store.SetInfoForce(r.Context(), t, b.Use); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if it != nil && s.Scanner != nil {
		_ = s.Scanner.ApplyInfoNow(r.Context(), it.ID)
	}
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

var infoPictureExt = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

// PUT /api/wittyinfo/{target}/{id}/images/{role}: a picture (JPEG, PNG or WebP) as the body,
// shown for the role in place of the one shown before; ?add=1 adds it after the role's
// other pictures. With no body, ?first=<name> shows that picture first.
func (s *Server) handleInfoImagePut(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, wittyinfo.MaxImage+1))
	if err != nil {
		writeErr(w, 413, "A picture can be 16 MB at most.")
		return
	}
	c := wittyinfo.ImageChange{Add: r.URL.Query().Get("add") == "1", First: r.URL.Query().Get("first")}
	if len(b) > 0 {
		if c.Ext = infoPictureExt[http.DetectContentType(b)]; c.Ext == "" {
			writeErr(w, 400, "Send a JPEG, PNG or WebP picture.")
			return
		}
		c.Picture = b
	}
	x := s.currentInfo(r, t, it)
	if err := x.ChangeImages(r.PathValue("role"), c); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.saveInfo(w, r, t, it, x) {
		return
	}
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

// DELETE /api/wittyinfo/{target}/{id}/images/{role}: every picture of the role, or with
// ?name=<name> that one.
func (s *Server) handleInfoImageDelete(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	x := s.currentInfo(r, t, it)
	if err := x.ChangeImages(r.PathValue("role"), wittyinfo.ImageChange{Remove: true, Name: r.URL.Query().Get("name")}); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if !s.saveInfo(w, r, t, it, x) {
		return
	}
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

// GET /api/wittyinfo/{target}/{id}/files/images/...: a picture from the file, to show.
// Nothing else in the archive is served.
func (s *Server) handleInfoFile(w http.ResponseWriter, r *http.Request) {
	t, _, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	name := r.PathValue("name")
	ext := strings.ToLower(filepath.Ext(name))
	if !strings.HasPrefix(name, "images/") || wittyinfo.ImageExts[ext] == "" {
		writeErr(w, 404, "not found")
		return
	}
	l := s.Store.LoadInfo(r.Context(), t)
	if l.Info == nil || l.Info.Files[name] == nil {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Content-Type", "image/"+wittyinfo.ImageExts[ext])
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Cache-Control", "private, max-age=60")
	_, _ = w.Write(l.Info.Files[name])
}

// ------------------------------------------------------------------ the player

// playbackInfo is what a video's .wittyinfo tells the player beyond skip times.
type playbackInfo struct {
	// Origin: where it was read from ("stored" or "file"); Match how it fits this cut.
	Origin string           `json:"origin"`
	Match  *wittyinfo.Match `json:"match,omitempty"`
	// Filters: content a viewer may want skipped or muted; their own settings decide.
	Filters []wittyinfo.Filter `json:"filters,omitempty"`
	// StartAt: where playing starts the first time.
	StartAt *float64 `json:"startAt,omitempty"`
	// Defaults: a season's or a show's file adds times.
	Defaults bool `json:"defaults,omitempty"`
	// VolumeDB: make it louder or quieter by this much. AspectRatio: show the picture at this
	// shape ("16:9", "2.39:1"), for a file that says it wrong.
	VolumeDB    *float64 `json:"volumeDb,omitempty"`
	AspectRatio string   `json:"aspectRatio,omitempty"`
	// Language: the language the labels and scene titles are in.
	Language string `json:"language,omitempty"`
}

// fillInfo puts what the .wittyinfo files say in a playback answer: scenes in place of the
// file's chapters, the content filters and where to start.
func fillInfo(infos library.ItemInfos, out *playbackResponse) {
	x := infos.UsableItem()
	defaults := infos.Season.Info != nil || infos.Show.Info != nil
	if x == nil && infos.Item.Info == nil && !defaults {
		return
	}
	pi := &playbackInfo{Origin: infos.Item.Origin, Defaults: defaults}
	if infos.Item.Info != nil {
		m := infos.Match
		pi.Match = &m
	}
	if x != nil {
		if len(x.Scenes) > 0 {
			out.Chapters = out.Chapters[:0]
			for _, sc := range x.Scenes {
				c := chapter{Start: float64(sc.Start), Title: sc.Title}
				if sc.ThumbnailAt != nil {
					v := float64(*sc.ThumbnailAt)
					c.ThumbnailAt = &v
				}
				out.Chapters = append(out.Chapters, c)
			}
		}
		pi.Filters = x.Filters
		pi.Language = x.Language
		if p := x.Playback; p != nil {
			if p.StartAt != nil {
				v := float64(*p.StartAt)
				pi.StartAt = &v
			}
			pi.VolumeDB, pi.AspectRatio = p.VolumeDB, p.AspectRatio
		}
	}
	out.Info = pi
}

// ------------------------------------------------------------------ languages and playing

type viewerLangKey struct{}

// viewerLanguage is the language the person asking reads: ?lang=, else what their browser or
// app says it reads (Accept-Language), else the app's language, else the disc menus'.
func (s *Server) viewerLanguage(r *http.Request) string {
	if v := r.URL.Query().Get("lang"); langtag.Valid(v) {
		return v
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, _, _ := strings.Cut(strings.TrimSpace(part), ";")
		if tag != "*" && langtag.Valid(tag) {
			return tag
		}
	}
	if v := s.DB.GetSetting(r.Context(), "uiLanguage", ""); v != "" {
		return v
	}
	return s.DB.GetSetting(r.Context(), "menuLanguage", "")
}

// withViewerLanguage is the request's context, carrying the viewer's language for what
// reads .wittyinfo text further down.
func (s *Server) withViewerLanguage(r *http.Request) context.Context {
	return context.WithValue(r.Context(), viewerLangKey{}, s.viewerLanguage(r))
}

func langFrom(ctx context.Context) string { v, _ := ctx.Value(viewerLangKey{}).(string); return v }

// infoPlayback is what a video's .wittyinfo says about playing it, or nothing.
func (s *Server) infoPlayback(ctx context.Context, it *library.Item) wittyinfo.Playback {
	infos := s.Store.InfosFor(ctx, it)
	if x := infos.UsableItem(); x != nil && x.Playback != nil {
		return *x.Playback
	}
	return wittyinfo.Playback{}
}

// subtitleByMode follows a file's subtitleMode when the viewer didn't pick a track: off, only
// forced subtitles (in the sound's language, else any), or always (the language chosen,
// else the first text track).
func subtitleByMode(src playback.Source, pb wittyinfo.Playback, chosen playback.Chosen) int {
	switch pb.SubtitleMode {
	case "off":
		return -1
	case "forced":
		audio := ""
		if chosen.Audio >= 0 && chosen.Audio < len(src.Audio) {
			audio = src.Audio[chosen.Audio].Language
		}
		any := -1
		for i, st := range src.Subtitles {
			if !st.Forced {
				continue
			}
			if langtag.Same(st.Language, audio) {
				return i
			}
			if any < 0 {
				any = i
			}
		}
		return any
	case "always":
		if chosen.Subtitle >= 0 {
			return chosen.Subtitle
		}
		for i, st := range src.Subtitles {
			if pb.SubtitleLanguage == "" || langtag.Same(st.Language, pb.SubtitleLanguage) {
				return i
			}
		}
		if len(src.Subtitles) > 0 {
			return 0
		}
	}
	return chosen.Subtitle
}

var infoSubtitleCodecs = map[string]string{"srt": "subrip", "vtt": "webvtt", "ass": "ass", "ssa": "ssa"}

// addInfoSubtitles lists the subtitles a video's .wittyinfo carries after its own, as files
// beside it are: each is a track players can pick, converted to WebVTT like any other.
func (s *Server) addInfoSubtitles(ctx context.Context, it *library.Item, src *playback.Source) {
	for _, f := range s.Store.InfoSubtitles(ctx, it, s.Cfg.CacheDir) {
		src.Subtitles = append(src.Subtitles, playback.Subtitle{Codec: infoSubtitleCodecs[f.Format], Language: f.Language, Title: f.Title,
			Forced: f.Forced, SDH: f.SDH, File: f.Path, Format: f.Format})
	}
}

// ------------------------------------------------------------------ subtitles in the file

var infoSubtitleName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,80}\.(srt|vtt|ass|ssa)$`)

// PUT /api/wittyinfo/{target}/{id}/subtitles/{name}?language=de&forced=1&sdh=1&title=…: a
// subtitle file (SRT, WebVTT, ASS or SSA, UTF-8, 2 MB at most) as the body, kept in the file
// as subtitles/<name>. The same name again replaces it.
func (s *Server) handleInfoSubtitlePut(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	if t.Kind != library.InfoItem {
		writeErr(w, 400, "Subtitles belong to one video.")
		return
	}
	name := r.PathValue("name")
	if !infoSubtitleName.MatchString(name) {
		writeErr(w, 400, "Name it like de.forced.srt: letters, digits, dots and dashes, ending .srt, .vtt, .ass or .ssa.")
		return
	}
	q := r.URL.Query()
	lang := q.Get("language")
	if lang != "" && !langtag.Valid(lang) {
		writeErr(w, 400, "language: a language code such as de or pt-BR")
		return
	}
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, wittyinfo.MaxSubtitle+1))
	if err != nil || len(b) > wittyinfo.MaxSubtitle {
		writeErr(w, 413, "A subtitle file can be 2 MB at most.")
		return
	}
	member := "subtitles/" + name
	if err := wittyinfo.CheckSubtitle(member, b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	x := s.currentInfo(r, t, it)
	if x.Files == nil {
		x.Files = map[string][]byte{}
	}
	x.Files[member] = b
	sub := wittyinfo.Subtitle{File: member, Language: lang, Title: q.Get("title"), Forced: q.Get("forced") == "1", SDH: q.Get("sdh") == "1"}
	kept := x.Subtitles[:0]
	for _, old := range x.Subtitles {
		if old.File != member {
			kept = append(kept, old)
		}
	}
	x.Subtitles = append(kept, sub)
	if !s.saveInfo(w, r, t, it, x) {
		return
	}
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

// DELETE /api/wittyinfo/{target}/{id}/subtitles/{name}
func (s *Server) handleInfoSubtitleDelete(w http.ResponseWriter, r *http.Request) {
	t, it, ok := s.infoTarget(w, r)
	if !ok {
		return
	}
	member := "subtitles/" + r.PathValue("name")
	x := s.currentInfo(r, t, it)
	kept := x.Subtitles[:0]
	for _, old := range x.Subtitles {
		if old.File != member {
			kept = append(kept, old)
		}
	}
	x.Subtitles = kept
	if !s.saveInfo(w, r, t, it, x) {
		return
	}
	writeJSON(w, 200, s.infoAnswerFor(r, t, it))
}

// ------------------------------------------------------------------ songs

// songInfo is a song's own .wittyaudio when it fits the song, else its album's, in lang.
func (s *Server) songInfo(ctx context.Context, it *library.Item, lang string) (song, album *wittyinfo.Info) {
	if !library.IsSong(it) {
		return nil, nil
	}
	infos := s.Store.InfosFor(ctx, it)
	infos.Localize(lang)
	return infos.UsableItem(), infos.Album.Info
}

// infoLyrics are a song's lyrics from its .wittyaudio, as the lyrics reader has them.
func infoLyrics(l *wittyinfo.Lyrics) lyrics.Doc {
	if l == nil {
		return lyrics.Doc{}
	}
	var d lyrics.Doc
	ms := func(v wittyinfo.Seconds) int64 { return int64(float64(v)*1000 + 0.5) }
	for i, ln := range l.Lines {
		out := lyrics.Line{T: ms(ln.Start), Text: ln.Text}
		if ln.End != nil {
			out.End = ms(*ln.End)
		} else if i+1 < len(l.Lines) {
			out.End = ms(l.Lines[i+1].Start)
		}
		for _, w := range ln.Words {
			out.Words = append(out.Words, lyrics.Word{T: ms(w.Start), Text: w.Text})
		}
		d.Timed = append(d.Timed, out)
	}
	if len(d.Timed) == 0 {
		d.Plain = l.Plain
	}
	return d
}

// infoLoudness is a song's loudness from its .wittyaudio (or its album's), if it says.
func (s *Server) infoLoudness(ctx context.Context, it *library.Item) (loudness, bool) {
	song, album := s.songInfo(ctx, it, "")
	var out loudness
	got := false
	for _, x := range []*wittyinfo.Info{album, song} {
		if x == nil || x.Audio == nil || x.Audio.Loudness == nil {
			continue
		}
		g := x.Audio.Loudness
		if g.TrackGain != nil {
			out.TrackDB, got = g.TrackGain, true
		}
		if g.TrackPeak != nil {
			out.TrackPeak, got = g.TrackPeak, true
		}
		if g.AlbumGain != nil {
			out.AlbumDB, got = g.AlbumGain, true
		}
		if g.AlbumPeak != nil {
			out.AlbumPeak, got = g.AlbumPeak, true
		}
	}
	return out, got
}
