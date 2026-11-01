package gameid

import (
	"regexp"
	"strconv"
	"strings"
)

// What a name can say. These are weaker evidence than a file's bytes, but they're what
// sets of cartridges have to go on inside an archive, and what tells a BIOS from a game
// when both have good headers.

var (
	// Written in capitals on purpose: there are games with Bios in their names.
	biosNameRe = regexp.MustCompile(`\bBIOS\b|^\[BIOS\]|(?i)^(?:scph[-_ ]?[0-9]+|dc_boot|dc_flash|gba_bios|bios[79]|syscard[0-9]|disksys|lynxboot|dmg_boot|cgb_boot|sgb[0-9]?_boot|firmware)\b`)
	// Devices and test carts.
	utilityNameRe = regexp.MustCompile(`(?i)\b(?:test (?:program|cartridge|cassette|rom|kit)|diagnostics?|copier|game genie|action replay|game ?shark|xploder|x-terminator|x-band modem|nintendo power menu|menu program|dsp loader)\b|\((?:test kit|test program|test cartridge|program)\)`)
	// A cart that only hosts other cartridges or does nothing by itself.
	hostNameRe = regexp.MustCompile(`(?i)^(?:super game boy|sufami turbo|bs-x)\b`)

	plusBIOSRe  = regexp.MustCompile(`\+\s*(?:[A-Za-z]+\s+)?BIOS\b`)
	updateTagRe = regexp.MustCompile(`(?i)[\(\[](?:[^\)\]]*\b)?(?:update|patch)(?:\b[^\)\]]*)?[\)\]]`)
	dlcTagRe    = regexp.MustCompile(`(?i)[\(\[](?:[^\)\]]*\b)?(?:dlc|add-?on)(?:\b[^\)\]]*)?[\)\]]|\bdata pack\b|\bkoma data\b`)
	// Tracks of a disc, kept as files of their own: the first has the game's data.
	trackNameRe = regexp.MustCompile(`(?i)[\s(\[_-]track[\s_-]*0*([0-9]+)\)?\]?\s*$`)
)

// ClassifyName says what a file's name, without its extension, says it is. Most names
// say nothing: "" means it's left to the bytes.
func ClassifyName(stem string) (Kind, string) {
	bare := tagFreeTitle(stem)
	// "Game + ST BIOS" is a game that comes with what it needs.
	if plusBIOSRe.MatchString(stem) {
		return KindUnknown, ""
	}
	switch {
	case biosNameRe.MatchString(stem):
		return KindBIOS, "Its name says it's a BIOS."
	case hostNameRe.MatchString(bare):
		return KindUtility, "It's a cartridge that hosts other games, and has nothing to play by itself."
	case utilityNameRe.MatchString(stem):
		return KindUtility, "Its name says it's a utility, not a game."
	case dlcTagRe.MatchString(stem):
		return KindDLC, "Its name says it's extra content for a game."
	case updateTagRe.MatchString(stem):
		return KindUpdate, "Its name says it's an update for a game."
	}
	return KindUnknown, ""
}

var bracketRe = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)

func tagFreeTitle(s string) string {
	return strings.TrimSpace(bracketRe.ReplaceAllString(s, ""))
}

// TrackNumber says which track of a disc a name is, or 0.
func TrackNumber(stem string) int {
	m := trackNameRe.FindStringSubmatch(stem)
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n > 99 {
		return 0
	}
	return n
}

