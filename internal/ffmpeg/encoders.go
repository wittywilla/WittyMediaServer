package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// HWEncoder describes a hardware H.264 encoder WittyMediaServer can use.
type HWEncoder struct {
	Name   string `json:"name"`   // ffmpeg encoder name
	Label  string `json:"label"`  // shown in the UI
	Vendor string `json:"vendor"` // nvidia | intel | amd | apple | vaapi
}

// HWEncoders in default preference order.
var HWEncoders = []HWEncoder{
	{"h264_nvenc", "NVIDIA NVENC", "nvidia"},
	{"h264_qsv", "Intel Quick Sync", "intel"},
	{"h264_videotoolbox", "Apple VideoToolbox", "apple"},
	{"h264_amf", "AMD AMF (VCE/VCN)", "amd"},
	{"h264_vaapi", "VAAPI (Linux: Intel/AMD)", "vaapi"},
}

// HWStatus says whether an encoder is compiled into ffmpeg and actually
// works on this machine (ffmpeg lists encoders it was built with even when
// the GPU or driver is missing, so each one is test-encoded).
type HWStatus struct {
	HWEncoder
	Compiled bool   `json:"compiled"`
	Tested   bool   `json:"tested"`
	Working  bool   `json:"working"`
	Error    string `json:"error,omitempty"`
}

type hwState struct {
	mu     sync.Mutex
	status map[string]*HWStatus
	done   bool
	once   sync.Once
}

var hw = &hwState{status: map[string]*HWStatus{}}

const vaapiDevice = "/dev/dri/renderD128"

// StartHardwareProbe test-encodes in the background, once. It starts with the
// server only if hardware encoding is switched on, else when Settings first
// asks: on a small machine every test is a process that starts up, looks for
// a device and fails.
func (t *Tools) StartHardwareProbe() {
	hw.once.Do(func() {
		logging.Go("hardware encoder test", func() { t.ProbeHardware(context.Background()) }, "cat", "transcode")
	})
}

// ProbeHardware test-encodes a few frames with every compiled hardware
// encoder.
func (t *Tools) ProbeHardware(ctx context.Context) {
	log := slog.Default().With("cat", "transcode")
	for _, e := range HWEncoders {
		st := &HWStatus{HWEncoder: e, Compiled: t.Encoders[e.Name]}
		if st.Compiled && !(e.Vendor == "apple" && runtime.GOOS != "darwin") && !(e.Vendor == "vaapi" && runtime.GOOS != "linux" && runtime.GOOS != "freebsd") {
			plan := t.planFor(e.Name, Quality{})
			args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
			args = append(args, plan.InputArgs...)
			args = append(args, "-f", "lavfi", "-i", "color=c=black:s=320x240:r=25:d=0.4")
			vf := "format=" + plan.PixFmt
			if plan.Upload != "" {
				vf += "," + plan.Upload
			}
			args = append(args, "-vf", vf, "-c:v", plan.Name)
			args = append(args, plan.Args...)
			args = append(args, "-f", "null", "-")
			cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			cmd := Command(cctx, t.FFmpeg, args...)
			var stderr logging.Tail
			cmd.Stderr = &stderr
			started := time.Now()
			err := cmd.Run()
			cancel()
			// FFmpeg lists encoders this machine has no hardware for, so most of
			// these failing says nothing is wrong: the details are only a debug line.
			logging.ChildExit(log, "ffmpeg", err, true, &stderr, started, "encoder", e.Name)
			st.Tested, st.Working = true, err == nil
			if err == nil {
				log.Info("hardware encoder works", "encoder", e.Name)
			} else {
				msg := strings.TrimSpace(stderr.String())
				if i := strings.LastIndex(msg, "\n"); i > 0 && len(msg) > 300 {
					msg = msg[i+1:]
				}
				if len(msg) > 300 {
					msg = msg[:300]
				}
				st.Error = msg
				log.Info("hardware encoder can't be used", "encoder", e.Name, "err", msg)
			}
		}
		hw.mu.Lock()
		hw.status[e.Name] = st
		hw.mu.Unlock()
	}
	hw.mu.Lock()
	hw.done = true
	hw.mu.Unlock()
}

