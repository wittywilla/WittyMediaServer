// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/watchxref"
)

// Media extras: a looping video behind a song or title (canvas), the separated parts of a
// song (stems), where to skip in a video (markers), and where else a title can be watched.
// What's found is worked out by internal/mediaextras and internal/watchxref; these routes
// check who's asking and open files only through the library's own checks.

func (s *Server) mediaExtrasRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/media/{id}/canvas", a(s.handleCanvas))
	m.Handle("GET /api/items/{id}/canvas", a(s.handleCanvas))
	m.Handle("GET /api/media/{id}/canvas/file", a(s.handleCanvasFile))
	m.Handle("PUT /api/media/{id}/canvas", s.requireAdmin(s.handleCanvasUpload))
	m.Handle("DELETE /api/media/{id}/canvas", s.requireAdmin(s.handleCanvasDelete))
	m.Handle("GET /api/media/{id}/stems", a(s.handleStems))
	m.Handle("GET /api/media/{id}/stems/{stem}", a(s.handleStemFile))
	m.Handle("PUT /api/media/{id}/stems/{stem}", s.requireAdmin(s.handleStemUpload))
	m.Handle("POST /api/media/{id}/stems/approximate", a(s.handleStemsApproximate))
	m.Handle("POST /api/media/{id}/stems/separate", s.requireAdmin(s.handleStemsSeparate))
	m.Handle("POST /api/media/{id}/canvas/generate", s.requireAdmin(s.handleCanvasGenerate))
	m.Handle("DELETE /api/media/{id}/stems/{stem}", s.requireAdmin(s.handleStemDelete))
	m.Handle("GET /api/media/{id}/markers", a(s.handleMarkers))
	m.Handle("PUT /api/media/{id}/markers", s.requireAdmin(s.handleMarkersPut))
	m.Handle("POST /api/shows/{id}/detect-intros", s.requireAdmin(s.handleDetectIntros))
	m.Handle("GET /api/media/{id}/where-to-watch", a(s.handleWhereToWatch))
	m.Handle("GET /api/discover/where-to-watch", a(s.handleDiscoverWhereToWatch))
}

// extrasItem is the item of a request, if the account may see it.
func (s *Server) extrasItem(w http.ResponseWriter, r *http.Request) *library.Item {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "not found")
		return nil
	}
	return it
}

// extrasDir is where uploads for an item are kept: in the server's images folder, never
// beside the media.
func (s *Server) extrasRoot() string { return filepath.Join(s.Cfg.ImagesDir, mediaextras.DirName) }

// localFileItem says an item is a file in a library folder (not a web address, a CD track
// or a stream), whose folder can hold files that go with it.
func localFileItem(it *library.Item) bool {
	return it.Path != "" && !isWeb(it.Path) && !it.IsCDTrack() && filepath.IsAbs(it.Path)
}

// extraFile is a file found for an item: either uploaded (in the server's folder) or
// beside the media (opened only through the library's checks).
type extraFile struct {
	path    string
	source  string // upload, sidecar, metadata-folder
	library bool   // opened through the library
}

func (s *Server) openExtra(ctx context.Context, f extraFile) (*os.File, os.FileInfo, error) {
	if f.library {
		return s.Store.OpenLibraryFile(ctx, f.path)
	}
	return mediaextras.OpenPlain(f.path)
}

// serveExtra sends a found file with byte ranges, an ETag and a cache lifetime.
func (s *Server) serveExtra(w http.ResponseWriter, r *http.Request, f extraFile, mime string) {
	fh, st, err := s.openExtra(r.Context(), f)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	defer fh.Close()
	h := w.Header()
	h.Set("Content-Type", mime)
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("Cache-Control", "private, max-age=86400")
	h.Set("ETag", extraETag(st))
	http.ServeContent(s.capWriter(w, r), r, "", st.ModTime(), fh)
}

func extraETag(st os.FileInfo) string {
	return `"x` + strconv.FormatInt(st.Size(), 36) + "-" + strconv.FormatInt(st.ModTime().UnixNano(), 36) + `"`
}

// ------------------------------------------------------------------ canvas

