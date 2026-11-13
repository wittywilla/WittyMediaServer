// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import (
	"fmt"
	"strconv"
	"strings"
)

// How an Item becomes a Card. Nobody writes field paths for this: each card
// style knows which of an item's fields it shows, and these rules are the
// whole of the binding. A source's "map" overrides title, subtitle or
// synopsis with another field for the unusual case.

// MapFields are the item fields a source's "map" may put into a card's
// title, subtitle or synopsis.
var MapFields = []string{"title", "subtitle", "overview", "tagline", "year", "studio", "contentRating", "artist", "showTitle", "count", "number", "runtime", "genres"}

// MapProps are the card props a source's "map" may set.
var MapProps = []string{"title", "subtitle", "synopsis"}

// itemField reads one of MapFields from an item as text.
func itemField(it *Item, f string) string {
	switch f {
	case "title":
		return it.Title
	case "subtitle":
		return it.Subtitle
	case "overview":
		return it.Overview
	case "tagline":
		return it.Tagline
	case "year":
		if it.Year > 0 {
			return strconv.Itoa(it.Year)
		}
	case "studio":
		return it.Studio
	case "contentRating":
		return it.ContentRating
	case "artist":
		return it.Artist
	case "showTitle":
		return it.ShowTitle
	case "count":
		return countText(it)
	case "number":
		return it.Number
	case "runtime":
		return FormatRuntime(it.RuntimeSec)
	case "genres":
		return strings.Join(it.Genres[:min(len(it.Genres), 3)], ", ")
	}
	return ""
}

// FormatRuntime writes a length in seconds the way cards and facts show it:
// "45 sec", "42 min", "2 hr", "1 hr 58 min". Minutes are rounded to the
// nearest. Zero and negative lengths are "".
func FormatRuntime(sec float64) string {
	if sec <= 0 {
		return ""
	}
	if sec < 59.5 {
		return strconv.Itoa(max(1, int(sec+0.5))) + " sec"
	}
	m := int(sec/60 + 0.5)
	if m < 60 {
		return strconv.Itoa(m) + " min"
	}
	if m%60 == 0 {
		return strconv.Itoa(m/60) + " hr"
	}
	return fmt.Sprintf("%d hr %d min", m/60, m%60)
}

// firstImage is the first of an item's pictures among kinds.
func firstImage(it *Item, kinds []string) string {
	for _, k := range kinds {
		if u := it.Images[k]; u != "" {
			return u
		}
	}
	return ""
}

// CardTitle and CardSubtitle are what a card of a style shows for an item
// before any "map": the binding rules, one place.
//
//   - episode: "<episode>. <title>", and the running time under it;
//   - progress: an episode shows its show's name with "S<s> E<e> · <title>"
//     under it; anything else its own title and subtitle;
//   - every other style: the item's title and subtitle.
func CardTitle(style string, it *Item) string {
	switch style {
	case "episode":
		if it.Episode > 0 {
			return strconv.Itoa(it.Episode) + ". " + it.Title
		}
	case "progress":
		if it.Kind == "episode" && it.ShowTitle != "" {
			return it.ShowTitle
		}
	}
	return it.Title
}

func CardSubtitle(style string, it *Item) string {
	switch style {
	case "episode":
		return FormatRuntime(it.RuntimeSec)
	case "progress":
		if it.Kind == "episode" && it.ShowTitle != "" {
			return fmt.Sprintf("S%d E%d · %s", it.Season, it.Episode, it.Title)
		}
	case "circle", "chip", "photo", "tile":
		return ""
	}
	return it.Subtitle
}

// Remaining is the "42 min left" of a progress card, "" when it isn't known.
func Remaining(it *Item) string {
	if it.RuntimeSec <= 0 || it.Progress <= 0 || it.Progress >= 1 {
		return ""
	}
	left := FormatRuntime(it.RuntimeSec * (1 - it.Progress))
	if left == "" {
		return ""
	}
	return left + " left"
}