var (
	discWordRe = regexp.MustCompile(`(?i)(?:^|[\s_\-\(\[])(?:dis[ck]|cd|dvd|gd)[\s_\-]*([0-9]{1,2}|one|two|three|four|five|six)(?:[\s_\-]*(?:of|/)[\s_\-]*([0-9]{1,2}))?(?:[\s_\-\)\]]|$)`)
	discWords  = map[string]int{"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6}
	// A disc's product code, written in any of the ways it gets written.
	serialInNameRe = regexp.MustCompile(`(?i)(?:^|[\s_\-\(\[])(SL[UEPKA][SMC]|SC[UEPKA][SMC]|SCED|SLED|SLPM|PAPX|PBPX|UL[UEJK][SM]|UC[UEJK][SM]|NP[UEJH][GH])[-_ ]?([0-9]{3})[-_. ]?([0-9]{2})(?:[\s_\-\)\]]|$)`)
	// Files named like web addresses keep their region as one lone letter.
	slugRegionRe = regexp.MustCompile(`(?i)-(u|e|j|usa|eur|jpn)(-|$)`)
)

// DiscIn reads which disc of how many a name says it is, and what's left of the name
// without that: "Mega Game (Disc 2 of 3)" is 2, 3 and "Mega Game". 0 when it doesn't say.
func DiscIn(stem string) (disc, total int, rest string) {
	loc := discWordRe.FindStringSubmatchIndex(stem)
	if loc == nil {
		return 0, 0, stem
	}
	word := strings.ToLower(stem[loc[2]:loc[3]])
	if n, ok := discWords[word]; ok {
		disc = n
	} else {
		disc, _ = strconv.Atoi(word)
	}
	if disc <= 0 {
		return 0, 0, stem
	}
	if loc[4] >= 0 {
		total, _ = strconv.Atoi(stem[loc[4]:loc[5]])
	}
	rest = strings.TrimSpace(stem[:loc[0]] + " " + stem[loc[1]:])
	return disc, total, rest
}

// SerialIn finds a disc's product code in a name, written as databases write it.
func SerialIn(name string) string {
	m := serialInNameRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1]) + "-" + m[2] + m[3]
}

// IsSlug says a name was written for a web address, not a person: no spaces, and
// words joined by dashes.
func IsSlug(stem string) bool {
	return !strings.ContainsAny(stem, " _") && strings.Count(stem, "-") >= 2
}

// Unslug makes a person's name of a web address's: final-fantasy-vii-u-scus-94163- is
// "final fantasy vii". What's taken off goes in the tags.
func Unslug(stem string) string {
	s := slugRegionRe.ReplaceAllString(stem, "$2")
	if m := serialInNameRe.FindStringIndex(s); m != nil {
		s = s[:m[0]] + "-" + s[m[1]:]
	}
	s = strings.ReplaceAll(s, "-", " ")
	return strings.Join(strings.Fields(s), " ")
}

// A set's entries, sorted by what their names say.
var (
	badDumpRe    = regexp.MustCompile(`\[(?:b|h|o|t|p|f|a)[0-9]*[^\]]*\]`)
	translatedRe = regexp.MustCompile(`\[[Tt][-+][^\]]*\]`)
	hackTagRe    = regexp.MustCompile(`(?i)\([^)]*\bhack\b[^)]*\)|\(hack\)`)
	publicDomain = regexp.MustCompile(`\(PD\)`)
	prereleaseRe = regexp.MustCompile(`(?i)\((?:beta|alpha|proto|prototype|demo|sample|pre-?release|preview)[^)]*\)`)
)

// Quality is how good a copy a set's entry is, from what its name says.
type Quality int

// From best to worst.
const (
	QualityClean      Quality = iota // a plain dump of a released game
	QualityPrerelease                // beta, demo, sample
	QualityTranslated                // someone's translation
	QualityAltered                   // bad dump, overdump, trainer, fix
	QualityHack                      // someone's game built on this one
	QualityPD                        // homemade: demos, intros
)

// QualityOf says what an entry's name says about it.
func QualityOf(name string) Quality {
	switch {
	case publicDomain.MatchString(name):
		return QualityPD
	case hackTagRe.MatchString(name):
		return QualityHack
	case translatedRe.MatchString(name):
		return QualityTranslated
	case badDumpRe.MatchString(name):
		return QualityAltered
	case prereleaseRe.MatchString(name):
		return QualityPrerelease
	}
	return QualityClean
}
