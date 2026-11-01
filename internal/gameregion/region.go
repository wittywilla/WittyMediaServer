// Package gameregion holds the owner's order of regions: which version of a game to
// prefer when there's more than one. USA first isn't right for everyone.
package gameregion

import (
	"slices"
	"strings"
	"sync/atomic"
)

// The regions someone can put in order, and what's the default order. Anything else
// comes after them all.
const (
	USA       = "usa"
	Europe    = "europe"
	Australia = "australia"
	Japan     = "japan"
	Asia      = "asia"
	Brazil    = "brazil"
)

// Default is the order with nothing chosen: what's most useful to an English speaker.
var Default = []string{USA, Europe, Australia, Japan, Asia, Brazil}

// words says what a region is called in file names, and in the tags made from them.
var words = map[string]string{
	"usa": USA, "us": USA, "america": USA, "canada": USA,
	"europe": Europe, "eur": Europe, "uk": Europe, "germany": Europe, "france": Europe, "spain": Europe, "italy": Europe, "netherlands": Europe,
	"holland": Europe, "sweden": Europe, "denmark": Europe, "norway": Europe, "finland": Europe, "portugal": Europe, "greece": Europe, "poland": Europe,
	"australia": Australia,
	"japan":     Japan, "jpn": Japan,
	"asia": Asia, "korea": Asia, "kor": Asia, "china": Asia, "taiwan": Asia, "hong": Asia,
	"brazil": Brazil,
}

// codes are the short forms older sets use, in brackets: (U), (JU). Other short words in
// brackets are usually languages, so only these count, and only in a file's name.
var codes = map[string][]string{
	"u": {USA}, "ue": {USA, Europe}, "ju": {Japan, USA}, "jue": {Japan, USA, Europe},
	"e": {Europe}, "g": {Europe}, "f": {Europe}, "s": {Europe}, "i": {Europe}, "h": {Europe},
	"a": {Australia}, "j": {Japan}, "k": {Asia}, "c": {Asia}, "b": {Brazil},
}

// world is for every region, so it's as good as the first of them.
const world = "world"

var current atomic.Pointer[[]string]

// Parse reads a setting: known regions, in order. Any left out go after those, in the
// usual order, so a saved order still works when a region is added.
func Parse(setting string) []string {
	out := make([]string, 0, len(Default))
	for _, p := range strings.Split(setting, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if slices.Contains(Default, p) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	for _, r := range Default {
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// Valid says a setting names only known regions, each once.
func Valid(setting string) bool {
	seen := map[string]bool{}
	for _, p := range strings.Split(setting, ",") {
		p = strings.TrimSpace(p)
		if !slices.Contains(Default, p) || seen[p] {
			return false
		}
		seen[p] = true
	}
	return true
}

// Set makes this order the one in use.
func Set(setting string) {
	o := Parse(setting)
	current.Store(&o)
}

// Order is the order in use.
func Order() []string {
	if o := current.Load(); o != nil {
		return *o
	}
	return Default
}

// Unknown is the rank of something from no region in the order.
func Unknown() int { return len(Default) }

// Rank says how wanted these tags are, lowest best: the best of the regions they name. A
// tag has to be a whole word: "Thousand" says nothing about the USA.
func Rank(tags string) int {
	best := Unknown()
	order := Order()
	eachWord(strings.ToLower(tags), func(w string) {
		if w == world {
			best = 0
			return
		}
		if r, ok := words[w]; ok {
			best = min(best, slices.Index(order, r))
		}
	})
	return best
}

// RankFile is Rank for a file's name, where only what's in brackets says anything:
// a game can be called America. It reads the short forms older sets use, too.
func RankFile(name string) int {
	best := Unknown()
	order := Order()
	n := strings.ToLower(name)
	for {
		start := strings.IndexByte(n, '(')
		if start < 0 {
			return best
		}
		n = n[start+1:]
		end := strings.IndexByte(n, ')')
		if end < 0 {
			return best
		}
		group := n[:end]
		n = n[end+1:]
		if rs, ok := codes[group]; ok {
			for _, r := range rs {
				best = min(best, slices.Index(order, r))
			}
			continue
		}
		if group == "w" {
			best = 0
			continue
		}
		best = min(best, Rank(group))
	}
}

func eachWord(s string, do func(string)) {
	start := -1
	for i := 0; i <= len(s); i++ {
		if i < len(s) && s[i] >= 'a' && s[i] <= 'z' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			do(s[start:i])
			start = -1
		}
	}
}