// canvasInfo is what GET /media/{id}/canvas says.
type canvasInfo struct {
	Available  bool   `json:"available"`
	URL        string `json:"url,omitempty"`
	MIME       string `json:"mime,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Loop       bool   `json:"loop"`
	// Source: upload, generated (cut from the video by the server), sidecar or metadata-folder.
	Source string `json:"source,omitempty"`
	// Reason says why a file that was found isn't used: too-big or too-long.
	Reason string `json:"reason,omitempty"`
	// Task is the task making one now, when one is.
	Task string `json:"task,omitempty"`
	// CanGenerate: an administrator can have one cut from the video.
	CanGenerate bool `json:"canGenerate"`
	// Error is why the last try at cutting one failed, for the administrator who asked.
	Error string `json:"error,omitempty"`
	// Limited: this viewer is away from home and the owner limits the data of a stream
	// there. The canvas is still served (slowed to the limit), but it is decoration, and an
	// app should leave it out rather than spend the viewer's data on it.
	Limited bool `json:"limited,omitempty"`
}

// canvasKinds are the items a canvas can go with.
func canvasKind(it *library.Item) bool {
	return it.Kind == "audio" || it.Kind == "video" || library.IsDiscKind(it.Kind)
}

// findCanvas looks for an item's canvas: an upload, then beside the media, then in the
// editable metadata folder.
func (s *Server) findCanvas(ctx context.Context, it *library.Item) (extraFile, bool) {
	if !canvasKind(it) {
		return extraFile{}, false
	}
	if p := mediaextras.FirstPlain(s.extrasRoot(), mediaextras.Key(it.ID, it.Path)+"/canvas", mediaextras.CanvasExts); p != "" {
		source := "upload"
		if _, err := os.Lstat(filepath.Join(filepath.Dir(p), mediaextras.GeneratedMark)); err == nil {
			source = "generated"
		}
		return extraFile{path: p, source: source}, true
	}
	if localFileItem(it) {
		showDir := ""
		if it.ShowID != nil {
			if sh, err := s.Store.ShowFacts(ctx, *it.ShowID); err == nil && sh.Path != "" {
				showDir = sh.Path
			}
		}
		isDir := it.Kind == "dvd-folder" || it.Kind == "bluray-folder"
		for _, p := range mediaextras.CanvasCandidates(it.Path, isDir, showDir) {
			if s.Store.LibraryFile(ctx, p) {
				return extraFile{path: p, source: "sidecar", library: true}, true
			}
		}
	}
	if p := mediaextras.FindInMetaFolder(s.metaFolderDir(), it.ID, "canvas", mediaextras.CanvasExts); p != "" {
		return extraFile{path: p, source: "metadata-folder"}, true
	}
	return extraFile{}, false
}

// canvasProbe is what FFprobe said about a canvas, kept by its path, size and time.
var canvasProbes sync.Map // key -> canvasInfo (width, height, durationMs only)

func (s *Server) probeCanvas(ctx context.Context, f extraFile, st os.FileInfo) (w, h int, ms int64) {
	if s.Tools == nil || s.Tools.FFprobe == "" {
		return 0, 0, 0
	}
	key := f.path + "|" + strconv.FormatInt(st.Size(), 10) + "|" + strconv.FormatInt(st.ModTime().UnixNano(), 10)
	if v, ok := canvasProbes.Load(key); ok {
		c := v.(canvasInfo)
		return c.Width, c.Height, c.DurationMs
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	p, err := s.Tools.ProbeFile(pctx, f.path)
	if err != nil {
		return 0, 0, 0
	}
	var c canvasInfo
	if v := p.Video(); v != nil {
		c.Width, c.Height = v.DisplaySize()
	}
	if d := p.Duration(); d > 0 && d < 3600 {
		c.DurationMs = int64(d * 1000)
	}
	canvasProbes.Store(key, c)
	return c.Width, c.Height, c.DurationMs
}

// canvasRefused says why a file that was found isn't used as a canvas: "too-big" past
// MaxCanvasSidecar, "too-long" past MaxCanvasSeconds (when FFprobe can say), "" when it is
// fine. Uploads were held to MaxCanvas when they arrived.
func (s *Server) canvasRefused(ctx context.Context, f extraFile, st os.FileInfo) string {
	if st.Size() > mediaextras.MaxCanvasSidecar {
		return "too-big"
	}
	if _, _, ms := s.probeCanvas(ctx, f, st); ms > mediaextras.MaxCanvasSeconds*1000 {
		return "too-long"
	}
	return ""
}

// GET /api/media/{id}/canvas, and /api/items/{id}/canvas.
func (s *Server) handleCanvas(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	out := canvasInfo{Loop: true, Task: s.mediaJobTask(fmt.Sprintf("canvas:%d", it.ID))}
	out.CanGenerate = userFrom(r).IsAdmin && it.Kind == "video" && localFileItem(it) && s.canvasEncoder() != "" && s.Tools.FFmpeg != ""
	if _, rate := s.remoteCaps(r); rate != "" {
		out.Limited = true
	}
	if err := s.mediaJobError(fmt.Sprintf("canvas:%d", it.ID)); err != nil && userFrom(r).IsAdmin {
		out.Error = err.Error()
	}
	f, ok := s.findCanvas(r.Context(), it)
	if ok {
		if fh, st, err := s.openExtra(r.Context(), f); err == nil {
			fh.Close()
			if out.Reason = s.canvasRefused(r.Context(), f, st); out.Reason == "" {
				out.Available, out.Source, out.MIME = true, f.source, mediaextras.CanvasMIME(f.path)
				out.URL = fmt.Sprintf("/api/media/%d/canvas/file?v=%s", it.ID, strings.Trim(extraETag(st), `"`))
				out.Width, out.Height, out.DurationMs = s.probeCanvas(r.Context(), f, st)
			}
		}
	}
	writeJSON(w, 200, out)
}

