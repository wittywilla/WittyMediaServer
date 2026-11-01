package libtranscode

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// Verdicts: what happens to a file as a whole.
const (
	VerdictConvert = "convert" // something is encoded again
	VerdictRemux   = "remux"   // only rewrapped or tidied: nothing can look or sound worse
	VerdictSkip    = "skip"    // already matches, or nothing here for this recipe to do
	VerdictRefuse  = "refuse"  // left alone on purpose, with a reason
)

// What happens to one track.
const (
	ActKeep    = "keep"
	ActConvert = "convert"
	ActDrop    = "drop"
	ActAdd     = "add"
	ActBurn    = "burn"
	ActExport  = "export"
	ActEmbed   = "embed"
	ActLeave   = "leave"
)

// Env is what the machine offers.
type Env struct {
	Encoders  map[string]bool
	HasFilter func(string) bool
	// Hardware: hardware encoders the owner allows and a test encode has shown to work.
	Hardware []string
	// Sidecars: files may be written beside the video (Save next to media).
	Sidecars bool
	// Threads for an encoder. 0 lets FFmpeg decide.
	Threads int
	// ToneMap is the filter chain that makes HDR ordinary, "" when this FFmpeg can't.
	ToneMap string
}

func (e Env) filter(name string) bool { return e.HasFilter != nil && e.HasFilter(name) }

// Step is what happens to one track, in words for the preview, plus what the command
// needs. Steps of kept tracks are in the order the new file will have them.
type Step struct {
	Kind   string `json:"kind"`   // video, cover, audio, subtitle, attachment, data, file
	Action string `json:"action"` // keep, convert, drop, add, burn, export, embed, leave
	From   string `json:"from"`
	To     string `json:"to,omitempty"`
	Why    string `json:"why,omitempty"`

	input       int    // which input
	index       int    // the stream's number in that input
	subNumber   int    // for burning: which subtitle of the file
	encoder     string // "" copies
	args        []string
	filters     []string
	inputArgs   []string
	channels    int
	bitrate     string
	language    string
	title       string
	disposition string
	cfr         float64
	tag         string
	path        string // export: a suffix for the file; embed: the file
	text        bool
	relabel     bool // the language tag changes
}

// Plan is a decision about one file.
type Plan struct {
	Verdict   string   `json:"verdict"`
	Reason    string   `json:"reason"`
	Container string   `json:"container"` // mkv, mp4
	Ext       string   `json:"ext"`       // of the new file, without the dot
	Steps     []Step   `json:"steps"`
	Warnings  []string `json:"warnings"`
	Notes     []string `json:"notes"`
	Encoder   string   `json:"encoder,omitempty"`
	// Estimates, for this file. See estimate.go.
	EstBytes int64   `json:"estBytes"`
	EstSec   float64 `json:"estSec"`

	copyVideo bool
	exports   []Step
	embeds    []Step
	burn      *Step
	minSaving int
}

// PlanOptions change how a file is decided on.
type PlanOptions struct {
	// ForceVideo: the picture is converted even if it fits, because another part of the
	// same split film has to be. They'll be joined when played.
	ForceVideo bool
}

func refuse(reason string) *Plan {
	return &Plan{Verdict: VerdictRefuse, Reason: reason, Steps: []Step{}, Warnings: []string{}, Notes: []string{}}
}

// Decide makes the plan. It reads and runs nothing.
func Decide(src *Source, p Policy, env Env, opt PlanOptions) *Plan {
	if len(src.Video) == 0 {
		return &Plan{Verdict: VerdictSkip, Reason: "This file has no picture. Sound-only files are left as they are, because these recipes are about video.", Steps: []Step{}, Warnings: []string{}, Notes: []string{}}
	}
	pl := &Plan{Verdict: VerdictSkip, Steps: []Step{}, Warnings: []string{}, Notes: []string{}, minSaving: p.Video.MinSavingPercent}
	pl.Container = containerFor(src, p)
	subs := decideSubs(src, p, env, pl)
	if subs.keepInMKV && pl.Container == "mp4" {
		pl.Container = "mkv"
		pl.Notes = append(pl.Notes, "This file stays an MKV: it has subtitles stored as pictures, which an MP4 can't hold, and you chose to keep them.")
		subs = decideSubs(src, p, env, &Plan{Container: "mkv", Notes: []string{}, Warnings: []string{}})
	}
	if subs.burn != nil && p.Video.Codec == Keep {
		pl.Notes = append(pl.Notes, "A forced subtitle stays a track rather than being burned in, because burning means converting the picture and this recipe leaves the picture alone.")
		subs.addBack()
	}
	if !decideVideo(src, p, env, pl, subs.burn, opt) {
		return pl
	}
	decideCovers(src, pl)
	audio := decideAudio(src, p, pl)
	pl.Steps = append(pl.Steps, audio...)
	pl.Steps = append(pl.Steps, subs.steps...)
	pl.burn = subs.burn
	pl.exports = subs.exports
	pl.embeds = subs.embeds
	if subs.burn != nil {
		pl.Steps = append(pl.Steps, *subs.burn)
	}
	pl.Steps = append(pl.Steps, subs.exports...)
	pl.Steps = append(pl.Steps, subs.embeds...)
	pl.Steps = append(pl.Steps, subs.leave...)
	decideRest(src, pl)
	pl.Ext = outputExt(src, pl.Container)
	finish(src, p, pl)
	estimate(src, pl)
	return pl
}

