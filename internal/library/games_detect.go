package library

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gameid"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
	"github.com/wittywilla/wittyplay"
)

// Deciding what a file in a games library is. A file's name is where the question
// starts, not where it ends: a folder and an extension make a guess, and the file's own
// bytes settle it (see gameid). A verdict says how sure it is, and why. A file that can't
// be shown to be a game doesn't become one: it goes on the "needs a look" list with a
// reason and a suggestion, so one click settles it. What's plainly not a game goes on
// the "left out" list, where it can be looked at and overruled.

// gameJudgeVersion is raised when these rules change, so that scans judge again.
const gameJudgeVersion = 1

// How a verdict was reached.
const (
	howBytes  = "bytes"     // the file's own header proves it
	howExt    = "extension" // an extension only one console uses
	howFolder = "folder"    // the folder or library says so, and nothing in the file disagrees
	howChosen = "chosen"    // a person said so
)

// Review kinds. needs is what wants a person's attention; the rest are left out for a reason.
const (
	reviewNeeds     = "needs"
	reviewBlocked   = "blocked" // a game, listed, that can't play as it is
	reviewDuplicate = "duplicate"
	reviewIgnored   = "ignored"
)

// gameVerdict is what a scan decided about one file.
type gameVerdict struct {
	// Console is what it's for, when it's a game. Empty when it's left out.
	Console string `json:"console,omitempty"`
	How     string `json:"how,omitempty"`
	Sure    int    `json:"sure,omitempty"` // out of 100
	Proof   string `json:"proof,omitempty"`
	// Title, Serial and Region are what the file calls itself.
	Title  string `json:"title,omitempty"`
	Serial string `json:"serial,omitempty"`
	Region string `json:"region,omitempty"`
	// Problem is why a game can't play as it is, such as encryption.
	Problem string `json:"problem,omitempty"`
	NewOnly bool   `json:"newOnly,omitempty"`
	// Left says what it's left out as: needs, or a gameid.Kind. Suggest lists consoles it
	// might be, best first.
	Left    string   `json:"left,omitempty"`
	Why     string   `json:"why,omitempty"`
	Suggest []string `json:"suggest,omitempty"`
	// Entry is, for an archive, the file inside that would be played, and Entries how many
	// game files it holds. Name is what to call the game, when the file's own name won't do.
	Entry   string `json:"entry,omitempty"`
	Entries int    `json:"entries,omitempty"`
	Name    string `json:"name,omitempty"`
	// Payload and CRC identify the game's main file, for finding duplicates. CRC is 0
	// until it's needed: reading a whole disc to find out costs too much to do by default.
	Payload int64  `json:"payload,omitempty"`
	CRC     uint32 `json:"crc,omitempty"`
}

func (v gameVerdict) isGame() bool { return v.Console != "" && v.Left == "" }

// consoleTable is the list of consoles, kept for a scan. Going through it is the point:
// a scan asks about each of thousands of files, and the list comes from another process.
type consoleTable struct {
	list []wittyplay.Console
	ids  map[string]*wittyplay.Console
	only map[string]string // an extension only one console has as its own
	any  map[string]bool
}

var consolesNow atomic.Pointer[consoleTable]

// freshConsoles reads the list again. A scan starts with this, in case someone has added a
// console since the last one.
func freshConsoles() *consoleTable {
	t := &consoleTable{list: wittyplay.All(), ids: map[string]*wittyplay.Console{}, only: map[string]string{}, any: map[string]bool{}}
	for i := range t.list {
		c := &t.list[i]
		t.ids[c.ID] = c
		for _, x := range c.Extensions {
			if _, taken := t.only[x]; !taken {
				t.only[x] = c.ID
			}
			t.any[x] = true
		}
		for _, x := range c.Shared {
			t.any[x] = true
		}
	}
	consolesNow.Store(t)
	return t
}

func consoles() *consoleTable {
	if t := consolesNow.Load(); t != nil {
		return t
	}
	return freshConsoles()
}

func (t *consoleTable) byID(id string) *wittyplay.Console { return t.ids[id] }

func (t *consoleTable) byExtension(ext string) string { return t.only[ext] }

func (t *consoleTable) known(ext string) bool { return t.any[ext] }

func isCustomConsole(id string) bool { return strings.HasPrefix(id, wittyplay.CustomPrefix) }

