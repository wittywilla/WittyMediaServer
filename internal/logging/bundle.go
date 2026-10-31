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
	"strings"
	"time"
)

// A support bundle is a zip the owner downloads and may send to someone: recent
// logs, crash reports and a summary of the system. It is made on request and
// never uploaded. Secrets are removed again on the way in, in case a file is
// from before a redaction rule existed; titles and paths come out if asked.

// BundleOptions say what goes in.
type BundleOptions struct {
	Days    int      // how far back; 0 = 3
	Redact  bool     // hide titles and paths
	MaxLogs int64    // bytes of log text at most; 0 = 64 MB
	Roots   []string // library folders, for hiding their names
	Extra   []BundleFile
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

// BundlePreview is what would be in a bundle, before making it.
type BundlePreview struct {
	Parts  []BundlePart `json:"parts"`
	Total  int64        `json:"total"`
	Sample []string     `json:"sample"` // a few lines as they would appear
	Redact bool         `json:"redact"`
	Days   int          `json:"days"`
}

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
}

// bundleLogs are the files to take, newest first, within the limits.
func (l *Logger) bundleLogs(o *BundleOptions) []FileInfo {
	cutoff := time.Now().AddDate(0, 0, -o.Days)
	var out []FileInfo
	var total int64
	for _, f := range l.Files() {
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
  diagnoser/      reports from WittyDiagnoser, if you ran it.

`

// Preview says what WriteBundle would write.
func (l *Logger) Preview(g *CrashGuard, o BundleOptions) BundlePreview {
	o.defaults()
	p := BundlePreview{Redact: o.Redact, Days: o.Days, Sample: []string{}}
	add := func(name, desc string, size int64) {
		p.Parts = append(p.Parts, BundlePart{name, desc, size})
		p.Total += size
	}
	add("README.txt", "What this is", int64(len(bundleReadme)))
	for _, e := range o.Extra {
		add(e.Name, e.Description, int64(len(e.Data)))
	}
	logs := l.bundleLogs(&o)
	for _, f := range logs {
		add("logs/"+f.Name, "Log file from "+f.Modified.Format("2 Jan 2006"), f.Size)
	}
	for _, c := range g.List() {
		add("crashes/"+c.ID+".txt", "Crash report: "+c.Summary, c.Size)
	}
	if len(logs) > 0 {
		var scrub *Scrubber
		if o.Redact {
			scrub = NewScrubber(o.Roots)
		}
		_ = eachBundleLine(filepath.Join(l.Dir(), logs[0].Name), scrub, 40, func(line []byte) error {
			p.Sample = append(p.Sample, strings.TrimRight(string(line), "\n"))
			return nil
		})
	}
	return p
}

// WriteBundle writes the zip. Whoever calls it streams it to the browser, so it
// is limited here rather than held in memory.
func (l *Logger) WriteBundle(w io.Writer, g *CrashGuard, o BundleOptions) error {
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
	if err := write("README.txt", []byte(bundleReadme)); err != nil {
		return err
	}
	var scrub *Scrubber
	if o.Redact {
		scrub = NewScrubber(o.Roots)
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
	for _, f := range l.bundleLogs(&o) {
		out, err := zw.Create("logs/" + f.Name)
		if err != nil {
			return err
		}
		if err := eachBundleLine(filepath.Join(l.Dir(), f.Name), scrub, 0, func(line []byte) error {
			_, err := out.Write(line)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// eachBundleLine reads a log file line by line, re-redacting every line and
// hiding titles and paths if there's a scrubber. Lines come out in the same
// format. limit stops early (for the preview); 0 reads to the end.
func eachBundleLine(p string, scrub *Scrubber, limit int, emit func([]byte) error) error {
	f, err := os.Open(p)
	if err != nil {
		return nil // rotated away since the list was made
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 512<<10)
	var buf []byte
	n := 0
	for sc.Scan() {
		rec := ParseLine(sc.Text())
		rec.Msg = Redact(rec.Msg)
		for i := range rec.Attrs {
			if secretKey(rec.Attrs[i].K) {
				rec.Attrs[i].V = hidden
			} else {
				rec.Attrs[i].V = Redact(rec.Attrs[i].V)
			}
		}
		if scrub != nil {
			scrub.Record(&rec)
		}
		buf = rec.appendLine(buf[:0])
		if err := emit(buf); err != nil {
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
func scrubCrash(s *Scrubber, text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "time=") {
			rec := ParseLine(line)
			s.Record(&rec)
			lines[i] = strings.TrimRight(string(rec.appendLine(nil)), "\n")
			continue
		}
		// Source files in a stack are the program's, not the owner's: they are kept.
		if strings.HasPrefix(line, "\t") && goSourceLine.MatchString(line) {
			continue
		}
		lines[i] = s.Text(line)
	}
	return strings.Join(lines, "\n")
}

var goSourceLine = regexp.MustCompile(`\.go:\d+`)

// scrubJSONish hides titles and paths in a summary or report: JSON is walked,
// by key, and anything else is treated as text.
func scrubJSONish(s *Scrubber, data []byte) []byte {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return []byte(s.Text(string(data)))
	}
	v = scrubValue(s, "", v)
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte(fmt.Sprintf("{\"error\":%q}", "could not hide titles and paths in this file"))
	}
	return out
}

func scrubValue(s *Scrubber, key string, v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, sub := range x {
			x[k] = scrubValue(s, k, sub)
		}
	case []any:
		for i, sub := range x {
			x[i] = scrubValue(s, key, sub)
		}
	case string:
		return s.Attr(key, x)
	}
	return v
}
