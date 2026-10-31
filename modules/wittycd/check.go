package wittycd

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Facts and Check are what a diagnostic tool needs: structure and counts, never a
// byte of audio or a file's full path, so a report is safe to share.

// Facts is a disc's structure in plain data.
type Facts struct {
	Format      string    `json:"format"`
	DiscID      string    `json:"discId"`
	CDDBID      string    `json:"cddbId"`
	TOC         string    `json:"toc"`
	Tracks      []Track   `json:"tracks"`
	Sessions    int       `json:"sessions"`
	DurationSec float64   `json:"durationSec"`
	HiddenTrack bool      `json:"hiddenTrack"`
	HasGraphics bool      `json:"hasGraphics"`
	Problem     string    `json:"problem,omitempty"`
	Warnings    []Warning `json:"warnings"`
}

// Facts reads a disc's structure.
func (d *Disc) Facts() Facts {
	toc := d.TOC()
	f := Facts{Format: d.Format, DiscID: toc.MusicBrainzID(), CDDBID: toc.CDDBID(), TOC: toc.LookupString(), Tracks: d.Tracks,
		Sessions: d.Sessions, DurationSec: d.DurationSec(), HiddenTrack: d.HiddenTrack, HasGraphics: d.HasGraphics, Problem: d.Problem, Warnings: d.Warnings}
	if f.Warnings == nil {
		f.Warnings = []Warning{}
	}
	return f
}

// Severity is how much a finding matters.
type Severity string

const (
	SeverityOK      Severity = "ok"
	SeverityNote    Severity = "note"
	SeverityProblem Severity = "problem"
)

// Finding is one thing a check found.
type Finding struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	Text     string   `json:"text"`
	// Track is the track concerned, 0 for the whole disc.
	Track int `json:"track,omitempty"`
}

// Report is the outcome of reading a whole disc.
type Report struct {
	Facts       Facts     `json:"facts"`
	AudioTracks int       `json:"audioTracks"`
	Sectors     int       `json:"sectors"`
	Unreadable  int       `json:"unreadable"`
	Findings    []Finding `json:"findings"`
	Worst       Severity  `json:"worst"`
	Summary     string    `json:"summary"`
}

func (r *Report) add(s Severity, code string, track int, format string, a ...any) {
	r.Findings = append(r.Findings, Finding{Severity: s, Code: code, Track: track, Text: fmt.Sprintf(format, a...)})
	switch {
	case s == SeverityProblem:
		r.Worst = SeverityProblem
	case s == SeverityNote && r.Worst == SeverityOK:
		r.Worst = SeverityNote
	}
}

// Check reads every audio sector of the disc and reports what it finds. progress,
// if given, is called now and then with the fraction done and can stop the check
// by returning false. It is the hook for the Disk Diagnoser and for `cd check`.
func (d *Disc) Check(progress func(done float64) bool) *Report {
	r := &Report{Facts: d.Facts(), Findings: []Finding{}, Worst: SeverityOK}
	audio := d.AudioTracks()
	r.AudioTracks = len(audio)
	for _, w := range d.Warnings {
		sev := SeverityNote
		switch w.Code {
		case "truncated", "truncated-tracks", "missing-file", "noise", "dts", "bad-tracks":
			sev = SeverityProblem
		}
		r.add(sev, w.Code, 0, "%s", w.Message)
	}
	total := 0
	for _, t := range audio {
		total += t.End - t.Pregap
	}
	buf := make([]byte, 75*SectorBytes)
	done := 0
	silent := 0
	var clipped int64
	for _, t := range audio {
		trackSilent := true
		for lba := t.Pregap; lba < t.End; lba += 75 {
			n := min(t.End-lba, 75)
			if err := d.readAudioStrict(lba, buf[:n*SectorBytes]); err != nil {
				r.Unreadable += n
				r.add(SeverityProblem, "unreadable", t.Number, "Track %d can't be read at %s: %v", t.Number, msfText(lba), err)
				break
			}
			if lba >= t.Start {
				for i := 0; i+1 < n*SectorBytes; i += 2 {
					v := int16(binary.LittleEndian.Uint16(buf[i:]))
					if v != 0 {
						trackSilent = false
					}
					if v == 32767 || v == -32768 {
						clipped++
					}
				}
			}
			done += n
			r.Sectors += n
			if progress != nil && !progress(float64(done)/float64(max(total, 1))) {
				r.Summary = "stopped before the end"
				return r
			}
		}
		if trackSilent {
			silent++
			r.add(SeverityNote, "silent-track", t.Number, "Track %d is completely silent.", t.Number)
		}
	}
	if silent == len(audio) && len(audio) > 0 {
		r.add(SeverityProblem, "all-silent", 0, "Every track on this disc is silent. The image may be empty or damaged.")
	}
	if p := float64(clipped) / float64(max(r.Sectors*SectorSamples*2, 1)); p > 0.05 {
		r.add(SeverityNote, "clipped", 0, "%.0f%% of this disc's samples are at full level, which usually means damaged audio or a very loud master.", p*100)
	}
	if d.Text.Title == "" && !trackHasText(d) {
		r.add(SeverityNote, "no-text", 0, "This image doesn't name its album or tracks, so names come from a lookup or the file name.")
	}
	switch r.Worst {
	case SeverityProblem:
		r.Summary = "problems found"
	case SeverityNote:
		r.Summary = "plays fine, with notes"
	default:
		r.Summary = "no problems found"
	}
	return r
}

func trackHasText(d *Disc) bool {
	for i := range d.Tracks {
		if d.Tracks[i].Text.Title != "" {
			return true
		}
	}
	return false
}

func msfText(sectors int) string {
	return fmt.Sprintf("%02d:%02d:%02d", sectors/FramesPerSecond/60, sectors/FramesPerSecond%60, sectors%FramesPerSecond)
}

// readAudioStrict reads audio and reports a short read as an error, which playing
// doesn't: a check wants to know, a listener wants the music to go on.
func (d *Disc) readAudioStrict(lba int, p []byte) error {
	for len(p) > 0 {
		e := d.extentAt(lba)
		if e == nil {
			return fmt.Errorf("nothing in the image supplies this sector")
		}
		n := min(len(p), SectorBytes)
		if e.src == nil {
			clear(p[:n])
		} else {
			m, err := e.src.ReadAt(p[:n], e.offset+int64(lba-e.lba)*e.stride)
			if m < n {
				if err == nil || err == io.EOF {
					return fmt.Errorf("the image ends here")
				}
				return err
			}
			if e.bigEndian {
				swap16(p[:n], 0)
			}
		}
		p = p[n:]
		lba++
	}
	return nil
}
