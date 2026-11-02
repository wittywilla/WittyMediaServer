package libtranscode

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// A run ending cancels the read: that once stopped a good conversion for the default 10 GB limit,
// on a server asked to keep 1 GB.
func TestSettingsNowKeepsThemWhenTheReadFails(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx := context.Background()
	set := DefaultSettings()
	set.MinFreeGB = 1
	if err := SaveSettings(ctx, d, set); err != nil {
		t.Fatal(err)
	}
	gone, cancel := context.WithCancel(ctx)
	cancel()
	if got := settingsNow(gone, d, set).MinFreeGB; got != 1 {
		t.Fatalf("a failed read gave a limit of %d GB", got)
	}
	set.MinFreeGB = 2
	if err := SaveSettings(ctx, d, set); err != nil {
		t.Fatal(err)
	}
	if got := settingsNow(ctx, d, DefaultSettings()).MinFreeGB; got != 2 {
		t.Fatalf("a good read gave %d GB", got)
	}
}
