// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libtranscode

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// TaskType is how conversions appear in the activity panel.
const TaskType = "convert"

// Engine runs conversions, one library at a time.
type Engine struct {
	DB      *db.DB
	Store   *library.Store
	Scanner *library.Scanner
	Tools   *ffmpeg.Tools
	Tasks   *tasks.Registry
	Log     *slog.Logger
	// Busy counts people playing something right now. May be nil.
	Busy func() int
	// Playing says whether someone is playing this item right now. May be nil.
	Playing func(itemID int64) bool
	// SaveMeta: the server may write beside media (Settings, Metadata).
	SaveMeta func() bool
	// HardwareAllowed lists the hardware encoders ticked on the Transcoding page.
	HardwareAllowed func() []string
	// Now replaces the clock in tests.
	Now func() time.Time
	// HoldTick replaces how often a running conversion looks at its conditions, in tests.
	HoldTick time.Duration
	// TdarrPoll replaces how often a connected Tdarr is asked what it's doing, in tests.
	TdarrPoll time.Duration
	// AutoSettle replaces how long a new file must hold still, in tests.
	AutoSettle time.Duration

	tdarrW tdarrWatch

	slot chan struct{}
	once sync.Once
	mu   sync.Mutex
	live map[int64]context.CancelFunc
	// busy: which library each real run is on.
	busy map[int64]int64
	// auto: the libraries with someone watching their new files.
	auto map[int64]*autoWatch
	// tested holds what test encodes found, by encoder.
	tested map[string]bool
}

func (e *Engine) init() {
	e.once.Do(func() {
		e.slot = make(chan struct{}, 1)
		e.live = map[int64]context.CancelFunc{}
		e.busy = map[int64]int64{}
		e.auto = map[int64]*autoWatch{}
		e.tested = map[string]bool{}
		if e.Log == nil {
			e.Log = slog.Default().With("cat", "transcode")
		}
	})
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// Env says what this machine offers, right now.
func (e *Engine) Env(ctx context.Context) Env {
	e.init()
	s := LoadSettings(ctx, e.DB)
	env := Env{Encoders: e.Tools.Encoders, HasFilter: e.Tools.HasFilter, ToneMap: e.Tools.ToneMapFilter, Threads: s.Threads,
		Sidecars: e.SaveMeta == nil || e.SaveMeta()}
	if env.Threads == 0 {
		env.Threads = max(runtime.NumCPU()/2, 1)
	}
	if s.UseHardware && e.HardwareAllowed != nil {
		env.Hardware = e.workingHardware(ctx, e.HardwareAllowed())
	}
	return env
}

// workingHardware test-encodes the encoders of each allowed family, once each. FFmpeg
// lists encoders the machine has no hardware for, so only a test says.
func (e *Engine) workingHardware(ctx context.Context, allowed []string) []string {
	var out []string
	for _, name := range allowed {
		family := HardwareFamily(name)
		if family == "" {
			continue
		}
		for _, codec := range []string{"h264", "hevc", "av1"} {
			enc := hardwareName(codec, family)
			if e.Tools.Encoders[enc] && e.testEncoder(ctx, enc, codec, family) {
				out = append(out, enc)
			}
		}
	}
	return out
}

func (e *Engine) testEncoder(ctx context.Context, name, codec, family string) bool {
	e.mu.Lock()
	ok, known := e.tested[name]
	e.mu.Unlock()
	if known {
		return ok
	}
	c := hardwareEncoder(name, codec, family, 23, "balanced", 240)
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	args = append(args, c.inputArgs...)
	args = append(args, "-f", "lavfi", "-i", "color=c=black:s=320x240:r=25:d=0.4")
	if len(c.filters) > 0 {
		args = append(args, "-vf", strings.Join(c.filters, ","))
	}
	args = append(args, "-c:v", name)
	args = append(args, c.args...)
	args = append(args, "-f", "null", "-")
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	_, _, err := ffmpeg.Run(cctx, e.Tools.FFmpeg, args, "job", "test a hardware encoder")
	cancel()
	e.mu.Lock()
	e.tested[name] = err == nil
	e.mu.Unlock()
	return err == nil
}

// Eligible says whether a library's type holds video files this can work on.
func Eligible(libType string) bool {
	switch libType {
	case "movies", "shows", "anime", "videos", library.CommercialsType:
		return true
	}
	return library.IsCreatorLibrary(libType)
}

// Blocked says why a real run can't start here, "" when it can. A dry run reads only, so
// nothing stops it.
func (e *Engine) Blocked(ctx context.Context, lib *library.Library, r Rule) string {
	s := LoadSettings(ctx, e.DB)
	switch {
	case !Eligible(lib.Type):
		return "This kind of library doesn't hold video files to convert."
	case !s.Enabled:
		return "Library transcoding is off. Read the warning and turn it on first."
	case lib.ReadOnly:
		return "This library is set to read-only, so its files can't be changed."
	case e.SaveMeta != nil && !e.SaveMeta() && !r.AllowWrites:
		return "This server is set to keep metadata out of media folders (Save next to media, in Metadata settings). To convert here anyway, allow it for this library on this page. Trying a file and checking the library still work."
	}
	return ""
}

// RunRequest asks for a run.
type RunRequest struct {
	Library int64
	Policy  Policy
	Output  string
	Dry     bool
	// Items limits it to these items, when given.
	Items []int64
	// Auto: a scan found these, and nobody pressed anything.
	Auto bool
	By   tasks.By
}

// Start begins a run and returns its id.
func (e *Engine) Start(ctx context.Context, q RunRequest) (int64, error) {
	e.init()
	lib, err := e.Store.Library(ctx, q.Library)
	if err != nil {
		return 0, err
	}
	if !q.Dry {
		if why := e.Blocked(ctx, lib, LoadRule(ctx, e.DB, q.Library)); why != "" {
			return 0, errors.New(why)
		}
	} else if !Eligible(lib.Type) {
		return 0, errors.New("this kind of library doesn't hold video files to convert")
	}
	if !ValidOutput(q.Output) {
		return 0, errors.New("that's not one of the ways of writing the new file")
	}
	// It would wait its turn, but two runs of the same library would plan the same files.
	if !q.Dry && e.libraryBusy(q.Library) {
		return 0, errBusy
	}
	id, err := e.createRun(ctx, q.Library, q.Policy, q.Output, q.Dry, q.Auto, q.By.ID, q.By.Name)
	if err != nil {
		return 0, err
	}
	rctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.live[id] = cancel
	if !q.Dry {
		e.busy[id] = q.Library
	}
	e.mu.Unlock()
	title := "Converting " + lib.Name
	if q.Auto {
		title = "Converting new files in " + lib.Name
	}
	if q.Dry {
		title = "Checking " + lib.Name + " for conversion"
	}
	unit := "seconds of video"
	if q.Dry {
		unit = "files"
	}
	t := e.Tasks.Queue(tasks.Spec{Type: TaskType, Title: title, Phase: "Waiting for its turn", LibraryID: lib.ID, LibraryName: lib.Name, Unit: unit,
		By: q.By, Cancel: cancel, Pausable: true, RetryKey: itoa(id), Quiet: q.Dry || q.Auto})
	sid := logging.NewID("convert")
	rctx = logging.WithSession(rctx, sid)
	logging.Go("library conversion", func() {
		defer func() {
			e.mu.Lock()
			delete(e.live, id)
			delete(e.busy, id)
			e.mu.Unlock()
			cancel()
		}()
		e.runOne(rctx, id, lib, q, t)
	}, "cat", "transcode", "session", sid)
	return id, nil
}

// errBusy: a library is being converted already.
var errBusy = errors.New("this library is being converted already")

// libraryBusy says whether a real run of this library is going or waiting its turn.
func (e *Engine) libraryBusy(library int64) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, l := range e.busy {
		if l == library {
			return true
		}
	}
	return false
}

