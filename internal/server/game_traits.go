// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"strings"

	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// What every client needs to know about a console to lay out its games and build its game menu
// (docs/GAMING-API.md, Consoles): its era and years for the shelves and the Eras page, how its
// games come (cartridge, handheld cartridge, disc), and its capabilities: screens, input, media,
// play and picture. A client builds its menus from these and leaves out what a console doesn't
// have, so a new console needs a row here, not new screens.
//
// Port counts, accessories and options are what the usual core for a console offers. The session
// says what the running core really has (its discs and its controller types).

// Eras, oldest first. Clients name them.
const (
	eraFirst   = "first"    // before the 8-bit consoles
	era8Bit    = "8-bit"    // NES, Master System
	era16Bit   = "16-bit"   // SNES, Mega Drive
	eraEarly3D = "early-3d" // the first consoles drawn in 3D
	eraDisc    = "disc"     // the disc consoles after them
	eraPocket  = "pocket"   // handhelds, of any year
)

// How a console's games come, which sets the shape of their spines on a shelf.
const (
	mediaCart   = "cart"
	mediaPocket = "pocket"
	mediaDisc   = "disc"
)

// ConsoleCapabilities is everything a game menu, a player and a controller screen build from.
type ConsoleCapabilities struct {
	Screens ScreenCaps  `json:"screens"`
	Input   InputCaps   `json:"input"`
	Media   MediaCaps   `json:"media"`
	Play    PlayCaps    `json:"play"`
	Picture PictureCaps `json:"picture"`
}

// ScreenCaps: one screen, or two (DS, 3DS), and how two can be arranged.
type ScreenCaps struct {
	Count int `json:"count"`
	// Touch is the screen that takes touch, counted from 1, or 0 for none.
	Touch int `json:"touch,omitempty"`
	// Layouts a client can offer, in order: stacked, side, big (one big, the other small),
	// single (one at a time).
	Layouts []string `json:"layouts,omitempty"`
	// Options are how each core sets those layouts, by core ID.
	Options map[string]ScreenOptions `json:"options,omitempty"`
	// Frame is where each screen is in a frame of the stacked layout the core is started in, as
	// fractions of the frame, so a client can draw each where the person placed it (Edit layout).
	Frame []ScreenRect `json:"frame,omitempty"`
}

// ScreenRect is one screen's part of a frame.
type ScreenRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// ScreenOptions: the core option that picks a layout, and the value for each of ours, as it is and
// with the two screens swapped. A core that swaps with an option of its own names that too.
type ScreenOptions struct {
	Layout     string               `json:"layout"`
	Values     map[string][2]string `json:"values"`
	Swap       string               `json:"swap,omitempty"`
	SwapValues *[2]string           `json:"swapValues,omitempty"`
}

// InputCaps: what a player can hold. The console's own buttons are in its pad
// (GET /consoles/{id}/controls).
type InputCaps struct {
	Ports          int  `json:"ports"`
	Multitap       int  `json:"multitap,omitempty"` // how many players with an adapter
	Sticks         int  `json:"sticks,omitempty"`
	AnalogTriggers bool `json:"analogTriggers,omitempty"`
	Touch          bool `json:"touch,omitempty"` // a touch screen: pointer states go to it
	LightGun       bool `json:"lightGun,omitempty"`
	Mouse          bool `json:"mouse,omitempty"`
	Motion         bool `json:"motion,omitempty"`
	Keyboard       bool `json:"keyboard,omitempty"`
	Mic            bool `json:"mic,omitempty"`
}

// MediaCaps: discs, memory cards, BIOS and region.
type MediaCaps struct {
	Discs       bool `json:"discs,omitempty"` // games can come on several discs, changed from the game menu
	MemoryCards int  `json:"memoryCards,omitempty"`
	// BIOS is none, optional or required, from the console's list of BIOS files.
	BIOS string `json:"bios"`
	// Region is the core option that picks the console's region, by core ID.
	Region map[string]string `json:"region,omitempty"`
}

// PlayCaps: what a game menu can offer while playing.
type PlayCaps struct {
	Rumble bool `json:"rumble,omitempty"`
	// RewindByDefault: rewind is on for these games unless someone turns it off. A console's
	// states have to be small enough to keep 20 seconds of them in the memory an
	// administrator allows (gamingRewindMB).
	RewindByDefault bool `json:"rewindByDefault,omitempty"`
}

