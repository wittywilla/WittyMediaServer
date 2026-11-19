// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func zipOf(t *testing.T, members map[string][]byte, order ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	if len(order) == 0 {
		for k := range members {
			order = append(order, k)
		}
	}
	for _, name := range order {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(members[name])
	}
	zw.Close()
	return b.Bytes()
}

func problemsOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		var out []string
		for _, p := range e.Problems {
			out = append(out, p.String())
		}
		return strings.Join(out, " | ")
	}
	if err == nil {
		return ""
	}
	return "not an *Error: " + err.Error()
}

const handWritten = `{
  "format": "wittyinfo", "version": 1,
  "identity": {"durationSec": "1:30:00", "fingerprint": {"kind": "sha256-headtail-v1", "value": "ab"}},
  "segments": [
    {"type": "credits", "start": "1:25:00"},
    {"type": "intro", "start": 45, "end": "1:30", "action": "auto", "label": "Opening"},
    {"type": "postcredits", "start": 5390, "end": 5400}
  ],
  "absent": ["recap"],
  "scenes": [{"title": "The heist", "start": "12:00", "thumbnailAt": 725.5}],
  "filters": [{"category": "language", "severity": 2, "start": 600, "end": 601.2, "action": "mute"}],
  "metadata": {"title": "A Film", "year": 2023, "genres": ["Drama"], "someday": true},
  "playback": {"startAt": 3, "subtitleMode": "forced", "volumeDb": -3},
  "notes": "Times checked by hand.",
  "fromTheFuture": {"anything": [1, 2, 3]}
}`

func TestParseHandWrittenManifest(t *testing.T) {
	x, err := Parse(append([]byte{0xef, 0xbb, 0xbf}, handWritten...))
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	if x.Identity.DurationSec != 5400 || x.Segments[1].End == nil || *x.Segments[1].End != 90 {
		t.Errorf("clock strings read wrong: %+v %+v", x.Identity, x.Segments[1])
	}
	if x.Segments[2].Act() != ActMark || x.Segments[0].Act() != ActButton || x.Segments[1].Act() != ActAuto {
		t.Errorf("actions: %v %v %v", x.Segments[0].Act(), x.Segments[1].Act(), x.Segments[2].Act())
	}
	if string(x.Unknown["fromTheFuture"]) != `{"anything": [1, 2, 3]}` || string(x.Metadata.Unknown["someday"]) != "true" {
		t.Errorf("unknown fields not kept: %v %v", x.Unknown, x.Metadata.Unknown)
	}
}

func TestRoundTripKeepsEverything(t *testing.T) {
	x, err := Parse([]byte(handWritten))
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	if _, err := x.SetImage("poster", ".png", pngBytes(t, 20, 30)); err != nil {
		t.Fatal(err)
	}
	x.Files["subtitles/en.forced.srt"] = []byte("1\n00:00:01,000 --> 00:00:02,000\nHi\n")
	x.Subtitles = []Subtitle{{File: "subtitles/en.forced.srt", Language: "en", Forced: true}}
	x.Files["tool-data/state.bin"] = []byte{1, 2, 3} // another tool's, kept unused
	x.Stamp("Willow", "test", "1", time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC))
	b, err := x.Encode()
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	if !IsArchive(b) {
		t.Fatal("not written as an archive")
	}
	y, err := Parse(b)
	if err != nil {
		t.Fatal(problemsOf(err))
	}
	if y.Artwork.First("poster") != "images/poster.png" || len(y.Files["images/poster.png"]) == 0 || len(y.Files["tool-data/state.bin"]) != 3 {
		t.Errorf("members lost: %v %v", y.Artwork, len(y.Files))
	}
	if y.Segments[0].Type != SegIntro || y.Segments[0].ID == "" {
		t.Errorf("not tidied in time order with ids: %+v", y.Segments)
	}
	var future bytes.Buffer
	json.Compact(&future, y.Unknown["fromTheFuture"])
	if future.String() != `{"anything":[1,2,3]}` || string(y.Metadata.Unknown["someday"]) != "true" {
		t.Errorf("unknown fields lost on writing: %v", y.Unknown)
	}
	if y.Authorship.Author != "Willow" || y.Authorship.Created != "2026-11-18T12:00:00Z" {
		t.Errorf("%+v", y.Authorship)
	}
	again, _ := y.Encode()
	if !bytes.Equal(again, b) {
		t.Error("the same content should write the same bytes")
	}
	// Dropping the poster from the artwork drops the picture.
	delete(y.Artwork, "poster")
	y.Tidy()
	if _, ok := y.Files["images/poster.png"]; ok {
		t.Error("a picture nothing names should go")
	}
}

