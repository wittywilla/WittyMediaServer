// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// waitTask waits for a task to end and answers it.
func waitTask(t *testing.T, ts *testServer, admin, id string) tasks.Task {
	t.Helper()
	for deadline := time.Now().Add(90 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		for _, tk := range decode[TaskList](t, ts.do(t, "GET", "/api/system/tasks", admin, nil)).Recent {
			if tk.ID == id && tk.State.Finished() {
				return tk
			}
		}
	}
	t.Fatalf("task %s never finished", id)
	return tasks.Task{}
}

type stemsAnswer struct {
	Stems []struct {
		Name, Source, URL, MIME string
		Approximate             bool
	}
	Separation separationInfo
}

func TestStemSeparationRefusals(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	song, video := f.url("/api/media/%d/stems", f.song), f.url("/api/media/%d/stems", f.video)
	// This test server has no FFmpeg and no separator: the listing says so, and so do the routes.
	got := decode[stemsAnswer](t, ts.do(t, "GET", song, f.adult, nil))
	if got.Separation.Approximate.Available || got.Separation.Approximate.Reason != "no-ffmpeg" || got.Separation.External.Available || got.Separation.External.Reason != "not-configured" {
		t.Errorf("separation: %+v", got.Separation)
	}
	if got := decode[stemsAnswer](t, ts.do(t, "GET", video, f.adult, nil)); got.Separation.Approximate.Reason != "not-a-song" {
		t.Errorf("a film's separation: %+v", got.Separation)
	}
	for _, c := range []struct {
		name, path, tok string
		want            int
	}{
		{"approximate without FFmpeg", song + "/approximate", f.adult, 503},
		{"approximate, signed out", song + "/approximate", "", 401},
		{"approximate, library not allowed", song + "/approximate", f.blocked, 404},
		{"approximate for a film", video + "/approximate", f.adult, 400},
		{"separate, not an administrator", song + "/separate", f.adult, 403},
		{"separate, signed out", song + "/separate", "", 401},
		{"separate with none set up", song + "/separate", f.admin, 409},
		{"separate for a film", video + "/separate", f.admin, 400},
		{"no such song", "/api/media/999999/stems/separate", f.admin, 404},
	} {
		if w := ts.do(t, "POST", c.path, c.tok, map[string]any{}); w.Code != c.want {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body)
		}
	}
	// A command without its two placeholders is refused before anything runs.
	ts.Cfg.StemSeparator = "separator --everything"
	if w := ts.do(t, "POST", song+"/separate", f.admin, nil); w.Code != 409 || !strings.Contains(w.Body.String(), "{input}") {
		t.Errorf("a command with no placeholders: %d %s", w.Code, w.Body)
	}
}

