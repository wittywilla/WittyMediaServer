package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// TestFFmpegBench measures what FFmpeg costs the server in the common ways of playing
// something: how long until the first segment, how fast it runs after that, its processor
// time and peak memory, and how long a seek takes. Every case goes through the real API
// (or, for a channel, the live session the API would start), so it measures what a viewer
// waits for. It reads the media and writes nothing beside it.
//
// It does nothing without WMS_FFBENCH=1. docs/FFMPEG-BENCHMARK.md says how to run it and
// what each setting does.
func TestFFmpegBench(t *testing.T) {
	if os.Getenv("WMS_FFBENCH") == "" {
		t.Skip("set WMS_FFBENCH=1 to measure FFmpeg (docs/FFMPEG-BENCHMARK.md)")
	}
	tools, err := ffmpeg.Locate(os.Getenv("WMS_FFMPEG"), os.Getenv("WMS_FFPROBE"))
	if err != nil {
		t.Skip("no FFmpeg here: set WMS_FFMPEG")
	}
	cfg := benchConfigFromEnv()
	ts := formatServer(t, tools)
	no := func() bool { return false }
	ts.Scanner.SaveMeta, ts.Scanner.WikidataOn, ts.Scanner.TVMazeOn, ts.Scanner.MusicBrainzOn = no, no, no, no
	ctx := context.Background()
	for k, v := range cfg.settings {
		if err := ts.DB.SetSetting(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	if cfg.settings["hwEncoders"] != "" {
		tools.ProbeHardware(ctx)
		list, _ := tools.HardwareStatus()
		for _, h := range list {
			if h.Tested {
				t.Logf("hardware encoder %s: working=%v %s", h.Name, h.Working, h.Error)
			}
		}
	}
	_, tok := ts.user(t, "bench", auth.KindAdult, true, "")
	srv := httptest.NewServer(ts.h)
	defer srv.Close()
	b := &bench{t: t, ts: ts, cfg: cfg, base: srv.URL, tok: tok, client: srv.Client(), libs: map[string]int64{}, items: map[string]*library.Item{}}

	var results []benchResult
	for _, c := range benchCases(cfg) {
		if cfg.only != nil && !cfg.only.MatchString(c.name) {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			b.t = t
			src := c.find(b)
			if src == "" {
				t.Skip("no media for this case under " + strings.Join(cfg.roots, ", "))
			}
			t.Logf("playing %s", src)
			if c.modules != nil {
				modtest.Need(t, c.modules...)
			}
			for run := 1; run <= cfg.runs; run++ {
				r := c.run(b, src)
				r.Case, r.Run = c.name, run
				t.Logf("run %d: %s", run, r.line())
				results = append(results, r)
				b.settle()
			}
		})
	}
	b.t = t
	t.Log("\n" + benchTable(results))
	if cfg.out != "" {
		host, _ := os.Hostname()
		raw, _ := json.MarshalIndent(map[string]any{
			"when": time.Now().Format(time.RFC3339), "host": host, "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(),
			"ffmpeg": tools.Version, "settings": cfg.settings, "segments": cfg.segs, "results": results,
		}, "", "  ")
		if err := os.WriteFile(cfg.out, raw, 0o644); err != nil {
			t.Error(err)
		}
	}
}

type benchConfig struct {
	roots    []string // where media is looked for, in this order
	runs     int
	segs     int // segments timed after the first, for the speed
	only     *regexp.Regexp
	out      string
	settings map[string]string
	wall     time.Duration // the longest a case's speed is timed for
}

func benchConfigFromEnv() benchConfig {
	c := benchConfig{runs: 3, segs: 8, settings: map[string]string{"saveMetadata": "0"}, wall: 90 * time.Second}
	roots := os.Getenv("WMS_FFBENCH_MEDIA")
	if roots == "" {
		if runtime.GOOS == "windows" {
			roots = `D:\WittyDemoMedia;D:\ServerMedia`
		} else {
			roots = "/mnt/d/WittyDemoMedia:/mnt/d/ServerMedia"
		}
	}
	for _, r := range filepath.SplitList(roots) {
		if st, err := os.Stat(r); err == nil && st.IsDir() {
			c.roots = append(c.roots, r)
		}
	}
	if n, err := strconv.Atoi(os.Getenv("WMS_FFBENCH_RUNS")); err == nil && n > 0 {
		c.runs = n
	}
	if n, err := strconv.Atoi(os.Getenv("WMS_FFBENCH_SEGMENTS")); err == nil && n > 0 {
		c.segs = n
	}
	if v := os.Getenv("WMS_FFBENCH_ONLY"); v != "" {
		c.only = regexp.MustCompile(v)
	}
	c.out = os.Getenv("WMS_FFBENCH_OUT")
	// Settings as the admin would set them: hwEncoders=h264_nvenc,lowPower=1,...
	for _, kv := range strings.Split(os.Getenv("WMS_FFBENCH_SETTINGS"), ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
			c.settings[k] = v
		}
	}
	return c
}

