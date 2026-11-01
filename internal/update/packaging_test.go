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
		return strings.ReplaceAll(string(b), "\r\n", "\n")
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

// Every FFmpeg download in a release build is named in the lock, by hash, and nothing fetches a moving address.
func TestFFmpegDownloadsArePinned(t *testing.T) {
	lock, err := os.ReadFile("../../scripts/ffmpeg.lock")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, l := range strings.Split(strings.ReplaceAll(string(lock), "\r", ""), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		if len(f) != 3 || len(f[1]) != 64 || !strings.HasPrefix(f[2], "https://") {
			t.Errorf("bad line: %q", l)
			continue
		}
		if strings.Contains(f[2], "/latest") || strings.Contains(f[2], "redirect") {
			t.Errorf("a moving address: %q", l)
		}
		names[f[0]] = true
	}
	script, err := os.ReadFile("../../scripts/build-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(script)
	for _, bad := range []string{"releases/download/latest", "/redirect/latest", "ffmpeg-master-latest"} {
		if strings.Contains(s, bad) {
			t.Errorf("the build still uses %s", bad)
		}
	}
	for _, want := range []string{"win64", "winarm64", "linux64", "linuxarm64", "linux-armhf", "linux-i686", "mac-amd64-ffmpeg", "mac-amd64-ffprobe", "mac-arm64-ffmpeg", "mac-arm64-ffprobe"} {
		if !names[want] {
			t.Errorf("the lock has no %s", want)
		}
	}
}
