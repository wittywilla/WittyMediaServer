package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A package holds no module's program. A starter set of signed modules, when a release has one, is
// a folder, and each way of installing a package copies only what it names.
func TestEveryInstallBringsTheStarterSet(t *testing.T) {
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "scripts", name))
		if err != nil {
			t.Fatal(err)
		}
		return strings.ReplaceAll(string(b), "
", "
")
	}
	if !strings.Contains(read("install.sh"), `"$HERE/modules"`) {
		t.Error("install.sh leaves out the starter set")
	}
	if !strings.Contains(read("install.ps1"), `$here\modules`) {
		t.Error("install.ps1 leaves out the starter set")
	}
	build := read("build-release.sh")
	if !strings.Contains(build, `"$out/modules"`) {
		t.Error("build-release.sh doesn't copy a starter set")
	}
	for _, name := range []string{"install.sh", "install.ps1", "build-release.sh"} {
		if strings.Contains(read(name), "wittyplay") {
			t.Errorf("%s names a module's program", name)
		}
	}
	start := strings.Index(build, "make_app_bundle() {")
	end := strings.Index(build, "cat > \"$app/Info.plist\"")
	if start < 0 || end < start || !strings.Contains(build[start:end], `"$out/modules" "$app/MacOS/"`) {
		t.Error("the Mac app leaves out the starter set")
	}
}
