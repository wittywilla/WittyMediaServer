// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyplay

import (
	"context"
	"sync"

	"github.com/wittywilla/wittymodule/service"
)

// Written by hand: what holds state here, or takes a function.

const module = "wittyplay"

// Consoles lists the consoles the module has built in.
func Consoles() []Console {
	var out []Console
	_ = service.Call(context.Background(), module, "wittyplay.Consoles", nil, nil, nil, []any{&out})
	return out
}

var (
	customMu   sync.Mutex
	customList []Console
	customOnce sync.Once
)

// SetCustomConsoles gives the module the consoles someone made. It's kept here as well, since a
// service that stopped has forgotten them.
func SetCustomConsoles(list []Console) {
	customMu.Lock()
	customList = list
	customMu.Unlock()
	customOnce.Do(func() {
		service.OnStart(module, func(c *service.Conn) {
			customMu.Lock()
			l := customList
			customMu.Unlock()
			_ = c.Call(context.Background(), "wittyplay.SetCustomConsoles", nil, nil, []any{l}, nil)
		})
	})
	_ = service.Call(context.Background(), module, "wittyplay.SetCustomConsoles", nil, nil, []any{list}, nil)
}

// CleanProfile checks a profile. consoleOK says which consoles it may name.
func CleanProfile(p Profile, consoleOK func(string) bool) (Profile, error) {
	var known []string
	for _, c := range All() {
		if consoleOK == nil || consoleOK(c.ID) {
			known = append(known, c.ID)
		}
	}
	var out Profile
	err := service.Call(context.Background(), module, "wittyplay.CleanProfile", nil, nil, []any{p, known}, []any{&out})
	return out, err
}
