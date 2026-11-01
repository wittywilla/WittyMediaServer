package playback

import (
	"fmt"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
)

// Explanation says in words how a file plays and why, for the player's information:
// what happens to the picture, the sound and the subtitles, and how hard the server
// works for it. It is written for people, so apps show it as it is.
type Explanation struct {
	// Summary is one sentence about the whole thing.
	Summary string `json:"summary"`
	// Source says what the file is: "HEVC 10-bit in Matroska, DTS 5.1".
	Source string `json:"source"`
	// Picture, Sound and Subtitles say what happens to each.
	Picture   string `json:"picture"`
	Sound     string `json:"sound"`
	Subtitles string `json:"subtitles,omitempty"`
	// Reasons say why it isn't simpler, one sentence each.
	Reasons []string `json:"reasons"`
	// Load is what it costs the server: none (it only sends the file), light (it copies
	// or converts sound) or heavy (it converts the picture).
	Load string `json:"load"`
}

var reasonWords = map[string]string{
	ReasonForced:       "A conversion was asked for.",
	ReasonDirectOff:    "Direct play is turned off in the server's settings.",
	ReasonContainer:    "This player can't open this kind of file, though it may play what's inside.",
	ReasonVideoCodec:   "This player can't decode the picture.",
	ReasonVideoTag:     "This player needs HEVC labelled in a different way.",
	ReasonVideoHeight:  "The picture has more lines than allowed for this player or by the server.",
	ReasonVideoBitrate: "The picture needs more data than this connection is allowed.",
	ReasonVideoDepth:   "This player can't show a picture of this colour depth.",
	ReasonVideoRange:   "This screen doesn't show HDR, so the picture is converted to look right on it.",
	ReasonInterlaced:   "The picture is interlaced, and this player wouldn't deinterlace it.",
	ReasonAudioCodec:   "This player can't decode the sound.",
	ReasonAudioChans:   "The sound has more channels than this player takes.",
	ReasonAudioTrack:   "This player can only change sound tracks if the server rewraps the file.",
	ReasonSubtitle:     "These subtitles are pictures, so the server draws them into the picture.",
	ReasonNoVideo:      "This file only has sound.",
	ReasonBurnAsked:    "Subtitles drawn into the picture were asked for, to keep their styling.",
	ReasonHLSVideo:     "This player can't take the picture as a stream without converting it.",
	ReasonRemuxDisable: "This player doesn't take rewrapped streams.",
	ReasonSplit:        "The film is split across files, which play as one stream.",
	ReasonVideoTrack:   "Another video stream of the file was chosen.",
	ReasonFlatten3D:    "A 3D picture is being shown in 2D.",
	ReasonRemoteLimit:  "The file holds more data than this server allows for viewers away from home.",
}

// ReasonText puts a reason in words.
func ReasonText(code string) string {
	if t, ok := reasonWords[code]; ok {
		return t
	}
	return code
}

func codecName(id string) string {
	if c := mediatypes.CodecByID(id); c != nil {
		return c.Name
	}
	return strings.ToUpper(id)
}

func containerName(id string) string {
	if f := mediatypes.ByID(id); f != nil {
		return f.Name
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
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	}
	return fmt.Sprintf("%d channels", n)
}

func pictureWords(src Source) string {
	parts := []string{codecName(src.VideoCodec)}
	if src.BitDepth > 8 {
		parts = append(parts, fmt.Sprintf("%d-bit", src.BitDepth))
	}
	if src.Height > 0 {
		parts = append(parts, fmt.Sprintf("%dp", src.Height))
	}
	switch {
	case src.Dynamic != "":
		parts = append(parts, src.Dynamic)
	case src.Range == "hdr10":
		parts = append(parts, "HDR10")
	case src.Range == "hlg":
		parts = append(parts, "HLG")
	case src.Range == "dv5":
		parts = append(parts, "Dolby Vision")
	}
	if src.Interlaced {
		parts = append(parts, "interlaced")
	}
	if src.Stereo != "" {
		parts = append(parts, "3D")
	}
	return strings.Join(parts, " ")
}

