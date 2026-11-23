// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A request the viewer gave up on (the page closed while the server was still counting) is
// noted, not logged as a server error; one that failed while the viewer waited still is.
func TestAViewerLeavingIsNoError(t *testing.T) {
	ts, _ := logServer(t)
	ts.Logs.SetCategoryLevel("http", slog.LevelInfo) // by default it isn't written at all
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	left := httptest.NewRequest("GET", "/api/wittyinfo/convert/counts", nil).WithContext(ctx)
	ts.logRequest(left, "r-left", &statusWriter{code: 500, errBody: []byte(`{"error":"context canceled"}`)}, time.Now())
	stayed := httptest.NewRequest("GET", "/api/libraries", nil)
	ts.logRequest(stayed, "r-stayed", &statusWriter{code: 500, errBody: []byte(`{"error":"disk full"}`)}, time.Now())

	var leftLine, stayedLine string
	for _, l := range strings.Split(logFiles(t, ts), "\n") {
		switch {
		case strings.Contains(l, "r-left"):
			leftLine = l
		case strings.Contains(l, "r-stayed"):
			stayedLine = l
		}
	}
	if !strings.Contains(leftLine, "level=INFO") || !strings.Contains(leftLine, "left=true") {
		t.Errorf("a viewer who left: %q", leftLine)
	}
	if !strings.Contains(stayedLine, "level=ERROR") || strings.Contains(stayedLine, "left=") {
		t.Errorf("a real failure: %q", stayedLine)
	}
}
