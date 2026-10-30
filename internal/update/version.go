// Package update finds, verifies and installs new releases of
// WittyMediaServer from its GitHub releases.
//
// Nothing is installed unless the release's checksum list carries a valid
// Ed25519 signature from a key built into this program, and the downloaded
// archive matches its checksum. Installing replaces the program's files
// next to the running ones and never touches the data folder.
package update

import (
	"strconv"
	"strings"
)

// version is a parsed semantic version: 1.2.3-alpha.1 (a leading v and
// build metadata after + are ignored).
type version struct {
	core [3]int
	pre  []string
}

func parseVersion(s string) (version, bool) {
	var v version
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" {
			return v, false
		}
		v.core[i] = n
	}
	if hasPre {
		if pre == "" {
			return v, false
		}
		v.pre = strings.Split(pre, ".")
		for _, id := range v.pre {
			if id == "" {
				return v, false
			}
		}
	}
	return v, true
}

// Compare orders two versions: -1 if a is older than b, 0 if equal, 1 if
// newer. A pre-release (0.4.0-alpha) comes before its release (0.4.0).
// Versions that cannot be read sort before everything else.
func Compare(a, b string) int {
	va, oka := parseVersion(a)
	vb, okb := parseVersion(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	for i := range va.core {
		if c := cmpInt(va.core[i], vb.core[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(va.pre) == 0 && len(vb.pre) == 0:
		return 0
	case len(va.pre) == 0:
		return 1
	case len(vb.pre) == 0:
		return -1
	}
	for i := 0; i < len(va.pre) && i < len(vb.pre); i++ {
		x, y := va.pre[i], vb.pre[i]
		nx, ex := strconv.Atoi(x)
		ny, ey := strconv.Atoi(y)
		var c int
		switch {
		case ex == nil && ey == nil:
			c = cmpInt(nx, ny)
		case ex == nil: // numbers sort before words
			c = -1
		case ey == nil:
			c = 1
		default:
			c = strings.Compare(x, y)
		}
		if c != 0 {
			return c
		}
	}
	return cmpInt(len(va.pre), len(vb.pre))
}

// Valid reports whether s is a version this package can compare.
func Valid(s string) bool { _, ok := parseVersion(s); return ok }

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
