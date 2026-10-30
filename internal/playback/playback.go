// Package playback decides how a client plays a file: as it is (direct),
// rewrapped into HLS without touching the picture (remux), or converted
// (transcode). It is plain logic with no I/O: the server describes the file
// and the client, and gets back a decision, so every rule can be tested
// without FFmpeg or a real device. See docs/CLIENT-API.md.
package playback

import (
	"slices"
	"strings"
)

// Modes.
const (
	Direct    = "direct"
	Remux     = "remux"
	Transcode = "transcode"
)

// How a subtitle track reaches the screen.
const (
	SubNone     = "none"     // no subtitle chosen
	SubEmbedded = "embedded" // inside the file; the player draws it
	SubSidecar  = "sidecar"  // a separate WebVTT file the player or app draws
	SubHLS      = "hls"      // a WebVTT rendition in the HLS master playlist
	SubBurn     = "burn"     // drawn into the picture by the server
)

// Reasons a file can't play as it is. They are part of the API: apps may
// show them, and logs quote them.
const (
	ReasonForced       = "forced"            // the client asked for a conversion
	ReasonDirectOff    = "direct-play-off"   // an administrator turned direct play off
	ReasonContainer    = "container"         // the player can't open this kind of file
	ReasonVideoCodec   = "video-codec"       // the player can't decode the picture
	ReasonVideoTag     = "video-tag"         // HEVC stored as hev1, which Apple players refuse
	ReasonVideoHeight  = "video-height"      // more lines than the client or the server allows
	ReasonVideoBitrate = "video-bitrate"     // more data than the client can take
	ReasonVideoDepth   = "video-depth"       // 10-bit picture on a client that can't show it
	ReasonVideoRange   = "video-range"       // HDR the display can't show
	ReasonInterlaced   = "interlaced"        // the player doesn't deinterlace
	ReasonAudioCodec   = "audio-codec"       // the player can't decode this sound
	ReasonAudioChans   = "audio-channels"    // more channels than the client takes
	ReasonAudioTrack   = "audio-track"       // the player can't switch tracks inside a file
	ReasonSubtitle     = "subtitle"          // this subtitle needs HLS or burning in
	ReasonNoVideo      = "no-video"          // nothing to decide on (a sound-only file)
	ReasonBurnAsked    = "burn-subtitle"     // the client asked for burned-in subtitles
	ReasonHLSVideo     = "hls-video-codec"   // the picture can't be carried in HLS for this client
	ReasonRemuxDisable = "remux-unavailable" // this client doesn't take remuxed streams
)

// Profile says what a client can play. The built-in ones are a safe default
// for each platform; an app that can ask its device (MediaCodecList on
// Android, AVPlayer on tvOS) sends what it found as Capabilities.
type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Direct play: file extensions without the dot, FFmpeg codec names.
	Containers  []string `json:"containers"`
	VideoCodecs []string `json:"videoCodecs"`
	AudioCodecs []string `json:"audioCodecs"`
	// HDR lists the kinds of HDR the display shows: hdr10, hlg, dv5.
	HDR []string `json:"hdr"`
	// TenBit: 10-bit HEVC, VP9 and AV1. HiTenH264 is 10-bit H.264, which
	// hardly anything but a browser's software decoder plays.
	TenBit     bool `json:"tenBit"`
	HiTenH264  bool `json:"hi10pH264"`
	Interlaced bool `json:"interlaced"` // the player deinterlaces by itself
	// HVC1Only: HEVC must be tagged hvc1 (Apple), so hev1 files are rewrapped.
	HVC1Only bool `json:"hvc1Only"`
	// 0 = no limit.
	MaxHeight        int `json:"maxHeight"`
	MaxBitrateKbps   int `json:"maxBitrateKbps"`
	MaxAudioChannels int `json:"maxAudioChannels"`
	// SwitchAudio: the player can pick another sound track inside a file, so
	// choosing one needs no new stream.
	SwitchAudio bool `json:"switchAudio"`
	// EmbeddedSubtitles: subtitle codecs the player itself draws from inside
	// a directly played file. SidecarSubtitles: it (or the app) can show a
	// separate WebVTT file over a directly played file.
	EmbeddedSubtitles []string `json:"embeddedSubtitles"`
	SidecarSubtitles  bool     `json:"sidecarSubtitles"`
	// HLS: Remux allows rewrapping; HLSVideo and HLSAudio are the codecs
	// carried unchanged; FMP4Video must use fragmented MP4 segments (MPEG-TS
	// otherwise). HLSSubtitles: WebVTT renditions in the master playlist.
	Remux        bool     `json:"remux"`
	HLSVideo     []string `json:"hlsVideo"`
	HLSAudio     []string `json:"hlsAudio"`
	FMP4Video    []string `json:"fmp4Video"`
	HLSSubtitles bool     `json:"hlsSubtitles"`
}