type bench struct {
	t      *testing.T
	ts     *testServer
	cfg    benchConfig
	base   string
	tok    string
	client *http.Client
	libs   map[string]int64
	found  map[string]string
	items  map[string]*library.Item
}

// benchResult is one run of one case. Times are milliseconds; 0 means not measured.
type benchResult struct {
	Case     string  `json:"case"`
	Run      int     `json:"run"`
	Decision string  `json:"decision"`
	StartMs  float64 `json:"startMs"`    // the request that sets playing up
	FirstMs  float64 `json:"firstSegMs"` // from that request to the first segment in hand
	Speed    float64 `json:"speed"`      // times real time, after the first segment
	CPUPerM  float64 `json:"cpuSecPerMin"`
	PeakMB   float64 `json:"peakMB"` // the largest FFmpeg process, at its peak
	Procs    int     `json:"procs"`
	SeekMs   float64 `json:"seekMs"` // the mean of the seeks
	Err      string  `json:"err,omitempty"`
}

func (r benchResult) line() string {
	if r.Err != "" {
		return "FAILED: " + r.Err
	}
	return fmt.Sprintf("%s start %.0f ms, first segment %.0f ms, %.1fx, %.1f CPU s/min, peak %.0f MB (%d procs), seek %.0f ms",
		r.Decision, r.StartMs, r.FirstMs, r.Speed, r.CPUPerM, r.PeakMB, r.Procs, r.SeekMs)
}

type benchCase struct {
	name    string
	modules []string
	find    func(b *bench) string
	run     func(b *bench, src string) benchResult
}

func benchCases(cfg benchConfig) []benchCase {
	h264 := probeFind("h264-1080", func(p *ffmpeg.Probe) bool {
		v := p.Video()
		return v != nil && v.CodecName == "h264" && v.Height >= 1000 && v.Height <= 1100
	})
	// Matroska, which browsers don't open, so the server copies the picture into HLS.
	h264mkv := probeFind("h264-1080-mkv", func(p *ffmpeg.Probe) bool {
		v := p.Video()
		return v != nil && v.CodecName == "h264" && v.Height >= 1000 && v.Height <= 1100 && strings.HasPrefix(p.Format.FormatName, "matroska")
	})
	return []benchCase{
		{name: "remux-h264-1080p", find: h264mkv, run: playFile(map[string]any{})},
		{name: "x264-1080p", find: h264, run: playFile(map[string]any{"forceTranscode": true})},
		{name: "x264-1080p-to-720p-4M", find: h264, run: playFile(map[string]any{"forceTranscode": true, "maxHeight": 720, "maxBitrateKbps": 4000})},
		{name: "hevc-1080p-to-h264", find: probeFind("hevc-1080", func(p *ffmpeg.Probe) bool {
			v := p.Video()
			return v != nil && v.CodecName == "hevc" && v.Height >= 1000 && v.Height <= 1100 && v.Range() == "sdr"
		}), run: playFile(map[string]any{"forceTranscode": true})},
		{name: "tonemap-4k-hdr-to-1080p", find: probeFind("hdr-4k", func(p *ffmpeg.Probe) bool {
			v := p.Video()
			return v != nil && v.Width >= 3800 && v.Range() != "sdr"
		}), run: playFile(map[string]any{"forceTranscode": true, "maxHeight": 1080})},
		{name: "burn-text-subtitles", find: sidecarFind, run: playFile(map[string]any{"forceTranscode": true, "burnSubtitle": true, "subtitle": "text"})},
		{name: "burn-picture-subtitles", find: probeFind("pgs", func(p *ffmpeg.Probe) bool {
			v := p.Video()
			return v != nil && v.Height <= 1100 && slices.ContainsFunc(p.Subtitles(), func(s ffmpeg.Stream) bool {
				return s.CodecName == "hdmv_pgs_subtitle" || s.CodecName == "dvd_subtitle"
			})
		}), run: playFile(map[string]any{"forceTranscode": true, "burnSubtitle": true, "subtitle": "picture"})},
		{name: "audio-transcode", find: probeFind("audio", func(p *ffmpeg.Probe) bool {
			return p.Video() == nil && len(p.Audio()) > 0 && p.Duration() > 120
		}), run: playAudio},
		// A channel copies H.264 as it is (the default), and converts anything else.
		{name: "live-channel-1080p", find: h264, run: playLive(true)},
		{name: "live-channel-1080p-converted", find: h264, run: playLive(false)},
		{name: "dvd-title", modules: []string{"wittydvd"}, find: discFind(`(?i)\.iso$`, "DVD"), run: playDisc},
		{name: "bluray-title", modules: []string{"wittybluray"}, find: discFind(`(?i)\.iso$`, "Blu-ray", "BluRay ISOs"), run: playDisc},
	}
}

