// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"crypto/ed25519"
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"unicode/utf8"
	"weak"

	"github.com/wittywilla/wittymodule"
)

// A catalogue for module API 4 copies each version's status and a summary of the SDUI it adds
// (WittyModule's docs/SPEC.md, sections 4 and 18), so a store can show them before anything is
// downloaded. They are read from the same signed bytes as the rest of the catalogue. They only
// describe: what a server does with a version goes by the signed manifest it downloads.

// StoreUI is a catalogue's summary of the SDUI a version adds.
type StoreUI struct {
	Pages      []UIPage `json:"pages"`
	Navigation []string `json:"navigation"`
	Settings   bool     `json:"settings,omitempty"`
	Sources    int      `json:"sources,omitempty"`
	Inserts    int      `json:"inserts,omitempty"`
}

// versionExtra is what a catalogue says of one version beyond what this server's copy of the
// catalogue format reads.
type versionExtra struct {
	Status     string
	StatusNote string
	UI         *StoreUI
}

type catalogExtras map[string]map[string]versionExtra // module ID, version

var (
	extrasMu sync.Mutex
	extras   = map[weak.Pointer[wittymodule.Catalog]]catalogExtras{} // weak: a catalogue replaced takes its extras with it
)

// verifyCatalog is wittymodule.VerifyCatalog, keeping the fields it doesn't read for the store.
func verifyCatalog(raw, sig []byte, keys []ed25519.PublicKey) (*wittymodule.Catalog, error) {
	c, err := wittymodule.VerifyCatalog(raw, sig, keys)
	if err != nil {
		return nil, err
	}
	if x := parseExtras(raw); len(x) > 0 {
		key := weak.Make(c)
		extrasMu.Lock()
		extras[key] = x
		extrasMu.Unlock()
		runtime.AddCleanup(c, func(key weak.Pointer[wittymodule.Catalog]) {
			extrasMu.Lock()
			delete(extras, key)
			extrasMu.Unlock()
		}, key)
	}
	return c, nil
}

// extraOf is what c says of a version beyond the fields this server's catalogue format reads.
func extraOf(c *wittymodule.Catalog, id, version string) versionExtra {
	if c == nil {
		return versionExtra{}
	}
	extrasMu.Lock()
	defer extrasMu.Unlock()
	return extras[weak.Make(c)][id][version]
}

func parseExtras(raw []byte) catalogExtras {
	var doc struct {
		Modules []struct {
			ID       string `json:"id"`
			Versions []struct {
				Version    string          `json:"version"`
				Status     string          `json:"status"`
				StatusNote string          `json:"statusNote"`
				UI         json.RawMessage `json:"ui"`
			} `json:"versions"`
		} `json:"modules"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	out := catalogExtras{}
	for _, m := range doc.Modules {
		for _, v := range m.Versions {
			x := versionExtra{Status: knownStatus(v.Status), StatusNote: clipTo(cleanText(v.StatusNote), 500)}
			if x.Status == StatusStable && v.Status == "" {
				x.Status = ""
			}
			x.UI = parseStoreUI(v.UI)
			if x == (versionExtra{}) {
				continue
			}
			if out[m.ID] == nil {
				out[m.ID] = map[string]versionExtra{}
			}
			out[m.ID][v.Version] = x
		}
	}
	return out
}

// knownStatus is a status this server knows; anything else counts as the least finished, since
// a version saying something this server can't read shouldn't look complete.
func knownStatus(s string) string {
	switch s {
	case "", StatusStable:
		return StatusStable
	case StatusBeta, StatusExperimental, StatusIncomplete:
		return s
	}
	return StatusIncomplete
}

// parseStoreUI reads a version's UI summary within the limits the format sets, dropping what's
// past them rather than the whole summary.
func parseStoreUI(raw json.RawMessage) *StoreUI {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var u StoreUI
	if json.Unmarshal(raw, &u) != nil {
		return nil
	}
	ui := &StoreUI{Pages: []UIPage{}, Navigation: []string{}, Settings: u.Settings, Sources: max(0, min(u.Sources, 64)), Inserts: max(0, min(u.Inserts, 32))}
	for _, p := range u.Pages {
		if t := clipTo(cleanText(p.Title), 120); t != "" && len(ui.Pages) < 32 {
			ui.Pages = append(ui.Pages, UIPage{ID: clipTo(p.ID, 64), Title: t})
		}
	}
	for _, n := range u.Navigation {
		if l := clipTo(cleanText(n), 40); l != "" && len(ui.Navigation) < 16 {
			ui.Navigation = append(ui.Navigation, l)
		}
	}
	return ui
}

func clipTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
