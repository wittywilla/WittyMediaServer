package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/instance"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

func TestReadTrayStop(t *testing.T) {
	for line, want := range map[string]string{
		"stop tray-quit\n":       logging.StopTrayQuit,
		"stop os-shutdown":       logging.StopShutdown,
		"stop tray-stop\r\n":     logging.StopTrayStop,
		"hello\n":                "",
		"stop something-made-up": "",
		"stop api\n":             "", // not the tray's to say
	} {
		if got := trayStopReason(line); got != want {
			t.Errorf("%q: %q, want %q", line, got, want)
		}
	}
	// A line far too long to be the tray's is skipped whole, and reading carries on.
	readTrayStop(strings.NewReader(strings.Repeat("x", 1000) + "stop tray-quit\nstop tray-restart\n"))
	if got := logging.StopNoted().Reason; got != logging.StopTrayRestart {
		t.Fatalf("after a long line: %q", got)
	}
}

// What a stop says, and what the next start says about it: the tray app's Quit, then a
// kill that leaves no time to say anything.
func TestStopAndStartSayWhy(t *testing.T) {
	if testing.Short() {
		t.Skip("starts real servers")
	}
	needFFmpeg(t)
	dir := filepath.Join(t.TempDir(), "data")

	first := startServer(t, dir, freePort(t), "-watch-stdin")
	if r := instance.Find(context.Background(), dir, 90*time.Second); !r.Answering {
		t.Fatalf("didn't come up:\n%s", first.out.String())
	}
	if _, err := first.stdin.Write([]byte("stop tray-quit\n")); err != nil {
		t.Fatal(err)
	}
	_ = first.stdin.Close()
	if code := first.exitWithin(t, 60*time.Second); code != 0 {
		t.Fatalf("exit %d:\n%s", code, first.out.String())
	}
	if out := first.out.String(); !strings.Contains(out, "stopping") || !strings.Contains(out, "tray-quit") || !strings.Contains(out, "Quit was chosen in the tray icon") {
		t.Fatalf("no reason in the stop:\n%s", out)
	}

	second := startServer(t, dir, freePort(t))
	if r := instance.Find(context.Background(), dir, 90*time.Second); !r.Answering {
		t.Fatalf("didn't come up:\n%s", second.out.String())
	}
	if out := second.out.String(); !strings.Contains(out, "(tray-quit), ran ") {
		t.Fatalf("the start doesn't say how the last run ended:\n%s", out)
	}
	_ = second.cmd.Process.Kill()
	_ = second.cmd.Wait()

	third := startServer(t, dir, freePort(t))
	if r := instance.Find(context.Background(), dir, 90*time.Second); !r.Answering {
		t.Fatalf("didn't come up:\n%s", third.out.String())
	}
	if out := third.out.String(); !strings.Contains(out, "the last run ended without stopping cleanly") {
		t.Fatalf("a kill wasn't noticed:\n%s", out)
	}
	_ = third.cmd.Process.Kill()
}
