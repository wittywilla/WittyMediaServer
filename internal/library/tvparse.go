package library

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// EpisodeRef is what a file name says about an episode.
type EpisodeRef struct {
	Show       string // from the show folder (or the file name when there is none)
	ShowYear   *int
	ShowDir    string // folder that groups the show's files
	Season     int
	Episode    int
	EpisodeEnd int // last episode of a multi-episode file, 0 when single
	Title      string
	// Absolute: the file only had an episode number counted across the whole
	// show ("Show - 012"), as anime releases usually do. Season is 1 then.
	Absolute bool
}

var (
	// S02E15, S02E15-E16, S02E15E16, S02E15-16, s2.e5
	sxeRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])s(\d{1,2})[ ._-]*e(\d{1,3})(?:(?:[ ._-]*-[ ._-]*|[ ._]*)e?(\d{1,3})\b)?`)
	// 2x05, 2x05-06
	xRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(\d{1,2})x(\d{2,3})(?:-(\d{2,3}))?(?:[^0-9]|$)`)
	// "Episode 5", "Ep 05", "E05"
	epOnlyRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:episode|ep|e)[ ._-]*(\d{1,3})(?:[^0-9]|$)`)
	// Leading number: "05 - Title.mkv"
	leadNumRe     = regexp.MustCompile(`^(\d{1,3})(?:[ ._-]+|$)`)
	seasonDirRe   = regexp.MustCompile(`(?i)^(?:season|series|staffel|saison|temporada)[ ._-]*(\d{1,3})$|^s(\d{1,2})$`)
	specialsDirRe = regexp.MustCompile(`(?i)^(specials?|extras?)$`)
	trailingSepRe = regexp.MustCompile(`^[\s._-]+|[\s._-]+$`)
	// Anime-style absolute numbers: "[Group] Show - 012 [1080p]", "Show - 12v2",
	// "Show 012 (BD)". The number must be followed by a tag, a version or the end.
	absRe = regexp.MustCompile(`^(.*?\S)[ ._]+(?:-[ ._]+)?(?:#|ep?[ ._]?)?(\d{1,4})(?:v\d)?(?:[ ._]*(?:[\[(].*)?|[ ._]+-[ ._]+(.*))$`)
	// Release-group and quality tags in square brackets.
	bracketTagRe = regexp.MustCompile(`\[[^\]]*\]`)
)

// ParseEpisode reads show/season/episode from a path relative to the
// library root. Typical layouts:
//
//	Show Name (2002)/Season 02/Show Name - S02E01 - Title.mkv
//	Show Name/Show.Name.S02E01.Title.1080p.mkv
//	Show Name/2x01 - Title.mkv
//	Show.Name.S02E01.mkv                       (no folder)
func ParseEpisode(root, path string) (EpisodeRef, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = filepath.Base(path)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	file := parts[len(parts)-1]
	dirs := parts[:len(parts)-1]
	name := strings.TrimSuffix(file, filepath.Ext(file))

	var ref EpisodeRef
	seasonFromDir := -1
	if len(dirs) > 0 {
		last := dirs[len(dirs)-1]
		if m := seasonDirRe.FindStringSubmatch(last); m != nil {
			seasonFromDir = atoiFirst(m[1], m[2])
		} else if specialsDirRe.MatchString(last) {
			seasonFromDir = 0
		}
	}

	matched := false
	var titleFrom int
	var prefix string
	if loc := sxeRe.FindStringSubmatchIndex(name); loc != nil {
		ref.Season = atoiFirst(name[loc[2]:loc[3]])
		ref.Episode = atoiFirst(name[loc[4]:loc[5]])
		if loc[6] >= 0 {
			ref.EpisodeEnd = atoiFirst(name[loc[6]:loc[7]])
		}
		prefix, titleFrom, matched = name[:loc[0]], loc[1], true
	} else if loc := xRe.FindStringSubmatchIndex(name); loc != nil {
		ref.Season = atoiFirst(name[loc[2]:loc[3]])
		ref.Episode = atoiFirst(name[loc[4]:loc[5]])
		if loc[6] >= 0 {
			ref.EpisodeEnd = atoiFirst(name[loc[6]:loc[7]])
		}
		end := loc[1]
		if end > loc[5] && (loc[6] < 0 || end > loc[7]) && end <= len(name) && end > 0 && !isDigit(name[end-1]) {
			end-- // the trailing non-digit belongs to the title
		}
		prefix, titleFrom, matched = name[:loc[0]], end, true
	} else if seasonFromDir >= 0 {
		if loc := epOnlyRe.FindStringSubmatchIndex(name); loc != nil {
			ref.Season, ref.Episode = seasonFromDir, atoiFirst(name[loc[2]:loc[3]])
			prefix, titleFrom, matched = name[:loc[0]], loc[3], true
		} else if loc := leadNumRe.FindStringSubmatchIndex(name); loc != nil {
			ref.Season, ref.Episode = seasonFromDir, atoiFirst(name[loc[2]:loc[3]])
			titleFrom, matched = loc[1], true
		}
	}
	if !matched {
		// Last resort: anime-style absolute numbering.
		bare := strings.TrimSpace(bracketTagRe.ReplaceAllString(name, " "))
		if m := absRe.FindStringSubmatch(bare); m != nil {
			n := atoiFirst(m[2])
			if n > 0 && !(n >= 1900 && n <= 2099) { // never mistake a year for an episode
				ref.Season, ref.Episode, ref.Absolute = 1, n, true
				prefix, titleFrom, matched = m[1], len(name), true
				if m[3] != "" {
					ref.Title = cleanEpisodeTitle(m[3])
				}
				name = bare
			}
		}
	}
	if !matched {
		return ref, false
	}
	if ref.EpisodeEnd != 0 && ref.EpisodeEnd <= ref.Episode {
		ref.EpisodeEnd = 0
	}
	if titleFrom < len(name) {
		ref.Title = cleanEpisodeTitle(name[titleFrom:])
	}

	// The show is the first folder under the root, unless that folder is a
	// season folder (then the file name prefix names it).
	showDirs := dirs
	if len(showDirs) > 0 && (seasonDirRe.MatchString(showDirs[len(showDirs)-1]) || specialsDirRe.MatchString(showDirs[len(showDirs)-1])) {
		showDirs = showDirs[:len(showDirs)-1]
	}
	if len(showDirs) > 0 {
		ref.ShowDir = filepath.Join(root, filepath.FromSlash(showDirs[0]))
		ref.Show, ref.ShowYear = ParseName(strings.TrimSpace(bracketTagRe.ReplaceAllString(showDirs[0], " ")))
	} else {
		p := cleanEpisodeTitle(prefix)
		if p == "" {
			return ref, false
		}
		ref.Show, ref.ShowYear = ParseName(p)
		ref.ShowDir = filepath.Join(root, ref.Show)
	}
	return ref, true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func atoiFirst(v ...string) int {
	for _, s := range v {
		if s != "" {
			n, _ := strconv.Atoi(s)
			return n
		}
	}
	return 0
}

// cleanEpisodeTitle strips separators and release junk from the text after
// the episode number. Dots are kept when the title already uses spaces
// ("Operation - C.A.T.S"), otherwise they are word separators.
func cleanEpisodeTitle(t string) string {
	t = junkRe.ReplaceAllString(t, "")
	if strings.Contains(t, " ") {
		t = strings.ReplaceAll(t, "_", " ")
	} else {
		t = spacersRe.ReplaceAllString(t, " ")
	}
	t = trailingSepRe.ReplaceAllString(t, "")
	return wsRe.ReplaceAllString(t, " ")
}