// HardwareStatus reports every known hardware encoder (testing still in
// progress shows as Tested=false).
func (t *Tools) HardwareStatus() (list []HWStatus, done bool) {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	for _, e := range HWEncoders {
		if st, ok := hw.status[e.Name]; ok {
			list = append(list, *st)
		} else {
			list = append(list, HWStatus{HWEncoder: e, Compiled: t.Encoders[e.Name]})
		}
	}
	return list, hw.done
}

func hwWorking(name string) bool {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	st, ok := hw.status[name]
	return ok && st.Working
}

// Quality is the transcode quality profile chosen in Settings.
type Quality struct {
	// HW lists the hardware encoders the admin enabled; the first one that
	// works on this machine is used, else libx264.
	HW        []string
	Preset    string // libx264 preset (ultrafast … slow); "" = veryfast
	CRF       int    // constant quality (lower = better); 0 = 21. Mapped onto each hardware encoder's scale.
	MaxRate   string // e.g. "8M": cap the bitrate (helps remote/slow networks)
	AudioRate string // e.g. "192k"; "" = 192k stereo / 384k surround
	// CapRate is a hard limit on the video bitrate, e.g. "4M" (see WithCap).
	// Every encoder is told in the way it really obeys, and expert options
	// can't lift it.
	CapRate string
	// Expert mode: replaces the encoder options (after -c:v) entirely, and
	// extra output options appended before the HLS muxer options.
	VideoArgs  []string
	OutputArgs []string
}

// Plan is everything needed to run one encoder.
type Plan struct {
	Name      string   // ffmpeg encoder
	Args      []string // encoder options
	PixFmt    string   // software pixel format fed to the encoder
	InputArgs []string // options placed before -i (device setup)
	Upload    string   // filter appended after format= (hwupload for VAAPI)
}

// Plan picks the encoder for a quality profile.
func (t *Tools) Plan(q Quality) Plan {
	for _, name := range q.HW {
		if t.Encoders[name] && hwWorking(name) {
			return t.planFor(name, q)
		}
	}
	if t.Encoders["libx264"] {
		return t.planFor("libx264", q)
	}
	if t.Encoders["libopenh264"] {
		return t.planFor("libopenh264", q)
	}
	p := Plan{Name: "mpeg4", Args: []string{"-q:v", "4"}, PixFmt: "yuv420p"}
	if q.CapRate != "" {
		p.Args = capArgs("mpeg4", p.Args, q)
	}
	return p
}

func (t *Tools) planFor(name string, q Quality) Plan {
	crf := q.CRF
	if crf <= 0 {
		crf = 21
	}
	c := strconv.Itoa(crf)
	p := Plan{Name: name, PixFmt: "yuv420p"}
	switch name {
	case "h264_nvenc":
		// p1 (fastest) … p7 (best); map the x264 preset onto it.
		p.Args = []string{"-preset", nvencPreset(q.Preset), "-tune", "ll", "-rc", "vbr", "-cq", strconv.Itoa(crf + 2), "-b:v", "0"}
	case "h264_qsv":
		p.PixFmt = "nv12"
		p.Args = []string{"-preset", qsvPreset(q.Preset), "-global_quality", strconv.Itoa(crf + 2)}
	case "h264_videotoolbox":
		vq := 102 - 2*crf // crf 21 -> q 60 (1-100, higher is better)
		if vq < 1 {
			vq = 1
		}
		p.Args = []string{"-q:v", strconv.Itoa(vq), "-realtime", "1"}
	case "h264_amf":
		p.Args = []string{"-quality", amfQuality(q.Preset), "-rc", "cqp", "-qp_i", strconv.Itoa(crf + 1), "-qp_p", strconv.Itoa(crf + 3)}
	case "h264_vaapi":
		p.PixFmt = "nv12"
		p.InputArgs = []string{"-vaapi_device", vaapiDevice}
		p.Upload = "hwupload"
		p.Args = []string{"-qp", strconv.Itoa(crf + 2)}
	case "libx264":
		preset := q.Preset
		if preset == "" {
			preset = "veryfast"
		}
		p.Args = []string{"-preset", preset, "-crf", c, "-profile:v", "high", "-level", "4.1", "-pix_fmt", "yuv420p", "-x264-params", "scenecut=0:open_gop=0"}
	case "libopenh264":
		p.Args = []string{"-b:v", "6M", "-pix_fmt", "yuv420p", "-profile:v", "high"}
	}
	if len(q.VideoArgs) > 0 {
		p.Args = append([]string(nil), q.VideoArgs...)
	}
	if q.CapRate != "" {
		p.Args = capArgs(name, p.Args, q)
	}
	return p
}

