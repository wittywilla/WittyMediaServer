// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Transcode is how long a film took from asking to play it (forced to transcode, as a
// device that can't take the file would) to the first segment's last byte.
type Transcode struct {
	Item  int64
	Ms    []float64
	Error string `json:",omitempty"`
}

// transcodeStart times a forced transcode of the first film from the playback request to
// the first segment, n times, stopping the stream between.
func (s *server) transcodeStart(n int) Transcode {
	var t Transcode
	_, b := s.call("GET", "/api/libraries", nil)
	var ls []struct {
		ID   int64
		Type string
	}
	_ = json.Unmarshal(b, &ls)
	for _, l := range ls {
		if l.Type != "movies" {
			continue
		}
		_, b := s.call("GET", fmt.Sprintf("/api/items?library=%d&limit=1", l.ID), nil)
		var it struct{ Items []struct{ ID int64 } }
		_ = json.Unmarshal(b, &it)
		if len(it.Items) > 0 {
			t.Item = it.Items[0].ID
			break
		}
	}
	if t.Item == 0 {
		t.Error = "no film to play"
		return t
	}
	for i := 0; i < n; i++ {
		t0 := time.Now()
		if err := s.firstSegment(t.Item); err != nil {
			t.Error = err.Error()
			return t
		}
		t.Ms = append(t.Ms, ms(time.Since(t0)))
	}
	return t
}

// firstSegment asks to play an item, forced to transcode, and follows the playlists to the
// first segment. The stream is stopped afterwards.
func (s *server) firstSegment(id int64) error {
	code, b := s.call("POST", fmt.Sprintf("/api/items/%d/playback", id), map[string]any{"profile": "web", "forceTranscode": true, "startSec": 0})
	if code != 200 {
		return fmt.Errorf("playback: %d %s", code, cut(string(b), 200))
	}
	var p struct{ Protocol, URL string }
	_ = json.Unmarshal(b, &p)
	if p.Protocol != "hls" || p.URL == "" {
		return fmt.Errorf("playback answered %q, not hls", p.Protocol)
	}
	base, _ := url.Parse(s.base + "/")
	at := base.ResolveReference(mustURL(p.URL))
	defer func() {
		// The stream's id is the path element after /api/hls/.
		if parts := strings.Split(at.Path, "/"); len(parts) > 3 && parts[2] == "hls" {
			s.call("DELETE", "/api/hls/"+parts[3]+"?"+at.RawQuery, nil)
		}
	}()
	// master, then the first variant, then the first segment; each may answer 503 while
	// FFmpeg starts.
	for hop := 0; hop < 3; hop++ {
		body, err := s.getRetry(at.String())
		if err != nil {
			return err
		}
		if !bytes.HasPrefix(bytes.TrimSpace(body), []byte("#EXTM3U")) {
			return nil // a segment
		}
		next := ""
		for _, ln := range strings.Split(string(body), "\n") {
			if ln = strings.TrimSpace(ln); ln != "" && !strings.HasPrefix(ln, "#") {
				next = ln
				break
			}
		}
		if next == "" {
			return fmt.Errorf("a playlist with nothing in it: %s", cut(string(body), 200))
		}
		at = at.ResolveReference(mustURL(next))
	}
	return errors.New("no segment after three playlists")
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	check(err)
	return u
}

