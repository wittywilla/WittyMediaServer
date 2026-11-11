// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package logging

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A support bundle is a zip the owner downloads and may send to someone: recent
// logs, crash reports and a summary of the system. It is made on request and
// never uploaded. Secrets are removed again on the way in, in case a file is
// from before a redaction rule existed. Sanitized, as it is unless someone asks
// otherwise, names, addresses, titles and paths come out too: see sanitize.go.

// BundleOptions say what goes in.
type BundleOptions struct {
	Days     int      // how far back; 0 = 3
	Sanitize bool     // replace names, addresses, titles and paths with placeholders
	Redact   bool     // the earlier name for Sanitize; either turns it on
	MaxLogs  int64    // bytes of log text at most; 0 = 64 MB
	Known    Known    // what the server can name exactly
	Roots    []string // library folders, added to Known.Folders
	Extra    []BundleFile
	// Earlier are logs and crash reports set aside by a reset, newest first. Those from
	// within Days go in beside today's, under their copy's name.
	Earlier []Earlier
}

// Earlier is one copy a reset made: its name ("before-reset-20261102-165543"), the log
// folder inside it and its crash folder, either of which may be missing.
type Earlier struct {
	Name, Logs, Crashes string
}

// BundleFile is something more to include.
type BundleFile struct {
	Name        string
	Description string
	Data        []byte
}

// BundlePart is one file in the preview.
type BundlePart struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Size        int64  `json:"size"`
}

// ChangedLine shows what sanitizing does to one line.
type ChangedLine struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// BundlePreview is what would be in a bundle, before making it.
type BundlePreview struct {
	Parts    []BundlePart   `json:"parts"`
	Total    int64          `json:"total"`
	Sample   []string       `json:"sample"` // a few lines as they would appear
	Redact   bool           `json:"redact"` // the earlier name for Sanitized
	Sanitize bool           `json:"sanitize"`
	Days     int            `json:"days"`
	Scanned  int            `json:"scanned"`            // how many lines the counts come from
	Replaced map[string]int `json:"replaced,omitempty"` // how many places were changed, by kind
	Changed  []ChangedLine  `json:"changed,omitempty"`  // a few lines, before and after
}

// How much the preview looks at: it's made each time a choice on the page changes.
const (
	previewScanLines = 5000
	previewSample    = 40
	previewChanged   = 6
)

func (o *BundleOptions) defaults() {
	if o.Days <= 0 {
		o.Days = 3
	}
	if o.Days > 14 {
		o.Days = 14
	}
	if o.MaxLogs <= 0 {
		o.MaxLogs = 64 << 20
	}
	if o.Redact {
		o.Sanitize = true
	}
}

func (o *BundleOptions) sanitizer() *Sanitizer {
	if !o.Sanitize {
		return nil
	}
	k := o.Known
	k.Folders = append(append([]string{}, k.Folders...), o.Roots...)
	return NewSanitizer(k)
}

// learn goes through what's going in for names, before any of it is written, so a name is
// hidden from the first file even if only the last one says what it is.
func learn(s *Sanitizer, dir string, logs []FileInfo, g *CrashGuard, earlier []earlierPart, limit int) {
	learnFrom := func(dir string, logs []FileInfo, g *CrashGuard) {
		for _, f := range logs {
			if r, err := os.Open(filepath.Join(dir, f.Name)); err == nil {
				s.learnLines(r, limit)
				r.Close()
			}
		}
		for _, c := range g.List() {
			if b, err := g.Read(c.ID); err == nil {
				s.learnLines(bytes.NewReader(b), limit)
			}
			if p, err := g.PartPath(c.ID, PartFlight); err == nil {
				if r, err := os.Open(p); err == nil {
					s.learnLines(r, limit)
					r.Close()
				}
			}
		}
	}
	learnFrom(dir, logs, g)
	for _, e := range earlier {
		learnFrom(e.Logs, e.logs, e.crashes)
	}
	s.settle()
}

// takeLogs are the files to take from a folder, newest first, within the limits, with
// used bytes of log text already taken.
func takeLogs(dir string, o *BundleOptions, used int64) ([]FileInfo, int64) {
	cutoff := time.Now().AddDate(0, 0, -o.Days)
	var out []FileInfo
	for _, f := range listLogFiles(dir) {
		if f.Modified.Before(cutoff) || used+f.Size > o.MaxLogs {
			continue
		}
		used += f.Size
		out = append(out, f)
	}
	return out, used
}

