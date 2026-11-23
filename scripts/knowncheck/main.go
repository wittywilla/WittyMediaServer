// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command knowncheck is a developer and CI tool and never ships. It checks that the list of
// modules built into the server (internal/modules/known.json) says what the published catalogue
// says, apart from versions, the serial and the date. A server shows that list until it has read
// a catalogue, and an old copy once had every module's icon hash wrong (#384, #396).
//
//	go run ./scripts/knowncheck          # fails, naming each difference
//	go run ./scripts/knowncheck -write   # copies the published catalogue into known.json
//
// -catalog reads another address or a local file instead of the published one.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/update"
)

const knownPath = "internal/modules/known.json"

// ignored differ between any two catalogues and say nothing about what a module is.
var (
	ignoredTop    = map[string]bool{"serial": true, "generated": true}
	ignoredModule = map[string]bool{"versions": true}
)

func main() {
	from := flag.String("catalog", update.ModuleCatalog(), "the catalogue to compare with: an https address or a file")
	write := flag.Bool("write", false, "copy the catalogue into "+knownPath+" instead of comparing")
	flag.Parse()

	live, err := read(*from)
	if err != nil {
		fail("reading the catalogue: %v", err)
	}
	if *write {
		if _, err := decode(live); err != nil {
			fail("the catalogue is unreadable: %v", err)
		}
		if err := os.WriteFile(knownPath, live, 0o644); err != nil {
			fail("%v", err)
		}
		fmt.Println("wrote", knownPath)
		return
	}
	known, err := os.ReadFile(knownPath)
	if err != nil {
		fail("%v", err)
	}
	diffs, err := compare(known, live)
	if err != nil {
		fail("%v", err)
	}
	if len(diffs) == 0 {
		fmt.Println(knownPath, "matches the published catalogue, apart from versions")
		return
	}
	for _, d := range diffs {
		fmt.Println(d)
	}
	fmt.Printf("\n%s differs from the published catalogue in %d places. Run: go run ./scripts/knowncheck -write\n", knownPath, len(diffs))
	os.Exit(1)
}

func read(from string) ([]byte, error) {
	if !strings.HasPrefix(from, "https://") && !strings.HasPrefix(from, "http://") {
		return os.ReadFile(from)
	}
	c := &http.Client{Timeout: time.Minute}
	var last error
	for try := 0; try < 3; try++ {
		if try > 0 {
			time.Sleep(time.Duration(try) * 5 * time.Second)
		}
		resp, err := c.Get(from)
		if err != nil {
			last = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		resp.Body.Close()
		if err == nil && resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("%s: %s", from, resp.Status)
		}
		if err == nil {
			return body, nil
		}
		last = err
	}
	return nil, last
}

func decode(b []byte) (map[string]any, error) {
	var v map[string]any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	if _, ok := v["modules"].([]any); !ok {
		return nil, fmt.Errorf("no list of modules")
	}
	return v, nil
}

// compare names every difference between two catalogues, apart from the ignored fields.
func compare(knownJSON, liveJSON []byte) ([]string, error) {
	known, err := decode(knownJSON)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", knownPath, err)
	}
	live, err := decode(liveJSON)
	if err != nil {
		return nil, fmt.Errorf("the catalogue: %v", err)
	}
	var diffs []string
	for _, k := range keys(known, live) {
		if k != "modules" && !ignoredTop[k] && !reflect.DeepEqual(known[k], live[k]) {
			diffs = append(diffs, fmt.Sprintf("%q: built in %s, published %s", k, show(known[k]), show(live[k])))
		}
	}
	km, lm := byID(known), byID(live)
	for _, id := range keys(km, lm) {
		a, b := km[id], lm[id]
		switch {
		case a == nil:
			diffs = append(diffs, id+": published, but not built in")
		case b == nil:
			diffs = append(diffs, id+": built in, but no longer published")
		default:
			for _, f := range keys(a, b) {
				if !ignoredModule[f] && !reflect.DeepEqual(a[f], b[f]) {
					diffs = append(diffs, fmt.Sprintf("%s %s: built in %s, published %s", id, f, show(a[f]), show(b[f])))
				}
			}
		}
	}
	return diffs, nil
}

func byID(cat map[string]any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, m := range cat["modules"].([]any) {
		if mm, ok := m.(map[string]any); ok {
			id, _ := mm["id"].(string)
			out[id] = mm
		}
	}
	return out
}

func keys[V any](maps ...map[string]V) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out
}

func show(v any) string {
	if v == nil {
		return "nothing"
	}
	b, _ := json.Marshal(v)
	if len(b) > 120 {
		return string(b[:117]) + "..."
	}
	return string(b)
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "knowncheck: "+format+"\n", a...)
	os.Exit(2)
}
