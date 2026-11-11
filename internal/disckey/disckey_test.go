// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package disckey

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKeyFollowsTheDisc(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "disc.iso")
	if err := os.WriteFile(img, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, ok := Of(img, "v1")
	if !ok {
		t.Fatal("an image that is there has no key")
	}
	if b, _ := Of(img, "v1"); a != b {
		t.Error("the same disc has two keys")
	}
	if b, _ := Of(img, "v2"); a == b {
		t.Error("another module version shares the key")
	}
	if err := os.WriteFile(img, []byte("longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := Of(img, "v1"); a == b {
		t.Error("a disc that changed size kept its key")
	}
	if _, ok := Of(filepath.Join(dir, "gone.iso"), ""); ok {
		t.Error("a disc that isn't there has a key")
	}

	// A folder: replacing its index changes the key though the folder's own time doesn't.
	ts := filepath.Join(dir, "film", "VIDEO_TS")
	if err := os.MkdirAll(ts, 0o755); err != nil {
		t.Fatal(err)
	}
	ifo := filepath.Join(ts, "VIDEO_TS.IFO")
	if err := os.WriteFile(ifo, []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	f1, _ := Of(filepath.Join(dir, "film"), "")
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(ifo, later, later); err != nil {
		t.Fatal(err)
	}
	if f2, _ := Of(filepath.Join(dir, "film"), ""); f1 == f2 {
		t.Error("a folder whose index changed kept its key")
	}
}

func TestCacheIsBounded(t *testing.T) {
	c := Cache[int]{Max: 3}
	for i := 0; i < 10; i++ {
		c.Put(fmt.Sprint(i), i)
	}
	if c.Len() != 3 {
		t.Fatalf("%d remembered, want 3", c.Len())
	}
	if _, ok := c.Get("6"); ok {
		t.Error("the oldest wasn't forgotten")
	}
	if v, ok := c.Get("9"); !ok || v != 9 {
		t.Error("the newest was forgotten")
	}
	c.Put("9", 90)
	if v, _ := c.Get("9"); v != 90 || c.Len() != 3 {
		t.Error("putting again didn't replace")
	}
	c.Clear()
	if c.Len() != 0 {
		t.Error("clear left something")
	}
}
