// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libtranscode

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
	"github.com/wittywilla/wittymediaserver/internal/playback"
)

// Source is a file as the planner sees it, read from a probe.
type Source struct {
	Path     string
	Ext      string // lower case, no dot
	Duration float64
	Size     int64
	Kbps     int // the whole file's average

	Video       []VideoStream
	Covers      []Picture
	Audio       []AudioStream
	Subs        []SubStream
	Attachments []Attachment
	Other       []OtherStream // data and anything else that isn't watched or heard
	Chapters    int
	Sidecars    []Sidecar
	// Original is the language the film was made in, when something says so.
	Original string
}

// VideoStream is a real picture stream.
type VideoStream struct {
	Index        int // the stream's number in the file
	Codec        string
	Profile      string
	Tag          string
	PixFmt       string
	Width        int
	Height       int
	Depth        int
	Range        string // sdr, hdr10, hlg, dv5
	Dynamic      string // "HDR10+", "Dolby Vision profile 8.1"
	DV           bool
	DVProfile    int
	Interlaced   bool
	Stereo       string // sbs, tab, ""
	MVC          bool   // a second eye stored in a way a conversion would lose
	Variable     bool   // variable frame rate
	FPS          float64
	Kbps         int
	Default      bool
	Language     string
	Title        string
	Captions     bool // closed captions inside the picture
	Mastering    string
	ContentLight string
}

// Picture is cover art: a still stored as a video stream.
type Picture struct {
	Index int
	Codec string
	Title string
	Name  string // its file name, in a file that attaches it
	// Number is its place among the video streams of the file, for pulling it out.
	Number int
}

// AudioStream is one sound track.
type AudioStream struct {
	Index       int
	Codec       string
	Profile     string
	Channels    int
	Language    string // short code, "" when untagged
	Title       string
	Default     bool
	Commentary  bool
	Description bool
	Original    bool
	Lossless    bool
	Objects     bool // Atmos or DTS:X
	Kbps        int
}

// SubStream is one subtitle track inside the file.
type SubStream struct {
	Index    int
	Codec    string
	Language string
	Title    string
	Default  bool
	Forced   bool
	SDH      bool
	Text     bool
}

// Attachment is a file carried inside the file: usually a font for styled subtitles.
type Attachment struct {
	Index int
	Name  string
}

// OtherStream is a stream that isn't picture, sound or subtitles.
type OtherStream struct {
	Index int
	Type  string
	Codec string
}

// Sidecar is a subtitle file beside the video.
type Sidecar struct {
	Path     string
	Codec    string
	Language string
	Forced   bool
	SDH      bool
	Text     bool
}

var (
	commentaryRe  = regexp.MustCompile(`(?i)\bcomment(ary|aries|aire)?\b|\bdirector'?s? (track|audio)\b`)
	descriptionRe = regexp.MustCompile(`(?i)\b(audio descri|descriptive|described video|video description|dvs)\b|\bAD\b`)
	sdhRe         = regexp.MustCompile(`(?i)\b(sdh|hoh|hearing|deaf|closed captions?|cc)\b`)
	forcedRe      = regexp.MustCompile(`(?i)\b(forced|foreign|signs?( and| &)? songs?)\b`)
)

// lossless sound, which a person may want to shrink.
func isLossless(codec, profile string) bool {
	switch codec {
	case "truehd", "mlp", "flac", "alac", "wavpack", "tta", "ape", "shorten":
		return true
	case "dts":
		return strings.Contains(profile, "HD MA") || strings.Contains(profile, "DTS-HD MA")
	}
	return strings.HasPrefix(codec, "pcm_")
}

// hasObjects: Atmos or DTS:X. Their extra layer doesn't survive a conversion.
func hasObjects(profile string) bool {
	p := strings.ToLower(profile)
	return strings.Contains(p, "atmos") || strings.Contains(p, "dts:x") || strings.Contains(p, "dts-x")
}

