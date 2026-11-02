package metafolder

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// How bad a problem is. An error means the file is skipped; a warning means something
// in it was left out or changed, and the rest is used.
const (
	SevError   = "error"
	SevWarning = "warning"
)

// Format is the version of the folder's files.
const Format = 1

// Limits on one file.
const (
	MaxJSON  = 256 << 10
	MaxNFO   = 1 << 20
	maxDepth = 8
	maxNodes = 200_000
)

// The two things a file can describe.
const (
	TargetItem = "item"
	TargetShow = "show"
)

// Problem is something wrong with a file, in words, and where it is.
type Problem struct {
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Field    string `json:"field,omitempty"`
	Msg      string `json:"msg"`
	Severity string `json:"severity"`
}

// String is how the command line shows it: file:line: what.
func (p Problem) String() string {
	where := p.File
	if p.Line > 0 {
		where += ":" + strconv.Itoa(p.Line)
	}
	return where + ": " + p.Msg
}

// Art is a reference to a picture: a web address, a file beside this one, or nothing,
// which takes the picture away.
type Art struct {
	URL  string `json:"url,omitempty"`
	File string `json:"file,omitempty"`
}

// Doc is what a file says, cleaned and checked. Fields hold text in the form a title's
// fields take it: a number as digits, a list joined with "|". Only what the file
// mentions is there, so a file can be cut down to the one thing it changes. Art is apart
// because a picture has to be checked and copied, not just written down.
type Doc struct {
	File       string            `json:"file"`
	Kind       string            `json:"kind"` // json or nfo
	Target     string            `json:"target,omitempty"`
	ID         int64             `json:"id,omitempty"`
	Path       string            `json:"path,omitempty"` // what it was exported for, to catch a mix-up
	Fields     map[string]string `json:"fields"`
	Art        map[string]Art    `json:"art,omitempty"`
	Lines      map[string]int    `json:"-"`
	Locked     []string          `json:"locked,omitempty"`
	HasLocked  bool              `json:"-"`
	Provider   string            `json:"provider,omitempty"`
	ProviderID string            `json:"providerId,omitempty"`
}

// HasErrors says whether any of these stops a file being used.
func HasErrors(ps []Problem) bool {
	for _, p := range ps {
		if p.Severity == SevError {
			return true
		}
	}
	return false
}

// What a field holds, and its limits. These are the limits a title's fields have when
// a person types into them.
type fieldKind int

const (
	kindLine fieldKind = iota
	kindText
	kindList
	kindInt
	kindFloat
	kindArt
)

type fieldSpec struct {
	kind     fieldKind
	max      int     // text: characters. list: items
	each     int     // list: characters in each
	lo, hi   float64 // numbers
	required bool    // can't be emptied
	hint     string  // an example, for a message
}

var fields = map[string]fieldSpec{
	"title":         {kind: kindLine, max: 300, required: true},
	"sortTitle":     {kind: kindLine, max: 300},
	"year":          {kind: kindInt, lo: 1800, hi: 3000, hint: "a year such as 2001"},
	"overview":      {kind: kindText, max: 20000},
	"tagline":       {kind: kindLine, max: 500},
	"genres":        {kind: kindList, max: 30, each: 60},
	"tags":          {kind: kindList, max: 50, each: 60},
	"contentRating": {kind: kindLine, max: 20},
	"rating":        {kind: kindFloat, lo: 0, hi: 10, hint: "a number from 0 to 10"},
	"studios":       {kind: kindList, max: 12, each: 120},
	"network":       {kind: kindLine, max: 120},
	"status":        {kind: kindLine, max: 40},
	"director":      {kind: kindLine, max: 300},
	"edition":       {kind: kindList, max: 8, each: 60},
	"aired":         {kind: kindLine, max: 20},
	"season":        {kind: kindInt, lo: 0, hi: 9999, hint: "a season number"},
	"episode":       {kind: kindInt, lo: 0, hi: 99999, hint: "an episode number"},
	"episodeEnd":    {kind: kindInt, lo: 0, hi: 99999, hint: "an episode number"},
	"introStart":    {kind: kindFloat, lo: 0, hi: 86400, hint: "a time in seconds"},
	"introEnd":      {kind: kindFloat, lo: 0, hi: 86400, hint: "a time in seconds"},
	"poster":        {kind: kindArt},
	"backdrop":      {kind: kindArt},
	"banner":        {kind: kindArt},
	"logo":          {kind: kindArt},
}

