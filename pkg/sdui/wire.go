// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

// Wire is a look's layout as a picker draws it: a few words for each part of the screen, so a
// person sees the layout they are choosing and not only its colours. It is worked out from the
// experience's own data (its menu on the form factor, its page changes, its effects), so there
// is one source of truth, and it is what the design canvas's wire thumbnails draw.
type Wire struct {
	// Nav is where the menu is: top, side, rail, bottom or none (a crossbar's categories are
	// part of the page). Open: it is drawn open all the time (a sidebar). Style is how its
	// entries are drawn: pill, gloss, under, cable, bevel, tele or crossbar.
	Nav   string `json:"nav"`
	Open  bool   `json:"open,omitempty"`
	Style string `json:"style"`
	// Home is the shape of Home: hero (a billboard), banner, carousel, cross (a crossbar),
	// tiles, widgets or list.
	Home string `json:"home"`
	// Cards are the card style of films and shows: poster, landscape, plate, block or logo.
	Cards string `json:"cards"`
	// Dock: a dock of keys (Look, Sort, Filter ...) is drawn, as on a television or in a headset.
	Dock bool `json:"dock,omitempty"`
	// Brand is where the server's name sits: left of a top bar, at the top of a side menu or
	// tab bar, or in the centre (a crossbar's).
	Brand string `json:"brand"`
	// Finish is the surface: flat, gloss, bevel, glass or crt.
	Finish string `json:"finish"`
}

// WireFor is the experience as a picker draws it on a form factor.
func WireFor(e *Experience, formFactor string) Wire {
	nav := e.Nav[formFactor]
	if nav == "" {
		nav = e.Nav[FormWeb]
	}
	w := Wire{Style: "pill", Brand: "left", Finish: "flat"}
	switch nav {
	case NavSidebar:
		w.Nav, w.Open, w.Brand = "side", true, "top"
	case NavTVSideMenu:
		w.Nav, w.Brand = "side", "top"
	case NavRail, NavRailDrawer:
		w.Nav, w.Brand = "rail", "top"
	case NavBottomTabs, NavOrnament:
		w.Nav, w.Brand = "bottom", "top"
	case NavCrossbar:
		w.Nav, w.Style, w.Brand = "none", "crossbar", "centre"
	case NavFloatingGlass:
		w.Nav, w.Style = "top", "gloss"
	case NavCableBar:
		w.Nav, w.Style = "top", "cable"
	case NavBevelBar:
		w.Nav, w.Style = "top", "bevel"
	case NavTeleKeys:
		w.Nav, w.Style = "top", "tele"
	default: // topTabs, tvTopMenu
		w.Nav = "top"
		if e.Pages.SectionStyle == "underline" {
			w.Style = "under"
		}
	}
	switch {
	case e.Pages.Crossbar:
		w.Home = "cross"
	case e.Pages.Widgets:
		w.Home = "widgets"
	case e.Pages.RowsAsLists:
		w.Home = "list"
	case e.Home.Rows == "tiles":
		w.Home = "tiles"
	case e.Pages.Hero == "banner":
		w.Home = "banner"
	case e.Pages.Hero == "carousel":
		w.Home = "carousel"
	default:
		w.Home = "hero"
	}
	card := e.Pages.CardStyle
	if card == "" {
		card = e.Shape.CardSet
	}
	switch card {
	case "plate", "landscape", "block", "logo":
		w.Cards = card
	case "tile": // big picture's tiles are wide cards
		w.Cards = "landscape"
	default:
		w.Cards = "poster"
	}
	w.Dock = formFactor == FormTV || formFactor == FormHeadset
	switch {
	case nav == NavTeleKeys:
		w.Finish = "crt"
	case e.Effects.Gloss == "full":
		w.Finish = "gloss"
	case e.Effects.Bevel == "full":
		w.Finish = "bevel"
	case e.Effects.Glass == "full":
		w.Finish = "glass"
	}
	return w
}
