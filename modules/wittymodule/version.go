package wittymodule

import (
	"strconv"
	"strings"
)

// version is a parsed semantic version: 1.2.3-alpha.1 (a leading v and build
// metadata after + are ignored).
type version struct {
	core [3]int
	pre  []string
}

func parseVersion(s string) (version, bool) {
	var v version
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	// Versions become folder names, so nothing else can be allowed in one.
	if strings.IndexFunc(s, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '.' || r == '-' || r == '+')
	}) >= 0 {
		return v, false
	}
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

// Prerelease reports whether s is a pre-release such as 0.4.0-alpha.
func Prerelease(s string) bool {
	v, ok := parseVersion(s)
	return ok && len(v.pre) > 0
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// parseMax reads a maxServer value: an exact version, or a series such as 0.5.x
// or 0.x, which covers every release and pre-release in it. A lone x is refused,
// because every module states a real maximum.
func parseMax(s string) (series []int, exact string, ok bool) {
	if Valid(s) {
		return nil, s, true
	}
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	if len(parts) < 2 || len(parts) > 3 || parts[len(parts)-1] != "x" {
		return nil, "", false
	}
	for _, p := range parts[:len(parts)-1] {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, "", false
		}
		series = append(series, n)
	}
	return series, "", true
}

// atMost: v is not past the maximum.
func atMost(v, max string) bool {
	series, exact, ok := parseMax(max)
	if !ok {
		return false
	}
	if series == nil {
		return Compare(v, exact) <= 0
	}
	p, ok := parseVersion(v)
	if !ok {
		return false
	}
	for i, n := range series {
		if c := cmpInt(p.core[i], n); c != 0 {
			return c < 0
		}
	}
	return true
}
