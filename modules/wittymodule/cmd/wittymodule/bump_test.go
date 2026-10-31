package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextVersion(t *testing.T) {
	for _, c := range []struct{ cur, part, want string }{
		{"0.2.0", "patch", "0.2.1"},
		{"0.2.9", "minor", "0.3.0"},
		{"0.2.9", "major", "1.0.0"},
		{"0.4.0-alpha", "patch", "0.4.1-alpha"},
		{"v1.2.3", "patch", "1.2.4"},
	} {
		if got, err := nextVersion(c.cur, c.part); err != nil || got != c.want {
			t.Errorf("%s %s: %q %v", c.cur, c.part, got, err)
		}
	}
	if _, err := nextVersion("0.2.0", "huge"); err == nil {
		t.Error("an unknown part")
	}
}

const manifestText = `{
  "id": "wittything",
  "name": "WittyThing",
  "version": "0.2.0",
  "repo": "wittywilla/WittyThing",
  "delivery": "library",
  "api": 2,
  "minServer": "0.4.0-alpha",
  "maxServer": "0.5.x"
}
`

func plugin(t *testing.T) (dir, server string) {
	t.Helper()
	root := t.TempDir()
	dir = filepath.Join(root, "wittything")
	server = filepath.Join(root, "server")
	for path, text := range map[string]string{
		filepath.Join(dir, "module.json"):         manifestText,
		filepath.Join(dir, "package.json"):        `{"name":"@wittywilla/wittything","version":"0.2.0","dependencies":{"version":"1"}}`,
		filepath.Join(dir, "go.mod"):              "module github.com/wittywilla/wittything\n\ngo 1.26.0\n",
		filepath.Join(server, "go.mod"):           "module x\n\nrequire (\n\tgithub.com/wittywilla/wittything v0.2.0\n\tgithub.com/wittywilla/wittythingelse v0.2.0\n)\n",
		filepath.Join(server, "web/package.json"): `{"dependencies":{"@wittywilla/wittything":"github:wittywilla/WittyThing#v0.2.0","other":"github:wittywilla/Other#v0.2.0"}}`,
		filepath.Join(server, "CHANGELOG.md"):     "# Changelog\n\n## Unreleased\n\n### Older\n\n- Something.\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, server
}

func read(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// One command changes every place the version is, and nothing else.
func TestBumpChangesEveryPlace(t *testing.T) {
	dir, server := plugin(t)
	changelog := filepath.Join(server, "CHANGELOG.md")
	if err := bump("patch", "", "Reads a new format.", changelog, server, []string{dir}); err != nil {
		t.Fatal(err)
	}
	if s := read(t, dir, "module.json"); !strings.Contains(s, `"version": "0.2.1"`) || !strings.Contains(s, `"repo": "wittywilla/WittyThing"`) {
		t.Errorf("module.json: %s", s)
	}
	if s := read(t, dir, "package.json"); !strings.Contains(s, `"version":"0.2.1"`) || !strings.Contains(s, `"dependencies":{"version":"1"}`) {
		t.Errorf("package.json: %s", s)
	}
	if s := read(t, server, "go.mod"); !strings.Contains(s, "wittything v0.2.1") || !strings.Contains(s, "wittythingelse v0.2.0") {
		t.Errorf("go.mod: %s", s)
	}
	if s := read(t, server, "web/package.json"); !strings.Contains(s, "WittyThing#v0.2.1") || !strings.Contains(s, "Other#v0.2.0") {
		t.Errorf("web/package.json: %s", s)
	}
	if s := read(t, changelog); !strings.Contains(s, "## Unreleased\n\n- **WittyThing 0.2.1:** Reads a new format.\n") {
		t.Errorf("changelog: %s", s)
	}
}

func TestBumpRefusesWhatCantBeRight(t *testing.T) {
	dir, server := plugin(t)
	if err := bump("patch", "0.2.0", "", "", "", []string{dir}); err == nil {
		t.Error("the same version")
	}
	if err := bump("patch", "0.1.0", "", "", "", []string{dir}); err == nil {
		t.Error("an older version")
	}
	if err := bump("patch", "", "", filepath.Join(server, "CHANGELOG.md"), "", []string{dir}); err == nil {
		t.Error("a changelog with no note")
	}
	if s := read(t, dir, "module.json"); !strings.Contains(s, `"version": "0.2.0"`) {
		t.Errorf("changed anyway: %s", s)
	}
}

// A plugin that changed since its tag must have a new number; documentation doesn't count.
func TestNeedsBump(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir, _ := plugin(t)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "core.autocrlf=false"}, args...)...)
		cmd.Dir = dir
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, b)
		}
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("add", ".")
	git("commit", "-qm", "first")
	if err := needsBump([]string{dir}); err != nil {
		t.Fatalf("no tag yet: %v", err)
	}
	git("tag", "v0.2.0")
	if err := needsBump([]string{dir}); err != nil {
		t.Fatalf("unchanged: %v", err)
	}
	write("README.md", "words")
	git("add", ".")
	git("commit", "-qm", "docs")
	if err := needsBump([]string{dir}); err != nil {
		t.Fatalf("only documentation: %v", err)
	}
	write("code.go", "package x")
	git("add", ".")
	git("commit", "-qm", "code")
	if err := needsBump([]string{dir}); err == nil || !strings.Contains(err.Error(), "wittymodule bump") {
		t.Fatalf("changed with no new number: %v", err)
	}
	if err := bump("patch", "", "", "", "", []string{dir}); err != nil {
		t.Fatal(err)
	}
	if err := needsBump([]string{dir}); err != nil {
		t.Fatalf("after a bump: %v", err)
	}
}

// A signature only covers files of this module and this version.
func TestSignRefusesOtherFiles(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	if err := keygen(keyPath); err != nil {
		t.Fatal(err)
	}
	write := func(text string) string {
		p := filepath.Join(dir, "sums")
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("aa  wittydvd-0.2.1-web.tar.gz\nbb  wittydvd-0.2.1-web/index.js\n")
	if err := sign(keyPath, "wittydvd-0.2.1", []string{good}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"aa  wittytuner-0.2.1-web.tar.gz\n", "aa  wittydvd-0.2.0-web.tar.gz\n", "aa  wittydvd-0.2.1-web/../../x\n"} {
		if err := sign(keyPath, "wittydvd-0.2.1", []string{write(bad)}); err == nil {
			t.Errorf("signed %q", bad)
		}
	}
}

// A list on a release can't get a signature on top of it unless it already had one.
func TestWithdrawChecksTheListThere(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	if err := keygen(keyPath); err != nil {
		t.Fatal(err)
	}
	list := filepath.Join(dir, "wittydvd-withdrawn.json")
	if err := withdraw(list, "wittydvd", "broken", "first", keyPath, []string{"0.1.0"}); err != nil {
		t.Fatal(err)
	}
	if err := sign(keyPath, "", []string{list}); err != nil {
		t.Fatal(err)
	}
	if err := withdraw(list, "wittydvd", "broken", "second", keyPath, []string{"0.1.1"}); err != nil {
		t.Fatalf("a list carrying its signature: %v", err)
	}
	if err := withdraw(list, "wittydvd", "broken", "third", keyPath, []string{"0.1.2"}); err == nil {
		t.Fatal("the list changed since it was signed, so it should be refused")
	}
	if err := os.WriteFile(list, []byte(`{"module":"wittydvd","serial":999,"versions":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := withdraw(list, "wittydvd", "broken", "fourth", keyPath, []string{"0.1.3"}); err == nil {
		t.Fatal("a planted list should be refused")
	}
}
