// Package modtest gives a test real modules, from a catalogue on this machine: the one
// scripts/make-test-catalog.sh makes. The server has no module's code in it, so a test of what it
// does with a real one needs that catalogue, and is skipped without it.
//
//	WMS_TEST_CATALOG=<folder> go test ./...
package modtest

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
)

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
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	_ = exe
	abs, err := filepath.Abs(catalog)
	if err != nil {
		return nil, err
	}
	manager = modules.New(modules.Config{Dir: dir, Server: "0.6.0-alpha", Starter: abs, Keys: []ed25519.PublicKey{ed25519.PublicKey(key)},
		Log: slog.New(slog.DiscardHandler), WriteFolders: func(string) []string { return []string{os.TempDir()} }})
	manager.UseServices()
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
	}
	return m
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

var _ = wittymodule.API