// probeFind looks through the media folders for the first file that fits, probing files
// of a useful size, and remembers it for the other cases that want the same.
func probeFind(key string, fits func(*ffmpeg.Probe) bool) func(b *bench) string {
	return func(b *bench) string {
		if b.found == nil {
			b.found = map[string]string{}
		}
		if p, ok := b.found[key]; ok {
			return p
		}
		exts := map[string]bool{".mkv": true, ".mp4": true, ".webm": true, ".m4v": true, ".mov": true, ".ts": true, ".m2ts": true}
		least := int64(20 << 20)
		if key == "audio" {
			// Lossless first: what a browser most often can't play.
			exts, least = map[string]bool{".flac": true, ".wav": true, ".m4a": true, ".wma": true, ".mp3": true}, 2<<20
		}
		var hit string
		probed := 0
		for _, root := range b.cfg.roots {
			_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil || hit != "" || probed > 400 {
					return filepath.SkipDir
				}
				if d.IsDir() {
					if strings.HasPrefix(d.Name(), "_") || strings.HasPrefix(d.Name(), ".") {
						return filepath.SkipDir
					}
					return nil
				}
				if !exts[strings.ToLower(filepath.Ext(p))] {
					return nil
				}
				if st, err := d.Info(); err != nil || st.Size() < least {
					return nil
				}
				probed++
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				pr, err := b.ts.Tools.ProbeFile(ctx, p)
				cancel()
				if err == nil && pr.Duration() >= 120 && fits(pr) {
					hit = p
				}
				return nil
			})
			if hit != "" {
				break
			}
		}
		b.found[key] = hit
		return hit
	}
}

// sidecarFind: a film with a subtitle file beside it, small enough to convert in real time.
func sidecarFind(b *bench) string {
	for _, root := range b.cfg.roots {
		var hit string
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || hit != "" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".srt") {
				return nil
			}
			dir := filepath.Dir(p)
			entries, _ := os.ReadDir(dir)
			for _, e := range entries {
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if ext != ".mkv" && ext != ".mp4" && ext != ".webm" {
					continue
				}
				v := filepath.Join(dir, e.Name())
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				pr, err := b.ts.Tools.ProbeFile(ctx, v)
				cancel()
				if err == nil && pr.Video() != nil && pr.Video().Height <= 1100 && pr.Duration() >= 120 {
					hit = v
					return filepath.SkipAll
				}
			}
			return nil
		})
		if hit != "" {
			return hit
		}
	}
	return ""
}

