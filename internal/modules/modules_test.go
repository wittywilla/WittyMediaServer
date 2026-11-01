package modules

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// TestMain lets this test program be the launcher, as the server is, so the tests run
// their helpers under the real limits.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == SandboxFlag {
		os.Exit(RunSandboxed(os.Args[2:]))
	}
	os.Exit(m.Run())
}

var (
	fakeHelper []byte
	buildOnce  sync.Once
	buildErr   error
)

func helperBytes(t *testing.T) []byte {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fakehelper-*")
		if err != nil {
			buildErr = err
			return
		}
		defer os.RemoveAll(dir)
		out := filepath.Join(dir, "fake")
		cmd := exec.Command("go", "build", "-o", out, "./testdata/fakehelper")
		if b, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("%w\n%s", err, b)
			return
		}
		fakeHelper, buildErr = os.ReadFile(out)
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return fakeHelper
}

const builtinVersion = "0.1.0"

func manifestJSON(ver, min, max string, api int, test ...string) []byte {
	m := wittymodule.Manifest{ID: "wittyphotos", Name: "WittyPhotos", Version: ver, Repo: "wittywilla/WittyPhotos", Delivery: wittymodule.DeliveryHelper,
		Entry: "wittyphotos-module", API: api, MinServer: min, MaxServer: max}
	if len(test) > 0 {
		m.Capabilities = map[string][]string{"test": test}
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return b
}

// github stands in for GitHub at the transport, so the real address checks run.
type github struct {
	mu        sync.Mutex
	files     map[string][]byte
	rels      []update.GHRelease
	requests  int
	offline   bool
	notFound  bool
	limited   bool // answers every request as GitHub does when it has had enough
	apiCalls  int  // to the release list
	full      int  // of those, how many cost a full answer rather than a 304
	sawAgent  string
	forbidden bool               // answers 403 with nothing about the limit, as for one repository
	feedOnly  []update.GHRelease // what the list of the newest shows, when not everything
}

func (g *github) RoundTrip(req *http.Request) (*http.Response, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.requests++
	if g.offline {
		return nil, errors.New("no route to host")
	}
	answer := func(code int, b []byte) (*http.Response, error) {
		return &http.Response{StatusCode: code, Status: fmt.Sprint(code), Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Header: http.Header{}, Request: req}, nil
	}
	if g.notFound {
		return answer(404, nil)
	}
	if g.limited {
		res, _ := answer(403, nil)
		res.Header.Set("X-RateLimit-Remaining", "0")
		res.Header.Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(30*time.Minute).Unix()))
		return res, nil
	}
	if g.forbidden {
		return answer(403, nil)
	}
	if req.URL.Host == "api.github.com" && strings.Contains(req.URL.Path, "/releases/tags/") {
		tag := req.URL.Path[strings.LastIndexByte(req.URL.Path, '/')+1:]
		for _, r := range g.rels {
			if r.TagName == tag {
				b, _ := json.Marshal(r)
				return answer(200, b)
			}
		}
		return answer(404, nil)
	}
	if req.URL.Host == "api.github.com" {
		g.apiCalls++
		list := g.rels
		if g.feedOnly != nil {
			list = g.feedOnly
		}
		b, _ := json.Marshal(list)
		tag := `"` + sha(b) + `"`
		if req.Header.Get("If-None-Match") == tag {
			return answer(304, nil)
		}
		g.full++
		res, err := answer(200, b)
		res.Header.Set("ETag", tag)
		return res, err
	}
	if b, ok := g.files[req.URL.Path]; ok {
		return answer(200, b)
	}
	return answer(404, nil)
}

func (g *github) set(path string, b []byte) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.files[path] = b
}

type world struct {
	t      *testing.T
	key    ed25519.PrivateKey
	pub    ed25519.PublicKey
	gh     *github
	dir    string
	server string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &world{t: t, key: key, pub: pub, gh: &github{files: map[string][]byte{}}, dir: filepath.Join(t.TempDir(), "modules"), server: "0.4.0"}
}

