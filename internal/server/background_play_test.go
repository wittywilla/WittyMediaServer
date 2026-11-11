// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"testing"
)

// What a device that hasn't chosen does with "Keep playing when this tab is in the background":
// on to start with, the owner can change it for new devices, and every client reads it from the guide.
func TestBackgroundPlayStartsOnAndTheOwnerCanChangeIt(t *testing.T) {
	h, admin, kid, _ := tvServer(t)
	guideSays := func(token string) (bool, bool) {
		t.Helper()
		w := tvCall(h, "GET", "/api/guide?list=1", token, "")
		if w.Code != 200 {
			t.Fatalf("guide: %d %s", w.Code, w.Body)
		}
		var g map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
			t.Fatal(err)
		}
		v, ok := g["backgroundPlay"].(bool)
		return v, ok
	}
	for _, who := range []string{admin, kid} {
		if on, sent := guideSays(who); !sent || !on {
			t.Errorf("a new server: backgroundPlay %v, sent %v", on, sent)
		}
	}
	if w := tvCall(h, "GET", "/api/tv/settings", admin, ""); w.Code != 200 || !json.Valid(w.Body.Bytes()) || !containsKV(t, w.Body.Bytes(), "tvBackgroundPlay", "1") {
		t.Errorf("settings: %d %s", w.Code, w.Body)
	}
	for _, bad := range []string{"2", "yes", "true", ""} {
		if w := tvCall(h, "PUT", "/api/tv/settings", admin, `{"tvBackgroundPlay":"`+bad+`"}`); w.Code != 400 {
			t.Errorf("%q: %d", bad, w.Code)
		}
	}
	if w := tvCall(h, "PUT", "/api/tv/settings", kid, `{"tvBackgroundPlay":"0"}`); w.Code != 403 {
		t.Errorf("a Kids profile changed it: %d", w.Code)
	}
	if w := tvCall(h, "PUT", "/api/tv/settings", admin, `{"tvBackgroundPlay":"0"}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	for _, who := range []string{admin, kid} {
		if on, sent := guideSays(who); !sent || on {
			t.Errorf("after the owner turned it off: backgroundPlay %v, sent %v", on, sent)
		}
	}
	if w := tvCall(h, "PUT", "/api/tv/settings", admin, `{"tvBackgroundPlay":"1"}`); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if on, _ := guideSays(kid); !on {
		t.Error("turned on again, it still says off")
	}
}

func containsKV(t *testing.T, raw []byte, k, v string) bool {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m[k] == v
}
