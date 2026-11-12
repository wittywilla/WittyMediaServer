// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package dvdremote

import (
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/disckey"
	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymodule"
)

// analyzeTimeout is how long one call of an analysis may take. Ranking reads the index
// files and the menus' buttons, never the video, and takes a few seconds on a slow disk
// for the worst disc measured; a helper quiet for longer than this is stopped, and that
// counts against its version like any hang.
const analyzeTimeout = 2 * time.Minute

// analyses remembers the latest answers, by disc and module version. An answer is a few
// kilobytes for an ordinary disc and at most a few hundred for one of 99 titles.
var analyses = disckey.Cache[*dvdsession.Analysis]{Max: 24}

// analysing lets one analysis run at a time: each is a process reading a disc, and a
// library scan asks for one disc after another anyway.
var analysing sync.Mutex

// Analyze has the WittyDVD in use rank a disc's titles, in a helper of its own that ends
// with the answer. It is what dvdsession.SetAnalyzer takes.
func (o Opener) Analyze(path string) (*dvdsession.Analysis, error) {
	a := o.M.Attach(moduleID, analyzeTimeout)
	if a == nil {
		return nil, o.M.Missing(moduleID)
	}
	version := a.Version()
	key, keyed := disckey.Of(path, version)
	if keyed {
		if got, ok := analyses.Get(key); ok {
			a.Close() // nothing was started
			return got, nil
		}
	}
	analysing.Lock()
	defer analysing.Unlock()
	if keyed {
		if got, ok := analyses.Get(key); ok {
			a.Close()
			return got, nil
		}
	}
	report := func(reason string) { _ = o.M.ReportFailure(moduleID, version, reason) }
	got, err := analyze(a, report, o.Log, path)
	if err != nil {
		return nil, err
	}
	got.Engine = version
	if keyed {
		analyses.Put(key, got)
	}
	return got, nil
}

// analyze asks one helper, and closes it. The disc's structure is read for itself here
// even when the video is encrypted: index files never are, a helper reading them decrypts
// nothing, and nothing of such a disc is played whatever the analysis says. The server's
// own finding goes with the answer.
func analyze(h helper, report reporter, log *slog.Logger, path string) (*dvdsession.Analysis, error) {
	defer h.Close()
	if log == nil {
		log = slog.Default()
	}
	path = helperPath(path)
	out := &dvdsession.Analysis{Protected: rawScrambled(path)}
	var opened dvd.Opened
	blob, err := h.Call(dvd.MethodOpen, dvd.OpenParams{Path: path}, &opened)
	if err != nil {
		log.Warn("dvd: the disc couldn't be opened for its title ranking", "cat", "discs", "path", path, "err", err)
		return nil, err
	}
	d, err := dvd.DecodeDisc(blob)
	if err == nil {
		err = checkDisc(d)
	}
	if err != nil {
		report(err.Error())
		log.Warn("dvd: the disc's structure was refused by the server", "cat", "discs", "path", path, "err", err)
		return nil, err
	}
	out.Repairs = d.Repairs
	blob, err = h.Call(dvd.MethodAnalyze, nil, nil)
	var ce *wittymodule.CallError
	switch {
	case errors.As(err, &ce) && ce.Code == dvd.CodeUnknownCall:
		// A WittyDVD from before the ranking: a working helper, and the older rule.
		return out, nil
	case errors.As(err, &ce):
		// It answered no about this disc. That is no fault of the helper's.
		log.Info("dvd: the title ranking was refused", "cat", "discs", "path", path, "err", err)
		return out, nil
	case err != nil:
		// It hung or died, which Attached.Call has counted. The older rule still works.
		log.Warn("dvd: the title ranking failed", "cat", "discs", "path", path, "err", err)
		return nil, err
	}
	r, err := dvd.DecodeRanking(blob, d)
	if err != nil {
		report("it sent a bad answer: " + err.Error())
		log.Warn("dvd: the title ranking was refused by the server", "cat", "discs", "path", path, "err", err)
		return nil, &badAnswer{err}
	}
	out.Ranking = r
	// What was padded while ranking. Not being told is no reason to lose the ranking.
	if blob, err := h.Call(dvd.MethodReadStats, nil, nil); err == nil {
		if st, err := dvd.DecodeReadStats(blob); err == nil {
			out.Stats = st
		} else {
			log.Debug("dvd: the read statistics were unreadable", "cat", "discs", "path", path, "err", err)
		}
	} else {
		log.Debug("dvd: no read statistics", "cat", "discs", "path", path, "err", err)
	}
	log.Debug("dvd: titles ranked", "cat", "discs", "path", path, "main", r.Main, "confidence", r.Confidence, "level", r.Level,
		"candidates", len(r.Candidates), "decisions", len(r.Decisions), "warnings", strings.Join(r.Warnings, "; "),
		"repairs", len(out.Repairs), "protected", out.Protected)
	return out, nil
}
