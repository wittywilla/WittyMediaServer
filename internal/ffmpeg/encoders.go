package ffmpeg

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"runtime"
	"slices"
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
	Error    string `json:"error,omitempty"`  // the end of FFmpeg's message
	Reason   string `json:"reason,omitempty"` // why it can't be used, in plain words
	Device   string `json:"device,omitempty"` // the device it uses or was tried on
}

type hwState struct {
	mu     sync.Mutex
	status map[string]*HWStatus
	gpus   []GPUDevice
	vaapi  string // the render node for VA-API
	qsv    string // the Intel render node Quick Sync is given, "" to let FFmpeg choose
	done   bool
	once   sync.Once
}

var hw = &hwState{status: map[string]*HWStatus{}}

// defaultVAAPIDevice is used until the devices have been looked at.
const defaultVAAPIDevice = "/dev/dri/renderD128"

// unixGPU says whether this system's graphics devices are files in /dev that
// can be looked at before a test encode.
var unixGPU = runtime.GOOS == "linux" || runtime.GOOS == "freebsd"

// StartHardwareProbe test-encodes in the background, once. It starts with the
// server only if hardware encoding is switched on, else when Settings first
// asks: on a small machine every test is a process that starts up, looks for
// a device and fails.
func (t *Tools) StartHardwareProbe() {
	hw.once.Do(func() {
		logging.Go("hardware encoder test", func() { t.ProbeHardware(context.Background()) }, "cat", "transcode")
	})
}

// ProbeHardware looks at the graphics devices, then test-encodes a few frames
// with every compiled hardware encoder that could work with them.
func (t *Tools) ProbeHardware(ctx context.Context) {
	log := slog.Default().With("cat", "transcode")
	gpus := FindGPUs()
	container := InContainer()
	for _, d := range gpus {
		if d.Usable {
			log.Info("graphics device found", "device", d.Path, "vendor", d.Vendor, "driver", d.Driver, "group", d.Group)
		} else {
			log.Warn("graphics device can't be used: "+d.Problem+" "+d.Fix, "device", d.Path, "vendor", d.Vendor, "group", d.Group, "mode", d.Mode)
		}
	}
	vaapi, qsv := defaultVAAPIDevice, ""
	if d, ok := pickRender(gpus, ""); ok {
		vaapi = d.Path
	}
	if d, ok := pickRender(gpus, "intel"); ok {
		qsv = d.Path
	}
	hw.mu.Lock()
	hw.gpus, hw.vaapi, hw.qsv = gpus, vaapi, qsv
	hw.mu.Unlock()
	if unixGPU && len(gpus) == 0 {
		log.Info("no graphics device found: " + whyNoRender(nil, "", "", container))
	}

	for _, e := range HWEncoders {
		st := &HWStatus{HWEncoder: e, Compiled: t.Encoders[e.Name]}
		switch {
		case !st.Compiled:
		case e.Vendor == "apple" && runtime.GOOS != "darwin":
		case e.Vendor == "vaapi" && !unixGPU:
		default:
			// Where the devices are files, an encoder with nothing to run on isn't
			// started at all: on a small machine each test is a process that fails.
			if reason := missingDevice(e.Vendor, gpus, container); reason != "" {
				st.Reason = reason
				log.Info("hardware encoder can't be used", "encoder", e.Name, "reason", reason)
				break
			}
			t.testEncoder(ctx, log, st, container)
		}
		hw.mu.Lock()
		hw.status[e.Name] = st
		hw.mu.Unlock()
	}
	hw.mu.Lock()
	hw.done = true
	hw.mu.Unlock()
}

