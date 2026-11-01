// Package subtitle turns subtitle text into WebVTT, the form players show beside a
// video. It reads the formats FFmpeg doesn't (YouTube's SBV, TTML and DFXP, timed
// lyrics), and straightens out the encodings of the ones it does. The result is only
// ever shown as cue text, which a browser doesn't run.
package subtitle

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/ebook"
)

// MaxInput is the most of a subtitle file that is read. A film's subtitles are a few
// hundred kilobytes.
const MaxInput = 32 << 20

const maxCues = 1 << 20

// ReadsItself says Convert handles this format with no help.
func ReadsItself(format string) bool {
	switch format {
	case "vtt", "sbv", "ttml", "dfxp", "lrc", "srt":
		return true
	}
	return false
}

// Normalize returns text as UTF-8, whatever it was written in, without a byte order
// mark and with plain line ends.
func Normalize(b []byte) []byte {
	s := ebook.DecodeText(b)
	s = strings.TrimPrefix(s, string(rune(0xFEFF)))
	return []byte(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n"))
}

// Convert makes WebVTT from a format ReadsItself says it can read.
func Convert(format string, b []byte) ([]byte, error) {
	if len(b) > MaxInput {
		return nil, fmt.Errorf("this subtitle file is too large")
	}
	text := Normalize(b)
	switch format {
	case "vtt":
		if !bytes.HasPrefix(text, []byte("WEBVTT")) {
			return append([]byte("WEBVTT\n\n"), text...), nil
		}
		return text, nil
	case "srt":
		return fromSRT(text), nil
	case "sbv":
		return fromSBV(text), nil
	case "lrc":
		return fromLRC(text), nil
	case "ttml", "dfxp":
		return fromTTML(text)
	}
	return nil, fmt.Errorf("no converter for %q", format)
}

var srtTimeRe = regexp.MustCompile(`(\d{1,3}:\d{1,2}:\d{1,2})[,.](\d{1,3})\s*-->\s*(\d{1,3}:\d{1,2}:\d{1,2})[,.](\d{1,3})`)
var srtTagRe = regexp.MustCompile(`(?i)</?(font|span|div|p|a|br)\b[^>]*>|\{\\[^}]*\}`)

func fromSRT(text []byte) []byte {
	var out bytes.Buffer
	out.WriteString("WEBVTT\n\n")
	for _, block := range strings.Split(string(text), "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		for i, l := range lines {
			m := srtTimeRe.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			fmt.Fprintf(&out, "%s.%s --> %s.%s\n", pad(m[1]), padMs(m[2]), pad(m[3]), padMs(m[4]))
			for _, t := range lines[i+1:] {
				out.WriteString(cueText(srtTagRe.ReplaceAllString(t, "")) + "\n")
			}
			out.WriteString("\n")
			break
		}
	}
	return out.Bytes()
}

func pad(t string) string {
	parts := strings.Split(t, ":")
	for i, p := range parts {
		if len(p) < 2 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":")
}

func padMs(s string) string {
	for len(s) < 3 {
		s += "0"
	}
	return s
}

// cueText keeps a line of a cue from being read as the end of it, or as markup other
// than the few tags WebVTT shows. "-->" in text would start a new cue.
func cueText(s string) string {
	return strings.ReplaceAll(strings.TrimRight(s, " "), "-->", "-&gt;")
}

var sbvLineRe = regexp.MustCompile(`^(\d{1,2}:\d{1,2}:\d{1,2}\.\d{1,3}),(\d{1,2}:\d{1,2}:\d{1,2}\.\d{1,3})\s*$`)

func fromSBV(text []byte) []byte {
	var out bytes.Buffer
	out.WriteString("WEBVTT\n\n")
	lines := strings.Split(string(text), "\n")
	for i := 0; i < len(lines); i++ {
		m := sbvLineRe.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			continue
		}
		a, b := vttFromSBV(m[1]), vttFromSBV(m[2])
		fmt.Fprintf(&out, "%s --> %s\n", a, b)
		for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" {
			i++
			out.WriteString(cueText(strings.ReplaceAll(lines[i], "[br]", "\n")) + "\n")
		}
		out.WriteString("\n")
	}
	return out.Bytes()
}

