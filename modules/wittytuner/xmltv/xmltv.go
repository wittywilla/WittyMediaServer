// Package xmltv reads XMLTV guide files as they stream by, so a very large
// guide needs very little memory.
package xmltv

import (
	"bufio"
	"compress/gzip"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"
)

// Channel is a <channel> element.
type Channel struct {
	ID    string
	Names []string
	Icon  string
}

// Programme is a <programme> element.
type Programme struct {
	Channel  string
	Start    time.Time
	Stop     time.Time
	Title    string
	Sub      string
	Desc     string
	Category string
	Rating   string
	Season   *int
	Episode  *int
	Year     int
}

// MaxBytes bounds what Parse will read, even from a compressed file.
const MaxBytes = 1 << 30

type xmlChannel struct {
	ID    string   `xml:"id,attr"`
	Names []string `xml:"display-name"`
	Icon  struct {
		Src string `xml:"src,attr"`
	} `xml:"icon"`
}

type xmlEpisode struct {
	System string `xml:"system,attr"`
	Value  string `xml:",chardata"`
}

type xmlProgramme struct {
	Start    string       `xml:"start,attr"`
	Stop     string       `xml:"stop,attr"`
	Channel  string       `xml:"channel,attr"`
	Titles   []string     `xml:"title"`
	Subs     []string     `xml:"sub-title"`
	Descs    []string     `xml:"desc"`
	Category []string     `xml:"category"`
	Date     string       `xml:"date"`
	Episodes []xmlEpisode `xml:"episode-num"`
	Rating   []struct {
		Value string `xml:"value"`
	} `xml:"rating"`
}

// Parse reads a guide, gzipped or not, calling back for each element. A
// callback returning false stops the read. Unknown elements are ignored; one
// bad programme does not end the read.
func Parse(r io.Reader, onChannel func(Channel) bool, onProgramme func(Programme) bool) error {
	br := bufio.NewReader(r)
	var src io.Reader = br
	if m, _ := br.Peek(2); len(m) == 2 && m[0] == 0x1f && m[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			return err
		}
		defer gz.Close()
		src = gz
	}
	dec := xml.NewDecoder(io.LimitReader(src, MaxBytes))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "channel":
			var c xmlChannel
			if dec.DecodeElement(&c, &se) != nil || c.ID == "" {
				continue
			}
			if onChannel != nil && !onChannel(Channel{ID: c.ID, Names: c.Names, Icon: c.Icon.Src}) {
				return nil
			}
		case "programme":
			var p xmlProgramme
			if dec.DecodeElement(&p, &se) != nil {
				continue
			}
			pr, ok := convert(p)
			if ok && onProgramme != nil && !onProgramme(pr) {
				return nil
			}
		}
	}
}

func convert(p xmlProgramme) (Programme, bool) {
	out := Programme{Channel: p.Channel}
	var ok bool
	if out.Start, ok = ParseTime(p.Start); !ok || p.Channel == "" || len(p.Titles) == 0 {
		return out, false
	}
	if out.Stop, ok = ParseTime(p.Stop); !ok {
		// A guide may leave the end out: the next programme's start ends it.
		out.Stop = time.Time{}
	}
	out.Title = strings.TrimSpace(p.Titles[0])
	if out.Title == "" {
		return out, false
	}
	if len(p.Subs) > 0 {
		out.Sub = strings.TrimSpace(p.Subs[0])
	}
	if len(p.Descs) > 0 {
		out.Desc = strings.TrimSpace(p.Descs[0])
	}
	if len(p.Category) > 0 {
		out.Category = strings.TrimSpace(p.Category[0])
	}
	if len(p.Rating) > 0 {
		out.Rating = strings.TrimSpace(p.Rating[0].Value)
	}
	if len(p.Date) >= 4 {
		out.Year, _ = strconv.Atoi(p.Date[:4])
	}
	for _, e := range p.Episodes {
		if e.System == "xmltv_ns" {
			out.Season, out.Episode = parseNS(e.Value)
			break
		}
		if e.System == "onscreen" && out.Season == nil {
			out.Season, out.Episode = parseOnscreen(e.Value)
		}
	}
	return out, true
}

// ParseTime reads "20061231235959 +0100"; every part after the date is
// optional, and a missing zone means UTC.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	digits, zone, hasZone := strings.Cut(s, " ")
	if !hasZone && len(digits) > 14 {
		digits, zone = digits[:14], digits[14:]
	}
	if len(digits) < 8 || len(digits) > 14 {
		return time.Time{}, false
	}
	for len(digits) < 14 {
		digits += "0"
	}
	loc := time.UTC
	zone = strings.TrimSpace(zone)
	if len(zone) == 5 && (zone[0] == '+' || zone[0] == '-') {
		h, e1 := strconv.Atoi(zone[1:3])
		m, e2 := strconv.Atoi(zone[3:5])
		if e1 == nil && e2 == nil {
			off := h*3600 + m*60
			if zone[0] == '-' {
				off = -off
			}
			loc = time.FixedZone(zone, off)
		}
	}
	t, err := time.ParseInLocation("20060102150405", digits, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// parseNS reads xmltv_ns "1 . 4 . 0/1": zero-based season and episode.
func parseNS(s string) (season, episode *int) {
	p := strings.Split(s, ".")
	if len(p) < 2 {
		return nil, nil
	}
	first := func(v string) *int {
		v, _, _ = strings.Cut(strings.TrimSpace(v), "/")
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil
		}
		n++
		return &n
	}
	return first(p[0]), first(p[1])
}

// parseOnscreen reads S01E05 and the like.
func parseOnscreen(s string) (season, episode *int) {
	u := strings.ToUpper(strings.ReplaceAll(s, " ", ""))
	si, ei := strings.Index(u, "S"), strings.Index(u, "E")
	if si < 0 || ei <= si+1 {
		return nil, nil
	}
	a, err1 := strconv.Atoi(u[si+1 : ei])
	end := ei + 1
	for end < len(u) && u[end] >= '0' && u[end] <= '9' {
		end++
	}
	b, err2 := strconv.Atoi(u[ei+1 : end])
	if err1 != nil || err2 != nil {
		return nil, nil
	}
	return &a, &b
}
