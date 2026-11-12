// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package sdui is WittyMediaServer's server-driven UI: the model an
// administrator writes pages in, the catalogue that describes it, the
// validator, and the renderer that turns one page into the tree of
// components one profile's app draws on one device.
//
// A Page is an ordered list of Sections. Each section is a component from the
// catalogue (Hero, Shelf, Grid, DetailHeader, SeasonList, ...), most of them
// filled from a data Source ({"type": "recentlyAdded", "library": 3}) that a
// Resolver answers. Conditions, variants, repeats and a profile's own
// arrangement decide which sections a request gets, and a device profile
// (Layout) decides how much of each.
//
//	issues := sdui.ValidatePage(&page)          // what is wrong, and how to fix it
//	doc, err := sdui.Render(page, &sdui.RenderContext{
//		Profile: sdui.ProfileFor(sdui.FormTV, "4k"), ProfileKind: "adult",
//		Resolver: myResolver,                    // where the items come from
//	})
//	patches := sdui.Diff(previous, doc)          // for a client holding an older render
//
// The package is pure Go and standard library only. It knows nothing about
// HTTP, storage or the media library: the server supplies a Resolver, and the
// conformance kit (package conformance) supplies one made of canned data.
//
// docs/SDUI.md is the contract and the reference; its reference section is
// written from Catalog by a test, as is docs/sdui.schema.json from the types.
//
// # Versioning
//
// SchemaVersion is the wire version of what a client is sent; it changes only
// for something a client built for it could not survive. CatalogRevision
// counts changes to the catalogue within it. Every renderer draws every component
// of the revision: there is no negotiation and no fallback.
package sdui
