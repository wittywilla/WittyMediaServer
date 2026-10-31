// Package wittyplay knows the consoles a game library can hold and how to tell
// which console a file belongs to. It has no dependencies, so the server's
// scanner can use it without pulling in anything that runs or streams a game.
//
// The sub-packages do the rest: stream (sessions, on the server), host and
// libretro (the separate process that runs a game), cores (installing them).
package wittyplay

import (
	"path/filepath"
	"strings"
)

// Core is a libretro core we can point at. Licences are from the core-info files
// libretro publishes under the MIT licence. Non-commercial cores are never in
// this table, so they are never offered for download.
type Core struct {
	ID      string `json:"id"` // the file name without _libretro and its extension
	Name    string `json:"name"`
	License string `json:"license"`
}

// BIOS is a file a core may want, named so people can supply their own. Nothing
// here ever downloads or links to one.
type BIOS struct {
	File     string `json:"file"`
	For      string `json:"for"`
	Required bool   `json:"required"`
}

// Console is one console a library can hold.
type Console struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Short string `json:"short"`
	// Extensions only this console uses; a file with one can be sorted by
	// extension alone.
	Extensions []string `json:"extensions"`
	// Shared extensions are valid here, but other consoles use them too, so
	// they need a console library or a folder name to be sure.
	Shared []string `json:"shared,omitempty"`
	// Aliases are folder names that mean this console, written without spaces or
	// punctuation (see normalise).
	Aliases []string `json:"-"`
	Cores   []Core   `json:"cores"`
	BIOS    []BIOS   `json:"bios,omitempty"`
	// Unavailable, when set, says why games for this console can be listed but
	// not played yet.
	Unavailable string `json:"unavailable,omitempty"`
	// Hidden consoles are for testing and never shown to people.
	Hidden bool `json:"-"`
	// PadLike, on a custom console, is a console whose controller to copy.
	PadLike string `json:"padLike,omitempty"`
}

// TestConsole is the console of the built-in test core, shown only when testing.
const TestConsole = "wittytest"

