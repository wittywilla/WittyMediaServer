// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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
	if !strings.Contains(read("packaging", "install.sh"), `cp -R "$HERE/bin"`) {
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
	if !strings.Contains(read("packaging", "freebsd", "build-pkg.sh"), `cp -R "$STAGE/bin"`) {
		t.Error("the FreeBSD package leaves out bin")
	}
	// A package holds no module's program. A starter set of signed modules, when a release has
	// one, is a folder that each way of installing copies by name, or that comes with the app.
	if !strings.Contains(build, `"$out/modules"`) {
		t.Error("build-release.sh doesn't copy a starter set")
	}
	if !strings.Contains(build, `"$app/Resources/modules"`) {
		t.Error("the Mac app and installer leave out the starter set")
	}
	if !strings.Contains(read("packaging", "install.sh"), `"$HERE/modules"`) {
		t.Error("install.sh leaves out the starter set")
	}
	if !strings.Contains(read("packaging", "linux", "build-packages.sh"), "stage/modules") {
		t.Error("the .deb and .rpm leave out the starter set")
	}
	if !strings.Contains(read("packaging", "windows", "WittyMediaServer.wxs"), `modules\**`) {
		t.Error("the MSI leaves out the starter set")
	}
	if !strings.Contains(read("packaging", "freebsd", "build-pkg.sh"), `"$STAGE/modules"`) {
		t.Error("the FreeBSD package leaves out the starter set")
	}
	if strings.Contains(build, "wittyplay") {
		t.Error("build-release.sh names a module's program")
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
