// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package library

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Someone who can put files in a library, aiming at the scan: a sheet that leads out of its folder,
// or at a pipe or a device, must neither hang it nor read what's out there.
func TestHostileGameFilesDontHangAScan(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "games")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(root, "secret.bin")
	if err := os.WriteFile(secret, []byte("not for the scan"), 0o600); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) string {
		p := filepath.Join(lib, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := syscall.Mkfifo(filepath.Join(lib, "pipe.bin"), 0o644); err != nil {
		t.Skip("no pipes here:", err)
	}
	if err := syscall.Mkfifo(filepath.Join(lib, "pipe.nes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/dev/zero", filepath.Join(lib, "zero.bin")); err != nil {
		t.Skip("no links here:", err)
	}
	if err := os.Symlink("/dev/zero", filepath.Join(lib, "zero.nes")); err != nil {
		t.Fatal(err)
	}
	sheets := map[string]string{
		"out.cue":    `FILE "../secret.bin" BINARY` + "\n  TRACK 01 MODE1/2352\n",
		"back.cue":   `FILE "..\secret.bin" BINARY` + "\n  TRACK 01 MODE1/2352\n",
		"abs.cue":    `FILE "` + secret + `" BINARY` + "\n  TRACK 01 MODE1/2352\n",
		"pipe.cue":   `FILE "pipe.bin" BINARY` + "\n  TRACK 01 MODE1/2352\n",
		"zero.cue":   `FILE "zero.bin" BINARY` + "\n  TRACK 01 MODE1/2352\n",
		"device.cue": `FILE "/dev/zero" BINARY` + "\n  TRACK 01 MODE1/2352\n",
		"self.cue":   `FILE "self.cue" BINARY` + "\n  TRACK 01 MODE1/2352\n",
		"list.m3u":   "../secret.bin\n/dev/zero\npipe.bin\nlist.m3u\n",
		"out.gdi":    "1\n1 0 4 2352 \"../secret.bin\" 0\n",
		"long.cue":   strings.Repeat("REM x\n", 200000),
	}
	var paths []string
	for name, body := range sheets {
		paths = append(paths, write(name, body))
	}
	paths = append(paths, filepath.Join(lib, "pipe.nes"), filepath.Join(lib, "zero.nes"))
	for _, p := range paths {
		name := filepath.Base(p)
		done := make(chan struct{})
		go func() {
			defer close(done)
			main := mainFileOf(p)
			if name != "self.cue" && strings.HasSuffix(main, "secret.bin") {
				t.Errorf("%s led the scan to a file outside its folder", name)
			}
			if st, err := os.Stat(main); (err != nil || !st.Mode().IsRegular()) && main != p {
				t.Errorf("%s named %s, which isn't a file", name, main)
			}
			_ = crcOfFile(main)
			_ = sizeOf(main)
			_ = judgeFile(p, name, "", 1<<20)
		}()
		select {
		case <-done:
		case <-time.After(20 * time.Second):
			t.Fatalf("%s hung the scan", name)
		}
	}
}