// GET /api/media/{id}/canvas/file: the video itself, with byte ranges.
func (s *Server) handleCanvasFile(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	f, ok := s.findCanvas(r.Context(), it)
	if !ok {
		writeErr(w, 404, "no canvas")
		return
	}
	// The same limits as the listing: what it wouldn't name isn't served.
	if fh, st, err := s.openExtra(r.Context(), f); err == nil {
		fh.Close()
		if s.canvasRefused(r.Context(), f, st) != "" {
			writeErr(w, 404, "no canvas")
			return
		}
	}
	s.serveExtra(w, r, f, mediaextras.CanvasMIME(f.path))
}

// uploadErr answers a refused upload.
func uploadErr(w http.ResponseWriter, err error, what string, max int64) {
	var mb *http.MaxBytesError
	switch {
	case errors.Is(err, mediaextras.ErrTooBig) || errors.As(err, &mb):
		writeErr(w, 413, fmt.Sprintf("that file is too big: the limit for %s is %d MB", what, max>>20))
	case errors.Is(err, mediaextras.ErrBadType):
		writeErr(w, 415, "that isn't a kind of file "+what+" can be")
	case errors.Is(err, mediaextras.ErrBadName):
		writeErr(w, 400, err.Error())
	default:
		writeErr(w, 500, "couldn't keep the file: "+err.Error())
	}
}

// PUT /api/media/{id}/canvas: an MP4 or WebM, as the body, of at most 30 MB.
func (s *Server) handleCanvasUpload(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	if !canvasKind(it) {
		writeErr(w, 400, "only songs, videos and discs have a canvas")
		return
	}
	rel := mediaextras.Key(it.ID, it.Path) + "/canvas"
	dst, size, err := mediaextras.SaveUpload(s.extrasRoot(), rel, http.MaxBytesReader(w, r.Body, mediaextras.MaxCanvas), mediaextras.MaxCanvas, mediaextras.VideoExt)
	if err != nil {
		uploadErr(w, err, "a canvas", mediaextras.MaxCanvas)
		return
	}
	s.removeOthers(rel, mediaextras.CanvasExts, dst)
	_ = os.Remove(filepath.Join(filepath.Dir(dst), mediaextras.GeneratedMark))
	s.Log.InfoContext(r.Context(), "canvas uploaded", "cat", "extras", "item", it.ID, "bytes", size)
	writeJSON(w, 200, map[string]any{"ok": true, "size": size, "mime": mediaextras.CanvasMIME(dst)})
}

// removeOthers takes away an upload's other kinds once a new one is in place.
func (s *Server) removeOthers(rel string, exts []string, keep string) {
	for _, ext := range exts {
		if p := filepath.Join(s.extrasRoot(), filepath.FromSlash(rel)+ext); p != keep {
			_, _ = mediaextras.RemoveMatching(s.extrasRoot(), rel, []string{ext})
		}
	}
}

