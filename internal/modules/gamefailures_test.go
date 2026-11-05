package modules

import (
	"io"
	"log/slog"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// A WittyPlay version that games failing to start took out of use is put back once, on any
// system, and only that: a version out of use for its own reasons stays out.
func TestLiftGameFailureBlocks(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	// What a Mac without a graphics card left behind: Nintendo 64, PlayStation 2, then PSP.
	gpuBlock := runnerStopped + ": the game runner ended by itself: signal: segmentation fault)"
	blocked := func() *entry {
		man := &wittymodule.Manifest{ID: gameModule, Name: "WittyPlay", Version: "0.1.0", Delivery: wittymodule.DeliveryHelper}
		return &entry{builtin: man, man: man, known: map[string]*knownRelease{}, state: moduleState{Active: "0.3.4", NestedSandboxFixed: true, Blocked: map[string]string{
			"0.3.4": gpuBlock,
			"0.3.2": "files have changed since they were installed.",
			"0.3.1": "it kept failing (it stopped without answering)",
		}}}
	}
	e := blocked()
	if !liftGameFailureBlocks(gameModule, e, log) {
		t.Fatal("nothing changed")
	}
	if r := e.state.Blocked["0.3.4"]; r != "" {
		t.Errorf("0.3.4 is still out of use: %s", r)
	}
	for _, ver := range []string{"0.3.2", "0.3.1"} {
		if e.state.Blocked[ver] == "" {
			t.Errorf("%s was put back, but it went out of use for its own reasons", ver)
		}
	}
	if n := len(e.state.Events); n != 1 || e.state.Events[0].Kind != "put-back" || e.state.Events[0].Version != "0.3.4" {
		t.Errorf("events: %+v", e.state.Events)
	}
	// Once only.
	e.state.Blocked["0.3.4"] = gpuBlock
	if liftGameFailureBlocks(gameModule, e, log) || e.state.Blocked["0.3.4"] == "" {
		t.Error("put back a second time")
	}
	for _, c := range []struct {
		name, id string
		e        *entry
	}{
		{"another module", tunerID, blocked()},
		{"a fork", gameModule, func() *entry { e := blocked(); e.state.Source = &Source{Repo: "someone/WittyPlay"}; return e }()},
	} {
		if liftGameFailureBlocks(c.id, c.e, log) || c.e.state.Blocked["0.3.4"] == "" {
			t.Errorf("%s: a version was put back", c.name)
		}
	}
}
