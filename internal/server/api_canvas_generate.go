// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Making a canvas from the video itself, for a title that came without one: a few seconds
// cut from it, silent, small, and joined end to start so it loops without a jump. An
// administrator asks; it is a media job (queued, one at a time, low priority, a task).

// canvasShape is how big a made canvas is on this machine. A canvas plays behind text, so
// it is small everywhere, and smaller where FFmpeg has less to work with.
type canvasShape struct {
	height, fps int
	seconds     float64
}

func canvasShapeFor(c pace.Class) canvasShape {
	switch c {
	case pace.Tiny:
		return canvasShape{360, 15, 5}
	case pace.Small:
		return canvasShape{480, 24, 6}
	}
	return canvasShape{720, 24, 8}
}

const (
	// canvasFade is how much of the end is blended into the start, in seconds.
	canvasFade = 1.0
	// maxCanvasSeconds is the longest canvas that can be asked for.
	maxCanvasSeconds = 15.0
)

// canvasArgs are FFmpeg's arguments after the input for a loop of dur seconds. With fade,
// the clip's last second is cross-faded into its first, and the first second is then left
// off: the last frame shown is the frame just before the first one shown.
func canvasArgs(sh canvasShape, dur float64, fade bool, encoder, out string) []string {
	scale := fmt.Sprintf("scale=-2:'min(%d,ih)':flags=bilinear,fps=%d,format=yuv420p", sh.height, sh.fps)
	args := []string{"-an", "-sn", "-dn", "-map_metadata", "-1", "-map_chapters", "-1"}
	if fade {
		f := strconv.FormatFloat(canvasFade, 'f', -1, 64)
		d := strconv.FormatFloat(dur, 'f', 3, 64)
		filter := "[0:v:0]" + scale + ",split=2[main][head];" +
			"[head]trim=0:" + f + ",setpts=PTS-STARTPTS[h];" +
			"[main]trim=" + f + ":" + strconv.FormatFloat(dur+canvasFade, 'f', 3, 64) + ",setpts=PTS-STARTPTS[m];" +
			"[m][h]xfade=transition=fade:duration=" + f + ":offset=" + strconv.FormatFloat(dur-canvasFade, 'f', 3, 64) + ",format=yuv420p[out]"
		args = append(args, "-filter_complex", filter, "-map", "[out]", "-t", d)
	} else {
		args = append(args, "-map", "0:v:0", "-vf", scale, "-t", strconv.FormatFloat(dur, 'f', 3, 64))
	}
	args = append(args, "-c:v", encoder)
	if encoder == "libx264" {
		args = append(args, "-preset", "veryfast", "-crf", "28", "-profile:v", "main")
	} else {
		args = append(args, "-b:v", "900k")
	}
	return append(args, "-pix_fmt", "yuv420p", "-threads", "1", "-movflags", "+faststart", "-f", "mp4", out)
}

// canvasEncoder is an H.264 encoder this FFmpeg has that needs no graphics card, or "".
func (s *Server) canvasEncoder() string {
	for _, e := range []string{"libx264", "libopenh264"} {
		if s.Tools != nil && s.Tools.Encoders[e] {
			return e
		}
	}
	return ""
}

// POST /api/media/{id}/canvas/generate {startSec?, durationSec?}: cut a looping canvas from
// the video. It becomes the item's uploaded canvas, replacing one made or uploaded before.
func (s *Server) handleCanvasGenerate(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	var b struct {
		StartSec    *float64 `json:"startSec"`
		DurationSec *float64 `json:"durationSec"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body: {} or {startSec, durationSec}")
		return
	}
	if it.Kind != "video" {
		writeErr(w, 400, "A canvas can be made from a video file. A song or a disc has none to cut from: upload one, or put a canvas.mp4 beside it.")
		return
	}
	if !localFileItem(it) || !s.Store.LibraryFile(r.Context(), it.Path) {
		writeErr(w, 409, "this video isn't a file in a library")
		return
	}
	enc := s.canvasEncoder()
	if s.Tools == nil || s.Tools.FFmpeg == "" || enc == "" {
		writeErr(w, 503, "This server's FFmpeg can't make one: it has no H.264 encoder (libx264).")
		return
	}
	sh := canvasShapeFor(pace.Machine())
	dur := sh.seconds
	if b.DurationSec != nil {
		dur = *b.DurationSec
	}
	if !(dur >= 2 && dur <= maxCanvasSeconds) {
		writeErr(w, 400, fmt.Sprintf("durationSec: from 2 to %g seconds", maxCanvasSeconds))
		return
	}
	// By default, a quarter of the way in: past the logos and titles, before any ending.
	start := it.RuntimeSec * 0.25
	if b.StartSec != nil {
		start = *b.StartSec
	}
	if !(start >= 0) || (it.RuntimeSec > 0 && start+dur+canvasFade > it.RuntimeSec) {
		writeErr(w, 400, "startSec: the clip has to fit inside the video")
		return
	}
	item := *it
	id, err := s.startMediaJob(fmt.Sprintf("canvas:%d", it.ID), userFrom(r),
		tasks.Spec{Type: tasks.TypeOther, Title: "Making a canvas", AdminOnly: true},
		func(ctx context.Context, task *tasks.Handle) error {
			task.Item(item.Title, "")
			return s.makeCanvas(ctx, &item, sh, start, dur, enc)
		})
	jobAnswer(w, id, err)
}

func (s *Server) makeCanvas(ctx context.Context, it *library.Item, sh canvasShape, start, dur float64, enc string) error {
	rel := mediaextras.Key(it.ID, it.Path) + "/canvas"
	dir := filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".wms-*.tmp")
	if err != nil {
		return err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	fade := s.Tools.HasFilter("xfade")
	args := []string{"-v", "error", "-nostdin", "-y", "-ss", strconv.FormatFloat(start, 'f', 3, 64), "-t", strconv.FormatFloat(dur+canvasFade+1, 'f', 3, 64)}
	args = append(args, ffmpeg.FileInput(it.Path)...)
	args = append(args, canvasArgs(sh, dur, fade, enc, tmp.Name())...)
	cctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	if _, tail, err := ffmpeg.Run(cctx, s.Tools.FFmpeg, args, "for", "canvas", "item", it.ID); err != nil {
		return fmt.Errorf("FFmpeg couldn't make it: %w: %s", err, tail)
	}
	fi, err := os.Stat(tmp.Name())
	if err != nil || fi.Size() == 0 {
		return errors.New("FFmpeg made nothing")
	}
	if fi.Size() > mediaextras.MaxCanvas {
		return fmt.Errorf("the canvas came out at %d MB, over the %d MB limit; ask for a shorter one", fi.Size()>>20, mediaextras.MaxCanvas>>20)
	}
	dst := filepath.Join(dir, "canvas.mp4")
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return err
	}
	s.removeOthers(rel, mediaextras.CanvasExts, dst)
	_ = os.WriteFile(filepath.Join(dir, mediaextras.GeneratedMark), nil, 0o644)
	s.Log.Info("canvas made", "cat", "library", "item", it.ID, "bytes", fi.Size(), "seconds", dur, "height", sh.height)
	return nil
}
