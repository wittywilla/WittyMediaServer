package diagnose

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittydisc/isofs"
	"github.com/wittywilla/wittydvd"
)

// Every test makes its own small fixtures. Nothing here reads a real disc.

const contentMark = "CONTENT-THAT-MUST-STAY-ON-THE-DISC"

func testJob() *Job {
	return &Job{ID: "diag-test", env: &Env{}, cat: "discs", log: slog.New(slog.NewTextHandler(io.Discard, nil)), report: Report{Counts: map[Severity]int{}, Findings: []Finding{}}}
}

func hasFinding(j *Job, sev Severity, title string) bool {
	for _, f := range j.report.Findings {
		if f.Severity == sev && strings.Contains(f.Title, title) {
			return true
		}
	}
	return false
}

func listFindings(j *Job) string {
	var b strings.Builder
	for _, f := range j.report.Findings {
		b.WriteString(string(f.Severity) + ": " + f.Title + " / " + f.What + " " + strings.Join(f.Details, " | ") + "\n")
	}
	return b.String()
}

func realDir(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// ---------------------------------------------------------------- where it may look

func TestDiagnoseWithin(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "media")
	if runtime.GOOS == "windows" {
		base = `D:\media`
	}
	a, ab := filepath.Join(base, "a"), filepath.Join(base, "ab")
	cases := []struct {
		root, target string
		want         bool
	}{
		{a, a, true},
		{a, filepath.Join(a, "disc.iso"), true},
		{a, filepath.Join(a, "x", "y", "z"), true},
		{a, ab, false}, // not just a matching start
		{a, filepath.Join(ab, "disc.iso"), false},
		{a, base, false},
		{a, filepath.Join(a, "..", "b"), false},
		{"", a, false},
		{a, "", false},
	}
	for _, c := range cases {
		if got := Within(c.root, filepath.Clean(c.target)); got != c.want {
			t.Errorf("Within(%q, %q) = %v", c.root, c.target, got)
		}
	}
	if runtime.GOOS == "windows" {
		if !Within(`D:\Media\A`, `d:\media\a\disc.iso`) {
			t.Error("case should not matter on Windows")
		}
		if Within(`D:\media`, `E:\media\a`) {
			t.Error("another drive is outside")
		}
		if Within(`\\nas\share\media`, `\\nas\share\mediaplus\x`) {
			t.Error("a sibling on a share is outside")
		}
	}
}

