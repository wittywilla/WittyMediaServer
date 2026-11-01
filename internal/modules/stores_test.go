package modules

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymodule"
)

// fakeNet answers for addresses by their whole text, as the internet would, with no sockets.
type fakeNet struct {
	mu    sync.Mutex
	files map[string][]byte
	asked []string
	wrap  func(u string, res *http.Response) *http.Response
}

func (f *fakeNet) RoundTrip(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := r.URL.String()
	f.asked = append(f.asked, u)
	b, ok := f.files[u]
	res := &http.Response{StatusCode: 200, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Request: r}
	if !ok {
		res.StatusCode, res.Status = 404, "404 Not Found"
	}
	if f.wrap != nil {
		res = f.wrap(u, res)
	}
	return res, nil
}

func (f *fakeNet) put(u string, b []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.files == nil {
		f.files = map[string][]byte{}
	}
	f.files[u] = b
}

func (f *fakeNet) asks(part string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.asked {
		if strings.Contains(u, part) {
			n++
		}
	}
	return n
}

const storeURL = "https://store.example/modules/catalog.json"

func testKey(t *testing.T) (string, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(pub), priv
}

func signed(key ed25519.PrivateKey, raw []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw)) + "\n")
}

func testStoreCatalog(serial int64, mods ...wittymodule.CatalogModule) []byte {
	c := wittymodule.Catalog{Format: wittymodule.CatalogFormat, Name: "A store", Serial: serial, Generated: "2026-11-01T12:00:00Z", Modules: mods}
	if c.Modules == nil {
		c.Modules = []wittymodule.CatalogModule{}
	}
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	return b
}

func listedModule(id, name string) wittymodule.CatalogModule {
	return wittymodule.CatalogModule{ID: id, Name: name, Summary: "Does a thing.", Publisher: wittymodule.Publisher{Name: "Someone"}, Repo: "someone/" + id}
}

// publishStore puts a signed catalogue at the store's address.
func publishStore(n *fakeNet, key ed25519.PrivateKey, pub string, serial int64, mods ...wittymodule.CatalogModule) {
	raw := testStoreCatalog(serial, mods...)
	n.put(storeURL, raw)
	n.put(storeURL+".sig", signed(key, raw))
	if pub != "" {
		n.put(storeURL+".key", []byte(pub+"\n"))
	}
}

// storesWorld is a server with no modules but those a catalogue in its starter folder lists.
type storesWorld struct {
	m       *Manager
	s       *Stores
	net     *fakeNet
	dir     string
	starter string
	key     ed25519.PrivateKey
	release string // files for publishing
}

