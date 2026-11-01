package stream

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Every game starts a runner, and the server runs for months.
func TestStartingARunnerLeavesNoHandleBehind(t *testing.T) {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetProcessHandleCount")
	count := func() uint32 {
		var n uint32
		if r, _, err := proc.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&n))); r == 0 {
			t.Fatal(err)
		}
		return n
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	save := t.TempDir()
	round := func() {
		cmd := exec.Command("no-such-program")
		prepareSandbox(cmd, save, log)
		sandboxStarted(cmd)
	}
	round() // anything that's only loaded the once
	before := count()
	for range 200 {
		round()
	}
	if after := count(); after > before+50 {
		t.Errorf("%d handles became %d in 200 starts", before, after)
	}
}

// The token and job the server gives a runner, tried on a copy of this test
// program: it may write to its save folder and nowhere else, and it can't start
// another program.
func TestWindowsSandbox(t *testing.T) {
	if os.Getenv("WITTYPLAY_SANDBOX_CHILD") == "1" {
		save := os.Getenv("WITTYPLAY_SAVE")
		if err := os.WriteFile(filepath.Join(save, "ok"), []byte("x"), 0o644); err != nil {
			t.Fatalf("the save folder must stay writable: %v", err)
		}
		outside := filepath.Join(os.Getenv("WITTYPLAY_OUTSIDE"), "escape")
		if err := os.WriteFile(outside, []byte("x"), 0o644); err == nil {
			t.Fatal("writing outside the save folder should fail")
		}
		if err := exec.Command(os.Args[0], "-test.list=x").Run(); err == nil {
			t.Fatal("starting a program should fail inside the job")
		}
		return
	}
	save, outside := t.TempDir(), t.TempDir()
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	cmd := exec.Command(os.Args[0], "-test.run=TestWindowsSandbox$", "-test.v")
	prepareSandbox(cmd, save, log)
	cmd.Env = append(os.Environ(), "WITTYPLAY_SANDBOX_CHILD=1", "WITTYPLAY_SAVE="+save, "WITTYPLAY_OUTSIDE="+outside)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// The child is racing the job here, but the run takes far longer than this.
	release := lockDown(cmd, 2048, log)
	defer release()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("%v\n%s\nlogs: %s", err, out.String(), logs.String())
	}
	if logs.Len() > 0 {
		t.Fatalf("the sandbox did not fully apply: %s", logs.String())
	}
	if _, err := os.Stat(filepath.Join(outside, "escape")); err == nil {
		t.Fatal("the child wrote outside its save folder")
	}
}

// The graphics card check runs in the same sandbox as a game, with no pipes of its
// own to claim from, and stays out of every other folder.
func TestProbeGPUInSandbox(t *testing.T) {
	got := ProbeGPU(buildRunner(t), filepath.Join(t.TempDir(), "probe"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if got.Again || strings.Contains(got.Error, "could not check") {
		t.Fatalf("the check itself failed: %+v", got)
	}
	t.Logf("%+v", got)
}

func TestAPathTooLongForWindowsIsSaidPlainly(t *testing.T) {
	deep := `C:\` + strings.Repeat("d", 250)
	_, err := Probe(context.Background(), ProbeOptions{Runner: "wittyplay-host", Core: `C:\core.dll`, WorkDir: deep})
	var pe *ProbeError
	if !errors.As(err, &pe) || !strings.Contains(pe.Reason, "shorter path") {
		t.Errorf("got %v", err)
	}
}
