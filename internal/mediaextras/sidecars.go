// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"path/filepath"
	"strings"
)

// The files that go with a title are media files themselves: a stem is a FLAC, a canvas
// is an MP4. A library scan that took each for a title of its own would list "Song.vocals"
// as a track of the album and the album's canvas as a film. These say, by name alone and
// with the same rules FindStems and CanvasCandidates find them by, which title a file
// would belong to. The scan then leaves it out only if that title is really there, so a
// song that merely has a dot in its name ("Mr. Bass.flac") is still a song.

// Owner names a title a file could go with: the folder the title's file is in, and the
// title's file name without its extension, in lower case. Base is "" for a canvas that
// goes with everything in the folder.
type Owner struct {
	Dir  string
	Base string
}

// StemOwners are the songs p would be a stem of, if they exist. For "Song.flac":
//
//	Song.vocals.flac, Song.stem.mp4        beside the song
//	Song.stems/vocals.wav                  in a folder named after it
//	stems/Song/vocals.wav                  in the album's stems folder
//	stems/Song.vocals.wav, stems/Song.stem.mp4
//
// A file can fit more than one of these, so each is given. Nothing is opened.
func StemOwners(p string) []Owner {
	dir, name := filepath.Dir(p), filepath.Base(p)
	lname := strings.ToLower(name)
	var out []Owner
	// The song's name, a dot and the part; or the Native Instruments file.
	base := ""
	if strings.HasSuffix(lname, ".stem.mp4") {
		base = name[:len(name)-len(".stem.mp4")]
	} else if stemExt(name) {
		rest := strings.TrimSuffix(name, filepath.Ext(name))
		if i := strings.LastIndexByte(rest, '.'); i > 0 && StemName(rest[i+1:]) != "" && StemName(rest[i+1:]) != Multitrack {
			base = rest[:i]
		}
	}
	if base != "" {
		out = append(out, Owner{Dir: dir, Base: strings.ToLower(base)})
		if strings.EqualFold(filepath.Base(dir), "stems") {
			out = append(out, Owner{Dir: filepath.Dir(dir), Base: strings.ToLower(base)})
		}
	}
	// A part by its own name, in the song's folder of stems.
	if stemExt(name) {
		if part := StemName(strings.TrimSuffix(name, filepath.Ext(name))); part != "" && part != Multitrack {
			folder := filepath.Base(dir)
			if lf := strings.ToLower(folder); strings.HasSuffix(lf, ".stems") && len(lf) > len(".stems") {
				out = append(out, Owner{Dir: filepath.Dir(dir), Base: lf[:len(lf)-len(".stems")]})
			}
			if up := filepath.Dir(dir); strings.EqualFold(filepath.Base(up), "stems") {
				out = append(out, Owner{Dir: filepath.Dir(up), Base: strings.ToLower(folder)})
			}
		}
	}
	return out
}

// CanvasOwner is the title p would be the canvas of: "Film.canvas.mp4" goes with "Film"
// beside it, and "canvas.mp4" with everything in its folder and the folders inside it (an
// album's songs, a show's episodes), which is said with an empty Base. ok is false for
// any other name, and for a file too big to be taken for a canvas (MaxCanvasSidecar):
// that one is a film someone named "canvas", and stays a film. A canvas kept in a hidden
// ".canvas" folder isn't asked about: scans don't go into hidden folders.
func CanvasOwner(p string, size int64) (Owner, bool) {
	if size > MaxCanvasSidecar {
		return Owner{}, false
	}
	name := filepath.Base(p)
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".mp4" && ext != ".webm" {
		return Owner{}, false
	}
	base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	switch {
	case base == "canvas":
		return Owner{Dir: filepath.Dir(p)}, true
	case strings.HasSuffix(base, ".canvas") && len(base) > len(".canvas"):
		return Owner{Dir: filepath.Dir(p), Base: base[:len(base)-len(".canvas")]}, true
	}
	return Owner{}, false
}
