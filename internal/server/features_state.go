// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/diagnose"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/separator"
	"github.com/wittywilla/wittymodule/service"
)

// featureState is how the newer features are doing, for Repair's Features checks and the
// support bundle's features.json. It asks nothing slow: what each part already knows, and
// WittyPlay for a few seconds at most. No secrets: no keys, invites, codes or device addresses.
func (s *Server) featureState(ctx context.Context) *diagnose.FeatureState {
	f := &diagnose.FeatureState{Overrides: []diagnose.OverrideState{}, Channels: []diagnose.ChannelState{}}

	// Peer to peer.
	cfg := s.loadP2P(ctx)
	p := &diagnose.P2PState{On: cfg.Enabled, RelayLayer: cfg.Layers.Relay, PortMapLayer: cfg.Layers.PortMap, Devices: len(s.p2p.devices.list())}
	if node := s.p2pNode(); node != nil {
		st := node.Status()
		p.Running, p.Reachability, p.NATUDP, p.NATTCP = true, st.Reachability, st.NATUDP, st.NATTCP
		p.Relays, p.Connected = len(st.Relays), len(st.Devices)
		p.PortMapped, p.MappingError, p.Shared = st.Mapping.On, logging.Redact(st.Mapping.Error), st.Mapping.Shared
	} else if cfg.Enabled {
		p.Error = "not started; the log's Peer to peer area says why"
	}
	f.P2P = p

	// Modules under an override.
	if s.Modules != nil {
		for _, m := range s.Modules.Report().Modules {
			if o := m.Override; o != nil {
				f.Overrides = append(f.Overrides, diagnose.OverrideState{Module: m.ID, Version: o.Version, Kinds: o.Kinds, By: o.By, At: o.At, Source: o.Source})
			}
		}
	}

	// WittyPlay's standalone emulators.
	if service.Running != nil && service.Running(wittyPlayID) {
		em := &diagnose.EmulatorState{}
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var raw json.RawMessage
		if err := service.Call(cctx, wittyPlayID, "docker.Summary", nil, nil, nil, []any{&raw}); err == nil && len(raw) > 0 {
			if json.Unmarshal(raw, em) == nil {
				em.Known = true
				em.Error = logging.Redact(em.Error)
			}
		}
		cancel()
		f.Emulators = em
	}

	// Karaoke's separator: only what was found already, unless nobody has looked yet.
	sep := &diagnose.SeparatorState{Wanted: s.DB.GetSetting(ctx, "karaokeAuto", "auto") != "never", Looked: len(separator.Options{}.SearchDirs())}
	if words, name, version, _ := s.separator(ctx); len(words) > 0 {
		sep.Found, sep.Name, sep.Version = true, name, version
	}
	f.Separator = sep

	// Channels' last health checks.
	if s.TV != nil {
		if tuners, err := s.TV.Tuners(ctx); err == nil {
			for _, t := range tuners {
				if h := s.TV.Health(t.ID); h != nil && h.Done > 0 {
					f.Channels = append(f.Channels, diagnose.ChannelState{Source: t.Name, Total: h.Done, OK: h.OK, Failing: h.Failing, Checked: h.Started})
				}
			}
		}
	}

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

// wittyPlayID is the module that plays games.
const wittyPlayID = "wittyplay"
