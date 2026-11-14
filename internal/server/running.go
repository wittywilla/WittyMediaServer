// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// RunningNow is what the server is in the middle of, as log attributes, for the line that
// says why it's stopping: what a stop cut short is half of what someone reading it needs.
// Counts and kinds only, never titles.
func (s *Server) RunningNow() []any {
	transcodes, games, connections := 0, 0, 0
	if s.HLS != nil {
		transcodes = s.HLS.ActiveCount()
	}
	for _, v := range s.modSessions.List() {
		if v.Kind == "game" {
			games++
		}
	}
	if s.Conns != nil {
		connections = s.Conns.Open()
	}
	kinds := map[string]int{}
	if s.Tasks != nil {
		for _, t := range s.Tasks.Live() {
			if t.State == tasks.Running || t.State == tasks.Queued || t.State == tasks.Paused {
				kinds[t.Type]++
			}
		}
	}
	names := make([]string, 0, len(kinds))
	for k, n := range kinds {
		names = append(names, fmt.Sprintf("%s×%d", k, n))
	}
	sort.Strings(names)
	list := strings.Join(names, ",")
	if list == "" {
		list = "none"
	}
	return []any{"transcodes", transcodes, "games", games, "connections", connections, "tasks", list}
}
