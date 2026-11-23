// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command testshard runs one share of the Go tests, so that CI can run the shares side by side on
// separate machines: the race detector makes the whole set too slow for one (#395).
//
//	go run ./scripts/testshard -shards 4 -shard 1 [-split ./internal/server] -- -race -count=1 ./internal/...
//
// Each package goes whole to one share, the heaviest first to the lightest share. A package named
// with -split is too slow for one machine: its top-level tests are shared out one by one, by name,
// so each share runs a part of it with -run. Every test is in exactly one share: -check lists the
// shares and fails if one is missed or run twice, without running anything.
//
// The weights are only for balance: a package missing from them counts as light, and a wrong
// weight makes one share slower, never a test left out.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

// weights are seconds under the race detector on a CI machine (run 45100755163), for the packages
// that take more than a few. Anything else counts as defaultWeight.
var weights = map[string]float64{
	"internal/server":       2740,
	"internal/brandart":     907,
	"internal/library":      448,
	"internal/libtranscode": 422,
	"internal/modules":      162,
	"internal/backup":       148,
	"internal/diagnose":     140,
	"internal/livetv":       108,
	"internal/auth":         105,
	"internal/p2p":          77,
	"internal/legal":        74,
	"internal/archive":      64,
	"internal/db":           63,
	"internal/channels":     47,
	"internal/hls":          45,
	"internal/zipsafe":      37,
	"internal/seamcheck":    33,
	"internal/docscheck":    27,
}

const defaultWeight = 5

var (
	shards = flag.Int("shards", 1, "how many shares")
	shard  = flag.Int("shard", 1, "which share to run, from 1")
	split  = flag.String("split", "", "packages to share out test by test, comma-separated (./internal/server)")
	check  = flag.Bool("check", false, "list every share and check each test is in exactly one; run nothing")
	module = "github.com/wittywilla/wittymediaserver/"
)

type run struct {
	pkgs  []string // whole packages
	split map[string][]string
}

func main() {
	flag.Parse()
	var goArgs, patterns []string
	// Packages are named from the repository (./internal/...) or by import path; anything
	// else is a flag for go test, or a flag's value (-timeout 60m).
	for _, a := range flag.Args() {
		if strings.HasPrefix(a, "./") || strings.HasPrefix(a, module) {
			patterns = append(patterns, a)
		} else {
			goArgs = append(goArgs, a)
		}
	}
	if *shards < 1 || *shard < 1 || *shard > *shards || len(patterns) == 0 {
		fmt.Fprintln(os.Stderr, "usage: testshard -shards N -shard I [-split pkgs] [-check] -- [go test flags] packages")
		os.Exit(2)
	}
	pkgs := testPackages(patterns, goArgs)
	splitSet := map[string]bool{}
	for _, p := range strings.Split(*split, ",") {
		if p = strings.TrimSpace(p); p != "" {
			splitSet[importPath(p)] = true
		}
	}
	tests := map[string][]string{}
	for p := range splitSet {
		tests[p] = listTests(p, goArgs)
	}
	plan := assign(pkgs, tests, *shards)
	if *check {
		os.Exit(verify(plan, pkgs, tests))
	}
	r := plan[*shard-1]
	failed := false
	if len(r.pkgs) > 0 {
		failed = !goTest(append(append([]string{}, goArgs...), r.pkgs...)) || failed
	}
	for _, p := range sortedKeys(r.split) {
		names := r.split[p]
		args := append(append([]string{}, goArgs...), "-run", "^("+strings.Join(quote(names), "|")+")$", p)
		failed = !goTest(args) || failed
	}
	if failed {
		os.Exit(1)
	}
}

// assign shares the packages out, heaviest first to the lightest share; a split package's tests
// go round the shares in an order fixed by their names, each share taking a part of its weight.
func assign(pkgs []string, tests map[string][]string, n int) []run {
	plan := make([]run, n)
	load := make([]float64, n)
	for i := range plan {
		plan[i].split = map[string][]string{}
	}
	for p, names := range tests {
		sorted := append([]string{}, names...)
		sort.Slice(sorted, func(i, j int) bool {
			return hash(sorted[i]) < hash(sorted[j]) || hash(sorted[i]) == hash(sorted[j]) && sorted[i] < sorted[j]
		})
		for i, name := range sorted {
			plan[i%n].split[p] = append(plan[i%n].split[p], name)
		}
		for i := range load {
			load[i] += weight(p) / float64(n)
		}
	}
	var whole []string
	for _, p := range pkgs {
		if _, ok := tests[p]; !ok {
			whole = append(whole, p)
		}
	}
	sort.SliceStable(whole, func(i, j int) bool {
		if weight(whole[i]) != weight(whole[j]) {
			return weight(whole[i]) > weight(whole[j])
		}
		return whole[i] < whole[j]
	})
	for _, p := range whole {
		low := 0
		for i := range load {
			if load[i] < load[low] {
				low = i
			}
		}
		plan[low].pkgs = append(plan[low].pkgs, p)
		load[low] += weight(p)
	}
	return plan
}

