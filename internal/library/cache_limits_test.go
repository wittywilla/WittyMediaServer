package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Thumbnails go when the cache is over its limit, the oldest folders first, and not more than once
// a day however many scans there are.
func TestCachesKeepToTheirLimits(t *testing.T) {
	was := maxThumbCache
	maxThumbCache = 3000
	defer func() { maxThumbCache = was }()
	dir := t.TempDir()
	s := &Scanner{CacheDir: dir}
	thumb := func(bucket string, age time.Duration) {
		t.Helper()
		d := filepath.Join(dir, "thumbs", bucket)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "x.jpg"), make([]byte, 1000), 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(-age)
		if err := os.Chtimes(d, at, at); err != nil {
			t.Fatal(err)
		}
	}
	there := func(bucket string) bool {
		_, err := os.Stat(filepath.Join(dir, "thumbs", bucket, "x.jpg"))
		return err == nil
	}
	thumb("aa", 3*time.Hour)
	thumb("bb", 2*time.Hour)
	thumb("cc", time.Hour)
	thumb("dd", 0)
	s.trimCaches()
	if there("aa") || !there("bb") || !there("cc") || !there("dd") {
		t.Fatalf("after trimming: %v %v %v %v", there("aa"), there("bb"), there("cc"), there("dd"))
	}
	thumb("aa", 0)
	s.trimCaches()
	if !there("aa") || !there("bb") {
		t.Fatal("trimmed twice in a day")
	}
}