func (e *Engine) runOne(ctx context.Context, id int64, lib *library.Library, q RunRequest, t *tasks.Handle) {
	select {
	case e.slot <- struct{}{}:
		defer func() { <-e.slot }()
	case <-ctx.Done():
		e.setRunState(context.WithoutCancel(ctx), id, "cancelled", nil)
		t.Finish(ctx.Err())
		return
	}
	t.Begin()
	e.setRunState(ctx, id, "running", nil)
	e.Log.InfoContext(ctx, "conversion started", "library", lib.Name, "dry", q.Dry, "preset", q.Policy.Preset, "output", q.Output)
	units, sum, err := e.survey(ctx, id, lib, q, t)
	if err == nil && !q.Dry {
		// Written now as well, so that a server that stops partway leaves a true picture.
		e.setRunState(ctx, id, "running", &sum)
		err = e.convertAll(ctx, id, lib, q, units, &sum, t)
	}
	state := "done"
	switch {
	case errors.Is(err, context.Canceled):
		state = "cancelled"
	case err != nil:
		state = "failed"
	}
	bg := context.WithoutCancel(ctx)
	e.setRunState(bg, id, state, &sum)
	t.Item("", "")
	nothing := sum.Convert+sum.Remux == 0
	if q.Auto {
		e.memoAuto(bg, id, q.Policy, state)
		if nothing && err == nil {
			// Scans come round often, and this would fill the list with no news.
			if _, derr := e.DB.ExecContext(bg, `DELETE FROM ltc_runs WHERE id = ?`, id); derr != nil {
				e.Log.WarnContext(ctx, "could not tidy away a conversion with nothing to do", "run", id, "err", derr)
			}
		}
	}
	if !q.Auto || !nothing {
		t.Result(summarize(q.Dry, sum))
	}
	e.Log.InfoContext(ctx, "conversion finished", "library", lib.Name, "state", state, "done", sum.Done, "failed", sum.Failed, "err", err)
	t.Finish(err)
}

// unit is an item's files: one, or the parts of a split film, which go together.
type unit struct {
	item  library.ConversionItem
	files []*part
}

type part struct {
	row   int64
	src   *Source
	plan  *Plan
	size  int64
	mtime int64
	temp  string
	final string
	sups  []string
	kept  string
	// covers: pictures taken out of the old file for the new one to attach.
	covers []string
}

// Refusals for what isn't a plain video file.
const (
	reasonDisc    = "Disc images and folders aren't converted: they'd lose their menus. They play as they are, with the server converting while playing if a device needs it."
	reasonMissing = "This file can't be found right now, so it's left out."
	reasonOutside = "This file isn't inside the library's folders once links are followed, so it's left alone."
)

// survey looks at every file and records the plan. A real run then works from these rows.
func (e *Engine) survey(ctx context.Context, run int64, lib *library.Library, q RunRequest, t *tasks.Handle) ([]*unit, Summary, error) {
	sum := Summary{}
	t.Phase("Looking at each file")
	items, err := e.Store.ConversionItems(ctx, lib.ID)
	if err != nil {
		return nil, sum, err
	}
	if len(q.Items) > 0 {
		items = slices.DeleteFunc(items, func(it library.ConversionItem) bool { return !slices.Contains(q.Items, it.ID) })
	}
	env := e.Env(ctx)
	var units []*unit
	if q.Dry {
		t.Progress(0, int64(len(items)))
	}
	for i, it := range items {
		if err := t.Wait(ctx); err != nil {
			return units, sum, err
		}
		t.Item(filepath.Base(it.Path), it.Path)
		if q.Dry {
			t.Progress(int64(i), int64(len(items)))
		}
		u, err := e.planItem(ctx, lib, it, q.Policy, env)
		if err != nil {
			if ctx.Err() != nil {
				return units, sum, ctx.Err()
			}
			words, fix := tasks.Explain(err)
			t.Problem(it.Path, strings.TrimSpace("This file couldn't be read. "+words+" "+fix))
			continue
		}
		for n, f := range u.files {
			if err := e.recordPlan(ctx, run, u, n, f, q.Dry); err != nil {
				return units, sum, err
			}
			sum.addPlan(f)
		}
		units = append(units, u)
	}
	e.fillRoom(ctx, units, q.Output, &sum)
	if q.Dry {
		t.Progress(int64(len(items)), int64(len(items)))
	}
	return units, sum, nil
}

func (sum *Summary) addPlan(f *part) {
	sum.Files++
	sum.BytesBefore += f.size
	switch f.plan.Verdict {
	case VerdictConvert:
		sum.Convert++
	case VerdictRemux:
		sum.Remux++
	case VerdictSkip:
		sum.Skip++
	case VerdictRefuse:
		sum.Refuse++
	}
	if f.plan.Verdict == VerdictConvert || f.plan.Verdict == VerdictRemux {
		sum.EstBytes += f.plan.EstBytes
		sum.EstSec += f.plan.EstSec
	} else {
		sum.EstBytes += f.size
	}
}

// planItem reads an item's files and decides on them. Nothing is written.
func (e *Engine) planItem(ctx context.Context, lib *library.Library, it library.ConversionItem, p Policy, env Env) (*unit, error) {
	u := &unit{item: it}
	refused := func(path, reason string) *unit {
		pl := refuse(reason)
		st, err := os.Stat(path)
		f := &part{src: &Source{Path: path}, plan: pl}
		if err == nil {
			f.size, f.mtime = st.Size(), st.ModTime().Unix()
		}
		u.files = []*part{f}
		return u
	}
	if it.Kind != "video" {
		if library.IsDiscKind(it.Kind) || strings.Contains(it.Kind, "cd") {
			return refused(it.Path, reasonDisc), nil
		}
		return refused(it.Path, "This isn't a video file, so it's left as it is."), nil
	}
	if it.Missing {
		return refused(it.Path, reasonMissing), nil
	}
	paths := library.StackedParts(it.Path)
	for _, path := range paths {
		if !insideLibrary(path, lib.Paths) {
			return refused(path, reasonOutside), nil
		}
	}
	srcs := make([]*Source, 0, len(paths))
	protected := false
	for _, path := range paths {
		st, err := os.Lstat(path)
		if err != nil {
			return refused(path, reasonMissing), nil
		}
		pctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		probe, err := e.Tools.ProbeFile(pctx, path)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if Detect(path, nil) {
				return refused(path, ProtectedReason), nil
			}
			return refused(path, "This file couldn't be read as a video. It may be damaged."), nil
		}
		if Detect(path, probe) {
			protected = true
		}
		src := Inspect(path, probe)
		if src.Size == 0 {
			src.Size = st.Size()
		}
		src.AddSidecars(library.FindSubtitles(path))
		e.lookAtFrames(ctx, src)
		srcs = append(srcs, src)
	}
	if protected {
		return refused(it.Path, ProtectedReason), nil
	}
	// The first pass says whether any part needs its picture converted. If so they all do,
	// because they're joined when played and have to match.
	force := false
	if len(srcs) > 1 {
		for _, src := range srcs {
			pl := Decide(src, p, env, PlanOptions{})
			for _, st := range pl.Steps {
				if st.Kind == "video" && st.Action == ActConvert {
					force = true
				}
			}
		}
	}
	for _, src := range srcs {
		st, err := os.Stat(src.Path)
		if err != nil {
			return refused(src.Path, reasonMissing), nil
		}
		pl := Decide(src, p, env, PlanOptions{ForceVideo: force})
		if len(srcs) > 1 && (pl.Verdict == VerdictConvert || pl.Verdict == VerdictRemux) {
			pl.Notes = append(pl.Notes, fmt.Sprintf("This is one of %d parts of a split film. The parts are converted together with the same settings and stay separate files.", len(srcs)))
		}
		if pl.Verdict == VerdictRefuse && len(srcs) > 1 {
			return refused(src.Path, "One part of this split film can't be converted, so all its parts are left alone. "+pl.Reason), nil
		}
		if e.recall(ctx, src.Path, p, st.Size(), st.ModTime().Unix()) == FileNotWorth && (pl.Verdict == VerdictConvert) {
			pl.Verdict, pl.Reason = VerdictSkip, "Last time this recipe was tried on this file it came out no smaller, so it isn't tried again."
		}
		u.files = append(u.files, &part{src: src, plan: pl, size: st.Size(), mtime: st.ModTime().Unix()})
	}
	return u, nil
}