// Inspect reads what a probe found. A probe may be of anything, so nothing is assumed.
func Inspect(path string, p *ffmpeg.Probe) *Source {
	s := &Source{Path: path, Ext: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), Duration: p.Duration(), Chapters: len(p.Chapters)}
	s.Size, _ = strconv.ParseInt(p.Format.Size, 10, 64)
	s.Kbps = kbpsOf(p.Format.BitRate)
	videoSeen := 0
	for i := range p.Streams {
		st := &p.Streams[i]
		lang := languageOf(st.Tags["language"])
		title := cleanTitle(st.Tags["title"])
		switch st.CodecType {
		case "video":
			videoSeen++
			cover := Picture{Index: st.Index, Codec: st.CodecName, Title: title, Name: cleanTitle(st.Tags["filename"]), Number: videoSeen - 1}
			if st.Disposition["attached_pic"] == 1 || st.Disposition["still_image"] == 1 || st.Disposition["timed_thumbnails"] == 1 {
				s.Covers = append(s.Covers, cover)
				continue
			}
			// A still with no flag on it, as an MKV written by FFmpeg holds a cover. Beside a
			// real picture it can only be a cover; alone, it's a film in Motion JPEG.
			if stillCodec(st.CodecName) && realPictures(p) > 0 {
				s.Covers = append(s.Covers, cover)
				continue
			}
			v := VideoStream{Index: st.Index, Codec: st.CodecName, Profile: st.Profile, Tag: st.CodecTag, PixFmt: st.PixFmt, Language: lang, Title: title,
				Depth: st.Depth(), Range: st.Range(), Dynamic: st.DynamicRange(), Interlaced: st.IsInterlaced(), Stereo: st.StereoMode(path),
				Kbps: kbpsOf(st.BitRate), Default: st.Disposition["default"] == 1, Captions: st.ClosedCaptions == 1}
			v.Width, v.Height = st.DisplaySize()
			v.DVProfile, _, v.DV = st.DolbyVision()
			prof := strings.ToLower(st.Profile)
			v.MVC = strings.Contains(prof, "stereo") || strings.Contains(prof, "multiview")
			avg, nominal := st.FrameRates()
			v.FPS = avg
			if v.FPS <= 0 {
				v.FPS = nominal
			}
			v.Variable = avg > 0 && nominal > 0 && abs(avg-nominal)/nominal > 0.01
			v.Mastering, v.ContentLight = hdrParams(st)
			s.Video = append(s.Video, v)
		case "audio":
			a := AudioStream{Index: st.Index, Codec: st.CodecName, Profile: st.Profile, Channels: st.Channels, Language: lang, Title: title, Default: st.Disposition["default"] == 1,
				Commentary:  st.Disposition["comment"] == 1 || commentaryRe.MatchString(title),
				Description: st.Disposition["visual_impaired"] == 1 || descriptionRe.MatchString(title),
				Original:    st.Disposition["original"] == 1,
				Lossless:    isLossless(st.CodecName, st.Profile), Objects: hasObjects(st.Profile), Kbps: kbpsOf(st.BitRate)}
			if a.Description {
				a.Commentary = false
			}
			s.Audio = append(s.Audio, a)
		case "subtitle":
			s.Subs = append(s.Subs, SubStream{Index: st.Index, Codec: st.CodecName, Language: lang, Title: title, Default: st.Disposition["default"] == 1,
				Forced: st.Disposition["forced"] == 1 || forcedRe.MatchString(title),
				SDH:    st.Disposition["hearing_impaired"] == 1 || sdhRe.MatchString(title),
				Text:   mediatypes.IsTextSubtitle(st.CodecName)})
		case "attachment":
			s.Attachments = append(s.Attachments, Attachment{Index: st.Index, Name: st.Tags["filename"]})
		default:
			s.Other = append(s.Other, OtherStream{Index: st.Index, Type: st.CodecType, Codec: st.CodecName})
		}
	}
	// Commentary marked by nothing but a low place in the list stays a plain track: a
	// guess that hides someone's only copy of a language would be worse.
	return s
}

// AddSidecars lists the subtitle files beside the video.
func (s *Source) AddSidecars(found []library.SubtitleFile) {
	for _, f := range found {
		s.Sidecars = append(s.Sidecars, Sidecar{Path: f.Path, Codec: f.Codec, Language: languageOf(f.Language), Forced: f.Forced, SDH: f.SDH, Text: f.Text()})
	}
}

func stillCodec(codec string) bool {
	switch codec {
	case "mjpeg", "png", "bmp", "webp":
		return true
	}
	return false
}

// realPictures counts the video streams that can't be stills.
func realPictures(p *ffmpeg.Probe) int {
	n := 0
	for i := range p.Streams {
		st := &p.Streams[i]
		if st.CodecType == "video" && !stillCodec(st.CodecName) && st.Disposition["attached_pic"] != 1 {
			n++
		}
	}
	return n
}

