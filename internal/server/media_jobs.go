// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/logging"
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
	// failed is why the last try at a key failed, until it is tried again. The task says so
	// too, but not everyone who may start a job may see tasks, and whoever pressed the
	// button is owed the reason. Memory only, and bounded by maxJobFailures.
	failed map[string]error
}

// maxJobFailures is how many reasons are remembered. Past it the oldest known go first,
// in no particular order: it is a courtesy, not a record (the task history is that).
const maxJobFailures = 64

// jobSessionKind is the kind of session id a job's key gets: "sep" for a karaoke
// separation, "canvas", "intro", and "job" for anything else.
func jobSessionKind(key string) string {
	kind, _, _ := strings.Cut(key, ":")
	switch kind {
	case "stems-separate":
		return "sep"
	case "canvas", "intro":
		return kind
	}
	return "job"
}

// jobLog is the logger for work a media job does: category extras, and the job's
// session when ctx carries one.
func (s *Server) jobLog(ctx context.Context) *slog.Logger {
	log := s.Log.With("cat", "extras")
	if id := logging.SessionFrom(ctx); id != "" {
		log = log.With("session", id)
	}
	return log
}

// startMediaJob queues fn under key ("stems-approx:12"), unless that is already queued or
// the queue is full. It answers the task's id. fn gets a context that ends when the task
// is stopped or the server closes, marked as background work, and carrying the job's
// session id (jobSessionKind), so FFmpeg's and the job's own lines can be followed.
func (s *Server) startMediaJob(key string, by *auth.User, spec tasks.Spec, fn func(ctx context.Context, task *tasks.Handle) error) (string, error) {
	j := &s.mediaJobs
	j.mu.Lock()
	if j.queued == nil {
		j.queued, j.turn = map[string]string{}, make(chan struct{}, 1)
	}
	if id, ok := j.queued[key]; ok {
		j.mu.Unlock()
		s.Log.Debug("a media job is already queued", "cat", "extras", "job", key, "task", id)
		return id, errJobQueued
	}
	if len(j.queued) >= maxMediaJobs {
		j.mu.Unlock()
		s.Log.Debug("the media job queue is full", "cat", "extras", "job", key, "queued", maxMediaJobs)
		return "", errJobsFull
	}
	delete(j.failed, key) // a new try: the old reason no longer stands
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
	sid := logging.NewID(jobSessionKind(key))
	log := s.Log.With("cat", "extras", "session", sid, "job", key)
	log.Debug("media job queued", "task", task.ID(), "by", spec.By)
	started := s.background("media job", func(bg context.Context) {
		defer forget()
		ctx, cancel := context.WithCancel(pace.Background(logging.WithSession(bg, sid)))
		defer cancel()
		defer context.AfterFunc(stopped, cancel)()
		select {
		case j.turn <- struct{}{}:
			defer func() { <-j.turn }()
		case <-ctx.Done():
			log.Debug("media job cancelled", "while", "waiting")
			task.Finish(context.Canceled)
			return
		}
		task.Begin()
		if err := pace.Yield(ctx); err != nil {
			log.Debug("media job cancelled", "while", "waiting")
			task.Finish(context.Canceled)
			return
		}
		begun := time.Now()
		log.Info("media job started", "task", task.ID())
		err := fn(ctx, task)
		if err != nil && ctx.Err() != nil {
			err = context.Canceled
		}
		took := time.Since(begun).Round(time.Millisecond).String()
		switch {
		case err == nil:
			log.Debug("media job finished", "took", took)
		case errors.Is(err, context.Canceled):
			log.Debug("media job cancelled", "while", "running", "took", took)
		case errors.Is(err, errNotStereo):
			// About the song, not the server: nothing to fix.
			log.Info("a media job could not be done", "reason", "mono", "took", took)
		default:
			log.Error("a media job failed", "err", err, "took", took)
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			j.mu.Lock()
			if j.failed == nil {
				j.failed = map[string]error{}
			}
			for k := range j.failed {
				if len(j.failed) < maxJobFailures {
					break
				}
				delete(j.failed, k)
			}
			j.failed[key] = err
			j.mu.Unlock()
		}
		task.Finish(err)
	}, "cat", "extras", "session", sid)
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

// mediaJobError is why the last try at key failed, nil when it didn't or it is being
// tried again.
func (s *Server) mediaJobError(key string) error {
	j := &s.mediaJobs
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.failed[key]
}
