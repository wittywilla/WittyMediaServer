// Package logging is the server's log: structured records in categories with
// their own levels, kept in memory for the viewer and written to daily files in
// a folder the owner chooses. See docs/LOGGING.md for how every part should use
// it.
//
// Logging never takes the server down or slows it. A record is built on the
// caller's goroutine, put in a memory ring and a bounded queue, and written by
// one goroutine: when the disk is slow, full or gone the queue drops with a
// count instead of making anyone wait. Secrets are removed before a record is
// stored anywhere.
//
// Log files stay on the server's disk. Nothing is ever uploaded; they are only
// seen by someone the owner sends them to.
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Category is one area of the server, with its own level.
type Category struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Default     string `json:"default"` // the level it starts at
}

// Categories, in the order the settings page shows them. Warnings and errors
// of every category are written unless the owner lowers it; the ones that name
// what you watch or who signs in say so, and stay quiet until asked.
var Categories = []Category{
	{"playback", "Playback", "What is played and how: direct play or transcoding, codecs, resolution, the chosen audio and subtitle tracks. Includes titles of what you watch.", "warn"},
	{"transcode", "Transcoding", "FFmpeg: what it was asked to do, including file paths, and what it said when it stopped.", "info"},
	{"discs", "Discs", "Every step the DVD and Blu-ray navigators take: menus, buttons pressed, program chains and cells. Useful when a disc misbehaves.", "warn"},
	{"games", "Games", "Game sessions: the runner, emulator cores, controllers and the stream to the browser. Includes game titles.", "info"},
	{"tv", "Live TV", "Tuners, channels, the guide and recordings.", "info"},
	{"library", "Library scans", "Scans, metadata lookups and how files were matched. At the most detailed level, each file and folder name.", "info"},
	{"modules", "Modules", "Module helpers starting and stopping, checks for module updates and installing them.", "info"},
	{"update", "Server updates", "Checks for a new version, downloading and installing it.", "info"},
	{"remote", "Remote access", "HTTPS certificates, your router's port mapping, finding the server on the network.", "info"},
	{"auth", "Sign-ins", "Each sign-in and failed sign-in: account name, address and whether it came from home or away. Lock-outs are always recorded.", "warn"},
	{"clients", "App errors", "Errors in the web app and the apps on your devices, sent to this server: crashes, failed requests, and player and game stream errors.", "warn"},
	{"http", "Web requests", "Each request to the server: address, page, result and time taken. Sign-in tokens are removed. Failed requests are always recorded.", "warn"},
	{"storage", "Storage", "WittyDiagnoser's storage checks and problems reading or writing folders.", "info"},
	{"server", "Everything else", "Starting and stopping, the database, settings and anything without an area of its own.", "info"},
}

// CatDefault is the category of a message that doesn't name one.
const CatDefault = "server"

// Records from older code, and old settings, may use these names.
var catAliases = map[string]string{"dvd": "discs", "bluray": "discs", "ffmpeg": "transcode", "signin": "auth", "client": "clients", "game": "games"}

// NormalizeCat turns any name into a known category.
func NormalizeCat(id string) string {
	if a, ok := catAliases[id]; ok {
		return a
	}
	for i := range Categories {
		if Categories[i].ID == id {
			return id
		}
	}
	return CatDefault
}

// LevelCrash marks a recovered panic: written whatever the levels say.
const LevelCrash = slog.Level(12)

// LevelOff silences a category, except for crashes.
const LevelOff = slog.Level(100)

// ParseLevel reads a level's name.
func ParseLevel(v string) (slog.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug, true
	case "info", "":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	case "off":
		return LevelOff, true
	}
	return 0, false
}

// LevelName is ParseLevel backwards.
func LevelName(lv slog.Level) string {
	switch {
	case lv >= LevelOff:
		return "off"
	case lv >= LevelCrash:
		return "crash"
	case lv >= slog.LevelError:
		return "error"
	case lv >= slog.LevelWarn:
		return "warn"
	case lv >= slog.LevelInfo:
		return "info"
	}
	return "debug"
}

// levels is a whole table of categories, swapped in at once.
type levels struct {
	by   map[string]slog.Level
	mods map[string]modLevel
	min  slog.Level
}

