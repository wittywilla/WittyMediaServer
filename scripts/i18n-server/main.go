// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command i18n-server finds the English words the server sends to people and checks that each one goes through
// translation: that it's written where the scanner knows it reaches a person (a writeErr message, a label or
// description field, i18n.T), and that every language's catalogue in web/src/i18n/locales has it.
//
// From the repository root:
//
//	go run ./scripts/i18n-server            lists what is missing, and fails when anything is
//	go run ./scripts/i18n-server -write     rewrites web/src/i18n/server-keys.json (the list of the server's keys)
//	go run ./scripts/i18n-server -keys      prints every key found, one per line
//
// Two things fail the check:
//   - a sentence of English written somewhere the scanner doesn't know (passed to a function, kept in a variable)
//     without i18n.T around it, in a file that sends answers to people. Wrap it in i18n.T, or, if no person ever
//     reads it (a log line, a test name, a file format), add it to scripts/i18n-server/allow.txt.
//   - a key with no translation in a language's catalogue.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Fields of the server's answers whose text a person reads.
var userFields = map[string]bool{
	"Label": true, "Desc": true, "Description": true, "Message": true, "Summary": true, "Hint": true,
	"Detail": true, "Note": true, "Home": true, "Away": true, "Help": true, "Placeholder": true, "Why": true,
	"Reason": true, "Problem": true, "Fix": true, "Action": true, "Explain": true, "Text": true, "Gets": true,
	"Sees": true, "Title": true, "Heading": true, "Body": true, "Caption": true, "Status": true, "Error": true,
	"Warning": true, "Advice": true, "Tip": true, "Blurb": true, "Subtitle": true, "Button": true, "Prompt": true,
}

// Keys of a map[string]any answer whose text a person reads.
var userKeys = map[string]bool{"error": true, "message": true, "label": true, "desc": true, "hint": true, "detail": true, "summary": true, "note": true, "reason": true, "warning": true}

// Calls whose text isn't for people: logs, tests, formatting, errors kept inside the server.
var quietCalls = regexp.MustCompile(`^(slog|log|logger|l|lg|t|b|tb|f|fmt|errors|strings|regexp|filepath|path|os|exec|sql|db|tx|json|xml|http|url|time|bytes|strconv|sort|w|h|hash|io|bufio|zap|testing|template|flag|cmd|logf|debugf|errorf|warnf|infof)\.`)

type found struct {
	key, pos string
	one      string // a count's message (i18n.N): the English for one, where key is the English for more
}

