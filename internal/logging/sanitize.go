package logging

import (
	"context"
	"database/sql"
	"net/netip"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A sanitized bundle is one the owner can hand to anyone. Redact, in redact.go, keeps
// secrets out of every log as it's written. The Sanitizer goes further for a copy that
// leaves: addresses, names, paths and titles become numbered placeholders.
//
// The same value gets the same placeholder throughout one bundle, so what happened to
// one file can still be followed. The numbers are handed out as values turn up and the
// list is thrown away with the Sanitizer: it's never written anywhere.

// Known is what the server can name exactly: anything here comes out wherever it turns
// up, including in free text no pattern would recognise.
type Known struct {
	Users     []string
	Libraries []string
	Folders   []string // library folders, the data folder and its neighbours
	Hosts     []string // this machine's name, and the address it's reached at
	Titles    []string // what's in the libraries
}

// Limits on what's kept in memory for one bundle, for a modest server.
const (
	maxKnownTitles = 100_000
	maxKnownLen    = 300
	minKnownLen    = 3
)

// Words too common in the log's own text to hide when a name is exactly one of them: a
// title called "Error" or a library called "Movies" must not blot out the diagnosis, and
// a name this general says nothing about anyone.
var commonWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`error errors warn warning info debug trace fatal panic crash failed failure server client
		admin administrator root user users guest test tests file files path paths folder data library libraries stream streams
		play player playback video audio music movie movies show shows tv photo photos book books game games scan start started
		stop stopped http https update updates module modules log logs home dev network local localhost default true false none
		null unknown request session transcode disc discs dvd bluray storage auth remote system windows linux darwin channel
		channels radio podcasts audiobooks anime videos shorts commercials`) {
		m[w] = true
	}
	return m
}()

type knownEntry struct {
	text string
	kind string
	one  bool // a single word: matched exactly, and not in a line's fixed message
}

// Sanitizer replaces what could identify someone in the text of one bundle. It isn't
// safe for use from several goroutines at once: a bundle is written by one.
type Sanitizer struct {
	numbers map[string]int          // kind + value -> its number
	next    map[string]int          // kind -> the last number given out
	counts  map[string]int          // kind -> how many places were changed
	folders []knownFolder           // longest first
	words   map[string][]knownEntry // first word, in lower case -> what can start there
	entries int
}

type knownFolder struct{ lower, slashed string }

