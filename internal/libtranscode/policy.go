// Package libtranscode converts the files of a library ahead of time, so they play
// without being converted later. That is a different job from live playback conversion
// (internal/hls): this one rewrites people's files, so everything about it leans towards
// keeping them safe. A file is only replaced after its new version has been checked.
//
// The planning (policy.go, source.go, plan.go, args.go) is plain logic that any rule can
// be tested on without FFmpeg. engine.go runs the plans. docs/LIBRARY-TRANSCODING.md
// explains it for people, tdarr/ connects to a separately installed Tdarr server.
package libtranscode

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/playback"
)

// What a run does with the file it was given.
const (
	OutputKeep    = "keep"    // the new file takes the name; the old one moves to a hidden folder beside it
	OutputReplace = "replace" // the new file takes the name; the old one is deleted once it has been checked
	OutputBeside  = "beside"  // the old file stays in use; the new one is written next to it
)

// Keep values for tracks.
const (
	KeepAll       = "all"
	KeepLanguages = "languages"
	KeepNone      = "none"
)

// Yes-or-no choices about a kind of track.
const (
	Keep = "keep"
	Drop = "drop"
)

// Presets.
const (
	PresetCompatible = "compatible"
	PresetSaveSpace  = "save-space"
	PresetBest       = "best-quality"
	PresetCustom     = "custom"
)

// VideoPolicy says what becomes of the picture.
type VideoPolicy struct {
	// Codec: "keep" never converts the picture. Otherwise h264, hevc or av1.
	Codec string `json:"codec"`
	// Accept: other codecs left as they are rather than converted to Codec.
	Accept []string `json:"accept"`
	// Quality is on the familiar constant-quality scale: 18 looks like the source, 23 is
	// good, 28 is small. Each encoder is told its own equivalent.
	Quality int `json:"quality"`
	// Speed: faster | balanced | slower.
	Speed string `json:"speed"`
	// MaxHeight: taller pictures are scaled down. 0 keeps the size.
	MaxHeight int `json:"maxHeight"`
	// HDR: keep | tonemap (to an ordinary picture) | refuse (leave these files alone).
	HDR string `json:"hdr"`
	// DolbyVision: keep (only by leaving the picture untouched, else the file is left
	// alone) | base (convert from the layer other players show, losing Dolby Vision).
	DolbyVision string `json:"dolbyVision"`
	// EightBit: 10-bit and deeper pictures become 8-bit.
	EightBit bool `json:"eightBit"`
	// Deinterlace: an interlaced picture is deinterlaced, which also means converted.
	Deinterlace bool `json:"deinterlace"`
	// ConstantRate: a variable frame rate becomes a constant one.
	ConstantRate bool `json:"constantRate"`
	// ThreeD: keep (as stored) | flatten (one eye) | refuse.
	ThreeD string `json:"threeD"`
	// ExtraVideo: other picture streams beside the main one: keep | drop.
	ExtraVideo string `json:"extraVideo"`
	// MinSavingPercent: the new file must be this much smaller or the old one stays.
	MinSavingPercent int `json:"minSavingPercent"`
	// SkipBelowKbps: 1080p pictures below this are left alone (scaled for other sizes),
	// because converting them again would lose more than it saves.
	SkipBelowKbps int `json:"skipBelowKbps"`
}

// AudioPolicy says which sound tracks stay, in what order, and in what form.
type AudioPolicy struct {
	// Keep: all, or languages (those in Languages and, if asked, the original).
	Keep string `json:"keep"`
	// Languages, in the order wanted.
	Languages []string `json:"languages"`
	// KeepOriginal keeps the language the film was made in.
	KeepOriginal bool `json:"keepOriginal"`
	// Untagged: tracks with no language: keep | drop.
	Untagged string `json:"untagged"`
	// UntaggedAs labels those tracks with this language.
	UntaggedAs string `json:"untaggedAs"`
	// Commentary and Description (audio description): keep | drop.
	Commentary  string `json:"commentary"`
	Description string `json:"description"`
	// Default: the language that plays first. "" keeps what the file says.
	Default string `json:"default"`
	// Codec: "keep" copies every track. aac, ac3, eac3 or opus converts what isn't in
	// Accept. A track is always converted if the container can't hold it.
	Codec string `json:"codec"`
	// Accept: codecs left as they are.
	Accept []string `json:"accept"`
	// Lossless (TrueHD, DTS-HD MA, FLAC, PCM...): keep | convert.
	Lossless string `json:"lossless"`
	// Objects (Atmos, DTS:X): keep | convert. Converting loses the objects.
	Objects string `json:"objects"`
	// MaxChannels: 0 keeps them. Otherwise wider tracks are converted down.
	MaxChannels int `json:"maxChannels"`
	// AddStereo adds a stereo track beside a surround one that has none. StereoFirst
	// puts it ahead of the surround track, so plain devices start with it.
	AddStereo   bool `json:"addStereo"`
	StereoFirst bool `json:"stereoFirst"`
}

