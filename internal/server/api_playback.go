package server

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/internal/playback"
	"github.com/wittywilla/wittymediaserver/internal/subtitle"
)

// One door for playing anything: POST /api/items/{id}/playback. The app says
// what it can play, the server decides and answers with ready-to-use addresses
// (carrying a stream token), the tracks, where to start and what comes next.
// See docs/CLIENT-API.md.

type playbackRequest struct {
	Profile      string                 `json:"profile"`
	Capabilities *playback.Capabilities `json:"capabilities"`
	// Tracks; left out, the server picks them (see playback.ChooseTracks).
	Audio            *int   `json:"audio"`
	Subtitle         *int   `json:"subtitle"`
	AudioLanguage    string `json:"audioLanguage"`
	SubtitleLanguage string `json:"subtitleLanguage"`
	// StartSec: where to start; left out, from the viewer's saved position.
	StartSec       *float64 `json:"startSec"`
	MaxHeight      int      `json:"maxHeight"`
	MaxBitrateKbps int      `json:"maxBitrateKbps"`
	ForceTranscode bool     `json:"forceTranscode"`
	BurnSubtitle   bool     `json:"burnSubtitle"`
	// Video: which video stream of a file with several (see videos in the answer).
	Video *int `json:"video"`
	// Flatten3D shows one eye of a 3D picture, for a screen that isn't 3D.
	Flatten3D bool `json:"flatten3d"`
	// Streaming is this device's own choice of streaming mode: standard or robust. Left
	// out (or "auto"), the server's setting decides.
	Streaming string `json:"streaming"`
	// MeasuredKbps is what this device's connection carried when it last measured it
	// (GET /playback/speedtest), for a robust stream to start at a rate that fits.
	MeasuredKbps int `json:"measuredKbps"`
}

// playbackVideoStream is one picture of a file with several.
type playbackVideoStream struct {
	Index  int    `json:"index"`
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Title  string `json:"title,omitempty"`
}

type playbackAudio struct {
	Index    int    `json:"index"`
	Language string `json:"language"`
	// LanguageName is the language in English words, "" when not known.
	LanguageName string `json:"languageName,omitempty"`
	Title        string `json:"title,omitempty"`
	Codec        string `json:"codec"`
	Channels     int    `json:"channels,omitempty"`
	Default      bool   `json:"default,omitempty"`
	Commentary   bool   `json:"commentary,omitempty"`
}

type playbackSubtitle struct {
	Index        int    `json:"index"`
	Language     string `json:"language"`
	LanguageName string `json:"languageName,omitempty"`
	Title        string `json:"title,omitempty"`
	Codec        string `json:"codec"`
	Default      bool   `json:"default,omitempty"`
	Forced       bool   `json:"forced,omitempty"`
	SDH          bool   `json:"sdh,omitempty"`
	Text         bool   `json:"text"`
	// Delivery: how this track would reach the screen if chosen now.
	Delivery string `json:"delivery"`
	// URL is the WebVTT file (text tracks only).
	URL string `json:"url,omitempty"`
}

type playbackIntro struct {
	StartSec float64 `json:"startSec"`
	EndSec   float64 `json:"endSec"`
}

// playbackDisc tells a client how to start a disc.
type playbackDisc struct {
	Type         string `json:"type"` // dvd | bluray
	StartMode    string `json:"startMode"`
	InsertEffect bool   `json:"insertEffect"`
	InsertSound  bool   `json:"insertSound"`
	HasMenus     bool   `json:"hasMenus"`
	MainTitle    int    `json:"mainTitle"`
	Scrambled    bool   `json:"scrambled"`
	OpenURL      string `json:"openUrl"`
	ResumeTitle  int    `json:"resumeTitle,omitempty"`
}

type playbackResponse struct {
	ItemID int64 `json:"itemId"`
	// Decision: direct | remux | transcode | disc | audio-direct | audio-transcode | radio | cd.
	Decision string   `json:"decision"`
	Reasons  []string `json:"reasons"`
	// Protocol: file (a plain file with byte ranges) | hls | adts | disc-session |
	// stream (an endless audio stream, for radio).
	Protocol string `json:"protocol"`
	URL      string `json:"url,omitempty"`
	// StreamToken is already in URL and in the track addresses; add it as
	// ?st= to any other address of this item (Presentation media, artwork).
	StreamToken  string `json:"streamToken"`
	TokenExpires int    `json:"tokenExpiresInSec"`
	// StreamID names the HLS stream: DELETE /api/hls/{id} when finished.
	StreamID    string  `json:"streamId,omitempty"`
	DurationSec float64 `json:"durationSec"`
	Container   string  `json:"container,omitempty"`
	// Seek: native (seek in the player, anywhere) | restart (the stream is a
	// growing remux: past what has been made, ask again with startSec) | none (live radio).
	Seek string `json:"seek"`
	// StartAtSec: seek the player here before it starts. TimeOffsetSec: add
	// this to the player's clock to get the position in the film.
	StartAtSec    float64            `json:"startAtSec"`
	TimeOffsetSec float64            `json:"timeOffsetSec"`
	ResumeSec     float64            `json:"resumeSec"`
	Audio         []playbackAudio    `json:"audio"`
	Subtitles     []playbackSubtitle `json:"subtitles"`
	AudioIndex    int                `json:"audioIndex"`
	SubtitleIndex int                `json:"subtitleIndex"`
	// SubtitleDelivery: none | embedded | sidecar | hls | burn.
	SubtitleDelivery string         `json:"subtitleDelivery"`
	Intro            *playbackIntro `json:"intro,omitempty"`
	NextID           *int64         `json:"nextId,omitempty"`
	Disc             *playbackDisc  `json:"disc,omitempty"`
	// NowPlayingURL: radio only. What the station says it is playing right now.
	NowPlayingURL string `json:"nowPlayingUrl,omitempty"`
	// CDURL: for a CD (decision cd), its tracks and layout: GET it for the map of
	// the stream in URL.
	CDURL       string `json:"cdUrl,omitempty"`
	ProgressURL string `json:"progressUrl"`
	// Chapters are named points in a video, from the file.
	Chapters []chapter `json:"chapters,omitempty"`
	// Explain says in words how it plays and why, for the player's information.
	Explain *playback.Explanation `json:"explain,omitempty"`
	// Videos: a file that holds several pictures lists them; VideoIndex is the one playing.
	Videos     []playbackVideoStream `json:"videos,omitempty"`
	VideoIndex int                   `json:"videoIndex,omitempty"`
	// Stereo says a 3D picture is stored in the frame (sbs | tab), so that the player can
	// offer to show it in 2D.
	Stereo    string `json:"stereo,omitempty"`
	Flattened bool   `json:"flattened,omitempty"`
	// Streaming says how a video is streamed (standard or robust) and what the player
	// should do for that. Videos only.
	Streaming *playbackStreaming `json:"streaming,omitempty"`
}

