package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/playback"
)

// playInfo tells the client how to play a regular video item.
type playInfo struct {
	Type        string      `json:"type"` // direct | hls
	URL         string      `json:"url"`
	DurationSec float64     `json:"durationSec"`
	Audio       []trackInfo `json:"audio"`
	Subtitles   []trackInfo `json:"subtitles"`
	AudioIndex  int         `json:"audioIndex"`
	SubIndex    int         `json:"subtitleIndex"`
	Container   string      `json:"container"`
	Chapters    []chapter   `json:"chapters,omitempty"`
}

// chapter is a named point in a video, from the file.
type chapter struct {
	Start float64 `json:"start"`
	Title string  `json:"title"`
}

type trackInfo struct {
	Index    int    `json:"index"`
	Language string `json:"language"`
	Title    string `json:"title,omitempty"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels,omitempty"`
}

func (s *Server) audioCodec(r *http.Request) string {
	if v := r.URL.Query().Get("audioCodec"); v == "mp3" || v == "aac" {
		return v
	}
	if s.DB.GetSetting(r.Context(), "audioCodec", "aac") == "mp3" {
		return "mp3"
	}
	return "aac"
}

// quality builds the transcode profile from Settings -> Transcoding:
//
//	easy      a named profile (speed | balanced | quality)
//	advanced  preset, constant quality, bitrate cap and audio bitrate
//	expert    advanced + raw ffmpeg encoder/output options
//
// Hardware encoders are used in the order of ffmpeg.HWEncoders when enabled
// and found working on this machine.
func (s *Server) quality(r *http.Request) (q ffmpeg.Quality, maxHeight int) {
	ctx := r.Context()
	get := func(k, def string) string { return s.DB.GetSetting(ctx, k, def) }
	maxHeight, _ = strconv.Atoi(get("maxHeight", "0"))
	// A viewer can ask for less than the admin's limit, not for more.
	if v := qInt(r, "maxHeight", 0); v >= 144 && v <= 4320 && (maxHeight <= 0 || v < maxHeight) {
		maxHeight = v
	}
	enabled := get("hwEncoders", "")
	if enabled == "" && get("hwAccel", "0") == "1" { // settings from older versions: "use any"
		for _, e := range ffmpeg.HWEncoders {
			q.HW = append(q.HW, e.Name)
		}
	}
	for _, name := range strings.Split(enabled, ",") {
		if name = strings.TrimSpace(name); name != "" {
			q.HW = append(q.HW, name)
		}
	}
	switch get("qualityMode", "easy") {
	case "advanced", "expert":
		if v := get("x264Preset", "veryfast"); ffmpeg.ValidPreset(v) {
			q.Preset = v
		}
		q.CRF, _ = strconv.Atoi(get("crf", "21"))
		if v := get("maxBitrate", ""); ffmpeg.ValidRate(v) {
			q.MaxRate = v
		}
		if v := get("audioBitrate", ""); ffmpeg.ValidRate(v) {
			q.AudioRate = v
		}
		if get("qualityMode", "") == "expert" {
			// Saved by an older version, perhaps: checked again before use.
			for _, a := range []struct {
				setting string
				to      *[]string
			}{{"ffmpegVideoArgs", &q.VideoArgs}, {"ffmpegOutputArgs", &q.OutputArgs}} {
				args := ffmpeg.SplitArgs(get(a.setting, ""))
				if err := ffmpeg.CheckExpertArgs(args); err != nil {
					s.Log.Warn("ignoring expert FFmpeg options", "setting", a.setting, "err", err)
					continue
				}
				*a.to = args
			}
		}
	default:
		switch get("qualityProfile", "balanced") {
		case "speed":
			q.Preset, q.CRF, q.AudioRate = "superfast", 23, "160k"
		case "quality":
			q.Preset, q.CRF, q.AudioRate = "medium", 19, "256k"
		default:
			q.Preset, q.CRF, q.AudioRate = "veryfast", 21, "192k"
		}
	}
	// The limits for viewers away from home always win.
	if h, rate := s.remoteCaps(r); h > 0 || rate != "" {
		if h > 0 && (maxHeight == 0 || h < maxHeight) {
			maxHeight = h
		}
		q = q.WithCap(rate)
	}
	return q, maxHeight
}

