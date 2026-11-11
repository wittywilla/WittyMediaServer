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

// NewTopShelf builds Top Shelf content from rows of items (the same objects
// screens are given: id, title, imageUrl, shape, progress, route, playRoute),
// cut to the limits, with each picture asked for at the size the profile
// gives its shape. Items without a route are left out: there'd be nowhere to
// go. Rows that end up empty are left out too.
func NewTopShelf(rows []map[string]any, l *Layout, ttl int) TopShelf {
	if l == nil || l.FormFactor != FormTV {
		l = ProfileFor(FormTV, "hd")
	}
	out := TopShelf{Version: SchemaVersion, Style: "sectioned", Sections: []TopShelfSection{}, ExpiresInSec: ttl}
	for _, row := range rows {
		if len(out.Sections) >= MaxTopShelfSections {
			break
		}
		sec := TopShelfSection{Title: stringify(row["title"]), Items: []TopShelfItem{}}
		rowShape, _ := row["cardShape"].(string)
		for _, v := range anyList(row["items"]) {
			it, ok := v.(map[string]any)
			if !ok || len(sec.Items) >= MaxTopShelfItems {
				continue
			}
			route, _ := it["route"].(string)
			if !strings.HasPrefix(route, "/") {
				continue
			}
			shape, _ := it["shape"].(string)
			if shape == "" {
				shape = rowShape
			}
			if shape != "wide" && shape != "square" {
				shape = "poster"
			}
			item := TopShelfItem{ID: stringify(it["id"]), Title: stringify(it["title"]), Shape: shape, DisplayRoute: route}
			item.PlayRoute, _ = it["playRoute"].(string)
			if p, ok := toFloat(it["progress"]); ok && p > 0 && p < 1 {
				item.Progress = p
			}
			size := l.Images["topShelf"+strings.ToUpper(shape[:1])+shape[1:]]
			if u, _ := it["imageUrl"].(string); strings.HasPrefix(u, "/api/") && size.Width > 0 {
				sep := "?"
				if strings.Contains(u, "?") {
					sep = "&"
				}
				item.Image = &ImageRef{URL: u + sep + "w=" + strconv.Itoa(size.Width), Width: size.Width, Height: size.Height, Aspect: size.Aspect}
			}
			sec.Items = append(sec.Items, item)
		}
		if len(sec.Items) > 0 {
			out.Sections = append(out.Sections, sec)
		}
	}
	return out
}
