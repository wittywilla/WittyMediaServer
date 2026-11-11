// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package instance

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLockBusyAndReleased(t *testing.T) {
	dir := t.TempDir()
	a, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(dir); !errors.Is(err, ErrBusy) {
		t.Fatalf("second lock: %v, want busy", err)
	}
	if !Held(dir) {
		t.Fatal("Held says free while locked")
	}
	if err := a.Publish(Info{PID: os.Getpid(), URLs: []string{"http://localhost:1"}}); err != nil {
		t.Fatal(err)
	}
	info, err := Read(dir)
	if err != nil || info.Instance != a.ID() || info.Instance == "" {
		t.Fatalf("instance.json: %+v %v", info, err)
	}
	a.Release()
	if Held(dir) {
		t.Fatal("Held says busy after release")
	}
	if _, err := os.Stat(filepath.Join(dir, InfoFile)); !os.IsNotExist(err) {
		t.Fatalf("instance.json left after release: %v", err)
	}
	b, err := Acquire(dir)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	b.Release()
	b.Release() // twice is harmless
}

// Release leaves another run's instance.json alone.
func TestReleaseKeepsOthersInfo(t *testing.T) {
	dir := t.TempDir()
	a, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, InfoFile), []byte(`{"instance":"someone-else"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	a.Release()
	if _, err := os.Stat(filepath.Join(dir, InfoFile)); err != nil {
		t.Fatal("removed an instance.json that wasn't this run's")
	}
}

// The helper process: takes the lock, says so, and waits to be killed.
func TestMain(m *testing.M) {
	if dir := os.Getenv("WMS_INSTANCE_HOLD"); dir != "" {
		l, err := Acquire(dir)
		if err != nil {
			os.Stdout.WriteString("error " + err.Error() + "\n")
			os.Exit(1)
		}
		_ = l.Publish(Info{PID: os.Getpid()})
		os.Stdout.WriteString("locked\n")
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// A process that dies without letting go (kill -9, TerminateProcess) leaves nothing that
// blocks the next start.
func TestLockFreedWhenProcessKilled(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "WMS_INSTANCE_HOLD="+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := out.Read(buf)
	if !strings.HasPrefix(string(buf[:n]), "locked") {
		_ = cmd.Process.Kill()
		t.Fatalf("helper: %q", buf[:n])
	}
	if _, err := Acquire(dir); !errors.Is(err, ErrBusy) {
		t.Fatalf("lock while the helper holds it: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil { // SIGKILL; TerminateProcess on Windows
		t.Fatal(err)
	}
	_ = cmd.Wait()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatalf("lock after the holder was killed: %v", err)
	}
	l.Release()
	// The killed run's instance.json is still there, and harmless.
	if info, err := Read(dir); err == nil && info.PID == os.Getpid() {
		t.Fatal("unexpected pid")
	}
}

func TestAcquireWait(t *testing.T) {
	dir := t.TempDir()
	a, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(400 * time.Millisecond); a.Release() }()
	b, err := AcquireWait(dir, 5*time.Second)
	if err != nil {
		t.Fatalf("after the first let go: %v", err)
	}
	b.Release()
	c, _ := Acquire(dir)
	defer c.Release()
	start := time.Now()
	if _, err := AcquireWait(dir, 300*time.Millisecond); !errors.Is(err, ErrBusy) || time.Since(start) > 3*time.Second {
		t.Fatalf("busy: %v after %v", err, time.Since(start))
	}
}

func health(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "instance": id})
	}
}

func TestFindAnswering(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	ts := httptest.NewServer(health(l.ID()))
	defer ts.Close()
	if err := l.Publish(Info{PID: os.Getpid(), URLs: []string{"http://127.0.0.1:1", ts.URL}}); err != nil {
		t.Fatal(err)
	}
	r := Find(context.Background(), dir, 3*time.Second)
	if !r.Answering || r.URL != ts.URL || r.PID != os.Getpid() {
		t.Fatalf("found %+v", r)
	}
	if line := Line(r, dir); !strings.Contains(line, ts.URL) || !strings.Contains(line, "already running") {
		t.Fatalf("line %q", line)
	}
}

// Some other program on the port, or another server, doesn't count as the one on this folder.
func TestFindNotFooledByAnotherProgram(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	other := httptest.NewServer(health("not-this-one"))
	defer other.Close()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello")) }))
	defer plain.Close()
	if err := l.Publish(Info{PID: os.Getpid(), URLs: []string{other.URL, plain.URL}}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	r := Find(context.Background(), dir, 700*time.Millisecond)
	if r.Answering || r.PID != os.Getpid() {
		t.Fatalf("found %+v", r)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("waited %v", time.Since(start))
	}
	if line := Line(r, dir); !strings.Contains(line, "not answering") {
		t.Fatalf("line %q", line)
	}
	if id, ok := HealthID(context.Background(), other.URL); !ok || id != "not-this-one" {
		t.Fatalf("HealthID %q %v", id, ok)
	}
	if _, ok := HealthID(context.Background(), plain.URL); ok {
		t.Fatal("a program that isn't a server counted as one")
	}
}

func TestFindNoInfoYet(t *testing.T) {
	r := Find(context.Background(), t.TempDir(), 400*time.Millisecond)
	if r.Answering || r.PID != 0 {
		t.Fatalf("found %+v", r)
	}
	if line := Line(r, "x"); !strings.Contains(line, "hasn't said where") {
		t.Fatalf("line %q", line)
	}
}

func TestFreePort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	taken := ln.Addr().(*net.TCPAddr).Port
	if p := FreePort("127.0.0.1", taken); p == taken || p == 0 {
		t.Fatalf("FreePort gave %d with %d taken", p, taken)
	}
	if p := FreePort("127.0.0.1", taken-1, taken-1); p == taken-1 || p == taken {
		t.Fatalf("FreePort gave a skipped or taken port %d", p)
	}
}

func TestSuggestFolder(t *testing.T) {
	base := filepath.Join(t.TempDir(), "WittyMediaServer")
	if got := SuggestFolder(base); got != base+"-2" {
		t.Fatalf("got %s", got)
	}
	l, err := Acquire(base + "-2")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if got := SuggestFolder(base); got != base+"-3" {
		t.Fatalf("with -2 in use, got %s", got)
	}
}

// fakeAsk answers the questions in turn and records them.
type fakeAsk struct {
	answers []int
	asked   []string
}

func (f *fakeAsk) ask(title, text string, buttons []string) int {
	f.asked = append(f.asked, title+" | "+strings.Join(buttons, ","))
	if len(f.answers) == 0 {
		return -1
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return a
}

func question(f *fakeAsk) Question {
	return Question{
		Running:  Running{Info: Info{PID: 1, Started: time.Date(2026, 11, 4, 14, 2, 0, 0, time.Local)}, URL: "http://localhost:9797", Answering: true},
		Dir:      filepath.Join(os.TempDir(), "wms-test-data"),
		Ask:      f.ask,
		FreePort: func() int { return 9798 },
		InUse:    func(string) bool { return false },
		Suggest:  func(d string) string { return d + "-2" },
		Now:      time.Date(2026, 11, 4, 16, 0, 0, 0, time.Local),
	}
}

func TestDecide(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answers []int
		pick    func() (string, error)
		inUse   func(string) bool
		want    Decision
		asks    int
	}{
		{name: "open is the default", answers: []int{0}, want: Decision{Choice: ChoiceOpen}, asks: 1},
		{name: "escape exits", answers: []int{-1}, want: Decision{Choice: ChoiceExit}, asks: 1},
		{name: "exit", answers: []int{2}, want: Decision{Choice: ChoiceExit}, asks: 1},
		{name: "another, suggested folder", answers: []int{1, 0}, want: Decision{Choice: ChoiceAnother, Folder: filepath.Join(os.TempDir(), "wms-test-data") + "-2", Port: 9798}, asks: 2},
		{name: "another, back, open", answers: []int{1, 2, 0}, want: Decision{Choice: ChoiceOpen}, asks: 3},
		{name: "another, escape goes back", answers: []int{1, -1, 2}, want: Decision{Choice: ChoiceExit}, asks: 3},
		{name: "another, chosen folder", answers: []int{1, 1, 0}, pick: func() (string, error) { return "/srv/second", nil }, want: Decision{Choice: ChoiceAnother, Folder: "/srv/second", Port: 9798}, asks: 3},
		{name: "another, picker cancelled", answers: []int{1, 1, 0}, pick: func() (string, error) { return "", errors.New("cancelled") }, want: Decision{Choice: ChoiceAnother, Folder: filepath.Join(os.TempDir(), "wms-test-data") + "-2", Port: 9798}, asks: 3},
		{name: "another, the running folder refused", answers: []int{1, 1, 0, 0}, pick: func() (string, error) { return filepath.Join(os.TempDir(), "wms-test-data"), nil }, want: Decision{Choice: ChoiceAnother, Folder: filepath.Join(os.TempDir(), "wms-test-data") + "-2", Port: 9798}, asks: 4},
		{name: "another, a folder in use refused", answers: []int{1, 1, 0, 0}, pick: func() (string, error) { return "/srv/busy", nil }, inUse: func(d string) bool { return d == "/srv/busy" }, want: Decision{Choice: ChoiceAnother, Folder: filepath.Join(os.TempDir(), "wms-test-data") + "-2", Port: 9798}, asks: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeAsk{answers: tc.answers}
			q := question(f)
			q.Pick = tc.pick
			if tc.inUse != nil {
				q.InUse = tc.inUse
			}
			got := Decide(q)
			if got != tc.want {
				t.Fatalf("got %+v, want %+v (asked %q)", got, tc.want, f.asked)
			}
			if len(f.asked) != tc.asks {
				t.Fatalf("asked %d times: %q", len(f.asked), f.asked)
			}
			if !strings.HasPrefix(f.asked[0], TitleRunning+" | "+ButtonOpen+",") {
				t.Fatalf("first question %q", f.asked[0])
			}
		})
	}
}

func TestDecideNoPortOrPicker(t *testing.T) {
	f := &fakeAsk{answers: []int{1, 0, 2}}
	q := question(f)
	q.FreePort = func() int { return 0 }
	if got := Decide(q); got.Choice != ChoiceExit || len(f.asked) != 3 || !strings.HasSuffix(f.asked[1], "| OK") {
		t.Fatalf("no port: %+v %q", got, f.asked)
	}
	f = &fakeAsk{answers: []int{1, 0}}
	q = question(f)
	if got := Decide(q); got.Choice != ChoiceAnother || strings.Contains(f.asked[1], ButtonChoose) {
		t.Fatalf("without a picker: %+v %q", got, f.asked)
	}
}

// A person who never decides doesn't keep the program going forever.
func TestDecideBounded(t *testing.T) {
	q := question(&fakeAsk{})
	n := 0
	q.Ask = func(string, string, []string) int { n++; return 1 + n%2 } // another, back, another…
	if got := Decide(q); got.Choice != ChoiceExit || n > 100 {
		t.Fatalf("%+v after %d", got, n)
	}
}

func TestRunningText(t *testing.T) {
	now := time.Date(2026, 11, 4, 16, 0, 0, 0, time.Local)
	r := Running{Info: Info{Started: time.Date(2026, 11, 4, 14, 2, 0, 0, time.Local)}, URL: "http://localhost:9797"}
	if s := RunningText(r, now); s != "It's at http://localhost:9797 and has been running since 14:02." {
		t.Fatal(s)
	}
	r.Started = time.Date(2026, 11, 2, 9, 5, 0, 0, time.Local)
	if s := RunningText(r, now); !strings.Contains(s, "2 November 09:05") {
		t.Fatal(s)
	}
	if s := AnotherText("/x", 9798); !strings.Contains(s, "/x") || !strings.Contains(s, strconv.Itoa(9798)) {
		t.Fatal(s)
	}
}

// The lock file names its holder, so an instance.json left by a killed run isn't believed.
func TestHolderAndStaleInfo(t *testing.T) {
	dir := t.TempDir()
	if _, running, known := HeldBy(dir); running || known {
		t.Fatal("a folder never locked")
	}
	// What a killed run left: an instance.json with its process number and a dead address.
	if err := os.WriteFile(filepath.Join(dir, InfoFile), []byte(`{"pid": 999999, "urls": ["http://127.0.0.1:1"], "instance": "old"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if h := Holder(dir); h != os.Getpid() {
		t.Fatalf("holder %d, want %d", h, os.Getpid())
	}
	r := Find(context.Background(), dir, 500*time.Millisecond)
	if r.PID != os.Getpid() || r.Instance == "old" {
		t.Fatalf("believed a killed run's instance.json: %+v", r)
	}
	if !strings.Contains(Line(r, dir), "process "+strconv.Itoa(os.Getpid())) {
		t.Fatal(Line(r, dir))
	}
	// This process holding it isn't "another server" to a repair it runs itself.
	if _, running, known := HeldBy(dir); running || !known {
		t.Fatalf("own lock: running %v known %v", running, known)
	}
}

func TestRunningSeesAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "WMS_INSTANCE_HOLD="+dir)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	buf := make([]byte, 64)
	if n, _ := out.Read(buf); !strings.HasPrefix(string(buf[:n]), "locked") {
		t.Fatalf("helper: %q", buf[:n])
	}
	if pid, running, known := HeldBy(dir); !running || !known || pid != cmd.Process.Pid {
		t.Fatalf("pid %d running %v known %v, want %d", pid, running, known, cmd.Process.Pid)
	}
}