func (w *world) manager() *Manager {
	exe, _ := os.Executable() // this test program is the launcher too: see TestMain
	return New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{manifestJSON(builtinVersion, "0.1.0", "0.x", 1)},
		Keys: []ed25519.PublicKey{w.pub}, Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Launcher: exe,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
}

func entryName() string {
	if runtime.GOOS == "windows" {
		return "wittyphotos-module.exe"
	}
	return "wittyphotos-module"
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func packFiles(top string, files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		for _, n := range names {
			f, _ := zw.Create(top + "/" + n)
			_, _ = f.Write(files[n])
		}
		_ = zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range names {
		_ = tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(files[n]))})
		_, _ = tw.Write(files[n])
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// publish puts a signed release of the fake helper on the fake GitHub, and
// returns the addresses of its files so a test can spoil one.
func (w *world) publish(ver string, man []byte) map[string]string {
	w.t.Helper()
	const id = "wittyphotos"
	top := wittymodule.TopFolder(id, ver, runtime.GOOS, runtime.GOARCH)
	files := map[string][]byte{wittymodule.ManifestFile: man, entryName(): helperBytes(w.t), "LICENSE": []byte("license")}
	archive := packFiles(top, files)
	archiveName := wittymodule.ArchiveName(id, ver, runtime.GOOS, runtime.GOARCH)
	manName := wittymodule.ManifestName(id, ver)
	var sums strings.Builder
	fmt.Fprintf(&sums, "%s  %s\n%s  %s\n", sha(archive), archiveName, sha(man), manName)
	for n, b := range files {
		fmt.Fprintf(&sums, "%s  %s/%s\n", sha(b), top, n)
	}
	list := []byte(sums.String())
	sumsName := wittymodule.SumsName(id, ver)
	base := "/wittywilla/WittyPhotos/releases/download/v" + ver + "/"
	assets := map[string][]byte{archiveName: archive, manName: man, sumsName: list, sumsName + ".sig": update.Sign(w.key, list)}
	paths := map[string]string{"archive": base + archiveName, "manifest": base + manName, "sums": base + sumsName, "sig": base + sumsName + ".sig"}
	g := update.GHRelease{TagName: "v" + ver, Name: "WittyPhotos " + ver, HTMLURL: "https://github.com/wittywilla/WittyPhotos/releases/tag/v" + ver, Prerelease: wittymodule.Prerelease(ver)}
	for name, b := range assets {
		w.gh.set(base+name, b)
		g.Assets = append(g.Assets, update.GHAsset{Name: name, Size: int64(len(b)), URL: "https://github.com" + base + name})
	}
	w.gh.mu.Lock()
	w.gh.rels = append([]update.GHRelease{g}, w.gh.rels...)
	w.gh.mu.Unlock()
	return paths
}

func one(t *testing.T, m *Manager) Status {
	t.Helper()
	r := m.Report()
	if len(r.Modules) != 1 {
		t.Fatalf("%d modules", len(r.Modules))
	}
	return r.Modules[0]
}

