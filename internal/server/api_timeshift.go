// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/channels"
	"github.com/wittywilla/wittymediaserver/internal/hls"
)

// Time-shift for library channels (#648). Start over plays the programme on now from its
// beginning for one viewer, while the shared channel runs on live for everyone else. The
// viewer's stream is the channel's own schedule, played a while behind the clock (a shifted
// live session), so it costs one FFmpeg like any channel and keeps only the usual small
// window on disk. Rewinding further back than the window, or skipping a break, starts the
// viewer's stream again from that moment. Its program date-times carry the schedule's time,
// so a player knows where in the programme it is from the stream itself.
//
// Tuner channels pause and rewind within their rolling buffer (tvTunerBufferMin); their
// start over is the source's catch-up archive, where it has one. Nothing is decrypted.

func (s *Server) timeshiftRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/tv/{key}/startover", a(s.handleStartOver))
	m.Handle("GET /api/tv/{key}/timeline", a(s.handleTimeline))
	m.Handle("GET /api/tv/{key}/shift/{viewer}/playlist.m3u8", a(s.handleShiftPlaylist))
	m.Handle("GET /api/tv/{key}/shift/{viewer}/seg/{name}", a(s.handleShiftSegment))
}

// liveEdgeSec: a start-over closer to now than this is live; the player goes back to the
// shared stream instead of making one of its own.
const liveEdgeSec = 20

// span is a stretch of a channel's schedule, in Unix seconds.
type span struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// timeline is what a time-shifting player needs to know about the programme it is in: where
// it starts and ends, where its breaks are, and where live is now. Times are the schedule's.
type timeline struct {
	Programme guideProgramme `json:"programme"`
	// Breaks are the runs of bumpers and commercials inside the programme and after it, up to
	// the next one: what "Skip break" skips.
	Breaks []span `json:"breaks"`
	// From is the moment of the schedule the stream starts at (start over: Programme.Start).
	From int64 `json:"from,omitempty"`
	// Live is the moment on the air now, and Earliest the furthest back start over goes.
	Live     int64 `json:"live"`
	Earliest int64 `json:"earliest"`
}

// startOverBack is how far back a viewer may start a library channel from.
const startOverBack = 6 * time.Hour

// timelineAt finds the programme on a library channel at t as the guide tells it, and its
// breaks: the runs of anything but the programme itself between its start and the next.
func (s *Server) timelineAt(ctx context.Context, c *channels.Channel, t time.Time) (*timeline, error) {
	cur, _, err := s.Channels.GuideNow(ctx, c, t)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, channels.ErrNotFound
	}
	list, err := s.Channels.Window(ctx, c, time.Unix(cur.Start, 0), time.Unix(cur.End, 0).Add(3*time.Hour))
	if err != nil {
		return nil, err
	}
	tl := &timeline{Programme: fromSchedule(*cur), Breaks: breaksIn(list, cur)}
	return tl, nil
}

// breaksIn lists the breaks of a programme: every run of fillers and slates from its start up
// to the next programme of its own (its pre-roll bumpers included, as those lead into it).
func breaksIn(list []channels.Programme, cur *channels.Programme) []span {
	out := []span{}
	var open *span
	for i := range list {
		p := &list[i]
		if p.End <= cur.Start {
			continue
		}
		if p.Kind == channels.KindShow {
			if p.Start >= cur.End && !(cur.ItemID != 0 && p.ItemID == cur.ItemID && p.Offset > 0) {
				break // the next programme
			}
			if open != nil {
				out = append(out, *open)
				open = nil
			}
			continue
		}
		if p.Kind == channels.KindFiller && p.Role == channels.RolePre && p.Start >= cur.End {
			break // leads into the next programme
		}
		if open == nil {
			open = &span{Start: max(p.Start, cur.Start), End: p.End}
		} else {
			open.End = p.End
		}
	}
	if open != nil {
		out = append(out, *open)
	}
	return out
}

// shiftID names one viewer's start-over stream of a channel: their account, the name their
// player gave itself, and their quality choice. Letters, digits and dashes.
func (s *Server) shiftID(r *http.Request, id int64, viewer string) string {
	var uid int64
	if u := userFrom(r); u != nil {
		uid = u.ID
	}
	return s.shiftIDFor(id, uid, viewer) + s.streamTag(r)
}

func (s *Server) shiftIDFor(id, uid int64, viewer string) string {
	h := sha256.Sum256([]byte(viewer))
	return fmt.Sprintf("lib-%d-so-u%d-%s", id, uid, hex.EncodeToString(h[:6]))
}

