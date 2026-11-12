// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// A canvas is decoration. A viewer away from home, on a server whose owner limits the data
// of a stream there, is told so with the canvas, so an app can leave it out; at home, or
// with no limit set, nothing is said.
func TestCanvasSaysWhenTheViewerIsLimited(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	put(t, filepath.Join(filepath.Dir(f.song.Path), "canvas.mp4"), testMP4)
	ask := func(addr string) canvasInfo {
		t.Helper()
		r := httptest.NewRequest("GET", f.url("/api/media/%d/canvas", f.song), nil)
		r.Host = "media.example.com"
		r.RemoteAddr = addr
		r.Header.Set("Authorization", "Bearer "+f.admin)
		w := httptest.NewRecorder()
		ts.h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", addr, w.Code, w.Body)
		}
		return decode[canvasInfo](t, w)
	}
	const home, away = "192.168.1.50:4000", "100.64.0.9:5000" // the second is a VPN: away from home
	if w := ts.do(t, "PUT", "/api/remote", f.admin, map[string]any{"enabled": true}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if c := ask(away); !c.Available || c.Limited {
		t.Errorf("away with no limit set: %+v", c)
	}
	if w := ts.do(t, "PUT", "/api/remote", f.admin, map[string]any{"enabled": true, "maxBitrate": "2M"}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if c := ask(away); !c.Available || !c.Limited {
		t.Errorf("away with a limit: %+v", c)
	}
	if c := ask(home); !c.Available || c.Limited {
		t.Errorf("at home: %+v", c)
	}
}
