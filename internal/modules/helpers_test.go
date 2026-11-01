package modules

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

var builtPrograms sync.Map

// programBytes builds a package once for the whole test run.
func programBytes(t *testing.T, pkg string) []byte {
	t.Helper()
	if b, ok := builtPrograms.Load(pkg); ok {
		return b.([]byte)
	}
	dir, err := os.MkdirTemp("", "built-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "program")
	if b, err := exec.Command("go", "build", "-o", out, pkg).CombinedOutput(); err != nil {
		t.Fatalf("building %s: %v\n%s", pkg, err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	builtPrograms.Store(pkg, b)
	return b
}

// withInstalled makes a manager with one module whose version in use is a signed
// folder already on disk, as an install would leave it. Other files can go beside it.
func withInstalled(t *testing.T, w *world, builtin, raw, exe []byte, extra map[string][]byte) *Manager {
	t.Helper()
	man, err := wittymodule.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.Abs(filepath.Join(w.dir, man.ID, "versions", man.Version))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{wittymodule.ManifestFile: raw, man.EntryFile(runtime.GOOS): exe}
	for k, v := range extra {
		files[k] = v
	}
	top := man.Top(runtime.GOOS, runtime.GOARCH)
	var list strings.Builder
	for n, b := range files {
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o755); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&list, "%s  %s/%s\n", sha(b), top, n)
	}
	sums := []byte(list.String())
	if err := os.WriteFile(filepath.Join(dir, signedList), sums, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, signedSig), update.Sign(w.key, sums), 0o644); err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(map[string]moduleState{man.ID: {Active: man.Version}})
	if err := os.WriteFile(filepath.Join(w.dir, "state.json"), state, 0o644); err != nil {
		t.Fatal(err)
	}
	return New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{builtin}, Keys: []ed25519.PublicKey{w.pub},
		Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

// photoInfo is what the fake helper answers.
type photoInfo struct{ Make string }

// PhotoEngine reads photos through a version in use, as the real one does, for tests of a
// helper's life. Nothing here depends on what a photo is.
type PhotoEngine struct{ M *Manager }

func (p PhotoEngine) ReadFile(path string) (*photoInfo, error) {
	var in photoInfo
	_, err := p.M.Call("wittyphotos", "read", pathParams{path}, &in)
	if err != nil {
		return nil, err
	}
	return &in, nil
}

func (p PhotoEngine) Dimensions(path string) (w, h int) {
	var d struct{ W, H int }
	if _, err := p.M.Call("wittyphotos", "dimensions", pathParams{path}, &d); err == nil {
		return d.W, d.H
	}
	return 0, 0
}

func (p PhotoEngine) RawPreview(path string) ([]byte, error) {
	return p.M.Call("wittyphotos", "preview", pathParams{path}, nil)
}

func (p PhotoEngine) IsRaw(name string) bool {
	for _, e := range p.M.Capability("wittyphotos", "rawExtensions") {
		if strings.EqualFold(e, filepath.Ext(name)) {
			return true
		}
	}
	return false
}
