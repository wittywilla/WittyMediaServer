package dvdremote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelperPathFindsVideoTSInAnyCase(t *testing.T) {
	dir := t.TempDir()
	mk := func(p string) string {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	upper := mk(filepath.Join(dir, "Upper"))
	mk(filepath.Join(upper, "VIDEO_TS"))
	lower := mk(filepath.Join(dir, "Lower"))
	ts := mk(filepath.Join(lower, "video_ts"))
	image := filepath.Join(dir, "disc.iso")
	if err := os.WriteFile(image, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := helperPath(upper); got != upper {
		t.Errorf("a disc with VIDEO_TS opens as itself, got %s", got)
	}
	// On a file system that doesn't mind case, the folder is found under either name.
	if got := helperPath(lower); got != lower && got != ts {
		t.Errorf("a disc with video_ts opens at %s", got)
	}
	if _, err := os.Stat(filepath.Join(lower, "VIDEO_TS")); err != nil && helperPath(lower) != ts {
		t.Errorf("on a file system that minds case, video_ts itself is opened")
	}
	if got := helperPath(ts); got != ts {
		t.Errorf("a VIDEO_TS folder opens as itself, got %s", got)
	}
	if got := helperPath(image); got != image {
		t.Errorf("an image opens as itself, got %s", got)
	}
}
