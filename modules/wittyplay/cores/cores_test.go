package cores

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittyplay"
)

func TestNoNonCommercialCoreOffered(t *testing.T) {
	for i := range wittyplay.Consoles {
		for _, c := range wittyplay.Consoles[i].Cores {
			if wittyplay.IsNonCommercial(c.ID) {
				t.Errorf("%s is non-commercial and must not be offered", c.ID)
			}
			if strings.Contains(strings.ToLower(c.License), "non") {
				t.Errorf("%s has licence %q", c.ID, c.License)
			}
			if !validID.MatchString(c.ID) {
				t.Errorf("%q is not a valid core name", c.ID)
			}
		}
	}
	for _, id := range []string{"snes9x", "genesis_plus_gx", "picodrive", "fbneo", "mame2003_plus", "../../evil", ""} {
		if _, err := DownloadURL(id); err == nil {
			t.Errorf("DownloadURL(%q) should refuse", id)
		}
	}
}

func TestDownloadURLIsFixedHost(t *testing.T) {
	if !CanDownload() {
		t.Skip("no prebuilt cores for this platform")
	}
	u, err := DownloadURL("fceumm")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(u, "https://buildbot.libretro.com/") {
		t.Fatal(u)
	}
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for n, s := range files {
		f, err := w.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(s))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestExtractOneTakesOnlyTheNamedFile(t *testing.T) {
	data := zipOf(t, map[string]string{"../../evil.dll": "bad", "other.txt": "no", "x_libretro.dll": "good"})
	b, err := extractOne(data, "x_libretro.dll")
	if err != nil || string(b) != "good" {
		t.Fatalf("got %q, %v", b, err)
	}
	if _, err := extractOne(data, "missing.dll"); err == nil {
		t.Fatal("a missing file should be an error")
	}
}

func TestRedirectAwayIsRefused(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example/evil.zip", http.StatusFound)
	}))
	defer srv.Close()
	m := &Manager{Dir: t.TempDir(), Client: srv.Client()}
	if _, err := m.fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("following a redirect to another host should fail")
	}
}

func TestListAndForConsole(t *testing.T) {
	dir, other := t.TempDir(), t.TempDir()
	touch := func(d, id string) { _ = os.WriteFile(filepath.Join(d, fileName(id)), []byte("x"), 0o644) }
	touch(dir, "nestopia")
	touch(other, "fceumm")
	touch(other, "snes9x")
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644)
	m := &Manager{Dir: dir, Extra: []string{other}}
	all := m.List()
	if len(all) != 3 {
		t.Fatalf("got %+v", all)
	}
	nes := wittyplay.ByID("nes")
	if got := m.ForConsole(nes, ""); got == nil || got.ID != "fceumm" {
		t.Fatalf("expected the first core in the console's list, got %+v", got)
	}
	if got := m.ForConsole(nes, "nestopia"); got == nil || got.ID != "nestopia" {
		t.Fatalf("expected the preferred core, got %+v", got)
	}
	if got := m.Find("snes9x"); got == nil || !got.NonCommercial {
		t.Fatalf("a non-commercial core someone supplied should be labelled: %+v", got)
	}
	if err := m.Remove("../nope"); err == nil {
		t.Fatal("a bad name should be refused")
	}
}

// A core with support files gets them too, kept inside the system folder.
func TestAssetsRefuseToEscape(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../outside.txt")
	_, _ = w.Write([]byte("x"))
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(buf.Bytes()) }))
	defer srv.Close()
	dir := t.TempDir()
	m := &Manager{Dir: filepath.Join(dir, "cores"), System: filepath.Join(dir, "system"), Client: redirectClient(srv)}
	if err := m.installAssets(context.Background(), "PPSSPP"); err == nil {
		t.Fatal("an archive naming a path outside its folder should be refused")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside.txt")); err == nil {
		t.Fatal("a file escaped")
	}
}

func TestAssetsInstall(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	_, _ = zw.Create("PPSSPP/")
	w, _ := zw.Create("PPSSPP/font.ttf")
	_, _ = w.Write([]byte("glyphs"))
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(buf.Bytes()) }))
	defer srv.Close()
	dir := t.TempDir()
	m := &Manager{Dir: filepath.Join(dir, "cores"), System: filepath.Join(dir, "system"), Client: redirectClient(srv)}
	if err := m.installAssets(context.Background(), "PPSSPP"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "system", "PPSSPP", "font.ttf")); err != nil || string(b) != "glyphs" {
		t.Fatalf("%q %v", b, err)
	}
}

func TestPlayableSkipsACoreMissingItsBIOS(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"pcsx2", "play"} {
		if err := os.WriteFile(filepath.Join(dir, fileName(id)), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	system := t.TempDir()
	m := &Manager{Dir: dir, Extra: []string{}}
	ps2 := wittyplay.ByID("ps2")
	if ok, _ := m.Playable(ps2, "", system); ok == nil || ok.ID != "play" {
		t.Fatalf("with no BIOS the core that needs none should play, got %+v", ok)
	}
	if ok, blocked := m.Playable(ps2, "pcsx2", system); ok != nil || blocked == nil || blocked.ID != "pcsx2" {
		t.Fatalf("asking for the core that needs one should say so: %+v %+v", ok, blocked)
	}
	bios := filepath.Join(system, "pcsx2", "bios")
	if err := os.MkdirAll(bios, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bios, "scph39001.bin"), make([]byte, 4<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, _ := m.Playable(ps2, "", system); ok == nil || ok.ID != "pcsx2" {
		t.Fatalf("with a BIOS the first core should play, got %+v", ok)
	}
}

// redirectClient answers every request from a test server, whatever host it names.
func redirectClient(srv *httptest.Server) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		out := r.Clone(r.Context())
		out.URL.Scheme, out.URL.Host = "http", strings.TrimPrefix(srv.URL, "http://")
		return http.DefaultTransport.RoundTrip(out)
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