// containerFor applies the choice of container. It may still change for a reason given.
func containerFor(src *Source, p Policy) string {
	switch p.Container {
	case "mp4", "mkv":
		return p.Container
	}
	switch src.Ext {
	case "mp4", "m4v":
		return "mp4"
	}
	return "mkv"
}

func outputExt(src *Source, container string) string {
	if container == "mp4" {
		if src.Ext == "m4v" {
			return "m4v"
		}
		return "mp4"
	}
	return "mkv"
}

// What each container can hold.
var (
	mp4Video = []string{"h264", "hevc", "av1", "vp9", "mpeg4", "mpeg2video", "mjpeg"}
	mp4Audio = []string{"aac", "mp3", "ac3", "eac3", "alac", "opus", "flac", "mp2"}
	mkvSubs  = []string{"subrip", "ass", "ssa", "webvtt", "hdmv_pgs_subtitle", "dvd_subtitle", "dvb_subtitle", "text"}
)

func containerHoldsVideo(c, codec string) bool {
	return c != "mp4" || slices.Contains(mp4Video, codec)
}

func containerHoldsAudio(c, codec string) bool {
	return c != "mp4" || slices.Contains(mp4Audio, codec)
}

// ------------------------------------------------------------------ picture

// decideVideo returns false when the file as a whole is refused.
func decideVideo(src *Source, p Policy, env Env, pl *Plan, burn *Step, opt PlanOptions) bool {
	main := src.Main()
	order := []int{main}
	for i := range src.Video {
		if i != main {
			order = append(order, i)
		}
	}
	allCopied := true
	for rank, i := range order {
		v := src.Video[i]
		if rank > 0 && p.Video.ExtraVideo == Drop {
			pl.Steps = append(pl.Steps, Step{Kind: "video", Action: ActDrop, From: v.describe(), Why: "Only the main picture stays. Most players only ever show that one."})
			continue
		}
		var b *Step
		if rank == 0 {
			b = burn
		}
		st, reason := decideOneVideo(src, v, p, env, b, opt.ForceVideo, pl)
		if reason != "" {
			*pl = *refuse(reason)
			return false
		}
		if rank > 0 && st.Action != ActDrop && len(src.Video) > 1 {
			st.Why = strings.TrimSpace("Another picture stream of this file. " + st.Why)
		}
		if st.encoder != "" {
			allCopied = false
		}
		pl.Steps = append(pl.Steps, st)
	}
	pl.copyVideo = allCopied
	return true
}

