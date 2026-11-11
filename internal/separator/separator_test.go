// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package separator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func fake(t *testing.T, dir, name, script string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDiscover(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in programs are shell scripts")
	}
	ctx := context.Background()
	opt := func(dirs ...string) Options { return Options{Dirs: dirs, Timeout: 10 * time.Second} }

	empty := t.TempDir()
	if f := Discover(ctx, opt(empty)); f != nil {
		t.Fatalf("found %+v in an empty folder", f)
	}
	// A relative folder on the PATH is never searched.
	if f := Discover(ctx, opt(".", "")); f != nil {
		t.Fatalf("found %+v in a relative folder", f)
	}

	// Spleeter alone, saying its version.
	a := t.TempDir()
	sp := fake(t, a, "spleeter", `[ "$1" = --version ] && echo "Spleeter Version: 2.4.0"`)
	f := Discover(ctx, opt(empty, a))
	if f == nil || f.Name != "Spleeter" || f.Version != "2.4.0" || f.Path != sp || f.Words[0] != sp {
		t.Fatalf("spleeter: %+v", f)
	}
	if j := strings.Join(f.Words, " "); !strings.Contains(j, "{input}") || !strings.Contains(j, "{output}") {
		t.Errorf("the command has no placeholders: %v", f.Words)
	}

	// Demucs wins over Spleeter wherever it is on the path, and has no version to say.
	b := t.TempDir()
	dm := fake(t, b, "demucs", `exit 2`)
	f = Discover(ctx, opt(a, b))
	if f == nil || f.Name != "Demucs" || f.Version != "" || f.Path != dm {
		t.Fatalf("demucs: %+v", f)
	}

	// Python's module, when there's no launcher: the import must work.
	c := t.TempDir()
	py := fake(t, c, "python3", `[ "$1" = -c ] && echo 4.0.1`)
	f = Discover(ctx, opt(c, a))
	if f == nil || f.Name != "Demucs" || f.Version != "4.0.1" || len(f.Words) < 3 || f.Words[0] != py || f.Words[1] != "-m" || f.Words[2] != "demucs" {
		t.Fatalf("python -m demucs: %+v", f)
	}
	d := t.TempDir()
	fake(t, d, "python3", `exit 1`)
	if f = Discover(ctx, opt(d, a)); f == nil || f.Name != "Spleeter" {
		t.Fatalf("a Python without the module: %+v", f)
	}

	// audio-separator is the last resort; a file that can't be run isn't a program.
	e := t.TempDir()
	fake(t, e, "audio-separator", `echo "audio-separator 0.30.1"`)
	if f = Discover(ctx, opt(e)); f == nil || f.Name != "audio-separator" || f.Version != "0.30.1" {
		t.Fatalf("audio-separator: %+v", f)
	}
	g := t.TempDir()
	if err := os.WriteFile(filepath.Join(g, "demucs"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if f = Discover(ctx, opt(g)); f != nil {
		t.Fatalf("a file that isn't executable: %+v", f)
	}

	// A program that hangs when asked its version is still found, without one.
	h := t.TempDir()
	fake(t, h, "spleeter", `sleep 30`)
	start := time.Now()
	f = Discover(ctx, Options{Dirs: []string{h}, Timeout: 300 * time.Millisecond})
	if f == nil || f.Version != "" || time.Since(start) > 10*time.Second {
		t.Fatalf("a hanging program: %+v after %s", f, time.Since(start))
	}
}

func TestUsualDirsAreAbsolute(t *testing.T) {
	for _, d := range UsualDirs() {
		if !filepath.IsAbs(d) {
			t.Errorf("%q isn't absolute", d)
		}
	}
}