// discFind: the first disc image in a folder whose path names one of these. Encrypted
// discs are refused, so there is nothing of theirs to measure.
func discFind(pattern string, folders ...string) func(b *bench) string {
	re := regexp.MustCompile(pattern)
	return func(b *bench) string {
		for _, root := range b.cfg.roots {
			var hit string
			_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
				if err != nil || hit != "" {
					return filepath.SkipDir
				}
				if d.IsDir() || !re.MatchString(p) {
					return nil
				}
				for _, f := range folders {
					if strings.Contains(p, string(filepath.Separator)+f) && !strings.Contains(p, "3D") && !bdsession.Scrambled(p) {
						hit = p
						return filepath.SkipAll
					}
				}
				return nil
			})
			if hit != "" {
				return hit
			}
		}
		return ""
	}
}

// library gives the folder of src a library of its own, once.
func (b *bench) library(typ, dir string) int64 {
	if id, ok := b.libs[typ+dir]; ok {
		return id
	}
	id := b.ts.library(b.t, typ, dir)
	b.libs[typ+dir] = id
	return id
}

// fileItem stores a file as an item without scanning its folder, which may hold hundreds.
func (b *bench) fileItem(src, kind string) *library.Item {
	if it, ok := b.items[kind+src]; ok {
		return it
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pr, err := b.ts.Tools.ProbeFile(ctx, src)
	if err != nil {
		b.t.Fatal(err)
	}
	st, _ := os.Stat(src)
	typ := "movies"
	if kind == "audio" {
		typ = "music"
	}
	it := &library.Item{LibraryID: b.library(typ, filepath.Dir(src)), Kind: kind, Path: src, Title: filepath.Base(src), RuntimeSec: pr.Duration(), SizeBytes: st.Size()}
	if kind == "audio" {
		it.VideoCodec, it.Container = pr.Audio()[0].CodecName, strings.TrimPrefix(filepath.Ext(src), ".")
	}
	b.items[kind+src] = b.ts.item(b.t, it)
	return it
}

func (b *bench) do(method, target string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = strings.NewReader(string(raw))
	}
	if !strings.HasPrefix(target, "http") {
		target = b.base + target
	}
	req, err := http.NewRequest(method, target, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+b.tok)
	return b.client.Do(req)
}

