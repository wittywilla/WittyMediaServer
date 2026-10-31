package diagnose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	fb "github.com/wittywilla/wittyintegratedfilebrowser"

	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// Storage checks look at a library folder or one of the server's own folders. A
// target is only ever chosen by its ID, so a request can't name a path. They read,
// and only the write test writes: one small temp file in one of the server's own
// folders, never in or around a library, always removed.

// StorageTarget is a folder to check.
type StorageTarget struct {
	ID        string
	Role      string // library, data, cache, transcode, images or logs
	Label     string // what to call it
	Plain     string // what to call it with the owner's names taken out
	Path      string
	LibraryID int64
}

// StorageOptions choose the parts of a storage check.
type StorageOptions struct {
	ReadTest  bool
	WriteTest bool
	Walk      bool
	// Libraries lists every library folder: the write test stays out of them,
	// whichever folder it's asked to write in.
	Libraries []string
}

// Limits on a check.
const (
	maxWalkEntries  = 200000
	walkBudget      = 2 * time.Minute
	pickEntries     = 5000 // without a walk, how far to look for files to read
	pickBudget      = 15 * time.Second
	maxExamples     = 50
	maxItemChecks   = 20000
	speedBytes      = 256 << 20
	speedBudget     = 15 * time.Second
	speedRandom     = 32
	writeProbeBytes = 32 << 20
	writeTempPrefix = ".wms-diag-"
	gib             = int64(1) << 30
	// An hour of 1080p from the transcoder takes about this much room.
	transcodeHourBytes = 4 * gib
	// How fast streams read, in bits a second.
	remuxBits  = 100e6
	blurayBits = 40e6
	hdBits     = 10e6
)

// writeRoles are the folders that may hold the write test's temp file.
var writeRoles = map[string]bool{"data": true, "transcode": true, "cache": true, "logs": true, "images": true}

// SubmitStorage queues a check of some folders.
func (m *Manager) SubmitStorage(targets []StorageTarget, o StorageOptions) (*Job, error) {
	scrub := make([]scrubRule, 0, len(targets))
	for _, t := range targets {
		scrub = append(scrub, scrubRule{path: t.Path, with: "this folder"})
	}
	opts := map[string]bool{"readTest": o.ReadTest, "writeTest": o.WriteTest, "walk": o.Walk}
	subject := plural(len(targets), "folder", "folders")
	return m.Submit(KindStorage, subject, opts, scrub, func(ctx context.Context, j *Job) {
		health := map[string]*healthResult{}
		for i, t := range targets {
			if ctx.Err() != nil {
				return
			}
			c := &storageCheck{ctx: ctx, j: j, t: t, o: o, from: float64(i) / float64(len(targets)), span: 1 / float64(len(targets)), health: health}
			j.Engine("checking a folder", c.run)
		}
	})
}

type storageCheck struct {
	ctx    context.Context
	j      *Job
	t      StorageTarget
	o      StorageOptions
	from   float64
	span   float64
	fs     sysinfo.FS
	health map[string]*healthResult
}

func (c *storageCheck) add(f Finding) {
	f.Target, f.TargetPlain = c.t.Label, c.t.Plain
	c.j.Add(f)
}

func (c *storageCheck) step(what string, part float64) {
	c.j.Step(c.t.Label+": "+what, c.from+c.span*part)
}

