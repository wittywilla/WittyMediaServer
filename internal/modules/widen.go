// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"regexp"

	"github.com/wittywilla/wittymodule"
)

// minorRange is a pre-1.0 limit such as "0.6.x"; the module API number decides real fit.
var minorRange = regexp.MustCompile(`^0\.\d+\.x$`)

func widen(max string) string {
	if minorRange.MatchString(max) {
		return "0.x"
	}
	return max
}

// supports checks a manifest, letting modules made for any 0.x server run on this one when the API fits.
func supports(man *wittymodule.Manifest, server string) error {
	c := *man
	c.MaxServer = widen(c.MaxServer)
	return c.Supports(server, wittymodule.MinAPI, wittymodule.API)
}