func TestRefusesBadFiles(t *testing.T) {
	man := func(extra string) []byte {
		return []byte(`{"format":"wittyinfo","version":1` + extra + `}`)
	}
	cases := map[string][]byte{
		"not json":           []byte("hello"),
		"wrong format":       []byte(`{"format":"other","version":1}`),
		"trailing":           []byte(`{"format":"wittyinfo","version":1} {}`),
		"end before start":   man(`,"segments":[{"type":"intro","start":90,"end":45}]`),
		"negative":           man(`,"segments":[{"type":"intro","start":-1,"end":45}]`),
		"unknown type":       man(`,"segments":[{"type":"explode","start":1,"end":45}]`),
		"unknown action":     man(`,"segments":[{"type":"intro","start":1,"end":45,"action":"run"}]`),
		"bad clock":          man(`,"segments":[{"type":"intro","start":"1:99","end":45}]`),
		"url artwork":        man(`,"artwork":{"poster":"https://example.com/p.jpg"}`),
		"path artwork":       man(`,"artwork":{"poster":"../../etc/passwd"}`),
		"missing member":     man(`,"artwork":{"poster":"images/poster.jpg"}`),
		"scenes in a show":   man(`,"scope":"show","scenes":[{"title":"x","start":1}]`),
		"season with no num": man(`,"scope":"season"`),
		"too deep":           man(`,"extensions":{"a":` + strings.Repeat("[", 40) + strings.Repeat("]", 40) + `}`),
		"control chars":      man(`,"notes":"a\u0007b"`),
		"duplicate ids":      man(`,"segments":[{"id":"a","type":"intro","start":1,"end":2},{"id":"a","type":"recap","start":3,"end":4}]`),
		"too long label":     man(`,"segments":[{"type":"intro","start":1,"end":2,"label":"` + strings.Repeat("x", 201) + `"}]`),
	}
	many := `,"segments":[` + strings.TrimSuffix(strings.Repeat(`{"type":"custom","start":1,"end":2},`, MaxSegments+1), ",") + `]`
	cases["too many segments"] = man(many)
	for name, b := range cases {
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: read without complaint", name)
		} else if !strings.Contains(err.Error(), "not a usable .wittyinfo") {
			t.Errorf("%s: unclear error %v", name, err)
		}
	}
}

func TestRefusesBadArchives(t *testing.T) {
	ok := []byte(`{"format":"wittyinfo","version":1}`)
	pic := pngBytes(t, 4, 4)
	cases := map[string][]byte{
		"no manifest":   zipOf(t, map[string][]byte{"images/a.png": pic}),
		"climbs out":    zipOf(t, map[string][]byte{ManifestName: ok, "../evil.png": pic}),
		"absolute":      zipOf(t, map[string][]byte{ManifestName: ok, "/abs.png": pic}),
		"backslash":     zipOf(t, map[string][]byte{ManifestName: ok, `images\a.png`: pic}),
		"twice":         zipOf(t, map[string][]byte{ManifestName: ok, "Manifest.json": ok}, ManifestName, "Manifest.json"),
		"fake picture":  zipOf(t, map[string][]byte{ManifestName: ok, "images/a.png": []byte("MZ this is a program")}),
		"wrong ext":     zipOf(t, map[string][]byte{ManifestName: ok, "images/a.jpg": pic}),
		"svg":           zipOf(t, map[string][]byte{ManifestName: ok, "images/a.svg": []byte("<svg/>")}),
		"binary subs":   zipOf(t, map[string][]byte{ManifestName: ok, "subtitles/a.srt": {0xff, 0xfe, 0x00}}),
		"huge manifest": zipOf(t, map[string][]byte{ManifestName: append(ok[:len(ok)-1], []byte(`,"notes":"`+strings.Repeat(" ", MaxManifest)+`"}`)...)}),
	}
	// A bomb: a member that unpacks far beyond what it takes up.
	cases["bomb"] = zipOf(t, map[string][]byte{ManifestName: ok, "other/zeros.bin": make([]byte, MaxOther-1)})
	for name, b := range cases {
		if _, err := Parse(b); err == nil {
			t.Errorf("%s: read without complaint", name)
		}
	}
	// A symbolic link in the archive.
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create(ManifestName)
	w.Write(ok)
	h := &zip.FileHeader{Name: "images/link.png"}
	h.SetMode(0o777 | 1<<27) // os.ModeSymlink
	w, _ = zw.CreateHeader(h)
	w.Write([]byte("/etc/passwd"))
	zw.Close()
	if _, err := Parse(b.Bytes()); err == nil {
		t.Error("a link was read")
	}
}

