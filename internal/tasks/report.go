// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package tasks

import (
	"context"
	"sync"
	"time"
)

// Report is what a module's process says about one of its own tasks: wittymodule's
// TaskReport, after its Validate has passed. The process may be someone's fork, so
// nothing here is believed further than that module's own tasks.
type Report struct {
	Key   string // the module's own name for the task
	Title string
	State State // Running, Done or Failed
	Done  int64
	Total int64
	Item  string
}

const (
	maxPerSource = 16 // live tasks per module; a report past that is dropped
	reportGap    = 250 * time.Millisecond
)

// Reporter takes the reports of one running module process. It never blocks for long,
// since it runs on the goroutine reading that process. Reports replace each other: when
// they come too quickly, the latest is applied a moment later and the rest are lost,
// which costs only steps along the way. A finish is never held back.
type Reporter struct {
	reg    *Registry
	source string

	mu     sync.Mutex
	tasks  map[string]*reported
	closed bool
}

type reported struct {
	h       *Handle
	last    time.Time
	pending *Report
	timer   *time.Timer
}

// ForSource makes a Reporter for a module process. On a nil registry it does nothing.
func (r *Registry) ForSource(moduleID string) *Reporter {
	return &Reporter{reg: r, source: moduleID, tasks: map[string]*reported{}}
}

// Take applies one report.
func (m *Reporter) Take(rep Report) {
	if m == nil || m.reg == nil || rep.Key == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	e := m.tasks[rep.Key]
	if e == nil {
		if rep.State.Finished() || len(m.tasks) >= maxPerSource {
			return
		}
		h := m.reg.Start(Spec{Type: TypeModule, Title: rep.Title, Source: m.source, Key: m.source + "/" + rep.Key, AdminOnly: true})
		if h == nil {
			return
		}
		e = &reported{h: h}
		m.tasks[rep.Key] = e
	}
	if rep.State.Finished() {
		m.finish(rep.Key, e, rep)
		return
	}
	now := m.reg.now()
	if wait := reportGap - now.Sub(e.last); wait > 0 && !e.last.IsZero() {
		r := rep
		e.pending = &r
		if e.timer == nil {
			key := rep.Key
			e.timer = time.AfterFunc(wait, func() { m.flushOne(key, e) })
		}
		return
	}
	e.last = now
	apply(e.h, rep)
}

func (m *Reporter) flushOne(key string, e *reported) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e.timer = nil
	if m.closed || m.tasks[key] != e || e.pending == nil {
		return
	}
	apply(e.h, *e.pending)
	e.pending = nil
	e.last = m.reg.now()
}

func apply(h *Handle, rep Report) {
	h.update(func(t *Task) {
		if rep.Title != "" {
			t.Title = clip(rep.Title)
		}
		t.Item = clip(rep.Item)
		t.Done, t.Total = rep.Done, rep.Total
		h.noteRate(rep.Done)
	})
}

// finish ends a task. m.mu is held.
func (m *Reporter) finish(key string, e *reported, rep Report) {
	if e.timer != nil {
		e.timer.Stop()
	}
	apply(e.h, rep)
	if rep.State == Failed {
		e.h.Fail("The module doing this says it failed.", "Try again. If it happens again, Settings, Modules has its log.", nil)
	} else {
		e.h.Finish(nil)
	}
	delete(m.tasks, key)
}

// CloseAll ends whatever the process left going, since it has stopped: as failed if it
// failed, and otherwise as cancelled, because nobody can say that work finished.
func (m *Reporter) CloseAll(failed bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for k, e := range m.tasks {
		if e.timer != nil {
			e.timer.Stop()
		}
		if failed {
			e.h.Fail("The module doing this stopped working before it finished.", "Try again. If it happens again, Settings, Modules has its log.", nil)
		} else {
			e.h.Finish(context.Canceled)
		}
		delete(m.tasks, k)
	}
}
