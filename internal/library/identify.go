package library

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
)

// What a scan does with a file's content. A name is believed when only media uses it.
// A name others use too (.ts is also TypeScript) is checked against the first bytes.
// A file with no name the registry knows is recognised by its content. And a new file
// whose content is clearly something else, a program called "film.mkv", is left out.

// nameHolds says a name of this category can be believed: nothing about it needs
// confirming, or its first bytes don't contradict it.
func nameHolds(path, name string, cats ...string) bool {
	f := mediatypes.Find(name, cats...)
	if f == nil {
		return false
	}
	if !f.Confirm {
		return true
	}
	return mediatypes.Confirms(f, readHead(path))
}

// readHead reads what the registry looks at. It is never more than MaxSniff, and an
// unreadable file reads as nothing.
func readHead(path string) []byte {
	fh, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer fh.Close()
	head := make([]byte, mediatypes.MaxSniff)
	n, _ := io.ReadFull(fh, head)
	return head[:n]
}

// maxSniffsPerWalk bounds what a folder of unknown files can cost one scan.
const maxSniffsPerWalk = 2000

// Names that say the file is unfinished, or plainly isn't media. They aren't worth a
// read, and a half-downloaded film shouldn't be listed.
var neverSniffed = map[string]bool{
	".part": true, ".partial": true, ".tmp": true, ".temp": true, ".crdownload": true, ".download": true, ".aria2": true, ".!qb": true, ".!ut": true, ".bak": true,
	".nfo": true, ".txt": true, ".xml": true, ".json": true, ".log": true, ".db": true, ".ini": true, ".url": true, ".lnk": true, ".sfv": true, ".md5": true, ".sha1": true,
	".sha256": true, ".torrent": true, ".exe": true, ".dll": true, ".so": true, ".dylib": true, ".msi": true, ".bat": true, ".cmd": true, ".sh": true, ".ps1": true, ".js": true,
	".css": true, ".py": true, ".go": true, ".html": true, ".htm": true, ".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
}

// sniffBudget recognises files by content during one walk, up to a limit.
type sniffBudget struct{ used int }

// sniff recognises a file whose name the registry doesn't know, if its content is of
// one of these categories. nil otherwise, and for anything too small to be media.
func (b *sniffBudget) sniff(path, name string, size int64, cats ...string) *mediatypes.Format {
	ext := mediatypes.Ext(name)
	if len(mediatypes.ByExt(name)) > 0 && ext != "" || neverSniffed[ext] || strings.HasPrefix(name, ".") || size < 512 || b.used >= maxSniffsPerWalk {
		return nil
	}
	b.used++
	f := mediatypes.SniffFile(path)
	if f == nil {
		return nil
	}
	for _, c := range cats {
		if f.Category == c {
			return f
		}
	}
	return nil
}

// isVideoDiscImage: the file may be a DVD or Blu-ray. An .iso or .img is looked into
// by the scan. Other image types are also used for other things, so they count only
// when they hold a volume at the usual place: an image of plain data sectors.
func isVideoDiscImage(path, name string) bool {
	f := mediatypes.Find(name, mediatypes.Disc)
	if f == nil {
		return false
	}
	switch f.ID {
	case "iso", "img":
		return true
	case "nrg", "mdf", "bin":
		c := mediatypes.Sniff(readHead(path))
		return c != nil && c.ID == "iso"
	}
	return false
}

// leftOut checks a new or changed file's content against what it's being indexed as.
// A file that hasn't changed was checked when it was new.
func (s *Scanner) leftOut(ctx context.Context, lib *Library, c candidate) bool {
	if c.format != "" { // it was recognised by content in the first place
		return false
	}
	var cats []string
	switch c.kind {
	case "video":
		cats = []string{mediatypes.Video, mediatypes.Audio, mediatypes.Photo}
	case "audio":
		cats = []string{mediatypes.Audio, mediatypes.Video}
	case "photo":
		cats = []string{mediatypes.Photo, mediatypes.Video}
	case "book":
		cats = []string{mediatypes.Book, mediatypes.Comic, mediatypes.Photo}
	default:
		return false
	}
	if c.unchanged() != nil {
		return false
	}
	if existing, err := s.Store.itemByPath(ctx, lib.ID, c.path); err == nil && existing.MTime == c.mtime && existing.SizeBytes == c.size {
		return false
	}
	content := mediatypes.Sniff(readHead(c.path))
	if content == nil || mediatypes.Check(c.path, content) != mediatypes.Differs || mediatypes.Fits(content, cats...) {
		return false
	}
	s.Log.WarnContext(ctx, "left a file out of the library: its name and its content disagree", "library", lib.Name, "file", c.path, "content", content.Name)
	return true
}

