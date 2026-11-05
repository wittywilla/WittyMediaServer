package server

import (
	"context"
	"crypto/md5"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	wittyplay "github.com/wittywilla/wittymediaserver/internal/play"
)

// BIOS files people keep beside their games, found by what they are rather than what they're
// called. WittyPlay's own collector (CollectBIOS) takes files whose names a core asks for; this
// one hashes files of the right sizes in BIOS folders and compares them with the checksums in
// biosknown.txt, so "PSX BIOS (USA) v4.1.bin" is found as scph5501.bin, and a file with the
// right name and the wrong contents is never used. A match is copied into the server's own
// system folder under the name its core looks for, and where it came from is kept beside it.
// Nothing is ever fetched: the table only says how to recognise firmware someone already has.

//go:embed biosknown.txt
var biosKnownText string

// knownBIOS is one file the table recognises.
type knownBIOS struct {
	MD5     string
	Size    int64
	Console string
	Dest    string // under the system folder, with forward slashes
	Region  string // usa, europe, japan, ... or "" when it doesn't matter
}

type biosTable struct {
	byMD5 map[string]knownBIOS
	sizes map[int64]bool
}

var biosKnown = parseBIOSTable(biosKnownText)

func parseBIOSTable(text string) biosTable {
	t := biosTable{byMD5: map[string]knownBIOS{}, sizes: map[int64]bool{}}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) != 5 || strings.HasPrefix(f[0], "#") {
			continue
		}
		size, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil {
			continue
		}
		k := knownBIOS{MD5: f[0], Size: size, Console: f[2], Dest: strings.ReplaceAll(f[3], "%20", " "), Region: f[4]}
		if k.Region == "-" {
			k.Region = ""
		}
		t.byMD5[k.MD5] = k
		t.sizes[size] = true
	}
	return t
}

// How far to look and how much to read. The folders are someone else's, and may be large.
const (
	biosWalkDepth = 3
	biosWalkFiles = 512
	biosMaxBytes  = 32 << 20
	// biosRecordFile is beside the firmware, in the system folder: what came from where.
	biosRecordFile = ".bios-found.json"
	// biosSweepEvery is the least time between two looks through every games library.
	biosSweepEvery = 10 * time.Minute
)

// biosRecord says where a BIOS in the system folder came from.
type biosRecord struct {
	File    string `json:"file"` // under the system folder, with forward slashes
	Console string `json:"console"`
	From    string `json:"from"` // the file it was copied from, or found as
	MD5     string `json:"md5"`
	Region  string `json:"region,omitempty"`
	At      int64  `json:"at"`
}

// biosFinder remembers what it has hashed, so a folder looked at again costs a directory read.
type biosFinder struct {
	mu        sync.Mutex
	hashes    map[string]hashedFile // by path
	sweeping  bool
	lastSweep time.Time
}

type hashedFile struct {
	size  int64
	mtime time.Time
	md5   string
}

func (b *biosFinder) hash(path string, st os.FileInfo) string {
	b.mu.Lock()
	h, ok := b.hashes[path]
	b.mu.Unlock()
	if ok && h.size == st.Size() && h.mtime.Equal(st.ModTime()) {
		return h.md5
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sum := md5.New()
	if _, err := io.Copy(sum, io.LimitReader(f, biosMaxBytes)); err != nil {
		return ""
	}
	m := hex.EncodeToString(sum.Sum(nil))
	b.mu.Lock()
	if b.hashes == nil {
		b.hashes = map[string]hashedFile{}
	}
	b.hashes[path] = hashedFile{st.Size(), st.ModTime(), m}
	b.mu.Unlock()
	return m
}

// biosFoldersNear are the BIOS folders beside a game, and beside its folder: a console's
// folder usually holds both its games and its BIOS folder.
func biosFoldersNear(gamePath string) []string {
	dir := filepath.Dir(gamePath)
	return biosFoldersIn(dir, filepath.Dir(dir))
}

// biosFoldersIn are folders named BIOS, in any capitals, directly in these folders.
func biosFoldersIn(dirs ...string) []string {
	var out []string
	for _, dir := range dirs {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() && strings.EqualFold(e.Name(), wittyplay.BIOSFolderName) {
				p := filepath.Join(dir, e.Name())
				if !slices.Contains(out, p) {
					out = append(out, p)
				}
			}
		}
	}
	return out
}

