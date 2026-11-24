// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package diagnose

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// FeatureState is how the newer parts of a running server are doing: peer to peer,
// module overrides, WittyPlay's emulators, karaoke's separator, the channels' health
// checks and the pages' configuration. The server fills it in (DoctorEnv.Features); the
// checks below read it, and the support bundle carries it as features.json. It holds no
// secrets: no keys, invites, codes, addresses of devices or file contents.
type FeatureState struct {
	P2P       *P2PState       `json:"p2p,omitempty"`
	Overrides []OverrideState `json:"overrides"`
	Modules   []ModuleHealth  `json:"modules"`
	Separator *SeparatorState `json:"separator,omitempty"`
	SDUI      *SDUIState      `json:"sdui,omitempty"`
}

// P2PState is the peer-to-peer node, as Settings shows it.
type P2PState struct {
	On           bool   `json:"on"`
	Running      bool   `json:"running"`
	Error        string `json:"error,omitempty"` // why it isn't running
	Reachability string `json:"reachability,omitempty"`
	NATUDP       string `json:"natUdp,omitempty"`
	NATTCP       string `json:"natTcp,omitempty"`
	Relays       int    `json:"relays"`
	RelayLayer   bool   `json:"relayLayer"`
	PortMapLayer bool   `json:"portMapLayer"`
	PortMapped   bool   `json:"portMapped"`
	MappingError string `json:"mappingError,omitempty"`
	Shared       bool   `json:"sharedAddress,omitempty"` // the router's outside address isn't public
	Devices      int    `json:"devices"`
	Connected    int    `json:"connected"`
}

// OverrideState is one module running under a manual override.
type OverrideState struct {
	Module  string   `json:"module"`
	Version string   `json:"version"`
	Kinds   []string `json:"kinds"`
	By      string   `json:"by,omitempty"`
	At      int64    `json:"at,omitempty"`
	Source  string   `json:"source,omitempty"`
}

