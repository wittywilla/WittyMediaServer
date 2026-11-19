// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

// seedProbe gives a file the probe a real one would have: a picture, Japanese and English
// sound, and an English subtitle track inside.
func seedProbe(t *testing.T, ts *testServer, path string) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	pr := &ffmpeg.Probe{Format: ffmpeg.Format{FormatName: "mov,mp4,m4a,3gp,3g2,mj2", Duration: "1400"},
		Streams: []ffmpeg.Stream{
			{Index: 0, CodecType: "video", CodecName: "h264", Width: 1920, Height: 1080, PixFmt: "yuv420p", Profile: "High", Level: 40},
			{Index: 1, CodecType: "audio", CodecName: "aac", Channels: 2, Tags: map[string]string{"language": "eng"}, Disposition: map[string]int{"default": 1}},
			{Index: 2, CodecType: "audio", CodecName: "aac", Channels: 2, Tags: map[string]string{"language": "jpn"}},
			{Index: 3, CodecType: "subtitle", CodecName: "mov_text", Tags: map[string]string{"language": "eng"}},
		}}
	ts.probes.Lock()
	if ts.probes.m == nil {
		ts.probes.m = map[string]*ffmpeg.Probe{}
	}
	ts.probes.m[fmt.Sprintf("%s|%d|%d", path, st.Size(), st.ModTime().UnixNano())] = pr
	ts.probes.Unlock()
}