// A second server gets a config of its own, so nothing in the first one's points it at the
// first one's folders.
func TestPrepareAnother(t *testing.T) {
	t.Setenv("WMS_TRANSCODE_DIR", "/the/first/servers/transcode")
	folder := filepath.Join(t.TempDir(), "WittyMediaServer-2")
	args, err := PrepareAnother([]string{"-config", "/first/wms.json"}, ":9797", Decision{Choice: ChoiceAnother, Folder: folder, Port: 9798})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-config", "/first/wms.json", "-config", filepath.Join(folder, "wms.json"), "-data", folder, "-listen", ":9798", "-log-dir", filepath.Join(folder, "logs")}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("args %q", args)
	}
	var conf map[string]string
	b, err := os.ReadFile(filepath.Join(folder, "wms.json"))
	if err != nil || json.Unmarshal(b, &conf) != nil || conf["dataDir"] != folder || conf["listen"] != ":9798" {
		t.Fatalf("wms.json: %s %v", b, err)
	}
	if os.Getenv("WMS_TRANSCODE_DIR") != "" {
		t.Fatal("the first server's transcode folder is still set")
	}
	// Again on the same folder: its wms.json, which may have been changed since, is kept.
	if err := os.WriteFile(filepath.Join(folder, "wms.json"), []byte(`{"dataDir": "kept"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareAnother(nil, "127.0.0.1:9797", Decision{Folder: folder, Port: 9799}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(folder, "wms.json")); !strings.Contains(string(b), "kept") {
		t.Fatalf("overwrote %s", b)
	}
}
