// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestCouchFailureTakesAsLongWhateverHappened(t *testing.T) {
	old := couchAnswerWait
	couchAnswerWait = 300 * time.Millisecond
	defer func() { couchAnswerWait = old }()
	ts := newTestServer(t)
	admin, _ := ts.user(t, "boss", "adult", true, "")
	adult, _ := ts.user(t, "bob", "adult", false, "")
	r := httptest.NewRequest("GET", "/", nil)
	start := time.Now()
	padCouchFailure(r.Context(), adult, start)
	if d := time.Since(start); d < 250*time.Millisecond {
		t.Errorf("an adult's answer came after %v", d)
	}
	start = time.Now()
	padCouchFailure(r.Context(), admin, start)
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("an administrator waited %v", d)
	}
}