// biosFoldersInLibrary are the BIOS folders at a library's roots and one folder down, where a
// library of many consoles keeps one folder for each.
func biosFoldersInLibrary(roots []string) []string {
	var dirs []string
	for _, root := range roots {
		dirs = append(dirs, root)
		if ents, err := os.ReadDir(root); err == nil {
			for _, e := range ents {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					dirs = append(dirs, filepath.Join(root, e.Name()))
				}
			}
		}
	}
	return biosFoldersIn(dirs...)
}

// findBIOS looks through BIOS folders for files the table knows, and puts each into the system
// folder. It says how many it put there; files already there count when they came from here.
func (g *gameService) findBIOS(table biosTable, folders []string) int {
	put, seen := 0, 0
	for _, folder := range folders {
		g.walkBIOS(table, folder, 0, &seen, &put)
	}
	return put
}

func (g *gameService) walkBIOS(table biosTable, dir string, depth int, seen, put *int) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			if depth < biosWalkDepth && !strings.HasPrefix(e.Name(), ".") {
				g.walkBIOS(table, p, depth+1, seen, put)
			}
			continue
		}
		if *seen >= biosWalkFiles {
			return
		}
		*seen++
		if !e.Type().IsRegular() {
			continue
		}
		st, err := e.Info()
		if err != nil || !table.sizes[st.Size()] {
			continue
		}
		k, ok := table.byMD5[g.bios.hash(p, st)]
		if !ok {
			continue
		}
		if g.installBIOS(k, p) {
			*put++
		}
	}
}

// installBIOS puts one recognised file where its core looks, unless the same firmware is there
// already, under that name or another one in the same folder. A different file under that
// name is left alone: it's someone's own, and wasn't put there by this.
func (g *gameService) installBIOS(k knownBIOS, from string) bool {
	sys := g.systemDir()
	dest := filepath.Join(sys, filepath.FromSlash(k.Dest))
	record := func(file string) {
		rel, err := filepath.Rel(sys, file)
		if err != nil {
			return
		}
		g.noteBIOS(biosRecord{File: filepath.ToSlash(rel), Console: k.Console, From: from, MD5: k.MD5, Region: k.Region, At: time.Now().Unix()})
	}
	if st, err := os.Stat(dest); err == nil {
		if g.bios.hash(dest, st) == k.MD5 {
			record(dest)
		}
		return false
	}
	if same := g.sameBIOSIn(filepath.Dir(dest), k); same != "" {
		record(same)
		return false
	}
	if err := copyFileAtomic(from, dest, biosMaxBytes); err != nil {
		return false
	}
	record(dest)
	return true
}

// sameBIOSIn finds a file in a folder that is this firmware under another name, such as one
// WittyPlay copied by its name.
func (g *gameService) sameBIOSIn(dir string, k knownBIOS) string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		st, err := e.Info()
		if err != nil || !e.Type().IsRegular() || st.Size() != k.Size {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if g.bios.hash(p, st) == k.MD5 {
			return p
		}
	}
	return ""
}

func copyFileAtomic(from, to string, limit int64) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(to), ".bios-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, io.LimitReader(in, limit)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), to)
}

// biosRecords are where the firmware in the system folder came from, by file.
func (g *gameService) biosRecords() map[string]biosRecord {
	g.bios.mu.Lock()
	defer g.bios.mu.Unlock()
	return g.readBIOSRecords()
}

