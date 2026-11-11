// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPublicErrorKeepsAddressesAndFoldersOut(t *testing.T) {
	addr := "http://provider.invalid/live/USERNAME/PASSWORD/1.ts"
	for _, err := range []error{
		fmt.Errorf("the stream could not be reached: %w", &url.Error{Op: "Get", URL: addr, Err: errors.New("no such host")}),
		errors.New("a tool said: " + addr),
		fmt.Errorf("the recordings folder could not be used: %w", &fs.PathError{Op: "mkdir", Path: "/home/someone/recordings", Err: fs.ErrPermission}),
	} {
		got := PublicError(err)
		if strings.Contains(got, "USERNAME") || strings.Contains(got, "someone") {
			t.Errorf("%v: %q", err, got)
		}
	}
	if got := PublicError(errors.New("no tuner was free when the recording started")); !strings.Contains(got, "no tuner") {
		t.Errorf("a plain reason was lost: %q", got)
	}
}

// A row can come from a backup, and a backup's password only says who made it. Removing a recording
// removes a file in the recordings folder and nothing else.
func TestARecordingRowCantNameAnyOtherFile(t *testing.T) {
	m := testManager(t)
	c := liveChannel(t, m, newLiveFake(t, nil))
	base := t.TempDir()
	folder := filepath.Join(base, "recordings")
	if err := os.MkdirAll(filepath.Join(folder, "News"), 0o750); err != nil {
		t.Fatal(err)
	}
	m.Folder = func() string { return folder }
	ctx := context.Background()
	write := func(p string) string {
		if err := os.WriteFile(p, []byte("keep me"), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	outside := write(filepath.Join(base, "wms.json"))
	mine := write(filepath.Join(folder, "News", "News.ts"))
	row := func(path string) int64 {
		now := time.Now().Unix()
		rec, err := m.Schedule(ctx, c.ID, 1, "News", "", now+3600, now+7200)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.DB.ExecContext(ctx, `UPDATE recordings SET path = ?, status = 'done' WHERE id = ?`, path, rec.ID); err != nil {
			t.Fatal(err)
		}
		return rec.ID
	}
	for _, p := range []string{outside, filepath.Join(folder, "..", "wms.json"), folder + string(filepath.Separator) + ".." + string(filepath.Separator) + "wms.json"} {
		id := row(p)
		got, err := m.Recording(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if got.SizeBytes != 0 {
			t.Errorf("%s: its size was read: %d", p, got.SizeBytes)
		}
		if err := m.CancelRecording(ctx, id, true); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if _, err := os.Stat(outside); err != nil {
			t.Fatalf("removing a recording named %s deleted a file outside the folder: %v", p, err)
		}
	}
	id := row(mine)
	if got, err := m.Recording(ctx, id); err != nil || got.SizeBytes == 0 {
		t.Errorf("a real recording's size: %+v, %v", got, err)
	}
	if err := m.CancelRecording(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mine); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a real recording stayed: %v", err)
	}
}

// A recording stops rather than fill the disk the server's database may be on.
func TestRecordingStopsWhenTheDiskIsNearlyFull(t *testing.T) {
	old := recordingFreeFloor
	recordingFreeFloor = math.MaxInt64
	defer func() { recordingFreeFloor = old }()
	m := testManager(t)
	f := newLiveFake(t, nil)
	c := liveChannel(t, m, f)
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	ctx := context.Background()
	now := time.Now().Unix()
	rec, err := m.Schedule(ctx, c.ID, 1, "News", "", now, now+120)
	if err != nil {
		t.Fatal(err)
	}
	m.startDue(ctx)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := m.Recording(ctx, rec.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == "failed" {
			if got.Error != diskFullReason {
				t.Errorf("%q", got.Error)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("it never stopped")
}