func (s *Server) handlePlayInfo(w http.ResponseWriter, r *http.Request) {
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
	if it.Kind != "video" {
		writeErr(w, 400, "only videos play here: discs use the disc player, and audio its stream")
		return
	}
	if !s.Store.LibraryFile(r.Context(), it.Path) {
		writeErr(w, 404, "file missing")
		return
	}
	pctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	pr, err := s.probeOnce(pctx, it.Path)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		w.Header().Set("Retry-After", "5")
		writeErr(w, 503, "the server is busy; try again in a moment")
		return
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	info := playInfo{DurationSec: pr.Duration(), Container: it.Container, AudioIndex: qInt(r, "audio", 0), SubIndex: qInt(r, "subtitle", -1)}
	if n := len(pr.Audio()); info.AudioIndex < 0 || info.AudioIndex >= max(n, 1) {
		info.AudioIndex = 0
	}
	if info.SubIndex >= len(pr.Subtitles()) || info.SubIndex < -1 {
		info.SubIndex = -1
	}
	for i, a := range pr.Audio() {
		info.Audio = append(info.Audio, trackInfo{Index: i, Language: a.Tags["language"], Title: a.Tags["title"], Codec: a.CodecName, Channels: a.Channels})
	}
	for i, st := range pr.Subtitles() {
		info.Subtitles = append(info.Subtitles, trackInfo{Index: i, Language: st.Tags["language"], Title: st.Tags["title"], Codec: st.CodecName})
	}
	for _, c := range pr.Chapters {
		if at, err := strconv.ParseFloat(c.Start, 64); err == nil {
			info.Chapters = append(info.Chapters, chapter{Start: at, Title: c.Tags["title"]})
		}
	}
	if info.Audio == nil {
		info.Audio = []trackInfo{}
	}
	if info.Subtitles == nil {
		info.Subtitles = []trackInfo{}
	}
	// Tracks that don't exist are not asked of FFmpeg (a negative one would
	// panic below), and don't each start a stream of their own.
	if info.AudioIndex < 0 || info.AudioIndex >= len(info.Audio) {
		info.AudioIndex = 0
	}
	if info.SubIndex < 0 || info.SubIndex >= len(info.Subtitles) {
		info.SubIndex = -1
	}
	v := pr.Video()
	q, maxHeight := s.quality(r)
	ext := strings.ToLower(filepath.Ext(it.Path))
	directPlay := s.DB.GetSetting(r.Context(), "directPlay", "1") == "1"
	web, _ := playback.Find("web")
	d := playback.Decide(web, probeSource(it, pr), playback.Request{AudioIndex: info.AudioIndex, SubtitleIndex: info.SubIndex, MaxHeight: maxHeight, AllowDirect: directPlay, ForceTranscode: qBool(r, "transcode")})
	if d.Mode == playback.Direct && s.fitsCap(r, it.SizeBytes, pr.Duration()) {
		info.Type = "direct"
		info.URL = fmt.Sprintf("/api/items/%d/file", it.ID)
		s.Log.InfoContext(r.Context(), "playback started", "cat", "playback", "item", it.ID, "title", it.Title, "mode", "direct", "video", v.CodecName, "height", v.Height)
		writeJSON(w, 200, info)
		return
	}
	qh := sha1.Sum([]byte(fmt.Sprint(q)))
	var vf string
	if v != nil && (v.FieldOrder == "tt" || v.FieldOrder == "bb" || v.FieldOrder == "tb" || v.FieldOrder == "bt") {
		vf = "bwdif=mode=send_frame:parity=auto:deint=interlaced"
	}
	channels := 2
	if info.AudioIndex < len(info.Audio) && info.Audio[info.AudioIndex].Channels > 2 && qBool(r, "surround") && q.CapRate == "" {
		channels = 6
	}
	hid := fmt.Sprintf("v-%s-a%d-s%d-h%d-c%d-%s-%s", hls.Key(it.Path), info.AudioIndex, info.SubIndex, maxHeight, channels, s.audioCodec(r), hex.EncodeToString(qh[:3]))
	audioIdx := info.AudioIndex
	if len(info.Audio) == 0 {
		audioIdx = -1
	}
	s.HLS.Ensure(hls.Spec{
		ID: hid, InputArgs: ffmpeg.FileInput(it.Path), Source: it.Path, Duration: pr.Duration(), SeekMargin: pr.SeekMargin(), VideoFilter: vf,
		AudioIndex: audioIdx, AudioChannels: channels, SubtitleIndex: info.SubIndex, SubtitleText: s.textBurn(info.SubIndex, pr), Quality: q, MaxHeight: maxHeight, AudioCodec: s.audioCodec(r),
	})
	info.Type = "hls"
	info.URL = "/api/hls/" + hid + "/playlist.m3u8"
	reason := "browser cannot play the file as is"
	if !directPlay {
		reason = "direct play is off"
	} else if d.Mode == playback.Direct && q.CapRate != "" {
		reason = "over the limit for viewers away from home"
	}
	s.Log.InfoContext(r.Context(), "playback started", "cat", "playback", "session", hid, "item", it.ID, "title", it.Title, "mode", "transcode", "why", reason,
		"container", ext, "audioTrack", info.AudioIndex, "subtitleTrack", info.SubIndex, "maxHeight", maxHeight)
	writeJSON(w, 200, info)
}

