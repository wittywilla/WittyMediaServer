// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// Console photos: a picture of each console's hardware for WittyPlay's Consoles tab, from Wikimedia Commons by way of the
// console's Wikidata entity (its "image", P18). Wikidata is CC0; Commons states each file's licence and author, and only files
// whose licence allows this use are taken (FreeLogoLicense), with the author and licence kept to be shown beside the picture.

// consoleWikidata is each console's Wikidata entity, by the console ids of WittyPlay's tables (consoleLogoNames has the
// same ids). A console missing here is looked for by its name; an entity whose description isn't a console's is never used.
var consoleWikidata = map[string]string{
	"nes": "Q172742", "snes": "Q183259", "n64": "Q184839", "gc": "Q182172", "wii": "Q8079",
	"gb": "Q186437", "gbc": "Q203992", "gba": "Q188642", "nds": "Q170323", "3ds": "Q203597", "vb": "Q164651",
	"ps1": "Q10677", "ps2": "Q10680", "psp": "Q170325",
	"md": "Q10676", "sms": "Q209868", "gg": "Q751719", "saturn": "Q200912", "dc": "Q184198",
	"a2600": "Q206261", "lynx": "Q753600", "pce": "Q1057377",
}

// ConsoleWikidataID is a console's Wikidata entity, or "".
func ConsoleWikidataID(console string) string { return consoleWikidata[console] }

// ErrNoPhoto: Commons has no picture of the console that may be used.
var ErrNoPhoto = errors.New("no free picture of this console")

// ConsolePhoto finds the picture of a console: its Wikidata entity's image, with Commons' licence and author. name is used
// to search when the console has no entity in the table.
func (w *WikimediaLogos) ConsolePhoto(ctx context.Context, console, name string) (LogoCandidate, error) {
	apiBase := w.WikidataAPI
	if apiBase == "" {
		apiBase = wikidataAPI
	}
	ids := []string{}
	if q := consoleWikidata[console]; q != "" {
		ids = append(ids, q)
	} else if name != "" {
		var search struct {
			Search []struct {
				ID string `json:"id"`
			} `json:"search"`
		}
		if err := w.get(ctx, apiBase, url.Values{"action": {"wbsearchentities"}, "search": {name}, "language": {"en"}, "type": {"item"}, "limit": {"6"}}, &search); err != nil {
			return LogoCandidate{}, err
		}
		for _, r := range search.Search {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return LogoCandidate{}, ErrNoPhoto
	}
	var ents struct {
		Entities map[string]struct {
			Labels       map[string]struct{ Value string }   `json:"labels"`
			Aliases      map[string][]struct{ Value string } `json:"aliases"`
			Descriptions map[string]struct{ Value string }   `json:"descriptions"`
			Claims       map[string][]logoClaim              `json:"claims"`
		} `json:"entities"`
	}
	if err := w.get(ctx, apiBase, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids, "|")}, "props": {"labels|aliases|descriptions|claims"}, "languages": {"en"}}, &ents); err != nil {
		return LogoCandidate{}, err
	}
	key := BrandKey(name)
	for _, id := range ids {
		e, ok := ents.Entities[id]
		if !ok {
			continue
		}
		desc := strings.ToLower(e.Descriptions["en"].Value)
		if containsAny(desc, logoNotWords...) || !containsAny(desc, logoKindWords[LogoConsole]...) {
			continue
		}
		// Found by a search, the entity must be called what was asked.
		if consoleWikidata[console] == "" {
			exact := BrandKey(e.Labels["en"].Value) == key
			for _, a := range e.Aliases["en"] {
				exact = exact || BrandKey(a.Value) == key
			}
			if !exact {
				continue
			}
		}
		file := bestLogoFile(e.Claims[wdImage])
		if file == "" {
			continue
		}
		info, err := w.fileInfo(ctx, []string{"File:" + file})
		if err != nil {
			return LogoCandidate{}, err
		}
		if c, ok := info[normalFileTitle("File:"+file)]; ok {
			c.Name, c.Note, c.Exact = e.Labels["en"].Value, e.Descriptions["en"].Value, true
			return c, nil
		}
	}
	return LogoCandidate{}, ErrNoPhoto
}
