// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package update

import (
	"strings"
	"testing"
)

const testChangelog = `# Changelog

Intro.

## Unreleased

- Not out yet.

## 0.7.0-alpha

- **Updates are easier to find.** A banner.

  More words.

` + "```" + `
## not a heading, it's in code
` + "```" + `

## 0.6.2-alpha

- **The Windows installer updates an installed copy again.**

## 0.6.1-alpha

- **DVDs and Blu-rays in Firefox.**
- A stream link.

## 0.6.0-alpha

- Lots.
`

func TestChangesBetween(t *testing.T) {
	got := changesBetween(testChangelog, "0.6.1-alpha", "0.7.0-alpha")
	if len(got) != 2 || got[0].Version != "0.7.0-alpha" || got[1].Version != "0.6.2-alpha" {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(got[0].Notes, "not a heading") || !strings.HasPrefix(got[0].Notes, "- **Updates") {
		t.Errorf("0.7.0 notes: %q", got[0].Notes)
	}
	if strings.Contains(got[1].Notes, "Firefox") {
		t.Error("the running version's notes were included")
	}
	// Someone on a version the changelog doesn't know (a build from source) sees no more than 20.
	if n := len(changesBetween(testChangelog, "dev", "0.7.0-alpha")); n != 4 {
		t.Errorf("from dev: %d versions", n)
	}
	if c := changesBetween("## 9.0.0\n\n"+strings.Repeat("- word\n", 20000), "1.0.0", "9.0.0"); len(c) != 1 || len(c[0].Notes) > maxChangeBytes+100 || !strings.HasSuffix(c[0].Notes, "*More is on the release page.*") {
		t.Errorf("long notes weren't clipped: %d bytes", len(c[0].Notes))
	}
}

func TestReleaseSummary(t *testing.T) {
	old := "## Which file to download\n\n| System | File |\n|---|---|\n| Windows | `.msi` |\n\n## What's Changed\n* A PR by @someone\n\n\n**Full Changelog**: https://github.com/x/compare/a...b\n"
	if got := releaseSummary(old); got != "" {
		t.Errorf("an old release's notes kept the download table or the PR list: %q", got)
	}
	withNew := "## What's new in 0.7.0-alpha\n\n- **Banner.**\n\n## Which file to download\n\n| a | b |\n"
	if got := releaseSummary(withNew); got != "- **Banner.**" {
		t.Errorf("got %q", got)
	}
	other := "Some words first.\n\n## Known problems\n\n- One.\n"
	if got := releaseSummary(other); !strings.Contains(got, "Some words first.") || !strings.Contains(got, "### Known problems") {
		t.Errorf("got %q", got)
	}
	rels := []ghRelease{
		{TagName: "v0.7.0-alpha", Body: withNew},
		{TagName: "v0.6.2-alpha", Body: old},
		{TagName: "v0.6.3-alpha", Body: other},
		{TagName: "v0.6.1-alpha", Body: other},
		{TagName: "v0.7.1-alpha", Body: other, Draft: true},
	}
	got := releaseChanges(rels, "0.6.1-alpha", "0.7.0-alpha")
	if len(got) != 2 || got[0].Version != "0.7.0-alpha" || got[1].Version != "0.6.3-alpha" {
		t.Errorf("got %+v", got)
	}
}

func TestInstallerName(t *testing.T) {
	for _, c := range []struct{ goos, goarch, kind, want string }{
		{"windows", "amd64", "msi", "wittymediaserver-1.0.0-windows-amd64.msi"},
		{"windows", "arm64", "msi", "wittymediaserver-1.0.0-windows-arm64.msi"},
		{"windows", "386", "msi", ""},
		{"darwin", "arm64", "pkg", "wittymediaserver-1.0.0-macos-arm64.pkg"},
		{"linux", "arm", "deb", "wittymediaserver-1.0.0-linux-armv7.deb"},
		{"linux", "amd64", "rpm", "wittymediaserver-1.0.0-linux-amd64.rpm"},
		{"freebsd", "amd64", "freebsd-pkg", "wittymediaserver-1.0.0-freebsd-amd64.pkg"},
		{"linux", "amd64", "msi", ""},
		{"linux", "amd64", "archive", "wittymediaserver-1.0.0-linux-amd64.tar.gz"},
		{"windows", "amd64", "", "wittymediaserver-1.0.0-windows-amd64.zip"},
	} {
		if got := InstallerName("1.0.0", c.goos, c.goarch, c.kind); got != c.want {
			t.Errorf("%s/%s %s: %q, want %q", c.goos, c.goarch, c.kind, got, c.want)
		}
	}
	for name, want := range map[string]string{
		"wittymediaserver-0.7.0-alpha-windows-amd64.msi":      "0.7.0-alpha",
		"wittymediaserver-0.7.0-alpha.2-linux-armv7.deb":      "0.7.0-alpha.2",
		"wittymediaserver-1.0.0-freebsd-amd64.pkg":            "1.0.0",
		"wittymediaserver-0.7.0-alpha-windows-amd64.msi.part": "",
		"something-else.msi":                                  "",
	} {
		if got := installerVersion(name); got != want {
			t.Errorf("installerVersion(%q) = %q, want %q", name, got, want)
		}
	}
	if got := installCommand("deb", "/var/cache/wittymediaserver/updates/x.deb"); got != "sudo apt install /var/cache/wittymediaserver/updates/x.deb" {
		t.Error(got)
	}
	if got := installCommand("rpm", "/home/a b/it's.rpm"); got != `sudo dnf install '/home/a b/it'\''s.rpm'` {
		t.Error(got)
	}
	if installCommand("msi", `C:\x.msi`) != "" {
		t.Error("a command for the Windows installer")
	}
}
