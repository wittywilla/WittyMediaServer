package dvdremote

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittydvd/wire"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymodule"
)

func titleCellsOf(t *testing.T, path string) []wittydvd.TitleCell {
	t.Helper()
	o, err := wittydvd.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	return o.Disc.MainTitle().Cells
}

// An installed helper from before "runs" is in good order: the title plays, the helper
// keeps the disc, and nothing counts against that version.
func TestOlderHelperWithoutRuns(t *testing.T) {
	path := makeDisc(t)
	f := &fake{}
	f.hook = func(method string, _ json.RawMessage) ([]byte, error, bool) {
		if method == wire.MethodRuns {
			return nil, &wittymodule.CallError{Code: wire.CodeUnknownCall, Message: "this version has no \"runs\""}, true
		}
		return nil, nil, false
	}
	e, r := openFake(t, f, path)
	cells := titleCellsOf(t, path)
	runs, err := e.TitleRuns(titleSpace, cells)
	if err != nil {
		t.Fatal(err)
	}
	if !isRemote(e) || f.closed || len(r.got) != 0 {
		t.Fatalf("the helper was blamed: closed %v, reports %v", f.closed, r.got)
	}
	if len(runs) != len(cells) {
		t.Fatalf("%d runs of %d cells", len(runs), len(cells))
	}
	for i, run := range runs {
		if run.FirstCell != i || run.Cells != 1 || run.Duration != cells[i].Cell.Duration || len(run.Extents) == 0 {
			t.Errorf("run %d: %+v", i, run)
		}
	}
}

// Even so, what it says is where the video is gets looked at.
func TestOlderHelperWithoutRunsStillChecked(t *testing.T) {
	path := makeDisc(t)
	f := &fake{}
	f.hook = func(method string, _ json.RawMessage) ([]byte, error, bool) {
		switch method {
		case wire.MethodRuns:
			return nil, &wittymodule.CallError{Code: wire.CodeUnknownCall, Message: "no"}, true
		case wire.MethodCell:
			return extentsOf(t, wittydvd.FileExtent{File: `..\..\secret`, Length: 2048}), nil, true
		}
		return nil, nil, false
	}
	e, _ := openFake(t, f, path)
	runs, err := e.TitleRuns(titleSpace, titleCellsOf(t, path))
	if err != nil {
		t.Fatal(err)
	}
	if f.closed == false {
		t.Error("a helper naming other files kept the disc")
	}
	for _, run := range runs {
		for _, x := range run.Extents {
			if x.File == `..\..\secret` {
				t.Errorf("%+v", x)
			}
		}
	}
}

// A cell with nothing to read is left out by the engine itself, which is no reason to
// blame the helper. Anything out of order or out of range still is.
func TestRunsOfACutShortImage(t *testing.T) {
	path := makeDisc(t)
	cells := titleCellsOf(t, path)
	three := []wittydvd.TitleCell{cells[0], cells[1], cells[1]}
	realExt := func(t *testing.T) []wittydvd.FileExtent {
		o, err := wittydvd.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer o.Close()
		ext, err := o.CellExtents(titleSpace, cells[0].Cell)
		if err != nil {
			t.Fatal(err)
		}
		return ext
	}
	answer := func(runs ...wittydvd.TitleRun) func(string, json.RawMessage) ([]byte, error, bool) {
		return func(method string, _ json.RawMessage) ([]byte, error, bool) {
			if method != wire.MethodRuns {
				return nil, nil, false
			}
			b, err := wire.Encode(runs)
			if err != nil {
				t.Fatal(err)
			}
			return b, nil, true
		}
	}
	ext := realExt(t)
	good := []wittydvd.TitleRun{{Extents: ext, FirstCell: 0, Cells: 1, Duration: 1}, {Extents: ext, FirstCell: 2, Cells: 1, Duration: 1}}
	f := &fake{hook: answer(good...)}
	e, r := openFake(t, f, path)
	runs, err := e.TitleRuns(titleSpace, three)
	if err != nil || len(runs) != 2 || f.closed || len(r.got) != 0 {
		t.Fatalf("%v %+v, closed %v, reports %v", err, runs, f.closed, r.got)
	}
	for name, bad := range map[string][]wittydvd.TitleRun{
		"overlapping":  {{Extents: ext, FirstCell: 0, Cells: 2}, {Extents: ext, FirstCell: 1, Cells: 1}},
		"past the end": {{Extents: ext, FirstCell: 2, Cells: 2}},
		"beyond it":    {{Extents: ext, FirstCell: 3, Cells: 1}},
		"backwards":    {{Extents: ext, FirstCell: 2, Cells: 1}, {Extents: ext, FirstCell: 0, Cells: 1}},
		"no cells":     {{Extents: ext, FirstCell: 0, Cells: 0}},
	} {
		f := &fake{hook: answer(bad...)}
		e, _ := openFake(t, f, path)
		if _, err := e.TitleRuns(titleSpace, three); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if !f.closed {
			t.Errorf("%s: believed", name)
		}
	}
}

// A helper that answers with no step means no change, as Tick does, and that doesn't take
// the request down with a nil.
func TestHelperWithNoStep(t *testing.T) {
	path := makeDisc(t)
	f := &fake{}
	f.hook = func(method string, _ json.RawMessage) ([]byte, error, bool) {
		if method == wire.MethodPlayTitle || method == wire.MethodCellEnded {
			b, err := wire.Encode(&wire.NavAnswer{})
			if err != nil {
				t.Fatal(err)
			}
			return b, nil, true
		}
		return nil, nil, false
	}
	r := &reports{}
	_, s := playAs(t, path, f, r)
	ctx := context.Background()
	first := s.Start(ctx)
	if first == nil {
		t.Fatal("no presentation")
	}
	if p := s.PlayTitle(ctx, 1, 1); p != first {
		t.Errorf("no step should mean no change: %+v", p)
	}
	s.Do(ctx, dvdsession.Action{Type: "ended"})
}

// A title with no buttons goes straight to a stream, with no menu scan to notice. Asked
// for directly it's refused like any other way into a scrambled disc, and only then.
func TestScrambledTitleIsRefusedOnTheBuiltinCopy(t *testing.T) {
	play := func(t *testing.T, scrambledAt ...int) *dvdsession.Presentation {
		path := makeDisc(t)
		w, err := os.OpenFile(filepath.Join(path, "VIDEO_TS", "VTS_01_1.VOB"), os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		if _, err := w.WriteAt(navSector(false), 0); err != nil {
			t.Fatal(err)
		}
		for _, at := range scrambledAt {
			if _, err := w.WriteAt(scrambledPack(), int64(at)*wittydvd.Sector); err != nil {
				t.Fatal(err)
			}
		}
		m := sessions(t)
		s, err := m.Open(1, path, dvdsession.Prefs{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { m.CloseSession(s.ID) })
		return s.PlayTitle(context.Background(), 1, 1)
	}
	if p := play(t); p.Kind != "title" {
		t.Fatalf("a clear title should play: %+v", p)
	}
	for _, at := range []int{1, 2, 11} {
		if p := play(t, at); p.Kind != "stop" || !p.Scrambled {
			t.Errorf("scrambled at %d: %+v", at, p)
		}
	}
}
