package bdremote

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/helper"
)

// A playlist written straight into a step has had none of the checks. Only one asked
// for by number counts, so an answer like this is the helper's fault.
func TestStepCantCarryItsOwnPlaylist(t *testing.T) {
	w := newWorld(t, func() process {
		return &inProc{tamper: func(method string, blob []byte, err error) ([]byte, error) {
			if method != "start" {
				return blob, err
			}
			b, jerr := json.Marshal(helper.Answer{Step: &wittybluray.Step{Kind: wittybluray.StepPlay,
				Playlist: &wittybluray.Playlist{Number: 1, Items: []wittybluray.PlayItem{{Clip: "../../../secret"}}}}})
			if jerr != nil {
				t.Fatal(jerr)
			}
			return b, nil
		}}
	})
	s := w.open(clearDisc(t))
	p := s.Start(context.Background())
	if w.engine.h != nil {
		t.Error("the helper is still in use")
	}
	if p.Kind == "stop" {
		t.Errorf("the built-in copy should have taken over: %+v", p)
	}
}

// Past the limit an answer isn't decoded at all: gob grows a stranger's bytes 1500 times.
func TestLargeAnswersAreNotDecoded(t *testing.T) {
	for method, limit := range answerLimits {
		w := newWorld(t, func() process {
			return &inProc{tamper: func(m string, blob []byte, err error) ([]byte, error) {
				if m == method {
					return make([]byte, limit+1), nil
				}
				return blob, err
			}}
		})
		w.open(clearDisc(t))
		switch method {
		case "playlist":
			w.engine.Playlist(1)
		case "clip":
			w.engine.Clip("00001")
		case "extents":
			_, _ = w.engine.ClipExtents("00001")
		case "cliplead":
			w.engine.ClipLead("00001", 0x1011, 0)
		case "bdjobject":
			_, _ = w.engine.BDJObject("00000")
		case "text":
			_, _ = w.engine.TextSubtitles("00001", 0x1800, 0)
		default:
			t.Fatalf("no way to ask for %s", method)
		}
		if w.engine.h != nil {
			t.Errorf("%s: the helper is still in use", method)
		}
	}
}

// What real discs answer stays well inside the limits.
func TestLimitsAreFarAboveARealAnswer(t *testing.T) {
	const items, streams = 999, 8
	pl := wittybluray.Playlist{Number: 1}
	for i := 0; i < items; i++ {
		it := wittybluray.PlayItem{Clip: "00001", Codec: "M2TS"}
		for j := 0; j < streams; j++ {
			it.Audio = append(it.Audio, wittybluray.Stream{PID: 0x1100, Coding: 0x83, Lang: "eng"})
		}
		pl.Items = append(pl.Items, it)
	}
	if n := len(gobbed(t, &pl)); n > answerLimits["playlist"] {
		t.Errorf("the most play items a disc can have, with %d streams each, is %d bytes", streams, n)
	}
}