// decideOneVideo says what becomes of one picture stream; a reason means the whole file
// is left alone.
func decideOneVideo(src *Source, v VideoStream, p Policy, env Env, burn *Step, force bool, pl *Plan) (Step, string) {
	st := Step{Kind: "video", Action: ActKeep, From: v.describe(), input: 0, index: v.Index, language: v.Language}
	if p.Video.ThreeD == "refuse" && (v.Stereo != "" || v.MVC) {
		return st, "This is a 3D file, and you chose to leave those alone."
	}
	if p.Video.HDR == "refuse" && (v.Range != "sdr" || v.DV) {
		return st, "This is an HDR file, and you chose to leave those alone."
	}
	// What says the picture has to be converted.
	var needs []string
	if p.Video.Codec != Keep {
		accepted := v.Codec == p.Video.Codec || slices.Contains(p.Video.Accept, v.Codec)
		if !accepted {
			needs = append(needs, codecName(v.Codec)+" isn't the format you chose")
		}
		if !containerHoldsVideo(pl.Container, v.Codec) {
			needs = append(needs, "an MP4 can't hold "+codecName(v.Codec))
		}
		if p.Video.MaxHeight > 0 && v.Height > p.Video.MaxHeight {
			needs = append(needs, fmt.Sprintf("it's taller than %dp", p.Video.MaxHeight))
		}
		if p.Video.EightBit && (v.Depth > 8 || !strings.Contains(v.PixFmt, "420") && v.PixFmt != "") {
			needs = append(needs, "plain players need 8-bit 4:2:0 pictures")
		}
		if p.Video.HDR == "tonemap" && (v.Range != "sdr" || v.DV) {
			needs = append(needs, "it's HDR and you chose an ordinary picture")
		}
		if p.Video.Deinterlace && v.Interlaced {
			needs = append(needs, "it's interlaced")
		}
		if p.Video.ConstantRate && v.Variable {
			needs = append(needs, "its frame rate varies")
		}
		if p.Video.ThreeD == "flatten" && v.Stereo != "" {
			needs = append(needs, "it's 3D and you chose one eye")
		}
		if burn != nil {
			needs = append(needs, "a subtitle is being burned in")
		}
		if force {
			needs = append(needs, "the other parts of this split film are being converted")
		}
		// Converting a small file again loses more than it saves.
		if len(needs) == 1 && !accepted && p.Video.SkipBelowKbps > 0 {
			kbps := v.Kbps
			if kbps == 0 {
				kbps = src.Kbps
			}
			limit := float64(p.Video.SkipBelowKbps) * float64(v.Width*v.Height) / (1920 * 1080)
			if kbps > 0 && float64(kbps) < limit {
				pl.Notes = append(pl.Notes, "The picture is already small for its size, so converting it again would lose more than it saves. It's left as it is.")
				needs = nil
			}
		}
	} else if !containerHoldsVideo(pl.Container, v.Codec) {
		pl.Container = "mkv"
		pl.Notes = append(pl.Notes, "This file stays an MKV: an MP4 can't hold "+codecName(v.Codec)+" and this recipe doesn't convert the picture.")
	}
	if len(needs) == 0 {
		if pl.Container == "mp4" && v.Codec == "hevc" {
			st.tag = "hvc1"
		}
		st.To = "Stays exactly as it is"
		if v.DV {
			st.Why = "Dolby Vision is kept because the picture isn't touched."
		} else if v.Dynamic == "HDR10+" {
			st.Why = "HDR10+ is kept because the picture isn't touched."
		}
		return st, ""
	}
	// The picture has to be converted. Is that safe?
	if v.MVC && p.Video.ThreeD != "flatten" {
		return st, "This 3D file stores its second eye in a way conversion would lose. Choose \"one eye\" for 3D to convert it, or leave it as it is."
	}
	wantTone := p.Video.HDR == "tonemap" && (v.Range != "sdr" || v.DV)
	if v.DV {
		if v.DVProfile == 5 {
			return st, "This Dolby Vision file (profile 5) has no ordinary HDR layer to convert from, so its colours would come out wrong. It's left as it is."
		}
		if p.Video.DolbyVision == Keep && !wantTone {
			return st, "Dolby Vision doesn't survive conversion, and you chose to keep it, so this file is left as it is."
		}
	}
	if wantTone && env.ToneMap == "" {
		return st, "This HDR file can't be made an ordinary picture: this server's FFmpeg doesn't have the filters for it."
	}
	isHDR := (v.Range != "sdr" || v.DV) && !wantTone
	target := p.Video.Codec
	if isHDR && target == "h264" {
		return st, "H.264 can't carry HDR. Choose H.265 or AV1, or making it an ordinary picture, to convert this file."
	}
	depth := 8
	if !p.Video.EightBit && !wantTone && (v.Depth > 8 || isHDR) {
		depth = 10
	}
	enc, note := chooseEncoder(target, depth, isHDR, p, env, v)
	if enc.name == "" {
		return st, "This server's FFmpeg has no encoder for " + codecName(target) + ", so this file can't be converted."
	}
	if isHDR && enc.codec == "h264" {
		return st, "This server's FFmpeg has no H.265 or AV1 encoder, and H.264 can't carry HDR, so this file is left as it is."
	}
	if note != "" {
		pl.Notes = append(pl.Notes, note)
	}
	st.Action, st.encoder, st.args = ActConvert, enc.name, enc.args
	st.inputArgs = enc.inputArgs
	pl.Encoder = enc.name
	// Filters, in the order they have to go.
	if v.Interlaced && (p.Video.Deinterlace || wantTone) {
		st.filters = append(st.filters, "bwdif=mode=send_frame:parity=auto:deint=all")
	}
	if p.Video.ThreeD == "flatten" && v.Stereo != "" {
		st.filters = append(st.filters, flattenFilterFor(v))
	}
	if wantTone {
		st.filters = append(st.filters, toneChain(env.ToneMap, v))
		st.args = append(st.args, "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709")
	} else if isHDR {
		st.args = append(st.args, hdrColorArgs(v)...)
	}
	h := v.Height
	if p.Video.MaxHeight > 0 && v.Height > p.Video.MaxHeight {
		h = p.Video.MaxHeight
		st.filters = append(st.filters, fmt.Sprintf("scale=-2:%d:flags=bicubic", h))
	}
	st.filters = append(st.filters, enc.filters...)
	if p.Video.ConstantRate && v.Variable && v.FPS > 0 {
		st.cfr = min(v.FPS, 120)
	}
	if pl.Container == "mp4" && enc.codec == "hevc" {
		st.tag = "hvc1"
	}
	st.To = codecName(enc.codec) + ", " + fmt.Sprintf("%dp", h)
	if depth > 8 {
		st.To += ", 10-bit"
	}
	if isHDR {
		st.To += ", " + hdrName(v)
	} else if wantTone {
		st.To += ", ordinary picture"
	}
	st.Why = "Because " + joinReasons(needs) + "."
	// What won't survive, said before anything starts.
	if v.DV {
		pl.Warnings = append(pl.Warnings, "Dolby Vision won't survive: this converts from the layer other players show.")
	}
	if v.Dynamic == "HDR10+" && isHDR {
		pl.Warnings = append(pl.Warnings, "HDR10+ scene-by-scene brightness information won't survive. The picture stays HDR10.")
	}
	if wantTone {
		pl.Notes = append(pl.Notes, "HDR is converted to an ordinary picture. Colours and highlights won't match what an HDR screen shows.")
	}
	if v.Stereo != "" && p.Video.ThreeD == Keep {
		pl.Notes = append(pl.Notes, "This 3D picture stays stored the same way: both eyes in one frame.")
	}
	if v.Captions {
		pl.Notes = append(pl.Notes, "This picture carries closed captions inside it. They're passed on where the encoder can, but check them if you rely on them.")
	}
	if v.Variable && !p.Video.ConstantRate {
		pl.Notes = append(pl.Notes, "This file's frame rate varies, and it's kept that way so sound stays in step.")
	}
	return st, ""
}