func TestMatchVideo(t *testing.T) {
	x := New(ScopeItem)
	x.Identity = &Identity{DurationSec: 5400, Fingerprint: &Fingerprint{Kind: FingerprintKind, Value: "abc"}}
	for _, c := range []struct {
		dur    float64
		fp     *Fingerprint
		result string
		usable bool
	}{
		{5400, &Fingerprint{Kind: FingerprintKind, Value: "abc"}, MatchExact, true},
		{1, &Fingerprint{Kind: FingerprintKind, Value: "abc"}, MatchExact, true},
		{5401.5, nil, MatchSameCut, true},
		{5420, &Fingerprint{Kind: FingerprintKind, Value: "zzz"}, MatchSameCut, true}, // within 0.5 %
		{6100, nil, MatchOtherCut, false},                                             // the extended cut
		{5100, nil, MatchOtherCut, false},
		{0, nil, MatchNoCheck, true},
	} {
		m := x.MatchVideo(c.dur, c.fp)
		if m.Result != c.result || m.Usable != c.usable {
			t.Errorf("%v: %+v", c.dur, m)
		}
	}
	tight := 0.5
	x.Identity.ToleranceSec = &tight
	if m := x.MatchVideo(5401, nil); m.Usable {
		t.Errorf("own tolerance ignored: %+v", m)
	}
	if m := New(ScopeShow).MatchVideo(10, nil); m.Result != MatchNotNeeded || !m.Usable {
		t.Errorf("%+v", m)
	}
	if m := New(ScopeItem).MatchVideo(10, nil); m.Result != MatchNoCheck {
		t.Errorf("%+v", m)
	}
}

func TestFingerprint(t *testing.T) {
	small := bytes.Repeat([]byte("a"), 100)
	a, _ := FingerprintReader(bytes.NewReader(small), int64(len(small)))
	big := bytes.Repeat([]byte("b"), 3<<20)
	b1, _ := FingerprintReader(bytes.NewReader(big), int64(len(big)))
	big[len(big)/2] = 'c' // the middle isn't read
	b2, _ := FingerprintReader(bytes.NewReader(big), int64(len(big)))
	big[len(big)-1] = 'c'
	b3, _ := FingerprintReader(bytes.NewReader(big), int64(len(big)))
	if a.Value == b1.Value || b1.Value != b2.Value || b1.Value == b3.Value || a.Kind != FingerprintKind {
		t.Errorf("%v %v %v %v", a, b1, b2, b3)
	}
}

func TestClock(t *testing.T) {
	for s, want := range map[string]float64{"85.5": 85.5, "1:05": 65, "1:02:03": 3723, "0:00:01.25": 1.25} {
		if got, ok := ParseClock(s); !ok || got != want {
			t.Errorf("%s: %v %v", s, got, ok)
		}
	}
	for _, s := range []string{"", "1:60", "a", "-1", "1e3", "1:2:3:4", "1.5:00"} {
		if _, ok := ParseClock(s); ok {
			t.Errorf("%q read", s)
		}
	}
	if Clock(3723.25) != "1:02:03.25" || Clock(65) != "1:05" {
		t.Error(Clock(3723.25), Clock(65))
	}
	b, _ := json.Marshal(Seconds(1.23456))
	if string(b) != "1.235" {
		t.Error(string(b))
	}
}

func TestSchemaKnowsEveryField(t *testing.T) {
	// The JSON Schema in docs/formats lists the same top-level fields as Info.
	raw, err := readSchema()
	if err != nil {
		t.Skip("schema not found:", err)
	}
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	for k := range fieldsOf(typeOfInfo()) {
		if _, ok := s.Properties[k]; !ok {
			t.Errorf("the schema doesn't describe %q", k)
		}
	}
	for k := range s.Properties {
		if !fieldsOf(typeOfInfo())[k] {
			t.Errorf("the schema describes %q, which Info doesn't have", k)
		}
	}
}
