// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bdremote

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// fakeProcess answers a helper's calls from a script of JSON answers.
type fakeProcess struct {
	answers map[string]func() (string, error)
	closed  bool
}

func (f *fakeProcess) Call(method string, params, out any) ([]byte, error) {
	a, ok := f.answers[method]
	if !ok {
		return nil, &wittymodule.CallError{Code: "unknown-method", Message: "this version has no " + method}
	}
	s, err := a()
	if err != nil {
		return nil, err
	}
	if out != nil {
		if err := json.Unmarshal([]byte(s), out); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (f *fakeProcess) Close() { f.closed = true }

func say(s string) func() (string, error) { return func() (string, error) { return s, nil } }

func TestBlurayAnalysisAgainstHelpersOldAndNew(t *testing.T) {
	open := say(`{"playlists":[0,1,800]}`)
	ranked := `{"ranking":{"candidates":[{"playlist":800,"durationSec":5600,"distinctSec":5600,"chapters":10,"items":4,"clips":4,"score":96,"class":"main"},
		{"playlist":1,"durationSec":60,"distinctSec":60,"chapters":1,"items":1,"clips":1,"score":1,"class":"trailer"}],
		"main":800,"playAll":-1,"confidence":0.9,"confidenceLabel":"high","why":["Title 1 plays it."],
		"secondOpinion":{"source":"libbluray 1.3.4","agreesOnMain":true,"main":800}},"total":3,"note":"compared"}`
	cases := []struct {
		name    string
		second  bool
		answers map[string]func() (string, error)
		main    int // -2: no ranking
		err     bool
		report  bool
		opinion bool
	}{
		{name: "a WittyBluray with the ranking", answers: map[string]func() (string, error){"open": open, "ranking": say(ranked), "consistency": say(`{}`)}, main: 800},
		{name: "a second opinion only when asked", second: true, answers: map[string]func() (string, error){"open": open, "ranking": say(ranked)}, main: 800, opinion: true},
		{name: "one from before the ranking", answers: map[string]func() (string, error){"open": open}, main: -2},
		{name: "one that says no", answers: map[string]func() (string, error){"open": open, "ranking": func() (string, error) {
			return "", &wittymodule.CallError{Code: "bad-call", Message: "no"}
		}}, main: -2},
		{name: "one that dies", answers: map[string]func() (string, error){"open": open, "ranking": func() (string, error) { return "", errors.New("gone") }}, err: true},
		{name: "a playlist the disc hasn't", answers: map[string]func() (string, error){"open": open, "ranking": say(strings.ReplaceAll(ranked, `"main":800,"playAll"`, `"main":4242,"playAll"`))}, err: true, report: true},
		{name: "cross-checks that fail don't lose the ranking", answers: map[string]func() (string, error){"open": open, "ranking": say(ranked), "consistency": func() (string, error) { return "", errors.New("gone") }}, main: 800},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := &fakeProcess{answers: c.answers}
			reported := ""
			got, err := analyze(h, t.TempDir(), c.second, slog.New(slog.DiscardHandler), func(r string) { reported = r })
			if !h.closed {
				t.Error("the helper was left running")
			}
			if (err != nil) != c.err {
				t.Fatalf("err %v, want an error: %v", err, c.err)
			}
			if (reported != "") != c.report {
				t.Errorf("reported %q", reported)
			}
			if err != nil {
				return
			}
			if c.main == -2 {
				if got.Ranked != nil {
					t.Errorf("a ranking from a helper without one")
				}
				return
			}
			if got.Ranked == nil || got.Ranked.Ranking.Main != c.main {
				t.Fatalf("ranking %+v", got.Ranked)
			}
			if (got.Ranked.Ranking.SecondOpinion != nil) != c.opinion {
				t.Errorf("second opinion %+v, asked: %v", got.Ranked.Ranking.SecondOpinion, c.second)
			}
		})
	}
}

func TestCapabilitiesOfAnOlderBluray(t *testing.T) {
	c, err := capabilities(&fakeProcess{})
	if err != nil || c.Libbluray.Available || !strings.Contains(c.Libbluray.Reason, "Update WittyBluray") {
		t.Errorf("an older WittyBluray: %+v %v", c, err)
	}
	c, err = capabilities(&fakeProcess{answers: map[string]func() (string, error){"capabilities": say(`{"libbluray":{"available":true,"version":"1.3.4","path":"/usr/lib/libbluray.so.2"}}`)}})
	if err != nil || !c.Libbluray.Available || c.Libbluray.Version != "1.3.4" {
		t.Errorf("a current one: %+v %v", c, err)
	}
}