// Consoles is every console, in the order the add-library flow lists them.
var Consoles = []Console{
	{ID: "nes", Name: "Nintendo Entertainment System", Short: "NES", Extensions: []string{".nes", ".fds", ".unf", ".unif"},
		Aliases: []string{"nes", "famicom", "nintendoentertainmentsystem", "nintendonintendoentertainmentsystem", "fc", "famicomdisksystem", "fds"},
		Cores:   []Core{{"fceumm", "FCEUmm", "GPL-2.0"}, {"nestopia", "Nestopia UE", "GPL-2.0"}, {"mesen", "Mesen", "GPL-3.0"}},
		BIOS:    []BIOS{{"disksys.rom", "Famicom Disk System games", false}}},
	{ID: "snes", Name: "Super Nintendo", Short: "SNES", Extensions: []string{".sfc", ".smc", ".fig", ".swc", ".bs", ".gd3", ".gd7", ".dx2", ".bsx"},
		Aliases: []string{"snes", "sfc", "supernintendo", "superfamicom", "supernes", "nintendosupernintendoentertainmentsystem", "supernintendoentertainmentsystem"},
		Cores:   []Core{{"mednafen_supafaust", "Beetle Supafaust", "GPL-2.0-or-later"}, {"bsnes2014_performance", "bsnes 2014 Performance", "GPL-3.0"}, {"mesen-s", "Mesen-S", "GPL-3.0"}},
		BIOS:    []BIOS{{SatellaviewBIOS, "Satellaview games in .bs files, on Mesen-S; nothing else needs it", false}}},
	{ID: "gb", Name: "Game Boy and Game Boy Color", Short: "Game Boy", Extensions: []string{".gb", ".gbc", ".dmg", ".cgb", ".sgb"},
		Aliases: []string{"gb", "gbc", "gameboy", "gameboycolor", "gameboycolour", "nintendogameboy", "nintendogameboycolor"},
		Cores:   []Core{{"gambatte", "Gambatte", "GPL-2.0"}, {"sameboy", "SameBoy", "MIT"}, {"mgba", "mGBA", "MPL-2.0"}}},
	{ID: "gba", Name: "Game Boy Advance", Short: "GBA", Extensions: []string{".gba"},
		Aliases: []string{"gba", "gameboyadvance", "nintendogameboyadvance"},
		Cores:   []Core{{"mgba", "mGBA", "MPL-2.0"}, {"vba_next", "VBA Next", "GPL-2.0"}},
		BIOS:    []BIOS{{"gba_bios.bin", "better accuracy; games run without it", false}}},
	{ID: "md", Name: "Sega Mega Drive / Genesis", Short: "Mega Drive", Extensions: []string{".md", ".gen", ".smd", ".68k", ".sgd"}, Shared: []string{".bin"},
		Aliases: []string{"md", "genesis", "megadrive", "segagenesis", "segamegadrive", "segamegadrivegenesis"},
		Cores:   []Core{{"blastem", "BlastEm", "GPL-3.0"}, {"clownmdemu", "ClownMDEmu", "AGPL-3.0"}}},
	{ID: "sms", Name: "Sega Master System", Short: "Master System", Extensions: []string{".sms"},
		Aliases: []string{"sms", "mastersystem", "segamastersystem", "segamastersystemmarkiii", "markiii"},
		Cores:   []Core{{"gearsystem", "Gearsystem", "GPL-3.0"}, {"smsplus", "SMS Plus GX", "GPL-2.0"}}},
	{ID: "gg", Name: "Sega Game Gear", Short: "Game Gear", Extensions: []string{".gg"},
		Aliases: []string{"gg", "gamegear", "segagamegear"},
		Cores:   []Core{{"gearsystem", "Gearsystem", "GPL-3.0"}, {"smsplus", "SMS Plus GX", "GPL-2.0"}}},
	{ID: "ps1", Name: "Sony PlayStation", Short: "PlayStation", Extensions: []string{".pbp"}, Shared: []string{".cue", ".chd", ".m3u", ".iso", ".img", ".toc", ".ccd", ".bin"},
		Aliases: []string{"ps1", "psx", "psone", "playstation", "playstation1", "sonyplaystation"},
		Cores:   []Core{{"pcsx_rearmed", "PCSX ReARMed", "GPL-2.0"}, {"mednafen_psx", "Beetle PSX", "GPL-2.0"}, {"swanstation", "SwanStation (graphics card)", "GPL-3.0"}, {"mednafen_psx_hw", "Beetle PSX HW (graphics card)", "GPL-2.0"}},
		BIOS:    []BIOS{{"scph5501.bin", "US consoles; PCSX ReARMed runs most games without one", false}, {"scph5500.bin", "Japanese consoles", false}, {"scph5502.bin", "European consoles", false}}},
	{ID: "pce", Name: "PC Engine / TurboGrafx-16", Short: "PC Engine", Extensions: []string{".pce"}, Shared: []string{".cue", ".chd", ".ccd", ".toc", ".m3u"},
		Aliases: []string{"pce", "pcengine", "turbografx16", "turbografx", "tg16", "necpcenginetg16", "pcenginecd"},
		Cores:   []Core{{"mednafen_pce_fast", "Beetle PCE Fast", "GPL-2.0"}},
		BIOS:    []BIOS{{"syscard3.pce", "CD games", false}}},
	{ID: "a2600", Name: "Atari 2600", Short: "Atari 2600", Extensions: []string{".a26"}, Shared: []string{".bin"},
		Aliases: []string{"a2600", "atari2600", "2600", "atari"},
		Cores:   []Core{{"stella2014", "Stella 2014", "GPL-2.0"}, {"stella", "Stella", "GPL-2.0"}}},
	{ID: "a7800", Name: "Atari 7800", Short: "Atari 7800", Extensions: []string{".a78"}, Shared: []string{".bin"},
		Aliases: []string{"a7800", "atari7800", "7800"},
		Cores:   []Core{{"prosystem", "ProSystem", "GPL-2.0"}},
		BIOS:    []BIOS{{"7800 BIOS (U).rom", "the Atari start-up screen; optional", false}}},
	{ID: "lynx", Name: "Atari Lynx", Short: "Lynx", Extensions: []string{".lnx", ".lyx"},
		Aliases: []string{"lynx", "atarilynx"},
		Cores:   []Core{{"handy", "Handy", "Zlib"}},
		BIOS:    []BIOS{{"lynxboot.img", "most games need it", false}}},
	{ID: "ws", Name: "WonderSwan and WonderSwan Color", Short: "WonderSwan", Extensions: []string{".ws", ".wsc", ".pc2"},
		Aliases: []string{"ws", "wsc", "wonderswan", "wonderswancolor", "bandaiwonderswan"},
		Cores:   []Core{{"mednafen_wswan", "Beetle WonderSwan", "GPL-2.0"}}},
	{ID: "ngp", Name: "Neo Geo Pocket and Color", Short: "Neo Geo Pocket", Extensions: []string{".ngp", ".ngc", ".ngpc", ".npc"},
		Aliases: []string{"ngp", "ngpc", "neogeopocket", "neogeopocketcolor", "snkneogeopocket"},
		Cores:   []Core{{"mednafen_ngp", "Beetle NeoPop", "GPL-2.0"}}},
	{ID: "vb", Name: "Virtual Boy", Short: "Virtual Boy", Extensions: []string{".vb", ".vboy"},
		Aliases: []string{"vb", "virtualboy", "nintendovirtualboy"},
		Cores:   []Core{{"mednafen_vb", "Beetle VB", "GPL-2.0"}}},
	{ID: "n64", Name: "Nintendo 64", Short: "N64", Extensions: []string{".n64", ".z64", ".v64"},
		Aliases: []string{"n64", "nintendo64"},
		Cores:   []Core{{"mupen64plus_next", "Mupen64Plus-Next", "GPL-2.0"}, {"parallel_n64", "ParaLLEl N64", "GPL-2.0"}}},
	{ID: "nds", Name: "Nintendo DS", Short: "DS", Extensions: []string{".nds"},
		Aliases: []string{"nds", "ds", "nintendods"},
		Cores:   []Core{{"melonds", "melonDS", "GPL-3.0"}, {"desmume", "DeSmuME", "GPL-2.0"}},
		BIOS:    []BIOS{{"bios7.bin", "melonDS: better accuracy; games run without it", false}, {"bios9.bin", "melonDS: better accuracy; games run without it", false}, {"firmware.bin", "melonDS: the DS's own start-up menu", false}}},
	{ID: "3ds", Name: "Nintendo 3DS", Short: "3DS", Extensions: []string{".3ds", ".3dsx", ".cci", ".cxi", ".cia", ".zcci", ".zcxi", ".z3dsx"}, Shared: []string{".app", ".elf", ".axf"},
		Aliases: []string{"3ds", "n3ds", "nintendo3ds"},
		Cores:   []Core{{"azahar", "Azahar", "GPL-2.0-or-later"}, {"panda3ds", "Panda3DS", "GPL-3.0"}}},
	{ID: "ps2", Name: "Sony PlayStation 2", Short: "PS2", Shared: []string{".iso", ".bin", ".cue", ".chd", ".cso", ".zso", ".ciso", ".gz", ".img", ".mdf", ".nrg", ".m3u"},
		Aliases: []string{"ps2", "playstation2", "sonyplaystation2", "pstwo"},
		Cores:   []Core{{"pcsx2", "LRPS2 (PCSX2)", "GPL-3.0"}, {"play", "Play!", "MIT"}},
		BIOS:    []BIOS{{"scph39001.bin", "LRPS2 needs one BIOS from a PlayStation 2 you own, and any region will do; Play! runs without one", true}}},
	{ID: "psp", Name: "PlayStation Portable", Short: "PSP", Extensions: []string{".cso"}, Shared: []string{".iso", ".pbp", ".chd"},
		Aliases: []string{"psp", "playstationportable", "sonyplaystationportable"},
		Cores:   []Core{{"ppsspp", "PPSSPP", "GPL-2.0"}}},
	{ID: "dc", Name: "Sega Dreamcast", Short: "Dreamcast", Extensions: []string{".gdi", ".cdi"}, Shared: []string{".chd", ".cue", ".m3u"},
		Aliases: []string{"dc", "dreamcast", "segadreamcast"},
		Cores:   []Core{{"flycast", "Flycast", "GPL-2.0"}},
		BIOS:    []BIOS{{"dc/dc_boot.bin", "better compatibility; most games run without it", false}, {"dc/dc_flash.bin", "the console's settings", false}}},
	{ID: TestConsole, Name: "WittyPlay test console", Short: "Test", Extensions: []string{".wittytest"},
		Aliases: []string{"wittytest"}, Hidden: true},
}

