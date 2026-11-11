// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/gamearchive"
	"github.com/wittywilla/wittymediaserver/internal/gameid"
)

// Run by hand with WMS_REAL_GAMES set to a real folder of console folders, and
// WMS_REAL_OUT to a folder for a report outside any repository: it names games. It only
// reads the games folder, and shows what a scan makes of each console's folder.
func TestRealGamesReport(t *testing.T) {
	root, out := os.Getenv("WMS_REAL_GAMES"), os.Getenv("WMS_REAL_OUT")
	if root == "" || out == "" {
		t.Skip("set WMS_REAL_GAMES and WMS_REAL_OUT to run this")
	}
	start := time.Now()
	ix := newArchiveIndex(nil, 0)
	defer ix.close()
	found, err := ix.discover(root, "auto")
	if err != nil {
		t.Log(err)
	}
	t.Logf("%d games in %v", len(found), time.Since(start).Round(time.Millisecond))
	files := map[string]int{}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			files[strings.SplitN(rel, string(filepath.Separator), 2)[0]]++
		}
		return nil
	})
	type line struct {
		Files int
		Games map[string]int
		Left  map[string]int
	}
	report := map[string]*line{}
	at := func(p string) *line {
		rel, _ := filepath.Rel(root, p)
		top := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if report[top] == nil {
			report[top] = &line{Files: files[top], Games: map[string]int{}, Left: map[string]int{}}
		}
		return report[top]
	}
	f, err := os.Create(filepath.Join(out, "report.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, c := range found {
		at(c.path).Games[c.console]++
		v := gameVerdict{}
		if c.verdict != nil {
			v = *c.verdict
		}
		discs := []string{}
		for _, d := range c.discs {
			discs = append(discs, filepath.Base(d.path))
		}
		title, tags := gameFileTitle(fileBaseName(c.path, v), v)
		_ = enc.Encode(map[string]any{"game": filepath.Base(c.path), "console": c.console, "how": v.How, "sure": v.Sure, "why": v.Proof, "entry": v.Entry, "entries": v.Entries,
			"title": title, "tags": tags, "headerTitle": v.Title, "serial": v.Serial, "discs": discs, "problem": v.Problem})
	}
	for _, n := range ix.notes {
		at(n.path).Left[n.kind]++
		_ = enc.Encode(map[string]any{"left": n.kind, "file": filepath.Base(n.path), "why": n.reason, "suggest": n.suggest})
	}
	// Whether what would be played really is for its console, by its own bytes. It reads
	// into every archive, so it's only done when asked for.
	if os.Getenv("WMS_REAL_VERIFY") != "" {
		tally := map[string]int{}
		l := gamearchive.NewLister()
		defer l.Close()
		for _, c := range found {
			if c.verdict == nil || c.verdict.Entry == "" || !gamearchive.IsArchive(c.path) || c.verdict.How == howBytes {
				continue
			}
			b, err := l.Peek(c.path, c.verdict.Entry)
			f := gameid.Finding{}
			if err == nil {
				f = gameid.SniffHead(b, max(int64(len(b)), c.verdict.Payload))
			}
			key := c.console + " " + f.Console
			if err != nil {
				key = c.console + " unreadable"
			}
			tally[key]++
			if f.Console != c.console {
				_ = enc.Encode(map[string]any{"unproven": filepath.Base(c.path), "entry": c.verdict.Entry, "console": c.console, "says": f.Console, "kind": f.Kind, "proof": f.Proof, "err": fmt.Sprint(err), "size": c.verdict.Payload})
			}
		}
		t.Logf("checking what would be played, by console and what its bytes say: %v", tally)
	}
	keys := make([]string, 0, len(report))
	for k := range report {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%-40s files %5d games %v left %v", k, report[k].Files, report[k].Games, report[k].Left)
	}
}
