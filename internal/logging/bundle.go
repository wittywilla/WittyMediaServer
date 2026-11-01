package logging

import (
	"archive/zip"
	"bufio"
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

// bundleLogs are the files to take, newest first, within the limits.
func bundleLogs(dir string, o *BundleOptions) []FileInfo {
	cutoff := time.Now().AddDate(0, 0, -o.Days)
	var out []FileInfo
	var total int64
	for _, f := range listLogFiles(dir) {
		if f.Modified.Before(cutoff) || total+f.Size > o.MaxLogs {
			continue
		}
		total += f.Size
		out = append(out, f)
	}
	return out
}

const bundleReadme = `WittyMediaServer support bundle

This is what the server recorded, to help someone work out a problem. It was made
on the server when you asked, and has not been sent anywhere. You decide who
sees it.

  summary.json    the server's version, modules, system, disk space and settings
                  that matter. No passwords, tokens or API keys.
  logs/           recent log files. Passwords, tokens, API keys and setup codes
                  are removed from them.
  crashes/        reports from when the server crashed or stopped unexpectedly.
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
	add("README.txt", "What this is", int64(len(readmeFor(o.Sanitize))))
	scrub := o.sanitizer()
	for _, e := range o.Extra {
		add(e.Name, e.Description, int64(len(e.Data)))
		if scrub != nil {
			_ = scrubJSONish(scrub, e.Data)
		}
	}
	logs := bundleLogs(dir, &o)
	for _, f := range logs {
		add("logs/"+f.Name, "Log file from "+f.Modified.Format("2 Jan 2006"), f.Size)
	}
	for _, c := range g.List() {
		summary := c.Summary
		if scrub != nil {
			summary = scrub.Text(summary)
		}
		add("crashes/"+c.ID+".txt", "Crash report: "+summary, c.Size)
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
	if err := write("README.txt", []byte(readmeFor(o.Sanitize))); err != nil {
		return err
	}
	scrub := o.sanitizer()
	for _, e := range o.Extra {
		data := e.Data
		if scrub != nil {
			data = scrubJSONish(scrub, data)
		}
		if err := write(e.Name, data); err != nil {
			return err
		}
	}
	for _, c := range g.List() {
		b, err := g.Read(c.ID)
		if err != nil {
			continue
		}
		text := Redact(string(b))
		if scrub != nil {
			text = scrubCrash(scrub, text)
		}
		if err := write("crashes/"+c.ID+".txt", []byte(text)); err != nil {
			return err
		}
	}
	for _, f := range bundleLogs(dir, &o) {
		out, err := zw.Create("logs/" + f.Name)
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

// SanitizeLog copies any text log, sanitized: a line of ours is handled field by field,
// and any other line as free text.
func SanitizeLog(w io.Writer, r io.Reader, k Known) (map[string]int, error) {
	s := NewSanitizer(k)
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