// PictureCaps: the picture's shape and the Look it starts with.
type PictureCaps struct {
	Aspect  string `json:"aspect"`
	Integer bool   `json:"integer,omitempty"` // whole-pixel scaling suits it
	// Look is the display preset a console starts with until someone chooses: Clean,
	// Scanlines or Handheld LCD (GET /games/settings/schema, presets).
	Look string `json:"look"`
}

// consoleTraits is one console's row.
type consoleTraits struct {
	Era      string
	Launched int
	Ended    int
	Media    string
	Caps     ConsoleCapabilities
}

const (
	lookClean     = "Clean"
	lookScanlines = "Scanlines"
	lookLCD       = "Handheld LCD"
)

// The two-screen layouts, and how each core sets them.
var (
	twoScreenLayouts = []string{"stacked", "side", "big", "single"}
	melonDSScreens   = ScreenOptions{
		Layout: "melonds_screen_layout",
		Values: map[string][2]string{
			"stacked": {"Top/Bottom", "Bottom/Top"},
			"side":    {"Left/Right", "Right/Left"},
			"big":     {"Hybrid Top", "Hybrid Bottom"},
			"single":  {"Top Only", "Bottom Only"},
		},
	}
	azaharScreens = ScreenOptions{
		Layout: "citra_layout_option",
		Values: map[string][2]string{
			"stacked": {"default", "default"},
			"side":    {"side_by_side", "side_by_side"},
			"big":     {"large_screen", "large_screen"},
			"single":  {"single_screen", "single_screen"},
		},
		Swap:       "citra_swap_screen",
		SwapValues: &[2]string{"Top", "Bottom"},
	}
)

func caps(ports int, look, aspect string, integer bool) ConsoleCapabilities {
	return ConsoleCapabilities{
		Screens: ScreenCaps{Count: 1},
		Input:   InputCaps{Ports: ports},
		Picture: PictureCaps{Aspect: aspect, Integer: integer, Look: look},
	}
}