// IsField says whether a name is a field of a title.
func IsField(name string) bool { _, ok := fields[name]; return ok }

// Clean is a stored value as this package would have read it: a value that only differs
// from it by what cleaning removes hasn't changed.
func Clean(field, v string) string {
	spec, ok := fields[field]
	if !ok {
		return v
	}
	switch spec.kind {
	case kindLine:
		return Line(v, spec.max)
	case kindText:
		return Text(v, spec.max)
	case kindList:
		return strings.Join(List(strings.Split(v, "|"), spec.max, spec.each), "|")
	}
	return v
}

// ValueFor is a stored value as a file shows it: a number as a number, a list as a list,
// text cleaned. ok is false for what a file leaves out: a picture that isn't a web
// address (the server's own, which a file can't name) and times nobody has set.
func ValueFor(field, stored string) (v any, ok bool) {
	spec, known := fields[field]
	if !known {
		return nil, false
	}
	switch spec.kind {
	case kindLine, kindText:
		return Clean(field, stored), true
	case kindList:
		return List(strings.Split(stored, "|"), spec.max, spec.each), true
	case kindInt, kindFloat:
		if strings.TrimSpace(stored) == "" {
			if field == "introStart" || field == "introEnd" {
				return nil, false
			}
			return nil, true
		}
		n, err := strconv.ParseFloat(stored, 64)
		if err != nil {
			return nil, true
		}
		if spec.kind == kindInt {
			return int64(n), true
		}
		return n, true
	case kindArt:
		if u, good := URL(stored); good {
			return u, true
		}
		return nil, false
	}
	return nil, false
}

// KnownProvider says whether a file can name this service for a match.
func KnownProvider(p string) bool { return providers[p] }

// ProviderID cleans an id from a file: it ends up in addresses, so it has few characters.
func ProviderID(s string) (string, bool) {
	s = strings.TrimSpace(s)
	return s, providerID.MatchString(s)
}

// The controls a file has besides its fields.
var controls = map[string]bool{"wittyMetadata": true, "target": true, "id": true, "path": true, "locked": true, "provider": true, "providerId": true, "_note": true}

// Services a match can name.
var providers = map[string]bool{"tmdb": true, "tvdb": true, "tvmaze": true, "anilist": true, "wikidata": true, "imdb": true}

var providerID = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

// The id in a file or folder's name: "My Film (2001) [item-123].json".
var nameID = regexp.MustCompile(`\[(item|show)-([0-9]{1,18})\]`)

// IDFromName reads the id in a name, if it has one.
func IDFromName(name string) (target string, id int64, ok bool) {
	m := nameID.FindAllStringSubmatch(name, -1)
	if len(m) == 0 {
		return "", 0, false
	}
	last := m[len(m)-1]
	n, err := strconv.ParseInt(last[2], 10, 64)
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return last[1], n, true
}

func lineAt(data []byte, off int64) int {
	if off < 0 {
		return 1
	}
	if off > int64(len(data)) {
		off = int64(len(data))
	}
	return 1 + bytes.Count(data[:off], []byte{'\n'})
}

// firstBad says where the first thing that isn't UTF-8 is, or -1.
func firstBad(b []byte) int {
	for i := 0; i < len(b); {
		r, n := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && n == 1 {
			return i
		}
		i += n
	}
	return -1
}