func vttFromSBV(t string) string {
	head, ms, _ := strings.Cut(t, ".")
	return pad(head) + "." + padMs(ms)
}

var (
	lrcTimeRe = regexp.MustCompile(`\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	// The word-by-word timing some lyrics carry inside a line.
	lrcWordRe = regexp.MustCompile(`<\d{1,3}:\d{1,2}(?:[.:]\d{1,3})?>`)
)

type lrcLine struct {
	at   float64
	text string
}

func fromLRC(text []byte) []byte {
	var cues []lrcLine
	offset := 0.0
	for _, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "[offset:") {
			v := strings.TrimSuffix(strings.TrimSpace(line[8:]), "]")
			if ms, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && math.Abs(ms) < 1e7 {
				offset = ms / 1000
			}
			continue
		}
		times := lrcTimeRe.FindAllStringSubmatch(line, 64)
		if times == nil || len(cues) >= maxCues {
			continue
		}
		body := strings.TrimSpace(lrcWordRe.ReplaceAllString(lrcTimeRe.ReplaceAllString(line, ""), ""))
		for _, t := range times {
			min, _ := strconv.ParseFloat(t[1], 64)
			sec, _ := strconv.ParseFloat(t[2], 64)
			frac := 0.0
			if t[3] != "" {
				v, _ := strconv.ParseFloat(t[3], 64)
				frac = v / math.Pow(10, float64(len(t[3])))
			}
			cues = append(cues, lrcLine{at: min*60 + sec + frac, text: body})
		}
	}
	sortLRC(cues)
	var out bytes.Buffer
	out.WriteString("WEBVTT\n\n")
	for i, c := range cues {
		if c.text == "" {
			continue
		}
		end := c.at + 6
		if i+1 < len(cues) {
			end = cues[i+1].at
		}
		fmt.Fprintf(&out, "%s --> %s\n%s\n\n", clock(max(0, c.at-offset)), clock(max(0, end-offset)), cueText(c.text))
	}
	return out.Bytes()
}

func sortLRC(c []lrcLine) {
	// Few lines, mostly in order already.
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && c[j].at < c[j-1].at; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}

func clock(sec float64) string {
	ms := int64(math.Round(sec * 1000))
	return fmt.Sprintf("%02d:%02d:%02d.%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}

// fromTTML reads TTML and DFXP: the text of each <p> and its times.
func fromTTML(text []byte) ([]byte, error) {
	d := xml.NewDecoder(bytes.NewReader(text))
	d.Strict = false
	d.Entity = xml.HTMLEntity
	var out bytes.Buffer
	out.WriteString("WEBVTT\n\n")
	frameRate, tickRate := 30.0, 10000000.0
	inP := 0
	var cue strings.Builder
	var begin, end float64
	haveTimes := false
	cues := 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break // what was read so far still shows
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "tt":
				for _, a := range t.Attr {
					switch a.Name.Local {
					case "frameRate":
						if v, err := strconv.ParseFloat(a.Value, 64); err == nil && v > 0 && v < 1000 {
							frameRate = v
						}
					case "tickRate":
						if v, err := strconv.ParseFloat(a.Value, 64); err == nil && v > 0 {
							tickRate = v
						}
					}
				}
			case "p":
				inP++
				if inP == 1 {
					cue.Reset()
					var b, e, dur string
					for _, a := range t.Attr {
						switch a.Name.Local {
						case "begin":
							b = a.Value
						case "end":
							e = a.Value
						case "dur":
							dur = a.Value
						}
					}
					bv, ok1 := ttmlTime(b, frameRate, tickRate)
					ev, ok2 := ttmlTime(e, frameRate, tickRate)
					if !ok2 && dur != "" {
						if dv, ok := ttmlTime(dur, frameRate, tickRate); ok {
							ev, ok2 = bv+dv, true
						}
					}
					begin, end, haveTimes = bv, ev, ok1 && ok2
				}
			case "br":
				if inP > 0 {
					cue.WriteString("\n")
				}
			}
		case xml.EndElement:
			if t.Name.Local == "p" && inP > 0 {
				inP--
				if inP == 0 && haveTimes && strings.TrimSpace(cue.String()) != "" && cues < maxCues {
					cues++
					fmt.Fprintf(&out, "%s --> %s\n", clock(begin), clock(end))
					for _, l := range strings.Split(strings.TrimSpace(cue.String()), "\n") {
						out.WriteString(cueText(strings.TrimSpace(l)) + "\n")
					}
					out.WriteString("\n")
				}
			}
		case xml.CharData:
			if inP > 0 && cue.Len() < 1<<16 {
				cue.WriteString(strings.Join(strings.Fields(string(t)), " "))
			}
		}
	}
	return out.Bytes(), nil
}

var ttmlClockRe = regexp.MustCompile(`^(\d{1,3}):(\d{1,2}):(\d{1,2})(?:[.,](\d{1,9}))?(?::(\d{1,3}))?$`)
var ttmlOffsetRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)(h|ms|m|s|f|t)$`)

func ttmlTime(s string, frameRate, tickRate float64) (float64, bool) {
	s = strings.TrimSpace(s)
	if m := ttmlClockRe.FindStringSubmatch(s); m != nil {
		h, _ := strconv.ParseFloat(m[1], 64)
		mi, _ := strconv.ParseFloat(m[2], 64)
		se, _ := strconv.ParseFloat(m[3], 64)
		v := h*3600 + mi*60 + se
		if m[4] != "" {
			f, _ := strconv.ParseFloat("0."+m[4], 64)
			v += f
		}
		if m[5] != "" {
			f, _ := strconv.ParseFloat(m[5], 64)
			v += f / frameRate
		}
		return v, true
	}
	if m := ttmlOffsetRe.FindStringSubmatch(s); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		switch m[2] {
		case "h":
			return v * 3600, true
		case "m":
			return v * 60, true
		case "s":
			return v, true
		case "ms":
			return v / 1000, true
		case "f":
			return v / frameRate, true
		case "t":
			return v / tickRate, true
		}
	}
	return 0, false
}

