package logging

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func read(t *testing.T, l *Logger) string {
	t.Helper()
	l.Flush()
	var b strings.Builder
	for _, f := range l.Files() {
		data, _ := os.ReadFile(filepath.Join(l.Dir(), f.Name))
		b.Write(data)
	}
	return b.String()
}

func newTest(t *testing.T) (*Logger, *slog.Logger) {
	t.Helper()
	l, err := New(t.TempDir(), slog.LevelInfo, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	return l, slog.New(l.Handler())
}

func TestCategoriesAndRedaction(t *testing.T) {
	l, log := newTest(t)

	log.Info("plain info")
	log.Debug("plain debug")
	log.Info("dvd step", "cat", "dvd")
	log.With("cat", "ffmpeg").Debug("ffmpeg start", "args", "-i x.mkv")
	log.Warn("dvd warning", "cat", "dvd")
	out := read(t, l)
	for _, want := range []string{"plain info", "dvd warning", "cat=discs"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, not := range []string{"plain debug", "dvd step", "ffmpeg start"} {
		if strings.Contains(out, not) {
			t.Errorf("%q logged although its category is quiet", not)
		}
	}

	l.SetCategories([]string{"dvd", "ffmpeg", "bogus"})
	if got := strings.Join(l.Enabled(), ","); got != "transcode,discs" && got != "discs,transcode" {
		t.Errorf("enabled = %s", got)
	}
	log.Info("dvd step 2", "cat", "dvd")
	log.With("cat", "ffmpeg").Debug("ffmpeg start 2")
	log.Debug("plain debug 2")
	log.Info("request", "url", "/api/hls/x?token=abc123&x=1", "password", "hunter2", "auth", "Bearer eyJabc.def")
	out = read(t, l)
	for _, want := range []string{"dvd step 2", "ffmpeg start 2", "token=[hidden]"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, not := range []string{"plain debug 2", "abc123", "hunter2", "eyJabc"} {
		if strings.Contains(out, not) {
			t.Errorf("%q should not be in the log", not)
		}
	}
}

// Levels change live, one category at a time.
func TestLevelsPerCategoryLive(t *testing.T) {
	l, log := newTest(t)
	log.Debug("before", "cat", "transcode")
	l.SetCategoryLevel("transcode", slog.LevelDebug)
	log.Debug("after", "cat", "transcode")
	log.Debug("other", "cat", "tv")
	l.SetLevels(map[string]slog.Level{"tv": LevelOff, "ffmpeg": slog.LevelError})
	log.Error("silenced", "cat", "tv")
	log.Warn("too low now", "cat", "transcode")
	log.Error("still goes", "cat", "transcode")
	ReportPanicTo(log, "tv crash", "cat", "tv")
	out := read(t, l)
	for _, want := range []string{"after", "still goes", "tv crash"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, not := range []string{"before", "other", "silenced", "too low now"} {
		if strings.Contains(out, not) {
			t.Errorf("%q should not be in\n%s", not, out)
		}
	}
}

// Each module has a level of its own and options to turn on, live, and nothing
// about one touches the others.
func TestModuleLevelsAndTopics(t *testing.T) {
	l, log := newTest(t)
	photos := log.With("cat", "modules", "module", "wittyphotos")
	dvd := log.With("cat", "discs", "module", "WittyDVD") // names are compared in lower case
	photos.Debug("quiet at first")
	dvd.Debug("quiet at first too", "topic", "nav")

	l.SetModules(map[string]ModuleLog{"wittyphotos": {Level: "debug"}, "wittydvd": {Topics: map[string]bool{"nav": true, "off": false}}})
	photos.Debug("photos in detail")
	dvd.Debug("a navigation step", "topic", "nav")
	dvd.Debug("another topic stays quiet", "topic", "menus")
	dvd.Debug("no topic stays quiet")
	log.Debug("another module stays quiet", "cat", "modules", "module", "wittytuner")
	log.Debug("the area stays quiet", "cat", "discs")
	log.Debug("named on the line itself", "cat", "modules", "module", "wittyphotos")

	l.SetModules(map[string]ModuleLog{"wittyphotos": {Level: "error"}})
	photos.Warn("a module can be made quieter than its area")
	photos.Error("errors still get through")
	log.Warn("another module's warning", "cat", "modules", "module", "wittytuner")

	l.SetModules(nil)
	photos.Debug("back to normal")
	photos.Warn("back to warnings")

	out := read(t, l)
	for _, want := range []string{"photos in detail", "a navigation step", "module=wittydvd topic=nav", "named on the line itself", "errors still get through", "another module's warning", "back to warnings"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	for _, not := range []string{"quiet at first", "another topic stays quiet", "no topic stays quiet", "another module stays quiet", "the area stays quiet", "can be made quieter", "back to normal"} {
		if strings.Contains(out, not) {
			t.Errorf("%q should not be in\n%s", not, out)
		}
	}
	page, err := l.Query(Filter{MinLevel: slog.LevelDebug, Module: "wittydvd"})
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Topic != "nav" {
		t.Errorf("%v %+v", err, page.Entries)
	}
	// What a helper process is told.
	l.SetModules(map[string]ModuleLog{"wittyphotos": {Level: "debug", Topics: map[string]bool{"b": true, "a": true}}})
	if got := strings.Join(l.ModuleEnv("wittyphotos"), " "); got != "WITTYMODULE_LOG=debug WITTYMODULE_LOG_TOPICS=a,b" {
		t.Errorf("%s", got)
	}
	if got := l.ModuleEnv("other"); got != nil {
		t.Errorf("%v", got)
	}
}

// ReportPanicTo logs at the crash level, which goes through whatever the level.
func ReportPanicTo(log *slog.Logger, msg string, args ...any) {
	log.Log(context.Background(), LevelCrash, msg, args...)
}

// An HTTP client's error quotes the address it asked, API key and all, and it
// reaches the log as an error value rather than text.
func TestErrorValuesAreRedacted(t *testing.T) {
	l, log := newTest(t)
	log.Warn("lookup failed", "err", errors.New(`Get "https://api.example.com/3/search?api_key=abc123&query=x": timeout`))
	if got := read(t, l); strings.Contains(got, "abc123") || !strings.Contains(got, "timeout") {
		t.Fatalf("log: %s", got)
	}
}

// Secrets planted every way we can think of. Each has a marker that must not
// survive in the file, in memory, or in a bundle.
func TestRedactPlantedSecrets(t *testing.T) {
	const m = "PLANTED0secret9value"
	hex64 := strings.Repeat("a1", 32)
	AddSecret("exact-known-" + m)
	cases := map[string][]any{
		"attr password":    {"password", m},
		"attr token":       {"token", m},
		"attr session key": {"sessionToken", m},
		"attr tmdb":        {"tmdbKey", m},
		"attr apikey":      {"apiKey", m},
		"attr cookie":      {"Cookie", m},
		"attr setup":       {"setupCode", m},
		"attr recovery":    {"recoveryCode", m},
		"attr group":       {slog.Group("req", "token", m)},
		"url token":        {"url", "http://h/a?token=" + m + "&x=1"},
		"url st":           {"url", "http://h/api/hls/1/seg/2?st=st1." + m},
		"url apikey":       {"url", "https://a/b?api_key=" + m},
		"url encoded name": {"url", "http://h/a?%74oken=" + m},
		"url login":        {"url", "rtsp://admin:" + m + "@camera/stream"},
		"share link":       {"p", "/api/share/" + m + "/file/5"},
		"short share":      {"p", "/s/" + m},
		"bearer":           {"h", "Authorization: Bearer " + m},
		"bare bearer":      {"h", "Bearer " + m},
		"basic":            {"h", "Authorization: Basic " + m + "=="},
		"cookie text":      {"h", "Cookie: wms_token=" + m + "; other=1"},
		"set-cookie":       {"h", "Set-Cookie: wms_token=" + m + "; HttpOnly"},
		"x-wms":            {"h", "X-WMS-Token: " + m},
		"ffmpeg args":      {"args", `-headers "Authorization: Bearer ` + m + `\r\n" -i "http://tuner/s?token=` + m + `"`},
		"json password":    {"body", `{"name":"a","password":"` + m + `"}`},
		"json token":       {"body", `{"accessToken": "` + m + `"}`},
		"stream token":     {"text", "tried st1." + m + " and failed"},
		"exact value":      {"text", "something exact-known-" + m + " odd"},
		"long hex":         {"text", "session " + hex64 + " ok"},
		"pin":              {"url", "http://h/login?pin=" + m},
		"setup code url":   {"url", "/api/setup?setupCode=" + m},
		"secret in err":    {"err", fmt.Errorf(`Post "http://x?password=%s": EOF`, m)},
		"stderr":           {"stderr", "Opening 'http://h/a?token=" + m + "' for reading"},
		"stack":            {"stack", "goroutine 1\nGet https://h/?key=" + m},
		"otpauth":          {"url", "otpauth://totp/WMS?secret=" + m},
		"pair code":        {"url", "/p?pairCode=" + m},
	}
	l, log := newTest(t)
	for name, args := range cases {
		log.Warn("case "+name, args...)
		log.Warn("in message: http://h/?token=" + m)
	}
	out := read(t, l)
	if strings.Contains(out, m) || strings.Contains(out, hex64) {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, m) || strings.Contains(line, hex64) {
				t.Errorf("leaked to the file: %s", line)
			}
		}
	}
	page, err := l.Query(Filter{MinLevel: slog.LevelDebug, Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) < len(cases) {
		t.Fatalf("only %d entries", len(page.Entries))
	}
	for _, e := range page.Entries {
		if strings.Contains(fmt.Sprint(e), m) {
			t.Errorf("leaked to memory: %+v", e)
		}
	}
	// A bundle redacts again, in case a file was written before a rule existed.
	old := filepath.Join(l.Dir(), "wms-2000-01-01.log")
	line := `time=2026-01-01T00:00:00.000000Z level=WARN cat=server msg="old url http://h/?token=` + m + `" password=` + m + "\n"
	if err := os.WriteFile(old, []byte(line), 0o640); err != nil {
		t.Fatal(err)
	}
	var zipped bytes.Buffer
	if err := l.WriteBundle(&zipped, nil, BundleOptions{Days: 1}); err != nil {
		t.Fatal(err)
	}
	if text := unzipAll(t, zipped.Bytes()); strings.Contains(text, m) {
		t.Errorf("leaked to the bundle:\n%s", text)
	}
}

func unzipAll(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString("=== " + f.Name + "\n")
		_, _ = io.Copy(&b, rc)
		rc.Close()
	}
	return b.String()
}

// Text from outside can't make a line of its own, a field of its own, or do
// anything to a terminal.
func TestForgedLines(t *testing.T) {
	l, log := newTest(t)
	evil := "x\ntime=2026-01-01T00:00:00Z level=ERROR msg=\"forged\" admin=yes\r\n\x1b[2J\u202egnp.exe"
	log.Warn(evil, "file", evil, evil, "value", "cat", "evil\ncat", "session", "a b\nc")
	out := read(t, l)
	if n := strings.Count(strings.TrimRight(out, "\n"), "\n"); n != 0 {
		t.Fatalf("one record made %d lines:\n%s", n+1, out)
	}
	if strings.ContainsAny(out, "\x1b\r") || strings.Contains(out, "\u202e") {
		t.Errorf("control characters survived: %q", out)
	}
	rec := ParseLine(strings.TrimRight(out, "\n"))
	if rec.Cat != CatDefault || rec.Session != "" {
		t.Errorf("category or session were forged: %+v", rec)
	}
	for _, a := range rec.Attrs {
		if a.K == "admin" {
			t.Errorf("a field was forged: %+v", rec)
		}
	}
}

func TestLineRoundTrip(t *testing.T) {
	in := Record{Time: time.Now().Truncate(time.Microsecond), Lv: slog.LevelError, Cat: "games", Session: "abc-1", Req: "r-1", Msg: `a "quoted" message`,
		Attrs: []Attr{{"err", "line one\nline two"}, {"plain", "word"}, {"empty", ""}, {"unicode", "日本語 テスト"}}}
	out := ParseLine(string(in.appendLine(nil)))
	if !out.Time.Equal(in.Time) || out.Lv != in.Lv || out.Cat != in.Cat || out.Session != in.Session || out.Req != in.Req || out.Msg != in.Msg {
		t.Fatalf("got %+v", out)
	}
	if fmt.Sprint(out.Attrs) != fmt.Sprint(in.Attrs) {
		t.Fatalf("attrs %+v", out.Attrs)
	}
	// Cut off by a crash or a full disk, it still reads.
	line := string(in.appendLine(nil))
	if r := ParseLine(line[:len(line)/2]); r.Msg == "" {
		t.Fatalf("half a line: %+v", r)
	}
	if r := ParseLine("not a log line"); r.Msg != "not a log line" {
		t.Fatalf("%+v", r)
	}
	// What older versions wrote.
	r := ParseLine(`time=2026-10-29T12:00:00.123+01:00 level=WARN msg="old style" cat=dvd err="oops"`)
	if r.Lv != slog.LevelWarn || r.Cat != "discs" || r.Msg != "old style" || r.Time.IsZero() {
		t.Fatalf("%+v", r)
	}
}

func TestLogNames(t *testing.T) {
	for name, ok := range map[string]bool{"wms-2026-10-29.log": true, "wms-2026-10-29-2.log": true, "../wms-x.log": false, `a\wms-x.log`: false, "other.log": false} {
		if IsLogName(name) != ok {
			t.Errorf("%s: want %v", name, ok)
		}
	}
}

func shrink(t *testing.T) {
	t.Helper()
	oldFile, oldTotal, oldRetry, oldFlush, oldCheck := maxFileSize, maxTotalSize, retryOpen, flushEvery, checkEvery
	t.Cleanup(func() {
		maxFileSize, maxTotalSize, retryOpen, flushEvery, checkEvery = oldFile, oldTotal, oldRetry, oldFlush, oldCheck
	})
}

func TestRotationBySizeAndTotal(t *testing.T) {
	shrink(t)
	maxFileSize, maxTotalSize = 16<<10, 64<<10
	l, log := newTest(t)
	big := strings.Repeat("x", 1000)
	for i := 0; i < 400; i++ {
		log.Warn("a line", "n", i, "pad", big)
		if i%40 == 0 {
			l.Flush()
			time.Sleep(time.Millisecond)
		}
	}
	files := l.Files()
	if len(files) < 2 {
		t.Fatalf("no rotation: %+v", files)
	}
	var total int64
	for _, f := range files {
		total += f.Size
		if f.Size > maxFileSize+32<<10 {
			t.Errorf("%s has grown to %d", f.Name, f.Size)
		}
	}
	if total > maxTotalSize+2*maxFileSize+32<<10 {
		t.Errorf("total %d is over the limit", total)
	}
}

func TestPruneByAge(t *testing.T) {
	l, log := newTest(t)
	old := filepath.Join(l.Dir(), "wms-2000-01-01.log")
	if err := os.WriteFile(old, []byte("x\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	long := time.Now().AddDate(-1, 0, 0)
	if err := os.Chtimes(old, long, long); err != nil {
		t.Fatal(err)
	}
	log.Warn("opens the file")
	l.Flush()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old file still there: %v", err)
	}
}

// The folder goes away or can't be written: nobody notices but the settings
// page, the viewer still works, and logging comes back when the folder does.
func TestUnwritableFolder(t *testing.T) {
	shrink(t)
	retryOpen = 20 * time.Millisecond
	dir := t.TempDir()
	blocked := filepath.Join(dir, "file-not-folder")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// A folder that can't be made: a file is in the way.
	l, err := New(filepath.Join(blocked, "logs"), slog.LevelInfo, nil)
	if err != nil {
		t.Fatalf("an unusable folder must not stop the server: %v", err)
	}
	defer l.Close()
	log := slog.New(l.Handler())
	start := time.Now()
	for i := 0; i < 2000; i++ {
		log.Warn("nowhere to go", "n", i)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("logging was held up for %v", d)
	}
	l.Flush()
	h := l.Health()
	if h.Error == "" || h.Lost == 0 {
		t.Errorf("health doesn't say so: %+v", h)
	}
	page, err := l.Query(Filter{MinLevel: slog.LevelDebug, Limit: 5})
	if err != nil || len(page.Entries) == 0 {
		t.Fatalf("the viewer should still work: %v %+v", err, page)
	}
	// Moved somewhere that works: back in business.
	good := filepath.Join(dir, "good")
	if err := l.SetDir(good); err != nil {
		t.Fatal(err)
	}
	log.Warn("after moving")
	if got := read(t, l); !strings.Contains(got, "after moving") {
		t.Fatalf("not written: %q", got)
	}
	if h := l.Health(); h.Error != "" || !h.Writing {
		t.Errorf("health didn't recover: %+v", h)
	}
	if err := l.SetDir(blocked); err == nil {
		t.Error("the settings page should be told a folder is no good")
	}
}

// failingFile stands in for a full disk.
func TestFullDisk(t *testing.T) {
	shrink(t)
	retryOpen = 10 * time.Millisecond
	l, log := newTest(t)
	log.Warn("first")
	l.Flush()
	// The disk fills: every write fails from now on.
	done := make(chan struct{})
	l.ctl <- func(w *fileWriter) {
		w.buf.Reset(failWriter{})
		close(done)
	}
	<-done
	for i := 0; i < 100; i++ {
		log.Warn("while full", "n", i)
	}
	l.Flush()
	if h := l.Health(); h.Lost == 0 {
		t.Errorf("lost lines not counted: %+v", h)
	}
	// It clears: a new file opens at the next retry, and the loss is reported.
	time.Sleep(30 * time.Millisecond)
	log.Warn("after the disk cleared")
	got := read(t, l)
	for _, want := range []string{"first", "after the disk cleared", "some log messages were not written"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if line != "" && !strings.HasPrefix(line, "time=") {
			t.Errorf("damaged line: %q", line)
		}
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("no space left on device") }

// With the disk nearly full, only warnings and errors.
func TestLowDisk(t *testing.T) {
	shrink(t)
	flushEvery, checkEvery = 5*time.Millisecond, time.Millisecond
	old := FreeSpace
	FreeSpace = func(string) int64 { return 1 << 20 }
	defer func() { FreeSpace = old }()
	l, log := newTest(t)
	deadline := time.Now().Add(5 * time.Second)
	for !l.Health().LowDisk && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !l.Health().LowDisk {
		t.Fatal("low disk not noticed")
	}
	log.Info("quiet please")
	log.Warn("still written")
	got := read(t, l)
	if strings.Contains(got, "quiet please") || !strings.Contains(got, "still written") || !strings.Contains(got, "nearly full") {
		t.Errorf("%s", got)
	}
}

// blockingWriter is a console nobody is reading, or a disk that has hung.
type blockingWriter struct{ release chan struct{} }

func (b blockingWriter) Write(p []byte) (int, error) {
	<-b.release
	return len(p), nil
}

// A hung console must not hold up logging, and flooding must not hold up the
// caller: the queue drops, with a count.
func TestFloodNeverBlocks(t *testing.T) {
	release := make(chan struct{})
	l, err := New(t.TempDir(), slog.LevelInfo, blockingWriter{release})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { close(release); l.Close() }()
	// Hold the file writer too, as a hung disk would.
	stuck := make(chan struct{})
	l.ctl <- func(*fileWriter) { <-stuck }
	log := slog.New(l.Handler())
	start := time.Now()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < queueSize; i++ {
				log.Warn(fmt.Sprintf("message %d %d", g, i), "n", i)
			}
		}(g)
	}
	wg.Wait()
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("callers were held up for %v", d)
	}
	close(stuck)
	l.Flush()
	if l.Health().Dropped == 0 {
		t.Error("nothing dropped, though nothing could be written")
	}
	if n := l.ring.n; n > ringEntries {
		t.Errorf("ring holds %d", n)
	}
}

// One message over and over is counted, not written without end.
func TestFloodSameMessage(t *testing.T) {
	old := floodWindow
	floodWindow = 200 * time.Millisecond
	defer func() { floodWindow = old }()
	l, log := newTest(t)
	for i := 0; i < 5000; i++ {
		log.Warn("the same thing again")
	}
	log.Warn("something else")
	time.Sleep(250 * time.Millisecond)
	log.Warn("the same thing again")
	out := read(t, l)
	if n := strings.Count(out, "the same thing again"); n > floodLimit+2 {
		t.Errorf("written %d times", n)
	}
	if !strings.Contains(out, "something else") {
		t.Error("the flood buried another message")
	}
	if !strings.Contains(out, "repeated=") {
		t.Errorf("no count of what was left out:\n%s", out)
	}
}

// Big text doesn't grow without bound.
func TestBoundedRecords(t *testing.T) {
	l, log := newTest(t)
	huge := strings.Repeat("y", 1<<20)
	log.Warn(huge, "stderr", huge, "other", huge)
	page, err := l.Query(Filter{MinLevel: slog.LevelDebug, Limit: 1})
	if err != nil || len(page.Entries) != 1 {
		t.Fatal(err, page)
	}
	e := page.Entries[0]
	if len(e.Msg) > maxMsg+64 {
		t.Errorf("message %d", len(e.Msg))
	}
	for _, a := range e.Attrs {
		if len(a.V) > maxLongAttr+64 {
			t.Errorf("%s is %d", a.K, len(a.V))
		}
	}
	for i := 0; i < ringEntries*2; i++ {
		l.ring.add(&Record{Time: time.Now(), Cat: "server", Msg: strings.Repeat("z", 4000)})
	}
	if l.ring.bytes > ringBytes {
		t.Errorf("ring holds %d bytes", l.ring.bytes)
	}
}

type badValue struct{}

func (badValue) String() string { panic("a bad Stringer") }

type badLogValue struct{}

func (badLogValue) LogValue() slog.Value { panic("a bad LogValuer") }

// A value that panics when it's turned into text must not take anyone down.
func TestPanickyValues(t *testing.T) {
	l, log := newTest(t)
	log.Warn("one", "v", badValue{})
	log.Warn("two", "v", badLogValue{})
	log.Warn("three")
	if got := read(t, l); !strings.Contains(got, "three") {
		t.Fatalf("%s", got)
	}
}

func TestSessionsAndQuery(t *testing.T) {
	l, log := newTest(t)
	l.SetCategoryLevel("playback", slog.LevelDebug)
	ctx := WithRequest(context.Background(), "r-123")
	playing := log.With("cat", "playback", "session", "stream-a")
	playing.InfoContext(ctx, "started")
	log.With("cat", "transcode", "session", "stream-a").Error("ffmpeg stopped", "stderr", "Invalid data found when processing input")
	log.With("cat", "transcode", "session", "stream-b").Warn("not this one")
	log.Log(WithSession(context.Background(), "stream-a"), slog.LevelWarn, "client: player error", "cat", "clients")
	log.WarnContext(ctx, "same request, no session", "cat", "http")

	page, err := l.Query(Filter{MinLevel: slog.LevelDebug, Session: "stream-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 3 {
		t.Fatalf("got %+v", page.Entries)
	}
	var errRec *Record
	for i := range page.Entries {
		if page.Entries[i].Msg == "ffmpeg stopped" {
			errRec = &page.Entries[i]
		}
	}
	if errRec == nil || errRec.Explain == nil || errRec.Explain.ID != "damaged-file" {
		t.Errorf("explanation: %+v", errRec)
	}
	page, _ = l.Query(Filter{MinLevel: slog.LevelDebug, Session: "r-123"})
	if len(page.Entries) != 2 {
		t.Errorf("by request: %+v", page.Entries)
	}
	page, _ = l.Query(Filter{MinLevel: slog.LevelError})
	if len(page.Entries) != 1 {
		t.Errorf("by level: %+v", page.Entries)
	}
	page, _ = l.Query(Filter{MinLevel: slog.LevelDebug, Cats: map[string]bool{"clients": true}})
	if len(page.Entries) != 1 {
		t.Errorf("by category: %+v", page.Entries)
	}
	page, _ = l.Query(Filter{MinLevel: slog.LevelDebug, Text: "INVALID DATA"})
	if len(page.Entries) != 1 {
		t.Errorf("by text: %+v", page.Entries)
	}
	// The live tail.
	last := page.Last
	log.Warn("newer")
	page, _ = l.Query(Filter{MinLevel: slog.LevelDebug, After: last})
	if len(page.Entries) != 1 || page.Entries[0].Msg != "newer" {
		t.Errorf("after: %+v", page.Entries)
	}
}

// Old messages come from the files.
func TestQueryHistory(t *testing.T) {
	l, log := newTest(t)
	var lines strings.Builder
	base := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 500; i++ {
		rec := Record{Time: base.Add(time.Duration(i) * time.Second), Lv: slog.LevelWarn, Cat: "tv", Session: "rec-1", Msg: fmt.Sprintf("old %d", i)}
		lines.Write(rec.appendLine(nil))
	}
	if err := os.WriteFile(filepath.Join(l.Dir(), "wms-2000-01-01.log"), []byte(lines.String()), 0o640); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(filepath.Join(l.Dir(), "wms-2000-01-01.log"), old, old)
	log.Warn("current", "cat", "tv", "session", "rec-1")

	page, err := l.Query(Filter{MinLevel: slog.LevelDebug, Session: "rec-1", History: true, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 50 || page.Entries[0].Msg != "old 451" || page.Entries[49].Msg != "current" || !page.More {
		t.Fatalf("%d entries, first %q, last %q, more %v", len(page.Entries), page.Entries[0].Msg, page.Entries[len(page.Entries)-1].Msg, page.More)
	}
	// Going further back.
	page2, err := l.Query(Filter{MinLevel: slog.LevelDebug, Session: "rec-1", History: true, Limit: 50, Before: page.Entries[0].Time})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 50 || page2.Entries[49].Msg != "old 450" {
		t.Fatalf("%+v", page2.Entries[len(page2.Entries)-1])
	}
}

func TestExplain(t *testing.T) {
	cases := map[string]string{
		"No such file or directory":                    "missing-file",
		"open x: The system cannot find the file":      "missing-file",
		"Permission denied":                            "permission",
		"moov atom not found":                          "damaged-file",
		"Invalid data found when processing input":     "damaged-file",
		"write: no space left on device":               "disk-full",
		"There is not enough space on the disk":        "disk-full",
		"Cannot load nvcuvid.dll":                      "hw-encoder",
		"database is locked":                           "db-busy",
		"database disk image is malformed":             "db-damaged",
		"CSS-scrambled":                                "encrypted",
		"bind: address already in use":                 "port-busy",
		"files have changed since they were installed": "verify-failed",
		"the runner failed":                            "game-runner",
		"core failed to load":                          "game-runner",
		"tuner did not answer":                         "tuner",
		"acme: error":                                  "tls",
		"connection refused":                           "network",
		"input/output error":                           "io-error",
		"MEDIA_ERR_DECODE":                             "player",
		"a part of the server crashed":                 "crash",
	}
	for text, want := range cases {
		got := Explain(&Record{Msg: "x", Attrs: []Attr{{"err", text}}})
		if got == nil || got.ID != want {
			t.Errorf("%q: got %+v, want %s", text, got, want)
		}
		if got != nil && (got.Title == "" || got.What == "" || got.Fix == "") {
			t.Errorf("%q: incomplete %+v", text, got)
		}
	}
	if got := Explain(&Record{Msg: "everything is fine"}); got != nil {
		t.Errorf("got %+v", got)
	}
}

func TestTailIsBounded(t *testing.T) {
	var tail Tail
	if !tail.Empty() {
		t.Error("should be empty")
	}
	_, _ = tail.Write([]byte("the start\n"))
	chunk := bytes.Repeat([]byte("middle "), 1000)
	for i := 0; i < 2000; i++ {
		_, _ = tail.Write(chunk)
	}
	_, _ = tail.Write([]byte("the very end"))
	s := tail.String()
	if !strings.HasPrefix(s, "the start") || !strings.HasSuffix(s, "the very end") || !strings.Contains(s, "bytes cut") {
		t.Errorf("%q...", s[:60])
	}
	if len(s) > tailHead+tailEnd+64 {
		t.Errorf("kept %d", len(s))
	}
	// One huge write.
	var t2 Tail
	_, _ = t2.Write(bytes.Repeat([]byte("z"), 10<<20))
	if len(t2.String()) > tailHead+tailEnd+64 {
		t.Errorf("kept %d", len(t2.String()))
	}
	if !strings.HasSuffix(t2.Last(5), "zzzzz") {
		t.Error("last")
	}
}

func TestExitReasonAndChildExit(t *testing.T) {
	l, log := newTest(t)
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	cmd := exec.Command(exe, "-test.run=TestHelperExit")
	cmd.Env = append(os.Environ(), "WMS_LOG_TEST_EXIT=7")
	var tail Tail
	cmd.Stderr = &tail
	start := time.Now()
	err = cmd.Run()
	if err == nil {
		t.Fatal("should have failed")
	}
	if got := ExitReason(err); got != "exit code 7" {
		t.Errorf("reason %q", got)
	}
	ChildExit(log.With("cat", "transcode", "session", "s-1"), "helper", err, false, &tail, start)
	ChildExit(log.With("cat", "transcode", "session", "s-2"), "helper", err, true, &tail, start) // on purpose: only at debug
	got := read(t, l)
	for _, want := range []string{"process ended", "exit code 7", "session=s-1", "going down now", "cat=transcode"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "session=s-2") {
		t.Errorf("a process stopped on purpose should be quiet:\n%s", got)
	}
	ChildExit(log.With("cat", "transcode", "session", "s-3"), "helper", nil, false, &tail, start) // finished, with something to say
	if got := read(t, l); !strings.Contains(got, "session=s-3") {
		t.Errorf("a process that finished after printing errors should be on record:\n%s", got)
	}
	if got := ExitReason(exec.ErrNotFound); !strings.Contains(got, "not found") {
		t.Errorf("%q", got)
	}
	if got := ExitReason(nil); got != "finished normally" {
		t.Errorf("%q", got)
	}
}

func TestHelperExit(t *testing.T) {
	if v := os.Getenv("WMS_LOG_TEST_EXIT"); v != "" {
		fmt.Fprintln(os.Stderr, "going down now")
		os.Exit(7)
	}
}

func TestRecoverAndLoop(t *testing.T) {
	l, log := newTest(t)
	slog.SetDefault(log)
	defer slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	guard, _, err := StartCrashGuard(t.TempDir(), l.Dir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()

	done := make(chan struct{})
	Go("a task", func() {
		defer close(done)
		panic("boom with token=PLANTEDSECRET")
	}, "cat", "tv", "session", "tv-1")
	<-done
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(read(t, l), "a part of the server crashed") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got := read(t, l)
	for _, want := range []string{"a part of the server crashed", "a task", "cat=tv", "session=tv-1", "level=CRASH", "stack="} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "PLANTEDSECRET") {
		t.Error("the panic's value leaked a secret")
	}
	crashes := guard.List()
	if len(crashes) != 1 || crashes[0].Kind != "panic" {
		t.Fatalf("reports: %+v", crashes)
	}
	body, err := guard.Read(crashes[0].ID)
	if err != nil || strings.Contains(string(body), "PLANTEDSECRET") || !strings.Contains(string(body), "goroutine") {
		t.Errorf("report: %v\n%s", err, body)
	}
	// The same panic again leaves no second report.
	done2 := make(chan struct{})
	Go("a task", func() {
		defer close(done2)
		panic("boom with token=PLANTEDSECRET")
	})
	<-done2
	time.Sleep(50 * time.Millisecond)
	if n := len(guard.List()); n != 1 {
		t.Errorf("%d reports", n)
	}

	// Seen, then gone.
	id := crashes[0].ID
	if err := guard.MarkSeen(id); err != nil {
		t.Fatal(err)
	}
	if !guard.List()[0].Seen {
		t.Error("not seen")
	}
	if err := guard.MarkSeen("../../etc/passwd"); err == nil {
		t.Error("a path was accepted as a report")
	}
	if _, err := guard.Read(`..\..\wms.db`); err == nil {
		t.Error("a path was accepted as a report")
	}
	if err := guard.Delete(id); err != nil || len(guard.List()) != 0 {
		t.Errorf("delete: %v", err)
	}

	// Loop runs it again after a panic.
	var mu sync.Mutex
	runs := 0
	finished := make(chan struct{})
	Loop("a loop", func() {
		mu.Lock()
		runs++
		n := runs
		mu.Unlock()
		if n == 1 {
			panic("first time")
		}
		close(finished)
	})
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("the loop didn't start again")
	}
}

// A real fatal crash, in a real process: the report survives it and the next
// start finds it. Also: a process that just vanishes.
func TestCrashSurvivesProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	crashDir, logDir := filepath.Join(dir, "crash"), filepath.Join(dir, "logs")
	run := func(mode string) (string, error) {
		cmd := exec.Command(exe, "-test.run=TestCrashHelper")
		cmd.Env = append(os.Environ(), "WMS_CRASH_TEST="+mode, "WMS_CRASH_DIR="+crashDir, "WMS_CRASH_LOGS="+logDir)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// A clean run leaves nothing to report.
	if out, err := run("clean"); err != nil {
		t.Fatalf("clean run failed: %v\n%s", err, out)
	}
	if g, found, err := StartCrashGuard(crashDir, logDir, "next"); err != nil || len(found) != 0 {
		t.Fatalf("after a clean run: %v %+v", err, found)
	} else {
		g.Close()
	}

	// A panic nobody catches, on a goroutine.
	out, err := run("panic")
	if err == nil {
		t.Fatalf("the helper should have crashed:\n%s", out)
	}
	g, found, err := StartCrashGuard(crashDir, logDir, "next")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	if len(found) != 1 || found[0].Kind != "fatal" || !strings.Contains(found[0].Summary, "the panic value") || found[0].Version != "crashing" {
		t.Fatalf("reports: %+v\nhelper said:\n%s", found, out)
	}
	body, err := g.Read(found[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"goroutine", "crashHelperFunction", "the last thing logged"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("report is missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "PLANTEDSECRET") {
		t.Error("the report holds a secret")
	}
	g.Close()

	// A fatal error that can't be recovered (no panic).
	if out, err := run("fatal"); err == nil {
		t.Fatalf("the helper should have crashed:\n%s", out)
	}
	g, found, err = StartCrashGuard(crashDir, logDir, "next")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	if len(found) != 1 || found[0].Kind != "fatal" || !strings.Contains(found[0].Summary, "internal error in") || !strings.Contains(found[0].Summary, "TestCrashHelper") {
		t.Fatalf("reports: %+v", found)
	}
	g.Close()

	// Killed, power cut: no last words at all.
	if out, err := run("vanish"); err == nil {
		t.Fatalf("the helper should have vanished:\n%s", out)
	}
	g, found, err = StartCrashGuard(crashDir, logDir, "next")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(g.Close)
	if len(found) != 1 || found[0].Kind != "unclean" || !strings.Contains(found[0].Summary, "without shutting down") {
		t.Fatalf("reports: %+v", found)
	}
	g.Close()
	if n := len(func() []Crash { g, _, _ := StartCrashGuard(crashDir, logDir, "x"); defer g.Close(); return g.List() }()); n != 3 {
		t.Errorf("%d reports kept", n)
	}
}

func crashHelperFunction() {
	panic("the panic value with token=PLANTEDSECRET")
}

func TestCrashHelper(t *testing.T) {
	mode := os.Getenv("WMS_CRASH_TEST")
	if mode == "" {
		return
	}
	l, err := New(os.Getenv("WMS_CRASH_LOGS"), slog.LevelInfo, nil)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(l.Handler())
	log.Error("the last thing logged")
	l.Flush()
	g, _, err := StartCrashGuard(os.Getenv("WMS_CRASH_DIR"), os.Getenv("WMS_CRASH_LOGS"), "crashing")
	if err != nil {
		t.Fatal(err)
	}
	switch mode {
	case "clean":
		g.Close()
		l.Close()
		return
	case "panic":
		go crashHelperFunction()
		select {}
	case "fatal":
		// Unlocking what isn't locked stops the program with a fatal error that recover
		// can't catch.
		var mu sync.Mutex
		mu.Unlock()
	case "vanish":
		os.Exit(9) // nothing about it reaches the marker
	}
}

func TestBundleRedactsTitlesAndPaths(t *testing.T) {
	l, log := newTest(t)
	const title = "A Very Secret Movie Title"
	win := `D:\Media Library\Movies\` + title + ` (2004)\` + title + `.mkv`
	unix := `/mnt/media/Movies/` + title + `.mkv`
	log.Warn("could not open", "file", win, "title", title, "user", "alice", "from", "192.168.1.20")
	log.Warn("failed", "err", "open "+win+": permission denied")
	log.Warn("failed", "err", "open "+unix+": permission denied")
	log.Warn("ffmpeg stopped", "stderr", "Input #0, matroska, from '"+win+"':\n  Metadata:\n    title           : "+title+"\n  Duration: 01:00:00")
	log.Warn("in a web address", "url", "http://tuner/"+title+"/stream")
	log.Warn("http", "p", "/api/items/5/play", "s", 500)
	guard, _, err := StartCrashGuard(t.TempDir(), l.Dir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()

	opts := BundleOptions{Days: 1, Roots: []string{`D:\Media Library`}, Extra: []BundleFile{{Name: "summary.json", Description: "s", Data: []byte(`{"folders":[{"role":"library","path":"` + strings.ReplaceAll(win, `\`, `\\`) + `"}],"version":"1"}`)}}}
	var plain bytes.Buffer
	if err := l.WriteBundle(&plain, guard, opts); err != nil {
		t.Fatal(err)
	}
	text := unzipAll(t, plain.Bytes())
	if !strings.Contains(text, title) {
		t.Error("the unredacted bundle should have the title: it's the owner's own log")
	}
	for _, want := range []string{"README.txt", "summary.json", "logs/wms-"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %s", want)
		}
	}

	opts.Redact = true
	var red bytes.Buffer
	if err := l.WriteBundle(&red, guard, opts); err != nil {
		t.Fatal(err)
	}
	text = unzipAll(t, red.Bytes())
	for _, not := range []string{title, "Media Library", "alice", "192.168.1.20", "Secret"} {
		if strings.Contains(text, not) {
			t.Errorf("%q survived redaction:\n%s", not, text)
		}
	}
	for _, want := range []string{"permission denied", "/api/items/5/play", "Duration: 01:00:00", "[path:", `"version": "1"`} {
		if !strings.Contains(text, want) {
			t.Errorf("redaction took too much, %q is gone:\n%s", want, text)
		}
	}
	// The preview says the same.
	p := l.Preview(guard, opts)
	if len(p.Parts) < 3 || p.Total == 0 || len(p.Sample) == 0 {
		t.Fatalf("%+v", p)
	}
	for _, line := range p.Sample {
		if strings.Contains(line, title) {
			t.Errorf("preview shows the title: %s", line)
		}
	}
}

func TestHandlerWithGroupAndAttrs(t *testing.T) {
	l, log := newTest(t)
	g := log.With("cat", "tv", "session", "s1", "a", 1).WithGroup("grp").With("b", 2)
	g.Warn("hello", "c", 3, slog.Group("sub", "d", 4))
	got := read(t, l)
	for _, want := range []string{"cat=tv", "session=s1", " a=1", "grp.b=2", "grp.c=3", "grp.sub.d=4"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// How much logging costs. See docs/LOGGING.md.

func benchLogger(b *testing.B) (*Logger, *slog.Logger) {
	b.Helper()
	l, err := New(b.TempDir(), slog.LevelInfo, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(l.Close)
	return l, slog.New(l.Handler())
}

// A debug line in a quiet category: what a hot loop pays.
func BenchmarkQuietCategory(b *testing.B) {
	_, log := benchLogger(b)
	sub := log.With("cat", "discs", "session", "abc")
	b.ReportAllocs()
	for b.Loop() {
		sub.Debug("step", "cell", 5, "pgc", 3)
	}
}

// A line that is written, with a few attributes: what an event pays.
func BenchmarkWrittenLine(b *testing.B) {
	_, log := benchLogger(b)
	sub := log.With("cat", "transcode", "session", "abc")
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		sub.Info("segment finished", "n", i%37, "ms", 120, "file", `D:\Media\Movie (2004)\Movie.mkv`)
	}
}

// The same quiet line while some other module has settings of its own.
func BenchmarkQuietWithModuleSettings(b *testing.B) {
	l, log := benchLogger(b)
	l.SetModules(map[string]ModuleLog{"wittyphotos": {Level: "debug"}})
	sub := log.With("cat", "discs", "session", "abc", "module", "wittydvd")
	b.ReportAllocs()
	for b.Loop() {
		sub.Debug("step", "cell", 5, "pgc", 3)
	}
}

func BenchmarkRedactPlainText(b *testing.B) {
	s := "the scanner could not find anything to read"
	b.ReportAllocs()
	for b.Loop() {
		_ = Redact(s)
	}
}

func BenchmarkRedactAddress(b *testing.B) {
	s := `Get "https://api.example.com/3/search/movie?api_key=abc123&query=some+words": context deadline exceeded`
	b.ReportAllocs()
	for b.Loop() {
		_ = Redact(s)
	}
}