// folderHint is what the path says: the console of the nearest folder named after one.
func folderHint(rel string) string {
	dir := filepath.ToSlash(filepath.Dir(rel))
	all := consoles().list
	for dir != "." && dir != "/" && dir != "" {
		for _, name := range hintNames(path.Base(dir)) {
			for i := range all {
				if !all[i].Hidden && containsString(all[i].Aliases, name) {
					return all[i].ID
				}
			}
		}
		dir = path.Dir(dir)
	}
	return ""
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func normalName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hintNames is what a folder's name might mean, most exact first: "Super Nintendo
// (SNES)" is all of that, and "Super Nintendo", and "SNES".
func hintNames(folder string) []string {
	names := []string{normalName(folder)}
	if before, rest, ok := strings.Cut(folder, "("); ok {
		inner, _, _ := strings.Cut(rest, ")")
		names = append(names, normalName(before), normalName(inner))
	}
	return names
}

// mustHaveHeader lists the consoles whose every game has a header their cores rely on.
// A file for one of them without it won't play, whatever it's called.
var mustHaveHeader = map[string]bool{"gb": true, "gba": true, "n64": true, "nds": true, "ngp": true, "ps1": true, "ps2": true, "psp": true, "3ds": true}

// opaqueExts are formats a header can't be read from here: compressed in a way that would
// take a decoder of its own, or with one offset nothing can guess. They go by where they're
// kept. Nothing about them speaks against the folder.
var opaqueExts = map[string]bool{".chd": true, ".cdi": true, ".nrg": true, ".mdf": true, ".gz": true, ".zcci": true, ".zcxi": true, ".z3dsx": true, ".bcci": true, ".bcxi": true, ".elf": true, ".axf": true, ".gdi": true, ".toc": true, ".ccd": true, ".m3u": true, ".cue": true}

// extHasNoProof are extensions that belong to a console's cartridges, but are also what
// other things are often called, so they need a header as well: .md is usually text, .dmg
// a Mac disk image, .vb source code, .ws a script, .bs and .app anything.
var extHasNoProof = map[string]bool{".md": true, ".bin": true, ".iso": true, ".img": true, ".dmg": true, ".cgb": true, ".sgb": true, ".app": true}

// headerless consoles have no header to check, so .bin is as good a name as any there.
var headerless = map[string]bool{"a2600": true, "a7800": true, "pce": true, "vb": true, "ws": true}

// minCartridge is smaller than any real cartridge. Below it, a name proves nothing.
const minCartridge = 2 << 10

// consoleName says a console's name, for messages.
func consoleName(id string) string {
	if c := consoles().byID(id); c != nil {
		return c.Name
	}
	switch id {
	case gameid.Sega32X:
		return "Sega 32X"
	case gameid.SegaCD:
		return "Sega CD"
	case gameid.Saturn:
		return "Sega Saturn"
	case gameid.GameCube:
		return "Nintendo GameCube"
	case gameid.Wii:
		return "Nintendo Wii"
	case gameid.Xbox:
		return "Xbox"
	case gameid.ThreeDO:
		return "3DO"
	case gameid.PCFX:
		return "PC-FX"
	case gameid.NeoGeoCD:
		return "Neo Geo CD"
	}
	return id
}

func needsLook(why string, suggest ...string) gameVerdict {
	var clean []string
	for _, s := range suggest {
		if s != "" && !containsString(clean, s) && consoles().byID(s) != nil {
			clean = append(clean, s)
		}
	}
	return gameVerdict{Left: reviewNeeds, Why: why, Suggest: clean}
}

func leftOut(kind gameid.Kind, why string, suggest ...string) gameVerdict {
	v := needsLook(why, suggest...)
	v.Left = string(kind)
	return v
}

// accept is the verdict for a game.
func accept(console, how string, sure int, proof string) gameVerdict {
	return gameVerdict{Console: console, How: how, Sure: sure, Proof: proof}
}

func proven(f gameid.Finding, library string) gameVerdict {
	c := consoles().byID(f.Console)
	if c == nil {
		return needsLook(fmt.Sprintf("This is a %s disc or cartridge. %s", consoleName(f.Console), f.Proof+" This server has no core for that console."), library)
	}
	v := accept(f.Console, howBytes, 100, f.Proof)
	v.Title, v.Serial, v.Region, v.NewOnly = f.Title, f.Serial, f.Region, f.NewOnly
	if f.Encrypted {
		v.Problem = "encrypted"
	}
	return v
}

// judge decides about a file, or an archive. An empty verdict means the scan needn't
// mention it: pictures, text and whatever else lives in a games folder.
func (ix *archiveIndex) judge(p, rel, library string, size int64) gameVerdict {
	name := filepath.Base(p)
	if gamearchive.IsArchive(name) && !strings.EqualFold(filepath.Ext(name), ".gz") {
		return ix.judgeArchive(p, rel, library)
	}
	if strings.EqualFold(filepath.Ext(name), ".gz") {
		// Only a compressed PS2 image is plain .gz; anything else is an archive like any other.
		if inner := strings.ToLower(filepath.Ext(strings.TrimSuffix(name, filepath.Ext(name)))); !strings.EqualFold(library, "ps2") && inner != ".iso" {
			return ix.judgeArchive(p, rel, library)
		}
	}
	return judgeFile(p, rel, library, size)
}

// judgeFile decides about one file that isn't an archive.
func judgeFile(p, rel, library string, size int64) gameVerdict {
	ext := strings.ToLower(filepath.Ext(p))
	stem := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	hint := library
	if hint == wittyplay.Auto || hint == "" {
		hint = folderHint(rel)
	}
	if !consoles().known(ext) && (hint == "" || !consoles().byID(hint).Accepts(ext)) {
		return gameVerdict{}
	}
	// A console someone added themselves: only they know what its files look like. The
	// built-in test console's files are whatever a test writes.
	if c := consoles().byID(hint); c != nil && (isCustomConsole(hint) || c.Hidden) && c.Accepts(ext) {
		return accept(hint, howFolder, 70, "It's in a folder or library for this console.")
	}
	if only := consoles().byExtension(ext); only != "" && (isCustomConsole(only) || consoles().byID(only).Hidden) {
		return accept(only, howExt, 70, "Only this console uses "+ext+" files.")
	}
	f, err := gameid.SniffFile(p)
	if err != nil {
		// Not an answer about the file: the next scan asks again.
		if c := consoles().byID(hint); c != nil && c.Accepts(ext) {
			return accept(hint, howFolder, 40, "It couldn't be read just now, so this goes by its folder.")
		}
		return gameVerdict{}
	}
	return decide(f, ext, stem, hint, library, size, func(console string) (bool, string) {
		h, err := os.Open(p)
		if err != nil {
			return true, ""
		}
		defer h.Close()
		return gameid.CheckFor(console, h, size)
	})
}

// decide is what's the same for a file and for a file inside an archive.
func decide(f gameid.Finding, ext, stem, hint, library string, size int64, fits func(string) (bool, string)) gameVerdict {
	switch f.Kind {
	case gameid.KindBIOS, gameid.KindUpdate, gameid.KindDLC, gameid.KindUtility, gameid.KindAudio, gameid.KindOther:
		return leftOut(f.Kind, f.Proof, hint)
	case gameid.KindData:
		return needsLook(f.Proof, hint)
	}
	if kind, why := gameid.ClassifyName(stem); kind != gameid.KindUnknown {
		v := leftOut(kind, why, hint, f.Console)
		return v
	}
	if gameid.TrackNumber(stem) > 1 && !f.Proven() {
		return leftOut(gameid.KindData, "This is one track of a disc, with no cue sheet to say which game it belongs to.", hint)
	}
	if f.Proven() {
		v := proven(f, hint)
		switch {
		case library != "" && library != wittyplay.Auto && v.Console != "" && v.Console != library:
			return needsLook(fmt.Sprintf("This library is set to %s, but this file is for %s. %s", consoleName(library), consoleName(v.Console), f.Proof), v.Console, library)
		case hint != "" && v.Console != "" && v.Console != hint:
			v.Proof += " It's kept in a folder named for " + consoleName(hint) + ", but the file itself decides."
		}
		return v
	}
	if hint != "" {
		return decideByFolder(f, ext, hint, size, fits)
	}
	// No folder or library to go on, and nothing in the file.
	if only := consoles().byExtension(ext); only != "" && !extHasNoProof[ext] && !mustHaveHeader[only] && size >= minCartridge {
		if ok, why := fits(only); ok {
			return accept(only, howExt, 60, "Only "+consoleName(only)+" uses "+ext+" files. "+why)
		}
	}
	why := "Nothing in this file says which console it's for, and neither does its folder."
	if f.Proof != "" {
		why = f.Proof + " Neither does its folder."
	}
	return needsLook(why, consoles().byExtension(ext))
}

// decideByFolder: the folder or library says which console, and the file can't confirm it.
func decideByFolder(f gameid.Finding, ext, hint string, size int64, fits func(string) (bool, string)) gameVerdict {
	c := consoles().byID(hint)
	name := consoleName(hint)
	if !c.Accepts(ext) {
		if only := consoles().byExtension(ext); only != "" {
			return needsLook(fmt.Sprintf("It's in a folder for %s, but only %s uses %s files.", name, consoleName(only), ext), only, hint)
		}
		return gameVerdict{}
	}
	proof := "Its folder is named for this console, and " + ext + " files are what it uses."
	switch {
	case opaqueExts[ext]:
		if f.Media == "gd" && hint != "dc" {
			return needsLook("This is a GD-ROM, which only a Dreamcast uses, in a folder for "+name+".", "dc", hint)
		}
		if f.Media == "dvd" && hint != "ps2" && hint != "psp" {
			return needsLook("This is a DVD image, in a folder for "+name+", which doesn't use DVDs.", hint)
		}
		return accept(hint, howFolder, 70, proof)
	case mustHaveHeader[hint], extHasNoProof[ext] && !headerless[hint]:
		why := fmt.Sprintf("It's in a folder for %s, but it doesn't have the header every %s game has, so a core won't be able to start it.", name, name)
		if f.Proof != "" {
			why = fmt.Sprintf("It's in a folder for %s, but: %s", name, f.Proof)
		}
		return needsLook(why, hint)
	}
	if size < minCartridge {
		return needsLook(fmt.Sprintf("It's in a folder for %s, but at %d bytes it's too small to be a game.", name, size), hint)
	}
	if ok, why := fits(hint); !ok {
		return needsLook(fmt.Sprintf("It's in a folder for %s. %s", name, why), hint)
	} else if why != "" {
		proof = "Its folder is named for this console. " + why
	}
	return accept(hint, howFolder, 75, proof)
}

// ---------------------------------------------------------------- archives

var genericStemRe = regexp.MustCompile(`(?i)^(?:game|rom|disc|disk|image|dump|file|data|new( folder)?|untitled)[\s_-]*[0-9]*$`)

// judgeArchive decides about an archive from what it lists, and looks inside only when
// the names can't settle it.
func (ix *archiveIndex) judgeArchive(p, rel, library string) gameVerdict {
	name := filepath.Base(p)
	if gamearchive.IsLaterVolume(name) {
		return gameVerdict{}
	}
	hint := library
	if hint == wittyplay.Auto || hint == "" {
		hint = folderHint(rel)
	}
	stem := gamearchive.BaseName(name)
	l, problem := ix.listing(p)
	if problem != "" {
		return needsLook(archiveProblem(problem), hint)
	}
	// What's in it that any console could use. Anything else is a readme or a picture.
	var entries []gamearchive.Entry
	for _, e := range l.Entries {
		x := strings.ToLower(path.Ext(e.Name))
		if consoles().known(x) || (hint != "" && consoles().byID(hint).Accepts(x)) {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		if hint != "" {
			return leftOut(gameid.KindOther, fmt.Sprintf("This archive has no game file in it for %s.", consoleName(hint)), hint)
		}
		return gameVerdict{}
	}
	// Whose are they? An extension only one console uses counts, by how much of the
	// archive it is: a small readme.md can't outvote a big .bin.
	bytesFor := map[string]int64{}
	var total int64
	for _, e := range entries {
		x := strings.ToLower(path.Ext(e.Name))
		sz := max(e.Size, 1)
		total += sz
		if c := consoles().byExtension(x); c != "" && !extHasNoProof[x] {
			bytesFor[c] += sz
		}
	}
	exclusive := ""
	var most int64
	for c, n := range bytesFor {
		if n > most || (n == most && c < exclusive) {
			exclusive, most = c, n
		}
	}
	if most*2 < total {
		exclusive = "" // mostly files that say nothing
	}
	console := hint
	if c := consoles().byID(hint); exclusive != "" && (c == nil || !gamearchive.HoldsGame(extsOf(entries), c)) {
		console = exclusive
	}
	if console == "" {
		console = exclusive
	}
	var v gameVerdict
	if c := consoles().byID(console); c != nil && gamearchive.HoldsGame(extsOf(entries), c) {
		v = ix.judgeSet(p, stem, l, c, entries, hint, library)
	} else {
		v = ix.judgeAmbiguous(p, stem, l, entries, hint, library)
	}
	return v
}

func extsOf(list []gamearchive.Entry) []string {
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, strings.ToLower(path.Ext(e.Name)))
	}
	return out
}