// Text subtitle codecs that convert to WebVTT. Anything else is a picture
// (Blu-ray PGS, DVD and DVB bitmaps) and can only be drawn or burned in.
var textSubs = []string{"subrip", "srt", "ass", "ssa", "webvtt", "mov_text", "text", "microdvd", "subviewer", "subviewer1", "sami", "realtext", "stl", "vplayer", "mpl2", "pjs", "jacosub", "ttml", "dfxp", "eia_608", "cea608"}

// IsTextSubtitle reports whether a subtitle codec is text, not pictures.
func IsTextSubtitle(codec string) bool { return slices.Contains(textSubs, codec) }

// Profiles returns the built-in client profiles.
//
// Apple TV follows Apple's HLS authoring specification: HEVC only in fMP4,
// hvc1 tags, AAC, AC-3 and E-AC-3 sound, WebVTT renditions in the master
// playlist; AVPlayer doesn't open MKV and can't show a sidecar file over a
// progressive download by itself. Android TV follows the Media3 format
// tables: its extractors read MKV, WebM and MPEG-TS as well, decoders depend
// on the device, so the default stays with what every Android TV device must
// decode. The app adds what MediaCodecList reports (AC-3, DTS, AV1, Dolby
// Vision...) in Capabilities.
func Profiles() []Profile {
	return []Profile{
		{
			ID: "web", Name: "Web browser",
			Containers: []string{"mp4", "m4v", "mov", "webm"}, VideoCodecs: []string{"h264", "vp9", "av1"},
			AudioCodecs: []string{"aac", "mp3", "opus", "vorbis"},
			HDR:         []string{"hdr10", "hlg", "dv5"}, TenBit: true, HiTenH264: true, Interlaced: true,
		},
		{
			ID: "appletv", Name: "Apple TV 4K",
			Containers: []string{"mp4", "m4v", "mov"}, VideoCodecs: []string{"h264", "hevc"},
			AudioCodecs: []string{"aac", "ac3", "eac3", "alac", "mp3"},
			HDR:         []string{"hdr10", "hlg", "dv5"}, TenBit: true, HVC1Only: true, MaxHeight: 2160, SwitchAudio: true,
			EmbeddedSubtitles: []string{"mov_text"},
			Remux:             true, HLSVideo: []string{"h264", "hevc"}, HLSAudio: []string{"aac", "ac3", "eac3"}, FMP4Video: []string{"hevc"}, HLSSubtitles: true,
		},
		{
			ID: "appletv-hd", Name: "Apple TV HD",
			Containers: []string{"mp4", "m4v", "mov"}, VideoCodecs: []string{"h264"},
			AudioCodecs: []string{"aac", "ac3", "eac3", "alac", "mp3"},
			HDR:         []string{}, MaxHeight: 1080, SwitchAudio: true,
			EmbeddedSubtitles: []string{"mov_text"},
			Remux:             true, HLSVideo: []string{"h264"}, HLSAudio: []string{"aac", "ac3", "eac3"}, FMP4Video: []string{}, HLSSubtitles: true,
		},
		{
			ID: "androidtv", Name: "Android TV",
			Containers: []string{"mp4", "m4v", "mov", "mkv", "webm", "ts"}, VideoCodecs: []string{"h264", "hevc", "vp9"},
			AudioCodecs: []string{"aac", "mp3", "flac", "opus", "vorbis"},
			HDR:         []string{}, TenBit: true, MaxHeight: 2160, SwitchAudio: true,
			EmbeddedSubtitles: []string{"subrip", "ass", "ssa", "webvtt", "mov_text", "hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle"},
			SidecarSubtitles:  true,
			Remux:             true, HLSVideo: []string{"h264", "hevc"}, HLSAudio: []string{"aac", "mp3"}, FMP4Video: []string{"hevc"}, HLSSubtitles: true,
		},
	}
}

// Find returns a built-in profile by id.
func Find(id string) (Profile, bool) {
	for _, p := range Profiles() {
		if p.ID == id {
			return p, true
		}
	}
	return Profile{}, false
}

