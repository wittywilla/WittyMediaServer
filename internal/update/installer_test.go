// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package update

import (
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

	"github.com/wittywilla/wittymediaserver/internal/layout"
)

// A copy installed by a package (here a .deb on Linux, the MSI on Windows) gets the installer for
// its system, checked against the signed list, and what's new from the changelog.
func TestFetchInstaller(t *testing.T) {
	kind := map[string]string{"windows": layout.KindMSI, "linux": layout.KindDeb, "darwin": layout.KindPkg, "freebsd": layout.KindFreeBSD}[runtime.GOOS]
	if kind == "" || InstallerName("1.0.0", runtime.GOOS, runtime.GOARCH, kind) == "" {
		t.Skip("no installer is made for this system")
	}
	priv, pub := newKey(t)
	ver := "9.9.9-alpha"
	name := InstallerName(ver, runtime.GOOS, runtime.GOARCH, kind)
	installer := []byte("an installer " + strings.Repeat("x", 5000))
	archive := []byte("the archive")
	sumOf := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	sums := []byte(sumOf(installer) + "  " + name + "\n" + sumOf(archive) + "  " + ArchiveName(ver, runtime.GOOS, runtime.GOARCH) + "\n")
	sig := Sign(priv, sums)

	served := map[string][]byte{name: installer, ArchiveName(ver, runtime.GOOS, runtime.GOARCH): archive, SumsName(ver): sums, SumsName(ver) + ".sig": sig}
	var srv *httptest.Server
	changelogAsked := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			var assets []any
			for n, b := range served {
				assets = append(assets, map[string]any{"name": n, "size": len(b), "browser_download_url": srv.URL + "/d/" + n})
			}
			json.NewEncoder(w).Encode([]map[string]any{{
				"tag_name": "v" + ver, "name": "Test", "body": "## Which file to download\n\n| a | b |\n", "html_url": "https://github.com/x", "prerelease": true, "assets": assets,
			}})
		case "/changelog":
			changelogAsked++
			if r.URL.Query().Get("ref") != "v"+ver {
				http.NotFound(w, r)
				return
			}
			io.WriteString(w, "## 9.9.9-alpha\n\n- **Installers come checked.**\n\n## 0.3.0-alpha\n\n- Old.\n")
		default:
			b, ok := served[strings.TrimPrefix(r.URL.Path, "/d/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write(b)
		}
	}))
	defer srv.Close()
	devFeedURL, devTransport, devChangelog = srv.URL+"/releases", http.DefaultTransport, srv.URL+"/changelog"
	devAllowURL = func(u *url.URL) bool { return strings.HasPrefix(u.String(), srv.URL) }
	devKeys = []string{base64.StdEncoding.EncodeToString(pub)}
	t.Cleanup(func() { devFeedURL, devTransport, devAllowURL, devKeys, devChangelog = "", nil, nil, nil, "" })

	dir := install(t)
	writeFile(t, filepath.Join(dir, layout.Marker), []byte(`{"product":"WittyMediaServer","kind":"`+kind+`"}`))
	u := New("0.3.0-alpha", dir, []string{filepath.Join(dir, "data")}, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	u.Restart = func() {}
	u.DownloadDir = filepath.Join(t.TempDir(), "updates")
	if kind == layout.KindMSI {
		// The MSI for one account can update itself; the one for everyone can't write there.
		u.reason, u.reasonAt = "Windows protects the program folder", time.Now().Add(time.Hour)
	}

	st := u.Check(t.Context(), 0)
	if st.Latest == nil || st.Method != MethodInstaller || st.Installer == nil || st.Installer.Name != name || st.CanApply {
		t.Fatalf("check: %+v", st)
	}
	if len(st.Latest.Changes) != 1 || st.Latest.Changes[0].Notes != "- **Installers come checked.**" {
		t.Errorf("what's new: %+v", st.Latest.Changes)
	}
	u.Check(t.Context(), 0)
	if changelogAsked != 1 {
		t.Errorf("the changelog was read %d times for one version", changelogAsked)
	}

	wait := func() Installer {
		t.Helper()
		// Long enough for a machine that's busy with other work.
		deadline := time.Now().Add(2 * time.Minute)
		for time.Now().Before(deadline) {
			in := *u.Status().Installer
			if in.Phase == InstallerReady || in.Phase == InstallerFailed {
				return in
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("timed out")
		return Installer{}
	}

	// A file that doesn't match the signed checksum is never offered.
	served[name] = append([]byte{}, installer...)
	served[name][10] ^= 1
	if err := u.FetchInstaller(); err != nil {
		t.Fatal(err)
	}
	if in := wait(); in.Phase != InstallerFailed || !strings.Contains(in.Error, "checksum") {
		t.Fatalf("tampered: %+v", in)
	}
	if _, _, ok := u.InstallerFile(); ok {
		t.Fatal("a tampered installer is offered")
	}
	if ents, _ := os.ReadDir(u.DownloadDir); len(ents) != 0 {
		t.Errorf("a failed download left %d files", len(ents))
	}

	// The right file: kept, offered, with the command for a package manager.
	served[name] = installer
	writeFile(t, filepath.Join(u.DownloadDir, "wittymediaserver-9.0.0-alpha-linux-amd64.deb"), []byte("an older one"))
	if err := u.FetchInstaller(); err != nil {
		t.Fatal(err)
	}
	in := wait()
	if in.Phase != InstallerReady || in.Path != filepath.Join(u.DownloadDir, name) || readFile(t, in.Path) != string(installer) {
		t.Fatalf("ready: %+v", in)
	}
	if (kind == layout.KindDeb) != strings.HasPrefix(in.Command, "sudo apt install ") {
		t.Errorf("command: %q", in.Command)
	}
	if _, err := os.Stat(filepath.Join(u.DownloadDir, "wittymediaserver-9.0.0-alpha-linux-amd64.deb")); err == nil {
		t.Error("an older installer was kept")
	}
	if p, n, ok := u.InstallerFile(); !ok || p != in.Path || n != name {
		t.Errorf("InstallerFile = %q %q %v", p, n, ok)
	}

	// Asked again after a restart: the file already here is checked, not fetched again.
	u2 := New("0.3.0-alpha", dir, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	u2.Restart, u2.DownloadDir, u2.reason, u2.reasonAt = func() {}, u.DownloadDir, u.reason, u.reasonAt
	u2.CleanDownloads()
	u2.Check(t.Context(), 0)
	delete(served, name)
	if err := u2.FetchInstaller(); err != nil {
		t.Fatal(err)
	}
	u = u2
	if in := wait(); in.Phase != InstallerReady {
		t.Fatalf("reused: %+v", in)
	}

	// Once that version runs, its installer goes.
	u3 := New(ver, dir, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	u3.DownloadDir = u.DownloadDir
	u3.CleanDownloads()
	if ents, _ := os.ReadDir(u.DownloadDir); len(ents) != 0 {
		t.Errorf("%d installers kept after updating", len(ents))
	}
}
