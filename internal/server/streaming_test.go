package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/playback"
)

// The server's setting is for viewers away from home; a device's own choice wins anywhere;
// and until anyone chooses, nothing changes.
func TestStreamingMode(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	auth := "Authorization: Bearer " + tok
	putRemote(t, h, tok, map[string]any{"enabled": true})
	req := func(from string) *http.Request {
		r := httptest.NewRequest("POST", "/api/items/1/playback", nil)
		r.RemoteAddr = from + ":1"
		return r.WithContext(context.WithValue(r.Context(), clientKey, s.state().policy.Resolve(r)))
	}
	check := func(from, device, mode, by string) {
		t.Helper()
		if m, f := s.streamMode(req(from), device); m != mode || f != by {
			t.Errorf("from %s, device %q: %s (%s), want %s (%s)", from, device, m, f, mode, by)
		}
	}
	check(lan, "", playback.StreamStandard, playback.StreamFromDefault)
	check(tail, "", playback.StreamStandard, playback.StreamFromDefault)
	check(lan, playback.StreamRobust, playback.StreamRobust, playback.StreamFromDevice)

	if r := call(h, "PUT", "/api/settings", lan, map[string]string{remoteStreamingKey: "turbo"}, auth); r.code != 400 {
		t.Errorf("an unknown mode was saved: %d %s", r.code, r.body)
	}
	if r := call(h, "PUT", "/api/settings", lan, map[string]string{remoteStreamingKey: playback.StreamRobust}, auth); r.code != 200 {
		t.Fatalf("PUT settings: %d %s", r.code, r.body)
	}
	check(tail, "", playback.StreamRobust, playback.StreamFromServer)
	check(lan, "", playback.StreamStandard, playback.StreamFromDefault)
	check(tail, playback.StreamStandard, playback.StreamStandard, playback.StreamFromDevice)

	// Apps read the server's choice without signing in.
	var caps struct {
		Features []string `json:"features"`
		Playback struct {
			RemoteStreaming string   `json:"remoteStreaming"`
			Modes           []string `json:"streamingModes"`
		} `json:"playback"`
	}
	call(h, "GET", "/api/capabilities", lan, nil).json(t, &caps)
	if caps.Playback.RemoteStreaming != playback.StreamRobust || len(caps.Playback.Modes) != 2 || !strings.Contains(strings.Join(caps.Features, ","), "robust-streaming") {
		t.Errorf("capabilities: %+v", caps)
	}
}

func TestSpeedTest(t *testing.T) {
	s, h := remoteServer(t, nil)
	tok := adminToken(t, s, h)
	auth := "Authorization: Bearer " + tok
	r := call(h, "GET", "/api/playback/speedtest?kb=64", lan, nil, auth)
	if r.code != 200 || len(r.body) != 64<<10 || r.h.Get("Cache-Control") != "no-store" {
		t.Fatalf("speed test: %d, %d bytes, %q", r.code, len(r.body), r.h.Get("Cache-Control"))
	}
	if strings.Count(r.body, "\x00") > 1024 {
		t.Error("the bytes should not compress")
	}
	for _, q := range []string{"?kb=1", "?kb=5000"} {
		if r := call(h, "GET", "/api/playback/speedtest"+q, lan, nil, auth); r.code != 400 {
			t.Errorf("%s: %d", q, r.code)
		}
	}
	if r := call(h, "GET", "/api/playback/speedtest", lan, nil); r.code != 401 {
		t.Errorf("without signing in: %d", r.code)
	}
}

// Every rung is its own stream in one group, with segments short enough to change rung or
// pick up again quickly, and a rate it keeps to.
func TestLadderSpecs(t *testing.T) {
	base := hls.Spec{ID: "v-x-n1", Duration: 600, Quality: ffmpeg.Quality{Preset: "veryfast", CRF: 21, AudioRate: "192k"},
		Variant: &hls.Variant{BandwidthKbps: 9000, Codecs: "avc1.640028,mp4a.40.2", Width: 1920, Height: 1080, Range: "SDR"}}
	rungs := []playback.Rung{{Height: 480, Kbps: 1400}, {Height: 720, Kbps: 2800}, {Height: 240, Kbps: 400}}
	specs := ladderSpecs(base, "v-x-r1", rungs, 1920, 1080)
	if len(specs) != 3 {
		t.Fatalf("%d specs", len(specs))
	}
	seen := map[string]bool{}
	for i, sp := range specs {
		if sp.Group != "v-x-r1" || !strings.HasPrefix(sp.ID, "v-x-r1-q") || seen[sp.ID] || !hls.IsSessionDir(sp.ID) {
			t.Errorf("rung %d: id %q group %q", i, sp.ID, sp.Group)
		}
		seen[sp.ID] = true
		if sp.SegmentSec != playback.RobustSegmentSec || sp.MaxHeight != rungs[i].Height {
			t.Errorf("rung %d: %v s segments, %d lines", i, sp.SegmentSec, sp.MaxHeight)
		}
		if cap := ffmpeg.ParseRate(sp.Quality.CapRate); cap <= 0 || cap >= int64(rungs[i].Kbps)*1000 {
			t.Errorf("rung %d: video limit %q for a %d kb/s rung", i, sp.Quality.CapRate, rungs[i].Kbps)
		}
		if sp.Variant.BandwidthKbps != rungs[i].Kbps || sp.Variant.Height != rungs[i].Height || sp.Variant.Width%2 != 0 {
			t.Errorf("rung %d: variant %+v", i, sp.Variant)
		}
	}
	if specs[1].Variant.Width != 1280 || specs[2].Quality.AudioRate != "96k" {
		t.Errorf("720p should be 1280 wide (%d), and the lowest rung's sound 96k (%s)", specs[1].Variant.Width, specs[2].Quality.AudioRate)
	}
	if base.Variant.BandwidthKbps != 9000 || base.SegmentSec != 0 {
		t.Error("the base spec was changed")
	}
	if got := ladderParam(rungs); got != "480,720,240" {
		t.Errorf("ladder parameter %q", got)
	}
}
