// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/wittywilla/wittymodule"
)

// A catalogue copies, for each version, how finished it says it is, a summary of the SDUI it
// adds and, from module API 5, a summary of what its integration.json adds to a server
// (WittyModule's docs/SPEC.md), so a store can show them before anything is downloaded.
// wittymodule.ParseCatalog reads and checks them with the rest of the signed catalogue. They
// only describe: what a server does with a version goes by the signed files it downloads.

// StoreUI is a catalogue's summary of the SDUI a version adds.
type StoreUI struct {
	Pages      []UIPage `json:"pages"`
	Navigation []string `json:"navigation"`
	Settings   bool     `json:"settings,omitempty"`
	Sources    int      `json:"sources,omitempty"`
	Inserts    int      `json:"inserts,omitempty"`
}

// StoreIntegration is what a version adds to a server (its integration.json), for the store's
// "What this adds": from the catalogue before it is installed, from the installed copy after.
type StoreIntegration struct {
	// Routes is how many HTTP routes of its own it answers, under /api/v1/modules/<id>/.
	Routes int `json:"routes"`
	// RouteKinds says who may call them and what they do; known only for a copy on this
	// server, since a catalogue gives only the count.
	RouteKinds *RouteKinds `json:"routeKinds,omitempty"`
	// Screens are the web app's addresses it draws (react-router style, "/couch/:id").
	Screens []string `json:"screens"`
	// Settings are the sections it adds to Settings, by label.
	Settings []string `json:"settings"`
	// Setup are the steps it adds to the server's first-run setup, by title (installed copy only).
	Setup []string `json:"setup,omitempty"`
	// Data: it keeps a database of its own.
	Data bool `json:"data,omitempty"`
	// Feature is the parental and profile-limits feature it all belongs to, such as "games".
	Feature string `json:"feature,omitempty"`
	// Libraries are the kinds of library whose pages it draws (installed copy only; the store
	// falls back to the module's own list of library kinds).
	Libraries []string `json:"libraries,omitempty"`
}

// RouteKinds counts a version's routes by what they are. One route can be several.
type RouteKinds struct {
	User    int `json:"user"`    // anyone signed in may call it
	Admin   int `json:"admin"`   // administrators only
	Ticket  int `json:"ticket"`  // no sign-in: a secret in the address, answered in a sandboxed frame
	Items   int `json:"items"`   // about one library item, which the profile must be allowed to see
	Streams int `json:"streams"` // a stream or a WebSocket
	Plays   int `json:"plays"`   // starts playing something, so it counts towards screen time
}

// Limits a summary is held to, whoever wrote it: integration.json's own.
const (
	maxStoreScreens  = 64
	maxStoreSettings = 32
	maxStoreRoutes   = 1000
	maxStoreLabel    = 120
)

// catalogExtras is what a catalogue says of version v beyond the fields every server reads:
// its status ("" when it says none, which is stable) and note, and its UI and integration
// summaries, cleaned of anything a page could be fooled by.
func catalogExtras(v *wittymodule.CatalogVersion) (status, note string, ui *StoreUI, in *StoreIntegration) {
	if v.Status != "" {
		status = knownStatus(v.Status)
	}
	return status, clipTo(cleanText(v.StatusNote), 500), storeUI(v.UI), storeIntegration(v.Integration)
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

// storeUI holds a catalogue's UI summary to the limits sdui.json has, dropping what's past
// them rather than the whole summary.
func storeUI(u *wittymodule.CatalogUI) *StoreUI {
	if u == nil {
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

// storeIntegration holds a catalogue's integration summary to the limits integration.json has.
func storeIntegration(s *wittymodule.IntegrationSummary) *StoreIntegration {
	if s == nil {
		return nil
	}
	return &StoreIntegration{
		Routes:   max(0, min(s.Routes, maxStoreRoutes)),
		Screens:  labels(s.Screens, maxStoreScreens),
		Settings: labels(s.Settings, maxStoreSettings),
		Data:     s.Data,
		Feature:  clipTo(cleanText(s.Feature), 64),
	}
}

// integrationAdds sums up an installed copy's integration.json, which says more than a
// catalogue's summary: who may call its routes, its setup steps and the libraries it draws.
func integrationAdds(in *wittymodule.Integration) *StoreIntegration {
	if in == nil {
		return nil
	}
	s := in.Summary()
	out := storeIntegration(&s)
	k := &RouteKinds{}
	for _, r := range in.HTTP {
		switch r.Auth {
		case wittymodule.AuthAdmin:
			k.Admin++
		case wittymodule.AuthTicket:
			k.Ticket++
		default:
			k.User++
		}
		if r.Item != "" {
			k.Items++
		}
		if r.Stream || r.Upgrade != "" {
			k.Streams++
		}
		if r.Plays {
			k.Plays++
		}
	}
	if len(in.HTTP) > 0 {
		out.RouteKinds = k
	}
	var kinds []string
	if w := in.Web; w != nil {
		var setup []string
		for _, st := range w.Setup {
			setup = append(setup, cmp.Or(strings.TrimSpace(st.Title), st.ID))
		}
		out.Setup = labels(setup, maxStoreSettings)
		for _, t := range w.Takeover {
			kinds = append(kinds, t.LibraryKind)
		}
	}
	slices.Sort(kinds)
	out.Libraries = labels(slices.Compact(kinds), maxStoreSettings)
	return out
}

// labels cleans a list of short texts, keeping at most n that say something.
func labels(in []string, n int) []string {
	out := []string{}
	for _, s := range in {
		if t := clipTo(cleanText(s), maxStoreLabel); t != "" && len(out) < n {
			out = append(out, t)
		}
	}
	return out
}

func clipTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