func (s *server) getRetry(u string) ([]byte, error) {
	for try := 0; ; try++ {
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("Authorization", "Bearer "+s.token)
		r, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode == 200 {
			return b, nil
		}
		if r.StatusCode != 503 || try > 240 {
			return nil, fmt.Errorf("%s: %d %s", u, r.StatusCode, cut(string(b), 200))
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// Budget is the most a run may use (scripts/perf/budget.json). Memory is what an idle
// server holds after scanning the test library; a scan's floor is items a second, which a
// slower machine lowers, so it is set well under what CI's runners reach.
type Budget struct {
	IdleRSSMiB        float64            `json:"idleRssMiB"`
	IdleHeapLiveMiB   float64            `json:"idleHeapLiveMiB"`
	ScanPeakRSSMiB    float64            `json:"scanPeakRssMiB"`
	ScanItemsPerSec   map[string]float64 `json:"scanItemsPerSecond"`
	RouteAllocKiB     map[string]float64 `json:"routeAllocKiB"`
	RouteStatements   map[string]float64 `json:"routeStatements"`
	TranscodeStartSec float64            `json:"transcodeStartSeconds"`
}

func checkBudget(r *Results, file string) []string {
	b, err := os.ReadFile(file)
	check(err)
	var bu Budget
	check(json.Unmarshal(b, &bu))
	var over []string
	idle := r.IdleAfter
	if bu.IdleRSSMiB > 0 && idle.RSSMiB > bu.IdleRSSMiB {
		over = append(over, fmt.Sprintf("idle memory after the scans is %.1f MiB (RSS), over the budget of %.0f MiB: see what a scan leaves behind with go tool pprof on heap-after.pprof in the -out folder", idle.RSSMiB, bu.IdleRSSMiB))
	}
	if bu.IdleHeapLiveMiB > 0 && idle.HeapLiveMiB > bu.IdleHeapLiveMiB {
		over = append(over, fmt.Sprintf("the Go heap holds %.1f MiB live while idle after the scans, over the budget of %.0f MiB: something is kept after a scan (a cache without a limit?)", idle.HeapLiveMiB, bu.IdleHeapLiveMiB))
	}
	for _, sc := range r.Scans {
		if sc.Error != "" {
			over = append(over, fmt.Sprintf("the %s library wasn't scanned: %s", sc.Type, sc.Error))
			continue
		}
		if bu.ScanPeakRSSMiB > 0 && sc.PeakRSSMiB > bu.ScanPeakRSSMiB {
			over = append(over, fmt.Sprintf("scanning %s took the server to %.0f MiB, over the budget of %.0f MiB", sc.Type, sc.PeakRSSMiB, bu.ScanPeakRSSMiB))
		}
		if floor := bu.ScanItemsPerSec[sc.Type]; floor > 0 && sc.Seconds > 0 {
			if rate := float64(sc.Items) / sc.Seconds; rate < floor {
				over = append(over, fmt.Sprintf("the %s scan found %d items in %.1f s: %.1f a second, under the floor of %.0f a second", sc.Type, sc.Items, sc.Seconds, rate, floor))
			}
		}
	}
	for _, rt := range r.Routes {
		key := routeKey(rt.Path)
		lim, ok := bu.RouteAllocKiB[key]
		if ok && rt.Status != 200 {
			over = append(over, fmt.Sprintf("%s answered %d, not 200", rt.Path, rt.Status))
		}
		if ok && rt.AllocKiB > lim {
			over = append(over, fmt.Sprintf("%s allocates %.0f KiB a request, over the budget of %.0f KiB", rt.Path, rt.AllocKiB, lim))
		}
		if lim, ok := bu.RouteStatements[key]; ok && rt.Statements > lim {
			over = append(over, fmt.Sprintf("%s runs %.1f database statements a request, over the budget of %.0f", rt.Path, rt.Statements, lim))
		}
	}
	if bu.TranscodeStartSec > 0 {
		if r.Transcode.Error != "" {
			over = append(over, "a transcode didn't start: "+r.Transcode.Error)
		} else if len(r.Transcode.Ms) > 0 {
			m := slices.Clone(r.Transcode.Ms)
			sort.Float64s(m)
			if med := m[len(m)/2] / 1000; med > bu.TranscodeStartSec {
				over = append(over, fmt.Sprintf("a transcode's first segment took %.1f s (median), over the budget of %.0f s", med, bu.TranscodeStartSec))
			}
		}
	}
	return over
}

// routeKey is a route's path with library and item numbers taken out, so that a budget
// names /api/items/{id} whatever the item.
func routeKey(p string) string {
	path, q, _ := strings.Cut(p, "?")
	parts := strings.Split(path, "/")
	for i, x := range parts {
		if _, err := strconv.Atoi(x); err == nil && x != "" {
			parts[i] = "{id}"
		}
	}
	path = strings.Join(parts, "/")
	if q == "" {
		return path
	}
	vals, _ := url.ParseQuery(q)
	var keys []string
	for k := range vals {
		if k == "library" {
			keys = append(keys, k+"={id}")
		} else {
			keys = append(keys, k+"="+vals.Get(k))
		}
	}
	sort.Strings(keys)
	return path + "?" + strings.Join(keys, "&")
}
