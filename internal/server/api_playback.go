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
	"github.com/wittywilla/wittymediaserver/internal/playback"
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
	// Decision: direct | remux | transcode | disc | audio-direct | audio-transcode.
	Decision string   `json:"decision"`
	Reasons  []string `json:"reasons"`
	// Protocol: file (a plain file with byte ranges) | hls | adts | disc-session.
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
	// growing remux: past what has been made, ask again with startSec).
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
	ProgressURL      string         `json:"progressUrl"`
}

// probeSource turns a probe into what the decision needs.
func probeSource(it *library.Item, pr *ffmpeg.Probe) playback.Source {
	src := playback.Source{Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(it.Path)), ".")}
	if v := pr.Video(); v != nil {
		src.HasVideo = true
		src.VideoCodec = v.CodecName
		src.VideoTag = v.CodecTag
		src.Height = v.Height
		src.BitDepth = bitDepth(v)
		src.Range = videoRange(v)
		src.Interlaced = interlaced(v)
		src.BitrateKbps = kbps(v.BitRate)
		if src.BitrateKbps == 0 {
			src.BitrateKbps = kbps(pr.Format.BitRate)
		}
	}
	for _, a := range pr.Audio() {
		title := strings.ToLower(a.Tags["title"])
		src.Audio = append(src.Audio, playback.Audio{Codec: a.CodecName, Channels: a.Channels, Language: a.Tags["language"], Title: a.Tags["title"],
			Default: a.Disposition["default"] == 1, Comment: a.Disposition["comment"] == 1 || strings.Contains(title, "commentary")})
	}
	for _, st := range pr.Subtitles() {
		title := strings.ToLower(st.Tags["title"])
		src.Subtitles = append(src.Subtitles, playback.Subtitle{Codec: st.CodecName, Language: st.Tags["language"], Title: st.Tags["title"],
			Default: st.Disposition["default"] == 1, Forced: st.Disposition["forced"] == 1 || strings.Contains(title, "forced"),
			SDH: st.Disposition["hearing_impaired"] == 1 || strings.Contains(title, "sdh")})
	}
	return src
}

func kbps(s string) int {
	n, _ := strconv.Atoi(s)
	return n / 1000
}

func interlaced(v *ffmpeg.Stream) bool {
	return v.FieldOrder == "tt" || v.FieldOrder == "bb" || v.FieldOrder == "tb" || v.FieldOrder == "bt"
}

func bitDepth(v *ffmpeg.Stream) int {
	if n, _ := strconv.Atoi(v.BitsPerSample); n > 0 {
		return n
	}
	switch {
	case strings.Contains(v.PixFmt, "12"):
		return 12
	case strings.Contains(v.PixFmt, "10"):
		return 10
	}
	return 8
}

// videoRange classes the picture: sdr, hdr10 (PQ), hlg or dv5. Dolby Vision
// with a base layer other players can show counts as that base layer; profile
// 5 has none (its colours come out wrong without a Dolby Vision decoder).
func videoRange(v *ffmpeg.Stream) string {
	for _, sd := range v.SideData {
		if t, _ := sd["side_data_type"].(string); t != "DOVI configuration record" {
			continue
		}
		if p, _ := sd["dv_profile"].(float64); p == 5 {
			return "dv5"
		}
	}
	switch v.ColorTransfer {
	case "smpte2084":
		return "hdr10"
	case "arib-std-b67":
		return "hlg"
	}
	return "sdr"
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
	if b.MaxHeight < 0 || b.MaxBitrateKbps < 0 {
		writeErr(w, 400, "maxHeight and maxBitrateKbps can't be negative")
		return
	}
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
		NextID: s.Store.NextEpisode(r.Context(), it)}
	if it.Progress != nil {
		out.ResumeSec = it.Progress.ResumeSec
	}
	switch {
	case isDiscKind(it):
		s.playbackDisc(r, it, &out)
	case it.Kind == "audio":
		s.playbackAudio(r, it, p, b, &out)
	case it.Kind == "video":
		if err := s.playbackVideo(r, it, p, b, &out); err != nil {
			code := 500
			if errors.As(err, new(errBadTrack)) {
				code = 400
			}
			writeErr(w, code, err.Error())
			return
		}
	default:
		writeErr(w, 400, "this kind of item isn't played: books open with GET /api/items/{id}/book")
		return
	}
	writeJSON(w, 200, out)
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
	// VideoCodec holds the sound's codec for audio items.
	if !slices.Contains(p.AudioCodecs, it.VideoCodec) || !library.DirectAudio[it.VideoCodec] || b.ForceTranscode {
		u += "?transcode=1"
		out.Decision, out.Protocol, out.Seek = "audio-transcode", "adts", "restart"
		out.Reasons = append(out.Reasons, playback.ReasonAudioCodec)
		if b.StartSec != nil && *b.StartSec > 0 {
			u += "&start=" + strconv.FormatFloat(*b.StartSec, 'f', 3, 64)
			out.TimeOffsetSec = *b.StartSec
		}
	} else if b.StartSec != nil {
		out.StartAtSec = *b.StartSec
	}
	out.URL = withStream(u, out.StreamToken)
}