// probeSource turns a probe into what the decision needs. The container comes from what
// the file really is, not what it's called. Subtitle files beside it follow its own
// subtitle tracks in one list, and their place in that list is their number.
func probeSource(it *library.Item, pr *ffmpeg.Probe) playback.Source {
	src := playback.Source{}
	vcodec, acodec := "", ""
	if v := pr.Video(); v != nil {
		vcodec = v.CodecName
		src.HasVideo = true
		src.VideoCodec = v.CodecName
		src.VideoTag = v.CodecTag
		src.Height = v.Height
		src.BitDepth = v.Depth()
		src.Range = v.Range()
		src.Dynamic = v.DynamicRange()
		src.Interlaced = v.IsInterlaced()
		src.Stereo = v.StereoMode(it.Path)
		src.BitrateKbps = kbps(v.BitRate)
		if src.BitrateKbps == 0 {
			src.BitrateKbps = kbps(pr.Format.BitRate)
		}
	}
	for _, a := range pr.Audio() {
		if acodec == "" {
			acodec = a.CodecName
		}
		title := strings.ToLower(a.Tags["title"])
		src.Audio = append(src.Audio, playback.Audio{Codec: a.CodecName, Channels: a.Channels, Language: a.Tags["language"], Title: a.Tags["title"],
			Default: a.Disposition["default"] == 1, Comment: a.Disposition["comment"] == 1 || strings.Contains(title, "commentary")})
	}
	src.Container = mediatypes.Canonical(pr.Format.FormatName, it.Path, vcodec, acodec)
	for _, st := range pr.Subtitles() {
		title := strings.ToLower(st.Tags["title"])
		src.Subtitles = append(src.Subtitles, playback.Subtitle{Codec: st.CodecName, Language: st.Tags["language"], Title: st.Tags["title"],
			Default: st.Disposition["default"] == 1, Forced: st.Disposition["forced"] == 1 || strings.Contains(title, "forced"),
			SDH: st.Disposition["hearing_impaired"] == 1 || strings.Contains(title, "sdh")})
	}
	for _, f := range library.FindSubtitles(it.Path) {
		src.Subtitles = append(src.Subtitles, playback.Subtitle{Codec: f.Codec, Language: f.Language, Title: f.Title, Default: f.Default, Forced: f.Forced, SDH: f.SDH, File: f.Path, Format: f.Format})
	}
	return src
}

func kbps(s string) int {
	n, _ := strconv.Atoi(s)
	return n / 1000
}

// variantInfo fills in the master playlist's description of a stream.
// RFC 6381 names are only given when known for certain: a wrong one makes
// players refuse a stream that would have played.
func variantInfo(it *library.Item, pr *ffmpeg.Probe, d playback.Decision, src playback.Source, audio int) *hls.Variant {
	v := pr.Video()
	out := &hls.Variant{BandwidthKbps: src.BitrateKbps}
	if v != nil {
		out.Width, out.Height = v.Width, v.Height
	}
	if d.Mode == playback.Transcode {
		// H.264 High at a level that covers what is being made: encoders may
		// stay below that, and claiming a little more is safe.
		out.Range = "SDR"
		out.Codecs = "avc1.640028"
		if out.Height > 1080 {
			out.Codecs = "avc1.640033"
		}
		if audio >= 0 {
			out.Codecs += ",mp4a.40.2"
		}
		if out.BandwidthKbps == 0 || out.BandwidthKbps > 20000 {
			out.BandwidthKbps = 12000
		}
		return out
	}
	switch src.Range {
	case "hdr10", "dv5":
		out.Range = "PQ"
	case "hlg":
		out.Range = "HLG"
	default:
		out.Range = "SDR"
	}
	vc := videoCodecString(v)
	ac := ""
	if audio >= 0 && audio < len(src.Audio) {
		switch {
		case !d.CopyAudio || src.Audio[audio].Codec == "aac":
			ac = "mp4a.40.2"
		case src.Audio[audio].Codec == "ac3":
			ac = "ac-3"
		case src.Audio[audio].Codec == "eac3":
			ac = "ec-3"
		case src.Audio[audio].Codec == "mp3":
			ac = "mp4a.40.34"
		}
		if ac == "" {
			vc = "" // unknown sound: name nothing rather than half
		}
	}
	if vc != "" {
		out.Codecs = vc
		if ac != "" {
			out.Codecs += "," + ac
		}
	}
	return out
}