// SeparatorState is the program that separates vocals for karaoke.
type SeparatorState struct {
	Wanted  bool   `json:"wanted"` // karaoke may separate songs (the setting isn't "never")
	Found   bool   `json:"found"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Looked  int    `json:"looked"` // how many folders were searched
}

// ModuleHealth is one installed module as the running server sees it, with what its own
// checks found. A module declares its checks in its manifest (capabilities.doctor) and
// answers them itself; the server names no module here.
type ModuleHealth struct {
	ID       string          `json:"id"`
	Name     string          `json:"name,omitempty"`
	Version  string          `json:"version,omitempty"`
	State    string          `json:"state"` // installed | builtin | fallback | disabled
	Delivery string          `json:"delivery,omitempty"`
	Running  bool            `json:"running"`          // a helper's process is up
	Notice   string          `json:"notice,omitempty"` // why the version wanted isn't in use
	LastFail string          `json:"lastFailure,omitempty"`
	Checks   []string        `json:"checks,omitempty"` // "id|title" from its manifest
	Findings []ModuleFinding `json:"findings,omitempty"`
	AskError string          `json:"askError,omitempty"` // its checks couldn't be asked
}

// ModuleFinding is one thing a module's own check found.
type ModuleFinding struct {
	Check    string   `json:"check"`
	Key      string   `json:"key"`
	Severity string   `json:"severity"` // info | warn | problem
	Title    string   `json:"title"`
	What     string   `json:"what"`
	Steps    []string `json:"steps,omitempty"`
}

// SDUIState is the pages' configuration.
type SDUIState struct {
	Revision     int64    `json:"revision"`
	DraftPending bool     `json:"draftPending"`
	Refused      []string `json:"refusedModules,omitempty"` // modules whose pages weren't loaded
}

func featureChecks() []Check {
	return []Check{
		{ID: "module-health", Area: AreaModules, Title: "Is every installed module running, and do its own checks pass?", Run: checkModuleHealth},
		{ID: "p2p-reach", Area: AreaFeatures, Title: "Can devices away from home reach this server peer to peer?", Run: checkP2P},
		{ID: "module-overrides", Area: AreaFeatures, Title: "Is any module running under a manual override?", Run: checkOverrides},
		{ID: "separator", Area: AreaFeatures, Title: "Can karaoke separate vocals?", Run: checkSeparator},
	}
}

// features asks the running server once per run. From the command line there's no
// running server, and these checks say so.
func (e *DoctorEnv) features(ctx context.Context) *FeatureState {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.featuresDone {
		return e.featureState
	}
	e.featuresDone = true
	if e.Features != nil {
		e.featureState = e.Features(ctx)
	}
	return e.featureState
}

func notRunning(key, what string) []Issue {
	return []Issue{{Key: key, Severity: SevInfo, Safety: SafetyManual, NotChecked: true, Title: what + " wasn't checked",
		What: "This is only known to a running server. Run Repair from Settings to check it."}}
}

func checkP2P(ctx context.Context, e *DoctorEnv) []Issue {
	f := e.features(ctx)
	if f == nil {
		return notRunning("features", "Peer to peer, module overrides, emulators, karaoke and channel health")
	}
	p := f.P2P
	if p == nil || !p.On {
		return nil
	}
	steps := []string{"Open Settings, Remote Access, Peer to peer, and press Test the connection.", "Read Settings, WittyLog, with Follow set to A peer-to-peer connection, for what each layer said."}
	if !p.Running {
		return []Issue{{Key: "p2p-reach", Severity: SevProblem, Safety: SafetyManual, Title: "Peer-to-peer connections are on, but not running",
			What: "They couldn't start: " + orWords(p.Error, "the reason wasn't recorded") + ". Devices away from home can't reach this server this way.", Steps: steps}}
	}
	if p.Reachability == "public" {
		return nil
	}
	var why []string
	if p.NATUDP == "strict" || p.NATTCP == "strict" {
		why = append(why, "the router changes the outside port for every connection (a strict NAT), so a direct path can't be punched")
	}
	if p.PortMapLayer && !p.PortMapped {
		why = append(why, "the router didn't accept a port mapping"+suffix(p.MappingError))
	}
	if p.Shared {
		why = append(why, "the router's own outside address isn't a public one (a second router, or an internet provider that shares addresses)")
	}
	if p.Relays > 0 {
		if len(why) == 0 || p.Reachability == "unknown" {
			return nil
		}
		return []Issue{{Key: "p2p-reach", Severity: SevInfo, Safety: SafetyManual, Title: "Devices away from home reach this server only through a relay",
			What: "It works, but more slowly: " + strings.Join(why, "; ") + ". " + fmt.Sprint(p.Relays) + " relay(s) hold a place for it.", Steps: steps}}
	}
	if p.Reachability != "private" {
		return nil // still finding out
	}
	if !p.RelayLayer {
		why = append(why, "relays are turned off")
	} else {
		why = append(why, "no relay holds a place for it")
	}
	return []Issue{{Key: "p2p-reach", Severity: SevProblem, Safety: SafetyManual, Title: "Devices away from home can't reach this server peer to peer",
		What:  "Computers outside your network tried and couldn't connect: " + strings.Join(why, "; ") + ".",
		Steps: append(steps, "Turn on Relays, or let the router map a port (UPnP or NAT-PMP), or forward the peer-to-peer port by hand.")}}
}

func checkOverrides(ctx context.Context, e *DoctorEnv) []Issue {
	f := e.features(ctx)
	if f == nil {
		return nil // said once, by checkP2P
	}
	var out []Issue
	for _, o := range f.Overrides {
		when := ""
		if o.At > 0 {
			when = " on " + time.Unix(o.At, 0).Format("2 Jan 2006")
		}
		who := ""
		if o.By != "" {
			who = " by " + o.By
		}
		out = append(out, Issue{Key: "module-override-" + o.Module, Severity: SevWarn, Safety: SafetyManual,
			Title: o.Module + " " + o.Version + " runs under a manual override",
			What: "Checks it would normally have to pass were skipped" + who + when + ": " + strings.Join(o.Kinds, ", ") +
				". It isn't updated by itself while overridden.",
			Steps: []string{"Open Settings, Module store, " + o.Module + ", and remove the override when a version that passes the checks is out.",
				"Settings, WittyLog, Follow: Audit records lists every override and who made it."}})
	}
	return out
}

func checkSeparator(ctx context.Context, e *DoctorEnv) []Issue {
	f := e.features(ctx)
	if f == nil {
		return nil // said once, by checkP2P
	}
	s := f.Separator
	if s == nil || !s.Wanted || s.Found {
		return nil
	}
	return []Issue{{Key: "separator", Severity: SevWarn, Safety: SafetyManual, Title: "Karaoke can't separate vocals: no separator was found",
		What:  fmt.Sprintf("None of Demucs, Spleeter or audio-separator was found in the %d places the server looks. Songs get an approximate instrumental, made by cancelling the centre of the mix, instead.", s.Looked),
		Steps: []string{"Install Demucs (pip install demucs), Spleeter or audio-separator for the account that runs the server.", "Start the server again, or open Settings, Music, Karaoke, so it looks again."}}}
}

func checkModuleHealth(ctx context.Context, e *DoctorEnv) []Issue {
	f := e.features(ctx)
	if f == nil {
		return nil // said once, by checkP2P
	}
	var out []Issue
	for _, m := range f.Modules {
		name := orWords(m.Name, m.ID)
		steps := []string{"Open Settings, Module store, " + name + ", for its state and what it last said.",
			"Settings, WittyLog, Module: " + name + " shows its lines; turn on its options there for more."}
		switch {
		case m.State == "fallback":
			out = append(out, Issue{Key: "module-health-" + m.ID, Severity: SevProblem, Safety: SafetyManual, Title: name + " isn't running the version it should",
				What: orWords(m.Notice, "A version was refused") + ".", Steps: steps})
			continue
		case m.State == "disabled":
			continue
		case m.Delivery == "helper" && !m.Running && m.LastFail != "":
			out = append(out, Issue{Key: "module-health-" + m.ID, Severity: SevProblem, Safety: SafetyManual, Title: name + " isn't running",
				What: "Its helper stopped: " + m.LastFail + ".", Steps: steps})
			continue
		}
		if len(m.Checks) > 0 && m.AskError != "" {
			out = append(out, Issue{Key: "module-health-" + m.ID, Severity: SevWarn, Safety: SafetyManual, Title: name + "'s own checks couldn't run",
				What: "It was asked and didn't answer: " + m.AskError + ".", Steps: steps})
		}
		for _, fd := range m.Findings {
			sev := SevInfo
			switch fd.Severity {
			case "problem":
				sev = SevProblem
			case "warn":
				sev = SevWarn
			}
			out = append(out, Issue{Key: "module-" + m.ID + "-" + orWords(fd.Key, fd.Check), Severity: sev, Safety: SafetyManual,
				Title: fd.Title, What: fd.What, Steps: fd.Steps, Details: []string{name + " " + m.Version + ", check " + fd.Check}})
		}
	}
	return out
}

func orWords(s, alt string) string {
	if strings.TrimSpace(s) == "" {
		return alt
	}
	return s
}

func suffix(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}
