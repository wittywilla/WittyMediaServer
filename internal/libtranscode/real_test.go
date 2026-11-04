package libtranscode

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// These tests convert real files with the real FFmpeg, checking what comes out with the
// real ffprobe. Every sample is made here, so no media needs to be around.

type lab struct {
	t       *testing.T
	root    string
	tools   *ffmpeg.Tools
	store   *library.Store
	scanner *library.Scanner
	engine  *Engine
	lib     *library.Library
	tasks   *tasks.Registry
	db      *db.DB
}

var realTools *ffmpeg.Tools

func newLab(t *testing.T) *lab {
	t.Helper()
	if realTools == nil {
		tools, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), os.Getenv("WMS_FFPROBE"))
		if err != nil {
			t.Skip("no FFmpeg here")
		}
		realTools = tools
	}
	if !realTools.Encoders["libx264"] {
		t.Skip("this FFmpeg has no libx264")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	if err := d.SetSetting(ctx, "saveMetadata", "0"); err != nil {
		t.Fatal(err)
	}
	store := &library.Store{DB: d}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	slog.SetDefault(log)
	scanner := library.NewScanner(store, realTools, filepath.Join(dir, "cache"), func() string { return "" }, log)
	if err := os.MkdirAll(filepath.Join(dir, "cache", "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "lib")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	lib, err := store.CreateLibrary(ctx, "Test videos", "videos", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	reg := tasks.New(nil)
	e := &Engine{DB: d, Store: store, Scanner: scanner, Tools: realTools, Tasks: reg, Log: log, HoldTick: 40 * time.Millisecond, SaveMeta: func() bool { return true }}
	set := DefaultSettings()
	// No free-space limit, unless a test is about that: these conversions need kilobytes, and how
	// full the temporary folder's drive is at the moment depends on whatever else is running.
	set.Enabled, set.Acknowledged, set.MinFreeGB = true, true, 0
	set.PauseWhileStreaming, set.Threads = false, 2
	if err := SaveSettings(ctx, d, set); err != nil {
		t.Fatal(err)
	}
	return &lab{t: t, root: root, tools: realTools, store: store, scanner: scanner, engine: e, lib: lib, tasks: reg, db: d}
}

func has(enc string) bool { return realTools != nil && realTools.Encoders[enc] }

// gen makes a sample file with FFmpeg.
func (l *lab) gen(name string, args ...string) string {
	l.t.Helper()
	p := filepath.Join(l.root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		l.t.Fatal(err)
	}
	all := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}, args...)
	all = append(all, p)
	cmd := exec.Command(realTools.FFmpeg, all...)
	if out, err := cmd.CombinedOutput(); err != nil {
		l.t.Fatalf("making %s: %v\n%s", name, err, out)
	}
	return p
}

// Sources FFmpeg can make from nothing.
func pic(sec int, size string) []string {
	return []string{"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%s:rate=25:duration=%d", size, sec)}
}

func tone(sec int, freq int, layout string) []string {
	return []string{"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d:sample_rate=48000:duration=%d,aformat=channel_layouts=%s", freq, sec, layout)}
}

func join(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// scan indexes the library and waits.
func (l *lab) scan() {
	l.t.Helper()
	if !l.scanner.Start(l.lib.ID) {
		l.t.Fatal("a scan is already going")
	}
	deadline := time.Now().Add(time.Minute)
	for l.scanner.Snapshot(l.lib.ID) != nil {
		if time.Now().After(deadline) {
			l.t.Fatal("the scan didn't finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *lab) item(path string) library.ConversionItem {
	l.t.Helper()
	items, err := l.store.ConversionItems(context.Background(), l.lib.ID)
	if err != nil {
		l.t.Fatal(err)
	}
	for _, it := range items {
		if it.Path == path {
			return it
		}
	}
	l.t.Fatalf("%s isn't in the library: %+v", path, items)
	return library.ConversionItem{}
}

// run does a whole run over the library and waits for it to end.
func (l *lab) run(p Policy, output string, items ...int64) *Run {
	l.t.Helper()
	if err := p.Validate(); err != nil {
		l.t.Fatal(err)
	}
	id, err := l.engine.Start(context.Background(), RunRequest{Library: l.lib.ID, Policy: p, Output: output, Items: items})
	if err != nil {
		l.t.Fatal(err)
	}
	return l.wait(id)
}

func (l *lab) wait(id int64) *Run {
	l.t.Helper()
	deadline := time.Now().Add(5 * time.Minute)
	for {
		r, err := l.engine.GetRun(context.Background(), id)
		if err != nil {
			l.t.Fatal(err)
		}
		if r.State != "queued" && r.State != "running" {
			return r
		}
		if time.Now().After(deadline) {
			l.t.Fatal("the run didn't finish")
		}
		time.Sleep(15 * time.Millisecond)
	}
}

// String lets a failed test say what happened to a file, rather than print a pointer.
func (f *FileRow) String() string {
	return fmt.Sprintf("{%s %s %s: %s %s}", filepath.Base(f.Path), f.Verdict, f.State, f.Reason, f.Message)
}

func (l *lab) files(run *Run) []*FileRow {
	l.t.Helper()
	rows, _, err := l.engine.ListFiles(context.Background(), run.ID, "", 200, 0)
	if err != nil {
		l.t.Fatal(err)
	}
	return rows
}

// fileNamed finds a run's row by file name.
func (l *lab) row(run *Run, name string) *FileRow {
	l.t.Helper()
	for _, f := range l.files(run) {
		if filepath.Base(f.Path) == name {
			return f
		}
	}
	l.t.Fatalf("no row for %s in %+v", name, l.files(run))
	return nil
}

func (l *lab) probe(path string) *ffmpeg.Probe {
	l.t.Helper()
	p, err := l.tools.ProbeFile(context.Background(), path)
	if err != nil {
		l.t.Fatalf("probing %s: %v", path, err)
	}
	return p
}

func (l *lab) noneLeftBehind() {
	l.t.Helper()
	filepath.WalkDir(l.root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && d.Name() == library.WorkFolder {
			entries, _ := os.ReadDir(p)
			l.t.Errorf("work folder left behind with %d files: %s", len(entries), p)
		}
		return nil
	})
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func preset(t *testing.T, id string) Policy {
	t.Helper()
	p, ok := PresetPolicy(id)
	if !ok {
		t.Fatal(id)
	}
	return p
}

func audioCodecs(p *ffmpeg.Probe) []string {
	var out []string
	for _, a := range p.Audio() {
		out = append(out, a.CodecName)
	}
	return out
}

func audioLangs(p *ffmpeg.Probe) []string {
	var out []string
	for _, a := range p.Audio() {
		out = append(out, a.Tags["language"])
	}
	return out
}

func subCodecs(p *ffmpeg.Probe) []string {
	var out []string
	for _, a := range p.Subtitles() {
		out = append(out, a.CodecName)
	}
	return out
}

func TestRealConvertForAnyDevice(t *testing.T) {
	l := newLab(t)
	meta := filepath.Join(t.TempDir(), "meta.txt")
	os.WriteFile(meta, []byte(";FFMETADATA1\ntitle=A Made Up Film\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1500\ntitle=One\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=1500\nEND=3000\ntitle=Two\n"), 0o644)
	src := l.gen("Made Up Film (1999).mkv", join(pic(3, "1920x1080"), tone(3, 440, "5.1"),
		[]string{"-f", "ffmetadata", "-i", meta, "-map", "0", "-map", "1", "-map_metadata", "2", "-map_chapters", "2", "-c:v", "mpeg2video", "-c:a", "ac3", "-metadata:s:a:0", "language=eng"})...)
	l.scan()
	it := l.item(src)
	ctx := context.Background()
	// Someone has watched some of it, and someone has edited its details.
	if _, err := l.db.ExecContext(ctx, `INSERT INTO users(id, name, password_hash, is_admin, created_at) VALUES(50, 'viewer', 'x', 0, 0)`); err != nil {
		t.Fatal(err)
	}
	if err := l.store.SetProgress(ctx, 50, it.ID, library.Progress{PositionSec: 1.5, DurationSec: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.db.ExecContext(ctx, `UPDATE items SET overview = 'Someone wrote this by hand.' WHERE id = ?`, it.ID); err != nil {
		t.Fatal(err)
	}
	r := l.run(preset(t, PresetCompatible), OutputKeep)
	if r.Summary.Done != 1 || r.State != "done" {
		t.Fatalf("run: %+v", r)
	}
	out := strings.TrimSuffix(src, ".mkv") + ".mp4"
	if !exists(out) || exists(src) {
		t.Fatalf("expected the new file in place: new %v, old %v", exists(out), exists(src))
	}
	kept := filepath.Join(l.root, library.OriginalsFolder, filepath.Base(src))
	if !exists(kept) {
		t.Fatal("the original wasn't kept")
	}
	p := l.probe(out)
	v := p.Video()
	if v == nil || v.CodecName != "h264" || v.Height != 1080 || v.PixFmt != "yuv420p" {
		t.Fatalf("picture: %+v", v)
	}
	if got := audioCodecs(p); !slices.Equal(got, []string{"aac", "ac3"}) {
		t.Fatalf("sound should be AAC stereo then the surround track: %v", got)
	}
	if a := p.Audio()[0]; a.Channels != 2 || a.Disposition["default"] != 1 || a.Tags["language"] != "eng" {
		t.Fatalf("first track: %+v", a)
	}
	if len(p.Chapters) != 2 {
		t.Fatalf("chapters: %d", len(p.Chapters))
	}
	if p.Format.Tags["title"] != "A Made Up Film" {
		t.Fatalf("title lost: %v", p.Format.Tags)
	}
	// The item is the same item, at the new path.
	after := l.item(out)
	if after.ID != it.ID {
		t.Fatalf("the item changed id: %d became %d", it.ID, after.ID)
	}
	full, err := l.store.Item(ctx, it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if full.VideoCodec != "h264" {
		t.Fatalf("the item's facts weren't updated: %+v", full)
	}
	if full.Overview != "Someone wrote this by hand." {
		t.Fatalf("its details were lost: %q", full.Overview)
	}
	list := []*library.Item{full}
	if err := l.store.AttachProgress(ctx, 50, list); err != nil {
		t.Fatal(err)
	}
	if full.Progress == nil || full.Progress.PositionSec != 1.5 {
		t.Fatalf("what they'd watched was lost: %+v", full.Progress)
	}
	// A scan finds nothing new, and doesn't make a second item.
	l.scan()
	items, _ := l.store.ConversionItems(ctx, l.lib.ID)
	if len(items) != 1 || items[0].ID != it.ID || items[0].Missing {
		t.Fatalf("after a scan: %+v", items)
	}
	if after, _ := l.store.Item(ctx, it.ID); after.Overview != "Someone wrote this by hand." {
		t.Fatalf("a scan after converting changed its details: %q", after.Overview)
	}
	// Doing it again changes nothing.
	r2 := l.run(preset(t, PresetCompatible), OutputKeep)
	if r2.Summary.Skip != 1 || r2.Summary.Convert != 0 {
		t.Fatalf("the second go should find nothing to do: %+v", r2.Summary)
	}
	// Undo.
	rows := l.files(r)
	if err := l.engine.Undo(ctx, rows[0].ID); err != nil {
		t.Fatal(err)
	}
	if !exists(src) || exists(out) || exists(kept) {
		t.Fatalf("undo: original %v, converted %v, kept %v", exists(src), exists(out), exists(kept))
	}
	if back := l.item(src); back.ID != it.ID {
		t.Fatal("undo changed the item")
	}
	l.noneLeftBehind()
}

func TestRealOutputs(t *testing.T) {
	t.Run("replace", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Replace Me.avi", join(pic(2, "320x240"), tone(2, 440, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		id := l.item(src).ID
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r)[0])
		}
		out := strings.TrimSuffix(src, ".avi") + ".mp4"
		if exists(src) || !exists(out) || exists(filepath.Join(l.root, library.OriginalsFolder)) {
			t.Fatalf("replace left: old %v new %v originals %v", exists(src), exists(out), exists(filepath.Join(l.root, library.OriginalsFolder)))
		}
		if l.item(out).ID != id {
			t.Fatal("new id")
		}
	})
	t.Run("same name", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Same Name.mp4", join(pic(2, "320x240"), tone(2, 440, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "aac"})...)
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v", r.Summary)
		}
		if got := l.probe(src).Video().CodecName; got != "h264" {
			t.Fatal(got)
		}
		if files, _ := os.ReadDir(l.root); len(files) != 1 {
			t.Fatalf("expected one file, got %v", files)
		}
	})
	t.Run("beside", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Beside Me.avi", join(pic(2, "320x240"), tone(2, 440, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		it := l.item(src)
		r := l.run(preset(t, PresetCompatible), OutputBeside)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v", r.Summary)
		}
		copyPath := filepath.Join(l.root, "Beside Me.wms-converted.mp4")
		if !exists(src) || !exists(copyPath) {
			t.Fatalf("old %v copy %v", exists(src), exists(copyPath))
		}
		if l.item(src).ID != it.ID {
			t.Fatal("the item moved")
		}
		// A scan doesn't list the copy as a film.
		l.scan()
		if items, _ := l.store.ConversionItems(context.Background(), l.lib.ID); len(items) != 1 {
			t.Fatalf("the copy became an item: %+v", items)
		}
		left, _, err := l.engine.Leftovers(context.Background(), l.lib.ID, 50)
		if err != nil || len(left) != 1 {
			t.Fatalf("leftovers: %v %v", left, err)
		}
		// Putting the copy into use moves the original aside and keeps the item.
		if err := l.engine.UseCopy(context.Background(), left[0].ID); err != nil {
			t.Fatal(err)
		}
		used := filepath.Join(l.root, "Beside Me.mp4")
		if !exists(used) || exists(src) || exists(copyPath) {
			t.Fatalf("use copy: used %v old %v copy %v", exists(used), exists(src), exists(copyPath))
		}
		if l.item(used).ID != it.ID {
			t.Fatal("new id")
		}
		kept := filepath.Join(l.root, library.OriginalsFolder, "Beside Me.avi")
		if !exists(kept) {
			t.Fatal("original not kept")
		}
		// And deleting what's left deletes only that.
		left, _, _ = l.engine.Leftovers(context.Background(), l.lib.ID, 50)
		if err := l.engine.DeleteLeftover(context.Background(), left[0].ID); err != nil {
			t.Fatal(err)
		}
		if exists(kept) || !exists(used) {
			t.Fatalf("delete: kept %v used %v", exists(kept), exists(used))
		}
	})
	t.Run("delete a copy", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Delete Copy.avi", join(pic(2, "320x240"), tone(2, 440, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		l.run(preset(t, PresetCompatible), OutputBeside)
		left, _, _ := l.engine.Leftovers(context.Background(), l.lib.ID, 50)
		if len(left) != 1 {
			t.Fatal(left)
		}
		if err := l.engine.DeleteLeftover(context.Background(), left[0].ID); err != nil {
			t.Fatal(err)
		}
		if !exists(src) || exists(filepath.Join(l.root, "Delete Copy.wms-converted.mp4")) {
			t.Fatal("deleted the wrong thing")
		}
	})
	t.Run("a name already taken is not overwritten", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Taken.avi", join(pic(2, "320x240"), tone(2, 440, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		other := filepath.Join(l.root, "Taken.mp4")
		os.WriteFile(other, []byte("not this one"), 0o644)
		l.scan()
		l.run(preset(t, PresetCompatible), OutputKeep, l.item(src).ID)
		b, _ := os.ReadFile(other)
		if string(b) != "not this one" || !exists(src) {
			t.Fatalf("overwrote a file: %q, old %v", b, exists(src))
		}
		l.noneLeftBehind()
	})
}

