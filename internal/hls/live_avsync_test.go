// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// A channel's stream across programme changes (#687): each programme is a run of its own, joined
// part way through (a copy, and a conversion) or from the start. In every run the sound and the
// picture start and end together, neither has a hole, the picture never stands still, and every
// join is a discontinuity. Before #687 a copy joined part way through started with the picture
// alone, up to a keyframe's distance, and players froze the picture there or played on out of step.
func TestLiveAVSyncAcrossProgrammes(t *testing.T) {
	bin := realFFmpeg(t)
	probe := os.Getenv("WMS_FFPROBE")
	if probe == "" {
		probe = filepath.Join(filepath.Dir(bin), "ffprobe"+filepath.Ext(bin))
		if _, err := os.Stat(probe); err != nil {
			if probe, err = exec.LookPath("ffprobe"); err != nil {
				t.Skip("no ffprobe here")
			}
		}
	}
	dir := t.TempDir()
	// Three programmes: H.264 a browser takes as it is (so a channel copies it), a keyframe every
	// 2 s, a picture that changes every frame, and a tone of its own each.
	clip := func(name string, hz int) string {
		p := filepath.Join(dir, name+".mkv")
		out, err := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25", "-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d:sample_rate=48000", hz),
			"-t", "10", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "50", "-keyint_min", "50", "-sc_threshold", "0",
			"-c:a", "aac", "-shortest", p).CombinedOutput()
		if err != nil {
			t.Skipf("can't make a programme: %v %s", err, out)
		}
		return p
	}
	a, b, c := clip("a", 440), clip("b", 660), clip("c", 880)
	inputs := []*LiveInput{
		{InputArgs: ffmpeg.FileInput(a), Seek: 3.3, Duration: 4.7, AudioIndex: 0, CopyVideo: true}, // joined inside a GOP: picture from 2.0
		{InputArgs: ffmpeg.FileInput(b), Duration: 6, AudioIndex: 0, CopyVideo: true},              // from the start
		{InputArgs: ffmpeg.FileInput(c), Seek: 5.1, Duration: 3.9, AudioIndex: 0, CopyVideo: true}, // inside a GOP again
		{InputArgs: ffmpeg.FileInput(a), Seek: 2.5, Duration: 4, AudioIndex: 0, Height: 180},       // converted
	}
	m := liveTestManager(t)
	m.tools.FFmpeg, m.tools.FFprobe = bin, probe
	var next atomic.Int32
	done := make(chan struct{})
	s := m.newLive(LiveSpec{ID: "avsync", SegmentSec: 2, WindowSec: 600, Feed: func(ctx context.Context, _ time.Time) (*LiveInput, error) {
		i := int(next.Add(1)) - 1
		if i < len(inputs) {
			return inputs[i], nil
		}
		close(done)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	m.live.sessions = map[string]*LiveSession{"avsync": s}
	go s.loop()
	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		t.Fatalf("the channel didn't get through its programmes; %d asked for", next.Load())
	}
	playlist := string(s.Playlist(func(f string) string { return f }))
	if err := s.Err(); err != nil {
		t.Fatalf("the stream failed: %v", err)
	}
	if n := strings.Count(playlist, "#EXT-X-DISCONTINUITY\n"); n != len(inputs)-1 {
		t.Errorf("%d joins marked, want %d:\n%s", n, len(inputs)-1, playlist)
	}
	// The segments of each run, in order.
	runs := map[string][]string{}
	var order []string
	for _, line := range strings.Split(playlist, "\n") {
		if strings.HasSuffix(line, ".ts") {
			r := strings.SplitN(line, "-", 2)[0]
			if runs[r] == nil {
				order = append(order, r)
			}
			runs[r] = append(runs[r], filepath.Join(s.Dir, line))
		}
	}
	if len(order) != len(inputs) {
		t.Fatalf("%d runs in the playlist, want %d:\n%s", len(order), len(inputs), playlist)
	}
	for i, r := range order {
		v, au := packets(t, probe, runs[r])
		if len(v) == 0 || len(au) == 0 {
			t.Errorf("run %d: %d pictures, %d sound frames", i+1, len(v), len(au))
			continue
		}
		// Start and end together: a player lines the next run up by where this one ends.
		if d := au[0].t - v[0].t; math.Abs(d) > 0.1 {
			t.Errorf("run %d: the sound starts %.3f s after the picture", i+1, d)
		}
		// The sound may run on a moment past the picture (the next run's takes over); it mustn't
		// stop short, which leaves a hole.
		if d := au[len(au)-1].end() - v[len(v)-1].end(); d < -0.1 || d > 0.3 {
			t.Errorf("run %d: the sound ends %.3f s after the picture", i+1, d)
		}
		// No holes, across the run's segments too.
		if g := biggestGap(v); g > 0.5/25 {
			t.Errorf("run %d: %.3f s without a picture", i+1, g)
		}
		if g := biggestGap(au); g > 0.05 {
			t.Errorf("run %d: %.3f s without sound", i+1, g)
		}
		// The picture never stands still: the test picture changes every frame.
		if frozen := freezes(t, bin, dir, r, runs[r]); frozen != "" {
			t.Errorf("run %d: the picture stands still: %s", i+1, frozen)
		}
	}
}

type pkt struct{ t, d float64 }

func (p pkt) end() float64 { return p.t + p.d }

// packets reads a run's segments as one stream: the pictures' and the sound's times, in order.
func packets(t *testing.T, probe string, files []string) (video, audio []pkt) {
	t.Helper()
	for _, f := range files {
		out, err := exec.Command(probe, "-v", "error", "-show_entries", "packet=codec_type,pts_time,duration_time", "-show_entries", "stream=index,codec_type",
			"-of", "json", f).Output()
		if err != nil {
			t.Fatalf("ffprobe %s: %v", f, err)
		}
		var j struct {
			Packets []struct {
				Type string `json:"codec_type"`
				PTS  string `json:"pts_time"`
				Dur  string `json:"duration_time"`
			} `json:"packets"`
		}
		if err := json.Unmarshal(out, &j); err != nil {
			t.Fatal(err)
		}
		for _, p := range j.Packets {
			var x pkt
			_, _ = fmt.Sscan(p.PTS, &x.t)
			_, _ = fmt.Sscan(p.Dur, &x.d)
			switch p.Type {
			case "video":
				video = append(video, x)
			case "audio":
				audio = append(audio, x)
			}
		}
	}
	sort.Slice(video, func(i, j int) bool { return video[i].t < video[j].t })
	sort.Slice(audio, func(i, j int) bool { return audio[i].t < audio[j].t })
	return video, audio
}

// biggestGap is the longest stretch between the end of one packet and the start of the next.
func biggestGap(ps []pkt) float64 {
	g := 0.0
	for i := 1; i < len(ps); i++ {
		g = max(g, ps[i].t-ps[i-1].end())
	}
	return g
}

// freezes decodes a run's segments and says where FFmpeg's freezedetect finds the picture still.
func freezes(t *testing.T, bin, dir, run string, files []string) string {
	t.Helper()
	list := filepath.Join(dir, run+".txt")
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "file '%s'\n", filepath.ToSlash(f))
	}
	if err := os.WriteFile(list, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "-hide_banner", "-nostdin", "-f", "concat", "-safe", "0", "-i", list,
		"-vf", "freezedetect=n=0.001:d=0.2", "-map", "0:v:0", "-f", "null", "-").CombinedOutput()
	if err != nil {
		t.Fatalf("decoding run %s: %v %s", run, err, out)
	}
	var found []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "freeze_start") || strings.Contains(line, "freeze_duration") {
			found = append(found, strings.TrimSpace(line[strings.Index(line, "]")+1:]))
		}
	}
	return strings.Join(found, "; ")
}