// installWait installs and waits for the result.
func installWait(t *testing.T, m *Manager) Status {
	t.Helper()
	if err := m.Install("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		s := one(t, m)
		if s.Phase == PhaseIdle || s.Phase == PhaseFailed {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the install never finished")
	return Status{}
}

func check(t *testing.T, m *Manager) Status {
	t.Helper()
	m.Check(context.Background(), 0)
	return one(t, m)
}

func (w *world) installed(ver string) bool {
	_, err := os.Stat(filepath.Join(w.dir, "wittyphotos", "versions", ver))
	return err == nil
}

// usesHelper: calls really go to the helper, as this version.
func usesHelper(t *testing.T, m *Manager, ver string) {
	t.Helper()
	p := PhotoEngine{m}
	in, err := p.ReadFile("anything.jpg")
	if err != nil || in.Make != "fake "+ver {
		t.Fatalf("read: %+v %v, want the helper's %s", in, err, ver)
	}
	if w, h := p.Dimensions("anything.jpg"); w != 11 || h != 22 {
		t.Fatalf("dimensions: %d %d", w, h)
	}
	if b, err := p.RawPreview("anything.cr2"); err != nil || string(b) != "jpeg from "+ver {
		t.Fatalf("preview: %q %v", b, err)
	}
}

func usesBuiltin(t *testing.T, m *Manager) {
	t.Helper()
	s := one(t, m)
	if s.Version != builtinVersion {
		t.Fatalf("using %s (%s), want the built-in copy", s.Version, s.Notice)
	}
	p := PhotoEngine{m}
	if w, _ := p.Dimensions(filepath.Join(t.TempDir(), "missing.jpg")); w != 0 {
		t.Fatalf("the built-in copy found dimensions of a missing file: %d", w)
	}
}

func TestInRangeInstallRunsAndSurvivesRestart(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	if s := one(t, m); s.State != StateBuiltin || s.Version != builtinVersion {
		t.Fatalf("before: %+v", s)
	}
	s := check(t, m)
	if s.Latest == nil || s.Latest.Version != "0.2.0" {
		t.Fatalf("not offered: %+v %s", s, m.Report().CheckError)
	}
	if s = installWait(t, m); s.Phase != PhaseIdle || s.State != StateInstalled || s.Version != "0.2.0" {
		t.Fatalf("after: %+v", s)
	}
	usesHelper(t, m, "0.2.0")
	m.Close()

	m2 := w.manager()
	defer m2.Close()
	if s := one(t, m2); s.State != StateInstalled || s.Version != "0.2.0" {
		t.Fatalf("after a restart: %+v", s)
	}
	usesHelper(t, m2, "0.2.0")
}

func TestOutOfRangeIsNotOfferedOrInstallable(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.5.0", "0.5.x", 1)) // needs a newer server
	m := w.manager()
	defer m.Close()
	s := check(t, m)
	if s.Latest != nil {
		t.Fatalf("offered: %+v", s.Latest)
	}
	if s.TooNew == nil || !strings.Contains(s.TooNew.Reason, "0.5.0 or newer") {
		t.Fatalf("no plain reason: %+v", s.TooNew)
	}
	if err := m.Install("wittyphotos"); err == nil {
		t.Fatal("installed anyway")
	}
	usesBuiltin(t, m)
}

func TestOlderCompatibleReleaseIsOfferedUnderATooNewOne(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.4.x", 1))
	w.publish("0.3.0", manifestJSON("0.3.0", "0.5.0", "0.5.x", 1))
	w.publish("0.3.1", manifestJSON("0.3.1", "0.4.0", "0.5.x", wittymodule.API+1)) // a newer module API
	m := w.manager()
	defer m.Close()
	s := check(t, m)
	if s.Latest == nil || s.Latest.Version != "0.2.0" {
		t.Fatalf("latest: %+v", s.Latest)
	}
	if s.TooNew == nil || s.TooNew.Version != "0.3.1" || !strings.Contains(s.TooNew.Reason, "module API") {
		t.Fatalf("tooNew: %+v", s.TooNew)
	}
}

func TestServerUpgradeOutOfRangeFallsBack(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.4.x", 1))
	m := w.manager()
	check(t, m)
	if s := installWait(t, m); s.Version != "0.2.0" {
		t.Fatalf("install: %+v", s)
	}
	w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.4.5", 1))
	check(t, m)
	if s := installWait(t, m); s.Version != "0.3.0" || s.Previous != "0.2.0" {
		t.Fatalf("second install: %+v", s)
	}
	m.Close()

	// 0.4.6: the newest is past its maximum, the one before it still fits.
	w.server = "0.4.6"
	m = w.manager()
	s := one(t, m)
	if s.Version != "0.2.0" || s.State != StateFallback || !strings.Contains(s.Notice, "up to 0.4.5") || !strings.Contains(s.Notice, "version 0.2.0") {
		t.Fatalf("to the previous version: %+v", s)
	}
	usesHelper(t, m, "0.2.0")
	m.Close()

	// 0.5.0: neither fits, so the built-in copy.
	w.server = "0.5.0"
	m = w.manager()
	s = one(t, m)
	if s.Version != builtinVersion || s.State != StateFallback || !strings.Contains(s.Notice, "built-in") {
		t.Fatalf("to the built-in copy: %+v", s)
	}
	usesBuiltin(t, m)
	if !w.installed("0.3.0") || !w.installed("0.2.0") {
		t.Fatal("refused versions must stay, in case the server is rolled back")
	}
	m.Close()

	// Back on the old server, everything is as it was.
	w.server = "0.4.0"
	m = w.manager()
	defer m.Close()
	if s := one(t, m); s.Version != "0.3.0" || s.State != StateInstalled {
		t.Fatalf("after going back: %+v", s)
	}
}

func TestNewerBuiltinWins(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	check(t, m)
	installWait(t, m)
	m.Close()
	m2 := New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{manifestJSON("0.3.0", "0.1.0", "0.x", 1)}, Keys: []ed25519.PublicKey{w.pub},
		Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m2.Close()
	if s := one(t, m2); s.Version != "0.3.0" || s.Latest != nil {
		t.Fatalf("%+v", s)
	}
}

func TestPrereleases(t *testing.T) {
	// A server on the alpha can't use a module that starts at the release.
	w := newWorld(t)
	w.server = "0.4.0-alpha"
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	s := check(t, m)
	if s.Latest != nil || s.TooNew == nil {
		t.Fatalf("alpha server, module from 0.4.0: %+v", s)
	}
	m.Close()

	// A module that says it works on the alpha does.
	w = newWorld(t)
	w.server = "0.4.0-alpha"
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0-alpha", "0.5.x", 1))
	m = w.manager()
	if s := check(t, m); s.Latest == nil {
		t.Fatalf("alpha server, module from the alpha: %+v", s)
	}
	m.Close()

	// A stable server isn't offered a pre-release module, an alpha server is.
	w = newWorld(t)
	w.publish("0.2.0-beta", manifestJSON("0.2.0-beta", "0.4.0-alpha", "0.5.x", 1))
	m = w.manager()
	if s := check(t, m); s.Latest != nil {
		t.Fatalf("stable server offered a pre-release: %+v", s.Latest)
	}
	m.Close()
	w.server = "0.4.1-rc1"
	m = w.manager()
	defer m.Close()
	if s := check(t, m); s.Latest == nil {
		t.Fatalf("pre-release server not offered a pre-release: %+v", s)
	}
}

func TestBadSignatureAtCheckIsIgnored(t *testing.T) {
	w := newWorld(t)
	paths := w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	w.gh.set(paths["sig"], update.Sign(other, w.gh.files[paths["sums"]]))
	m := w.manager()
	defer m.Close()
	s := check(t, m)
	if s.Latest != nil {
		t.Fatalf("offered: %+v", s.Latest)
	}
	if r := m.Report(); !strings.Contains(r.CheckError, "signature") {
		t.Fatalf("no reason: %q", r.CheckError)
	}
}

func TestBadSignatureAtInstallChangesNothing(t *testing.T) {
	w := newWorld(t)
	paths := w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	w.gh.set(paths["sig"], update.Sign(other, w.gh.files[paths["sums"]]))
	s := installWait(t, m)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "signature") {
		t.Fatalf("%+v", s)
	}
	usesBuiltin(t, m)
	if w.installed("0.2.0") {
		t.Fatal("a version with a bad signature is on disk")
	}
}

func TestListCantBeReplacedByAnUnsignedOne(t *testing.T) {
	w := newWorld(t)
	paths := w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	w.gh.set(paths["sums"], append(w.gh.files[paths["sums"]], []byte("# changed\n")...))
	if s := installWait(t, m); s.Phase != PhaseFailed {
		t.Fatalf("%+v", s)
	}
	usesBuiltin(t, m)
}

func TestCorruptDownloadChangesNothing(t *testing.T) {
	w := newWorld(t)
	paths := w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	good := w.gh.files[paths["archive"]]
	bad := append([]byte{}, good...)
	bad[len(bad)/2] ^= 0xFF
	w.gh.set(paths["archive"], bad)
	s := installWait(t, m)
	if s.Phase != PhaseFailed || !strings.Contains(s.Error, "does not match its signed checksum") {
		t.Fatalf("%+v", s)
	}
	usesBuiltin(t, m)
	if w.installed("0.2.0") {
		t.Fatal("a corrupt version is on disk")
	}
	entries, _ := os.ReadDir(filepath.Join(w.dir, "wittyphotos"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".work-") {
			t.Fatal("left work behind")
		}
	}
	// A good download works straight after a bad one.
	w.gh.set(paths["archive"], good)
	if s := installWait(t, m); s.Phase != PhaseIdle || s.Version != "0.2.0" {
		t.Fatalf("retry: %+v", s)
	}
}

func TestInstallFailureKeepsTheVersionInUse(t *testing.T) {
	for name, man := range map[string][]byte{
		"dies at start": manifestJSON("0.3.0", "0.4.0", "0.5.x", 1, "die-at-start"),
		"wrong version": manifestJSON("0.3.0", "0.4.0", "0.5.x", 1, "wrong-version"),
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
			m := w.manager()
			defer m.Close()
			check(t, m)
			installWait(t, m)
			usesHelper(t, m, "0.2.0")
			w.publish("0.3.0", man)
			check(t, m)
			s := installWait(t, m)
			if s.Phase != PhaseFailed || !strings.Contains(s.Error, "didn't start") {
				t.Fatalf("%+v", s)
			}
			if s.Version != "0.2.0" || w.installed("0.3.0") {
				t.Fatalf("not rolled back: %+v", s)
			}
			usesHelper(t, m, "0.2.0")
		})
	}
}