func videoCodecString(v *ffmpeg.Stream) string {
	if v == nil || v.Level <= 0 {
		return ""
	}
	switch v.CodecName {
	case "h264":
		var profile, constraint int
		switch v.Profile {
		case "Constrained Baseline":
			profile, constraint = 0x42, 0x40
		case "Baseline":
			profile = 0x42
		case "Main":
			profile = 0x4d
		case "High":
			profile = 0x64
		default:
			return ""
		}
		return fmt.Sprintf("avc1.%02x%02x%02x", profile, constraint, v.Level)
	case "hevc":
		switch v.Profile {
		case "Main":
			return fmt.Sprintf("hvc1.1.6.L%d.B0", v.Level)
		case "Main 10":
			return fmt.Sprintf("hvc1.2.4.L%d.B0", v.Level)
		}
	}
	return ""
}

// POST /api/items/{id}/playback
func (s *Server) handlePlayback(w http.ResponseWriter, r *http.Request) {
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
	var b playbackRequest
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	p, ok := playback.Find(b.Profile)
	if !ok {
		writeErr(w, 400, "unknown profile: GET /api/playback/profiles lists them")
		return
	}
	p = b.Capabilities.Apply(p)
	if b.MaxHeight < 0 || b.MaxBitrateKbps < 0 || b.MeasuredKbps < 0 {
		writeErr(w, 400, "maxHeight, maxBitrateKbps and measuredKbps can't be negative")
		return
	}
	if b.Streaming == "auto" {
		b.Streaming = ""
	}
	if b.Streaming != "" && !playback.ValidStreamMode(b.Streaming) {
		writeErr(w, 400, "streaming should be standard, robust or auto")
		return
	}
	b.MeasuredKbps = min(b.MeasuredKbps, 10000000)
	if b.StartSec != nil && (*b.StartSec < 0 || math.IsNaN(*b.StartSec) || math.IsInf(*b.StartSec, 0)) {
		writeErr(w, 400, "startSec can't be negative")
		return
	}
	_ = s.Store.AttachProgress(r.Context(), userFrom(r).ID, []*library.Item{it})
	tok, exp, err := s.stream.mint(s.sessionToken(r, userFrom(r)), userFrom(r).ID, it)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := playbackResponse{ItemID: it.ID, Reasons: []string{}, StreamToken: tok, TokenExpires: int(time.Until(exp).Seconds()), Seek: "native",
		Audio: []playbackAudio{}, Subtitles: []playbackSubtitle{}, SubtitleIndex: -1, SubtitleDelivery: playback.SubNone,
		ProgressURL: fmt.Sprintf("/api/items/%d/progress", it.ID), DurationSec: float64(it.RuntimeSec), Container: it.Container,
		NextID: s.Store.NextEpisode(r.Context(), it, s.access(r))}
	if it.Progress != nil {
		out.ResumeSec = it.Progress.ResumeSec
	}
	switch {
	case isDiscKind(it):
		s.playbackDisc(r, it, &out)
	case it.Kind == "audio":
		s.playbackAudio(r, it, p, b, &out)
	case it.Kind == "radio":
		s.playbackRadio(r, it, &out)
	case it.Kind == library.CDKind:
		s.playbackCD(it, p, &out)
	case it.Kind == "video":
		if err := s.playbackVideo(r, it, p, b, &out); err != nil {
			code := 500
			if errors.As(err, new(errBadTrack)) {
				code = 400
			} else if errors.Is(err, library.ErrNotFound) {
				code = 404
			}
			writeErr(w, code, s.viewerError(r, err))
			return
		}
	default:
		writeErr(w, 400, "this kind of item isn't played: books open with GET /api/items/{id}/book")
		return
	}
	s.streams.note(r, it, out.Decision, out.Reasons, out.StreamID)
	writeJSON(w, 200, out)
}

// playbackCD answers with the whole disc as one stream: the format the player
// names that needs least from it, FLAC, then plain samples, then AAC.
func (s *Server) playbackCD(it *library.Item, p playback.Profile, out *playbackResponse) {
	format, seek := "aac", "restart"
	switch {
	case slices.Contains(p.AudioCodecs, "flac"):
		format = "flac"
	case slices.Contains(p.AudioCodecs, "pcm_s16le"):
		format, seek = "wav", "native"
	}
	out.Decision, out.Protocol, out.Seek = "cd", "file", seek
	out.CDURL = withStream(fmt.Sprintf("/api/cd/%d", it.ID), out.StreamToken)
	out.URL = withStream(fmt.Sprintf("/api/cd/%d/audio?format=%s", it.ID, format), out.StreamToken)
}

func (s *Server) playbackDisc(r *http.Request, it *library.Item, out *playbackResponse) {
	s.fillDiscStart(r.Context(), it)
	d := &playbackDisc{Type: "dvd", StartMode: it.StartMode, InsertEffect: it.InsertEffect != nil && *it.InsertEffect,
		InsertSound: s.DB.GetSetting(r.Context(), "insertSound", "1") == "1", OpenURL: fmt.Sprintf("/api/items/%d/dvd", it.ID)}
	if isBluray(it) {
		d.Type = "bluray"
	}
	if it.Disc != nil {
		d.HasMenus, d.MainTitle, d.Scrambled = it.Disc.HasMenus, it.Disc.MainTitle, it.Disc.Scrambled
	}
	if it.Progress != nil && it.Progress.DiscTitle != nil {
		d.ResumeTitle = *it.Progress.DiscTitle
	}
	out.Decision, out.Protocol, out.Disc = "disc", "disc-session", d
	// The disc's own session starts when the client opens it (handleDVDOpen).
	s.Log.InfoContext(r.Context(), "playback decided", "cat", "playback", "item", it.ID, "title", it.Title, "mode", out.Decision, "format", d.Type)
}

