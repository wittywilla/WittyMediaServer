// Command gen writes internal/licenses/libraries.json.gz: every library compiled into
// what a release ships, with its license and its full text, for Settings, About.
//
// Run it from the repository's root after adding or changing a dependency, with the web
// app's packages installed (npm ci in web/):
//
//	go run ./internal/licenses/gen
//
// A license it can't name stops it, so that nothing is listed as unknown: add the
// module to overrides after reading what its license says.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/licenses"
)

// overrides are read by hand: one file holding several licenses, or wording the
// patterns below don't catch.
var overrides = map[string]string{
	"github.com/klauspost/compress": "BSD-3-Clause, Apache-2.0 and MIT",
}

const inHouse = "github.com/wittywilla/"

func main() {
	out := licenses.File{}
	text := map[string]int{}
	add := func(group, name, url, license, body string) {
		i, ok := text[body]
		if !ok {
			i = len(out.Texts)
			text[body] = i
			out.Texts = append(out.Texts, body)
		}
		out.Libraries = append(out.Libraries, licenses.Library{Group: group, Name: name, URL: url, License: license, Text: i})
	}

	goroot, err := output("", "go", "env", "GOROOT")
	check(err)
	std, err := os.ReadFile(filepath.Join(strings.TrimSpace(goroot), "LICENSE"))
	check(err)
	add(licenses.GroupServer, "The Go runtime and standard library", "https://go.dev", "BSD-3-Clause", string(std))

	mods := goModules()
	var paths []string
	for p := range mods {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		body := readLicenses(mods[p])
		add(licenses.GroupServer, p, moduleURL(p), name(p, body, ""), body)
	}

	for _, pkg := range webPackages() {
		body := readLicenses(pkg.dir)
		add(licenses.GroupWeb, pkg.name, "https://www.npmjs.com/package/"+pkg.name, name(pkg.name, body, pkg.spdx), body)
	}

	var b bytes.Buffer
	z, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	enc := json.NewEncoder(z)
	check(enc.Encode(out))
	check(z.Close())
	check(os.WriteFile(filepath.Join("internal", "licenses", "libraries.json.gz"), b.Bytes(), 0o644))
	fmt.Printf("%d libraries, %d different license texts, %d KB\n", len(out.Libraries), len(out.Texts), b.Len()/1024)
}

// goModules finds every module compiled into a program a release ships, on any
// platform, since some are only used on one.
func goModules() map[string]string {
	targets := []string{"./cmd/wms", "./cmd/wms-tray", "./cmd/wms-setup"}
	cmds, _ := filepath.Glob(filepath.Join("modules", "*", "cmd", "*"))
	for _, c := range cmds {
		targets = append(targets, "./"+filepath.ToSlash(c))
	}
	mods := map[string]string{}
	for _, goos := range []string{"linux", "windows", "darwin", "freebsd"} {
		args := append([]string{"list", "-e", "-deps", "-f", `{{with .Module}}{{if not .Main}}{{.Path}}|{{.Dir}}{{end}}{{end}}`}, targets...)
		cmd := exec.Command("go", args...)
		cmd.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=0")
		b, err := cmd.Output()
		if err != nil {
			fail("go list for %s: %v", goos, err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			p, dir, ok := strings.Cut(strings.TrimSpace(line), "|")
			if ok && dir != "" && !strings.HasPrefix(p, inHouse) {
				mods[p] = dir
			}
		}
	}
	return mods
}

var versionSuffix = regexp.MustCompile(`/v[0-9]+$`)

func moduleURL(p string) string {
	if strings.HasPrefix(p, "github.com/") {
		return "https://" + versionSuffix.ReplaceAllString(p, "")
	}
	return "https://pkg.go.dev/" + p
}

type webPackage struct{ name, dir, spdx string }

type npmNode struct {
	Dependencies map[string]npmNode `json:"dependencies"`
}

// webPackages finds what ends up in the web app: not its build tools, nor our own modules.
func webPackages() []webPackage {
	web := "web"
	cmd := "npm"
	if runtime.GOOS == "windows" {
		cmd = "npm.cmd"
	}
	raw, err := output(web, cmd, "ls", "--omit=dev", "--all", "--json")
	if err != nil && raw == "" {
		fail("npm ls: %v. Run npm ci in web/ first", err)
	}
	var root npmNode
	check(json.Unmarshal([]byte(raw), &root))
	seen := map[string]bool{}
	var walk func(n npmNode)
	walk = func(n npmNode) {
		for name, dep := range n.Dependencies {
			if !strings.HasPrefix(name, "@wittywilla/") && !strings.HasPrefix(name, "@napi-rs/") {
				seen[name] = true // @napi-rs: what pdf.js uses under Node, never in a browser
			}
			walk(dep)
		}
	}
	walk(root)
	var out []webPackage
	for name := range seen {
		dir := filepath.Join(web, "node_modules", filepath.FromSlash(name))
		var pkg struct {
			License string `json:"license"`
		}
		b, err := os.ReadFile(filepath.Join(dir, "package.json"))
		if err != nil {
			continue // an optional package this platform didn't install
		}
		check(json.Unmarshal(b, &pkg))
		out = append(out, webPackage{name: name, dir: dir, spdx: pkg.License})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

var licenseFile = regexp.MustCompile(`(?i)^(licen[cs]e|copying|notice)`)

// readLicenses joins every license and notice file at the top of a package.
func readLicenses(dir string) string {
	entries, err := os.ReadDir(dir)
	check(err)
	var parts []string
	for _, e := range entries {
		if e.IsDir() || !licenseFile.MatchString(e.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		check(err)
		parts = append(parts, strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n")))
	}
	return strings.Join(parts, "\n\n----------------------------------------\n\n")
}

// name says which license a text is. Only the wording of the text counts for a Go
// module; a web package also says in its package.json.
func name(pkg, body, declared string) string {
	if o, ok := overrides[pkg]; ok {
		return o
	}
	if declared != "" {
		if body == "" {
			fail("%s says %s but ships no license file", pkg, declared)
		}
		return declared
	}
	if body == "" {
		fail("%s has no license file: read its repository, and add it to overrides", pkg)
	}
	switch t := strings.Join(strings.Fields(body), " "); {
	case strings.Contains(t, "Apache License") && strings.Contains(t, "Version 2.0"):
		return "Apache-2.0"
	case strings.Contains(t, "Mozilla Public License") && strings.Contains(t, "2.0"):
		return "MPL-2.0"
	case strings.Contains(t, "Permission is hereby granted, free of charge"):
		return "MIT"
	case strings.Contains(t, "Permission to use, copy, modify, and") && strings.Contains(t, "distribute this software for any purpose"):
		return "ISC"
	case strings.Contains(t, "Redistribution and use in source and binary forms"):
		if strings.Contains(t, "endorse or promote") {
			return "BSD-3-Clause"
		}
		return "BSD-2-Clause"
	case strings.Contains(t, "This software is provided 'as-is'"):
		return "Zlib"
	}
	fail("%s: this license isn't one the patterns know. Read it, and add it to overrides", pkg)
	return ""
}

func output(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	b, err := cmd.Output()
	return string(b), err
}

func check(err error) {
	if err != nil {
		fail("%v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