var vttTimingRe = regexp.MustCompile(`(?m)^((?:\d{1,3}:)?\d{2}:\d{2}\.\d{3}) --> ((?:\d{1,3}:)?\d{2}:\d{2}\.\d{3})(.*)$`)

// Shift moves every cue earlier by this many seconds, for a player whose clock starts
// that far into the film. Cues that have ended by then go; one that spans the start
// begins at zero.
func Shift(vtt []byte, by float64) []byte {
	if by <= 0 {
		return vtt
	}
	blocks := strings.Split(strings.ReplaceAll(string(vtt), "\r\n", "\n"), "\n\n")
	var out strings.Builder
	for i, b := range blocks {
		m := vttTimingRe.FindStringSubmatchIndex(b)
		if m == nil {
			if i == 0 {
				out.WriteString(b + "\n\n")
			}
			continue
		}
		start, ok1 := parseClock(b[m[2]:m[3]])
		end, ok2 := parseClock(b[m[4]:m[5]])
		if !ok1 || !ok2 || end <= by {
			continue
		}
		out.WriteString(b[:m[0]])
		out.WriteString(clock(max(0, start-by)) + " --> " + clock(end-by) + b[m[6]:m[7]])
		out.WriteString(b[m[1]:])
		out.WriteString("\n\n")
	}
	return []byte(out.String())
}

func parseClock(s string) (float64, bool) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	v := 0.0
	for _, p := range parts {
		f, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0, false
		}
		v = v*60 + f
	}
	return v, true
}