// DELETE /api/media/{id}/canvas: forgets an uploaded canvas. One beside the media stays.
func (s *Server) handleCanvasDelete(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	found, err := mediaextras.RemoveMatching(s.extrasRoot(), mediaextras.Key(it.ID, it.Path)+"/canvas", mediaextras.CanvasExts)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !found {
		writeErr(w, 404, "there's no uploaded canvas: one beside the media is removed from the folder itself")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ------------------------------------------------------------------ stems

// stemView is one stem as listed.
type stemView struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	MIME       string `json:"mime"`
	Size       int64  `json:"size"`
	Multitrack bool   `json:"multitrack,omitempty"`
	Source     string `json:"source"`
	// Approximate: FFmpeg's centre-channel removal, not a real stem.
	Approximate bool `json:"approximate,omitempty"`
}

// stemsOf lists an item's stems. Those beside the media count only if they're inside a
// library once links are followed.
func (s *Server) stemsOf(ctx context.Context, it *library.Item) []mediaextras.Stem {
	if it.Kind != "audio" {
		return nil
	}
	mediaPath := it.Path
	if !localFileItem(it) {
		mediaPath = filepath.Join(string(filepath.Separator), "nowhere", "x")
	}
	var out []mediaextras.Stem
	uploads := filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path))
	separated := mediaextras.Separated(filepath.Join(uploads, "stems"))
	real := false
	for _, st := range mediaextras.FindStems(mediaPath, uploads) {
		if !st.Uploaded && (!localFileItem(it) || !s.Store.LibraryFile(ctx, st.Path)) {
			continue
		}
		if st.Uploaded && separated[st.Name] {
			st.Source = "separated"
		}
		if st.Name == "instrumental" {
			real = true
		}
		out = append(out, st)
	}
	// The approximate instrumental stands in only while there is no real one.
	if p := filepath.Join(uploads, mediaextras.ApproxDir, "instrumental"+mediaextras.ApproxExt); !real {
		if fi, err := os.Lstat(p); err == nil && fi.Mode().IsRegular() {
			st := mediaextras.Stem{Name: "instrumental", Path: p, Size: fi.Size(), MIME: mediaextras.StemMIME(p), Source: "approximate", Approximate: true, Uploaded: true}
			out = append([]mediaextras.Stem{st}, out...)
		}
	}
	return out
}

func stemViews(it *library.Item, stems []mediaextras.Stem, token string) []stemView {
	out := []stemView{}
	for _, st := range stems {
		out = append(out, stemView{Name: st.Name, URL: withStream(fmt.Sprintf("/api/media/%d/stems/%s", it.ID, st.Name), token), MIME: st.MIME, Size: st.Size, Multitrack: st.Multitrack, Source: st.Source, Approximate: st.Approximate})
	}
	return out
}

// GET /api/media/{id}/stems
func (s *Server) handleStems(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	writeJSON(w, 200, map[string]any{"stems": stemViews(it, s.stemsOf(r.Context(), it), ""), "names": mediaextras.StemNames, "separation": s.separationFor(r, it)})
}

// GET /api/media/{id}/stems/{stem}: one stem, with byte ranges.
func (s *Server) handleStemFile(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	dir := filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path))
	// "full" is the whole song, for blending against an instrumental (see api_karaoke_auto.go).
	if r.PathValue("stem") == mediaextras.ApproxFull {
		if f, mime, _, ok := s.karaokeFull(r.Context(), it); ok {
			mediaextras.TouchKaraoke(dir)
			s.serveExtra(w, r, f, mime)
			return
		}
		writeErr(w, 404, "no such stem")
		return
	}
	name := mediaextras.StemName(r.PathValue("stem"))
	if name == "" {
		writeErr(w, 404, "no such stem")
		return
	}
	for _, st := range s.stemsOf(r.Context(), it) {
		if st.Name == name {
			if st.Uploaded {
				mediaextras.TouchKaraoke(dir)
			}
			s.serveExtra(w, r, extraFile{path: st.Path, source: st.Source, library: !st.Uploaded}, st.MIME)
			return
		}
	}
	writeErr(w, 404, "no such stem")
}

