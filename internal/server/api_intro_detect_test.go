// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/introdetect"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// tune is sec seconds of made-up music from a seed, at rate samples a second: a few notes
// at a time, changing twice a second. chatter is noise that is different for every seed.
func tune(seed int64, sec float64, rate int) []float64 {
	r := rand.New(rand.NewSource(seed))
	out := make([]float64, int(sec*float64(rate)))
	var f [4]float64
	for i := range out {
		if i%(rate/2) == 0 {
			for k := range f {
				f[k] = 300 * math.Pow(2, r.Float64()*2.7)
			}
		}
		for k := range f {
			out[i] += math.Sin(2*math.Pi*f[k]*float64(i)/float64(rate)) / float64(k+1)
		}
	}
	return out
}

func chatter(seed int64, sec float64, rate int) []float64 {
	r := rand.New(rand.NewSource(seed))
	out := make([]float64, int(sec*float64(rate)))
	low := 0.0
	for i := range out {
		low = 0.9*low + 0.1*(r.Float64()*2-1)
		out[i] = low * 4 * (0.5 + 0.5*math.Sin(2*math.Pi*3*float64(i)/float64(rate)))
	}
	return out
}

func s16(parts ...[]float64) []byte {
	var out []byte
	for _, p := range parts {
		for _, v := range p {
			out = binary.LittleEndian.AppendUint16(out, uint16(int16(max(-32768, min(32767, math.Round(v*6000))))))
		}
	}
	return out
}

func wav(rate int, pcm []byte) []byte {
	h := []byte("RIFF")
	h = binary.LittleEndian.AppendUint32(h, uint32(36+len(pcm)))
	h = append(h, "WAVEfmt "...)
	h = binary.LittleEndian.AppendUint32(h, 16)
	h = binary.LittleEndian.AppendUint16(h, 1) // PCM
	h = binary.LittleEndian.AppendUint16(h, 1) // mono
	h = binary.LittleEndian.AppendUint32(h, uint32(rate))
	h = binary.LittleEndian.AppendUint32(h, uint32(rate*2))
	h = binary.LittleEndian.AppendUint16(h, 2)
	h = binary.LittleEndian.AppendUint16(h, 16)
	h = append(h, "data"...)
	h = binary.LittleEndian.AppendUint32(h, uint32(len(pcm)))
	return append(h, pcm...)
}

type introFixture struct {
	ts           *testServer
	admin, adult string
	show         *library.Show
	eps          []*library.Item
	before       []float64 // seconds of talk before each episode's theme
}

func newIntroFixture(t *testing.T, ts *testServer, ext string, file func(i int) []byte) *introFixture {
	t.Helper()
	root := t.TempDir()
	f := &introFixture{ts: ts, before: []float64{4, 22, 9}}
	_, f.admin = ts.user(t, "owner", "adult", true, "")
	_, f.adult = ts.user(t, "alex", "adult", false, "")
	tv := ts.library(t, "shows", filepath.Join(root, "tv"))
	f.show = &library.Show{LibraryID: tv, Title: "A Show", SortTitle: "A Show", Path: filepath.Join(root, "tv", "A Show")}
	if err := ts.Store.UpsertShow(context.Background(), f.show); err != nil {
		t.Fatal(err)
	}
	for i := range f.before {
		season, ep := 1, i+1
		p := filepath.Join(root, "tv", "A Show", fmt.Sprintf("S01E%02d%s", ep, ext))
		put(t, p, file(i))
		f.eps = append(f.eps, ts.item(t, &library.Item{LibraryID: tv, Kind: "video", Path: p, Title: fmt.Sprintf("Episode %d", ep), ShowID: &f.show.ID, Season: &season, Episode: &ep, RuntimeSec: 80}))
	}
	return f
}

func (f *introFixture) run(t *testing.T) tasks.Task {
	t.Helper()
	w := f.ts.do(t, "POST", fmt.Sprintf("/api/shows/%d/detect-intros", f.show.ID), f.admin, map[string]any{})
	if w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body)
	}
	id, _ := decode[map[string]any](t, w)["task"].(string)
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		for _, tk := range decode[TaskList](t, f.ts.do(t, "GET", "/api/system/tasks", f.admin, nil)).Recent {
			if tk.ID == id && tk.State.Finished() {
				return tk
			}
		}
	}
	t.Fatal("the task never finished")
	return tasks.Task{}
}

