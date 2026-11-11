// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libtranscode

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Automatic runs, with real files and the real FFmpeg, and real scans.

// autoLab is a lab with this turned on for its library, with new files settling fast.
func autoLab(t *testing.T) *lab {
	t.Helper()
	l := newLab(t)
	l.engine.AutoSettle = 300 * time.Millisecond
	l.scanner.AfterScan = l.engine.ScanFinished
	t.Cleanup(l.engine.StopAuto)
	l.setAuto(true)
	return l
}

func (l *lab) setAuto(on bool) {
	l.t.Helper()
	// Files dated before this second are from before it was turned on.
	time.Sleep(1100 * time.Millisecond)
	p := preset(l.t, PresetCompatible)
	if err := SaveRule(context.Background(), l.db, Rule{LibraryID: l.lib.ID, Policy: p, Output: OutputKeep, Auto: on}); err != nil {
		l.t.Fatal(err)
	}
}

func (l *lab) old(name string) string {
	l.t.Helper()
	return l.gen(name, join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
}

// settled waits for the watcher to go away and every run to end.
func (l *lab) settled() {
	l.t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		l.engine.mu.Lock()
		quiet := len(l.engine.auto) == 0 && len(l.engine.live) == 0
		l.engine.mu.Unlock()
		if quiet {
			return
		}
		if time.Now().After(deadline) {
			l.t.Fatal("automatic conversion never settled")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (l *lab) runs() []*Run {
	l.t.Helper()
	r, err := l.engine.ListRuns(context.Background(), l.lib.ID, 50)
	if err != nil {
		l.t.Fatal(err)
	}
	return r
}

func TestAutoIsOffUnlessAskedFor(t *testing.T) {
	l := newLab(t)
	l.engine.AutoSettle = 100 * time.Millisecond
	l.scanner.AfterScan = l.engine.ScanFinished
	src := l.old("Left Alone.avi")
	l.scan()
	time.Sleep(600 * time.Millisecond)
	l.settled()
	if !exists(src) || len(l.runs()) != 0 {
		t.Fatalf("converted something nobody asked for: %+v", l.runs())
	}
	if r := LoadRule(context.Background(), l.db, l.lib.ID); r.Auto {
		t.Fatal("a library starts with this on")
	}
}

func TestAutoConvertsOnlyWhatsNew(t *testing.T) {
	l := newLab(t)
	l.engine.AutoSettle = 300 * time.Millisecond
	t.Cleanup(l.engine.StopAuto)
	before := l.old("Already Here.avi")
	l.scan() // with nothing listening: it's from before this was turned on
	l.scanner.AfterScan = l.engine.ScanFinished
	l.setAuto(true)
	src := l.old("New Film.avi")
	l.scan()
	it := l.item(src)
	l.settled()
	out := strings.TrimSuffix(src, ".avi") + ".mp4"
	if !exists(out) || exists(src) {
		t.Fatalf("the new file wasn't converted: %v %v", exists(out), exists(src))
	}
	if !exists(before) {
		t.Fatal("touched what was there before")
	}
	if l.item(out).ID != it.ID {
		t.Fatal("the item changed")
	}
	runs := l.runs()
	if len(runs) != 1 || !runs[0].Auto || runs[0].Summary.Done != 1 || runs[0].Output != OutputKeep {
		t.Fatalf("%+v", runs)
	}
	if !exists(filepath.Join(l.root, library.OriginalsFolder, "New Film.avi")) {
		t.Fatal("the recipe says keep the original")
	}
	// It's in the activity history, under a name with no path in it.
	found := false
	for _, task := range l.tasks.Recent() {
		if task.Title == "Converting new files in "+l.lib.Name && task.Result != "" && task.LibraryID == l.lib.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("nothing in the activity panel: %+v", l.tasks.Recent())
	}
	// Another scan, and another, find nothing to do and leave no trace.
	l.scan()
	l.settled()
	l.scan()
	l.settled()
	if got := l.runs(); len(got) != 1 {
		t.Fatalf("it ran again: %+v", got)
	}
	if !exists(before) {
		t.Fatal("touched what was there before")
	}
}

func TestAutoWaitsForAFileToStopGrowing(t *testing.T) {
	l := autoLab(t)
	// A real film, arriving a piece at a time, as a copy over a network does.
	whole := filepath.Join(t.TempDir(), "whole.mkv")
	cmd := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}, join(pic(6, "640x360"), tone(6, 300, "stereo"), []string{"-c:v", "mpeg4", "-q:v", "2", "-c:a", "aac", whole})...)
	if _, _, err := ffmpeg.Run(context.Background(), realTools.FFmpeg, cmd); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(whole)
	in, err := os.Open(whole)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	dst := filepath.Join(l.root, "Arriving.mkv")
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	// The first part arrives, a scan finds it, and then the rest keeps coming.
	piece := max(info.Size()/40, 1)
	if _, err := io.CopyN(out, in, piece*2); err != nil {
		t.Fatal(err)
	}
	out.Sync()
	l.scan()
	var startedEarly atomic.Bool
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			if len(l.runs()) > 0 {
				startedEarly.Store(true)
			}
			if _, err := io.CopyN(out, in, piece); err != nil {
				out.Close()
				return
			}
			time.Sleep(60 * time.Millisecond) // well inside the settling time
		}
	}()
	// While it's arriving, the activity panel says what it's waiting for.
	saw := false
	for !saw {
		select {
		case <-finished:
			t.Fatal("the file finished arriving before anything said it was waiting")
		default:
		}
		for _, task := range l.tasks.Live() {
			if strings.Contains(task.Title, "New files in") && strings.Contains(task.Phase, "stop changing") {
				saw = true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	<-finished
	if startedEarly.Load() {
		t.Fatal("it started on a file that was still growing")
	}
	l.settled()
	conv := filepath.Join(l.root, "Arriving.mp4")
	if !exists(conv) {
		t.Fatalf("not converted once it stopped: %+v", l.runs())
	}
	if d := l.probe(conv).Duration(); d < 5.5 {
		t.Fatalf("it converted part of the file: %.1f seconds of 6", d)
	}
}

// fakeFailing is an FFmpeg that fails, and counts how often it was asked to convert.
func fakeFailing(t *testing.T, count string) *ffmpeg.Tools {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a shell script")
	}
	fake := filepath.Join(t.TempDir(), "ffmpeg")
	script := "#!/bin/sh\ncase \"$*\" in *-progress*) echo x >> \"" + count + "\"; echo no >&2; exit 1;; esac\nexec " + realTools.FFmpeg + " \"$@\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	tools := *realTools
	tools.FFmpeg = fake
	return &tools
}

func lines(path string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}

func TestAutoNeverLoopsOnAFailure(t *testing.T) {
	l := autoLab(t)
	count := filepath.Join(t.TempDir(), "count")
	l.engine.Tools = fakeFailing(t, count)
	src := l.old("Keeps Failing.avi")
	want, _ := os.ReadFile(src)
	l.scan()
	l.settled()
	if got := lines(count); got != 1 {
		t.Fatalf("tried %d times", got)
	}
	got, _ := os.ReadFile(src)
	if string(got) != string(want) {
		t.Fatal("the old file changed")
	}
	// The activity panel has it as a problem.
	recent := l.tasks.Recent()
	if len(recent) == 0 || recent[0].ProblemCount != 1 {
		t.Fatalf("%+v", recent)
	}
	// However many scans come and go, and however long, it's not tried again.
	for range 3 {
		l.scan()
		l.settled()
	}
	time.Sleep(700 * time.Millisecond)
	l.settled()
	if got := lines(count); got != 1 {
		t.Fatalf("looped: tried %d times", got)
	}
	if got := l.runs(); len(got) != 1 {
		t.Fatalf("%d runs", len(got))
	}
	// Once the file changes, it's a new file, and gets one more go.
	time.Sleep(1100 * time.Millisecond)
	l.old("Keeps Failing.avi")
	l.scan()
	l.settled()
	if got := lines(count); got != 2 {
		t.Fatalf("a changed file: tried %d times", got)
	}
	// And someone asking still works, whatever's remembered.
	l.engine.Tools = realTools
	runs := l.runs()
	id, err := l.engine.Retry(context.Background(), runs[0].ID, tasks.By{})
	if err != nil {
		t.Fatal(err)
	}
	if r := l.wait(id); r.Summary.Done != 1 {
		t.Fatalf("%+v", r.Summary)
	}
	if !exists(filepath.Join(l.root, "Keeps Failing.mp4")) {
		t.Fatal("not converted when asked")
	}
}

func TestAutoSkipsWhatsAlreadyFine(t *testing.T) {
	l := autoLab(t)
	src := l.gen("Fine.mp4", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac"})...)
	want, _ := os.ReadFile(src)
	locked := l.gen("Locked.mp4", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac",
		"-encryption_scheme", "cenc-aes-ctr", "-encryption_key", "76a6c65c5ea762046bd749a2e632ccbb", "-encryption_kid", "a7e61c373e219033c21091fa607bf3b8"})...)
	lockedWant, _ := os.ReadFile(locked)
	l.scan()
	l.settled()
	got, _ := os.ReadFile(src)
	if string(got) != string(want) {
		t.Fatal("changed a file that fits")
	}
	got, _ = os.ReadFile(locked)
	if string(got) != string(lockedWant) {
		t.Fatal("touched a protected file")
	}
	if r := l.runs(); len(r) != 0 {
		t.Fatalf("nothing to do should leave no run behind: %+v", r)
	}
	for _, task := range l.tasks.Recent() {
		if task.Type == TaskType {
			t.Fatalf("nothing to do should leave nothing in the history: %+v", task)
		}
	}
	// And it isn't looked at all over again next time.
	ctx := context.Background()
	list, err := l.engine.autoCandidates(ctx, l.lib.ID, LoadRule(ctx, l.db, l.lib.ID))
	if err != nil || len(list) != 0 {
		t.Fatalf("%+v %v", list, err)
	}
}

