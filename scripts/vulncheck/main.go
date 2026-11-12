// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command vulncheck is a developer and CI tool and never ships. It reads govulncheck's JSON and
// fails on every vulnerability this code can reach, except the few listed in accepted below,
// each with the reason it can't be fixed and why it doesn't hurt here. An accepted one stops
// being accepted as soon as a fixed version exists, or when it is reached through any other
// module, so an exception can't outlive its reason.
//
//	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -format json ./internal/... ./cmd/... | go run ./scripts/vulncheck
//
// Plain govulncheck exits non-zero on the accepted ones too; this is what CI runs.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// accepted are the vulnerabilities this code reaches knowingly: no release fixes them, and
// what they allow is understood and bounded here. Each is held to one module.
var accepted = map[string]struct {
	module string
	why    string
}{
	// Content censorship by Kademlia DHT abuse (CVE-2023-26248, GHSA-mqr9-hjr8-2m9w): anyone
	// may run many DHT servers with IDs close to a key and answer for it, hiding a record. It
	// is how a public Kademlia DHT works, every version of go-libp2p-kad-dht is listed, and no
	// fix exists or is planned. Peer-to-peer connections use the DHT only to find a server and
	// community relays (docs/P2P.md, "Finding the server"): an attacker who knows a server's
	// connection ID could stop an app finding it there, and nothing more. Every connection is
	// still proved against the server's key, apps try the addresses they remember and the
	// invite's first, a join code's key comes from the code through Argon2id, the DHT layer can
	// be switched off (dht), and the feature is off until an owner turns it on.
	"GO-2024-3218": {module: "github.com/libp2p/go-libp2p-kad-dht", why: "a public DHT can be made to hide a record; peer to peer only finds the server through it (docs/P2P.md)"},
}

type frame struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Package  string `json:"package"`
	Function string `json:"function"`
	Receiver string `json:"receiver"`
	Position *struct {
		Filename string `json:"filename"`
		Line     int    `json:"line"`
	} `json:"position"`
}

type finding struct {
	OSV   string  `json:"osv"`
	Fixed string  `json:"fixed_version"`
	Trace []frame `json:"trace"`
}

type osv struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

// problem is one vulnerability that fails the check.
type problem struct {
	id, summary, module, version, fixed, why, via string
}

// check reads govulncheck's JSON stream and says what fails the build, and what was accepted.
func check(r io.Reader) (problems []problem, acceptedSeen []string, err error) {
	dec := json.NewDecoder(bufio.NewReader(r))
	summaries := map[string]string{}
	type key struct{ id, module string }
	seen := map[key]bool{}
	ok := map[string]bool{}
	var findings []finding
	sawConfig := false
	for {
		var msg struct {
			Config  json.RawMessage `json:"config"`
			OSV     *osv            `json:"osv"`
			Finding *finding        `json:"finding"`
		}
		if err := dec.Decode(&msg); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, nil, fmt.Errorf("govulncheck's output can't be read: %w", err)
		}
		if msg.Config != nil {
			sawConfig = true
		}
		if msg.OSV != nil {
			summaries[msg.OSV.ID] = msg.OSV.Summary
		}
		if msg.Finding != nil {
			findings = append(findings, *msg.Finding)
		}
	}
	if !sawConfig {
		return nil, nil, errors.New("this isn't govulncheck's JSON (no config message): run it with -format json")
	}
	for _, f := range findings {
		// Only a finding with a function at its top is code this module calls; the others
		// are in modules or packages it merely depends on.
		if len(f.Trace) == 0 || f.Trace[0].Function == "" {
			continue
		}
		top := f.Trace[0]
		k := key{f.OSV, top.Module}
		if seen[k] {
			continue
		}
		seen[k] = true
		p := problem{id: f.OSV, summary: summaries[f.OSV], module: top.Module, version: top.Version, fixed: f.Fixed, via: describe(f.Trace)}
		a, listed := accepted[f.OSV]
		switch {
		case !listed:
		case a.module != top.Module:
			p.why = "accepted only in " + a.module
		case f.Fixed != "":
			p.why = "accepted while nothing fixed it, and " + f.Fixed + " does: upgrade, and take it off the accepted list"
		default:
			ok[f.OSV] = true
			continue
		}
		problems = append(problems, p)
	}
	for id := range ok {
		acceptedSeen = append(acceptedSeen, id)
	}
	sort.Strings(acceptedSeen)
	sort.Slice(problems, func(i, j int) bool { return problems[i].id+problems[i].module < problems[j].id+problems[j].module })
	return problems, acceptedSeen, nil
}

// describe is the shortest useful account of how the code reaches it: the first frame in this
// repository's own code, and the vulnerable function.
func describe(trace []frame) string {
	name := func(f frame) string {
		n := f.Function
		if f.Receiver != "" {
			n = strings.TrimPrefix(f.Receiver, "*") + "." + n
		}
		if f.Package != "" {
			n = f.Package[strings.LastIndex(f.Package, "/")+1:] + "." + n
		}
		return n
	}
	vuln := name(trace[0])
	for i := len(trace) - 1; i > 0; i-- {
		if f := trace[i]; f.Position != nil && f.Function != "" {
			return fmt.Sprintf("%s:%d %s calls %s", f.Position.Filename, f.Position.Line, name(f), vuln)
		}
	}
	return vuln
}

func main() {
	problems, ok, err := check(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "vulncheck:", err)
		os.Exit(2)
	}
	for _, id := range ok {
		fmt.Printf("accepted %s in %s: %s\n", id, accepted[id].module, accepted[id].why)
	}
	if len(problems) == 0 {
		fmt.Println("No vulnerabilities this code reaches, other than those accepted above.")
		return
	}
	for _, p := range problems {
		fmt.Printf("\n%s: %s\n  in %s@%s\n", p.id, p.summary, p.module, p.version)
		if p.fixed != "" {
			fmt.Printf("  fixed in %s\n", p.fixed)
		} else {
			fmt.Println("  no fixed version yet")
		}
		fmt.Printf("  reached: %s\n", p.via)
		if p.why != "" {
			fmt.Printf("  not accepted: %s\n", p.why)
		}
		fmt.Printf("  https://pkg.go.dev/vuln/%s\n", p.id)
	}
	fmt.Printf("\n%d vulnerabilities this code reaches.\n", len(problems))
	os.Exit(1)
}