func languageOf(tag string) string {
	n := playback.NormalizeLanguage(tag)
	if n == "und" || n == "unk" || n == "zxx" || n == "mis" || n == "mul" {
		return ""
	}
	return n
}

// cleanTitle keeps a track's title to something that can go on a command line and in a
// sentence. The title comes from the file, so it's treated as hostile.
func cleanTitle(t string) string {
	t = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, t)
	if len(t) > 200 {
		t = t[:200]
	}
	return strings.TrimSpace(t)
}

func kbpsOf(s string) int {
	n, _ := strconv.Atoi(s)
	return n / 1000
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// hdrParams writes a stream's mastering display and light level in the form an x265
// encoder asks for. Empty when the file doesn't say.
func hdrParams(st *ffmpeg.Stream) (mastering, light string) { return hdrSideData(st.SideData) }

// hdrSideData reads them from a stream's or a frame's side data, which look the same.
func hdrSideData(list []map[string]any) (mastering, light string) {
	for _, sd := range list {
		switch t, _ := sd["side_data_type"].(string); t {
		case "Mastering display metadata":
			g, gOK := ratioIn(sd, "green_x", "green_y", 50000)
			b, bOK := ratioIn(sd, "blue_x", "blue_y", 50000)
			r, rOK := ratioIn(sd, "red_x", "red_y", 50000)
			w, wOK := ratioIn(sd, "white_point_x", "white_point_y", 50000)
			l, lOK := ratioIn(sd, "max_luminance", "min_luminance", 10000)
			if gOK && bOK && rOK && wOK && lOK {
				mastering = fmt.Sprintf("G(%s)B(%s)R(%s)WP(%s)L(%s)", g, b, r, w, l)
			}
		case "Content light level metadata":
			mx, ok1 := sd["max_content"].(float64)
			av, ok2 := sd["max_average"].(float64)
			if ok1 && ok2 {
				light = fmt.Sprintf("%d,%d", int(mx), int(av))
			}
		}
	}
	return mastering, light
}

// ratioIn reads two "a/b" values and writes them as whole numbers over scale.
func ratioIn(sd map[string]any, xKey, yKey string, scale float64) (string, bool) {
	x, ok1 := sd[xKey].(string)
	y, ok2 := sd[yKey].(string)
	if !ok1 || !ok2 {
		return "", false
	}
	return fmt.Sprintf("%d,%d", int(ratioOf(x)*scale+0.5), int(ratioOf(y)*scale+0.5)), true
}

func ratioOf(s string) float64 {
	a, b, ok := strings.Cut(s, "/")
	n, _ := strconv.ParseFloat(a, 64)
	if !ok {
		return n
	}
	d, _ := strconv.ParseFloat(b, 64)
	if d == 0 {
		return 0
	}
	return n / d
}

// Main picks the picture a film is: one marked default that isn't simply the first (tools
// mark the first by themselves, so that says nothing), else the largest, else the first.
func (s *Source) Main() int {
	best := -1
	count := 0
	for i, v := range s.Video {
		if v.Default {
			count++
			if i > 0 && best < 0 {
				best = i
			}
		}
	}
	if best >= 0 && count == 1 {
		return best
	}
	best = -1
	for i, v := range s.Video {
		if best < 0 || v.Width*v.Height > s.Video[best].Width*s.Video[best].Height {
			best = i
		}
	}
	return best
}

// OriginalLanguage says which language the film was made in, and how that's known.
func (s *Source) OriginalLanguage() (lang, how string) {
	if s.Original != "" {
		return s.Original, "from this title's details"
	}
	for _, a := range s.Audio {
		if a.Original && a.Language != "" {
			return a.Language, "the file marks that track as the original"
		}
	}
	for _, a := range s.Audio {
		if a.Default && !a.Commentary && !a.Description && a.Language != "" {
			return a.Language, "the file's default track, since nothing says otherwise"
		}
	}
	for _, a := range s.Audio {
		if !a.Commentary && !a.Description && a.Language != "" {
			return a.Language, "the file's first track, since nothing says otherwise"
		}
	}
	return "", ""
}

// fileLanguage is the form a language is written in inside files: three letters, which is
// all an MP4 can hold.
func fileLanguage(code string) string {
	if long, ok := threeLetters[code]; ok {
		return long
	}
	return code
}

var threeLetters = map[string]string{"en": "eng", "nl": "dut", "fr": "fre", "de": "ger", "es": "spa", "it": "ita", "pt": "por", "ja": "jpn", "zh": "chi", "ko": "kor",
	"ru": "rus", "sv": "swe", "da": "dan", "no": "nor", "fi": "fin", "pl": "pol", "cs": "cze", "hu": "hun", "el": "gre", "tr": "tur", "he": "heb", "ar": "ara",
	"hi": "hin", "th": "tha", "ro": "rum", "uk": "ukr", "vi": "vie", "id": "ind", "ca": "cat"}

// languageWords is a language in a person's words.
func languageWords(code string) string {
	if code == "" {
		return "No language"
	}
	if n := playback.LanguageName(code); n != "" {
		return n
	}
	return strings.ToUpper(code)
}

func codecName(id string) string {
	switch id {
	case "h264":
		return "H.264"
	case "hevc":
		return "H.265"
	case "av1":
		return "AV1"
	case "vp9":
		return "VP9"
	case "vp8":
		return "VP8"
	case "mpeg2video":
		return "MPEG-2"
	case "mpeg4":
		return "MPEG-4"
	case "msmpeg4v3":
		return "DivX 3"
	case "aac":
		return "AAC"
	case "ac3":
		return "Dolby Digital"
	case "eac3":
		return "Dolby Digital Plus"
	case "truehd":
		return "Dolby TrueHD"
	case "dts":
		return "DTS"
	case "flac":
		return "FLAC"
	case "opus":
		return "Opus"
	case "vorbis":
		return "Vorbis"
	case "mp3":
		return "MP3"
	case "mp2":
		return "MP2"
	case "alac":
		return "ALAC"
	case "subrip":
		return "SRT text"
	case "ass", "ssa":
		return "styled text (ASS)"
	case "webvtt":
		return "WebVTT text"
	case "mov_text":
		return "MP4 text"
	case "hdmv_pgs_subtitle":
		return "Blu-ray pictures (PGS)"
	case "dvd_subtitle":
		return "DVD pictures (VobSub)"
	case "dvb_subtitle":
		return "broadcast pictures (DVB)"
	case "xsub":
		return "DivX pictures"
	}
	if strings.HasPrefix(id, "pcm_") {
		return "PCM"
	}
	for _, c := range mediatypes.Codecs() {
		if c.ID == id {
			return c.Name
		}
	}
	return strings.ToUpper(id)
}

func channelWords(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 3:
		return "2.1"
	case 6:
		return "5.1"
	case 7:
		return "6.1"
	case 8:
		return "7.1"
	}
	return strconv.Itoa(n) + " channels"
}