// fetch reads a whole answer and fails on anything but success.
func (b *bench) fetch(method, target string, body any) ([]byte, error) {
	resp, err := b.do(method, target, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: %d %s", method, target, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, err
}

// settle waits for the last case's FFmpeg to go, so it isn't counted in the next.
func (b *bench) settle() {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && len(benchChildren(nil)) > 0 {
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second) // the disc's cache and the CPU's clock settle too
}

type playbackReply struct {
	Decision  string  `json:"decision"`
	Protocol  string  `json:"protocol"`
	URL       string  `json:"url"`
	StreamID  string  `json:"streamId"`
	Duration  float64 `json:"durationSec"`
	Subtitles []struct {
		Index int    `json:"index"`
		Codec string `json:"codec"`
		Text  bool   `json:"text"`
	} `json:"subtitles"`
}

func (b *bench) playback(id int64, body map[string]any) (playbackReply, error) {
	var a playbackReply
	body["profile"] = "web"
	data, err := b.fetch("POST", fmt.Sprintf("/api/items/%d/playback", id), body)
	if err != nil {
		return a, err
	}
	return a, json.Unmarshal(data, &a)
}

// playFile plays a file as the web app would, with these extras in the request.
func playFile(extra map[string]any) func(b *bench, src string) benchResult {
	return func(b *bench, src string) benchResult {
		it := b.fileItem(src, "video")
		body := map[string]any{}
		for k, v := range extra {
			body[k] = v
		}
		// "subtitle" names a kind of track; the request needs its number.
		if kind, ok := body["subtitle"].(string); ok {
			a, err := b.playback(it.ID, map[string]any{})
			if err != nil {
				return benchResult{Err: err.Error()}
			}
			n := -1
			for _, s := range a.Subtitles {
				if s.Text == (kind == "text") {
					n = s.Index
					break
				}
			}
			if a.StreamID != "" {
				_, _ = b.fetch("DELETE", "/api/hls/"+a.StreamID, nil)
			}
			if n < 0 {
				return benchResult{Err: "no " + kind + " subtitles"}
			}
			body["subtitle"] = n
			b.settle()
		}
		s := startSampler()
		defer s.close()
		t0 := time.Now()
		a, err := b.playback(it.ID, body)
		if err != nil {
			return benchResult{Err: err.Error()}
		}
		r := benchResult{Decision: a.Decision, StartMs: ms(time.Since(t0))}
		if a.Protocol != "hls" {
			r.Err = "plays as " + a.Protocol
			return r
		}
		defer func() { _, _ = b.fetch("DELETE", "/api/hls/"+a.StreamID, nil) }()
		b.timeHLS(&r, a.URL, t0, s)
		if r.Err != "" {
			return r
		}
		r.SeekMs = b.seekFile(it.ID, body, a, s)
		return r
	}
}

// hlsEntry is a segment of a playlist: its address and length.
type hlsEntry struct {
	uri string
	dur float64
}

var mapRe = regexp.MustCompile(`#EXT-X-MAP:URI="([^"]+)"`)

func (b *bench) playlist(u string) (entries []hlsEntry, init string, err error) {
	data, err := b.fetch("GET", u, nil)
	if err != nil {
		return nil, "", err
	}
	text := string(data)
	if strings.Contains(text, "#EXT-X-STREAM-INF") { // a master playlist: follow its stream
		for _, line := range strings.Split(text, "\n") {
			if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
				return b.playlist(resolve(u, line))
			}
		}
	}
	if m := mapRe.FindStringSubmatch(text); m != nil {
		init = resolve(u, m[1])
	}
	var dur float64
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, "#EXTINF:"); ok {
			dur, _ = strconv.ParseFloat(strings.TrimSuffix(strings.Split(v, ",")[0], ","), 64)
		} else if line != "" && !strings.HasPrefix(line, "#") {
			entries = append(entries, hlsEntry{uri: resolve(u, line), dur: dur})
		}
	}
	return entries, init, nil
}

func resolve(base, ref string) string {
	bu, err := url.Parse(base)
	if err != nil {
		return ref
	}
	ru, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return bu.ResolveReference(ru).String()
}

// timeHLS fetches the first segment, then the next ones in order as a player would, and
// fills in the timings and what FFmpeg used up to the last of them.
func (b *bench) timeHLS(r *benchResult, playlist string, t0 time.Time, s *sampler) {
	entries, init, err := b.playlist(playlist)
	if err != nil {
		r.Err = err.Error()
		return
	}
	if len(entries) == 0 {
		r.Err = "an empty playlist"
		return
	}
	if init != "" {
		if _, err := b.fetch("GET", init, nil); err != nil {
			r.Err = err.Error()
			return
		}
	}
	if _, err := b.fetch("GET", entries[0].uri, nil); err != nil {
		r.Err = err.Error()
		return
	}
	t1 := time.Now()
	r.FirstMs = ms(t1.Sub(t0))
	media := entries[0].dur
	var played float64
	t2 := t1
	for i := 1; i <= b.cfg.segs && time.Since(t1) < b.cfg.wall; i++ {
		for i >= len(entries) && time.Since(t1) < b.cfg.wall { // a growing playlist
			time.Sleep(200 * time.Millisecond)
			if entries, _, err = b.playlist(playlist); err != nil {
				r.Err = err.Error()
				return
			}
		}
		if i >= len(entries) {
			break
		}
		if _, err := b.fetch("GET", entries[i].uri, nil); err != nil {
			r.Err = err.Error()
			return
		}
		t2 = time.Now()
		played += entries[i].dur
	}
	media += played
	if wall := t2.Sub(t1).Seconds(); wall > 0 {
		r.Speed = played / wall
	}
	cpu, peak, n := s.snapshot()
	if media > 0 {
		r.CPUPerM = cpu / media * 60
	}
	r.PeakMB, r.Procs = float64(peak)/(1<<20), n
}

