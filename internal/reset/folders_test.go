// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package reset_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/reset"
)

// Issue #66: a reset ran at start, after the folders had been made, and took the cache
// and its images folder with it, so every thumbnail failed until the next restart. The
// start makes them again once the reset is done.
func TestAResetLeavesTheFoldersTheServerWritesTo(t *testing.T) {
	for _, k := range []string{"WMS_DATA_DIR", "WMS_CACHE_DIR", "WMS_METADATA_DIR", "WMS_TRANSCODE_DIR", "WMS_LOG_DIR", "WMS_CONFIG"} {
		t.Setenv(k, "")
	}
	data := filepath.Join(t.TempDir(), "data")
	cfg, err := config.Parse([]string{"-data", data})
	if err != nil {
		t.Fatal(err)
	}
	folders := []string{cfg.CacheDir, cfg.ImagesDir, cfg.TranscodeDir, cfg.LogDir}
	for _, d := range folders {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Fatalf("%s wasn't made: %v", d, err)
		}
	}
	if err := reset.Ask(cfg.DataDir, reset.Request{Mode: reset.ModeKeepCopy, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if res := reset.Apply(reset.Folders{Data: cfg.DataDir}); res == nil || !res.OK {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(cfg.ImagesDir); err == nil {
		t.Fatal("the reset left the images folder, so this test shows nothing")
	}
	if err := cfg.MakeFolders(); err != nil {
		t.Fatal(err)
	}
	for _, d := range folders {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			t.Fatalf("%s is still missing after the reset: %v", d, err)
		}
	}
}