// earlierPart is what goes in from one copy a reset made, and what was left out.
type earlierPart struct {
	Earlier
	logs    []FileInfo
	crashes *CrashGuard
	reports []Crash
	skipped int // log files left out, for being too old or too many
}

// bundleEarlier finds what goes in from before each reset, within what today's logs left
// of the limit.
func bundleEarlier(o *BundleOptions, used int64) []earlierPart {
	var out []earlierPart
	for _, e := range o.Earlier {
		if !copyNameOK(e.Name) {
			continue
		}
		p := earlierPart{Earlier: e}
		if e.Logs != "" {
			all := listLogFiles(e.Logs)
			p.logs, used = takeLogs(e.Logs, o, used)
			p.skipped = len(all) - len(p.logs)
		}
		if e.Crashes != "" {
			p.crashes = OpenCrashReports(e.Crashes)
			p.reports = p.crashes.List()
		}
		if len(p.logs) > 0 || len(p.reports) > 0 || p.skipped > 0 {
			out = append(out, p)
		}
	}
	return out
}

// copyNameOK accepts only the names a reset gives its copies: they become folders in the zip.
func copyNameOK(name string) bool {
	if !strings.HasPrefix(name, "before-reset-") || len(name) > 64 {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c == '-') {
			return false
		}
	}
	return true
}

// contents says, at the end of the readme, what this bundle covers: the person reading it
// shouldn't have to wonder whether something is missing or just didn't happen.
func contents(logs []FileInfo, crashes []Crash, earlier []earlierPart, days int) string {
	var b strings.Builder
	b.WriteString("What this one holds\n\n")
	from, to := time.Time{}, time.Time{}
	note := func(files []FileInfo) {
		for _, f := range files {
			if to.IsZero() || f.Modified.After(to) {
				to = f.Modified
			}
			if from.IsZero() || f.Modified.Before(from) {
				from = f.Modified
			}
		}
	}
	note(logs)
	for _, e := range earlier {
		note(e.logs)
	}
	if to.IsZero() {
		fmt.Fprintf(&b, "  No log files from the last %d days.\n", days)
	} else {
		// A file's time is its last line: the oldest file started a little earlier.
		fmt.Fprintf(&b, "  Log files last written between %s and %s, from the last %d days.\n", from.Format("2 Jan 2006 15:04"), to.Format("2 Jan 2006 15:04"), days)
	}
	n := len(crashes)
	for _, e := range earlier {
		n += len(e.reports)
	}
	if n == 0 {
		b.WriteString("  No crash reports: the server hasn't crashed or stopped unexpectedly in the last 90 days.\n")
	} else {
		fmt.Fprintf(&b, "  %d crash reports.\n", n)
	}
	for _, e := range earlier {
		fmt.Fprintf(&b, "  From before a reset, in %s: %d log files and %d crash reports", e.Name, len(e.logs), len(e.reports))
		if e.skipped > 0 {
			fmt.Fprintf(&b, ". %d older log files weren't included; they're still in that folder inside the data folder", e.skipped)
		}
		b.WriteString(".\n")
	}
	b.WriteString("\n")
	return b.String()
}

const noCrashes = "No crash reports. The server hasn't crashed or stopped unexpectedly in the last 90 days,\nor the reports were deleted.\n"

const bundleReadme = `WittyMediaServer support bundle

This is what the server recorded, to help someone work out a problem. It was made
on the server when you asked, and has not been sent anywhere. You decide who
sees it.

  summary.json    the server's version, modules, system, disk space and settings
                  that matter. No passwords, tokens or API keys.
  logs/           recent log files. Passwords, tokens, API keys and setup codes
                  are removed from them. Those from before a reset are in a
                  folder named after the copy the reset made.
  crashes/        reports from when the server crashed, stopped unexpectedly or
                  stopped answering, and the ones saved from Settings, or
                  none.txt when there are none. Beside some: .flight.log, the
                  last minutes of everything at the most detailed level;
                  .goroutines.txt, what every part of the server was waiting
                  for; .trace, an execution trace for "go tool trace", which
                  holds only places in the program's code and timings, nothing
                  of yours.
  diagnostics/    reports from WittyDiagnoser, if you ran it.

`