func (c *storageCheck) run() {
	t := c.t
	library := t.Role == "library"
	c.step("looking at the folder", 0.02)
	st, err := os.Stat(t.Path)
	if err != nil {
		f := Finding{Severity: SevProblem, Title: "This folder can't be reached", What: "The server got this answer: " + plainErr(err) + "."}
		if errors.Is(err, fs.ErrNotExist) {
			f.What = "It isn't there. If it's on another drive, that drive may not be connected."
		}
		f.Fix = "Check that the drive is connected and that the folder wasn't moved or renamed."
		if library {
			f.What += " Items in this library are kept while their folder is away, and come back with it."
		}
		c.add(f)
		return
	}
	if !st.IsDir() {
		c.add(Finding{Severity: SevProblem, Title: "This isn't a folder", What: "A file is where the folder should be.", Fix: "Point the server at the folder that holds it."})
		return
	}
	info, ferr := sysinfo.FSInfo(t.Path)
	c.fs = info
	if ferr != nil {
		c.add(Finding{Severity: SevInfo, Title: "The file system couldn't be identified", What: "The system's answer: " + plainErr(ferr) + ". The checks that depend on it are left out."})
	} else {
		c.fsFinding()
	}
	c.spaceFinding()
	c.step("trying to read it", 0.08)
	c.readable()
	if c.ctx.Err() != nil {
		return
	}
	var walk *walkResult
	if c.o.Walk {
		c.step("going through every file", 0.1)
		walk = c.walk(maxWalkEntries, walkBudget, true)
		c.walkFindings(walk)
	}
	if c.ctx.Err() != nil {
		return
	}
	if library && c.o.Walk {
		c.step("looking for items that have gone", 0.45)
		c.missingItems()
	}
	if c.ctx.Err() != nil {
		return
	}
	if c.o.ReadTest {
		c.step("measuring how fast it reads", 0.55)
		pick := walk
		if pick == nil {
			pick = c.walk(pickEntries, pickBudget, false)
		}
		c.readSpeed(pick.largest)
	}
	if c.ctx.Err() != nil {
		return
	}
	if c.o.WriteTest {
		c.step("trying a small write", 0.82)
		c.writeTest()
	}
	if c.ctx.Err() != nil {
		return
	}
	c.step("asking about the drive's health", 0.92)
	c.healthFinding()
}

func (c *storageCheck) fsFinding() {
	f, role := c.fs, c.t.Role
	kind := map[string]string{"fixed": "a fixed drive", "removable": "a removable drive", "sdcard": "an SD card", "network": "a network drive", "optical": "an optical disc", "ram": "memory"}[f.Kind]
	if kind == "" {
		kind = "a drive of a kind the system didn't name"
	}
	name := strings.ToUpper(f.Type)
	if name == "" {
		name = "An unnamed file system"
	}
	line := name + " on " + kind
	det := []string{}
	if f.ReadOnly {
		det = append(det, "Mounted read-only")
	}
	c.add(Finding{Severity: SevInfo, Title: line, Details: det})

	own := role != "library"
	switch {
	case f.Network() && role == "data":
		c.add(Finding{Severity: SevProblem, Title: "The server's database is on a network drive", What: "The database needs file locks that network drives don't keep reliably. It can slow to a crawl, or be damaged.", Fix: "Strongly advised: move the data folder to a drive inside this computer (see Config, the data folder)."})
	case f.Network() && role == "transcode":
		c.add(Finding{Severity: SevWarn, Title: "The transcode folder is on a network drive", What: "Video being converted is written here and read back right away, and that's slow over a network: playback may stall.", Fix: "Move it to a drive inside this computer (see Config, transcodeDir)."})
	case f.Network() && own:
		c.add(Finding{Severity: SevInfo, Title: "This folder is on a network drive", What: "It works, but more slowly than a drive inside this computer."})
	case (f.Kind == "removable" || f.Kind == "sdcard") && role == "transcode":
		c.add(Finding{Severity: SevWarn, Title: "The transcode folder is on a removable drive or SD card", What: "Converting video writes a lot, all the time. Cards and small USB drives are slow at it and wear out early.", Fix: "Move it to a drive inside this computer (see Config, transcodeDir)."})
	case (f.Kind == "removable" || f.Kind == "sdcard") && role == "data":
		c.add(Finding{Severity: SevWarn, Title: "The data folder is on a removable drive or SD card", What: "If it's unplugged, the server loses its database while it's running. Cards are also slow at what a database asks of them.", Fix: "Move it to a drive inside this computer."})
	case f.Kind == "ram" && role == "data":
		c.add(Finding{Severity: SevProblem, Title: "The data folder is held in memory", What: "Your database is gone at the next restart.", Fix: "Move it to a real drive."})
	}
	if f.Mapped {
		sev, what := SevWarn, "A drive letter for a share only exists for the person who mapped it. A server run as a service can't see it, and neither can it after a restart, before someone signs in."
		c.add(Finding{Severity: sev, Title: "This is a mapped drive letter", What: what, Fix: `Use the share's own address instead, in the form \\server\share\folder.`})
	}
	if f.ReadOnly {
		if own {
			c.add(Finding{Severity: SevProblem, Title: "This folder can't be written to", What: "It's on a read-only file system, and the server needs to save things here.", Fix: "Mount it read-write, or move the folder."})
		} else {
			c.add(Finding{Severity: SevInfo, Title: "This library is read-only", What: "That's fine for watching. Anything saved next to the media, such as artwork or metadata, won't work.", Fix: "Nothing, unless you want those saved beside the files."})
		}
	}
	switch {
	case f.Type == "fat32" || f.Type == "vfat" || f.Type == "msdos" || f.Type == "fat":
		c.add(Finding{Severity: SevWarn, Title: "FAT32 can't hold files of 4 GB or more", What: "A disc image or a film at good quality is usually bigger. A copy to this drive fails or gets cut short at 4 GB.", Fix: "Use the drive with NTFS, exFAT or ext4. That means emptying it first, so copy everything off before you change it."})
	case (f.Type == "exfat") && role == "data":
		c.add(Finding{Severity: SevWarn, Title: "The data folder is on exFAT", What: "exFAT keeps no journal, so a power cut or an unplugged drive can damage the database.", Fix: "Prefer NTFS, ext4, or another journaling file system for the data folder."})
	case (f.Type == "ntfs3" || f.Type == "ntfs" || f.Type == "fuseblk") && runtime.GOOS != "windows" && role == "data":
		c.add(Finding{Severity: SevWarn, Title: "The data folder is on NTFS, from Linux", What: "Locks and speed on this driver aren't what a database wants.", Fix: "Prefer ext4 or XFS for the data folder."})
	case f.Type == "overlay" && own:
		c.add(Finding{Severity: SevWarn, Title: "This folder is inside the container", What: "What's in it goes away when the container is replaced.", Fix: "Mount a volume here, as the sample compose file does."})
	case strings.HasPrefix(f.Type, "fuse.") && role == "data":
		c.add(Finding{Severity: SevWarn, Title: "The data folder is on a user-space file system", What: "These often don't keep file locks as a database needs. Damage to the database is possible.", Fix: "Move the data folder to a plain local drive."})
	case strings.HasPrefix(f.Type, "fuse."):
		c.add(Finding{Severity: SevInfo, Title: "This folder is on a user-space file system", What: "These add some delay to each read: fine for most uses, and seen most when jumping around in a film."})
	}
}