// toneChain says outright what the source is: a file that leaves its colours unmarked in the
// stream (it may say so only in its container) otherwise stops the filter with no way to
// tell what it's converting from.
func toneChain(chain string, v VideoStream) string {
	trc := "smpte2084"
	if v.Range == "hlg" {
		trc = "arib-std-b67"
	}
	return strings.Replace(chain, "zscale=t=linear", "zscale=pin=bt2020:tin="+trc+":min=bt2020nc:t=linear", 1)
}

func joinReasons(r []string) string {
	switch len(r) {
	case 0:
		return ""
	case 1:
		return r[0]
	}
	return strings.Join(r[:len(r)-1], ", ") + " and " + r[len(r)-1]
}

func hdrName(v VideoStream) string {
	if v.Range == "hlg" {
		return "HLG"
	}
	return "HDR10"
}

func hdrColorArgs(v VideoStream) []string {
	trc := "smpte2084"
	if v.Range == "hlg" {
		trc = "arib-std-b67"
	}
	return []string{"-color_primaries", "bt2020", "-color_trc", trc, "-colorspace", "bt2020nc"}
}

func flattenFilterFor(v VideoStream) string {
	wide := v.Height > 0 && float64(v.Width)/float64(v.Height) >= 2.8
	tall := v.Height > 0 && float64(v.Width)/float64(v.Height) <= 1.1
	switch {
	case v.Stereo == "sbs" && wide:
		return "crop=iw/2:ih:0:0"
	case v.Stereo == "sbs":
		return "crop=iw/2:ih:0:0,scale=iw*2:ih,setsar=1"
	case v.Stereo == "tab" && tall:
		return "crop=iw:ih/2:0:0"
	}
	return "crop=iw:ih/2:0:0,scale=iw:ih*2,setsar=1"
}

// ------------------------------------------------------------------ cover art

func decideCovers(src *Source, pl *Plan) {
	for _, c := range src.Covers {
		st := Step{Kind: "cover", Action: ActKeep, From: "Cover picture", To: "Stays as it is", index: c.Index, disposition: "attached_pic", title: c.Name, subNumber: c.Number, tag: c.Codec}
		if c.Codec != "mjpeg" && c.Codec != "png" {
			if pl.Container == "mp4" {
				st.Action, st.To, st.Why = ActDrop, "", "An MP4 only holds JPEG and PNG covers."
			} else {
				st.Action, st.To, st.Why = ActDrop, "", "Only JPEG and PNG covers can be carried across."
			}
		}
		pl.Steps = append(pl.Steps, st)
	}
}

// ------------------------------------------------------------------ sound

