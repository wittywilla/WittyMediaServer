package gamearchive

import (
	"errors"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/wittywilla/wittyplay"
)

// ErrNoGame: nothing in the archive is a game for this console.
var ErrNoGame = errors.New("this archive holds no game for this console")

// A Plan is which files of an archive to unpack and which one a core opens.
type Plan struct {
	// Want is what to unpack and where to.
	Want []Want
	// Entry is the file the core opens, relative to the game's folder.
	Entry string
	// Discs, when set, are the discs of a set with no playlist of its own. The
	// playlist is written to Entry once they've arrived.
	Discs []string
	// Size is how much unpacking should write, as far as the archive says.
	Size int64
}

// HoldsGame says whether these extensions include a game for a console.
func HoldsGame(exts []string, c *wittyplay.Console) bool {
	for _, x := range exts {
		if c.Accepts(x) {
			return true
		}
	}
	return false
}

// ConsoleOf sorts an archive that isn't in a console's folder by what's inside: the
// first extension that only one console uses.
func ConsoleOf(exts []string) string {
	for _, x := range exts {
		if c := wittyplay.ByExtension(x); c != "" {
			return c
		}
	}
	return ""
}

// cartridgeName is the name a one-file game is always given. It keeps a game's own
// files (a core may name its saves after what it opened) from depending on what's
// in the archive, and matches how .zip games have always been run.
const cartridgeName = "game"

// junk is never needed to run a game. Disc games unpack everything else, since what
// a core wants beside a disc goes beyond what a .cue names (.sbi and .sub files).
var junk = map[string]bool{".txt": true, ".nfo": true, ".diz": true, ".pdf": true, ".doc": true, ".rtf": true, ".htm": true, ".html": true, ".url": true,
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".webp": true, ".sfv": true, ".md5": true, ".sha1": true, ".dat": true}

// Disc files in the order a choice is made: what lists the discs, then what lists a
// disc's tracks, then the older kinds of those.
var (
	playlistExts   = map[string]bool{".m3u": true}
	descriptorExts = map[string]bool{".cue": true, ".gdi": true}
	olderDiscExts  = map[string]bool{".ccd": true, ".toc": true}
	imageExts      = map[string]bool{".chd": true, ".pbp": true, ".cdi": true, ".iso": true, ".cso": true, ".img": true, ".bin": true, ".rvz": true, ".gcz": true}
)

// Choose decides what to unpack for a console. archiveName is the archive's own
// file name: a file inside named the same is the likely one.
func Choose(l *Listing, c *wittyplay.Console, archiveName string) (*Plan, error) {
	var cands []Entry
	for _, e := range l.Entries {
		if c.Accepts(strings.ToLower(path.Ext(e.Name))) {
			cands = append(cands, e)
		}
	}
	if len(cands) == 0 {
		return nil, ErrNoGame
	}
	base := strings.ToLower(BaseName(archiveName))
	for _, tier := range []map[string]bool{playlistExts, descriptorExts, olderDiscExts} {
		if got := withExts(cands, tier); len(got) > 0 {
			return choosePlan(l, c, got, base, true), nil
		}
	}
	if got := withExts(cands, imageExts); len(got) > 1 && discSet(got) {
		return choosePlan(l, c, got, base, false), nil
	}
	return onePlan(best(cands, base)), nil
}

func withExts(list []Entry, exts map[string]bool) []Entry {
	var out []Entry
	for _, e := range list {
		if exts[strings.ToLower(path.Ext(e.Name))] {
			out = append(out, e)
		}
	}
	return out
}

// choosePlan is for a disc game. described: these files name others, so everything
// that might be one of those comes too. Discs of one set with no playlist get one, on
// consoles that can open one.
func choosePlan(l *Listing, c *wittyplay.Console, got []Entry, base string, described bool) *Plan {
	p := &Plan{}
	switch {
	case playlistExts[strings.ToLower(path.Ext(got[0].Name))]:
		p.Entry = best(got, base).Name
	case len(got) > 1 && discSet(got) && c.Accepts(".m3u"):
		naturalSort(got)
		for _, e := range got {
			p.Discs = append(p.Discs, e.Name)
		}
		p.Entry = cartridgeName + ".m3u"
	case len(got) > 1 && discSet(got):
		naturalSort(got)
		p.Entry = got[0].Name
	default:
		p.Entry = best(got, base).Name
	}
	if !described {
		// Bare images name nothing else: take only the discs.
		for _, e := range got {
			p.Want = append(p.Want, Want{Name: e.Name, Out: e.Name})
			p.Size += max(e.Size, 0)
		}
		return p
	}
	for _, e := range l.Entries {
		if !junk[strings.ToLower(path.Ext(e.Name))] {
			p.Want = append(p.Want, Want{Name: e.Name, Out: e.Name})
			p.Size += max(e.Size, 0)
		}
	}
	return p
}

// onePlan is a game in one file.
func onePlan(e Entry) *Plan {
	out := cartridgeName + strings.ToLower(path.Ext(e.Name))
	return &Plan{Want: []Want{{Name: e.Name, Out: out}}, Entry: out, Size: max(e.Size, 0)}
}

var discRe = regexp.MustCompile(`(?i)[\s(\[_-](disc|disk|cd|side)[\s_-]*[0-9a-d]+\b`)

// discSet: names that all say which disc they are, rather than being different
// versions of one disc.
func discSet(list []Entry) bool {
	for _, e := range list {
		if !discRe.MatchString(path.Base(e.Name)) {
			return false
		}
	}
	return true
}

var numberRe = regexp.MustCompile(`[0-9]+`)

// naturalSort puts Disc 2 before Disc 10.
func naturalSort(list []Entry) {
	key := func(s string) string {
		return numberRe.ReplaceAllStringFunc(strings.ToLower(s), func(m string) string {
			n, err := strconv.Atoi(m)
			if err != nil || n > 99999999 {
				return m
			}
			return strconv.Itoa(100000000 + n)
		})
	}
	sort.SliceStable(list, func(i, j int) bool { return key(list[i].Name) < key(list[j].Name) })
}

// best picks one of several candidates. Sets that merge many versions of a game into
// one archive are the usual reason there's a choice at all: the one named after the
// archive, then a verified dump in a widely-played region, and bad dumps and hacks last.
// A tie goes to the first in the archive, as it always has.
func best(list []Entry, base string) Entry {
	top := list[0]
	topScore := score(top.Name, base)
	for _, e := range list[1:] {
		if s := score(e.Name, base); s > topScore {
			top, topScore = e, s
		}
	}
	return top
}

var (
	badDumpRe = regexp.MustCompile(`(?i)\[(b|h|o|t|p|f|a)[0-9]*[^\]]*\]|\((beta|proto|prototype|demo|sample|hack|pirate|unl)[^)]*\)`)
	usaRe     = regexp.MustCompile(`(?i)\((usa?|world)[,)]|\((u|w|ue|ju|jue)\)`)
	europeRe  = regexp.MustCompile(`(?i)\((europe|e)[,)]`)
	japanRe   = regexp.MustCompile(`(?i)\((japan|j)[,)]`)
)

func score(name, base string) int {
	n := path.Base(name)
	s := 0
	if strings.EqualFold(strings.TrimSuffix(n, path.Ext(n)), base) {
		s += 32
	}
	if strings.Contains(n, "[!]") {
		s += 8
	}
	switch {
	case usaRe.MatchString(n):
		s += 4
	case europeRe.MatchString(n):
		s += 2
	case japanRe.MatchString(n):
		s++
	}
	if badDumpRe.MatchString(n) {
		s -= 16
	}
	return s
}
