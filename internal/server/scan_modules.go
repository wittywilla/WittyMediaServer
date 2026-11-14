// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/service"
)

// Scan hooks (module API 5, docs/MODULES-API5.md#scan-hooks): a library of a kind a module's
// integration.json scan part lists is scanned through that module (library/scan_hooks.go).
// This is the server's side of it: which module scans what, its calls, and what it says a
// library can be held to (scan.choices: a games library's consoles).

// managerScans answers the scanner from the module manager, and calls a module's service.
type managerScans struct{ m *modmgr.Manager }

func (a managerScans) ScanModule(kind string) (library.ScanModule, bool) {
	by, ok := a.m.ScanModule(kind)
	if !ok {
		return library.ScanModule{}, false
	}
	return library.ScanModule{ID: by.ID, Name: by.Name, Version: by.Version, On: by.On, Describe: by.Part.Describe, Facet: by.Part.Facet}, true
}

// Call asks a module's service a scan call. A scan call has no time limit of its own (the last
// identify call groups every file of a library at once, 16,000 and more in a big games library):
// it ends when the scan's context does, which a person stopping the scan or the scan's
// no-progress watchdog does (library.StuckAfter). A service call's usual limit applies only
// without a scan around it (scan.choices has its own).
func (a managerScans) Call(ctx context.Context, module, call string, args, out any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, noDeadline)
		defer cancel()
	}
	// The service is started (or found) first, and kept from being stopped as idle while it
	// answers: a long call is the service in use, not a service with nothing to do.
	if _, err := a.m.ServiceConn(module); err != nil {
		return err
	}
	defer a.m.Using(module)()
	return service.Call(ctx, module, call, nil, nil, []any{args}, []any{out})
}

// noDeadline is a deadline no call reaches: service.Call gives a call without one a limit.
var noDeadline = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)

// useScanModules lets scans reach the modules in use. Tests give the scanner their own first.
func (s *Server) useScanModules() {
	if s.Scanner != nil && s.Scanner.Modules == nil && s.Modules != nil {
		s.Scanner.Modules = managerScans{s.Modules}
	}
}

// scanModule is the module that scans libraries of a kind, if one is in use and on.
func (s *Server) scanModule(kind string) (library.ScanModule, bool) {
	if s.Scanner == nil || s.Scanner.Modules == nil {
		return library.ScanModule{}, false
	}
	m, ok := s.Scanner.Modules.ScanModule(kind)
	return m, ok && m.On
}

// moduleScans says whether a module scans libraries of this kind now (host API: only those
// libraries' items and scans are its to touch).
func (s *Server) moduleScans(module, kind string) bool {
	m, ok := s.scanModule(kind)
	return ok && m.ID == module
}

// afterModuleScan tells modules a scan of one of their kinds of library finished.
func (s *Server) afterModuleScan(id int64) {
	lib, err := s.Store.Library(context.Background(), id)
	if err != nil {
		return
	}
	if _, ok := s.scanModule(lib.Type); ok {
		s.hostEvents().publish("scan.finished", map[string]int64{"library": id})
	}
}

// scanChoicesCache keeps what a module's scan.choices said, by its version: it doesn't change
// while the version doesn't.
type scanChoicesCache struct {
	mu   sync.Mutex
	kept map[string]wittymodule.ScanChoices
}

var choicesCache = &scanChoicesCache{kept: map[string]wittymodule.ScanChoices{}}

// scanChoices are the values a library of this kind can be held to (a console), as the module
// that scans it says; false without one, or when it couldn't say.
func (s *Server) scanChoices(ctx context.Context, kind string) (wittymodule.ScanChoices, bool) {
	mod, ok := s.scanModule(kind)
	if !ok || mod.Facet == "" {
		return wittymodule.ScanChoices{}, false
	}
	testing := s.testingGames()
	key := mod.ID + "|" + mod.Version + "|" + kind
	if testing {
		key += "|testing"
	}
	choicesCache.mu.Lock()
	c, ok := choicesCache.kept[key]
	choicesCache.mu.Unlock()
	if ok {
		return c, true
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := s.Scanner.Modules.Call(ctx, mod.ID, wittymodule.CallScanChoices, wittymodule.ScanChoicesArgs{LibraryKind: kind, Testing: testing}, &c); err != nil {
		s.Log.WarnContext(ctx, "a module couldn't say what a library can hold", "cat", "modules", "module", mod.ID, "err", err)
		return wittymodule.ScanChoices{}, false
	}
	choicesCache.mu.Lock()
	if len(choicesCache.kept) > 64 {
		clear(choicesCache.kept)
	}
	choicesCache.kept[key] = c
	choicesCache.mu.Unlock()
	return c, true
}

// validFacet says whether a library of this kind may be held to this value: "auto", or one of
// the module's choices.
func (s *Server) validFacet(ctx context.Context, kind, value string) bool {
	if value == wittymodule.ScanAuto {
		return true
	}
	c, ok := s.scanChoices(ctx, kind)
	return ok && slices.ContainsFunc(c.Values, func(v wittymodule.ScanChoice) bool { return v.ID == value })
}

// consoleShort is a console's short name (SNES, Mega Drive), so a card can draw a game in its
// console's case; its id when it has none.
func (s *Server) consoleShort(ctx context.Context, id string) string {
	if c, ok := s.scanChoices(ctx, "games"); ok {
		for _, v := range c.Values {
			if v.ID == id {
				if v.Short != "" {
					return v.Short
				}
				if v.Name != "" {
					return v.Name
				}
				break
			}
		}
	}
	return id
}

// scanModuleMissing refuses a library of a kind only a module scans while none that does is
// installed and on, the way any kind that needs its module is refused.
func (s *Server) scanModuleMissing(w http.ResponseWriter, kind string) bool {
	if library.OwnScan(kind) {
		return false
	}
	if _, ok := s.scanModule(kind); ok {
		return false
	}
	var mods []modmgr.ModuleRef
	if s.Modules != nil {
		for _, lt := range s.Modules.LibraryTypes() {
			if lt.Type == kind {
				mods = lt.Modules
			}
		}
	}
	if mods == nil {
		mods = []modmgr.ModuleRef{}
	}
	err := &modmgr.ErrModuleMissing{Type: kind, Modules: mods}
	writeJSON(w, 409, map[string]any{"error": err.Error(), "code": "module-missing", "modules": mods})
	return true
}

// testingGames: the module that scans games offers its test console too (a developer's server,
// or WMS_GAMING_TESTCORE=1).
func (s *Server) testingGames() bool {
	return (s.Cfg != nil && s.Cfg.Dev) || os.Getenv("WMS_GAMING_TESTCORE") == "1"
}
