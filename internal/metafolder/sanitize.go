// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package metafolder is the format of the editable metadata folder, and the checks on
// anything read back from a file someone can edit: the folder itself, and the NFO and
// JSON files beside media. It holds no server state and knows nothing of the database,
// so the server, the command line and the repair tools all read files the same way.
//
// Everything here treats a file as hostile. A value that leaves this package is plain
// text of a bounded length, a web address, or the name of a file with nothing in its
// way to anywhere else.
package metafolder

import (
	"html"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits on URLs and names, the same as the fields they end up in.
const (
	maxURL  = 2000
	maxName = 120
)

// hiddenRune says whether a character changes how text looks or reads without being
// seen: controls, text-direction overrides and isolates, the invisible tag characters
// and byte order marks. Marks a script needs to join letters stay.
func hiddenRune(r rune) bool {
	switch {
	case r == '\n' || r == '\t' || r == '\r':
		return false
	case unicode.IsControl(r):
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	case r == 0xFEFF, r == 0xFFFE, r == 0xFFFF, r == 0xFFFD:
		return true
	case r >= 0xE0000 && r <= 0xE0FFF:
		return true
	}
	return false
}

func dropHidden(s string) string {
	s = strings.ToValidUTF8(s, "")
	return strings.Map(func(r rune) rune {
		if r == 0x2028 || r == 0x2029 {
			return '\n'
		}
		if hiddenRune(r) {
			return -1
		}
		return r
	}, s)
}

// Elements whose content is code or isn't text: all of it goes, not only the tags.
var dropContent = map[string]bool{"script": true, "style": true, "iframe": true, "object": true, "embed": true, "noscript": true, "template": true, "svg": true, "math": true, "xmp": true, "noembed": true, "noframes": true, "applet": true, "frameset": true}

// Tags that end a line, where a plot with some markup still reads as paragraphs.
var breakTag = map[string]bool{"br": true, "p": true, "div": true, "li": true, "tr": true}

func isTagStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '/' || c == '!' || c == '?'
}

// tagEnd finds the '>' closing a tag that opened at s[0], stepping over quoted
// attribute values, which can hold one. -1 if it never closes. A quote that never
// closes doesn't hide the rest of the text: the first '>' then ends it.
func tagEnd(s string) int {
	var quote byte
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '>':
			return i
		}
	}
	return strings.IndexByte(s, '>')
}

func tagName(s string) string {
	i := 1
	if i < len(s) && (s[i] == '/' || s[i] == '!' || s[i] == '?') {
		i++
	}
	j := i
	for j < len(s) {
		c := s[j]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			j++
			continue
		}
		break
	}
	return strings.ToLower(s[i:j])
}

// stripTags turns markup into text. "<3" and "a < b" aren't tags, and stay.
func stripTags(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		s = s[i:]
		if len(s) < 2 || !isTagStart(s[1]) {
			b.WriteByte('<')
			s = s[1:]
			continue
		}
		if strings.HasPrefix(s, "<!--") {
			if j := strings.Index(s, "-->"); j >= 0 {
				s = s[j+3:]
			} else {
				s = ""
			}
			continue
		}
		name := tagName(s)
		end := tagEnd(s)
		if end < 0 {
			// A tag that never closes. A browser wouldn't run it, but it isn't text
			// anyone meant either, if it carries attributes or names something dangerous.
			if dropContent[name] || strings.Contains(s, "=") {
				break
			}
			b.WriteByte('<')
			s = s[1:]
			continue
		}
		closing := s[1] == '/'
		rest := s[end+1:]
		if dropContent[name] && !closing {
			// Up to its own closing tag, or to the end if it has none.
			low := strings.ToLower(rest)
			if j := strings.Index(low, "</"+name); j >= 0 {
				rest = rest[j:]
				if k := strings.IndexByte(rest, '>'); k >= 0 {
					rest = rest[k+1:]
				} else {
					rest = ""
				}
			} else {
				rest = ""
			}
		} else if breakTag[name] {
			b.WriteByte('\n')
		}
		s = rest
	}
	return b.String()
}

