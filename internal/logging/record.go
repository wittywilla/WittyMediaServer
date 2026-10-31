package logging

import (
	"bytes"
	"fmt"
	"hash/maphash"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// Attr is one key and value of a record, already cleaned.
type Attr struct {
	K string `json:"k"`
	V string `json:"v"`
}

// Record is one log message.
type Record struct {
	Seq     uint64       `json:"seq"`
	Time    time.Time    `json:"t"`
	Lv      slog.Level   `json:"-"`
	Level   string       `json:"level"`
	Cat     string       `json:"cat"`
	Session string       `json:"session,omitempty"`
	Req     string       `json:"req,omitempty"`
	Module  string       `json:"module,omitempty"` // which module it came from, if any
	Topic   string       `json:"topic,omitempty"`  // an advanced option of that module
	Msg     string       `json:"msg"`
	Attrs   []Attr       `json:"attrs,omitempty"`
	Explain *Explanation `json:"explain,omitempty"`
}

// How much one record can hold. The long ones are what a process said and
// where a crash happened.
const (
	maxMsg      = 2 << 10
	maxAttr     = 4 << 10
	maxLongAttr = 16 << 10
)

func cleanAttr(key string, v slog.Value) Attr {
	key = cleanKey(key)
	if secretKey(key) {
		return Attr{key, hidden}
	}
	limit := maxAttr
	switch key {
	case "stack", "stderr", "output":
		limit = maxLongAttr
	}
	return Attr{key, clean(Redact(valueText(v)), limit)}
}

func valueText(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindAny:
		switch x := v.Any().(type) {
		case error:
			return x.Error()
		case fmt.Stringer:
			return x.String()
		case []string:
			return strings.Join(x, " ")
		case []byte:
			return string(x)
		default:
			return fmt.Sprintf("%+v", x)
		}
	}
	return v.String()
}

func cleanKey(k string) string {
	ok := k != ""
	for i := 0; i < len(k) && ok; i++ {
		c := k[i]
		ok = c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-'
	}
	if ok {
		return k
	}
	b := make([]byte, 0, len(k))
	for i := 0; i < len(k); i++ {
		c := k[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-' {
			b = append(b, c)
		}
	}
	if len(b) == 0 {
		return "key"
	}
	return string(b)
}

// clean makes text from anywhere safe to keep: valid UTF-8, nothing that moves a
// terminal's cursor or reorders what a person reads, and not too long. Newlines
// and tabs stay: a stack needs them, and the file format escapes them.
func clean(s string, limit int) string {
	if len(s) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…[" + strconv.Itoa(len(s)-cut) + " bytes cut]"
	}
	safe := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 && c != '\n' && c != '\t' || c >= 0x7f {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r == utf8.RuneError:
			b.WriteRune(r)
		case dangerousRune(r):
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func dangerousRune(r rune) bool {
	switch {
	case r < 0x20, r >= 0x7f && r < 0xa0:
		return true
	case r >= 0x200b && r <= 0x200f, r >= 0x2028 && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0xfeff:
		return true
	}
	return !unicode.IsPrint(r) && !unicode.IsSpace(r)
}

// ---------------------------------------------------------------- the file format

// One line per record, key=value, the way the standard logfmt text is written.
// Everything that isn't plain is quoted and escaped, so nothing in a value can
// start a new line in the file or pass for a field of its own.

const timeFormat = "2006-01-02T15:04:05.000000Z07:00"

func (rec *Record) appendLine(b []byte) []byte {
	b = append(b, "time="...)
	b = rec.Time.AppendFormat(b, timeFormat)
	b = append(b, " level="...)
	b = append(b, strings.ToUpper(LevelName(rec.Lv))...)
	b = append(b, " cat="...)
	b = append(b, rec.Cat...)
	if rec.Session != "" {
		b = append(b, " session="...)
		b = appendValue(b, rec.Session)
	}
	if rec.Req != "" {
		b = append(b, " req="...)
		b = appendValue(b, rec.Req)
	}
	if rec.Module != "" {
		b = append(b, " module="...)
		b = appendValue(b, rec.Module)
	}
	if rec.Topic != "" {
		b = append(b, " topic="...)
		b = appendValue(b, rec.Topic)
	}
	b = append(b, " msg="...)
	b = appendValue(b, rec.Msg)
	for i := range rec.Attrs {
		b = append(b, ' ')
		b = append(b, rec.Attrs[i].K...)
		b = append(b, '=')
		b = appendValue(b, rec.Attrs[i].V)
	}
	return append(b, '\n')
}

func appendValue(b []byte, v string) []byte {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c <= ' ' || c == '=' || c == '"' || c == '\\' || c >= 0x7f {
			return strconv.AppendQuote(b, v)
		}
	}
	if v == "" {
		return append(b, `""`...)
	}
	return append(b, v...)
}