// capArgs gives an encoder a hard bitrate limit. libx264 and NVENC keep their
// constant quality and only stop at the limit; the others ignore a limit in
// their constant quality modes, so they switch to a target bitrate below it.
func capArgs(name string, args []string, q Quality) []string {
	bits := ParseRate(q.CapRate)
	if bits <= 0 {
		return args
	}
	limit := []string{"-maxrate", rateArg(bits), "-bufsize", rateArg(2 * bits)}
	target := rateArg(bits * 4 / 5)
	if len(q.VideoArgs) > 0 {
		return append(args, limit...) // expert options: added last, so they win
	}
	switch name {
	case "libx264", "h264_nvenc":
		return append(args, limit...)
	case "h264_qsv":
		return append([]string{"-preset", qsvPreset(q.Preset), "-b:v", target}, limit...)
	case "h264_videotoolbox":
		return append([]string{"-b:v", target, "-realtime", "1"}, limit...)
	case "h264_amf":
		return append([]string{"-quality", amfQuality(q.Preset), "-rc", "vbr_peak", "-b:v", target}, limit...)
	case "h264_vaapi":
		return append([]string{"-rc_mode", "VBR", "-b:v", target}, limit...)
	case "libopenh264":
		return append([]string{"-b:v", target, "-pix_fmt", "yuv420p", "-profile:v", "high"}, limit...)
	}
	return append([]string{"-b:v", target}, limit...)
}