func (c *storageCheck) spaceFinding() {
	d, err := sysinfo.DiskUsage(c.t.Path)
	if err != nil {
		c.add(Finding{Severity: SevInfo, Title: "Free space couldn't be read", What: "The system's answer: " + plainErr(err) + "."})
		return
	}
	free, total := d.Free, d.Total
	pct := 0.0
	if total > 0 {
		pct = 100 * float64(free) / float64(total)
	}
	line := fmt.Sprintf("%s free of %s (%.0f%%)", bytesText(free), bytesText(total), pct)
	transcodes := c.t.Role == "transcode" || c.t.Role == "data" || c.t.Role == "cache"
	hours := float64(free) / float64(transcodeHourBytes)
	switch {
	case transcodes && free < gib:
		c.add(Finding{Severity: SevProblem, Title: "This drive is almost full", What: line + ". Converting video will fail here, and the database can't grow.", Fix: "Free some space, or move this folder to a drive with more."})
	case transcodes && free < 10*gib:
		c.add(Finding{Severity: SevWarn, Title: "There's little room for converting video", What: fmt.Sprintf("%s. That's about %.1f hours of 1080p being converted, for everyone watching together.", line, hours), Fix: "Free some space, or move this folder to a drive with more."})
	case total > 0 && pct < 5:
		c.add(Finding{Severity: SevWarn, Title: "This drive is nearly full", What: line + ". Drives slow down when this full, and new files may not fit.", Fix: "Free some space."})
	case transcodes:
		c.add(Finding{Severity: SevOK, Title: "There's plenty of room", What: fmt.Sprintf("%s. That's about %.0f hours of 1080p being converted.", line, hours)})
	default:
		c.add(Finding{Severity: SevOK, Title: "There's room on this drive", What: line + "."})
	}
}

