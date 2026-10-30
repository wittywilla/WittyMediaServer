package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
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
	return s.DB.GetSetting(r.Context(), "audioCodec", "aac")
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
	if v := qInt(r, "maxHeight", 0); v > 0 {
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
		q.Preset = get("x264Preset", "veryfast")
		q.CRF, _ = strconv.Atoi(get("crf", "21"))
		q.MaxRate = get("maxBitrate", "")
		q.AudioRate = get("audioBitrate", "")
		if get("qualityMode", "") == "expert" {
			q.VideoArgs = ffmpeg.SplitArgs(get("ffmpegVideoArgs", ""))
			q.OutputArgs = ffmpeg.SplitArgs(get("ffmpegOutputArgs", ""))
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
	pctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	pr, err := s.Tools.ProbeFile(pctx, it.Path)
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
	v := pr.Video()
	q, maxHeight := s.quality(r)
	ext := strings.ToLower(filepath.Ext(it.Path))
	directPlay := s.DB.GetSetting(r.Context(), "directPlay", "1") == "1"
	web, _ := playback.Find("web")
	d := playback.Decide(web, probeSource(it, pr), playback.Request{AudioIndex: info.AudioIndex, SubtitleIndex: info.SubIndex, MaxHeight: maxHeight, AllowDirect: directPlay, ForceTranscode: qBool(r, "transcode")})
	if d.Mode == playback.Direct {
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
	f, err := os.Open(it.Path)
	if err != nil {
		writeErr(w, 404, "file missing")
		return
	}
	defer f.Close()
	st, _ := f.Stat()
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, r, filepath.Base(it.Path), st.ModTime(), f)
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
	// A stream token doesn't carry over to relative addresses by itself.
	st := r.URL.Query().Get("st")
	seg := func(n int) string {
		if n < 0 {
			return withStream("seg/init.mp4", st)
		}
		return withStream(fmt.Sprintf("seg/%d%s", n, sess.SegmentExt()), st)
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	if sess.Spec.Growing {
		b, err := sess.GrowingPlaylist(r.Context(), seg)
		if err != nil {
			s.Log.Warn("playlist", "id", id, "err", err)
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
	if err != nil {
		s.Log.Warn("segment", "id", id, "n", name, "err", err)
		writeErr(w, 500, err.Error())
		return
	}
	if sess.Spec.FMP4 {
		w.Header().Set("Content-Type", "video/mp4")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, p)
}

func (s *Server) handleHLSStop(w http.ResponseWriter, r *http.Request) {
	s.HLS.Stop(r.PathValue("id"))
	writeJSON(w, 200, map[string]any{"ok": true})
}

var _ = library.Item{}