func TestRealAudioLanguages(t *testing.T) {
	l := newLab(t)
	src := l.gen("Languages.mkv", join(pic(2, "320x240"), tone(2, 300, "5.1"), tone(2, 400, "stereo"), tone(2, 500, "stereo"), tone(2, 600, "stereo"), tone(2, 700, "stereo"), tone(2, 800, "stereo"), []string{
		"-map", "0", "-map", "1", "-map", "2", "-map", "3", "-map", "4", "-map", "5", "-map", "6", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
		"-metadata:s:a:0", "language=fre",
		"-metadata:s:a:1", "language=jpn", "-disposition:a:1", "default",
		"-metadata:s:a:2", "language=eng",
		"-metadata:s:a:3", "language=eng", "-metadata:s:a:3", "title=Director's Commentary",
		"-metadata:s:a:4", "language=eng", "-metadata:s:a:4", "title=Audio Description", "-disposition:a:4", "visual_impaired",
		"-metadata:s:a:5", "title=mystery track",
	})...)
	l.scan()
	it := l.item(src)
	p := preset(t, PresetBest)
	p.Preset = PresetCustom
	p.Audio.Keep = KeepLanguages
	p.Audio.Languages = []string{"en"}
	p.Audio.KeepOriginal = true // the file's default track says Japanese
	p.Audio.Default = "en"
	p.Audio.Commentary = Drop
	p.Audio.Description = Keep
	p.Audio.Untagged = Drop
	p.Audio.AddStereo = false
	r := l.run(p, OutputReplace, it.ID)
	if r.Summary.Done != 1 {
		t.Fatalf("%+v %+v", r.Summary, l.files(r))
	}
	out := l.probe(src)
	if got := audioLangs(out); !slices.Equal(got, []string{"eng", "jpn", "eng"}) {
		t.Fatalf("order: %v (want English, Japanese, then the English description)", got)
	}
	if out.Audio()[0].Disposition["default"] != 1 || out.Audio()[1].Disposition["default"] != 0 {
		t.Fatalf("default: %+v", out.Audio()[0].Disposition)
	}
	if out.Audio()[2].Disposition["visual_impaired"] != 1 {
		t.Fatalf("description flag: %+v", out.Audio()[2].Disposition)
	}
	if !strings.Contains(out.Audio()[2].Tags["title"], "Description") {
		t.Fatal("title lost")
	}
}