// modLevel is how one module is logged, ready to be checked for every record.
type modLevel struct {
	level    slog.Level
	hasLevel bool
	topics   map[string]bool
}

func newLevels(by map[string]slog.Level, mods map[string]modLevel) *levels {
	t := &levels{by: by, mods: mods, min: LevelOff}
	for _, lv := range by {
		if lv < t.min {
			t.min = lv
		}
	}
	for _, m := range mods {
		if m.hasLevel && m.level < t.min {
			t.min = m.level
		}
		if len(m.topics) > 0 {
			t.min = slog.LevelDebug // a topic line is usually Debug: it has to get as far as Handle
		}
	}
	return t
}

func defaultLevels() map[string]slog.Level {
	by := make(map[string]slog.Level, len(Categories))
	for i := range Categories {
		lv, _ := ParseLevel(Categories[i].Default)
		by[Categories[i].ID] = lv
	}
	return by
}

// Limits. Variables so tests can shrink them.
var (
	maxFileSize  int64 = 20 << 20  // start a new file after this
	maxTotalSize int64 = 200 << 20 // the newest files that fit, whatever their age
	keepDays           = 14
	lowDiskBytes int64 = 100 << 20 // below this much free, only warnings and errors
	retryOpen          = 30 * time.Second
	flushEvery         = 2 * time.Second
	checkEvery         = time.Minute
)

const (
	queueSize        = 4096
	consoleQueueSize = 1024
	ringEntries      = 4096
	ringBytes        = 6 << 20
)

// Logger owns the log files, the memory ring and the writer goroutine.
type Logger struct {
	table atomic.Pointer[levels]
	floor atomic.Int64 // from -log debug, or while recording: no category is quieter than this
	rec   struct {
		sync.Mutex
		base  slog.Level // the floor when not recording
		until time.Time
		timer *time.Timer
	}
	dir atomic.Pointer[string]

	ring  ring
	flood flood

	queue   chan *Record
	console chan *Record
	ctl     chan func(*fileWriter)
	done    chan struct{}
	stopped sync.WaitGroup
	closing atomic.Bool

	dropped atomic.Int64 // queue full: never written
	h       health
	freeFn  func(string) int64 // free bytes where the logs are; -1 unknown
}

// FreeSpace, set by whoever can ask the system (see package sysinfo): lets a
// nearly full disk make logging quiet instead of making things worse.
var FreeSpace func(dir string) int64

var active atomic.Pointer[Logger]

// New starts logging to dir (created if needed) and to console (nil = none). A
// folder that can't be used is not fatal: the server still runs with its log in
// memory and on the console, says so, and keeps trying the folder.
func New(dir string, level slog.Level, console io.Writer) (*Logger, error) {
	if dir == "" {
		return nil, fmt.Errorf("no log folder")
	}
	l := &Logger{queue: make(chan *Record, queueSize), ctl: make(chan func(*fileWriter), 8), done: make(chan struct{}), freeFn: FreeSpace}
	l.floor.Store(int64(LevelOff))
	l.rec.base = LevelOff
	by := defaultLevels()
	by[CatDefault] = level
	l.table.Store(newLevels(by, nil))
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	l.dir.Store(&abs)
	fw := &fileWriter{l: l, dir: abs}
	if err := checkDir(abs); err != nil {
		l.h.setErr(err)
	}
	l.stopped.Add(1)
	go l.run(fw)
	if console != nil {
		l.console = make(chan *Record, consoleQueueSize)
		l.stopped.Add(1)
		go l.runConsole(console)
	}
	active.Store(l)
	return l, nil
}

// Handler returns the slog handler for the server.
func (l *Logger) Handler() slog.Handler { return &handler{l: l} }

// StdLogger is for parts of the standard library that want a *log.Logger, such
// as http.Server's ErrorLog, whose panics and TLS errors would otherwise reach
// only the console.
func (l *Logger) StdLogger(cat string, lv slog.Level) *log.Logger {
	return slog.NewLogLogger(slog.New(l.Handler()).With("cat", cat).Handler(), lv)
}

