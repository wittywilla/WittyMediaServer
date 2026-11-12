// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

// What each core on offer is good at, in a line, for choosing between a console's cores: in the
// core picker on a game's page and in Settings, and for the apps (GET /games/consoles gives
// coreAbout; the core choices give each one's). Where it runs and how it draws are corebuilds.txt
// and coreSupport; its licence is the module's console table. Every core in corebuilds.txt has a
// note here (TestEveryCoreHasANote).

// What a core is chosen for, beside its console's other cores.
const (
	strengthAccuracy = "accuracy" // closest to the console, for more of the processor
	strengthSpeed    = "speed"    // lightest on the server, for slower machines
	strengthBalanced = "balanced" // between the two
)

// coreNote is one core's line.
type coreNote struct {
	// Strength is accuracy, speed or balanced, beside its console's other cores; "" for a
	// console's only core.
	Strength string `json:"strength,omitempty"`
	About    string `json:"about"`
	// BIOS, when the core won't start without one of these files of the owner's own and its traits
	// in the module don't already say so. Any one of them will do.
	BIOS []string `json:"-"`
}

var psxBIOS = []string{"scph5501.bin", "scph5500.bin", "scph5502.bin"}

var coreNotes = map[string]coreNote{
	// NES
	"fceumm":   {strengthBalanced, "Fast and compatible, with the widest support for unusual cartridges and the Famicom Disk System.", nil},
	"nestopia": {strengthAccuracy, "Accurate, and plays nearly every game, the Famicom Disk System too.", nil},
	"mesen":    {strengthAccuracy, "The most accurate, and plays every game. Uses more of the processor.", nil},
	"quicknes": {strengthSpeed, "The lightest, for slow servers. Plays the well-known games, not unusual cartridges.", nil},
	// Super Nintendo
	"mednafen_supafaust":    {strengthSpeed, "Fast and accurate for nearly every game, spread over several processor cores.", nil},
	"bsnes2014_performance": {strengthBalanced, "bsnes's lighter profile from 2014: accurate, and easy on most servers.", nil},
	"mesen-s":               {strengthAccuracy, "Very accurate, and the one that plays Satellaview games (with BS-X.bin).", nil},
	"bsnes":                 {strengthAccuracy, "Today's bsnes, the reference for accuracy, and the heaviest on the processor.", nil},
	// Game Boy, Game Boy Color
	"gambatte": {strengthAccuracy, "Very accurate, and light.", nil},
	"sameboy":  {strengthAccuracy, "The most accurate, with Super Game Boy borders and colours.", nil},
	"mgba":     {strengthBalanced, "Accurate and fast, with the most features.", nil},
	"gearboy":  {strengthSpeed, "Light and fast, with good compatibility.", nil},
	// Game Boy Advance
	"vba_next": {strengthSpeed, "Fast, for slower servers, and less accurate.", nil},
	"vbam":     {strengthBalanced, "VisualBoyAdvance-M: long-tested, with good compatibility.", nil},
	"gpsp":     {strengthSpeed, "The lightest, for low-power servers, and the least accurate. Has a BIOS of its own built in.", nil},
	// Mega Drive
	"blastem":    {strengthAccuracy, "Very accurate.", nil},
	"clownmdemu": {strengthBalanced, "A newer, accurate core.", nil},
	// Master System, Game Gear
	"gearsystem": {strengthAccuracy, "Accurate, for Master System, Game Gear and SG-1000.", nil},
	"smsplus":    {strengthSpeed, "Light and fast.", nil},
	// PlayStation
	"pcsx_rearmed":    {strengthSpeed, "Fast and light, and plays most games without a BIOS file.", nil},
	"mednafen_psx":    {strengthAccuracy, "The most accurate, drawn by the processor at the console's own size.", psxBIOS},
	"swanstation":     {strengthBalanced, "Fast, and sharper: drawn on a graphics card at higher resolutions.", psxBIOS},
	"mednafen_psx_hw": {strengthAccuracy, "Beetle PSX's accuracy, drawn on a graphics card at higher resolutions.", psxBIOS},
	// PC Engine
	"mednafen_pce_fast":   {strengthSpeed, "Fast, and plays CD games with your System Card file.", nil},
	"mednafen_pce":        {strengthAccuracy, "The full Beetle PCE: more accurate, and plays SuperGrafx games too.", nil},
	"mednafen_supergrafx": {strengthBalanced, "Made for the SuperGrafx, and plays PC Engine games as well.", nil},
	// Atari
	"stella2014":    {strengthSpeed, "Stella from 2014: light, and plays nearly every game.", nil},
	"stella":        {strengthAccuracy, "Today's Stella: the most accurate, with the newest fixes.", nil},
	"prosystem":     {"", "Plays Atari 7800 games, with the start-up screen when you add its BIOS.", nil},
	"handy":         {strengthSpeed, "Light, and plays most games; better with the Lynx's boot ROM.", nil},
	"mednafen_lynx": {strengthAccuracy, "Mednafen's version of Handy: more accurate.", []string{"lynxboot.img"}},
	// Handhelds
	"mednafen_wswan": {"", "Accurate WonderSwan and WonderSwan Color.", nil},
	"mednafen_ngp":   {strengthAccuracy, "Accurate Neo Geo Pocket and Color.", nil},
	"race":           {strengthSpeed, "Light and fast, with good compatibility.", nil},
	"mednafen_vb":    {"", "Accurate Virtual Boy.", nil},
	// Nintendo 64
	"mupen64plus_next": {strengthBalanced, "Plays most games, and sharper at higher resolutions on a graphics card.", nil},
	"parallel_n64":     {strengthAccuracy, "Its own graphics plugins, for games Mupen64Plus-Next draws wrongly. Plays libdragon homebrew too.", nil},
	// Nintendo DS
	"melonds": {strengthAccuracy, "Accurate, with the DS's own start-up menu when you add its firmware.", nil},
	"desmume": {strengthBalanced, "Long-tested, and plays most games.", nil},
	// Nintendo 3DS
	"azahar":   {strengthAccuracy, "Carries on from Citra, and plays the most games.", nil},
	"panda3ds": {"", "A newer core, which plays fewer games.", nil},
	// GameCube and Wii
	"dolphin": {strengthAccuracy, "Dolphin, as a core: plays GameCube and Wii games on the server's graphics card.", nil},
	// PlayStation 2
	"pcsx2": {strengthAccuracy, "LRPS2, made from PCSX2: plays the most games. Needs a BIOS from your console.", nil},
	"play":  {strengthBalanced, "Play!: needs no BIOS, and plays fewer games.", nil},
	// PSP, Dreamcast
	"ppsspp":  {"", "Plays nearly every PSP game, sharper at higher resolutions on a graphics card.", nil},
	"flycast": {"", "Plays Dreamcast games, sharper at higher resolutions on a graphics card.", nil},
}

// coreNoteFor is a core's line, or nothing for a core this server doesn't know, such as a custom one.
func coreNoteFor(id string) *coreNote {
	if n, ok := coreNotes[id]; ok {
		return &n
	}
	return nil
}
