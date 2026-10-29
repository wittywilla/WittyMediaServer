package ffmpeg

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
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
}

var hw = &hwState{status: map[string]*HWStatus{}}

const vaapiDevice = "/dev/dri/renderD128"

// ProbeHardware test-encodes a few frames with every compiled hardware
// encoder. It runs once in the background at start-up.
func (t *Tools) ProbeHardware(ctx context.Context) {
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
			cmd := exec.CommandContext(cctx, t.FFmpeg, args...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			hideWindow(cmd)
			err := cmd.Run()
			cancel()
			st.Tested, st.Working = true, err == nil
			if err != nil {
				msg := strings.TrimSpace(stderr.String())
				if i := strings.LastIndex(msg, "\n"); i > 0 && len(msg) > 300 {
					msg = msg[i+1:]
				}
				if len(msg) > 300 {
					msg = msg[:300]
				}
				st.Error = msg
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
	return Plan{Name: "mpeg4", Args: []string{"-q:v", "4"}, PixFmt: "yuv420p"}
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
	return p
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