// card makes the Card of one item in one style.
func (r *renderer) card(parent, style, size string, it *Item, src Source) Node {
	spec, ok := CardStyle(style)
	if !ok {
		spec, _ = CardStyle("poster")
		style = "poster"
	}
	lim := r.l.TextLimits
	title, subtitle := CardTitle(style, it), CardSubtitle(style, it)
	synopsis := ""
	if contains(spec.Shows, "synopsis") {
		synopsis = it.Overview
	}
	if m, _ := src["map"].(map[string]any); m != nil {
		for prop, f := range m {
			field, _ := f.(string)
			switch prop {
			case "title":
				title = firstNonEmpty(itemField(it, field), title)
			case "subtitle":
				subtitle = itemField(it, field)
			case "synopsis":
				synopsis = itemField(it, field)
			}
		}
	}
	p := map[string]any{"style": style, "shape": spec.Shape, "title": Truncate(title, lim.Title), "itemId": it.ID, "kind": it.Kind}
	if subtitle != "" && (spec.SubtitleLines > 0 || src["map"] != nil) {
		p["subtitle"] = Truncate(subtitle, lim.Subtitle)
	}
	if synopsis != "" {
		p["synopsis"] = Truncate(synopsis, lim.Synopsis)
	}
	shows := func(f string) bool { return contains(spec.Shows, f) }
	if shows("progress") && it.Progress > 0 && it.Progress < 1 && it.Now == nil {
		p["progress"] = round3(it.Progress)
	}
	if shows("remaining") {
		if rem := Remaining(it); rem != "" {
			p["remaining"] = rem
		}
	}
	if shows("watched") && it.Watched {
		p["watched"] = true
	}
	if shows("unwatched") && it.Unwatched > 0 {
		p["unwatched"] = it.Unwatched
	}
	if shows("badges") && len(it.Badges) > 0 {
		p["badges"] = it.Badges[:min(len(it.Badges), 3)]
	}
	if shows("count") && it.Count > 0 {
		p["count"] = it.Count
	}
	if shows("number") && it.Number != "" {
		p["number"] = it.Number
	}
	if shows("now") && it.Now != nil {
		p["now"] = programme(it.Now, lim.Title)
		if it.Now.End > it.Now.Start {
			now := r.ctx.Now.Unix()
			if f := float64(now-it.Now.Start) / float64(it.Now.End-it.Now.Start); f > 0 && f < 1 {
				p["progress"] = round3(f)
			}
		}
	}
	if shows("next") && it.Next != nil {
		p["next"] = programme(it.Next, lim.Title)
	}
	m := r.l.Cards[style]
	if srcURL := firstImage(it, spec.Image); srcURL != "" && spec.Aspect != "" {
		kind := spec.Image[0]
		for _, k := range spec.Image {
			if it.Images[k] == srcURL {
				kind = k
				break
			}
		}
		p["image"] = r.picture(srcURL, m.Width[size], spec.Aspect, kind)
	} else if it.Icon != "" {
		p["icon"] = it.Icon
	}
	n := Node{Type: "Card", ID: r.uniqueID(parent + ":" + safeID(it.ID)), Props: p}
	if it.Route != "" {
		n.Action = &Action{Type: ActNavigate, To: it.Route}
	}
	if it.Action != nil {
		n.Action = it.Action
	}
	if it.Console != "" {
		p["console"] = it.Console
	}
	if acts := cleanActions(it.Actions); len(acts) > 0 {
		p["actions"] = acts
	}
	if c := cleanControl(it.Control); c != nil {
		p["control"] = c
	}
	if it.PlayID != "" && contains(it.Can, "play") {
		p["playId"] = it.PlayID
	}
	if it.Scope != "" {
		r.scope(&n, it.Scope)
	}
	return n
}

func programme(pg *Programme, limit int) map[string]any {
	m := map[string]any{"title": Truncate(pg.Title, limit)}
	if pg.Start > 0 {
		m["start"] = pg.Start
	}
	if pg.End > 0 {
		m["end"] = pg.End
	}
	return m
}

// cleanActions are an item's detail-panel buttons as a card carries them: at most four, each with a
// label and an action, and a glyph only of the pad's four keys.
func cleanActions(in []ItemAction) []ItemAction {
	var out []ItemAction
	for _, a := range in {
		if a.Action == nil || a.Label == "" || len(out) == 4 {
			continue
		}
		if !contains(Glyphs, a.Glyph) {
			a.Glyph = ""
		}
		out = append(out, a)
	}
	return out
}
