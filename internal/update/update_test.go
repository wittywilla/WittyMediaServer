package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	order := []string{"0.2.9", "0.3.0-alpha", "0.3.0-alpha.1", "0.3.0-alpha.2", "0.3.0-alpha.10", "0.3.0-alpha.beta", "0.3.0-beta", "0.3.0-rc.1", "0.3.0", "v0.3.1-alpha", "0.3.1", "0.10.0", "1.0.0"}
	for i := range order {
		for j := range order {
			want := cmpInt(i, j)
			if got := Compare(order[i], order[j]); got != want {
				t.Errorf("Compare(%q, %q) = %d, want %d", order[i], order[j], got, want)
			}
		}
	}
	if Compare("0.3.0+build5", "0.3.0") != 0 {
		t.Error("build metadata should not count")
	}
	for _, bad := range []string{"", "dev", "1.2", "1.2.3.4", "1.2.x", "1.2.3-", "1.2.3-a..b", "-1.2.3"} {
		if Valid(bad) {
			t.Errorf("Valid(%q) = true", bad)
		}
		if Compare(bad, "0.0.1") != -1 {
			t.Errorf("unreadable %q should sort first", bad)
		}
	}
}

func newKey(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

func TestVerifySums(t *testing.T) {
	priv, pub := newKey(t)
	other, otherPub := newKey(t)
	sums := []byte(strings.Repeat("a", 64) + "  wittymediaserver-0.4.0-windows-amd64.zip\n")
	sig := Sign(priv, sums)
	if err := VerifySums(sums, sig, []ed25519.PublicKey{otherPub, pub}); err != nil {
		t.Fatalf("good signature refused: %v", err)
	}
	if VerifySums(sums, Sign(other, sums), []ed25519.PublicKey{pub}) == nil {
		t.Error("signature from an unknown key accepted")
	}
	tampered := bytes.Replace(sums, []byte("aaaa"), []byte("aaab"), 1)
	if VerifySums(tampered, sig, []ed25519.PublicKey{pub}) == nil {
		t.Error("changed checksum list accepted")
	}
	for _, bad := range [][]byte{nil, []byte("not base64!"), []byte(base64.StdEncoding.EncodeToString([]byte("short")))} {
		if VerifySums(sums, bad, []ed25519.PublicKey{pub}) == nil {
			t.Errorf("bad signature %q accepted", bad)
		}
	}
	if VerifySums(sums, sig, nil) == nil {
		t.Error("accepted with no trusted keys")
	}
	if len(TrustedKeys()) == 0 {
		t.Error("no release key is built in")
	}
}

func TestParseSums(t *testing.T) {
	h := strings.Repeat("0f", 32)
	m, err := ParseSums([]byte(h + "  a.zip\n" + strings.ToUpper(h) + " *b.tar.gz\n\n"))
	if err != nil || m["a.zip"] != h || m["b.tar.gz"] != h {
		t.Fatalf("got %v, %v", m, err)
	}
	for _, bad := range []string{"", "xyz  a.zip", h[:10] + "  a.zip", h} {
		if _, err := ParseSums([]byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestAllowedURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://github.com/wittywilla/WittyMediaServer/releases/download/v1/x.zip": true,
		"https://objects.githubusercontent.com/x":                                   true,
		"https://release-assets.githubusercontent.com/x":                            true,
		"https://api.github.com/repos/x":                                            true,
		"http://github.com/x":                                                       false,
		"https://github.com:8443/x":                                                 false,
		"https://user@github.com/x":                                                 false,
		"https://github.com.evil.example/x":                                         false,
		"https://evil.example/github.com":                                           false,
		"https://127.0.0.1/x":                                                       false,
		"file:///etc/passwd":                                                        false,
	} {
		u, _ := url.Parse(raw)
		if got := allowedURL(u); got != want {
			t.Errorf("allowedURL(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestLatest(t *testing.T) {
	asset := func(n string) struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		URL  string `json:"browser_download_url"`
	} {
		return struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
			URL  string `json:"browser_download_url"`
		}{n, 10, "https://github.com/d/" + n}
	}
	rel := func(tag string, draft bool, names ...string) ghRelease {
		g := ghRelease{TagName: tag, Draft: draft, HTMLURL: "https://github.com/x"}
		for _, n := range names {
			g.Assets = append(g.Assets, asset(n))
		}
		return g
	}
	full := func(v string) []string {
		return []string{ArchiveName(v, "windows", "amd64"), SumsName(v), SumsName(v) + ".sig"}
	}
	rels := []ghRelease{
		rel("v0.5.0", true, full("0.5.0")...),                                    // draft
		rel("v0.4.1-alpha", false, ArchiveName("0.4.1-alpha", "linux", "amd64")), // other platform only
		rel("v0.4.0-alpha", false, full("0.4.0-alpha")...),
		rel("v0.3.1-alpha", false, full("0.3.1-alpha")...),
		rel("v0.2.0", false, full("0.2.0")...),
		rel("nonsense", false, full("nonsense")...),
	}
	r := latest(rels, "0.3.0-alpha", "windows", "amd64")
	if r == nil || r.Version != "0.4.0-alpha" || !r.Signed() || r.Title == "" {
		t.Fatalf("latest = %+v", r)
	}
	if latest(rels, "0.4.0-alpha", "windows", "amd64") != nil {
		t.Error("offered a version that is not newer")
	}
	if got := ArchiveName("1.0.0", "linux", "arm"); got != "wittymediaserver-1.0.0-linux-armv7.tar.gz" {
		t.Error(got)
	}
}

// pkgFile is one file of a fake release package.
type pkgFile struct {
	name string
	body string
	mode int64
	link bool
}

func makeZip(t *testing.T, files []pkgFile) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(w, f.body)
	}
	zw.Close()
	return buf.Bytes()
}

func makeTarGz(t *testing.T, files []pkgFile) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range files {
		h := &tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.body)), Typeflag: tar.TypeReg}
		if f.mode == 0 {
			h.Mode = 0o644
		}
		if f.link {
			h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, f.body, 0
		}
		tw.WriteHeader(h)
		if !f.link {
			io.WriteString(tw, f.body)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func writeFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestExtract(t *testing.T) {
	dir := t.TempDir()
	good := []pkgFile{
		{name: "pkg/wms.exe", body: "new wms", mode: 0o755},
		{name: "pkg/bin/ffmpeg.exe", body: "ff"},
		{name: "pkg/data/library.db", body: "EVIL"}, // must be left out
		{name: "pkg/wms.json", body: "{}"},          // must be left out
	}
	for _, kind := range []string{"zip", "tar.gz"} {
		arch := filepath.Join(dir, "p."+kind)
		if kind == "zip" {
			writeFile(t, arch, makeZip(t, good))
		} else {
			writeFile(t, arch, makeTarGz(t, good))
		}
		out := filepath.Join(dir, "out-"+kind)
		if err := Extract(arch, out, 1<<20); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if readFile(t, filepath.Join(out, "wms.exe")) != "new wms" || readFile(t, filepath.Join(out, "bin", "ffmpeg.exe")) != "ff" {
			t.Errorf("%s: files not extracted", kind)
		}
		for _, n := range []string{"data", "wms.json"} {
			if _, err := os.Stat(filepath.Join(out, n)); err == nil {
				t.Errorf("%s: %s was extracted", kind, n)
			}
		}
		if kind == "tar.gz" && runtime.GOOS != "windows" {
			if st, _ := os.Stat(filepath.Join(out, "wms.exe")); st.Mode()&0o100 == 0 {
				t.Error("executable bit lost")
			}
		}
	}

	bad := map[string][]pkgFile{
		"climb":     {{name: "pkg/../../evil", body: "x"}},
		"absolute":  {{name: "/etc/evil", body: "x"}},
		"backslash": {{name: "pkg\\..\\evil", body: "x"}},
		"drive":     {{name: "pkg/C:evil", body: "x"}},
		"two tops":  {{name: "a/x", body: "x"}, {name: "b/y", body: "y"}},
		"top file":  {{name: "loose", body: "x"}},
		"too large": {{name: "pkg/big", body: strings.Repeat("x", 2<<20)}},
	}
	for name, files := range bad {
		arch := filepath.Join(dir, "bad.zip")
		writeFile(t, arch, makeZip(t, files))
		if err := Extract(arch, filepath.Join(dir, "bad-"+strings.ReplaceAll(name, " ", "")), 1<<20); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	arch := filepath.Join(dir, "link.tar.gz")
	writeFile(t, arch, makeTarGz(t, []pkgFile{{name: "pkg/wms", body: "/etc/passwd", link: true}}))
	if err := Extract(arch, filepath.Join(dir, "link"), 1<<20); err == nil {
		t.Error("symlink accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "evil")); err == nil {
		t.Error("a file was written outside the folder")
	}
}

// install makes a program folder as a release package leaves it, run in
// place: its data folder holds the library.
func install(t *testing.T) string {
	dir := filepath.Join(t.TempDir(), "WittyMediaServer")
	writeFile(t, filepath.Join(dir, "wms.exe"), []byte("old wms"))
	writeFile(t, filepath.Join(dir, "wms"), []byte("old wms"))
	writeFile(t, filepath.Join(dir, "LICENSE"), []byte("licence"))
	writeFile(t, filepath.Join(dir, "bin", "ffmpeg.exe"), []byte("old ff"))
	writeFile(t, filepath.Join(dir, "mine.txt"), []byte("the owner's own file"))
	writeFile(t, filepath.Join(dir, "wms.json"), []byte(`{"portable":true}`))
	writeFile(t, filepath.Join(dir, "data", "library.db"), []byte("precious"))
	writeFile(t, filepath.Join(dir, "data", "cache", "x.jpg"), []byte("precious too"))
	return dir
}

func checkDataKept(t *testing.T, dir string) {
	t.Helper()
	if readFile(t, filepath.Join(dir, "data", "library.db")) != "precious" || readFile(t, filepath.Join(dir, "data", "cache", "x.jpg")) != "precious too" {
		t.Fatal("the data folder was changed")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "data"))
	if len(entries) != 2 {
		t.Fatalf("the data folder gained files: %v", entries)
	}
	if readFile(t, filepath.Join(dir, "wms.json")) != `{"portable":true}` || readFile(t, filepath.Join(dir, "mine.txt")) != "the owner's own file" {
		t.Fatal("the owner's files were changed")
	}
}

func TestSwapKeepsData(t *testing.T) {
	dir := install(t)
	staged := filepath.Join(dir, StageDir, "work", "new")
	writeFile(t, filepath.Join(staged, "wms.exe"), []byte("new wms"))
	writeFile(t, filepath.Join(staged, "bin", "ffmpeg.exe"), []byte("new ff"))
	writeFile(t, filepath.Join(staged, "bin", "ffprobe.exe"), []byte("new probe"))
	// Even if a package brought them, these are never written.
	writeFile(t, filepath.Join(staged, "data", "library.db"), []byte("EVIL"))
	writeFile(t, filepath.Join(staged, "DATA", "new.db"), []byte("EVIL"))
	writeFile(t, filepath.Join(staged, "wms.json"), []byte("EVIL"))

	olds, err := Swap(dir, staged, []string{filepath.Join(dir, "data")})
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "wms.exe")) != "new wms" || readFile(t, filepath.Join(dir, "bin", "ffmpeg.exe")) != "new ff" || readFile(t, filepath.Join(dir, "bin", "ffprobe.exe")) != "new probe" {
		t.Fatal("new files not in place")
	}
	if readFile(t, filepath.Join(dir, "wms.exe.old")) != "old wms" || len(olds) != 2 {
		t.Fatalf("old files not set aside: %v", olds)
	}
	checkDataKept(t, dir)

	// Next start: the old files go, everything else stays.
	if err := RecordOld(dir, olds); err != nil {
		t.Fatal(err)
	}
	Cleanup(dir)
	if _, err := os.Stat(filepath.Join(dir, "wms.exe.old")); err == nil {
		t.Error("old file not removed")
	}
	if _, err := os.Stat(filepath.Join(dir, StageDir)); err == nil {
		t.Error("staging folder not removed")
	}
	checkDataKept(t, dir)
}

