// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// syncBuffer is a log a test can read while the manager writes to it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// A fresh server has read no catalogue yet: its modules come from the list it was built with,
// whose pictures have addresses relative to the official catalogue. They're fetched from beside
// it, not from a bare path that's refused, and a picture that can't be fetched is logged.
func TestAFreshServerFetchesIconsFromBesideTheCatalogue(t *testing.T) {
	const catalog = "https://github.com/wittywilla/WittyModuleCatalog/releases/download/catalog/catalog.json"
	var pic bytes.Buffer
	if err := png.Encode(&pic, image.NewGray(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	gh := &github{files: map[string][]byte{}}
	gh.set("/wittywilla/WittyModuleCatalog/releases/download/catalog/test-icon.png", pic.Bytes())
	logs := &syncBuffer{}
	dir := t.TempDir() + "/modules" // no folder yet, as on a fresh data folder
	m := New(Config{Dir: dir, Server: "0.8.0", Known: true, CatalogURL: catalog,
		Fetch: &update.Fetcher{UserAgent: "test", Transport: gh}, Log: slog.New(slog.NewTextHandler(logs, nil))})
	defer m.Close()

	m.mu.Lock()
	if m.cat != nil {
		m.mu.Unlock()
		t.Fatal("a fresh server shouldn't have a catalogue yet")
	}
	var id string
	for _, k := range m.order {
		if e := m.mods[k]; e.listed != nil && e.listed.Icon != nil {
			id = k
			// The test's own picture, in place of the real one the built-in list names.
			e.listed.Icon = &wittymodule.CatalogFile{URL: "test-icon.png", Size: int64(pic.Len()), SHA256: sha(pic.Bytes())}
			break
		}
	}
	m.mu.Unlock()
	if id == "" {
		t.Fatal("the built-in list names no module with a picture")
	}

	p, err := m.IconFile(context.Background(), id)
	if err != nil {
		t.Fatalf("the picture wasn't fetched: %v", err)
	}
	if b, err := os.ReadFile(p); err != nil || !bytes.Equal(b, pic.Bytes()) {
		t.Fatalf("the kept picture isn't the one fetched: %v", err)
	}

	// No picture at all isn't worth a line in the log.
	if _, err := m.IconFile(context.Background(), "no-such-module"); !errors.Is(err, ErrUnknown) {
		t.Fatalf("a module with no picture: %v", err)
	}
	if strings.Contains(logs.String(), "picture") {
		t.Fatalf("nothing should be logged yet:\n%s", logs)
	}

	// One that can't be fetched is logged, once.
	m.mu.Lock()
	m.mods[id].listed.Icon = &wittymodule.CatalogFile{URL: "missing.png", Size: 10, SHA256: strings.Repeat("0", 64)}
	m.mu.Unlock()
	for range 2 {
		if _, err := m.IconFile(context.Background(), id); err == nil || errors.Is(err, ErrUnknown) {
			t.Fatalf("a missing picture: %v", err)
		}
	}
	out := logs.String()
	if n := strings.Count(out, "couldn't be fetched"); n != 1 || !strings.Contains(out, "level=WARN") || !strings.Contains(out, "cat=modules") || !strings.Contains(out, "module="+id) {
		t.Fatalf("expected one warning about %s, got:\n%s", id, out)
	}
}