func archiveProblem(code string) string {
	switch code {
	case gamearchive.ErrEncrypted.Code:
		return "This archive is protected by a password, so it can't be read. No password is ever tried."
	case gamearchive.ErrDamaged.Code:
		return "This archive is damaged, or isn't really an archive."
	case gamearchive.ErrTooBig.Code, gamearchive.ErrTooMany.Code:
		return "This archive is too large to read safely."
	case gamearchive.ErrUnsafe.Code:
		return "This archive has a file in it with a name that climbs out of its folder, so it was refused."
	}
	return "This archive couldn't be read."
}

// judgeAmbiguous is for an archive whose files could belong to several consoles: its
// main file has to say.
func (ix *archiveIndex) judgeAmbiguous(p, stem string, l *gamearchive.Listing, entries []gamearchive.Entry, hint, library string) gameVerdict {
	main := mainEntry(entries)
	f := ix.peek(p, main)
	if f.Kind != gameid.KindUnknown && f.Kind != gameid.KindGame {
		return leftOut(f.Kind, "Inside the archive: "+f.Proof, hint)
	}
	if !f.Proven() {
		if hint != "" {
			return needsLook(fmt.Sprintf("It's in a folder for %s, but what's in this archive doesn't have what a %s game has.", consoleName(hint), consoleName(hint)), hint)
		}
		return needsLook("Nothing in this archive says which console it's for, and neither does its folder.")
	}
	c := consoles().byID(f.Console)
	if c == nil {
		return needsLook(fmt.Sprintf("This archive holds a %s game. This server has no core for that console.", consoleName(f.Console)), hint)
	}
	v := ix.judgeSet(p, stem, l, c, entries, hint, library)
	return v
}