// ---------------------------------------------------------------------------
// Films split across files
// ---------------------------------------------------------------------------

// A part marker at the end of a name. CD, DVD and disc mean what they say. "Part" is a
// word titles also use ("Night Falls Part 2" is a sequel), so it only counts run
// together ("part2") or after a year, which ends a title.
var (
	partMarkerRe = regexp.MustCompile(`(?i)^(.*?)[\s._-]*(?:cd|dvd|dis[ck])[\s._-]*(\d{1,2})$`)
	partWordRe   = regexp.MustCompile(`(?i)^(.*?)[\s._-]*(?:part|pt)(\d{1,2})$`)
	partYearRe   = regexp.MustCompile(`(?i)^(.*(?:19|20)\d{2}\)?.*?)[\s._-]+(?:part|pt)[\s._-]*(\d{1,2})$`)
	seasonRe     = regexp.MustCompile(`(?i)\bs\d{1,3}\s*e\d{1,4}\b|\b\d{1,2}x\d{2,3}\b`)
)

const maxParts = 16

// partOf reads a part marker: the name without it, and the number.
func partOf(stem string) (string, int, bool) {
	if seasonRe.MatchString(stem) {
		return "", 0, false // an episode split in two is two episodes
	}
	for _, re := range []*regexp.Regexp{partMarkerRe, partWordRe, partYearRe} {
		if m := re.FindStringSubmatch(stem); m != nil && strings.TrimSpace(m[1]) != "" {
			n, _ := strconv.Atoi(m[2])
			if n >= 1 && n <= maxParts {
				return strings.ToLower(m[1]), n, true
			}
		}
	}
	return "", 0, false
}

// stackParts keeps the first part of each split film and takes out the rest, which
// play with it. Parts must be in one folder, share a name and a type, and run 1, 2, 3
// with no gap; anything else is left as separate files.
func stackParts(in []candidate) []candidate {
	type key struct{ dir, name, ext string }
	groups := map[key]map[int]int{}
	for i, c := range in {
		if c.kind != "video" {
			continue
		}
		base := filepath.Base(c.path)
		ext := filepath.Ext(base)
		if name, n, ok := partOf(strings.TrimSuffix(base, ext)); ok {
			k := key{filepath.Dir(c.path), name, strings.ToLower(ext)}
			if groups[k] == nil {
				groups[k] = map[int]int{}
			}
			groups[k][n] = i
		}
	}
	drop := map[int]bool{}
	for _, g := range groups {
		if len(g) < 2 {
			continue
		}
		run := 0
		for hasKey(g, run+1) {
			run++
		}
		if run < 2 || run != len(g) {
			continue
		}
		for n := 2; n <= run; n++ {
			drop[g[n]] = true
		}
	}
	if len(drop) == 0 {
		return in
	}
	out := in[:0]
	for i, c := range in {
		if !drop[i] {
			out = append(out, c)
		}
	}
	return out
}

func hasKey(m map[int]int, k int) bool {
	_, ok := m[k]
	return ok
}

// StackedParts lists the files of a split film, first part first, when this path is
// its first part. Any other file is just itself.
func StackedParts(path string) []string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name, n, ok := partOf(strings.TrimSuffix(base, ext))
	if !ok || n != 1 {
		return []string{path}
	}
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{path}
	}
	found := map[int]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ext) || !plainMode(e.Type()) {
			continue
		}
		if other, m, ok := partOf(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))); ok && other == name {
			found[m] = filepath.Join(dir, e.Name())
		}
	}
	nums := make([]int, 0, len(found))
	for m := range found {
		nums = append(nums, m)
	}
	sort.Ints(nums)
	if len(nums) < 2 {
		return []string{path}
	}
	out := make([]string, 0, len(nums))
	for i, m := range nums {
		if m != i+1 { // a gap: these aren't one film, or one is missing
			return []string{path}
		}
		out = append(out, found[m])
	}
	return out
}
