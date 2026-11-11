// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var routeRow = regexp.MustCompile("^\\| `((?:GET|POST|PUT|PATCH|DELETE) [^`]+)` \\| ([^|]+) \\| (.+) \\|$")

// docs/SECURITY-ROUTES.md is the public statement of who may call what. It names every route and
// no other, says Admin and Public where the code does, and says what routeRules says.
func TestRouteTableMatchesTheCode(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "docs", "SECURITY-ROUTES.md"))
	if err != nil {
		t.Fatal(err)
	}
	who, what := map[string]string{}, map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		if m := routeRow.FindStringSubmatch(line); m != nil {
			if who[m[1]] != "" {
				t.Errorf("%s is in the table twice", m[1])
			}
			who[m[1]], what[m[1]] = m[2], m[3]
		}
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range routeRe.FindAllSubmatch(src, -1) {
			route, handler := string(m[1]), string(m[3])
			seen[route] = true
			w := who[route]
			if w == "" {
				t.Errorf("%s (%s) isn't in docs/SECURITY-ROUTES.md", route, name)
				continue
			}
			admin := strings.HasPrefix(handler, "s.requireAdmin") || strings.HasPrefix(handler, "ad(") || strings.HasPrefix(handler, "adm(")
			if admin != strings.HasPrefix(w, "Admin") {
				t.Errorf("%s goes through %s, and the table says %q", route, handler, w)
			}
			if publicRoutes[route] != strings.HasPrefix(w, "Public") {
				t.Errorf("%s: public %v, and the table says %q", route, publicRoutes[route], w)
			}
			if rule, ok := routeRules[route]; ok && rule != w+": "+what[route] {
				t.Errorf("%s: the table says %q and route_rules_test.go %q", route, w+": "+what[route], rule)
			}
		}
	}
	if len(seen) < 400 {
		t.Fatalf("found only %d routes: has the way they are declared changed?", len(seen))
	}
	for route := range who {
		if !seen[route] {
			t.Errorf("%s is in docs/SECURITY-ROUTES.md but isn't a route", route)
		}
	}
}
