package wittytuner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// V4L2: analogue TV tuner and capture cards on Linux. FFmpeg captures; the
// tuner's frequency and input are set directly (v4l2_linux.go). Not yet tried
// with real hardware.

func init() {
	register(Driver{
		ID: "v4l2", Name: "Analogue and capture cards (V4L2)", Status: "untested", Platforms: []string{"linux"},
		Description: "Analogue TV tuners, composite, S-Video and HDMI capture on Linux. Not yet tried with a real card.",
		Fields: []Field{
			{Key: "device", Label: "Video device", Kind: "path", Required: true, Default: "/dev/video0"},
			{Key: "audio", Label: "ALSA audio device", Kind: "text", Help: "Such as hw:1,0. Optional."},
			{Key: "input", Label: "Input number", Kind: "number", Default: "0"},
			{Key: "standard", Label: "TV standard", Kind: "select", Options: []string{"", "ntsc", "pal", "secam"}, Help: "Optional."},
			{Key: "channels", Label: "Analogue channels", Kind: "textarea", Help: "Tuners only, one per line: name=frequency in MHz, such as Channel 5=77.25. Leave empty for a capture input."},
			{Key: "interlaced", Label: "The picture is interlaced", Kind: "toggle", Default: "1"},
		},
		build: func(c Config, e Env) (Source, error) {
			dev := c.Options["device"]
			if !strings.HasPrefix(dev, "/dev/video") || strings.ContainsAny(dev[len("/dev/video"):], "/.") {
				return nil, errors.New("the video device looks like /dev/video0")
			}
			if a := c.Options["audio"]; strings.HasPrefix(a, "-") {
				return nil, errors.New("that audio device cannot be used")
			}
			return &v4l2{cfg: c, env: e}, nil
		},
	})
}

type v4l2 struct {
	cfg Config
	env Env
}

func (v *v4l2) Info() Info {
	return Info{Driver: "v4l2", Name: v.cfg.Name, MaxStreams: 1, Detail: v.cfg.Options["device"]}
}

func (v *v4l2) Channels(context.Context) ([]Channel, error) {
	text := strings.TrimSpace(v.cfg.Options["channels"])
	if text == "" {
		return []Channel{{ID: "capture", Number: "1", Name: firstNonEmpty(v.cfg.Name, "Capture")}}, nil
	}
	var out []Channel
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		name, f, err := frequencyLine(line)
		if err != nil {
			return nil, err
		}
		out = append(out, Channel{ID: strconv.FormatFloat(f, 'f', 3, 64), Number: strconv.Itoa(len(out) + 1), Name: name, Locator: strconv.FormatFloat(f, 'f', 3, 64)})
	}
	return out, nil
}

func (v *v4l2) Scan(ctx context.Context, _ ScanOptions, p func(Progress)) ([]Channel, error) {
	return noScan(v, ctx, p)
}

func (v *v4l2) Close() error { return nil }

func (v *v4l2) Open(_ context.Context, ch Channel) (*Stream, error) {
	input, _ := strconv.Atoi(firstNonEmpty(v.cfg.Options["input"], "0"))
	freq, _ := strconv.ParseFloat(ch.Locator, 64)
	if err := setV4L2(v.cfg.Options["device"], input, freq); err != nil {
		return nil, fmt.Errorf("the card could not be set up: %w", err)
	}
	return &Stream{Input: v.input()}, nil
}

func (v *v4l2) input() []string {
	in := []string{"-f", "v4l2", "-thread_queue_size", "1024"}
	if std := v.cfg.Options["standard"]; std != "" {
		in = append(in, "-standard", strings.ToUpper(std))
	}
	in = append(in, "-i", v.cfg.Options["device"])
	if a := v.cfg.Options["audio"]; a != "" {
		in = append(in, "-f", "alsa", "-thread_queue_size", "1024", "-i", a)
	}
	return in
}