// readable lists the folder and opens a few files from it.
func (c *storageCheck) readable() {
	ents, err := os.ReadDir(c.t.Path)
	if err != nil {
		c.add(Finding{Severity: SevProblem, Title: "The server can't open this folder", What: "The system's answer: " + plainErr(err) + ".", Fix: "Give the account the server runs as permission to read it. If it runs as a service, that's a different account from yours."})
		return
	}
	tried, failed := 0, 0
	var names []string
	var lastErr error
	for _, e := range ents {
		if tried >= 20 {
			break
		}
		if !e.Type().IsRegular() {
			continue
		}
		tried++
		h, err := os.Open(filepath.Join(c.t.Path, e.Name()))
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				failed++
				lastErr = err
				names = append(names, e.Name())
			}
			continue
		}
		_ = h.Close() // only opened
	}
	if failed > 0 {
		c.add(Finding{Severity: SevWarn, Title: "Some files here can't be opened", What: fmt.Sprintf("%d of the %d tried. The system's answer: %s.", failed, tried, plainErr(lastErr)), Fix: "Give the account the server runs as permission to read them.", Names: names})
		return
	}
	c.add(Finding{Severity: SevOK, Title: "The server can read this folder", What: fmt.Sprintf("It listed %s and opened %s.", plural(len(ents), "entry", "entries"), plural(tried, "file", "files"))})
}

type bigFile struct {
	path string
	size int64
}

// walkResult is what going through a folder found. It holds counts and a few
// names, however large the folder is.
type walkResult struct {
	entries, files, dirs         int
	bytes                        int64
	unreadable, zero, long, odd  int
	unreadableDirs, nearFATLimit int
	namesUnreadable, namesZero   []string
	namesLong, namesOdd          []string
	largest                      []bigFile
	stoppedAt                    string
}

func keepName(list []string, name string) []string {
	if len(list) < maxExamples {
		return append(list, name)
	}
	return list
}

// oddName says what may trip up FFmpeg or other tools about a file name. "" if
// nothing does.
func oddName(name string) string {
	if !utf8.ValidString(name) {
		return "isn't valid text"
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "has a control character in it"
		}
		if r == '\uFFFD' {
			return "has a broken character in it"
		}
	}
	if strings.TrimSpace(name) != name {
		return "starts or ends with a space"
	}
	if strings.HasSuffix(name, ".") {
		return "ends with a dot"
	}
	if runtime.GOOS != "windows" && strings.ContainsAny(name, `:*?"<>|\`) {
		return `has a character Windows can't use, or that tools treat as special`
	}
	return ""
}

