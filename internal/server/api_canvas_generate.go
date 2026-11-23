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
	"regexp"
	"strconv"
	"strings"
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

// canvasArgsCrop is canvasArgs with the picture first cut to crop (FFmpeg's crop=w:h:x:y), when it is not "".
func canvasArgsCrop(sh canvasShape, dur float64, fade bool, encoder, out, crop string) []string {
	scale := fmt.Sprintf("scale=-2:'min(%d,ih)':flags=bilinear,fps=%d,format=yuv420p", sh.height, sh.fps)
	if crop != "" {
		scale = crop + "," + scale
	}
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

// POST /api/media/{id}/canvas/generate {startSec?, durationSec?, crossfade?, fromId?}: cut a looping canvas
// from the item's own video, or from the video fromId names (a song's from its music video, say). It becomes
// the item's uploaded canvas, replacing one made or uploaded before. crossfade (on unless false) blends the
// clip's end into its start.
func (s *Server) handleCanvasGenerate(w http.ResponseWriter, r *http.Request) {
	it := s.extrasItem(w, r)
	if it == nil {
		return
	}
	var b cutRequest
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body: {} or {startSec, durationSec, crossfade, fromId}")
		return
	}
	if !canvasKind(it) {
		writeErr(w, 400, "only songs, videos and discs have a canvas")
		return
	}
	src, err := s.canvasSourceItem(r, it, b.FromID)
	if err != nil {
		code := 400
		if strings.HasPrefix(err.Error(), "that video isn't") {
			code = 409
		}
		writeErr(w, code, err.Error())
		return
	}
	enc := s.canvasEncoder()
	if s.Tools == nil || s.Tools.FFmpeg == "" || enc == "" {
		writeErr(w, 503, "This server's FFmpeg can't make one: it has no H.264 encoder (libx264).")
		return
	}
	sh := canvasShapeFor(pace.Machine())
	dur, err := cutLength(b)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// By default, a quarter of the way in: past the logos and titles, before any ending.
	start := src.RuntimeSec * 0.25
	if b.StartSec != nil {
		start = *b.StartSec
	}
	fade := b.Crossfade == nil || *b.Crossfade
	need := dur
	if fade {
		need += canvasFade // the blend takes a second more of the video
	}
	if !(start >= 0) || (src.RuntimeSec > 0 && start+need > src.RuntimeSec) {
		writeErr(w, 400, "startSec: the clip has to fit inside the video")
		return
	}
	target, from := *it, src.Path
	id, err := s.startMediaJob(fmt.Sprintf("canvas:%d", it.ID), userFrom(r),
		tasks.Spec{Type: tasks.TypeOther, Title: "Making a canvas", AdminOnly: true},
		func(ctx context.Context, task *tasks.Handle) error {
			task.Item(target.Title, "")
			return s.cutCanvas(ctx, &target, from, sh, start, dur, enc, fade)
		})
	jobAnswer(w, id, err)
}

// cutCanvas cuts dur seconds from the video file at from, starting at start, into it's canvas.
func (s *Server) cutCanvas(ctx context.Context, it *library.Item, from string, sh canvasShape, start, dur float64, enc string, wantFade bool) error {
	log := s.jobLog(ctx).With("item", it.ID)
	log.Debug("making a canvas", "start", start, "seconds", dur, "height", sh.height, "encoder", enc)
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
	fade := wantFade && s.Tools.HasFilter("xfade")
	args := []string{"-v", "error", "-nostdin", "-y", "-ss", strconv.FormatFloat(start, 'f', 3, 64), "-t", strconv.FormatFloat(dur+canvasFade+1, 'f', 3, 64)}
	args = append(args, ffmpeg.FileInput(from)...)
	// A film's own black bars are left out: a canvas fills a cover's square or the screen, where bars would show.
	args = append(args, canvasArgsCrop(sh, dur, fade, enc, tmp.Name(), s.letterbox(ctx, from, start))...)
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
	s.dropCanvasLoop(it)
	if err := os.WriteFile(filepath.Join(dir, mediaextras.GeneratedMark), nil, 0o644); err != nil {
		// The canvas is kept, but without the mark it counts as one someone put there.
		log.Warn("could not mark the canvas as made by the server", "dir", dir, "err", err)
	}
	log.Info("canvas made", "bytes", fi.Size(), "seconds", dur, "height", sh.height)
	return nil
}

// cropLine is what FFmpeg's cropdetect says it found.
var cropLine = regexp.MustCompile(`crop=(\d+):(\d+):(\d+):(\d+)`)

// letterbox finds black bars baked into a video, from two seconds of it at start, as a crop for FFmpeg, or "" when
// there are none worth cutting (under 8 pixels a side) or it couldn't tell. It reads only those two seconds.
func (s *Server) letterbox(ctx context.Context, from string, start float64) string {
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"-v", "info", "-nostdin", "-ss", strconv.FormatFloat(start, 'f', 3, 64), "-t", "2"}
	args = append(args, ffmpeg.FileInput(from)...)
	args = append(args, "-an", "-sn", "-dn", "-vf", "cropdetect=limit=24:round=2:reset=0", "-f", "null", "-")
	_, tail, err := ffmpeg.Run(cctx, s.Tools.FFmpeg, args, "for", "canvas letterbox")
	if err != nil || tail == nil {
		return ""
	}
	m := cropLine.FindAllStringSubmatch(tail.String(), -1)
	if len(m) == 0 {
		return ""
	}
	last := m[len(m)-1]
	x, _ := strconv.Atoi(last[3])
	y, _ := strconv.Atoi(last[4])
	if x < 8 && y < 8 {
		return ""
	}
	return "crop=" + last[1] + ":" + last[2] + ":" + last[3] + ":" + last[4]
}