// Capabilities are what an app found out about its own device. A field left
// out keeps the profile's value.
type Capabilities struct {
	Containers        []string `json:"containers"`
	VideoCodecs       []string `json:"videoCodecs"`
	AudioCodecs       []string `json:"audioCodecs"`
	HDR               []string `json:"hdr"`
	TenBit            *bool    `json:"tenBit"`
	MaxHeight         *int     `json:"maxHeight"`
	MaxBitrateKbps    *int     `json:"maxBitrateKbps"`
	MaxAudioChannels  *int     `json:"maxAudioChannels"`
	EmbeddedSubtitles []string `json:"embeddedSubtitles"`
	SidecarSubtitles  *bool    `json:"sidecarSubtitles"`
	HLSAudio          []string `json:"hlsAudio"`
	HLSVideo          []string `json:"hlsVideo"`
}

// Apply returns the profile with the capabilities filled in. Everything a
// client sends is normalised: short lower-case names and sane numbers only.
func (c *Capabilities) Apply(p Profile) Profile {
	if c == nil {
		return p
	}
	list := func(dst *[]string, src []string) {
		if src != nil {
			*dst = names(src)
		}
	}
	list(&p.Containers, c.Containers)
	list(&p.VideoCodecs, c.VideoCodecs)
	list(&p.AudioCodecs, c.AudioCodecs)
	list(&p.HDR, c.HDR)
	list(&p.EmbeddedSubtitles, c.EmbeddedSubtitles)
	list(&p.HLSAudio, c.HLSAudio)
	list(&p.HLSVideo, c.HLSVideo)
	if c.TenBit != nil {
		p.TenBit = *c.TenBit
	}
	if c.SidecarSubtitles != nil {
		p.SidecarSubtitles = *c.SidecarSubtitles
	}
	num := func(dst *int, src *int, max int) {
		if src != nil && *src >= 0 && *src <= max {
			*dst = *src
		}
	}
	num(&p.MaxHeight, c.MaxHeight, 8640)
	num(&p.MaxBitrateKbps, c.MaxBitrateKbps, 1000000)
	num(&p.MaxAudioChannels, c.MaxAudioChannels, 16)
	return p
}

// names keeps up to 32 plausible names, lower-cased.
func names(in []string) []string {
	out := make([]string, 0, min(len(in), 32))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" || len(s) > 32 || len(out) == 32 {
			continue
		}
		ok := true
		for _, c := range s {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}

// Audio is one sound track of a file.
type Audio struct {
	Codec    string
	Channels int
	Language string
	Title    string
	Default  bool
	Comment  bool // commentary, never chosen by itself
}

// Subtitle is one subtitle track of a file.
type Subtitle struct {
	Codec    string
	Language string
	Title    string
	Default  bool
	Forced   bool
	SDH      bool
}

// Source describes a file, from the media probe.
type Source struct {
	Container   string // file extension without the dot
	HasVideo    bool
	VideoCodec  string
	VideoTag    string // hvc1, hev1, avc1...
	Height      int
	BitDepth    int
	Range       string // sdr | hdr10 | hlg | dv5
	Interlaced  bool
	BitrateKbps int
	Audio       []Audio
	Subtitles   []Subtitle
}

// Request is what the viewer chose.
type Request struct {
	AudioIndex     int
	SubtitleIndex  int // -1 = off
	MaxHeight      int // the lower of the client's wish and the server's setting; 0 = none
	MaxBitrateKbps int
	AllowDirect    bool // the server's Direct Play setting
	ForceTranscode bool
	BurnSubtitle   bool // the client wants this subtitle in the picture (styled ASS)
}

// Decision is how the file will play.
type Decision struct {
	Mode    string   `json:"mode"`
	Reasons []string `json:"reasons"` // why not a simpler mode; empty for direct
	// For HLS: what is copied, and how segments are stored.
	CopyVideo bool   `json:"copyVideo"`
	CopyAudio bool   `json:"copyAudio"`
	FMP4      bool   `json:"fmp4"`
	Subtitle  string `json:"subtitle"` // one of the Sub* delivery values
	// ToneMap: HDR converted to SDR because the display can't show it.
	ToneMap bool `json:"toneMap"`
	// AudioChannels is the channel count of converted sound: 2 or 6.
	AudioChannels int `json:"audioChannels"`
}