func main() {
	write := flag.Bool("write", false, "rewrite web/src/i18n/server-keys.json")
	keysOnly := flag.Bool("keys", false, "print the keys")
	strays := flag.Bool("strays", false, "print the unmarked sentences as allow.txt lines")
	strict := flag.Bool("strict", false, "fail when a language has no translation for a key (the translation pass before a release)")
	missingIn := flag.String("missing", "", "print the keys a language has no translation for, with where each is, as JSON")
	flag.Parse()

	var keys []found
	var loose []found
	allow := readAllow()
	fset := token.NewFileSet()
	for _, root := range []string{"internal", "cmd", "pkg"} {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
			if err != nil {
				return nil
			}
			if ast.IsGenerated(f) {
				return nil
			}
			if allowFile(allow, filepath.ToSlash(p)+":0") {
				return nil // written for developers, or shown where nothing translates: no keys, and nothing to mark
			}
			k, l := scan(fset, f, allow)
			keys = append(keys, k...)
			loose = append(loose, l...)
			return nil
		})
	}

	seen := map[string]bool{}
	var list []string
	for _, k := range keys {
		if !seen[k.key] {
			seen[k.key] = true
			list = append(list, k.key)
		}
	}
	sort.Strings(list)
	// The English for one of each count's message, to the English for more (its key): the server sends "1 album", and
	// the web app finds "{count} albums" from it.
	plurals := map[string]string{}
	for _, k := range keys {
		if k.one != "" {
			plurals[k.one] = k.key
		}
	}
	if *keysOnly {
		for _, k := range list {
			fmt.Println(k)
		}
		return
	}
	if *write {
		b, _ := json.MarshalIndent(list, "", "  ")
		pl, _ := json.MarshalIndent(plurals, "", "  ")
		for name, data := range map[string][]byte{"server-keys.json": b, "server-plurals.json": pl} {
			if err := os.WriteFile(filepath.FromSlash("web/src/i18n/"+name), append(data, '\n'), 0o644); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		fmt.Printf("%d keys written to web/src/i18n/server-keys.json, %d counts to server-plurals.json\n", len(list), len(plurals))
		return
	}

	bad := 0
	var strayLines []string
	for _, l := range loose {
		if seen[l.key] || allow[l.key] || allowFile(allow, l.pos) {
			continue
		}
		if *strays {
			strayLines = append(strayLines, l.key)
			continue
		}
		fmt.Printf("%s: English a person may read isn't marked for translation (wrap it in i18n.T, or add it to scripts/i18n-server/allow.txt): %q\n", l.pos, l.key)
		bad++
	}
	if *strays {
		sort.Strings(strayLines)
		for _, s := range strayLines {
			fmt.Println(strconv.Quote(s))
		}
		return
	}

	// The committed key list must match the source, so the web app's checks see the same keys.
	var committed []string
	if b, err := os.ReadFile(filepath.FromSlash("web/src/i18n/server-keys.json")); err == nil {
		_ = json.Unmarshal(b, &committed)
	}
	var committedPlurals map[string]string
	if b, err := os.ReadFile(filepath.FromSlash("web/src/i18n/server-plurals.json")); err == nil {
		_ = json.Unmarshal(b, &committedPlurals)
	}
	samePlurals := len(committedPlurals) == len(plurals)
	for one, other := range plurals {
		samePlurals = samePlurals && committedPlurals[one] == other
	}
	if strings.Join(committed, "\x00") != strings.Join(list, "\x00") || !samePlurals {
		fmt.Println("web/src/i18n/server-keys.json or server-plurals.json is out of date: run  go run ./scripts/i18n-server -write")
		bad++
	}

	firstAt := map[string]string{}
	for _, k := range keys {
		if _, ok := firstAt[k.key]; !ok {
			firstAt[k.key] = k.pos
		}
	}
	dir := filepath.FromSlash("web/src/i18n/locales")
	langs, _ := os.ReadDir(dir)
	untranslated := 0
	for _, lang := range langs {
		if *missingIn != "" && lang.Name() != *missingIn {
			continue
		}
		cat := map[string]any{}
		files, _ := os.ReadDir(filepath.Join(dir, lang.Name()))
		for _, f := range files {
			b, err := os.ReadFile(filepath.Join(dir, lang.Name(), f.Name()))
			if err != nil {
				continue
			}
			part := map[string]any{}
			if err := json.Unmarshal(b, &part); err != nil {
				fmt.Printf("%s/%s: %v\n", lang.Name(), f.Name(), err)
				bad++
				continue
			}
			for k, v := range part {
				cat[k] = v
			}
		}
		missing := 0
		var gaps []map[string]string
		for _, k := range list {
			if _, ok := cat[k]; !ok {
				if *strict && missing < 5 {
					fmt.Printf("%s: no translation for %q\n", lang.Name(), k)
				}
				gaps = append(gaps, map[string]string{"key": k, "at": firstAt[k]})
				missing++
			}
		}
		if *missingIn != "" {
			b, _ := json.MarshalIndent(gaps, "", "  ")
			fmt.Println(string(b))
			return
		}
		if missing > 0 {
			fmt.Printf("%s: %d of %d server keys have no translation yet\n", lang.Name(), missing, len(list))
			untranslated++
		}
	}
	// A key with no translation yet shows in English, as before it was marked; the translation pass before a release
	// (-strict) is where every language has to have them all.
	if *strict && untranslated > 0 {
		bad++
	}
	if bad > 0 {
		os.Exit(1)
	}
	if untranslated > 0 {
		fmt.Printf("%d server keys, every one marked; %d languages have some still to translate (-missing de lists them, -strict fails on them)\n", len(list), untranslated)
		return
	}
	fmt.Printf("%d server keys, every one translated in %d languages\n", len(list), len(langs))
}