func (g *gameService) readBIOSRecords() map[string]biosRecord {
	out := map[string]biosRecord{}
	b, err := os.ReadFile(filepath.Join(g.systemDir(), biosRecordFile))
	if err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func (g *gameService) noteBIOS(r biosRecord) {
	g.bios.mu.Lock()
	defer g.bios.mu.Unlock()
	all := g.readBIOSRecords()
	if old, ok := all[r.File]; ok && old.From == r.From && old.MD5 == r.MD5 {
		return
	}
	all[r.File] = r
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return
	}
	path := filepath.Join(g.systemDir(), biosRecordFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
}

// findBIOSNear looks beside a game, when it's checked or started.
func (s *Server) findBIOSNear(gamePath string) {
	s.games().findBIOS(biosKnown, biosFoldersNear(gamePath))
}

// sweepBIOS looks through every games library's BIOS folders, in the background, after a scan
// or when Settings shows what's there. One at a time, and not again for a while.
func (s *Server) sweepBIOS(force bool) {
	g := s.games()
	g.bios.mu.Lock()
	if g.bios.sweeping || (!force && time.Since(g.bios.lastSweep) < biosSweepEvery) {
		g.bios.mu.Unlock()
		return
	}
	g.bios.sweeping, g.bios.lastSweep = true, time.Now()
	g.bios.mu.Unlock()
	go func() {
		defer func() {
			g.bios.mu.Lock()
			g.bios.sweeping = false
			g.bios.mu.Unlock()
		}()
		libs, err := s.Store.Libraries(context.Background())
		if err != nil {
			return
		}
		var roots []string
		for _, l := range libs {
			if l.Type == "games" {
				roots = append(roots, l.Paths...)
			}
		}
		if n := g.findBIOS(biosKnown, biosFoldersInLibrary(roots)); n > 0 {
			s.Log.Info("BIOS files found by their checksums", "cat", "games", "copied", n)
		}
	}()
}

// biosRegion is a BIOS file's region: what it was recognised as, or what its name says.
func biosRegion(name string, rec biosRecord, known bool) string {
	if known && rec.Region != "" {
		return rec.Region
	}
	n := strings.ToLower(name)
	for _, w := range []struct{ word, region string }{
		{"usa", "usa"}, {"(u)", "usa"}, {"ntsc-u", "usa"},
		{"eur", "europe"}, {"_uk_", "europe"}, {"pal", "europe"},
		{"jap", "japan"}, {"jpn", "japan"}, {"(j)", "japan"},
		{"_hk_", "asia"}, {"asia", "asia"}, {"chn", "china"}, {"rus", "russia"},
	} {
		if strings.Contains(n, w.word) {
			return w.region
		}
	}
	return ""
}

// gameRegion is where a game was sold, from its disc's serial or its file's name: "" when
// neither says.
func gameRegion(serial, name string) string {
	s := strings.ToUpper(strings.TrimSpace(serial))
	switch {
	case strings.HasPrefix(s, "SLUS"), strings.HasPrefix(s, "SCUS"), strings.HasPrefix(s, "PBPX"):
		return "usa"
	case strings.HasPrefix(s, "SLES"), strings.HasPrefix(s, "SCES"), strings.HasPrefix(s, "SCED"):
		return "europe"
	case strings.HasPrefix(s, "SLPS"), strings.HasPrefix(s, "SLPM"), strings.HasPrefix(s, "SCPS"), strings.HasPrefix(s, "SCAJ"), strings.HasPrefix(s, "SLAJ"):
		return "japan"
	case strings.HasPrefix(s, "SCKA"), strings.HasPrefix(s, "SLKA"):
		return "asia"
	}
	n := strings.ToLower(name)
	for _, w := range []struct{ word, region string }{{"(usa", "usa"}, {"(europe", "europe"}, {"(japan", "japan"}, {"(korea", "asia"}, {"(asia", "asia"}} {
		if strings.Contains(n, w.word) {
			return w.region
		}
	}
	return ""
}

// pickRegionBIOS chooses among a core's BIOS files for a game from one region: the one it
// would take anyway if that's the game's region or the region isn't known, else one from the
// game's region, a first-generation late model where there is one (they run the most), else
// the first by name. regionOf says each file's region.
func pickRegionBIOS(files []string, current, region string, regionOf func(string) string) string {
	if region == "" || current == "" || regionOf(current) == region {
		return current
	}
	var match []string
	for _, f := range files {
		if regionOf(f) == region {
			match = append(match, f)
		}
	}
	if len(match) == 0 {
		return current
	}
	slices.Sort(match)
	for _, f := range match {
		if strings.Contains(f, "3900") || strings.Contains(f, "-0160") {
			return f
		}
	}
	return match[0]
}

// regionBIOSFor is the BIOS a game starts a core with, of those in folder: one from the game's
// own region where there is one, and otherwise current, the one the core would take anyway.
// PlayStation 2 is the console this matters for: LRPS2 takes any region's BIOS.
func (s *Server) regionBIOSFor(ctx context.Context, it *library.Item, folder, current string) string {
	serial := ""
	if d, err := s.Store.GameDetails(ctx, it.ID); err == nil && d != nil {
		serial = d.Serial
	}
	region := gameRegion(serial, filepath.Base(it.Path))
	dir := filepath.Join(s.games().systemDir(), filepath.FromSlash(folder))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return current
	}
	records := s.games().biosRecords()
	var files []string
	for _, e := range ents {
		if st, err := e.Info(); err == nil && e.Type().IsRegular() && st.Size() >= 4<<20 && st.Size() <= 5<<20 {
			files = append(files, e.Name())
		}
	}
	return pickRegionBIOS(files, current, region, func(name string) string {
		rec, ok := records[filepath.ToSlash(filepath.Join(folder, name))]
		return biosRegion(name, rec, ok)
	})
}