func decideAudio(src *Source, p Policy, pl *Plan) []Step {
	if len(src.Audio) == 0 {
		return nil
	}
	a := p.Audio
	orig, how := src.OriginalLanguage()
	wanted := func(t AudioStream) (bool, string) {
		switch {
		case t.Commentary && a.Commentary == Drop:
			return false, "You chose to remove commentary."
		case t.Description && a.Description == Drop:
			return false, "You chose to remove audio description."
		case t.Language == "" && a.Untagged == Drop && a.UntaggedAs == "":
			return false, "This track doesn't say its language, and you chose to remove those."
		case t.Language == "":
			return true, ""
		case a.Keep == KeepAll:
			return true, ""
		case slices.Contains(a.Languages, t.Language):
			return true, ""
		case a.KeepOriginal && t.Language == orig:
			return true, "The original language (" + how + ")."
		}
		return false, languageWords(t.Language) + " isn't one of the languages you're keeping."
	}
	type kept struct {
		t   AudioStream
		why string
	}
	var keep []kept
	var dropped []Step
	for _, t := range src.Audio {
		if ok, why := wanted(t); ok {
			keep = append(keep, kept{t, why})
		} else {
			dropped = append(dropped, Step{Kind: "audio", Action: ActDrop, From: t.describe(), Why: why, index: t.Index})
		}
	}
	// A film never ends up silent.
	if len(keep) == 0 {
		fallback := 0
		for i, t := range src.Audio {
			if !t.Commentary && !t.Description {
				fallback = i
				if t.Default {
					break
				}
			}
		}
		t := src.Audio[fallback]
		keep = append(keep, kept{t, "Kept because a film must keep at least one sound track, and no track matches your choices."})
		dropped = slices.DeleteFunc(dropped, func(s Step) bool { return s.index == t.Index })
		pl.Warnings = append(pl.Warnings, "No sound track matches your languages, so one is kept anyway: "+t.describe()+".")
	}
	if a.Keep == KeepLanguages && orig == "" && a.KeepOriginal {
		pl.Notes = append(pl.Notes, "Nothing says which language this film was made in, so \"keep the original language\" had nothing to go on.")
	}

	// The order: the languages in the order asked for, the plain tracks of a language
	// ahead of its commentary and description, and otherwise as the file has them.
	var seen []string
	for _, k := range keep {
		if !slices.Contains(seen, k.t.Language) {
			seen = append(seen, k.t.Language)
		}
	}
	rank := func(t AudioStream) int {
		if i := slices.Index(a.Languages, t.Language); i >= 0 {
			return i
		}
		if a.Default != "" && t.Language == a.Default {
			return -1
		}
		if len(a.Languages) == 0 && a.Default == "" {
			if a.StereoFirst {
				// Each language's tracks stay together, with its stereo one first.
				return slices.Index(seen, t.Language)
			}
			return 0
		}
		return len(a.Languages) + 1
	}
	stereoFirst := func(t AudioStream) int {
		if a.StereoFirst && !t.Commentary && !t.Description && t.Channels <= 2 && (t.Codec == "aac" || willConvertToAAC(t, a, pl.Container)) {
			return 0
		}
		return 1
	}
	extra := func(t AudioStream) int {
		if t.Commentary || t.Description {
			return 1
		}
		return 0
	}
	sort.SliceStable(keep, func(i, j int) bool {
		x, y := keep[i].t, keep[j].t
		if extra(x) != extra(y) {
			return extra(x) < extra(y)
		}
		if rank(x) != rank(y) {
			return rank(x) < rank(y)
		}
		return stereoFirst(x) < stereoFirst(y)
	})

	// Which track is the one that plays first.
	defaultAt := -1
	if a.Default != "" {
		for i, k := range keep {
			if k.t.Language == a.Default && extra(k.t) == 0 {
				defaultAt = i
				break
			}
		}
	}
	if defaultAt < 0 {
		for i, k := range keep {
			if k.t.Default && extra(k.t) == 0 {
				defaultAt = i
				break
			}
		}
	}
	if defaultAt < 0 {
		for i, k := range keep {
			if extra(k.t) == 0 {
				defaultAt = i
				break
			}
		}
	}
	if defaultAt < 0 {
		defaultAt = 0
	}

	// Each track's form, and where a stereo track has to be added.
	out := make([]Step, 0, len(keep)+2)
	hasStereo := map[string]bool{}
	for _, k := range keep {
		if extra(k.t) == 0 && k.t.Channels <= 2 && (!a.StereoFirst || k.t.Codec == "aac" || k.t.Codec == "mp3" || willConvertToAAC(k.t, a, pl.Container)) {
			hasStereo[k.t.Language] = true
		}
	}
	for i, k := range keep {
		st := audioStep(k.t, k.why, a, pl)
		st.disposition = dispositionOf(k.t, i == defaultAt)
		if a.UntaggedAs != "" && k.t.Language == "" {
			st.language, st.relabel = fileLanguage(a.UntaggedAs), true
			st.To = strings.TrimSpace(st.To + " Labelled " + languageWords(a.UntaggedAs) + ".")
		}
		if a.AddStereo && extra(k.t) == 0 && k.t.Channels > 2 && !hasStereo[k.t.Language] {
			hasStereo[k.t.Language] = true
			add := Step{Kind: "audio", Action: ActAdd, From: "New track made from " + k.t.describe(), To: languageWords(orDefault(k.t.Language, a.UntaggedAs)) + ", AAC stereo",
				Why: "So devices without surround sound have something to play.", index: k.t.Index, encoder: "aac", channels: 2, bitrate: "192k", title: "Stereo", language: fileLanguage(orDefault(k.t.Language, a.UntaggedAs))}
			if a.StereoFirst {
				if i == defaultAt {
					add.disposition = "default"
					st.disposition = dispositionOf(k.t, false)
				} else {
					add.disposition = "0"
				}
				out = append(out, add, st)
				continue
			}
			add.disposition = "0"
			out = append(out, st, add)
			continue
		}
		out = append(out, st)
	}
	return append(out, dropped...)
}

func willConvertToAAC(t AudioStream, a AudioPolicy, container string) bool {
	return a.Codec == "aac" && audioNeedsConverting(t, a, container) != ""
}

func dispositionOf(t AudioStream, isDefault bool) string {
	var flags []string
	if isDefault {
		flags = append(flags, "default")
	}
	if t.Commentary {
		flags = append(flags, "comment")
	}
	if t.Description {
		flags = append(flags, "visual_impaired")
	}
	if t.Original {
		flags = append(flags, "original")
	}
	if len(flags) == 0 {
		return "0"
	}
	return strings.Join(flags, "+")
}