func (f *introFixture) check(t *testing.T) {
	t.Helper()
	for i, ep := range f.eps {
		m := decode[mediaextras.Markers](t, f.ts.do(t, "GET", fmt.Sprintf("/api/media/%d/markers", ep.ID), f.adult, nil))
		if m.Intro == nil || m.Sources["intro"] != mediaextras.SourceDetected || math.Abs(m.Intro.StartSec-f.before[i]) > 2 || math.Abs(m.Intro.EndSec-(f.before[i]+25)) > 2 {
			t.Errorf("episode %d: intro %+v from %q, want %.0f-%.0f detected", i+1, m.Intro, m.Sources["intro"], f.before[i], f.before[i]+25)
		}
	}
}

// The whole path with made-up sound in FFmpeg's place: three episodes whose theme starts at
// different times each get an intro marker where their own theme is.
func TestDetectIntros(t *testing.T) {
	ts := newTestServer(t)
	f := newIntroFixture(t, ts, ".mkv", func(int) []byte { return []byte("not really video") })
	theme := tune(1, 25, introdetect.SampleRate)
	introDetectPCM = func(_ context.Context, _ *Server, it *library.Item, tail bool) ([]byte, error) {
		if tail {
			return nil, fmt.Errorf("no tail")
		}
		i := *it.Episode - 1
		return s16(chatter(int64(100+i), f.before[i], introdetect.SampleRate), theme, chatter(int64(200+i), 30, introdetect.SampleRate)), nil
	}
	t.Cleanup(func() { introDetectPCM = nil })

	for name, tok := range map[string]string{"signed out": "", "not an administrator": f.adult} {
		want := 403
		if tok == "" {
			want = 401
		}
		if w := ts.do(t, "POST", fmt.Sprintf("/api/shows/%d/detect-intros", f.show.ID), tok, map[string]any{}); w.Code != want {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if w := ts.do(t, "POST", "/api/shows/999999/detect-intros", f.admin, map[string]any{}); w.Code != 404 {
		t.Errorf("no such show: %d", w.Code)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/shows/%d/detect-intros", f.show.ID), f.admin, map[string]any{"season": 7}); w.Code != 409 {
		t.Errorf("a season with no episodes: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "POST", fmt.Sprintf("/api/shows/%d/detect-intros", f.show.ID), f.admin, map[string]any{"season": -1}); w.Code != 400 {
		t.Errorf("a season that isn't one: %d", w.Code)
	}
	tk := f.run(t)
	if tk.State != tasks.Done || tk.Done != 3 || tk.Total != 3 || tk.Type != tasks.TypeIntro {
		t.Fatalf("task: %+v", tk)
	}
	f.check(t)

	// What a person set still wins, and taking it away brings the detected one back.
	ep := f.eps[0]
	if w := ts.do(t, "PUT", fmt.Sprintf("/api/media/%d/markers", ep.ID), f.admin, map[string]any{"intro": map[string]any{"start": 1, "end": 2}}); w.Code != 200 {
		t.Fatalf("manual marker: %d %s", w.Code, w.Body)
	}
	if m := decode[mediaextras.Markers](t, ts.do(t, "GET", fmt.Sprintf("/api/media/%d/markers", ep.ID), f.adult, nil)); m.Sources["intro"] != mediaextras.SourceManual || m.Intro.EndSec != 2 {
		t.Errorf("a manual marker lost to a detected one: %+v", m)
	}
	// Episodes that no longer share anything lose their detected markers on the next run.
	introDetectPCM = func(_ context.Context, _ *Server, it *library.Item, _ bool) ([]byte, error) {
		return s16(chatter(int64(900+*it.Episode), 60, introdetect.SampleRate)), nil
	}
	if tk := f.run(t); tk.State != tasks.Done {
		t.Fatalf("second run: %+v", tk)
	}
	if m := decode[mediaextras.Markers](t, ts.do(t, "GET", fmt.Sprintf("/api/media/%d/markers", f.eps[1].ID), f.adult, nil)); m.Intro != nil {
		t.Errorf("a stale detected intro was kept: %+v", m.Intro)
	}
}

// The same with real files and the real FFmpeg, when there is one: WAV files at another
// sample rate, decoded and resampled by FFmpeg as an episode's sound would be.
func TestDetectIntrosWithFFmpeg(t *testing.T) {
	tools, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("no FFmpeg here")
	}
	ts := newTestServerWith(t, tools)
	before := []float64{4, 22, 9}
	theme := tune(1, 25, 22050)
	f := newIntroFixture(t, ts, ".wav", func(i int) []byte {
		return wav(22050, s16(chatter(int64(100+i), before[i], 22050), theme, chatter(int64(200+i), 30, 22050)))
	})
	if tk := f.run(t); tk.State != tasks.Done {
		t.Fatalf("task: %+v", tk)
	}
	f.check(t)
}
