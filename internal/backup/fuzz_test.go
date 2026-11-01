package backup

import (
	"archive/zip"
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A payload's own numbers are read before anything about it is proven.
func FuzzOpenPayload(f *testing.F) {
	h := &CryptHeader{Alg: "AES-256-GCM", KDF: "argon2id", Time: 1, MemoryKiB: 8 << 10, Threads: 1, Salt: "c2FsdHNhbHQ=", Prefix: "AAAAAAAAAA==", Chunk: chunkSize}
	f.Add([]byte{0, 0, 0, 16}, "correct horse")
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF}, "x")
	f.Add(bytes.Repeat([]byte{0}, 64), "")
	f.Fuzz(func(t *testing.T, b []byte, password string) {
		if len(b) > 1<<16 {
			return
		}
		start := time.Now()
		_ = openPayload(io.Discard, bytes.NewReader(b), h, password, 1<<20, nil)
		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("%d bytes took %v", len(b), took)
		}
	})
}

// The first thing read from anyone's archive, listed or checked.
func FuzzManifest(f *testing.F) {
	f.Add([]byte(`{"format":1,"app":"WittyMediaServer","parts":[],"files":[{"path":"a","size":1}]}`))
	f.Add([]byte(`{"format":99,"app":"WittyMediaServer","serverVersion":"<b>"}`))
	f.Add([]byte(`{"crypt":{"alg":"AES-256-GCM","time":4294967295}}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 1<<16 {
			return
		}
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(ManifestName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
		if err != nil {
			t.Fatal(err)
		}
		m, err := readManifest(zr.File[0], maxManifest)
		if err != nil {
			return
		}
		if m.Crypt != nil {
			_ = m.Crypt.check()
		}
		if s := cleanWords(m.ServerVersion); strings.Contains(s, "<b>") {
			t.Errorf("markup in a message: %q", s)
		}
	})
}

// A name in an archive that passes has to stay inside its folder, on any system.
func FuzzNames(f *testing.F) {
	for _, s := range []string{"artwork/a.jpg", "../x", "a/../../x", `a\..\x`, "C:/x", "/etc/passwd", "a/./b", "CON.txt", "a ", "a.", "\xff", "a\u202eb"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if unsafeName(name) != "" {
			return
		}
		root := t.TempDir()
		p, err := safeTarget(root, "artwork", name)
		if err != nil {
			return
		}
		rel, err := filepath.Rel(filepath.Join(root, "artwork"), p)
		if err != nil || !filepath.IsLocal(rel) {
			t.Errorf("%q came to %s", name, p)
		}
		if _, n := sourceFor(name); strings.Contains(n, "..") && strings.Contains(strings.ReplaceAll(n, "\\", "/"), "../") {
			t.Errorf("%q leaves its folder", name)
		}
	})
}
