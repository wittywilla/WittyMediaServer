package livetv

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPublicErrorKeepsAddressesAndFoldersOut(t *testing.T) {
	addr := "http://provider.invalid/live/USERNAME/PASSWORD/1.ts"
	for _, err := range []error{
		fmt.Errorf("the stream could not be reached: %w", &url.Error{Op: "Get", URL: addr, Err: errors.New("no such host")}),
		errors.New("a tool said: " + addr),
		fmt.Errorf("the recordings folder could not be used: %w", &fs.PathError{Op: "mkdir", Path: "/home/someone/recordings", Err: fs.ErrPermission}),
	} {
		got := PublicError(err)
		if strings.Contains(got, "USERNAME") || strings.Contains(got, "someone") {
			t.Errorf("%v: %q", err, got)
		}
	}
	if got := PublicError(errors.New("no tuner was free when the recording started")); !strings.Contains(got, "no tuner") {
		t.Errorf("a plain reason was lost: %q", got)
	}
}

// A recording stops rather than fill the disk the server's database may be on.
func TestRecordingStopsWhenTheDiskIsNearlyFull(t *testing.T) {
	old := recordingFreeFloor
	recordingFreeFloor = math.MaxInt64
	defer func() { recordingFreeFloor = old }()
	m := testManager(t)
	f := newLiveFake(t, nil)
	c := liveChannel(t, m, f)
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	ctx := context.Background()
	now := time.Now().Unix()
	rec, err := m.Schedule(ctx, c.ID, 1, "News", "", now, now+120)
	if err != nil {
		t.Fatal(err)
	}
	m.startDue(ctx)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := m.Recording(ctx, rec.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == "failed" {
			if got.Error != diskFullReason {
				t.Errorf("%q", got.Error)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("it never stopped")
}
