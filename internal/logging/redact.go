package logging

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
)

// Every string that reaches a record passes through here, so no call site has to
// remember. There are three nets: the name of the attribute, patterns in free
// text (addresses, command lines, headers, JSON) and the exact values of the
// secrets the server knows about.

const hidden = "[hidden]"

// Attribute names whose value is never kept. Compared in lower case, with the
// part after the last dot, and matched at the end of the name ("tmdbKey" is
// caught by apikey-like endings below, "userToken" by "token").
var secretEndings = []string{"token", "password", "passwd", "passphrase", "secret", "apikey", "api_key", "authorization", "cookie", "credential", "credentials", "privatekey", "private_key", "setupcode", "setup_code", "recoverycode", "recovery_code", "paircode", "pair_code", "pairingcode", "pairing_code", "bearer"}

var secretExact = map[string]bool{"pin": true, "st": true, "otp": true, "totp": true, "pass": true, "pwd": true, "sig": true, "signature": true, "key": true, "tmdbkey": true, "tvdbkey": true, "tvdbpin": true}

func secretKey(k string) bool {
	k = strings.ToLower(k)
	if i := strings.LastIndexByte(k, '.'); i >= 0 {
		k = k[i+1:]
	}
	if secretExact[k] {
		return true
	}
	for _, s := range secretEndings {
		if strings.HasSuffix(k, s) {
			return true
		}
	}
	return false
}

// Each pattern says what any match must contain, so the regular expressions, which
// are slow, only run on text that could hold one. Words are checked in lower case.
var textPatterns = []struct {
	chars string   // needs all of these characters
	words []string // and one of these, if any
	re    *regexp.Regexp
	with  string
}{
	// In an address or a command line: token=..., ?st=..., password=...
	{"=", nil, regexp.MustCompile(`(?i)(\b(?:token|access_token|refresh_token|id_token|auth|api[_-]?key|apikey|password|passwd|pwd|pass|secret|pin|key|st|sig|signature|setup[_-]?code|recovery[_-]?code|pair(?:ing)?[_-]?code|otp|totp)=)[^&\s"'<>]+`), "${1}" + hidden},
	// The same with its name percent-encoded: %74oken=... signs in just the same.
	{"%=", nil, regexp.MustCompile(`(?i)((?:%[0-9a-f]{2}|[a-z_-])*%[0-9a-f]{2}(?:%[0-9a-f]{2}|[a-z_-])*=)[^&\s"'<>]+`), "${1}" + hidden},
	// A header, as a line of text or in FFmpeg's -headers.
	{"", []string{"authorization", "cookie", "x-wms-token", "x-api-key"}, regexp.MustCompile(`(?i)\b(proxy-authorization|authorization|set-cookie|cookie|x-wms-token|x-api-key)(\s*[:=]\s*)[^\r\n"']+`), "${1}${2}" + hidden},
	{"", []string{"bearer"}, regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]+`), "${1}" + hidden},
	{"", []string{"basic "}, regexp.MustCompile(`\b(Basic\s+)[A-Za-z0-9+/]*[A-Z0-9+/][A-Za-z0-9+/]{10,}={0,2}`), "${1}" + hidden},
	// JSON: "password": "..."
	{`":`, nil, regexp.MustCompile(`(?i)("(?:[a-z_-]*token|password|passwd|secret|api[_-]?key|apikey|pin|setup[_-]?code|recovery[_-]?code|pair(?:ing)?[_-]?code)"\s*:\s*)"(?:[^"\\]|\\.)*"`), `${1}"` + hidden + `"`},
	// A login in an address: rtsp://user:password@camera/
	{":@", nil, regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^/\s@]+@`), "${1}" + hidden + "@"},
	// A share link's token.
	{"/", []string{"/s/", "/share/"}, regexp.MustCompile(`(/(?:api/)?(?:s|share)/)[A-Za-z0-9_-]{8,}`), "${1}" + hidden},
	// The sign-in cookie and stream tokens, however they got there.
	{"", []string{"wms_token=", "st1."}, regexp.MustCompile(`(?i)\b(wms_token=|st1\.)[A-Za-z0-9._~-]+`), "${1}" + hidden},
}

// A long run of hex standing alone is what a session token looks like.
var longHex = regexp.MustCompile(`\b[a-fA-F0-9]{48,}\b`)

var (
	secretsMu sync.Mutex
	secrets   atomic.Pointer[[]string]
)

// AddSecret makes sure this exact value never appears in the log, wherever it
// turns up: an API key, a setup code. Short values are ignored, as they would
// blot out ordinary words, and so are repeats.
func AddSecret(v string) {
	if len(v) < 6 {
		return
	}
	secretsMu.Lock()
	defer secretsMu.Unlock()
	cur := secrets.Load()
	var next []string
	if cur != nil {
		for _, s := range *cur {
			if s == v {
				return
			}
		}
		next = append(next, *cur...)
	}
	if len(next) >= 64 {
		next = next[1:] // the oldest; the patterns are still there
	}
	next = append(next, v)
	secrets.Store(&next)
}