func readAllow() map[string]bool {
	out := map[string]bool{}
	f, err := os.Open(filepath.FromSlash("scripts/i18n-server/allow.txt"))
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "file "); ok {
			out["file "+strings.TrimSpace(strings.SplitN(rest, "#", 2)[0])] = true
			continue
		}
		if s, err := strconv.Unquote(strings.TrimSpace(strings.SplitN(line, " #", 2)[0])); err == nil {
			out[s] = true
		}
	}
	return out
}

// allowFile: whether a "file:line" is in a file (or folder, ending in /) that allow.txt leaves out as a whole.
func allowFile(allow map[string]bool, pos string) bool {
	file := pos[:strings.LastIndexByte(pos, ':')]
	for p := file; p != "" && p != "."; {
		if allow["file "+p] || allow["file "+p+"/"] {
			return true
		}
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			break
		}
		p = p[:i]
	}
	return false
}

// A sentence: starts with a capital letter, has at least three words and some lower-case letters.
var sentence = regexp.MustCompile(`^[\p{Lu}][\p{L}'’]*( +[^ ]+){2,}`)

var verb = regexp.MustCompile(`%[-+# 0-9.*]*[a-zA-Z]`)

// SQL, which reads like a sentence that starts with a capital.
var sqlStart = regexp.MustCompile(`^(SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP|PRAGMA|WITH|ATTACH|DETACH|REPLACE|VACUUM|BEGIN|COMMIT|ROLLBACK|ANALYZE|REINDEX) `)

func isWords(s string) bool {
	if sqlStart.MatchString(s) {
		return false
	}
	s = verb.ReplaceAllString(s, "x")
	if !strings.ContainsFunc(s, func(r rune) bool { return r >= 'a' && r <= 'z' }) {
		return false
	}
	if strings.ContainsAny(s, "%\\/<>=_`") || strings.Contains(s, "://") {
		return false
	}
	return strings.Contains(s, " ") || (len(s) > 1 && s[0] >= 'A' && s[0] <= 'Z')
}

