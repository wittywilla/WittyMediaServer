// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Each page of a DjVu book starts a program, for anyone who can open the book, so turning pages
// quickly (or asking for all of them at once) can't mean as many programs as there are pages.
func TestDjvuDrawsAFewAtATime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	bin := t.TempDir()
	state := t.TempDir()
	script := "#!/bin/sh\nmkdir -p '" + state + "/active'\ntouch \"" + state + "/active/$$\"\nls '" + state + "/active' | wc -l >> '" + state + "/counts'\nsleep 0.3\nrm -f \"" + state + "/active/$$\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(bin, "ddjvu"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	usePath(t, bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	book := filepath.Join(t.TempDir(), "book.djvu")
	if err := os.WriteFile(book, []byte("AT&TFORM"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Scanner{CacheDir: t.TempDir()}
	it := &Item{Path: book}
	var wg sync.WaitGroup
	for page := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.DjvuPageFile(context.Background(), it, page)
		}()
	}
	wg.Wait()
	b, err := os.ReadFile(filepath.Join(state, "counts"))
	if err != nil {
		t.Fatal(err)
	}
	most := 0
	for _, l := range strings.Fields(string(b)) {
		if n, _ := strconv.Atoi(l); n > most {
			most = n
		}
	}
	if most > bookDrawsAtOnce {
		t.Errorf("%d programs at once", most)
	}
}

// Listing a DjVu book's pages asks a program too, and the reader asks on every page turned.
func TestDjvuPageCountIsAskedOnce(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	bin := t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	script := "#!/bin/sh\necho x >> '" + calls + "'\necho 7\n"
	if err := os.WriteFile(filepath.Join(bin, "djvused"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	usePath(t, bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	book := filepath.Join(t.TempDir(), "book.djvu")
	if err := os.WriteFile(book, []byte("AT&TFORM"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if got := DjvuPageList(book); len(got) != 7 {
			t.Fatalf("%d pages", len(got))
		}
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "x"); n != 1 {
		t.Errorf("asked %d times", n)
	}
}

// usePath sets PATH for one test, and forgets the DjVuLibre programs found before.
func usePath(t *testing.T, path string) {
	t.Setenv("PATH", path)
	forget := func() {
		djvuFound.Lock()
		clear(djvuFound.m)
		djvuFound.Unlock()
	}
	forget()
	t.Cleanup(forget)
}

// The formats page asks whether DjVuLibre is there on every visit, and looking through
// PATH costs a stat for each folder and, on Windows, each extension.
func TestDjvuToolIsLookedForOnce(t *testing.T) {
	usePath(t, t.TempDir())
	if HasDjVu() {
		t.Fatal("found in an empty PATH")
	}
	djvuFound.Lock()
	djvuFound.m["ddjvu"] = foundTool{path: "/kept", at: time.Now()}
	djvuFound.Unlock()
	if got := djvuTool("ddjvu"); got != "/kept" {
		t.Errorf("looked again: %q", got)
	}
	djvuFound.Lock()
	djvuFound.m["ddjvu"] = foundTool{path: "/kept", at: time.Now().Add(-djvuLookAgain - time.Second)}
	djvuFound.Unlock()
	if got := djvuTool("ddjvu"); got != "" {
		t.Errorf("an answer older than a minute was kept: %q", got)
	}
}

func BenchmarkHasDjVu(b *testing.B) {
	for b.Loop() {
		HasDjVu()
	}
}