// audioNeedsConverting says why a kept track can't stay as it is, "" when it can.
func audioNeedsConverting(t AudioStream, a AudioPolicy, container string) string {
	if !containerHoldsAudio(container, t.Codec) {
		return "An MP4 can't hold " + codecName(t.Codec) + "."
	}
	if a.MaxChannels > 0 && t.Channels > a.MaxChannels {
		return fmt.Sprintf("You chose at most %s.", channelWords(a.MaxChannels))
	}
	if t.Objects {
		if a.Objects == "convert" {
			return "You chose to convert object sound."
		}
		return ""
	}
	if a.Codec == Keep {
		return ""
	}
	if t.Lossless {
		if a.Lossless == "convert" {
			return "You chose to convert lossless sound, which takes the most room."
		}
		return ""
	}
	if a.StereoFirst && t.Channels <= 2 && t.Codec != "aac" && t.Codec != "mp3" {
		return "Every device plays AAC stereo."
	}
	if len(a.Accept) > 0 && !slices.Contains(a.Accept, t.Codec) {
		return codecName(t.Codec) + " isn't one of the formats you're keeping."
	}
	return ""
}

func audioStep(t AudioStream, why string, a AudioPolicy, pl *Plan) Step {
	st := Step{Kind: "audio", Action: ActKeep, From: t.describe(), To: "Stays as it is", Why: why, index: t.Index}
	reason := audioNeedsConverting(t, a, pl.Container)
	if reason == "" {
		return st
	}
	codec := a.Codec
	if codec == Keep {
		codec = "aac" // only a container that can't hold it gets here
	}
	if codec == "opus" && pl.Container == "mp4" {
		codec = "aac"
	}
	ch := t.Channels
	if ch <= 0 {
		ch = 2
	}
	if a.MaxChannels > 0 && ch > a.MaxChannels {
		ch = a.MaxChannels
	}
	// Surround formats top out at 5.1 here, and stereo Dolby is an odd thing to make.
	if (codec == "ac3" || codec == "eac3") && ch > 6 {
		ch = 6
	}
	if (codec == "ac3" || codec == "eac3") && ch <= 2 {
		codec = "aac"
	}
	st.Action, st.encoder, st.channels, st.bitrate = ActConvert, encoderOfAudio(codec), ch, audioRate(codec, ch)
	st.To = codecName(codec) + ", " + channelWords(ch)
	st.Why = reason
	if t.Objects {
		pl.Warnings = append(pl.Warnings, "Object sound won't survive: "+t.describe()+" becomes "+st.To+".")
	}
	return st
}

func encoderOfAudio(codec string) string {
	if codec == "opus" {
		return "libopus"
	}
	return codec
}

func audioRate(codec string, ch int) string {
	switch codec {
	case "aac":
		switch {
		case ch <= 1:
			return "96k"
		case ch == 2:
			return "192k"
		case ch <= 6:
			return "384k"
		}
		return "512k"
	case "opus":
		switch {
		case ch <= 2:
			return "128k"
		case ch <= 6:
			return "256k"
		}
		return "384k"
	}
	if ch <= 2 {
		return "224k"
	}
	return "640k"
}

// ------------------------------------------------------------------ subtitles

type subPlan struct {
	steps     []Step
	exports   []Step
	embeds    []Step
	leave     []Step
	burn      *Step
	burnFrom  SubStream
	keepInMKV bool
}

// addBack gives up on burning: the track stays a track.
func (s *subPlan) addBack() {
	if s.burn == nil {
		return
	}
	st := Step{Kind: "subtitle", Action: ActKeep, From: s.burnFrom.describe(), To: "Stays as it is", index: s.burnFrom.Index, language: s.burnFrom.Language, disposition: "forced"}
	s.steps = append(s.steps, st)
	s.burn = nil
}