// The owner's separator, stood in for by a script: it is given the song and a folder, and
// what it writes there becomes the song's stems.
func TestStemSeparatorRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in separator is a shell script")
	}
	f := newExtrasFixture(t)
	ts := f.ts
	bin := t.TempDir()
	script := filepath.Join(bin, "fake separator.sh")
	// Like Demucs: <output>/<model>/<song>/<part>. One part isn't audio at all.
	put(t, script, []byte("#!/bin/sh\nset -e\n[ \"$1\" = fail ] && { echo 'out of memory' >&2; exit 3; }\nmkdir -p \"$3/model/song\"\ncp \"$2\" \"$3/model/song/vocals.flac\"\ncp \"$2\" \"$3/model/song/no_vocals.flac\"\necho 'not audio' > \"$3/model/song/drums.wav\"\n"))
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	ts.Cfg.StemSeparator = `"` + script + `" run {input} {output}`
	path := f.url("/api/media/%d/stems", f.song)
	if got := decode[stemsAnswer](t, ts.do(t, "GET", path, f.admin, nil)); !got.Separation.External.Available {
		t.Fatalf("separation for an administrator: %+v", got.Separation)
	}
	if got := decode[stemsAnswer](t, ts.do(t, "GET", path, f.adult, nil)); got.Separation.External.Available || got.Separation.External.Reason != "admin-only" {
		t.Errorf("separation for someone else: %+v", got.Separation)
	}
	w := ts.do(t, "POST", path+"/separate", f.admin, nil)
	if w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	tk := waitTask(t, ts, f.admin, fmt.Sprint(decode[map[string]any](t, w)["task"]))
	if tk.State != tasks.Done || !strings.Contains(tk.Result, "vocals") {
		t.Fatalf("task: %+v", tk)
	}
	got := decode[stemsAnswer](t, ts.do(t, "GET", path, f.adult, nil))
	names := map[string]string{}
	for _, s := range got.Stems {
		names[s.Name] = s.Source
	}
	if names["vocals"] != "separated" || names["instrumental"] != "separated" || names["drums"] != "" {
		t.Errorf("stems after separating: %v", names)
	}
	if w := ts.do(t, "GET", path+"/vocals", f.adult, nil); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "fLaC") {
		t.Errorf("a separated stem: %d", w.Code)
	}
	// An upload over a separated part is an upload again.
	if w := ts.do(t, "PUT", path+"/vocals", f.admin, testFLAC); w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	for _, s := range decode[stemsAnswer](t, ts.do(t, "GET", path, f.adult, nil)).Stems {
		if s.Name == "vocals" && s.Source != "upload" {
			t.Errorf("an uploaded stem is listed as %q", s.Source)
		}
	}
	// A separator that fails: the task fails with what it said, and nothing changes.
	ts.Cfg.StemSeparator = `"` + script + `" fail {input} {output}`
	w = ts.do(t, "POST", path+"/separate", f.admin, nil)
	tk = waitTask(t, ts, f.admin, fmt.Sprint(decode[map[string]any](t, w)["task"]))
	if tk.State != tasks.Failed || !strings.Contains(tk.Error+tk.Detail, "out of memory") {
		t.Errorf("a failing separator: %+v", tk)
	}
	// A program that isn't there.
	ts.Cfg.StemSeparator = filepath.Join(bin, "nothing-here") + " {input} {output}"
	w = ts.do(t, "POST", path+"/separate", f.admin, nil)
	if tk = waitTask(t, ts, f.admin, fmt.Sprint(decode[map[string]any](t, w)["task"])); tk.State != tasks.Failed {
		t.Errorf("a missing separator: %+v", tk)
	}
}

// The queue is bounded, and the same job isn't queued twice.
func TestMediaJobsAreBounded(t *testing.T) {
	ts := newTestServer(t)
	admin, _ := ts.user(t, "owner", "adult", true, "")
	release := make(chan struct{})
	block := func(ctx context.Context, _ *tasks.Handle) error {
		select {
		case <-release:
		case <-ctx.Done():
		}
		return ctx.Err()
	}
	for i := 0; i < maxMediaJobs; i++ {
		if _, err := ts.startMediaJob(fmt.Sprintf("job:%d", i), admin, tasks.Spec{Title: "A job"}, block); err != nil {
			t.Fatalf("job %d: %v", i, err)
		}
	}
	if _, err := ts.startMediaJob("job:0", admin, tasks.Spec{Title: "A job"}, block); err != errJobQueued {
		t.Errorf("the same job again: %v", err)
	}
	if _, err := ts.startMediaJob("one-more", admin, tasks.Spec{Title: "A job"}, block); err != errJobsFull {
		t.Errorf("one more than the queue holds: %v", err)
	}
	if ts.mediaJobTask("job:3") == "" {
		t.Error("a queued job has no task")
	}
	close(release)
	for deadline := time.Now().Add(10 * time.Second); ts.mediaJobTask("job:15") != "" && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
	}
	if _, err := ts.startMediaJob("one-more", admin, tasks.Spec{Title: "A job"}, func(context.Context, *tasks.Handle) error { return nil }); err != nil {
		t.Errorf("after the queue emptied: %v", err)
	}
}