// A file's playing preferences and its own subtitles are followed by the server, for every
// player; its labels come in the viewer's language.
func TestInfoPlaying(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	seedProbe(t, ts, f.video.Path)
	x := itemInfo(1400, wittyinfo.Segment{ID: "dream", Type: "custom", Start: 100, End: wittyinfo.Sec(130), Label: "Dream sequence"})
	x.Language = "en"
	x.Scenes = []wittyinfo.Scene{{ID: "s1", Title: "The heist", Start: 60}}
	x.Playback = &wittyinfo.Playback{StartAt: wittyinfo.Sec(8), AudioLanguage: "jpn", SubtitleLanguage: "ger", SubtitleMode: "always", VolumeDB: ptr(-3.0), AspectRatio: "2.39:1"}
	x.Files = map[string][]byte{"subtitles/de.srt": []byte("1\n00:00:20,000 --> 00:00:22,000\nHallo\n")}
	x.Subtitles = []wittyinfo.Subtitle{{File: "subtitles/de.srt", Language: "de", Title: "Deutsch"}}
	x.Translations = map[string]*wittyinfo.Translation{
		"de":    {Segments: map[string]wittyinfo.TextPart{"dream": {Label: "Traumsequenz"}}, Scenes: map[string]wittyinfo.TextPart{"s1": {Title: "Der Raub"}}},
		"pt-BR": {Segments: map[string]wittyinfo.TextPart{"dream": {Label: "Sequência do sonho"}}},
	}
	put(t, strings.TrimSuffix(f.video.Path, ".mkv")+".wittyinfo", encodeInfo(t, x))

	pb := decode[playbackResponse](t, ts.do(t, "POST", f.url("/api/items/%d/playback", f.video), f.adult, map[string]any{"profile": "web"}, "Accept-Language", "de-AT,de;q=0.9,en;q=0.5"))
	if pb.AudioIndex != 1 {
		t.Errorf("the file's sound language: track %d of %+v", pb.AudioIndex, pb.Audio)
	}
	if len(pb.Subtitles) != 2 || pb.Subtitles[1].Language != "de" || pb.SubtitleIndex != 1 {
		t.Fatalf("the file's own subtitles, in its language: %d of %+v", pb.SubtitleIndex, pb.Subtitles)
	}
	if pb.StartAtSec+pb.TimeOffsetSec != 8 {
		t.Errorf("starts at %v / %v", pb.StartAtSec, pb.TimeOffsetSec)
	}
	if pb.Info == nil || pb.Info.VolumeDB == nil || *pb.Info.VolumeDB != -3 || pb.Info.AspectRatio != "2.39:1" || pb.Info.Language != "de" {
		t.Errorf("info: %+v", pb.Info)
	}
	if len(pb.Segments) != 1 || pb.Segments[0].Label != "Traumsequenz" || pb.Chapters[0].Title != "Der Raub" {
		t.Errorf("labels in German: %+v %+v", pb.Segments, pb.Chapters)
	}
	// The subtitle file is served like any other, as WebVTT.
	if pb.Subtitles[1].URL != "" {
		if w := ts.do(t, "GET", pb.Subtitles[1].URL, f.adult, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Hallo") {
			t.Errorf("subtitle %s: %d %q %+v", pb.Subtitles[1].URL, w.Code, w.Body.String(), pb.Subtitles)
		}
	}
	// Someone carrying on, or choosing their own tracks, isn't overruled.
	pb = decode[playbackResponse](t, ts.do(t, "POST", f.url("/api/items/%d/playback", f.video), f.adult, map[string]any{"profile": "web", "startSec": 0, "audio": 0, "subtitle": -1}))
	if pb.AudioIndex != 0 || pb.SubtitleIndex != -1 || pb.StartAtSec+pb.TimeOffsetSec != 0 {
		t.Errorf("the viewer's choice: %d %d %v", pb.AudioIndex, pb.SubtitleIndex, pb.StartAtSec)
	}

	// Markers in other languages: ?lang wins, then Accept-Language; the same language in
	// another region counts; one with no translation keeps the file's own.
	for _, c := range []struct{ q, hdr, want string }{
		{"?lang=pt-BR", "", "Sequência do sonho"}, {"?lang=por", "", "Sequência do sonho"}, {"", "de-CH", "Traumsequenz"}, {"?lang=ja", "", "Dream sequence"},
	} {
		w := ts.do(t, "GET", f.url("/api/media/%d/markers", f.video)+c.q, f.adult, nil, "Accept-Language", c.hdr)
		m := decode[mediaextras.Markers](t, w)
		if len(m.Custom) != 1 || m.Custom[0].Label != c.want {
			t.Errorf("%s %s: %+v", c.q, c.hdr, m.Custom)
		}
	}
	// The editor gets the file as it is, every language.
	a := decode[infoAnswer](t, ts.do(t, "GET", f.url("/api/wittyinfo/item/%d", f.video), f.admin, nil, "Accept-Language", "de"))
	if a.Info.Segments[0].Label != "Dream sequence" || len(a.Info.Translations) != 2 {
		t.Errorf("editor: %+v", a.Info.Segments)
	}

	// The subtitle mode "off" turns subtitles off unless someone picks one.
	x.Playback.SubtitleMode = "off"
	put(t, strings.TrimSuffix(f.video.Path, ".mkv")+".wittyinfo", encodeInfo(t, x))
	library.ForgetInfoFile(strings.TrimSuffix(f.video.Path, ".mkv") + ".wittyinfo")
	if pb := decode[playbackResponse](t, ts.do(t, "POST", f.url("/api/items/%d/playback", f.video), f.adult, map[string]any{"profile": "web"})); pb.SubtitleIndex != -1 {
		t.Errorf("mode off: %d", pb.SubtitleIndex)
	}
	_ = ctx
}

// A show's show.wittyinfo gives the show its title and pictures, in the server's language.
func TestShowInfoDetails(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	ctx := context.Background()
	if err := ts.DB.SetSetting(ctx, "menuLanguage", "fr"); err != nil {
		t.Fatal(err)
	}
	ts.library(t, "shows", filepath.Join(f.root, "tv"))
	showDir := filepath.Join(f.root, "tv", "Une Série")
	x := wittyinfo.New(wittyinfo.ScopeShow)
	x.Language = "en"
	x.Metadata = &wittyinfo.Metadata{Title: "A Show", Overview: "In English."}
	x.Translations = map[string]*wittyinfo.Translation{"fr": {Metadata: &wittyinfo.Metadata{Title: "Une série"}}}
	put(t, filepath.Join(showDir, "show.wittyinfo"), encodeInfo(t, x))
	sh := &library.Show{Title: "Found Title", Path: showDir}
	ts.Scanner.ApplyShowInfoForTest(ctx, sh)
	if sh.Title != "Une série" || sh.Overview != "In English." {
		t.Errorf("%q %q", sh.Title, sh.Overview)
	}
}

func ptr[T any](v T) *T { return &v }
