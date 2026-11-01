package libtranscode

import (
	"math"
	"runtime"
)

// Rough figures, shown as rough. A sample encode (engine.go) measures the real ones for
// a chosen file, and they're used in place of these once someone has run one.

// estimate fills a plan's EstBytes and EstSec.
func estimate(src *Source, pl *Plan) {
	if pl.Verdict == VerdictSkip || pl.Verdict == VerdictRefuse || src.Duration <= 0 {
		return
	}
	// A file can claim to last for days: one stray subtitle will do it. Nothing with a picture
	// runs at a tiny fraction of an audio track's bitrate, so its size is a limit on its length.
	duration := src.Duration
	if src.Size > 0 {
		duration = min(duration, float64(src.Size)/2000)
	}
	var kbpsOut float64
	var seconds float64
	cores := float64(max(runtime.NumCPU(), 1))
	for _, st := range pl.Steps {
		switch st.Action {
		case ActDrop, ActLeave:
			continue
		}
		switch st.Kind {
		case "video":
			v := videoOf(src, st.index)
			if v == nil {
				continue
			}
			if st.encoder == "" {
				kbpsOut += float64(knownVideoKbps(src, *v))
				continue
			}
			fps := max(v.FPS, 24)
			w, h := float64(v.Width), float64(v.Height)
			if pl.minSaving >= 0 && h > 0 {
				// Scaling.
				for _, f := range st.filters {
					if len(f) > 9 && f[:9] == "scale=-2:" {
						var nh float64
						for _, c := range f[9:] {
							if c < '0' || c > '9' {
								break
							}
							nh = nh*10 + float64(c-'0')
						}
						if nh > 0 {
							w, h = w*nh/h, nh
						}
					}
				}
			}
			kbps := videoKbpsFor(st.encoder, w, h, st)
			if known := float64(knownVideoKbps(src, *v)); known > 0 {
				kbps = math.Min(kbps, known*1.05) // an already small file doesn't grow much
			}
			kbpsOut += kbps
			seconds += duration * fps / encodeFPS(st.encoder, w*h, cores)
		case "audio":
			if st.encoder == "" {
				kbpsOut += float64(knownAudioKbps(src, st.index))
			} else {
				kbpsOut += float64(rateKbps(st.bitrate))
			}
		case "subtitle":
			kbpsOut += 8
		}
	}
	pl.EstBytes = int64(kbpsOut * 1000 / 8 * duration)
	// Copying takes as long as reading the file.
	seconds = math.Max(seconds, float64(src.Size)/(60*1024*1024))
	pl.EstSec = seconds
}

func videoOf(src *Source, index int) *VideoStream {
	for i := range src.Video {
		if src.Video[i].Index == index {
			return &src.Video[i]
		}
	}
	return nil
}

// knownVideoKbps: what the file says, or what's left of its average after the sound.
func knownVideoKbps(src *Source, v VideoStream) int {
	if v.Kbps > 0 {
		return v.Kbps
	}
	if src.Kbps <= 0 {
		return 0
	}
	sound := 0
	for _, a := range src.Audio {
		sound += knownAudioKbps(src, a.Index)
	}
	return max(src.Kbps-sound, src.Kbps/5)
}

func knownAudioKbps(src *Source, index int) int {
	for _, a := range src.Audio {
		if a.Index == index {
			if a.Kbps > 0 {
				return a.Kbps
			}
			switch {
			case a.Lossless:
				return 1800
			case a.Channels > 2:
				return 640
			}
			return 192
		}
	}
	return 192
}

func rateKbps(s string) int {
	if len(s) < 2 {
		return 192
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// videoKbpsFor: a film-like picture at quality 23 in H.264 is around 5 Mb/s at 1080p,
// more detail takes less than its share of extra data, and each 6 steps doubles it.
func videoKbpsFor(encoder string, w, h float64, st Step) float64 {
	q := qualityOf(st)
	base := 5000 * math.Pow((w*h)/(1920*1080), 0.75) * math.Pow(2, (23-q)/6)
	switch encoderCodec(encoder) {
	case "hevc":
		base *= 0.6
	case "av1":
		base *= 0.5
	}
	if isHardware(encoder) {
		base *= 1.3
	}
	return base
}

// qualityOf reads the quality back from the options of a step, on the common scale.
func qualityOf(st Step) float64 {
	for i := 0; i+1 < len(st.args); i += 2 {
		switch st.args[i] {
		case "-crf":
			n := float64(rateKbpsPlain(st.args[i+1]))
			switch st.encoder {
			case "libx265":
				return n - 4
			case "libsvtav1":
				return n - 8
			}
			return n
		case "-cq", "-global_quality", "-qp":
			return float64(rateKbpsPlain(st.args[i+1])) - 2
		}
	}
	return 23
}

func rateKbpsPlain(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func encoderCodec(name string) string {
	switch {
	case name == "libx265", len(name) > 5 && name[:5] == "hevc_":
		return "hevc"
	case name == "libsvtav1", len(name) > 4 && name[:4] == "av1_":
		return "av1"
	}
	return "h264"
}

func isHardware(name string) bool { return HardwareFamily(name) != "" && name[:3] != "lib" }

// encodeFPS: frames a second, roughly, per the machine's cores.
func encodeFPS(encoder string, pixels, cores float64) float64 {
	scale := (1920.0 * 1080.0) / math.Max(pixels, 1)
	if isHardware(encoder) {
		return 150 * scale
	}
	perCore := 6.0
	switch encoder {
	case "libx265":
		perCore = 1.5
	case "libsvtav1":
		perCore = 2.5
	}
	return math.Max(perCore*cores*0.6*scale, 0.5)
}