// ByID finds a console.
func ByID(id string) *Console {
	all := All()
	for i := range all {
		if all[i].ID == id {
			return &all[i]
		}
	}
	return nil
}

// Auto is the Library console value for one folder of many consoles.
const Auto = "auto"

// ValidLibraryConsole says whether a library may be set to this console.
func ValidLibraryConsole(id string, testing bool) bool {
	if id == Auto {
		return true
	}
	c := ByID(id)
	return c != nil && (!c.Hidden || testing)
}

// Accepts reports whether a file extension (lower case, with the dot) belongs to
// this console, by itself or as a shared one.
func (c *Console) Accepts(ext string) bool {
	return contains(c.Extensions, ext) || contains(c.Shared, ext)
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// normalise makes a folder name comparable: "Sega - Mega Drive!" -> "segamegadrive".
func normalise(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// withoutBrackets drops anything in round brackets from a folder name.
func withoutBrackets(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(':
			depth++
		case r == ')':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Detect sorts a file into a console, for a library holding many. The folders
// between the library's root and the file come first, nearest first, since they
// are what the owner chose; after that, an extension only one console uses.
// It returns "" when nothing is sure.
func Detect(relPath string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	dir := filepath.Dir(filepath.ToSlash(relPath))
	for dir != "." && dir != "/" && dir != "" {
		all := All()
		for _, name := range folderNames(filepath.Base(dir)) {
			for i := range all {
				c := &all[i]
				if !c.Hidden && contains(c.Aliases, name) && (c.Accepts(ext) || archiveExts[ext]) {
					return c.ID
				}
			}
		}
		dir = filepath.Dir(dir)
	}
	return ByExtension(ext)
}

// archiveExts can hold any console's games, so an extension says nothing about them.
var archiveExts = map[string]bool{".zip": true, ".7z": true, ".rar": true, ".tar": true, ".gz": true, ".tgz": true, ".bz2": true, ".tbz": true, ".tbz2": true,
	".xz": true, ".txz": true, ".zst": true, ".tzst": true, ".001": true}

// folderNames is what a folder's name might mean, most exact first: "Super Nintendo
// (SNES)" is the whole of that, and also "Super Nintendo", and also "SNES".
func folderNames(folder string) []string {
	names := []string{normalise(folder)}
	if before, rest, ok := strings.Cut(folder, "("); ok {
		inner, _, _ := strings.Cut(rest, ")")
		names = append(names, normalise(before), normalise(inner))
	}
	return names
}

// KnownExtension says whether any console uses an extension.
func KnownExtension(ext string) bool {
	all := All()
	for i := range all {
		if all[i].Accepts(ext) {
			return true
		}
	}
	return false
}

// ByExtension is the one console that uses an extension, or "" for none or several.
func ByExtension(ext string) string {
	all := All()
	for i := range all {
		if contains(all[i].Extensions, ext) {
			return all[i].ID
		}
	}
	return ""
}

// CoreLicense looks up a core we know about, for showing beside one an
// administrator placed in the folder themselves.
func CoreLicense(id string) (Core, bool) {
	for i := range Consoles {
		for _, c := range Consoles[i].Cores {
			if c.ID == id {
				return c, true
			}
		}
	}
	if lic, ok := nonCommercial[id]; ok {
		return Core{ID: id, Name: id, License: lic}, true
	}
	return Core{}, false
}

// nonCommercial are well-known cores with licences that forbid commercial use.
// They are never offered, but if an administrator supplies one it should be
// labelled for what it is.
var nonCommercial = map[string]string{
	"snes9x": "Non-commercial", "snes9x2010": "Non-commercial", "snes9x2005": "Non-commercial", "snes9x2002": "Non-commercial",
	"genesis_plus_gx": "Non-commercial", "picodrive": "Non-commercial", "fbneo": "Non-commercial", "fbalpha2012": "Non-commercial",
	"mame2003": "Non-commercial", "mame2003_plus": "Non-commercial", "mame2010": "Non-commercial", "mame2000": "Non-commercial",
}

// IsNonCommercial says whether a core's licence forbids commercial use, as far
// as this table knows.
func IsNonCommercial(id string) bool {
	_, ok := nonCommercial[id]
	return ok
}
