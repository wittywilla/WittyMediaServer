package logging

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Health is how the log itself is doing, for the settings page: the one place a
// problem with logging can be told when the log can't be written.
type Health struct {
	Writing   bool      `json:"writing"`             // lines are reaching the disk
	Error     string    `json:"error,omitempty"`     // why they aren't
	Since     time.Time `json:"since,omitempty"`     // when that started
	Lost      int64     `json:"lost,omitempty"`      // lines that could not be written
	Dropped   int64     `json:"dropped,omitempty"`   // lines dropped because the disk could not keep up
	Quieted   int64     `json:"quieted,omitempty"`   // lines left out while the disk is nearly full
	LowDisk   bool      `json:"lowDisk,omitempty"`   // only warnings and errors are being written
	TotalSize int64     `json:"totalSize,omitempty"` // of all the log files
	MaxSize   int64     `json:"maxSize"`
	KeepDays  int       `json:"keepDays"`
}

type health struct {
	mu sync.Mutex
	h  Health
}

func (s *health) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.h.Error == "" {
		s.h.Since = time.Now()
	}
	s.h.Error = clean(Redact(err.Error()), maxMsg)
	s.h.Writing = false
}

func (s *health) ok() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.h.Error, s.h.Since, s.h.Writing = "", time.Time{}, true
}

func (s *health) update(f func(*Health)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(&s.h)
}

// Health reports how the log is doing.
func (l *Logger) Health() Health {
	l.h.mu.Lock()
	h := l.h.h
	l.h.mu.Unlock()
	h.Dropped = l.dropped.Load()
	h.MaxSize, h.KeepDays = maxTotalSize, keepDays
	return h
}

// fileWriter is only ever touched by the writer goroutine, so nobody else can be
// held up by the disk.
type fileWriter struct {
	l       *Logger
	dir     string
	day     string
	part    int
	file    *os.File
	buf     *bufio.Writer
	written int64
	line    []byte

	unflushed  int64 // lines in the buffer, which are lost if flushing fails
	lost       int64 // since the last report
	reported   int64 // of the queue's drops, what the log has been told
	quieted    int64
	retryAt    time.Time
	lowDisk    bool
	lastCheck  time.Time
	partialEnd bool // the last write failed part way, so start on a new line
}

func (l *Logger) run(w *fileWriter) {
	defer l.stopped.Done()
	flush := time.NewTicker(flushEvery)
	defer flush.Stop()
	defer w.close()
	for {
		select {
		case rec := <-l.queue:
			w.write(rec)
			w.drain()
		case f := <-l.ctl:
			f(w)
		case <-flush.C:
			w.flush()
			w.maintain(time.Now())
		case <-l.done:
			w.drain()
			w.flush()
			return
		}
	}
}

// drain writes what's queued without going round the loop for each line. Errors
// reach the disk at once, in case the process is about to die.
func (w *fileWriter) drain() {
	urgent := false
	for i := 0; i < queueSize; i++ {
		select {
		case rec := <-w.l.queue:
			urgent = w.write(rec) || urgent
		default:
			if urgent {
				w.flush()
			}
			return
		}
	}
	if urgent {
		w.flush()
	}
}

func (w *fileWriter) write(rec *Record) (urgent bool) {
	if w.lowDisk && rec.Lv < slog.LevelWarn {
		w.quieted++
		w.l.h.update(func(h *Health) { h.Quieted = w.quieted })
		return false
	}
	if !w.ready(rec.Time) {
		w.lost++
		w.l.h.update(func(h *Health) { h.Lost++ })
		return rec.Lv >= slog.LevelError
	}
	w.line = rec.appendLine(w.line[:0])
	if w.partialEnd {
		w.partialEnd = false
		_, _ = w.buf.WriteString("\n")
	}
	if _, err := w.buf.Write(w.line); err != nil {
		w.failed(err)
		w.lost++
		w.l.h.update(func(h *Health) { h.Lost++ })
		return false
	}
	w.written += int64(len(w.line))
	w.unflushed++
	if rec.Lv >= slog.LevelError {
		w.flush()
	}
	return false
}

// ready makes sure a file is open, and the right one.
func (w *fileWriter) ready(now time.Time) bool {
	day := now.Format("2006-01-02")
	if w.file != nil && day == w.day && w.written <= maxFileSize {
		return true
	}
	if w.file == nil && now.Before(w.retryAt) && !w.retryAt.IsZero() {
		return false
	}
	if day != w.day {
		w.part = 0
	} else if w.file != nil {
		w.part++
	}
	return w.open(day, now)
}

func (w *fileWriter) open(day string, now time.Time) bool {
	w.close()
	w.day = day
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		w.failedOpen(err, now)
		return false
	}
	for {
		name := "wms-" + day + ".log"
		if w.part > 0 {
			name = fmt.Sprintf("wms-%s-%d.log", day, w.part)
		}
		p := filepath.Join(w.dir, name)
		f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			w.failedOpen(err, now)
			return false
		}
		st, err := f.Stat()
		if err == nil && st.Size() > maxFileSize {
			f.Close()
			w.part++ // already full: a restart in the middle of the day
			continue
		}
		w.file, w.buf = f, bufio.NewWriterSize(f, 64<<10)
		w.written, w.unflushed = 0, 0
		if st != nil {
			w.written = st.Size()
		}
		break
	}
	w.retryAt = time.Time{}
	w.l.h.ok()
	w.prune(0)
	w.reportLoss()
	return true
}