func (s *Server) playbackAudio(r *http.Request, it *library.Item, p playback.Profile, b playbackRequest, out *playbackResponse) {
	out.Protocol = "file"
	out.Decision = "audio-direct"
	// Audiobooks and podcasts carry on from any position; songs start over.
	out.ResumeSec = 0
	if lib, err := s.Store.Library(r.Context(), it.LibraryID); err == nil && lib.Type != "music" && it.Progress != nil && !it.Progress.Watched {
		out.ResumeSec = it.Progress.PositionSec
	}
	if b.StartSec == nil && out.ResumeSec > 0 {
		b.StartSec = &out.ResumeSec
	}
	u := fmt.Sprintf("/api/items/%d/stream", it.ID)
	container := mediatypes.Canonical(it.Container, it.Path, "", it.VideoCodec)
	// VideoCodec holds the sound's codec for audio items. A client that doesn't list a
	// container is judged by the codec alone, as apps that open anything do.
	direct := library.PlaysDirect(it) && slices.Contains(p.AudioCodecs, it.VideoCodec)
	switch {
	case it.VideoCodec == "midi":
		// Made once on the server, and then a plain FLAC file: seeking just works.
		out.Decision = "audio-transcode"
		out.Reasons = append(out.Reasons, playback.ReasonAudioCodec)
		if b.StartSec != nil {
			out.StartAtSec = *b.StartSec
		}
	case !direct || b.ForceTranscode:
		u += "?transcode=1"
		out.Decision, out.Protocol, out.Seek = "audio-transcode", "adts", "restart"
		out.Reasons = append(out.Reasons, playback.ReasonAudioCodec)
		if b.StartSec != nil && *b.StartSec > 0 {
			u += "&start=" + strconv.FormatFloat(*b.StartSec, 'f', 3, 64)
			out.TimeOffsetSec = *b.StartSec
		}
	default:
		if b.StartSec != nil {
			out.StartAtSec = *b.StartSec
		}
	}
	out.URL = withStream(u, out.StreamToken)
	ex := playback.ExplainAudio(out.Decision == "audio-direct", container, it.VideoCodec)
	out.Explain = &ex
	s.Log.InfoContext(r.Context(), "playback started", "cat", "playback", "item", it.ID, "title", it.Title, "profile", p.ID, "mode", out.Decision, "codec", it.VideoCodec)
}