// seekFile jumps to 40% and 70% of the way in, as a viewer dragging the bar would, and
// says how long the picture took to come back on average.
func (b *bench) seekFile(id int64, body map[string]any, a playbackReply, s *sampler) float64 {
	var total float64
	n := 0
	for _, at := range []float64{0.4, 0.7} {
		t0 := time.Now()
		if a.Decision == "remux" || a.Decision == "remux+audio" || strings.HasPrefix(a.Decision, "remux") {
			// A copy can't be entered part-way: the player asks for a new stream there.
			nb := map[string]any{"startSec": a.Duration * at}
			for k, v := range body {
				nb[k] = v
			}
			na, err := b.playback(id, nb)
			if err != nil {
				return 0
			}
			entries, init, err := b.playlist(na.URL)
			if err != nil || len(entries) == 0 {
				return 0
			}
			if init != "" {
				_, _ = b.fetch("GET", init, nil)
			}
			if _, err := b.fetch("GET", entries[0].uri, nil); err != nil {
				return 0
			}
			_, _ = b.fetch("DELETE", "/api/hls/"+na.StreamID, nil)
		} else {
			entries, init, err := b.playlist(a.URL)
			if err != nil || len(entries) == 0 {
				return 0
			}
			k := int(float64(len(entries)) * at)
			if init != "" {
				_, _ = b.fetch("GET", init, nil)
			}
			if _, err := b.fetch("GET", entries[k].uri, nil); err != nil {
				return 0
			}
		}
		total += ms(time.Since(t0))
		n++
	}
	return total / float64(n)
}

