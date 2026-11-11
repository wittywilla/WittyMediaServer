// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
)

// dropTitleExtras takes out of what a scan found the files that go with a title and
// aren't titles themselves: a song's stems ("Song.vocals.flac", "Song.stems/drums.wav",
// "stems/Song/bass.wav", "Song.stem.mp4") and a canvas ("Film.canvas.mp4", or "canvas.mp4"
// in an album's or a show's folder). They are sound and video files, so a walk finds them
// like any other. internal/mediaextras says by name which title each would go with; it is
// left out only when that title was found too, so a real song with a dot in its name
// stays. What a scan no longer finds is removed from the library by the scan as usual, so
// ones listed before this rule existed go at the next scan.
func dropTitleExtras(found []candidate) []candidate {
	type key struct{ dir, base string }
	titles := map[key]bool{} // every sound or video file found, by folder and name
	songs := map[key]bool{}  // the songs among them
	canvases := map[string]mediaextras.Owner{}
	var holds map[string]bool // folders with a title in them or beneath them, made when first needed
	for _, c := range found {
		if c.kind != "audio" && c.kind != "video" {
			continue
		}
		k := key{filepath.Dir(c.path), strings.ToLower(strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path)))}
		if o, ok := mediaextras.CanvasOwner(c.path, c.size); ok {
			canvases[c.path] = o
			continue // a canvas is nobody's title, so nothing is its canvas or its stem
		}
		titles[k] = true
		if c.kind == "audio" {
			songs[k] = true
		}
	}
	if len(titles) == 0 {
		return found
	}
	holding := func(dir string) bool {
		if holds == nil {
			holds = map[string]bool{}
			for _, c := range found {
				if _, isCanvas := canvases[c.path]; isCanvas {
					continue
				}
				for d := filepath.Dir(c.path); !holds[d]; d = filepath.Dir(d) {
					holds[d] = true
					if filepath.Dir(d) == d {
						break
					}
				}
			}
		}
		return holds[dir]
	}
	out := found[:0]
	for _, c := range found {
		if o, ok := canvases[c.path]; ok {
			if (o.Base == "" && holding(o.Dir)) || (o.Base != "" && titles[key{o.Dir, o.Base}]) {
				continue
			}
			out = append(out, c)
			continue
		}
		drop := false
		if c.kind == "audio" || c.kind == "video" {
			self := key{filepath.Dir(c.path), strings.ToLower(strings.TrimSuffix(filepath.Base(c.path), filepath.Ext(c.path)))}
			for _, o := range mediaextras.StemOwners(c.path) {
				if k := (key{o.Dir, o.Base}); k != self && songs[k] {
					drop = true
					break
				}
			}
		}
		if !drop {
			out = append(out, c)
		}
	}
	return out
}