// Years are when each first went on sale and when it stopped being made, roughly: they place
// a console on the Eras page, nothing more.
var consoleTraitsTable = map[string]consoleTraits{
	"a2600": {Era: eraFirst, Launched: 1977, Ended: 1992, Media: mediaCart, Caps: with(caps(2, lookScanlines, "4:3", true), rewindOn)},
	"a7800": {Era: era8Bit, Launched: 1986, Ended: 1992, Media: mediaCart, Caps: with(caps(2, lookScanlines, "4:3", true), rewindOn)},
	"nes": {Era: era8Bit, Launched: 1983, Ended: 1995, Media: mediaCart, Caps: with(caps(2, lookScanlines, "4:3", true), rewindOn, func(c *ConsoleCapabilities) {
		c.Input.Multitap, c.Input.LightGun = 4, true
	})},
	"sms": {Era: era8Bit, Launched: 1985, Ended: 1992, Media: mediaCart, Caps: with(caps(2, lookScanlines, "4:3", true), rewindOn, func(c *ConsoleCapabilities) {
		c.Input.LightGun = true
	})},
	"snes": {Era: era16Bit, Launched: 1990, Ended: 2003, Media: mediaCart, Caps: with(caps(2, lookScanlines, "4:3", true), rewindOn, func(c *ConsoleCapabilities) {
		c.Input.Multitap, c.Input.LightGun, c.Input.Mouse = 5, true, true
	})},
	"md": {Era: era16Bit, Launched: 1988, Ended: 1997, Media: mediaCart, Caps: with(caps(2, lookScanlines, "4:3", true), rewindOn, func(c *ConsoleCapabilities) {
		c.Input.Multitap, c.Input.LightGun, c.Input.Mouse = 4, true, true
	})},
	"pce": {Era: era16Bit, Launched: 1987, Ended: 1994, Media: mediaCart, Caps: with(caps(2, lookScanlines, "4:3", true), rewindOn, func(c *ConsoleCapabilities) {
		c.Input.Multitap, c.Media.Discs = 5, true
	})},
	"gb":   {Era: eraPocket, Launched: 1989, Ended: 2003, Media: mediaPocket, Caps: with(caps(1, lookLCD, "10:9", true), rewindOn)},
	"gba":  {Era: eraPocket, Launched: 2001, Ended: 2010, Media: mediaPocket, Caps: with(caps(1, lookLCD, "3:2", true), rewindOn, rumble)},
	"gg":   {Era: eraPocket, Launched: 1990, Ended: 1997, Media: mediaPocket, Caps: with(caps(1, lookLCD, "10:9", true), rewindOn)},
	"lynx": {Era: eraPocket, Launched: 1989, Ended: 1995, Media: mediaPocket, Caps: with(caps(1, lookLCD, "160:102", true), rewindOn)},
	"ws":   {Era: eraPocket, Launched: 1999, Ended: 2003, Media: mediaPocket, Caps: with(caps(1, lookLCD, "14:9", true), rewindOn)},
	"ngp":  {Era: eraPocket, Launched: 1998, Ended: 2001, Media: mediaPocket, Caps: with(caps(1, lookLCD, "20:19", true), rewindOn)},
	"vb":   {Era: eraPocket, Launched: 1995, Ended: 1996, Media: mediaPocket, Caps: with(caps(1, lookClean, "384:224", true), rewindOn)},
	"n64": {Era: eraEarly3D, Launched: 1996, Ended: 2002, Media: mediaCart, Caps: with(caps(4, lookClean, "4:3", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Sticks = 1
	})},
	"ps1": {Era: eraEarly3D, Launched: 1994, Ended: 2006, Media: mediaDisc, Caps: with(caps(2, lookClean, "4:3", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Multitap, c.Input.Sticks, c.Input.LightGun, c.Input.Mouse = 8, 2, true, true
		c.Media.Discs, c.Media.MemoryCards = true, 2
		c.Media.Region = map[string]string{"pcsx_rearmed": "pcsx_rearmed_region"}
	})},
	"dc": {Era: eraDisc, Launched: 1998, Ended: 2001, Media: mediaDisc, Caps: with(caps(4, lookClean, "4:3", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Sticks, c.Input.AnalogTriggers, c.Input.LightGun, c.Input.Mouse, c.Input.Keyboard = 1, true, true, true, true
		c.Media.Discs = true
	})},
	"ps2": {Era: eraDisc, Launched: 2000, Ended: 2013, Media: mediaDisc, Caps: with(caps(2, lookClean, "4:3", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Multitap, c.Input.Sticks = 8, 2
		c.Media.Discs, c.Media.MemoryCards = true, 2
	})},
	"psp": {Era: eraPocket, Launched: 2004, Ended: 2014, Media: mediaPocket, Caps: with(caps(1, lookClean, "16:9", false), func(c *ConsoleCapabilities) {
		c.Input.Sticks = 1
	})},
	"nds": {Era: eraPocket, Launched: 2004, Ended: 2014, Media: mediaPocket, Caps: with(caps(1, lookClean, "4:3", true), func(c *ConsoleCapabilities) {
		c.Screens = ScreenCaps{Count: 2, Touch: 2, Layouts: twoScreenLayouts, Options: map[string]ScreenOptions{"melonds": melonDSScreens},
			Frame: []ScreenRect{{0, 0, 1, 0.5}, {0, 0.5, 1, 0.5}}} // Top/Bottom, no gap: the two halves
		c.Input.Touch, c.Input.Mic = true, true
	})},
	"3ds": {Era: eraPocket, Launched: 2011, Ended: 2020, Media: mediaPocket, Caps: with(caps(1, lookClean, "5:3", false), func(c *ConsoleCapabilities) {
		c.Screens = ScreenCaps{Count: 2, Touch: 2, Layouts: twoScreenLayouts, Options: map[string]ScreenOptions{"azahar": azaharScreens},
			Frame: []ScreenRect{{0, 0, 1, 0.5}, {0.1, 0.5, 0.8, 0.5}}} // the default layout: the bottom screen (320) centred under the top (400)
		c.Input.Touch, c.Input.Sticks, c.Input.Motion, c.Input.Mic = true, 1, true, true
		c.Media.Region = map[string]string{"azahar": "citra_region_value"}
	})},
	// Played by Dolphin's core or a standalone emulator (games_standalone.go). The Switch is a
	// home console here for its shelf, and a pocket one only in how it's held.
	"gc": {Era: eraDisc, Launched: 2001, Ended: 2007, Media: mediaDisc, Caps: with(caps(4, lookClean, "4:3", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Sticks, c.Input.AnalogTriggers = 2, true
		c.Media.Discs, c.Media.MemoryCards = true, 2
	})},
	"wii": {Era: eraDisc, Launched: 2006, Ended: 2013, Media: mediaDisc, Caps: with(caps(4, lookClean, "16:9", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Sticks, c.Input.Motion, c.Input.LightGun = 1, true, true
		c.Media.Discs = true
	})},
	"wiiu": {Era: eraDisc, Launched: 2012, Ended: 2017, Media: mediaDisc, Caps: with(caps(1, lookClean, "16:9", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Sticks, c.Input.Touch, c.Input.Motion, c.Input.Mic = 2, true, true, true
		c.Media.Discs = true
	})},
	"ps3": {Era: eraDisc, Launched: 2006, Ended: 2017, Media: mediaDisc, Caps: with(caps(2, lookClean, "16:9", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Sticks, c.Input.Motion = 2, true
		c.Media.Discs = true
	})},
	"xbox": {Era: eraDisc, Launched: 2001, Ended: 2006, Media: mediaDisc, Caps: with(caps(4, lookClean, "4:3", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Sticks, c.Input.AnalogTriggers = 2, true
		c.Media.Discs = true
	})},
	"switch": {Era: eraDisc, Launched: 2017, Media: mediaCart, Caps: with(caps(1, lookClean, "16:9", false), rumble, func(c *ConsoleCapabilities) {
		c.Input.Sticks, c.Input.Touch, c.Input.Motion = 2, true, true
	})},
}