// jsonDepth is how deeply a file nests, without trusting anything to parse it.
func jsonDepth(b []byte) int {
	depth, most := 0, 0
	inString, escaped := false, false
	for _, c := range b {
		switch {
		case inString:
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '{' || c == '[':
			depth++
			if depth > most {
				most = depth
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return most
}

// plainJSONError says what the parser means, for someone who has never read JSON.
func plainJSONError(msg string) string {
	switch {
	case strings.Contains(msg, "unexpected end of JSON"):
		return "The file ends too soon. Something is missing, probably a closing } or \" at the end"
	case strings.Contains(msg, "looking for beginning of object key string"):
		return "Something is wrong just before here. Often it's a comma after the last line in the file, which isn't allowed, or a name without \"quotes\""
	case strings.Contains(msg, "after object key:value pair"):
		return "A comma is missing at the end of the line before this one, or a \" that ends the text on it"
	case strings.Contains(msg, "after array element"):
		return "A comma or a closing ] is missing in a list"
	case strings.Contains(msg, "in string literal"), strings.Contains(msg, "in string escape code"):
		return "There's something a text can't hold as it is. A line break must be written \\n, a \" must be written \\\", and a \\ must be written \\\\"
	case strings.Contains(msg, "looking for beginning of value"):
		return "This line has something that isn't a value. Text needs \"quotes\", a number doesn't, and a list goes in [square brackets]"
	case strings.Contains(msg, "after top-level value"):
		return "There's something after the closing } at the end of the file. Everything has to be inside the { and }"
	}
	return "This isn't valid JSON: " + msg
}

// closeName is the field someone probably meant.
func closeName(name string) string {
	best, bestD := "", 3
	low := strings.ToLower(name)
	names := make([]string, 0, len(fields))
	for f := range fields {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		if strings.ToLower(f) == low {
			return f
		}
		if d := editDistance(low, strings.ToLower(f)); d < bestD {
			best, bestD = f, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	if len(a) > 40 || len(b) > 40 {
		return 99
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			c := prev[j-1]
			if a[i-1] != b[j-1] {
				c++
			}
			cur[j] = min(c, prev[j]+1, cur[j-1]+1)
		}
		prev = cur
	}
	return prev[len(b)]
}

// ParseJSON reads one file of the folder. It always answers what it could read, and
// every problem with its line. The name gives the id if the file doesn't.
func ParseJSON(name string, data []byte) (*Doc, []Problem) {
	var ps []Problem
	fail := func(line int, field, msg string) {
		ps = append(ps, Problem{File: name, Line: line, Field: field, Msg: msg, Severity: SevError})
	}
	warn := func(line int, field, msg string) {
		ps = append(ps, Problem{File: name, Line: line, Field: field, Msg: msg, Severity: SevWarning})
	}
	d := &Doc{File: name, Kind: "json", Fields: map[string]string{}, Art: map[string]Art{}, Lines: map[string]int{}}
	if len(data) > MaxJSON {
		fail(0, "", fmt.Sprintf("This file is too big to be one title's information: the limit is %s", sizeWords(MaxJSON)))
		return d, ps
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		fail(1, "", "This file is saved as UTF-16. Save it as UTF-8 in your text editor")
		return d, ps
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if off := firstBad(data); off >= 0 {
		fail(lineAt(data, int64(off)), "", "This line has characters that aren't UTF-8. Save the file as UTF-8 in your text editor")
		return d, ps
	}
	if len(bytes.TrimSpace(data)) == 0 {
		fail(1, "", "This file is empty")
		return d, ps
	}
	if jsonDepth(data) > maxDepth {
		fail(0, "", "This file has more levels inside each other than a title's information ever needs")
		return d, ps
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syn):
			fail(lineAt(data, syn.Offset), "", plainJSONError(syn.Error()))
		case errors.As(err, &typ):
			fail(lineAt(data, typ.Offset), "", "The whole file has to be one { ... } with names and values inside")
		default:
			fail(1, "", plainJSONError(err.Error()))
		}
		return d, ps
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil {
		fail(1, "", plainJSONError(err.Error()))
		return d, ps
	}
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			fail(lineAt(data, dec.InputOffset()), "", plainJSONError(err.Error()))
			return d, ps
		}
		key, _ := keyTok.(string)
		line := lineAt(data, dec.InputOffset())
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			fail(line, key, plainJSONError(err.Error()))
			return d, ps
		}
		if seen[key] {
			fail(line, key, "\""+key+"\" is here twice. Keep only one")
			continue
		}
		seen[key] = true
		if len(seen) > 64 {
			fail(line, "", "This file has more entries than a title has fields")
			return d, ps
		}
		if controls[key] {
			readControl(d, key, raw, line, fail, warn)
			continue
		}
		spec, ok := fields[key]
		if !ok {
			msg := "\"" + Line(key, 60) + "\" isn't something this file can set, so it was left out"
			if near := closeName(key); near != "" {
				msg += ". Did you mean \"" + near + "\"?"
			}
			warn(line, Line(key, 60), msg)
			continue
		}
		d.Lines[key] = line
		readField(d, key, spec, raw, line, fail, warn)
	}
	finishDoc(d, name, seen["id"], fail)
	return d, ps
}

type report func(line int, field, msg string)

func readControl(d *Doc, key string, raw json.RawMessage, line int, fail, warn report) {
	switch key {
	case "wittyMetadata":
		var n int
		if json.Unmarshal(raw, &n) != nil {
			fail(line, key, "\"wittyMetadata\" has to be a number. Leave it as it was exported")
		} else if n > Format {
			fail(line, key, "This file was written by a newer version of WittyMediaServer. Update the server, or export the folder again")
		}
	case "target":
		var s string
		if json.Unmarshal(raw, &s) != nil || (s != TargetItem && s != TargetShow) {
			fail(line, key, "\"target\" has to be \"item\" or \"show\". Leave it as it was exported")
			return
		}
		d.Target = s
	case "id":
		var n int64
		if json.Unmarshal(raw, &n) != nil || n <= 0 {
			fail(line, key, "\"id\" has to be a whole number. Leave it as it was exported")
			return
		}
		d.ID = n
		d.Lines["id"] = line
	case "path":
		var s string
		if json.Unmarshal(raw, &s) != nil {
			fail(line, key, "\"path\" has to be text in \"quotes\". Leave it as it was exported")
			return
		}
		d.Path = clipRunes(dropHidden(s), 4096)
		d.Lines["path"] = line
	case "provider":
		var s string
		if json.Unmarshal(raw, &s) != nil {
			fail(line, key, "\"provider\" has to be text in \"quotes\"")
			return
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && !providers[s] {
			warn(line, key, "\""+Line(s, 30)+"\" isn't a service this can name, so it was left out. Use tmdb, tvdb, tvmaze, anilist, wikidata or imdb")
			return
		}
		d.Provider = s
		d.Lines["provider"] = line
	case "providerId":
		var s string
		if json.Unmarshal(raw, &s) != nil {
			var n json.Number
			if json.Unmarshal(raw, &n) != nil {
				fail(line, key, "\"providerId\" has to be text in \"quotes\"")
				return
			}
			s = n.String()
		}
		s = strings.TrimSpace(s)
		if s != "" && !providerID.MatchString(s) {
			fail(line, key, "That doesn't look like an id. An id has only letters, digits and - _ . :")
			return
		}
		d.ProviderID = s
		d.Lines["providerId"] = line
	case "locked":
		var list []string
		if json.Unmarshal(raw, &list) != nil {
			fail(line, key, "\"locked\" has to be a list of field names, such as [\"title\", \"year\"]")
			return
		}
		d.HasLocked = true
		d.Lines["locked"] = line
		for _, f := range list {
			if !IsField(f) {
				warn(line, key, "\""+Line(f, 60)+"\" isn't a field, so it can't be locked")
				continue
			}
			d.Locked = append(d.Locked, f)
		}
	}
}

func readField(d *Doc, key string, spec fieldSpec, raw json.RawMessage, line int, fail, warn report) {
	isNull := string(bytes.TrimSpace(raw)) == "null"
	cleaned := func(before, after string) {
		// Spaces at the ends and limits on length aren't worth a word.
		if strings.TrimSpace(before) != strings.TrimSpace(after) && len([]rune(before)) <= fieldMaxOr(spec) {
			warn(line, key, "HTML or hidden characters were taken out of \""+key+"\"")
		}
	}
	switch spec.kind {
	case kindLine, kindText:
		var s string
		if !isNull && json.Unmarshal(raw, &s) != nil {
			fail(line, key, "\""+key+"\" has to be text in \"quotes\"")
			return
		}
		var v string
		if spec.kind == kindLine {
			v = Line(s, spec.max)
		} else {
			v = Text(s, spec.max)
		}
		cleaned(strings.ReplaceAll(s, "\r\n", "\n"), v)
		if v == "" && spec.required {
			fail(line, key, "A title can't be empty")
			return
		}
		d.Fields[key] = v
	case kindList:
		var list []string
		if !isNull && json.Unmarshal(raw, &list) != nil {
			fail(line, key, "\""+key+"\" has to be a list in [square brackets], such as [\"Drama\", \"Comedy\"]")
			return
		}
		out := List(list, spec.max, spec.each)
		for _, v := range list {
			if c := Line(v, spec.each); c != strings.TrimSpace(v) && len([]rune(v)) <= spec.each {
				warn(line, key, "HTML or hidden characters were taken out of \""+key+"\"")
				break
			}
		}
		d.Fields[key] = strings.Join(out, "|")
	case kindInt, kindFloat:
		if isNull {
			d.Fields[key] = ""
			return
		}
		var n float64
		if json.Unmarshal(raw, &n) != nil || n != n {
			fail(line, key, "\""+key+"\" has to be "+spec.hint+", with no \"quotes\"")
			return
		}
		if n < spec.lo || n > spec.hi || (spec.kind == kindInt && n != float64(int64(n))) {
			fail(line, key, "\""+key+"\" has to be "+spec.hint)
			return
		}
		if spec.kind == kindInt {
			d.Fields[key] = strconv.FormatInt(int64(n), 10)
		} else {
			d.Fields[key] = strconv.FormatFloat(n, 'f', -1, 64)
		}
	case kindArt:
		var s string
		if !isNull && json.Unmarshal(raw, &s) != nil {
			fail(line, key, "\""+key+"\" has to be text in \"quotes\": a web address or a file name")
			return
		}
		art, msg := ParseArt(s)
		if msg != "" {
			fail(line, key, "\""+key+"\": "+msg)
			return
		}
		d.Art[key] = art
	}
}

func fieldMaxOr(spec fieldSpec) int {
	if spec.max > 0 {
		return spec.max
	}
	return 1 << 30
}

// ParseArt reads a reference to a picture. The message is empty if it's fine.
func ParseArt(s string) (Art, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Art{}, ""
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "://") || strings.HasPrefix(low, "javascript:") || strings.HasPrefix(low, "data:") || strings.HasPrefix(low, "file:") {
		u, ok := URL(s)
		if !ok {
			return Art{}, "only http and https addresses are accepted, with no login in them"
		}
		return Art{URL: u}, ""
	}
	n, ok := FileName(s)
	if !ok {
		return Art{}, "a picture has to be a web address or the plain name of a file in the same folder as this file, with no folders or .. in it"
	}
	return Art{File: n}, ""
}

func finishDoc(d *Doc, name string, hasID bool, fail report) {
	clean := strings.ReplaceAll(name, `\`, "/")
	base := path.Base(clean)
	target, id, ok := IDFromName(base)
	if !ok && base == "show.json" {
		target, id, ok = IDFromName(path.Base(path.Dir(clean)))
	}
	if !ok {
		return
	}
	if d.ID == 0 {
		d.ID = id
	} else if hasID && d.ID != id {
		fail(d.Lines["id"], "id", fmt.Sprintf("This file's name says it's for %d, but \"id\" says %d. Leave both as they were exported", id, d.ID))
	}
	if d.Target == "" {
		d.Target = target
	} else if d.Target != target {
		fail(d.Lines["id"], "target", "This file's name and \"target\" don't agree on whether it's a film or a show")
	}
}

// ---------------------------------------------------------------- NFO

// nfoDoc is every element of the kinds of NFO this reads.
type nfoDoc struct {
	XMLName   xml.Name
	Title     string   `xml:"title"`
	SortTitle string   `xml:"sorttitle"`
	Year      string   `xml:"year"`
	Plot      string   `xml:"plot"`
	Outline   string   `xml:"outline"`
	Tagline   string   `xml:"tagline"`
	Genres    []string `xml:"genre"`
	Tags      []string `xml:"tag"`
	Rating    string   `xml:"rating"`
	MPAA      string   `xml:"mpaa"`
	Studios   []string `xml:"studio"`
	Director  string   `xml:"director"`
	Status    string   `xml:"status"`
	Premiered string   `xml:"premiered"`
	Aired     string   `xml:"aired"`
	Season    string   `xml:"season"`
	Episode   string   `xml:"episode"`
	Thumb     []string `xml:"thumb"`
	Fanart    struct {
		Thumb []string `xml:"thumb"`
	} `xml:"fanart"`
	UniqueIDs []struct {
		Type string `xml:"type,attr"`
		ID   string `xml:",chardata"`
	} `xml:"uniqueid"`
}

// The kinds of NFO this reads.
var nfoRoots = map[string]bool{"movie": true, "tvshow": true, "episodedetails": true, "musicvideo": true}

// CheckXML looks through XML without believing any of it: it may not declare anything,
// and has limits on how deep and how large it goes. The first thing wrong is answered,
// with its line, or line 0 if it applies to the whole file.
func CheckXML(data []byte) (line int, msg string) {
	if len(data) > 32<<20 {
		return 0, "This file is far too big to be an NFO"
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	depth, nodes := 0, 0
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			return 0, ""
		}
		if err != nil {
			var syn *xml.SyntaxError
			if errors.As(err, &syn) {
				return syn.Line, plainXMLError(syn.Msg)
			}
			return 0, plainXMLError(err.Error())
		}
		if nodes++; nodes > maxNodes {
			return 0, "This file has far more in it than an NFO ever needs"
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if depth++; depth > 64 {
				return lineAt(data, dec.InputOffset()), "This file has more levels inside each other than an NFO ever needs"
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			low := strings.ToLower(string(t))
			if strings.HasPrefix(low, "doctype") || strings.Contains(low, "entity") {
				return lineAt(data, dec.InputOffset()), "A DOCTYPE or ENTITY declaration isn't allowed in an NFO, because it can be used to read other files. Delete that line"
			}
		}
	}
}

func plainXMLError(msg string) string {
	switch {
	case strings.Contains(msg, "element <") && strings.Contains(msg, "closed by"):
		return "A closing tag doesn't match the tag it closes: " + Line(msg, 200)
	case strings.Contains(msg, "unexpected EOF"):
		return "The file ends too soon. A closing tag such as </movie> is probably missing"
	case strings.Contains(msg, "invalid character entity"):
		return "A & has to be written &amp; in an NFO: " + Line(msg, 200)
	}
	return "This isn't valid XML: " + Line(msg, 200)
}

// DecodeXML checks XML and reads it into v. A wrong root element comes back like any
// other problem, if v's type names one.
func DecodeXML(data []byte, v any) (line int, msg string) {
	if line, msg = CheckXML(data); msg != "" {
		return line, msg
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	if err := dec.Decode(v); err != nil {
		var syn *xml.SyntaxError
		if errors.As(err, &syn) {
			return syn.Line, plainXMLError(syn.Msg)
		}
		return 0, plainXMLError(err.Error())
	}
	return 0, ""
}

// ParseNFO reads a hand-written NFO, in the folder or beside media: <movie>, <tvshow>,
// <episodedetails> or <musicvideo>. Pictures in one may only be web addresses.
func ParseNFO(name string, data []byte) (*Doc, []Problem) {
	var ps []Problem
	fail := func(line int, field, msg string) {
		ps = append(ps, Problem{File: name, Line: line, Field: field, Msg: msg, Severity: SevError})
	}
	warn := func(line int, field, msg string) {
		ps = append(ps, Problem{File: name, Line: line, Field: field, Msg: msg, Severity: SevWarning})
	}
	d := &Doc{File: name, Kind: "nfo", Fields: map[string]string{}, Art: map[string]Art{}, Lines: map[string]int{}}
	if len(data) > MaxNFO {
		fail(0, "", fmt.Sprintf("This file is too big to be an NFO: the limit is %s", sizeWords(MaxNFO)))
		return d, ps
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	// Many tools leave a web address after the closing tag; that's only noise.
	data = cutAfterRoot(data)
	var m nfoDoc
	if line, msg := DecodeXML(data, &m); msg != "" {
		fail(line, "", msg)
		return d, ps
	}
	root := m.XMLName.Local
	if !nfoRoots[root] {
		fail(1, "", "This NFO starts with <"+Line(root, 40)+">. An NFO here has to be a <movie>, <tvshow>, <episodedetails> or <musicvideo>")
		return d, ps
	}
	if root == "tvshow" {
		d.Target = TargetShow
	} else {
		d.Target = TargetItem
	}
	line := func(f, v string, max int) {
		if strings.TrimSpace(v) == "" {
			return
		}
		c := Line(v, max)
		if c == "" {
			warn(0, f, "\""+f+"\" had nothing left once HTML and hidden characters were taken out")
			return
		}
		if c != strings.TrimSpace(v) && len([]rune(v)) <= max {
			warn(0, f, "HTML or hidden characters were taken out of \""+f+"\"")
		}
		d.Fields[f] = c
	}
	line("title", m.Title, 300)
	line("sortTitle", m.SortTitle, 300)
	line("tagline", m.Tagline, 500)
	line("contentRating", m.MPAA, 20)
	line("director", m.Director, 300)
	line("status", m.Status, 40)
	line("aired", firstNonEmpty(m.Aired, m.Premiered), 20)
	if p := firstNonEmpty(m.Plot, m.Outline); p != "" {
		d.Fields["overview"] = Text(p, 20000)
	}
	if list := List(m.Genres, 30, 60); len(list) > 0 {
		d.Fields["genres"] = strings.Join(list, "|")
	}
	if list := List(m.Tags, 50, 60); len(list) > 0 {
		d.Fields["tags"] = strings.Join(list, "|")
	}
	if list := List(m.Studios, 12, 120); len(list) > 0 {
		if root == "tvshow" {
			d.Fields["network"] = list[0]
		} else {
			d.Fields["studios"] = strings.Join(list, "|")
		}
	}
	number := func(f, v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		spec := fields[f]
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || n < spec.lo || n > spec.hi || (spec.kind == kindInt && n != float64(int64(n))) {
			fail(0, f, "\""+f+"\" has to be "+spec.hint)
			return
		}
		d.Fields[f] = strconv.FormatFloat(n, 'f', -1, 64)
	}
	year := strings.TrimSpace(m.Year)
	if year == "" && len(strings.TrimSpace(m.Premiered)) >= 4 {
		year = strings.TrimSpace(m.Premiered)[:4]
	}
	number("year", year)
	number("rating", m.Rating)
	number("season", m.Season)
	number("episode", m.Episode)
	pic := func(f string, list []string) {
		for _, t := range list {
			if strings.TrimSpace(t) == "" {
				continue
			}
			if u, ok := URL(t); ok {
				d.Art[f] = Art{URL: u}
				return
			}
			warn(0, f, "A picture in an NFO has to be an http or https address, so \""+Line(t, 80)+"\" was left out")
		}
	}
	pic("poster", m.Thumb)
	pic("backdrop", m.Fanart.Thumb)
	for _, u := range m.UniqueIDs {
		t, id := strings.ToLower(strings.TrimSpace(u.Type)), strings.TrimSpace(u.ID)
		if providers[t] && providerID.MatchString(id) {
			d.Provider, d.ProviderID = t, id
			break
		}
	}
	finishDoc(d, name, false, fail)
	return d, ps
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func cutAfterRoot(b []byte) []byte {
	for root := range nfoRoots {
		closing := []byte("</" + root + ">")
		if i := bytes.LastIndex(b, closing); i >= 0 {
			return b[:i+len(closing)]
		}
	}
	return b
}

// ReadSidecar reads a file that sits beside media, where a link may be an ordinary part
// of someone's folders: size limited, and only an ordinary file in the end.
func ReadSidecar(p string, max int64) ([]byte, error) {
	st, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a file")
	}
	if st.Size() > max {
		return nil, fmt.Errorf("it's too big: the limit is %s", sizeWords(max))
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("it's too big: the limit is %s", sizeWords(max))
	}
	return b, nil
}

// JSONErrorLine says where a problem from encoding/json is, and in words.
func JSONErrorLine(data []byte, err error) (int, string) {
	var syn *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syn):
		return lineAt(data, syn.Offset), plainJSONError(syn.Error())
	case errors.As(err, &typ):
		return lineAt(data, typ.Offset), "\"" + Line(typ.Field, 60) + "\" has the wrong kind of value here"
	}
	return 0, plainJSONError(err.Error())
}