// PUT /api/media/{id}/stems/{stem}: an audio file as the body. multitrack takes a
// Native Instruments .stem.mp4.
func (s *Server) handleStemUpload(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	name := mediaextras.StemName(r.PathValue("stem"))
	if name == "" {
		writeErr(w, 400, "a stem is one of: "+strings.Join(mediaextras.StemNames, ", ")+", or multitrack")
		return
	}
	if it.Kind != "audio" {
		writeErr(w, 400, "only songs have stems")
		return
	}
	check, exts, rel := mediaextras.AudioExt, mediaextras.StemExts, mediaextras.Key(it.ID, it.Path)+"/stems/"+name
	if name == mediaextras.Multitrack {
		check, exts, rel = mediaextras.MultitrackExt, []string{".mp4"}, mediaextras.Key(it.ID, it.Path)+"/stems/"+name+".stem"
	}
	dst, size, err := mediaextras.SaveUpload(s.extrasRoot(), rel, http.MaxBytesReader(w, r.Body, mediaextras.MaxStem), mediaextras.MaxStem, check)
	if err != nil {
		uploadErr(w, err, "a stem", mediaextras.MaxStem)
		return
	}
	s.removeOthers(rel, exts, dst)
	s.forgetSeparated(filepath.Dir(dst), name)
	s.Log.InfoContext(r.Context(), "stem uploaded", "cat", "extras", "item", it.ID, "stem", name, "bytes", size)
	writeJSON(w, 200, map[string]any{"ok": true, "name": name, "size": size, "mime": mediaextras.StemMIME(dst)})
}

// forgetSeparated takes a part off the list of those a separator made: an upload has
// replaced it.
func (s *Server) forgetSeparated(stemsDir, name string) {
	list := mediaextras.Separated(stemsDir)
	if !list[name] {
		return
	}
	delete(list, name)
	names := make([]string, 0, len(list))
	for k := range list {
		names = append(names, k)
	}
	if err := os.WriteFile(filepath.Join(stemsDir, mediaextras.SeparatedList), []byte(strings.Join(names, "\n")+"\n"), 0o644); err != nil {
		s.Log.Warn("could not save the list of separated stems", "cat", "extras", "dir", stemsDir, "err", err)
	}
}

// DELETE /api/media/{id}/stems/{stem}: forgets an uploaded stem.
func (s *Server) handleStemDelete(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	name := mediaextras.StemName(r.PathValue("stem"))
	if name == "" {
		writeErr(w, 400, "no such stem")
		return
	}
	rel, exts := mediaextras.Key(it.ID, it.Path)+"/stems/"+name, mediaextras.StemExts
	if name == mediaextras.Multitrack {
		rel, exts = rel+".stem", []string{".mp4"}
	}
	found, err := mediaextras.RemoveMatching(s.extrasRoot(), rel, exts)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if !found {
		writeErr(w, 404, "there's no uploaded stem by that name: one beside the song is removed from the folder itself")
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ------------------------------------------------------------------ markers

// readSidecar reads a small file beside the media, through the library's checks.
func (s *Server) readSidecar(ctx context.Context, p string) []byte {
	if !s.Store.LibraryFile(ctx, p) {
		return nil
	}
	f, _, err := s.Store.OpenLibraryFile(ctx, p)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, mediaextras.MaxSidecar+1))
	if err != nil || len(b) > mediaextras.MaxSidecar {
		return nil
	}
	return b
}

// sidecarNames are where a sidecar of this kind can be: "Film.mkv.edl" and "Film.edl".
func sidecarNames(mediaPath, ext string) []string {
	trimmed := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	if trimmed == mediaPath {
		return []string{mediaPath + ext}
	}
	return []string{trimmed + ext, mediaPath + ext}
}