// lookAtFrames finds out what only the frames say. HDR10+ is carried frame by frame, so a
// look at the stream doesn't show it, and it's the difference between "kept" and "lost" in
// what the preview promises. Only HDR10 files pay for the extra look.
func (e *Engine) lookAtFrames(ctx context.Context, src *Source) {
	for i := range src.Video {
		v := &src.Video[i]
		if v.Range != "hdr10" || v.DV {
			continue
		}
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		args := []string{"-v", "error", "-select_streams", strconv.Itoa(v.Index), "-read_intervals", "%+#2", "-show_frames", "-of", "json"}
		args = append(args, ffmpeg.FileInput(src.Path)...)
		out, _, err := ffmpeg.Run(pctx, e.Tools.FFprobe, args, "job", "look for HDR10+")
		cancel()
		if err != nil {
			continue
		}
		var frames struct {
			Frames []struct {
				SideData []map[string]any `json:"side_data_list"`
			} `json:"frames"`
		}
		if json.Unmarshal(out, &frames) != nil {
			continue
		}
		for _, fr := range frames.Frames {
			for _, sd := range fr.SideData {
				if t, _ := sd["side_data_type"].(string); strings.Contains(t, "SMPTE2094-40") {
					v.Dynamic = "HDR10+"
				}
			}
			mastering, light := hdrSideData(fr.SideData)
			if v.Mastering == "" {
				v.Mastering = mastering
			}
			if v.ContentLight == "" {
				v.ContentLight = light
			}
		}
	}
}

