// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package i18n marks the English the server sends to people, so it can be translated where it's shown.
//
// The English is the key, as in the web app (web/src/i18n). The server sends English; the web app looks each text up
// in the language's catalogue (web/src/i18n/locales/<language>/server.json) and shows the translation. A text with
// something filled in is written with {names}, and the web app matches the filled-in text back to its key:
//
//	writeErr(w, 400, i18n.T("That invite has expired."))
//	s.Home = i18n.F("Devices at home: {where}", "where", where)
//
// scripts/i18n-server lists every key (into web/src/i18n/server-keys.json) and fails CI when a sentence a person may
// read isn't marked, or written where the scanner knows it reaches people (writeErr, Label and Desc fields and the
// like). Text no person reads (logs, file formats) needs nothing.
package i18n

import (
	"errors"
	"fmt"
	"strings"
)

// T marks English text that a person reads. It returns the text unchanged.
func T(english string) string { return english }

// F fills a message's {names} with values, given as name, value pairs: F("Hello, {name}", "name", n).
// Values are written with fmt's %v. A name with no value stays as it is.
func F(english string, pairs ...any) string {
	if len(pairs) < 2 {
		return english
	}
	var b strings.Builder
	b.Grow(len(english) + 16)
	rest := english
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			b.WriteString(rest)
			break
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			b.WriteString(rest)
			break
		}
		name := rest[i+1 : i+j]
		b.WriteString(rest[:i])
		if v, ok := value(pairs, name); ok {
			b.WriteString(v)
		} else {
			b.WriteString(rest[i : i+j+1])
		}
		rest = rest[i+j+1:]
	}
	return b.String()
}

func value(pairs []any, name string) (string, bool) {
	for k := 0; k+1 < len(pairs); k += 2 {
		if s, ok := pairs[k].(string); ok && s == name {
			switch v := pairs[k+1].(type) {
			case string:
				return v, true
			case error:
				return v.Error(), true
			default:
				return fmt.Sprint(v), true
			}
		}
	}
	return "", false
}

// Error is an error whose message a person reads: errors.New with the text marked, and {names} filled as in F.
func Error(english string, pairs ...any) error { return errors.New(F(english, pairs...)) }

// N is a message with a count in it: the English for one and for more, with {count} in each, as the web app's tn().
// The web app finds the language's form for the count: N("{count} album", "{count} albums", n).
func N(one, other string, count int, pairs ...any) string {
	all := append([]any{"count", count}, pairs...)
	if count == 1 {
		return F(one, all...)
	}
	return F(other, all...)
}