// noMarkup takes markup out, then entities, then whatever they turned out to hide:
// "&lt;script&gt;" is the same thing one step further on.
func noMarkup(s string) string {
	for pass := 0; pass < 3; pass++ {
		next := stripTags(s)
		if strings.IndexByte(next, '&') >= 0 && strings.IndexByte(next, ';') >= 0 {
			next = html.UnescapeString(next)
		}
		if next == s {
			break
		}
		s = next
	}
	return stripTags(s)
}

func clipRunes(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// Text cleans text that may run over several lines, such as a plot: no markup, no
// hidden characters, line breaks as "\n", at most max characters.
func Text(s string, max int) string {
	s = dropHidden(s)
	s = noMarkup(s)
	s = dropHidden(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\t", " ")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(clipRunes(strings.TrimSpace(s), max))
}

// Line cleans text that has to stay on one line, such as a title.
func Line(s string, max int) string {
	s = Text(s, 0)
	if strings.ContainsAny(s, "\n") {
		s = strings.Join(strings.Fields(s), " ")
	}
	return strings.TrimSpace(clipRunes(s, max))
}

// List cleans a list: no empties, no repeats whatever the case, at most maxItems of at
// most maxLen characters. "|" separates a list where it's stored, so it can't be in one.
func List(items []string, maxItems, maxLen int) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, v := range items {
		v = strings.ReplaceAll(Line(v, maxLen), "|", "/")
		k := strings.ToLower(v)
		if v == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
		if maxItems > 0 && len(out) == maxItems {
			break
		}
	}
	return out
}

// URL accepts only a web address: http or https, with a host and no login in it. Not
// javascript:, data:, file: or anything else that's not somewhere on the web.
func URL(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxURL {
		return "", false
	}
	for _, r := range s {
		if r <= ' ' || r == '\\' || hiddenRune(r) || r == 0x7F {
			return "", false
		}
	}
	u, err := url.Parse(s)
	if err != nil || u.Opaque != "" || u.User != nil || u.Host == "" {
		return "", false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return s, true
	}
	return "", false
}

// FileName accepts a plain file name, the only thing a picture reference to a file may
// be: nothing that can lead out of its folder, by any spelling on any system.
func FileName(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 255 || s == "." || s == ".." || strings.HasPrefix(s, ".") {
		return "", false
	}
	for _, r := range s {
		switch {
		case r == '/' || r == '\\' || r == ':' || r < ' ' || hiddenRune(r):
			return "", false
		case strings.ContainsRune(`<>"|?*`, r):
			return "", false
		}
	}
	if s != strings.TrimRight(s, ". ") || reservedName(s) {
		return "", false
	}
	return s, true
}

// reservedName: names Windows keeps for devices, with any extension.
func reservedName(s string) bool {
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	switch u := strings.ToUpper(strings.TrimSpace(s)); u {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	default:
		if len(u) == 4 && (strings.HasPrefix(u, "COM") || strings.HasPrefix(u, "LPT")) && u[3] >= '0' && u[3] <= '9' {
			return true
		}
	}
	return false
}

// SafeName makes a name for a file or folder from a title, legal on every system: no
// separators, nothing Windows refuses, no trailing dots, not too long. It's for reading
// only, as the id in the name is what counts.
func SafeName(s string) string {
	s = Line(s, 0)
	s = strings.Map(func(r rune) rune {
		switch {
		case strings.ContainsRune(`<>:"/\|?*`, r):
			return ' '
		case r < ' ' || hiddenRune(r):
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimLeft(s, ".- ")
	s = clipBytes(s, maxName)
	s = strings.TrimRight(s, ". ")
	if s == "" {
		return "Untitled"
	}
	if reservedName(s) {
		return "_" + s
	}
	return s
}

// clipBytes cuts to at most n bytes without splitting a character: a file name's limit
// is in bytes on most systems.
func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