// playbackVideo decides how to play a video file and sets the stream up.
func (s *Server) playbackVideo(r *http.Request, it *library.Item, p playback.Profile, b playbackRequest, out *playbackResponse) error {
	ctx := r.Context()
	// FFmpeg reads what a link swapped in after the scan points at, if it gets the chance.
	if !s.Store.LibraryFile(ctx, it.Path) {
		return library.ErrNotFound
	}
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pr, err := s.probeOnce(pctx, it.Path)
	if err != nil {
		return err
	}
	src := probeSource(it, pr)
	parts := library.StackedParts(it.Path)
	src.Parts = len(parts)
	chosen := playback.ChooseTracks(src, firstNonEmpty(b.AudioLanguage, s.DB.GetSetting(ctx, "audioLanguage", "")), firstNonEmpty(b.SubtitleLanguage, s.DB.GetSetting(ctx, "subtitleLanguage", "")))
	if b.Audio != nil {
		chosen.Audio = *b.Audio
	}
	if b.Subtitle != nil {
		chosen.Subtitle = *b.Subtitle
	}
	if chosen.Audio < 0 || (chosen.Audio > 0 && chosen.Audio >= len(src.Audio)) {
		return errBadTrack{"no such audio track"}
	}
	if chosen.Subtitle < -1 || chosen.Subtitle >= len(src.Subtitles) {
		return errBadTrack{"no such subtitle track"}
	}
	pictures := pr.VideoStreams()
	videoPick := 0
	if b.Video != nil {
		if *b.Video < 0 || *b.Video >= max(len(pictures), 1) {
			return errBadTrack{"no such video stream"}
		}
		videoPick = *b.Video
	}
	q, serverMax := s.quality(r)
	mode, modeFrom := s.streamMode(r, b.Streaming)
	robust := mode == playback.StreamRobust
	req := playback.Request{AudioIndex: chosen.Audio, SubtitleIndex: chosen.Subtitle, MaxBitrateKbps: b.MaxBitrateKbps, BurnSubtitle: b.BurnSubtitle,
		AllowDirect: s.DB.GetSetting(ctx, "directPlay", "1") == "1", ForceTranscode: b.ForceTranscode, MaxHeight: playback.Smallest(b.MaxHeight, serverMax),
		VideoIndex: videoPick, Flatten3D: b.Flatten3D, OverLimit: !s.fitsCap(r, it.SizeBytes, pr.Duration()),
		Robust: robust && src.HasVideo && !playback.FitsRobust(src.BitrateKbps, b.MeasuredKbps)}
	d := playback.Decide(p, src, req)
	out.Streaming = &playbackStreaming{Mode: mode, From: modeFrom, MeasuredKbps: b.MeasuredKbps, Player: playback.Hints(mode)}

	out.DurationSec = pr.Duration()
	if len(parts) > 1 || out.DurationSec <= 0 {
		out.DurationSec = it.RuntimeSec // the scan worked it out, from every part
	}
	out.Decision, out.Reasons, out.SubtitleDelivery = d.Mode, d.Reasons, d.Subtitle
	out.AudioIndex, out.SubtitleIndex = chosen.Audio, chosen.Subtitle
	out.VideoIndex, out.Stereo = videoPick, src.Stereo
	out.Flattened = b.Flatten3D && src.Stereo != ""
	ex := playback.Explain(d, src, req)
	// The web app follows the sound format chosen in Settings. Apps ask by what they play.
	audioCodec := "aac"
	if p.ID == "web" {
		audioCodec = s.audioCodec(r)
		if audioCodec == "mp3" {
			ex.Sound = strings.Replace(ex.Sound, "AAC", "MP3", 1)
		}
	}
	out.Explain = &ex
	ex.Streaming = playback.ExplainStreaming(mode, modeFrom, nil, b.MeasuredKbps)
	if len(pictures) > 1 {
		for i, v := range pictures {
			out.Videos = append(out.Videos, playbackVideoStream{Index: i, Codec: v.CodecName, Width: v.Width, Height: v.Height, Title: v.Tags["title"]})
		}
	}
	for _, c := range pr.Chapters {
		if at, err := strconv.ParseFloat(c.Start, 64); err == nil {
			out.Chapters = append(out.Chapters, chapter{Start: at, Title: c.Tags["title"]})
		}
	}
	for i, a := range src.Audio {
		out.Audio = append(out.Audio, playbackAudio{Index: i, Language: a.Language, LanguageName: playback.LanguageName(a.Language), Title: a.Title, Codec: a.Codec, Channels: a.Channels, Default: a.Default, Commentary: a.Comment})
	}
	start := out.ResumeSec
	if b.StartSec != nil {
		start = *b.StartSec
	}
	if start > out.DurationSec && out.DurationSec > 0 {
		start = 0
	}
	if it.IntroEnd == nil && it.ShowID != nil {
		if sh, err := s.Store.Show(ctx, *it.ShowID, 0); err == nil {
			it.IntroStart, it.IntroEnd = sh.IntroStart, sh.IntroEnd
		}
	}
	if it.IntroEnd != nil {
		in := playbackIntro{EndSec: *it.IntroEnd}
		if it.IntroStart != nil {
			in.StartSec = *it.IntroStart
		}
		out.Intro = &in
	}
	logStarted := func(session string) {
		s.Log.InfoContext(ctx, "playback started", "cat", "playback", "session", session, "item", it.ID, "title", it.Title, "profile", p.ID, "mode", d.Mode, "reasons", strings.Join(d.Reasons, ","), "audioTrack", chosen.Audio, "subtitleTrack", chosen.Subtitle,
			"streaming", mode, "streamingFrom", modeFrom, "measuredKbps", b.MeasuredKbps, "ladder", ladderParam(out.Streaming.Ladder))
	}

	token := out.StreamToken
	subURL := func(i int) string {
		u := fmt.Sprintf("/api/items/%d/subtitles/%d.vtt", it.ID, i)
		if out.TimeOffsetSec > 0 { // the player's clock starts this far into the film
			u += "?offset=" + strconv.FormatFloat(out.TimeOffsetSec, 'f', 3, 64)
		}
		return withStream(u, token)
	}
	if d.Mode == playback.Direct {
		logStarted("") // nothing runs here, so only the request ties to it
		out.Protocol = "file"
		out.StartAtSec = start
		out.URL = withStream(fmt.Sprintf("/api/items/%d/file", it.ID), token)
		fillSubtitles(out, src, p, true, subURL)
		if d.Subtitle == playback.SubSidecar {
			s.warmSubtitle("", it.Path, chosen.Subtitle, src)
		}
		return nil
	}

	// HLS: the same stream is shared by viewers who get the same thing.
	vf := ""
	if src.HasVideo && !d.CopyVideo {
		if src.Interlaced {
			vf = "bwdif=mode=send_frame:parity=auto:deint=interlaced"
		}
		if out.Flattened {
			if v := pr.Video(); v != nil {
				vf = strings.Trim(vf+","+ffmpeg.FlattenFilter(src.Stereo, v.Width, v.Height), ",")
			}
		}
		if d.ToneMap {
			if tm := s.Tools.ToneMapFilter(src.Range); tm != "" {
				vf = strings.Trim(vf+","+tm, ",")
			}
		}
	}
	audioIdx := chosen.Audio
	if len(src.Audio) == 0 {
		audioIdx = -1
	}
	channels := 2
	if d.AudioChannels > 2 {
		channels = 6
	}
	burn := -1
	if d.Subtitle == playback.SubBurn {
		burn = chosen.Subtitle
	}
	input := it.Path
	if abs, err := filepath.Abs(input); err == nil {
		input = abs // fMP4 streams run FFmpeg in another folder
	}
	inputArgs := ffmpeg.FileInput(input)
	if len(parts) > 1 {
		h := sha1.Sum([]byte(it.Path))
		if inputArgs, err = ffmpeg.PartsInput(filepath.Join(s.Cfg.CacheDir, "lists"), "parts-"+hex.EncodeToString(h[:8]), parts); err != nil {
			return err
		}
	}
	spec := hls.Spec{
		ItemID:    it.ID,
		InputArgs: inputArgs, Source: input, Duration: out.DurationSec, SeekMargin: pr.SeekMargin(), VideoFilter: vf, AudioIndex: audioIdx, AudioChannels: channels,
		SubtitleIndex: burn, Quality: q, MaxHeight: req.MaxHeight, AudioCodec: audioCodec, VideoIndex: pr.VideoIndex() + videoPick, NoVideo: !src.HasVideo,
	}
	if burn >= 0 {
		if st := src.Subtitles[burn]; st.File != "" {
			file, err := s.subtitleForBurning(ctx, st)
			if err != nil {
				return err
			}
			spec.SubtitleFile, spec.SubtitleText = file, playback.IsTextSubtitle(st.Codec)
			if spec.SubtitleText && !s.Tools.HasFilter("subtitles") {
				spec.SubtitleText = false
			}
		} else {
			spec.SubtitleText = s.textBurn(burn, pr)
		}
	}
	spec.MaxHeight = playback.Smallest(spec.MaxHeight, p.MaxHeight)
	if d.CopyVideo {
		spec.CopyVideo, spec.CopyAudio, spec.FMP4, spec.Growing = true, d.CopyAudio, d.FMP4, true
		if d.FMP4 && src.VideoCodec == "hevc" {
			spec.VideoTag = "hvc1"
		}
		if start > 0 && len(parts) == 1 {
			// A copy can only start at a keyframe. Starting at exactly that one means
			// the player's clock is right, whatever the distance between keyframes.
			kctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if k, ok := s.Tools.KeyframeNear(kctx, input, spec.VideoIndex, start); ok {
				start = k
			}
			cancel()
		}
		spec.StartSec = start
		out.TimeOffsetSec = start
		out.Seek = "restart"
	} else {
		out.StartAtSec = start
	}
	v := variantInfo(it, pr, d, src, audioIdx)
	if p.HLSSubtitles {
		for i, st := range src.Subtitles {
			if !playback.IsTextSubtitle(st.Codec) {
				continue
			}
			name := firstNonEmpty(st.Title, playback.LanguageName(st.Language), st.Language, "Subtitles")
			v.Subtitles = append(v.Subtitles, hls.Rendition{Stream: i, File: st.File, Format: st.Format, Name: name, Language: playback.NormalizeLanguage(st.Language),
				Default: i == chosen.Subtitle && d.Subtitle == playback.SubHLS, Forced: st.Forced})
		}
	}
	spec.Variant = v
	sum := sha1.Sum([]byte(fmt.Sprint(p.ID, d, chosen, spec.MaxHeight, q, start*boolFloat(spec.Growing), vf, len(v.Subtitles), v.Codecs, spec.VideoIndex, spec.SubtitleFile)))
	spec.ID = fmt.Sprintf("v-%s-n%s", hls.Key(it.Path), hex.EncodeToString(sum[:6]))
	out.Protocol = "hls"
	out.Streaming.SegmentSec = playback.StandardSegmentSec
	if robust && src.HasVideo && !d.CopyVideo {
		// A ladder of steady rates, one stream per rung, made only for the rungs a player
		// asks for. Its master playlist lists them in the order of the ladder parameter.
		lad := playback.RobustLadder(src.Height, spec.MaxHeight, s.robustCapKbps(r, b.MaxBitrateKbps), b.MeasuredKbps)
		order := append([]playback.Rung{lad.Rungs[lad.Start]}, slices.Delete(slices.Clone(lad.Rungs), lad.Start, lad.Start+1)...)
		group := fmt.Sprintf("v-%s-r%s", hls.Key(it.Path), hex.EncodeToString(sum[:6]))
		specs := ladderSpecs(spec, group, order, v.Width, v.Height)
		for _, sp := range specs {
			s.HLS.Ensure(sp)
		}
		out.Streaming.SegmentSec, out.Streaming.Ladder = playback.RobustSegmentSec, order
		logStarted(specs[0].ID)
		out.StreamID = specs[0].ID
		out.URL = withStream("/api/hls/"+specs[0].ID+"/master.m3u8?ladder="+ladderParam(order), token)
	} else {
		s.HLS.Ensure(spec)
		logStarted(spec.ID)
		out.StreamID = spec.ID
		if len(v.Subtitles) > 0 {
			out.URL = withStream("/api/hls/"+spec.ID+"/master.m3u8", token)
		} else {
			out.URL = withStream("/api/hls/"+spec.ID+"/playlist.m3u8", token)
		}
	}
	ex.Streaming = playback.ExplainStreaming(mode, modeFrom, out.Streaming.Ladder, b.MeasuredKbps)
	fillSubtitles(out, src, p, false, subURL)
	if d.Subtitle == playback.SubHLS || d.Subtitle == playback.SubSidecar {
		s.warmSubtitle(spec.ID, it.Path, chosen.Subtitle, src)
	}
	return nil
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

type errBadTrack struct{ msg string }

func (e errBadTrack) Error() string { return e.msg }

// fillSubtitles lists the subtitle tracks and how each would be delivered.
func fillSubtitles(out *playbackResponse, src playback.Source, p playback.Profile, direct bool, url func(int) string) {
	for i, st := range src.Subtitles {
		t := playbackSubtitle{Index: i, Language: st.Language, LanguageName: playback.LanguageName(st.Language), Title: st.Title, Codec: st.Codec, Default: st.Default, Forced: st.Forced, SDH: st.SDH, Text: playback.IsTextSubtitle(st.Codec)}
		switch {
		case direct && slices.Contains(p.EmbeddedSubtitles, st.Codec):
			t.Delivery = playback.SubEmbedded
		case !direct && t.Text && p.HLSSubtitles:
			t.Delivery = playback.SubHLS
		case t.Text && p.SidecarSubtitles:
			t.Delivery = playback.SubSidecar
		case direct && t.Text && p.HLSSubtitles:
			t.Delivery = playback.SubHLS // asks again: the answer changes to an HLS stream
		default:
			t.Delivery = playback.SubBurn
		}
		if t.Text {
			t.URL = url(i)
		}
		out.Subtitles = append(out.Subtitles, t)
	}
}

// ---------------------------------------------------------------------------
// Subtitles as WebVTT
// ---------------------------------------------------------------------------

// subtitleFiles converts text subtitles to WebVTT once and keeps the result in
// the cache. Reading a subtitle track means reading the whole file, so at most
// two run at a time, each track at most once, and a client that gives up
// doesn't waste the work.
type subtitleFiles struct {
	mu      sync.Mutex
	running map[string]chan struct{}
}

var subtitleSlots = make(chan struct{}, 2)

func (s *Server) subtitlePath(path string, n int) string {
	var stamp int64
	if st, err := os.Stat(path); err == nil {
		stamp = st.ModTime().Unix()
	}
	h := sha1.Sum([]byte(fmt.Sprint(path, "|", stamp)))
	return filepath.Join(s.Cfg.CacheDir, "subtitles", fmt.Sprintf("%s-%d.vtt", hex.EncodeToString(h[:10]), n))
}

// subtitleVTT returns the WebVTT file of a text subtitle, making it if needed.
func (s *Server) subtitleVTT(ctx context.Context, path string, n int) (string, error) {
	out := s.subtitlePath(path, n)
	if st, err := os.Stat(out); err == nil && st.Size() > 0 {
		return out, nil
	}
	sf := &s.subtitles
	sf.mu.Lock()
	if sf.running == nil {
		sf.running = map[string]chan struct{}{}
	}
	done, busy := sf.running[out]
	if !busy {
		done = make(chan struct{})
		sf.running[out] = done
		session := logging.SessionFrom(ctx)
		logging.Go("subtitle conversion", func() {
			defer func() {
				sf.mu.Lock()
				delete(sf.running, out)
				sf.mu.Unlock()
				close(done)
			}()
			subtitleSlots <- struct{}{}
			defer func() { <-subtitleSlots }()
			if err := s.convertSubtitle(session, path, n, out); err != nil {
				s.Log.Warn("could not convert a subtitle track", "cat", "transcode", "session", session, "file", path, "track", n, "err", err)
			}
		}, "cat", "transcode", "session", session)
	}
	sf.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return "", fmt.Errorf("this subtitle track can't be converted")
	}
	return out, nil
}