// missingDevice says why an encoder has no device to work with here, "" when it
// may have one (or this system can't tell before trying).
func missingDevice(vendor string, gpus []GPUDevice, container bool) string {
	if !unixGPU {
		return ""
	}
	switch vendor {
	case "vaapi":
		if _, ok := pickRender(gpus, ""); !ok {
			return whyNoRender(gpus, "", "", container)
		}
	case "intel":
		if _, ok := pickRender(gpus, "intel"); !ok {
			return whyNoRender(gpus, "intel", "Intel", container)
		}
	case "nvidia":
		// Under WSL, and Docker Desktop on Windows, NVIDIA's driver is reached through /dev/dxg.
		if slices.ContainsFunc(gpus, func(d GPUDevice) bool { return d.Path == "/dev/dxg" && d.Usable }) {
			return ""
		}
		for _, d := range gpus {
			if strings.HasPrefix(d.Path, "/dev/nvidia") {
				if d.Usable {
					return ""
				}
				return strings.TrimSpace(d.Problem + " " + d.Fix)
			}
		}
		if container {
			return "No NVIDIA device is in the container: run it with the NVIDIA Container Toolkit (gpus: all)."
		}
		return "No NVIDIA device (/dev/nvidia0) was found."
	case "amd":
		// AMF on Linux needs AMD's own driver; on an AMD chip, VA-API is the usual way.
		if _, ok := pickRender(gpus, "amd"); !ok {
			return whyNoRender(gpus, "amd", "AMD", container)
		}
	}
	return ""
}

// testEncoder encodes a few black frames with one encoder.
func (t *Tools) testEncoder(ctx context.Context, log *slog.Logger, st *HWStatus, container bool) {
	plan := t.planFor(st.Name, Quality{})
	st.Device = planDevice(plan)
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
	logging.ChildExit(log, "ffmpeg", err, true, &stderr, started, "encoder", st.Name)
	st.Tested, st.Working = true, err == nil
	if err == nil {
		log.Info("hardware encoder works", "encoder", st.Name, "device", st.Device)
		return
	}
	msg := strings.TrimSpace(stderr.String())
	st.Reason = explainEncoderError(st.Vendor, msg, container)
	if st.Vendor == "vaapi" && st.Reason != "" {
		// A driver "missing" for a chip that has none here, such as WSL's own device.
		for _, d := range GPUs() {
			if d.Path == st.Device && d.Vendor != "" && d.Vendor != "intel" && d.Vendor != "amd" {
				st.Reason = "The graphics device " + d.Path + " isn't Intel's or AMD's (its driver is " + cmp.Or(d.Driver, "unknown") + "): VA-API here has drivers for those only."
			}
		}
	}
	if i := strings.LastIndex(msg, "\n"); i > 0 && len(msg) > 300 {
		msg = msg[i+1:]
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	st.Error = msg
	log.Info("hardware encoder can't be used", "encoder", st.Name, "device", st.Device, "reason", st.Reason, "err", msg)
}

// planDevice is the device a plan's input options name, if any.
func planDevice(p Plan) string {
	for i, a := range p.InputArgs {
		if a == "-vaapi_device" && i+1 < len(p.InputArgs) {
			return p.InputArgs[i+1]
		}
		if _, dev, ok := strings.Cut(a, "child_device="); ok {
			return dev
		}
	}
	return ""
}

// GPUs are the graphics devices found by the last hardware test.
func GPUs() []GPUDevice {
	hw.mu.Lock()
	defer hw.mu.Unlock()
	return slices.Clone(hw.gpus)
}

// DeviceArgs are the input options that give a hardware encoder family (vaapi,
// qsv) the device found for it, nil for one that needs none.
func DeviceArgs(family string) []string {
	hw.mu.Lock()
	vaapi, qsv := hw.vaapi, hw.qsv
	hw.mu.Unlock()
	switch family {
	case "vaapi":
		if vaapi == "" {
			vaapi = defaultVAAPIDevice
		}
		return []string{"-vaapi_device", vaapi}
	case "qsv":
		// On Linux Quick Sync works through VA-API, on the first render node unless
		// told: wrong when an AMD or NVIDIA card's node comes before Intel's. When
		// Intel's is the first, FFmpeg's own choice is left alone.
		if qsv != "" && qsv != defaultVAAPIDevice && unixGPU {
			return []string{"-init_hw_device", "qsv=qs:hw,child_device=" + qsv}
		}
	}
	return nil
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
		p.InputArgs = DeviceArgs("qsv")
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
		p.InputArgs = DeviceArgs("vaapi")
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
