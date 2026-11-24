// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/pace"
)

// lookahead reads the details of the next new films (ffprobe) while the scan is still on an
// earlier one, so that one film's FFmpeg work (its probe) runs beside another's (its thumbnail
// and its rows) instead of after it. It changes nothing the scan decides: each file is still
// indexed in turn, and takes the details read for it, or reads them itself when none were.
// Only details every new film needs are read ahead; a thumbnail is a last resort after the
// online lookups, so making it early could be wasted work (#643).
//
// On two processors a scan of new films is about a third quicker; one processor gets no
// lookahead at all.
type lookahead struct {
	s     *Scanner
	ctx   context.Context
	width int // how many are read ahead at once
	next  int // the first candidate not yet looked at
	mu    sync.Mutex
	jobs  map[string]*aheadJob
	busy  chan struct{}
}

type aheadJob struct {
	done chan struct{}
	p    *ffmpeg.Probe
	err  error
}

type aheadKey struct{}

// probeTimeout is how long ffprobe may take over one file.
const probeTimeout = 60 * time.Second

// aheadWidth: how many films may be read ahead, beside the one being indexed. A processor
// each, at most three, and never more than the machine's probes at once allow.
func aheadWidth() int { return min(pace.Slots(pace.Probes), pace.Cores(), 4) - 1 }

// newLookahead is the lookahead for a library's scan, nil where nothing is read ahead: a
// library whose files aren't films taken one by one (shows and anime, which group into
// series; music, books, photos and creators' videos), or a single-processor machine.
func (s *Scanner) newLookahead(ctx context.Context, lib *Library) *lookahead {
	switch {
	case lib.Type == "shows", lib.Type == "anime", IsAudioLibrary(lib.Type), lib.Type == "books", lib.Type == "photos", IsCreatorLibrary(lib.Type):
		return nil
	}
	w := aheadWidth()
	if w < 1 || s.Tools == nil {
		return nil
	}
	return &lookahead{s: s, ctx: ctx, width: w, jobs: map[string]*aheadJob{}, busy: make(chan struct{}, w)}
}

// feed starts reading ahead for the new films after all[i], while fewer than width are under way.
func (l *lookahead) feed(all []candidate, i int) {
	if l == nil {
		return
	}
	l.next = max(l.next, i+1)
	for l.next < len(all) {
		c := &all[l.next]
		if c.kind != "video" || c.unchanged() != nil {
			l.next++ // a disc, or a file the library already has as it is: nothing to read
			continue
		}
		select {
		case l.busy <- struct{}{}:
		default:
			return // all under way; the next call starts more
		}
		j := &aheadJob{done: make(chan struct{})}
		l.mu.Lock()
		l.jobs[c.path] = j
		l.mu.Unlock()
		l.next++
		go func(path string) {
			defer func() { <-l.busy; close(j.done) }()
			// A scan gives way to someone watching (pace.Yield), and so does what it reads ahead.
			if j.err = pace.Yield(l.ctx); j.err != nil {
				return
			}
			pctx, cancel := context.WithTimeout(l.ctx, probeTimeout)
			defer cancel()
			j.p, j.err = l.s.Tools.ProbeFile(pctx, path)
		}(c.path)
	}
}

// take is what was read ahead for path, waiting for it if it's under way; ok is false when
// nothing was (and the caller reads the file itself).
func (l *lookahead) take(ctx context.Context, path string) (p *ffmpeg.Probe, err error, ok bool) {
	if l == nil {
		return nil, nil, false
	}
	l.mu.Lock()
	j := l.jobs[path]
	delete(l.jobs, path)
	l.mu.Unlock()
	if j == nil {
		return nil, nil, false
	}
	select {
	case <-j.done:
	case <-ctx.Done():
		return nil, ctx.Err(), true
	}
	if j.err != nil && ctx.Err() == nil && (errors.Is(j.err, context.DeadlineExceeded) || errors.Is(j.err, context.Canceled)) {
		// Ran out of time ahead (the machine was busy, or it waited for someone watching):
		// read it now, as the scan would have, with the time that gives it.
		return nil, nil, false
	}
	return j.p, j.err, true
}

// probeFile is ProbeFile, taking what the scan's lookahead read when it has it.
func (s *Scanner) probeFile(ctx context.Context, path string) (*ffmpeg.Probe, error) {
	if l, _ := ctx.Value(aheadKey{}).(*lookahead); l != nil {
		if p, err, ok := l.take(ctx, path); ok {
			return p, err
		}
	}
	return s.Tools.ProbeFile(ctx, path)
}