func (s *Server) convertSubtitle(session, path string, n int, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(logging.WithSession(context.Background(), session), 15*time.Minute)
	defer cancel()
	tmp := out + ".tmp"
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	args = append(args, ffmpeg.FileInput(path)...)
	args = append(args, "-map", fmt.Sprintf("0:s:%d", n), "-vn", "-an", "-c:s", "webvtt", "-f", "webvtt", tmp)
	if _, stderr, err := ffmpeg.Run(ctx, s.Tools.FFmpeg, args, "track", n); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: %s", err, stderr.Last(400))
	}
	return os.Rename(tmp, out)
}

// warmSubtitle starts converting the chosen track, so it is ready soon after the
// picture starts.
func (s *Server) warmSubtitle(session, path string, n int, src playback.Source) {
	if n < 0 || s.Cfg == nil || n >= len(src.Subtitles) {
		return
	}
	st := src.Subtitles[n]
	logging.Go("subtitle warm-up", func() {
		ctx, cancel := context.WithTimeout(logging.WithSession(context.Background(), session), 15*time.Minute)
		defer cancel()
		if st.File != "" {
			_, _ = s.subtitleFileVTT(ctx, st.File, st.Format)
			return
		}
		_, _ = s.subtitleVTT(ctx, path, n) // nobody is waiting yet; subtitleVTT logs a failure
	}, "cat", "transcode", "session", session)
}

