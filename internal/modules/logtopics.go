// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

// LogCapability is where a module's manifest lists the advanced logging options it
// offers, each as "id|label|description". The module prints the detail when asked
// by WITTYMODULE_LOG_TOPICS. See docs/MODULES.md.
const LogCapability = "logTopics"

// LogTopics is what the version of a module in use says it can log.
func (m *Manager) LogTopics(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil || e.man == nil {
		return nil
	}
	return e.man.Capabilities[LogCapability]
}

// DoctorCapability is where a module's manifest lists the checks it runs for Repair, each
// as "id|title". The server asks a running helper for their findings with the service
// call DoctorMethod. See docs/MODULES.md.
const DoctorCapability = "doctor"

// DoctorMethod is the service call that answers a module's doctor checks: no arguments,
// and a list of DoctorFinding back.
const DoctorMethod = "doctor.Check"

// DoctorFinding is one thing a module's own check found, in plain words.
type DoctorFinding struct {
	Check    string   `json:"check"`    // one of the ids its manifest lists
	Key      string   `json:"key"`      // the same finding gets the same key next time
	Severity string   `json:"severity"` // info | warn | problem
	Title    string   `json:"title"`
	What     string   `json:"what"`
	Steps    []string `json:"steps,omitempty"`
}

// DoctorChecks is what the version of a module in use says it checks.
func (m *Manager) DoctorChecks(id string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil || e.man == nil {
		return nil
	}
	return e.man.Capabilities[DoctorCapability]
}
