// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"strings"
	"testing"
)

func TestValidNavDefault(t *testing.T) {
	good := []string{
		`{}`,
		`{"order":["home","lib:3"],"hidden":["tv"],"grouped":true,"groups":{"lib:3":"Listen"},"overflow":"more","collapse":"rail","style":"top"}`,
	}
	for _, v := range good {
		if !validNavDefault(v) {
			t.Errorf("should pass: %s", v)
		}
	}
	bad := []string{
		`not json`,
		`[]`,
		`{"order":"home"}`,
		`{"style":"sideways"}`,
		`{"overflow":"hide"}`,
		`{"collapse":"away"}`,
		`{"somethingElse":1}`,
		`{"order":["` + strings.Repeat("x", 65) + `"]}`,
		`{"order":["` + strings.Repeat("x", 9000) + `"]}`,
		`{} {}`,
	}
	for _, v := range bad {
		if validNavDefault(v) {
			t.Errorf("should fail: %.60s", v)
		}
	}
}
