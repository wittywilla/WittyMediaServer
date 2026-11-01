package layout

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const marker = `{"product":"WittyMediaServer","kind":"msi"}`

func TestFindPackage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "WittyMediaServer")
	write(t, filepath.Join(root, Marker), marker)
	for _, exe := range []string{filepath.Join(root, "bin", "wittymediaserver"), filepath.Join(root, "WittyMediaServer")} {
		p := Find(exe)
		if p.Root != root || p.Bin != filepath.Join(root, "bin") || p.Kind != KindMSI {
			t.Errorf("%s: got %+v", exe, p)
		}
	}
}

func TestFindMacApp(t *testing.T) {
	contents := filepath.Join(t.TempDir(), "WittyMediaServer.app", "Contents")
	write(t, filepath.Join(contents, "Resources", Marker), `{"product":"WittyMediaServer","kind":"pkg"}`)
	for _, exe := range []string{filepath.Join(contents, "MacOS", "WittyMediaServer"), filepath.Join(contents, "Helpers", "wittymediaserver")} {
		p := Find(exe)
		if p.Root != contents || p.Bin != filepath.Join(contents, "Helpers") || p.Kind != KindPkg {
			t.Errorf("%s: got %+v", exe, p)
		}
	}
}

// Someone's ~/go/bin or /usr/local/bin is not a package, and whatever is one level up
// isn't this program's.
func TestABinFolderIsNotAPackage(t *testing.T) {
	home := t.TempDir()
	exe := filepath.Join(home, "go", "bin", "wittymediaserver")
	p := Find(exe)
	if p.Root != filepath.Dir(exe) || p.Bin != filepath.Dir(exe) || p.Kind != KindSource {
		t.Errorf("got %+v", p)
	}
}

// A file of the same name from something else doesn't make a folder ours.
func TestSomeoneElsesMarker(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, Marker), `{"product":"another program","kind":"msi"}`)
	if p := Find(filepath.Join(root, "bin", "x")); p.Kind != KindSource || p.Root != filepath.Join(root, "bin") {
		t.Errorf("got %+v", p)
	}
}

func TestClearObsolete(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, Marker), marker)
	write(t, filepath.Join(root, "bin", Exe(ServerCommand)), "new")
	write(t, filepath.Join(root, "wms-setup.exe"), "old")
	write(t, filepath.Join(root, "data", "wms.db"), "precious")
	write(t, filepath.Join(root, "wms.json"), "{}")
	write(t, filepath.Join(root, "my notes.txt"), "the owner's")
	Find(filepath.Join(root, "bin", "x")).ClearObsolete()
	if fileExists(filepath.Join(root, "wms-setup.exe")) {
		t.Error("an old part is still there")
	}
	for _, keep := range []string{"data/wms.db", "wms.json", "my notes.txt"} {
		if !fileExists(filepath.Join(root, keep)) {
			t.Errorf("%s was removed", keep)
		}
	}
}

// Without the new server in place, nothing of the old copy may go.
func TestClearObsoleteNeedsTheNewServer(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, Marker), marker)
	write(t, filepath.Join(root, "wms-setup.exe"), "old")
	Find(filepath.Join(root, "bin", "x")).ClearObsolete()
	if !fileExists(filepath.Join(root, "wms-setup.exe")) {
		t.Error("removed from a package that has no new server")
	}
}

// A starter set sits in the root of a package, and with the app's resources on a Mac.
func TestWhereAStarterSetIs(t *testing.T) {
	if got, want := (Package{Root: filepath.Join("opt", "wms"), Bin: filepath.Join("opt", "wms", "bin")}).Modules(), filepath.Join("opt", "wms", "modules"); got != want {
		t.Errorf("%q, want %q", got, want)
	}
	if got, want := (Package{Root: filepath.Join("A.app", "Contents")}).Modules(), filepath.Join("A.app", "Contents", "Resources", "modules"); got != want {
		t.Errorf("%q, want %q", got, want)
	}
}
