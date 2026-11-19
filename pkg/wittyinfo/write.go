// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// New starts a file of the given scope.
func New(scope string) *Info {
	return &Info{Format: Format, Version: Version, Scope: scope, Files: map[string][]byte{}}
}

// Tidy puts a file in the order it is written: segments, scenes and filters by start, each
// with an id, the scope and format filled in, and members nothing names anymore dropped
// (pictures and subtitles only: other members are kept, unused).
func (x *Info) Tidy() {
	if x.Format != FormatAudio {
		x.Format = Format
	}
	if x.Version < Version {
		x.Version = Version
	}
	x.Scope = x.ScopeOr()
	sort.SliceStable(x.Segments, func(i, j int) bool { return x.Segments[i].Start < x.Segments[j].Start })
	sort.SliceStable(x.Scenes, func(i, j int) bool { return x.Scenes[i].Start < x.Scenes[j].Start })
	sort.SliceStable(x.Filters, func(i, j int) bool { return x.Filters[i].Start < x.Filters[j].Start })
	for i := range x.Segments {
		if x.Segments[i].ID == "" {
			x.Segments[i].ID = newID("seg")
		}
	}
	for i := range x.Scenes {
		if x.Scenes[i].ID == "" {
			x.Scenes[i].ID = newID("scene")
		}
	}
	for i := range x.Filters {
		if x.Filters[i].ID == "" {
			x.Filters[i].ID = newID("filter")
		}
	}
	used := map[string]bool{}
	for _, p := range x.Artwork {
		for _, v := range p {
			used[v] = true
		}
	}
	for _, s := range x.Scenes {
		used[s.Image] = true
	}
	for _, s := range x.Subtitles {
		used[s.File] = true
	}
	for name := range x.Files {
		if k := memberKind(name); (k == "image" || k == "subtitle") && !used[name] {
			delete(x.Files, name)
		}
	}
	if len(x.Artwork) == 0 {
		x.Artwork = nil
	}
	x.tidyLanguages()
}

func newID(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}

// Stamp records who saved the file and with what, now.
func (x *Info) Stamp(author, tool, toolVersion string, now time.Time) {
	if x.Authorship == nil {
		x.Authorship = &Authorship{}
	}
	a := x.Authorship
	ts := now.UTC().Format(time.RFC3339)
	if a.Created == "" {
		a.Created = ts
	}
	a.Updated = ts
	if author != "" {
		a.Author = author
	}
	if tool != "" {
		if a.Tool == nil {
			a.Tool = &Tool{}
		}
		a.Tool.Name, a.Tool.Version = tool, toolVersion
	}
}

// Manifest is the manifest alone, indented, as it is written into the archive.
func (x *Info) Manifest() ([]byte, error) {
	b, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Encode checks the file and writes it as an archive: manifest.json first (stored, so
// that it can be read without unpacking), then the other members in name order.
func (x *Info) Encode() ([]byte, error) {
	x.Tidy()
	if err := x.Check(); err != nil {
		return nil, err
	}
	man, err := x.Manifest()
	if err != nil {
		return nil, err
	}
	if len(man) > MaxManifest {
		return nil, refuse(ManifestName, "the manifest is larger than %d KB", MaxManifest>>10)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // the same bytes for the same content
	if a := x.Authorship; a != nil {
		if t, err := time.Parse(time.RFC3339, a.Updated); err == nil {
			stamp = t
		}
	}
	add := func(name string, b []byte, method uint16) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method, Modified: stamp})
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
	if err := add(ManifestName, man, zip.Store); err != nil {
		return nil, err
	}
	names := sortedKeys(x.Files)
	for _, name := range names {
		if strings.EqualFold(name, ManifestName) || !ValidMemberName(name) {
			continue
		}
		method := zip.Deflate
		if memberKind(name) == "image" {
			method = zip.Store // pictures are packed already
		}
		if err := add(name, x.Files[name], method); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if buf.Len() > MaxFile {
		return nil, refuse("", "the file would be larger than %d MB", MaxFile>>20)
	}
	return buf.Bytes(), nil
}

// Clone copies a file, members and all, so that changing one leaves the other alone.
func (x *Info) Clone() *Info {
	b, _ := json.Marshal(x)
	var out Info
	_ = json.Unmarshal(b, &out)
	out.Files = make(map[string][]byte, len(x.Files))
	for k, v := range x.Files {
		out.Files[k] = v
	}
	return &out
}