const bundleReadmeSanitized = bundleReadme + `This copy is SANITIZED, so it can be shared. These were replaced with placeholders:

  [ip-1]      IP addresses            [user-1]    account names
  [host-1]    computer and web names  [email-1]   email addresses
  [path-1]    file paths              [folder-1]  library and server folders
  [title-1]   titles of what you own  [library-1] library names
  [mac-1]     network card addresses  [device-1]  device names

The same thing has the same number throughout this bundle, so what happened to one
file can still be followed. The list of what each number stood for was never saved.
Versions, codecs, status codes, routes, timings and errors are left as they were.

`

const bundleReadmeRaw = bundleReadme + `This copy is NOT sanitized. It holds your folder and file names, what's in your
libraries, account names and network addresses. Read it before you pass it on.

`

func readmeFor(sanitized bool) string {
	if sanitized {
		return bundleReadmeSanitized
	}
	return bundleReadmeRaw
}

// Preview says what WriteBundle would write.
func (l *Logger) Preview(g *CrashGuard, o BundleOptions) BundlePreview {
	l.Flush()
	return PreviewDir(l.Dir(), g, o)
}

// PreviewDir is Preview for a folder of logs, with no server running.
func PreviewDir(dir string, g *CrashGuard, o BundleOptions) BundlePreview {
	o.defaults()
	p := BundlePreview{Redact: o.Sanitize, Sanitize: o.Sanitize, Days: o.Days, Sample: []string{}}
	add := func(name, desc string, size int64) {
		p.Parts = append(p.Parts, BundlePart{name, desc, size})
		p.Total += size
	}
	scrub := o.sanitizer()
	logs, used := takeLogs(dir, &o, 0)
	earlier := bundleEarlier(&o, used)
	crashes := g.List()
	add("README.txt", "What this is, and what it holds", int64(len(readmeFor(o.Sanitize))+len(contents(logs, crashes, earlier, o.Days))))
	if scrub != nil {
		learn(scrub, dir, logs, g, earlier, previewScanLines)
	}
	for _, e := range o.Extra {
		add(e.Name, e.Description, int64(len(e.Data)))
		if scrub != nil {
			_ = scrubJSONish(scrub, e.Data)
		}
	}
	for _, f := range logs {
		add("logs/"+f.Name, "Log file from "+f.Modified.Format("2 Jan 2006"), f.Size)
	}
	crashPart := func(prefix string, c Crash) {
		summary := c.Summary
		if scrub != nil {
			summary = scrub.Text(summary)
		}
		add(prefix+c.ID+".txt", "Crash report: "+summary, c.Size)
		for _, part := range c.Parts {
			if path, err := g.PartPath(c.ID, part); err == nil {
				if st, err := os.Stat(path); err == nil {
					add(prefix+c.ID+"."+part, partWords[part], st.Size())
				}
			}
		}
	}
	for _, c := range crashes {
		crashPart("crashes/", c)
	}
	for _, e := range earlier {
		for _, f := range e.logs {
			add("logs/"+e.Name+"/"+f.Name, "Log file from "+f.Modified.Format("2 Jan 2006")+", before a reset", f.Size)
		}
		for _, c := range e.reports {
			crashPart("crashes/"+e.Name+"/", c)
		}
	}
	if len(crashes) == 0 && !earlierCrashes(earlier) {
		add("crashes/none.txt", "There are no crash reports", int64(len(noCrashes)))
	}
	for _, f := range logs {
		if p.Scanned >= previewScanLines {
			break
		}
		_ = eachBundleLine(filepath.Join(dir, f.Name), scrub, previewScanLines-p.Scanned, func(line, before []byte) error {
			p.Scanned++
			text := strings.TrimRight(string(line), "\n")
			if len(p.Sample) < previewSample {
				p.Sample = append(p.Sample, text)
			}
			if scrub != nil && len(p.Changed) < previewChanged && string(before) != string(line) {
				p.Changed = append(p.Changed, ChangedLine{strings.TrimRight(string(before), "\n"), text})
			}
			return nil
		})
	}
	if scrub != nil {
		p.Replaced = scrub.Counts()
	}
	return p
}

