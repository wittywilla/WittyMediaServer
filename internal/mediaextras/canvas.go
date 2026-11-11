// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// A canvas is a short, silent, looping video shown behind a song or a title while it
// plays. It's found beside the media, or uploaded by an administrator.

// MaxCanvas is the largest canvas taken as an upload.
const MaxCanvas = 30 << 20

// CanvasExts are the kinds of file a canvas can be, most wanted first.
var CanvasExts = []string{".mp4", ".webm"}

// CanvasMIME is the type a canvas is sent with, by its name only.
func CanvasMIME(p string) string {
	if strings.EqualFold(filepath.Ext(p), ".webm") {
		return "video/webm"
	}
	return "video/mp4"
}

// VideoExt says from its first bytes whether a file is an MP4 (".mp4") or a WebM
// (".webm") video, whatever its name says, or "" for anything else.
func VideoExt(head []byte) string {
	switch {
	case isMP4(head):
		return ".mp4"
	case isWebM(head):
		return ".webm"
	}
	return ""
}

// isMP4: an ISO media file starts with a box whose type is "ftyp".
func isMP4(b []byte) bool {
	return len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp"))
}

// isWebM: an EBML header that names the "webm" document type.
func isWebM(b []byte) bool {
	return len(b) >= 4 && bytes.Equal(b[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) && bytes.Contains(b, []byte("webm"))
}

// CanvasCandidates are the files beside the media that can be its canvas, in the order
// they're looked for: the track's or the film's own, then the folder's (an album's), then
// the show's. mediaPath is the item's file, or its folder for a disc folder (isDir).
// showDir is "" for anything that isn't an episode. Nothing is opened here.
//
//	Song.canvas.mp4      Song.canvas.webm      the file's own
//	.canvas/Song.mp4     .canvas/Song.webm     the same, in a hidden folder
//	canvas.mp4           canvas.webm           the folder's (an album, a film's folder)
//	.canvas/canvas.mp4   .canvas/canvas.webm
//	<show>/canvas.mp4 ...                       a show's, for each of its episodes
func CanvasCandidates(mediaPath string, isDir bool, showDir string) []string {
	var out []string
	dir, base := filepath.Dir(mediaPath), strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	if isDir {
		dir, base = mediaPath, ""
	}
	add := func(d, name string) {
		for _, ext := range CanvasExts {
			out = append(out, filepath.Join(d, name+ext))
		}
	}
	if base != "" {
		add(dir, base+".canvas")
		add(filepath.Join(dir, ".canvas"), base)
	}
	add(dir, "canvas")
	add(filepath.Join(dir, ".canvas"), "canvas")
	if showDir != "" && filepath.Clean(showDir) != filepath.Clean(dir) {
		add(showDir, "canvas")
		add(filepath.Join(showDir, ".canvas"), "canvas")
	}
	return out
}

// maxMetaEntries bounds how much of the metadata folder is read looking for one file.
const maxMetaEntries = 20000

// FindInMetaFolder looks in the editable metadata folder (internal/metafolder) for a file
// someone put there by hand for an item: "<anything> [item-<id>]-<suffix><ext>", in a
// library's folder or a show's folder inside it. Only ordinary files count.
func FindInMetaFolder(root string, itemID int64, suffix string, exts []string) string {
	tag := "[item-" + strconv.FormatInt(itemID, 10) + "]-" + suffix
	seen := 0
	var look func(dir string, depth int) string
	look = func(dir string, depth int) string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return ""
		}
		for _, e := range entries {
			if seen++; seen > maxMetaEntries {
				return ""
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if e.IsDir() {
				if depth < 2 {
					if p := look(filepath.Join(dir, name), depth+1); p != "" {
						return p
					}
				}
				continue
			}
			for _, ext := range exts {
				if strings.HasSuffix(strings.ToLower(name), strings.ToLower(tag+ext)) {
					if p := filepath.Join(dir, name); plainExists(p) {
						return p
					}
				}
			}
		}
		return ""
	}
	return look(root, 0)
}
