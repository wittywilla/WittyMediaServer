// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/playback"
)

// A viewer's choice of picture size and data rate (playback.Choice), for every stream the
// server makes: files, discs, channels, catch-up. It comes from, in this order:
//
//   - the X-WMS-Quality header, or ?quality= for a player that can't set headers: what the
//     player's quality menu, or this device's own default, asks for;
//   - the profile's saved default, one for home and one for away from home (PUT /me/streaming);
//
// and is then held to what the profile is allowed (an administrator's limits for it) and,
// away from home, to the limits for viewers away from home. See docs/CLIENT-API.md, Quality.

const qualityHeader = "X-WMS-Quality"

// streamPrefs is a profile's saved defaults, as choice strings ("" = Auto).
type streamPrefs struct {
	Home string `json:"home"`
	Away string `json:"away"`
}

func streamPrefsKey(uid int64) string  { return "streamPrefs." + strconv.FormatInt(uid, 10) }
func streamLimitsKey(uid int64) string { return "streamLimits." + strconv.FormatInt(uid, 10) }

func (s *Server) userStreamPrefs(ctx context.Context, uid int64) streamPrefs {
	var p streamPrefs
	if raw := s.DB.GetSetting(ctx, streamPrefsKey(uid), ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &p)
	}
	// Written by this server, but read as if it weren't.
	p.Home, p.Away = playback.ParseChoice(p.Home).String(), playback.ParseChoice(p.Away).String()
	return p
}

// userStreamLimits is what an administrator allows a profile (none by default).
func (s *Server) userStreamLimits(ctx context.Context, uid int64) playback.Limits {
	var l playback.Limits
	if raw := s.DB.GetSetting(ctx, streamLimitsKey(uid), ""); raw != "" {
		_ = json.Unmarshal([]byte(raw), &l)
	}
	return cleanLimits(l)
}

func cleanLimits(l playback.Limits) playback.Limits {
	c := playback.Choice{MaxHeight: l.MaxHeight, MaxKbps: l.MaxKbps}.Clean()
	return playback.Limits{MaxHeight: c.MaxHeight, MaxKbps: c.MaxKbps}
}

// awayLimits are the limits for viewers away from home, for this request (none at home).
func (s *Server) awayLimits(r *http.Request) playback.Limits {
	h, rate := s.remoteCaps(r)
	return playback.Limits{MaxHeight: h, MaxKbps: int(ffmpeg.ParseRate(rate) / 1000)}
}

// viewerLimits are every limit on this request's streams: the profile's and, away from
// home, those for viewers away from home.
func (s *Server) viewerLimits(r *http.Request) []playback.Limits {
	out := []playback.Limits{s.awayLimits(r)}
	if u := userFrom(r); u != nil {
		out = append(out, s.userStreamLimits(r.Context(), u.ID))
	}
	return out
}

// viewerChoice is what this request asks for, before the limits.
func (s *Server) viewerChoice(r *http.Request) playback.Choice {
	c := playback.ParseChoice(r.Header.Get(qualityHeader))
	if !c.Set {
		c = playback.ParseChoice(r.URL.Query().Get("quality"))
	}
	if !c.Set {
		if u := userFrom(r); u != nil {
			p := s.userStreamPrefs(r.Context(), u.ID)
			if clientFrom(r).Where.AtHome() {
				c = playback.ParseChoice(p.Home)
			} else {
				c = playback.ParseChoice(p.Away)
			}
		}
	}
	return c
}

// streamChoice is the choice held to the limits: what streams are made to.
func (s *Server) streamChoice(r *http.Request) playback.Choice {
	return s.viewerChoice(r).Within(s.viewerLimits(r)...)
}

// streamTag names a choice in the IDs of shared streams (a channel, a disc), so viewers who
// chose differently get streams of their own and viewers who chose alike share one. "" for
// none, the stream everyone else gets.
func (s *Server) streamTag(r *http.Request) string {
	c := s.streamChoice(r)
	if !c.Set || (c.MaxHeight == 0 && c.MaxKbps == 0 && c.AudioKbps == 0) {
		return ""
	}
	return "-q" + c.String() // letters, digits and dashes, as stream IDs allow
}

// qualityView is what a player's quality menu needs: the sizes it may offer, what is
// chosen, and what it may not go above.
type qualityView struct {
	// Choice is what streams are made to now, in the form of the X-WMS-Quality header
	// ("" = Auto, "original" = no limits of the viewer's own).
	Choice string `json:"choice"`
	// Asked is what the viewer asked for, before the limits.
	Asked   string            `json:"asked"`
	Presets []playback.Preset `json:"presets"`
	// Limits are the highest the menu may offer (0 = none): the profile's and, away from
	// home, those for viewers away from home.
	Limits playback.Limits `json:"limits"`
	Away   bool            `json:"away"`
	// Source is the picture's own size and rate, when known.
	SourceWidth  int `json:"sourceWidth,omitempty"`
	SourceHeight int `json:"sourceHeight,omitempty"`
	SourceKbps   int `json:"sourceKbps,omitempty"`
	// VideoCodecs and AudioRates are what the advanced menu may offer.
	VideoCodecs []string `json:"videoCodecs"`
	AudioRates  []int    `json:"audioRates"`
}