// WriteBundle writes the zip. Whoever calls it streams it to the browser, so it
// is limited here rather than held in memory.
func (l *Logger) WriteBundle(w io.Writer, g *CrashGuard, o BundleOptions) error {
	l.Flush()
	return WriteBundleDir(w, l.Dir(), g, o)
}

// WriteBundleDir is WriteBundle for a folder of logs, with no server running.
func WriteBundleDir(w io.Writer, dir string, g *CrashGuard, o BundleOptions) error {
	o.defaults()
	zw := zip.NewWriter(w)
	defer zw.Close()
	write := func(name string, data []byte) error {
		f, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	scrub := o.sanitizer()
	logs, used := takeLogs(dir, &o, 0)
	earlier := bundleEarlier(&o, used)
	crashes := g.List()
	if err := write("README.txt", []byte(readmeFor(o.Sanitize)+contents(logs, crashes, earlier, o.Days))); err != nil {
		return err
	}
	if scrub != nil {
		learn(scrub, dir, logs, g, earlier, 0)
	}
	for _, e := range o.Extra {
		data := e.Data
		if scrub != nil {
			data = scrubJSONish(scrub, data)
		}
		if err := write(e.Name, data); err != nil {
			return err
		}
	}
	writeCrashes := func(prefix string, g *CrashGuard, list []Crash) error {
		for _, c := range list {
			b, err := g.Read(c.ID)
			if err != nil {
				continue
			}
			text := Redact(string(b))
			if scrub != nil {
				text = scrubCrash(scrub, text)
			}
			if err := write(prefix+c.ID+".txt", []byte(text)); err != nil {
				return err
			}
			for _, part := range c.Parts {
				if err := writePart(zw, prefix, g, c.ID, part, scrub); err != nil {
					return err
				}
			}
		}
		return nil
	}
	writeLogs := func(prefix, dir string, logs []FileInfo) error {
		for _, f := range logs {
			out, err := zw.Create(prefix + f.Name)
			if err != nil {
				return err
			}
			if err := eachBundleLine(filepath.Join(dir, f.Name), scrub, 0, func(line, _ []byte) error {
				_, err := out.Write(line)
				return err
			}); err != nil {
				return err
			}
		}
		return nil
	}
	if err := writeCrashes("crashes/", g, crashes); err != nil {
		return err
	}
	if len(crashes) == 0 && !earlierCrashes(earlier) {
		if err := write("crashes/none.txt", []byte(noCrashes)); err != nil {
			return err
		}
	}
	if err := writeLogs("logs/", dir, logs); err != nil {
		return err
	}
	for _, e := range earlier {
		if err := writeCrashes("crashes/"+e.Name+"/", e.crashes, e.reports); err != nil {
			return err
		}
		if err := writeLogs("logs/"+e.Name+"/", e.Logs, e.logs); err != nil {
			return err
		}
	}
	return nil
}

func earlierCrashes(earlier []earlierPart) bool {
	for _, e := range earlier {
		if len(e.reports) > 0 {
			return true
		}
	}
	return false
}

var partWords = map[string]string{
	PartFlight:     "The last minutes before it, at the most detailed level",
	PartGoroutines: "What every part of the server was waiting for",
	PartTrace:      "An execution trace, for go tool trace",
}

// writePart adds the evidence beside a report: the flight recorder as log lines, the
// stacks as a crash's text, the trace as it is.
func writePart(zw *zip.Writer, prefix string, g *CrashGuard, id, part string, scrub *Sanitizer) error {
	path, err := g.PartPath(id, part)
	if err != nil {
		return nil
	}
	out, err := zw.Create(prefix + id + "." + part)
	if err != nil {
		return err
	}
	switch part {
	case PartFlight:
		return eachBundleLine(path, scrub, 0, func(line, _ []byte) error {
			_, err := out.Write(line)
			return err
		})
	case PartGoroutines:
		b, err := readBounded(path, 4*maxCrashSize)
		if err != nil {
			return nil
		}
		text := Redact(string(b))
		if scrub != nil {
			text = scrubCrash(scrub, text)
		}
		_, err = io.WriteString(out, text)
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	_, err = io.Copy(out, io.LimitReader(f, 64<<20))
	return err
}

// SanitizeLog copies any text log, sanitized: a line of ours is handled field by field,
// and any other line as free text.
func SanitizeLog(w io.Writer, r io.Reader, k Known) (map[string]int, error) {
	s := NewSanitizer(k)
	if seeker, ok := r.(io.ReadSeeker); ok {
		s.learnLines(seeker, 0)
		s.settle()
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	}
	bw := bufio.NewWriter(w)
	br := bufio.NewReaderSize(r, 64<<10)
	var buf []byte
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			text := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(text, "time=") {
				rec := ParseLine(text)
				buf = sanitizedLine(s, &rec, buf)
				if _, werr := bw.Write(buf); werr != nil {
					return nil, werr
				}
			} else if _, werr := bw.WriteString(s.Final(s.crashLine(text)) + "\n"); werr != nil {
				return nil, werr
			}
		}
		if err == io.EOF {
			return s.Counts(), bw.Flush()
		}
		if err != nil {
			return nil, err
		}
	}
}