// probeOnce keeps what ffprobe said about the files played last: starting
// FFprobe takes a moment on a small machine, far longer on a sleeping network
// drive, and every change of audio or subtitle track asks again.
func (s *Server) probeOnce(ctx context.Context, path string) (*ffmpeg.Probe, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, errors.New("the file is missing")
	}
	key := fmt.Sprintf("%s|%d|%d", path, st.Size(), st.ModTime().UnixNano())
	s.probes.Lock()
	pr := s.probes.m[key]
	s.probes.Unlock()
	if pr != nil {
		return pr, nil
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pr, err = s.Tools.ProbeFile(pctx, path)
	if err != nil {
		return nil, err
	}
	s.probes.Lock()
	if s.probes.m == nil || len(s.probes.m) >= 64 {
		s.probes.m = map[string]*ffmpeg.Probe{}
	}
	s.probes.m[key] = pr
	s.probes.Unlock()
	return pr, nil
}

// textBurn: this subtitle stream is text, and this FFmpeg can draw text.
func (s *Server) textBurn(n int, pr *ffmpeg.Probe) bool {
	subs := pr.Subtitles()
	return n >= 0 && n < len(subs) && playback.IsTextSubtitle(subs[n].CodecName) && s.Tools.HasFilter("subtitles")
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil || (it.Kind != "video" && it.Kind != "book") {
		writeErr(w, 404, "item not found")
		return
	}
	if !s.Store.LibraryFile(r.Context(), it.Path) {
		writeErr(w, 404, "file missing")
		return
	}
	f, err := os.Open(it.Path)
	if err != nil {
		writeErr(w, 404, "file missing")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		writeErr(w, 404, "file missing")
		return
	}
	// The type comes from this list, never from what the file's first bytes
	// look like: a "movie" that is really a web page stays a download.
	w.Header().Set("Content-Type", mediaType(it.Path))
	if strings.EqualFold(filepath.Ext(it.Path), ".pdf") {
		w.Header().Del("Content-Security-Policy") // the browser's own viewer may show it in a tab; PDFs can't run scripts on this site
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(s.capWriter(w, r), r, "", st.ModTime(), f)
}

var mediaTypes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/mp4", ".mov": "video/quicktime", ".webm": "video/webm", ".mkv": "video/x-matroska",
	".avi": "video/x-msvideo", ".wmv": "video/x-ms-wmv", ".ts": "video/mp2t", ".m2ts": "video/mp2t", ".mts": "video/mp2t", ".flv": "video/x-flv",
	".mpg": "video/mpeg", ".mpeg": "video/mpeg", ".vob": "video/mpeg", ".ogv": "video/ogg", ".3gp": "video/3gpp",
	".pdf": "application/pdf", ".epub": "application/epub+zip", ".cbz": "application/vnd.comicbook+zip", ".cbr": "application/vnd.comicbook-rar",
}

