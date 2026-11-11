// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"sync"

	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gameid"
)

// What's known about a games library's files, remembered so that a scan reads a file
// once and never again until it changes. Archives are only read in a worker process.

// archiveIndex lives for one scan of one library. Nil store: remembers nothing past it.
type archiveIndex struct {
	store     *Store
	libraryID int64
	lister    *gamearchive.Lister
	// reading, when set, is told about each archive that really has to be opened: on a
	// slow network folder the first scan can take a while, and this shows it's moving.
	reading func(path string)
	// stopped, when set, says the scan has been cancelled. Looking through a large library
	// can be minutes of work, so discover stops at the next file. What it worked out by then
	// is kept, so the next scan carries on from there.
	stopped func() bool
	// looking, when set, is told how far through a folder's files the scan has got.
	looking func(done, total int)

	verdictOnce   sync.Once
	verdicts      map[string]cachedVerdict
	seenVerdicts  map[string]cachedVerdict
	freshVerdicts map[string]bool
	decisions     map[string]gameDecision
	notes         []gameFileNote
	lastLibrary   string
	peekedAt      map[string]bool
	told          map[string]bool
}

func newArchiveIndex(store *Store, libraryID int64) *archiveIndex {
	return &archiveIndex{store: store, libraryID: libraryID, seenVerdicts: map[string]cachedVerdict{}, freshVerdicts: map[string]bool{},
		peekedAt: map[string]bool{}, told: map[string]bool{}}
}

func (ix *archiveIndex) close() {
	if ix.lister != nil {
		ix.lister.Close()
		ix.lister = nil
	}
}

func (ix *archiveIndex) open(path string) *gamearchive.Lister {
	if ix.lister == nil {
		ix.lister = gamearchive.NewLister()
	}
	if ix.reading != nil && !ix.told[path] {
		ix.told[path] = true
		ix.reading(path)
	}
	return ix.lister
}

// listing reads what an archive holds, or says why it can't be read.
func (ix *archiveIndex) listing(path string) (*gamearchive.Listing, string) {
	l, err := ix.open(path).List(path, gamearchive.ScanBudget)
	if err != nil {
		code := gamearchive.CodeOf(err)
		if code == "" {
			code = gamearchive.ErrDamaged.Code
		}
		return nil, code
	}
	return l, ""
}

// peek looks at the start of one file in an archive. A file that can't be reached says
// nothing, and doesn't count against the archive: see peeked.
func (ix *archiveIndex) peek(path string, e gamearchive.Entry) gameid.Finding {
	b, err := ix.open(path).Peek(path, e.Name)
	if err != nil {
		return gameid.Finding{}
	}
	ix.peekedAt[path] = true
	size := e.Size
	if size <= 0 {
		size = int64(len(b))
	}
	return gameid.SniffHead(b, size)
}

// peeked says a look inside this archive really did read something.
func (ix *archiveIndex) peeked(path string) bool { return ix.peekedAt[path] }

// save keeps what this scan worked out. prune: every folder was reachable, so a file
// that wasn't seen is gone.
func (ix *archiveIndex) save(ctx context.Context, prune bool) {
	ix.verdictOnce.Do(ix.loadVerdicts)
	ix.saveVerdicts(ctx)
	if prune {
		ix.forgetVerdicts(ctx)
	}
}
