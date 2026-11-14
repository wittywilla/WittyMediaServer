// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/host"
)

// A module of API 5 says what it adds to this server in integration.json (wittymodule
// Integration): its routes, what its web bundle adds to the web app, its data and its
// lifecycle. The server reads it from the version's folder, checks it before the version is
// used, and applies it while the version is in use (docs/MODULES-API5.md).

// Offers is what this server offers a module, host feature by host feature: the versions a
// module's integration.json ranges are checked against.
func Offers() map[string]int {
	return map[string]int{
		wittymodule.HostHTTP: 1,
		wittymodule.HostWeb:  5,
		wittymodule.HostSDUI: sdui.CatalogRevision,
		wittymodule.HostAPI:  host.Version,
	}
}

// ReservedPaths are the first parts of /api/modules/{id}/... that are the server's own, which a
// module's route can't start with.
var ReservedPaths = []string{"install", "rollback", "versions", "pin", "autoupdate", "latest", "source",
	"icon", "notices", "add", "remove", "enable", "disable", "web", "failed", "pending", "override", "check"}

// readIntegration reads and checks a version's integration.json: nil and no error when it has
// none and needn't.
func (m *Manager) readIntegration(dir string, man *wittymodule.Manifest) (*wittymodule.Integration, error) {
	b, err := os.ReadFile(filepath.Join(dir, wittymodule.IntegrationFile))
	if errors.Is(err, fs.ErrNotExist) {
		if man.API >= IntegrationAPI && man.NeedsIntegration() {
			return nil, fmt.Errorf("it is built for module API %d, where a module says what it adds to a server, and it has no %s", man.API, wittymodule.IntegrationFile)
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if man.API < IntegrationAPI {
		// An older version's file isn't read: what it adds is the server's own copy's to do.
		return nil, nil
	}
	in, err := wittymodule.ParseIntegration(b)
	if err != nil {
		return nil, err
	}
	if err := in.Validate(man); err != nil {
		return nil, fmt.Errorf("its %s doesn't pass the checks: %s", wittymodule.IntegrationFile, firstLines(err.Error(), 3))
	}
	if err := host.Needs(in, Offers()); err != nil {
		return nil, fmt.Errorf("this server is too old or too new for it: %w", err)
	}
	for _, r := range in.HTTP {
		first, _, _ := strings.Cut(strings.TrimPrefix(r.Path, "/"), "/")
		for _, res := range ReservedPaths {
			if first == res {
				return nil, fmt.Errorf("its route %s starts with %q, which is the server's own", r.Path, res)
			}
		}
	}
	if m.cfg.IntegrationCheck != nil {
		if err := m.cfg.IntegrationCheck(man.ID, in); err != nil {
			return nil, err
		}
	}
	return in, nil
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "; ")
}

// InUse is a module's integration while one of its versions is in use.
type InUse struct {
	ID          string
	Name        string
	Version     string
	Integration *wittymodule.Integration
}

// Integrations are the integrations of the modules in use and turned on, in module order, and a
// number that changes whenever the versions in use may have, so a caller can keep what it makes
// of them.
func (m *Manager) Integrations() ([]InUse, uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []InUse
	for _, id := range m.order {
		e := m.mods[id]
		if e.integ != nil && e.dir != "" && !e.state.Disabled {
			out = append(out, InUse{ID: id, Name: e.man.Name, Version: e.man.Version, Integration: e.integ})
		}
	}
	return out, m.uiGen
}

// Integration is one module's integration in use, nil when it has none: a module with none in
// use (absent, turned off, or a version older than API 5) is the server's own copy's to answer.
func (m *Manager) Integration(id string) *wittymodule.Integration {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.mods[id]; e != nil && e.dir != "" && !e.state.Disabled {
		return e.integ
	}
	return nil
}
