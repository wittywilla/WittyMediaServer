// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/playback"
)

// Streaming modes (see playback.ChooseStreamMode). remoteStreaming is the server's choice
// for viewers away from home; a device can choose for itself with streaming in /playback.

const remoteStreamingKey = "remoteStreaming"

func init() {
	settingKeys = append(settingKeys, remoteStreamingKey)
	settingDefaults[remoteStreamingKey] = playback.StreamStandard
	settingChecks[remoteStreamingKey] = oneOf(playback.StreamStandard, playback.StreamRobust)
	summarySettings = append(summarySettings, remoteStreamingKey)
}

// playbackStreaming says how a video is being streamed, and what the player should do
// for that, so that every app behaves the same way.
type playbackStreaming struct {
	// Mode: standard | robust.
	Mode string `json:"mode"`
	// From says who chose it: default | server (its setting, for viewers away from
	// home) | device (streaming in the request).
	From string `json:"from"`
	// SegmentSec is the length of a segment of an HLS stream; 0 for a file sent as it is.
	SegmentSec int `json:"segmentSec,omitempty"`
	// MeasuredKbps is the connection speed the server went by: the request's measuredKbps.
	MeasuredKbps int `json:"measuredKbps,omitempty"`
	// Ladder lists a robust stream's rungs in the order of its master playlist: the first
	// is where it starts, chosen from MeasuredKbps; the others are best first.
	Ladder []playback.Rung `json:"ladder,omitempty"`
	// Player is what the player should do in this mode.
	Player playback.PlayerHints `json:"player"`
}

// streamMode is the mode for this request, and who chose it.
func (s *Server) streamMode(r *http.Request, device string) (mode, from string) {
	return playback.ChooseStreamMode(s.DB.GetSetting(r.Context(), remoteStreamingKey, playback.StreamStandard), device, !clientFrom(r).Where.AtHome())
}

// robustCapKbps is the most a robust ladder may use: the limit for viewers away from
// home, and the device's own.
func (s *Server) robustCapKbps(r *http.Request, deviceKbps int) int {
	_, rate := s.remoteCaps(r)
	return playback.Smallest(int(ffmpeg.ParseRate(rate)/1000), deviceKbps)
}

// ladderSpecs makes one stream per rung from the spec of a converted stream. They share a
// group, and each rung's ID is the group's plus its height, so viewers whose connections
// measured differently still share the rungs they have in common.
func ladderSpecs(base hls.Spec, group string, rungs []playback.Rung, srcW, srcH int) []hls.Spec {
	out := make([]hls.Spec, 0, len(rungs))
	for _, rung := range rungs {
		sp := base
		sp.ID = rungID(group, rung.Height)
		sp.Group = group
		sp.SegmentSec = playback.RobustSegmentSec
		sp.MaxHeight = rung.Height
		sp.Quality = base.Quality.WithCap(strconv.Itoa(rung.Kbps) + "k")
		if rung.Kbps <= 800 {
			// At these rates the picture needs every bit: the sound takes less than WithCap
			// allows for it, which leaves the picture a little room under its limit.
			sp.Quality.AudioRate = "96k"
		}
		if base.Variant != nil {
			v := *base.Variant
			v.BandwidthKbps = rung.Kbps
			v.Height = rung.Height
			v.Width = 0
			if srcW > 0 && srcH > 0 {
				v.Width = (rung.Height*srcW/srcH + 1) / 2 * 2
			}
			sp.Variant = &v
		}
		out = append(out, sp)
	}
	return out
}

// trackedStream is the name a stream is counted under in the list of who is watching: a
// ladder's rungs are one stream there, whichever the player is on.
func (s *Server) trackedStream(id string) string {
	if id == "" || s.HLS == nil {
		return id
	}
	if sess := s.HLS.Get(id); sess != nil && sess.Spec.Group != "" {
		return sess.Spec.Group
	}
	return id
}

func rungID(group string, height int) string { return fmt.Sprintf("%s-q%d", group, height) }

// ladderParam names a ladder's rungs in its master playlist's address, in order.
func ladderParam(rungs []playback.Rung) string {
	hs := make([]string, len(rungs))
	for i, r := range rungs {
		hs[i] = strconv.Itoa(r.Height)
	}
	return strings.Join(hs, ",")
}

// ladderEntries reads a master playlist's ladder parameter back into the rungs of sess's
// group, skipping anything that isn't one.
func (s *Server) ladderEntries(r *http.Request, sess *hls.Session, param, st string) []hls.LadderEntry {
	var out []hls.LadderEntry
	seen := map[int]bool{}
	for _, f := range strings.Split(param, ",") {
		h, err := strconv.Atoi(f)
		if err != nil || h <= 0 || h > 4320 || seen[h] || len(out) == 8 {
			continue
		}
		seen[h] = true
		o := sess.Sibling(rungID(sess.Spec.Group, h))
		if o == nil || s.visibleStream(r, o.Spec.ID) == nil || o.Spec.Variant == nil {
			continue
		}
		out = append(out, hls.LadderEntry{Media: withStream("../"+o.Spec.ID+"/playlist.m3u8", st), Variant: *o.Spec.Variant})
	}
	return out
}

// GET /api/playback/speedtest?kb=n: n kilobytes (64 to 4096, 512 by default) that don't
// compress, for a player to time before it asks for a robust stream. It is sent within
// the limits for viewers away from home, like any stream, so it measures what a stream
// would get.
func (s *Server) handleSpeedTest(w http.ResponseWriter, r *http.Request) {
	kb := qInt(r, "kb", 512)
	if kb < 64 || kb > 4096 {
		writeErr(w, 400, "kb should be from 64 to 4096")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(kb<<10))
	out := s.capWriter(w, r)
	block := speedBlock()
	for left := kb << 10; left > 0; {
		n := min(left, len(block))
		if _, err := out.Write(block[:n]); err != nil {
			return
		}
		left -= n
	}
}

// speedBlock is made once: random, so no compression on the way makes the test look
// faster than a stream would be.
var speedBlock = sync.OnceValue(func() []byte {
	b := make([]byte, 64<<10)
	_, _ = rand.Read(b)
	return b
})
