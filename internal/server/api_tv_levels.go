// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"net/http"
	"sort"
	"strings"
)

// What a television shows in place of an editor it doesn't offer (docs/TV-MODE.md): a few lines of
// what is set now, so the card isn't a dead end, and the addresses on the home network for its QR
// code, since the address the television itself uses may be one a phone can't open (the desktop
// app's own, on this computer).

type tvSummary struct {
	ID    string   `json:"id"`
	Lines []string `json:"lines"`
	// Addresses: this server's addresses on the home network, asked from there. Empty away from home.
	Addresses []string `json:"addresses"`
}

// GET /api/sdui/tv-summary/{id}: id is a screen's summary in the catalogue's tvLevels. Each line is
// only what the asker may already see: administrators' screens say nothing to anyone else.
func (s *Server) handleTVSummary(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	out := tvSummary{ID: id, Lines: []string{}, Addresses: []string{}}
	if onLocalNetwork(r) {
		if a := s.lanURLs(r); a != nil {
			out.Addresses = a
		}
	}
	u := userFrom(r)
	ctx := r.Context()
	plural := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	switch id {
	case "libraries":
		libs, err := s.visibleLibraries(ctx, s.access(r))
		if err != nil {
			break
		}
		kinds := map[string]int{}
		for _, l := range libs {
			kinds[l.Type]++
		}
		out.Lines = append(out.Lines, plural(len(libs), "library", "libraries"))
		names := make([]string, 0, len(kinds))
		for k := range kinds {
			names = append(names, k)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, k := range names {
			parts = append(parts, fmt.Sprintf("%s %d", k, kinds[k]))
		}
		if len(parts) > 0 {
			out.Lines = append(out.Lines, strings.Join(parts, ", "))
		}
	case "channels":
		if s.Channels == nil {
			break
		}
		list, err := s.Channels.List(ctx)
		if err != nil {
			break
		}
		if !u.IsAdmin {
			a := s.access(r)
			n := 0
			for _, c := range list {
				if s.Channels.Visible(ctx, c, u, a) {
					n++
				}
			}
			out.Lines = append(out.Lines, plural(n, "channel", "channels"))
			break
		}
		out.Lines = append(out.Lines, plural(len(list), "channel", "channels"))
		bumpers := 0
		if libs, err := s.visibleLibraries(ctx, s.access(r)); err == nil {
			for _, l := range libs {
				if l.Type == "commercials" {
					bumpers += l.ItemCount
				}
			}
		}
		if bumpers > 0 {
			out.Lines = append(out.Lines, plural(bumpers, "commercial or bumper", "commercials and bumpers"))
		}
	case "modules":
		if !u.IsAdmin || s.Modules == nil {
			break
		}
		inUse := 0
		for _, m := range s.Modules.Report().Modules {
			if m.Version != "" {
				inUse++
			}
		}
		out.Lines = append(out.Lines, plural(inUse, "module in use", "modules in use"))
	case "backup":
		if !u.IsAdmin {
			break
		}
		sc := s.loadBackupSchedule(ctx)
		if sc.Frequency == "" || sc.Frequency == "off" {
			out.Lines = append(out.Lines, i18n.T("Automatic backups: off"))
		} else {
			out.Lines = append(out.Lines, fmt.Sprintf(i18n.T("Automatic backups: %s at %02d:00, keeping %d"), sc.Frequency, sc.Hour, sc.Keep))
		}
	case "transcoding":
		if !u.IsAdmin {
			break
		}
		hw := s.DB.GetSetting(ctx, "hwEncoders", "")
		if hw == "" && s.DB.GetSetting(ctx, "hwAccel", "0") == "1" {
			hw = "any"
		}
		if hw == "" || hw == "none" {
			out.Lines = append(out.Lines, i18n.T("Graphics chip: not used"))
		} else {
			out.Lines = append(out.Lines, "Graphics chip: "+hw)
		}
		if h := s.DB.GetSetting(ctx, "maxHeight", ""); h != "" && h != "0" {
			out.Lines = append(out.Lines, "Largest picture: "+h+"p")
		}
	case "network":
		if !u.IsAdmin {
			break
		}
		out.Lines = append(out.Lines, "Listening on "+s.Cfg.Listen)
	}
	writeJSON(w, 200, out)
}