func TestSwapRefusesProtected(t *testing.T) {
	dir := install(t)
	staged := filepath.Join(t.TempDir(), "new")
	writeFile(t, filepath.Join(staged, "wms.exe"), []byte("new wms"))
	writeFile(t, filepath.Join(staged, "library", "x.db"), []byte("EVIL"))
	// The owner keeps the library in <program>/library (wms.json dataDir).
	writeFile(t, filepath.Join(dir, "library", "x.db"), []byte("precious"))
	// Where file names ignore case (Windows, macOS), LIBRARY is the same folder.
	protect := "library"
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		protect = "LIBRARY"
	}
	if _, err := Swap(dir, staged, []string{filepath.Join(dir, protect)}); err == nil {
		t.Fatal("wrote into a protected folder")
	}
	if readFile(t, filepath.Join(dir, "wms.exe")) != "old wms" || readFile(t, filepath.Join(dir, "library", "x.db")) != "precious" {
		t.Fatal("files changed although the update was refused")
	}
}

func TestSwapRollsBack(t *testing.T) {
	dir := install(t)
	staged := filepath.Join(t.TempDir(), "new")
	writeFile(t, filepath.Join(staged, "LICENSE"), []byte("new licence"))
	writeFile(t, filepath.Join(staged, "wms.exe"), []byte("new wms"))
	// A folder where the package has a file: the swap fails part-way.
	writeFile(t, filepath.Join(staged, "zzz"), []byte("file"))
	writeFile(t, filepath.Join(dir, "zzz", "inside"), []byte("a folder"))
	if _, err := Swap(dir, staged, nil); err == nil {
		t.Fatal("expected a failure")
	}
	if readFile(t, filepath.Join(dir, "wms.exe")) != "old wms" || readFile(t, filepath.Join(dir, "LICENSE")) != "licence" {
		t.Fatal("not rolled back")
	}
	if _, err := os.Stat(filepath.Join(dir, "wms.exe.old")); err == nil {
		t.Error("old copy left behind after rollback")
	}
}