// mainEntry is the file holding the game: the biggest, leaving out what only describes
// one. A disc's first track is usually it, and is usually biggest.
func mainEntry(entries []gamearchive.Entry) gamearchive.Entry {
	best := entries[0]
	for _, e := range entries {
		switch strings.ToLower(path.Ext(e.Name)) {
		case ".cue", ".gdi", ".m3u", ".ccd", ".toc":
			continue
		}
		bx := strings.ToLower(path.Ext(best.Name))
		if bx == ".cue" || bx == ".gdi" || bx == ".m3u" || bx == ".ccd" || bx == ".toc" || e.Size > best.Size {
			best = e
		}
	}
	return best
}

// judgeSet decides about an archive once it's known whose files it holds: whether
// they're games at all, whether they're one game, and which of them to play.
func (ix *archiveIndex) judgeSet(p, stem string, l *gamearchive.Listing, c *wittyplay.Console, entries []gamearchive.Entry, hint, library string) gameVerdict {
	var mine []gamearchive.Entry
	for _, e := range entries {
		if c.Accepts(strings.ToLower(path.Ext(e.Name))) {
			mine = append(mine, e)
		}
	}
	all := len(mine)
	// Tiny files beside a real one aren't versions of it: a readme.md is no Mega Drive game.
	var biggest int64
	for _, e := range mine {
		biggest = max(biggest, e.Size)
	}
	real := mine[:0:0]
	for _, e := range mine {
		switch strings.ToLower(path.Ext(e.Name)) {
		case ".cue", ".gdi", ".m3u", ".ccd", ".toc":
			real = append(real, e)
			continue
		}
		if e.Size*64 >= biggest {
			real = append(real, e)
		}
	}
	mine = real

	kinds := map[gameid.Kind]int{}
	quality := map[gameid.Quality]int{}
	titles := map[string]bool{}
	stemTitle, _ := gamemeta.CleanName(stem)
	stemKey := gamemeta.NameKey(stemTitle)
	stemMatches := false
	for _, e := range mine {
		base := path.Base(e.Name)
		s := strings.TrimSuffix(base, path.Ext(base))
		k, _ := gameid.ClassifyName(s)
		kinds[k]++
		q := gameid.QualityOf(s)
		quality[q]++
		if k == gameid.KindUnknown {
			t, _ := gamemeta.CleanName(s)
			key := gamemeta.NameKey(t)
			titles[key] = true
			// One game's versions, or one series: a set is named for what it holds.
			if key == stemKey || strings.HasPrefix(key, stemKey+" ") || sameStart(key, stemKey) {
				stemMatches = true
			}
		}
	}
	// Everything in it is one thing that isn't a game.
	for _, kind := range []gameid.Kind{gameid.KindBIOS, gameid.KindUtility, gameid.KindUpdate, gameid.KindDLC} {
		if kinds[kind] == len(mine) {
			return leftOut(kind, whatItHolds(kind), c.ID)
		}
	}
	if k, why := gameid.ClassifyName(stem); k != gameid.KindUnknown && kinds[gameid.KindUnknown] == 0 {
		return leftOut(k, why, c.ID)
	}
	// A pack of unrelated things has no single game to list. A collection of versions
	// of one game, however many, always has something named like the archive.
	switch {
	case quality[gameid.QualityPD] == len(mine) && len(mine) > 1:
		return needsLook(fmt.Sprintf("This archive holds %d homemade demos and programs, not one game. They can't be listed one by one while they're in one archive: unpack them to play them.", all), c.ID)
	case quality[gameid.QualityHack] == len(mine) && len(titles) >= 4:
		return needsLook(fmt.Sprintf("This archive holds %d different fan-made hacks, not one game. They can't be listed one by one while they're in one archive: unpack them to play them.", len(mine)), c.ID)
	case len(titles) >= 8 && !stemMatches:
		return needsLook(fmt.Sprintf("This archive holds %d different titles, not one game. They can't be listed one by one while they're in one archive: unpack them to play them.", len(titles)), c.ID)
	}

	plan, err := gamearchive.Choose(l, c, filepath.Base(p))
	if err != nil {
		return needsLook(fmt.Sprintf("Nothing in this archive plays on %s.", c.Name), hint)
	}
	played := plan.Entry
	if len(plan.Want) > 0 {
		played = plan.Want[0].Name
		if strings.Contains(plan.Entry, "/") || strings.EqualFold(path.Ext(plan.Entry), ".cue") || strings.EqualFold(path.Ext(plan.Entry), ".gdi") || strings.EqualFold(path.Ext(plan.Entry), ".m3u") {
			played = plan.Entry
		}
	}
	// What's played has to be a game, even if something else in the set is.
	playedStem := strings.TrimSuffix(path.Base(played), path.Ext(played))
	if k, why := gameid.ClassifyName(playedStem); k != gameid.KindUnknown {
		return leftOut(k, why, c.ID)
	}
	v := ix.proofForSet(p, c, mine, played, hint, library)
	if v.Left != "" {
		return v
	}
	if library != "" && library != wittyplay.Auto && v.Console != library {
		return needsLook(fmt.Sprintf("This library is set to %s, but this archive holds a game for %s. %s", consoleName(library), consoleName(v.Console), v.Proof), v.Console, library)
	}
	v.Entry = played
	v.Entries = len(mine)
	if gameid.IsSlug(stem) || genericStemRe.MatchString(stem) || len(stem) < 2 {
		v.Name = playedStem
	}
	main := mainEntry(mine)
	v.Payload, v.CRC = main.Size, main.CRC
	return v
}