// SubtitlePolicy says which subtitles stay and how.
type SubtitlePolicy struct {
	// Keep: all, languages or none.
	Keep      string   `json:"keep"`
	Languages []string `json:"languages"`
	// Forced (only foreign dialogue) tracks: keep | drop | burn (into the picture, which
	// means converting the picture) | burn-images (only those that are stored as pictures,
	// which can't stay in every container; text ones stay text).
	Forced string `json:"forced"`
	// SDH (with sound descriptions for people who can't hear): keep | drop.
	SDH string `json:"sdh"`
	// Untagged: keep | drop.
	Untagged string `json:"untagged"`
	// Images are subtitles stored as pictures (Blu-ray PGS, DVD). They can't become text
	// without character recognition, which this doesn't do. keep | drop | export (Blu-ray
	// ones to a .sup file beside the video when the output can't hold them).
	Images string `json:"images"`
	// Text: keep as they are, or srt (plain text, losing styling). A container that can't
	// hold a format gets its own, whatever this says.
	Text string `json:"text"`
	// Sidecars are subtitle files beside the video: leave | embed.
	Sidecars string `json:"sidecars"`
}

// Policy is a complete recipe for converting a file.
type Policy struct {
	Preset string `json:"preset"`
	// Container: mkv, mp4 or keep (a file already in MKV or MP4 stays so, others go to MKV).
	Container string         `json:"container"`
	Video     VideoPolicy    `json:"video"`
	Audio     AudioPolicy    `json:"audio"`
	Subtitles SubtitlePolicy `json:"subtitles"`
}

// PresetInfo is what a person reads when choosing.
type PresetInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Summary string   `json:"summary"`
	Details []string `json:"details"`
	Cost    string   `json:"cost"`
	Policy  Policy   `json:"policy"`
}

