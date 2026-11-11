// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package gamemeta

import (
	"bytes"
	"testing"
)

func FuzzGameParsers(f *testing.F) {
	f.Add([]byte("RARCHDB\x00"), "Some Game (USA) (Rev 1).sfc")
	f.Add([]byte("\x81\xa4name\xa1x"), "[BIOS] x.bin")
	f.Add([]byte("SEGA SEGASATURN "), "2001-01-01")
	f.Add(bytes.Repeat([]byte{0xff}, 4096), "9.5 / 10")
	f.Fuzz(func(t *testing.T, b []byte, s string) {
		_ = parseRDB(b, func(*rdbRecord) {})
		CleanName(s)
		cleanDate(s)
		parseReview(s)
		r := bytes.NewReader(b)
		playStationSerial(r)
		segaSerial(r, s)
		isoFile(r, s, 1<<20)
	})
}
