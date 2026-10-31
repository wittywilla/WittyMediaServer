package logging

import (
	"bufio"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Filter says which records a viewer wants. Everything is optional.
type Filter struct {
	MinLevel slog.Level      // 0 = Info, so use slog.LevelDebug for everything
	Cats     map[string]bool // nil = all
	Session  string          // the session or web request they belong to
	Module   string          // only what this module logged
	Text     string          // anywhere in the record, ignoring case
	After    uint64          // only records after this Seq: the live tail
	Before   time.Time       // only records before this: going back in time
	Limit    int             // 0 = 200
	History  bool            // look in the files too when memory runs out
}

// Page is the answer: oldest first.
type Page struct {
	Entries []Record `json:"entries"`
	Last    uint64   `json:"last"`    // pass as After to get what comes next
	More    bool     `json:"more"`    // there is older to ask for, with Before
	Partial bool     `json:"partial"` // the files were only partly searched
	Health  *Health  `json:"health,omitempty"`
}

const (
	maxQueryLimit = 1000
	maxScanBytes  = 128 << 20 // a query looks through this much of the files, at most
)

// ErrBusy: someone else is searching the files. On a small machine one at a time
// is plenty.
var ErrBusy = errors.New("another search of the log files is running")

var fileSearch sync.Mutex

func (f *Filter) wants(rec *Record) bool {
	if rec.Lv < f.MinLevel {
		return false
	}
	if f.Cats != nil && !f.Cats[rec.Cat] {
		return false
	}
	if f.Session != "" && rec.Session != f.Session && rec.Req != f.Session {
		return false
	}
	if f.Module != "" && rec.Module != f.Module {
		return false
	}
	if f.Text != "" && !rec.matches(f.Text) {
		return false
	}
	return true
}

// Activity is how much a module has logged lately.
type Activity struct{ Lines, Warnings, Errors int }

// ModuleActivity counts what each module has in memory, for the settings page.
func (l *Logger) ModuleActivity() map[string]Activity {
	recs, _ := l.ring.snapshot()
	out := map[string]Activity{}
	for _, rec := range recs {
		if rec.Module == "" {
			continue
		}
		a := out[rec.Module]
		a.Lines++
		switch {
		case rec.Lv >= slog.LevelError:
			a.Errors++
		case rec.Lv >= slog.LevelWarn:
			a.Warnings++
		}
		out[rec.Module] = a
	}
	return out
}

// Query looks through the records in memory and, if asked, the files.
func (l *Logger) Query(f Filter) (Page, error) {
	if f.Limit <= 0 {
		f.Limit = 200
	}
	if f.Limit > maxQueryLimit {
		f.Limit = maxQueryLimit
	}
	f.Text = strings.ToLower(f.Text)
	recs, last := l.ring.snapshot()
	page := Page{Last: last, Entries: []Record{}}

	var oldestInRing time.Time
	if len(recs) > 0 {
		oldestInRing = recs[0].Time
	}
	// Newest first, to stop as soon as the page is full.
	picked := make([]*Record, 0, f.Limit)
	for i := len(recs) - 1; i >= 0; i-- {
		rec := recs[i]
		if rec.Seq <= f.After {
			break
		}
		if !f.Before.IsZero() && !rec.Time.Before(f.Before) {
			continue
		}
		if !f.wants(rec) {
			continue
		}
		if len(picked) == f.Limit {
			page.More = true
			break
		}
		picked = append(picked, rec)
	}
	for i := len(picked) - 1; i >= 0; i-- {
		page.Entries = append(page.Entries, *picked[i])
	}

	// Older than what's in memory.
	if f.History && f.After == 0 && len(picked) < f.Limit {
		before := oldestInRing
		if !f.Before.IsZero() && (before.IsZero() || f.Before.Before(before)) {
			before = f.Before
		}
		older, more, partial, err := l.searchFiles(f, before, f.Limit-len(picked))
		if err != nil {
			return page, err
		}
		page.Entries = append(older, page.Entries...)
		page.More = more
		page.Partial = partial
	}
	for i := range page.Entries {
		rec := &page.Entries[i]
		if rec.Lv >= slog.LevelWarn {
			rec.Explain = Explain(rec)
		}
	}
	return page, nil
}

// searchFiles reads log files, newest first, for the last n matches before a
// time. Whoever writes the files is still appending to one: only finished lines
// are read.
func (l *Logger) searchFiles(f Filter, before time.Time, n int) (out []Record, more, partial bool, err error) {
	if !fileSearch.TryLock() {
		return nil, false, false, ErrBusy
	}
	defer fileSearch.Unlock()
	l.Flush()
	dir := l.Dir()
	var budget int64 = maxScanBytes
	for _, fi := range listLogFiles(dir) {
		if budget <= 0 {
			return out, true, true, nil
		}
		got, over, used := searchFile(filepath.Join(dir, fi.Name), &f, before, n-len(out), budget)
		budget -= used
		out = append(got, out...)
		if over || len(out) >= n {
			return out, true, false, nil
		}
	}
	return out, false, false, nil
}

// searchFile returns the file's last n matches (oldest first), and whether there
// were more than that.
func searchFile(p string, f *Filter, before time.Time, n int, budget int64) (out []Record, over bool, used int64) {
	file, err := os.Open(p)
	if err != nil {
		return nil, false, 0
	}
	defer file.Close()
	if st, err := file.Stat(); err == nil && st.Size() > budget {
		_, _ = file.Seek(st.Size()-budget, 0)
		over = true
	}
	sc := bufio.NewScanner(file)
	sc.Buffer(make([]byte, 64<<10), 256<<10)
	keep := make([]Record, 0, n)
	start := 0
	for sc.Scan() {
		line := sc.Text()
		used += int64(len(line)) + 1
		if len(line) == 0 {
			continue
		}
		rec := ParseLine(line)
		if !before.IsZero() && !rec.Time.IsZero() && !rec.Time.Before(before) {
			continue
		}
		if !f.wants(&rec) {
			continue
		}
		if len(keep) < n {
			keep = append(keep, rec)
		} else {
			keep[start] = rec
			start = (start + 1) % n
			over = true
		}
	}
	out = make([]Record, 0, len(keep))
	out = append(out, keep[start:]...)
	out = append(out, keep[:start]...)
	return out, over, used
}