func scan(fset *token.FileSet, f *ast.File, allowed map[string]bool) (keys, loose []found) {
	pos := func(n ast.Node) string {
		p := fset.Position(n.Pos())
		return filepath.ToSlash(p.Filename) + ":" + strconv.Itoa(p.Line)
	}
	var lit func(e ast.Expr) (string, bool)
	lit = func(e ast.Expr) (string, bool) {
		if x, ok := e.(*ast.BinaryExpr); ok && x.Op == token.ADD { // "a " + "b": one text, written on two lines
			a, ok1 := lit(x.X)
			b, ok2 := lit(x.Y)
			return a + b, ok1 && ok2
		}
		b, ok := e.(*ast.BasicLit)
		if !ok || b.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(b.Value)
		return s, err == nil
	}
	// The i18n package's name in this file.
	i18nName := ""
	for _, im := range f.Imports {
		if strings.HasSuffix(strings.Trim(im.Path.Value, `"`), "/internal/i18n") {
			i18nName = "i18n"
			if im.Name != nil {
				i18nName = im.Name.Name
			}
		}
	}
	if f.Name.Name == "i18n" {
		i18nName = "."
	}
	marked := map[ast.Expr]bool{}
	quiet := map[ast.Expr]bool{}
	add := func(e ast.Expr) {
		if s, ok := lit(e); ok && isWords(s) {
			keys = append(keys, found{key: s, pos: pos(e)})
			marked[e] = true
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			name := callName(x.Fun)
			switch {
			case i18nName != "" && (name == i18nName+".N" || (i18nName == "." && name == "N")) && len(x.Args) >= 2:
				one, ok1 := lit(x.Args[0])
				other, ok2 := lit(x.Args[1])
				if ok1 && ok2 {
					keys = append(keys, found{key: other, pos: pos(x.Args[1]), one: one})
					marked[x.Args[0]], marked[x.Args[1]] = true, true
				}
			case i18nName != "" && (name == i18nName+".T" || name == i18nName+".F" || name == i18nName+".Error" || (i18nName == "." && (name == "T" || name == "F"))):
				if len(x.Args) > 0 {
					if s, ok := lit(x.Args[0]); ok {
						keys = append(keys, found{key: s, pos: pos(x.Args[0])})
						ast.Inspect(x.Args[0], func(m ast.Node) bool {
							if e, ok := m.(ast.Expr); ok {
								marked[e] = true
							}
							return true
						})
					}
				}
			case name == "writeErr" && len(x.Args) >= 3:
				add(x.Args[2])
			case name == "http.Error" && len(x.Args) >= 2:
				add(x.Args[1])
			case quietCalls.MatchString(name) || strings.HasSuffix(name, "Printf") || strings.HasSuffix(name, "Println") ||
				strings.HasSuffix(name, ".Info") || strings.HasSuffix(name, ".Warn") || strings.HasSuffix(name, ".Debug") ||
				strings.HasSuffix(name, ".Error") || strings.HasSuffix(name, "Errorf") || strings.HasSuffix(name, "Fatal") ||
				strings.HasSuffix(name, "Fatalf") || strings.HasSuffix(name, ".Log") || strings.HasSuffix(name, "Logf") || name == "panic":
				args := x.Args
				if name == "fmt.Sprintf" && len(args) > 0 {
					args = args[1:] // the text it makes may well be read by a person: its format is checked like any other
				}
				for _, a := range args {
					ast.Inspect(a, func(m ast.Node) bool {
						if e, ok := m.(ast.Expr); ok {
							quiet[e] = true
						}
						return true
					})
				}
			}
		case *ast.KeyValueExpr:
			switch k := x.Key.(type) {
			case *ast.Ident:
				if userFields[k.Name] {
					add(x.Value)
				}
			case *ast.BasicLit:
				if s, ok := lit(k); ok && userKeys[s] {
					add(x.Value)
				}
			}
		case *ast.AssignStmt:
			for i, l := range x.Lhs {
				if sel, ok := l.(*ast.SelectorExpr); ok && userFields[sel.Sel.Name] && i < len(x.Rhs) {
					add(x.Rhs[i])
				}
			}
		}
		return true
	})
	// A sentence kept in a const is a key: a const can't call i18n.T, and a sentence is kept in one to be said.
	ast.Inspect(f, func(d ast.Node) bool {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.CONST {
			ast.Inspect(g, func(n ast.Node) bool {
				if b, ok := n.(*ast.BasicLit); ok {
					if s, ok := lit(b); ok && sentence.MatchString(s) && isWords(s) && !allowed[s] {
						keys = append(keys, found{key: s, pos: pos(b)})
						marked[b] = true
					}
				}
				return true
			})
			return false
		}
		return true
	})
	// Every other sentence-like literal: may reach a person by a path the scanner can't follow.
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ImportSpec, *ast.Field:
			return false
		case *ast.GenDecl:
			if x.Tok == token.IMPORT {
				return false
			}
		case *ast.BasicLit:
			if x.Kind != token.STRING || marked[x] || quiet[x] {
				return true
			}
			s, ok := lit(x)
			if ok && sentence.MatchString(s) && isWords(s) && !strings.Contains(s, "\n") && len(s) < 400 {
				loose = append(loose, found{key: s, pos: pos(x)})
			}
		}
		return true
	})
	return keys, loose
}

func callName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.SelectorExpr:
		return callName(x.X) + "." + x.Sel.Name
	case *ast.CallExpr:
		return callName(x.Fun) + "()"
	case *ast.IndexExpr:
		return callName(x.X)
	}
	return ""
}
