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
		Modules: []ModuleHealth{
			{ID: "wittyplay", Name: "WittyPlay", Version: "0.5.0", State: "installed", Delivery: "helper", Running: true, Checks: []string{"host|Can emulators run here?"},
				Findings: []ModuleFinding{{Check: "host", Key: "permission-screen-recording", Severity: "problem", Title: "macOS hasn't let the server record the screen", What: "…", Steps: []string{"Open System Settings"}}}},
			{ID: "wittytuner", Name: "WittyTuner", State: "installed", Delivery: "helper", Running: true, Checks: []string{"channels|Channel health"}, AskError: "timed out"},
			{ID: "wittydvd", Name: "WittyDVD", State: "installed", Delivery: "helper", LastFail: "it exited with status 2"},
			{ID: "wittycd", Name: "WittyCD", State: "fallback", Notice: "its signature doesn't check out"},
			{ID: "wittyphotos", Name: "WittyPhotos", State: "installed", Delivery: "helper", Running: true},
			{ID: "wittystats", Name: "WittyStats", State: "disabled", Delivery: "web"},
		},
		Separator: &SeparatorState{Wanted: true, Looked: 7},
	})
	want := map[string]Severity{
		"p2p-reach":                SevProblem,
		"module-override-wittydvd": SevWarn,
		"module-wittyplay-permission-screen-recording": SevProblem,
		"module-health-wittytuner":                     SevWarn,
		"module-health-wittydvd":                       SevProblem,
		"module-health-wittycd":                        SevProblem,
		"separator":                                    SevWarn,
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
		Modules:   []ModuleHealth{{ID: "wittyplay", State: "installed", Delivery: "helper", Running: true, Checks: []string{"host|Host"}, Findings: []ModuleFinding{}}},
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
