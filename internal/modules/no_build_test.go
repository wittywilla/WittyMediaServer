// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// A module with no build for this system says so in plain words, where a button would otherwise
// lead to a failure, and so does a module built on it.
func TestNoBuildForThisSystemIsSaidPlainly(t *testing.T) {
	w := newWorld(t)
	m := w.manager()
	defer m.Close()
	other := "freebsd-arm64"
	if runtime.GOOS == "freebsd" {
		other = "linux-amd64"
	}
	version := func(platform string) []wittymodule.CatalogVersion {
		return []wittymodule.CatalogVersion{{Version: "1.0.0", Delivery: wittymodule.DeliveryHelper, API: wittymodule.API, MinServer: "0.6.0-alpha", MaxServer: "0.x",
			Sums: "a-SHA256SUMS.txt", Sig: "a-SHA256SUMS.txt.sig", Manifest: "module.json",
			Platforms: map[string]wittymodule.CatalogFile{platform: {URL: "a.tar.gz", SHA256: strings.Repeat("0", 64), Size: 1}}}}
	}
	m.mu.Lock()
	m.applyCatalog(&wittymodule.Catalog{Modules: []wittymodule.CatalogModule{
		{ID: "wittyelsewhere", Name: "WittyElsewhere", Repo: "someone/WittyElsewhere", Versions: version(other)},
		{ID: "wittyabove", Name: "WittyAbove", Repo: "someone/WittyAbove", Requires: []string{"wittyelsewhere"}, Versions: version(runtime.GOOS + "-" + runtime.GOARCH)},
	}}, "https://catalog.example/catalog.json", nil)
	m.mu.Unlock()

	var none *ErrNoVersion
	err := m.AddModule("wittyelsewhere")
	if !errors.As(err, &none) || !strings.Contains(err.Error(), "WittyElsewhere has no build for ") {
		t.Errorf("adding it: %v", err)
	}
	if err := m.AddModule("wittyabove"); !errors.As(err, &none) || !strings.Contains(err.Error(), "no build for ") {
		t.Errorf("adding what's built on it: %v", err)
	}
	said := map[string]string{}
	for _, s := range m.Report().Modules {
		said[s.ID] = s.Unavailable
	}
	if !strings.Contains(said["wittyelsewhere"], "has no build for ") {
		t.Errorf("the page isn't told: %q", said["wittyelsewhere"])
	}
	if !strings.Contains(said["wittyabove"], "is built on WittyElsewhere, which has no build for ") {
		t.Errorf("the page isn't told about what it's built on: %q", said["wittyabove"])
	}
}

func TestSystemNames(t *testing.T) {
	for _, c := range [][3]string{{"freebsd", "arm64", "FreeBSD on arm64"}, {"darwin", "amd64", "macOS on amd64"}, {"plan9", "386", "plan9 on 386"}} {
		if got := systemName(c[0], c[1]); got != c[2] {
			t.Errorf("%q, want %q", got, c[2])
		}
	}
}
