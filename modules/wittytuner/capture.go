package wittytuner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Capture devices through FFmpeg: V4L2 on Linux (v4l2.go), DirectShow on
// Windows and AVFoundation on macOS. They show one picture, so each is one
// channel. FFmpeg opens them; a device name never reaches a shell.

func init() {
	register(Driver{
		ID: "dshow", Name: "Windows capture card (DirectShow)", Status: "untested", Platforms: []string{"windows"},
		Description: "Analogue capture cards, HDMI capture and USB video on Windows. Not yet tried with a real card.",
		Fields: []Field{
			{Key: "video", Label: "Video device", Kind: "text", Required: true, Help: "The name Windows gives it. Search finds these."},
			{Key: "audio", Label: "Audio device", Kind: "text"},
			{Key: "crossbar", Label: "Crossbar input", Kind: "number", Help: "Optional: which input of the card."},
			{Key: "interlaced", Label: "The picture is interlaced", Kind: "toggle", Default: "1"},
		},
		build: func(c Config, e Env) (Source, error) {
			if err := checkDeviceName(c.Options["video"], c.Options["audio"]); err != nil {
				return nil, err
			}
			return &capture{cfg: c, env: e, driver: "dshow"}, nil
		},
	})
	register(Driver{
		ID: "avfoundation", Name: "macOS capture device (AVFoundation)", Status: "untested", Platforms: []string{"darwin"},
		Description: "Capture cards and USB video on macOS. Not yet tried with a real device.",
		Fields: []Field{
			{Key: "video", Label: "Video device", Kind: "text", Required: true, Help: "The name macOS gives it. Search finds these."},
			{Key: "audio", Label: "Audio device", Kind: "text"},
			{Key: "framerate", Label: "Frame rate", Kind: "number", Default: "30"},
		},
		build: func(c Config, e Env) (Source, error) {
			if err := checkDeviceName(c.Options["video"], c.Options["audio"]); err != nil {
				return nil, err
			}
			return &capture{cfg: c, env: e, driver: "avfoundation"}, nil
		},
	})
	register(Driver{
		ID: "bda", Name: "Windows digital tuner (BDA)", Status: "planned", Planned: true, Platforms: []string{"windows"},
		Description: "Digital TV cards on Windows through their own driver interface.",
		Note:        "Not built yet: it needs a COM filter graph. Until then, run such a card through an external backend.",
		Fields:      []Field{},
	})
}

// checkDeviceName refuses what FFmpeg would read as something other than a
// device: an option, a chained input, or a filter.
func checkDeviceName(names ...string) error {
	for _, n := range names {
		if strings.HasPrefix(n, "-") || strings.ContainsAny(n, "\r\n\"") || len(n) > 200 {
			return errors.New("that device name cannot be used")
		}
	}
	return nil
}

type capture struct {
	cfg    Config
	env    Env
	driver string
}

func (c *capture) Info() Info {
	return Info{Driver: c.driver, Name: c.cfg.Name, MaxStreams: 1, Detail: c.cfg.Options["video"]}
}

func (c *capture) Channels(context.Context) ([]Channel, error) {
	return []Channel{{ID: "capture", Number: "1", Name: firstNonEmpty(c.cfg.Name, c.cfg.Options["video"])}}, nil
}

func (c *capture) Scan(ctx context.Context, _ ScanOptions, p func(Progress)) ([]Channel, error) {
	return noScan(c, ctx, p)
}

func (c *capture) Close() error { return nil }

func (c *capture) Open(context.Context, Channel) (*Stream, error) {
	if err := checkDeviceName(c.cfg.Options["video"], c.cfg.Options["audio"]); err != nil {
		return nil, err
	}
	return &Stream{Input: c.input()}, nil
}