func TestDiagnoseResolveRefuses(t *testing.T) {
	top := realDir(t)
	lib := filepath.Join(top, "a")
	sibling := filepath.Join(top, "ab")
	outside := filepath.Join(top, "secret")
	for _, d := range []string{lib, sibling, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	disc := filepath.Join(lib, "disc.iso")
	secret := filepath.Join(outside, "other.iso")
	for _, f := range []string{disc, secret, filepath.Join(sibling, "x.iso")} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRoots([]string{lib})

	if got, err := r.Resolve(disc); err != nil || got != disc {
		t.Fatalf("a disc in the library: %q, %v", got, err)
	}
	if got, err := r.Resolve(lib); err != nil || got != lib {
		t.Fatalf("the library itself: %q, %v", got, err)
	}
	if _, err := r.Resolve(filepath.Join(lib, "missing.iso")); !errors.Is(err, ErrMissing) {
		t.Fatalf("missing inside: %v", err)
	}

	refused := []string{
		"",
		"disc.iso",
		secret,
		outside,
		top,
		filepath.Join(sibling, "x.iso"), // /media/a must not match /media/ab
		lib + string(filepath.Separator) + ".." + string(filepath.Separator) + "secret" + string(filepath.Separator) + "other.iso",
		lib + string(filepath.Separator) + ".." + string(filepath.Separator) + "ab",
		disc + "\x00.iso",
		"\x00",
		filepath.Join(top, "nothing-here"), // no telling what exists elsewhere
	}
	if runtime.GOOS == "windows" {
		refused = append(refused,
			`\\?\`+disc,
			`//?/`+disc,
			`\\.\`+disc,
			`\\.\PhysicalDrive0`,
			`\??\`+disc,
			disc+`:stream`,
			disc+`::$DATA`,
			disc[:2]+`disc.iso`, // relative to a drive
			`\\localhost\c$\Windows`,
			`\\127.0.0.1\c$\Windows`,
			`\\evil\share\x.iso`,
		)
	} else {
		refused = append(refused, "/etc/passwd", "../../etc/passwd")
	}
	for _, p := range refused {
		if got, err := r.Resolve(p); !errors.Is(err, ErrOutside) {
			t.Errorf("Resolve(%q) = %q, %v; want it refused as outside", p, got, err)
		}
	}
	if runtime.GOOS == "windows" {
		if got, err := r.Resolve(strings.ToUpper(disc)); err != nil || !strings.EqualFold(got, disc) {
			t.Errorf("case should not matter on Windows: %q, %v", got, err)
		}
	}
	for _, msg := range []string{ErrOutside.Error(), ErrMissing.Error(), ErrLinkOut.Error()} {
		if strings.Contains(msg, top) {
			t.Errorf("a refusal gives a path away: %s", msg)
		}
	}
}

func TestDiagnoseResolveLinks(t *testing.T) {
	top := realDir(t)
	lib, outside := filepath.Join(top, "lib"), filepath.Join(top, "outside")
	inner := filepath.Join(lib, "inner")
	for _, d := range []string{inner, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	secret := filepath.Join(outside, "secret.iso")
	if err := os.WriteFile(secret, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "fine.iso"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewRoots([]string{lib})
	out := filepath.Join(lib, "out.iso")
	if err := os.Symlink(secret, out); err != nil {
		t.Skip("this system won't let a test make links:", err)
	}
	if _, err := r.Resolve(out); !errors.Is(err, ErrOutside) {
		t.Errorf("a link to a file outside: %v", err)
	}
	outDir := filepath.Join(lib, "outdir")
	if err := os.Symlink(outside, outDir); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve(filepath.Join(outDir, "secret.iso")); !errors.Is(err, ErrOutside) {
		t.Errorf("through a linked folder: %v", err)
	}
	// A link that stays inside is fine, and comes back as the real place.
	in := filepath.Join(lib, "in.iso")
	if err := os.Symlink(filepath.Join(inner, "fine.iso"), in); err != nil {
		t.Fatal(err)
	}
	if got, err := r.Resolve(in); err != nil || got != filepath.Join(inner, "fine.iso") {
		t.Errorf("a link inside: %q, %v", got, err)
	}
	// The root can be a link itself.
	rootLink := filepath.Join(top, "rootlink")
	if err := os.Symlink(lib, rootLink); err != nil {
		t.Fatal(err)
	}
	if got, err := NewRoots([]string{rootLink}).Resolve(filepath.Join(rootLink, "inner", "fine.iso")); err != nil || got != filepath.Join(inner, "fine.iso") {
		t.Errorf("a linked root: %q, %v", got, err)
	}
	// A disc folder with a link out inside it.
	if err := os.Symlink(secret, filepath.Join(inner, "link.iso")); err != nil {
		t.Fatal(err)
	}
	if err := r.CheckInnerLinks(inner); !errors.Is(err, ErrLinkOut) {
		t.Errorf("a link out inside a disc folder: %v", err)
	}
}

// ---------------------------------------------------------------- jobs

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func viewState(t *testing.T, m *Manager, id string) string {
	t.Helper()
	v, ok := m.JobView(id)
	if !ok {
		t.Fatalf("job %s has gone", id)
	}
	return v.State
}

func TestDiagnoseQueueOneAtATimeAndBounded(t *testing.T) {
	m := NewManager(Env{DataDir: t.TempDir(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	var running, most, ran atomic.Int32
	release := make(chan struct{})
	job := func(ctx context.Context, j *Job) {
		n := running.Add(1)
		for {
			old := most.Load()
			if n <= old || most.CompareAndSwap(old, n) {
				break
			}
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		running.Add(-1)
		ran.Add(1)
	}
	first, err := m.Submit(KindDisc, "first", nil, nil, job)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the first job to start", func() bool { return viewState(t, m, first.ID) == StateRunning })
	var queued []*Job
	for i := range maxWaiting {
		j, err := m.Submit(KindDisc, "next", nil, nil, job)
		if err != nil {
			t.Fatalf("job %d of the queue: %v", i, err)
		}
		queued = append(queued, j)
	}
	if _, err := m.Submit(KindDisc, "too many", nil, nil, job); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("past the limit: %v", err)
	}
	if v, _ := m.JobView(queued[0].ID); v.Position != 2 {
		t.Errorf("the first waiting job's position: %d", v.Position)
	}

	// Cancelling a job that's waiting: it never runs, and makes room.
	if _, ok := m.Cancel(queued[3].ID); !ok {
		t.Fatal("cancel a queued job")
	}
	if got := viewState(t, m, queued[3].ID); got != StateCancelled {
		t.Errorf("cancelled while waiting: %s", got)
	}
	if _, err := m.Submit(KindDisc, "after", nil, nil, job); err != nil {
		t.Fatalf("a cancelled job should make room: %v", err)
	}

	// Cancelling the running job ends it, and the next begins.
	if _, ok := m.Cancel(first.ID); !ok {
		t.Fatal("cancel the running job")
	}
	waitFor(t, "the cancelled job to end", func() bool { return viewState(t, m, first.ID) == StateCancelled })
	close(release)
	waitFor(t, "the rest to run", func() bool { return ran.Load() == maxWaiting+1 })
	if most.Load() != 1 {
		t.Errorf("%d jobs ran at once", most.Load())
	}
	if _, ok := m.Cancel("diag-nothing"); ok {
		t.Error("cancelling nothing")
	}
}

func TestDiagnoseReportsAreKeptAndBounded(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(Env{DataDir: dir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	var last *Job
	for range keepReports + 3 {
		var err error
		for {
			last, err = m.Submit(KindDisc, "a.iso", nil, nil, func(ctx context.Context, j *Job) {
				j.Add(Finding{Severity: SevWarn, Title: "something"})
			})
			if err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	waitFor(t, "the last report", func() bool { return last.isSaved() })
	waitFor(t, "the worker to finish", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return !m.working
	})
	files, err := filepath.Glob(filepath.Join(dir, "diagnostics", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != keepReports {
		t.Errorf("%d reports kept", len(files))
	}
	if r, ok := m.JobView(last.ID); !ok || r.Worst != SevWarn {
		t.Errorf("reading a report back: %+v", r)
	}
	// An ID from a request can't name a file elsewhere.
	for _, id := range []string{"../secret", `..\secret`, "diag-../../x", "diag-a/b", "secret"} {
		if _, err := loadReport(dir, id); err == nil {
			t.Errorf("loadReport(%q) worked", id)
		}
	}
	b := m.BundleFiles(2)
	if len(b) != 2 || !strings.HasPrefix(b[0].Name, "diagnostics/diag-") {
		t.Errorf("bundle: %+v", b)
	}
}

func TestDiagnoseRestsForPlayback(t *testing.T) {
	j := testJob()
	j.env.Busy = func() bool { return true }
	start := time.Now()
	if !j.Rest(context.Background(), 40*time.Millisecond) {
		t.Fatal("not cancelled")
	}
	if d := time.Since(start); d < 35*time.Millisecond {
		t.Errorf("rested only %v", d)
	}
	j.env.Busy = func() bool { return false }
	start = time.Now()
	j.Rest(context.Background(), time.Hour)
	if time.Since(start) > time.Second {
		t.Error("rested with nobody watching")
	}
	j.env.Busy = func() bool { return true }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if j.Rest(ctx, time.Hour) {
		t.Error("cancelled, but carried on")
	}
}

func TestDiagnoseEnginePanicBecomesFinding(t *testing.T) {
	j := testJob()
	ok := j.Engine("reading this disc", func() {
		var m map[string]int
		m["x"] = 1 // a crash, as an engine might have on a strange disc
	})
	if ok {
		t.Fatal("the crash wasn't noticed")
	}
	if !hasFinding(j, SevProblem, "engine crashed") {
		t.Fatal(listFindings(j))
	}
	// And the whole job survives one, with what it had so far saved.
	dir := t.TempDir()
	m := NewManager(Env{DataDir: dir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	job, err := m.Submit(KindDisc, "a.iso", nil, nil, func(ctx context.Context, j *Job) {
		j.Add(Finding{Severity: SevOK, Title: "before the crash"})
		panic("boom")
	})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the job to end", func() bool { return job.isSaved() })
	v, _ := m.JobView(job.ID)
	if v.State != StateDone || v.Worst != SevProblem || v.Counts[SevOK] != 1 {
		t.Errorf("%+v", v)
	}
}

// ---------------------------------------------------------------- scrambling

// sampleDisc is a VOB space in memory: a run of packs, some flagged scrambled.
type sampleDisc struct {
	sectors   int
	scrambled func(sector int) bool
}

func videoPack(scrambled bool) []byte {
	p := make([]byte, wittydvd.Sector)
	binary.BigEndian.PutUint32(p, 0x000001ba)
	p[4] = 0x44
	p[13] = 0 // no stuffing
	copy(p[14:], []byte{0, 0, 1, 0xe0, 0x07, 0xec, 0x80, 0})
	if scrambled {
		p[20] = 0x80 | 0x30 // the flag's two bits
	}
	copy(p[64:], contentMark)
	return p
}

func navPack() []byte {
	p := make([]byte, wittydvd.Sector)
	binary.BigEndian.PutUint32(p, 0x000001ba)
	p[4] = 0x44
	copy(p[14:], []byte{0, 0, 1, 0xbf, 0x03, 0xd4})
	return p
}

func (d sampleDisc) ReadSectors(first uint32, count int) ([]byte, error) {
	out := []byte{}
	for s := int(first); s < int(first)+count && s < d.sectors; s++ {
		if s%16 == 0 {
			out = append(out, navPack()...) // never scrambled, never counted
			continue
		}
		out = append(out, videoPack(d.scrambled(s))...)
	}
	return out, nil
}

func TestDiagnoseScrambledPacksAreDetected(t *testing.T) {
	if !wittydvd.PackScrambled(videoPack(true)) || wittydvd.PackScrambled(videoPack(false)) {
		t.Fatal("the fixture doesn't match the engine's idea of a scrambled pack")
	}
	if !packHoldsStream(videoPack(false)) || packHoldsStream(navPack()) {
		t.Fatal("telling packs apart")
	}
	const n = 16000
	all := []packRange{{0, n - 1}}
	j := testJob()
	ctx := context.Background()

	clear := samplePacks(ctx, j, sampleDisc{n, func(int) bool { return false }}, all, 32)
	if clear.total == 0 || clear.scrambled != 0 {
		t.Errorf("clear: %+v", clear)
	}
	whole := samplePacks(ctx, j, sampleDisc{n, func(int) bool { return true }}, all, 32)
	if whole.total == 0 || whole.scrambled != whole.total {
		t.Errorf("all scrambled: %+v", whole)
	}
	// Scrambled only from halfway on: a sample of the start alone would miss it.
	half := samplePacks(ctx, j, sampleDisc{n, func(s int) bool { return s >= n/2 }}, all, 32)
	if f := float64(half.scrambled) / float64(half.total); f < 0.4 || f > 0.6 {
		t.Errorf("half scrambled gave %+v", half)
	}
	// Past the end reads as nothing, and doesn't count.
	if got := samplePacks(ctx, j, sampleDisc{n, func(int) bool { return true }}, []packRange{{n + 5, n + 500}}, 8); got.total != 0 {
		t.Errorf("past the end: %+v", got)
	}
	if !strings.Contains(half.String(), "%") {
		t.Errorf("wording: %s", half)
	}
}

// ---------------------------------------------------------------- damage

// isoFile is a file of the VIDEO_TS folder in a test image.
type isoFile struct {
	name string
	data []byte
}

func dirRecord(name string, lba, size uint32, dir bool) []byte {
	l := 33 + len(name)
	if l%2 != 0 {
		l++
	}
	r := make([]byte, l)
	r[0] = byte(l)
	binary.LittleEndian.PutUint32(r[2:], lba)
	binary.BigEndian.PutUint32(r[6:], lba)
	binary.LittleEndian.PutUint32(r[10:], size)
	binary.BigEndian.PutUint32(r[14:], size)
	if dir {
		r[25] = 2
	}
	r[32] = byte(len(name))
	copy(r[33:], name)
	return r
}

// buildISO writes a small ISO 9660 image with a VIDEO_TS folder. claim is what its
// volume descriptor says the size is (0: the truth), and sizes can overstate a
// file's size to run past the end.
func buildISO(t *testing.T, path string, files []isoFile, claim uint32, overstate map[string]uint32) int64 {
	t.Helper()
	const rootLBA, dirLBA = 18, 19
	next := uint32(20)
	type placed struct {
		isoFile
		lba uint32
	}
	var out []placed
	for _, f := range files {
		out = append(out, placed{f, next})
		next += uint32(len(f.data)+2047) / 2048
	}
	img := make([]byte, int(next)*2048)
	if claim == 0 {
		claim = next
	}
	pvd := img[16*2048:]
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	pvd[6] = 1
	copy(pvd[40:], "TESTLABEL")
	binary.LittleEndian.PutUint32(pvd[80:], claim)
	copy(pvd[156:], dirRecord("\x00", rootLBA, 2048, true))
	term := img[17*2048:]
	term[0] = 255
	copy(term[1:], "CD001")
	root := img[rootLBA*2048:]
	p := copy(root, dirRecord("\x00", rootLBA, 2048, true))
	p += copy(root[p:], dirRecord("\x01", rootLBA, 2048, true))
	copy(root[p:], dirRecord("VIDEO_TS", dirLBA, 2048, true))
	dir := img[dirLBA*2048:]
	p = copy(dir, dirRecord("\x00", dirLBA, 2048, true))
	p += copy(dir[p:], dirRecord("\x01", rootLBA, 2048, true))
	for _, f := range out {
		size := uint32(len(f.data))
		if o := overstate[f.name]; o != 0 {
			size = o
		}
		p += copy(dir[p:], dirRecord(f.name+";1", f.lba, size, false))
		copy(img[int(f.lba)*2048:], f.data)
	}
	if err := os.WriteFile(path, img, 0o644); err != nil {
		t.Fatal(err)
	}
	return int64(len(img))
}

// goodVOB is n sectors of proper packs.
func goodVOB(n int) []byte {
	var b []byte
	for range n {
		b = append(b, videoPack(false)...)
	}
	return b
}

func TestDiagnoseDamageScanOfAnImage(t *testing.T) {
	vob := goodVOB(40)
	vob = append(vob, make([]byte, 32*2048)...) // what a copying tool leaves for a bad patch
	junk := make([]byte, 8*2048)
	for i := range junk {
		junk[i] = byte(i%251 + 1)
	}
	vob = append(vob, junk...)
	vob = append(vob, goodVOB(8)...)
	img := filepath.Join(t.TempDir(), "disc.iso")
	buildISO(t, img, []isoFile{{"VIDEO_TS.IFO", []byte("not really")}, {"VTS_01_1.VOB", vob}}, 0, nil)

	src, err := openSource(img)
	if err != nil {
		t.Fatal(err)
	}
	defer src.close()
	if !src.HasDVD || src.Format != "iso9660" || src.Label != "TESTLABEL" {
		t.Fatalf("%+v", src)
	}
	j := testJob()
	scanDamage(context.Background(), j, src, damageOptions{packs: true, where: func(string, int64) string { return "the main title" }})
	if !hasFinding(j, SevWarn, "empty") || !hasFinding(j, SevWarn, "right format") {
		t.Fatal(listFindings(j))
	}
	all := listFindings(j)
	if !strings.Contains(all, "VTS_01_1.VOB, at sector 40: 32 sectors (the main title)") {
		t.Errorf("where the zeros are:\n%s", all)
	}
	if !strings.Contains(all, "at sector 72: 8 sectors") {
		t.Errorf("where the bad packs are:\n%s", all)
	}
	if hasFinding(j, SevOK, "No damage") || hasFinding(j, SevProblem, "") {
		t.Errorf("verdict:\n%s", all)
	}
}

func TestDiagnoseDamageScanFindsAShortImage(t *testing.T) {
	img := filepath.Join(t.TempDir(), "disc.iso")
	size := buildISO(t, img, []isoFile{{"VTS_01_1.VOB", goodVOB(16)}}, 1000, map[string]uint32{"VTS_01_1.VOB": 2048 * 500})
	if size >= 1000*2048 {
		t.Fatal("the fixture isn't short")
	}
	src, err := openSource(img)
	if err != nil {
		t.Fatal(err)
	}
	defer src.close()
	j := testJob()
	scanDamage(context.Background(), j, src, damageOptions{packs: true})
	if !hasFinding(j, SevProblem, "incomplete") || !hasFinding(j, SevProblem, "past the end") {
		t.Fatal(listFindings(j))
	}
}

func TestDiagnoseCleanImageIsClean(t *testing.T) {
	img := filepath.Join(t.TempDir(), "disc.iso")
	buildISO(t, img, []isoFile{{"VTS_01_1.VOB", goodVOB(64)}}, 0, nil)
	src, err := openSource(img)
	if err != nil {
		t.Fatal(err)
	}
	defer src.close()
	j := testJob()
	scanDamage(context.Background(), j, src, damageOptions{packs: true})
	if !hasFinding(j, SevOK, "No damage") || len(j.report.Findings) != 1 {
		t.Fatal(listFindings(j))
	}
}

// failingReader fails on a few sectors, as a bad patch of a disk does.
type failingReader struct {
	data []byte
	bad  map[int64]bool
}

func (r failingReader) ReadAt(p []byte, off int64) (int, error) {
	for s := off / scanSector; s*scanSector < off+int64(len(p)); s++ {
		if r.bad[s] {
			return 0, errors.New("input/output error")
		}
	}
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	return copy(p, r.data[off:]), nil
}

func TestDiagnoseUnreadableSectorsAreCountedOneByOne(t *testing.T) {
	data := goodVOB(2048) // 4 MiB: several chunks
	bad := map[int64]bool{700: true, 701: true, 702: true, 1500: true}
	j := testJob()
	s := &damageScan{ctx: context.Background(), j: j, o: damageOptions{packs: true}, buf: make([]byte, scanChunk), zero: make([]byte, scanSector), total: int64(len(data))}
	name := "VIDEO_TS/VTS_01_1.VOB"
	var seen int64
	s.readAll(failingReader{data, bad}, 0, int64(len(data)),
		func(off int64, b []byte) { seen += int64(len(b)); s.feed(name, kindVOB, off, b) },
		func(off int64) { s.unread.add(name, off/scanSector, 1, func() string { return "" }) })
	if s.unread.total != 4 || s.unread.stretches != 2 {
		t.Fatalf("%+v", s.unread)
	}
	if seen != int64(len(data))-4*scanSector {
		t.Errorf("read %d", seen)
	}
	if s.unread.spots[0].at != 700 || s.unread.spots[0].count != 3 || s.unread.spots[1].at != 1500 {
		t.Errorf("%+v", s.unread.spots)
	}
	if s.packs.total != 0 {
		t.Errorf("good packs flagged: %+v", s.packs)
	}
}

func TestDiagnoseDamageScanOfBlurayStreams(t *testing.T) {
	dir := t.TempDir()
	stream := filepath.Join(dir, "BDMV", "STREAM")
	if err := os.MkdirAll(stream, 0o755); err != nil {
		t.Fatal(err)
	}
	// 2000 units, of which 50 in a row lost their sync byte.
	b := make([]byte, 2000*tsUnit)
	for i := range 2000 {
		b[i*tsUnit] = 1 // the timestamp isn't empty
		b[i*tsUnit+5] = 0x11
		if i < 1000 || i >= 1050 {
			b[i*tsUnit+4] = 0x47
		}
	}
	if err := os.WriteFile(filepath.Join(stream, "00001.m2ts"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := openSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !src.HasBD || len(src.Files) != 1 {
		t.Fatalf("%+v", src)
	}
	j := testJob()
	scanDamage(context.Background(), j, src, damageOptions{ts: true})
	if !hasFinding(j, SevWarn, "rhythm") {
		t.Fatal(listFindings(j))
	}
	if all := listFindings(j); !strings.Contains(all, "00001.m2ts, at packet 1000: 50 packets") {
		t.Errorf("%s", all)
	}
	// On a scrambled disc that's expected, and nothing is said.
	j = testJob()
	scanDamage(context.Background(), j, src, damageOptions{ts: true, scrambled: true})
	if hasFinding(j, SevWarn, "rhythm") || !hasFinding(j, SevOK, "No damage") {
		t.Fatal(listFindings(j))
	}
}

func TestDiagnoseImageRegionsSurviveOverlaps(t *testing.T) {
	files := []discFile{
		{Name: "A.VOB", Exts: []isofs.Extent{{Offset: 0, Length: 4096}, {Offset: 8192, Length: 2048}}},
		{Name: "B.VOB", Exts: []isofs.Extent{{Offset: 2048, Length: 4096}}},   // overlaps A
		{Name: "C.VOB", Exts: []isofs.Extent{{Offset: 100000, Length: 4096}}}, // past the end
	}
	got := imageRegions(files, 16384)
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	if got[1].name != "B.VOB" || got[1].start != 4096 || got[1].logical != 2048 {
		t.Errorf("%+v", got[1])
	}
	if got[2].name != "A.VOB" || got[2].logical != 4096 {
		t.Errorf("%+v", got[2])
	}
}

// ---------------------------------------------------------------- a whole check, and its report

func TestDiagnoseDiscCheckAndWhatTheReportHolds(t *testing.T) {
	dataDir := t.TempDir()
	top := realDir(t)
	folder := filepath.Join(top, "My Private Folder")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(folder, "My Secret Film.iso")
	buildISO(t, img, []isoFile{{"VIDEO_TS.IFO", []byte(contentMark)}, {"VTS_01_1.VOB", goodVOB(32)}}, 0, nil)

	m := NewManager(Env{DataDir: dataDir, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	j, err := m.SubmitDisc(img, DiscOptions{Crawl: true, Damage: true})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the check", j.isSaved)
	v, _ := m.JobView(j.ID)
	if v.Subject != "My Secret Film.iso" || v.Label != "TESTLABEL" {
		t.Errorf("%+v", v.Report)
	}
	found := map[string]bool{}
	for _, f := range v.Findings {
		found[f.Title] = true
	}
	if !found["Index files of this DVD are damaged"] || !found["The DVD can't be opened"] || !found["No damage found"] {
		t.Errorf("%+v", v.Findings)
	}

	saved, err := os.ReadFile(filepath.Join(dataDir, "diagnostics", j.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	full, _ := m.ReportFor(j.ID)
	text := full.Text()
	for what, got := range map[string]string{"saved": string(saved), "text": text} {
		// Even in JSON escapes, on Windows: never a path, never the disc's bytes.
		for _, no := range []string{folder, filepath.ToSlash(folder), "My Private Folder", contentMark, top, jsonEscaped(folder)} {
			if strings.Contains(got, no) {
				t.Errorf("the %s report holds %q:\n%s", what, no, got)
			}
		}
	}
	red := full.Redact()
	rb, err := json.Marshal(red)
	if err != nil {
		t.Fatal(err)
	}
	for what, got := range map[string]string{"json": string(rb), "text": red.Text()} {
		for _, no := range []string{"My Secret Film", "TESTLABEL", folder, contentMark} {
			if strings.Contains(got, no) {
				t.Errorf("the redacted %s holds %q:\n%s", what, no, got)
			}
		}
	}
	if !red.Redacted {
		t.Error("not marked")
	}
}

func jsonEscaped(s string) string {
	b, _ := json.Marshal(s)
	return strings.Trim(string(b), `"`)
}

func TestDiagnoseThingsThatArentDiscs(t *testing.T) {
	dir := t.TempDir()
	run := func(p string) *Job {
		j := testJob()
		j.scrub = []scrubRule{{path: p, with: "this disc"}, {path: dir, with: "its folder"}}
		checkDisc(context.Background(), j, p, DiscOptions{Crawl: true, Damage: true})
		return j
	}
	tiny := filepath.Join(dir, "tiny.iso")
	if err := os.WriteFile(tiny, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	j := run(tiny)
	if !hasFinding(j, SevProblem, "isn't a disc image") {
		t.Error(listFindings(j))
	}
	if strings.Contains(listFindings(j), dir) {
		t.Errorf("a path got through: %s", listFindings(j))
	}
	raw := filepath.Join(dir, "raw.bin")
	b := make([]byte, 64*2352)
	copy(b[16*2352:], []byte{0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0})
	copy(b[16*2352+17:], "CD001")
	b[0] = 1
	if err := os.WriteFile(raw, b, 0o644); err != nil {
		t.Fatal(err)
	}
	j = run(raw)
	if !strings.Contains(listFindings(j), "2352-byte") {
		t.Error(listFindings(j))
	}
	if !hasFinding(run(filepath.Join(dir, "nothing.iso")), SevProblem, "can't be opened") {
		t.Error("a missing file")
	}
	empty := filepath.Join(dir, "plain folder")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(empty, "film.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if j := run(empty); !hasFinding(j, SevProblem, "isn't a DVD or Blu-ray") {
		t.Error(listFindings(j))
	}
	// A folder with broken index files says which ones.
	broken := filepath.Join(dir, "broken", "VIDEO_TS")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "VIDEO_TS.IFO"), []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	j = run(filepath.Dir(broken))
	if !hasFinding(j, SevProblem, "Index files") || !strings.Contains(listFindings(j), "VIDEO_TS.IFO") {
		t.Error(listFindings(j))
	}
	// A broken Blu-ray too.
	bd := filepath.Join(dir, "bd", "BDMV")
	if err := os.MkdirAll(bd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bd, "index.bdmv"), []byte("broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if j := run(filepath.Dir(bd)); !hasFinding(j, SevProblem, "Blu-ray can't be opened") {
		t.Error(listFindings(j))
	}
}

// ---------------------------------------------------------------- storage

func storageFixture(t *testing.T) (library, data string) {
	t.Helper()
	top := realDir(t)
	library, data = filepath.Join(top, "library"), filepath.Join(top, "data")
	for _, d := range []string{library, data} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return library, data
}

func listAll(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != dir {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDiagnoseWriteProbeCleansUp(t *testing.T) {
	dir := t.TempDir()
	res, err := writeProbe(context.Background(), dir, 3<<20)
	if err != nil || !res.matched {
		t.Fatalf("%+v, %v", res, err)
	}
	if left := listAll(t, dir); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
	// Cancelled partway, and into a folder that can't take it.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := writeProbe(ctx, dir, 3<<20); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: %v", err)
	}
	if left := listAll(t, dir); len(left) != 0 {
		t.Errorf("left behind after cancelling: %v", left)
	}
	if _, err := writeProbe(context.Background(), filepath.Join(dir, "not-there"), 1<<20); err == nil {
		t.Error("a folder that isn't there")
	}
}

func TestDiagnoseMayNeverWriteInAMediaFolder(t *testing.T) {
	library, data := storageFixture(t)
	inData := filepath.Join(data, "movies")
	aroundData := filepath.Dir(data)
	if err := os.MkdirAll(inData, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		role, path string
		libs       []string
		want       bool
	}{
		{"data", data, []string{library}, true},
		{"transcode", data, nil, true},
		{"logs", data, []string{library}, true},
		{"library", library, []string{library}, false},
		{"library", data, nil, false},
		{"other", data, nil, false},
		{"data", library, []string{library}, false},                     // the same folder
		{"data", data, []string{inData}, false},                         // a library inside it
		{"data", inData, []string{data}, false},                         // inside a library
		{"data", data, []string{aroundData}, false},                     // ditto, higher up
		{"data", filepath.Join(data, "gone"), []string{library}, false}, // can't be resolved
	}
	for _, c := range cases {
		if got := mayWrite(c.role, c.path, c.libs); got != c.want {
			t.Errorf("mayWrite(%s, %q, %q) = %v", c.role, c.path, c.libs, got)
		}
	}
	// And through a link, in either direction.
	link := filepath.Join(filepath.Dir(library), "to-library")
	if err := os.Symlink(library, link); err == nil {
		if mayWrite("data", link, []string{library}) {
			t.Error("a link to the library")
		}
		if mayWrite("data", library, []string{link}) {
			t.Error("a library named through a link")
		}
	}
}

func runStorage(t *testing.T, m *Manager, targets []StorageTarget, o StorageOptions) *View {
	t.Helper()
	j, err := m.SubmitStorage(targets, o)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the check", j.isSaved)
	v, _ := m.JobView(j.ID)
	return v
}

func TestDiagnoseStorageCheckWritesOnlyWhereAllowed(t *testing.T) {
	library, data := storageFixture(t)
	film := filepath.Join(library, "Private Film Name.mkv")
	if err := os.WriteFile(film, make([]byte, 3<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(library, "empty.mkv"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(library, " odd name.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := len(listAll(t, library))
	gone := filepath.Join(library, "Gone Film.mkv")
	dataDir := t.TempDir()
	m := NewManager(Env{DataDir: dataDir, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		ItemPaths: func(ctx context.Context, lib int64, limit int) ([]string, error) {
			return []string{film, gone, "https://example.com/x"}, nil
		}})
	v := runStorage(t, m, []StorageTarget{
		{ID: "lib:1:0", Role: "library", Label: "Movies: library", Plain: "Library 1, folder 1", Path: library, LibraryID: 1},
		{ID: "data:data", Role: "data", Label: "Data folder", Plain: "Data folder", Path: data},
		{ID: "data:gone", Role: "transcode", Label: "Transcode folder", Plain: "Transcode folder", Path: filepath.Join(data, "not-made")},
	}, StorageOptions{ReadTest: true, WriteTest: true, Walk: true, Libraries: []string{library}})
	if v.State != StateDone {
		t.Fatalf("%+v", v)
	}
	has := func(target, title string) bool {
		for _, f := range v.Findings {
			if f.Target == target && strings.Contains(f.Title, title) {
				return true
			}
		}
		return false
	}
	for _, c := range [][2]string{
		{"Movies: library", "write test isn't run"},
		{"Movies: library", "Reads"},
		{"Movies: library", "file is empty"},
		{"Movies: library", "item in the library is missing"},
		{"Movies: library", "may cause trouble"},
		{"Movies: library", "server can read"},
		{"Data folder", "Writing here works"},
		{"Transcode folder", "can't be reached"},
	} {
		if !has(c[0], c[1]) {
			t.Errorf("no %q for %s in %+v", c[1], c[0], v.Findings)
		}
	}
	if after := listAll(t, library); len(after) != before {
		t.Errorf("the library changed: %v", after)
	}
	if left := listAll(t, data); len(left) != 0 {
		t.Errorf("the data folder has leftovers: %v", left)
	}
	full, _ := m.ReportFor(v.ID)
	b, _ := json.Marshal(full)
	for _, no := range []string{library, jsonEscaped(library), data, jsonEscaped(data)} {
		if strings.Contains(string(b), no) {
			t.Errorf("a path reached the report: %s", no)
		}
	}
	if !strings.Contains(string(b), "Gone Film.mkv") {
		t.Error("names of what's missing should be in the unredacted report")
	}
	red := full.Redact()
	rb, _ := json.Marshal(red)
	for _, no := range []string{"Gone Film", "empty.mkv", "library\"", "Movies: library"} {
		if strings.Contains(string(rb), no) {
			t.Errorf("redacted report holds %q", no)
		}
	}
	if !strings.Contains(red.Text(), "Library 1, folder 1") {
		t.Errorf("the plain name is gone too:\n%s", red.Text())
	}
}

func TestDiagnoseWalkIsCancelledAndCounted(t *testing.T) {
	library, _ := storageFixture(t)
	for i := range 40 {
		if err := os.WriteFile(filepath.Join(library, "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".mkv"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	j := testJob()
	c := &storageCheck{ctx: context.Background(), j: j, t: StorageTarget{Path: library, Role: "library"}, span: 1}
	r := c.walk(maxWalkEntries, walkBudget, true)
	if r.files != 40 || len(r.largest) != 3 {
		t.Errorf("%+v", r)
	}
	// The limit on entries holds.
	r = c.walk(10, walkBudget, true)
	if r.stoppedAt == "" || r.entries > 10 {
		t.Errorf("%+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.ctx = ctx
	if r := c.walk(maxWalkEntries, walkBudget, true); r.files != 0 {
		t.Errorf("cancelled: %+v", r)
	}
}

func TestDiagnoseOddNames(t *testing.T) {
	for _, n := range []string{"bad\nname.mkv", " leading.mkv", "trailing.mkv ", "dot.", "bad\xffname.mkv"} {
		if oddName(n) == "" {
			t.Errorf("%q should be odd", n)
		}
	}
	for _, n := range []string{"Film (2001).mkv", "Amélie.mkv", "日本語.iso", "It's fine.mp4"} {
		if why := oddName(n); why != "" {
			t.Errorf("%q: %s", n, why)
		}
	}
}

func TestDiagnoseSpeedWords(t *testing.T) {
	if f := speedFinding(1<<30, time.Second, []time.Duration{time.Millisecond}); f.Severity != SevOK || !strings.Contains(f.What, "cache") {
		t.Errorf("%+v", f)
	}
	if f := speedFinding(1<<20, 10*time.Second, nil); f.Severity != SevWarn {
		t.Errorf("%+v", f)
	}
}