func TestRealNeverSilent(t *testing.T) {
	l := newLab(t)
	src := l.gen("Only French.mkv", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-metadata:s:a:0", "language=fre"})...)
	l.scan()
	p := preset(t, PresetBest)
	p.Preset = PresetCustom
	p.Audio.Keep = KeepLanguages
	p.Audio.Languages = []string{"en"}
	p.Audio.KeepOriginal = false
	p.Audio.AddStereo = false
	pv, err := l.engine.PreviewItem(context.Background(), l.lib, l.item(src).ID, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Parts[0].Plan.Warnings) == 0 {
		t.Fatal("no warning that a track is kept anyway")
	}
	r := l.run(p, OutputKeep)
	// Nothing changes, so nothing is written, and the sound is still there.
	if r.Summary.Skip != 1 || len(l.probe(src).Audio()) != 1 {
		t.Fatalf("%+v", r.Summary)
	}
}

func TestRealSoundFormats(t *testing.T) {
	l := newLab(t)
	if !has("flac") {
		t.Skip("no flac encoder")
	}
	l.gen("Flac 7.1.mkv", join(pic(2, "320x240"), tone(2, 300, "7.1"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "flac"})...)
	l.gen("Side layout.mkv", join(pic(2, "320x240"), tone(2, 300, "5.1(side)"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "flac"})...)
	l.gen("Mono pcm.mov", join(pic(2, "320x240"), tone(2, 300, "mono"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "pcm_s16le"})...)
	l.gen("Dts.mkv", join(pic(2, "320x240"), tone(2, 300, "5.1"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "dca", "-strict", "-2"})...)
	l.scan()
	r := l.run(preset(t, PresetCompatible), OutputReplace)
	if r.Summary.Failed != 0 || r.Summary.Done != 4 {
		for _, f := range l.files(r) {
			t.Logf("%s: %s %s", filepath.Base(f.Path), f.State, f.Message)
		}
		t.Fatalf("%+v", r.Summary)
	}
	want := map[string][]string{
		"Flac 7.1.mp4":    {"aac", "aac"},
		"Side layout.mp4": {"aac", "aac"},
		"Mono pcm.mp4":    {"aac"},
		"Dts.mp4":         {"aac", "aac"},
	}
	for name, codecs := range want {
		p := l.probe(filepath.Join(l.root, name))
		if got := audioCodecs(p); !slices.Equal(got, codecs) {
			t.Errorf("%s: %v, want %v", name, got, codecs)
		}
		if name != "Mono pcm.mp4" {
			if a := p.Audio(); a[0].Channels != 2 || a[1].Channels != 6 {
				t.Errorf("%s: channels %d, %d: want stereo then 5.1", name, a[0].Channels, a[1].Channels)
			}
		}
	}
	// Save space turns lossless into Dolby Digital Plus, leaving others be.
	t.Run("save space", func(t *testing.T) {
		l := newLab(t)
		l.gen("Lossless.mkv", join(pic(2, "320x240"), tone(2, 300, "5.1"), tone(2, 400, "stereo"), []string{"-map", "0", "-map", "1", "-map", "2", "-c:v", "libx264", "-preset", "ultrafast", "-c:a:0", "flac", "-c:a:1", "aac"})...)
		l.scan()
		p := preset(t, PresetSaveSpace)
		p.Video.SkipBelowKbps, p.Video.MinSavingPercent = 0, 0
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		got := audioCodecs(l.probe(filepath.Join(l.root, "Lossless.mkv")))
		if !slices.Equal(got, []string{"eac3", "aac"}) {
			t.Fatal(got)
		}
	})
	t.Run("opus", func(t *testing.T) {
		if !has("libopus") {
			t.Skip("no libopus")
		}
		l := newLab(t)
		l.gen("To Opus.mkv", join(pic(2, "320x240"), tone(2, 300, "5.1(side)"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "ac3"})...)
		l.scan()
		p := preset(t, PresetBest)
		p.Preset = PresetCustom
		p.Audio.Codec = "opus"
		p.Audio.Accept = []string{"aac"}
		p.Audio.AddStereo = false
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		if got := audioCodecs(l.probe(filepath.Join(l.root, "To Opus.mkv"))); !slices.Equal(got, []string{"opus"}) {
			t.Fatal(got)
		}
	})
}

// writeSUP writes a tiny but real PGS subtitle stream: one small box, shown and cleared.
func writeSUP(t *testing.T, path string) {
	t.Helper()
	var b []byte
	seg := func(pts uint32, kind byte, body []byte) {
		h := make([]byte, 13)
		copy(h, "PG")
		binary.BigEndian.PutUint32(h[2:], pts)
		h[10] = kind
		binary.BigEndian.PutUint16(h[11:], uint16(len(body)))
		b = append(b, h...)
		b = append(b, body...)
	}
	display := func(pts uint32, show bool) {
		pcs := []byte{0x01, 0x40, 0x00, 0xF0, 0x10, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00}
		if show {
			pcs = append(pcs[:10], 1, 0, 0, 0, 0, 0, 0x10, 0, 0x10, 0)
			pcs[10] = 1
		} else {
			pcs = append(pcs[:10], 0)
			pcs[7] = 0
		}
		seg(pts, 0x16, pcs)
		if show {
			seg(pts, 0x17, []byte{1, 0, 0, 0x10, 0, 0x10, 0, 0x10, 0, 0x10})
			seg(pts, 0x14, []byte{0, 0, 1, 235, 128, 128, 255})
			rle := []byte{}
			for range 16 {
				rle = append(rle, 0x01)
				for range 15 {
					rle = append(rle, 0x01)
				}
				rle = append(rle, 0x00, 0x00)
			}
			ods := []byte{0, 0, 0, 0xC0}
			n := len(rle) + 4
			ods = append(ods, byte(n>>16), byte(n>>8), byte(n), 0, 0x10, 0, 0x10)
			ods = append(ods, rle...)
			seg(pts, 0x15, ods)
		} else {
			seg(pts, 0x17, []byte{1, 0, 0, 0x10, 0, 0x10, 0, 0x10, 0, 0x10})
		}
		seg(pts, 0x80, nil)
	}
	display(90000/2, true)
	display(90000, false)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRealSubtitles(t *testing.T) {
	newLab(t) // finds FFmpeg
	work := t.TempDir()
	srt := filepath.Join(work, "a.srt")
	os.WriteFile(srt, []byte("1\n00:00:00,100 --> 00:00:01,500\nHello there\n"), 0o644)
	ass := filepath.Join(work, "a.ass")
	os.WriteFile(ass, []byte("[Script Info]\nScriptType: v4.00+\n[V4+ Styles]\nFormat: Name, Fontsize\nStyle: Default,20\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.10,0:00:01.50,Default,,0,0,0,,Bonjour\n"), 0o644)
	sup := filepath.Join(work, "a.sup")
	writeSUP(t, sup)

	base := func(l *lab, name string, extra ...string) string {
		args := join(pic(2, "640x360"), tone(2, 300, "stereo"), []string{"-i", srt, "-i", ass, "-i", sup,
			"-map", "0", "-map", "1", "-map", "2", "-map", "2", "-map", "3", "-map", "4", "-map", "4", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
			"-c:s:0", "srt", "-c:s:1", "srt", "-c:s:2", "ass", "-c:s:3", "copy", "-c:s:4", "copy",
			"-metadata:s:s:0", "language=eng",
			"-metadata:s:s:1", "language=eng", "-disposition:s:1", "hearing_impaired",
			"-metadata:s:s:2", "language=fre",
			"-metadata:s:s:3", "language=eng", "-disposition:s:3", "forced",
			"-metadata:s:s:4", "language=ger",
			"-metadata:s:a:0", "language=eng"}, extra)
		return l.gen(name, args...)
	}
	t.Run("sample has what it should", func(t *testing.T) {
		l := newLab(t)
		src := base(l, "Check.mkv")
		if got := subCodecs(l.probe(src)); !slices.Equal(got, []string{"subrip", "subrip", "ass", "hdmv_pgs_subtitle", "hdmv_pgs_subtitle"}) {
			t.Fatal(got)
		}
	})
	t.Run("any device burns the forced one and exports the rest", func(t *testing.T) {
		l := newLab(t)
		src := base(l, "Device.mkv")
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		out := filepath.Join(l.root, "Device.mp4")
		p := l.probe(out)
		if got := subCodecs(p); !slices.Equal(got, []string{"mov_text", "mov_text", "mov_text"}) {
			t.Fatalf("%v", got)
		}
		if v := p.Video(); v == nil || v.CodecName != "h264" {
			t.Fatalf("%+v", v)
		}
		if !exists(filepath.Join(l.root, "Device.de.sup")) {
			entries, _ := os.ReadDir(l.root)
			t.Fatalf("no exported subtitle file: %v", entries)
		}
		if got := library.FindSubtitles(out); len(got) != 1 || got[0].Language != "de" || got[0].Codec != "hdmv_pgs_subtitle" {
			t.Fatalf("the server doesn't read the exported file back: %+v", got)
		}
		plan := l.row(r, "Device.mkv").Plan
		burned, styled := false, false
		for _, st := range plan.Steps {
			if st.Action == ActBurn {
				burned = true
			}
			if strings.Contains(st.Why, "Styling") {
				styled = true
			}
		}
		if !burned || !styled {
			t.Fatalf("the plan should say what happened: %+v", plan.Steps)
		}
		_ = src
	})
	t.Run("saved next to media off means no file beside it", func(t *testing.T) {
		l := newLab(t)
		base(l, "No Sidecar.mkv")
		l.scan()
		l.engine.SaveMeta = func() bool { return false }
		if err := SaveRule(context.Background(), l.db, Rule{LibraryID: l.lib.ID, Policy: preset(t, PresetCompatible), Output: OutputKeep, AllowWrites: true}); err != nil {
			t.Fatal(err)
		}
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		if exists(filepath.Join(l.root, "No Sidecar.de.sup")) {
			t.Fatal("wrote beside the video with that off")
		}
		found := false
		for _, w := range l.row(r, "No Sidecar.mkv").Plan.Warnings {
			if strings.Contains(w, "will be lost") {
				found = true
			}
		}
		if !found {
			t.Fatal("no warning about the lost subtitle")
		}
	})
	t.Run("keeping picture subtitles keeps the MKV", func(t *testing.T) {
		l := newLab(t)
		base(l, "Stays Mkv.mkv")
		l.scan()
		p := preset(t, PresetCompatible)
		p.Preset = PresetCustom
		p.Subtitles.Images = Keep
		p.Subtitles.Forced = Keep
		r := l.run(p, OutputReplace)
		if r.Summary.Skip != 1 {
			t.Fatalf("it fits as it is, in its MKV: %+v %+v", r.Summary, l.files(r))
		}
		got := subCodecs(l.probe(filepath.Join(l.root, "Stays Mkv.mkv")))
		if !slices.Equal(got, []string{"subrip", "subrip", "ass", "hdmv_pgs_subtitle", "hdmv_pgs_subtitle"}) {
			t.Fatal(got)
		}
	})
	t.Run("DVD subtitles", func(t *testing.T) {
		l := newLab(t)
		// FFmpeg turns one kind of picture subtitle into the other.
		src := l.gen("Vobsub.mkv", join(pic(2, "640x360"), tone(2, 300, "stereo"), []string{"-i", sup, "-map", "0", "-map", "1", "-map", "2", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-c:s", "dvdsub", "-metadata:s:s:0", "language=eng"})...)
		if got := subCodecs(l.probe(src)); !slices.Equal(got, []string{"dvd_subtitle"}) {
			t.Fatal(got)
		}
		l.scan()
		pv, err := l.engine.PreviewItem(context.Background(), l.lib, l.item(src).ID, preset(t, PresetCompatible))
		if err != nil {
			t.Fatal(err)
		}
		lost := false
		for _, w := range pv.Parts[0].Plan.Warnings {
			if strings.Contains(w, "will be lost") && strings.Contains(w, "DVD") {
				lost = true
			}
		}
		if !lost {
			t.Fatalf("no word about the DVD subtitle: %+v", pv.Parts[0].Plan)
		}
		p := preset(t, PresetSaveSpace)
		p.Video.Codec, p.Video.Accept, p.Video.SkipBelowKbps, p.Video.MinSavingPercent = "h264", nil, 0, 0
		p.Video.MaxHeight = 240
		if r := l.run(p, OutputReplace); r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		if got := subCodecs(l.probe(src)); !slices.Equal(got, []string{"dvd_subtitle"}) {
			t.Fatalf("an MKV keeps them: %v", got)
		}
	})
	t.Run("languages SDH and images", func(t *testing.T) {
		l := newLab(t)
		base(l, "Choose.mkv")
		l.scan()
		p := preset(t, PresetBest)
		p.Preset = PresetCustom
		p.Audio.AddStereo = false
		p.Subtitles.Keep = KeepLanguages
		p.Subtitles.Languages = []string{"fr", "en"}
		p.Subtitles.SDH = Drop
		p.Subtitles.Images = Drop
		p.Subtitles.Forced = Drop
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		o := l.probe(filepath.Join(l.root, "Choose.mkv"))
		if got := subCodecs(o); !slices.Equal(got, []string{"ass", "subrip"}) {
			t.Fatalf("order and choice: %v", got)
		}
	})
	t.Run("to plain text", func(t *testing.T) {
		l := newLab(t)
		base(l, "Plain.mkv")
		l.scan()
		p := preset(t, PresetBest)
		p.Preset = PresetCustom
		p.Audio.AddStereo = false
		p.Subtitles.Text = "srt"
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		o := l.probe(filepath.Join(l.root, "Plain.mkv"))
		if got := subCodecs(o); !slices.Equal(got, []string{"subrip", "subrip", "subrip", "hdmv_pgs_subtitle", "hdmv_pgs_subtitle"}) {
			t.Fatal(got)
		}
	})
	t.Run("burn text and odd names", func(t *testing.T) {
		l := newLab(t)
		// A name that would break a filter graph or a shell, if either saw it unescaped.
		name := "It's 100% [odd], a; b=c.mkv"
		l.gen(name, join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-i", srt, "-map", "0", "-map", "1", "-map", "2",
			"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-c:s", "srt", "-disposition:s:0", "forced", "-metadata:s:s:0", "language=eng", "-metadata:s:a:0", "language=eng"})...)
		l.scan()
		p := preset(t, PresetBest)
		p.Preset = PresetCustom
		p.Video.Codec = "h264"
		p.Video.Quality = 23
		p.Audio.AddStereo = false
		p.Subtitles.Forced = "burn"
		r := l.run(p, OutputReplace)
		if !l.tools.HasFilter("subtitles") {
			t.Skip("this FFmpeg can't burn text")
		}
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		o := l.probe(filepath.Join(l.root, name))
		if len(o.Subtitles()) != 0 {
			t.Fatalf("the burned track is still a track: %v", subCodecs(o))
		}
		if v := o.Video(); v.CodecName != "h264" {
			t.Fatal(v.CodecName)
		}
	})
	t.Run("subtitle files beside the video", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Sidecar.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		os.WriteFile(filepath.Join(l.root, "Sidecar.en.srt"), []byte("1\n00:00:00,100 --> 00:00:01,500\nHello\n"), 0o644)
		os.WriteFile(filepath.Join(l.root, "Sidecar.fr.forced.srt"), []byte("1\n00:00:00,100 --> 00:00:01,500\nBonjour\n"), 0o644)
		l.scan()
		// Left where they are, they still belong: only the extension changes.
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		if !exists(filepath.Join(l.root, "Sidecar.en.srt")) {
			t.Fatal("moved a subtitle file it should have left")
		}
		if got := library.FindSubtitles(filepath.Join(l.root, "Sidecar.mp4")); len(got) != 2 {
			t.Fatalf("the new file lost its subtitle files: %+v", got)
		}
		_ = src
	})
	t.Run("embedding files", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Embed.mkv", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		side := filepath.Join(l.root, "Embed.en.srt")
		os.WriteFile(side, []byte("1\n00:00:00,100 --> 00:00:01,500\nHello\n"), 0o644)
		l.scan()
		p := preset(t, PresetBest)
		p.Preset = PresetCustom
		p.Audio.AddStereo = false
		p.Subtitles.Sidecars = "embed"
		r := l.run(p, OutputKeep)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		o := l.probe(src)
		if got := subCodecs(o); !slices.Equal(got, []string{"subrip"}) || o.Subtitles()[0].Tags["language"] != "en" && o.Subtitles()[0].Tags["language"] != "eng" {
			t.Fatalf("%v %+v", got, o.Subtitles())
		}
		if exists(side) {
			t.Fatal("the file is still beside it, so it'd show up twice")
		}
		if !exists(filepath.Join(l.root, library.OriginalsFolder, "Embed.en.srt")) {
			t.Fatal("the file went with the original, or should have")
		}
		// Undoing it puts the file back too.
		if err := l.engine.Undo(context.Background(), l.row(r, "Embed.mkv").ID); err != nil {
			t.Fatal(err)
		}
		if !exists(side) || len(l.probe(src).Subtitles()) != 0 {
			t.Fatalf("undo left the subtitle out: %v %v", exists(side), subCodecs(l.probe(src)))
		}
	})
}

func TestRealMultipleVideo(t *testing.T) {
	make := func(l *lab, name string) string {
		return l.gen(name, join(pic(2, "320x240"), pic(2, "1280x720"), tone(2, 300, "stereo"), []string{"-map", "0", "-map", "1", "-map", "2", "-c:v", "mpeg4", "-c:a", "aac"})...)
	}
	t.Run("keep all", func(t *testing.T) {
		l := newLab(t)
		make(l, "Two Pictures.mkv")
		l.scan()
		p := preset(t, PresetSaveSpace)
		p.Video.Codec = "h264"
		p.Video.SkipBelowKbps, p.Video.MinSavingPercent = 0, 0
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		o := l.probe(filepath.Join(l.root, "Two Pictures.mkv"))
		vs := o.VideoStreams()
		if len(vs) != 2 || vs[0].CodecName != "h264" || vs[1].CodecName != "h264" {
			t.Fatalf("%+v", vs)
		}
		// The largest picture is the main one and goes first.
		if vs[0].Height != 720 {
			t.Fatalf("the main picture should come first: %d", vs[0].Height)
		}
	})
	t.Run("main only", func(t *testing.T) {
		l := newLab(t)
		make(l, "One Picture.mkv")
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		vs := l.probe(filepath.Join(l.root, "One Picture.mp4")).VideoStreams()
		if len(vs) != 1 || vs[0].Height != 720 {
			t.Fatalf("%+v", vs)
		}
	})
}

func TestRealCoverArt(t *testing.T) {
	newLab(t) // finds FFmpeg
	work := t.TempDir()
	cover := filepath.Join(work, "cover.jpg")
	if out, err := exec.Command(realTools.FFmpeg, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=red:s=64x64", "-frames:v", "1", cover).CombinedOutput(); err != nil {
		t.Skipf("can't make a picture: %v %s", err, out)
	}
	attached := []string{"-c:v", "mpeg4", "-c:a", "aac", "-attach", cover, "-metadata:s:t:0", "mimetype=image/jpeg", "-metadata:s:t:0", "filename=cover.jpg"}
	for _, ext := range []string{"mkv", "mp4"} {
		t.Run(ext, func(t *testing.T) {
			l := newLab(t)
			extra := []string{"-i", cover, "-map", "0", "-map", "1", "-map", "2", "-c:v:0", "mpeg4", "-c:a", "aac", "-c:v:1", "copy", "-disposition:v:1", "attached_pic"}
			if ext == "mkv" {
				extra = attached
			}
			src := l.gen("With Cover."+ext, join(pic(2, "320x240"), tone(2, 300, "stereo"), extra)...)
			before := Inspect(src, l.probe(src))
			if len(before.Covers) != 1 || len(before.Video) != 1 {
				t.Fatalf("the sample should have one cover: %+v", before)
			}
			l.scan()
			r := l.run(preset(t, PresetCompatible), OutputReplace)
			if r.Summary.Done != 1 {
				t.Fatalf("%+v %+v", r.Summary, l.files(r))
			}
			after := Inspect(filepath.Join(l.root, "With Cover.mp4"), l.probe(filepath.Join(l.root, "With Cover.mp4")))
			if len(after.Covers) != 1 || len(after.Video) != 1 || after.Video[0].Codec != "h264" {
				t.Fatalf("after: %+v", after)
			}
		})
	}
	t.Run("MKV to MKV", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Cover Stays.mkv", join(pic(2, "320x240"), tone(2, 300, "stereo"), attached)...)
		l.scan()
		p := preset(t, PresetSaveSpace)
		p.Video.Codec = "h264"
		p.Video.SkipBelowKbps, p.Video.MinSavingPercent = 0, 0
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		after := Inspect(src, l.probe(src))
		if len(after.Covers) != 1 {
			t.Fatalf("cover lost: %+v", after)
		}
	})
}