func TestManifestCantClaimAnotherVersion(t *testing.T) {
	w := newWorld(t)
	// A signed 0.2.0 manifest attached to a 0.3.0 tag.
	w.publish("0.3.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	if s := check(t, m); s.Latest != nil {
		t.Fatalf("offered: %+v", s.Latest)
	}
}

func TestChangedFilesAreRefused(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	check(t, m)
	installWait(t, m)
	usesHelper(t, m, "0.2.0")
	m.Close()
	exe := filepath.Join(w.dir, "wittyphotos", "versions", "0.2.0", entryName())

	// Changed while the server was off.
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	writeExe(t, exe, append(b, 0))
	m = w.manager()
	s := one(t, m)
	if s.Version != builtinVersion || !strings.Contains(s.Notice, "changed") {
		t.Fatalf("%+v", s)
	}
	m.Close()

	// A file added beside it.
	writeExe(t, exe, b)
	if err := os.WriteFile(filepath.Join(filepath.Dir(exe), "extra.dll"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m = w.manager()
	if s := one(t, m); s.Version != builtinVersion || !strings.Contains(s.Notice, "doesn't belong") {
		t.Fatalf("%+v", s)
	}
	m.Close()
	if err := os.Remove(filepath.Join(filepath.Dir(exe), "extra.dll")); err != nil {
		t.Fatal(err)
	}

	// Changed while the server runs: refused at the next start of the helper, and
	// the call still gets an answer from the built-in copy.
	m = w.manager()
	defer m.Close()
	if s := one(t, m); s.Version != "0.2.0" {
		t.Fatalf("%+v", s)
	}
	writeExe(t, exe, append(b, 0))
	if _, err := (PhotoEngine{m}).ReadFile(filepath.Join(t.TempDir(), "missing.jpg")); err == nil {
		t.Fatal("expected the built-in copy's error for a missing file")
	}
	usesBuiltin(t, m)
	if s := one(t, m); !strings.Contains(s.Notice, "changed") {
		t.Fatalf("%+v", s)
	}
}

func TestCrashingVersionRollsBackByItself(t *testing.T) {
	for _, bad := range []string{"crash", "garbage"} {
		t.Run(bad, func(t *testing.T) {
			w := newWorld(t)
			w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
			m := w.manager()
			defer m.Close()
			check(t, m)
			installWait(t, m)
			w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.5.x", 1, bad))
			check(t, m)
			if s := installWait(t, m); s.Version != "0.3.0" {
				t.Fatalf("%+v", s)
			}
			// Every call is answered: by the built-in copy while it fails, and by the
			// previous version once it has been given up on.
			for i := 0; i < failLimit; i++ {
				if w, _ := (PhotoEngine{m}).Dimensions(filepath.Join(t.TempDir(), "missing.jpg")); w != 0 {
					t.Fatalf("call %d: %d", i, w)
				}
			}
			s := one(t, m)
			if s.Version != "0.2.0" || s.State != StateFallback || !strings.Contains(s.Notice, "kept failing") {
				t.Fatalf("%+v", s)
			}
			usesHelper(t, m, "0.2.0")
			m.Close()

			// It stays given up on across a restart, and isn't offered for automatic
			// install again.
			m2 := w.manager()
			defer m2.Close()
			if s := one(t, m2); s.Version != "0.2.0" || !strings.Contains(s.Notice, "kept failing") {
				t.Fatalf("after a restart: %+v", s)
			}
			s = check(t, m2)
			if s.Latest == nil || s.Latest.Blocked == "" {
				t.Fatalf("offer: %+v", s.Latest)
			}
			m2.autoInstall()
			time.Sleep(50 * time.Millisecond)
			if s := one(t, m2); s.Version != "0.2.0" {
				t.Fatalf("installed automatically: %+v", s)
			}
		})
	}
}

func TestHangingVersionIsKilled(t *testing.T) {
	old := callTimeout
	callTimeout = 200 * time.Millisecond
	defer func() { callTimeout = old }()
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1, "hang"))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	start := time.Now()
	for i := 0; i < failLimit; i++ {
		(PhotoEngine{m}).Dimensions(filepath.Join(t.TempDir(), "missing.jpg"))
	}
	if time.Since(start) > 20*time.Second {
		t.Fatal("took too long")
	}
	usesBuiltin(t, m)
}

