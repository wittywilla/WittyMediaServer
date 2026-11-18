// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// Separating the whole music library in the background, so every song is ready before anyone
// sings it. Off unless an administrator turns it on (karaokeLibrary "idle"), and gentle when it is:
// one song at a time, only while nothing else is being made and nobody is being served work it
// would slow, never on a tiny machine, and it stops well before the space for karaoke versions
// is full, so it never clears what people actually sang to make room for what they might.

const (
	karaokeLibraryEvery = 5 * time.Minute
	// karaokeLibraryFill is how full the space for karaoke versions may get from this, in percent.
	karaokeLibraryFill = 80
)

func init() {
	settingKeys = append(settingKeys, "karaokeLibrary")
	settingDefaults["karaokeLibrary"] = "off"
	settingChecks["karaokeLibrary"] = oneOf("off", "idle")
}

type karaokeLibraryState struct {
	once sync.Once
	mu   sync.Mutex
	skip map[int64]bool // songs that failed or can't be done, this run of the server
	said string         // the last reason it waited, said once
}

// startKaraokeLibrary starts the loop; it does nothing while the setting is off.
func (s *Server) startKaraokeLibrary() {
	s.kLib.once.Do(func() {
		logging.Go("karaoke library", func() {
			t := time.NewTicker(karaokeLibraryEvery)
			defer t.Stop()
			for range t.C {
				s.karaokeLibraryStep(context.Background())
			}
		}, "cat", "extras")
	})
}

// karaokeLibraryStep separates at most one song, if everything says it may.
func (s *Server) karaokeLibraryStep(ctx context.Context) {
	if s.DB.GetSetting(ctx, "karaokeLibrary", "off") != "idle" {
		return
	}
	wait := func(why string) {
		s.kLib.mu.Lock()
		defer s.kLib.mu.Unlock()
		if s.kLib.said != why {
			s.kLib.said = why
			s.Log.Info("separating the music library is waiting", "cat", "extras", "why", why)
		}
	}
	words, _, _, _ := s.separator(ctx)
	switch {
	case words == nil:
		wait("no separator is installed")
		return
	case pace.Machine() == pace.Tiny:
		wait("this machine is too small")
		return
	case karaokeBusy():
		wait("someone is being served")
		return
	case s.mediaJobsBusy():
		wait("something else is being made")
		return
	}
	used, _ := mediaextras.KaraokeUsage(s.extrasRoot())
	if used*100 >= s.karaokeCap(ctx)*karaokeLibraryFill {
		wait(fmt.Sprintf("the space for karaoke versions is %d%% full", karaokeLibraryFill))
		return
	}
	it := s.nextSongToSeparate(ctx)
	if it == nil {
		wait("every song is done")
		return
	}
	s.kLib.mu.Lock()
	s.kLib.said = ""
	s.kLib.mu.Unlock()
	item := *it
	_, err := s.startMediaJob(fmt.Sprintf("stems-separate:%d", it.ID), nil,
		tasks.Spec{Type: tasks.TypeOther, Title: "Separating the music library", Phase: "Waiting", AdminOnly: true},
		func(ctx context.Context, task *tasks.Handle) error {
			task.Item(item.Title, "")
			err := s.separateStems(ctx, &item, words, task)
			if err != nil && ctx.Err() == nil {
				s.karaoke.fail("real", item.ID, "failed")
				s.kLib.mu.Lock()
				s.kLib.skip[item.ID] = true
				s.kLib.mu.Unlock()
			}
			return err
		})
	if err != nil {
		s.kLib.mu.Lock()
		s.kLib.skip[item.ID] = true
		s.kLib.mu.Unlock()
	}
}

// nextSongToSeparate is the first song, oldest added first, in a music library, that is a file
// and has no separated parts yet.
func (s *Server) nextSongToSeparate(ctx context.Context) *library.Item {
	s.kLib.mu.Lock()
	if s.kLib.skip == nil {
		s.kLib.skip = map[int64]bool{}
	}
	s.kLib.mu.Unlock()
	libs, err := s.Store.Libraries(ctx)
	if err != nil {
		return nil
	}
	for _, lib := range libs {
		if lib.Type != "music" {
			continue
		}
		for off := 0; off < 200_000; off += 200 {
			items, _, err := s.Store.Items(ctx, library.ItemQuery{LibraryID: lib.ID, Kind: "audio", Sort: "added", Limit: 200, Offset: off, NoTotal: true, Brief: true})
			if err != nil || len(items) == 0 {
				break
			}
			for i := range items {
				it := items[i]
				s.kLib.mu.Lock()
				skip := s.kLib.skip[it.ID]
				s.kLib.mu.Unlock()
				if skip || s.karaoke.failure("real", it.ID) != "" || !localFileItem(it) {
					continue
				}
				if q, _, _ := s.karaokeMix(ctx, it, false); q == "stems" {
					continue
				}
				return it
			}
		}
	}
	return nil
}

// mediaJobsBusy says a media job is running or waiting.
func (s *Server) mediaJobsBusy() bool {
	j := &s.mediaJobs
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.busy || len(j.line) > 0
}
