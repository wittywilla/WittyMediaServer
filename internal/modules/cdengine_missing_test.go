// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/wittywilla/wittymodule"
)

// WittyCD's "missing" answer is a missing file here, with its words kept.
func TestACDsMissingFileIsNotExist(t *testing.T) {
	err := cdError(&wittymodule.CallError{Code: "missing", Message: `the file "a.wav" this image needs isn't in its folder`})
	if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "a.wav") {
		t.Errorf("%v", err)
	}
	if err := cdError(&wittymodule.CallError{Code: "", Message: "x"}); errors.Is(err, os.ErrNotExist) {
		t.Error("an uncoded error became a missing file")
	}
}
