package modules

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymodule"
)

func TestHelperOutputLines(t *testing.T) {
	var out bytes.Buffer
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	log := slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: level}))
	tail := &logging.Tail{}
	o := &helperOutput{tail: tail, log: log}

	// Off: kept, and nothing logged or held on to.
	if _, err := o.Write([]byte("first\npartial")); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || len(o.line) != 0 {
		t.Fatalf("logged %q, held %q", out.String(), o.line)
	}
	if !strings.Contains(tail.String(), "first") {
		t.Fatalf("tail: %q", tail.String())
	}

	// On: line by line, however the writes are cut.
	level.Set(slog.LevelDebug)
	_, _ = o.Write([]byte("sec"))
	_, _ = o.Write([]byte("ond\r\nthird\n"))
	_, _ = o.Write([]byte("no end"))
	o.flush()
	got := out.String()
	for _, want := range []string{"output=second", "output=third", `output="no end"`, "topic=output"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in: %s", want, got)
		}
	}
	if strings.Count(got, "helper says") != 3 {
		t.Errorf("want 3 lines: %s", got)
	}

	// A line without end is cut, whatever arrives.
	out.Reset()
	for i := 0; i < 5; i++ {
		_, _ = o.Write(bytes.Repeat([]byte("x"), maxOutputLine))
		if len(o.line) > maxOutputLine {
			t.Fatalf("holding %d bytes", len(o.line))
		}
	}
	_, _ = o.Write([]byte("\n"))
	if n := strings.Count(out.String(), "x"); n != maxOutputLine {
		t.Fatalf("logged %d of a long line", n)
	}
}

func TestProcPassesLogSettings(t *testing.T) {
	p := newProc(&wittymodule.Manifest{ID: "wittyphotos"}, t.TempDir(), nil, slog.Default())
	if env := strings.Join(p.env(), "\n"); strings.Contains(env, "WITTYMODULE_LOG") {
		t.Fatalf("no settings, but: %s", env)
	}
	var asked string
	p.logEnv = func(id string) []string {
		asked = id
		return []string{"WITTYMODULE_LOG=debug"}
	}
	if env := strings.Join(p.env(), "\n"); !strings.Contains(env, "WITTYMODULE_LOG=debug") || asked != "wittyphotos" {
		t.Fatalf("env %s, asked %q", env, asked)
	}
}