// sameStart says two names begin with the same two words.
func sameStart(a, b string) bool {
	x, y := strings.Fields(a), strings.Fields(b)
	return len(x) >= 2 && len(y) >= 2 && x[0] == y[0] && x[1] == y[1]
}

func whatItHolds(kind gameid.Kind) string {
	switch kind {
	case gameid.KindBIOS:
		return "Everything in this archive is named as a BIOS."
	case gameid.KindUtility:
		return "Everything in this archive is named as a utility, not a game."
	case gameid.KindUpdate:
		return "Everything in this archive is named as an update for a game."
	case gameid.KindDLC:
		return "Everything in this archive is named as extra content for a game."
	}
	return ""
}

// proofForSet says how sure an archive's console is. Reading what's inside costs little
// in a zip, and in anything else when there's no better way to know, so that's when it's done.
func (ix *archiveIndex) proofForSet(p string, c *wittyplay.Console, mine []gamearchive.Entry, played, hint, library string) gameVerdict {
	main := mainEntry(mine)
	x := strings.ToLower(path.Ext(main.Name))
	ambiguous := extHasNoProof[x] || consoles().byExtension(x) == ""
	if strings.EqualFold(filepath.Ext(p), ".zip") || ambiguous {
		f := ix.peek(p, main)
		switch {
		case f.Proven() && f.Console == c.ID:
			v := proven(f, hint)
			v.Proof = "Inside the archive: " + f.Proof
			return v
		case f.Proven():
			if consoles().byID(f.Console) == nil {
				return needsLook(fmt.Sprintf("This archive holds a %s game. This server has no core for that console.", consoleName(f.Console)), hint)
			}
			if library != "" && library != wittyplay.Auto && library != f.Console {
				return needsLook(fmt.Sprintf("This library is set to %s, but this archive holds a game for %s. %s", consoleName(library), consoleName(f.Console), f.Proof), f.Console, library)
			}
			v := proven(f, hint)
			v.Proof = "Inside the archive: " + f.Proof
			return v
		case f.Kind != gameid.KindUnknown && f.Kind != gameid.KindGame:
			return leftOut(f.Kind, "Inside the archive: "+f.Proof, c.ID)
		case f.Kind == gameid.KindUnknown && main.Size > 0 && ix.peeked(p) && (mustHaveHeader[c.ID] && !opaqueExts[x]):
			return needsLook(fmt.Sprintf("Its name and folder say %s, but the file inside doesn't have the header every %s game has.", c.Name, c.Name), c.ID)
		}
	}
	switch {
	case hint == c.ID && consoles().byExtension(x) == c.ID && !extHasNoProof[x]:
		return accept(c.ID, howExt, 80, "Its folder is named for this console, and the file inside is a kind only this console uses.")
	case hint == c.ID:
		return accept(c.ID, howFolder, 70, "Its folder is named for this console, and the file inside is a kind this console uses.")
	case hint != "" && consoles().byExtension(x) == c.ID:
		if library != "" && library != wittyplay.Auto {
			return needsLook(fmt.Sprintf("This library is set to %s, but the file inside is a kind only %s uses.", consoleName(library), c.Name), c.ID, library)
		}
		return accept(c.ID, howExt, 70, fmt.Sprintf("The file inside is a kind only %s uses, whatever its folder is called.", c.Name))
	case consoles().byExtension(x) == c.ID:
		return accept(c.ID, howExt, 60, fmt.Sprintf("The file inside is a kind only %s uses.", c.Name))
	}
	return needsLook("Nothing in this archive says which console it's for, and neither does its folder.", c.ID)
}