// Presets are the three plain choices. Their details are the place to see exactly what
// each does, and a person can change any of it, which makes it custom.
func Presets() []PresetInfo {
	return []PresetInfo{
		{
			ID: PresetCompatible, Name: "Make everything play on any device",
			Summary: "Files become MP4 with H.264 picture and AAC sound, which every phone, TV, tablet and browser plays without the server converting anything.",
			Details: []string{
				"Pictures taller than 1080p are made 1080p. HDR becomes an ordinary picture.",
				"A stereo AAC track comes first, and a surround track stays beside it where one exists.",
				"Text subtitles are kept. Subtitles stored as pictures can't go in an MP4: forced ones are burned in, and Blu-ray ones are saved as a file beside the video.",
				"Files that already fit are left alone.",
			},
			Cost: "Heavy work for the processor. Files may grow if they were very efficient to begin with.",
			Policy: Policy{
				Preset: PresetCompatible, Container: "mp4",
				Video: VideoPolicy{Codec: "h264", Quality: 21, Speed: "balanced", MaxHeight: 1080, HDR: "tonemap", DolbyVision: "base", EightBit: true, Deinterlace: true, ConstantRate: true, ThreeD: Keep, ExtraVideo: Drop},
				Audio: AudioPolicy{Keep: KeepAll, KeepOriginal: true, Untagged: Keep, Commentary: Keep, Description: Keep, Codec: "aac", Accept: []string{"aac", "ac3", "eac3"}, Lossless: "convert", Objects: "keep", MaxChannels: 6, AddStereo: true, StereoFirst: true},
				Subtitles: SubtitlePolicy{Keep: KeepAll, Forced: "burn-images", SDH: Keep, Untagged: Keep, Images: "export", Text: Keep, Sidecars: "leave"},
			},
		},
		{
			ID: PresetSaveSpace, Name: "Save space",
			Summary: "Pictures are converted to a much more efficient format, usually taking 30 to 60% less room for a picture that looks the same to most people.",
			Details: []string{
				"Uses H.265 (HEVC) when this server's FFmpeg can, otherwise a higher-compression H.264.",
				"HDR stays HDR. Dolby Vision files are left as they are.",
				"Lossless sound such as TrueHD or DTS-HD MA becomes Dolby Digital Plus. Other sound is left alone.",
				"Every track and subtitle stays, and files already in an efficient format are left alone.",
				"If a new file doesn't come out at least 10% smaller, the old one stays.",
			},
			Cost: "The slowest choice for the processor. Older devices may need the server to convert these files while playing.",
			Policy: Policy{
				Preset: PresetSaveSpace, Container: "keep",
				Video: VideoPolicy{Codec: "hevc", Accept: []string{"hevc", "av1", "vp9"}, Quality: 23, Speed: "balanced", HDR: Keep, DolbyVision: Keep, Deinterlace: true, ThreeD: Keep, ExtraVideo: Keep, MinSavingPercent: 10, SkipBelowKbps: 2000},
				Audio: AudioPolicy{Keep: KeepAll, KeepOriginal: true, Untagged: Keep, Commentary: Keep, Description: Keep, Codec: "eac3", Lossless: "convert", Objects: Keep},
				Subtitles: SubtitlePolicy{Keep: KeepAll, Forced: Keep, SDH: Keep, Untagged: Keep, Images: Keep, Text: Keep, Sidecars: "leave"},
			},
		},
		{
			ID: PresetBest, Name: "Keep the best quality",
			Summary: "Picture and sound aren't touched at all. Files are tidied into MKV, with the tracks you want in the order you want, and a stereo track added where only surround exists.",
			Details: []string{
				"Nothing is re-encoded, so nothing can look or sound worse, and it's quick even on small machines.",
				"Everything is kept unless you choose languages below.",
				"A file in a format no device can play still won't play: the server converts that one while it plays.",
			},
			Cost: "Very little: mostly copying, plus a short conversion for an added stereo track.",
			Policy: Policy{
				Preset: PresetBest, Container: "mkv",
				Video: VideoPolicy{Codec: Keep, HDR: Keep, DolbyVision: Keep, ThreeD: Keep, ExtraVideo: Keep},
				Audio: AudioPolicy{Keep: KeepAll, KeepOriginal: true, Untagged: Keep, Commentary: Keep, Description: Keep, Codec: Keep, Lossless: Keep, Objects: Keep, AddStereo: true},
				Subtitles: SubtitlePolicy{Keep: KeepAll, Forced: Keep, SDH: Keep, Untagged: Keep, Images: Keep, Text: Keep, Sidecars: "leave"},
			},
		},
	}
}

// PresetPolicy returns a preset's recipe.
func PresetPolicy(id string) (Policy, bool) {
	for _, p := range Presets() {
		if p.ID == id {
			return p.Policy, true
		}
	}
	return Policy{}, false
}

func choice(what, v string, allowed ...string) error {
	if slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("%s can't be %q: choose one of %s", what, v, strings.Join(allowed, ", "))
}