func decideSubs(src *Source, p Policy, env Env, pl *Plan) *subPlan {
	out := &subPlan{}
	sp := p.Subtitles
	// The language being heard decides which forced track matters.
	heard := ""
	for _, a := range src.Audio {
		if a.Default && !a.Commentary {
			heard = a.Language
			break
		}
	}
	if heard == "" && len(src.Audio) > 0 {
		heard = src.Audio[0].Language
	}
	if p.Audio.Default != "" && src.hasAudioLanguage(p.Audio.Default) {
		heard = p.Audio.Default
	}
	wanted := func(language string, forced, sdh bool) (bool, string) {
		switch {
		case forced && sp.Forced == Drop:
			return false, "You chose to remove forced subtitles."
		case forced && (sp.Keep != KeepNone || sp.Forced != Keep) && (language == "" || language == heard || sp.Keep == KeepAll || slices.Contains(sp.Languages, language)):
			return true, ""
		case sp.Keep == KeepNone:
			return false, "You chose to remove subtitles."
		case sdh && sp.SDH == Drop:
			return false, "You chose to remove SDH subtitles."
		case language == "" && sp.Untagged == Drop:
			return false, "This track doesn't say its language, and you chose to remove those."
		case language == "" || sp.Keep == KeepAll:
			return true, ""
		case slices.Contains(sp.Languages, language):
			return true, ""
		}
		return false, languageWords(language) + " isn't one of the subtitle languages you're keeping."
	}
	rank := func(language string) int {
		if i := slices.Index(sp.Languages, language); i >= 0 {
			return i
		}
		return len(sp.Languages)
	}
	type kept struct {
		st   SubStream
		rank int
	}
	var keep []kept
	for n, t := range src.Subs {
		ok, why := wanted(t.Language, t.Forced, t.SDH)
		if !ok {
			out.steps = append(out.steps, Step{Kind: "subtitle", Action: ActDrop, From: t.describe(), Why: why, index: t.Index})
			continue
		}
		// Burned in, if this is the one to burn.
		wantBurn := sp.Forced == "burn" && t.Forced || sp.Forced == "burn-images" && t.Forced && !t.Text && pl.Container == "mp4"
		if wantBurn && out.burn == nil && (t.Language == "" || t.Language == heard || heard == "") && canBurn(t, src, env) {
			st := Step{Kind: "subtitle", Action: ActBurn, From: t.describe(), To: "Drawn into the picture", Why: "A forced track carries dialogue the film needs. Once drawn in it can't be turned off.", index: t.Index, subNumber: n, text: t.Text}
			out.burn, out.burnFrom = &st, t
			continue
		}
		keep = append(keep, kept{t, rank(t.Language)})
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].rank < keep[j].rank })
	for _, k := range keep {
		t := k.st
		st := Step{Kind: "subtitle", Action: ActKeep, From: t.describe(), To: "Stays as it is", index: t.Index, language: t.Language, disposition: subDisposition(t)}
		switch {
		case t.Text:
			target := textTarget(t.Codec, sp.Text, pl.Container)
			if target != t.Codec {
				st.Action, st.encoder, st.To = ActConvert, subEncoder(target), codecName(target)
				if pl.Container == "mp4" {
					st.Why = "An MP4 holds only its own kind of text subtitle."
				} else {
					st.Why = "You chose plain text subtitles."
				}
				if t.Codec == "ass" || t.Codec == "ssa" {
					st.Why += " Styling and positioning are lost."
				}
			}
		case pl.Container == "mkv" && slices.Contains(mkvSubs, t.Codec):
			if sp.Images == Drop {
				st.Action, st.To, st.Why = ActDrop, "", "You chose to remove subtitles stored as pictures. They can't become text without character recognition."
			}
		case pl.Container == "mp4":
			switch {
			case sp.Images == Keep:
				out.keepInMKV = true
			case sp.Images == "export" && t.Codec == "hdmv_pgs_subtitle" && env.Sidecars:
				out.exports = append(out.exports, Step{Kind: "subtitle", Action: ActExport, From: t.describe(), To: "A .sup file beside the video",
					Why: "An MP4 can't hold subtitles stored as pictures, and they can't become text.", index: t.Index, language: t.Language, path: supSuffix(t)})
				st.Action = ""
			default:
				why := "An MP4 can't hold subtitles stored as pictures, and they can't become text without character recognition."
				if sp.Images == Drop {
					why = "You chose to remove subtitles stored as pictures."
				}
				st.Action, st.To, st.Why = ActDrop, "", why
				if sp.Images != Drop {
					pl.Warnings = append(pl.Warnings, "This subtitle will be lost: "+t.describe()+". Keep the original, or choose MKV, if you need it.")
				}
			}
		default:
			st.Action, st.To, st.Why = ActDrop, "", "An MKV can't hold "+codecName(t.Codec)+" subtitles."
			pl.Warnings = append(pl.Warnings, "This subtitle will be lost: "+t.describe()+".")
		}
		if st.Action != "" {
			out.steps = append(out.steps, st)
		}
	}
	// Dropped ones go after the kept ones, whatever order they were found in.
	sort.SliceStable(out.steps, func(i, j int) bool {
		return (out.steps[i].Action == ActDrop) != (out.steps[j].Action == ActDrop) && out.steps[j].Action == ActDrop
	})
	// Files beside the video.
	for _, f := range src.Sidecars {
		desc := languageWords(f.Language) + ", " + codecName(f.Codec) + " file beside the video"
		ok, _ := wanted(f.Language, f.Forced, f.SDH)
		switch {
		case sp.Sidecars == "embed" && f.Text && ok && embeddable(f.Codec):
			target := textTarget(f.Codec, sp.Text, pl.Container)
			enc := "copy"
			if target != f.Codec {
				enc = subEncoder(target)
			}
			flags := subDisposition(SubStream{Forced: f.Forced, SDH: f.SDH})
			out.embeds = append(out.embeds, Step{Kind: "file", Action: ActEmbed, From: desc, To: "A track inside the new file", path: f.Path, encoder: enc, language: f.Language, disposition: flags})
		default:
			out.leave = append(out.leave, Step{Kind: "file", Action: ActLeave, From: desc, To: "Stays beside the video", path: f.Path})
		}
	}
	return out
}