// ---------------------------------------------------------------- remembering

// knownVerdicts reads what the last scan decided about each file.
func (ix *archiveIndex) loadVerdicts() {
	ix.verdicts = map[string]cachedVerdict{}
	ix.decisions = map[string]gameDecision{}
	if ix.store == nil {
		return
	}
	ctx := context.Background()
	rows, err := ix.store.DB.QueryContext(ctx, `SELECT path, size, mtime, version, verdict FROM game_files WHERE library_id = ?`, ix.libraryID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p, raw string
			var c cachedVerdict
			var ver int
			if rows.Scan(&p, &c.size, &c.mtime, &ver, &raw) == nil && ver == gameJudgeVersion && json.Unmarshal([]byte(raw), &c.v) == nil {
				ix.verdicts[p] = c
			}
		}
		rows.Close()
	}
	dec, err := ix.store.DB.QueryContext(ctx, `SELECT path, action, console FROM game_decisions WHERE library_id = ?`, ix.libraryID)
	if err == nil {
		defer dec.Close()
		for dec.Next() {
			var p string
			var d gameDecision
			if dec.Scan(&p, &d.Action, &d.Console) == nil {
				ix.decisions[p] = d
			}
		}
	}
}

type cachedVerdict struct {
	size, mtime int64
	v           gameVerdict
}