// Decide applies the rules. Simplest first: play the file as it is; else
// rewrap it with the picture untouched; else convert it.
func Decide(p Profile, src Source, q Request) Decision {
	d := Decision{Reasons: []string{}, Subtitle: SubNone, AudioChannels: 2}
	var a *Audio
	if q.AudioIndex >= 0 && q.AudioIndex < len(src.Audio) {
		a = &src.Audio[q.AudioIndex]
	}
	var sub *Subtitle
	if q.SubtitleIndex >= 0 && q.SubtitleIndex < len(src.Subtitles) {
		sub = &src.Subtitles[q.SubtitleIndex]
	}
	maxH := Smallest(p.MaxHeight, q.MaxHeight)
	maxKbps := Smallest(p.MaxBitrateKbps, q.MaxBitrateKbps)
	if a != nil && a.Channels > 2 && (p.MaxAudioChannels == 0 || p.MaxAudioChannels >= 6) {
		d.AudioChannels = 6
	}

	// What stops the picture being passed on, whatever the wrapping.
	var picture []string
	if src.HasVideo {
		picture = pictureProblems(p, src, maxH, maxKbps)
	}
	var direct []string
	add := func(list *[]string, cond bool, r string) {
		if cond {
			*list = append(*list, r)
		}
	}
	if !src.HasVideo {
		direct = append(direct, ReasonNoVideo)
	}
	add(&direct, q.ForceTranscode, ReasonForced)
	add(&direct, !q.AllowDirect, ReasonDirectOff)
	add(&direct, !slices.Contains(p.Containers, src.Container), ReasonContainer)
	add(&direct, src.HasVideo && !slices.Contains(p.VideoCodecs, src.VideoCodec), ReasonVideoCodec)
	add(&direct, p.HVC1Only && src.VideoCodec == "hevc" && src.VideoTag != "hvc1", ReasonVideoTag)
	direct = append(direct, picture...)
	if a != nil {
		add(&direct, !slices.Contains(p.AudioCodecs, a.Codec), ReasonAudioCodec)
		add(&direct, p.MaxAudioChannels > 0 && a.Channels > p.MaxAudioChannels, ReasonAudioChans)
	}
	add(&direct, q.AudioIndex != 0 && !p.SwitchAudio, ReasonAudioTrack)

	// Subtitles: how each way of playing could show the chosen track.
	directSub, hlsSub := SubNone, SubNone
	if sub != nil {
		text := IsTextSubtitle(sub.Codec)
		switch {
		case q.BurnSubtitle:
			directSub, hlsSub = SubBurn, SubBurn
		default:
			directSub = SubBurn
			if slices.Contains(p.EmbeddedSubtitles, sub.Codec) {
				directSub = SubEmbedded
			} else if text && p.SidecarSubtitles {
				directSub = SubSidecar
			}
			hlsSub = SubBurn
			if text && p.HLSSubtitles {
				hlsSub = SubHLS
			} else if text && p.SidecarSubtitles {
				hlsSub = SubSidecar
			}
		}
	}
	add(&direct, directSub == SubBurn, ReasonSubtitle)
	if q.BurnSubtitle && sub != nil {
		direct = append(direct, ReasonBurnAsked)
	}

	if len(direct) == 0 {
		d.Mode, d.Subtitle = Direct, directSub
		return d
	}
	d.Reasons = direct

	// Rewrap: the picture goes through unchanged.
	if !src.HasVideo || q.ForceTranscode || hlsSub == SubBurn || len(picture) > 0 || !p.Remux || !slices.Contains(p.HLSVideo, src.VideoCodec) {
		if src.HasVideo && !p.Remux {
			d.Reasons = append(d.Reasons, ReasonRemuxDisable)
		} else if src.HasVideo && !slices.Contains(p.HLSVideo, src.VideoCodec) {
			d.Reasons = append(d.Reasons, ReasonHLSVideo)
		}
		d.Mode, d.Subtitle = Transcode, hlsSub
		d.ToneMap = src.Range != "" && src.Range != "sdr" && !slices.Contains(p.HDR, src.Range)
		return d
	}
	d.Mode, d.Subtitle, d.CopyVideo = Remux, hlsSub, true
	d.CopyAudio = a == nil || (slices.Contains(p.HLSAudio, a.Codec) && (p.MaxAudioChannels == 0 || a.Channels <= p.MaxAudioChannels))
	d.FMP4 = slices.Contains(p.FMP4Video, src.VideoCodec)
	return d
}

// pictureProblems: reasons the video stream can't reach this client as it is.
func pictureProblems(p Profile, src Source, maxH, maxKbps int) []string {
	var out []string
	if maxH > 0 && src.Height > maxH {
		out = append(out, ReasonVideoHeight)
	}
	if maxKbps > 0 && src.BitrateKbps > maxKbps {
		out = append(out, ReasonVideoBitrate)
	}
	if src.BitDepth > 8 && (src.VideoCodec == "h264" && !p.HiTenH264 || src.VideoCodec != "h264" && !p.TenBit) {
		out = append(out, ReasonVideoDepth)
	}
	if src.Range != "" && src.Range != "sdr" && !slices.Contains(p.HDR, src.Range) {
		out = append(out, ReasonVideoRange)
	}
	if src.Interlaced && !p.Interlaced {
		out = append(out, ReasonInterlaced)
	}
	return out
}

