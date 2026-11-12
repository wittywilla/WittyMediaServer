// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	builtin "github.com/wittywilla/wittymediaserver/themes"
)

// What the theme costs a page, on a small machine. A measurement, not a check: it runs only when
// asked, and prints what it found.
//
//	WMS_MEASURE=1 GOMAXPROCS=1 go test ./internal/server -run TestThemeCostPerPage -v
//
// GOMAXPROCS=1 stands in for a single-core board; it says nothing about a slow disk, where the
// stat each request makes per theme zip (internal/themes/memo.go) would count for more.
func TestThemeCostPerPage(t *testing.T) {
	if os.Getenv("WMS_MEASURE") != "1" {
		t.Skip("a measurement: WMS_MEASURE=1 to run it")
	}
	ts, admin, adult, _ := sduiAccounts(t)
	if err := ts.Themes.Install(builtin.Builtin); err != nil {
		t.Fatal(err)
	}
	// The heaviest ordinary case: a server theme with an override, which brings derived colours.
	if w := ts.do(t, "PUT", "/api/theme/global", admin, map[string]any{"id": "midnight", "tokens": map[string]string{"accent": "#ff8800"}}); w.Code != 200 {
		t.Fatalf("global: %d %s", w.Code, w.Body)
	}
	etag := ts.do(t, "GET", "/api/v1/theme", adult, nil).Header().Get("ETag")
	measure := func(name, target string, hdr ...string) {
		t.Helper()
		if w := ts.do(t, "GET", target, adult, nil, hdr...); w.Code != 200 && w.Code != 304 {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body)
		}
		r := testing.Benchmark(func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				req := httptest.NewRequest("GET", target, nil)
				req.Host, req.RemoteAddr = "127.0.0.1:9797", "127.0.0.1:50000"
				req.Header.Set("Authorization", "Bearer "+adult)
				for i := 0; i+1 < len(hdr); i += 2 {
					req.Header.Set(hdr[i], hdr[i+1])
				}
				ts.h.ServeHTTP(httptest.NewRecorder(), req)
			}
		})
		t.Logf("%-34s %9v/op %8d B/op %6d allocs/op", name, time.Duration(r.NsPerOp()), r.AllocedBytesPerOp(), r.AllocsPerOp())
	}
	t.Logf("GOMAXPROCS=%d, %s/%s", runtime.GOMAXPROCS(0), runtime.GOOS, runtime.GOARCH)
	measure("the resolved theme", "/api/v1/theme")
	measure("the resolved theme, unchanged", "/api/v1/theme", "If-None-Match", etag)
	measure("a theme's tokens", "/api/v1/themes/midnight/tokens.json")
	measure("Home's page description", "/api/v1/sdui/pages/home")
	measure("the menu", "/api/v1/sdui/navigation")
}