func sanitizedLine(s *Sanitizer, rec *Record, buf []byte) []byte {
	cleanRecord(rec)
	s.Record(rec)
	buf = rec.appendLine(buf[:0])
	return []byte(s.Final(string(buf)))
}

// cleanRecord takes out secrets again, in case the line is from before a rule existed.
func cleanRecord(rec *Record) {
	rec.Msg = Redact(rec.Msg)
	for i := range rec.Attrs {
		if secretKey(rec.Attrs[i].K) {
			rec.Attrs[i].V = hidden
		} else {
			rec.Attrs[i].V = Redact(rec.Attrs[i].V)
		}
	}
}

// eachBundleLine reads a log file line by line, re-redacting every line and
// hiding what the sanitizer hides if there is one. Lines come out in the same
// format. emit gets the line and the line as it was with only secrets taken out.
// limit stops early (for the preview); 0 reads to the end.
func eachBundleLine(p string, scrub *Sanitizer, limit int, emit func(line, before []byte) error) error {
	f, err := os.Open(p)
	if err != nil {
		return nil // rotated away since the list was made
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 512<<10)
	var buf, before []byte
	n := 0
	for sc.Scan() {
		rec := ParseLine(sc.Text())
		cleanRecord(&rec)
		before = rec.appendLine(before[:0])
		if scrub != nil {
			buf = sanitizedLine(scrub, &rec, buf)
		} else {
			buf = append(buf[:0], before...)
		}
		if err := emit(buf, before); err != nil {
			return err
		}
		n++
		if limit > 0 && n >= limit {
			return nil
		}
	}
	return sc.Err()
}

// CopyRedacted writes a log file with secrets taken out again, as a bundle does.
func CopyRedacted(w io.Writer, p string) error {
	bw := bufio.NewWriter(w)
	if err := eachBundleLine(p, nil, 0, func(line, _ []byte) error {
		_, err := bw.Write(line)
		return err
	}); err != nil {
		return err
	}
	return bw.Flush()
}

// A crash report is free text, with its log lines at the end.
func scrubCrash(s *Sanitizer, text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "time=") {
			rec := ParseLine(line)
			s.Record(&rec)
			lines[i] = s.Final(strings.TrimRight(string(rec.appendLine(nil)), "\n"))
			continue
		}
		// Source files in a stack are the program's, not the owner's: they are kept.
		lines[i] = s.Final(s.crashLine(line))
	}
	return strings.Join(lines, "\n")
}

var goSourceLine = regexp.MustCompile(`\.go:\d+`)

// scrubJSONish hides what could identify someone in a summary or report: JSON is
// walked, by key, and anything else is treated as text.
func scrubJSONish(s *Sanitizer, data []byte) []byte {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return []byte(s.Final(s.Text(string(data))))
	}
	v = scrubValue(s, "", v)
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte(fmt.Sprintf("{\"error\":%q}", "could not sanitize this file"))
	}
	return []byte(s.Final(string(out)))
}

func scrubValue(s *Sanitizer, key string, v any) any {
	switch x := v.(type) {
	case map[string]any:
		// A key can be a name too. In order, so numbers are given out the same way each time.
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(x))
		for _, k := range keys {
			out[s.Final(k)] = scrubValue(s, k, x[k])
		}
		return out
	case []any:
		for i, sub := range x {
			x[i] = scrubValue(s, key, sub)
		}
	case string:
		return s.Attr(key, x)
	}
	return v
}
