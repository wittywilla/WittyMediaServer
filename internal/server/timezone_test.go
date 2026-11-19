// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// The saved zone is set before anything runs, and applying it again at start writes nothing:
// a write there raced the log's goroutine reading the clock.
//
// It writes time.Local, which no test may do while another test's timers read the clock, so it
// runs alone in a copy of the test binary.
func TestTimeZoneIsSetBeforeAnythingRuns(t *testing.T) {
	if os.Getenv("WMS_TEST_TIMEZONE_ALONE") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTimeZoneIsSetBeforeAnythingRuns$", "-test.count=1")
		cmd.Env = append(os.Environ(), "WMS_TEST_TIMEZONE_ALONE=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	was := time.Local
	dir := t.TempDir()
	EarlyTimeZone(dir) // no database yet: nothing changes
	if time.Local != was {
		t.Fatalf("no database moved the zone to %s", time.Local)
	}
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SetSetting(context.Background(), "timeZone", "Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}
	d.Close()
	EarlyTimeZone(dir)
	if time.Local.String() != "Asia/Tokyo" {
		t.Fatalf("zone %s", time.Local)
	}
	early := time.Local
	d, err = db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	(&Server{DB: d}).applyTimeZone(context.Background())
	if time.Local != early {
		t.Error("the zone was written again at start")
	}
	if err := d.SetSetting(context.Background(), "timeZone", ""); err != nil {
		t.Fatal(err)
	}
	(&Server{DB: d}).applyTimeZone(context.Background())
	if time.Local != computerLoc {
		t.Errorf("back to the computer's: %s", time.Local)
	}
}