// playbackVideo decides how to play a video file and sets the stream up.
func (s *Server) playbackVideo(r *http.Request, it *library.Item, p playback.Profile, b playbackRequest, out *playbackResponse) error {
	ctx := r.Context()
	pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pr, err := s.Tools.ProbeFile(pctx, it.Path)
	if err != nil {
		return err
	}
	src := probeSource(it, pr)
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
	q, serverMax := s.quality(r)
	req := playback.Request{AudioIndex: chosen.Audio, SubtitleIndex: chosen.Subtitle, MaxBitrateKbps: b.MaxBitrateKbps, BurnSubtitle: b.BurnSubtitle,
		AllowDirect: s.DB.GetSetting(ctx, "directPlay", "1") == "1", ForceTranscode: b.ForceTranscode, MaxHeight: playback.Smallest(b.MaxHeight, serverMax)}
	d := playback.Decide(p, src, req)

	out.DurationSec = pr.Duration()
	out.Decision, out.Reasons, out.SubtitleDelivery = d.Mode, d.Reasons, d.Subtitle
	out.AudioIndex, out.SubtitleIndex = chosen.Audio, chosen.Subtitle
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
	s.Log.Info("play", "cat", "playback", "item", it.ID, "title", it.Title, "profile", p.ID, "mode", d.Mode, "reasons", strings.Join(d.Reasons, ","), "audioTrack", chosen.Audio, "subtitleTrack", chosen.Subtitle)

	token := out.StreamToken
	subURL := func(i int) string {
		return withStream(fmt.Sprintf("/api/items/%d/subtitles/%d.vtt", it.ID, i), token)
	}
	if d.Mode == playback.Direct {
		out.Protocol = "file"
		out.StartAtSec = start
		out.URL = withStream(fmt.Sprintf("/api/items/%d/file", it.ID), token)
		fillSubtitles(out, src, p, true, subURL)
		if d.Subtitle == playback.SubSidecar {
			s.warmSubtitle(it.Path, chosen.Subtitle)
		}
		return nil
	}

	// HLS: the same stream is shared by viewers who get the same thing.
	vf := ""
	if src.HasVideo && !d.CopyVideo {
		if src.Interlaced {
			vf = "bwdif=mode=send_frame:parity=auto:deint=interlaced"
		}
		if d.ToneMap {
			if tm := s.Tools.ToneMapFilter(); tm != "" {
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
	spec := hls.Spec{
		SubtitleText: s.textBurn(burn, pr), InputArgs: ffmpeg.FileInput(input), Source: input, Duration: pr.Duration(), VideoFilter: vf, AudioIndex: audioIdx, AudioChannels: channels,
		SubtitleIndex: burn, Quality: q, MaxHeight: req.MaxHeight, AudioCodec: "aac",
	}
	spec.MaxHeight = playback.Smallest(spec.MaxHeight, p.MaxHeight)
	if d.CopyVideo {
		spec.CopyVideo, spec.CopyAudio, spec.FMP4, spec.Growing = true, d.CopyAudio, d.FMP4, true
		if d.FMP4 && src.VideoCodec == "hevc" {
			spec.VideoTag = "hvc1"
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
			v.Subtitles = append(v.Subtitles, hls.Rendition{Stream: i, Name: name, Language: playback.NormalizeLanguage(st.Language),
				Default: i == chosen.Subtitle && d.Subtitle == playback.SubHLS, Forced: st.Forced})
		}
	}
	spec.Variant = v
	sum := sha1.Sum([]byte(fmt.Sprint(p.ID, d, chosen, spec.MaxHeight, q, start*boolFloat(spec.Growing), vf, len(v.Subtitles), v.Codecs)))
	spec.ID = fmt.Sprintf("v-%s-n%s", hls.Key(it.Path), hex.EncodeToString(sum[:6]))
	s.HLS.Ensure(spec)
	out.StreamID = spec.ID
	out.Protocol = "hls"
	if len(v.Subtitles) > 0 {
		out.URL = withStream("/api/hls/"+spec.ID+"/master.m3u8", token)
	} else {
		out.URL = withStream("/api/hls/"+spec.ID+"/playlist.m3u8", token)
	}
	fillSubtitles(out, src, p, false, subURL)
	if d.Subtitle == playback.SubHLS || d.Subtitle == playback.SubSidecar {
		s.warmSubtitle(it.Path, chosen.Subtitle)
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
		go func() {
			defer func() {
				sf.mu.Lock()
				delete(sf.running, out)
				sf.mu.Unlock()
				close(done)
			}()
			subtitleSlots <- struct{}{}
			defer func() { <-subtitleSlots }()
			if err := s.convertSubtitle(path, n, out); err != nil {
				s.Log.Warn("subtitle", "cat", "ffmpeg", "err", err)
			}
		}()
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

func (s *Server) convertSubtitle(path string, n int, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	tmp := out + ".tmp"
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	args = append(args, ffmpeg.FileInput(path)...)
	args = append(args, "-map", fmt.Sprintf("0:s:%d", n), "-vn", "-an", "-c:s", "webvtt", "-f", "webvtt", tmp)
	if b, err := ffmpeg.Command(ctx, s.Tools.FFmpeg, args...).CombinedOutput(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(b)))
	}
	return os.Rename(tmp, out)
}

// warmSubtitle starts converting the chosen track, so it is ready soon after
// the picture starts.
func (s *Server) warmSubtitle(path string, n int) {
	if n < 0 || s.Cfg == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		_, _ = s.subtitleVTT(ctx, path, n)
	}()
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
	if st, err := os.Stat(s.subtitlePath(it.Path, n)); err != nil || st.Size() == 0 {
		// Not converted yet: only text tracks that exist are worth reading the file for.
		pctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		pr, err := s.Tools.ProbeFile(pctx, it.Path)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		subs := pr.Subtitles()
		if n >= len(subs) || !playback.IsTextSubtitle(subs[n].CodecName) {
			writeErr(w, 404, "no such text subtitle")
			return
		}
	}
	p, err := s.subtitleVTT(r.Context(), it.Path, n)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, p)
}

// GET /api/hls/{id}/master.m3u8: the stream with its subtitle renditions.
func (s *Server) handleHLSMaster(w http.ResponseWriter, r *http.Request) {
	sess := s.HLS.Get(r.PathValue("id"))
	if sess == nil {
		writeErr(w, 404, "no such stream (it may have expired; start playback again)")
		return
	}
	st := r.URL.Query().Get("st")
	b := sess.MasterPlaylist(withStream("playlist.m3u8", st), func(i int) string {
		return withStream(fmt.Sprintf("subs/%d.m3u8", i), st)
	})
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

var hlsSubFile = regexp.MustCompile(`^([0-9]{1,3})\.(m3u8|vtt)$`)

// GET /api/hls/{id}/subs/{n}.m3u8 and {n}.vtt: one subtitle rendition (n is its
// place in the master playlist): a playlist with one segment, and that segment.
func (s *Server) handleHLSSubtitle(w http.ResponseWriter, r *http.Request) {
	sess := s.HLS.Get(r.PathValue("id"))
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
	p, err := s.subtitleVTT(r.Context(), sess.Spec.Source, sess.Spec.Variant.Subtitles[n].Stream)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		writeErr(w, 500, err.Error())
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
		return 0
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