// mediaMarkers works out an item's markers from every source, most trusted first.
func (s *Server) mediaMarkers(ctx context.Context, it *library.Item) mediaextras.Markers {
	stored := s.Store.Markers(ctx, it.ID)
	var layers []mediaextras.Layer
	if stored.Manual != nil {
		layers = append(layers, mediaextras.Layer{Source: mediaextras.SourceManual, Markers: *stored.Manual})
	}
	if localFileItem(it) && (it.Kind == "video" || library.IsDiscKind(it.Kind)) {
		for _, p := range sidecarNames(it.Path, ".skip.json") {
			if b := s.readSidecar(ctx, p); b != nil {
				if m, err := mediaextras.ParseSkipJSON(b, it.RuntimeSec); err == nil {
					layers = append(layers, mediaextras.Layer{Source: mediaextras.SourceSkipJSON, Markers: m})
					break
				}
			}
		}
		for _, p := range sidecarNames(it.Path, ".edl") {
			if b := s.readSidecar(ctx, p); b != nil {
				layers = append(layers, mediaextras.Layer{Source: mediaextras.SourceEDL, Markers: mediaextras.ParseEDL(b)})
				break
			}
		}
	}
	var ch mediaextras.Markers
	if stored.Scanned != nil {
		ch = *stored.Scanned
	}
	if ch.Intro == nil && it.IntroEnd != nil && *it.IntroEnd > 0 {
		r := mediaextras.Range{EndSec: *it.IntroEnd}
		if it.IntroStart != nil {
			r.StartSec = *it.IntroStart
		}
		ch.Intro = &r
	}
	layers = append(layers, mediaextras.Layer{Source: mediaextras.SourceChapters, Markers: ch})
	if stored.Detected != nil {
		layers = append(layers, mediaextras.Layer{Source: mediaextras.SourceDetected, Markers: *stored.Detected})
	}
	if it.ShowID != nil {
		if sh, err := s.Store.Show(ctx, *it.ShowID, 0); err == nil && sh.IntroEnd != nil && *sh.IntroEnd > 0 {
			r := mediaextras.Range{EndSec: *sh.IntroEnd}
			if sh.IntroStart != nil {
				r.StartSec = *sh.IntroStart
			}
			layers = append(layers, mediaextras.Layer{Source: mediaextras.SourceShow, Markers: mediaextras.Markers{Intro: &r}})
		}
	}
	m := mediaextras.Merge(layers...)
	if m.Credits != nil && m.Credits.EndSec <= 0 && it.RuntimeSec > m.Credits.StartSec {
		m.Credits.EndSec = it.RuntimeSec // "to the end"
	}
	return m
}

// GET /api/media/{id}/markers
func (s *Server) handleMarkers(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	writeJSON(w, 200, s.mediaMarkers(r.Context(), it))
}

// markersEdit is the body of PUT /api/media/{id}/markers: a skip file, or reset to go back
// to what the files say.
type markersEdit struct {
	mediaextras.SkipFile
	Reset bool `json:"reset,omitempty"`
}

// PUT /api/media/{id}/markers
func (s *Server) handleMarkersPut(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	var b markersEdit
	if err := readJSONUpTo(r, &b, 64<<10); err != nil {
		writeErr(w, 400, "expected a JSON body: "+err.Error())
		return
	}
	var keep *mediaextras.Markers
	if !b.Reset {
		m, err := b.SkipFile.Markers(it.RuntimeSec)
		if err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		keep = &m
	}
	if err := s.Store.SetManualMarkers(r.Context(), it.ID, keep); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.mediaMarkers(r.Context(), it))
}

// ------------------------------------------------------------------ where to watch

// watchProvidersFor lets tests put their own provider in TMDB's place.
var watchProvidersFor func(s *Server) []watchxref.Provider

func (s *Server) watchXref() *watchxref.Service {
	var providers []watchxref.Provider
	if watchProvidersFor != nil {
		providers = watchProvidersFor(s)
	} else if s.Scanner != nil {
		providers = []watchxref.Provider{watchxref.TMDB{Client: s.Scanner.TMDBClient}}
	}
	return &watchxref.Service{Providers: providers, Cache: watchxref.StoreCache{Store: s.Store}, MaxAge: library.WatchCacheMaxAge}
}

func (s *Server) watchRegion(r *http.Request) string {
	return watchxref.CleanRegion(r.URL.Query().Get("region"), watchxref.CleanRegion(s.Store.ListingsRegion(r.Context()), "US"))
}

// versionOf is one copy of a title.
func versionOf(it *library.Item, label string) watchxref.Version {
	return watchxref.Version{ItemID: it.ID, Label: label, Kind: it.Kind, Quality: watchxref.Quality(it.Kind, it.Width, it.Height),
		Width: it.Width, Height: it.Height, Container: it.Container, Codec: it.VideoCodec, SizeBytes: it.SizeBytes}
}

// localFor is what the library has of an item's title, as this viewer may see it.
func (s *Server) localFor(r *http.Request, it *library.Item) *watchxref.Local {
	ctx := r.Context()
	l := &watchxref.Local{ItemID: it.ID, Title: it.Title, Versions: []watchxref.Version{versionOf(it, "")}}
	versions, links, err := s.Store.AttachedTo(ctx, library.TargetItem, it.ID, library.LinkVersion)
	if err == nil {
		acc := s.access(r)
		for i, v := range versions {
			var sh *library.Show
			if acc != nil && v.ShowID != nil {
				sh, _ = s.Store.ShowFacts(ctx, *v.ShowID)
			}
			if acc == nil || acc.AllowsItem(v, sh) {
				l.Versions = append(l.Versions, versionOf(v, links[i].Label))
			}
		}
	}
	for _, v := range l.Versions {
		l.Quality = watchxref.Best(l.Quality, v.Quality)
	}
	return l
}

