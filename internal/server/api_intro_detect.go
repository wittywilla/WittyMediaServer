// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/introdetect"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Finding intros and credits nothing marks: the episodes of a season are compared by ear
// (internal/introdetect) and the stretch neighbours share is kept as a "detected" marker,
// trusted less than anything a person or the file itself says.
//
// It is work nobody waits for, and FFmpeg has to decode ten minutes of each episode's
// sound, so: an administrator starts it, for one show; one run at a time on the whole
// server; it is a task with progress that can be stopped; FFmpeg runs at low priority and
// the run gives way to people between episodes (internal/pace).

// introJob is the one run there can be.
type introJob struct {
	mu      sync.Mutex
	running bool
}

// introDetectPCM lets tests give the sound of an episode without FFmpeg.
var introDetectPCM func(ctx context.Context, s *Server, it *library.Item, tail bool) ([]byte, error)

// episodePCM decodes the first HeadSec seconds of an episode's first audio track, or its
// last TailSec, to mono 16-bit sound at introdetect.SampleRate.
func (s *Server) episodePCM(ctx context.Context, it *library.Item, tail bool) ([]byte, error) {
	if introDetectPCM != nil {
		return introDetectPCM(ctx, s, it, tail)
	}
	if s.Tools == nil || s.Tools.FFmpeg == "" {
		return nil, errors.New("FFmpeg isn't available")
	}
	args := []string{"-v", "error", "-nostdin", "-threads", "1"}
	if tail {
		args = append(args, "-sseof", "-"+strconv.Itoa(introdetect.TailSec))
	} else {
		args = append(args, "-t", strconv.Itoa(introdetect.HeadSec))
	}
	args = append(args, ffmpeg.FileInput(it.Path)...)
	args = append(args, "-map", "0:a:0", "-vn", "-sn", "-dn", "-ac", "1", "-ar", strconv.Itoa(introdetect.SampleRate), "-f", "s16le", "-")
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	out, _, err := ffmpeg.Run(cctx, s.Tools.FFmpeg, args, "for", "intro detection", "item", it.ID)
	if err != nil {
		return nil, err
	}
	// Never more than was asked for, whatever the file claims about itself.
	if limit := 2 * introdetect.SampleRate * (introdetect.HeadSec + 5); len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// introEpisodes are a show's episodes that are files here, by season, in episode order.
func (s *Server) introEpisodes(ctx context.Context, showID int64, season *int) (map[int][]*library.Item, error) {
	items, _, err := s.Store.Items(ctx, library.ItemQuery{ShowID: showID, NoTotal: true, Brief: true, Limit: 5000})
	if err != nil {
		return nil, err
	}
	out := map[int][]*library.Item{}
	for _, it := range items {
		if it.Kind != "video" || it.Season == nil || it.Episode == nil || !localFileItem(it) || (season != nil && *it.Season != *season) {
			continue
		}
		out[*it.Season] = append(out[*it.Season], it)
	}
	for k, eps := range out {
		sort.Slice(eps, func(i, j int) bool { return *eps[i].Episode < *eps[j].Episode })
		if len(eps) > introdetect.MaxEpisodes {
			eps = eps[:introdetect.MaxEpisodes]
		}
		out[k] = eps
		if len(eps) < 2 {
			delete(out, k) // one episode has nothing to be compared with
		}
	}
	return out, nil
}

// POST /api/shows/{id}/detect-intros {season?}: compare the episodes of a show's seasons
// (or one season) and keep what they share as intro and credits markers. Answers 202 with
// the task's id; GET /api/system/tasks follows it.
func (s *Server) handleDetectIntros(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	var b struct {
		Season *int `json:"season"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body: {} or {\"season\": 2}")
		return
	}
	if b.Season != nil && (*b.Season < 0 || *b.Season > 10000) {
		writeErr(w, 400, "season: a season number")
		return
	}
	sh, err := s.Store.Show(r.Context(), id, 0)
	if err != nil {
		writeErr(w, 404, "show not found")
		return
	}
	if introDetectPCM == nil && (s.Tools == nil || s.Tools.FFmpeg == "") {
		writeErr(w, 503, "FFmpeg isn't available, and it is what listens to the episodes")
		return
	}
	seasons, err := s.introEpisodes(r.Context(), id, b.Season)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	total := 0
	for _, eps := range seasons {
		total += len(eps)
	}
	if total == 0 {
		writeErr(w, 409, "There is nothing to compare: a season needs at least two episodes that are files in a library.")
		return
	}
	j := &s.introJob
	j.mu.Lock()
	if j.running {
		j.mu.Unlock()
		writeErr(w, 409, "Intros are already being looked for. Wait for that to finish, or stop it in Tasks.")
		return
	}
	j.running = true
	j.mu.Unlock()
	done := func() { j.mu.Lock(); j.running = false; j.mu.Unlock() }

	// Stopping the task ends stopped, and with it the run's own context below.
	stopped, stop := context.WithCancel(context.Background())
	task := s.Tasks.Start(tasks.Spec{Type: tasks.TypeIntro, Title: "Finding intros and credits", Unit: "episodes", By: byUser(userFrom(r)), Cancel: stop, AdminOnly: true})
	task.Item(sh.Title, "")
	task.Progress(0, int64(total))
	sid := logging.NewID("intro")
	log := s.Log.With("cat", "extras", "session", sid, "show", id)
	log.Info("looking for intros", "episodes", total, "seasons", len(seasons), "task", task.ID())
	began := time.Now()
	started := s.background("intro detection", func(bg context.Context) {
		defer done()
		defer stop()
		ctx, c := context.WithCancel(pace.Background(logging.WithSession(bg, sid)))
		defer c()
		defer context.AfterFunc(stopped, c)()
		found, err := s.detectIntros(ctx, seasons, task)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Error("looking for intros failed", "err", err)
			} else {
				log.Debug("looking for intros was stopped")
			}
			task.Finish(err)
			return
		}
		task.Result(fmt.Sprintf("Found an intro in %d of %d episodes, and credits in %d.", found.intros, total, found.credits))
		log.Info("intros looked for", "episodes", total, "intros", found.intros, "credits", found.credits, "took", time.Since(began).Round(time.Second).String())
		task.Finish(nil)
	}, "cat", "extras", "session", sid)
	if !started {
		done()
		stop()
		task.Finish(context.Canceled)
		writeErr(w, 503, "the server is stopping")
		return
	}
	writeJSON(w, 202, map[string]any{"ok": true, "task": task.ID(), "episodes": total, "seasons": len(seasons)})
}

type introCounts struct{ intros, credits int }

// detectIntros fingerprints each episode, compares each season, and keeps what was found.
// An episode FFmpeg can't read is a problem on the task, and the rest carry on.
func (s *Server) detectIntros(ctx context.Context, seasons map[int][]*library.Item, task *tasks.Handle) (introCounts, error) {
	var n introCounts
	log := s.jobLog(ctx)
	keys := make([]int, 0, len(seasons))
	for k := range seasons {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, season := range keys {
		items := seasons[season]
		task.Phase(fmt.Sprintf("Season %d", season))
		eps := make([]introdetect.Episode, 0, len(items))
		for _, it := range items {
			if err := pace.Yield(ctx); err != nil {
				return n, err
			}
			ep := introdetect.Episode{ID: it.ID}
			head, err := s.episodePCM(ctx, it, false)
			if ctx.Err() != nil {
				return n, ctx.Err()
			}
			if err != nil {
				log.Warn("an episode's sound couldn't be read", "item", it.ID, "title", it.Title, "err", err)
				task.Problem(it.Title, "its sound couldn't be read")
				task.Add(1)
				continue
			}
			ep.Head = introdetect.FromPCM(head)
			head = nil
			// Credits only where the episode is long enough that its end isn't its start.
			if it.RuntimeSec >= introdetect.HeadSec+introdetect.TailSec {
				if tail, err := s.episodePCM(ctx, it, true); err == nil {
					ep.Tail, ep.TailStart = introdetect.FromPCM(tail), it.RuntimeSec-introdetect.TailSec
				} else if ctx.Err() == nil {
					log.Debug("an episode's ending couldn't be read, so its credits aren't looked for", "item", it.ID, "err", err)
				}
				if ctx.Err() != nil {
					return n, ctx.Err()
				}
			}
			eps = append(eps, ep)
			task.Add(1)
		}
		found, err := introdetect.Season(ctx, eps, pace.Yield)
		log.Debug("season compared", "season", season, "episodes", len(eps), "found", len(found), "err", err)
		// What was found before a stop is still kept: it is right as far as it goes.
		for _, id := range introdetect.SortedIDs(found) {
			f := found[id]
			m := mediaextras.Markers{}
			if f.Intro != nil {
				m.Intro = &mediaextras.Range{StartSec: round1(f.Intro.Start), EndSec: round1(f.Intro.End)}
				n.intros++
			}
			if f.Credits != nil {
				m.Credits = &mediaextras.Range{StartSec: round1(f.Credits.Start), EndSec: round1(f.Credits.End)}
				n.credits++
			}
			if serr := s.Store.SaveDetectedMarkers(context.WithoutCancel(ctx), id, &m); serr != nil {
				return n, serr
			}
		}
		// An episode nothing was found for loses what an earlier run found: it may have
		// been replaced by another file since.
		for _, ep := range eps {
			if _, ok := found[ep.ID]; !ok && err == nil {
				if serr := s.Store.SaveDetectedMarkers(ctx, ep.ID, nil); serr != nil && ctx.Err() == nil {
					log.Warn("could not clear an episode's old intro markers", "item", ep.ID, "err", serr)
				}
			}
		}
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }
