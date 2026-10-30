// Package logging writes the server's log: to the console and to daily log
// files in a folder the owner chooses. Detailed logging is split into
// categories that are off until the owner turns them on, and secrets
// (tokens, passwords, API keys) are always removed.
//
// Log files stay on the server's disk. Nothing is ever uploaded; they are
// only seen by someone the owner sends them to.
package logging

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Category is one kind of detailed logging the owner can opt into.
type Category struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Categories, in the order the settings page shows them. Messages tagged
// with a category (slog attribute "cat") are written only when it is on;
// warnings and errors are always written.
var Categories = []Category{
	{"playback", "Playback", "What is played and how: direct play or transcoding, codecs, resolution, the chosen audio and subtitle tracks. Includes titles of what you watch."},
	{"dvd", "Discs (WittyDVD)", "Every step the disc navigator takes: menus, buttons pressed, program chains and cells. Useful when a disc misbehaves."},
	{"ffmpeg", "FFmpeg commands", "The full command line of every transcode, including file paths."},
	{"library", "Library scans", "Each file found while scanning and how it was matched. Includes file and folder names."},
	{"http", "Web requests", "Each request to the server: address, page, result and time taken. Sign-in tokens are removed."},
	{"client", "App errors", "Errors that happen in the web app and the apps on your devices, sent to this server."},
}

// Logger owns the log files.
type Logger struct {
	level   slog.LevelVar
	on      atomic.Value // map[string]bool
	console io.Writer

	mu      sync.Mutex
	dir     string
	day     string
	part    int
	file    *os.File
	buf     *bufio.Writer
	written int64
	stop    chan struct{}
}

const (
	maxFileSize = 50 << 20 // start a new file after 50 MB
	keepDays    = 14
)

// New starts logging to dir (created if needed) and to console (nil = none).
func New(dir string, level slog.Level, console io.Writer) (*Logger, error) {
	l := &Logger{console: console, stop: make(chan struct{})}
	l.level.Set(level)
	l.on.Store(map[string]bool{})
	if err := l.SetDir(dir); err != nil {
		return nil, err
	}
	go l.flusher()
	return l, nil
}

// Handler returns the slog handler for the server.
func (l *Logger) Handler() slog.Handler {
	return &handler{l: l, inner: slog.NewTextHandler(l, &slog.HandlerOptions{Level: slog.LevelDebug, ReplaceAttr: redactAttr})}
}

// SetLevel changes the level for messages without a category.
func (l *Logger) SetLevel(lv slog.Level) { l.level.Set(lv) }

// Level is the current level.
func (l *Logger) Level() slog.Level { return l.level.Level() }

// SetCategories turns on exactly the given categories.
func (l *Logger) SetCategories(ids []string) {
	m := map[string]bool{}
	for _, id := range ids {
		for _, c := range Categories {
			if c.ID == id {
				m[id] = true
			}
		}
	}
	l.on.Store(m)
}

// On reports whether a category is turned on.
func (l *Logger) On(id string) bool { return l.on.Load().(map[string]bool)[id] }

// Enabled lists the categories that are on.
func (l *Logger) Enabled() []string {
	var out []string
	for _, c := range Categories {
		if l.On(c.ID) {
			out = append(out, c.ID)
		}
	}
	return out
}

func (l *Logger) anyOn() bool { return len(l.on.Load().(map[string]bool)) > 0 }

// Dir is the folder the log files are written to.
func (l *Logger) Dir() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.dir
}