// SetLevel sets the level of messages without an area of their own.
func (l *Logger) SetLevel(lv slog.Level) { l.SetCategoryLevel(CatDefault, lv) }

// Level is the level of messages without an area of their own.
func (l *Logger) Level() slog.Level { return l.table.Load().by[CatDefault] }

// SetFloor makes every category at least this detailed, whatever its own level
// says (the -log debug flag). slog.Level above LevelOff, or LevelOff, removes it.
func (l *Logger) SetFloor(lv slog.Level) {
	l.rec.Lock()
	defer l.rec.Unlock()
	l.rec.base = lv
	if l.rec.timer == nil {
		l.floor.Store(int64(lv))
	}
}

// RecordEverything makes every area and module as detailed as it goes, for a
// while, to catch a problem while it happens. It ends by itself and changes none
// of the owner's settings, so nobody is left with detailed logging by mistake. A
// duration of 0 stops it. It says when it will end.
func (l *Logger) RecordEverything(d time.Duration) time.Time {
	l.rec.Lock()
	defer l.rec.Unlock()
	if l.rec.timer != nil {
		l.rec.timer.Stop()
		l.rec.timer = nil
	}
	if d <= 0 {
		l.rec.until = time.Time{}
		l.floor.Store(int64(l.rec.base))
		return time.Time{}
	}
	l.rec.until = time.Now().Add(d)
	l.floor.Store(int64(slog.LevelDebug))
	l.rec.timer = time.AfterFunc(d, func() {
		l.RecordEverything(0)
		l.emit(slog.LevelInfo, "recording everything has finished: logging is back to its usual settings")
	})
	return l.rec.until
}

// RecordingUntil says when RecordEverything will end, if it's on.
func (l *Logger) RecordingUntil() time.Time {
	l.rec.Lock()
	defer l.rec.Unlock()
	return l.rec.until
}

// SetCategoryLevel changes one category's level, live.
func (l *Logger) SetCategoryLevel(cat string, lv slog.Level) {
	cat = NormalizeCat(cat)
	old := l.table.Load().by
	by := make(map[string]slog.Level, len(old))
	for k, v := range old {
		by[k] = v
	}
	by[cat] = lv
	l.table.Store(newLevels(by, l.table.Load().mods))
}

// SetLevels changes several at once; unknown names are ignored.
func (l *Logger) SetLevels(want map[string]slog.Level) {
	old := l.table.Load().by
	by := make(map[string]slog.Level, len(old))
	for k, v := range old {
		by[k] = v
	}
	for k, v := range want {
		if a, ok := catAliases[k]; ok {
			k = a
		}
		if _, ok := by[k]; ok {
			by[k] = v
		}
	}
	l.table.Store(newLevels(by, l.table.Load().mods))
}

// Levels is the current level of each category, by name.
func (l *Logger) Levels() map[string]string {
	t := l.table.Load()
	out := make(map[string]string, len(t.by))
	for k, v := range t.by {
		out[k] = LevelName(v)
	}
	return out
}

func (l *Logger) catLevel(cat string) slog.Level {
	lv := l.table.Load().by[cat]
	if f := slog.Level(l.floor.Load()); f < lv {
		return f
	}
	return lv
}

// SetCategories turns on exactly these categories at their most detailed; the
// rest go back to where they start. The older way to set levels, for old
// settings and clients.
func (l *Logger) SetCategories(ids []string) {
	by := defaultLevels()
	by[CatDefault] = l.Level()
	for _, id := range ids {
		if a, ok := catAliases[id]; ok {
			id = a
		}
		if _, ok := by[id]; ok {
			by[id] = slog.LevelDebug
		}
	}
	l.table.Store(newLevels(by, l.table.Load().mods))
}

// ModuleLog is how one module is logged, on top of its area's level: a level of
// its own ("" follows the area) and the advanced options, or topics, turned on.
type ModuleLog struct {
	Level  string          `json:"level,omitempty"`
	Topics map[string]bool `json:"topics,omitempty"`
}

