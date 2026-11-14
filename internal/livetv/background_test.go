// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// What the manager starts on its own is ended and waited for by Close. Before, a test's manager
// could still be using its database as the test's folder was removed, and on a busy machine that
// wrote a new database into the folder ("TempDir RemoveAll cleanup: directory not empty").
func TestCloseEndsAndWaitsForBackgroundWork(t *testing.T) {
	m := New(nil, nil, slog.New(slog.DiscardHandler))
	started := make(chan struct{})
	var ended atomic.Bool
	if !m.background("test work", func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		time.Sleep(20 * time.Millisecond) // still finishing when Close is asked
		ended.Store(true)
	}) {
		t.Fatal("refused before Close")
	}
	<-started
	m.Close()
	if !ended.Load() {
		t.Error("Close returned before the work ended")
	}
	if m.background("late work", func(context.Context) { t.Error("ran after Close") }) {
		t.Error("started after Close")
	}
	if m.loop("late loop", func(context.Context) { t.Error("ran after Close") }) {
		t.Error("a loop started after Close")
	}
	m.Close() // a second time is harmless
}

// Work that won't stop doesn't hold the server's stopping for ever, and the log says what it was.
func TestCloseDoesNotWaitForEver(t *testing.T) {
	old := closeWait
	closeWait = 50 * time.Millisecond
	defer func() { closeWait = old }()
	buf := &syncBuffer{}
	m := New(nil, nil, slog.New(slog.NewTextHandler(buf, nil)))
	release := make(chan struct{})
	defer close(release)
	m.background("stuck work", func(context.Context) { <-release })
	took := time.Now()
	m.Close()
	if time.Since(took) > 5*time.Second {
		t.Errorf("Close took %s", time.Since(took))
	}
	if out := buf.String(); !strings.Contains(out, "stuck work") {
		t.Errorf("the log doesn't say what was still running: %s", out)
	}
}

// The manager's own work, as it runs: the guide refresher and recording scheduler, a channel
// scan, and a recording. Close ends each, and each has written what it had to before Close
// returns, so the database can close after it.
func TestCloseWaitsForTheManagersOwnWork(t *testing.T) {
	m, f := testManager(t)
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	ctx := context.Background()
	c := f.add(1, Channel{Number: "1", Name: "Slow", Driver: "iptv"}, "slow")
	now := time.Now().Unix()
	rec, err := m.Schedule(ctx, c.ID, 1, "Show", "", now, now+600)
	if err != nil {
		t.Fatal(err)
	}
	m.startDue(ctx)
	for deadline := time.Now().Add(10 * time.Second); ; {
		if got, err := m.Recording(ctx, rec.ID); err == nil && got.Path != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the recording never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	running := m.stillRunning()
	for _, want := range []string{"guide tidying", "recording scheduler", "recording"} {
		if !slices.Contains(strings.Split(running, ", "), want) {
			t.Errorf("%s isn't running: %s", want, running)
		}
	}

	m.Close()

	if left := m.stillRunning(); left != "" {
		t.Errorf("still running after Close: %s", left)
	}
	// The recording says the server stopped, and what it recorded stays.
	got, err := m.Recording(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || got.Error != stoppedReason {
		t.Errorf("%s: %q", got.Status, got.Error)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Errorf("what was recorded is gone: %v", err)
	}
	if n := f.used(1); n != 0 {
		t.Errorf("the tuner is still held: %d", n)
	}
}