// gameDecision is what a person said about a file.
type gameDecision struct {
	Action  string // add, ignore, or own (never treat as a version of another)
	Console string
}

// verdictFor gives the verdict on a file, working it out only if it's new or changed.
func (ix *archiveIndex) verdictFor(p, rel, library string, size, mtime int64) gameVerdict {
	ix.verdictOnce.Do(ix.loadVerdicts)
	key := library + "|" + p
	if c, ok := ix.verdicts[key]; ok && c.size == size && c.mtime == mtime {
		ix.seenVerdicts[key] = c
		return c.v
	}
	v := ix.judge(p, rel, library, size)
	c := cachedVerdict{size: size, mtime: mtime, v: v}
	ix.seenVerdicts[key] = c
	ix.freshVerdicts[key] = true
	if len(ix.freshVerdicts) >= 200 {
		ix.saveVerdicts(context.Background())
	}
	return v
}

// saveVerdicts keeps what's been worked out, as it goes, so that a scan that's stopped
// on a huge library carries on from where it got to.
func (ix *archiveIndex) saveVerdicts(ctx context.Context) {
	if ix.store == nil || len(ix.freshVerdicts) == 0 {
		return
	}
	tx, err := ix.store.DB.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()
	for key := range ix.freshVerdicts {
		c := ix.seenVerdicts[key]
		b, err := json.Marshal(c.v)
		if err != nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO game_files(library_id, path, size, mtime, version, verdict) VALUES(?,?,?,?,?,?)
			ON CONFLICT(library_id, path) DO UPDATE SET size = excluded.size, mtime = excluded.mtime, version = excluded.version, verdict = excluded.verdict`,
			ix.libraryID, key, c.size, c.mtime, gameJudgeVersion, string(b)); err != nil {
			return
		}
	}
	if tx.Commit() == nil {
		ix.freshVerdicts = map[string]bool{}
	}
}

// forgetVerdicts drops what's kept about files that have gone. prune: every folder could
// be reached, so what wasn't seen is really gone.
func (ix *archiveIndex) forgetVerdicts(ctx context.Context) {
	if ix.store == nil {
		return
	}
	for key := range ix.verdicts {
		if _, ok := ix.seenVerdicts[key]; !ok {
			_, _ = ix.store.DB.ExecContext(ctx, `DELETE FROM game_files WHERE library_id = ? AND path = ?`, ix.libraryID, key)
		}
	}
}

// ---------------------------------------------------------------- the review list

// gameFileNote is one file a scan didn't list as a game, or listed with something to say.
type gameFileNote struct {
	path    string
	size    int64
	kind    string
	reason  string
	suggest []string
	console string
}

// noteFile adds to what this scan has to say.
func (ix *archiveIndex) noteFile(p string, size int64, kind, reason, console string, suggest []string) {
	ix.notes = append(ix.notes, gameFileNote{path: p, size: size, kind: kind, reason: reason, suggest: suggest, console: console})
}

// saveNotes replaces what the last scan of these folders had to say. A file a person has
// decided about stays decided, and shows as that.
func (s *Store) saveNotes(ctx context.Context, libraryID int64, ix *archiveIndex, keepUnreachable bool) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if !keepUnreachable {
		if _, err := tx.ExecContext(ctx, `DELETE FROM game_review WHERE library_id = ?`, libraryID); err != nil {
			return err
		}
	}
	for _, n := range ix.notes {
		b, _ := json.Marshal(n.suggest)
		if n.suggest == nil {
			b = []byte("[]")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO game_review(library_id, path, size, kind, reason, suggest, console) VALUES(?,?,?,?,?,?,?)
			ON CONFLICT(library_id, path) DO UPDATE SET size = excluded.size, kind = excluded.kind, reason = excluded.reason, suggest = excluded.suggest, console = excluded.console`,
			libraryID, n.path, n.size, n.kind, n.reason, string(b), n.console); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GameReviewItem is one line of the review list.
type GameReviewItem struct {
	ID int64 `json:"id"`
	// Path is relative to the library's folder, Name the file's own name.
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// Kind is needs (a person should look), blocked (it's listed, and can't play as it is),
	// or what it was left out as: bios, update, dlc, utility, audio, data, other, duplicate,
	// or ignored.
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	// Suggest lists consoles it might be, best first, each with a name to show.
	Suggest []GameSuggestion `json:"suggest"`
	// ItemID is the game this file is listed as, when it is.
	ItemID int64 `json:"itemId,omitempty"`
}

// GameSuggestion is a console a file might be for.
type GameSuggestion struct {
	Console string `json:"console"`
	Name    string `json:"name"`
	Short   string `json:"short"`
}

// GameReview is the review list, with how many of each kind there are.
type GameReview struct {
	Items  []GameReviewItem `json:"items"`
	Counts map[string]int   `json:"counts"`
	Total  int              `json:"total"`
}

// GameReviewList reads the list for a library. kind narrows it: "needs", "blocked" or "left"
// for everything left out.
func (s *Store) GameReviewList(ctx context.Context, lib *Library, kind string, limit, offset int) (*GameReview, error) {
	out := &GameReview{Items: []GameReviewItem{}, Counts: map[string]int{}}
	rows, err := s.DB.QueryContext(ctx, `SELECT kind, COUNT(*) FROM game_review WHERE library_id = ? GROUP BY 1`, lib.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var k string
		var n int
		if rows.Scan(&k, &n) == nil {
			out.Counts[k] = n
		}
	}
	rows.Close()
	where := `r.library_id = ?`
	args := []any{lib.ID}
	switch kind {
	case "":
	case "left":
		where += ` AND r.kind NOT IN (?, ?)`
		args = append(args, reviewNeeds, reviewBlocked)
	default:
		where += ` AND r.kind = ?`
		args = append(args, kind)
	}
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM game_review r WHERE `+where, args...).Scan(&out.Total)
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT r.id, r.path, r.size, r.kind, r.reason, r.suggest, IFNULL(i.id, 0) FROM game_review r
		LEFT JOIN items i ON i.library_id = r.library_id AND i.path = r.path AND i.missing = 0 WHERE `+where+` ORDER BY r.kind, r.path LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var it GameReviewItem
		var p, sug string
		if err := rows.Scan(&it.ID, &p, &it.Size, &it.Kind, &it.Reason, &sug, &it.ItemID); err != nil {
			return nil, err
		}
		it.Path, it.Name = relativeToLibrary(lib, p), filepath.Base(p)
		var ids []string
		_ = json.Unmarshal([]byte(sug), &ids)
		it.Suggest = suggestionsFor(ids)
		out.Items = append(out.Items, it)
	}
	return out, rows.Err()
}

// suggestionsFor names consoles, and adds the others people can pick from after them.
func suggestionsFor(ids []string) []GameSuggestion {
	out := []GameSuggestion{}
	for _, id := range ids {
		if c := consoles().byID(id); c != nil && !c.Hidden {
			out = append(out, GameSuggestion{Console: id, Name: c.Name, Short: c.Short})
		}
	}
	return out
}

func relativeToLibrary(lib *Library, p string) string {
	roots := append([]string{}, lib.Paths...)
	sort.Slice(roots, func(a, b int) bool { return len(roots[a]) > len(roots[b]) })
	for _, root := range roots {
		if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.Base(p)
}

// DecideGameFile records what a person said about a file on the list, and applies it
// on the next scan, which the caller starts. action is add, ignore or forget.
func (s *Store) DecideGameFile(ctx context.Context, lib *Library, reviewID int64, action, console string) (string, error) {
	var p string
	if err := s.DB.QueryRowContext(ctx, `SELECT path FROM game_review WHERE id = ? AND library_id = ?`, reviewID, lib.ID).Scan(&p); err != nil {
		return "", ErrNotFound
	}
	return p, s.decideGamePath(ctx, lib.ID, p, action, console)
}

func (s *Store) decideGamePath(ctx context.Context, libraryID int64, p, action, console string) error {
	if action == "forget" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM game_decisions WHERE library_id = ? AND path = ?`, libraryID, p)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO game_decisions(library_id, path, action, console, at) VALUES(?,?,?,?,strftime('%s','now'))
		ON CONFLICT(library_id, path) DO UPDATE SET action = excluded.action, console = excluded.console, at = excluded.at`, libraryID, p, action, console)
	return err
}