func longWindowsPath(p string) bool {
	if runtime.GOOS != "windows" || len(p) <= 260 {
		return false
	}
	n := 0
	for _, r := range p {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n > 260
}

func isMediaName(name string) bool {
	switch fb.FileKind(name) {
	case "video", "audio", "disc":
		return true
	}
	return false
}

// walk goes through the folder, bounded. inspect also opens every file and
// collects what's odd; without it, it only looks for the largest files.
func (c *storageCheck) walk(maxEntries int, budget time.Duration, inspect bool) *walkResult {
	r := &walkResult{}
	began := time.Now()
	chunk := time.Now()
	var anyFiles []bigFile
	note := func(list []bigFile, p string, size int64) []bigFile {
		list = append(list, bigFile{p, size})
		slices.SortFunc(list, func(a, b bigFile) int {
			switch {
			case a.size > b.size:
				return -1
			case a.size < b.size:
				return 1
			}
			return strings.Compare(a.path, b.path)
		})
		if len(list) > 3 {
			list = list[:3]
		}
		return list
	}
	err := filepath.WalkDir(c.t.Path, func(p string, e fs.DirEntry, err error) error {
		if ctxErr := c.ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				r.unreadableDirs++
				r.namesUnreadable = keepName(r.namesUnreadable, filepath.Base(p))
			}
			return nil
		}
		if p == c.t.Path {
			return nil
		}
		name := e.Name()
		if fb.IsHidden(name) {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		r.entries++
		if r.entries >= maxEntries {
			r.stoppedAt = plural(maxEntries, "entry", "entries")
			return fs.SkipAll
		}
		if r.entries%256 == 0 {
			if time.Since(began) > budget {
				r.stoppedAt = budget.String()
				return fs.SkipAll
			}
			c.j.Progress(c.from + c.span*(0.1+0.3*float64(r.entries)/float64(maxEntries)))
			if !c.j.Rest(c.ctx, time.Since(chunk)) {
				return c.ctx.Err()
			}
			chunk = time.Now()
		}
		if inspect {
			if why := oddName(name); why != "" {
				r.odd++
				r.namesOdd = keepName(r.namesOdd, name+": "+why)
			}
			if longWindowsPath(p) {
				r.long++
				r.namesLong = keepName(r.namesLong, name)
			}
		}
		if e.IsDir() {
			r.dirs++
			return nil
		}
		if !e.Type().IsRegular() {
			return nil
		}
		info, err := e.Info()
		if err != nil {
			if inspect && !errors.Is(err, fs.ErrNotExist) {
				r.unreadable++
				r.namesUnreadable = keepName(r.namesUnreadable, name)
			}
			return nil
		}
		r.files++
		size := info.Size()
		r.bytes += size
		if isMediaName(name) {
			if len(r.largest) < 3 || size > r.largest[len(r.largest)-1].size {
				r.largest = note(r.largest, p, size)
			}
		} else if len(anyFiles) < 3 || size > anyFiles[len(anyFiles)-1].size {
			anyFiles = note(anyFiles, p, size)
		}
		if !inspect {
			return nil
		}
		if size == 0 {
			r.zero++
			r.namesZero = keepName(r.namesZero, name)
		}
		if size >= 4*gib-(1<<20) && size < 4*gib {
			r.nearFATLimit++
		}
		h, err := os.Open(p)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				r.unreadable++
				r.namesUnreadable = keepName(r.namesUnreadable, name)
			}
			return nil
		}
		_ = h.Close() // only opened
		return nil
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		c.j.log.Debug("walk stopped", "err", err)
	}
	if len(r.largest) == 0 {
		r.largest = anyFiles // a folder of the server's own files has no media to read
	}
	return r
}

func (c *storageCheck) walkFindings(r *walkResult) {
	if c.ctx.Err() != nil {
		return
	}
	library := c.t.Role == "library"
	c.add(Finding{Severity: SevInfo, Title: "What's in this folder", What: fmt.Sprintf("%s in %s, %s in all.", plural(r.files, "file", "files"), plural(r.dirs, "folder", "folders"), bytesText(r.bytes))})
	if r.stoppedAt != "" {
		c.add(Finding{Severity: SevInfo, Title: "Not every file was looked at", What: "The walk stopped at its limit of " + r.stoppedAt + ", so the numbers here are for the part it saw."})
	}
	if n := r.unreadable + r.unreadableDirs; n > 0 {
		c.add(Finding{Severity: SevWarn, Title: plural(n, "file or folder can't be opened", "files or folders can't be opened"), What: "They won't play or show up. It's usually a matter of permissions, or a file still being copied.", Fix: "Give the account the server runs as permission to read them.", Names: r.namesUnreadable, More: n - len(r.namesUnreadable)})
	}
	if r.zero > 0 && library {
		c.add(Finding{Severity: SevWarn, Title: plural(r.zero, "file is empty", "files are empty"), What: "A size of zero usually means a copy or a download that failed.", Fix: "Copy those files again.", Names: r.namesZero, More: r.zero - len(r.namesZero)})
	}
	if r.long > 0 {
		c.add(Finding{Severity: SevWarn, Title: plural(r.long, "path is too long for some programs", "paths are too long for some programs"), What: "These are over 260 characters. The server copes, but FFmpeg and many other Windows programs may not open them.", Fix: "Shorten the folder or file names.", Names: r.namesLong, More: r.long - len(r.namesLong)})
	}
	if r.odd > 0 {
		c.add(Finding{Severity: SevWarn, Title: plural(r.odd, "name has characters that may cause trouble", "names have characters that may cause trouble"), What: "FFmpeg and other tools may fail on these. Renaming them is enough.", Fix: "Rename them to use plain letters, digits and spaces.", Names: r.namesOdd, More: r.odd - len(r.namesOdd)})
	}
	if r.nearFATLimit > 0 {
		c.add(Finding{Severity: SevWarn, Title: plural(r.nearFATLimit, "file is just under 4 GB", "files are just under 4 GB"), What: "That's how a file looks when it was cut short by a drive that can't hold more (FAT32).", Fix: "Check that it plays to the end. If it doesn't, copy it again from a drive that isn't FAT32."})
	}
	if r.unreadable+r.unreadableDirs+r.long+r.odd == 0 && (r.zero == 0 || !library) {
		c.add(Finding{Severity: SevOK, Title: "Every file can be opened", What: "None has a name or path likely to cause trouble."})
	}
}

