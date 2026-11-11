// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// closeWait is how long Close waits for the manager's own work. A recording or an FFmpeg that
// doesn't stop when told mustn't hold up the server's stopping for ever.
var closeWait = 10 * time.Second

// errStopping: work asked for after Close, which doesn't start.
var errStopping = errors.New("the server is stopping")

// background runs fn on a goroutine of its own, as the manager's own work: a scan, a guide update,
// a recording, and nobody waits for it. ctx ends when the manager stops, and Close waits for fn, so
// the database can close with nothing still using it. After Close nothing starts, and this says
// false.
func (m *Manager) background(where string, fn func(ctx context.Context), attrs ...any) bool {
	m.bgMu.Lock()
	defer m.bgMu.Unlock()
	if m.bgClosed {
		return false
	}
	if m.bgRunning == nil {
		m.bgRunning = map[string]int{}
	}
	m.bgRunning[where]++
	m.bgWork.Add(1)
	go func() {
		defer m.bgWork.Done()
		defer m.bgEnded(where)
		defer logging.Recover(where, attrs...) // the crash is on record before Close stops waiting
		fn(m.stopCtx)
	}()
	return true
}

// loop is background for something that must keep running, as logging.Loop is: after a panic it
// starts again, a little later each time. It stops with the manager.
func (m *Manager) loop(where string, fn func(ctx context.Context), attrs ...any) bool {
	return m.background(where, func(ctx context.Context) {
		wait := 5 * time.Second
		for runOnce(ctx, where, fn, attrs) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if wait < 5*time.Minute {
				wait *= 2
			}
		}
	}, attrs...)
}

// runOnce runs fn, and says whether it panicked.
func runOnce(ctx context.Context, where string, fn func(ctx context.Context), attrs []any) (panicked bool) {
	defer func() {
		if p := recover(); p != nil {
			panicked = true
			logging.ReportPanic(where, p, attrs...)
		}
	}()
	fn(ctx)
	return false
}

func (m *Manager) bgEnded(where string) {
	m.bgMu.Lock()
	defer m.bgMu.Unlock()
	if m.bgRunning[where]--; m.bgRunning[where] <= 0 {
		delete(m.bgRunning, where)
	}
}

// stillRunning names the work that hasn't ended, for the log.
func (m *Manager) stillRunning() string {
	m.bgMu.Lock()
	defer m.bgMu.Unlock()
	out := make([]string, 0, len(m.bgRunning))
	for where, n := range m.bgRunning {
		if n > 1 {
			where = fmt.Sprintf("%s (%d)", where, n)
		}
		out = append(out, where)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}

// Close stops the manager's own work and releases its sources, then waits for that work to end:
// a recording going is saved as cut short by the server stopping. The database is closed after it,
// by whoever opened it. The server calls it as it stops; a test, when it ends, so nothing writes
// into its folder as that is removed. It waits closeWait at most, and says what was still running.
// It can be called more than once.
func (m *Manager) Close() {
	m.bgMu.Lock()
	m.bgClosed = true
	m.bgMu.Unlock()
	m.stop()
	m.mu.Lock()
	for _, s := range m.sources {
		_ = s.Close()
	}
	m.sources = map[int64]tuner.Source{}
	m.mu.Unlock()
	// Not holding m.mu: the work being waited for may want it. A second Close waits for the first.
	m.closeOnce.Do(func() {
		done := make(chan struct{})
		go func() {
			m.bgWork.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(closeWait):
			m.Log.Warn("live TV stopped without waiting any longer for some of its work", "waited", closeWait.String(), "running", m.stillRunning())
		}
	})
}