// mediaType is the Content-Type a library file is sent with.
func mediaType(path string) string {
	if ct := mediaTypes[strings.ToLower(filepath.Ext(path))]; ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// ---------------------------------------------------------------------------
// HLS
// ---------------------------------------------------------------------------

func (s *Server) handleHLSPlaylist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.HLS.Get(id)
	if sess == nil {
		writeErr(w, 404, "no such stream (it may have expired; start playback again)")
		return
	}
	sess.See(auth.TokenFrom(r))
	// A stream token doesn't carry over to relative addresses by itself. A player
	// that had to put its sign-in in the playlist's address can't send one with
	// the segments either, so their addresses carry one too: a stream token,
	// never the session's own.
	st := r.URL.Query().Get("st")
	suffix := ""
	if tok := tokenFrom(r); st == "" && tok != "" && tok == r.URL.Query().Get("token") {
		if !auth.IsStreamToken(tok) {
			tok, _ = auth.StreamToken(tok, "/api/hls/"+id+"/", auth.StreamTokenTTL)
		}
		suffix = "?token=" + url.QueryEscape(tok)
	}
	seg := func(n int) string {
		if n < 0 {
			return withStream("seg/init.mp4", st) + suffix
		}
		return withStream(fmt.Sprintf("seg/%d%s", n, sess.SegmentExt()), st) + suffix
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	if sess.Spec.Growing {
		b, err := sess.GrowingPlaylist(r.Context(), seg)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				s.Log.WarnContext(r.Context(), "could not serve a playlist", "cat", "transcode", "session", sess.LogSession(), "stream", id, "err", err)
			}
			writeErr(w, 500, err.Error())
			return
		}
		_, _ = w.Write(b)
		return
	}
	_, _ = w.Write(sess.Playlist(seg))
}

func (s *Server) handleHLSSegment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.HLS.Get(id)
	if sess == nil {
		writeErr(w, 404, "no such stream")
		return
	}
	sess.See(auth.TokenFrom(r))
	name := r.PathValue("n")
	var p string
	var err error
	if name == "init.mp4" {
		p, err = sess.InitSegment(r.Context())
	} else {
		n, nerr := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(name, ".ts"), ".m4s"))
		if nerr != nil {
			writeErr(w, 400, "bad segment")
			return
		}
		p, err = sess.Segment(r.Context(), n)
	}
	if errors.Is(err, hls.ErrBusy) {
		w.Header().Set("Retry-After", "5")
		writeErr(w, 503, err.Error())
		return
	}
	if errors.Is(err, context.Canceled) {
		return // the viewer moved on
	}
	if err != nil {
		s.Log.WarnContext(r.Context(), "could not serve a segment", "cat", "transcode", "session", sess.LogSession(), "stream", id, "segment", name, "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	if sess.Spec.FMP4 {
		w.Header().Set("Content-Type", "video/mp4")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(s.capWriter(w, r), r, p)
}

// DELETE /api/hls/{id}: this viewer is done (another device may still watch).
func (s *Server) handleHLSStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.HLS.Leave(id, auth.TokenFrom(r))
	s.Log.InfoContext(r.Context(), "viewer left a stream", "cat", "playback", "session", id)
	writeJSON(w, 200, map[string]any{"ok": true})
}