func with(c ConsoleCapabilities, more ...func(*ConsoleCapabilities)) ConsoleCapabilities {
	for _, f := range more {
		f(&c)
	}
	return c
}

func rewindOn(c *ConsoleCapabilities) { c.Play.RewindByDefault = true }
func rumble(c *ConsoleCapabilities)   { c.Play.Rumble = true }

// traitsFor gives a console's row, or nil for one this table doesn't know (a custom console
// takes the row of the console whose pad it copies, if any).
func traitsFor(c wittyplay.Console) *consoleTraits {
	if t, ok := consoleTraitsTable[c.ID]; ok {
		return &t
	}
	if c.PadLike != "" {
		if t, ok := consoleTraitsTable[c.PadLike]; ok {
			return &t
		}
	}
	return nil
}

// biosNeed sums up a console's BIOS list as none, optional or required.
func biosNeed(c wittyplay.Console) string {
	need := "none"
	for _, b := range c.BIOS {
		if b.Required {
			return "required"
		}
		need = "optional"
	}
	return need
}

// addTraits fills in a console view's era, years, media and capabilities.
func addTraits(v *consoleView) {
	t := traitsFor(v.Console)
	if t == nil {
		return
	}
	v.Era, v.Launched, v.Ended, v.Media = t.Era, t.Launched, t.Ended, t.Media
	c := t.Caps
	c.Media.BIOS = biosNeed(v.Console)
	if v.Pad.Pointer {
		c.Input.Touch = true
	}
	v.Capabilities = &c
	v.Shelf = shelfLookFor(v.Console)
}

// rewindByDefault: the consoles whose games rewind unless someone says otherwise.
func rewindByDefault(console string) bool {
	if t, ok := consoleTraitsTable[console]; ok {
		return t.Caps.Play.RewindByDefault
	}
	// Only a custom console (its ID starts custom-) can borrow another's row.
	if !strings.HasPrefix(console, "custom-") {
		return false
	}
	if c := wittyplay.ByID(console); c != nil {
		if t := traitsFor(*c); t != nil {
			return t.Caps.Play.RewindByDefault
		}
	}
	return false
}

const rewindKey = "emulation.rewind"

// withRewindDefault turns rewind on in the everyone-on-this-console layer of a console that rewinds
// by default, unless an administrator has said either way for everyone or for this console. It
// changes what's read, never what's stored, so an administrator's choice always wins and a person's
// own setting still wins over both.
func withRewindDefault(console string, server, serverConsole *wittyplay.Layer) {
	if serverConsole == nil || !rewindByDefault(console) {
		return
	}
	if _, set := server.Values[rewindKey]; set {
		return
	}
	if _, set := serverConsole.Values[rewindKey]; set {
		return
	}
	if serverConsole.Values == nil {
		serverConsole.Values = map[string]any{}
	}
	serverConsole.Values[rewindKey] = true
}

// ShelfLook is how a console's games stand on a shelf: its case, and its cartridge or disc, as
// shapes a client draws itself around the game's own cover. They follow the console's real forms
// in proportion and material (a cardboard box, a plastic clamshell, a CD jewel case, a DVD-style
// keep case, a cartridge, a card, a disc), never a maker's banners, logos, colours or exact outline.
type ShelfLook struct {
	Case ShelfShape `json:"case"`
	Cart ShelfShape `json:"cart"`
}

// ShelfShape: Kind is what it is; Aspect its front's width over height; Height how tall it stands
// beside the others of its kind (a full-size box or keep case is 1, a handheld's box about 0.6).
type ShelfShape struct {
	Kind   string  `json:"kind"`
	Aspect float64 `json:"aspect"`
	Height float64 `json:"height"`
}