func TestRealHDR(t *testing.T) {
	l := newLab(t)
	if !has("libx265") {
		t.Skip("no libx265 here")
	}
	// The tags go on the frames too: FFmpeg 9.0.2 writes an MKV's colours as unknown when they're
	// only output options. The server's own conversions do the same (hdrColorParams).
	hdr10 := []string{"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params",
		"log-level=error:hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", "-vf", "setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc", "-c:a", "aac"}
	hlg := []string{"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params", "log-level=error:colorprim=bt2020:transfer=arib-std-b67:colormatrix=bt2020nc",
		"-color_primaries", "bt2020", "-color_trc", "arib-std-b67", "-colorspace", "bt2020nc", "-vf", "setparams=color_primaries=bt2020:color_trc=arib-std-b67:colorspace=bt2020nc", "-c:a", "aac"}
	src := l.gen("Hdr10.mkv", join(pic(2, "640x360"), tone(2, 300, "stereo"), hdr10)...)
	l.gen("Hlg.mkv", join(pic(2, "640x360"), tone(2, 300, "stereo"), hlg)...)
	v := Inspect(src, l.probe(src)).Video[0]
	if v.Range != "hdr10" || v.Depth != 10 {
		t.Fatalf("not read as HDR10: %+v", v)
	}
	l.scan()
	t.Run("save space leaves efficient HDR alone", func(t *testing.T) {
		r := l.run(preset(t, PresetSaveSpace), OutputReplace)
		if r.Summary.Skip != 2 {
			t.Fatalf("%+v", r.Summary)
		}
	})
	t.Run("kept as HDR when it must be converted", func(t *testing.T) {
		p := preset(t, PresetSaveSpace)
		p.Preset = PresetCustom
		p.Video.MaxHeight = 240
		p.Video.SkipBelowKbps, p.Video.MinSavingPercent = 0, 0
		r := l.run(p, OutputBeside)
		if r.Summary.Done != 2 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		o := l.probe(filepath.Join(l.root, "Hdr10.wms-converted.mkv")).Video()
		if o.Height != 240 || o.CodecName != "hevc" || o.ColorTransfer != "smpte2084" || o.Depth() != 10 {
			t.Fatalf("%+v", o)
		}
		o2 := l.probe(filepath.Join(l.root, "Hlg.wms-converted.mkv")).Video()
		if o2.ColorTransfer != "arib-std-b67" || o2.Depth() != 10 {
			t.Fatalf("%+v", o2)
		}
		// x265 carries the mastering display inside the stream, which the frames show.
		out, err := exec.Command(l.tools.FFprobe, "-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#1", "-show_frames", "-of", "json", filepath.Join(l.root, "Hdr10.wms-converted.mkv")).Output()
		if err != nil || !strings.Contains(string(out), "Mastering display") {
			t.Fatalf("mastering display lost: %s %v", out, err)
		}
		left, _, _ := l.engine.Leftovers(context.Background(), l.lib.ID, 10)
		for _, f := range left {
			if err := l.engine.DeleteLeftover(context.Background(), f.ID); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("kept as HDR in AV1", func(t *testing.T) {
		if !has("libsvtav1") {
			t.Skip("no AV1 encoder")
		}
		p := preset(t, PresetSaveSpace)
		p.Preset = PresetCustom
		p.Video.Codec, p.Video.Accept = "av1", nil
		p.Video.SkipBelowKbps, p.Video.MinSavingPercent = 0, 0
		r := l.run(p, OutputBeside)
		if r.Summary.Done != 2 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		o := l.probe(filepath.Join(l.root, "Hdr10.wms-converted.mkv")).Video()
		if o.CodecName != "av1" || o.ColorTransfer != "smpte2084" || o.Depth() != 10 {
			t.Fatalf("%+v", o)
		}
		o = l.probe(filepath.Join(l.root, "Hlg.wms-converted.mkv")).Video()
		if o.ColorTransfer != "arib-std-b67" {
			t.Fatalf("%+v", o)
		}
		left, _, _ := l.engine.Leftovers(context.Background(), l.lib.ID, 10)
		for _, f := range left {
			l.engine.DeleteLeftover(context.Background(), f.ID)
		}
	})
	t.Run("made an ordinary picture", func(t *testing.T) {
		if l.tools.ToneMapFilter("hdr10") == "" {
			t.Skip("this FFmpeg has no tone mapping filters")
		}
		r := l.run(preset(t, PresetCompatible), OutputBeside)
		if r.Summary.Done != 2 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		for _, n := range []string{"Hdr10", "Hlg"} {
			o := l.probe(filepath.Join(l.root, n+".wms-converted.mp4")).Video()
			if o.CodecName != "h264" || o.PixFmt != "yuv420p" || o.ColorTransfer != "bt709" {
				t.Fatalf("%s: %+v", n, o)
			}
		}
		left, _, _ := l.engine.Leftovers(context.Background(), l.lib.ID, 10)
		for _, f := range left {
			l.engine.DeleteLeftover(context.Background(), f.ID)
		}
	})
	t.Run("refused when asked", func(t *testing.T) {
		p := preset(t, PresetCompatible)
		p.Preset = PresetCustom
		p.Video.HDR = "refuse"
		r := l.run(p, OutputKeep)
		if r.Summary.Refuse != 2 {
			t.Fatalf("%+v", r.Summary)
		}
		if !strings.Contains(l.row(r, "Hdr10.mkv").Reason, "HDR") {
			t.Fatal(l.row(r, "Hdr10.mkv").Reason)
		}
	})
	t.Run("H.264 can't carry HDR", func(t *testing.T) {
		p := preset(t, PresetSaveSpace)
		p.Preset = PresetCustom
		p.Video.Codec = "h264"
		p.Video.Accept = nil
		r := l.run(p, OutputKeep)
		if r.Summary.Refuse != 2 {
			t.Fatalf("%+v", r.Summary)
		}
		if !strings.Contains(l.row(r, "Hdr10.mkv").Reason, "H.264 can't carry HDR") {
			t.Fatal(l.row(r, "Hdr10.mkv").Reason)
		}
	})
}

// addDolbyVision inserts a Dolby Vision configuration box into an MP4's HEVC track. It
// describes the stream without the frames holding any of the data a real one has, so it
// tests what the server reads about Dolby Vision, and not that it could play any.
func addDolbyVision(t *testing.T, path string, profile, compat int) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dv := make([]byte, 32)
	binary.BigEndian.PutUint32(dv, 32)
	copy(dv[4:], "dvcC")
	dv[8], dv[9] = 1, 0
	bits := uint32(profile)<<25 | uint32(6)<<19 | 1<<18 | 1<<16 | uint32(compat)<<12 // level 6, rpu present, bl present
	binary.BigEndian.PutUint32(dv[10:], bits)
	// Find the sample entry and the boxes around it, and make room in each.
	entry := strings.Index(string(b), "hvc1") - 4
	hvcC := strings.Index(string(b), "hvcC") - 4
	if entry < 0 || hvcC < 0 {
		t.Fatal("no HEVC track in that MP4")
	}
	at := hvcC + int(binary.BigEndian.Uint32(b[hvcC:]))
	grow := func(kind string) {
		i := strings.Index(string(b), kind)
		if i < 4 || i-4 > at {
			t.Fatalf("no %s box", kind)
		}
		binary.BigEndian.PutUint32(b[i-4:], binary.BigEndian.Uint32(b[i-4:])+uint32(len(dv)))
	}
	for _, k := range []string{"moov", "stsd", "hvc1"} {
		grow(k)
	}
	// trak, mdia, minf and stbl: each holds the next, so walk down from moov.
	moov := strings.Index(string(b), "moov") - 4
	pos := moov + 8
	for _, k := range []string{"trak", "mdia", "minf", "stbl"} {
		for {
			n := int(binary.BigEndian.Uint32(b[pos:]))
			kind := string(b[pos+4 : pos+8])
			if kind == k && pos < at && at <= pos+n {
				binary.BigEndian.PutUint32(b[pos:], uint32(n+len(dv)))
				pos += 8
				break
			}
			pos += n
			if n < 8 || pos >= len(b) {
				t.Fatalf("no %s around the track", k)
			}
		}
	}
	// A movie header after the data leaves the data's offsets alone.
	if strings.Index(string(b), "mdat") > entry {
		t.Fatal("expected the movie header after the data")
	}
	out := append(append(append([]byte{}, b[:at]...), dv...), b[at:]...)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRealDolbyVision(t *testing.T) {
	l := newLab(t)
	if !has("libx265") {
		t.Skip("no libx265 here")
	}
	args := join(pic(2, "640x360"), tone(2, 300, "stereo"), []string{"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params", "log-level=error", "-tag:v", "hvc1",
		"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc", "-c:a", "aac", "-movflags", "+write_colr"})
	dv5 := l.gen("Dv Five.mp4", args...)
	dv8 := l.gen("Dv Eight.mp4", args...)
	addDolbyVision(t, dv5, 5, 0)
	addDolbyVision(t, dv8, 8, 1)
	if v := Inspect(dv8, l.probe(dv8)).Video[0]; !v.DV || v.DVProfile != 8 || v.Range != "hdr10" {
		t.Fatalf("not read as Dolby Vision 8.1: %+v", v)
	}
	if v := Inspect(dv5, l.probe(dv5)).Video[0]; !v.DV || v.DVProfile != 5 || v.Range != "dv5" {
		t.Fatalf("not read as Dolby Vision 5: %+v", v)
	}
	l.scan()
	t.Run("kept by leaving the picture alone", func(t *testing.T) {
		r := l.run(preset(t, PresetBest), OutputBeside)
		if r.Summary.Done != 2 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		for _, n := range []string{"Dv Five", "Dv Eight"} {
			v := Inspect("x.mkv", l.probe(filepath.Join(l.root, n+".wms-converted.mkv"))).Video[0]
			if !v.DV {
				t.Fatalf("%s lost Dolby Vision: %+v", n, v)
			}
		}
		left, _, _ := l.engine.Leftovers(context.Background(), l.lib.ID, 10)
		for _, f := range left {
			l.engine.DeleteLeftover(context.Background(), f.ID)
		}
	})
	t.Run("profile 5 refused, 8 converted from its base layer", func(t *testing.T) {
		if l.tools.ToneMapFilter("hdr10") == "" {
			t.Skip("no tone mapping")
		}
		r := l.run(preset(t, PresetCompatible), OutputBeside)
		five := l.row(r, "Dv Five.mp4")
		if five.Verdict != VerdictRefuse || !strings.Contains(five.Reason, "profile 5") {
			t.Fatalf("%+v", five)
		}
		eight := l.row(r, "Dv Eight.mp4")
		if eight.State != FileDone {
			var detail string
			l.db.QueryRowContext(context.Background(), `SELECT detail FROM ltc_files WHERE id = ?`, eight.ID).Scan(&detail)
			t.Fatalf("%+v\n%s", eight, detail)
		}
		warned := false
		for _, w := range eight.Plan.Warnings {
			if strings.Contains(w, "Dolby Vision won't survive") {
				warned = true
			}
		}
		if !warned {
			t.Fatal("no warning about losing Dolby Vision")
		}
		left, _, _ := l.engine.Leftovers(context.Background(), l.lib.ID, 10)
		for _, f := range left {
			l.engine.DeleteLeftover(context.Background(), f.ID)
		}
	})
	t.Run("keep refuses what would need converting", func(t *testing.T) {
		p := preset(t, PresetSaveSpace)
		p.Preset = PresetCustom
		p.Video.MaxHeight = 240
		r := l.run(p, OutputKeep)
		if r.Summary.Refuse != 2 {
			t.Fatalf("%+v", r.Summary)
		}
	})
}

func TestRealPictureOddities(t *testing.T) {
	l := newLab(t)
	l.gen("Ten Bit.mkv", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-c:a", "aac"})...)
	l.gen("Interlaced.mkv", join(pic(2, "720x576"), tone(2, 300, "stereo"), []string{"-vf", "tinterlace=interleave_top,fieldorder=tff", "-flags", "+ildct+ilme", "-c:v", "mpeg2video", "-c:a", "aac"})...)
	l.gen("Variable.mkv", join(pic(4, "320x240"), tone(4, 300, "stereo"), []string{"-vf", `select=lt(t\,2)+not(mod(n\,5))`, "-fps_mode", "vfr", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
	l.gen("Film.3D.SBS.mkv", join(pic(2, "640x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", "-metadata:s:v:0", "stereo_mode=left_right"})...)
	l.scan()
	r := l.run(preset(t, PresetCompatible), OutputReplace)
	for _, f := range l.files(r) {
		t.Logf("%s: %s %s %s %s", filepath.Base(f.Path), f.Verdict, f.State, f.Reason, f.Message)
	}
	if r.Summary.Failed != 0 {
		t.Fatalf("%+v", r.Summary)
	}
	if v := l.probe(filepath.Join(l.root, "Ten Bit.mp4")).Video(); v.PixFmt != "yuv420p" || v.CodecName != "h264" {
		t.Fatalf("10-bit: %+v", v)
	}
	if v := l.probe(filepath.Join(l.root, "Interlaced.mp4")).Video(); v.IsInterlaced() || v.CodecName != "h264" {
		t.Fatalf("interlaced: %+v", v)
	}
	if v := l.probe(filepath.Join(l.root, "Film.3D.SBS.mp4")).Video(); v.Width != 640 {
		t.Fatalf("the 3D picture changed: %+v", v)
	}
	t.Run("3D flattened", func(t *testing.T) {
		l := newLab(t)
		l.gen("Flat.3D.SBS.mkv", join(pic(2, "640x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		l.scan()
		p := preset(t, PresetCompatible)
		p.Preset = PresetCustom
		p.Video.ThreeD = "flatten"
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		// Half of 640 wide, 240 tall is 320x240 with its shape back, here at twice the width.
		if v := l.probe(filepath.Join(l.root, "Flat.3D.SBS.mp4")).Video(); v.Width != 640 || v.Height != 240 {
			t.Fatalf("%+v", v)
		}
	})
	t.Run("3D refused", func(t *testing.T) {
		l := newLab(t)
		l.gen("Keep.3D.SBS.mkv", join(pic(2, "640x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		l.scan()
		p := preset(t, PresetCompatible)
		p.Preset = PresetCustom
		p.Video.ThreeD = "refuse"
		r := l.run(p, OutputReplace)
		if r.Summary.Refuse != 1 {
			t.Fatalf("%+v", r.Summary)
		}
	})
	t.Run("variable frame rate keeps sound in step", func(t *testing.T) {
		l := newLab(t)
		// A run of frames, then far fewer of them.
		src := l.gen("Vfr.mkv", join(pic(6, "320x240"), tone(6, 300, "stereo"), []string{"-filter:v", `select=lt(t\,3)+not(mod(n\,5))`, "-fps_mode", "vfr", "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		s := Inspect(src, l.probe(src))
		if !s.Video[0].Variable {
			t.Logf("the sample isn't read as variable: %+v", s.Video[0])
		}
		l.scan()
		p := preset(t, PresetSaveSpace)
		p.Preset = PresetCustom
		p.Video.Codec = "h264"
		p.Video.Accept = nil
		p.Video.MaxHeight = 120
		p.Video.SkipBelowKbps, p.Video.MinSavingPercent = 0, 0
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		o := l.probe(src)
		if abs(o.Duration()-s.Duration) > 0.5 {
			t.Fatalf("length moved: %.2f to %.2f", s.Duration, o.Duration())
		}
	})
}

func TestRealBrokenFiles(t *testing.T) {
	t.Run("timestamps that jump back", func(t *testing.T) {
		l := newLab(t)
		a := l.gen("x/a.ts", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		b := l.gen("x/b.ts", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		ab, _ := os.ReadFile(a)
		bb, _ := os.ReadFile(b)
		joined := filepath.Join(l.root, "Jumpy.ts")
		os.WriteFile(joined, append(ab, bb...), 0o644)
		os.RemoveAll(filepath.Join(l.root, "x"))
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		row := l.row(r, "Jumpy.ts")
		t.Logf("%s %s %s", row.Verdict, row.State, row.Message)
		switch row.State {
		case FileDone:
			if d := l.probe(filepath.Join(l.root, "Jumpy.mp4")).Duration(); d < 1 {
				t.Fatalf("length %f", d)
			}
		case FileFailed:
			if !exists(joined) {
				t.Fatal("it failed and took the file with it")
			}
		default:
			t.Fatalf("%+v", row)
		}
		l.noneLeftBehind()
	})
	t.Run("a bare stream with no times at all", func(t *testing.T) {
		l := newLab(t)
		l.gen("Bare.h264", join(pic(2, "320x240"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-bsf:v", "h264_mp4toannexb", "-f", "h264"})...)
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		if len(l.files(r)) == 0 {
			t.Skip("a scan doesn't list bare streams here")
		}
		row := l.files(r)[0]
		t.Logf("%s %s %s", row.Verdict, row.State, row.Message)
		if row.State == FileFailed && !exists(filepath.Join(l.root, "Bare.h264")) {
			t.Fatal("it failed and took the file with it")
		}
	})
	t.Run("damage in the middle", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Damaged.mkv", join(pic(4, "640x480"), tone(4, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		b, _ := os.ReadFile(src)
		for i := len(b) / 3; i < len(b)*2/3; i++ {
			b[i] = 0
		}
		os.WriteFile(src, b, 0o644)
		want, _ := os.ReadFile(src)
		l.scan()
		r := l.run(preset(t, PresetSaveSpace), OutputKeep)
		row := l.row(r, "Damaged.mkv")
		t.Logf("%s %s %s", row.Verdict, row.State, row.Message)
		// Either way the file is either safely kept or the original bytes are where they were.
		if row.State != FileDone {
			got, _ := os.ReadFile(src)
			if string(got) != string(want) {
				t.Fatal("the old file changed")
			}
		}
		l.noneLeftBehind()
	})
	t.Run("not a video at all", func(t *testing.T) {
		l := newLab(t)
		p := filepath.Join(l.root, "Garbage.mkv")
		os.WriteFile(p, []byte(strings.Repeat("this is not a video ", 2000)), 0o644)
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		rows := l.files(r)
		if len(rows) == 1 && rows[0].Verdict != VerdictRefuse {
			t.Fatalf("%+v", rows[0])
		}
		if !exists(p) {
			t.Fatal("deleted it")
		}
	})
	if runtime.GOOS == "windows" {
		return
	}
	t.Run("FFmpeg dies part-way", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Dies.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		want, _ := os.ReadFile(src)
		l.scan()
		fake := filepath.Join(t.TempDir(), "ffmpeg")
		// Writes something to its output, as a real one would, then fails.
		os.WriteFile(fake, []byte("#!/bin/sh\nfor last; do :; done\necho half a file > \"$last\"\necho boom >&2\nexit 1\n"), 0o755)
		tools := *realTools
		tools.FFmpeg = fake
		l.engine.Tools = &tools
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		row := l.row(r, "Dies.avi")
		if row.State != FileFailed || r.Summary.Failed != 1 {
			t.Fatalf("%+v %+v", row, r.Summary)
		}
		got, _ := os.ReadFile(src)
		if string(got) != string(want) {
			t.Fatal("the old file changed")
		}
		l.noneLeftBehind()
		// The activity panel has it in its list of problems, and it can be tried again.
		recent := l.tasks.Recent()
		if len(recent) == 0 || recent[0].ProblemCount != 1 || !strings.Contains(recent[0].Problems[0].Message, "old one stays") {
			t.Fatalf("%+v", recent)
		}
		l.engine.Tools = realTools
		id, err := l.engine.Retry(context.Background(), r.ID, tasks.By{})
		if err != nil {
			t.Fatal(err)
		}
		if again := l.wait(id); again.Summary.Done != 1 {
			t.Fatalf("%+v", again.Summary)
		}
		if _, err := l.engine.Retry(context.Background(), id, tasks.By{}); err == nil {
			t.Fatal("retried a run with nothing left to do")
		}
	})
	t.Run("output comes out short", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Short.avi", join(pic(8, "320x240"), tone(8, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		want, _ := os.ReadFile(src)
		l.scan()
		fake := filepath.Join(t.TempDir(), "ffmpeg")
		// A real FFmpeg, told to stop early, as a crash that still closed the file would.
		os.WriteFile(fake, []byte("#!/bin/sh\nfor last; do :; done\nexec "+realTools.FFmpeg+" -hide_banner -y -loglevel error -i \""+src+"\" -t 2 -c:v libx264 -preset ultrafast -c:a aac -f mp4 \"$last\"\n"), 0o755)
		tools := *realTools
		tools.FFmpeg = fake
		l.engine.Tools = &tools
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		row := l.row(r, "Short.avi")
		if row.State != FileFailed || !strings.Contains(row.Message, "long") {
			t.Fatalf("%+v", row)
		}
		got, _ := os.ReadFile(src)
		if string(got) != string(want) {
			t.Fatal("the old file changed")
		}
		l.noneLeftBehind()
	})
	t.Run("the file changes while converting", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Changes.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		fake := filepath.Join(t.TempDir(), "ffmpeg")
		os.WriteFile(fake, []byte("#!/bin/sh\necho more >> \""+src+"\"\nexec "+realTools.FFmpeg+" \"$@\"\n"), 0o755)
		tools := *realTools
		tools.FFmpeg = fake
		l.engine.Tools = &tools
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		row := l.row(r, "Changes.avi")
		if row.State != FileChanged || r.Summary.Changed != 1 {
			t.Fatalf("%+v %+v", row, r.Summary)
		}
		if !exists(src) || exists(filepath.Join(l.root, "Changes.mp4")) {
			t.Fatal("it replaced a file that had changed")
		}
		l.noneLeftBehind()
	})
}

func TestRealSplitFilm(t *testing.T) {
	l := newLab(t)
	a := l.gen("Split Film (2001) CD1.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
	b := l.gen("Split Film (2001) CD2.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
	l.scan()
	items, _ := l.store.ConversionItems(context.Background(), l.lib.ID)
	if len(items) != 1 {
		t.Fatalf("a split film is one item: %+v", items)
	}
	id := items[0].ID
	r := l.run(preset(t, PresetCompatible), OutputKeep)
	if r.Summary.Done != 2 || r.Summary.Convert != 2 {
		t.Fatalf("%+v %+v", r.Summary, l.files(r))
	}
	na := strings.TrimSuffix(a, ".avi") + ".mp4"
	nb := strings.TrimSuffix(b, ".avi") + ".mp4"
	if !exists(na) || !exists(nb) || exists(a) || exists(b) {
		t.Fatalf("both parts should be new: %v %v %v %v", exists(na), exists(nb), exists(a), exists(b))
	}
	if got := library.StackedParts(na); len(got) != 2 {
		t.Fatalf("the parts no longer play as one: %v", got)
	}
	if it := l.item(na); it.ID != id {
		t.Fatal("new id")
	}
	full, _ := l.store.Item(context.Background(), id)
	if full.RuntimeSec < 3.5 {
		t.Fatalf("its length should be both parts: %f", full.RuntimeSec)
	}
	// A failure on the second part leaves the first as it was.
	t.Run("one part fails", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs a shell script")
		}
		l := newLab(t)
		a := l.gen("Other (2001) CD1.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		b := l.gen("Other (2001) CD2.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		fake := filepath.Join(t.TempDir(), "ffmpeg")
		os.WriteFile(fake, []byte("#!/bin/sh\ncase \"$*\" in *CD2*) echo no >&2; exit 1;; esac\nexec "+realTools.FFmpeg+" \"$@\"\n"), 0o755)
		tools := *realTools
		tools.FFmpeg = fake
		l.engine.Tools = &tools
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Failed != 1 || r.Summary.Done != 0 {
			t.Fatalf("%+v", r.Summary)
		}
		if !exists(a) || !exists(b) || exists(strings.TrimSuffix(a, ".avi")+".mp4") {
			t.Fatal("a part was replaced though the other failed")
		}
		l.noneLeftBehind()
	})
	// One part is fine and the other isn't: both are converted, so they'll still join.
	t.Run("parts that differ are made to match", func(t *testing.T) {
		l := newLab(t)
		l.gen("Mixed (2001) CD1.mp4", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac"})...)
		l.gen("Mixed (2001) CD2.mp4", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "aac"})...)
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Done != 2 || r.Summary.Convert != 2 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
	})
}

func TestRealRefusedAndSkipped(t *testing.T) {
	t.Run("sound only", func(t *testing.T) {
		l := newLab(t)
		l.gen("Only Sound.mkv", join(tone(2, 300, "stereo"), []string{"-c:a", "aac"})...)
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		row := l.files(r)[0]
		if row.Verdict != VerdictSkip || !strings.Contains(row.Reason, "no picture") {
			t.Fatalf("%+v", row)
		}
	})
	t.Run("disc folders and images", func(t *testing.T) {
		l := newLab(t)
		ctx := context.Background()
		iso := filepath.Join(l.root, "Disc.iso")
		os.WriteFile(iso, make([]byte, 4096), 0o644)
		for i, kind := range []string{"dvd-iso", "dvd-folder", "bluray-folder"} {
			if _, err := l.db.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, added_at, updated_at) VALUES(?, ?, ?, ?, ?, 0, 0)`,
				l.lib.ID, kind, filepath.Join(l.root, fmt.Sprintf("d%d", i)), "Disc", "Disc"); err != nil {
				t.Fatal(err)
			}
		}
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Refuse != 3 {
			t.Fatalf("%+v", r.Summary)
		}
		for _, f := range l.files(r) {
			if !strings.Contains(f.Reason, "Disc images and folders aren't converted") {
				t.Fatalf("%+v", f)
			}
		}
	})
	t.Run("a link leading out of the library", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("needs a link")
		}
		l := newLab(t)
		outside := filepath.Join(t.TempDir(), "secret.mkv")
		cmd := exec.Command(realTools.FFmpeg, join([]string{"-hide_banner", "-loglevel", "error", "-y"}, pic(2, "320x240"), []string{"-c:v", "mpeg4", outside})...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		link := filepath.Join(l.root, "Link.mkv")
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		if _, err := l.db.ExecContext(context.Background(), `INSERT INTO items(library_id, kind, path, title, sort_title, added_at, updated_at) VALUES(?, 'video', ?, 'Link', 'Link', 0, 0)`, l.lib.ID, link); err != nil {
			t.Fatal(err)
		}
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		row := l.files(r)[0]
		if row.Verdict != VerdictRefuse || !strings.Contains(row.Reason, "isn't inside the library") {
			t.Fatalf("%+v", row)
		}
		if !exists(outside) {
			t.Fatal("touched the file outside")
		}
	})
}

func TestRealCopyProtection(t *testing.T) {
	t.Run("an encrypted MP4 is refused", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Locked.mp4", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac",
			"-encryption_scheme", "cenc-aes-ctr", "-encryption_key", "76a6c65c5ea762046bd749a2e632ccbb", "-encryption_kid", "a7e61c373e219033c21091fa607bf3b8"})...)
		want, _ := os.ReadFile(src)
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		row := l.files(r)[0]
		if row.Verdict != VerdictRefuse || row.Reason != ProtectedReason {
			t.Fatalf("%+v", row)
		}
		got, _ := os.ReadFile(src)
		if string(got) != string(want) {
			t.Fatal("touched it")
		}
		pv, err := l.engine.PreviewItem(context.Background(), l.lib, row.ItemID, preset(t, PresetCompatible))
		if err != nil || pv.Parts[0].Plan.Verdict != VerdictRefuse {
			t.Fatalf("%+v %v", pv, err)
		}
	})
	t.Run("a scrambled transport stream is refused", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Scrambled.ts", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		b, _ := os.ReadFile(src)
		for i := 0; i+188 <= len(b); i += 188 {
			if b[i] == 0x47 && b[i+1]&0x1F != 0 { // leave the tables, as a real recording does
				b[i+3] |= 0x80
			}
		}
		os.WriteFile(src, b, 0o644)
		if !Detect(src, nil) {
			t.Fatal("not detected")
		}
	})
	t.Run("what's only called the same is left alone", func(t *testing.T) {
		l := newLab(t)
		// Words in a title, and bytes that look like marks, aren't protection.
		src := l.gen("Sinfonia.mp4", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", "-metadata", "title=sinf pssh encv tenc schm senc", "-metadata", "comment=drms enca"})...)
		if Detect(src, l.probe(src)) {
			t.Fatal("an ordinary MP4 was called protected")
		}
		mk := l.gen("Plain.mkv", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		if Detect(mk, l.probe(mk)) {
			t.Fatal("an ordinary MKV was called protected")
		}
		ts := l.gen("Plain.ts", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
		if Detect(ts, l.probe(ts)) {
			t.Fatal("an ordinary transport stream was called protected")
		}
	})
}

func TestRealSafetyRules(t *testing.T) {
	t.Run("off by default", func(t *testing.T) {
		l := newLab(t)
		l.gen("Off.avi", join(pic(2, "320x240"), []string{"-c:v", "mpeg4"})...)
		l.scan()
		SaveSettings(context.Background(), l.db, DefaultSettings())
		_, err := l.engine.Start(context.Background(), RunRequest{Library: l.lib.ID, Policy: preset(t, PresetCompatible), Output: OutputKeep})
		if err == nil || !strings.Contains(err.Error(), "off") {
			t.Fatalf("started while off: %v", err)
		}
		// A dry run only reads, and works.
		id, err := l.engine.Start(context.Background(), RunRequest{Library: l.lib.ID, Policy: preset(t, PresetCompatible), Output: OutputKeep, Dry: true})
		if err != nil {
			t.Fatal(err)
		}
		r := l.wait(id)
		if r.Summary.Convert != 1 || r.State != "done" {
			t.Fatalf("%+v", r)
		}
		if !exists(filepath.Join(l.root, "Off.avi")) || exists(filepath.Join(l.root, "Off.mp4")) {
			t.Fatal("a dry run changed something")
		}
		rows := l.files(r)
		if rows[0].State != FilePlanned || rows[0].Plan == nil || rows[0].EstBytes == 0 {
			t.Fatalf("%+v", rows[0])
		}
	})
	t.Run("read-only library", func(t *testing.T) {
		l := newLab(t)
		l.gen("Ro.avi", join(pic(2, "320x240"), []string{"-c:v", "mpeg4"})...)
		l.scan()
		ctx := context.Background()
		if err := l.store.SetLibraryReadOnly(ctx, l.lib.ID, true); err != nil {
			t.Fatal(err)
		}
		_, err := l.engine.Start(ctx, RunRequest{Library: l.lib.ID, Policy: preset(t, PresetCompatible), Output: OutputKeep})
		if err == nil || !strings.Contains(err.Error(), "read-only") {
			t.Fatalf("%v", err)
		}
		id, err := l.engine.Start(ctx, RunRequest{Library: l.lib.ID, Policy: preset(t, PresetCompatible), Output: OutputKeep, Dry: true})
		if err != nil {
			t.Fatal(err)
		}
		l.wait(id)
	})
	t.Run("save next to media off", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Nm.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		ctx := context.Background()
		l.engine.SaveMeta = func() bool { return false }
		_, err := l.engine.Start(ctx, RunRequest{Library: l.lib.ID, Policy: preset(t, PresetCompatible), Output: OutputKeep})
		if err == nil || !strings.Contains(err.Error(), "Save next to media") {
			t.Fatalf("%v", err)
		}
		// Asked for plainly, it goes ahead.
		if err := SaveRule(ctx, l.db, Rule{LibraryID: l.lib.ID, Policy: preset(t, PresetCompatible), Output: OutputKeep, AllowWrites: true}); err != nil {
			t.Fatal(err)
		}
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v", r.Summary)
		}
		_ = src
	})
	t.Run("not enough room", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Room.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		ctx := context.Background()
		set := LoadSettings(ctx, l.db)
		set.MinFreeGB = 100000
		SaveSettings(ctx, l.db, set)
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		row := l.row(r, "Room.avi")
		if row.State != FileFailed || !strings.Contains(row.Message, "Not enough free space") {
			t.Fatalf("%+v", row)
		}
		if !exists(src) {
			t.Fatal("lost the file")
		}
		if len(r.Summary.Drives) == 0 || r.Summary.Drives[0].Enough {
			t.Fatalf("the check before starting should have said so: %+v", r.Summary.Drives)
		}
	})
	t.Run("being played", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Playing.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		other := l.gen("Not Playing.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		id := l.item(src).ID
		l.engine.Playing = func(item int64) bool { return item == id }
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Done != 1 || r.Summary.Postponed != 1 {
			t.Fatalf("%+v", r.Summary)
		}
		if !exists(src) || exists(strings.TrimSuffix(src, ".avi")+".mp4") {
			t.Fatal("converted a file someone was playing")
		}
		_ = other
		if row := l.row(r, "Playing.avi"); row.State != FilePostponed {
			t.Fatalf("%+v", row)
		}
	})
	t.Run("a watcher arriving at the last moment", func(t *testing.T) {
		l := newLab(t)
		src := l.gen("Late.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
		l.scan()
		calls := 0
		l.engine.Playing = func(int64) bool {
			calls++
			return calls >= 2 && calls < 8 // not at the start; for a while once it's ready
		}
		r := l.run(preset(t, PresetCompatible), OutputKeep)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v", r.Summary)
		}
		if calls < 8 {
			t.Fatalf("didn't wait for the viewer: %d", calls)
		}
		_ = src
	})
}

// A longer job, for what can only be seen while it's running.
func longJob(l *lab) string {
	return l.gen("Long Job.avi", join(pic(60, "1280x720"), tone(60, 300, "stereo"), []string{"-c:v", "mpeg4", "-q:v", "6", "-c:a", "mp3"})...)
}

func (l *lab) taskOf(kind string) *tasks.Task {
	list := l.tasks.Live()
	for i := range list {
		if list[i].Type == kind {
			return &list[i]
		}
	}
	return nil
}

func TestRealRunningControls(t *testing.T) {
	waitFor := func(t *testing.T, what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(60 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatalf("waited too long for: %s", what)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	slow := func(t *testing.T) Policy {
		p := preset(t, PresetCompatible)
		p.Preset = PresetCustom
		p.Video.Quality = 18
		p.Video.Speed = "slower"
		return p
	}
	t.Run("cancel", func(t *testing.T) {
		l := newLab(t)
		src := longJob(l)
		want, _ := os.ReadFile(src)
		l.scan()
		id, err := l.engine.Start(context.Background(), RunRequest{Library: l.lib.ID, Policy: slow(t), Output: OutputKeep})
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, "work to start", func() bool {
			tk := l.taskOf(TaskType)
			return tk != nil && tk.Done > 0
		})
		tk := l.taskOf(TaskType)
		if strings.Contains(tk.Title, l.root) || strings.Contains(tk.Title, "Long Job") {
			t.Fatalf("a path or name in a title: %q", tk.Title)
		}
		if tk.LibraryID != l.lib.ID || !tk.CanCancel || !tk.CanPause {
			t.Fatalf("%+v", tk)
		}
		if err := l.tasks.Cancel(tk.ID); err != nil {
			t.Fatal(err)
		}
		r := l.wait(id)
		if r.State != "cancelled" {
			t.Fatalf("%+v", r)
		}
		got, _ := os.ReadFile(src)
		if string(got) != string(want) {
			t.Fatal("the old file changed")
		}
		l.noneLeftBehind()
	})
	t.Run("pause and resume hold FFmpeg still", func(t *testing.T) {
		l := newLab(t)
		longJob(l)
		l.scan()
		id, err := l.engine.Start(context.Background(), RunRequest{Library: l.lib.ID, Policy: slow(t), Output: OutputKeep})
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, "work to start", func() bool {
			tk := l.taskOf(TaskType)
			return tk != nil && tk.Done > 0
		})
		tk := l.taskOf(TaskType)
		if err := l.tasks.Pause(tk.ID); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "it to pause", func() bool { return l.taskOf(TaskType).State == tasks.Paused })
		time.Sleep(300 * time.Millisecond) // anything in FFmpeg's pipe arrives
		at := l.taskOf(TaskType).Done
		time.Sleep(700 * time.Millisecond)
		if now := l.taskOf(TaskType).Done; now != at {
			t.Fatalf("it kept going while paused: %d to %d", at, now)
		}
		if err := l.tasks.Resume(tk.ID); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "it to move again", func() bool {
			c := l.taskOf(TaskType)
			return c == nil || c.Done > at
		})
		l.tasks.Cancel(tk.ID)
		l.wait(id)
	})
	t.Run("holds while people stream and outside its hours", func(t *testing.T) {
		l := newLab(t)
		longJob(l)
		l.scan()
		ctx := context.Background()
		set := LoadSettings(ctx, l.db)
		set.PauseWhileStreaming = true
		set.HoursOn, set.HoursFrom, set.HoursTo = true, 22, 6
		SaveSettings(ctx, l.db, set)
		var busy, noon atomic32
		noon.Store(1)
		l.engine.Busy = func() int { return int(busy.Load()) }
		l.engine.Now = func() time.Time {
			if noon.Load() == 1 {
				return time.Date(2026, 1, 1, 12, 0, 0, 0, time.Local)
			}
			return time.Date(2026, 1, 1, 23, 0, 0, 0, time.Local)
		}
		id, err := l.engine.Start(ctx, RunRequest{Library: l.lib.ID, Policy: slow(t), Output: OutputKeep})
		if err != nil {
			t.Fatal(err)
		}
		waitFor(t, "it to say it's waiting for its hours", func() bool {
			tk := l.taskOf(TaskType)
			return tk != nil && strings.Contains(tk.Phase, "hours")
		})
		if tk := l.taskOf(TaskType); tk.Done != 0 {
			t.Fatalf("worked outside its hours: %+v", tk)
		}
		noon.Store(0)
		waitFor(t, "work to start", func() bool { return l.taskOf(TaskType).Done > 0 })
		busy.Store(1)
		waitFor(t, "it to say someone is streaming", func() bool { return strings.Contains(l.taskOf(TaskType).Phase, "streaming") })
		time.Sleep(300 * time.Millisecond)
		at := l.taskOf(TaskType).Done
		time.Sleep(500 * time.Millisecond)
		if now := l.taskOf(TaskType).Done; now != at {
			t.Fatalf("kept going while someone streamed: %d to %d", at, now)
		}
		busy.Store(0)
		waitFor(t, "it to move again", func() bool { return l.taskOf(TaskType).Done > at })
		l.tasks.Cancel(l.taskOf(TaskType).ID)
		l.wait(id)
	})
}

func TestRealRecover(t *testing.T) {
	l := newLab(t)
	src := l.gen("Crash.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
	other := l.gen("Crash2.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
	l.scan()
	ctx := context.Background()
	runID, err := l.engine.createRun(ctx, l.lib.ID, preset(t, PresetCompatible), OutputKeep, false, false, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	// The server stopped with the original moved aside and the new file not yet in place.
	work := filepath.Join(l.root, library.WorkFolder)
	originals := filepath.Join(l.root, library.OriginalsFolder)
	os.MkdirAll(work, 0o755)
	os.MkdirAll(originals, 0o755)
	temp := filepath.Join(work, "t1.mp4")
	os.WriteFile(temp, []byte("half done"), 0o644)
	kept := filepath.Join(originals, "Crash.avi")
	if err := os.Rename(src, kept); err != nil {
		t.Fatal(err)
	}
	insert := func(path, temp, final, kept, swap string) {
		_, err := l.db.ExecContext(ctx, `INSERT INTO ltc_files(run_id, item_id, path, verdict, state, temp_path, final_path, kept_path, swap) VALUES(?, ?, ?, 'convert', 'running', ?, ?, ?, ?)`,
			runID, l.item(pathOr(path, other)).ID, path, temp, final, kept, swap)
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = insert
	_, err = l.db.ExecContext(ctx, `INSERT INTO ltc_files(run_id, item_id, path, verdict, state, temp_path, final_path, kept_path, swap) VALUES(?, 1, ?, 'convert', 'running', ?, ?, ?, 'keep')`,
		runID, src, temp, strings.TrimSuffix(src, ".avi")+".mp4", kept)
	if err != nil {
		t.Fatal(err)
	}
	// And a job that only got as far as a temporary file.
	temp2 := filepath.Join(work, "t2.mp4")
	os.WriteFile(temp2, []byte("half done"), 0o644)
	if _, err = l.db.ExecContext(ctx, `INSERT INTO ltc_files(run_id, item_id, path, verdict, state, temp_path) VALUES(?, 2, ?, 'convert', 'running', ?)`, runID, other, temp2); err != nil {
		t.Fatal(err)
	}
	l.engine.setRunState(ctx, runID, "running", nil)
	l.engine.Recover(ctx)
	if !exists(src) || exists(kept) || exists(temp) || exists(temp2) || exists(work) {
		t.Fatalf("after recovery: original %v, kept %v, temp %v, temp2 %v, work %v", exists(src), exists(kept), exists(temp), exists(temp2), exists(work))
	}
	r, err := l.engine.GetRun(ctx, runID)
	if err != nil || r.State != "interrupted" {
		t.Fatalf("%+v %v", r, err)
	}
	// The other way: everything had moved, only the library hadn't heard.
	l2 := newLab(t)
	src2 := l2.gen("Crash3.avi", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
	l2.scan()
	it := l2.item(src2)
	final := strings.TrimSuffix(src2, ".avi") + ".mp4"
	l2.gen("Crash3.mp4", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac"})...)
	originals2 := filepath.Join(l2.root, library.OriginalsFolder)
	os.MkdirAll(originals2, 0o755)
	kept2 := filepath.Join(originals2, "Crash3.avi")
	os.Rename(src2, kept2)
	run2, _ := l2.engine.createRun(ctx, l2.lib.ID, preset(t, PresetCompatible), OutputKeep, false, false, 0, "")
	if _, err = l2.db.ExecContext(ctx, `INSERT INTO ltc_files(run_id, item_id, path, verdict, state, temp_path, final_path, kept_path, swap) VALUES(?, ?, ?, 'convert', 'running', ?, ?, ?, 'keep')`,
		run2, it.ID, src2, filepath.Join(l2.root, library.WorkFolder, "gone.mp4"), final, kept2); err != nil {
		t.Fatal(err)
	}
	l2.engine.Recover(ctx)
	if got := l2.item(final); got.ID != it.ID {
		t.Fatalf("the library wasn't told: %+v", got)
	}
}

func pathOr(p, other string) string {
	if exists(p) {
		return p
	}
	return other
}

func TestRealIdempotent(t *testing.T) {
	// Whatever a preset makes, running that preset again finds nothing to do.
	for _, id := range []string{PresetCompatible, PresetSaveSpace, PresetBest} {
		t.Run(id, func(t *testing.T) {
			l := newLab(t)
			if id == PresetSaveSpace && !has("libx265") {
				t.Skip("no libx265")
			}
			l.gen("Again.avi", join(pic(3, "640x360"), tone(3, 300, "5.1"), tone(3, 400, "stereo"), []string{"-map", "0", "-map", "1", "-map", "2", "-c:v", "mpeg4", "-q:v", "2", "-c:a", "ac3", "-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=fre"})...)
			l.scan()
			p := preset(t, id)
			if id == PresetSaveSpace {
				p.Video.MinSavingPercent, p.Video.SkipBelowKbps = 0, 0
			}
			r := l.run(p, OutputReplace)
			if r.Summary.Done != 1 {
				t.Fatalf("%+v %+v", r.Summary, l.files(r))
			}
			r2 := l.run(p, OutputReplace)
			if r2.Summary.Done != 0 || r2.Summary.Skip != 1 {
				for _, f := range l.files(r2) {
					t.Logf("%s %s: %+v", f.Verdict, f.Reason, f.Plan.Steps)
				}
				t.Fatalf("the second run should find nothing to do: %+v", r2.Summary)
			}
		})
	}
}

func TestRealNotWorthIt(t *testing.T) {
	l := newLab(t)
	if !has("libx265") {
		t.Skip("no libx265")
	}
	// Nearly nothing to compress: H.265 can't beat this by 90%.
	src := l.gen("Small.mkv", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx264", "-preset", "medium", "-crf", "35", "-c:a", "aac"})...)
	want, _ := os.ReadFile(src)
	l.scan()
	p := preset(t, PresetSaveSpace)
	p.Video.SkipBelowKbps = 0
	p.Video.MinSavingPercent = 90
	r := l.run(p, OutputKeep)
	row := l.row(r, "Small.mkv")
	if row.State != FileNotWorth || r.Summary.NotWorth != 1 {
		t.Fatalf("%+v %+v", row, r.Summary)
	}
	got, _ := os.ReadFile(src)
	if string(got) != string(want) {
		t.Fatal("the old file changed")
	}
	// It isn't tried again with the same recipe, until the file changes.
	r2 := l.run(p, OutputKeep)
	if r2.Summary.Skip != 1 || !strings.Contains(l.row(r2, "Small.mkv").Reason, "no smaller") {
		t.Fatalf("%+v %+v", r2.Summary, l.row(r2, "Small.mkv"))
	}
	l.noneLeftBehind()
}

func TestRealSample(t *testing.T) {
	l := newLab(t)
	src := l.gen("Sample.avi", join(pic(4, "320x240"), tone(4, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "mp3"})...)
	l.scan()
	cache := t.TempDir()
	r, err := l.engine.Sample(context.Background(), l.lib, l.item(src).ID, preset(t, PresetCompatible), cache)
	if err != nil {
		t.Fatal(err)
	}
	if r.Bytes == 0 || r.EstFileBytes == 0 || r.Speed <= 0 {
		t.Fatalf("%+v", r)
	}
	entries, _ := os.ReadDir(filepath.Join(cache, "convert-samples"))
	if len(entries) != 0 {
		t.Fatalf("a sample was left behind: %v", entries)
	}
	if !exists(src) || exists(filepath.Join(l.root, "Sample.mp4")) {
		t.Fatal("a sample touched the library")
	}
}

func TestRealAttachments(t *testing.T) {
	font := filepath.Join(t.TempDir(), "font.ttf")
	os.WriteFile(font, []byte("pretend this is a font"), 0o644)
	make := func(l *lab, name string) string {
		return l.gen(name, join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "mpeg4", "-c:a", "aac", "-attach", font, "-metadata:s:t:0", "mimetype=application/x-truetype-font"})...)
	}
	t.Run("kept in an MKV", func(t *testing.T) {
		l := newLab(t)
		src := make(l, "Fonts.mkv")
		l.scan()
		p := preset(t, PresetSaveSpace)
		p.Video.Codec = "h264"
		p.Video.SkipBelowKbps, p.Video.MinSavingPercent = 0, 0
		r := l.run(p, OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		if a := Inspect(src, l.probe(src)).Attachments; len(a) != 1 {
			t.Fatalf("attachment lost: %+v", a)
		}
	})
	t.Run("an MP4 can't hold one", func(t *testing.T) {
		l := newLab(t)
		make(l, "No Fonts.mkv")
		l.scan()
		r := l.run(preset(t, PresetCompatible), OutputReplace)
		if r.Summary.Done != 1 {
			t.Fatalf("%+v %+v", r.Summary, l.files(r))
		}
		said := false
		for _, st := range l.row(r, "No Fonts.mkv").Plan.Steps {
			if st.Kind == "attachment" && st.Action == ActDrop {
				said = true
			}
		}
		if !said {
			t.Fatal("the plan doesn't say the font is dropped")
		}
	})
}

// Only on a machine with a working NVIDIA encoder, which is all this can be tried on here.
func TestRealHardwareEncoder(t *testing.T) {
	l := newLab(t)
	l.engine.HardwareAllowed = func() []string { return []string{"h264_nvenc"} }
	env := l.engine.Env(context.Background())
	if !slices.Contains(env.Hardware, "h264_nvenc") || !slices.Contains(env.Hardware, "hevc_nvenc") {
		t.Skip("no working NVIDIA encoder here")
	}
	src := l.gen("Hardware.mkv", join(pic(3, "1280x720"), tone(3, 300, "stereo"), []string{"-c:v", "mpeg2video", "-q:v", "2", "-c:a", "aac"})...)
	hdr := l.gen("Hardware HDR.mkv", join(pic(3, "1280x720"), tone(3, 300, "stereo"), []string{"-c:v", "libx265", "-preset", "ultrafast", "-pix_fmt", "yuv420p10le", "-x265-params", "log-level=error:hdr10=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc", "-c:a", "aac"})...)
	l.scan()
	p := preset(t, PresetSaveSpace)
	p.Video.Accept, p.Video.SkipBelowKbps, p.Video.MinSavingPercent = nil, 0, 0
	p.Video.MaxHeight = 480
	r := l.run(p, OutputReplace)
	if r.Summary.Done != 2 {
		t.Fatalf("%+v %+v", r.Summary, l.files(r))
	}
	if got := l.row(r, "Hardware.mkv").Plan.Encoder; got != "hevc_nvenc" {
		t.Fatalf("an ordinary picture used %s", got)
	}
	if got := l.row(r, "Hardware HDR.mkv").Plan.Encoder; got != "libx265" {
		t.Fatalf("HDR used %s", got)
	}
	if v := l.probe(src).Video(); v.CodecName != "hevc" {
		t.Fatalf("%+v", v)
	}
	if v := l.probe(hdr).Video(); v.CodecName != "hevc" || v.ColorTransfer != "smpte2084" || v.Height != 480 {
		t.Fatalf("%+v", v)
	}
}

// A real HDR10+ file can't be made here (Ubuntu's x265 leaves it out), so what the plan
// says about one is checked with what a look at its frames would have found.
func TestHDR10PlusIsSaidToBeLost(t *testing.T) {
	src := &Source{Path: "/m/a.mkv", Ext: "mkv", Duration: 60, Video: []VideoStream{{Index: 0, Codec: "hevc", PixFmt: "yuv420p10le", Width: 3840, Height: 2160, Depth: 10, Range: "hdr10", Dynamic: "HDR10+", Default: true}}}
	p, _ := PresetPolicy(PresetSaveSpace)
	if pl := Decide(src, p, testEnv(), PlanOptions{}); pl.Verdict != VerdictSkip {
		t.Fatalf("left alone it stays HDR10+: %+v", pl)
	}
	p.Video.MaxHeight = 1080
	pl := Decide(src, p, testEnv(), PlanOptions{})
	said := false
	for _, w := range pl.Warnings {
		if strings.Contains(w, "HDR10+") {
			said = true
		}
	}
	if pl.Verdict != VerdictConvert || !said {
		t.Fatalf("%+v", pl)
	}
}

func TestRealHEVCInMP4IsTaggedForApple(t *testing.T) {
	l := newLab(t)
	if !has("libx265") {
		t.Skip("no libx265")
	}
	src := l.gen("Tag.mkv", join(pic(2, "320x240"), tone(2, 300, "stereo"), []string{"-c:v", "libx265", "-preset", "ultrafast", "-x265-params", "log-level=error", "-c:a", "aac"})...)
	l.scan()
	p := preset(t, PresetSaveSpace)
	p.Preset = PresetCustom
	p.Container = "mp4"
	r := l.run(p, OutputReplace)
	if r.Summary.Done != 1 {
		t.Fatalf("%+v %+v", r.Summary, l.files(r))
	}
	if v := l.probe(strings.TrimSuffix(src, ".mkv") + ".mp4").Video(); v.CodecTag != "hvc1" {
		t.Fatalf("%+v", v)
	}
}
