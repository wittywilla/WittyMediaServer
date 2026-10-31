package stream

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittyplay/cores"
	"github.com/wittywilla/wittyplay/testcore/fakecore"
)

// buildProbeRunner builds the real wittyplay-host once per test run.
func buildProbeRunner(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "wittyplay-host")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	b := exec.Command("go", "build", "-o", out, "../cmd/wittyplay-host")
	b.Env = append(os.Environ(), "CGO_ENABLED=0")
	if msg, err := b.CombinedOutput(); err != nil {
		t.Fatalf("building the runner: %v\n%s", err, msg)
	}
	return out
}

func fake(t *testing.T, v fakecore.Variant) string {
	t.Helper()
	p, err := fakecore.Build(t.TempDir(), "fake_libretro", v)
	if errors.Is(err, fakecore.ErrNoCompiler) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Every way a core can fail its test-load, through the real runner and the real
// sandbox: a crash is an error with words in it, and this test process keeps going.
func TestProbeHostileCores(t *testing.T) {
	runner := buildProbeRunner(t)
	probe := func(t *testing.T, core string, timeout time.Duration) (*ProbeResult, error) {
		return Probe(context.Background(), ProbeOptions{Runner: runner, Core: core, WorkDir: filepath.Join(t.TempDir(), "box"), SystemDir: t.TempDir(), Timeout: timeout})
	}

	t.Run("a good core reports itself", func(t *testing.T) {
		res, err := probe(t, fake(t, fakecore.Good), 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if res.Name != "WittyPlay fake core" || res.Version != "1.2.3" || strings.Join(res.Extensions, ",") != "wittytest,fake" {
			t.Errorf("it said %+v", res.Probe)
		}
	})
	for _, c := range []struct {
		name    string
		v       fakecore.Variant
		kind    string
		timeout time.Duration
		says    string
	}{
		{"crashes when started", fakecore.CrashAtInit, ProbeCrashed, 90 * time.Second, "crashed"},
		{"hangs when started", fakecore.HangAtInit, ProbeHung, 2 * time.Second, "stopped"},
		{"hangs when opened", fakecore.HangAtOpen, ProbeHung, 2 * time.Second, "stopped"},
		{"missing functions", fakecore.MissingSymbols, ProbeRefused, 90 * time.Second, "libretro core"},
		{"wrong API version", fakecore.WrongAPI, ProbeRefused, 90 * time.Second, "version"},
	} {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			_, err := probe(t, fake(t, c.v), c.timeout)
			var pe *ProbeError
			if !errors.As(err, &pe) {
				t.Fatalf("it should have failed plainly, and got %v", err)
			}
			if pe.Kind != c.kind || !strings.Contains(pe.Reason, c.says) {
				t.Errorf("got %s: %s", pe.Kind, pe.Reason)
			}
			if time.Since(start) > c.timeout+10*time.Second {
				t.Errorf("that took %v", time.Since(start))
			}
		})
	}
	// A crash while the library opens is either a crash or a refused load, depending
	// on the system's loader, and neither may come back as success.
	t.Run("crashes when opened", func(t *testing.T) {
		_, err := probe(t, fake(t, fakecore.CrashAtOpen), 20*time.Second)
		var pe *ProbeError
		if !errors.As(err, &pe) || (pe.Kind != ProbeCrashed && pe.Kind != ProbeRefused) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("what a hostile core reports comes back cleaned", func(t *testing.T) {
		res, err := probe(t, fake(t, fakecore.ChattyName), 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		clean := cores.CleanProbe(res.Probe)
		if len([]rune(clean.Name)) > 120 || strings.ContainsAny(clean.Name, "\x07\x1b‮") {
			t.Errorf("name = %q", clean.Name)
		}
	})
	t.Run("the runner missing is said plainly", func(t *testing.T) {
		_, err := Probe(context.Background(), ProbeOptions{Core: "x", WorkDir: t.TempDir()})
		var pe *ProbeError
		if !errors.As(err, &pe) || pe.Kind != ProbeRunner {
			t.Fatalf("got %v", err)
		}
	})
}

// A runner that writes nonsense on its protocol pipe, standing in for a core that
// takes over stdout: what comes back is a refusal, and nothing is allocated for it.
func TestProbeJunkOutput(t *testing.T) {
	ans := readProbe(strings.NewReader(string([]byte{250, 255, 255, 255, 255, 1, 2, 3})))
	if ans.err == nil || ans.err.Kind != ProbeInvalid {
		t.Fatalf("got %+v", ans)
	}
	if ans := readProbe(strings.NewReader("")); ans.probe != nil || ans.err != nil {
		t.Fatalf("an empty pipe is a process that went away, not an answer: %+v", ans)
	}
}

func TestExitDetail(t *testing.T) {
	if exitDetail(errors.New("nope")) != "" {
		t.Error("only an exit status has a detail")
	}
}
