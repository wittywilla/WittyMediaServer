// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package dvdremote

import (
	"bytes"
	"encoding/gob"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymodule"
)

// fakeHelper answers the calls of an analysis as a helper would, from a script.
type fakeHelper struct {
	answers map[string]func() ([]byte, error)
	calls   []string
	closed  bool
}

func (f *fakeHelper) Call(method string, params, out any) ([]byte, error) {
	f.calls = append(f.calls, method)
	if a, ok := f.answers[method]; ok {
		return a()
	}
	return nil, &wittymodule.CallError{Code: dvd.CodeUnknownCall, Message: "this version has no " + method}
}

func (f *fakeHelper) Close() { f.closed = true }

// twoTitleDisc is the structure of a disc of two titles in one title set, as a helper sends it.
func twoTitleDisc(t *testing.T) []byte {
	t.Helper()
	d := dvd.Disc{
		VMG:     &dvd.VMG{Titles: []dvd.TitleSearch{{Number: 1, VTS: 1, VTSTitle: 1, Angles: 1}, {Number: 2, VTS: 1, VTSTitle: 2, Angles: 1}}},
		VTS:     map[int]*dvd.VTS{1: {}},
		Repairs: []dvd.Repair{{VTS: 1, Table: "VTS_PTT_SRPT", From: "BUP", Fixed: true, Note: "the chapter table was read from the backup"}},
	}
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(&d); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func answer(s string) func() ([]byte, error) { return func() ([]byte, error) { return []byte(s), nil } }

func TestAnalyzeAgainstHelpersOldAndNew(t *testing.T) {
	disc := twoTitleDisc(t)
	open := func() ([]byte, error) { return disc, nil }
	ranked := `{"candidates":[{"title":2,"vts":1,"vtsTitle":2,"durationSec":5400,"distinctSec":5400,"chapters":12,"cells":30,"score":90,"class":"main","flags":["main"],"reasons":["the most distinct video"]},
		{"title":1,"vts":1,"vtsTitle":1,"durationSec":60,"distinctSec":60,"chapters":1,"cells":1,"score":2,"class":"trailer","flags":[],"reasons":[]}],
		"main":2,"confidence":0.93,"level":"high","why":{"kind":"main","titles":[2],"confidence":0.93,"level":"high","summary":"Title 2 is the film.","runnerUp":1,"margin":88}}`
	cases := []struct {
		name    string
		answers map[string]func() ([]byte, error)
		main    int // 0: no ranking
		err     bool
		report  bool
	}{
		{name: "a WittyDVD with the ranking", answers: map[string]func() ([]byte, error){"open": open, "analyze": answer(ranked), "readStats": answer(`{"badSectors":3,"retries":9}`)}, main: 2},
		{name: "one from before the ranking", answers: map[string]func() ([]byte, error){"open": open}},
		{name: "one that says no about this disc", answers: map[string]func() ([]byte, error){"open": open, "analyze": func() ([]byte, error) {
			return nil, &wittymodule.CallError{Code: "bad-call", Message: "that navigator isn't running"}
		}}},
		{name: "one that hangs or dies", answers: map[string]func() ([]byte, error){"open": open, "analyze": func() ([]byte, error) {
			return nil, errors.New("the helper stopped answering")
		}}, err: true},
		{name: "a title the disc hasn't", answers: map[string]func() ([]byte, error){"open": open, "analyze": answer(strings.Replace(ranked, `"main":2`, `"main":7`, 1))}, err: true, report: true},
		{name: "an impossible confidence", answers: map[string]func() ([]byte, error){"open": open, "analyze": answer(strings.Replace(ranked, `"confidence":0.93,"level":"high","why"`, `"confidence":4,"level":"high","why"`, 1))}, err: true, report: true},
		{name: "an answer too large", answers: map[string]func() ([]byte, error){"open": open, "analyze": func() ([]byte, error) { return make([]byte, 5<<20), nil }}, err: true, report: true},
		{name: "a ranking with no read count", answers: map[string]func() ([]byte, error){"open": open, "analyze": answer(ranked)}, main: 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := &fakeHelper{answers: c.answers}
			reported := ""
			got, err := analyze(h, func(r string) { reported = r }, slog.New(slog.DiscardHandler), t.TempDir())
			if !h.closed {
				t.Error("the helper was left running")
			}
			if (err != nil) != c.err {
				t.Fatalf("err %v, want an error: %v", err, c.err)
			}
			if (reported != "") != c.report {
				t.Errorf("reported %q, want a report: %v", reported, c.report)
			}
			if err != nil {
				return
			}
			if len(got.Repairs) != 1 {
				t.Errorf("repairs %+v", got.Repairs)
			}
			switch {
			case c.main == 0 && got.Ranking != nil:
				t.Errorf("a ranking from a helper without one: %+v", got.Ranking)
			case c.main != 0 && (got.Ranking == nil || got.Ranking.Main != c.main):
				t.Fatalf("ranking %+v, want main %d", got.Ranking, c.main)
			case c.main != 0 && got.Ranking.Why == nil:
				t.Error("the reason was lost")
			}
			if c.answers["readStats"] != nil && got.Stats.BadSectors != 3 {
				t.Errorf("stats %+v", got.Stats)
			}
		})
	}
}