// biosFileView is a BIOS file in the system folder, as Settings shows it.
type biosFileView struct {
	File   string `json:"file"`             // under the system folder
	From   string `json:"from,omitempty"`   // where it was found, when it was found by its checksum
	Region string `json:"region,omitempty"` // usa, europe, japan, ...
	Using  bool   `json:"using,omitempty"`  // what the console's core starts with, unless a game's region says otherwise
}

// biosFilesFor lists a console's firmware in the system folder that came from a games folder,
// and the file its core starts with.
func biosFilesFor(console, systemDir string, records map[string]biosRecord, using string) []biosFileView {
	var out []biosFileView
	for _, r := range records {
		if r.Console != console {
			continue
		}
		if _, err := os.Stat(filepath.Join(systemDir, filepath.FromSlash(r.File))); err != nil {
			continue
		}
		out = append(out, biosFileView{File: r.File, From: r.From, Region: r.Region, Using: using != "" && r.File == using})
	}
	if using != "" && !slices.ContainsFunc(out, func(v biosFileView) bool { return v.Using }) {
		out = append(out, biosFileView{File: using, Region: biosRegion(filepath.Base(using), biosRecord{}, false), Using: true})
	}
	slices.SortFunc(out, func(a, b biosFileView) int { return strings.Compare(a.File, b.File) })
	return out
}

// biosRuleMet says a console has a core whose required BIOS is there, under whatever name its
// rule accepts.
func biosRuleMet(console *wittyplay.Console, systemDir string) bool {
	for _, c := range console.Cores {
		if wittyplay.Traits(c.ID).RequiredBIOS != nil && wittyplay.MissingBIOS(c.ID, systemDir) == "" {
			return true
		}
	}
	return false
}

// defaultBIOS is the file a console's core with a required BIOS starts with, under the system
// folder, unless a game's region chooses another: "" when there's none, or no such core.
func defaultBIOS(console *wittyplay.Console, systemDir string) string {
	for _, c := range console.Cores {
		rule := wittyplay.Traits(c.ID).RequiredBIOS
		if rule == nil || rule.Option == "" || wittyplay.MissingBIOS(c.ID, systemDir) != "" {
			continue
		}
		if name := wittyplay.StartOptions(console.ID, c.ID, "", systemDir)[rule.Option]; name != "" {
			return filepath.ToSlash(filepath.Join(rule.Folder, name))
		}
	}
	return ""
}

// biosInUse is, for a game, the BIOS file its core with a required BIOS would start with, by
// name within the rule's folder.
func (s *Server) biosInUse(ctx context.Context, console *wittyplay.Console, it *library.Item, systemDir string) (*wittyplay.BIOSRule, string) {
	for _, c := range console.Cores {
		rule := wittyplay.Traits(c.ID).RequiredBIOS
		if rule == nil || rule.Option == "" || wittyplay.MissingBIOS(c.ID, systemDir) != "" {
			continue
		}
		current := wittyplay.StartOptions(console.ID, c.ID, "", systemDir)[rule.Option]
		return rule, s.regionBIOSFor(ctx, it, rule.Folder, current)
	}
	return nil, ""
}
