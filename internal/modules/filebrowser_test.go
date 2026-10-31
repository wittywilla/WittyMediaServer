package modules

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	fb "github.com/wittywilla/wittyintegratedfilebrowser"
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

func browserManifest(ver string, test ...string) []byte {
	m := wittymodule.Manifest{ID: fileBrowserID, Name: "WittyIntegratedFileBrowser", Version: ver, Repo: "wittywilla/WittyIntegratedFileBrowser",
		Delivery: wittymodule.DeliveryHelper, Entry: "wittyintegratedfilebrowser-module", API: 2, MinServer: "0.1.0", MaxServer: "0.x"}
	if len(test) > 0 {
		m.Capabilities = map[string][]string{"test": test}
	}
	b, _ := json.Marshal(m)
	return b
}

func folderWithFiles(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"film.mkv", "song.flac", "photo.CR3", "notes.txt", ".hidden.mkv"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "Disc", "VIDEO_TS"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Different from the built-in's, as the server's own list of kinds is.
func testKind(name string) string {
	if strings.HasSuffix(name, ".CR3") {
		return "image"
	}
	return fb.FileKind(name)
}

// The real helper answers just as the built-in copy does, on every point a page sees.
func TestFileBrowserHelperMatchesBuiltin(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "modules", "wittyintegratedfilebrowser", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	real, err := wittymodule.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	old := bytes.Replace(raw, []byte(`"version": "`+real.Version+`"`), []byte(`"version": "0.0.1"`), 1)
	w := newWorld(t)
	w.server = real.MinServer
	m := withInstalled(t, w, old, raw, programBytes(t, "github.com/wittywilla/wittyintegratedfilebrowser/cmd/wittyintegratedfilebrowser-module"), nil)
	defer m.Close()
	if s := m.Report().Modules[0]; s.Version != real.Version || s.State != StateInstalled {
		t.Fatalf("%+v", s)
	}
	dir := folderWithFiles(t)
	for _, o := range []fb.Options{{Kind: testKind}, {Kind: testKind, ShowHidden: true}, {Kind: testKind, MaxEntries: 2}} {
		want, err := fb.List(dir, o)
		if err != nil {
			t.Fatal(err)
		}
		if blob, err, handled := m.call(fileBrowserID, "list", map[string]any{"path": dir}, nil); !handled || err != nil || len(blob) == 0 {
			t.Fatalf("the helper wasn't used: %v %v", handled, err)
		}
		got, err := (FileBrowser{m}).List(dir, o)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			t.Fatalf("with %+v\n got %s\nwant %s", o.ShowHidden, a, b)
		}
	}
	// A file lists the folder holding it.
	if got, err := (FileBrowser{m}).List(filepath.Join(dir, "film.mkv"), fb.Options{Kind: testKind}); err != nil || got.Path != dir {
		t.Fatalf("%+v %v", got, err)
	}
	// The server's own words for what can't be opened.
	if _, err := (FileBrowser{m}).List(filepath.Join(dir, "gone"), fb.Options{}); err != fb.ErrNotFound {
		t.Fatalf("got %v", err)
	}
	// The helper finds the usual folders, though it is told nothing else about the account.
	lib := fb.Place{Name: "Films", Path: dir, Group: "libraries"}
	got, want := (FileBrowser{m}).Places(lib), fb.Places(lib)
	if !reflect.DeepEqual(got, want) {
		// Free space moves between two looks at a drive.
		for _, l := range [][]fb.Place{got, want} {
			for i := range l {
				l[i].Free = 0
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf(" got %+v\nwant %+v", got, want)
		}
	}
}

// A version that has no answer, or stops working, is passed over for the built-in copy.
func TestFileBrowserFallsBack(t *testing.T) {
	for _, test := range [][]string{nil, {"crash"}, {"garbage"}, {"die-at-start"}} {
		w := newWorld(t)
		m := withInstalled(t, w, browserManifest("0.1.0"), browserManifest("0.2.0", test...), helperBytes(t), nil)
		dir := folderWithFiles(t)
		want, _ := fb.List(dir, fb.Options{Kind: testKind})
		got, err := (FileBrowser{m}).List(dir, fb.Options{Kind: testKind})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%v: %+v %v", test, got, err)
		}
		if got := (FileBrowser{m}).Places(fb.Place{Name: "x", Path: dir, Group: "libraries"}); len(got) == 0 || got[len(got)-1].Path != dir {
			t.Errorf("%v: %+v", test, got)
		}
		m.Close()
	}
	if _, err := (FileBrowser{}).List(t.TempDir(), fb.Options{}); err != nil {
		t.Fatal("no manager at all is the built-in copy:", err)
	}
}

// What a helper says about where things are isn't taken on trust.
func TestFileBrowserChecksListings(t *testing.T) {
	dir := folderWithFiles(t)
	good := func() *fb.Listing {
		l, err := fb.List(dir, fb.Options{ShowHidden: true})
		if err != nil {
			t.Fatal(err)
		}
		return l
	}
	if checkedListing(dir, good(), fb.Options{ShowHidden: true}) == nil {
		t.Fatal("a true listing was refused")
	}
	other := t.TempDir()
	for name, spoil := range map[string]func(*fb.Listing){
		"another folder":   func(l *fb.Listing) { l.Path = other },
		"a relative path":  func(l *fb.Listing) { l.Path = "." },
		"an entry outside": func(l *fb.Listing) { l.Entries[0].Path = filepath.Join(other, l.Entries[0].Name) },
		"a climbing name":  func(l *fb.Listing) { l.Entries[0].Name = ".." },
		"a name with a path": func(l *fb.Listing) {
			l.Entries[0].Name = filepath.Join("a", "b")
			l.Entries[0].Path = filepath.Join(l.Path, l.Entries[0].Name)
		},
	} {
		l := good()
		spoil(l)
		if checkedListing(dir, l, fb.Options{ShowHidden: true}) != nil {
			t.Errorf("%s was accepted", name)
		}
	}
	// Hidden files stay hidden, whatever it says, and its counts and kinds are not used.
	l := good()
	l.Counts = map[string]int{"video": 999}
	for i := range l.Entries {
		l.Entries[i].Hidden = false
		l.Entries[i].Kind = "nonsense"
	}
	c := checkedListing(dir, l, fb.Options{})
	if c == nil || c.Counts["video"] != 1 || c.Counts["folder"] != 1 {
		t.Fatalf("%+v", c)
	}
	for _, e := range c.Entries {
		if e.Name == ".hidden.mkv" || e.Kind == "nonsense" {
			t.Fatalf("%+v", e)
		}
	}
	if r := realPlaces([]fb.Place{{Path: "relative"}, {Path: filepath.Join(dir, "gone")}, {Path: filepath.Join(dir, "film.mkv")}, {Path: dir}}); len(r) != 1 || r[0].Path != dir {
		t.Fatalf("%+v", r)
	}
}
