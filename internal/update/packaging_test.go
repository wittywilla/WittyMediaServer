package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Without the game runner no game can start, and a package holding it isn't enough: each
// way of installing a package copies only what it names.
func TestEveryInstallBringsTheGameRunner(t *testing.T) {
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if !strings.Contains(read("install.sh"), `cp "$HERE/wittyplay-host"`) {
		t.Error("install.sh leaves out the game runner")
	}
	if !strings.Contains(read("install.ps1"), `"wittyplay-host.exe"`) {
		t.Error("install.ps1 leaves out the game runner")
	}
	build := read("build-release.sh")
	if !strings.Contains(build, `-o "$abs_out/wittyplay-host$ext"`) {
		t.Error("build-release.sh doesn't build the game runner into the package")
	}
	start := strings.Index(build, "make_app_bundle() {")
	end := strings.Index(build, "cat > \"$app/Info.plist\"")
	if start < 0 || end < start || !strings.Contains(build[start:end], `"$out/wittyplay-host" "$app/MacOS/"`) {
		t.Error("the Mac app leaves out the game runner")
	}
}