func TestManualRollback(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	if err := m.Rollback("wittyphotos"); err == nil {
		t.Fatal("rolled back the built-in copy")
	}
	if err := m.Rollback("nothing"); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
	check(t, m)
	installWait(t, m)
	w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.5.x", 1))
	check(t, m)
	installWait(t, m)

	if err := m.Rollback("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	s := one(t, m)
	if s.Version != "0.2.0" || s.State != StateInstalled || s.Notice != "" {
		t.Fatalf("%+v", s)
	}
	usesHelper(t, m, "0.2.0")
	if err := m.Rollback("wittyphotos"); err != nil {
		t.Fatal(err)
	}
	usesBuiltin(t, m)

	// Choosing to install it again works, and clears its block.
	s = check(t, m)
	if s.Latest == nil || s.Latest.Version != "0.3.0" || s.Latest.Blocked == "" {
		t.Fatalf("%+v", s.Latest)
	}
	if s = installWait(t, m); s.Version != "0.3.0" || s.State != StateInstalled {
		t.Fatalf("%+v", s)
	}
}

func TestOfflineKeepsWorking(t *testing.T) {
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	w.publish("0.3.0", manifestJSON("0.3.0", "0.4.0", "0.5.x", 1))
	if s := check(t, m); s.Latest == nil {
		t.Fatal("not offered")
	}
	w.gh.mu.Lock()
	w.gh.offline = true
	w.gh.mu.Unlock()
	r := m.Check(context.Background(), 0)
	if !strings.Contains(r.CheckError, "no route to host") {
		t.Fatalf("%q", r.CheckError)
	}
	s := one(t, m)
	if s.Latest == nil || s.Version != "0.2.0" {
		t.Fatalf("lost what it knew: %+v", s)
	}
	usesHelper(t, m, "0.2.0")
	if s := installWait(t, m); s.Phase != PhaseFailed || s.Version != "0.2.0" {
		t.Fatalf("offline install: %+v", s)
	}
	m.Close()
	m2 := w.manager() // starting offline
	defer m2.Close()
	if s := one(t, m2); s.Version != "0.2.0" {
		t.Fatalf("%+v", s)
	}
	usesHelper(t, m2, "0.2.0")
}

func TestNothingPhonesHomeWithoutPermission(t *testing.T) {
	old := firstCheck
	firstCheck = time.Millisecond
	defer func() { firstCheck = old }()
	w := newWorld(t)
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", 1))
	var mu sync.Mutex
	allowCheck, allowInstall := false, false
	get := func(p *bool) func() bool {
		return func() bool { mu.Lock(); defer mu.Unlock(); return *p }
	}
	run := func(want string) {
		t.Helper()
		m := New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{manifestJSON(builtinVersion, "0.1.0", "0.x", 1)}, Keys: []ed25519.PublicKey{w.pub},
			Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), CheckEnabled: get(&allowCheck), AutoInstall: get(&allowInstall)})
		defer m.Close()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { m.Run(ctx); close(done) }()
		// On a busy machine a build of the helper takes time to scan and start, and the
		// first check may not have begun: wait for what is expected, not for a moment.
		deadline := time.Now().Add(2 * time.Minute)
		settled := func() bool {
			w.gh.mu.Lock()
			asked := w.gh.requests > 0
			w.gh.mu.Unlock()
			return one(t, m).Version == want && (asked || !get(&allowCheck)())
		}
		for !settled() && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		cancel()
		<-done
		if s := one(t, m); s.Version != want {
			t.Fatalf("using %s, want %s", s.Version, want)
		}
	}

	run(builtinVersion)
	w.gh.mu.Lock()
	n := w.gh.requests
	w.gh.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d requests with checks off", n)
	}

	mu.Lock()
	allowCheck = true
	mu.Unlock()
	run(builtinVersion)
	w.gh.mu.Lock()
	n = w.gh.requests
	w.gh.mu.Unlock()
	if n == 0 {
		t.Fatal("no check with checks on")
	}
	if w.installed("0.2.0") {
		t.Fatal("installed without being allowed to")
	}

	mu.Lock()
	allowInstall = true
	mu.Unlock()
	run("0.2.0")
}