// NewSanitizer makes one for a bundle. This machine's name, the home folder and the name of
// the account it runs under are always added.
func NewSanitizer(k Known) *Sanitizer {
	s := &Sanitizer{numbers: map[string]int{}, next: map[string]int{}, counts: map[string]int{}, words: map[string][]knownEntry{}}
	if h, err := os.Hostname(); err == nil {
		k.Hosts = append(k.Hosts, h)
		if i := strings.IndexByte(h, '.'); i > 0 {
			k.Hosts = append(k.Hosts, h[:i])
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		k.Folders = append(k.Folders, h)
	}
	if u, err := user.Current(); err == nil {
		name := u.Username
		if i := strings.LastIndexAny(name, `\/`); i >= 0 {
			name = name[i+1:]
		}
		k.Users = append(k.Users, name)
	}
	seen := map[string]bool{}
	for _, p := range k.Folders {
		s.addFolder(p, seen)
	}
	sort.SliceStable(s.folders, func(i, j int) bool { return len(s.folders[i].slashed) > len(s.folders[j].slashed) })
	add := func(kind string, list []string) {
		for _, v := range list {
			s.addName(kind, v)
		}
	}
	add("user", k.Users)
	add("host", k.Hosts)
	add("library", k.Libraries)
	for i, t := range k.Titles {
		if i >= maxKnownTitles {
			break
		}
		s.addName("title", t)
	}
	// Longest first, so "The Long Night Returns" isn't taken for "The Long Night".
	for _, list := range s.words {
		if len(list) > 1 {
			sort.SliceStable(list, func(i, j int) bool { return len(list[i].text) > len(list[j].text) })
		}
	}
	return s
}

// NewScrubber is NewSanitizer for callers that only know the library folders.
func NewScrubber(roots []string) *Sanitizer { return NewSanitizer(Known{Folders: roots}) }

func (s *Sanitizer) addFolder(p string, seen map[string]bool) {
	p = strings.TrimRight(strings.TrimSpace(p), `\/`)
	if len(p) < 3 { // "/" or "D:" would match everything
		return
	}
	slashed := strings.ToLower(strings.ReplaceAll(p, `\`, "/"))
	if seen[slashed] {
		return
	}
	seen[slashed] = true
	s.folders = append(s.folders, knownFolder{lower: strings.ToLower(p), slashed: slashed})
	// Asked for by name in free text too, however it's written.
	base := filepath.Base(strings.ReplaceAll(p, `\`, "/"))
	if strings.Contains(base, " ") {
		s.addName("folder", base)
	}
}

// addName files a value under its first word.
func (s *Sanitizer) addName(kind, v string) {
	v = strings.TrimFunc(strings.TrimSpace(v), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(v) < minKnownLen || len(v) > maxKnownLen {
		return
	}
	first, rest := firstWord(v)
	one := rest == ""
	lower := strings.ToLower(first)
	if one && commonWords[lower] {
		return
	}
	if one && len(v) < 4 && kind == "title" {
		return // Up, Her, Big: too likely to be part of ordinary text
	}
	for _, e := range s.words[lower] {
		if e.text == v {
			return
		}
	}
	s.words[lower] = append(s.words[lower], knownEntry{text: v, kind: kind, one: one})
	s.entries++
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func firstWord(v string) (word, rest string) {
	for i, r := range v {
		if !isWordRune(r) {
			return v[:i], v[i:]
		}
	}
	return v, ""
}

// stand gives the placeholder for a value, the same one each time.
func (s *Sanitizer) stand(kind, v string) string {
	key := kind + "\x00" + strings.ToLower(v)
	n, ok := s.numbers[key]
	if !ok {
		s.next[kind]++
		n = s.next[kind]
		s.numbers[key] = n
	}
	s.counts[kind]++
	return "[" + kind + "-" + strconv.Itoa(n) + "]"
}

// Counts says how many places were changed, by kind.
func (s *Sanitizer) Counts() map[string]int {
	out := make(map[string]int, len(s.counts))
	for k, v := range s.counts {
		out[k] = v
	}
	return out
}

// Attribute names that say what a value is, whatever it looks like.
var (
	titleKeys = map[string]bool{"title": true, "name": true, "game": true, "show": true, "album": true, "artist": true, "label": true, "movie": true, "episode": true, "series": true, "channel": true, "q": true, "query": true, "rom": true, "disc": true, "tuner": true, "subject": true, "target": true, "targetplain": true, "names": true, "track": true, "brand": true, "brandname": true, "servername": true}
	userKeys  = map[string]bool{"user": true, "account": true, "username": true, "by": true, "owner": true, "profile": true, "login": true, "who": true}
	hostKeys  = map[string]bool{"host": true, "hostname": true, "computer": true, "machine": true}
	ipKeys    = map[string]bool{"ip": true, "remote": true, "remoteaddr": true, "client": true, "clientip": true, "peer": true, "from": true}
	pathKeys  = map[string]bool{"path": true, "file": true, "dir": true, "folder": true, "src": true, "source": true, "image": true, "root": true, "data": true, "cwd": true, "exe": true, "binary": true, "datadir": true, "logdir": true, "cachedir": true, "ffmpeg": true}
	emailKeys = map[string]bool{"email": true, "mail": true}
	urlKeys   = map[string]bool{"url": true, "uri": true, "address": true, "link": true, "feed": true}
	macKeys   = map[string]bool{"mac": true, "macaddress": true}
	nameKeys  = map[string]string{"library": "library", "device": "device"}
)

var (
	windowsPath = regexp.MustCompile(`(?:\b[A-Za-z]:[\\/]|\\\\[^\\\s]+\\)[^"'\n\r]*?(?:: |"|'|$|\r|\n)`)
	// A folder's name may have spaces when another folder follows it; a file's only when it ends in an extension.
	unixPath   = regexp.MustCompile(`(^|[\s"'=(,\[])(/{1,2}(?:[^/\s"'\n:]+(?: [^/\s"'\n:]+)*/)*[^/\s"'\n:]+(?:[^/"'\n:]*\.[A-Za-z][A-Za-z0-9]{1,4}\b)?)`)
	webAddress = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	metaLine   = regexp.MustCompile(`(?im)^(\s*(?:title|album|artist|album_artist|comment|show|description|synopsis|encoded_by|filename|handler_name|service_name|service_provider|publisher|network)\s*:\s*)(.*)$`)
	metaArg    = regexp.MustCompile(`(?i)(\b(?:title|album|artist|album_artist|comment|show|description|synopsis|encoded_by|filename)=)("[^"]*"|'[^']*'|.*?)(\s+-[A-Za-z]|$)`)
	fromName   = regexp.MustCompile(`(?i)\b(from|to) '([^']*)'`)
	macAddress = regexp.MustCompile(`\b[0-9A-Fa-f]{2}(?:[:-][0-9A-Fa-f]{2}){5}\b|\b[0-9A-Fa-f]{4}\.[0-9A-Fa-f]{4}\.[0-9A-Fa-f]{4}\b`)
	email      = regexp.MustCompile(`(?i)\b[a-z0-9._+-]+(?:@|%40)[a-z0-9-]+(?:\.[a-z0-9-]+)*\.[a-z]{2,}\b`)
	ipv4       = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	ipv6Run    = regexp.MustCompile(`[0-9A-Fa-f:.]*:[0-9A-Fa-f:.]*:[0-9A-Fa-f:.]*(?:%[0-9A-Za-z_.-]+)?`)
	// A name with a dot that ends the way network names do.
	dnsName = regexp.MustCompile(`(?i)\b(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+(?:local|lan|home|internal|intranet|localdomain|arpa|box|corp|com|net|org|io|tv|dev|app|me|co|uk|de|fr|nl|eu|ca|au|us|info|xyz|online|site|cloud)\b`)
	// How Go's resolver names a host it couldn't find, even a bare computer name.
	lookupName = regexp.MustCompile(`(?i)\b(lookup )([a-z0-9][a-z0-9._-]*)`)
	percentRun = regexp.MustCompile(`[^\s"'&=?/\\]*%[0-9A-Fa-f]{2}[^\s"'&=?/\\]*`)
	plainRoute = regexp.MustCompile(`^[a-z0-9_.-]{1,40}$`)
)

// These name the project and its libraries: they say nothing about anyone.
var publicHosts = map[string]bool{"github.com": true, "golang.org": true, "modernc.org": true, "localhost": true}

// keepsPath: the app's own addresses name routes, not anyone's files.
func keepsPath(p string) bool {
	return strings.HasPrefix(p, "/assets/") || p == "/"
}

// standPath hides a path, keeping which known folder it's in and the kind of file it is.
func (s *Sanitizer) standPath(p string) string {
	ext := ""
	if e := filepath.Ext(strings.ReplaceAll(p, `\`, "/")); len(e) >= 2 && len(e) <= 6 && isAlnum(e[1:]) {
		ext = strings.ToLower(e)
	}
	slashed := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(p, `\\`, `\`), `\`, "/"))
	for _, f := range s.folders {
		if slashed == f.slashed || slashed == f.slashed+"/" {
			return s.stand("folder", f.slashed)
		}
		if strings.HasPrefix(slashed, f.slashed+"/") {
			return s.stand("folder", f.slashed) + "/" + s.stand("path", slashed) + ext
		}
	}
	return s.stand("path", slashed) + ext
}

func isASCII(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] >= 0x80 {
			return false
		}
	}
	return true
}

func isAlnum(v string) bool {
	for _, r := range v {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// route keeps the shape of one of the app's own addresses: the words and numbers of the
// route stay, its query goes, and anything else in it is hidden.
func (s *Sanitizer) route(p string) string {
	q := ""
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p, q = p[:i], "?…"
	}
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if part != "" && part != hidden && !plainRoute.MatchString(part) {
			parts[i] = s.standPath(part)
		}
	}
	return strings.Join(parts, "/") + q
}

// webAddr hides an address in parts: its login goes, its host and path have placeholders.
func (s *Sanitizer) webAddr(m string) string {
	i := strings.Index(m, "://")
	if i < 0 {
		return s.stand("address", m)
	}
	scheme, rest := m[:i+3], m[i+3:]
	path := ""
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest, path = rest[:j], rest[j:]
	}
	if at := strings.LastIndexByte(rest, '@'); at >= 0 {
		rest = rest[at+1:]
		s.counts["login"]++
		scheme += hidden + "@"
	}
	host, port := rest, ""
	if strings.HasPrefix(host, "[") {
		if end := strings.IndexByte(host, ']'); end > 0 {
			host, port = host[:end+1], host[end+1:]
		}
	} else if j := strings.LastIndexByte(host, ':'); j >= 0 {
		host, port = host[:j], host[j:]
	}
	if host != "" {
		bare := strings.Trim(host, "[]")
		switch a, err := netip.ParseAddr(bare); {
		case err == nil && keepsAddr(a):
		case err == nil:
			host = s.stand("ip", a.String())
		case strings.HasPrefix(host, "[") || publicHosts[strings.ToLower(host)]:
		default:
			host = s.stand("host", strings.ToLower(host))
		}
	}
	out := scheme + host + port
	if path != "" {
		switch {
		case strings.HasPrefix(path, "/api/"):
			out += s.route(path)
		case path == "/":
			out += "/"
		case strings.HasPrefix(path, "?") || strings.HasPrefix(path, "#"):
			out += "?…"
		default:
			clean := path
			if j := strings.IndexAny(clean, "?#"); j >= 0 {
				clean = clean[:j]
			}
			out += "/" + s.standPath(strings.TrimPrefix(clean, "/"))
		}
	}
	return out
}

// keepsAddr: this machine, or no address at all, says nothing about anyone and often
// explains a problem.
func keepsAddr(a netip.Addr) bool { return a.IsLoopback() || a.IsUnspecified() }

// Text hides what's in free text: an error, what FFmpeg said.
func (s *Sanitizer) Text(v string) string { return s.text(v, false) }

func (s *Sanitizer) text(v string, message bool) string {
	if v == "" {
		return v
	}
	v = Redact(v)
	v = metaLine.ReplaceAllStringFunc(v, func(m string) string {
		sub := metaLine.FindStringSubmatch(m)
		if strings.TrimSpace(sub[2]) == "" {
			return m
		}
		return sub[1] + s.stand("title", strings.TrimSpace(sub[2]))
	})
	v = metaArg.ReplaceAllStringFunc(v, func(m string) string {
		sub := metaArg.FindStringSubmatch(m)
		return sub[1] + s.stand("title", sub[2]) + sub[3]
	})
	v = fromName.ReplaceAllStringFunc(v, func(m string) string {
		sub := fromName.FindStringSubmatch(m)
		if strings.HasPrefix(sub[2], "pipe:") || sub[2] == "" {
			return m
		}
		return sub[1] + " " + s.standPath(sub[2])
	})
	v = webAddress.ReplaceAllStringFunc(v, s.webAddr)
	v = email.ReplaceAllStringFunc(v, func(m string) string {
		return s.stand("email", strings.ReplaceAll(strings.ToLower(m), "%40", "@"))
	})
	v = windowsPath.ReplaceAllStringFunc(v, func(m string) string {
		end, body := "", m
		for _, tail := range []string{": ", `"`, "'", "\r", "\n"} {
			if strings.HasSuffix(m, tail) {
				end, body = tail, strings.TrimSuffix(m, tail)
				break
			}
		}
		return s.standPath(body) + end
	})
	v = unixPath.ReplaceAllStringFunc(v, func(m string) string {
		sub := unixPath.FindStringSubmatch(m)
		switch {
		case len(sub) < 3 || keepsPath(sub[2]):
			return m
		case strings.HasPrefix(sub[2], "/api/"):
			return sub[1] + s.route(sub[2])
		}
		return sub[1] + s.standPath(sub[2])
	})
	v = macAddress.ReplaceAllStringFunc(v, func(m string) string {
		return s.stand("mac", strings.ToLower(strings.NewReplacer("-", ":", ".", "").Replace(m)))
	})
	v = s.hideIPv6(v)
	v = s.hideIPv4(v)
	v = dnsName.ReplaceAllStringFunc(v, func(m string) string {
		if publicHosts[strings.ToLower(m)] {
			return m
		}
		return s.stand("host", m)
	})
	v = lookupName.ReplaceAllStringFunc(v, func(m string) string {
		sub := lookupName.FindStringSubmatch(m)
		if publicHosts[strings.ToLower(sub[2])] || strings.HasPrefix(sub[2], "[") {
			return m
		}
		return sub[1] + s.stand("host", sub[2])
	})
	if strings.IndexByte(v, '%') >= 0 {
		v = percentRun.ReplaceAllStringFunc(v, s.encoded)
	}
	return s.known(v, message)
}

// encoded looks inside something percent-encoded: if what it says has anything to hide,
// all of it is hidden.
func (s *Sanitizer) encoded(m string) string {
	plain, err := url.QueryUnescape(m)
	if err != nil || plain == m {
		return m
	}
	// Encoded words are someone's text, not the program's.
	if strings.ContainsAny(plain, " /\\@") || !isASCII(plain) {
		return s.stand("name", plain)
	}
	// A copy, so that looking doesn't change the numbering or the counts.
	probe := &Sanitizer{numbers: map[string]int{}, next: map[string]int{}, counts: map[string]int{}, folders: s.folders, words: s.words}
	if probe.text(plain, false) == plain {
		return m
	}
	return s.stand("name", plain)
}

func (s *Sanitizer) hideIPv4(v string) string {
	if strings.Count(v, ".") < 3 {
		return v
	}
	return replaceAt(v, ipv4, func(m string, start, end int) string {
		if start > 0 {
			if c := v[start-1]; c == 'v' || c == 'V' || c == '.' {
				return m // a version, or a piece of a longer one
			}
		}
		a, err := netip.ParseAddr(m)
		if err != nil || keepsAddr(a) {
			return m
		}
		return s.stand("ip", a.String())
	})
}

func (s *Sanitizer) hideIPv6(v string) string {
	if strings.Count(v, ":") < 2 {
		return v
	}
	return replaceAt(v, ipv6Run, func(m string, start, end int) string {
		// std::string and the like: it runs into more of a word.
		if start > 0 && isWordByte(v[start-1]) || end < len(v) && isWordByte(v[end]) {
			return m
		}
		body, _, _ := strings.Cut(m, "%")
		trail := ""
		if t := strings.TrimRight(body, ":."); t != body && !strings.HasSuffix(body, "::") {
			trail, body = body[len(t):], t
		}
		a, err := netip.ParseAddr(body)
		if err != nil || !a.Is6() {
			return m
		}
		if keepsAddr(a) {
			return m
		}
		return s.stand("ip", a.String()) + trail
	})
}

func isWordByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 0x80
}

// replaceAt is ReplaceAllStringFunc for rules that need to see around a match.
func replaceAt(v string, re *regexp.Regexp, fn func(m string, start, end int) string) string {
	spots := re.FindAllStringIndex(v, -1)
	if spots == nil {
		return v
	}
	var b strings.Builder
	last := 0
	for _, at := range spots {
		b.WriteString(v[last:at[0]])
		b.WriteString(fn(v[at[0]:at[1]], at[0], at[1]))
		last = at[1]
	}
	b.WriteString(v[last:])
	return b.String()
}

// known hides the exact names this server knows, by looking up each word, so that it
// costs the same with a hundred thousand titles as with ten.
func (s *Sanitizer) known(v string, message bool) string {
	if len(s.words) == 0 {
		return v
	}
	var b strings.Builder
	last, i := 0, 0
	for i < len(v) {
		r, n := utf8.DecodeRuneInString(v[i:])
		if !isWordRune(r) {
			i += n
			continue
		}
		end := i + n
		for end < len(v) {
			r2, n2 := utf8.DecodeRuneInString(v[end:])
			if !isWordRune(r2) {
				break
			}
			end += n2
		}
		word := v[i:end]
		replaced := false
		if list := s.words[strings.ToLower(word)]; list != nil {
			for _, e := range list {
				if e.one && message && e.kind == "title" {
					continue
				}
				if m := matchKnown(v[i:], e); m > 0 {
					b.WriteString(v[last:i])
					b.WriteString(s.stand(e.kind, e.text))
					i += m
					last = i
					replaced = true
					break
				}
			}
		}
		if !replaced {
			i = end
		}
	}
	if last == 0 {
		return v
	}
	b.WriteString(v[last:])
	return b.String()
}

// matchKnown says how long the match at the start of v is, or 0. A word of a title or a
// name must end where it ends, so "malice" isn't taken for "Alice". One word alone must be
// spelt exactly, unless it's a name of an account or a machine, which don't depend on case.
func matchKnown(v string, e knownEntry) int {
	n := len(e.text)
	if len(v) < n {
		return 0
	}
	got := v[:n]
	exact := e.one && e.kind == "title" || e.one && e.kind == "library"
	if exact {
		if got != e.text {
			return 0
		}
	} else if !strings.EqualFold(got, e.text) {
		return 0
	}
	if n < len(v) {
		if r, _ := utf8.DecodeRuneInString(v[n:]); isWordRune(r) {
			return 0
		}
	}
	return n
}

// Attr hides one attribute's value, by its name and by what's in it.
func (s *Sanitizer) Attr(k, v string) string {
	if v == "" {
		return v
	}
	if secretKey(k) {
		return hidden
	}
	low := strings.ToLower(k)
	if i := strings.LastIndexByte(low, '.'); i >= 0 {
		low = low[i+1:]
	}
	switch {
	case low == "p": // the page of a web request
		if strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//") {
			return s.route(Redact(v))
		}
		return s.Text(v)
	case low == "stack":
		return s.stack(v)
	case titleKeys[low]:
		return s.stand("title", v)
	case userKeys[low]:
		return s.stand("user", v)
	case hostKeys[low]:
		if a, err := netip.ParseAddr(v); err == nil {
			if keepsAddr(a) {
				return v
			}
			return s.stand("ip", a.String())
		}
		if publicHosts[strings.ToLower(v)] {
			return v
		}
		return s.stand("host", v)
	case urlKeys[low]:
		// A whole address, even with spaces in it, which free text couldn't tell.
		if strings.Contains(v, "://") {
			return s.webAddr(Redact(v))
		}
		return s.Text(v)
	case emailKeys[low]:
		return s.stand("email", v)
	case macKeys[low]:
		return s.stand("mac", strings.ToLower(strings.ReplaceAll(v, "-", ":")))
	case ipKeys[low]:
		return s.Text(v)
	case nameKeys[low] != "":
		if isNumber(v) {
			return v
		}
		return s.stand(nameKeys[low], v)
	case pathKeys[low]:
		if keepsPath(v) {
			return v
		}
		if strings.HasPrefix(v, "/api/") {
			return s.route(Redact(v))
		}
		if strings.Contains(v, "://") {
			return s.Text(v)
		}
		return s.standPath(v)
	}
	return s.Text(v)
}

func isNumber(v string) bool {
	_, err := strconv.ParseFloat(v, 64)
	return err == nil
}

// stack keeps the program's own source lines, which are what a stack is for. A copy built at
// home has someone's home folder in them, so those lines lose whatever comes before the code.
func (s *Sanitizer) stack(v string) string {
	lines := strings.Split(v, "\n")
	for i, line := range lines {
		lines[i] = s.crashLine(line)
	}
	return strings.Join(lines, "\n")
}

func (s *Sanitizer) crashLine(line string) string {
	if strings.HasPrefix(line, "\t") && goSourceLine.MatchString(line) {
		return s.Final(sourceLine(line))
	}
	return s.Text(line)
}

// sourceLine keeps a stack's file, line and offset, and the part of the path from where the
// code starts, which is the same on every machine.
func sourceLine(line string) string {
	body := strings.ReplaceAll(strings.TrimPrefix(line, "\t"), `\`, "/")
	at := goSourceLine.FindStringIndex(body)
	if at == nil {
		return line
	}
	file := body[:at[1]]
	rest := body[at[1]:]
	for _, mark := range []string{"/pkg/mod/", "/src/", "/internal/", "/cmd/"} {
		if i := strings.LastIndex(file, mark); i >= 0 {
			start := i + len(mark)
			if mark == "/internal/" || mark == "/cmd/" {
				start = i + 1
			}
			return "\t…/" + file[start:] + rest
		}
	}
	if i := strings.LastIndexByte(file, '/'); i >= 0 {
		return "\t…/" + file[i+1:] + rest
	}
	return line
}

// Record hides everything in a whole record that isn't the server's own vocabulary.
func (s *Sanitizer) Record(rec *Record) {
	rec.Msg = s.text(rec.Msg, true)
	rec.Session = s.Text(rec.Session)
	rec.Req = s.Text(rec.Req)
	rec.Module = s.Text(rec.Module)
	rec.Topic = s.Text(rec.Topic)
	for i := range rec.Attrs {
		rec.Attrs[i].V = s.Attr(rec.Attrs[i].K, rec.Attrs[i].V)
	}
}

// Final is the last look at a finished line: any known folder or name still there goes,
// however it got through. Single common words were never known, so it can't blot those out.
func (s *Sanitizer) Final(line string) string {
	lower := strings.ToLower(line)
	if len(lower) == len(line) { // otherwise the two can't be lined up, and known names below still apply
		for _, f := range s.folders {
			for _, form := range []string{f.lower, f.slashed, strings.ReplaceAll(f.lower, `\`, `\\`)} {
				for {
					at := strings.Index(lower, form)
					if at < 0 {
						break
					}
					line = line[:at] + s.stand("folder", f.slashed) + line[at+len(form):]
					lower = strings.ToLower(line)
				}
			}
		}
	}
	return s.known(line, true)
}

// KnownFromDB reads the names a sanitized bundle should hide. A query that fails is left
// out: a broken database mustn't stop someone getting their logs.
func KnownFromDB(ctx context.Context, q interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}) Known {
	var k Known
	read := func(limit int, query string, into *[]string) {
		rows, err := q.QueryContext(ctx, query)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() && (limit == 0 || len(*into) < limit) {
			var v sql.NullString
			if rows.Scan(&v) == nil && v.String != "" {
				*into = append(*into, v.String)
			}
		}
	}
	read(0, `SELECT name FROM users`, &k.Users)
	read(0, `SELECT name FROM libraries`, &k.Libraries)
	read(0, `SELECT path FROM library_paths`, &k.Folders)
	read(0, `SELECT value FROM settings WHERE key = 'brandName'`, &k.Libraries)
	read(0, `SELECT value FROM settings WHERE key = 'logDir'`, &k.Folders)
	read(0, `SELECT value FROM settings WHERE key = 'publicUrl'`, &k.Hosts)
	read(maxKnownTitles, `SELECT DISTINCT title FROM shows`, &k.Titles)
	read(maxKnownTitles, `SELECT DISTINCT title FROM items`, &k.Titles)
	for i, h := range k.Hosts {
		if u, err := url.Parse(h); err == nil && u.Host != "" {
			k.Hosts[i] = u.Hostname()
		}
	}
	return k
}
