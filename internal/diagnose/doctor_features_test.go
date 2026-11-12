// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package diagnose

import (
	"context"
	"strings"
	"testing"
)

func runFeatures(t *testing.T, f *FeatureState) map[string]Issue {
	t.Helper()
	e := &DoctorEnv{Features: func(context.Context) *FeatureState { return f }}
	out := map[string]Issue{}
	for _, c := range featureChecks() {
		for _, i := range c.Run(context.Background(), e) {
			out[i.Key] = i
		}
	}
	return out
}

func TestFeatureFindings(t *testing.T) {
	got := runFeatures(t, &FeatureState{
		P2P:       &P2PState{On: true, Running: true, Reachability: "private", NATUDP: "strict", RelayLayer: true, PortMapLayer: true, MappingError: "no UPnP"},
		Overrides: []OverrideState{{Module: "wittydvd", Version: "0.4.0", Kinds: []string{"unsigned"}, By: "admin", At: 1762900000}},
		Emulators: &EmulatorState{Known: true, Backend: "host", Permissions: []EmulatorPermission{{Name: "screen-recording"}, {Name: "accessibility", Granted: true}}},
		Separator: &SeparatorState{Wanted: true, Looked: 7},
		Channels:  []ChannelState{{Source: "Playlist A", Total: 50, OK: 40, Failing: 10}, {Source: "Playlist B", Total: 5, Failing: 5}, {Source: "Fine", Total: 3, OK: 3}},
	})
	want := map[string]Severity{
		"p2p-reach":                            SevProblem,
		"module-override-wittydvd":             SevWarn,
		"emulator-permission-screen-recording": SevProblem,
		"separator":                            SevWarn,
		"channel-health-Playlist A":            SevWarn,
		"channel-health-Playlist B":            SevProblem,
	}
	for k, sev := range want {
		i, ok := got[k]
		if !ok {
			t.Errorf("no finding %s; got %v", k, keys(got))
			continue
		}
		if i.Severity != sev || i.Title == "" || i.What == "" || len(i.Steps) == 0 {
			t.Errorf("%s: %+v", k, i)
		}
	}
	if len(got) != len(want) {
		t.Errorf("findings %v, want %d", keys(got), len(want))
	}
	if !strings.Contains(got["p2p-reach"].What, "strict NAT") || !strings.Contains(got["p2p-reach"].What, "no relay") {
		t.Errorf("the reason is missing: %s", got["p2p-reach"].What)
	}
}

func TestFeatureFindingsWhenAllIsWell(t *testing.T) {
	got := runFeatures(t, &FeatureState{
		P2P:       &P2PState{On: true, Running: true, Reachability: "public"},
		Emulators: &EmulatorState{Known: true, Backend: "docker", Available: true},
		Separator: &SeparatorState{Wanted: true, Found: true},
	})
	if len(got) != 0 {
		t.Fatalf("findings on a healthy server: %v", keys(got))
	}
	// From the command line nothing is running: one note, not a finding per check.
	e := &DoctorEnv{}
	n := 0
	for _, c := range featureChecks() {
		for _, i := range c.Run(context.Background(), e) {
			if !i.NotChecked {
				t.Errorf("%s isn't marked not checked", i.Key)
			}
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d notes without a server, want 1", n)
	}
}

func keys(m map[string]Issue) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
