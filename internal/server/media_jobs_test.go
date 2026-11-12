// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

type jobLogBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *jobLogBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *jobLogBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// A media job's session reaches what it runs (ffmpeg.Run takes it from the context), its own
// lines are in "extras", and a song that is mono is no error of the server's.
func TestMediaJobSessions(t *testing.T) {
	ts := newTestServer(t)
	buf := &jobLogBuffer{}
	ts.Log = slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	run := func(key string, fail error) string {
		t.Helper()
		got := make(chan string, 1)
		if _, err := ts.startMediaJob(key, nil, tasks.Spec{Title: "A job"}, func(ctx context.Context, _ *tasks.Handle) error {
			got <- logging.SessionFrom(ctx)
			return fail
		}); err != nil {
			t.Fatal(err)
		}
		select {
		case id := <-got:
			for deadline := time.Now().Add(10 * time.Second); ts.mediaJobTask(key) != "" && time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			}
			return id
		case <-time.After(10 * time.Second):
			t.Fatal("the job never ran")
		}
		return ""
	}
	for key, kind := range map[string]string{"stems-separate:1": "sep-", "canvas:1": "canvas-", "stems-approx:1": "job-"} {
		id := run(key, nil)
		if !strings.HasPrefix(id, kind) {
			t.Errorf("%s: session %q, want %s…", key, id, kind)
		}
		if out := buf.String(); !strings.Contains(out, `msg="media job started" cat=extras session=`+id) || !strings.Contains(out, `msg="media job finished" cat=extras session=`+id) {
			t.Errorf("%s: %s", key, out)
		}
	}
	run("stems-approx:2", errNotStereo)
	if out := buf.String(); strings.Contains(out, "level=ERROR") || !strings.Contains(out, `msg="a media job could not be done"`) {
		t.Errorf("a mono song: %s", out)
	}
}
