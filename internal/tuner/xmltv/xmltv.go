// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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
	// Details is the rest of what a guide says about a programme, for its More info.
	Details Details
}

// Details is what a guide says about a programme beyond its line in the grid. Each part is
// bounded, so a guide that says a great deal still costs little.
type Details struct {
	Year         int      `json:"year,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	Credits      []Credit `json:"credits,omitempty"`
	Stars        []Star   `json:"stars,omitempty"`
	Language     string   `json:"language,omitempty"`
	OrigLanguage string   `json:"origLanguage,omitempty"`
	Country      string   `json:"country,omitempty"`
	LengthMin    int      `json:"lengthMin,omitempty"`
	Subtitles    []string `json:"subtitles,omitempty"` // languages, or the kind when none is named
	SignLanguage bool     `json:"signLanguage,omitempty"`
	Audio        string   `json:"audio,omitempty"` // mono, stereo, dolby, dolby digital, bilingual, surround
	HD           bool     `json:"hd,omitempty"`
	Premiere     bool     `json:"premiere,omitempty"`
	New          bool     `json:"new,omitempty"`
	Repeat       bool     `json:"repeat,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
	IMDb         string   `json:"imdb,omitempty"` // tt…
	TMDb         string   `json:"tmdb,omitempty"` // movie/123 or tv/123
}

// Credit is one person a guide credits: Role is director, actor, writer, adapter, producer,
// composer, editor, presenter, commentator or guest.
type Credit struct {
	Role      string `json:"role"`
	Name      string `json:"name"`
	Character string `json:"character,omitempty"`
	Guest     bool   `json:"guest,omitempty"`
}

// Star is a score as the guide gives it, "8.1/10", with who gave it.
type Star struct {
	Value  string `json:"value"`
	System string `json:"system,omitempty"`
}

// How much of a programme's details are kept.
const (
	maxCredits   = 40
	maxListItems = 8
	maxDetail    = 120
)

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
	Credits *struct {
		People []struct {
			XMLName xml.Name
			Role    string `xml:"role,attr"`
			Guest   string `xml:"guest,attr"`
			Name    string `xml:",chardata"`
		} `xml:",any"`
	} `xml:"credits"`
	Stars []struct {
		System string `xml:"system,attr"`
		Value  string `xml:"value"`
	} `xml:"star-rating"`
	Language     []string `xml:"language"`
	OrigLanguage []string `xml:"orig-language"`
	Country      []string `xml:"country"`
	Length       []struct {
		Units string `xml:"units,attr"`
		Value string `xml:",chardata"`
	} `xml:"length"`
	Subtitles []struct {
		Type     string   `xml:"type,attr"`
		Language []string `xml:"language"`
	} `xml:"subtitles"`
	Audio []struct {
		Stereo string `xml:"stereo"`
	} `xml:"audio"`
	Video []struct {
		Quality string `xml:"quality"`
	} `xml:"video"`
	Premiere *struct{} `xml:"premiere"`
	New      *struct{} `xml:"new"`
	Repeat   *struct{} `xml:"previously-shown"`
	Keywords []string  `xml:"keyword"`
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
		switch e.System {
		case "xmltv_ns":
			out.Season, out.Episode = parseNS(e.Value)
		case "onscreen":
			if out.Season == nil {
				out.Season, out.Episode = parseOnscreen(e.Value)
			}
		case "imdb.com":
			out.Details.IMDb = imdbID(e.Value)
		case "themoviedb.org":
			out.Details.TMDb = tmdbID(e.Value)
		}
	}
	out.Details = details(p, out.Details)
	out.Details.Year = out.Year
	return out, true
}

// details reads the rest of a programme, keeping each part short.
func details(p xmlProgramme, d Details) Details {
	for i, c := range p.Category {
		if c = clip(c); i > 0 && c != "" && len(d.Categories) < maxListItems {
			d.Categories = append(d.Categories, c)
		}
	}
	if p.Credits != nil {
		for _, c := range p.Credits.People {
			name := clip(c.Name)
			role := c.XMLName.Local
			if name == "" || len(d.Credits) >= maxCredits || !creditRoles[role] {
				continue
			}
			d.Credits = append(d.Credits, Credit{Role: role, Name: name, Character: clip(c.Role), Guest: c.Guest == "yes" || role == "guest"})
		}
	}
	for _, st := range p.Stars {
		if v := clip(st.Value); v != "" && len(d.Stars) < maxListItems {
			d.Stars = append(d.Stars, Star{Value: v, System: clip(st.System)})
		}
	}
	d.Language, d.OrigLanguage, d.Country = first(p.Language), first(p.OrigLanguage), first(p.Country)
	if len(p.Length) > 0 {
		n, _ := strconv.Atoi(strings.TrimSpace(p.Length[0].Value))
		switch strings.TrimSpace(p.Length[0].Units) {
		case "hours":
			n *= 60
		case "seconds":
			n /= 60
		}
		if n > 0 && n < 24*60 {
			d.LengthMin = n
		}
	}
	for _, sub := range p.Subtitles {
		if sub.Type == "deaf-signed" {
			d.SignLanguage = true
			continue
		}
		langs := sub.Language
		if len(langs) == 0 {
			langs = []string{sub.Type}
		}
		for _, l := range langs {
			if l = clip(l); l != "" && len(d.Subtitles) < maxListItems {
				d.Subtitles = append(d.Subtitles, l)
			}
		}
	}
	if len(p.Audio) > 0 {
		d.Audio = clip(p.Audio[0].Stereo)
	}
	for _, v := range p.Video {
		if q := strings.ToUpper(strings.TrimSpace(v.Quality)); q == "HDTV" || q == "UHD" || strings.Contains(q, "1080") || strings.Contains(q, "720") || strings.Contains(q, "2160") {
			d.HD = true
		}
	}
	d.Premiere, d.New, d.Repeat = p.Premiere != nil, p.New != nil, p.Repeat != nil
	for _, k := range p.Keywords {
		if k = clip(k); k != "" && len(d.Keywords) < maxListItems {
			d.Keywords = append(d.Keywords, k)
		}
	}
	return d
}

var creditRoles = map[string]bool{"director": true, "actor": true, "writer": true, "adapter": true, "producer": true,
	"composer": true, "editor": true, "presenter": true, "commentator": true, "guest": true}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return clip(v[0])
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxDetail {
		s = string(r[:maxDetail])
	}
	return s
}

// imdbID keeps an IMDb title id ("tt0123456"), from an id or an address.
func imdbID(v string) string {
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == '/' || r == ' ' }) {
		if len(part) > 2 && len(part) < 16 && strings.HasPrefix(part, "tt") && strings.Trim(part[2:], "0123456789") == "" {
			return part
		}
	}
	return ""
}

// tmdbID keeps "movie/123" or "tv/123", as TMDB names a title.
func tmdbID(v string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(v), "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		kind, id := parts[i], parts[i+1]
		if (kind == "movie" || kind == "tv") && id != "" && len(id) < 12 && strings.Trim(id, "0123456789") == "" {
			return kind + "/" + id
		}
	}
	return ""
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
