package helper

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittymodule"
)

func code(t *testing.T, err error) string {
	t.Helper()
	var ce *wittymodule.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("not a refusal: %v", err)
	}
	return ce.Code
}

// With no disc open, nothing panics, and the server is told why.
func TestNothingOpen(t *testing.T) {
	h := &Helper{}
	for _, method := range []string{"playlist", "start", "setaudio", "no such thing"} {
		if _, _, err := h.Handle(method, nil); code(t, err) != "not-open" {
			t.Errorf("%s: %v", method, err)
		}
	}
	if _, _, err := h.Handle("open", json.RawMessage(`{"path":`)); code(t, err) != "bad-call" {
		t.Errorf("unreadable parameters: %v", err)
	}
	raw, _ := json.Marshal(Params{Path: filepath.Join(t.TempDir(), "missing.iso")})
	if _, _, err := h.Handle("open", raw); code(t, err) != "" {
		t.Errorf("a missing disc: %v", err)
	}
}

// A step's effects can hold a pointer to false ("stills off"), which is why answers
// aren't gob, which would lose it.
func TestAnswersKeepPointersToFalse(t *testing.T) {
	off := false
	b, err := json.Marshal(&Answer{Timer: &wittybluray.NavTimer{}, Step: &wittybluray.Step{Kind: wittybluray.StepStay, Effects: &wittybluray.Effects{Still: &off}}})
	if err != nil {
		t.Fatal(err)
	}
	var a Answer
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	if a.Timer == nil || a.Step.Effects.Still == nil || *a.Step.Effects.Still {
		t.Errorf("%+v", a)
	}
}
