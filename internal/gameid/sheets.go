// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gameid

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Disc description files: a cue sheet, a GD-ROM list, a playlist. They're text naming
// other files, so what they prove comes from those files. Names in them can say
// anything, and none of them may lead outside the folder the sheet is in.

// Sheet is what one of these files lists.
type Sheet struct {
	// Files are every file it names that's inside its own folder, in order.
	Files []string
	// Data is the file holding the first data track: what a console reads.
	Data string
	// AudioOnly: every track is audio, so it's a music CD.
	AudioOnly bool
	// Missing are the files it names that aren't there.
	Missing []string
}

var (
	cueFileRe  = regexp.MustCompile(`(?i)^\s*FILE\s+(?:"([^"]+)"|(\S+))\s+[A-Z0-9]+\s*$`)
	cueTrackRe = regexp.MustCompile(`(?i)^\s*TRACK\s+[0-9]+\s+([A-Z0-9/]+)\s*$`)
	gdiLineRe  = regexp.MustCompile(`^\s*[0-9]+\s+[0-9]+\s+([0-9]+)\s+[0-9]+\s+(?:"([^"]+)"|(\S+))\s+[0-9]+\s*$`)
	tocFileRe  = regexp.MustCompile(`(?i)^\s*(?:DATA)?FILE\s+"([^"]+)"`)
)

// ReadSheet reads a .cue, .gdi, .ccd, .toc or .m3u. It reads at most 1 MB and 2,000 lines.
// A .ccd names nothing: its image is the .img beside it.
func ReadSheet(path string) (*Sheet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseSheet(filepath.Ext(path), filepath.Base(path), io.LimitReader(f, 1<<20), filepath.Dir(path), func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && st.Mode().IsRegular()
	})
}

// ParseSheet does the work of ReadSheet for a sheet from anywhere. exists says whether
// a file is there, given dir joined with what the sheet says.
func ParseSheet(ext, name string, in io.Reader, dir string, exists func(string) bool) (*Sheet, error) {
	ext = strings.ToLower(ext)
	s := &Sheet{}
	add := func(raw string, data bool) {
		full := filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(raw, `\`, "/")))
		rel, err := filepath.Rel(dir, full)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return // leads out of its folder: not something to read
		}
		if !exists(full) {
			// A sheet made on another machine may have written the case differently. What's found
			// still has to be a file: this also finds a pipe, or a link to a device, by its own name.
			if found := sameIgnoringCase(full); found != "" && exists(found) {
				full = found
			} else {
				s.Missing = append(s.Missing, filepath.Base(full))
				return
			}
		}
		s.Files = append(s.Files, full)
		if data && s.Data == "" {
			s.Data = full
		}
	}
	if ext == ".ccd" {
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		add(stem+".img", true)
		return s, nil
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 4096), 64<<10)
	lastFile := ""
	var tracks, audio int
	for lines := 0; sc.Scan() && lines < 2000; lines++ {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\xef\xbb\xbf"))
		switch ext {
		case ".cue":
			if m := cueFileRe.FindStringSubmatch(line); m != nil {
				lastFile = m[1] + m[2]
				add(lastFile, false)
			} else if m := cueTrackRe.FindStringSubmatch(line); m != nil {
				tracks++
				if strings.EqualFold(m[1], "AUDIO") {
					audio++
				} else if s.Data == "" && lastFile != "" {
					s.Data = lastData(s, lastFile)
				}
			}
		case ".gdi":
			if m := gdiLineRe.FindStringSubmatch(line); m != nil {
				tracks++
				file := m[2] + m[3]
				if m[1] == "0" {
					audio++
					add(file, false)
				} else {
					add(file, true)
				}
			}
		case ".toc":
			if m := tocFileRe.FindStringSubmatch(line); m != nil {
				add(m[1], true)
			}
		default: // a playlist
			if line != "" && !strings.HasPrefix(line, "#") {
				add(line, true)
			}
		}
	}
	s.AudioOnly = tracks > 0 && audio == tracks
	if err := sc.Err(); err != nil && err != bufio.ErrTooLong {
		return s, err
	}
	return s, nil
}

// lastData finds a file just listed, under the name it was found by.
func lastData(s *Sheet, raw string) string {
	want := filepath.Base(filepath.FromSlash(strings.ReplaceAll(raw, `\`, "/")))
	for i := len(s.Files) - 1; i >= 0; i-- {
		if strings.EqualFold(filepath.Base(s.Files[i]), want) {
			return s.Files[i]
		}
	}
	return ""
}

func sameIgnoringCase(full string) string {
	dir, want := filepath.Dir(full), filepath.Base(full)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), want) && !e.IsDir() {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// SniffFile opens a file and says what it is. A sheet is judged by what it names.
func SniffFile(path string) (Finding, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cue", ".gdi", ".ccd", ".toc", ".m3u":
		return sniffSheet(path, 0)
	}
	return sniffPlain(path)
}

func sniffPlain(path string) (Finding, error) {
	// Before opening it: opening a pipe waits for someone to write to it.
	if st, err := os.Stat(path); err == nil && !st.Mode().IsRegular() {
		return Finding{Kind: KindOther}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return Finding{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Finding{}, err
	}
	if !st.Mode().IsRegular() {
		return Finding{Kind: KindOther}, nil
	}
	return Sniff(f, st.Size()), nil
}

func sniffSheet(path string, depth int) (Finding, error) {
	sheet, err := ReadSheet(path)
	if err != nil {
		return Finding{}, err
	}
	base := filepath.Base(path)
	if sheet.AudioOnly {
		return Finding{Kind: KindAudio, Proof: base + " lists only audio tracks, so it's a music CD."}, nil
	}
	if sheet.Data == "" {
		if len(sheet.Missing) > 0 {
			return Finding{Kind: KindData, Proof: base + " names " + strings.Join(short(sheet.Missing), ", ") + ", which isn't there."}, nil
		}
		return Finding{Proof: base + " doesn't name anything to read."}, nil
	}
	var inner Finding
	switch strings.ToLower(filepath.Ext(sheet.Data)) {
	case ".cue", ".gdi", ".ccd", ".toc":
		if depth > 0 {
			return Finding{}, nil
		}
		inner, err = sniffSheet(sheet.Data, depth+1)
	default:
		inner, err = sniffPlain(sheet.Data)
	}
	if err != nil {
		return Finding{}, err
	}
	if len(sheet.Missing) > 0 {
		inner.Kind = KindData
		inner.Proof = base + " names " + strings.Join(short(sheet.Missing), ", ") + ", which isn't there."
	}
	return inner, nil
}

func short(list []string) []string {
	if len(list) <= 3 {
		return list
	}
	return append(append([]string{}, list[:3]...), "and others")
}
