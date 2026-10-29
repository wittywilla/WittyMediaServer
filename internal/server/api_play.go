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
	return s.DB.GetSetting(r.Context(), "audioCodec", "aac")
}

func (s *Server) prefs(r *http.Request) (hw bool, maxHeight int) {
	hw = s.DB.GetSetting(r.Context(), "hwAccel", "0") == "1"
	maxHeight, _ = strconv.Atoi(s.DB.GetSetting(r.Context(), "maxHeight", "0"))
	if v := qInt(r, "maxHeight", 0); v > 0 {
		maxHeight = v
	}
	return
}

func (s *Server) handlePlayInfo(w http.ResponseWriter, r *http.Request) {
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
	if it.Kind != "video" {
		writeErr(w, 400, "use the dvd endpoints for discs")
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
	hw, maxHeight := s.prefs(r)
	ext := strings.ToLower(filepath.Ext(it.Path))
	browserFriendly := v != nil && (v.CodecName == "h264" || v.CodecName == "vp9" || v.CodecName == "av1") &&
		(ext == ".mp4" || ext == ".m4v" || ext == ".mov" || ext == ".webm")
	audioOK := len(info.Audio) == 0 || info.Audio[0].Codec == "aac" || info.Audio[0].Codec == "mp3" || info.Audio[0].Codec == "opus" || info.Audio[0].Codec == "vorbis"
	if browserFriendly && audioOK && info.AudioIndex == 0 && info.SubIndex < 0 && !qBool(r, "transcode") && (maxHeight == 0 || v.Height <= maxHeight) {
		info.Type = "direct"
		info.URL = fmt.Sprintf("/api/items/%d/file", it.ID)
		writeJSON(w, 200, info)
		return
	}
	h := sha1.Sum([]byte(it.Path))
	hid := fmt.Sprintf("v-%s-a%d-s%d-h%d-%s", hex.EncodeToString(h[:8]), info.AudioIndex, info.SubIndex, maxHeight, s.audioCodec(r))
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
		ID: hid, InputArgs: []string{"-i", it.Path}, Duration: pr.Duration(), VideoFilter: vf,
		AudioIndex: audioIdx, AudioChannels: channels, SubtitleIndex: info.SubIndex, HW: hw, MaxHeight: maxHeight, AudioCodec: s.audioCodec(r),
	})
	info.Type = "hls"
	info.URL = "/api/hls/" + hid + "/playlist.m3u8"
	writeJSON(w, 200, info)
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.Store.Item(r.Context(), id)
	if err != nil || it.Kind != "video" {
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
