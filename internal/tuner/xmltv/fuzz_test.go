// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package xmltv

import (
	"bytes"
	"testing"
)

// FuzzParse: a guide from anywhere is read or refused, never a panic.
func FuzzParse(f *testing.F) {
	f.Add([]byte(`<tv><channel id="a"><display-name>A</display-name></channel><programme start="20260101000000 +0000" stop="20260101010000 +0000" channel="a"><title>T</title></programme></tv>`))
	f.Add([]byte(`<!DOCTYPE tv [<!ENTITY x "xxxxxxxx"><!ENTITY y "&x;&x;&x;">]><tv>&y;</tv>`))
	f.Add([]byte("\x1f\x8b\x08\x00\x00\x00\x00\x00"))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = Parse(bytes.NewReader(b), func(Channel) bool { return true }, func(Programme) bool { return true })
	})
}
