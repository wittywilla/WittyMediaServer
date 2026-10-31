package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymodule"
)

// A plugin's version moves in small steps as work lands, so raising it is one command
// that changes every place it appears, and can't leave them disagreeing. A bump isn't a
// release: nothing is tagged or published here.

var versionField = regexp.MustCompile(`("version"\s*:\s*")([^"]*)(")`)

// nextVersion raises one part of a version, keeping a pre-release label.
func nextVersion(cur, part string) (string, error) {
	core, pre, hasPre := strings.Cut(strings.TrimPrefix(cur, "v"), "-")
	if i := strings.IndexByte(core, '+'); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("%q is not a version", cur)
	}
	n := [3]int{}
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return "", fmt.Errorf("%q is not a version", cur)
		}
		n[i] = v
	}
	switch part {
	case "patch":
		n[2]++
	case "minor":
		n[1], n[2] = n[1]+1, 0
	case "major":
		n[0], n[1], n[2] = n[0]+1, 0, 0
	default:
		return "", fmt.Errorf("-part must be patch, minor or major, not %q", part)
	}
	out := fmt.Sprintf("%d.%d.%d", n[0], n[1], n[2])
	if hasPre {
		out += "-" + pre
	}
	return out, nil
}

// setVersion changes the first version field in a JSON file, leaving the rest as written.
func setVersion(path, ver string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	loc := versionField.FindSubmatchIndex(b)
	if loc == nil {
		return fmt.Errorf("%s has no version", path)
	}
	out := append(append(append([]byte{}, b[:loc[4]]...), ver...), b[loc[5]:]...)
	return os.WriteFile(path, out, 0o644)
}

// bump raises a plugin's version in its manifest and package.json and, when asked, in the
// server that requires it and in a changelog.
func bump(part, to, note, changelog, consumer string, args []string) error {
	if len(args) != 1 {
		return errors.New("which folder? wittymodule bump [-part patch|minor|major] [-to version] [-note text] [-changelog file] [-consumer folder] <folder>")
	}
	dir := args[0]
	m, _, err := readManifest(dir)
	if err != nil {
		return err
	}
	ver := strings.TrimPrefix(to, "v")
	if ver == "" {
		if ver, err = nextVersion(m.Version, part); err != nil {
			return err
		}
	}
	if !wittymodule.Valid(ver) {
		return fmt.Errorf("%q is not a version", ver)
	}
	if wittymodule.Compare(ver, m.Version) <= 0 {
		return fmt.Errorf("%s is not newer than %s, and tags never move, so a changed plugin gets a new number", ver, m.Version)
	}
	if changelog != "" && note == "" {
		return errors.New("a changelog entry needs -note: what changed, for the people reading it")
	}
	if err := setVersion(filepath.Join(dir, wittymodule.ManifestFile), ver); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
		if err := setVersion(filepath.Join(dir, "package.json"), ver); err != nil {
			return err
		}
	}
	fmt.Printf("%s: %s to %s\n", m.Name, m.Version, ver)
	if consumer != "" {
		if err := bumpConsumer(dir, m, ver, consumer); err != nil {
			return err
		}
	}
	if changelog != "" {
		if err := addChangelog(changelog, "**"+m.Name+" "+ver+":** "+note); err != nil {
			return err
		}
	}
	return checkOne(dir, "")
}

// bumpConsumer raises the requirement in what uses this plugin: its go.mod line and its
// web dependency, whichever it has. Run go mod tidy there afterwards.
func bumpConsumer(dir string, m *wittymodule.Manifest, ver, consumer string) error {
	changed := false
	if b, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
		mod := ""
		for _, line := range strings.Split(string(b), "\n") {
			if f := strings.Fields(line); len(f) == 2 && f[0] == "module" {
				mod = f[1]
				break
			}
		}
		path := filepath.Join(consumer, "go.mod")
		if data, err := os.ReadFile(path); err == nil && mod != "" {
			re := regexp.MustCompile(`(?m)^(\s*(?:require\s+)?` + regexp.QuoteMeta(mod) + `\s+)v\S+`)
			if re.Match(data) {
				out := re.ReplaceAll(data, []byte("${1}v"+ver))
				if err := os.WriteFile(path, out, 0o644); err != nil {
					return err
				}
				fmt.Println("raised", mod, "in", path, "(run go mod tidy there)")
				changed = true
			}
		}
	}
	path := filepath.Join(consumer, "web", "package.json")
	if data, err := os.ReadFile(path); err == nil {
		re := regexp.MustCompile(`(?i)(github:` + regexp.QuoteMeta(m.Repo) + `#)v[^"]+`)
		if re.Match(data) {
			if err := os.WriteFile(path, re.ReplaceAll(data, []byte("${1}v"+ver)), 0o644); err != nil {
				return err
			}
			fmt.Println("raised", m.Repo, "in", path)
			changed = true
		}
	}
	if !changed {
		fmt.Println("nothing in", consumer, "names a released", m.Name, "yet")
	}
	return nil
}

// addChangelog adds a line at the top of the Unreleased section.
func addChangelog(path, line string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	eol := "\n"
	if strings.Contains(s, "\r\n") {
		eol = "\r\n"
	}
	const head = "## Unreleased"
	i := strings.Index(s, head)
	if i < 0 {
		return fmt.Errorf("%s has no %q section", path, head)
	}
	at := i + len(head)
	if j := strings.Index(s[at:], "\n"); j >= 0 {
		at += j + 1
	}
	return os.WriteFile(path, []byte(s[:at]+eol+"- "+line+eol+s[at:]), 0o644)
}

// needsBump fails when a plugin changed since its last tag and still has that tag's
// version. Published tags never move, so what changed could otherwise never reach anyone
// under its own number. Documentation doesn't count. It runs in the plugin's own
// repository, where tags are its own.
func needsBump(args []string) error {
	if len(args) != 1 {
		return errors.New("which folder? wittymodule needs-bump <folder>")
	}
	dir := args[0]
	m, _, err := readManifest(dir)
	if err != nil {
		return err
	}
	out, err := gitIn(dir, "describe", "--tags", "--abbrev=0", "--match", "v*")
	if err != nil {
		fmt.Println("no tag yet, so nothing to compare with")
		return nil
	}
	tag := strings.TrimSpace(out)
	if wittymodule.Compare(m.Version, tag) > 0 {
		fmt.Printf("%s is ahead of %s\n", m.Version, tag)
		return nil
	}
	changed, err := gitIn(dir, "diff", "--name-only", tag, "HEAD", "--", ".", ":!*.md", ":!wiki", ":!.github", ":!LICENSE", ":!NOTICE")
	if err != nil {
		return fmt.Errorf("comparing with %s: %w", tag, err)
	}
	if strings.TrimSpace(changed) == "" {
		fmt.Println("nothing but documentation changed since", tag)
		return nil
	}
	return fmt.Errorf("%s changed since %s, and module.json still says %s. Run: wittymodule bump %s", m.Name, tag, m.Version, dir)
}

func gitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.Output()
	return string(b), err
}
