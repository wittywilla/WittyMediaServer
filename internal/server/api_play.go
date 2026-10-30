package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
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
	pr, err := s.Tools.ProbeFile(pctx, it.Path)
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
	for i, a := range pr.Audio() {
		info.Audio = append(info.Audio, trackInfo{Index: i, Language: a.Tags["language"], Title: a.Tags["title"], Codec: a.CodecName, Channels: a.Channels})
	}
	for i, st := range pr.Subtitles() {
		info.Subtitles = append(info.Subtitles, trackInfo{Index: i, Language: st.Tags["language"], Title: st.Tags["title"], Codec: st.CodecName})
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
	browserFriendly := v != nil && (v.CodecName == "h264" || v.CodecName == "vp9" || v.CodecName == "av1") &&
		(ext == ".mp4" || ext == ".m4v" || ext == ".mov" || ext == ".webm")
	audioOK := len(info.Audio) == 0 || info.Audio[0].Codec == "aac" || info.Audio[0].Codec == "mp3" || info.Audio[0].Codec == "opus" || info.Audio[0].Codec == "vorbis"
	if directPlay && browserFriendly && audioOK && info.AudioIndex == 0 && info.SubIndex < 0 && !qBool(r, "transcode") && (maxHeight <= 0 || v.Height <= maxHeight) {
		info.Type = "direct"
		info.URL = fmt.Sprintf("/api/items/%d/file", it.ID)
		s.Log.Info("play", "cat", "playback", "item", it.ID, "title", it.Title, "mode", "direct", "video", v.CodecName, "height", v.Height)
		writeJSON(w, 200, info)
		return
	}
	qh := sha1.Sum([]byte(fmt.Sprint(q)))
	hid := fmt.Sprintf("v-%s-a%d-s%d-h%d-%s-%s", hls.Key(it.Path), info.AudioIndex, info.SubIndex, maxHeight, s.audioCodec(r), hex.EncodeToString(qh[:3]))
	var vf string
	if v != nil && (v.FieldOrder == "tt" || v.FieldOrder == "bb" || v.FieldOrder == "tb" || v.FieldOrder == "bt") {
		vf = "bwdif=mode=send_frame:parity=auto:deint=interlaced"
	}
	channels := 2
	if info.AudioIndex < len(info.Audio) && info.Audio[info.AudioIndex].Channels > 2 && qBool(r, "surround") {
		channels = 6
	}
	audioIdx := info.AudioIndex
	if len(info.Audio) == 0 {
		audioIdx = -1
	}
	s.HLS.Ensure(hls.Spec{
		ID: hid, InputArgs: ffmpeg.FileInput(it.Path), Duration: pr.Duration(), VideoFilter: vf,
		AudioIndex: audioIdx, AudioChannels: channels, SubtitleIndex: info.SubIndex, Quality: q, MaxHeight: maxHeight, AudioCodec: s.audioCodec(r),
	})
	info.Type = "hls"
	info.URL = "/api/hls/" + hid + "/playlist.m3u8"
	reason := "browser cannot play the file as is"
	if !directPlay {
		reason = "direct play is off"
	}
	s.Log.Info("play", "cat", "playback", "item", it.ID, "title", it.Title, "mode", "transcode", "reason", reason,
		"container", ext, "audioTrack", info.AudioIndex, "subtitleTrack", info.SubIndex, "maxHeight", maxHeight)
	writeJSON(w, 200, info)
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
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, "", st.ModTime(), f)
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
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(sess.Playlist(func(n int) string { return fmt.Sprintf("seg/%d.ts", n) }))
}

func (s *Server) handleHLSSegment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.HLS.Get(id)
	if sess == nil {
		writeErr(w, 404, "no such stream")
		return
	}
	n, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("n"), ".ts"))
	if err != nil {
		writeErr(w, 400, "bad segment")
		return
	}
	p, err := sess.Segment(r.Context(), n)
	if errors.Is(err, hls.ErrBusy) {
		w.Header().Set("Retry-After", "5")
		writeErr(w, 503, err.Error())
		return
	}
	if err != nil {
		s.Log.Warn("segment", "id", id, "n", n, "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, p)
}

func (s *Server) handleHLSStop(w http.ResponseWriter, r *http.Request) {
	s.HLS.Stop(r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"ok": true})
}

var _ = library.Item{}