// shiftViewer is the viewer named in a shift stream's address: what the player calls itself.
func shiftViewer(r *http.Request) (string, bool) {
	v := r.PathValue("viewer")
	if v == "" || len(v) > 40 {
		return "", false
	}
	return v, true
}

// libraryTarget is the library channel a request names, when this profile may watch it.
func (s *Server) libraryTarget(w http.ResponseWriter, r *http.Request) (*channels.Channel, bool) {
	kind, id, ok := parseKey(r.PathValue("key"))
	if !ok || kind != "lib" {
		writeErr(w, 404, "channel not found")
		return nil, false
	}
	c, err := s.visibleChannel(r, id)
	if err != nil {
		writeErr(w, 404, "channel not found")
		return nil, false
	}
	return c, true
}

// startOverOn: whether the owner lets viewers start library channels over.
func (s *Server) startOverOn(ctx context.Context) bool { return s.tvSetting(ctx, "tvStartOver") != "0" }

// GET /api/tv/{key}/timeline?at=: the programme on at a moment, its breaks, and where live is.
// A player asks when its shifted stream moves on to the next programme. Starts nothing.
func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	c, ok := s.libraryTarget(w, r)
	if !ok {
		return
	}
	now := time.Now()
	at := now
	if v, err := strconv.ParseInt(r.URL.Query().Get("at"), 10, 64); err == nil && v > 0 && v <= now.Unix() {
		at = time.Unix(v, 0)
	}
	tl, err := s.timelineAt(r.Context(), c, at)
	if err != nil {
		writeErr(w, 404, "nothing is on then")
		return
	}
	tl.Live, tl.Earliest = now.Unix(), now.Add(-startOverBack).Unix()
	writeJSON(w, 200, tl)
}

// GET /api/tv/{key}/startover?viewer=&from=: the channel from a moment, for this viewer only.
// from is a moment of the schedule (Unix seconds); none means the start of the programme on
// now. A tuner channel starts over from its catch-up archive. The answer is shaped like
// /tune's, with the timeline in it; 409 when from is live (the player stays on the shared
// stream).
func (s *Server) handleStartOver(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := parseKey(r.PathValue("key"))
	if ok && kind == "tun" {
		s.startOverTuner(w, r, id)
		return
	}
	c, ok := s.libraryTarget(w, r)
	if !ok {
		return
	}
	if !s.startOverOn(r.Context()) {
		writeErr(w, 403, "start over is turned off on this server")
		return
	}
	viewer := viewerName(r)
	if viewer == "" {
		writeErr(w, 400, "the player must name itself (viewer=)")
		return
	}
	now := time.Now()
	from := now.Unix()
	if v := r.URL.Query().Get("from"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			writeErr(w, 400, "bad from")
			return
		}
		from = n
	} else if tl, err := s.timelineAt(r.Context(), c, now); err == nil {
		from = tl.Programme.Start
	}
	if from > now.Unix()-liveEdgeSec {
		writeErr(w, 409, "that is live now")
		return
	}
	if from < now.Add(-startOverBack).Unix() {
		from = now.Add(-startOverBack).Unix()
	}
	tl, err := s.timelineAt(r.Context(), c, time.Unix(from, 0))
	if err != nil {
		writeErr(w, 404, "nothing was on then")
		return
	}
	cur, _, err := s.Channels.Now(r.Context(), c, time.Unix(from, 0))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// The profile's live TV limits, for what was on at from (parental_live.go).
	if s.liveRefused(w, r, "lib", c.ID, time.Unix(from, 0)) {
		return
	}
	sid := s.shiftID(r, c.ID, viewer)
	// A new moment is a new stream: the old one of this viewer stops first.
	s.HLS.StopLive(sid)
	ls, err := s.ensureShift(r, c, sid, now.Sub(time.Unix(from, 0)))
	if err != nil {
		liveError(w, err)
		return
	}
	ls.Viewer(viewer)
	key := r.PathValue("key")
	tl.From, tl.Live, tl.Earliest = from, now.Unix(), now.Add(-startOverBack).Unix()
	info := tuneInfo{Key: key, Kind: "library", Number: c.Number, Name: c.Name, Description: c.Description, Accent: c.Look.Accent, Brand: brandOf(c),
		URL:         s.liveURL(r, ls, "/api/tv/"+key+"/shift/"+url.PathEscape(viewer)+"/playlist.m3u8"),
		PositionSec: float64(from - cur.Start + cur.Offset), ServerTime: now.Unix(), CanRecord: false,
		BackgroundPlay: s.backgroundPlay(r.Context()), BufferSec: float64(s.tvSettingInt(r.Context(), "tvLibraryBufferMin") * 60),
		Now: &tl.Programme, Timeline: tl, StartOver: true}
	if c.Logo != "" {
		info.Logo = fmt.Sprintf("/api/channels/%d/logo?v=%s", c.ID, c.Logo)
	}
	if err := s.addLiveTicket(r, &info); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, info)
}

