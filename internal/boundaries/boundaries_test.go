// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package boundaries

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const internal = "github.com/wittywilla/wittymediaserver/internal/"

// movable is each package that is to leave the server (docs/plans/module-isolation.md), with
// what else of the server it may import until then. A package's own folders are always allowed.
var movable = map[string][]string{
	"dvd":              nil, // generated from WittyDVD
	"bluray":           nil, // generated from WittyBluray
	"cd":               nil, // generated from WittyCD
	"cdlookup":         {"cd"},
	"tuner":            {"tuner/mpegts"},
	"tuner/mpegts":     nil, // WittyBroadcast
	"tuner/xmltv":      nil, // WittyBroadcast
	"gameid":           nil, // WittyGameCore
	"gameregion":       nil,
	"zipsafe":          nil,
	"play/hostsandbox": nil,
	"ebook":            nil, // WittyEbook
	"midi":             nil,
	"lyrics":           nil,
}

// Every file counts, tests and other systems' files included, since all of them move.
func TestMovablePackagesStandAlone(t *testing.T) {
	names := make([]string, 0, len(movable))
	for p := range movable {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, pkg := range names {
		root := filepath.Join("..", filepath.FromSlash(pkg))
		if _, err := os.Stat(root); err != nil {
			t.Errorf("internal/%s isn't there: if it moved, take it off this list", pkg)
			continue
		}
		allowed := map[string]bool{}
		for _, a := range movable[pkg] {
			allowed[a] = true
		}
		files := 0
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			files++
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imp := range f.Imports {
				p, _ := strconv.Unquote(imp.Path.Value)
				other, ok := strings.CutPrefix(p, internal)
				if !ok || other == pkg || strings.HasPrefix(other, pkg+"/") || allowed[other] {
					continue
				}
				rel, _ := filepath.Rel("..", path)
				t.Errorf("internal/%s imports internal/%s, so it can't leave the server on its own", filepath.ToSlash(rel), other)
			}
			return nil
		})
		if err != nil {
			t.Errorf("internal/%s: %v", pkg, err)
		}
		if files == 0 {
			t.Errorf("internal/%s has no Go files", pkg)
		}
	}
}