// ParseRate reads a bitrate such as "8M", "1500k" or "2000000" as bits per
// second (0 = not a rate).
func ParseRate(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	mul := 1.0
	switch s[len(s)-1] {
	case 'k', 'K':
		mul, s = 1e3, s[:len(s)-1]
	case 'm', 'M':
		mul, s = 1e6, s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 || v > 1e12 {
		return 0
	}
	return int64(v * mul)
}

func rateArg(bits int64) string { return strconv.FormatInt(bits/1000, 10) + "k" }

// WithCap applies a total bitrate limit (video and sound together, e.g. "4M")
// to a profile: sound stays within it, the video gets the rest, and a lower
// limit chosen in Settings still counts.
func (q Quality) WithCap(total string) Quality {
	bits := ParseRate(total)
	if bits <= 0 {
		return q
	}
	audio := ParseRate(q.AudioRate)
	if audio <= 0 || audio > 192000 {
		audio = 192000
		q.AudioRate = "192k"
	}
	if bits <= 2000000 {
		audio = min(audio, 128000)
		q.AudioRate = "128k"
	}
	video := max(bits-audio, 200000)
	if own := ParseRate(q.MaxRate); own > 0 && own < video {
		video = own
	}
	q.CapRate = rateArg(video)
	return q
}

func nvencPreset(x264 string) string {
	switch x264 {
	case "ultrafast", "superfast":
		return "p1"
	case "veryfast":
		return "p3"
	case "faster", "fast":
		return "p4"
	case "medium":
		return "p5"
	case "slow", "slower", "veryslow":
		return "p6"
	}
	return "p4"
}

func qsvPreset(x264 string) string {
	switch x264 {
	case "ultrafast", "superfast", "veryfast":
		return "veryfast"
	case "faster", "fast":
		return "faster"
	case "medium":
		return "medium"
	case "slow", "slower", "veryslow":
		return "slow"
	}
	return "veryfast"
}

func amfQuality(x264 string) string {
	switch x264 {
	case "medium", "slow", "slower", "veryslow":
		return "quality"
	case "fast", "faster":
		return "balanced"
	}
	return "speed"
}

// expertOptions are the options the expert fields may set: they tune encoding
// and nothing else. FFmpeg's options can also read and write any file (-i, an
// output name, -attach, -dump_attachment, -passlogfile), run filters that open
// files (-vf, -filter_complex) or reach the network, so anything not listed
// is refused: settings are not a way out onto the machine.
var expertOptions = map[string]bool{
	"preset": true, "tune": true, "profile": true, "profile:v": true, "level": true, "level:v": true,
	"crf": true, "cq": true, "qp": true, "qmin": true, "qmax": true, "qp_i": true, "qp_p": true, "qp_b": true,
	"b:v": true, "minrate": true, "maxrate": true, "bufsize": true, "rc": true, "multipass": true,
	"g": true, "bf": true, "refs": true, "keyint_min": true, "sc_threshold": true, "threads": true, "pix_fmt": true,
	"rc-lookahead": true, "look_ahead": true, "look_ahead_depth": true, "spatial-aq": true, "spatial_aq": true, "temporal-aq": true, "temporal_aq": true,
	"aq-strength": true, "b_ref_mode": true, "nonref_p": true, "global_quality": true, "async_depth": true, "quality": true, "realtime": true,
	"coder": true, "x264-params": true, "x265-params": true, "b:a": true, "ac": true, "ar": true, "max_muxing_queue_size": true, "movflags": true,
}

var (
	expertValueRe = regexp.MustCompile(`^(?:[A-Za-z0-9_.:=+,-]+|\d+/\d+)$`)
	// Encoder parameters that name a file.
	expertFileParamRe = regexp.MustCompile(`(?i)(^|:)[a-z0-9_-]*(stats|dump|file|csv|analysis|log|recon|zones|lut)[a-z0-9_-]*=`)
	rateRe            = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?[kKmMgG]?$`)
)

// CheckExpertArgs says whether the expert fields' options are allowed: pairs
// of an option from the list above and a plain value.
func CheckExpertArgs(args []string) error {
	if len(args)%2 != 0 {
		return errors.New("expert options come in pairs: -option value")
	}
	for i := 0; i < len(args); i += 2 {
		name, ok := strings.CutPrefix(args[i], "-")
		if !ok || !expertOptions[name] {
			return fmt.Errorf("the expert option %q is not allowed", args[i])
		}
		v := args[i+1]
		if !expertValueRe.MatchString(v) || (strings.HasPrefix(v, "-") && !isNumber(v)) {
			return fmt.Errorf("the value of %s is not allowed: %q", args[i], v)
		}
		if (name == "x264-params" || name == "x265-params") && expertFileParamRe.MatchString(v) {
			return fmt.Errorf("%s may not name files: %q", args[i], v)
		}
	}
	return nil
}

func isNumber(s string) bool {
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// ValidPreset reports whether v is a libx264 preset.
func ValidPreset(v string) bool {
	switch v {
	case "ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow", "placebo":
		return true
	}
	return false
}

// ValidRate reports whether v is empty or a bitrate like 8M or 192k.
func ValidRate(v string) bool { return v == "" || rateRe.MatchString(v) }

// SplitArgs splits an option string like a shell would (spaces separate,
// single or double quotes group). Used for the expert ffmpeg options.
func SplitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	have := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, have = r, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if have || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if have || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
