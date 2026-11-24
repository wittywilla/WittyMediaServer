// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"testing"
)

// The lookahead reads only new or changed video files, no more than its width at once, and a
// file it didn't read is read by the scan as before.
func TestLookaheadReadsNewFilmsOnly(t *testing.T) {
	s := testScanner(t, t.TempDir()) // its Tools have no ffprobe: every read fails at once
	ctx := context.Background()
	l := &lookahead{s: s, ctx: ctx, width: 1, jobs: map[string]*aheadJob{}, busy: make(chan struct{}, 1)}
	all := []candidate{
		{path: "/m/a.mkv", kind: "video"},
		{path: "/m/b.iso", kind: "dvd-iso"}, // a disc: not read ahead
		{path: "/m/c.mkv", kind: "video", mtime: 5, size: 9, known: &knownFile{mtime: 5, size: 9}}, // unchanged
		{path: "/m/d.mkv", kind: "video"},
		{path: "/m/e.mkv", kind: "video"},
	}
	l.feed(all, 0)
	l.mu.Lock()
	_, d := l.jobs["/m/d.mkv"]
	n := len(l.jobs)
	l.mu.Unlock()
	if !d || n != 1 {
		t.Fatalf("after the first film, read ahead %d (d: %v); want only d.mkv, past the disc and the unchanged file", n, d)
	}
	// The scan reaches d: it takes what was read (an error here, as there is no ffprobe).
	if _, err, ok := l.take(ctx, "/m/d.mkv"); !ok || err == nil {
		t.Fatalf("take(d) = ok %v, err %v; want what was read ahead", ok, err)
	}
	if _, _, ok := l.take(ctx, "/m/a.mkv"); ok {
		t.Fatal("a.mkv was never read ahead, so the scan must read it itself")
	}
	l.feed(all, 3)
	if _, _, ok := l.take(ctx, "/m/e.mkv"); !ok {
		t.Fatal("e.mkv should have been read ahead once d's turn came")
	}
	// No lookahead at all (one processor, or a music library): the scan reads every file.
	var none *lookahead
	none.feed(all, 0)
	if _, _, ok := none.take(ctx, "/m/a.mkv"); ok {
		t.Fatal("a nil lookahead has nothing to give")
	}
	lib := &Library{Type: "music"}
	if s.newLookahead(ctx, lib) != nil {
		t.Fatal("a music library's songs aren't read ahead")
	}
}