// Redact removes secrets from free text (URLs, command lines, headers, errors).
func Redact(s string) string {
	if s == "" {
		return s
	}
	if cur := secrets.Load(); cur != nil {
		for _, v := range *cur {
			if strings.Contains(s, v) {
				s = strings.ReplaceAll(s, v, hidden)
			}
		}
	}
	var lower string
	for i := range textPatterns {
		p := &textPatterns[i]
		if !hasAll(s, p.chars) {
			continue
		}
		if p.words != nil {
			if lower == "" {
				lower = strings.ToLower(s)
			}
			if !hasAnyWord(lower, p.words) {
				continue
			}
		}
		s = p.re.ReplaceAllString(s, p.with)
	}
	if hasHexRun(s) {
		s = longHex.ReplaceAllString(s, hidden)
	}
	return s
}

func hasAll(s, chars string) bool {
	for i := 0; i < len(chars); i++ {
		if strings.IndexByte(s, chars[i]) < 0 {
			return false
		}
	}
	return true
}

func hasAnyWord(lower string, words []string) bool {
	for _, w := range words {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

func hasHexRun(s string) bool {
	if len(s) < 48 {
		return false
	}
	run := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' {
			if run++; run >= 48 {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

// ---------------------------------------------------------------- shared bundles

// Scrubber takes titles and paths out of log text for a bundle that may be
// shared. The same value gets the same stand-in throughout a bundle, so what
// happened to one file can still be followed; the salt is thrown away, so the
// stand-in can't be turned back into the title.
type Scrubber struct {
	salt  [16]byte
	roots []string
}

// NewScrubber makes one for a bundle. roots are the library folders, whose
// names are hidden wherever they appear.
func NewScrubber(roots []string) *Scrubber {
	s := &Scrubber{roots: roots}
	_, _ = rand.Read(s.salt[:])
	return s
}

func (s *Scrubber) stand(kind, v string) string {
	h := sha256.New()
	h.Write(s.salt[:])
	h.Write([]byte(v))
	return "[" + kind + ":" + hex.EncodeToString(h.Sum(nil))[:6] + "]"
}

// Attribute names that hold what someone watches, plays or owns.
var titleKeys = map[string]bool{"title": true, "name": true, "game": true, "show": true, "album": true, "artist": true, "label": true, "movie": true, "episode": true, "series": true, "channel": true, "q": true, "query": true, "rom": true, "disc": true, "user": true, "account": true, "from": true, "ip": true, "host": true, "hostname": true, "library": true, "device": true, "tuner": true}

var pathKeys = map[string]bool{"path": true, "file": true, "dir": true, "folder": true, "src": true, "source": true, "image": true, "url": true, "addr": true, "root": true, "data": true, "cwd": true, "exe": true, "binary": true}

var (
	windowsPath = regexp.MustCompile(`(?:\b[A-Za-z]:[\\/]|\\\\[^\\\s]+\\)[^"'\n\r]*?(?:: |"|'|$|\r|\n)`)
	unixPath    = regexp.MustCompile(`(^|[\s"'=(,])(/(?:[^/\s"'\n:]+/)*[^/\s"'\n:]+(?:[^"'\n:]*\.[A-Za-z0-9]{2,5}\b)?)`)
	webAddress  = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	metaLine    = regexp.MustCompile(`(?im)^(\s*(?:title|album|artist|album_artist|comment|show|description|synopsis|encoded_by|filename)\s*:\s*).*$`)
	fromName    = regexp.MustCompile(`(?i)(from|to) '[^']*'`)
)

// keep the addresses of the API itself: they name routes, not anyone's files.
func keepsPath(p string) bool {
	return strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/assets/") || p == "/"
}

// Text hides paths, addresses and media details in free text: a message, an
// error, what FFmpeg said.
func (s *Scrubber) Text(v string) string {
	if v == "" {
		return v
	}
	v = metaLine.ReplaceAllString(v, "${1}[hidden]")
	v = fromName.ReplaceAllString(v, "$1 [path]")
	v = webAddress.ReplaceAllStringFunc(v, func(m string) string { return s.stand("address", m) })
	v = windowsPath.ReplaceAllStringFunc(v, func(m string) string {
		end := ""
		body := m
		for _, tail := range []string{": ", `"`, "'", "\r", "\n"} {
			if strings.HasSuffix(m, tail) {
				end, body = tail, strings.TrimSuffix(m, tail)
				break
			}
		}
		return s.stand("path", body) + end
	})
	v = unixPath.ReplaceAllStringFunc(v, func(m string) string {
		sub := unixPath.FindStringSubmatch(m)
		if len(sub) < 3 || keepsPath(sub[2]) {
			return m
		}
		return sub[1] + s.stand("path", sub[2])
	})
	// Last, so they don't break up the paths around them.
	for _, r := range s.roots {
		if r != "" && strings.Contains(v, r) {
			v = strings.ReplaceAll(v, r, s.stand("folder", r))
		}
	}
	return v
}

// Attr hides one attribute's value, by its name or by what's in it.
func (s *Scrubber) Attr(k, v string) string {
	low := strings.ToLower(k)
	if i := strings.LastIndexByte(low, '.'); i >= 0 {
		low = low[i+1:]
	}
	switch {
	case titleKeys[low]:
		if v == "" {
			return v
		}
		return s.stand("title", v)
	case pathKeys[low]:
		if keepsPath(v) || v == "" {
			return v
		}
		return s.stand("path", v)
	case low == "p": // the page of a web request
		return v
	}
	return s.Text(v)
}

// Record hides titles and paths in a whole record.
func (s *Scrubber) Record(rec *Record) {
	rec.Msg = s.Text(rec.Msg)
	for i := range rec.Attrs {
		rec.Attrs[i].V = s.Attr(rec.Attrs[i].K, rec.Attrs[i].V)
	}
}