// refFor is the TMDB title a local title is matched with.
func (s *Server) refFor(ctx context.Context, target string, id int64, provider, providerID string) watchxref.Ref {
	switch provider {
	case "tmdb":
		return watchxref.Ref{Kind: "movie", ID: providerID}
	case "tmdb-tv":
		return watchxref.Ref{Kind: "tv", ID: providerID}
	}
	kind, tid := s.Store.ListingTMDB(ctx, target, id)
	return watchxref.Ref{Kind: kind, ID: tid}
}

// GET /api/media/{id}/where-to-watch: this title here, and where else it is. region overrides
// the server's.
func (s *Server) handleWhereToWatch(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	ctx := r.Context()
	local := s.localFor(r, it)
	var ref watchxref.Ref
	if it.ShowID != nil {
		sh, err := s.visibleShow(r, *it.ShowID)
		if err != nil {
			writeErr(w, 404, "not found")
			return
		}
		local.ShowID, local.Title = sh.ID, sh.Title
		ref = s.refFor(ctx, library.TargetShow, sh.ID, sh.Provider, sh.ProviderID)
	} else {
		ref = s.refFor(ctx, library.TargetItem, it.ID, it.Provider, it.ProviderID)
	}
	writeJSON(w, 200, s.watchXref().Lookup(ctx, local, ref, s.watchRegion(r)))
}

// GET /api/discover/where-to-watch?tmdb=&type=movie|tv: a title that may not be here.
func (s *Server) handleDiscoverWhereToWatch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ref := watchxref.Ref{Kind: q.Get("type"), ID: q.Get("tmdb")}
	if ref.Kind == "" {
		ref.Kind = "movie"
	}
	if !ref.Valid() {
		writeErr(w, 400, "tmdb is a TMDB id (digits) and type is movie or tv")
		return
	}
	ctx := r.Context()
	var local *watchxref.Local
	items, shows := s.Store.ByTMDB(ctx, ref.Kind, ref.ID)
	for _, id := range items {
		if it, err := s.visibleItem(r, id); err == nil {
			local = s.localFor(r, it)
			break
		}
	}
	for _, id := range shows {
		if local != nil {
			break
		}
		if sh, err := s.visibleShow(r, id); err == nil {
			local = &watchxref.Local{ShowID: sh.ID, Title: sh.Title, Versions: []watchxref.Version{}}
		}
	}
	writeJSON(w, 200, s.watchXref().Lookup(ctx, local, ref, s.watchRegion(r)))
}

// mediaUploadPath: canvas and stem uploads are bigger than the usual limit, and set their
// own (30 MB and 200 MB) once they know an administrator is asking.
func mediaUploadPath(p string) bool {
	if rest, ok := strings.CutPrefix(p, apiPrefix); ok {
		p = "/api/" + rest
	}
	rest, ok := strings.CutPrefix(p, "/api/media/")
	if !ok {
		return false
	}
	_, tail, ok := strings.Cut(rest, "/")
	// Not the two that start a job: those take a small JSON body or none.
	return ok && (tail == "canvas" || (strings.HasPrefix(tail, "stems/") && tail != "stems/approximate" && tail != "stems/separate"))
}

// fillMarkers adds what GET /api/media/{id}/markers knows to a playback answer. An intro
// set by hand, or in a .skip.json or .edl, replaces the one the chapters gave.
func (s *Server) fillMarkers(ctx context.Context, it *library.Item, out *playbackResponse) {
	m := s.mediaMarkers(ctx, it)
	conv := func(r *mediaextras.Range) *playbackIntro {
		if r == nil || r.EndSec <= r.StartSec {
			return nil
		}
		return &playbackIntro{StartSec: r.StartSec, EndSec: r.EndSec}
	}
	if in := conv(m.Intro); in != nil {
		out.Intro = in
	}
	out.Recap, out.Credits = conv(m.Recap), conv(m.Credits)
	for i := range m.Commercials {
		if c := conv(&m.Commercials[i]); c != nil {
			out.Commercials = append(out.Commercials, *c)
		}
	}
}
