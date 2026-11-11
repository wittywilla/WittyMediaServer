// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #73: a reset moves the logs into the copy it keeps. A log folder elsewhere stays
// where it was, so only the copy's crash reports are earlier ones.
func TestEarlierLogsAreFoundInTheResetsCopy(t *testing.T) {
	data := t.TempDir()
	for _, name := range []string{"before-reset-20261101-090000", "before-reset-20261102-165543", "not-a-copy"} {
		if err := os.MkdirAll(filepath.Join(data, name, "logs"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := EarlierLogs(data, filepath.Join(data, "logs"))
	if len(got) != 2 || got[0].Name != "before-reset-20261102-165543" {
		t.Fatalf("%+v", got)
	}
	if got[0].Logs != filepath.Join(data, "before-reset-20261102-165543", "logs") || got[0].Crashes != filepath.Join(data, "before-reset-20261102-165543", "crash") {
		t.Fatalf("%+v", got[0])
	}
	elsewhere := EarlierLogs(data, filepath.Join(t.TempDir(), "logs"))
	if len(elsewhere) != 2 || elsewhere[0].Logs != "" || elsewhere[0].Crashes == "" {
		t.Fatalf("%+v", elsewhere)
	}
}
