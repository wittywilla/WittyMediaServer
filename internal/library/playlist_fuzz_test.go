// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import "testing"

// FuzzParseEntries: a playlist file from a library folder never makes the reader panic, and
// never yields more entries than the limit.
func FuzzParseEntries(f *testing.F) {
	for _, s := range []string{
		"#EXTM3U\n#EXTINF:1,a\n../a.mp3\n", "[playlist]\nFile1=a.mp3\nNumberOfEntries=1\n",
		`<?xml version="1.0"?><playlist><trackList><track><location>file:///a.mp3</location></track></trackList></playlist>`,
		`<asx version="3.0"><entry><ref href="a.mp3"/></entry></asx>`,
		`<!DOCTYPE a [<!ENTITY x "xxxxxxxx"><!ENTITY y "&x;&x;&x;&x;">]><playlist>&y;</playlist>`,
		"\xff\xfe#\x00E\x00", "File99999999999999999999=a",
	} {
		for _, format := range []string{"m3u", "pls", "xspf", "asx"} {
			f.Add(format, []byte(s))
		}
	}
	f.Fuzz(func(t *testing.T, format string, b []byte) {
		if got := ParseEntries(format, b); len(got) > maxPlaylistEntries {
			t.Fatalf("%d entries", len(got))
		}
	})
}