func TestWithin(t *testing.T) {
	base := filepath.Join(t.TempDir(), "app")
	cases := map[string]bool{
		filepath.Join(base, "data"):            true,
		filepath.Join(base, "data", "x"):       true,
		filepath.Join(base, "database"):        false,
		filepath.Join(base, "data", "..", "x"): false,
		base:                                   false,
	}
	for p, want := range cases {
		if got := within(p, filepath.Join(base, "data")); got != want {
			t.Errorf("within(%s) = %v, want %v", p, got, want)
		}
	}
	if within(base, "") {
		t.Error("empty folder protects everything")
	}
}

// TestApply runs a whole update against a fake release server: signature,
// checksum, download, swap and restart, with the data folder untouched.
func TestApply(t *testing.T) {
	priv, pub := newKey(t)
	exe := "wms"
	if runtime.GOOS == "windows" {
		exe = "wms.exe"
	}
	ver := "9.9.9-alpha"
	name := ArchiveName(ver, runtime.GOOS, runtime.GOARCH)
	files := []pkgFile{
		{name: "pkg/" + exe, body: "new wms", mode: 0o755},
		{name: "pkg/LICENSE", body: "new licence"},
		{name: "pkg/data/library.db", body: "EVIL"},
	}
	var archive []byte
	if strings.HasSuffix(name, ".zip") {
		archive = makeZip(t, files)
	} else {
		archive = makeTarGz(t, files)
	}
	sum := sha256.Sum256(archive)
	sums := []byte(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	sig := Sign(priv, sums)

	var srv *httptest.Server
	served := map[string][]byte{}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/releases" {
			a := func(n string, b []byte) map[string]any {
				return map[string]any{"name": n, "size": len(b), "browser_download_url": srv.URL + "/d/" + n}
			}
			json.NewEncoder(w).Encode([]map[string]any{{
				"tag_name": "v" + ver, "name": "Test", "body": "Notes", "html_url": "https://github.com/x", "prerelease": true,
				"assets": []any{a(name, archive), a(SumsName(ver), sums), a(SumsName(ver)+".sig", sig)},
			}})
			return
		}
		b, ok := served[strings.TrimPrefix(r.URL.Path, "/d/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	defer srv.Close()
	setServed := func(a, s, g []byte) {
		served[name], served[SumsName(ver)], served[SumsName(ver)+".sig"] = a, s, g
	}

	devFeedURL, devTransport = srv.URL+"/releases", http.DefaultTransport
	devAllowURL = func(u *url.URL) bool { return strings.HasPrefix(u.String(), srv.URL) }
	t.Cleanup(func() { devFeedURL, devTransport, devAllowURL, devKeys = "", nil, nil, nil })

	run := func(dir string) (Status, bool) {
		restarted := make(chan bool, 1)
		u := New("0.3.0-alpha", dir, []string{filepath.Join(dir, "data")}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
		u.Restart = func() { restarted <- true }
		st := u.Check(t.Context(), 0)
		if st.Latest == nil || st.Latest.Version != ver || !st.CanApply {
			t.Fatalf("check: %+v", st)
		}
		if err := u.Apply(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			st = u.Status()
			if st.Phase == PhaseFailed {
				return st, false
			}
			select {
			case <-restarted:
				return u.Status(), true
			case <-time.After(20 * time.Millisecond):
			}
		}
		t.Fatal("timed out")
		return st, false
	}

	// Signed by a key the program doesn't trust: refused, nothing changed.
	dir := install(t)
	setServed(archive, sums, sig)
	if st, ok := run(dir); ok || !strings.Contains(st.Error, "signature") {
		t.Fatalf("untrusted key: %+v", st)
	}
	if readFile(t, filepath.Join(dir, exe)) != "old wms" {
		t.Fatal("installed with an untrusted key")
	}

	devKeys = []string{base64.StdEncoding.EncodeToString(pub)}

	// A package that doesn't match its signed checksum: refused.
	tampered := append([]byte{}, archive...)
	tampered[len(tampered)/2] ^= 1
	setServed(tampered, sums, sig)
	if st, ok := run(dir); ok || !strings.Contains(st.Error, "checksum") {
		t.Fatalf("bad checksum: %+v", st)
	}
	if readFile(t, filepath.Join(dir, exe)) != "old wms" {
		t.Fatal("installed a package with a wrong checksum")
	}

	// Everything right: installed, restarted, data kept.
	setServed(archive, sums, sig)
	st, ok := run(dir)
	if !ok {
		t.Fatalf("update failed: %+v", st)
	}
	if readFile(t, filepath.Join(dir, exe)) != "new wms" || readFile(t, filepath.Join(dir, "LICENSE")) != "new licence" {
		t.Fatal("new files not installed")
	}
	checkDataKept(t, dir)
	Cleanup(dir)
	if _, err := os.Stat(filepath.Join(dir, exe+".old")); err == nil {
		t.Error("old program not cleaned up")
	}
	checkDataKept(t, dir)
}

// The Windows setup is a program with the package appended.
func TestExtractAppendedZip(t *testing.T) {
	dir := t.TempDir()
	exe := append([]byte("MZ"+strings.Repeat("\x00program", 5000)), makeZip(t, []pkgFile{{name: "pkg/wms.exe", body: "new wms"}})...)
	writeFile(t, filepath.Join(dir, "setup.exe"), exe)
	if err := Extract(filepath.Join(dir, "setup.exe"), filepath.Join(dir, "out"), 1<<20); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "out", "wms.exe")) != "new wms" {
		t.Fatal("not extracted")
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
