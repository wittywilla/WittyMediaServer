// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package themes

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"
)

// A theme uses its own files and nothing else. The page's policy has to let images come
// from the internet, for artwork, so a stylesheet could load one from anywhere. That's
// enough to read a page: a rule for each character a sign-in form's value might start
// with, each with its own address, tells that site a password as it's typed.
//
// The check is on the text with its escapes undone, not on a parse of it. Two readers of
// CSS that disagree about where a comment or a string ends are how this kind of check gets
// walked around, so an address counts wherever it appears, comments included.

// An image kept inside a stylesheet can have any of these in its base64, and it reaches
// nothing. The alphabet has no quote or bracket, so no address can begin inside the run.
var cssBase64 = regexp.MustCompile(`;base64,[a-z0-9+/=]+`)

// outsideMarks are what an address on another site starts with. The backslashes are
// there because browsers read one as a slash in a web address.
var outsideMarks = []string{"http:", "https:", "//", `/\`, `\/`, `\\`}

func outsideURL(css []byte) bool {
	t := bytes.ToLower(cssUnescaped(css))
	t = cssBase64.ReplaceAll(t, nil)
	// The name every SVG and namespace declaration has to have. Nothing is fetched from it.
	t = bytes.ReplaceAll(t, []byte("http://www.w3.org/"), nil)
	t = bytes.ReplaceAll(t, []byte("*//*"), nil) // two comments side by side
	for _, m := range outsideMarks {
		if bytes.Contains(t, []byte(m)) {
			return true
		}
	}
	return false
}

// cssUnescaped undoes CSS's escapes, as a browser will before it uses the text, and takes
// out what a browser drops from a web address (a tab or a line break, however written).
func cssUnescaped(b []byte) []byte {
	out := make([]byte, 0, len(b))
	dropped := func(c rune) bool { return c == '\t' || c == '\n' || c == '\r' || c == '\f' }
	for i := 0; i < len(b); {
		c := b[i]
		i++
		if c != '\\' {
			if !dropped(rune(c)) {
				out = append(out, c)
			}
			continue
		}
		if i >= len(b) {
			break
		}
		n, v := 0, rune(0)
		for i+n < len(b) && n < 6 && isHex(b[i+n]) {
			v = v<<4 | hexValue(b[i+n])
			n++
		}
		if n == 0 { // the next character is itself, and a line break is nothing at all
			if !dropped(rune(b[i])) {
				out = append(out, b[i])
			}
			i++
			continue
		}
		i += n
		if i < len(b) && (b[i] == ' ' || dropped(rune(b[i]))) { // one space ends the number
			if b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n' {
				i++
			}
			i++
		}
		if v == 0 || !utf8.ValidRune(v) {
			v = utf8.RuneError
		}
		if !dropped(v) {
			out = utf8.AppendRune(out, v)
		}
	}
	return out
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func hexValue(c byte) rune {
	switch {
	case c >= 'a':
		return rune(c-'a') + 10
	case c >= 'A':
		return rune(c-'A') + 10
	}
	return rune(c - '0')
}

func outsideErr(name string) error {
	return fmt.Errorf("%s names an address on another site, perhaps in a comment. A theme uses its own files so that nobody's browser contacts anyone else: put the image or font in the ZIP and take the address out", name)
}

// outsideColours: theme.json's colours become properties on the page that lists the themes.
func outsideColours(meta *Meta) error {
	vals := []string{meta.Preview.BG, meta.Preview.Surface, meta.Preview.Text, meta.Preview.Accent}
	for _, v := range meta.Colors {
		vals = append(vals, v)
	}
	for _, v := range vals {
		if outsideURL([]byte(v)) {
			return errors.New("the colours in theme.json have to be colours, not addresses")
		}
	}
	return nil
}
