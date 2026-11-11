// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Media jobs make something for one item that takes FFmpeg, or a helper program, a while:
// an approximate instrumental, separated stems, a canvas cut from a video. They are work
// nobody is waiting at the screen for, so they run one at a time on the whole server, at
// low priority, giving way to people (internal/pace), each as a task that shows progress
// and can be stopped. The queue is bounded: a script asking for a job for every song gets
// "try later", not a server out of memory.

// maxMediaJobs is how many may wait or run at once.
const maxMediaJobs = 16

var (
	errJobQueued = errors.New("that is already being made")
	errJobsFull  = errors.New("too many of these are waiting; try again when some have finished")
)

type mediaJobs struct {
	mu     sync.Mutex
	queued map[string]string // what is waiting or running, by key → its task
	turn   chan struct{}     // one job at a time
}

// startMediaJob queues fn under key ("stems-approx:12"), unless that is already queued or
// the queue is full. It answers the task's id. fn gets a context that ends when the task
// is stopped or the server closes, marked as background work.
func (s *Server) startMediaJob(key string, by *auth.User, spec tasks.Spec, fn func(ctx context.Context, task *tasks.Handle) error) (string, error) {
	j := &s.mediaJobs
	j.mu.Lock()
	if j.queued == nil {
		j.queued, j.turn = map[string]string{}, make(chan struct{}, 1)
	}
	if id, ok := j.queued[key]; ok {
		j.mu.Unlock()
		return id, errJobQueued
	}
	if len(j.queued) >= maxMediaJobs {
		j.mu.Unlock()
		return "", errJobsFull
	}
	stopped, stop := context.WithCancel(context.Background())
	spec.By, spec.Cancel = byUser(by), stop
	task := s.Tasks.Queue(spec)
	j.queued[key] = task.ID()
	j.mu.Unlock()
	forget := func() {
		stop()
		j.mu.Lock()
		delete(j.queued, key)
		j.mu.Unlock()
	}
	started := s.background("media job", func(bg context.Context) {
		defer forget()
		ctx, cancel := context.WithCancel(pace.Background(bg))
		defer cancel()
		defer context.AfterFunc(stopped, cancel)()
		select {
		case j.turn <- struct{}{}:
			defer func() { <-j.turn }()
		case <-ctx.Done():
			task.Finish(context.Canceled)
			return
		}
		task.Begin()
		if err := pace.Yield(ctx); err != nil {
			task.Finish(context.Canceled)
			return
		}
		err := fn(ctx, task)
		if err != nil && ctx.Err() != nil {
			err = context.Canceled
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			s.Log.Error("a media job failed", "cat", "library", "job", key, "err", err)
		}
		task.Finish(err)
	}, "cat", "library")
	if !started {
		forget()
		task.Finish(context.Canceled)
		return "", errors.New("the server is stopping")
	}
	return task.ID(), nil
}

// mediaJobTask is the task making key, "" when none is.
func (s *Server) mediaJobTask(key string) string {
	j := &s.mediaJobs
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.queued[key]
}