// subtitleFileVTT makes WebVTT of a subtitle file beside a video, once. What it can read
// itself it does; the rest goes through FFmpeg, after its encoding is straightened out,
// which an old file from anywhere but America usually needs.
func (s *Server) subtitleFileVTT(ctx context.Context, file, format string) (string, error) {
	st, err := os.Stat(file)
	if err != nil || !st.Mode().IsRegular() {
		return "", errors.New("the subtitle file is missing")
	}
	if st.Size() > subtitle.MaxInput {
		return "", errors.New("this subtitle file is too large")
	}
	h := sha1.Sum([]byte(fmt.Sprint(file, "|", st.Size(), "|", st.ModTime().UnixNano())))
	dir := filepath.Join(s.Cfg.CacheDir, "subtitles")
	out := filepath.Join(dir, hex.EncodeToString(h[:10])+"-file.vtt")
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		return out, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Each file once: the warm-up and the player can ask at the same moment.
	sf := &s.subtitles
	sf.mu.Lock()
	if sf.running == nil {
		sf.running = map[string]chan struct{}{}
	}
	done, busy := sf.running[out]
	if busy {
		sf.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return "", ctx.Err()
		}
		if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
			return out, nil
		}
		return "", errors.New("this subtitle file can't be converted")
	}
	done = make(chan struct{})
	sf.running[out] = done
	sf.mu.Unlock()
	defer func() {
		sf.mu.Lock()
		delete(sf.running, out)
		sf.mu.Unlock()
		close(done)
	}()
	select {
	case subtitleSlots <- struct{}{}:
		defer func() { <-subtitleSlots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if subtitle.ReadsItself(format) {
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		vtt, err := subtitle.Convert(format, raw)
		if err != nil {
			return "", err
		}
		return out, writeAtomic(out, vtt)
	}
	input := file
	if format != "mks" { // a Matroska file isn't text
		raw, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		tmp := out + ".src" + filepath.Ext(file)
		if err := os.WriteFile(tmp, subtitle.Normalize(raw), 0o644); err != nil {
			return "", err
		}
		defer os.Remove(tmp)
		input = tmp
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	tmp := out + ".tmp"
	args := append([]string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}, ffmpeg.FileInput(input)...)
	args = append(args, "-map", "0:s:0", "-vn", "-an", "-c:s", "webvtt", "-f", "webvtt", tmp)
	if _, stderr, err := ffmpeg.Run(cctx, s.Tools.FFmpeg, args, "file", file); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("%w: %s", err, stderr.Last(400))
	}
	return out, os.Rename(tmp, out)
}

// writeAtomic puts a file in place whole, so a reader never sees half of one.
func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// subtitleForBurning is the file FFmpeg should draw into the picture. Pictures are read
// as they are. ASS keeps its styling, in clean UTF-8; other text goes as WebVTT, which
// is also how formats FFmpeg can't read get drawn at all.
func (s *Server) subtitleForBurning(ctx context.Context, st playback.Subtitle) (string, error) {
	if !playback.IsTextSubtitle(st.Codec) {
		return st.File, nil
	}
	if st.Format != "ass" && st.Format != "ssa" {
		return s.subtitleFileVTT(ctx, st.File, st.Format)
	}
	info, err := os.Stat(st.File)
	if err != nil {
		return "", err
	}
	if info.Size() > subtitle.MaxInput {
		return "", errors.New("this subtitle file is too large")
	}
	h := sha1.Sum([]byte(fmt.Sprint(st.File, "|", info.Size(), "|", info.ModTime().UnixNano())))
	out := filepath.Join(s.Cfg.CacheDir, "subtitles", hex.EncodeToString(h[:10])+filepath.Ext(st.File))
	if fi, err := os.Stat(out); err == nil && fi.Size() > 0 {
		return out, nil
	}
	raw, err := os.ReadFile(st.File)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	return out, writeAtomic(out, subtitle.Normalize(raw))
}