func TestStateFileProblemsAreNotFatal(t *testing.T) {
	w := newWorld(t)
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(w.dir, "state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := w.manager()
	usesBuiltin(t, m)
	m.Close()

	// A version with a path in it goes nowhere near the disk.
	if err := os.WriteFile(filepath.Join(w.dir, "state.json"), []byte(`{"wittyphotos":{"active":"1.0.0-../../../evil"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	m = w.manager()
	defer m.Close()
	usesBuiltin(t, m)
}

func TestBuiltinModulesCantBeInstalled(t *testing.T) {
	w := newWorld(t)
	m := New(Config{Dir: w.dir, Server: w.server, Builtins: [][]byte{[]byte(`{"id":"wittydvd","name":"WittyDVD","version":"0.1.0","repo":"wittywilla/WittyDVD","delivery":"builtin","api":1,"minServer":"0.4.0","maxServer":"0.5.x"}`)},
		Keys: []ed25519.PublicKey{w.pub}, Fetch: &update.Fetcher{UserAgent: "test", Transport: w.gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()
	m.Check(context.Background(), 0)
	w.gh.mu.Lock()
	n := w.gh.requests
	w.gh.mu.Unlock()
	if n != 0 {
		t.Fatalf("a built-in module made %d requests", n)
	}
	if err := m.Install("wittydvd"); err == nil || !strings.Contains(err.Error(), "updates with it") {
		t.Fatal(err)
	}
	if err := m.Install("nothing"); !errors.Is(err, ErrUnknown) {
		t.Fatal(err)
	}
}

func TestEveryRealManifestIsSound(t *testing.T) {
	seen := map[string]bool{}
	for _, raw := range GoManifests() {
		m, err := wittymodule.ParseManifest(raw)
		if err != nil {
			t.Fatal(err)
		}
		if seen[m.ID] {
			t.Errorf("%s twice", m.ID)
		}
		seen[m.ID] = true
		if m.API < wittymodule.MinAPI || m.API > wittymodule.API {
			t.Errorf("%s: api %d", m.ID, m.API)
		}
	}
	if !seen["wittyphotos"] {
		t.Error("WittyPhotos is missing")
	}
}

// The whole way, with nothing fake: the real WittyPhotos helper, built, packed and
// signed by the real tool, installed, and reading a real picture.
func TestRealHelperEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the real helper")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	key := filepath.Join(out, "test.key")
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := os.WriteFile(key, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := func(args ...string) {
		t.Helper()
		cmd := exec.Command("go", append([]string{"run", "./modules/wittymodule/cmd/wittymodule"}, args...)...)
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("wittymodule %v: %v\n%s", args, err, b)
		}
	}
	tool("check", "modules/wittyphotos")
	tool("pack", "-out", out, "-targets", runtime.GOOS+"/"+runtime.GOARCH, "modules/wittyphotos")
	raw, err := os.ReadFile(filepath.Join(root, "modules", "wittyphotos", "module.json"))
	if err != nil {
		t.Fatal(err)
	}
	real, err := wittymodule.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	tool("sign", "-key", key, filepath.Join(out, wittymodule.SumsName(real.ID, real.Version)))

	gh := &github{files: map[string][]byte{}}
	g := update.GHRelease{TagName: "v" + real.Version}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "test.key" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(out, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		gh.files["/r/"+e.Name()] = b
		g.Assets = append(g.Assets, update.GHAsset{Name: e.Name(), Size: int64(len(b)), URL: "https://github.com/r/" + e.Name()})
	}
	gh.rels = []update.GHRelease{g}

	old := append([]byte{}, raw...)
	old = bytes.Replace(old, []byte(`"version": "`+real.Version+`"`), []byte(`"version": "0.0.1"`), 1)
	m := New(Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: real.MinServer, Builtins: [][]byte{old}, Keys: []ed25519.PublicKey{pub},
		Fetch: &update.Fetcher{UserAgent: "test", Transport: gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()
	s := check(t, m)
	if s.Latest == nil {
		t.Fatalf("not offered: %+v %s", s, m.Report().CheckError)
	}
	if s = installWait(t, m); s.Phase != PhaseIdle || s.Version != real.Version {
		t.Fatalf("%+v", s)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 7, 5))); err != nil {
		t.Fatal(err)
	}
	pic := filepath.Join(t.TempDir(), "real.png")
	if err := os.WriteFile(pic, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	var d struct{ W, H int }
	if _, err, handled := m.call("wittyphotos", "dimensions", pathParams{pic}, &d); !handled || err != nil || d.W != 7 || d.H != 5 {
		t.Fatalf("handled=%v err=%v %+v", handled, err, d)
	}
	if !(PhotoEngine{m}).IsRaw("x.CR3") || (PhotoEngine{m}).IsRaw("x.mp4") {
		t.Fatal("IsRaw")
	}
}

// writeExe replaces a helper's program. On Windows a virus scanner may hold one
// for a moment after it stopped.
func writeExe(t *testing.T, path string, b []byte) {
	t.Helper()
	var err error
	for i := 0; i < 50; i++ {
		if err = os.WriteFile(path, b, 0o755); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal(err)
}

// A module whose repository isn't published yet has nothing newer. That isn't a problem to report.
func TestUnpublishedRepositoryIsNotAnError(t *testing.T) {
	w := newWorld(t)
	w.gh.notFound = true
	m := w.manager()
	defer m.Close()
	if r := m.Check(context.Background(), 0); r.CheckError != "" || r.Modules[0].Latest != nil {
		t.Fatalf("%+v", r)
	}
}
