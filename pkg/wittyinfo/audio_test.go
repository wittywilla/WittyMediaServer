// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"strings"
	"testing"
)

const song = `{
  "format": "wittyaudio", "version": 1, "language": "en",
  "identity": {"durationSec": "3:30"},
  "metadata": {"title": "A Song", "year": 2024, "genres": ["Pop"]},
  "audio": {
    "artists": ["Someone", "Someone Else"], "album": "An Album", "track": 3, "trackTotal": 12, "disc": 1,
    "isrc": "GB-AAA-26-00001", "bpm": 120, "key": "A minor", "explicit": true,
    "loudness": {"trackGain": -6.5, "trackPeak": 0.98},
    "trimEnd": "3:20", "fadeOut": 4,
    "lyrics": {"language": "en", "lines": [
      {"start": 12.5, "text": "First line", "words": [{"start": 12.5, "text": "First"}, {"start": 13.1, "text": "line"}]},
      {"start": "0:16", "text": "Second line", "singer": "Someone Else"}
    ]}
  },
  "translations": {"es": {"lyrics": {"lines": [{"start": 12.5, "text": "Primera línea"}, {"start": 16, "text": "Segunda línea"}]}}}
}`

func TestSongRoundTrip(t *testing.T) {
	x, err := Parse([]byte(song))
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	if !x.IsAudio() || x.ScopeOr() != ScopeSong || !x.Single() || x.ExtOf() != ".wittyaudio" {
		t.Fatalf("%v %v", x.Format, x.ScopeOr())
	}
	b, err := x.Encode()
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	y, err := Parse(b)
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	a := y.Audio
	if y.Format != FormatAudio || y.Scope != ScopeSong || *a.Track != 3 || a.Lyrics.Lines[1].Start != 16 || len(a.Lyrics.Lines[0].Words) != 2 || *a.Loudness.TrackGain != -6.5 {
		t.Errorf("%+v", a)
	}
	if y.Translations["es"].Lyrics.Lines[0].Text != "Primera línea" {
		t.Error("translated lyrics lost")
	}
	if m := y.MatchVideo(211, nil); m.Result != MatchSameCut {
		t.Errorf("a song's length is checked too: %+v", m)
	}
	if album := NewAudio(ScopeAlbum); album.MatchVideo(1, nil).Result != MatchNotNeeded {
		t.Error("an album has no one length")
	}
}

func TestBadSongsAreRefused(t *testing.T) {
	audio := func(extra string) []byte { return []byte(`{"format":"wittyaudio","version":1,"audio":{` + extra + `}}`) }
	for name, b := range map[string][]byte{
		"music in a film's file": []byte(`{"format":"wittyinfo","version":1,"audio":{"album":"x"}}`),
		"a film's scope":         []byte(`{"format":"wittyaudio","version":1,"scope":"show"}`),
		"a song's scope in film": []byte(`{"format":"wittyinfo","version":1,"scope":"album"}`),
		"bad isrc":               audio(`"isrc":"nope"`),
		"bad key":                audio(`"key":"Q major"`),
		"lines out of order":     audio(`"lyrics":{"lines":[{"start":20,"text":"b"},{"start":10,"text":"a"}]}`),
		"line ends first":        audio(`"lyrics":{"lines":[{"start":20,"end":10,"text":"b"}]}`),
		"loud":                   audio(`"loudness":{"trackGain":99}`),
		"trim inside out":        audio(`"trimStart":30,"trimEnd":10`),
		"bad lyrics language":    audio(`"lyrics":{"language":"Spanish please"}`),
		"too many words":         audio(`"lyrics":{"lines":[{"start":1,"text":"x","words":[` + strings.TrimSuffix(strings.Repeat(`{"start":1,"text":"w"},`, MaxLyricWords+1), ",") + `]}]}`),
	} {
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: read without complaint", name)
		}
	}
}