func newStoresWorld(t *testing.T, mods ...wittymodule.CatalogModule) *storesWorld {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	w := &storesWorld{net: &fakeNet{}, dir: filepath.Join(t.TempDir(), "modules"), starter: filepath.Join(t.TempDir(), "starter"), key: key}
	if err := os.MkdirAll(w.starter, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := testStoreCatalog(1, mods...)
	write := func(name string, b []byte) {
		if err := os.WriteFile(filepath.Join(w.starter, name), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(wittymodule.CatalogName, raw)
	write(wittymodule.CatalogName+".sig", signed(key, raw))
	w.m = New(Config{Dir: w.dir, Server: "0.6.0-alpha", Starter: w.starter, Keys: []ed25519.PublicKey{pub}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(w.m.Close)
	w.s = NewStores(StoresConfig{Manager: w.m, Dir: w.dir, Transport: w.net, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	return w
}

// webRelease writes the signed files of a web bundle into the starter folder, and says where they
// are, for a catalogue. A module with no process needs no build of anything.
func webRelease(t *testing.T, starter string, key ed25519.PrivateKey, id, name, repo, ver string, requires ...string) wittymodule.CatalogVersion {
	t.Helper()
	man := wittymodule.Manifest{ID: id, Name: name, Version: ver, Repo: repo, Delivery: wittymodule.DeliveryWeb, Entry: "index.js", API: wittymodule.API, MinServer: MinModuleServer, MaxServer: "0.6.x", License: "MIT", Requires: requires}
	raw, _ := json.Marshal(man)
	files := map[string][]byte{wittymodule.ManifestFile: raw, "index.js": []byte("export const v = '" + ver + "'\n")}
	top := wittymodule.WebTopFolder(id, ver)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for n, b := range files {
		_ = tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(b))})
		_, _ = tw.Write(b)
	}
	_ = tw.Close()
	_ = gz.Close()
	archive := buf.Bytes()
	name2 := wittymodule.WebArchiveName(id, ver)
	manName := wittymodule.ManifestName(id, ver)
	var list strings.Builder
	fmt.Fprintf(&list, "%s  %s\n%s  %s\n", sum(archive), name2, sum(raw), manName)
	for n, b := range files {
		fmt.Fprintf(&list, "%s  %s/%s\n", sum(b), top, n)
	}
	sums := []byte(list.String())
	sumsName := wittymodule.SumsName(id, ver)
	dir := filepath.Join(starter, id, ver)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for n, b := range map[string][]byte{name2: archive, manName: raw, sumsName: sums, sumsName + ".sig": signed(key, sums)} {
		if err := os.WriteFile(filepath.Join(dir, n), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	at := func(n string) string { return id + "/" + ver + "/" + n }
	return wittymodule.CatalogVersion{Version: ver, Delivery: wittymodule.DeliveryWeb, API: wittymodule.API, MinServer: MinModuleServer, MaxServer: "0.6.x", License: "MIT", Released: "2026-11-01",
		Sums: at(sumsName), Sig: at(sumsName + ".sig"), Manifest: at(manName),
		Platforms: map[string]wittymodule.CatalogFile{wittymodule.PlatformAll: {URL: at(name2), Size: int64(len(archive)), SHA256: sum(archive)}}}
}

func TestAddingAStoreStepByStep(t *testing.T) {
	w := newStoresWorld(t, listedModule("wittydvd", "WittyDVD"))
	pub, key := testKey(t)
	publishStore(w.net, key, pub, 3, listedModule("wittydvd", "WittyDVD"), listedModule("wittytuner", "WittyTuner"), listedModule("bigclock", "Big Clock"))
	ctx := context.Background()

	p, err := w.s.Preview(ctx, storeURL, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if p.KeySource != "offered" || p.Key != pub || p.Fingerprint == "" || p.Count != 3 || len(p.Warning) == 0 {
		t.Fatalf("preview: %+v", p)
	}
	if len(w.s.List()) != 0 {
		t.Fatal("a preview kept something")
	}
	kinds := map[string]string{}
	for _, mod := range p.Modules {
		kinds[mod.ID] = mod.Kind
	}
	if kinds["wittydvd"] != KindFork || kinds["wittytuner"] != KindRefused || kinds["bigclock"] != KindDev {
		t.Fatalf("kinds: %v", kinds)
	}
	info, err := w.s.Add(ctx, storeURL, p.Key, false, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if info.Fingerprint != p.Fingerprint || !info.Enabled || info.Modules != 3 {
		t.Fatalf("added: %+v", info)
	}
	if _, err := w.s.Add(ctx, storeURL, p.Key, false, "admin"); err == nil {
		t.Fatal("added twice")
	}
	// It comes back after a restart, checked again.
	again := NewStores(StoresConfig{Manager: w.m, Dir: w.dir, Transport: w.net})
	if l := again.List(); len(l) != 1 || l[0].Modules != 3 {
		t.Fatalf("after a restart: %+v", l)
	}
}

func TestPreviewNeedsAKeyFromSomewhere(t *testing.T) {
	w := newStoresWorld(t)
	pub, key := testKey(t)
	publishStore(w.net, key, "", 1)
	if _, err := w.s.Preview(context.Background(), storeURL, "", false); !errors.As(err, new(*NeedKey)) {
		t.Fatalf("no key anywhere: %v", err)
	}
	p, err := w.s.Preview(context.Background(), storeURL, pub, false)
	if err != nil || p.KeySource != "pasted" {
		t.Fatalf("pasted: %v %+v", err, p)
	}
	other, _ := testKey(t)
	if _, err := w.s.Preview(context.Background(), storeURL, other, false); !errors.Is(err, wittymodule.ErrCatalogSignature) {
		t.Fatalf("the wrong key: %v", err)
	}
	w.net.put(storeURL+".key", []byte("not a key"))
	if _, err := w.s.Preview(context.Background(), storeURL, "", false); !errors.As(err, new(*NeedKey)) {
		t.Fatalf("a nonsense key: %v", err)
	}
}

// Everything a store could send instead of a good catalogue, none of which may change anything.
func TestHostileCatalogues(t *testing.T) {
	pub, key := testKey(t)
	_, otherKey := testKey(t)
	good := testStoreCatalog(5, listedModule("wittydvd", "WittyDVD"))
	for name, c := range map[string]struct {
		cat, sig []byte
		wantKind string
	}{
		"not json, signed":           {[]byte("{nope"), signed(key, []byte("{nope")), FailOther},
		"garbage, unsigned":          {[]byte("{nope"), []byte("garbage"), FailKey},
		"another key's signature":    {good, signed(otherKey, good), FailKey},
		"changed after signing":      {append(append([]byte(nil), good...), ' '), signed(key, good), FailKey},
		"an empty signature":         {good, []byte(""), FailKey},
		"a newer format":             {[]byte(`{"format":99,"name":"x","serial":9,"generated":"2026-11-01T12:00:00Z","modules":[]}`), signed(key, []byte(`{"format":99,"name":"x","serial":9,"generated":"2026-11-01T12:00:00Z","modules":[]}`)), FailTooNew},
		"an older serial":            {testStoreCatalog(4), signed(key, testStoreCatalog(4)), FailSignature},
		"the same serial, different": {testStoreCatalog(5), signed(key, testStoreCatalog(5)), FailSignature},
	} {
		t.Run(name, func(t *testing.T) {
			w := newStoresWorld(t)
			publishStore(w.net, key, pub, 5, listedModule("wittydvd", "WittyDVD"))
			if _, err := w.s.Add(context.Background(), storeURL, pub, false, "admin"); err != nil {
				t.Fatal(err)
			}
			w.net.put(storeURL, c.cat)
			w.net.put(storeURL+".sig", c.sig)
			if err := w.s.Refresh(context.Background(), w.s.List()[0].ID); err == nil {
				t.Fatal("it was accepted")
			}
			info := w.s.List()[0]
			if info.Problem == nil || info.Problem.Kind != c.wantKind {
				t.Fatalf("problem: %+v, want %s", info.Problem, c.wantKind)
			}
			if info.Modules != 1 || info.Serial != 5 {
				t.Fatalf("the last good catalogue should stay: %+v", info)
			}
		})
	}
}

func TestChangingKeyIsARefusal(t *testing.T) {
	w := newStoresWorld(t)
	pub, key := testKey(t)
	publishStore(w.net, key, pub, 1, listedModule("wittydvd", "WittyDVD"))
	ctx := context.Background()
	if _, err := w.s.Add(ctx, storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	id := w.s.List()[0].ID
	pinned := w.s.List()[0].Fingerprint

	newPub, newKey := testKey(t)
	publishStore(w.net, newKey, newPub, 2, listedModule("wittydvd", "WittyDVD"), listedModule("wittyphotos", "WittyPhotos"))
	if err := w.s.Refresh(ctx, id); err == nil {
		t.Fatal("a new key was accepted")
	}
	info := w.s.List()[0]
	if info.Problem == nil || info.Problem.Kind != FailKey {
		t.Fatalf("problem: %+v", info.Problem)
	}
	if info.Fingerprint != pinned || info.Serial != 1 || info.Modules != 1 {
		t.Fatalf("the pinned key and the last good list should stay: %+v", info)
	}
	if !strings.Contains(info.Problem.Detail, pinned) {
		t.Fatalf("it should name the key that was agreed to: %q", info.Problem.Detail)
	}
	if err := w.s.UseFork(id, "wittydvd", "admin", true); err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("nothing should install from it now: %v", err)
	}
	// It stays refused however many times it looks, and survives a restart.
	if err := w.s.Refresh(ctx, id); err == nil {
		t.Fatal("accepted the second time")
	}
	again := NewStores(StoresConfig{Manager: w.m, Dir: w.dir, Transport: w.net})
	if p := again.List()[0].Problem; p == nil || p.Kind != FailKey {
		t.Fatalf("after a restart: %+v", p)
	}
}

func TestStoreAddressRules(t *testing.T) {
	for _, bad := range []string{
		"http://store.example/catalog.json", "ftp://store.example/catalog.json", "file:///etc/passwd", "catalog.json", "",
		"https://user:pass@store.example/catalog.json", "https://store.example/catalog.json#x", "https://store.example/a b",
		"https://127.0.0.1/catalog.json", "https://localhost/catalog.json", "https://10.0.0.8/catalog.json", "https://192.168.1.2/catalog.json",
		"https://169.254.169.254/latest/meta-data", "https://[::1]/catalog.json", "https://[fe80::1]/catalog.json", "https://nas.local/catalog.json", "https://nas/catalog.json", "https://0.0.0.0/catalog.json", "https://100.64.0.1/catalog.json",
	} {
		if _, err := checkStoreURL(bad, false); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
	if _, err := checkStoreURL("https://store.example/modules/catalog.json", false); err != nil {
		t.Error(err)
	}
	// Allowed by the admin for this one store, an address on their own network works, and plain
	// http with it, but plain http to the internet still doesn't.
	if _, err := checkStoreURL("http://192.168.1.2:8000/catalog.json", true); err != nil {
		t.Error(err)
	}
	if _, err := checkStoreURL("http://store.example/catalog.json", true); err == nil {
		t.Error("http to the internet was accepted")
	}
	if _, err := checkStoreURL("file:///etc/passwd", true); err == nil {
		t.Error("a file address was accepted")
	}
}

func TestStoreRedirectsAreHeldToTheSameRules(t *testing.T) {
	for _, to := range []string{"http://store.example/catalog.json", "https://127.0.0.1/catalog.json", "https://169.254.169.254/", "file:///etc/passwd"} {
		n := &fakeNet{wrap: func(u string, res *http.Response) *http.Response {
			res.StatusCode, res.Status = 302, "302 Found"
			res.Header.Set("Location", to)
			return res
		}}
		f := newStoreFetcher(n, false)
		if _, err := f.small(context.Background(), storeURL, 1024); err == nil {
			t.Errorf("a redirect to %s was followed", to)
		}
		if n.asks("127.0.0.1") > 0 || n.asks("169.254") > 0 {
			t.Errorf("it asked: %v", n.asked)
		}
	}
}

func TestStoreLimitsAreEnforcedWhileReading(t *testing.T) {
	// An answer that doesn't say how long it is, and then goes on and on.
	n := &fakeNet{wrap: func(u string, res *http.Response) *http.Response {
		res.ContentLength = -1
		res.Body = io.NopCloser(io.LimitReader(zeroes{}, 1<<30))
		return res
	}}
	n.put(storeURL, nil)
	f := newStoreFetcher(n, false)
	start := time.Now()
	_, err := f.small(context.Background(), storeURL, 1024)
	var big *errStoreTooBig
	if !errors.As(err, &big) {
		t.Fatalf("over the limit: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("it read much more than it had to")
	}
	// An answer that says at once.
	n = &fakeNet{wrap: func(u string, res *http.Response) *http.Response { res.ContentLength = 1 << 40; return res }}
	n.put(storeURL, nil)
	if _, err := newStoreFetcher(n, false).small(context.Background(), storeURL, 1024); !errors.As(err, &big) {
		t.Fatalf("a large length: %v", err)
	}
}

type zeroes struct{}

func (zeroes) Read(b []byte) (int, error) { clear(b); return len(b), nil }

func TestLocalTransportRefusesLinkLocal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hello")) }))
	defer ts.Close()
	f := newStoreFetcher(nil, true)
	if b, err := f.small(context.Background(), ts.URL+"/x", 64); err != nil || string(b) != "hello" {
		t.Fatalf("a store on this machine, when allowed: %q %v", b, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := f.small(ctx, "http://[fe80::1]:9/x", 64); err == nil {
		t.Fatal("link-local was reached")
	}
	if _, err := f.small(ctx, "http://169.254.169.254:9/x", 64); err == nil {
		t.Fatal("link-local was reached")
	}
}

func TestStoreWordsAreCleaned(t *testing.T) {
	w := newStoresWorld(t, listedModule("wittyphotos", "WittyPhotos"))
	pub, key := testKey(t)
	mod := listedModule("bidiname", "Clean‮evil‬ Name")
	mod.Summary = "Has ​invisible⁦ parts\u0085"
	mod.Description = "<script>alert(1)</script>"
	look := listedModule("wittyphotox", "WITTYPHOTOS")
	publishStore(w.net, key, pub, 1, mod, look)
	if _, err := w.s.Add(context.Background(), storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	var clean, lookalike *StoreItem
	v := w.s.View()
	for i := range v.Items {
		switch v.Items[i].ID {
		case "bidiname":
			clean = &v.Items[i]
		case "wittyphotox":
			lookalike = &v.Items[i]
		}
	}
	if clean == nil || lookalike == nil {
		t.Fatalf("not listed: %+v", v.Items)
	}
	if clean.Name != "Cleanevil Name" || clean.Summary != "Has invisible parts" {
		t.Fatalf("not cleaned: %q %q", clean.Name, clean.Summary)
	}
	if clean.Official || clean.Kind != KindDev || clean.Fit.OK || clean.Fit.Kind != KindDev {
		t.Fatalf("a new module can't be offered as if it could be installed: %+v", clean)
	}
	if lookalike.Lookalike != "wittyphotos" {
		t.Fatalf("a module using another's name should say so: %+v", lookalike)
	}
}

func TestForkRules(t *testing.T) {
	w := newStoresWorld(t, listedModule("wittydvd", "WittyDVD"), listedModule("wittytuner", "WittyTuner"))
	pub, key := testKey(t)
	publishStore(w.net, key, pub, 1, listedModule("wittytuner", "WittyTuner"), listedModule("bigclock", "Big Clock"), listedModule("wittydvd", "WittyDVD"))
	if _, err := w.s.Add(context.Background(), storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	id := w.s.List()[0].ID
	if err := w.s.UseFork(id, "wittytuner", "admin", true); err == nil || !strings.Contains(err.Error(), "encrypted") {
		t.Fatalf("WittyTuner can't be replaced: %v", err)
	}
	if err := w.s.UseFork(id, "bigclock", "admin", true); err == nil || !strings.Contains(err.Error(), "development") {
		t.Fatalf("a new module can't be installed: %v", err)
	}
	var need *ErrNeedsConfirm
	if err := w.s.UseFork(id, "wittydvd", "admin", false); !errors.As(err, &need) || !strings.Contains(need.Warning, "fingerprint") {
		t.Fatalf("a fork asks first: %v", err)
	}
	if s, _ := w.s.statusOf("wittydvd"); s.Source != nil {
		t.Fatal("asking changed something")
	}
	if err := w.s.UseFork(id, "wittydvd", "admin", true); err == nil {
		t.Fatal("it has no version to use, so this should have said so")
	}
	if s, _ := w.s.statusOf("wittydvd"); s.Source != nil {
		t.Fatal("a fork with nothing to offer shouldn't have replaced the module")
	}
	// A store that's off offers nothing.
	if err := w.s.SetEnabled(id, false, "admin"); err != nil {
		t.Fatal(err)
	}
	if err := w.s.UseFork(id, "wittydvd", "admin", true); err == nil {
		t.Fatal("a store that's off was used")
	}
	for _, it := range w.s.View().Items {
		if it.Store == id {
			t.Fatalf("a store that's off is still listed: %+v", it)
		}
	}
}

func TestFitSaysWhy(t *testing.T) {
	base := wittymodule.CatalogVersion{Version: "1.0.0", Delivery: wittymodule.DeliveryWeb, API: wittymodule.API, MinServer: MinModuleServer, MaxServer: "0.6.x", License: "MIT", Released: "2026-11-01",
		Platforms: map[string]wittymodule.CatalogFile{wittymodule.PlatformAll: {URL: "a", Size: 1, SHA256: strings.Repeat("a", 64)}}}
	check := func(name, want string, change func(v *wittymodule.CatalogVersion), words ...string) {
		t.Helper()
		v := base
		change(&v)
		f := fitOf("Thing", &v, "0.6.0-alpha")
		if f.Kind != want {
			t.Errorf("%s: %+v, want %s", name, f, want)
		}
		for _, w := range words {
			if !strings.Contains(f.Why, w) {
				t.Errorf("%s: %q should say %q", name, f.Why, w)
			}
		}
	}
	check("a fit", FitOK, func(v *wittymodule.CatalogVersion) {})
	check("from before the cut-off, claiming to fit", FitTooOld, func(v *wittymodule.CatalogVersion) { v.MinServer = "0.5.0-alpha" }, "built for an older server", "beta")
	check("past its maximum", FitTooOld, func(v *wittymodule.CatalogVersion) { v.MinServer, v.MaxServer = "0.5.0-alpha", "0.5.x" }, "built for an older server")
	check("needs a newer server", FitTooNew, func(v *wittymodule.CatalogVersion) { v.MinServer, v.MaxServer = "0.7.0", "0.7.x" }, "0.7.0")
	check("needs a newer API", FitTooNew, func(v *wittymodule.CatalogVersion) { v.API = wittymodule.API + 1 }, "newer server")
	check("an unknown permission", FitTooNew, func(v *wittymodule.CatalogVersion) { v.Permissions = []string{"everything"} }, "everything")
	check("no build", FitNoBuild, func(v *wittymodule.CatalogVersion) {
		v.Delivery = wittymodule.DeliveryHelper
		v.Platforms = map[string]wittymodule.CatalogFile{"plan9-amd64": {URL: "a", Size: 1, SHA256: strings.Repeat("a", 64)}}
	}, "this kind of computer")
	check("withdrawn", FitWithdrawn, func(v *wittymodule.CatalogVersion) {
		v.Withdrawn = &wittymodule.CatalogWithdrawn{Severity: "security", Reason: "it leaks"}
	}, "it leaks")
}

func TestOrderByNeeds(t *testing.T) {
	needs := map[string][]string{"comics": {"ink"}, "ebook": {"ink"}, "ink": {"core"}, "core": nil, "loopa": {"loopb"}, "loopb": {"loopa"}}
	got := orderByNeeds([]string{"ebook", "comics", "ink", "photos", "loopa", "loopb"}, func(id string) []string { return needs[id] })
	at := map[string]int{}
	for i, id := range got {
		at[id] = i
	}
	if len(got) != 6 {
		t.Fatalf("%v", got)
	}
	if at["ink"] > at["comics"] || at["ink"] > at["ebook"] {
		t.Fatalf("what they're built on comes first: %v", got)
	}
}

func waitRun(t *testing.T, s *Stores) *Run {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if r := s.CurrentRun(); r != nil && r.Finished != 0 {
			return r
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the run never finished")
	return nil
}

// A catalogue with a chain of modules, one of them broken, all from the starter folder.
func TestInstallAndUpdateAll(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(nil)
	dir := filepath.Join(t.TempDir(), "modules")
	starter := filepath.Join(t.TempDir(), "starter")
	if err := os.MkdirAll(starter, 0o755); err != nil {
		t.Fatal(err)
	}
	mod := func(id, name string, vers []wittymodule.CatalogVersion, requires ...string) wittymodule.CatalogModule {
		m := listedModule(id, name)
		m.Repo = "wittywilla/" + name
		m.Requires = requires
		m.Versions = vers
		return m
	}
	rel := func(id, name, ver string, requires ...string) wittymodule.CatalogVersion {
		return webRelease(t, starter, key, id, name, "wittywilla/"+name, ver, requires...)
	}
	publish := func(serial int64, mods ...wittymodule.CatalogModule) {
		raw := testStoreCatalog(serial, mods...)
		_ = os.WriteFile(filepath.Join(starter, wittymodule.CatalogName), raw, 0o644)
		_ = os.WriteFile(filepath.Join(starter, wittymodule.CatalogName+".sig"), signed(key, raw), 0o644)
	}
	ink1, comics1, ebook1 := rel("wittyink", "WittyInk", "0.2.0"), rel("wittycomics", "WittyComics", "0.2.0", "wittyink"), rel("wittyebook", "WittyEbook", "0.2.0", "wittyink")
	publish(1, mod("wittyink", "WittyInk", []wittymodule.CatalogVersion{ink1}), mod("wittycomics", "WittyComics", []wittymodule.CatalogVersion{comics1}, "wittyink"), mod("wittyebook", "WittyEbook", []wittymodule.CatalogVersion{ebook1}, "wittyink"))
	cfg := Config{Dir: dir, Server: "0.6.0-alpha", Starter: starter, Keys: []ed25519.PublicKey{pub}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	m := New(cfg)
	s := NewStores(StoresConfig{Manager: m, Dir: dir, Log: cfg.Log})

	// Installing what's built on something explains that first, then brings both, in order.
	plan, err := s.InstallPlan("wittycomics")
	if err != nil || len(plan) != 2 || plan[0].ID != "wittyink" || plan[1].ID != "wittycomics" || plan[0].Why == "" {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	if _, err := s.InstallWithNeeds("wittycomics", "admin"); err != nil {
		t.Fatal(err)
	}
	r := waitRun(t, s)
	if r.Updated != 2 || r.Failed != 0 {
		t.Fatalf("install: %+v", r)
	}
	if st, _ := s.statusOf("wittyink"); !st.Installed || st.Version != "0.2.0" {
		t.Fatalf("the dependency: %+v", st)
	}
	if _, err := s.InstallWithNeeds("wittyebook", "admin"); err != nil {
		t.Fatal(err)
	}
	waitRun(t, s)

	// A new list: everything has an update, and one of them is broken.
	ink2, comics2, ebook2 := rel("wittyink", "WittyInk", "0.3.0"), rel("wittycomics", "WittyComics", "0.3.0", "wittyink"), rel("wittyebook", "WittyEbook", "0.3.0", "wittyink")
	broken := ebook2
	f := broken.Platforms[wittymodule.PlatformAll]
	f.SHA256 = strings.Repeat("b", 64)
	broken.Platforms = map[string]wittymodule.CatalogFile{wittymodule.PlatformAll: f}
	publish(2, mod("wittyink", "WittyInk", []wittymodule.CatalogVersion{ink2, ink1}), mod("wittycomics", "WittyComics", []wittymodule.CatalogVersion{comics2, comics1}, "wittyink"), mod("wittyebook", "WittyEbook", []wittymodule.CatalogVersion{broken, ebook1}, "wittyink"))
	m.Close()
	m = New(cfg)
	defer m.Close()
	s = NewStores(StoresConfig{Manager: m, Dir: dir, Log: cfg.Log})
	if n := s.UpdateCount(); n != 3 {
		t.Fatalf("updates: %d", n)
	}
	r, err = s.UpdateAll("admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateAll("admin"); !errors.Is(err, ErrRunBusy) && r.Finished == 0 {
		t.Fatalf("two at once: %v", err)
	}
	r = waitRun(t, s)
	if r.Updated != 2 || r.Failed != 1 {
		t.Fatalf("one failure doesn't stop the rest: %+v", r)
	}
	if r.Items[0].ID != "wittyink" {
		t.Fatalf("what the rest are built on goes first: %+v", r.Items)
	}
	for _, it := range r.Items {
		if it.ID == "wittyebook" {
			if it.State != RunFailed || it.Failure == nil || it.Failure.Kind != FailMismatch {
				t.Fatalf("the failure: %+v", it)
			}
		} else if it.State != RunDone {
			t.Fatalf("%+v", it)
		}
	}
	if st, _ := s.statusOf("wittyebook"); st.Version != "0.2.0" {
		t.Fatalf("the failed one stays as it was: %s", st.Version)
	}
	if !r.CanUndo {
		t.Fatal("it should be possible to take it back")
	}
	// The summary survives a restart.
	again := NewStores(StoresConfig{Manager: m, Dir: dir, Log: cfg.Log})
	if l := again.CurrentRun(); l == nil || l.ID != r.ID {
		t.Fatalf("after a restart: %+v", l)
	}

	if _, err := s.UndoRun("admin"); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.statusOf("wittyink"); st.Version != "0.2.0" {
		t.Fatalf("back: %s", st.Version)
	}
	if l := s.CurrentRun(); l.CanUndo || !l.Undone {
		t.Fatalf("taken back: %+v", l)
	}
	if _, err := s.UndoRun("admin"); err == nil {
		t.Fatal("it was taken back twice")
	}
}

// A store's version of a module, from the warning to the module running, and back again.
func TestAForkFromAStore(t *testing.T) {
	w := newStoresWorld(t, listedModule("wittycomics", "WittyComics"))
	pub, key := testKey(t)
	release := filepath.Join(t.TempDir(), "release")
	good := webRelease(t, release, key, "wittycomics", "WittyComics", "someone/wittycomics", "0.9.0")
	// Signed by someone else, though the store lists it: the store's key is what counts.
	_, thief := testKey(t)
	stolen := webRelease(t, release, thief, "wittycomics", "WittyComics", "someone/wittycomics", "0.9.1")
	const base = "https://store.example/modules/"
	for _, v := range []wittymodule.CatalogVersion{good, stolen} {
		for _, rel := range []string{v.Sums, v.Sig, v.Manifest, v.Platforms[wittymodule.PlatformAll].URL} {
			b, err := os.ReadFile(filepath.Join(release, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatal(err)
			}
			w.net.put(base+rel, b)
		}
	}
	mod := listedModule("wittycomics", "WittyComics")
	mod.Versions = []wittymodule.CatalogVersion{good}
	publishStore(w.net, key, pub, 1, mod)
	ctx := context.Background()
	if _, err := w.s.Add(ctx, storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	store := w.s.List()[0].ID

	waitPhase := func() Status {
		t.Helper()
		for i := 0; i < 500; i++ {
			st, _ := w.s.statusOf("wittycomics")
			if st.Phase == PhaseIdle || st.Phase == PhaseFailed {
				return st
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("it never finished")
		return Status{}
	}
	if err := w.s.UseFork(store, "wittycomics", "admin", true); err != nil {
		t.Fatal(err)
	}
	st := waitPhase()
	if st.Version != "0.9.0" || st.Source == nil || st.Source.Store != store || st.Error != "" {
		t.Fatalf("the fork: %+v", st)
	}
	if st.AutoUpdate {
		t.Fatal("a store's module must not update by itself")
	}
	var item *StoreItem
	for _, it := range w.s.View().Items {
		if it.Store == store && it.ID == "wittycomics" {
			it := it
			item = &it
		}
	}
	if item == nil || item.Status == nil || item.Official {
		t.Fatalf("it should be listed as the store's, in use: %+v", item)
	}

	// The store offers an update that isn't signed with its key: refused, and nothing changes.
	mod.Versions = []wittymodule.CatalogVersion{stolen, good}
	publishStore(w.net, key, pub, 2, mod)
	if err := w.s.Refresh(ctx, store); err != nil {
		t.Fatal(err)
	}
	if st, _ := w.s.statusOf("wittycomics"); st.Latest == nil || st.Latest.Version != "0.9.1" {
		t.Fatalf("the new version should be offered: %+v", st.Latest)
	}
	if err := w.m.InstallVersion("wittycomics", "0.9.1", true); err != nil {
		t.Fatal(err)
	}
	st = waitPhase()
	if st.Version != "0.9.0" || st.Phase != PhaseFailed || !strings.Contains(strings.ToLower(st.Error), "signature") {
		t.Fatalf("a release its key didn't sign: %+v", st)
	}
	if f := failureFor(&st); f == nil || f.Kind != FailSignature {
		t.Fatalf("explained as: %+v", f)
	}
	// Update All leaves it alone: an admin chooses each of its versions.
	for _, it := range w.s.updatePlan() {
		if it.ID == "wittycomics" && it.State != RunSkipped {
			t.Fatalf("Update All would change it: %+v", it)
		}
	}

	// A restart brings its versions back from the kept catalogue.
	again := NewStores(StoresConfig{Manager: w.m, Dir: w.dir, Transport: w.net})
	if l, err := w.m.Versions("wittycomics"); err != nil || len(l.Versions) == 0 {
		t.Fatalf("versions after a restart: %+v %v", l, err)
	}

	// Removing the store puts the module back, and asks nothing of a store that's gone.
	if err := again.Remove(store, "admin"); err != nil {
		t.Fatal(err)
	}
	st, _ = again.statusOf("wittycomics")
	if st.Source != nil || st.Installed {
		t.Fatalf("after removing the store: %+v", st)
	}
}

func pngOf(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMediaIsOnlyWhatACatalogueLists(t *testing.T) {
	w := newStoresWorld(t)
	pub, key := testKey(t)
	icon := pngOf(t)
	notPicture := []byte("<svg onload=alert(1)>")
	mod := listedModule("bigclock", "Big Clock")
	mod.Icon = &wittymodule.CatalogFile{URL: "clock.png", Size: int64(len(icon)), SHA256: sum(icon)}
	mod.Screenshots = []wittymodule.CatalogFile{{URL: "s.png", Size: int64(len(notPicture)), SHA256: sum(notPicture)}, {URL: "t.png", Size: int64(len(icon)), SHA256: strings.Repeat("c", 64)}}
	publishStore(w.net, key, pub, 1, mod)
	w.net.put("https://store.example/modules/clock.png", icon)
	w.net.put("https://store.example/modules/s.png", notPicture)
	w.net.put("https://store.example/modules/t.png", icon)
	ctx := context.Background()
	if _, err := w.s.Add(ctx, storeURL, pub, false, "admin"); err != nil {
		t.Fatal(err)
	}
	id := w.s.List()[0].ID
	p, typ, err := w.s.Media(ctx, id, sum(icon))
	if err != nil || typ != "image/png" {
		t.Fatalf("the icon: %v %q", err, typ)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, icon) {
		t.Fatal("it isn't the icon")
	}
	// Kept: a second look asks nobody.
	n := w.net.asks("clock.png")
	if _, _, err := w.s.Media(ctx, id, sum(icon)); err != nil || w.net.asks("clock.png") != n {
		t.Fatalf("not kept: %v %d", err, w.net.asks("clock.png"))
	}
	if _, _, err := w.s.Media(ctx, id, sum(notPicture)); err == nil {
		t.Fatal("something that isn't a picture was served")
	}
	if _, _, err := w.s.Media(ctx, id, strings.Repeat("c", 64)); err == nil {
		t.Fatal("a picture that isn't what the catalogue says was served")
	}
	for _, bad := range []string{strings.Repeat("d", 64), "../../../etc/passwd", ""} {
		if _, _, err := w.s.Media(ctx, id, bad); err == nil {
			t.Fatalf("%q was served", bad)
		}
	}
}

func TestExplain(t *testing.T) {
	for want, words := range map[string][]string{
		FailSignature: {"signature does not verify", "the download of a.tar.gz does not match its signed checksum; it was not installed", "files have changed since they were installed: x.exe is different"},
		FailNetwork:   {"download: dial tcp: lookup x: no such host", "download: context deadline exceeded", "GitHub answered 503 Service Unavailable"},
		FailDisk:      {"write /x: no space left on device", "There is not enough space on the disk."},
		FailTooOld:    {"WittyPlay 0.2.5 is too old for this server: it uses module API 2, and this server needs at least 3", "WittyPlay 0.2.5 supports WittyMediaServer up to 0.5.x, and this server is 0.6.0-alpha"},
		FailTooNew:    {"WittyPlay 9 needs WittyMediaServer 0.9.0 or newer, and this server is 0.6.0-alpha", "WittyPlay 9 needs a newer server: it uses module API 9, and this server has up to 3"},
		FailCrash:     {"this version didn't start on this server, which can happen if the data folder doesn't allow programs to run: exit status 3", "it kept failing (it stopped without answering)"},
		FailCorrupt:   {"unpacking: gzip: invalid header", "the package has no index.js", "download: the file is larger than the release says"},
		FailMismatch:  {"the catalogue and the release disagree: the catalogue says it asks for [], and the release asks for [network]", "the catalogue and the release's signed list disagree about the archive"},
		FailKey:       {"the signing key has changed: you agreed to aaaa, and it now offers bbbb"},
		FailOther:     {"something nobody has seen before"},
	} {
		for _, w := range words {
			f := Explain("Thing", w)
			if f.Kind != want {
				t.Errorf("%q came out as %s, not %s", w, f.Kind, want)
			}
			if f.Title == "" || f.Message == "" || len(f.Try) == 0 || f.Detail != w {
				t.Errorf("%q isn't fully explained: %+v", w, f)
			}
		}
	}
	if Explain("Thing", " ") != nil {
		t.Error("nothing wrong should explain nothing")
	}
}