// SetModules replaces how modules are logged, live. Any line that carries
// "module", id is then judged by its module's settings. Modules that aren't
// mentioned follow their area, and cost nothing extra.
func (l *Logger) SetModules(want map[string]ModuleLog) {
	mods := make(map[string]modLevel, len(want))
	for id, m := range want {
		var ml modLevel
		if m.Level != "" {
			if lv, ok := ParseLevel(m.Level); ok {
				ml.level, ml.hasLevel = lv, true
			}
		}
		for topic, on := range m.Topics {
			if on {
				if ml.topics == nil {
					ml.topics = map[string]bool{}
				}
				ml.topics[topic] = true
			}
		}
		if ml.hasLevel || ml.topics != nil {
			mods[strings.ToLower(id)] = ml
		}
	}
	if len(mods) == 0 {
		mods = nil
	}
	l.table.Store(newLevels(l.table.Load().by, mods))
}

// ModuleEnv is the same settings for a helper process, which can't see this
// logger: what it should print, if it cares to ask. See docs/MODULES.md.
func (l *Logger) ModuleEnv(id string) []string {
	m, ok := l.table.Load().mods[strings.ToLower(id)]
	if !ok {
		return nil
	}
	var env []string
	if m.hasLevel {
		env = append(env, "WITTYMODULE_LOG="+LevelName(m.level))
	}
	if len(m.topics) > 0 {
		topics := make([]string, 0, len(m.topics))
		for t := range m.topics {
			topics = append(topics, t)
		}
		sort.Strings(topics)
		env = append(env, "WITTYMODULE_LOG_TOPICS="+strings.Join(topics, ","))
	}
	return env
}

// On reports whether a category logs everyday detail (Info or lower).
func (l *Logger) On(id string) bool { return l.catLevel(NormalizeCat(id)) <= slog.LevelInfo }

// Accepts reports whether a category keeps messages of this level at all.
func (l *Logger) Accepts(id string, lv slog.Level) bool { return lv >= l.catLevel(NormalizeCat(id)) }

// Enabled lists the categories set to their most detailed level.
func (l *Logger) Enabled() []string {
	var out []string
	t := l.table.Load()
	for i := range Categories {
		if t.by[Categories[i].ID] <= slog.LevelDebug {
			out = append(out, Categories[i].ID)
		}
	}
	return out
}

// Dir is the folder the log files are written to.
func (l *Logger) Dir() string { return *l.dir.Load() }

// SetDir moves logging to another folder (new messages only).
func (l *Logger) SetDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("no log folder")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := checkDir(abs); err != nil {
		return err
	}
	l.dir.Store(&abs)
	l.do(func(w *fileWriter) { w.setDir(abs) })
	return nil
}

func checkDir(abs string) error {
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	// Make sure the folder is writable before switching to it.
	probe, err := os.CreateTemp(abs, ".write-test-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w", abs, err)
	}
	probe.Close()
	_ = os.Remove(probe.Name())
	return nil
}

// FileInfo describes one log file.
type FileInfo struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// Files lists the log files, newest first.
func (l *Logger) Files() []FileInfo {
	l.Flush()
	return listLogFiles(l.Dir())
}

func listLogFiles(dir string) []FileInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []FileInfo
	for _, e := range entries {
		if e.IsDir() || !IsLogName(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, FileInfo{Name: e.Name(), Size: info.Size(), Modified: info.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Modified.Equal(out[j].Modified) {
			return out[i].Name > out[j].Name
		}
		return out[i].Modified.After(out[j].Modified)
	})
	return out
}