// stereoWAV is 16-bit stereo sound from two channels of samples.
func stereoWAV(rate int, left, right []float64) []byte {
	pcm := make([]byte, 0, len(left)*4)
	for i := range left {
		for _, v := range []float64{left[i], right[i]} {
			pcm = binary.LittleEndian.AppendUint16(pcm, uint16(int16(math.Round(v*8000))))
		}
	}
	h := wav(rate, pcm)
	binary.LittleEndian.PutUint16(h[22:], 2)              // channels
	binary.LittleEndian.PutUint32(h[28:], uint32(rate*4)) // bytes a second
	binary.LittleEndian.PutUint16(h[32:], 4)              // bytes a frame
	return h
}

// tone is how strong one frequency is in mono 16-bit sound (Goertzel).
func tone(pcm []byte, rate int, hz float64) float64 {
	w := 2 * math.Pi * hz / float64(rate)
	c := 2 * math.Cos(w)
	var s1, s2 float64
	n := len(pcm) / 2
	for i := 0; i < n; i++ {
		s := float64(int16(binary.LittleEndian.Uint16(pcm[2*i:]))) + c*s1 - s2
		s2, s1 = s1, s
	}
	return math.Sqrt(s1*s1+s2*s2-c*s1*s2) / float64(n)
}

// With a real FFmpeg: a "voice" in the middle of a stereo mix is taken out, and what is
// only on one side stays. The result is listed as approximate, and plays.
func TestApproximateInstrumentalWithFFmpeg(t *testing.T) {
	tools, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("no FFmpeg here")
	}
	ts := newTestServerWith(t, tools)
	root := t.TempDir()
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "alex", "adult", false, "")
	music := ts.library(t, "music", filepath.Join(root, "music"))
	const rate = 44100
	n := rate * 4
	left, right, mono := make([]float64, n), make([]float64, n), make([]float64, n)
	for i := range left {
		ti := float64(i) / rate
		voice := math.Sin(2 * math.Pi * 1000 * ti) // the same in both: the centre
		left[i] = voice + math.Sin(2*math.Pi*400*ti)
		right[i] = voice + math.Sin(2*math.Pi*600*ti)
		mono[i] = voice
	}
	songPath, monoPath := filepath.Join(root, "music", "Album", "Song.wav"), filepath.Join(root, "music", "Album", "Mono.wav")
	put(t, songPath, stereoWAV(rate, left, right))
	put(t, monoPath, wav(rate, s16(mono)))
	song := ts.item(t, &library.Item{LibraryID: music, Kind: "audio", Path: songPath, Title: "Song"})
	monoSong := ts.item(t, &library.Item{LibraryID: music, Kind: "audio", Path: monoPath, Title: "Mono"})
	path := fmt.Sprintf("/api/media/%d/stems", song.ID)
	if got := decode[stemsAnswer](t, ts.do(t, "GET", path, adult, nil)); !got.Separation.Approximate.Available || got.Separation.Approximate.Note == "" || len(got.Stems) != 0 {
		t.Fatalf("before: %+v", got)
	}
	w := ts.do(t, "POST", path+"/approximate", adult, nil)
	if w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	if tk := waitTask(t, ts, admin, fmt.Sprint(decode[map[string]any](t, w)["task"])); tk.State != tasks.Done {
		t.Fatalf("task: %+v", tk)
	}
	got := decode[stemsAnswer](t, ts.do(t, "GET", path, adult, nil))
	if len(got.Stems) != 1 || got.Stems[0].Name != "instrumental" || !got.Stems[0].Approximate || got.Stems[0].Source != "approximate" || got.Stems[0].MIME != "audio/mp4" {
		t.Fatalf("after: %+v", got.Stems)
	}
	// Asking again makes nothing: it's there.
	if w := ts.do(t, "POST", path+"/approximate", adult, nil); w.Code != 200 || decode[map[string]any](t, w)["exists"] != true {
		t.Errorf("asking again: %d %s", w.Code, w.Body)
	}
	file := ts.do(t, "GET", path+"/instrumental", adult, nil)
	if file.Code != 200 || file.Header().Get("Content-Type") != "audio/mp4" {
		t.Fatalf("the file: %d %s", file.Code, file.Header().Get("Content-Type"))
	}
	if w := ts.do(t, "GET", path+"/instrumental", adult, nil, "Range", "bytes=4-7"); w.Code != 206 || w.Body.String() != "ftyp" {
		t.Errorf("a range of it: %d %q", w.Code, w.Body.String())
	}
	out := filepath.Join(root, "out.m4a")
	put(t, out, file.Body.Bytes())
	pcm, err := exec.Command(tools.FFmpeg, "-v", "error", "-i", out, "-ac", "1", "-ar", "44100", "-f", "s16le", "-").Output()
	if err != nil || len(pcm) < rate {
		t.Fatalf("decoding the result: %v, %d bytes", err, len(pcm))
	}
	voice, side := tone(pcm, rate, 1000), tone(pcm, rate, 400)+tone(pcm, rate, 600)
	if side < 50 || voice > side/20 {
		t.Errorf("the centre wasn't removed: 1 kHz voice %.1f, sides %.1f", voice, side)
	}
	// A real stem replaces the approximate one in the listing.
	if w := ts.do(t, "PUT", path+"/instrumental", admin, testFLAC); w.Code != 200 {
		t.Fatalf("upload: %d", w.Code)
	}
	if got := decode[stemsAnswer](t, ts.do(t, "GET", path, adult, nil)); len(got.Stems) != 1 || got.Stems[0].Approximate || got.Stems[0].Source != "upload" {
		t.Errorf("with a real instrumental: %+v", got.Stems)
	}
	// A mono song has nothing to cancel: the task says so.
	w = ts.do(t, "POST", fmt.Sprintf("/api/media/%d/stems/approximate", monoSong.ID), adult, nil)
	if tk := waitTask(t, ts, admin, fmt.Sprint(decode[map[string]any](t, w)["task"])); tk.State != tasks.Failed || !strings.Contains(tk.Error+tk.Detail, "mono") {
		t.Errorf("a mono song: %+v", tk)
	}
	// Whoever asked is told why by the listing too: not everyone may see tasks.
	type listing struct {
		Separation separationInfo `json:"separation"`
	}
	var said string
	for i := 0; i < 100 && said == ""; i++ { // the reason is kept a moment after the task ends
		said = decode[listing](t, ts.do(t, "GET", fmt.Sprintf("/api/media/%d/stems", monoSong.ID), adult, nil)).Separation.Approximate.Error
		if said == "" {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !strings.Contains(said, "mono") || strings.Contains(said, root) {
		t.Errorf("the listing after a mono song failed says %q", said)
	}
	if l := decode[listing](t, ts.do(t, "GET", fmt.Sprintf("/api/media/%d/stems", song.ID), adult, nil)); l.Separation.Approximate.Error != "" {
		t.Errorf("a song that worked has an error: %q", l.Separation.Approximate.Error)
	}
}

// With a real FFmpeg that can encode H.264: a canvas is cut from a video, small, silent,
// and as long as asked; one beside the media that is far too big isn't taken for one.
func TestCanvasGenerateWithFFmpeg(t *testing.T) {
	tools, err := ffmpeg.Locate("", "")
	if err != nil || !tools.Encoders["libx264"] {
		t.Skip("no FFmpeg with libx264 here")
	}
	ts := newTestServerWith(t, tools)
	root := t.TempDir()
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "alex", "adult", false, "")
	movies := ts.library(t, "movies", filepath.Join(root, "movies"))
	videoPath := filepath.Join(root, "movies", "Film", "Film.mp4")
	if err := os.MkdirAll(filepath.Dir(videoPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tools.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=duration=20:size=1280x960:rate=30", "-f", "lavfi", "-i", "sine=frequency=440:duration=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", videoPath).CombinedOutput(); err != nil {
		t.Skipf("couldn't make a test video: %v %s", err, out)
	}
	video := ts.item(t, &library.Item{LibraryID: movies, Kind: "video", Path: videoPath, Title: "Film", RuntimeSec: 20})
	path := fmt.Sprintf("/api/media/%d/canvas", video.ID)
	if c := decode[canvasInfo](t, ts.do(t, "GET", path, admin, nil)); c.Available || !c.CanGenerate {
		t.Fatalf("before: %+v", c)
	}
	if c := decode[canvasInfo](t, ts.do(t, "GET", path, adult, nil)); c.CanGenerate {
		t.Error("someone who isn't an administrator is told they can make one")
	}
	for name, c := range map[string]struct {
		tok  string
		body map[string]any
		want int
	}{
		"not an administrator": {adult, map[string]any{}, 403},
		"too long":             {admin, map[string]any{"durationSec": 100}, 400},
		"too short":            {admin, map[string]any{"durationSec": 0.5}, 400},
		"past the end":         {admin, map[string]any{"startSec": 18, "durationSec": 5}, 400},
		"before the start":     {admin, map[string]any{"startSec": -3}, 400},
	} {
		if w := ts.do(t, "POST", path+"/generate", c.tok, c.body); w.Code != c.want {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	w := ts.do(t, "POST", path+"/generate", admin, map[string]any{"startSec": 2, "durationSec": 3})
	if w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	if tk := waitTask(t, ts, admin, fmt.Sprint(decode[map[string]any](t, w)["task"])); tk.State != tasks.Done {
		t.Fatalf("task: %+v", tk)
	}
	c := decode[canvasInfo](t, ts.do(t, "GET", path, adult, nil))
	if !c.Available || c.Source != "generated" || c.MIME != "video/mp4" || c.Height > 720 || c.Height == 0 || c.DurationMs < 2700 || c.DurationMs > 3300 {
		t.Fatalf("the canvas made: %+v", c)
	}
	file := ts.do(t, "GET", path+"/file", adult, nil)
	if file.Code != 200 || string(file.Body.Bytes()[4:8]) != "ftyp" || file.Body.Len() > 4<<20 {
		t.Fatalf("the file: %d, %d bytes", file.Code, file.Body.Len())
	}
	made := filepath.Join(root, "made.mp4")
	put(t, made, file.Body.Bytes())
	p, err := tools.ProbeFile(context.Background(), made)
	if err != nil || len(p.Audio()) != 0 || p.Video() == nil {
		t.Errorf("the canvas should be video with no sound: %v %+v", err, p)
	}
	// An upload over it is an upload again.
	if w := ts.do(t, "PUT", path, admin, testMP4); w.Code != 200 {
		t.Fatalf("upload: %d", w.Code)
	}
	if c := decode[canvasInfo](t, ts.do(t, "GET", path, adult, nil)); c.Source != "upload" {
		t.Errorf("after an upload: %+v", c)
	}
}

// A file beside the media that is too big to be a canvas isn't listed or served as one.
func TestCanvasSidecarLimits(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	big := filepath.Join(filepath.Dir(f.song.Path), "canvas.mp4")
	put(t, big, testMP4)
	if err := os.Truncate(big, 65<<20); err != nil { // sparse: takes no room
		t.Fatal(err)
	}
	c := decode[canvasInfo](t, ts.do(t, "GET", f.url("/api/media/%d/canvas", f.song), f.adult, nil))
	if c.Available || c.Reason != "too-big" || c.URL != "" {
		t.Errorf("a 65 MB canvas: %+v", c)
	}
	if w := ts.do(t, "GET", f.url("/api/media/%d/canvas/file", f.song), f.adult, nil); w.Code != 404 {
		t.Errorf("its file: %d", w.Code)
	}
	// A song or a disc has nothing to cut a canvas from.
	if w := ts.do(t, "POST", f.url("/api/media/%d/canvas/generate", f.song), f.admin, map[string]any{}); w.Code != 400 {
		t.Errorf("generate for a song: %d", w.Code)
	}
	if w := ts.do(t, "POST", f.url("/api/media/%d/canvas/generate", f.video), f.admin, map[string]any{}); w.Code != 503 {
		t.Errorf("generate with no FFmpeg: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", f.url("/api/media/%d/canvas/generate", f.video), f.blocked, map[string]any{}); w.Code != 403 {
		t.Errorf("generate without being an administrator: %d", w.Code)
	}
}
