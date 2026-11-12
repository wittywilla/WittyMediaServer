// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command genicon draws every logo, icon and background of the WittyMediaServer family from the
// brand drawings in internal/brandart (docs/BRAND.md): vector first, written as SVG, Android
// vector drawables, Kotlin and Swift drawing code, and rasterised to every PNG, ICO, ICNS, BMP
// and JPEG size each platform asks for. Colours come from the design tokens (themes/).
//
//	go run ./scripts/genicon -target <name> -o <repository> [-concept wink|monogram|signal] [-check]
//	    writes one repository's set: server, server-package, client, player, reader, catalog,
//	    website. -check changes nothing and fails if any file differs.
//	go run ./scripts/genicon -sheet <out.png> [-concept ...]
//	    the comparison sheet of the three concepts (the -concept one marked as chosen)
//	go run ./scripts/genicon out.png [size] | out.ico | -bitmaps <folder>
//	    single files, as the older packaging scripts ask for them
//
// Each repository has scripts/sync-brand.sh, which runs this with its own target.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/brandart"
)

func main() {
	// The older forms: genicon out.png [size], genicon out.ico, genicon -bitmaps <folder>.
	if len(os.Args) >= 2 && !strings.HasPrefix(os.Args[1], "-") {
		legacy(os.Args[1:])
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "-bitmaps" {
		files, err := brandart.NewKit(brandart.DefaultConcept).Build("server-package", nil)
		check(err)
		for _, name := range []string{"banner.bmp", "welcome.bmp"} {
			check(os.WriteFile(filepath.Join(os.Args[2], name), files[name], 0o644))
		}
		return
	}

	concept := flag.String("concept", string(brandart.DefaultConcept), "the design: wink, monogram or signal")
	target := flag.String("target", "", "the set to write: "+strings.Join(brandart.Targets, ", "))
	out := flag.String("o", ".", "the repository (or, for server-package, the folder) to write into")
	checkOnly := flag.Bool("check", false, "change nothing; fail if a file differs from what would be written")
	sheet := flag.String("sheet", "", "write the comparison sheet of the three concepts to this PNG")
	flag.Parse()

	c, err := brandart.ParseConcept(*concept)
	check(err)
	kit := brandart.NewKit(c)

	if *sheet != "" {
		var b bytes.Buffer
		check(png.Encode(&b, brandart.Sheet(brandart.Concepts, c)))
		check(os.WriteFile(*sheet, b.Bytes(), 0o644))
		return
	}
	if *target == "" {
		fmt.Fprintln(os.Stderr, "genicon: say which set with -target (", strings.Join(brandart.Targets, ", "), ")")
		os.Exit(2)
	}

	var modules map[string]string
	if *target == "catalog" {
		modules, err = catalogModules(filepath.Join(*out, "catalog.json"))
		check(err)
	}
	files, err := kit.Build(*target, modules)
	check(err)

	differ := 0
	for _, p := range files.Paths() {
		dst := filepath.Join(*out, filepath.FromSlash(p))
		old, _ := os.ReadFile(dst)
		if bytes.Equal(old, files[p]) {
			continue
		}
		differ++
		if *checkOnly {
			fmt.Fprintln(os.Stderr, "differs:", p)
			continue
		}
		check(os.MkdirAll(filepath.Dir(dst), 0o755))
		check(os.WriteFile(dst, files[p], 0o644))
	}
	if *checkOnly && differ > 0 {
		fmt.Fprintf(os.Stderr, "%d of %d brand files are out of date: run scripts/sync-brand.sh\n", differ, len(files))
		os.Exit(1)
	}
	verb := "written"
	if *checkOnly {
		verb = "up to date"
	}
	fmt.Printf("%s: %d files, %d %s (concept %s)\n", *target, len(files), map[bool]int{true: len(files), false: differ}[*checkOnly], verb, c)
}

// catalogModules reads each module's id and category from WittyModuleCatalog's catalog.json.
func catalogModules(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cat struct {
		Modules []struct {
			ID    string `json:"id"`
			Group string `json:"group"`
		} `json:"modules"`
	}
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, m := range cat.Modules {
		out[m.ID] = m.Group
	}
	return out, nil
}

func legacy(args []string) {
	kit := brandart.NewKit(brandart.DefaultConcept)
	out := args[0]
	if strings.EqualFold(filepath.Ext(out), ".ico") {
		check(os.WriteFile(out, brandart.ICO(brandart.WindowsSizes, func(px int) brandart.Art { return kit.Tile(brandart.Server, px) }), 0o644))
		return
	}
	size := 256
	if len(args) > 1 {
		size, _ = strconv.Atoi(args[1])
	}
	check(os.WriteFile(out, kit.Tile(brandart.Server, size).PNG(size, size), 0o644))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "genicon:", err)
		os.Exit(1)
	}
}
