// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/diagnose"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/separator"
	"github.com/wittywilla/wittymodule/service"
)

// featureState is how the newer features are doing, for Repair's Features checks and the
// support bundle's features.json. It asks nothing slow: what each part already knows, and
// each module with checks of its own for a few seconds at most. No secrets: no keys, invites, codes or device addresses.
func (s *Server) featureState(ctx context.Context) *diagnose.FeatureState {
	f := &diagnose.FeatureState{Overrides: []diagnose.OverrideState{}, Modules: []diagnose.ModuleHealth{}}

	// Peer to peer.
	cfg := s.loadP2P(ctx)
	p := &diagnose.P2PState{On: cfg.Enabled, RelayLayer: cfg.Layers.Relay, PortMapLayer: cfg.Layers.PortMap, Devices: len(s.p2p.devices.list())}
	if node := s.p2pNode(); node != nil {
		st := node.Status()
		p.Running, p.Reachability, p.NATUDP, p.NATTCP = true, st.Reachability, st.NATUDP, st.NATTCP
		p.Relays = len(st.Relays)
		for _, d := range st.Devices {
			if !d.Limited { // a public relay's connection carries no requests: not connected in any way that counts
				p.Connected++
			}
		}
		p.PortMapped, p.MappingError, p.Shared = st.Mapping.On, logging.Redact(st.Mapping.Error), st.Mapping.Shared
	} else if cfg.Enabled {
		p.Error = "not started; the log's Peer to peer area says why"
	}
	f.P2P = p

	// Every module: its state, and what its own checks found (capabilities.doctor in its
	// manifest, answered by the module). Overrides are noted as well.
	if s.Modules != nil {
		for _, m := range s.Modules.Report().Modules {
			if o := m.Override; o != nil {
				f.Overrides = append(f.Overrides, diagnose.OverrideState{Module: m.ID, Version: o.Version, Kinds: o.Kinds, By: o.By, At: o.At, Source: o.Source})
			}
			if m.State == modules.StateAbsent {
				continue
			}
			h := diagnose.ModuleHealth{ID: m.ID, Name: m.Name, Version: m.Version, State: m.State, Delivery: m.Delivery,
				Notice: logging.Redact(m.Notice), LastFail: logging.Redact(m.LastFail), Checks: s.Modules.DoctorChecks(m.ID)}
			h.Running = service.Running != nil && service.Running(m.ID)
			// A helper that isn't running is started to answer: Repair and a bug report
			// are asked for, and its checks are what they're for.
			if len(h.Checks) > 0 && (m.State == modules.StateInstalled || m.State == modules.StateBuiltin) {
				h.Findings, h.AskError = askModuleDoctor(ctx, m.ID)
			}
			f.Modules = append(f.Modules, h)
		}
	}

	// Karaoke's separator: only what was found already, unless nobody has looked yet.
	sep := &diagnose.SeparatorState{Wanted: s.DB.GetSetting(ctx, "karaokeAuto", "auto") != "never", Looked: len(separator.Options{}.SearchDirs())}
	if words, name, version, _ := s.separator(ctx); len(words) > 0 {
		sep.Found, sep.Name, sep.Version = true, name, version
	}
	f.Separator = sep

	// The pages' configuration.
	if st, err := s.uiStore(); err == nil && st != nil {
		ui := &diagnose.SDUIState{}
		if c := st.Current(); c != nil {
			ui.Revision = c.Revision
		}
		ui.DraftPending = st.Draft() != nil
		f.SDUI = ui
	}
	return f
}

// askModuleDoctor runs a module's own checks. What it says is cleaned like anything else
// from outside: a known severity, bounded text, no secrets.
func askModuleDoctor(ctx context.Context, id string) ([]diagnose.ModuleFinding, string) {
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var got []modules.DoctorFinding
	if err := service.Call(cctx, id, modules.DoctorMethod, nil, nil, nil, []any{&got}); err != nil {
		return nil, logging.Redact(cutRunes(err.Error(), 300))
	}
	out := []diagnose.ModuleFinding{}
	for i, g := range got {
		if i == 20 {
			break
		}
		switch g.Severity {
		case "info", "warn", "problem":
		default:
			g.Severity = "warn"
		}
		fd := diagnose.ModuleFinding{Check: logging.ValidSession(g.Check), Key: logging.ValidSession(g.Key), Severity: g.Severity,
			Title: logging.Redact(cutRunes(g.Title, 160)), What: logging.Redact(cutRunes(g.What, 1000))}
		for j, st := range g.Steps {
			if j == 6 {
				break
			}
			fd.Steps = append(fd.Steps, logging.Redact(cutRunes(st, 300)))
		}
		out = append(out, fd)
	}
	return out, ""
}