func TestAutoKeepsToItsLimits(t *testing.T) {
	l := autoLab(t)
	ctx := context.Background()
	set := LoadSettings(ctx, l.db)
	set.PauseWhileStreaming = true
	set.HoursOn, set.HoursFrom, set.HoursTo = true, 22, 6
	if err := SaveSettings(ctx, l.db, set); err != nil {
		t.Fatal(err)
	}
	var busy, noon atomic32
	noon.Store(1)
	l.engine.Busy = func() int { return int(busy.Load()) }
	l.engine.Now = func() time.Time {
		day := time.Date(2026, 1, 1, 23, 0, 0, 0, time.Local)
		if noon.Load() == 1 {
			day = time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
		}
		return day.Add(time.Duration(time.Now().UnixNano() % int64(time.Hour/2)))
	}
	src := l.old("Patient.avi")
	l.scan()
	waitTask := func(words string) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for {
			for _, task := range l.tasks.Live() {
				if strings.Contains(task.Phase, words) {
					return
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("nothing says %q: %+v", words, l.tasks.Live())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitTask("hours")
	time.Sleep(500 * time.Millisecond)
	if len(l.runs()) != 0 || !exists(src) {
		t.Fatal("started outside its hours")
	}
	noon.Store(0)
	busy.Store(1)
	waitTask("streaming")
	time.Sleep(500 * time.Millisecond)
	if len(l.runs()) != 0 {
		t.Fatal("started while someone was streaming")
	}
	busy.Store(0)
	l.settled()
	if !exists(filepath.Join(l.root, "Patient.mp4")) {
		t.Fatalf("never converted: %+v", l.runs())
	}
}

func TestAutoTakesFilesOneAtATime(t *testing.T) {
	l := autoLab(t)
	var most atomic32
	// Sampling the running conversions while they run.
	l.old("One.avi")
	l.old("Two.avi")
	l.old("Three.avi")
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			n := int32(0)
			entries, _ := os.ReadDir(filepath.Join(l.root, library.WorkFolder))
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".mp4") {
					n++
				}
			}
			if n > most.Load() {
				most.Store(n)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	l.scan()
	l.settled()
	for _, n := range []string{"One", "Two", "Three"} {
		if !exists(filepath.Join(l.root, n+".mp4")) {
			t.Fatalf("%s wasn't converted: %+v", n, l.runs())
		}
	}
	if most.Load() > 1 {
		t.Fatalf("%d files at once, with a limit of 1", most.Load())
	}
}

func TestAutoRespectsACancel(t *testing.T) {
	l := autoLab(t)
	src := longJob(l)
	want, _ := os.ReadFile(src)
	l.scan()
	deadline := time.Now().Add(2 * time.Minute)
	var task *tasks.Task
	for task == nil {
		if time.Now().After(deadline) {
			t.Fatal("it never started")
		}
		for _, c := range l.tasks.Live() {
			if strings.HasPrefix(c.Title, "Converting new files") && c.Done > 0 {
				c := c
				task = &c
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !task.CanPause || !task.CanCancel {
		t.Fatalf("%+v", task)
	}
	if err := l.tasks.Cancel(task.ID); err != nil {
		t.Fatal(err)
	}
	l.settled()
	got, _ := os.ReadFile(src)
	if string(got) != string(want) {
		t.Fatal("the old file changed")
	}
	// Someone said no. A scan doesn't overrule them.
	l.scan()
	l.settled()
	if r := l.runs(); len(r) != 1 || r[0].State != "cancelled" {
		t.Fatalf("%+v", r)
	}
	l.noneLeftBehind()
}

func TestAutoRespectsAnUndo(t *testing.T) {
	l := autoLab(t)
	src := l.old("Put Back.avi")
	l.scan()
	l.settled()
	if exists(src) {
		t.Fatal("not converted")
	}
	if err := l.engine.Undo(context.Background(), l.files(l.runs()[0])[0].ID); err != nil {
		t.Fatal(err)
	}
	if !exists(src) {
		t.Fatal("not put back")
	}
	l.scan()
	l.settled()
	time.Sleep(600 * time.Millisecond)
	l.settled()
	if !exists(src) || exists(filepath.Join(l.root, "Put Back.mp4")) {
		t.Fatal("it converted what someone had just put back")
	}
}

func TestAutoComesBackForWhatWasBeingPlayed(t *testing.T) {
	l := autoLab(t)
	src := l.old("Being Watched.avi")
	var playing atomic.Bool
	playing.Store(true)
	l.engine.Playing = func(int64) bool { return playing.Load() }
	l.scan()
	l.settled()
	if !exists(src) {
		t.Fatal("converted while someone was playing it")
	}
	runs := l.runs()
	if len(runs) != 1 || runs[0].Summary.Postponed != 1 {
		t.Fatalf("%+v", runs)
	}
	// No spinning round on it meanwhile.
	time.Sleep(700 * time.Millisecond)
	l.settled()
	if got := l.runs(); len(got) != 1 {
		t.Fatalf("it went round again by itself: %d runs", len(got))
	}
	// The next scan looks again, once they've finished.
	playing.Store(false)
	l.scan()
	l.settled()
	if !exists(filepath.Join(l.root, "Being Watched.mp4")) {
		t.Fatalf("never converted: %+v", l.runs())
	}
}

func TestAutoNeedsPermission(t *testing.T) {
	check := func(t *testing.T, change func(l *lab)) {
		l := autoLab(t)
		change(l)
		src := l.old("Leave Me.avi")
		l.scan()
		time.Sleep(600 * time.Millisecond)
		l.settled()
		if !exists(src) || exists(strings.TrimSuffix(src, ".avi")+".mp4") || len(l.runs()) != 0 {
			t.Fatalf("converted anyway: %+v", l.runs())
		}
	}
	t.Run("read-only", func(t *testing.T) {
		check(t, func(l *lab) { l.store.SetLibraryReadOnly(context.Background(), l.lib.ID, true) })
	})
	t.Run("turned off", func(t *testing.T) {
		check(t, func(l *lab) { SaveSettings(context.Background(), l.db, DefaultSettings()) })
	})
	t.Run("save next to media off", func(t *testing.T) {
		check(t, func(l *lab) { l.engine.SaveMeta = func() bool { return false } })
	})
	t.Run("this library turned off again", func(t *testing.T) {
		check(t, func(l *lab) {
			if err := SaveRule(context.Background(), l.db, Rule{LibraryID: l.lib.ID, Policy: preset(l.t, PresetCompatible), Output: OutputKeep}); err != nil {
				l.t.Fatal(err)
			}
		})
	})
}

func TestAutoPicksUpAfterARestart(t *testing.T) {
	l := autoLab(t)
	// A scan found it, and the server stopped before anything happened.
	l.scanner.AfterScan = nil
	src := l.old("Left Waiting.avi")
	l.scan()
	if !exists(src) || len(l.runs()) != 0 {
		t.Fatal("the test expects nothing to have happened yet")
	}
	l.engine.KickAllAuto(context.Background())
	l.settled()
	if !exists(filepath.Join(l.root, "Left Waiting.mp4")) {
		t.Fatalf("not picked up: %+v", l.runs())
	}
}

func TestSettingAutoOnStartsCountingFromThen(t *testing.T) {
	l := newLab(t)
	ctx := context.Background()
	p := preset(t, PresetCompatible)
	if err := SaveRule(ctx, l.db, Rule{LibraryID: l.lib.ID, Policy: p, Output: OutputKeep}); err != nil {
		t.Fatal(err)
	}
	r := LoadRule(ctx, l.db, l.lib.ID)
	if r.Auto || r.AutoSince != 0 {
		t.Fatalf("%+v", r)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := SaveRule(ctx, l.db, Rule{LibraryID: l.lib.ID, Policy: p, Output: OutputKeep, Auto: true}); err != nil {
		t.Fatal(err)
	}
	first := LoadRule(ctx, l.db, l.lib.ID)
	if !first.Auto || first.AutoSince == 0 {
		t.Fatalf("%+v", first)
	}
	// Saving again with it still on keeps the date, or files would fall out of it.
	time.Sleep(1100 * time.Millisecond)
	if err := SaveRule(ctx, l.db, Rule{LibraryID: l.lib.ID, Policy: p, Output: OutputReplace, Auto: true}); err != nil {
		t.Fatal(err)
	}
	if got := LoadRule(ctx, l.db, l.lib.ID); got.AutoSince != first.AutoSince {
		t.Fatalf("%s moved to %s", strconv.FormatInt(first.AutoSince, 10), strconv.FormatInt(got.AutoSince, 10))
	}
	// A library's rule can be turned on with settle bounds that hold.
	s := DefaultSettings()
	if s.SettleSeconds != 120 {
		t.Fatal(s.SettleSeconds)
	}
	s.SettleSeconds = 1
	if s.Validate() == nil {
		t.Fatal("no real waiting")
	}
}
