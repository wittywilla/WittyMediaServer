package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// bundleFeed answers as GitHub does, with one signed release of a web bundle.
type bundleFeed struct {
	rels  []update.GHRelease
	files map[string][]byte
}

func (g *bundleFeed) RoundTrip(req *http.Request) (*http.Response, error) {
	b, code := g.files[req.URL.Path], 200
	if req.URL.Host == "api.github.com" {
		b, _ = json.Marshal(g.rels)
	} else if b == nil {
		code = 404
	}
	return &http.Response{StatusCode: code, Status: fmt.Sprint(code), Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Header: http.Header{}, Request: req}, nil
}

func hexSum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// installedBundle makes a manager with a real signed bundle of wittycomics in use.
func installedBundle(t *testing.T, script, style string) *modmgr.Manager {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const id, ver = "wittycomics", "0.2.0"
	raw, _ := json.Marshal(wittymodule.Manifest{ID: id, Name: "WittyComics", Version: ver, Repo: "wittywilla/WittyComics", Delivery: wittymodule.DeliveryWeb, Entry: "index.js", Style: "style.css", API: 2, MinServer: "0.4.0", MaxServer: "0.5.x"})
	top := wittymodule.WebTopFolder(id, ver)
	files := map[string][]byte{wittymodule.ManifestFile: raw, "index.js": []byte(script), "style.css": []byte(style)}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	var list strings.Builder
	for n, b := range files {
		if err := tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(b))}); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write(b)
		fmt.Fprintf(&list, "%s  %s/%s\n", hexSum(b), top, n)
	}
	_ = tw.Close()
	_ = gz.Close()
	archive, manName, archiveName, sumsName := buf.Bytes(), wittymodule.ManifestName(id, ver), wittymodule.WebArchiveName(id, ver), wittymodule.SumsName(id, ver)
	fmt.Fprintf(&list, "%s  %s\n%s  %s\n", hexSum(archive), archiveName, hexSum(raw), manName)
	sums := []byte(list.String())
	feed := &bundleFeed{files: map[string][]byte{}}
	rel := update.GHRelease{TagName: "v" + ver}
	base := "/wittywilla/WittyComics/releases/download/v" + ver + "/"
	for n, b := range map[string][]byte{archiveName: archive, manName: raw, sumsName: sums, sumsName + ".sig": update.Sign(key, sums)} {
		feed.files[base+n] = b
		rel.Assets = append(rel.Assets, struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
			URL  string `json:"browser_download_url"`
		}{n, int64(len(b)), "https://github.com" + base + n})
	}
	feed.rels = []update.GHRelease{rel}
	builtin, _ := json.Marshal(wittymodule.Manifest{ID: id, Name: "WittyComics", Version: "0.1.0", Repo: "wittywilla/WittyComics", Delivery: wittymodule.DeliveryWeb, API: 2, MinServer: "0.1.0", MaxServer: "0.x"})
	m := modmgr.New(modmgr.Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: "0.4.0", Builtins: [][]byte{builtin}, Keys: []ed25519.PublicKey{pub},
		Fetch: &update.Fetcher{UserAgent: "test", Transport: feed}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	t.Cleanup(m.Close)
	m.Check(context.Background(), 0)
	if err := m.Install(id); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(30 * time.Second); len(m.WebBundles()) == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the bundle never came into use: %+v", m.Report())
		}
	}
	return m
}