// IsLogName accepts only our own file names (never a path).
func IsLogName(name string) bool {
	return strings.HasPrefix(name, "wms-") && strings.HasSuffix(name, ".log") && !strings.ContainsAny(name, `/\`) && !strings.Contains(name, "..")
}

// do runs f on the writer goroutine and waits, for a while.
func (l *Logger) do(f func(*fileWriter)) bool {
	if l.closing.Load() {
		return false
	}
	ack := make(chan struct{})
	t := time.NewTimer(2 * time.Second)
	defer t.Stop()
	select {
	case l.ctl <- func(w *fileWriter) { f(w); close(ack) }:
	case <-t.C:
		return false
	case <-l.done:
		return false
	}
	select {
	case <-ack:
		return true
	case <-t.C:
		return false
	case <-l.done:
		return false
	}
}

// Flush writes what is queued to disk, and waits for it, for up to two seconds:
// a disk that has stopped answering must not stop the caller for longer.
func (l *Logger) Flush() { l.do(func(w *fileWriter) { w.drain(); w.flush() }) }

// Close writes what is left and stops.
func (l *Logger) Close() {
	if !l.closing.CompareAndSwap(false, true) {
		return
	}
	close(l.done)
	finished := make(chan struct{})
	go func() { l.stopped.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(3 * time.Second): // a hung disk must not hold up the shutdown
	}
	active.CompareAndSwap(l, nil)
}

// ---------------------------------------------------------------- correlation

type corrKey struct{}

type correlation struct{ session, req string }

// WithSession ties what is logged with this context to one playback, game,
// scan or job.
func WithSession(ctx context.Context, id string) context.Context {
	c, _ := ctx.Value(corrKey{}).(correlation)
	c.session = id
	return context.WithValue(ctx, corrKey{}, c)
}

// WithRequest ties what is logged with this context to one web request.
func WithRequest(ctx context.Context, id string) context.Context {
	c, _ := ctx.Value(corrKey{}).(correlation)
	c.req = id
	return context.WithValue(ctx, corrKey{}, c)
}

// SessionFrom is the session in a context, if any.
func SessionFrom(ctx context.Context) string {
	c, _ := ctx.Value(corrKey{}).(correlation)
	return c.session
}

// RequestFrom is the web request in a context, if any.
func RequestFrom(ctx context.Context) string {
	c, _ := ctx.Value(corrKey{}).(correlation)
	return c.req
}

var idCounter atomic.Uint64

var idPrefix = func() string {
	// Different on every start, so IDs from two runs in one log file don't meet.
	return fmt.Sprintf("%04x", uint16(time.Now().UnixNano()>>10))
}()

// NewID makes a short ID with a prefix saying what it is for: "scan", "rec",
// "diag". Short enough to read, and cheap enough for every web request.
func NewID(prefix string) string {
	return prefix + "-" + idPrefix + fmt.Sprintf("%05x", idCounter.Add(1))
}

// ValidSession cleans an ID that came from outside (a client's report): IDs are
// only ever letters, digits and a little punctuation, and not very long.
func ValidSession(id string) string {
	if len(id) > 64 {
		id = id[:64]
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == ':') {
			return ""
		}
	}
	return id
}

// ---------------------------------------------------------------- handler

type handler struct {
	l            *Logger
	attrs        []Attr // cleaned once, when With is called
	cat, session string
	module       string
	topic        string
	group        string
}

func (h *handler) Enabled(_ context.Context, lv slog.Level) bool {
	if lv >= LevelCrash {
		return true
	}
	if f := slog.Level(h.l.floor.Load()); lv >= f {
		return true
	}
	t := h.l.table.Load()
	if h.cat != "" {
		if t.mods == nil {
			return lv >= t.by[h.cat]
		}
		if h.module != "" { // a logger that knows its module can answer without building anything
			m, ok := t.mods[h.module]
			switch {
			case !ok:
				return lv >= t.by[h.cat]
			case len(m.topics) > 0:
				return true // the line may have an option that's on
			case m.hasLevel:
				return lv >= m.level
			}
			return lv >= t.by[h.cat]
		}
	}
	return lv >= t.min // the category or module may be among the message's attributes
}

func (h *handler) Handle(ctx context.Context, r slog.Record) (err error) {
	// A record's value can run code (Stringer, LogValuer, Error). Nothing about
	// a log message is worth a crash.
	defer func() {
		if p := recover(); p != nil {
			h.l.dropped.Add(1)
		}
	}()
	rec := &Record{Time: r.Time, Lv: r.Level, Msg: clean(Redact(r.Message), maxMsg), Cat: h.cat, Session: h.session, Module: h.module, Topic: h.topic}
	if rec.Time.IsZero() {
		rec.Time = time.Now()
	}
	rec.Time = rec.Time.Truncate(time.Microsecond) // what the file keeps, so the two can be compared
	if len(h.attrs) > 0 || r.NumAttrs() > 0 {
		rec.Attrs = make([]Attr, len(h.attrs), len(h.attrs)+r.NumAttrs())
		copy(rec.Attrs, h.attrs)
	}
	r.Attrs(func(a slog.Attr) bool {
		rec.add(h.group, a, 0)
		return true
	})
	if ctx != nil {
		if c, ok := ctx.Value(corrKey{}).(correlation); ok {
			rec.Req = c.req
			if rec.Session == "" {
				rec.Session = c.session
			}
		}
	}
	if rec.Cat == "" {
		rec.Cat = CatDefault
	}
	if r.Level < LevelCrash && !h.l.keeps(rec) {
		return nil
	}
	if !h.l.flood.allow(rec) {
		return nil
	}
	h.l.publish(rec)
	return nil
}

// keeps decides by the record's area and, if it has one, its module.
func (l *Logger) keeps(rec *Record) bool {
	threshold := l.catLevel(rec.Cat)
	if rec.Module != "" {
		if m, ok := l.table.Load().mods[rec.Module]; ok {
			if rec.Topic != "" && m.topics[rec.Topic] {
				return true
			}
			if m.hasLevel {
				threshold = m.level
				if f := slog.Level(l.floor.Load()); f < threshold {
					threshold = f
				}
			}
		}
	}
	return rec.Lv >= threshold
}

// add files one attribute: category and session go in the record's own fields,
// the rest are cleaned and kept.
func (rec *Record) add(group string, a slog.Attr, depth int) {
	a.Value = a.Value.Resolve()
	if a.Value.Kind() == slog.KindGroup {
		if depth > 4 {
			return
		}
		prefix := group
		if a.Key != "" {
			if prefix != "" {
				prefix += "."
			}
			prefix += a.Key
		}
		for _, sub := range a.Value.Group() {
			rec.add(prefix, sub, depth+1)
		}
		return
	}
	if a.Equal(slog.Attr{}) {
		return
	}
	if group == "" {
		switch a.Key {
		case "cat":
			rec.Cat = NormalizeCat(a.Value.String())
			return
		case "session":
			rec.Session = ValidSession(a.Value.String())
			return
		case "module":
			rec.Module = strings.ToLower(ValidSession(a.Value.String()))
			return
		case "topic":
			rec.Topic = ValidSession(a.Value.String())
			return
		}
	}
	key := a.Key
	if group != "" {
		key = group + "." + a.Key
	}
	rec.Attrs = append(rec.Attrs, cleanAttr(key, a.Value))
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	n := &handler{l: h.l, cat: h.cat, session: h.session, module: h.module, topic: h.topic, group: h.group}
	rec := Record{Cat: n.cat, Session: n.session, Module: n.module, Topic: n.topic, Attrs: append(make([]Attr, 0, len(h.attrs)+len(attrs)), h.attrs...)}
	for _, a := range attrs {
		rec.add(h.group, a, 0)
	}
	n.attrs, n.cat, n.session, n.module, n.topic = rec.Attrs, rec.Cat, rec.Session, rec.Module, rec.Topic
	return n
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	g := name
	if h.group != "" {
		g = h.group + "." + name
	}
	return &handler{l: h.l, attrs: h.attrs, cat: h.cat, session: h.session, module: h.module, topic: h.topic, group: g}
}

// publish stores a finished record: in the ring for the viewer, in the queues
// for the file and the console. It never waits.
func (l *Logger) publish(rec *Record) {
	l.ring.add(rec)
	select {
	case l.queue <- rec:
	default:
		l.dropped.Add(1)
	}
	if l.console != nil {
		select {
		case l.console <- rec:
		default:
		}
	}
}

// runConsole is on its own: a console nobody reads (a blocked pipe) must not
// hold up the files.
func (l *Logger) runConsole(w io.Writer) {
	defer l.stopped.Done()
	var buf []byte
	for {
		select {
		case rec := <-l.console:
			buf = rec.appendLine(buf[:0])
			_, _ = w.Write(buf)
		case <-l.done:
			for {
				select {
				case rec := <-l.console:
					buf = rec.appendLine(buf[:0])
					_, _ = w.Write(buf)
				default:
					return
				}
			}
		}
	}
}