func (w *fileWriter) failedOpen(err error, now time.Time) {
	w.l.h.setErr(err)
	w.retryAt = now.Add(retryOpen)
}

// failed: the disk stopped taking what we write. Give up the file for now and
// try again in a while; meanwhile the viewer and console still work. If the
// disk is full, make room by taking old logs away.
func (w *fileWriter) failed(err error) {
	w.lost += w.unflushed
	if w.unflushed > 0 {
		n := w.unflushed
		w.l.h.update(func(h *Health) { h.Lost += n })
	}
	w.unflushed = 0
	w.l.h.setErr(err)
	if w.buf != nil {
		w.buf.Reset(discard{}) // bufio keeps its error for good
	}
	if w.file != nil {
		_ = w.file.Close()
	}
	w.file, w.buf = nil, nil
	w.partialEnd = true
	w.retryAt = time.Now().Add(retryOpen)
	if diskFull(err) {
		w.prune(maxFileSize)
	}
}

// diskFull: only then are old logs worth giving up. A drive that went away for a
// moment is no reason to lose them.
func diskFull(err error) bool {
	if errors.Is(err, syscall.ENOSPC) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no space left") || strings.Contains(s, "not enough space") || strings.Contains(s, "disk is full") || strings.Contains(s, "quota")
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func (w *fileWriter) flush() {
	if w.buf == nil {
		return
	}
	if err := w.buf.Flush(); err != nil {
		w.failed(err)
		return
	}
	w.unflushed = 0
}

func (w *fileWriter) close() {
	if w.buf != nil {
		if err := w.buf.Flush(); err != nil {
			w.lost += w.unflushed
		}
		w.buf = nil
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	w.unflushed = 0
}

func (w *fileWriter) setDir(abs string) {
	w.flush()
	w.close()
	w.dir = abs
	w.day, w.part = "", 0
	w.retryAt = time.Time{}
}

// maintain runs with the regular flush: tries a failed folder again, tells the
// log what was lost, and watches the disk.
func (w *fileWriter) maintain(now time.Time) {
	if w.file == nil && !now.Before(w.retryAt) {
		w.ready(now)
	}
	if drops := w.l.dropped.Load(); drops > w.reported {
		w.lost += drops - w.reported
		w.reported = drops
	}
	if w.file != nil {
		w.reportLoss()
	}
	if now.Sub(w.lastCheck) < checkEvery {
		return
	}
	w.lastCheck = now
	w.prune(0)
	if w.l.freeFn != nil {
		free := w.l.freeFn(w.dir)
		low := free >= 0 && free < lowDiskBytes
		if low && !w.lowDisk {
			w.l.emit(slog.LevelError, "the disk holding the logs is nearly full: only warnings and errors are being written", Attr{"free", fmt.Sprintf("%d MB", free>>20)}, Attr{"dir", w.dir})
			w.prune(maxFileSize)
		} else if !low && w.lowDisk {
			w.l.emit(slog.LevelInfo, "the disk holding the logs has room again")
		}
		w.lowDisk = low
		w.l.h.update(func(h *Health) { h.LowDisk = low })
	}
}

func (w *fileWriter) reportLoss() {
	if w.lost == 0 {
		return
	}
	n := w.lost
	w.lost = 0
	w.l.emit(slog.LevelWarn, "some log messages were not written", Attr{"count", fmt.Sprint(n)})
}

// emit is the log talking about itself. It may be on the writer goroutine, and
// the queue may be full: publish doesn't wait.
func (l *Logger) emit(lv slog.Level, msg string, attrs ...Attr) {
	l.publish(&Record{Time: time.Now(), Lv: lv, Cat: CatDefault, Msg: msg, Attrs: attrs})
}

// prune deletes old log files: past the age limit, and the oldest past the size
// limit. A limit other than 0 is an emergency: the disk is full.
func (w *fileWriter) prune(limit int64) {
	if limit == 0 {
		limit = maxTotalSize
	}
	files := listLogFiles(w.dir)
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	var total int64
	for _, f := range files {
		total += f.Size
	}
	w.l.h.update(func(h *Health) { h.TotalSize = total })
	current := ""
	if w.file != nil {
		current = filepath.Base(w.file.Name())
	}
	for i := len(files) - 1; i >= 0; i-- { // oldest first
		f := files[i]
		if f.Name == current {
			continue
		}
		if total > limit || f.Modified.Before(cutoff) {
			if os.Remove(filepath.Join(w.dir, f.Name)) == nil {
				total -= f.Size
			}
		}
	}
	w.l.h.update(func(h *Health) { h.TotalSize = total })
}
