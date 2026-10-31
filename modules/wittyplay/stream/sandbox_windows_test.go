package stream

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	if strings.Contains(got.Error, "could not check") {
		t.Fatalf("the check itself failed: %+v", got)
	}
	t.Logf("%+v", got)
}
