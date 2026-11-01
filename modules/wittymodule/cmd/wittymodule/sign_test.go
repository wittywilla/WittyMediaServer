package main

import "testing"

// A signature covers exactly this module and version: a neighbouring version, or a
// suffix that makes it another one, must not pass.
func TestSignOnlyCoversThisVersion(t *testing.T) {
	const expect = "wittydvd-0.2.1"
	for name, want := range map[string]bool{
		"wittydvd-0.2.1-linux-amd64.tar.gz":      true,
		"wittydvd-0.2.1-linux-armv7.tar.gz":      true,
		"wittydvd-0.2.1-windows-arm64.zip":       true,
		"wittydvd-0.2.1-darwin-arm64.tar.gz":     true,
		"wittydvd-0.2.1-freebsd-amd64.tar.gz":    true,
		"wittydvd-0.2.1-linux-amd64/wittydvd":    true,
		"wittydvd-0.2.1-linux-amd64/module.json": true,
		"wittydvd-0.2.1-module.json":             true,
		"wittydvd-0.2.1-web.tar.gz":              true,
		"wittydvd-0.2.1-web/index.js":            true,

		"wittydvd-0.2.10-linux-amd64.tar.gz":      false,
		"wittydvd-0.2.1-beta-linux-amd64.tar.gz":  false,
		"wittydvd-0.2.1-evil-module.json":         false,
		"wittydvd-0.2.1-beta-module.json":         false,
		"wittydvd-0.2.1-webs.tar.gz":              false,
		"wittydvd-0.2.1-linux-amd64.exe":          false,
		"wittydvd-0.2.1":                          false,
		"wittydvd-0.2.1-":                         false,
		"wittytuner-0.2.1-linux-amd64.tar.gz":     false,
		"wittydvd-0.2.1.1-linux-amd64.tar.gz":     false,
		"other/wittydvd-0.2.1-linux-amd64.tar.gz": false,
	} {
		if got := fileOf(name, expect); got != want {
			t.Errorf("fileOf(%q) = %v, want %v", name, got, want)
		}
	}
}
