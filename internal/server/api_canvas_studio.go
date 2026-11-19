// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// The Canvas studio (web/src/canvas/CanvasStudio.tsx): a canvas for a song or a title can be cut from any
// video file the administrator can see (a music video, a film, an episode), or from a clip sent from their
// device, which is cut and then thrown away; and any canvas, however it came, can be given a loop of its
// own (a start and an end) that players keep to, with no new file made.

// maxCanvasSource is the largest clip a device can send to have a canvas cut from it: a phone's minute of
// 4K video fits. It is kept only while the canvas is made.
const maxCanvasSource = 512 << 20

// canvasLoopName is the file beside an item's canvas that holds its loop.
const canvasLoopName = "canvas-loop.json"

// canvasLoop: where the canvas's loop starts and ends, in milliseconds, and which file it was set for (its
// ETag), so a loop made for one canvas is never applied to the next.
type canvasLoop struct {
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	For     string `json:"for"`
}

func (s *Server) canvasLoopPath(it *library.Item) string {
	return filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path), canvasLoopName)
}

// readCanvasLoop is the loop set for the canvas whose ETag is etag, or nil.
func (s *Server) readCanvasLoop(it *library.Item, etag string) *canvasLoop {
	b, err := os.ReadFile(s.canvasLoopPath(it))
	if err != nil {
		return nil
	}
	var l canvasLoop
	if json.Unmarshal(b, &l) != nil || l.For != etag || l.EndMs <= l.StartMs {
		return nil
	}
	return &l
}

// dropCanvasLoop forgets a loop: a new canvas starts with none.
func (s *Server) dropCanvasLoop(it *library.Item) { _ = os.Remove(s.canvasLoopPath(it)) }

// PUT /api/media/{id}/canvas/loop {startMs, endMs}: the part of the canvas players loop.
// DELETE takes it away, and the whole canvas loops again.
func (s *Server) handleCanvasLoop(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	if r.Method == http.MethodDelete {
		s.dropCanvasLoop(it)
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	}
	var b struct {
		StartMs int64 `json:"startMs"`
		EndMs   int64 `json:"endMs"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected {startMs, endMs}")
		return
	}
	f, ok := s.findCanvas(r.Context(), it)
	if !ok {
		writeErr(w, 404, "this has no canvas to loop")
		return
	}
	fh, st, err := s.openExtra(r.Context(), f)
	if err != nil {
		writeErr(w, 404, "this has no canvas to loop")
		return
	}
	fh.Close()
	_, _, durMs := s.probeCanvas(r.Context(), f, st)
	if b.StartMs < 0 || b.EndMs-b.StartMs < 500 || (durMs > 0 && b.EndMs > durMs+50) {
		writeErr(w, 400, "the loop has to be at least half a second long, inside the canvas")
		return
	}
	l := canvasLoop{StartMs: b.StartMs, EndMs: b.EndMs, For: strings.Trim(extraETag(st), `"`)}
	data, _ := json.Marshal(l)
	p := s.canvasLoopPath(it)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		writeErr(w, 500, "couldn't keep the loop: "+err.Error())
		return
	}
	writeJSON(w, 200, l)
}

// cutRequest is what a cut asks for: where, how long, and whether its end is blended into its start.
type cutRequest struct {
	StartSec    *float64 `json:"startSec"`
	DurationSec *float64 `json:"durationSec"`
	Crossfade   *bool    `json:"crossfade"`
	FromID      *int64   `json:"fromId"`
}

// cutLength checks the length asked for, or gives this machine's own.
func cutLength(b cutRequest) (float64, error) {
	dur := canvasShapeFor(pace.Machine()).seconds
	if b.DurationSec != nil {
		dur = *b.DurationSec
	}
	if !(dur >= 2 && dur <= maxCanvasSeconds) {
		return 0, fmt.Errorf("durationSec: from 2 to %g seconds", maxCanvasSeconds)
	}
	return dur, nil
}

// PUT /api/media/{id}/canvas/source?startSec=&durationSec=&crossfade=: a clip from the administrator's own
// device (MP4, MOV or WebM, up to 512 MB) as the body. The part asked for is cut into the item's canvas as a
// task, as from a video in a library, and the clip is then deleted.
func (s *Server) handleCanvasSource(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	if !canvasKind(it) {
		writeErr(w, 400, "only songs, videos and discs have a canvas")
		return
	}
	enc := s.canvasEncoder()
	if s.Tools == nil || s.Tools.FFmpeg == "" || enc == "" {
		writeErr(w, 503, "This server's FFmpeg can't make one: it has no H.264 encoder (libx264).")
		return
	}
	q := r.URL.Query()
	var b cutRequest
	if v := q.Get("startSec"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 {
			writeErr(w, 400, "startSec: a number of seconds")
			return
		}
		b.StartSec = &f
	}
	if v := q.Get("durationSec"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			writeErr(w, 400, "durationSec: a number of seconds")
			return
		}
		b.DurationSec = &f
	}
	dur, err := cutLength(b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	start := 0.0
	if b.StartSec != nil {
		start = *b.StartSec
	}
	fade := q.Get("crossfade") != "0"
	rel := mediaextras.Key(it.ID, it.Path) + "/canvas-source"
	src, size, err := mediaextras.SaveUpload(s.extrasRoot(), rel, http.MaxBytesReader(w, r.Body, maxCanvasSource), maxCanvasSource, mediaextras.VideoExt)
	if err != nil {
		uploadErr(w, err, "a clip to cut a canvas from", maxCanvasSource)
		return
	}
	s.Log.InfoContext(r.Context(), "clip received to cut a canvas from", "cat", "extras", "item", it.ID, "bytes", size)
	target := *it
	sh := canvasShapeFor(pace.Machine())
	id, err := s.startMediaJob(fmt.Sprintf("canvas:%d", it.ID), userFrom(r),
		tasks.Spec{Type: tasks.TypeOther, Title: "Making a canvas", AdminOnly: true},
		func(ctx context.Context, task *tasks.Handle) error {
			defer os.Remove(src)
			task.Item(target.Title, "")
			return s.cutCanvas(ctx, &target, src, sh, start, dur, enc, fade)
		})
	if err != nil {
		_ = os.Remove(src)
	}
	jobAnswer(w, id, err)
}

// canvasSourceItem is the video a canvas is cut from: the item itself, or the one fromId names, which the
// one asking has to be able to see and which has to be a video file in a library.
func (s *Server) canvasSourceItem(r *http.Request, it *library.Item, fromID *int64) (*library.Item, error) {
	src := it
	if fromID != nil && *fromID != it.ID {
		v, err := s.visibleItem(r, *fromID)
		if err != nil {
			return nil, errors.New("fromId: no such video")
		}
		src = v
	}
	if src.Kind != "video" {
		return nil, errors.New("A canvas can be cut from a video file. A song or a disc has none to cut from: choose a video, upload a clip, or put a canvas.mp4 beside it.")
	}
	if !localFileItem(src) || !s.Store.LibraryFile(r.Context(), src.Path) {
		return nil, errors.New("that video isn't a file in a library")
	}
	return src, nil
}