// missingItems looks for what the database knows but the disk doesn't have.
func (c *storageCheck) missingItems() {
	env := c.j.env
	if env.ItemPaths == nil || c.t.LibraryID == 0 {
		return
	}
	paths, err := env.ItemPaths(c.ctx, c.t.LibraryID, maxItemChecks)
	if err != nil {
		if c.ctx.Err() == nil {
			c.j.log.Warn("could not list a library's items", "library", c.t.LibraryID, "err", err)
		}
		return
	}
	checked, missing := 0, 0
	var names []string
	chunk := time.Now()
	for _, p := range paths {
		if !filepath.IsAbs(p) || !Within(c.t.Path, p) {
			continue
		}
		checked++
		if _, err := os.Stat(p); err != nil && errors.Is(err, fs.ErrNotExist) {
			missing++
			names = keepName(names, filepath.Base(p))
		}
		if checked%256 == 0 {
			if !c.j.Rest(c.ctx, time.Since(chunk)) {
				return
			}
			chunk = time.Now()
		}
	}
	switch {
	case missing > 0:
		c.add(Finding{Severity: SevWarn, Title: plural(missing, "item in the library is missing from this folder", "items in the library are missing from this folder"), What: "The library knows these, but they aren't on the disk any more: moved, renamed or deleted.", Fix: "Put them back, or scan the library to forget them.", Names: names, More: missing - len(names)})
	case checked > 0:
		c.add(Finding{Severity: SevOK, Title: "Everything the library knows is here", What: plural(checked, "item", "items") + " checked."})
	}
}

// readSpeed reads from the largest files, from random places, at most so much for
// so long. It's a read: nothing changes. Time spent resting for playback doesn't
// count against the result.
func (c *storageCheck) readSpeed(files []bigFile) {
	var use []bigFile
	for _, f := range files {
		if f.size >= 1<<20 {
			use = append(use, f)
		}
	}
	if len(use) == 0 {
		c.add(Finding{Severity: SevInfo, Title: "Read speed wasn't measured", What: "There's no file here big enough to measure with."})
		return
	}
	buf := make([]byte, 1<<20)
	var total int64
	var spent time.Duration
	share := int64(speedBytes) / int64(len(use))
	began := time.Now()
	for _, f := range use {
		h, err := os.Open(f.path)
		if err != nil {
			continue
		}
		var off int64
		if f.size > share {
			off = rand.N(f.size - share)
		}
		var done int64
		for done < share && time.Since(began) < speedBudget && c.ctx.Err() == nil {
			t := time.Now()
			n, err := h.ReadAt(buf, off+done)
			took := time.Since(t)
			total += int64(n)
			done += int64(n)
			spent += took
			c.j.Progress(c.from + c.span*(0.55+0.2*float64(total)/speedBytes))
			if err != nil {
				break
			}
			c.j.Rest(c.ctx, took)
		}
		_ = h.Close() // only read from
	}
	if c.ctx.Err() != nil {
		return
	}
	if total == 0 || spent <= 0 {
		c.add(Finding{Severity: SevWarn, Title: "Read speed couldn't be measured", What: "None of the files tried could be read."})
		return
	}
	// A few jumps, as when someone seeks in a film.
	var times []time.Duration
	for i := range speedRandom {
		if c.ctx.Err() != nil {
			return
		}
		f := use[i%len(use)]
		h, err := os.Open(f.path)
		if err != nil {
			continue
		}
		off := rand.N(max(f.size-int64(len(buf)), 1))
		t := time.Now()
		_, err = h.ReadAt(buf, off)
		took := time.Since(t)
		_ = h.Close() // only read from
		if err == nil || int64(len(buf)) > f.size-off {
			times = append(times, took)
		}
		c.j.Rest(c.ctx, took)
	}
	c.add(speedFinding(total, spent, times))
}

