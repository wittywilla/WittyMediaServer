package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittyplay"
	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/ipc"
)

// paint fills a frame in 16-bit colour: each half of the picture its own way.
func paint(w, h int, stacked bool, first, second func(x, y int) uint16) []byte {
	b := make([]byte, w*h*2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			inFirst := y < h/2
			if !stacked {
				inFirst = x < w/2
			}
			v := second(x, y)
			if inFirst {
				v = first(x, y)
			}
			b[(y*w+x)*2], b[(y*w+x)*2+1] = byte(v), byte(v>>8)
		}
	}
	return b
}

func TestScreenColours(t *testing.T) {
	busy := func(x, y int) uint16 { return uint16((x + y) % 50) }
	blank := func(int, int) uint16 { return 7 }
	stacked := ipc.VideoHeader{Width: 256, Height: 384, Format: engine.FormatRGB565}
	if a, b := screenColors(stacked, paint(256, 384, true, busy, blank)); a < 10 || b != 1 {
		t.Errorf("stacked, lower screen blank: %d, %d", a, b)
	}
	if a, b := screenColors(stacked, paint(256, 384, true, blank, busy)); a != 1 || b < 10 {
		t.Errorf("stacked, upper screen blank: %d, %d", a, b)
	}
	sideBySide := ipc.VideoHeader{Width: 512, Height: 192, Format: engine.FormatRGB565}
	if a, b := screenColors(sideBySide, paint(512, 192, false, busy, blank)); a < 10 || b != 1 {
		t.Errorf("side by side, right screen blank: %d, %d", a, b)
	}
}

func TestAnEncryptedGameIsRefusedNotFailed(t *testing.T) {
	dir := t.TempDir()
	rom := filepath.Join(dir, "game.cia")
	// A CIA whose metadata says its content is encrypted.
	b := make([]byte, 0x8000)
	b[0], b[1] = 0x20, 0x20 // header size 0x2020
	tmd := 0x2040
	b[tmd+1], b[tmd+2], b[tmd+3] = 1, 0, 4 // RSA-2048 signature
	b[tmd+0x140+0xC4+0x900+7] = 1
	if err := os.WriteFile(rom, b, 0o644); err != nil {
		t.Fatal(err)
	}
	c := &config{out: dir, runner: "a program that must never be started"}
	r := checkGame(c, job{console: wittyplay.ByID("3ds"), core: coreRef{"azahar", "unused"}, game: rom, index: 1})
	if r.Outcome != OutcomeRefused || !strings.Contains(r.Reason, "encrypted") {
		t.Fatalf("%s: %s", r.Outcome, r.Reason)
	}
}

func TestBestMatchLinesUpRecordings(t *testing.T) {
	a := []uint64{1, 2, 3, 4, 5, 6}
	if got := bestMatch(a, []uint64{3, 4, 5, 6}, 5); got < 0.9 {
		t.Errorf("slid by two: %v", got)
	}
	if got := bestMatch(a, []uint64{9, 9, 9, 9}, 5); got != 0 {
		t.Errorf("nothing alike: %v", got)
	}
}

func TestReportFolderMustBeOutsideACheckout(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := outsideRepo(filepath.Join(dir, "report")); err == nil {
		t.Fatal("a report folder inside a checkout should be refused")
	}
}