// playAudio plays a long sound file converted, as for a browser that can't play it.
func playAudio(b *bench, src string) benchResult {
	it := b.fileItem(src, "audio")
	s := startSampler()
	defer s.close()
	t0 := time.Now()
	a, err := b.playback(it.ID, map[string]any{"forceTranscode": true})
	if err != nil {
		return benchResult{Err: err.Error()}
	}
	r := benchResult{Decision: a.Decision, StartMs: ms(time.Since(t0))}
	resp, err := b.do("GET", a.URL, nil)
	if err != nil {
		r.Err = err.Error()
		return r
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	if _, err := io.ReadFull(resp.Body, buf[:1]); err != nil {
		r.Err = err.Error()
		return r
	}
	t1 := time.Now()
	r.FirstMs = ms(t1.Sub(t0))
	_, _ = io.Copy(io.Discard, resp.Body)
	if wall := time.Since(t1).Seconds(); wall > 0 {
		r.Speed = it.RuntimeSec / wall
	}
	cpu, peak, n := s.snapshot()
	r.CPUPerM, r.PeakMB, r.Procs = cpu/it.RuntimeSec*60, float64(peak)/(1<<20), n
	// A seek is a new request from that point.
	t2 := time.Now()
	if resp2, err := b.do("GET", a.URL+"&start="+strconv.FormatFloat(it.RuntimeSec/2, 'f', 1, 64), nil); err == nil {
		_, _ = io.ReadFull(resp2.Body, buf[:1])
		r.SeekMs = ms(time.Since(t2))
		resp2.Body.Close()
	}
	return r
}

// playLive runs a channel's live session on a file, as a library channel does, for a while
// at viewing speed. Its speed is 1 by design, so CPU is what it costs to keep up.
func playLive(copyOK bool) func(b *bench, src string) benchResult {
	return func(b *bench, src string) benchResult { return b.live(src, copyOK) }
}

func (b *bench) live(src string, copyOK bool) benchResult {
	it := b.fileItem(src, "video")
	ts := b.ts
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.Background())
	q, h, preset := ts.liveQuality(req)
	id := fmt.Sprintf("bench-live-%d", time.Now().UnixNano())
	s := startSampler()
	defer s.close()
	t0 := time.Now()
	ls, err := ts.HLS.EnsureLive(hls.LiveSpec{
		ID: id, Quality: q, MaxHeight: h, Preset: preset,
		Feed: func(ctx context.Context, _ time.Time) (*hls.LiveInput, error) {
			// Every run plays the programme from its start.
			return ts.itemLiveInput(ctx, &channels.Programme{ItemID: it.ID}, 0, it.RuntimeSec, q, h, copyOK)
		},
	})
	if err != nil {
		return benchResult{Err: err.Error()}
	}
	defer ts.HLS.StopLive(id)
	r := benchResult{Decision: "live, converted", StartMs: ms(time.Since(t0))}
	if copyOK {
		r.Decision = "live, copied"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := ls.WaitReady(ctx); err != nil {
		r.Err = err.Error()
		return r
	}
	r.FirstMs = ms(time.Since(t0))
	t1 := time.Now()
	for time.Since(t1) < 30*time.Second {
		ls.Touch()
		time.Sleep(time.Second)
	}
	cpu, peak, n := s.snapshot()
	wall := time.Since(t0).Seconds()
	r.Speed = 1
	r.CPUPerM, r.PeakMB, r.Procs = cpu/wall*60, float64(peak)/(1<<20), n
	return r
}

// playDisc scans a disc's folder, opens its main title as the player does and plays it.
func playDisc(b *bench, src string) benchResult {
	dir := filepath.Dir(src)
	id, ok := b.libs["discs"+dir]
	if !ok {
		id = b.library("discs", dir)
		scanAndWait(b.t, b.ts.Server, id)
	}
	items, _, err := b.ts.Store.Items(context.Background(), library.ItemQuery{LibraryID: id, NoTotal: true, Limit: 50})
	if err != nil || len(items) == 0 {
		return benchResult{Err: fmt.Sprintf("the scan found nothing: %v", err)}
	}
	var it *library.Item
	for _, x := range items {
		if filepath.Clean(x.Path) == filepath.Clean(src) {
			it = x
		}
	}
	if it == nil {
		it = items[0]
	}
	s := startSampler()
	defer s.close()
	t0 := time.Now()
	data, err := b.fetch("POST", fmt.Sprintf("/api/items/%d/dvd", it.ID), map[string]any{"mode": "title"})
	if err != nil {
		return benchResult{Err: err.Error()}
	}
	var open struct {
		SessionID    string `json:"sessionId"`
		Presentation struct {
			Media *struct {
				URL      string  `json:"url"`
				Duration float64 `json:"durationSec"`
			} `json:"media"`
		} `json:"presentation"`
	}
	if err := json.Unmarshal(data, &open); err != nil || open.Presentation.Media == nil {
		return benchResult{Err: "the disc gave nothing to play: " + string(data)}
	}
	// A disc's stream outlives its session, for the next viewer: stop both, so each run starts afresh.
	defer func() {
		_, _ = b.fetch("DELETE", "/api/dvd/"+open.SessionID, nil)
		if id, ok := strings.CutPrefix(open.Presentation.Media.URL, "/api/hls/"); ok {
			_, _ = b.fetch("DELETE", "/api/hls/"+strings.Split(id, "/")[0], nil)
		}
	}()
	r := benchResult{Decision: "disc", StartMs: ms(time.Since(t0))}
	u := resolve(b.base+"/api/", open.Presentation.Media.URL)
	if strings.HasPrefix(open.Presentation.Media.URL, "/") {
		u = b.base + open.Presentation.Media.URL
	}
	b.timeHLS(&r, u, t0, s)
	if r.Err != "" {
		return r
	}
	r.SeekMs = b.seekFile(it.ID, nil, playbackReply{Decision: "disc", URL: u}, s)
	return r
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// ---------------------------------------------------------------------------
// FFmpeg's use of the machine
// ---------------------------------------------------------------------------

type benchProcInfo struct {
	pid, ppid int
	name      string
}

type benchProcStat struct {
	pid  int
	cpu  float64 // seconds
	peak uint64  // bytes
	h    benchProcHandle
}

// benchChildren lists the FFmpeg and ffprobe processes this test has started, at any depth
// (a module's helper may start them), leaving out those in skip.
func benchChildren(skip map[int]bool) []benchProcInfo {
	all := benchListProcs()
	kids := map[int][]benchProcInfo{}
	for _, p := range all {
		kids[p.ppid] = append(kids[p.ppid], p)
	}
	var out []benchProcInfo
	var walk func(pid, depth int)
	walk = func(pid, depth int) {
		if depth > 6 {
			return
		}
		for _, c := range kids[pid] {
			if c.pid == pid {
				continue
			}
			if (c.name == "ffmpeg" || c.name == "ffprobe") && !skip[c.pid] {
				out = append(out, c)
			}
			walk(c.pid, depth+1)
		}
	}
	walk(os.Getpid(), 0)
	return out
}

// sampler watches the FFmpeg processes that start after it does, ten times a second.
type sampler struct {
	mu   sync.Mutex
	skip map[int]bool
	seen map[int]*benchProcStat
	stop chan struct{}
	done chan struct{}
}

func startSampler() *sampler {
	s := &sampler{skip: map[int]bool{}, seen: map[int]*benchProcStat{}, stop: make(chan struct{}), done: make(chan struct{})}
	for _, p := range benchChildren(nil) {
		s.skip[p.pid] = true
	}
	s.tick()
	go func() {
		defer close(s.done)
		for {
			select {
			case <-s.stop:
				return
			case <-time.After(100 * time.Millisecond):
				s.tick()
			}
		}
	}()
	return s
}

func (s *sampler) tick() {
	kids := benchChildren(s.skip)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range kids {
		if s.seen[k.pid] == nil {
			s.seen[k.pid] = &benchProcStat{pid: k.pid}
		}
	}
	for _, st := range s.seen {
		benchReadProc(st)
	}
}

// snapshot is the processor time of every process seen so far, and the highest peak of any.
func (s *sampler) snapshot() (cpu float64, peak uint64, n int) {
	s.tick()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.seen {
		cpu += st.cpu
		peak = max(peak, st.peak)
	}
	return cpu, peak, len(s.seen)
}

func (s *sampler) close() {
	close(s.stop)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.seen {
		benchReleaseProc(st)
	}
}

// ---------------------------------------------------------------------------
// The table
// ---------------------------------------------------------------------------

// benchTable gives each case's median and its spread (lowest to highest) over the runs.
func benchTable(rs []benchResult) string {
	byCase := map[string][]benchResult{}
	var order []string
	for _, r := range rs {
		if _, ok := byCase[r.Case]; !ok {
			order = append(order, r.Case)
		}
		byCase[r.Case] = append(byCase[r.Case], r)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "| Case | Decision | Start ms | First segment ms | Speed × | CPU s per min | Peak MB | Seek ms |\n|---|---|---|---|---|---|---|---|\n")
	for _, c := range order {
		var ok []benchResult
		failed := ""
		for _, r := range byCase[c] {
			if r.Err == "" {
				ok = append(ok, r)
			} else {
				failed = r.Err
			}
		}
		if len(ok) == 0 {
			fmt.Fprintf(&b, "| %s | failed: %s | | | | | | |\n", c, strings.ReplaceAll(failed, "|", "/"))
			continue
		}
		col := func(f func(benchResult) float64, prec int) string {
			var v []float64
			for _, r := range ok {
				v = append(v, f(r))
			}
			sort.Float64s(v)
			med := v[len(v)/2]
			if len(v)%2 == 0 {
				med = (v[len(v)/2-1] + v[len(v)/2]) / 2
			}
			if len(v) == 1 {
				return strconv.FormatFloat(med, 'f', prec, 64)
			}
			return fmt.Sprintf("%s (%s–%s)", strconv.FormatFloat(med, 'f', prec, 64), strconv.FormatFloat(v[0], 'f', prec, 64), strconv.FormatFloat(v[len(v)-1], 'f', prec, 64))
		}
		dec := ok[0].Decision
		if len(ok) < len(byCase[c]) {
			dec += fmt.Sprintf(" (%d of %d failed)", len(byCase[c])-len(ok), len(byCase[c]))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", c, dec,
			col(func(r benchResult) float64 { return r.StartMs }, 0), col(func(r benchResult) float64 { return r.FirstMs }, 0),
			col(func(r benchResult) float64 { return r.Speed }, 1), col(func(r benchResult) float64 { return r.CPUPerM }, 1),
			col(func(r benchResult) float64 { return r.PeakMB }, 0), col(func(r benchResult) float64 { return r.SeekMs }, 0))
	}
	return b.String()
}
