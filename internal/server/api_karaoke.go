// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Making stems for a song that has none. internal/mediaextras/separate.go says what the
// two ways are and why neither is built in; docs/LYRICS-KARAOKE.md says it for people.
//
//   - POST .../stems/approximate: FFmpeg takes one channel from the other. Anyone who may
//     play the song may ask, because it costs about what playing it does; the result is
//     kept and labelled "approximate".
//   - POST .../stems/separate: the owner's separator, when one is named on the machine
//     (config.StemSeparator). Administrators only: it can hold a processor for minutes.
//
// Both are media jobs: queued, one at a time, low priority, a task each.

// separatorTimeout is how long one song may take the owner's separator.
var separatorTimeout = 45 * time.Minute

// separationInfo is the "separation" part of GET /api/media/{id}/stems: what this server
// can make for this song, and whether it is making it now.
type separationInfo struct {
	// Approximate: FFmpeg's centre-channel removal can be asked for.
	Approximate separationWay `json:"approximate"`
	// External: the owner's separator can be asked for (by an administrator).
	External separationWay `json:"external"`
	// Best is the best this server can make: "separator" or "approximate", "" for neither.
	// Separator names the one in use ("Demucs"), found on the machine or named in its
	// configuration.
	Best      string `json:"best"`
	Separator string `json:"separator,omitempty"`
}

type separationWay struct {
	Available bool `json:"available"`
	// Reason says why not: not-a-song, not-a-file, no-ffmpeg, not-configured, admin-only.
	Reason string `json:"reason,omitempty"`
	// Task is the task making it now, when one is.
	Task string `json:"task,omitempty"`
	// Note is what a person should know about the result.
	Note string `json:"note,omitempty"`
}

const approxNote = "Approximate: made by cancelling what the left and right channels share. The voice is quieter, not gone, and the music is mono."

func (s *Server) separatorWords() []string {
	words, _, _, _ := s.separator(context.Background())
	return words
}

func (s *Server) separationFor(r *http.Request, it *library.Item) separationInfo {
	var out separationInfo
	base := ""
	switch {
	case it.Kind != "audio":
		base = "not-a-song"
	case !localFileItem(it):
		base = "not-a-file"
	}
	out.Approximate = separationWay{Reason: base, Note: approxNote, Task: s.mediaJobTask(fmt.Sprintf("stems-approx:%d", it.ID))}
	if base == "" {
		if s.Tools == nil || s.Tools.FFmpeg == "" {
			out.Approximate.Reason = "no-ffmpeg"
		} else {
			out.Approximate.Available = true
		}
	}
	out.External = separationWay{Reason: base, Task: s.mediaJobTask(fmt.Sprintf("stems-separate:%d", it.ID))}
	words, name, _, _ := s.separator(r.Context())
	switch {
	case words != nil:
		out.Best, out.Separator = "separator", name
	case s.Tools != nil && s.Tools.FFmpeg != "":
		out.Best = "approximate"
	}
	if base == "" {
		switch {
		case words == nil:
			out.External.Reason = "not-configured"
		case !userFrom(r).IsAdmin:
			out.External.Reason = "admin-only"
		default:
			out.External.Available = true
		}
	}
	return out
}

// songForStems is the request's item if it is a song in a library folder.
func (s *Server) songForStems(w http.ResponseWriter, r *http.Request) *library.Item {
	it := s.extrasItem(w, r)
	if it == nil {
		return nil
	}
	if it.Kind != "audio" {
		writeErr(w, 400, "only songs have stems")
		return nil
	}
	if !localFileItem(it) || !s.Store.LibraryFile(r.Context(), it.Path) {
		writeErr(w, 409, "this song isn't a file in a library, so there is nothing to make stems from")
		return nil
	}
	return it
}

func jobAnswer(w http.ResponseWriter, id string, err error) {
	switch {
	case err == nil:
		writeJSON(w, 202, map[string]any{"ok": true, "task": id})
	case errors.Is(err, errJobQueued):
		writeJSON(w, 202, map[string]any{"ok": true, "task": id, "already": true})
	case errors.Is(err, errJobsFull):
		w.Header().Set("Retry-After", "120")
		writeErr(w, 429, err.Error())
	default:
		writeErr(w, 503, err.Error())
	}
}

// POST /api/media/{id}/stems/approximate: make the approximate instrumental, and keep it.
func (s *Server) handleStemsApproximate(w http.ResponseWriter, r *http.Request) {
	it := s.songForStems(w, r)
	if it == nil {
		return
	}
	if s.Tools == nil || s.Tools.FFmpeg == "" {
		writeErr(w, 503, "FFmpeg isn't available, and it is what makes this")
		return
	}
	for _, st := range s.stemsOf(r.Context(), it) {
		if st.Name == "instrumental" {
			writeJSON(w, 200, map[string]any{"ok": true, "exists": true, "approximate": st.Approximate, "source": st.Source})
			return
		}
	}
	dir := filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path), mediaextras.ApproxDir)
	item := *it
	id, err := s.startMediaJob(fmt.Sprintf("stems-approx:%d", it.ID), userFrom(r),
		tasks.Spec{Type: tasks.TypeOther, Title: "Making an approximate instrumental", Private: true},
		func(ctx context.Context, task *tasks.Handle) error {
			task.Item(item.Title, "")
			return s.makeApproxInstrumental(ctx, &item, dir)
		})
	jobAnswer(w, id, err)
}

// errNotStereo: there is no "difference between the channels" of a mono song.
var errNotStereo = errors.New("this song is mono, so there are no two channels to take from each other")