// Smallest is the lower of two limits, where 0 means no limit.
func Smallest(a, b int) int {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	}
	return min(a, b)
}

// Chosen says which tracks play when the client didn't name any.
type Chosen struct{ Audio, Subtitle int }

// ChooseTracks picks the default tracks, the same way for every client:
//
//   - sound: the wanted language, else the track the file marks as default,
//     else the first; commentary only if there's nothing else;
//   - subtitles: the wanted language; else, with none wanted, the forced track
//     in the language being heard (foreign dialogue in an otherwise understood
//     film); else off.
func ChooseTracks(src Source, audioLang, subLang string) Chosen {
	c := Chosen{Subtitle: -1}
	pickAudio := func(ok func(*Audio) bool) bool {
		for i := range src.Audio {
			if !src.Audio[i].Comment && ok(&src.Audio[i]) {
				c.Audio = i
				return true
			}
		}
		return false
	}
	if audioLang == "" || !pickAudio(func(a *Audio) bool { return SameLanguage(a.Language, audioLang) }) {
		if !pickAudio(func(a *Audio) bool { return a.Default }) {
			pickAudio(func(*Audio) bool { return true })
		}
	}
	heard := ""
	if c.Audio < len(src.Audio) {
		heard = src.Audio[c.Audio].Language
	}
	if subLang != "" {
		for i, s := range src.Subtitles {
			if SameLanguage(s.Language, subLang) && !s.Forced && !s.SDH {
				c.Subtitle = i
				return c
			}
		}
		for i, s := range src.Subtitles {
			if SameLanguage(s.Language, subLang) {
				c.Subtitle = i
				return c
			}
		}
	}
	for i, s := range src.Subtitles {
		if s.Forced && (heard == "" || SameLanguage(s.Language, heard)) {
			c.Subtitle = i
			return c
		}
	}
	return c
}

// languages maps the three-letter codes files carry to two-letter ones, so
// "eng", "en" and "en-US" all count as English.
var languages = map[string]string{
	"eng": "en", "dut": "nl", "nld": "nl", "fre": "fr", "fra": "fr", "ger": "de", "deu": "de", "spa": "es", "ita": "it", "por": "pt",
	"jpn": "ja", "chi": "zh", "zho": "zh", "kor": "ko", "rus": "ru", "swe": "sv", "dan": "da", "nor": "no", "nob": "no", "nno": "no",
	"fin": "fi", "pol": "pl", "cze": "cs", "ces": "cs", "hun": "hu", "gre": "el", "ell": "el", "tur": "tr", "heb": "he", "ara": "ar",
	"hin": "hi", "tha": "th", "rum": "ro", "ron": "ro", "ukr": "uk", "vie": "vi", "ind": "id", "cat": "ca",
}

// NormalizeLanguage is a language tag reduced to its short base code.
func NormalizeLanguage(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexAny(tag, "-_"); i > 0 {
		tag = tag[:i]
	}
	if s, ok := languages[tag]; ok {
		return s
	}
	return tag
}

// SameLanguage compares two language tags in any of the usual forms.
func SameLanguage(a, b string) bool {
	a, b = NormalizeLanguage(a), NormalizeLanguage(b)
	return a != "" && a != "und" && a == b
}

// languageNames names the short codes, for lists of tracks.
var languageNames = map[string]string{
	"en": "English", "nl": "Dutch", "fr": "French", "de": "German", "es": "Spanish", "it": "Italian", "pt": "Portuguese", "ja": "Japanese",
	"zh": "Chinese", "ko": "Korean", "ru": "Russian", "sv": "Swedish", "da": "Danish", "no": "Norwegian", "fi": "Finnish", "pl": "Polish",
	"cs": "Czech", "hu": "Hungarian", "el": "Greek", "tr": "Turkish", "he": "Hebrew", "ar": "Arabic", "hi": "Hindi", "th": "Thai",
	"ro": "Romanian", "uk": "Ukrainian", "vi": "Vietnamese", "id": "Indonesian", "ca": "Catalan",
}

// LanguageName is a language tag in English words, "" when it isn't known.
func LanguageName(tag string) string { return languageNames[NormalizeLanguage(tag)] }

// ResumeMinSec: a video started less than this long ago starts over.
const ResumeMinSec = 60
