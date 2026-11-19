// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"testing"
	"time"
)

// A session picks its encoder with its own lock held, while makeRoom holds the manager's lock and
// waits for each session's. So what a session asks of the manager about the graphics chip must not
// need the manager's lock: when it did, two streams starting at once could stop each other for good.
func TestChipBookkeepingDoesNotNeedTheManagersLock(t *testing.T) {
	m := gpuManager()
	m.mu.Lock() // as makeRoom holds it while it looks at the sessions
	defer m.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		sp := gpuBase
		sp.Quality.HW = []string{"h264_nvenc"}
		s := &Session{Spec: sp, log: discard, Dir: t.TempDir(), tools: m.tools, m: m}
		s.mu.Lock() // as startLocked holds it
		defer s.mu.Unlock()
		s.commandLine(0)
		m.hwStarted("h264_nvenc")
		m.atCap("h264_nvenc")
		m.hwEnded("h264_nvenc")
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a session waited for the manager's lock while the manager waited for the session")
	}
}