// speedFinding words a speed measurement.
func speedFinding(total int64, spent time.Duration, times []time.Duration) Finding {
	mbs := float64(total) / (1 << 20) / spent.Seconds()
	bits := float64(total) * 8 / spent.Seconds()
	streams := func(rate float64) int { return int(bits / rate) }
	det := []string{
		fmt.Sprintf("Read %s in %s", bytesText(total), spent.Round(time.Millisecond)),
		fmt.Sprintf("4K remux (about 100 Mbit/s): about %d at once", streams(remuxBits)),
		fmt.Sprintf("Blu-ray (about 40 Mbit/s): about %d at once", streams(blurayBits)),
		fmt.Sprintf("1080p stream (about 10 Mbit/s): about %d at once", streams(hdBits)),
	}
	if len(times) > 0 {
		slices.Sort(times)
		det = append(det, fmt.Sprintf("Jumping around, a 1 MB read took about %s (the slowest %s)", times[len(times)/2].Round(100*time.Microsecond), times[len(times)-1].Round(100*time.Microsecond)))
	}
	caution := " The system's cache can flatter this, if these files were read lately."
	switch {
	case bits >= remuxBits*1.2:
		return Finding{Severity: SevOK, Title: fmt.Sprintf("Reads at about %.0f MB/s", mbs), What: "It can keep up with what you're likely to play." + caution, Details: det}
	case bits >= hdBits*1.2:
		return Finding{Severity: SevInfo, Title: fmt.Sprintf("Reads at about %.0f MB/s", mbs), What: "Fine for most films. The very largest may stall." + caution, Details: det}
	}
	return Finding{Severity: SevWarn, Title: fmt.Sprintf("Reads slowly, at about %.1f MB/s", mbs), What: "That's too slow for a good 1080p stream. Something may be wrong with the drive or the connection to it, or it may be busy." + caution, Fix: "Try again when nothing else is using the drive. If it's still slow, check the cable or the network, and the drive's health.", Details: det}
}

// probeResult is what the write test found.
type probeResult struct {
	writeSec, readSec float64
	matched           bool
}

// writeProbe writes a temp file in dir, makes sure it reached the drive, reads it
// back to compare, and always removes it.
func writeProbe(ctx context.Context, dir string, size int64) (res probeResult, err error) {
	f, err := os.CreateTemp(dir, writeTempPrefix+"*")
	if err != nil {
		return res, err
	}
	name := f.Name()
	defer func() {
		_ = f.Close() // already closed on the good path
		if rerr := os.Remove(name); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) && err == nil {
			err = rerr
		}
	}()
	// A pattern that doesn't compress, so the drive really takes the bytes.
	buf := make([]byte, 1<<20)
	seed := rand.New(rand.NewPCG(rand.Uint64(), 7))
	for i := 0; i+8 <= len(buf); i += 8 {
		v := seed.Uint64()
		for k := range 8 {
			buf[i+k] = byte(v >> (8 * k))
		}
	}
	first := buf[0]
	start := time.Now()
	n := byte(0)
	for done := int64(0); done < size; done += int64(len(buf)) {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		buf[0] = first + n // each mebibyte differs
		n++
		if _, err := f.Write(buf); err != nil {
			return res, err
		}
	}
	if err := f.Sync(); err != nil {
		return res, err
	}
	res.writeSec = time.Since(start).Seconds()
	if err := f.Close(); err != nil {
		return res, err
	}
	r, err := os.Open(name)
	if err != nil {
		return res, err
	}
	defer r.Close()
	start = time.Now()
	got := make([]byte, len(buf))
	res.matched = true
	n = 0
	for done := int64(0); done < size; done += int64(len(buf)) {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		buf[0] = first + n
		n++
		if _, err := io.ReadFull(r, got); err != nil {
			return res, err
		}
		if !bytes.Equal(got, buf) {
			res.matched = false
			break
		}
	}
	res.readSec = time.Since(start).Seconds()
	return res, nil
}