// insideLibrary: the path is a plain file under a library folder, with links followed.
func insideLibrary(path string, roots []string) bool {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	st, err := os.Lstat(real)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	for _, r := range roots {
		rr, err := filepath.EvalSymlinks(r)
		if err != nil {
			rr = r
		}
		if rel, err := filepath.Rel(rr, real); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func (e *Engine) recordPlan(ctx context.Context, run int64, u *unit, n int, f *part, dry bool) error {
	state := FilePending
	switch {
	case f.plan.Verdict == VerdictSkip:
		state = FileSkipped
	case f.plan.Verdict == VerdictRefuse:
		state = FileRefused
	case dry:
		state = FilePlanned
	}
	b, err := json.Marshal(f.plan)
	if err != nil {
		return err
	}
	res, err := e.DB.ExecContext(ctx, `INSERT INTO ltc_files(run_id, item_id, part, path, verdict, reason, plan, state, size_before, mtime_before, est_bytes, est_sec, duration)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run, u.item.ID, n, f.src.Path, f.plan.Verdict, f.plan.Reason, string(b), state, f.size, f.mtime, f.plan.EstBytes, f.plan.EstSec, f.src.Duration)
	if err != nil {
		return err
	}
	f.row, _ = res.LastInsertId()
	return nil
}

// fillRoom works out what a job needs on each drive: the largest new files at once, as
// temporary files, and the new files for good where the old ones are being kept as well.
func (e *Engine) fillRoom(ctx context.Context, units []*unit, output string, sum *Summary) {
	floor := int64(LoadSettings(ctx, e.DB).MinFreeGB) << 30
	needs := map[string]int64{}
	free := map[string]int64{}
	for _, u := range units {
		for _, f := range u.files {
			if f.plan.Verdict != VerdictConvert && f.plan.Verdict != VerdictRemux {
				continue
			}
			dir := filepath.Dir(f.src.Path)
			if _, ok := free[dir]; !ok {
				free[dir] = sysinfo.FreeBytes(dir)
			}
			est := max(f.plan.EstBytes, f.size/4)
			switch output {
			case OutputKeep, OutputBeside:
				needs[dir] += est
			default:
				if est > needs[dir] {
					needs[dir] = est
				}
			}
		}
	}
	// Folders on one drive share its room.
	byDrive := map[string]*DriveRoom{}
	var order []string
	for dir, n := range needs {
		key := dir
		if fi, err := sysinfo.FSInfo(dir); err == nil && fi.Mount != "" {
			key = fi.Mount
		}
		d := byDrive[key]
		if d == nil {
			d = &DriveRoom{Folder: key, Free: free[dir]}
			byDrive[key] = d
			order = append(order, key)
		}
		d.Needs += n
	}
	slices.Sort(order)
	for _, k := range order {
		d := byDrive[k]
		d.Enough = d.Free < 0 || d.Free-d.Needs >= floor
		sum.NeedsBytes += d.Needs
		sum.Drives = append(sum.Drives, *d)
	}
}

// convertAll works through the files, some at a time.
func (e *Engine) convertAll(ctx context.Context, run int64, lib *library.Library, q RunRequest, units []*unit, sum *Summary, t *tasks.Handle) error {
	var todo []*unit
	var total float64
	for _, u := range units {
		if u.needsWork() {
			todo = append(todo, u)
			for _, f := range u.files {
				total += f.workSeconds()
			}
		}
	}
	if len(todo) == 0 {
		if !q.Auto {
			t.Progress(1, 1)
		}
		return nil
	}
	s, err := readSettings(ctx, e.DB)
	if err != nil {
		return fmt.Errorf("couldn't read the transcoding settings, so nothing was converted: %w", err)
	}
	env := e.Env(ctx)
	totalInt := int64(max(total, 1))
	var doneSec atomic.Int64 // thousandths of a second, finished work
	var live sync.Map        // file row -> *atomic.Int64 thousandths in progress
	report := func() {
		sumLive := doneSec.Load()
		live.Range(func(_, v any) bool { sumLive += v.(*atomic.Int64).Load(); return true })
		t.Progress(min(sumLive/1000, totalInt), totalInt)
	}
	t.Progress(0, totalInt)
	t.Phase("Converting: " + countFiles(len(todo)) + " to do")
	jobs := make(chan *unit)
	var wg sync.WaitGroup
	var sumMu sync.Mutex
	var finished atomic.Int64
	var postponed []*unit // someone was playing these: try again at the end
	worker := func() {
		defer wg.Done()
		for u := range jobs {
			if ctx.Err() != nil {
				continue
			}
			prog := func(f *part, sec float64) {
				v, _ := live.LoadOrStore(f.row, &atomic.Int64{})
				v.(*atomic.Int64).Store(int64(sec * 1000))
				report()
			}
			outcome := e.convertUnit(ctx, run, lib, q, u, s, env, t, prog, sumFunc(&sumMu, sum))
			if outcome == FilePostponed {
				sumMu.Lock()
				postponed = append(postponed, u)
				sumMu.Unlock()
			}
			for _, f := range u.files {
				live.Delete(f.row)
				doneSec.Add(int64(f.workSeconds() * 1000))
			}
			report()
			t.Phase(fmt.Sprintf("%d of %d files done", finished.Add(1), len(todo)))
		}
	}
	for range s.MaxJobs {
		wg.Add(1)
		go worker()
	}
	send := func(list []*unit) error {
		for _, u := range list {
			if err := e.gate(ctx, t); err != nil {
				return err
			}
			select {
			case jobs <- u:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}
	err = send(todo)
	close(jobs)
	wg.Wait()
	if err == nil {
		err = ctx.Err()
	}
	// Whatever was being played gets one more go, now that the rest is out of the way.
	if err == nil && len(postponed) > 0 {
		for _, u := range postponed {
			if ctx.Err() != nil || e.isPlaying(u.item.ID) {
				for _, f := range u.files {
					e.updateFile(ctx, f.row, map[string]any{"state": FilePostponed, "message": "Someone was playing this, so it was left for next time."})
				}
				sumFunc(&sumMu, sum)(func(s *Summary) { s.Postponed++ })
				t.Problem(u.item.Path, "Someone was playing this file, so it was left for next time. Retry when they've finished.")
				continue
			}
			prog := func(f *part, sec float64) {}
			if e.convertUnit(ctx, run, lib, q, u, s, env, t, prog, sumFunc(&sumMu, sum)) == FilePostponed {
				sumFunc(&sumMu, sum)(func(s *Summary) { s.Postponed++ })
				t.Problem(u.item.Path, "Someone was playing this file, so it was left for next time. Retry when they've finished.")
			}
		}
		err = ctx.Err()
	}
	return err
}

func sumFunc(mu *sync.Mutex, sum *Summary) func(func(*Summary)) {
	return func(f func(*Summary)) {
		mu.Lock()
		defer mu.Unlock()
		f(sum)
	}
}

func (u *unit) needsWork() bool {
	for _, f := range u.files {
		if f.plan.Verdict == VerdictConvert || f.plan.Verdict == VerdictRemux {
			return true
		}
	}
	return false
}

func (f *part) workSeconds() float64 {
	if f.plan.Verdict != VerdictConvert && f.plan.Verdict != VerdictRemux {
		return 0
	}
	return max(f.src.Duration, 1)
}

func (e *Engine) isPlaying(item int64) bool { return e.Playing != nil && e.Playing(item) }

// holdWords says why work is being held, "" when it can go ahead.
func (e *Engine) holdWords(s Settings) string {
	if !s.inHours(e.now().Hour()) {
		return fmt.Sprintf("Waiting for the hours it's allowed to run (%02d:00 to %02d:00)", s.HoursFrom, s.HoursTo)
	}
	if s.PauseWhileStreaming && e.Busy != nil && e.Busy() > 0 {
		return "Waiting: someone is streaming"
	}
	return ""
}

func (e *Engine) tick() time.Duration {
	if e.HoldTick > 0 {
		return e.HoldTick
	}
	return 3 * time.Second
}

// gate holds until work may go on: not paused, inside the hours, nobody streaming. Settings
// are read again each time, so changing them takes effect at once.
func (e *Engine) gate(ctx context.Context, t *tasks.Handle) error {
	for {
		if err := t.Wait(ctx); err != nil {
			return err
		}
		why := e.holdWords(LoadSettings(ctx, e.DB))
		if why == "" {
			return nil
		}
		t.Phase(why)
		select {
		case <-time.After(e.tick()):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// convertUnit converts one item's files. Nothing is replaced until every file of the item
// has been converted and checked. It returns the state it ended in.
func (e *Engine) convertUnit(ctx context.Context, run int64, lib *library.Library, q RunRequest, u *unit, s Settings, env Env, t *tasks.Handle, prog func(*part, float64), edit func(func(*Summary))) string {
	// Someone's watching it: come back to it at the end, rather than wait now.
	if e.isPlaying(u.item.ID) {
		for _, f := range u.files {
			e.updateFile(ctx, f.row, map[string]any{"state": FilePending, "message": "Waiting: someone is playing this."})
		}
		return FilePostponed
	}
	fail := func(state, message, detail string) string {
		e.cleanTemps(ctx, u)
		for _, f := range u.files {
			e.updateFile(ctx, f.row, map[string]any{"state": state, "message": message, "detail": detail, "temp_path": "", "finished_at": db.Now()})
		}
		if ctx.Err() == nil {
			t.Problem(u.item.Path, message)
		}
		edit(func(sum *Summary) {
			switch state {
			case FileChanged:
				sum.Changed++
			case FileNotWorth:
				sum.NotWorth++
			default:
				sum.Failed++
			}
		})
		e.Log.WarnContext(ctx, "a file wasn't converted", "file", u.item.Path, "state", state, "why", message, "detail", detail)
		return state
	}
	// Convert every file that needs it. A part whose plan is "nothing to do" keeps its file.
	for _, f := range u.files {
		if f.plan.Verdict != VerdictConvert && f.plan.Verdict != VerdictRemux {
			continue
		}
		t.Item(filepath.Base(f.src.Path), f.src.Path)
		if err := e.prepare(ctx, f, q.Output, s); err != nil {
			var nr *noRoom
			if errors.As(err, &nr) {
				return fail(FileFailed, err.Error(), noRoomDetail)
			}
			return fail(FileFailed, err.Error(), "")
		}
		e.updateFile(ctx, f.row, map[string]any{"state": FileRunning, "message": "", "temp_path": f.temp})
		if err := e.encode(ctx, f, s, t, prog); err != nil {
			if ctx.Err() != nil {
				e.cleanTemps(context.WithoutCancel(ctx), u)
				for _, g := range u.files {
					e.updateFile(ctx, g.row, map[string]any{"state": FilePending, "message": "Cancelled before this was done. The old file is untouched.", "temp_path": ""})
				}
				return FilePending
			}
			var nr *noRoom
			if errors.As(err, &nr) {
				return fail(FileFailed, nr.Error(), noRoomDetail)
			}
			words := "FFmpeg couldn't convert this file, so the old one stays as it was. It may be damaged. The log has the details."
			detail := err.Error()
			if strings.Contains(strings.ToLower(detail), "drm protected") || strings.Contains(strings.ToLower(detail), "encrypted") {
				words = ProtectedReason
			}
			return fail(FileFailed, words, detail)
		}
		t.Phase("Checking the new file")
		if err := Verify(ctx, e.Tools, f.src, f.plan, f.temp); err != nil {
			if ctx.Err() != nil {
				e.cleanTemps(context.WithoutCancel(ctx), u)
				return FilePending
			}
			return fail(FileFailed, err.Error(), "")
		}
		for _, sup := range f.sups {
			if st, err := os.Stat(sup); err != nil || st.Size() == 0 {
				return fail(FileFailed, "A subtitle file that had to be saved beside the video came out empty, so the old file stays.", "")
			}
		}
		// Someone may have replaced the file while this ran.
		if st, err := os.Stat(f.src.Path); err != nil || st.Size() != f.size || st.ModTime().Unix() != f.mtime {
			return fail(FileChanged, "This file changed while it was being converted, so the new version was thrown away. It'll be looked at again next time.", "")
		}
	}
	// Worth it?
	var before, after int64
	minSaving := 0
	for _, f := range u.files {
		if f.temp == "" {
			continue
		}
		before += f.size
		if st, err := os.Stat(f.temp); err == nil {
			after += st.Size()
		}
		minSaving = max(minSaving, f.plan.minSaving)
	}
	if minSaving > 0 && before > 0 && after > before*int64(100-minSaving)/100 {
		for _, f := range u.files {
			e.remember(ctx, f.src.Path, q.Policy, f.size, f.mtime, FileNotWorth)
		}
		return fail(FileNotWorth, fmt.Sprintf("This came out %s against %s: less than the %d%% saving you asked for, so the old file stays.", sizeWords(after), sizeWords(before), minSaving), "")
	}
	// Don't swap a file under someone who started watching it meanwhile.
	for e.isPlaying(u.item.ID) {
		t.Phase("Waiting: someone is playing a file that's ready to be replaced")
		select {
		case <-time.After(e.tick()):
		case <-ctx.Done():
			e.cleanTemps(context.WithoutCancel(ctx), u)
			return FilePending
		}
	}
	// Scans must not see the moment between the old file leaving and the new one arriving.
	t.Phase("Waiting for a library scan to finish")
	release, err := e.Scanner.ScanHold(ctx)
	if err != nil {
		e.cleanTemps(context.WithoutCancel(ctx), u)
		return FilePending
	}
	defer release()
	t.Phase("Putting the new file in place")
	if err := e.swapUnit(context.WithoutCancel(ctx), u, q, lib); err != nil {
		return fail(FileFailed, err.Error(), "")
	}
	done := 0
	for _, f := range u.files {
		if f.final != "" {
			done++
		}
	}
	edit(func(sum *Summary) {
		sum.Done += done
		sum.BytesAfter += after
	})
	return FileDone
}

// noRoom: the drive is running out, and a sentence says so.
type noRoom struct{ msg string }

func (n *noRoom) Error() string { return n.msg }

// prepare checks there's room, and names the temporary files.
func (e *Engine) prepare(ctx context.Context, f *part, output string, s Settings) error {
	dir := filepath.Dir(f.src.Path)
	floor := int64(s.MinFreeGB) << 30
	need := max(f.plan.EstBytes, f.size/4)
	if free := sysinfo.FreeBytes(dir); free >= 0 && free-need < floor {
		return &noRoom{fmt.Sprintf("Not enough free space: %s is free here, this file may need about %s, and you asked to keep %d GB free.", sizeWords(free), sizeWords(need), s.MinFreeGB)}
	}
	work := filepath.Join(dir, library.WorkFolder)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return fmt.Errorf("couldn't write in this file's folder, so nothing was changed. Check that the server may write there")
	}
	name, err := randomName()
	if err != nil {
		return err
	}
	f.temp = filepath.Join(work, name+"."+f.plan.Ext)
	f.sups = nil
	for i := range f.plan.exports {
		f.sups = append(f.sups, filepath.Join(work, fmt.Sprintf("%s-%d.sup", name, i)))
	}
	f.covers = nil
	for i, st := range f.plan.coverSteps() {
		out := filepath.Join(work, fmt.Sprintf("%s-cover%d.%s", name, i, coverExt(st.tag)))
		if _, _, err := ffmpeg.Run(ctx, e.Tools.FFmpeg, CoverArgs(f.src.Path, st, out), "job", "take a cover picture out of a file"); err != nil {
			return errors.New("the cover picture couldn't be read, so nothing was changed. This file may be damaged")
		}
		f.covers = append(f.covers, out)
	}
	return nil
}

func randomName() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// encode runs FFmpeg for one file, reports its progress, holds it still when asked, and
// stops it if the drive is running out.
func (e *Engine) encode(ctx context.Context, f *part, s Settings, t *tasks.Handle, prog func(*part, float64)) error {
	args := f.plan.Args(f.src, Output{Path: f.temp, Exports: f.sups, Covers: f.covers})
	jobCtx, stop := context.WithCancel(ctx)
	defer stop()
	cmd := ffmpeg.Command(jobCtx, e.Tools.FFmpeg, args...)
	dieWithServer(cmd)
	if s.LowPriority {
		lowPriority(cmd)
	}
	var stderr logging.Tail
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	started := time.Now()
	e.Log.InfoContext(ctx, "converting a file", "file", f.src.Path, "encoder", f.plan.Encoder, "args", ffmpeg.RedactArgs(args))
	if err := cmd.Start(); err != nil {
		return err
	}
	afterStart(cmd, s.LowPriority)
	var why atomic.Pointer[noRoom]
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		e.watch(jobCtx, cmd, f, t, s, stop, &why)
	}()
	readProgress(stdout, func(sec float64) { prog(f, sec) })
	err = cmd.Wait()
	stop()
	<-watcherDone
	logging.ChildExit(e.Log, "ffmpeg", err, errors.Is(ctx.Err(), context.Canceled), &stderr, started, "file", f.src.Path)
	// The watcher can look at the drive a moment after FFmpeg has finished: a file that came out
	// whole wasn't stopped, whatever it found.
	if nr := why.Load(); nr != nil && err != nil {
		return nr
	}
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// watch looks at a running FFmpeg's conditions a few times a minute: this only runs while
// a conversion does, which costs far more than a look at the clock.
func (e *Engine) watch(ctx context.Context, cmd *exec.Cmd, f *part, t *tasks.Handle, s Settings, stop func(), why *atomic.Pointer[noRoom]) {
	held := false
	var phased atomic.Bool
	tick := time.NewTicker(e.tick())
	defer tick.Stop()
	lastFree := time.Time{}
	for {
		select {
		case <-ctx.Done():
			if held {
				resume(cmd)
			}
			return
		case <-tick.C:
			if ctx.Err() != nil {
				continue // it has ended, and the case above sees to that
			}
		}
		s = settingsNow(ctx, e.DB, s)
		words := e.holdWords(s)
		paused := t.PauseWanted()
		switch {
		case (paused || words != "") && !held:
			if suspend(cmd) {
				held = true
			}
			if words != "" {
				t.Phase(words)
			}
		case !paused && words == "" && held:
			if resume(cmd) {
				held = false
				t.Phase("Converting")
			}
		case !paused && words == "" && !held:
			if phased.CompareAndSwap(false, true) {
				t.Phase("Converting")
			}
		}
		if paused {
			// The panel shows it as paused once this is called, and holds here.
			_ = t.Wait(ctx)
			continue
		}
		if time.Since(lastFree) >= 30*time.Second {
			lastFree = time.Now()
			floor := int64(s.MinFreeGB) << 30
			if free := sysinfo.FreeBytes(filepath.Dir(f.temp)); free >= 0 && free < floor {
				why.Store(&noRoom{fmt.Sprintf("Stopped: the drive got down to %s free and you asked to keep %d GB free. The old file is untouched.", sizeWords(free), s.MinFreeGB)})
				if held {
					resume(cmd)
				}
				stop()
				return
			}
		}
	}
}

// readProgress follows FFmpeg's progress lines.
func readProgress(r io.Reader, report func(sec float64)) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if ok && k == "out_time_us" {
			if us, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && us > 0 {
				report(float64(us) / 1e6)
			}
		}
	}
}

// swapUnit puts an item's new files in place. Each step is written down first, so that a
// crash at any moment can be finished or undone by Recover, and a failure part-way through
// an item puts everything back. Replacing works like keeping until the library has been
// updated: only then is the old file deleted, so there's always a way back.
func (e *Engine) swapUnit(ctx context.Context, u *unit, q RunRequest, lib *library.Library) error {
	var done []*part
	undo := func() {
		for _, f := range done {
			e.unswap(ctx, f, q.Output)
		}
	}
	first := ""
	for i, f := range u.files {
		if f.temp == "" {
			if i == 0 {
				first = f.src.Path
			}
			continue
		}
		if err := e.swapOne(ctx, f, q.Output, lib); err != nil {
			undo()
			e.cleanTemps(ctx, u)
			return err
		}
		done = append(done, f)
		if i == 0 {
			first = f.final
		}
	}
	// A copy beside the old file changes nothing about the item.
	if q.Output != OutputBeside {
		if err := e.Scanner.FileReplaced(ctx, u.item.ID, first); err != nil {
			undo()
			e.cleanTemps(ctx, u)
			return fmt.Errorf("the library couldn't be updated, so the old file was put back: %w", err)
		}
	}
	for _, f := range done {
		var after int64
		if st, err := os.Stat(f.final); err == nil {
			after = st.Size()
		}
		// A subtitle file made along the way, and an embedded one, go last, once all is safe.
		made := e.finishSidecars(ctx, f, q.Output)
		for _, c := range f.covers {
			_ = os.Remove(c)
		}
		f.covers = nil
		if q.Output == OutputReplace && f.kept != "" {
			_ = os.Remove(f.kept)
			_ = os.Remove(filepath.Dir(f.kept))
			f.kept = ""
		}
		e.updateFile(ctx, f.row, map[string]any{"state": FileDone, "message": "", "temp_path": "", "swap": "", "size_after": after, "final_path": f.final, "kept_path": f.kept, "finished_at": db.Now(), "sidecars": made.encode()})
	}
	removeEmpty(u)
	return nil
}

func removeEmpty(u *unit) {
	for _, f := range u.files {
		_ = os.Remove(filepath.Join(filepath.Dir(f.src.Path), library.WorkFolder))
	}
}

// finalName is where the new file ends up.
func finalName(f *part, output string) string {
	dir := filepath.Dir(f.src.Path)
	base := filepath.Base(f.src.Path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if output == OutputBeside {
		return filepath.Join(dir, stem+library.ConvertedInfix+f.plan.Ext)
	}
	if strings.EqualFold(filepath.Ext(base), "."+f.plan.Ext) {
		return f.src.Path
	}
	return filepath.Join(dir, stem+"."+f.plan.Ext)
}

func (e *Engine) swapOne(ctx context.Context, f *part, output string, lib *library.Library) error {
	f.final = finalName(f, output)
	// Paths were checked when planned. The names here come from the old file's own name in
	// its own folder, but it costs nothing to be certain.
	if filepath.Dir(f.final) != filepath.Dir(f.src.Path) || !insideRoots(f.final, lib.Paths) {
		return errors.New("the new file's name would leave the library, so nothing was changed")
	}
	if st, err := os.Stat(f.src.Path); err == nil {
		_ = os.Chmod(f.temp, st.Mode().Perm())
	}
	// A different film may already have the new name. That's never overwritten.
	if f.final != f.src.Path && output != OutputBeside {
		if _, err := os.Lstat(f.final); err == nil {
			return fmt.Errorf("a file called %s is already there, so nothing was changed", filepath.Base(f.final))
		}
	}
	if output == OutputBeside {
		e.updateFile(ctx, f.row, map[string]any{"swap": "beside", "temp_path": f.temp, "final_path": f.final})
		if err := os.Rename(f.temp, f.final); err != nil {
			e.updateFile(ctx, f.row, map[string]any{"swap": ""})
			return errors.New("the new file couldn't be moved into place. Nothing was changed")
		}
		return nil
	}
	folder := filepath.Join(filepath.Dir(f.src.Path), library.OriginalsFolder)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return errors.New("couldn't make a folder for the original beside the file, so nothing was changed")
	}
	kept := uniqueName(filepath.Join(folder, filepath.Base(f.src.Path)))
	f.kept = kept
	e.updateFile(ctx, f.row, map[string]any{"swap": "keep", "temp_path": f.temp, "final_path": f.final, "kept_path": kept})
	if err := os.Rename(f.src.Path, kept); err != nil {
		e.updateFile(ctx, f.row, map[string]any{"swap": "", "kept_path": ""})
		f.kept = ""
		return errors.New("the old file couldn't be moved aside. It may be in use. Nothing was changed")
	}
	if err := os.Rename(f.temp, f.final); err != nil {
		_ = os.Rename(kept, f.src.Path)
		e.updateFile(ctx, f.row, map[string]any{"swap": "", "kept_path": ""})
		f.kept = ""
		return errors.New("the new file couldn't be moved into place, so the old one was put back")
	}
	return nil
}

func insideRoots(p string, roots []string) bool {
	for _, r := range roots {
		if rel, err := filepath.Rel(r, p); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func uniqueName(p string) string {
	if _, err := os.Lstat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 2; i < 1000; i++ {
		c := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		if _, err := os.Lstat(c); err != nil {
			return c
		}
	}
	return p
}

// unswap puts a part back, after another part of its item failed.
func (e *Engine) unswap(ctx context.Context, f *part, output string) {
	_ = os.Remove(f.final)
	if output != OutputBeside && f.kept != "" {
		_ = os.Rename(f.kept, f.src.Path)
		_ = os.Remove(filepath.Dir(f.kept))
		f.kept = ""
	}
	e.updateFile(ctx, f.row, map[string]any{"swap": "", "kept_path": "", "final_path": ""})
}

// sidecarRecord says what a conversion did to the files around a video, for undoing it.
type sidecarRecord struct {
	Made  []string    `json:"made,omitempty"`  // subtitle files it wrote
	Moved [][2]string `json:"moved,omitempty"` // subtitle files it moved aside: from, to
}

func (r sidecarRecord) encode() string {
	b, err := json.Marshal(r)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// finishSidecars puts exported subtitle files beside the video, and looks after the files
// that were embedded. It says what it did.
func (e *Engine) finishSidecars(ctx context.Context, f *part, output string) sidecarRecord {
	var rec sidecarRecord
	base := filepath.Base(f.final)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	dir := filepath.Dir(f.final)
	for i, sup := range f.sups {
		if i < len(f.plan.exports) {
			name := uniqueName(filepath.Join(dir, stem+"."+f.plan.exports[i].path+".sup"))
			if err := os.Rename(sup, name); err != nil {
				e.Log.WarnContext(ctx, "could not put a subtitle file beside its video", "file", name, "err", err)
			} else {
				rec.Made = append(rec.Made, name)
			}
		}
	}
	f.sups = nil
	if output == OutputBeside {
		return rec
	}
	for _, em := range f.plan.embeds {
		if output == OutputReplace {
			_ = os.Remove(em.path)
			continue
		}
		folder := filepath.Join(dir, library.OriginalsFolder)
		to := uniqueName(filepath.Join(folder, filepath.Base(em.path)))
		if os.Rename(em.path, to) == nil {
			rec.Moved = append(rec.Moved, [2]string{em.path, to})
		}
	}
	return rec
}

// undoSidecars puts the files around a video back as they were. Everything it touches is
// named in the record the conversion wrote, and it stays inside the video's own folder.
func (e *Engine) undoSidecars(ctx context.Context, row *FileRow) {
	var raw string
	if err := e.DB.QueryRowContext(ctx, `SELECT sidecars FROM ltc_files WHERE id = ?`, row.ID).Scan(&raw); err != nil {
		return
	}
	var rec sidecarRecord
	if json.Unmarshal([]byte(raw), &rec) != nil {
		return
	}
	dir := filepath.Dir(row.Path)
	for _, made := range rec.Made {
		if filepath.Dir(made) == dir {
			_ = os.Remove(made)
		}
	}
	for _, mv := range rec.Moved {
		if filepath.Dir(mv[0]) == dir && isOriginalsPath(mv[1]) {
			_ = os.Rename(mv[1], mv[0])
		}
	}
}

// cleanTemps removes an item's temporary files, however far it got.
func (e *Engine) cleanTemps(ctx context.Context, u *unit) {
	for _, f := range u.files {
		if f.temp != "" {
			_ = os.Remove(f.temp)
			f.temp = ""
		}
		for _, s := range f.sups {
			_ = os.Remove(s)
		}
		for _, s := range f.covers {
			_ = os.Remove(s)
		}
		f.sups, f.covers = nil, nil
	}
	removeEmpty(u)
}

// Retry starts a run again, for the files that didn't get done. It works from what was
// recorded, so it works after a restart too.
func (e *Engine) Retry(ctx context.Context, run int64, by tasks.By) (int64, error) {
	e.init()
	old, err := e.GetRun(ctx, run)
	if err != nil {
		return 0, err
	}
	q := RunRequest{Library: old.LibraryID, Policy: old.Policy, Output: old.Output, Dry: old.Dry, By: by}
	if !old.Dry {
		rows, err := e.DB.QueryContext(ctx, `SELECT DISTINCT item_id FROM ltc_files WHERE run_id = ? AND state IN ('pending', 'running', 'failed', 'postponed', 'changed')`, run)
		if err != nil {
			return 0, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return 0, err
			}
			q.Items = append(q.Items, id)
		}
		if err := rows.Err(); err != nil {
			return 0, err
		}
		if len(q.Items) == 0 {
			return 0, errors.New("every file of that conversion is done")
		}
	}
	return e.Start(ctx, q)
}

// Cancel stops a run.
func (e *Engine) Cancel(run int64) bool {
	e.init()
	e.mu.Lock()
	defer e.mu.Unlock()
	if c, ok := e.live[run]; ok {
		c()
		return true
	}
	return false
}

// Recover tidies up after a server that stopped during a conversion: temporary files go,
// and a swap caught half-way is finished or undone, whichever keeps the file safe.
func (e *Engine) Recover(ctx context.Context) {
	e.init()
	rows, err := e.DB.QueryContext(ctx, `SELECT f.id, f.path, f.temp_path, f.final_path, f.kept_path, f.swap, f.item_id FROM ltc_files f WHERE f.temp_path <> '' OR f.swap <> ''`)
	if err != nil {
		e.Log.WarnContext(ctx, "could not look for conversions cut short", "err", err)
		return
	}
	type row struct {
		id                            int64
		path, temp, final, kept, swap string
		item                          int64
	}
	var list []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.path, &r.temp, &r.final, &r.kept, &r.swap, &r.item) == nil {
			list = append(list, r)
		}
	}
	rows.Close()
	for _, r := range list {
		e.recoverOne(ctx, r.id, r.path, r.temp, r.final, r.kept, r.swap, r.item)
	}
	_, _ = e.DB.ExecContext(ctx, `UPDATE ltc_files SET state = 'pending', message = 'The server stopped before this was done. The old file is untouched.' WHERE state = 'running'`)
	_, _ = e.DB.ExecContext(ctx, `UPDATE ltc_runs SET state = 'interrupted', finished_at = ? WHERE state IN ('queued', 'running')`, db.Now())
}

func (e *Engine) recoverOne(ctx context.Context, id int64, path, temp, final, kept, swap string, item int64) {
	exists := func(p string) bool {
		if p == "" {
			return false
		}
		_, err := os.Lstat(p)
		return err == nil
	}
	ours := strings.Contains(filepath.ToSlash(temp), "/"+library.WorkFolder+"/")
	dropTemp := func() {
		if ours {
			_ = os.Remove(temp)
			_ = os.Remove(filepath.Dir(temp))
		}
	}
	finished := func() {
		e.updateFile(ctx, id, map[string]any{"state": FileDone, "swap": "", "temp_path": "", "message": "", "finished_at": db.Now()})
	}
	switch swap {
	case "keep":
		switch {
		case exists(kept) && !exists(path) && exists(temp):
			// Moved aside, not yet replaced. The original goes back.
			if err := os.Rename(kept, path); err != nil {
				e.Log.WarnContext(ctx, "could not put back a file a conversion had moved aside", "file", path, "kept", kept, "err", err)
				return
			}
		case exists(kept) && exists(final) && !exists(temp):
			// All moved. Only the library hadn't heard.
			if e.Scanner != nil {
				if err := e.Scanner.FileReplaced(ctx, item, final); err != nil {
					e.Log.WarnContext(ctx, "could not finish a conversion cut short", "file", final, "err", err)
				}
			}
			finished()
			return
		}
		dropTemp()
		e.updateFile(ctx, id, map[string]any{"swap": "", "temp_path": "", "kept_path": "", "final_path": ""})
	case "beside":
		if !exists(temp) && exists(final) {
			finished()
			return
		}
		dropTemp()
		e.updateFile(ctx, id, map[string]any{"swap": "", "temp_path": "", "final_path": ""})
	default:
		dropTemp()
		e.updateFile(ctx, id, map[string]any{"temp_path": ""})
	}
}

func sizeWords(n int64) string {
	switch {
	case n >= 1<<40:
		return fmt.Sprintf("%.1f TB", float64(n)/(1<<40))
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
	}
	return fmt.Sprintf("%d KB", n/1024)
}

func summarize(dry bool, s Summary) string {
	if dry {
		return fmt.Sprintf("Looked at %d files: %d to convert, %d to tidy, %d already fine, %d left alone. Nothing was changed.", s.Files, s.Convert, s.Remux, s.Skip, s.Refuse)
	}
	var parts []string
	parts = append(parts, fmt.Sprintf("Converted %d of %s", s.Done, countFiles(s.Convert+s.Remux)))
	if s.BytesAfter > 0 {
		parts = append(parts, "now taking "+sizeWords(s.BytesAfter))
	}
	if s.NotWorth > 0 {
		parts = append(parts, fmt.Sprintf("%d came out no smaller and stayed as they were", s.NotWorth))
	}
	if s.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed with their old files untouched", s.Failed))
	}
	if s.Postponed > 0 {
		parts = append(parts, fmt.Sprintf("%d were being played and were left for next time", s.Postponed))
	}
	return strings.Join(parts, ", ") + "."
}

// Preview is what a chosen file would become.
type Preview struct {
	ItemID int64         `json:"itemId"`
	Title  string        `json:"title"`
	Parts  []PreviewPart `json:"parts"`
}

// PreviewPart is one file of it.
type PreviewPart struct {
	Path     string  `json:"path"`
	Size     int64   `json:"size"`
	Duration float64 `json:"duration"`
	Plan     *Plan   `json:"plan"`
}

// PreviewItem plans one item, reading only.
func (e *Engine) PreviewItem(ctx context.Context, lib *library.Library, id int64, p Policy) (*Preview, error) {
	e.init()
	items, err := e.Store.ConversionItems(ctx, lib.ID)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.ID != id {
			continue
		}
		u, err := e.planItem(ctx, lib, it, p, e.Env(ctx))
		if err != nil {
			return nil, err
		}
		out := &Preview{ItemID: it.ID, Title: it.Title}
		for _, f := range u.files {
			out.Parts = append(out.Parts, PreviewPart{Path: f.src.Path, Size: f.size, Duration: f.src.Duration, Plan: f.plan})
		}
		return out, nil
	}
	return nil, library.ErrNotFound
}

// SampleResult is what a short test conversion measured.
type SampleResult struct {
	Seconds      float64 `json:"seconds"`      // how much of the film was tried
	Took         float64 `json:"took"`         // how long that took here
	Speed        float64 `json:"speed"`        // times real time
	Bytes        int64   `json:"bytes"`        // what that much came to
	EstFileBytes int64   `json:"estFileBytes"` // the whole film at that rate
	EstFileSec   float64 `json:"estFileSec"`
	Note         string  `json:"note,omitempty"`
}

const sampleSeconds = 20.0

// Sample really converts a short stretch from the middle of a file, into the server's own
// cache folder, and measures it. One at a time: it's real work for the processor.
func (e *Engine) Sample(ctx context.Context, lib *library.Library, id int64, p Policy, cacheDir string) (*SampleResult, error) {
	e.init()
	items, err := e.Store.ConversionItems(ctx, lib.ID)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.ID != id {
			continue
		}
		u, err := e.planItem(ctx, lib, it, p, e.Env(ctx))
		if err != nil {
			return nil, err
		}
		f := u.files[0]
		if f.plan.Verdict != VerdictConvert && f.plan.Verdict != VerdictRemux {
			return nil, errors.New(f.plan.Reason)
		}
		return e.sampleOne(ctx, f, cacheDir)
	}
	return nil, library.ErrNotFound
}

func (e *Engine) sampleOne(ctx context.Context, f *part, cacheDir string) (*SampleResult, error) {
	select {
	case e.slot <- struct{}{}:
		defer func() { <-e.slot }()
	default:
		return nil, errors.New("a conversion is running. Try this again when it has finished")
	}
	dir := filepath.Join(cacheDir, "convert-samples")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name, err := randomName()
	if err != nil {
		return nil, err
	}
	out := filepath.Join(dir, name+"."+f.plan.Ext)
	defer os.Remove(out)
	length := min(sampleSeconds, f.src.Duration)
	start := max(f.src.Duration/2-length/2, 0)
	var covers []string
	for i, st := range f.plan.coverSteps() {
		pic := filepath.Join(dir, fmt.Sprintf("%s-cover%d.%s", name, i, coverExt(st.tag)))
		if _, _, err := ffmpeg.Run(ctx, e.Tools.FFmpeg, CoverArgs(f.src.Path, st, pic), "job", "take a cover picture out of a file"); err == nil {
			covers = append(covers, pic)
			defer os.Remove(pic)
		}
	}
	args := f.plan.Args(f.src, Output{Path: out, Covers: covers})
	args = withRange(args, start, length)
	cctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	began := time.Now()
	cmd := ffmpeg.Command(cctx, e.Tools.FFmpeg, args...)
	dieWithServer(cmd)
	lowPriority(cmd)
	var stderr logging.Tail
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	afterStart(cmd, true)
	if err := cmd.Wait(); err != nil {
		e.Log.WarnContext(ctx, "a test conversion failed", "file", f.src.Path, "err", err, "said", stderr.String())
		return nil, errors.New("the test conversion didn't work. This file may be damaged, and the log has the details")
	}
	took := time.Since(began).Seconds()
	st, err := os.Stat(out)
	if err != nil {
		return nil, err
	}
	r := &SampleResult{Seconds: length, Took: took, Bytes: st.Size()}
	if took > 0 {
		r.Speed = length / took
	}
	if length > 0 {
		r.EstFileBytes = int64(float64(st.Size()) / length * f.src.Duration)
		r.EstFileSec = took / length * f.src.Duration
	}
	r.Note = "Measured on 20 seconds from the middle of this file. Busier scenes take more."
	return r, nil
}

// withRange limits a command to a stretch of the film.
func withRange(args []string, start, length float64) []string {
	out := make([]string, 0, len(args)+6)
	for i, a := range args {
		if a == "-i" && (i == 0 || args[i-1] != "-map") && !slices.Contains(out, "-ss") {
			out = append(out, "-ss", strconv.FormatFloat(start, 'f', 3, 64), "-t", strconv.FormatFloat(length, 'f', 3, 64))
		}
		out = append(out, a)
	}
	return out
}

// ErrInUse: someone is playing the item.
var ErrInUse = errors.New("someone is playing this right now. Try again when they've finished")

// Undo puts the originals of an item back, in place of what a run made of them.
func (e *Engine) Undo(ctx context.Context, fileID int64) error {
	e.init()
	f, err := e.GetFile(ctx, fileID)
	if err != nil {
		return err
	}
	if e.isPlaying(f.ItemID) {
		return ErrInUse
	}
	rows, err := e.itemRows(ctx, f)
	if err != nil {
		return err
	}
	release, err := e.Scanner.ScanHold(ctx)
	if err != nil {
		return err
	}
	defer release()
	first := ""
	for _, r := range rows {
		if r.KeptPath == "" || !isOriginalsPath(r.KeptPath) {
			continue
		}
		if _, err := os.Stat(r.KeptPath); err != nil {
			return errors.New("the kept original isn't there any more, so this can't be undone")
		}
		if r.FinalPath != "" && r.FinalPath != r.Path {
			if _, err := os.Lstat(r.Path); err == nil {
				return fmt.Errorf("a file called %s is already there, so this can't be undone", filepath.Base(r.Path))
			}
		}
		if r.FinalPath != "" {
			_ = os.Remove(r.FinalPath)
		}
		if err := os.Rename(r.KeptPath, r.Path); err != nil {
			return errors.New("the original couldn't be moved back. It may be in use")
		}
		e.undoSidecars(ctx, r)
		_ = os.Remove(filepath.Dir(r.KeptPath))
		if r.Part == 0 {
			first = r.Path
		}
		e.updateFile(ctx, r.ID, map[string]any{"state": FileUndone, "kept_path": "", "final_path": "", "size_after": 0, "sidecars": "{}"})
	}
	if first == "" {
		return errors.New("there's no kept original to put back")
	}
	if err := e.Scanner.FileReplaced(ctx, f.ItemID, first); err != nil {
		return err
	}
	// Someone wanted this file as it was, so automatic conversion doesn't undo the undoing.
	if it, err := e.Store.Item(ctx, f.ItemID); err == nil {
		if st, err := os.Stat(first); err == nil {
			e.remember(ctx, first, LoadRule(ctx, e.DB, it.LibraryID).Policy, st.Size(), st.ModTime().Unix(), FileUndone)
		}
	}
	return nil
}

// DeleteLeftover deletes a kept original, or an unused converted copy, for good. It only
// ever touches a file in the hidden originals folder or one named as a converted copy.
func (e *Engine) DeleteLeftover(ctx context.Context, fileID int64) error {
	e.init()
	f, err := e.GetFile(ctx, fileID)
	if err != nil {
		return err
	}
	rows, err := e.itemRows(ctx, f)
	if err != nil {
		return err
	}
	removed := false
	for _, r := range rows {
		switch {
		case r.KeptPath != "" && isOriginalsPath(r.KeptPath):
			if err := os.Remove(r.KeptPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("that file couldn't be deleted. It may be in use")
			}
			_ = os.Remove(filepath.Dir(r.KeptPath))
			e.updateFile(ctx, r.ID, map[string]any{"kept_path": ""})
			removed = true
		case r.KeptPath == "" && r.FinalPath != "" && library.IsConvertedCopy(filepath.Base(r.FinalPath)):
			if err := os.Remove(r.FinalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("that file couldn't be deleted. It may be in use")
			}
			e.updateFile(ctx, r.ID, map[string]any{"final_path": ""})
			removed = true
		}
	}
	if !removed {
		return errors.New("there's nothing left over from this conversion")
	}
	return nil
}

// UseCopy puts a converted copy written beside its original into use, the original moving
// to the hidden folder as it would have if the run had kept it.
func (e *Engine) UseCopy(ctx context.Context, fileID int64) error {
	e.init()
	f, err := e.GetFile(ctx, fileID)
	if err != nil {
		return err
	}
	if e.isPlaying(f.ItemID) {
		return ErrInUse
	}
	rows, err := e.itemRows(ctx, f)
	if err != nil {
		return err
	}
	release, err := e.Scanner.ScanHold(ctx)
	if err != nil {
		return err
	}
	defer release()
	first := ""
	for _, r := range rows {
		if r.KeptPath != "" || r.FinalPath == "" || !library.IsConvertedCopy(filepath.Base(r.FinalPath)) {
			continue
		}
		dir := filepath.Dir(r.Path)
		base := filepath.Base(r.FinalPath)
		i := strings.Index(strings.ToLower(base), library.ConvertedInfix)
		target := filepath.Join(dir, base[:i]+"."+base[i+len(library.ConvertedInfix):])
		if target != r.Path {
			if _, err := os.Lstat(target); err == nil {
				return fmt.Errorf("a file called %s is already there, so this copy can't take its place", filepath.Base(target))
			}
		}
		folder := filepath.Join(dir, library.OriginalsFolder)
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return errors.New("couldn't make a folder for the original beside the file")
		}
		kept := uniqueName(filepath.Join(folder, filepath.Base(r.Path)))
		if err := os.Rename(r.Path, kept); err != nil {
			return errors.New("the old file couldn't be moved aside. It may be in use")
		}
		if err := os.Rename(r.FinalPath, target); err != nil {
			_ = os.Rename(kept, r.Path)
			return errors.New("the copy couldn't be moved into place, so the old file was put back")
		}
		if r.Part == 0 {
			first = target
		}
		e.updateFile(ctx, r.ID, map[string]any{"kept_path": kept, "final_path": target})
	}
	if first == "" {
		return errors.New("there's no converted copy to use")
	}
	return e.Scanner.FileReplaced(ctx, f.ItemID, first)
}

// itemRows finds the rows of all an item's parts in the same run.
func (e *Engine) itemRows(ctx context.Context, f *FileRow) ([]*FileRow, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT `+fileCols+` FROM ltc_files WHERE run_id = ? AND item_id = ? AND state = 'done' ORDER BY part`, f.RunID, f.ItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*FileRow{}
	for rows.Next() {
		r, err := scanFile(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, errors.New("this conversion has nothing to act on")
	}
	return out, rows.Err()
}

func isOriginalsPath(p string) bool {
	return filepath.Base(filepath.Dir(p)) == library.OriginalsFolder
}
