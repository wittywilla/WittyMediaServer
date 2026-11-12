// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package modtest gives a test real modules, from a catalogue on this machine: the one
// scripts/make-test-catalog.sh makes. The server has no module's code in it, so a test of what it
// does with a real one needs that catalogue, and is skipped without it.
//
//	WMS_TEST_CATALOG=<folder> go test ./...
package modtest

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/cd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/modules/bdremote"
	"github.com/wittywilla/wittymediaserver/internal/modules/dvdremote"
	"github.com/wittywilla/wittymodule/service"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/modules"
)

// Wire sets up what this package can't reach without going in a circle, as the server does at start.
var Wire func(*modules.Manager)

// EnvCatalog names the folder.
const EnvCatalog = "WMS_TEST_CATALOG"

var (
	mu      sync.Mutex
	manager *modules.Manager
	dir     string
	added   = map[string]error{}
)

func start(catalog string) (*modules.Manager, error) {
	if manager != nil {
		return manager, nil
	}
	pub, err := os.ReadFile(filepath.Join(catalog, "test-key.pub"))
	if err != nil {
		return nil, fmt.Errorf("the test catalogue has no key: %w", err)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(pub)))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("the test catalogue's key is unreadable: %v", err)
	}
	if dir, err = os.MkdirTemp("", "wms-modtest-*"); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(catalog)
	if err != nil {
		return nil, err
	}
	manager = modules.New(modules.Config{Dir: dir, Server: "0.6.0-alpha", Starter: abs, Keys: []ed25519.PublicKey{ed25519.PublicKey(key)},
		Log: slog.New(slog.DiscardHandler), WriteFolders: func(string) []string { return []string{os.TempDir()} }})
	manager.UseServices()
	cd.Open = modules.CDs{M: manager}.Open
	dvdsession.SetEngine(dvdremote.Opener{M: manager, Log: slog.New(slog.DiscardHandler)}.Open)
	dvdsession.SetAnalyzer(dvdremote.Opener{M: manager, Log: slog.New(slog.DiscardHandler)}.Analyze)
	bdremote.Use(manager)
	if Wire != nil {
		Wire(manager)
	}
	return manager, nil
}

// Need installs modules for a test from the catalogue, or skips the test if there isn't one.
func Need(t testing.TB, ids ...string) *modules.Manager {
	t.Helper()
	catalog := os.Getenv(EnvCatalog)
	if catalog == "" {
		t.Skipf("this needs real modules: set %s to a catalogue from scripts/make-test-catalog.sh", EnvCatalog)
	}
	mu.Lock()
	defer mu.Unlock()
	m, err := start(catalog)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err, done := added[id]; done {
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		err := add(m, id)
		added[id] = err
		if err != nil {
			t.Fatal(err)
		}
		if id == "wittyplay" {
			t.Cleanup(stopGames)
		}
	}
	return m
}

// stopGames leaves no game running: there's one set of them, for every test.
func stopGames() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_ = service.Call(ctx, "wittyplay", "stream.Manager.StopAll", nil, nil, nil, nil)
}

func add(m *modules.Manager, id string) error {
	if err := m.AddModule(id); err != nil {
		return fmt.Errorf("adding %s: %w", id, err)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		for _, st := range m.Report().Modules {
			if st.ID != id {
				continue
			}
			if st.Enabled {
				return nil
			}
			if st.Phase == modules.PhaseFailed {
				return fmt.Errorf("adding %s: %s", id, st.Error)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("adding %s took too long", id)
}

// Runner is the path of WittyPlay's game runner, from the catalogue.
func Runner(t testing.TB) string {
	t.Helper()
	m := Need(t, "wittyplay")
	p, _, ok := m.Program("wittyplay")
	if !ok {
		t.Fatal("WittyPlay has no game runner")
	}
	return p
}
