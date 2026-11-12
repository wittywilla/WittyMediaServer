// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// Catch-up: a programme a tuner channel has shown, from its source's archive. It plays like
// start-over TV: from the programme's start, at the speed it aired, as a stream of its own
// that can be paused and rewound within what's kept. Everyone who starts the same programme
// shares one stream, as viewers of a channel do.

func (s *Server) tvCatchupRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/tv/{key}/catchup", a(s.handleCatchup))
	m.Handle("GET /api/tv/{key}/catchup/{start}/playlist.m3u8", a(s.handleCatchupPlaylist))
	m.Handle("GET /api/tv/{key}/catchup/{start}/seg/{name}", a(s.handleCatchupSegment))
}

// catchupID is a catch-up stream's session: the channel and when its programme started.
func catchupID(id, start int64) string { return fmt.Sprintf("tun-%d@%d", id, start) }

// catchupTarget reads which tuner channel and programme start a catch-up request is for,
// and whether this profile may watch it.
func (s *Server) catchupTarget(w http.ResponseWriter, r *http.Request, start string) (*livetv.Channel, int64, bool) {
	kind, id, ok := parseKey(r.PathValue("key"))
	if !ok || kind != "tun" || s.TV == nil || !s.canWatchTuner(r, id) {
		writeErr(w, 404, "channel not found")
		return nil, 0, false
	}
	c, err := s.TV.Channel(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "channel not found")
		return nil, 0, false
	}
	at, err := strconv.ParseInt(start, 10, 64)
	now := time.Now().Unix()
	if err != nil || c.CatchupDays <= 0 || at <= 0 || at > now || at < now-int64(c.CatchupDays)*86400 {
		writeErr(w, 404, "that isn't in this channel's archive")
		return nil, 0, false
	}
	return c, at, true
}

// GET /api/tv/{key}/catchup?start=&end=&viewer=: the programme from its start. The answer
// is shaped like /tune's.
func (s *Server) handleCatchup(w http.ResponseWriter, r *http.Request) {
	c, start, ok := s.catchupTarget(w, r, r.URL.Query().Get("start"))
	if !ok {
		return
	}
	if c.Encrypted != "" {
		s.tuneRefused(w, r, c.ID, &tuner.EncryptedError{Channel: c.Name, Why: c.Encrypted})
		return
	}
	end := int64(qInt(r, "end", 0))
	// What the guide says was on then, so the banner can say what this is.
	var shown *guideProgramme
	if list, err := s.TV.Guide(r.Context(), c, time.Unix(start, 0), time.Unix(start+1, 0)); err == nil {
		for _, p := range list {
			if p.Start <= start && p.End > start {
				g := broadcast(p)
				shown = &g
				if end == 0 || end > p.End {
					end = p.End
				}
				break
			}
		}
	}
	if end != 0 && (end <= start || end-start > 24*3600) {
		writeErr(w, 400, "a catch-up programme is up to a day long")
		return
	}
	ls, err := s.ensureCatchup(r, c.ID, start, end)
	if err != nil {
		if errors.Is(err, livetv.ErrNoArchive) {
			writeErr(w, 404, err.Error())
			return
		}
		s.tuneRefused(w, r, c.ID, err)
		return
	}
	ls.Viewer(viewerName(r))
	key := r.PathValue("key")
	logo, brand := tunerLogo(c)
	info := tuneInfo{Key: key, Kind: "tuner", Number: c.Number, Name: c.Name, Logo: logo, Brand: brand,
		URL: fmt.Sprintf("/api/tv/%s/catchup/%d/playlist.m3u8", key, start), Now: shown, ServerTime: time.Now().Unix(),
		BackgroundPlay: s.backgroundPlay(r.Context()), BufferSec: float64(s.tvSettingInt(r.Context(), "tvTunerBufferMin") * 60),
		Notices: s.channelNotices(c), Security: securityOf(c)}
	if err := s.addLiveTicket(r, &info); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, info)
}

// ensureCatchup starts, or finds, the stream of one programme from the archive. When FFmpeg
// has to start again part-way (a dropped connection), it asks the archive for where the
// stream had got to, and the stream ends with the programme.
func (s *Server) ensureCatchup(r *http.Request, id, start, end int64) (*hls.LiveSession, error) {
	sid := catchupID(id, start)
	if ls := s.HLS.GetLive(sid); ls != nil {
		ls.Touch()
		return ls, nil
	}
	q, h, preset := s.liveQuality(r)
	copyOK := s.tvSetting(r.Context(), "tvCopyVideo") == "1"
	minutes := s.tvSettingInt(r.Context(), "tvTunerBufferMin")
	session := logging.NewID("tv")
	octx, ocancel := context.WithCancel(logging.WithSession(context.Background(), session))
	first, err := s.TV.OpenArchive(octx, id, start, end)
	if err != nil {
		ocancel()
		return nil, err
	}
	began := time.Now()
	spec := hls.LiveSpec{
		ID: sid, LogSession: session, Quality: q, MaxHeight: h, Preset: preset, WindowSec: float64(minutes * 60),
		Feed: func(ctx context.Context, at time.Time) (*hls.LiveInput, error) {
			o := first
			first = nil
			if o == nil {
				from := start + int64(at.Sub(began).Seconds())
				if end != 0 && from >= end-5 {
					return nil, &hls.FatalError{Err: errors.New("the programme has finished")}
				}
				var err error
				if o, err = s.TV.OpenArchive(logging.WithSession(ctx, session), id, from, end); err != nil {
					return nil, err
				}
			}
			return s.tunerInput(ctx, o, copyOK), nil
		},
		OnStop: ocancel,
	}
	ls, err := s.HLS.EnsureLive(spec)
	if err != nil {
		if first != nil {
			first.Release()
		}
		ocancel()
		return nil, err
	}
	return ls, nil
}

func (s *Server) handleCatchupPlaylist(w http.ResponseWriter, r *http.Request) {
	c, start, ok := s.catchupTarget(w, r, r.PathValue("start"))
	if !ok {
		return
	}
	ls := s.HLS.GetLive(catchupID(c.ID, start))
	if ls == nil {
		var err error
		if ls, err = s.ensureCatchup(r, c.ID, start, 0); err != nil {
			s.tuneError(w, r, err)
			return
		}
	}
	ls.Viewer(viewerName(r))
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	if err := ls.WaitReady(ctx); err != nil {
		if ctx.Err() != nil && ls.Err() == nil {
			writeErr(w, 503, "the programme is still starting")
			return
		}
		s.tuneError(w, r, err)
		return
	}
	if err := ls.Err(); err != nil {
		s.tuneError(w, r, err)
		return
	}
	suffix := ""
	for _, k := range []string{"st", "token"} {
		if v := r.URL.Query().Get(k); v != "" {
			suffix = "?" + k + "=" + url.QueryEscape(v)
			break
		}
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(ls.Playlist(func(f string) string { return "seg/" + f + suffix }))
}

func (s *Server) handleCatchupSegment(w http.ResponseWriter, r *http.Request) {
	c, start, ok := s.catchupTarget(w, r, r.PathValue("start"))
	if !ok {
		return
	}
	ls := s.HLS.GetLive(catchupID(c.ID, start))
	if ls == nil {
		writeErr(w, 404, "no such stream")
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
