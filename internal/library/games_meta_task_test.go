package library

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

func taskOfType(reg *tasks.Registry, kind string) (tasks.Task, bool) {
	for _, list := range [][]tasks.Task{reg.Live(), reg.Recent()} {
		for _, t := range list {
			if t.Type == kind {
				return t, true
			}
		}
	}
	return tasks.Task{}, false
}

// An import shows in the activity panel: how far it's got, and what came of it.
func TestAGamesImportShowsInTheActivityPanel(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Title: "Right Name", Method: gamemeta.MethodChecksum, Confidence: 100}, nil
	}}
	f := newMetaFixture(t, src)
	reg := tasks.New(nil)
	f.g.Tasks = reg
	f.g.work(f.ctx)
	if _, ok := taskOfType(reg, tasks.TypeGames); ok {
		t.Fatal("a round with nothing to do showed up")
	}
	f.scanFile(t, "One.sfc")
	f.scanFile(t, "Two.sfc")
	f.g.work(f.ctx)
	got, ok := taskOfType(reg, tasks.TypeGames)
	if !ok {
		t.Fatal("the lookup never showed up")
	}
	if got.State != tasks.Done {
		t.Fatalf("state %s: %+v", got.State, got)
	}
	if got.Unit != "games" || got.Done == 0 || got.Done != got.Total {
		t.Errorf("progress: %d of %d %s", got.Done, got.Total, got.Unit)
	}
	if !strings.Contains(got.Result, "Recognised 2 of 2 games") {
		t.Errorf("result: %q", got.Result)
	}
	if strings.Contains(got.Title, f.root) || strings.Contains(got.Phase, f.root) {
		t.Errorf("a path in what anyone may read: %+v", got)
	}
}

// Someone can hold the lookups back, and they carry on from where they were.
func TestAGamesImportCanBePaused(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Title: "Right Name", Method: gamemeta.MethodChecksum, Confidence: 100}, nil
	}}
	f := newMetaFixture(t, src)
	reg := tasks.New(nil)
	task := &gameMetaTask{g: f.g}
	f.g.Tasks = reg
	task.did(1)
	got, ok := taskOfType(reg, tasks.TypeGames)
	if !ok || !got.CanPause {
		t.Fatalf("%v %+v", ok, got)
	}
	if err := reg.Pause(got.ID); err != nil {
		t.Fatal(err)
	}
	held := make(chan struct{})
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	go func() {
		_ = task.wait(ctx)
		close(held)
	}()
	select {
	case <-held:
		t.Fatal("it didn't wait")
	case <-time.After(50 * time.Millisecond):
	}
	if err := reg.Resume(got.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-held:
	case <-time.After(5 * time.Second):
		t.Fatal("it never carried on")
	}
	task.finish(f.ctx, nil)
}

// What a scan of games says at the end is what someone wants to know from it.
func TestAScanOfGamesSaysWhatItFound(t *testing.T) {
	f := newMetaFixture(t, &fakeSource{id: "a"})
	put(t, f.root, "One.sfc", gameBytes("One.sfc"))
	reg := tasks.New(nil)
	sc := NewScanner(f.store, nil, t.TempDir(), func() string { return "" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sc.Tasks = reg
	if !sc.Start(f.lib.ID) {
		t.Fatal("no scan started")
	}
	deadline := time.Now().Add(30 * time.Second)
	for sc.Running(f.lib.ID) {
		if time.Now().After(deadline) {
			t.Fatal("the scan did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, ok := taskOfType(reg, tasks.TypeScan)
	if !ok {
		t.Fatal("no scan in the panel")
	}
	if !strings.Contains(got.Result, "Found 1 game in") {
		t.Fatalf("result: %q", got.Result)
	}
}