// mayWrite says whether the write test may touch a folder. It may only go in the
// server's own folders, and only when no library is in or around them.
func mayWrite(role, path string, libraries []string) bool {
	if !writeRoles[role] {
		return false
	}
	real, err := evalLinks(path)
	if err != nil {
		return false
	}
	for _, lib := range libraries {
		if Overlap(path, lib) || Overlap(real, lib) {
			return false
		}
		if libReal, err := evalLinks(lib); err == nil && Overlap(real, libReal) {
			return false
		}
	}
	return true
}

func (c *storageCheck) writeTest() {
	if c.t.Role == "library" {
		c.add(Finding{Severity: SevInfo, Title: "The write test isn't run on media folders", What: "Nothing is ever written where your media is."})
		return
	}
	if !mayWrite(c.t.Role, c.t.Path, c.o.Libraries) {
		c.add(Finding{Severity: SevInfo, Title: "The write test wasn't run here", What: "A library folder is in or around this folder, and nothing is ever written where your media is."})
		return
	}
	if d, err := sysinfo.DiskUsage(c.t.Path); err == nil && d.Free < 2*writeProbeBytes {
		c.add(Finding{Severity: SevInfo, Title: "The write test wasn't run here", What: "There's too little free space to try it."})
		return
	}
	res, err := writeProbe(c.ctx, c.t.Path, writeProbeBytes)
	if c.ctx.Err() != nil {
		return
	}
	if err != nil {
		c.add(Finding{Severity: SevProblem, Title: "The server can't write here", What: "The system's answer: " + plainErr(err) + ". The server needs to save things in this folder.", Fix: "Give the account the server runs as permission to write here, or free some space."})
		return
	}
	if !res.matched {
		c.add(Finding{Severity: SevProblem, Title: "What was written here came back different", What: "A small test file didn't read back the same as it was written. That points to a failing drive, or a bad cable or memory.", Fix: "Copy anything important off this drive and check it with its maker's tool."})
		return
	}
	mb := float64(writeProbeBytes) / (1 << 20)
	det := []string{fmt.Sprintf("Wrote %.0f MB at %.0f MB/s, down to the drive", mb, mb/max(res.writeSec, 0.001))}
	sev, title := SevOK, "Writing here works"
	what := "A small test file was written, read back the same, and removed."
	if mb/max(res.writeSec, 0.001) < 5 && (c.t.Role == "transcode" || c.t.Role == "data") {
		sev, title = SevWarn, "Writing here works, but slowly"
		what += " At this speed, converting video may not keep up."
	}
	c.add(Finding{Severity: sev, Title: title, What: what, Details: det})
}

// healthResult is what the operating system says about a drive.
type healthResult struct {
	lines []string
	bad   bool
	note  string // why not, when it wasn't checked
	// checked: the system answered.
	checked bool
}

func (c *storageCheck) healthFinding() {
	if c.fs.Network() {
		c.add(Finding{Severity: SevInfo, Title: "Drive health wasn't checked", What: "This drive is on another machine, so it's that machine's to report."})
		return
	}
	key := c.fs.Mount
	h, ok := c.health[key]
	if !ok {
		r := driveHealth(c.ctx, c.j.log, c.fs, c.t.Path)
		h = &r
		c.health[key] = h
	}
	switch {
	case h.bad:
		c.add(Finding{Severity: SevProblem, Title: "The system says this drive isn't healthy", What: "A drive that reports this can fail at any time.", Fix: "Copy anything important off it soon, and replace it.", Details: h.lines})
	case h.checked:
		c.add(Finding{Severity: SevOK, Title: "The system reports nothing wrong with this drive", What: "This is only what the drive says about itself. A drive can fail without warning.", Details: h.lines})
	default:
		note := h.note
		if note == "" {
			note = "Needs administrator rights, not checked."
		}
		c.add(Finding{Severity: SevInfo, Title: "Drive health wasn't checked", What: note, Details: h.lines})
	}
}