// languageList tidies a list a person gave: short base codes, at most 32, no repeats.
func languageList(what string, in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	if len(in) > 32 {
		return nil, fmt.Errorf("%s: at most 32 languages", what)
	}
	for _, l := range in {
		n := playback.NormalizeLanguage(l)
		if n == "" || len(n) > 8 {
			return nil, fmt.Errorf("%s: %q isn't a language code", what, l)
		}
		for _, r := range n {
			if r < 'a' || r > 'z' {
				return nil, fmt.Errorf("%s: %q isn't a language code", what, l)
			}
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out, nil
}

func codecList(what string, in []string) error {
	if len(in) > 32 {
		return fmt.Errorf("%s: too long a list", what)
	}
	for _, c := range in {
		if c == "" || len(c) > 32 {
			return fmt.Errorf("%s: %q isn't a codec", what, c)
		}
		for _, r := range c {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
				return fmt.Errorf("%s: %q isn't a codec", what, c)
			}
		}
	}
	return nil
}

// Validate checks a recipe from outside and tidies its lists. It's what stands between
// a request and FFmpeg's command line: every value ends up as one of a few known words,
// a small number or a language code.
func (p *Policy) Validate() error {
	if p.Preset != PresetCustom {
		if _, ok := PresetPolicy(p.Preset); !ok {
			return errors.New("that's not one of the presets")
		}
	}
	errs := []error{
		choice("container", p.Container, "mkv", "mp4", Keep),
		choice("video codec", p.Video.Codec, Keep, "h264", "hevc", "av1"),
		choice("video speed", orDefault(p.Video.Speed, "balanced"), "faster", "balanced", "slower"),
		choice("HDR", p.Video.HDR, Keep, "tonemap", "refuse"),
		choice("Dolby Vision", p.Video.DolbyVision, Keep, "base"),
		choice("3D", p.Video.ThreeD, Keep, "flatten", "refuse"),
		choice("other video streams", p.Video.ExtraVideo, Keep, Drop),
		choice("audio tracks", p.Audio.Keep, KeepAll, KeepLanguages),
		choice("untagged audio", p.Audio.Untagged, Keep, Drop),
		choice("commentary", p.Audio.Commentary, Keep, Drop),
		choice("audio description", p.Audio.Description, Keep, Drop),
		choice("audio codec", p.Audio.Codec, Keep, "aac", "ac3", "eac3", "opus"),
		choice("lossless audio", p.Audio.Lossless, Keep, "convert"),
		choice("object audio", p.Audio.Objects, Keep, "convert"),
		choice("subtitles", p.Subtitles.Keep, KeepAll, KeepLanguages, KeepNone),
		choice("forced subtitles", p.Subtitles.Forced, Keep, Drop, "burn", "burn-images"),
		choice("SDH subtitles", p.Subtitles.SDH, Keep, Drop),
		choice("untagged subtitles", p.Subtitles.Untagged, Keep, Drop),
		choice("picture subtitles", p.Subtitles.Images, Keep, Drop, "export"),
		choice("text subtitles", p.Subtitles.Text, Keep, "srt"),
		choice("subtitle files", p.Subtitles.Sidecars, "leave", "embed"),
		codecList("video codecs left alone", p.Video.Accept),
		codecList("audio codecs left alone", p.Audio.Accept),
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if p.Video.Speed == "" {
		p.Video.Speed = "balanced"
	}
	if p.Video.Codec != Keep && (p.Video.Quality < 14 || p.Video.Quality > 36) {
		return errors.New("video quality must be between 14 and 36")
	}
	switch h := p.Video.MaxHeight; {
	case h < 0, h > 4320:
		return errors.New("the maximum picture height must be between 0 and 4320")
	}
	if n := p.Video.MinSavingPercent; n < 0 || n > 90 {
		return errors.New("the smallest saving must be between 0 and 90%")
	}
	if n := p.Video.SkipBelowKbps; n < 0 || n > 200000 {
		return errors.New("that bitrate is out of range")
	}
	if n := p.Audio.MaxChannels; n != 0 && n != 2 && n != 6 && n != 8 {
		return errors.New("audio channels can be 2, 6, 8, or no limit")
	}
	var err error
	if p.Audio.Languages, err = languageList("audio languages", p.Audio.Languages); err != nil {
		return err
	}
	if p.Subtitles.Languages, err = languageList("subtitle languages", p.Subtitles.Languages); err != nil {
		return err
	}
	if p.Audio.Keep == KeepLanguages && len(p.Audio.Languages) == 0 && !p.Audio.KeepOriginal {
		return errors.New("choose at least one audio language, or keep the original language")
	}
	if p.Subtitles.Keep == KeepLanguages && len(p.Subtitles.Languages) == 0 {
		return errors.New("choose at least one subtitle language, or keep none")
	}
	if p.Audio.Default != "" {
		d, err := languageList("default audio language", []string{p.Audio.Default})
		if err != nil {
			return err
		}
		p.Audio.Default = d[0]
	}
	if p.Audio.UntaggedAs != "" {
		d, err := languageList("language for untagged audio", []string{p.Audio.UntaggedAs})
		if err != nil {
			return err
		}
		p.Audio.UntaggedAs = d[0]
	}
	return nil
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// ValidOutput checks an output mode.
func ValidOutput(v string) bool {
	return v == OutputKeep || v == OutputReplace || v == OutputBeside
}
