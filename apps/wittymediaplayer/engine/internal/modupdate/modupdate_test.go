package modupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaplayer/engine/internal/modhost"
	"github.com/wittywilla/wittymodule"
)

const testID = "wittydvdplayer"

// TestMain lets this test program stand in for a module's helper: a copy of it, beside a
// manifest, says hello as that version and answers like a module.
func TestMain(m *testing.M) {
	if len(os.Args) == 2 && os.Args[1] == wittymodule.HelperFlag {
		exe, _ := os.Executable()
		raw, err := os.ReadFile(filepath.Join(filepath.Dir(exe), wittymodule.ManifestFile))
		if err != nil {
			os.Exit(9)
		}
		man, err := wittymodule.ParseManifest(raw)
		if err != nil {
			os.Exit(9)
		}
		_ = wittymodule.Serve(os.Stdin, os.Stdout, wittymodule.Hello{Module: man.ID, Version: man.Version, API: man.API}, func(method string, params json.RawMessage) (any, []byte, error) {
			if method == "write" {
				var in struct{ Path string }
				_ = json.Unmarshal(params, &in)
				err := os.WriteFile(in.Path, []byte("x"), 0o644)
				return map[string]bool{"wrote": err == nil}, nil, nil
			}
			if method == "crash" {
				os.Exit(3)
			}
			if method == "refuse" {
				return nil, nil, &wittymodule.CallError{Code: "protected", Message: "no"}
			}
			return map[string]string{"said": method}, []byte("blob"), nil
		})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type feed struct {
	t        *testing.T
	priv     ed25519.PrivateKey
	files    map[string][]byte
	releases []map[string]any
}

func exeBytes(t *testing.T) []byte {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// release publishes a signed release, the way the release tool lays one out.
func (f *feed) release(srv *httptest.Server, version string, tweak func(man *wittymodule.Manifest, files map[string][]byte)) {
	man := wittymodule.Manifest{ID: testID, Name: "WittyDVD for WittyMediaPlayer", Version: version, Repo: "wittywilla/WittyDVD", Delivery: wittymodule.DeliveryHelper,
		Entry: "wittydvd-player", API: 1, MinServer: "0.1.0", MaxServer: "0.1.x"}
	files := map[string][]byte{man.EntryFile(runtime.GOOS): exeBytes(f.t)}
	if tweak != nil {
		tweak(&man, files)
	}
	raw, err := json.Marshal(man)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, ok := files[wittymodule.ManifestFile]; !ok {
		files[wittymodule.ManifestFile] = raw
	}
	id, v := man.ID, version
	top := wittymodule.TopFolder(id, v, runtime.GOOS, runtime.GOARCH)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	var list strings.Builder
	for name, b := range files {
		w, _ := z.Create(top + "/" + name)
		_, _ = w.Write(b)
		if !strings.Contains(name, "..") {
			fmt.Fprintf(&list, "%s  %s/%s\n", modhost.Sum(b), top, name)
		}
	}
	z.Close()
	arc := wittymodule.ArchiveName(id, v, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&list, "%s  %s\n%s  %s\n", modhost.Sum(buf.Bytes()), arc, modhost.Sum(raw), wittymodule.ManifestName(id, v))
	sums := []byte(list.String())
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(f.priv, sums)) + "\n")
	assets := []map[string]any{}
	for name, b := range map[string][]byte{arc: buf.Bytes(), wittymodule.SumsName(id, v): sums, wittymodule.SumsName(id, v) + ".sig": sig, wittymodule.ManifestName(id, v): raw} {
		f.files["/dl/"+v+"/"+name] = b
		assets = append(assets, map[string]any{"name": name, "browser_download_url": srv.URL + "/dl/" + v + "/" + name, "size": len(b)})
	}
	f.releases = append(f.releases, map[string]any{"tag_name": testID + "-v" + v, "assets": assets})
}

func setup(t *testing.T) (*Client, *modhost.Manager, modhost.Module, *feed, *httptest.Server) {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(nil)
	f := &feed{t: t, priv: priv, files: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases") {
			_ = json.NewEncoder(w).Encode(f.releases)
			return
		}
		b, ok := f.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(srv.Close)
	mod := modhost.Module{ID: testID, Name: "WittyDVD for WittyMediaPlayer", Repo: "wittywilla/WittyDVD", Builtin: "0.1.0"}
	m := &modhost.Manager{Dir: t.TempDir(), Host: "0.1.0", Keys: []ed25519.PublicKey{pub}, Modules: []modhost.Module{mod}}
	c := &Client{Mgr: m, HTTP: srv.Client(), API: srv.URL, AnyHost: true}
	return c, m, mod, f, srv
}

func TestCheckInstallAndRun(t *testing.T) {
	c, m, mod, f, srv := setup(t)
	ctx := context.Background()
	if o := c.Check(ctx, mod); o.Version != "" {
		t.Fatalf("nothing published, but offered %+v", o)
	}
	f.release(srv, "0.1.1", nil)
	f.release(srv, "0.1.2", nil)
	o := c.Check(ctx, mod)
	if o.Version != "0.1.2" {
		t.Fatalf("offered %+v", o)
	}
	// Until installed, nothing changes.
	if ch := m.Choose(mod); ch.Entry != "" || ch.Version != "0.1.0" {
		t.Fatalf("%+v", ch)
	}
	if err := c.Install(ctx, mod, "0.1.1"); err != nil {
		t.Fatal(err)
	}
	ch := m.Choose(mod)
	if ch.Entry == "" || ch.Version != "0.1.1" {
		t.Fatalf("%+v", ch)
	}
	// It runs, in its own process, and its answers come through.
	r, err := modhost.Start(ch.Entry, mod.ID, ch.Version)
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Call("art", json.RawMessage(`{}`))
	if err != nil || string(res.Blob) != "blob" || !strings.Contains(string(res.JSON), "art") {
		t.Errorf("%+v %v", res, err)
	}
	if _, err := r.Call("refuse", nil); err == nil {
		t.Error("a refusal should come through")
	} else if _, ok := err.(*wittymodule.CallError); !ok {
		t.Errorf("a refusal isn't a failure: %T %v", err, err)
	}
	r.Stop()
	// A newer one replaces it, with the old one kept to go back to.
	if err := c.Install(ctx, mod, "0.1.2"); err != nil {
		t.Fatal(err)
	}
	if got := m.LoadState(mod.ID); got.Active != "0.1.2" || got.Previous != "0.1.1" {
		t.Errorf("%+v", got)
	}
	if v, err := m.Rollback(mod); err != nil || v != "0.1.1" {
		t.Errorf("rollback: %s %v", v, err)
	}
	if o := c.Check(ctx, mod); o.Version != "" || !strings.Contains(o.Note, "went back") {
		t.Errorf("a version gone back from isn't offered again: %+v", o)
	}
	if v, err := m.Rollback(mod); err != nil || v != "0.1.0" {
		t.Errorf("back to built in: %s %v", v, err)
	}
	if _, err := m.Rollback(mod); err == nil {
		t.Error("nothing left to go back from")
	}
	if ch := m.Choose(mod); ch.Entry != "" {
		t.Errorf("%+v", ch)
	}
}

func TestOnlyWhatIsSignedRuns(t *testing.T) {
	c, m, mod, f, srv := setup(t)
	ctx := context.Background()
	f.release(srv, "0.2.0", func(man *wittymodule.Manifest, _ map[string][]byte) { man.MaxServer = "0.2.x" })
	// Signed by someone else.
	other := *c.Mgr
	otherPub, _, _ := ed25519.GenerateKey(nil)
	other.Keys = []ed25519.PublicKey{otherPub}
	bad := &Client{Mgr: &other, HTTP: c.HTTP, API: c.API, AnyHost: true}
	if o := bad.Check(ctx, mod); o.Version != "" || !strings.Contains(o.Note, "signature") {
		t.Errorf("a stranger's release: %+v", o)
	}
	if err := bad.Install(ctx, mod, "0.2.0"); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("installing a stranger's release: %v", err)
	}
	if _, err := os.Stat(m.VersionDir(mod.ID, "0.2.0")); err == nil {
		t.Error("a refused version was kept")
	}
	// After installing, a changed file stops it running.
	if err := c.Install(ctx, mod, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	dir := m.VersionDir(mod.ID, "0.2.0")
	if ch := m.Choose(mod); ch.Entry == "" {
		t.Fatalf("%+v", ch)
	}
	if err := os.WriteFile(filepath.Join(dir, "extra.dll"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ch := m.Choose(mod)
	if ch.Entry != "" || !strings.Contains(ch.Note, "isn't in its signed list") {
		t.Errorf("an added file: %+v", ch)
	}
	os.Remove(filepath.Join(dir, "extra.dll"))
	f2 := filepath.Join(dir, wittymodule.ManifestFile)
	raw, _ := os.ReadFile(f2)
	_ = os.WriteFile(f2, append(raw, ' '), 0o644)
	if ch := m.Choose(mod); ch.Entry != "" || !strings.Contains(ch.Note, "signed checksum") {
		t.Errorf("a changed file: %+v", ch)
	}
}

func TestReleasesThatDontFit(t *testing.T) {
	c, m, mod, f, srv := setup(t)
	ctx := context.Background()
	// For a later player.
	f.release(srv, "0.5.0", func(man *wittymodule.Manifest, _ map[string][]byte) { man.MinServer, man.MaxServer = "0.5.0", "0.5.x" })
	o := c.Check(ctx, mod)
	if o.Version != "" || !strings.Contains(o.Note, "WittyMediaPlayer 0.5.0 or newer") {
		t.Errorf("%+v", o)
	}
	if err := c.Install(ctx, mod, "0.5.0"); err == nil {
		t.Error("a release for a later player installed")
	}
	// But an older one that fits is still offered.
	f.release(srv, "0.2.0", func(man *wittymodule.Manifest, _ map[string][]byte) {})
	if o := c.Check(ctx, mod); o.Version != "0.2.0" {
		t.Errorf("%+v", o)
	}
	// A pre-release doesn't reach a stable player.
	f.release(srv, "0.6.0-beta", nil)
	if o := c.Check(ctx, mod); o.Version != "0.2.0" {
		t.Errorf("%+v", o)
	}
	// Another module's manifest under this module's release.
	f.release(srv, "0.7.0", func(man *wittymodule.Manifest, _ map[string][]byte) { man.Version = "0.1.5" })
	if o := c.Check(ctx, mod); o.Version != "0.2.0" {
		t.Errorf("%+v", o)
	}
	// A player that outgrows an installed version falls back by itself.
	if err := c.Install(ctx, mod, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	later := *m
	later.Host = "0.2.0"
	if ch := later.Choose(mod); ch.Entry != "" || !strings.Contains(ch.Note, "this player is 0.2.0") {
		t.Errorf("%+v", ch)
	}
}

func TestBadArchivesAreRefused(t *testing.T) {
	c, m, mod, f, srv := setup(t)
	ctx := context.Background()
	f.release(srv, "0.1.1", func(_ *wittymodule.Manifest, files map[string][]byte) { files["../../evil.exe"] = []byte("x") })
	if err := c.Install(ctx, mod, "0.1.1"); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("climbing out: %v", err)
	}
	// A helper that says it's something else.
	f.release(srv, "0.1.2", func(_ *wittymodule.Manifest, files map[string][]byte) {
		other, _ := json.Marshal(wittymodule.Manifest{ID: testID, Name: "x", Version: "0.0.9", Repo: "wittywilla/WittyDVD", Delivery: wittymodule.DeliveryHelper, Entry: "wittydvd-player", API: 1, MinServer: "0.1.0", MaxServer: "0.1.x"})
		files[wittymodule.ManifestFile] = other
	})
	if err := c.Install(ctx, mod, "0.1.2"); err == nil {
		t.Error("a helper with the wrong version installed")
	}
	if ch := m.Choose(mod); ch.Entry != "" {
		t.Errorf("%+v", ch)
	}
	entries, _ := os.ReadDir(filepath.Dir(m.VersionDir(mod.ID, "x")))
	if len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}

// Three failures in a row take a version back out, and a success resets the count.
func TestKeepsFailingRollsBack(t *testing.T) {
	c, m, mod, f, srv := setup(t)
	ctx := context.Background()
	f.release(srv, "0.1.1", nil)
	f.release(srv, "0.1.2", nil)
	for _, v := range []string{"0.1.1", "0.1.2"} {
		if err := c.Install(ctx, mod, v); err != nil {
			t.Fatal(err)
		}
	}
	if m.CallFailed(mod, "0.1.2", "crashed") || m.CallFailed(mod, "0.1.2", "crashed") {
		t.Fatal("too soon")
	}
	m.CallOK(mod)
	if m.CallFailed(mod, "0.1.2", "crashed") || m.CallFailed(mod, "0.1.2", "crashed") {
		t.Fatal("a success should reset the count")
	}
	if !m.CallFailed(mod, "0.1.2", "crashed") {
		t.Fatal("three in a row")
	}
	if ch := m.Choose(mod); ch.Version != "0.1.1" {
		t.Errorf("%+v", ch)
	}
	st := m.Statuses()[0]
	if st.InUse != "0.1.1" || !strings.Contains(st.Note, "kept failing") {
		t.Errorf("%+v", st)
	}
	// And a crash really is seen as one.
	ch := m.Choose(mod)
	r, err := modhost.Start(ch.Entry, mod.ID, ch.Version)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Call("crash", nil)
	if _, plain := err.(*wittymodule.CallError); err == nil || plain {
		t.Errorf("a crash: %v", err)
	}
	r.Stop()
}

// A helper can read discs, and can't write to the viewer's files.
func TestHelperIsLimited(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the limits on other systems come with their releases")
	}
	c, m, mod, f, srv := setup(t)
	f.release(srv, "0.1.1", nil)
	if err := c.Install(context.Background(), mod, "0.1.1"); err != nil {
		t.Fatal(err)
	}
	ch := m.Choose(mod)
	r, err := modhost.Start(ch.Entry, mod.ID, ch.Version)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	target := filepath.Join(t.TempDir(), "should-not-exist.txt")
	params, _ := json.Marshal(map[string]string{"path": target})
	res, err := r.Call("write", params)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(res.JSON), "true") {
		t.Error("a helper wrote to the viewer's own folder")
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("the file is there")
	}
}

func TestOnlyGitHubOverHTTPS(t *testing.T) {
	c := New(&modhost.Manager{})
	for raw, want := range map[string]bool{
		"https://api.github.com/repos/x": true, "https://github.com/x": true, "https://objects.githubusercontent.com/x": true,
		"http://github.com/x": false, "https://example.com/x": false, "https://github.com.evil.example/x": false, "file:///c:/x": false,
	} {
		if c.allowed(raw) != want {
			t.Errorf("%s: %v", raw, !want)
		}
	}
	if _, ok := versionOf("wittydvdplayer", "v1.2.3"); ok {
		t.Error("the engine's own tags aren't a player module's")
	}
	if v, ok := versionOf("wittydvdplayer", "wittydvdplayer-v1.2.3"); !ok || v != "1.2.3" {
		t.Error(v)
	}
}