// findBrowser looks for something that can load a page for real. WMS_TEST_BROWSER names
// one; without any the part of the test that needs it is skipped.
func findBrowser() string {
	if v := os.Getenv("WMS_TEST_BROWSER"); v != "" {
		return v
	}
	for _, n := range []string{"chrome", "google-chrome", "chromium", "chromium-browser", "msedge"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	if runtime.GOOS == "windows" {
		for _, p := range []string{`C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// A bundle's script answers with the API's policy, default-src 'none' and sandbox, while
// the page that loads it has the app's. A browser applies a script's own headers to
// nothing but its type, so it must still run: this loads one for real, under the real
// policy and through every layer of the real server. The app's reader depends on it.
func TestModuleScriptLoadsUnderThePagePolicy(t *testing.T) {
	ts := newTestServer(t)
	const script = `export const marker = 'script ran'; export default 'bundle';`
	const style = `#probe { color: rgb(1, 2, 3); }`
	ts.Modules = installedBundle(t, script, style)
	_, tok := ts.user(t, "reader", "adult", false, "")
	const base = "/api/modules/wittycomics/web/0.2.0/"

	// What a browser insists on, and the policy that this test is about.
	w := ts.do(t, "GET", base+"index.js", tok, nil)
	if w.Code != 200 || w.Body.String() != script {
		t.Fatalf("script: %d %q", w.Code, w.Body.String())
	}
	h := w.Header()
	if got := h.Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Errorf("script type %q", got)
	}
	if got := h.Get("Content-Security-Policy"); got != apiCSP {
		t.Errorf("opening a bundle's script directly must not be able to run it, got policy %q", got)
	}
	if h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Cache-Control"), "immutable") {
		t.Errorf("headers: %v", h)
	}
	if got := ts.do(t, "GET", base+"style.css", tok, nil).Header().Get("Content-Type"); got != "text/css; charset=utf-8" {
		t.Errorf("stylesheet type %q", got)
	}

	// A script can't send a header, so its address may carry the sign-in like a poster's.
	// Nothing else about modules takes it there.
	for _, c := range []struct {
		target string
		code   int
	}{
		{base + "index.js?token=" + tok, 200},
		{base + "index.js", 401},
		{base + "module.json?token=" + tok, 404},
		{"/api/modules/web?token=" + tok, 401},
	} {
		if w := ts.do(t, "GET", c.target, "", nil); w.Code != c.code {
			t.Errorf("%s: %d, want %d", strings.ReplaceAll(c.target, tok, "TOKEN"), w.Code, c.code)
		}
	}

	if testing.Short() {
		t.Skip("loading it for real takes a browser and some time")
	}
	browser := findBrowser()
	if browser == "" {
		t.Skip("no browser here to load it for real; set WMS_TEST_BROWSER")
	}
	ts.Web = fstest.MapFS{
		// The inline script must not run, which shows the app's policy is really in force.
		"index.html": {Data: []byte(`<!doctype html><html><body><div id="probe">probe</div><pre id="out">waiting</pre>` +
			`<script>document.documentElement.setAttribute('data-inline', 'ran')</script><script type="module" src="/probe.js"></script></body></html>`)},
		"probe.js": {Data: []byte(`
			const tok = new URLSearchParams(location.search).get('t'), q = '?token=' + encodeURIComponent(tok), out = document.getElementById('out')
			try {
				const m = await import('` + base + `index.js' + q)
				await new Promise((ok, no) => {
					const l = document.createElement('link')
					l.rel = 'stylesheet'; l.href = '` + base + `style.css' + q; l.onload = ok; l.onerror = () => no(new Error('stylesheet refused'))
					document.head.appendChild(l)
				})
				out.textContent = 'RESULT ' + m.marker + ' ' + m.default + ' ' + getComputedStyle(document.getElementById('probe')).color + ' inline=' + (document.documentElement.getAttribute('data-inline') || 'blocked')
			} catch (e) { out.textContent = 'RESULT failed: ' + e }
		`)},
	}
	srv := httptest.NewServer(ts.Server.Handler())
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	profile, err := os.MkdirTemp("", "wms-browser-*") // not t.TempDir: a browser's helpers can outlive it for a moment
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(profile)
	out, err := exec.CommandContext(ctx, browser, "--headless=new", "--no-sandbox", "--disable-gpu", "--no-first-run", "--disable-extensions", "--user-data-dir="+profile,
		"--virtual-time-budget=10000", "--dump-dom", srv.URL+"/?t="+url.QueryEscape(tok)).Output()
	if err != nil && !bytes.Contains(out, []byte("RESULT")) {
		t.Skipf("this browser wouldn't run: %v", err)
	}
	const want = "RESULT script ran bundle rgb(1, 2, 3) inline=blocked"
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("want %q in the page:\n%s", want, strings.ReplaceAll(string(out), tok, "TOKEN"))
	}
}

// A browser's report reaches the server for anyone signed in, and only them.
func TestModuleFailureReportRoute(t *testing.T) {
	ts := newTestServer(t)
	ts.Modules = installedBundle(t, `export default 1`, `a{}`)
	_, tok := ts.user(t, "reader", "adult", false, "")
	if w := ts.do(t, "POST", "/api/modules/wittycomics/failed", "", map[string]string{"version": "0.2.0", "reason": "x"}); w.Code != 401 {
		t.Errorf("signed out: %d", w.Code)
	}
	if w := ts.do(t, "POST", "/api/modules/wittycomics/failed", tok, map[string]string{"version": "0.2.0", "reason": "x"}); w.Code != 204 {
		t.Errorf("signed in: %d %s", w.Code, w.Body.String())
	}
}
