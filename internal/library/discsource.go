// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"path/filepath"
	"regexp"
	"strings"
)

// A video that is a copy of a disc's film: a loose file from a disc's folder, or a film
// copied from a disc into another container. It has no menus, but players give it the
// disc's own remote (see docs/DVD.md, "The remote").

var (
	// Names that say where a copy came from, as release names and ripping tools write them.
	blurayName = regexp.MustCompile(`(?i)(^|[^a-z0-9])(blu-?ray|bd-?rip|br-?rip|bd-?remux|bd25|bd50|bd66|bd100|bdmv)([^a-z0-9]|$)`)
	dvdName    = regexp.MustCompile(`(?i)(^|[^a-z0-9])(dvd|dvd-?rip|dvd-?remux|dvd5|dvd9|dvd-?r)([^a-z0-9]|$)`)
	// HD DVD isn't a DVD. It has no remote of its own yet.
	hdDVDName = regexp.MustCompile(`(?i)(^|[^a-z0-9])hd[ ._-]?dvd([^a-z0-9]|$)`)
	// MakeMKV names a disc's titles <label>_t00.mkv.
	makeMKVName = regexp.MustCompile(`(?i)_t\d{2}\.mkv$`)
)

// discSourceOf says which kind of disc a video was copied from: "dvd", "bluray", or "" for
// a video that isn't, or isn't known to be, from a disc.
func discSourceOf(path, videoCodec string, height int) string {
	name := filepath.Base(path)
	switch strings.ToLower(filepath.Ext(name)) {
	case ".vob", ".evo", ".vro":
		return "dvd"
	case ".m2ts", ".mts", ".m2t":
		return "bluray"
	}
	for _, n := range []string{strings.TrimSuffix(name, filepath.Ext(name)), filepath.Base(filepath.Dir(path))} {
		switch {
		case hdDVDName.MatchString(n):
			return ""
		case blurayName.MatchString(n):
			return "bluray"
		case dvdName.MatchString(n):
			return "dvd"
		}
	}
	if makeMKVName.MatchString(name) {
		if videoCodec == "mpeg2video" && height > 0 && height <= 576 {
			return "dvd"
		}
		return "bluray"
	}
	return ""
}
