// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"testing"
)

// An app saves its interface choices by writing back the whole document it read. What the
// server keeps there through routes of its own (a layout, a page arrangement, a profile's
// own colours) survives that, and can't be set this way.
func TestPutUIKeepsWhatTheServerOwns(t *testing.T) {
	ts, _, adult, _ := sduiAccounts(t)
	if w := ts.do(t, "PUT", "/api/v1/sdui/experience?formFactor=phone", adult, map[string]string{"layout": "compactList"}); w.Code != 200 {
		t.Fatalf("layout: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/v1/theme/personal", adult, map[string]any{"tokens": map[string]string{"accent": "#2E7D32"}}); w.Code != 200 {
		t.Fatalf("own colours: %d %s", w.Code, w.Body)
	}
	// An app that read the document before those were saved writes back what it had, and
	// tries to set the server's keys itself.
	stale := map[string]any{"activityHidden": true, "sdui": map[string]any{"layouts": map[string]string{"form:phone": "streaming"}}, "themePersonal": map[string]any{}}
	if w := ts.do(t, "PUT", "/api/me/ui", adult, stale); w.Code != 200 {
		t.Fatalf("PUT /me/ui: %d %s", w.Code, w.Body)
	}
	var doc struct {
		ActivityHidden bool `json:"activityHidden"`
		SDUI           struct {
			Experiences map[string]string `json:"experiences"`
		} `json:"sdui"`
		ThemePersonal struct {
			Tokens map[string]string `json:"tokens"`
		} `json:"themePersonal"`
	}
	_ = json.Unmarshal(ts.do(t, "GET", "/api/me/ui", adult, nil).Body.Bytes(), &doc)
	if !doc.ActivityHidden {
		t.Error("the app's own choice wasn't saved")
	}
	if doc.SDUI.Experiences["form:phone"] != "compactList" {
		t.Errorf("the layout: %+v", doc.SDUI.Experiences)
	}
	if doc.ThemePersonal.Tokens["accent"] == "" {
		t.Errorf("the profile's own colours were lost: %+v", doc.ThemePersonal)
	}
}