func (s *Server) qualityFor(r *http.Request, width, height, kbps int) *qualityView {
	var lim playback.Limits
	for _, l := range s.viewerLimits(r) {
		lim.MaxHeight = playback.Smallest(lim.MaxHeight, l.MaxHeight)
		lim.MaxKbps = playback.Smallest(lim.MaxKbps, l.MaxKbps)
	}
	asked := s.viewerChoice(r)
	v := &qualityView{Choice: asked.Within(lim).String(), Asked: asked.String(), Limits: lim, Away: !clientFrom(r).Where.AtHome(),
		SourceWidth: width, SourceHeight: height, SourceKbps: kbps, VideoCodecs: playback.VideoCodecs, AudioRates: playback.AudioRates}
	for _, p := range playback.PresetsFor(width, height) {
		if lim.MaxHeight > 0 && p.Height > lim.MaxHeight {
			continue
		}
		if lim.MaxKbps > 0 && p.Kbps > lim.MaxKbps {
			p.Kbps = lim.MaxKbps
		}
		v.Presets = append(v.Presets, p)
	}
	if v.Presets == nil {
		v.Presets = []playback.Preset{}
	}
	return v
}

// GET /api/me/streaming: the profile's saved quality defaults, its limits, and whether this
// device is away from home now.
func (s *Server) handleGetStreaming(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	writeJSON(w, 200, map[string]any{
		"prefs":   s.userStreamPrefs(r.Context(), u.ID),
		"quality": s.qualityFor(r, 0, 0, 0),
		"profile": s.userStreamLimits(r.Context(), u.ID),
		"remote":  s.awayLimitsAlways(),
	})
}

// awayLimitsAlways are the limits for viewers away from home, wherever this request is from.
func (s *Server) awayLimitsAlways() playback.Limits {
	st := s.state()
	return playback.Limits{MaxHeight: st.cfg.MaxHeight, MaxKbps: int(ffmpeg.ParseRate(st.cfg.MaxBitrate) / 1000)}
}

// PUT /api/me/streaming {home, away}: the profile's own defaults. Each is a choice string;
// "" is Auto.
func (s *Server) handlePutStreaming(w http.ResponseWriter, r *http.Request) {
	var in streamPrefs
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	in.Home, in.Away = playback.ParseChoice(in.Home).String(), playback.ParseChoice(in.Away).String()
	b, _ := json.Marshal(in)
	if err := s.DB.SetSetting(r.Context(), streamPrefsKey(userFrom(r).ID), string(b)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, in)
}

// GET /api/users/{id}/stream-limits and PUT {maxHeight, maxKbps}: what an administrator allows
// a profile. 0 is no limit.
func (s *Server) handleGetStreamLimits(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.Auth.User(r.Context(), id); err != nil {
		writeErr(w, 404, "no such profile")
		return
	}
	writeJSON(w, 200, s.userStreamLimits(r.Context(), id))
}

func (s *Server) handlePutStreamLimits(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if _, err := s.Auth.User(r.Context(), id); err != nil {
		writeErr(w, 404, "no such profile")
		return
	}
	var in playback.Limits
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.MaxHeight < 0 || in.MaxKbps < 0 {
		writeErr(w, 400, "maxHeight and maxKbps can't be negative")
		return
	}
	in = cleanLimits(in)
	b, _ := json.Marshal(in)
	if err := s.DB.SetSetting(r.Context(), streamLimitsKey(id), string(b)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "stream limits changed", "cat", "profiles", "account", id, "maxHeight", in.MaxHeight, "maxKbps", in.MaxKbps)
	writeJSON(w, 200, in)
}

// liveURL is the address of a live stream for its tune answer: with the viewer's choice in
// it when the stream was made to that choice, as a player fetches the playlist and its
// segments without the header.
func (s *Server) liveURL(r *http.Request, ls *hls.LiveSession, path string) string {
	tag := s.streamTag(r)
	if tag == "" || ls == nil || !strings.HasSuffix(ls.Spec.ID, tag) {
		return path
	}
	return path + "?quality=" + url.QueryEscape(s.viewerChoice(r).String())
}

// liveLookup finds the live stream a request is for: the one made to the viewer's choice,
// or, for a tuner that couldn't open the channel twice, the one everyone shares.
func (s *Server) liveLookup(r *http.Request, kind, key string) *hls.LiveSession {
	tag := s.streamTag(r)
	if ls := s.HLS.GetLive(key + tag); ls != nil || tag == "" || kind != "tun" {
		return ls
	}
	return s.HLS.GetLive(key)
}

// liveSuffix is what a live playlist's segment addresses keep of its own: the token of a
// player that can't set headers (a TV's own player), and the viewer's choice.
func liveSuffix(r *http.Request) string {
	v := url.Values{}
	for _, k := range []string{"st", "token"} {
		if t := r.URL.Query().Get(k); t != "" {
			v.Set(k, t)
			break
		}
	}
	if c := playback.ParseChoice(r.URL.Query().Get("quality")); c.Set {
		v.Set("quality", c.String())
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// GET /api/hls/{id}/work: what is converting the stream now (hls.Work), for the player's stream
// information: the graphics chip or the processor, and which encoder.
func (s *Server) handleHLSWork(w http.ResponseWriter, r *http.Request) {
	sess := s.visibleStream(r, r.PathValue("id"))
	if sess == nil {
		writeErr(w, 404, "no such stream")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, sess.Work())
}

// GET /api/tv/{key}/live/work?quality=: the same for the live stream this viewer watches.
func (s *Server) handleLiveWork(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	kind, id, ok := parseKey(key)
	if !ok || !s.canWatchLive(r, kind, id) {
		writeErr(w, 404, "channel not found")
		return
	}
	ls := s.liveLookup(r, kind, key)
	if ls == nil {
		writeErr(w, 404, "no such stream")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, ls.Work())
}
