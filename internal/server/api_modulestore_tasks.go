// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"fmt"
	"sync"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// watchModuleRuns shows Update All, and an install that brings others with it, in the activity
// panel, for admins only. A single install has its own task already, from the manager.
func (s *Server) watchModuleRuns() {
	if s.ModuleStores == nil {
		return
	}
	var mu sync.Mutex
	handles := map[string]*tasks.Handle{}
	reported := map[string]bool{}
	s.ModuleStores.Watch = func(r *modmgr.Run) {
		if r.Kind != modmgr.RunUpdateAll && len(r.Items) < 2 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		h := handles[r.ID]
		if h == nil {
			// Taking a finished run back also says how it stands, and that isn't new work.
			if r.Finished != 0 {
				return
			}
			title := "Installing modules"
			if r.Kind == modmgr.RunUpdateAll {
				title = "Updating modules"
			}
			h = s.Tasks.Start(tasks.Spec{Type: tasks.TypeModule, Title: title, Unit: "modules", AdminOnly: true})
			handles[r.ID] = h
		}
		done := int64(0)
		for _, it := range r.Items {
			switch it.State {
			case modmgr.RunWorking:
				h.Item(it.Name, "")
			case modmgr.RunFailed:
				done++
				if key := r.ID + it.ID; it.Failure != nil && !reported[key] {
					reported[key] = true
					h.Problem(it.Name, it.Failure.Title)
				}
			case modmgr.RunDone, modmgr.RunSkipped, modmgr.RunUndone:
				done++
			}
		}
		h.Progress(done, int64(len(r.Items)))
		if r.Finished != 0 {
			h.Result(fmt.Sprintf("%d updated, %d failed, %d skipped.", r.Updated, r.Failed, r.Skipped))
			var err error
			if r.Failed > 0 && r.Updated == 0 {
				err = errors.New("nothing could be updated")
			}
			h.Finish(err)
			delete(handles, r.ID)
		}
	}
}