var vttFile = regexp.MustCompile(`^([0-9]{1,3})\.vtt$`)

// GET /api/items/{id}/subtitles/{n}.vtt: a text subtitle track as WebVTT, for
// players that show it beside the picture.
func (s *Server) handleItemSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "video" {
		writeErr(w, 404, "item not found")
		return
	}
	m := vttFile.FindStringSubmatch(r.PathValue("file"))
	if m == nil {
		writeErr(w, 404, "no such subtitle")
		return
	}
	n, _ := strconv.Atoi(m[1])
	if !s.Store.LibraryFile(r.Context(), it.Path) {
		writeErr(w, 404, "file missing")
		return
	}
	pctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	pr, err := s.probeOnce(pctx, it.Path)
	if err != nil {
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	src := probeSource(it, pr)
	if n >= len(src.Subtitles) || !playback.IsTextSubtitle(src.Subtitles[n].Codec) {
		writeErr(w, 404, "no such text subtitle")
		return
	}
	var p string
	if st := src.Subtitles[n]; st.File != "" {
		p, err = s.subtitleFileVTT(r.Context(), st.File, st.Format)
	} else {
		p, err = s.subtitleVTT(r.Context(), it.Path, n)
	}
	if err != nil {
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	// A player whose clock doesn't start at the film's start needs the cues moved.
	if off, _ := strconv.ParseFloat(r.URL.Query().Get("offset"), 64); off > 0 && !math.IsInf(off, 0) {
		b, err := os.ReadFile(p)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		_, _ = w.Write(subtitle.Shift(b, off))
		return
	}
	http.ServeFile(w, r, p)
}

// GET /api/hls/{id}/master.m3u8: the stream with its subtitle renditions.
func (s *Server) handleHLSMaster(w http.ResponseWriter, r *http.Request) {
	sess := s.visibleStream(r, r.PathValue("id"))
	if sess == nil {
		writeErr(w, 404, "no such stream (it may have expired; start playback again)")
		return
	}
	st := r.URL.Query().Get("st")
	subs := func(i int) string { return withStream(fmt.Sprintf("subs/%d.m3u8", i), st) }
	var b []byte
	if rungs := s.ladderEntries(r, sess, r.URL.Query().Get("ladder"), st); len(rungs) > 0 {
		b = sess.LadderPlaylist(rungs, subs)
	} else {
		b = sess.MasterPlaylist(withStream("playlist.m3u8", st), subs)
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

var hlsSubFile = regexp.MustCompile(`^([0-9]{1,3})\.(m3u8|vtt)$`)

// GET /api/hls/{id}/subs/{n}.m3u8 and {n}.vtt: one subtitle rendition (n is its
// place in the master playlist): a playlist with one segment, and that segment.
func (s *Server) handleHLSSubtitle(w http.ResponseWriter, r *http.Request) {
	sess := s.visibleStream(r, r.PathValue("id"))
	if sess == nil || sess.Spec.Variant == nil {
		writeErr(w, 404, "no such stream")
		return
	}
	m := hlsSubFile.FindStringSubmatch(r.PathValue("file"))
	if m == nil {
		writeErr(w, 404, "no such subtitle")
		return
	}
	n, _ := strconv.Atoi(m[1])
	if n >= len(sess.Spec.Variant.Subtitles) {
		writeErr(w, 404, "no such subtitle")
		return
	}
	if m[2] == "m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(sess.SubtitlePlaylist(withStream(fmt.Sprintf("%d.vtt", n), r.URL.Query().Get("st"))))
		return
	}
	rendition := sess.Spec.Variant.Subtitles[n]
	var p string
	var err error
	if rendition.File != "" {
		p, err = s.subtitleFileVTT(logging.WithSession(r.Context(), sess.LogSession()), rendition.File, rendition.Format)
	} else {
		p, err = s.subtitleVTT(logging.WithSession(r.Context(), sess.LogSession()), sess.Spec.Source, rendition.Stream)
	}
	if err != nil {
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		writeErr(w, 500, s.viewerError(r, err))
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(timestampedVTT(b, s.hlsStartPTS(r.Context(), sess), sess.Spec.StartSec))
}

// hlsStartPTS is the stream clock at the start of the first segment, in the
// 90 kHz ticks X-TIMESTAMP-MAP uses. FFmpeg's MPEG-TS muxer starts a little
// above zero; fMP4 starts at zero.
func (s *Server) hlsStartPTS(ctx context.Context, sess *hls.Session) int64 {
	if sess.Spec.FMP4 {
		return 0
	}
	p, err := sess.Segment(ctx, 0)
	if err != nil {
		return 0 // FFmpeg's failure has its own record, and so has a failed probe below
	}
	pctx, cancel := context.WithTimeout(logging.WithSession(ctx, sess.LogSession()), 10*time.Second)
	defer cancel()
	pr, err := s.Tools.ProbeFile(pctx, p)
	if err != nil {
		return 0
	}
	t, _ := strconv.ParseFloat(pr.Format.StartTime, 64)
	return int64(math.Round(t * 90000))
}

// timestampedVTT adds the line HLS requires to WebVTT segments, tying cue times
// (which count from the start of the film) to the stream's clock.
func timestampedVTT(vtt []byte, pts int64, startSec float64) []byte {
	head, rest, _ := strings.Cut(strings.ReplaceAll(string(vtt), "\r\n", "\n"), "\n")
	return []byte(fmt.Sprintf("%s\nX-TIMESTAMP-MAP=MPEGTS:%d,LOCAL:%s\n%s", head, pts, vttClock(startSec), rest))
}

func vttClock(sec float64) string {
	ms := int64(math.Round(sec * 1000))
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