func embeddable(codec string) bool {
	return codec == "subrip" || codec == "ass" || codec == "ssa" || codec == "webvtt"
}

func canBurn(t SubStream, src *Source, env Env) bool {
	if t.Text {
		return env.filter("subtitles")
	}
	return env.filter("overlay")
}

func subDisposition(t SubStream) string {
	var flags []string
	if t.Default {
		flags = append(flags, "default")
	}
	if t.Forced {
		flags = append(flags, "forced")
	}
	if t.SDH {
		flags = append(flags, "hearing_impaired")
	}
	if len(flags) == 0 {
		return "0"
	}
	return strings.Join(flags, "+")
}

// supSuffix is what goes between the video's name and .sup, in the form the server reads back.
func supSuffix(t SubStream) string {
	s := t.Language
	if s == "" {
		s = "und"
	}
	if t.Forced {
		s += ".forced"
	}
	if t.SDH {
		s += ".sdh"
	}
	return s
}

// textTarget: which text format a subtitle ends up in, in this container.
func textTarget(codec, choice, container string) string {
	if container == "mp4" {
		return "mov_text"
	}
	if choice == "srt" {
		return "subrip"
	}
	switch codec {
	case "subrip", "ass", "ssa", "webvtt":
		return codec
	}
	return "subrip"
}

func subEncoder(target string) string {
	switch target {
	case "subrip":
		return "srt"
	}
	return target
}

// ------------------------------------------------------------------ the rest

func decideRest(src *Source, pl *Plan) {
	for _, a := range src.Attachments {
		if pl.Container == "mkv" {
			pl.Steps = append(pl.Steps, Step{Kind: "attachment", Action: ActKeep, From: "Attached file " + orDefault(cleanTitle(a.Name), "with no name"), To: "Stays as it is", index: a.Index})
		} else {
			pl.Steps = append(pl.Steps, Step{Kind: "attachment", Action: ActDrop, From: "Attached file " + orDefault(cleanTitle(a.Name), "with no name"), Why: "An MP4 can't hold attached files. These are usually fonts for styled subtitles."})
		}
	}
	for _, o := range src.Other {
		pl.Steps = append(pl.Steps, Step{Kind: "data", Action: ActDrop, From: "A " + orDefault(o.Type, "data") + " stream that isn't picture, sound or subtitles", Why: "Players don't use it, and it often can't move to another file."})
	}
}

// finish works out the verdict from what the steps come to.
func finish(src *Source, p Policy, pl *Plan) {
	sameContainer := pl.Ext == src.Ext || (pl.Container == "mp4" && (src.Ext == "mp4" || src.Ext == "m4v")) || (pl.Container == "mkv" && src.Ext == "mkv")
	encoded, changed := false, !sameContainer
	var keptAudio, keptSubs []int
	for _, st := range pl.Steps {
		switch st.Action {
		case ActConvert, ActAdd, ActBurn:
			encoded = true
			changed = true
		case ActDrop:
			if st.Kind != "data" {
				changed = true
			}
		case ActExport, ActEmbed:
			changed = true
		case ActKeep:
			switch st.Kind {
			case "audio":
				keptAudio = append(keptAudio, st.index)
				if st.relabel {
					changed = true
				}
			case "subtitle":
				keptSubs = append(keptSubs, st.index)
			}
		}
	}
	// A new order is a change, and so is a new default when one was asked for.
	if !slices.IsSorted(keptAudio) || !slices.IsSorted(keptSubs) {
		changed = true
	}
	if p.Audio.Default != "" && !changed {
		for _, st := range pl.Steps {
			if st.Kind == "audio" && st.Action == ActKeep && strings.Contains(st.disposition, "default") {
				for _, a := range src.Audio {
					if a.Index == st.index && !a.Default {
						changed = true
					}
				}
			}
		}
	}
	// A saving is only asked of files that were encoded again. Tidying a file doesn't shrink it.
	if !encoded {
		pl.minSaving = 0
	}
	switch {
	case !changed:
		pl.Verdict, pl.Reason = VerdictSkip, "Already matches: nothing would change."
		if len(pl.Notes) > 0 {
			pl.Reason = "Nothing would change."
		}
	case encoded:
		pl.Verdict = VerdictConvert
		pl.Reason = convertSummary(pl)
	default:
		pl.Verdict = VerdictRemux
		pl.Reason = "Only tidied: the picture and sound stay exactly as they are."
	}
}

func convertSummary(pl *Plan) string {
	var picture, sound bool
	for _, st := range pl.Steps {
		if st.Action == ActConvert && st.Kind == "video" {
			picture = true
		}
		if (st.Action == ActConvert || st.Action == ActAdd) && st.Kind == "audio" {
			sound = true
		}
	}
	switch {
	case picture && sound:
		return "The picture and some sound are converted."
	case picture:
		return "The picture is converted. The sound stays as it is."
	case sound:
		return "Some sound is converted. The picture stays exactly as it is."
	}
	return "Subtitles are converted. The picture and sound stay as they are."
}