// ParseLine reads a line of a log file. A line that isn't in the format, from
// an older version say, comes back as a message of its own.
func ParseLine(line string) Record {
	rec := Record{Cat: CatDefault, Lv: slog.LevelInfo, Level: "info"}
	rest := strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(rest, "time=") {
		rec.Msg = clean(rest, maxMsg)
		return rec
	}
	for rest != "" {
		eq := strings.IndexByte(rest, '=')
		if eq <= 0 {
			break
		}
		key := rest[:eq]
		rest = rest[eq+1:]
		var val string
		if strings.HasPrefix(rest, `"`) {
			end := 1
			for end < len(rest) && rest[end] != '"' {
				if rest[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(rest) {
				val, rest = rest[1:], "" // cut off in the middle: a crash, a full disk
			} else {
				q := rest[:end+1]
				rest = rest[end+1:]
				if u, err := strconv.Unquote(q); err == nil {
					val = u
				} else {
					val = q[1 : len(q)-1]
				}
			}
		} else if sp := strings.IndexByte(rest, ' '); sp >= 0 {
			val, rest = rest[:sp], rest[sp:]
		} else {
			val, rest = rest, ""
		}
		rest = strings.TrimLeft(rest, " ")
		switch key {
		case "time":
			if t, err := time.Parse(timeFormat, val); err == nil {
				rec.Time = t
			} else if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
				rec.Time = t
			}
		case "level":
			if lv, ok := parseFileLevel(val); ok {
				rec.Lv = lv
			}
		case "cat":
			rec.Cat = NormalizeCat(val)
		case "session":
			rec.Session = val
		case "req":
			rec.Req = val
		case "module":
			rec.Module = val
		case "topic":
			rec.Topic = val
		case "msg":
			rec.Msg = val
		default:
			rec.Attrs = append(rec.Attrs, Attr{key, val})
		}
	}
	rec.Level = LevelName(rec.Lv)
	return rec
}

func parseFileLevel(v string) (slog.Level, bool) {
	if strings.EqualFold(v, "crash") {
		return LevelCrash, true
	}
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(v)); err == nil {
		return lv, true
	}
	return 0, false
}

// ---------------------------------------------------------------- the ring

// ring keeps the newest records in memory, for the viewer: bounded by count and
// by size, so a flood of long messages can't grow it.
type ring struct {
	mu    sync.Mutex
	buf   [ringEntries]*Record
	start int // the oldest
	n     int
	bytes int
	seq   uint64
}

func (rec *Record) size() int {
	n := 128 + len(rec.Msg) + len(rec.Session) + len(rec.Req)
	for i := range rec.Attrs {
		n += 32 + len(rec.Attrs[i].K) + len(rec.Attrs[i].V)
	}
	return n
}

func (g *ring) add(rec *Record) {
	sz := rec.size()
	g.mu.Lock()
	g.seq++
	rec.Seq = g.seq
	rec.Level = LevelName(rec.Lv)
	if g.n == len(g.buf) {
		g.dropOldest()
	}
	for g.bytes+sz > ringBytes && g.n > 0 {
		g.dropOldest()
	}
	g.buf[(g.start+g.n)%len(g.buf)] = rec
	g.n++
	g.bytes += sz
	g.mu.Unlock()
}

func (g *ring) dropOldest() {
	old := g.buf[g.start]
	g.buf[g.start] = nil
	g.start = (g.start + 1) % len(g.buf)
	g.n--
	g.bytes -= old.size()
}

// snapshot copies the pointers: records never change once stored.
func (g *ring) snapshot() ([]*Record, uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]*Record, g.n)
	for i := 0; i < g.n; i++ {
		out[i] = g.buf[(g.start+i)%len(g.buf)]
	}
	return out, g.seq
}

// ---------------------------------------------------------------- flood control

// flood stops one message repeating without end from pushing everything else
// out of the log. It's a small table: two different messages sharing a slot
// just count together for a moment.
type flood struct {
	mu   sync.Mutex
	seed maphash.Seed
	once sync.Once
	tab  [256]floodSlot
}

type floodSlot struct {
	key   uint64
	start time.Time
	n     int
	over  int
}

var (
	floodWindow = 10 * time.Second
	floodLimit  = 40
)

// allow says whether to keep this record. The first to get through after a
// quiet spell says how many were left out.
func (f *flood) allow(rec *Record) bool {
	f.once.Do(func() { f.seed = maphash.MakeSeed() })
	if rec.Lv >= LevelCrash {
		return true
	}
	var h maphash.Hash
	h.SetSeed(f.seed)
	h.WriteString(rec.Cat)
	h.WriteString(rec.Msg)
	key := h.Sum64()
	now := rec.Time
	f.mu.Lock()
	s := &f.tab[key%uint64(len(f.tab))]
	if s.key != key || now.Sub(s.start) > floodWindow {
		over := s.over
		same := s.key == key
		*s = floodSlot{key: key, start: now, n: 1}
		f.mu.Unlock()
		if same && over > 0 {
			rec.Attrs = append(rec.Attrs, Attr{"repeated", strconv.Itoa(over)})
		}
		return true
	}
	s.n++
	if s.n > floodLimit {
		s.over++
		f.mu.Unlock()
		return false
	}
	f.mu.Unlock()
	return true
}

// ---------------------------------------------------------------- matching

// matches is for the viewer's text box: any field, ignoring case.
func (rec *Record) matches(lowerText string) bool {
	if containsFold(rec.Msg, lowerText) || containsFold(rec.Session, lowerText) || containsFold(rec.Req, lowerText) || containsFold(rec.Module, lowerText) {
		return true
	}
	for i := range rec.Attrs {
		if containsFold(rec.Attrs[i].V, lowerText) || containsFold(rec.Attrs[i].K, lowerText) {
			return true
		}
	}
	return false
}

func containsFold(s, lowerText string) bool {
	if len(lowerText) > len(s) {
		return false
	}
	return bytes.Contains(bytes.ToLower([]byte(s)), []byte(lowerText))
}
