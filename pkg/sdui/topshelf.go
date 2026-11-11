// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"strconv"
	"strings"
)

// Top Shelf is the strip tvOS shows above the Home screen's top row of apps
// when this app is focused there. It isn't a screen: the app's Top Shelf
// extension is asked for content by the system, has a few seconds and little
// memory to answer, and can show only titled sections of pictures that each
// open the app at one place. So it gets its own small document.

const (
	// MaxTopShelfSections and MaxTopShelfItems bound what is sent: the
	// extension is killed if it takes long, and nobody scrolls far there.
	MaxTopShelfSections = 4
	MaxTopShelfItems    = 10
)

// TopShelf is the content for a tvOS Top Shelf extension, in the "sectioned"
// style (TVTopShelfSectionedContent).
type TopShelf struct {
	// Version is SchemaVersion.
	Version int `json:"version"`
	// Style is "sectioned".
	Style    string            `json:"style"`
	Sections []TopShelfSection `json:"sections"`
	// ExpiresInSec is how long the extension may show this before asking again.
	ExpiresInSec int `json:"expiresInSec"`
}

// TopShelfSection is one titled row.
type TopShelfSection struct {
	Title string         `json:"title"`
	Items []TopShelfItem `json:"items"`
}

// TopShelfItem is one picture. tvOS opens the app with DisplayRoute when it
// is selected and PlayRoute when Play is pressed on it; both are routes in
// the app (the same ones Navigate actions use), which the app wraps in its
// own URL scheme.
type TopShelfItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Shape is poster (2:3), wide (16:9) or square (1:1): the three tvOS has.
	Shape string `json:"shape"`
	// Image is the picture at the size tvOS shows that shape.
	Image *ImageRef `json:"image,omitempty"`
	// Progress is 0..1 for something partly played, shown as a bar.
	Progress     float64 `json:"progress,omitempty"`
	DisplayRoute string  `json:"displayRoute"`
	PlayRoute    string  `json:"playRoute,omitempty"`
}

// ImageRef is a picture and the size to show it at, in pixels.
type ImageRef struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Aspect string `json:"aspect"`
}

// TopShelfRow is one row of items for NewTopShelf.
type TopShelfRow struct {
	Title string
	Items []Item
}

// NewTopShelf builds Top Shelf content from rows of items, cut to the limits,
// each picture asked for at the size tvOS wants for its shape: square for
// music, wide for a title part-way through, poster for the rest. Items
// without a route are left out: there'd be nowhere to go. Rows that end up
// empty are left out too.
func NewTopShelf(rows []TopShelfRow, l *Layout, ttl int) TopShelf {
	scale := 1
	if l != nil && l.FormFactor == FormTV {
		scale = max(l.Scale, 1)
	}
	// What tvOS asks of Top Shelf pictures (TVTopShelfSectionedItem's shapes), in points.
	sizes := map[string][3]any{"poster": {404, 608, "2/3"}, "wide": {908, 512, "16/9"}, "square": {608, 608, "1/1"}}
	out := TopShelf{Version: SchemaVersion, Style: "sectioned", Sections: []TopShelfSection{}, ExpiresInSec: ttl}
	for _, row := range rows {
		if len(out.Sections) >= MaxTopShelfSections {
			break
		}
		sec := TopShelfSection{Title: row.Title, Items: []TopShelfItem{}}
		for i := range row.Items {
			it := &row.Items[i]
			if len(sec.Items) >= MaxTopShelfItems {
				break
			}
			if !strings.HasPrefix(it.Route, "/") {
				continue
			}
			shape, kinds := "poster", []string{"poster"}
			switch it.Kind {
			case "album", "track", "artist", "podcast", "audiobook":
				shape, kinds = "square", []string{"square", "poster"}
			case "episode", "video", "channel", "recording":
				shape, kinds = "wide", []string{"backdrop", "poster"}
			}
			item := TopShelfItem{ID: it.ID, Title: it.Title, Shape: shape, DisplayRoute: it.Route}
			if it.PlayID != "" {
				item.PlayRoute = "/watch/" + it.PlayID
			}
			if it.Progress > 0 && it.Progress < 1 {
				item.Progress = round3(it.Progress)
			}
			size := sizes[shape]
			w, h := size[0].(int)*scale, size[1].(int)*scale
			if u := firstImage(it, kinds); strings.HasPrefix(u, "/api/") {
				sep := "?"
				if strings.Contains(u, "?") {
					sep = "&"
				}
				item.Image = &ImageRef{URL: u + sep + "w=" + strconv.Itoa(w), Width: w, Height: h, Aspect: size[2].(string)}
			}
			sec.Items = append(sec.Items, item)
		}
		if len(sec.Items) > 0 {
			out.Sections = append(out.Sections, sec)
		}
	}
	return out
}