// Case kinds: box (cardboard), clamshell (plastic, cartridge era), jewel (CD), keep (DVD-style).
// Cartridge kinds: cart, card (HuCard, DS and 3DS cards), disc.
var shelfLooks = map[string]ShelfLook{
	"a2600":  {Case: ShelfShape{"box", 0.72, 1}, Cart: ShelfShape{"cart", 0.85, 0.75}},
	"a7800":  {Case: ShelfShape{"box", 0.72, 1}, Cart: ShelfShape{"cart", 0.85, 0.75}},
	"nes":    {Case: ShelfShape{"box", 0.71, 1}, Cart: ShelfShape{"cart", 0.89, 1}},
	"sms":    {Case: ShelfShape{"clamshell", 0.71, 1}, Cart: ShelfShape{"cart", 0.8, 0.85}},
	"snes":   {Case: ShelfShape{"box", 1.39, 0.68}, Cart: ShelfShape{"cart", 1.18, 0.82}},
	"md":     {Case: ShelfShape{"clamshell", 0.71, 1}, Cart: ShelfShape{"cart", 0.83, 0.88}},
	"pce":    {Case: ShelfShape{"jewel", 1, 0.66}, Cart: ShelfShape{"card", 1.58, 0.45}},
	"gb":     {Case: ShelfShape{"box", 0.96, 0.6}, Cart: ShelfShape{"cart", 0.88, 0.5}},
	"gba":    {Case: ShelfShape{"box", 0.96, 0.6}, Cart: ShelfShape{"cart", 1.6, 0.45}},
	"gg":     {Case: ShelfShape{"clamshell", 0.75, 0.75}, Cart: ShelfShape{"cart", 1.28, 0.5}},
	"lynx":   {Case: ShelfShape{"box", 0.96, 0.6}, Cart: ShelfShape{"card", 1.4, 0.45}},
	"ws":     {Case: ShelfShape{"box", 0.96, 0.6}, Cart: ShelfShape{"cart", 1.4, 0.45}},
	"ngp":    {Case: ShelfShape{"box", 0.96, 0.6}, Cart: ShelfShape{"cart", 1.3, 0.45}},
	"vb":     {Case: ShelfShape{"box", 1.39, 0.68}, Cart: ShelfShape{"cart", 0.9, 0.7}},
	"n64":    {Case: ShelfShape{"box", 1.39, 0.68}, Cart: ShelfShape{"cart", 1.12, 0.78}},
	"ps1":    {Case: ShelfShape{"jewel", 1, 0.66}, Cart: ShelfShape{"disc", 1, 0.66}},
	"dc":     {Case: ShelfShape{"jewel", 1, 0.66}, Cart: ShelfShape{"disc", 1, 0.66}},
	"ps2":    {Case: ShelfShape{"keep", 0.71, 1}, Cart: ShelfShape{"disc", 1, 0.66}},
	"psp":    {Case: ShelfShape{"keep", 0.58, 0.95}, Cart: ShelfShape{"disc", 1, 0.45}},
	"nds":    {Case: ShelfShape{"keep", 0.91, 0.72}, Cart: ShelfShape{"card", 0.94, 0.45}},
	"3ds":    {Case: ShelfShape{"keep", 0.91, 0.72}, Cart: ShelfShape{"card", 0.94, 0.45}},
	"gc":     {Case: ShelfShape{"keep", 0.72, 0.95}, Cart: ShelfShape{"disc", 1, 0.45}},
	"wii":    {Case: ShelfShape{"keep", 0.71, 1}, Cart: ShelfShape{"disc", 1, 0.66}},
	"wiiu":   {Case: ShelfShape{"keep", 0.71, 1}, Cart: ShelfShape{"disc", 1, 0.66}},
	"ps3":    {Case: ShelfShape{"keep", 0.87, 0.9}, Cart: ShelfShape{"disc", 1, 0.66}},
	"xbox":   {Case: ShelfShape{"keep", 0.71, 1}, Cart: ShelfShape{"disc", 1, 0.66}},
	"switch": {Case: ShelfShape{"keep", 0.62, 0.9}, Cart: ShelfShape{"card", 0.72, 0.4}},
}

func shelfLookFor(c wittyplay.Console) *ShelfLook {
	if l, ok := shelfLooks[c.ID]; ok {
		return &l
	}
	if l, ok := shelfLooks[c.PadLike]; ok && c.PadLike != "" {
		return &l
	}
	return nil
}