// verify checks that every package and every test of a split package is in exactly one share.
func verify(plan []run, pkgs []string, tests map[string][]string) int {
	seen := map[string]int{}
	for i, r := range plan {
		w := 0.0
		for _, p := range r.pkgs {
			seen[p]++
			w += weight(p)
		}
		n := 0
		for p, names := range r.split {
			for _, t := range names {
				seen[p+" "+t]++
			}
			n += len(names)
			w += weight(p) / float64(len(plan))
		}
		fmt.Printf("share %d: %d packages, %d split tests, weight %.0f\n", i+1, len(r.pkgs), n, w)
	}
	bad := 0
	for _, p := range pkgs {
		if names, ok := tests[p]; ok {
			if len(names) == 0 {
				fmt.Printf("%s: no tests listed\n", p)
				bad++
			}
			for _, t := range names {
				if seen[p+" "+t] != 1 {
					fmt.Printf("%s %s: in %d shares\n", p, t, seen[p+" "+t])
					bad++
				}
			}
		} else if seen[p] != 1 {
			fmt.Printf("%s: in %d shares\n", p, seen[p])
			bad++
		}
	}
	if bad > 0 {
		return 1
	}
	fmt.Println("every package and test is in exactly one share")
	return 0
}

// testPackages is every package the patterns name that has tests to run.
func testPackages(patterns, goArgs []string) []string {
	args := append([]string{"list"}, tagArgs(goArgs)...)
	args = append(args, "-f", "{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}")
	out, err := exec.Command("go", append(args, patterns...)...).Output()
	if err != nil {
		fail("go list: %v", err)
	}
	var pkgs []string
	for _, l := range strings.Fields(string(out)) {
		pkgs = append(pkgs, l)
	}
	return pkgs
}

var listed = regexp.MustCompile(`^(Test|Fuzz|Example)\w*$`)

// listTests is a package's top-level tests, fuzz targets and examples: each runs under -run.
func listTests(pkg string, goArgs []string) []string {
	args := append([]string{"test", "-list", "."}, tagArgs(goArgs)...)
	cmd := exec.Command("go", append(args, pkg)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		fail("go test -list %s: %v\n%s%s", pkg, err, out, stderr.Bytes())
	}
	var names []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); listed.MatchString(l) {
			names = append(names, l)
		}
	}
	return names
}

// tagArgs keeps the flags that change which files are built, so the lists match what is run.
func tagArgs(goArgs []string) []string {
	var out []string
	for i, a := range goArgs {
		switch {
		case a == "-race" || strings.HasPrefix(a, "-tags="):
			out = append(out, a)
		case a == "-tags" && i+1 < len(goArgs):
			out = append(out, a, goArgs[i+1])
		}
	}
	return out
}

func goTest(args []string) bool {
	fmt.Printf("go test %s\n", strings.Join(shorten(args), " "))
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run() == nil
}

// shorten keeps the printed command readable: a -run list of hundreds of names is summarised.
func shorten(args []string) []string {
	out := append([]string{}, args...)
	for i, a := range out {
		if i > 0 && out[i-1] == "-run" && len(a) > 200 {
			out[i] = fmt.Sprintf("<%d tests>", strings.Count(a, "|")+1)
		}
	}
	return out
}

func quote(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = regexp.QuoteMeta(n)
	}
	return out
}

func importPath(p string) string {
	return module + strings.TrimPrefix(strings.TrimPrefix(p, "./"), module)
}

func weight(pkg string) float64 {
	if w, ok := weights[strings.TrimPrefix(pkg, module)]; ok {
		return w
	}
	return defaultWeight
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func sortedKeys(m map[string][]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "testshard: "+format+"\n", a...)
	os.Exit(1)
}