func (a AudioStream) describe() string {
	words := []string{languageWords(a.Language)}
	kind := codecName(a.Codec)
	if a.Objects {
		kind += " with objects"
	} else if a.Lossless && a.Codec == "dts" {
		kind = "DTS-HD MA"
	}
	words = append(words, kind)
	if c := channelWords(a.Channels); c != "" {
		words = append(words, c)
	}
	s := strings.Join(words, ", ")
	switch {
	case a.Commentary:
		s += " (commentary)"
	case a.Description:
		s += " (audio description)"
	}
	if a.Title != "" && !a.Commentary && !a.Description {
		s += " \"" + a.Title + "\""
	}
	return s
}

func (t SubStream) describe() string {
	s := languageWords(t.Language) + ", " + codecName(t.Codec)
	switch {
	case t.Forced:
		s += " (forced)"
	case t.SDH:
		s += " (SDH)"
	}
	return s
}

func (v VideoStream) describe() string {
	s := fmt.Sprintf("%s, %dx%d", codecName(v.Codec), v.Width, v.Height)
	if v.Depth > 8 {
		s += fmt.Sprintf(", %d-bit", v.Depth)
	}
	switch {
	case v.Dynamic != "":
		s += ", " + v.Dynamic
	case v.Range == "hdr10":
		s += ", HDR10"
	case v.Range == "hlg":
		s += ", HLG"
	}
	if v.Interlaced {
		s += ", interlaced"
	}
	if v.Variable {
		s += ", variable frame rate"
	}
	switch v.Stereo {
	case "sbs":
		s += ", 3D side by side"
	case "tab":
		s += ", 3D top and bottom"
	}
	return s
}

func (s *Source) hasAudioLanguage(lang string) bool {
	return slices.ContainsFunc(s.Audio, func(a AudioStream) bool { return a.Language == lang })
}