// Explain words a decision.
func Explain(d Decision, src Source, q Request) Explanation {
	e := Explanation{Reasons: []string{}}
	for _, r := range d.Reasons {
		if t := ReasonText(r); t != "" {
			e.Reasons = append(e.Reasons, t)
		}
	}
	var a *Audio
	if q.AudioIndex >= 0 && q.AudioIndex < len(src.Audio) {
		a = &src.Audio[q.AudioIndex]
	}
	soundWords := "no sound"
	if a != nil {
		soundWords = strings.TrimSpace(codecName(a.Codec) + " " + channelWords(a.Channels))
	}
	if src.HasVideo {
		e.Source = fmt.Sprintf("%s in %s, %s", pictureWords(src), containerName(src.Container), soundWords)
	} else {
		e.Source = fmt.Sprintf("%s, %s", containerName(src.Container), soundWords)
	}
	switch d.Mode {
	case Direct:
		e.Summary = "Playing the file as it is: your player does the work, and the server only sends it."
		e.Picture = "Passed on untouched."
		e.Sound = "Passed on untouched."
		e.Load = "none"
	case Remux:
		e.Picture = "Passed on untouched, in a wrapper this player can open."
		if d.CopyAudio {
			e.Summary = "Rewrapping the file: nothing is converted, only the wrapper changes. This costs the server very little."
			e.Sound = "Passed on untouched."
		} else {
			e.Summary = "Converting only the sound, which costs the server little. The picture is passed on untouched."
			e.Sound = fmt.Sprintf("Converted to AAC %s.", channelWords(d.AudioChannels))
		}
		e.Load = "light"
	default:
		e.Summary = "Converting the picture and sound. This is the most work for the server, so it's only done when nothing simpler would play."
		e.Picture = "Converted to H.264."
		if !src.HasVideo {
			e.Summary = "Converting the sound."
			e.Picture = "There's no picture in this file."
		}
		if d.ToneMap {
			e.Picture = "Converted to H.264, with HDR mapped to fit this screen."
			if src.Range == "dv5" {
				e.Picture = "Converted to H.264. This Dolby Vision profile has no plain HDR layer, so colours may look a little off."
			}
		}
		e.Sound = fmt.Sprintf("Converted to AAC %s.", channelWords(d.AudioChannels))
		e.Load = "heavy"
		if !src.HasVideo {
			e.Load = "light"
		}
	}
	if a == nil {
		e.Sound = "There's no sound in this file."
	}
	switch d.Subtitle {
	case SubNone:
	case SubEmbedded:
		e.Subtitles = "Drawn by the player, from inside the file."
	case SubSidecar:
		e.Subtitles = "Sent beside the picture as text, and drawn by the player."
	case SubHLS:
		e.Subtitles = "Sent as a text track of the stream, and drawn by the player."
	case SubBurn:
		e.Subtitles = "Drawn into the picture by the server."
	}
	return e
}

// ExplainAudio words what happens to a sound file.
func ExplainAudio(direct bool, container, codec string) Explanation {
	source := fmt.Sprintf("%s in %s", codecName(codec), containerName(container))
	if direct {
		return Explanation{Summary: "Playing the file as it is.", Source: source, Sound: "Passed on untouched.", Reasons: []string{}, Load: "none"}
	}
	why := ReasonText(ReasonAudioCodec)
	if codec == "midi" {
		return Explanation{Summary: "The server's synthesiser plays this file once, and keeps the sound.", Source: source, Sound: "Made into FLAC.", Reasons: []string{"A MIDI file holds notes rather than sound."}, Load: "light"}
	}
	return Explanation{Summary: "Converting the sound on the way to your player.", Source: source, Sound: "Converted to AAC.", Reasons: []string{why}, Load: "light"}
}
