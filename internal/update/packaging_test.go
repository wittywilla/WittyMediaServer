package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A helper that's in a package isn't enough: each way of installing a package copies only
// what it names. These names are in internal/layout.
func TestEveryInstallBringsTheServerAndItsHelpers(t *testing.T) {
	read := func(parts ...string) string {
		b, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	build := read("scripts", "build-release.sh")
	if !strings.Contains(build, `-o "$out/bin/wittymediaserver$ext"`) {
		t.Error("build-release.sh doesn't put the server in bin")
	}
	if !strings.Contains(read("scripts", "install.sh"), `cp -R "$HERE/bin"`) {
		t.Error("install.sh leaves out bin")
	}
	if !strings.Contains(read("packaging", "linux", "nfpm.yaml"), "${STAGE}/bin") {
		t.Error("the .deb and .rpm leave out bin")
	}
	if !strings.Contains(read("packaging", "windows", "WittyMediaServer.wxs"), `bin\**`) {
		t.Error("the MSI leaves out bin")
	}
	if !strings.Contains(read("packaging", "macos", "build-pkg.sh"), "Helpers") {
		t.Error("the Mac installer leaves out the helpers")
	}
}