// SetDir moves logging to another folder (new messages only).
func (l *Logger) SetDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("no log folder")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	// Make sure the folder is writable before switching to it.
	probe := filepath.Join(abs, ".write-test")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		return fmt.Errorf("cannot write to %s: %w", abs, err)
	}
	_ = os.Remove(probe)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closeFile()
	l.dir = abs
	l.day = ""
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
	entries, _ := os.ReadDir(l.Dir())
	var out []FileInfo
	for _, e := range entries {
		if e.IsDir() || !IsLogName(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil {
			out = append(out, FileInfo{Name: e.Name(), Size: info.Size(), Modified: info.ModTime()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	return out
}

// IsLogName accepts only our own file names (never a path).
func IsLogName(name string) bool {
	return strings.HasPrefix(name, "wms-") && strings.HasSuffix(name, ".log") && !strings.ContainsAny(name, `/\`) && !strings.Contains(name, "..")
}

// Write implements io.Writer for the text handler: one call per record.
func (l *Logger) Write(p []byte) (int, error) {
	if l.console != nil {
		_, _ = l.console.Write(p)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	day := now.Format("2006-01-02")
	if l.file == nil || day != l.day || l.written > maxFileSize {
		if day != l.day {
			l.part = 0
		} else {
			l.part++
		}
		l.openFile(day)
	}
	if l.buf == nil {
		return len(p), nil // could not open a file: console only
	}
	n, err := l.buf.Write(p)
	l.written += int64(n)
	return n, err
}

func (l *Logger) openFile(day string) {
	l.closeFile()
	l.day = day
	name := "wms-" + day + ".log"
	if l.part > 0 {
		name = fmt.Sprintf("wms-%s-%d.log", day, l.part)
	}
	f, err := os.OpenFile(filepath.Join(l.dir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	st, _ := f.Stat()
	l.file, l.buf = f, bufio.NewWriterSize(f, 64<<10)
	l.written = 0
	if st != nil {
		l.written = st.Size()
	}
	go l.prune()
}

func (l *Logger) closeFile() {
	if l.buf != nil {
		_ = l.buf.Flush()
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	l.file, l.buf = nil, nil
}

// prune deletes log files older than keepDays.
func (l *Logger) prune() {
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for _, f := range l.Files() {
		if f.Modified.Before(cutoff) {
			_ = os.Remove(filepath.Join(l.Dir(), f.Name))
		}
	}
}

// Flush writes buffered messages to disk.
func (l *Logger) Flush() {
	l.mu.Lock()
	if l.buf != nil {
		_ = l.buf.Flush()
	}
	l.mu.Unlock()
}

// The buffer is written every two seconds, not on every message, so logging
// never slows playback down on slow disks or SD cards.
func (l *Logger) flusher() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			l.Flush()
		case <-l.stop:
			return
		}
	}
}

// Close flushes and closes the current file.
func (l *Logger) Close() {
	select {
	case <-l.stop:
	default:
		close(l.stop)
	}
	l.mu.Lock()
	l.closeFile()
	l.mu.Unlock()
}

// ---------------------------------------------------------------- handler

type handler struct {
	l     *Logger
	inner slog.Handler
	cat   string
}

func (h *handler) Enabled(_ context.Context, lv slog.Level) bool {
	// Debug messages of a category the owner turned on must get through.
	return lv >= h.l.level.Level() || (h.l.anyOn() && (h.cat == "" || h.l.On(h.cat)))
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	cat := h.cat
	if cat == "" {
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == "cat" {
				cat = a.Value.String()
				return false
			}
			return true
		})
	}
	if r.Level < slog.LevelWarn {
		if cat != "" {
			if !h.l.On(cat) {
				return nil
			}
		} else if r.Level < h.l.level.Level() {
			return nil
		}
	}
	err := h.inner.Handle(ctx, r)
	if r.Level >= slog.LevelError {
		h.l.Flush() // errors reach the disk at once, in case the process dies
	}
	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cat := h.cat
	for _, a := range attrs {
		if a.Key == "cat" {
			cat = a.Value.String()
		}
	}
	return &handler{l: h.l, inner: h.inner.WithAttrs(attrs), cat: cat}
}

func (h *handler) WithGroup(name string) slog.Handler {
	return &handler{l: h.l, inner: h.inner.WithGroup(name), cat: h.cat}
}

// ---------------------------------------------------------------- redaction

var secretKeys = []string{"token", "password", "passwd", "secret", "apikey", "api_key", "authorization", "cookie", "pin"}

var secretInText = regexp.MustCompile(`(?i)((?:token|api_key|apikey|password|pin|key)=)[^&\s"']+|(Bearer\s+)[A-Za-z0-9._~+/=-]+`)

// Redact removes secrets from free text (URLs, command lines, headers).
func Redact(s string) string {
	if !strings.ContainsAny(s, "=B") {
		return s
	}
	return secretInText.ReplaceAllString(s, "${1}${2}[hidden]")
}

func redactAttr(_ []string, a slog.Attr) slog.Attr {
	k := strings.ToLower(a.Key)
	for _, s := range secretKeys {
		if k == s || strings.HasSuffix(k, s) {
			return slog.String(a.Key, "[hidden]")
		}
	}
	// Errors too: an HTTP client's error quotes the whole address it asked.
	if a.Value.Kind() == slog.KindAny {
		if err, ok := a.Value.Any().(error); ok {
			a = slog.String(a.Key, err.Error())
		}
	}
	if a.Value.Kind() == slog.KindString {
		if v := a.Value.String(); strings.Contains(v, "=") || strings.Contains(v, "Bearer") {
			return slog.String(a.Key, Redact(v))
		}
	}
	return a
}