func (c *capture) input() []string {
	switch c.driver {
	case "dshow":
		in := []string{"-f", "dshow", "-rtbufsize", "256M"}
		if v := c.cfg.Options["crossbar"]; v != "" {
			in = append(in, "-crossbar_video_input_pin_number", v)
		}
		dev := "video=" + c.cfg.Options["video"]
		if a := c.cfg.Options["audio"]; a != "" {
			dev += ":audio=" + a
		}
		return append(in, "-i", dev)
	default:
		dev := c.cfg.Options["video"]
		if a := c.cfg.Options["audio"]; a != "" {
			dev += ":" + a
		}
		return []string{"-f", "avfoundation", "-framerate", firstNonEmpty(c.cfg.Options["framerate"], "30"), "-i", dev}
	}
}

// Quoted device names in FFmpeg's device listing.
var (
	dshowLine = regexp.MustCompile(`"([^"]+)"\s*\((video|audio)\)`)
	avfLine   = regexp.MustCompile(`\[(\d+)\]\s+(.+)$`)
)

// ParseDShowDevices reads the output of ffmpeg -list_devices true -f dshow.
func ParseDShowDevices(out string) (video, audio []string) {
	section := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r ")
		switch {
		case strings.Contains(line, "DirectShow video devices"):
			section = "video"
		case strings.Contains(line, "DirectShow audio devices"):
			section = "audio"
		}
		if strings.Contains(line, "Alternative name") {
			continue
		}
		if m := dshowLine.FindStringSubmatch(line); m != nil {
			if m[2] == "video" {
				video = append(video, m[1])
			} else {
				audio = append(audio, m[1])
			}
			continue
		}
		// Older FFmpeg lists names under headings.
		if i := strings.Index(line, `"`); i >= 0 && section != "" {
			if j := strings.LastIndex(line, `"`); j > i {
				name := line[i+1 : j]
				if section == "video" {
					video = append(video, name)
				} else {
					audio = append(audio, name)
				}
			}
		}
	}
	return video, audio
}

// ParseAVFDevices reads the output of ffmpeg -f avfoundation -list_devices true.
func ParseAVFDevices(out string) (video, audio []string) {
	section := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r ")
		switch {
		case strings.Contains(line, "AVFoundation video devices"):
			section = "video"
			continue
		case strings.Contains(line, "AVFoundation audio devices"):
			section = "audio"
			continue
		}
		if m := avfLine.FindStringSubmatch(line); m != nil {
			// Skip the [AVFoundation indev @ 0x…] prefix: only the last bracket counts.
			if section == "video" {
				video = append(video, m[2])
			} else if section == "audio" {
				audio = append(audio, m[2])
			}
		}
	}
	return video, audio
}

func discoverCapture(ctx context.Context, env Env) []Found {
	if env.FFmpeg == "" || (runtime.GOOS != "windows" && runtime.GOOS != "darwin") {
		return nil
	}
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var args []string
	driver := "dshow"
	if runtime.GOOS == "windows" {
		args = []string{"-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy"}
	} else {
		driver = "avfoundation"
		args = []string{"-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", ""}
	}
	cmd := env.command(c, env.FFmpeg, args...)
	var out bytes.Buffer
	cmd.Stderr = &out
	_ = cmd.Run() // FFmpeg reports an error here even when the listing worked
	var video, audio []string
	if driver == "dshow" {
		video, audio = ParseDShowDevices(out.String())
	} else {
		video, audio = ParseAVFDevices(out.String())
	}
	found := make([]Found, 0, len(video))
	for _, v := range video {
		f := Found{Driver: driver, Name: v, Options: map[string]string{"video": v}}
		if len(audio) > 0 {
			f.Options["audio"] = audio[0]
			f.Detail = fmt.Sprintf("Sound from %s", audio[0])
		}
		found = append(found, f)
	}
	return found
}

// frequencyLine reads a line of the analogue channel list: "name=frequency in MHz".
func frequencyLine(line string) (string, float64, error) {
	name, v, ok := strings.Cut(line, "=")
	if !ok {
		return "", 0, fmt.Errorf("%q should look like Channel 5=77.25", line)
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || f <= 0 || f > 3000 {
		return "", 0, fmt.Errorf("could not read the frequency in %q", line)
	}
	return strings.TrimSpace(name), f, nil
}
