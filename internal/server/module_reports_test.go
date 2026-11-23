// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"strings"
	"testing"
	"time"
)

// One browser's reports about a version count once an hour; another browser's, or another
// version's, count on their own.
func TestOneBrowsersReportsCountOnceAnHour(t *testing.T) {
	var g moduleReportGate
	now := time.Now()
	if !g.counts("session-a", "wittystats", "0.3.1", now) {
		t.Fatal("the first report should count")
	}
	for _, later := range []time.Duration{time.Second, time.Minute, 59 * time.Minute} {
		if g.counts("session-a", "wittystats", "0.3.1", now.Add(later)) {
			t.Errorf("the same browser again after %v counted", later)
		}
	}
	if !g.counts("session-b", "wittystats", "0.3.1", now.Add(time.Minute)) {
		t.Error("another browser's report should count")
	}
	if !g.counts("session-a", "wittystats", "0.3.2", now.Add(time.Minute)) {
		t.Error("another version's report should count")
	}
	if !g.counts("session-a", "wittystats", "0.3.1", now.Add(time.Hour)) {
		t.Error("an hour on, the same browser's report should count again")
	}
	for k := range g.last {
		if strings.Contains(k, "session-a") {
			t.Errorf("the sign-in is kept as it is: %q", k)
		}
	}
}

// An administrator's browser reporting the same failure over and over (a page opened again and
// again) is one report, not three, and the rest are noted without counting.
func TestRepeatedReportsFromOneBrowserAreNoted(t *testing.T) {
	ts, _ := logServer(t)
	ts.Logs.SetCategoryLevel("modules", 0)
	ts.Modules = installedBundle(t, `export default 1`, `a{}`)
	_, tok := ts.user(t, "boss", "adult", true, "")
	for range 3 {
		if w := ts.do(t, "POST", "/api/modules/wittycomics/failed", tok, map[string]string{"version": "0.2.0", "reason": "x"}); w.Code != 204 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
	}
	logs := logFiles(t, ts)
	if n := strings.Count(logs, `msg="a browser reported a module failing"`); n != 1 {
		t.Errorf("counted %d times, want 1:\n%s", n, logs)
	}
	if n := strings.Count(logs, "reported a module failing again"); n != 2 {
		t.Errorf("noted again %d times, want 2", n)
	}
}