func (s *Server) makeApproxInstrumental(ctx context.Context, it *library.Item, dir string) error {
	if s.Tools.FFprobe != "" {
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		p, err := s.Tools.ProbeFile(pctx, it.Path)
		cancel()
		if err == nil {
			if a := p.Audio(); len(a) > 0 && a[0].Channels == 1 {
				return errNotStereo
			}
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// The instrumental, and the whole song encoded the same way beside it, so an app can
	// blend the two sample for sample.
	var tmps [2]string
	for i := range tmps {
		tmp, err := os.CreateTemp(dir, ".wms-*.tmp")
		if err != nil {
			return err
		}
		tmp.Close()
		tmps[i] = tmp.Name()
		defer os.Remove(tmp.Name())
	}
	args := append([]string{"-v", "error", "-nostdin", "-y", "-threads", "1"}, ffmpeg.FileInput(it.Path)...)
	args = append(args, mediaextras.ApproxPairArgs(tmps[0], tmps[1])...)
	cctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	if _, tail, err := ffmpeg.Run(cctx, s.Tools.FFmpeg, args, "for", "approximate instrumental", "item", it.ID); err != nil {
		return fmt.Errorf("FFmpeg couldn't make it: %w: %s", err, tail)
	}
	for _, p := range tmps {
		if fi, err := os.Stat(p); err != nil || fi.Size() == 0 || fi.Size() > mediaextras.MaxStem {
			return errors.New("FFmpeg made nothing usable")
		}
	}
	if err := os.Rename(tmps[1], filepath.Join(dir, mediaextras.ApproxFull+mediaextras.ApproxExt)); err != nil {
		return err
	}
	if err := os.Rename(tmps[0], filepath.Join(dir, "instrumental"+mediaextras.ApproxExt)); err != nil {
		return err
	}
	s.karaokeTidy(ctx, it)
	return nil
}

// POST /api/media/{id}/stems/separate: run the owner's separator on the song, and keep the
// parts it writes.
func (s *Server) handleStemsSeparate(w http.ResponseWriter, r *http.Request) {
	it := s.songForStems(w, r)
	if it == nil {
		return
	}
	words := s.separatorWords()
	if words == nil {
		writeErr(w, 409, "No stem separator is set up on this server. Its owner names one where the server starts: see \"Making stems\" in the lyrics and karaoke guide.")
		return
	}
	if _, _, err := mediaextras.SeparatorCommand(words, "in", "out"); err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	item := *it
	id, err := s.startMediaJob(fmt.Sprintf("stems-separate:%d", it.ID), userFrom(r),
		tasks.Spec{Type: tasks.TypeOther, Title: "Separating a song into stems", Phase: "Waiting", AdminOnly: true},
		func(ctx context.Context, task *tasks.Handle) error {
			task.Item(item.Title, "")
			return s.separateStems(ctx, &item, words, task)
		})
	jobAnswer(w, id, err)
}

func (s *Server) separateStems(ctx context.Context, it *library.Item, words []string, task *tasks.Handle) error {
	work, err := os.MkdirTemp(s.Cfg.CacheDir, "stems-*")
	if err != nil {
		if err = os.MkdirAll(s.Cfg.CacheDir, 0o755); err == nil {
			work, err = os.MkdirTemp(s.Cfg.CacheDir, "stems-*")
		}
		if err != nil {
			return err
		}
	}
	defer os.RemoveAll(work)
	prog, args, err := mediaextras.SeparatorCommand(words, it.Path, work)
	if err != nil {
		return err
	}
	task.Phase("Separating")
	cctx, cancel := context.WithTimeout(ctx, separatorTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, prog, args...)
	cmd.Dir = work
	said := &logging.Tail{}
	s.karaoke.setProgress(it.ID, 0)
	defer s.karaoke.setProgress(it.ID, -1)
	out := io.MultiWriter(said, &percentWatcher{last: -1, say: func(pct int) {
		s.karaoke.setProgress(it.ID, pct)
		task.Progress(int64(pct), 100)
	}})
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = 10 * time.Second
	started := time.Now()
	if err := ffmpeg.RunLow(cmd); err != nil {
		if cctx.Err() != nil && ctx.Err() == nil {
			return fmt.Errorf("the separator was stopped after %s without finishing", separatorTimeout)
		}
		var notFound *exec.Error
		if errors.As(err, &notFound) {
			return fmt.Errorf("the separator %q couldn't be started: %w", prog, err)
		}
		return fmt.Errorf("the separator failed: %w: %s", err, said)
	}
	task.Phase("Keeping the parts")
	s.compactStems(ctx, work, 0)
	dst := filepath.Join(s.extrasRoot(), mediaextras.Key(it.ID, it.Path), "stems")
	kept, err := mediaextras.CollectStems(work, dst)
	if err != nil {
		return err
	}
	list := mediaextras.Separated(dst)
	for _, k := range kept {
		list[k] = true
	}
	names := make([]string, 0, len(list))
	for k := range list {
		names = append(names, k)
	}
	_ = os.WriteFile(filepath.Join(dst, mediaextras.SeparatedList), []byte(strings.Join(names, "\n")+"\n"), 0o644)
	// Real parts replace the approximate pair.
	if list["instrumental"] || list["vocals"] {
		approx := filepath.Join(filepath.Dir(dst), mediaextras.ApproxDir)
		_ = os.Remove(filepath.Join(approx, "instrumental"+mediaextras.ApproxExt))
		_ = os.Remove(filepath.Join(approx, mediaextras.ApproxFull+mediaextras.ApproxExt))
	}
	s.karaokeTidy(ctx, it)
	task.Result(fmt.Sprintf("Kept %s.", strings.Join(kept, ", ")))
	s.Log.Info("a song was separated into stems", "cat", "library", "item", it.ID, "parts", kept, "took", time.Since(started).Round(time.Second).String())
	return nil
}
