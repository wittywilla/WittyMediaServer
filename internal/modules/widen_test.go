// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"testing"

	"github.com/wittywilla/wittymodule"
)

// A pre-1.0 range such as 0.6.x lets a module run on any later 0.x server whose module
// API it fits: the API number decides, not the minor version it was released beside.
func TestAPreOneRangeFitsLaterServers(t *testing.T) {
	cases := []struct {
		max, server string
		api         int
		fits        bool
	}{
		{"0.6.x", "0.8.0-alpha", 3, true},
		{"0.6.x", "0.8.0-alpha", 4, true},
		{"0.6.x", "0.8.0", 3, true},
		{"0.8.x", "0.8.0-alpha", 4, true},
		{"0.6.x", "0.8.0-alpha", 2, false}, // too old an API
		{"0.6.x", "0.8.0-alpha", 5, false}, // too new an API
		{"0.6.3", "0.8.0-alpha", 3, false}, // an exact top is kept
		{"0.6.x", "1.0.0", 3, false},       // a 1.x server is another line
	}
	for _, c := range cases {
		man := &wittymodule.Manifest{ID: "wittyx", Name: "X", Version: "0.1.0", Repo: "a/b", Delivery: wittymodule.DeliveryHelper, Entry: "x", API: c.api, MinServer: "0.6.0-alpha", MaxServer: c.max}
		err := supports(man, c.server)
		if (err == nil) != c.fits {
			t.Errorf("maxServer %s, API %d, on %s: %v", c.max, c.api, c.server, err)
		}
		v := wittymodule.CatalogVersion{Version: "0.1.0", Delivery: wittymodule.DeliveryHelper, API: c.api, MinServer: "0.6.0-alpha", MaxServer: c.max}
		// The entry names no files, so a version that fits stops at the next check: that
		// there is a build for this computer.
		f := fitOf("X", &v, c.server)
		if (f.Kind == FitOK || f.Kind == FitNoBuild) != c.fits && c.server != "1.0.0" {
			t.Errorf("store: maxServer %s, API %d, on %s: %+v", c.max, c.api, c.server, f)
		}
	}
}
