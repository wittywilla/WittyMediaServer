// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Disc folders: a folder that holds VIDEO_TS, BDMV or BDAV is one disc. Copied discs arrive
// with those names in any case, and on a file system that minds case, "video_ts" is not
// "VIDEO_TS". A library can also be one of those folders itself, when someone points it at
// the disc's own structure rather than the folder around it.

// discStructure says what disc dir is the structure of, when dir is a VIDEO_TS, BDMV or BDAV
// folder: "dvd-folder", "bluray-folder" or "". A BDAV folder is a recording, with clips and
// no program: it plays as a Blu-ray with no menus. Only files count as the disc's files, and
// a folder as its folder: a pipe by such a name would hold the scan.
func discStructure(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	has := func(folder bool, names ...string) bool {
		for _, e := range entries {
			linked := e.Type()&fs.ModeSymlink != 0
			if (folder && !e.IsDir()) || (!folder && !plainMode(e.Type()) && !(linked && plainFile(filepath.Join(dir, e.Name())))) {
				continue
			}
			for _, n := range names {
				if strings.EqualFold(e.Name(), n) {
					return true
				}
			}
		}
		return false
	}
	switch strings.ToUpper(filepath.Base(dir)) {
	case "VIDEO_TS":
		if has(false, "VIDEO_TS.IFO") {
			return "dvd-folder"
		}
	case "BDMV":
		if has(false, "index.bdmv", "INDEX.BDM") {
			return "bluray-folder"
		}
	case "BDAV":
		if has(true, "CLIPINF") || has(false, "info.bdav") {
			return "bluray-folder"
		}
	}
	return ""
}

// discStructureName: name is what a disc's own structure folder is called.
func discStructureName(name string) bool {
	switch strings.ToUpper(name) {
	case "VIDEO_TS", "BDMV", "BDAV":
		return true
	}
	return false
}

// discInside says what disc dir holds, in a folder of its own in any case: "dvd-folder",
// "bluray-folder" or "".
func discInside(dir string) string {
	if plainFile(filepath.Join(dir, "VIDEO_TS", "VIDEO_TS.IFO")) {
		return "dvd-folder"
	}
	if plainFile(filepath.Join(dir, "BDMV", "index.bdmv")) {
		return "bluray-folder"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && discStructureName(e.Name()) {
			if k := discStructure(filepath.Join(dir, e.Name())); k != "" {
				return k
			}
		}
	}
	return ""
}

// discAround is the disc whose structure folder is at p: the folder around it, or p itself
// when the library starts there. A camcorder card keeps it under PRIVATE/AVCHD or AVCHD, so
// the card is the folder above those.
func discAround(p, root string) string {
	if p == root {
		return p
	}
	disc := filepath.Dir(p)
	for disc != root {
		switch strings.ToUpper(filepath.Base(disc)) {
		case "AVCHD", "SONY", "PRIVATE":
			disc = filepath.Dir(disc)
			continue
		}
		break
	}
	return disc
}

// discFolderName is what a disc folder is called, for its title and artwork: its own name,
// or the name of the folder it's in when it is the disc's VIDEO_TS, BDMV or BDAV itself.
func discFolderName(path string) string {
	if discStructureName(filepath.Base(path)) {
		if up := filepath.Base(filepath.Dir(path)); up != "." && up != string(filepath.Separator) {
			return up
		}
	}
	return filepath.Base(path)
}

// dropInsideDiscs leaves out what was found inside a disc folder: a walk that comes to a
// disc's structure under a name in another case has already passed the files beside it.
func dropInsideDiscs(found []candidate) []candidate {
	var discs []string
	for _, c := range found {
		if c.kind == "dvd-folder" || c.kind == "bluray-folder" {
			discs = append(discs, c.path+string(filepath.Separator))
		}
	}
	if len(discs) == 0 {
		return found
	}
	out := found[:0]
	for _, c := range found {
		inside := false
		for _, d := range discs {
			if strings.HasPrefix(c.path, d) {
				inside = true
				break
			}
		}
		if !inside {
			out = append(out, c)
		}
	}
	return out
}