// startOverTuner sends a tuner channel's start over to its catch-up archive: the programme on
// now, from its start.
func (s *Server) startOverTuner(w http.ResponseWriter, r *http.Request, id int64) {
	if s.TV == nil || !s.canWatchTuner(r, id) {
		writeErr(w, 404, "channel not found")
		return
	}
	c, err := s.TV.Channel(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "channel not found")
		return
	}
	if c.CatchupDays <= 0 {
		writeErr(w, 404, "this channel has no archive to start over from; pause and rewind work within the buffer")
		return
	}
	now := time.Now()
	list, err := s.TV.Guide(r.Context(), c, now, now.Add(time.Second))
	if err != nil {
		writeErr(w, 404, "the guide doesn't say what is on")
		return
	}
	for _, p := range list {
		if p.Start <= now.Unix() && p.End > now.Unix() {
			q := r.URL.Query()
			q.Set("start", strconv.FormatInt(p.Start, 10))
			q.Set("end", strconv.FormatInt(p.End, 10))
			r.URL.RawQuery = q.Encode()
			s.handleCatchup(w, r)
			return
		}
	}
	writeErr(w, 404, "the guide doesn't say what is on")
}

// ensureShift starts a viewer's stream of a library channel, shift behind the clock.
func (s *Server) ensureShift(r *http.Request, c *channels.Channel, sid string, shift time.Duration) (*hls.LiveSession, error) {
	q, h, preset := s.liveQuality(r)
	copyOK := s.tvSetting(r.Context(), "tvCopyVideo") == "1"
	minutes := s.tvSettingInt(r.Context(), "tvLibraryBufferMin")
	return s.HLS.EnsureLive(hls.LiveSpec{
		ID: sid, Quality: q, MaxHeight: h, Preset: preset, WindowSec: float64(minutes * 60), Shift: shift,
		Feed: s.shiftedFeed(c.ID, q, h, copyOK, shift),
	})
}

// shiftTarget is the viewer's start-over stream a playlist or segment request is for.
func (s *Server) shiftTarget(w http.ResponseWriter, r *http.Request) *hls.LiveSession {
	c, ok := s.libraryTarget(w, r)
	if !ok {
		return nil
	}
	viewer, ok := shiftViewer(r)
	if !ok {
		writeErr(w, 404, "no such stream")
		return nil
	}
	ls := s.HLS.GetLive(s.shiftID(r, c.ID, viewer))
	if ls == nil {
		// It stopped (left too long, or the server restarted): the player starts over again
		// from where it was, which it knows from the program date-times.
		writeErr(w, 410, "this start-over stream has ended")
		return nil
	}
	// Every playlist and part, as on the shared stream, but for the moment of the schedule this
	// stream is at: a shifted stream stops when it reaches a programme the profile may not watch.
	if s.liveRefused(w, r, "lib", c.ID, time.Now().Add(-ls.Spec.Shift)) {
		return nil
	}
	return ls
}

func (s *Server) handleShiftPlaylist(w http.ResponseWriter, r *http.Request) {
	ls := s.shiftTarget(w, r)
	if ls == nil {
		return
	}
	viewer, _ := shiftViewer(r)
	ls.Viewer(viewer)
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	if err := ls.WaitReady(ctx); err != nil {
		if ctx.Err() != nil && ls.Err() == nil {
			writeErr(w, 503, "the programme is still starting")
			return
		}
		liveError(w, err)
		return
	}
	if err := ls.Err(); err != nil {
		liveError(w, err)
		return
	}
	suffix := liveSuffix(r)
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(ls.Playlist(func(f string) string { return "seg/" + f + suffix }))
}

func (s *Server) handleShiftSegment(w http.ResponseWriter, r *http.Request) {
	ls := s.shiftTarget(w, r)
	if ls == nil {
		return
	}
	p, ok := ls.SegmentPath(r.PathValue("name"))
	if !ok {
		writeErr(w, 404, "that part of the stream has gone")
		return
	}
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, p)
}

// leaveShift stops a viewer's start-over stream when its player goes back to live or away:
// it is nobody else's.
func (s *Server) leaveShift(r *http.Request, key, viewer string) bool {
	kind, id, ok := parseKey(key)
	if !ok || kind != "lib" || viewer == "" {
		return false
	}
	s.HLS.StopLive(s.shiftID(r, id, viewer))
	return true
}
