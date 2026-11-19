// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package wittyinfo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MaxPicturesPerRole is how many pictures one role may hold: a film's posters from several
// releases, an album's covers from its pressings, a show's backdrops.
const MaxPicturesPerRole = 32

// Pictures are the pictures for one role, the one to show first. In the manifest a role with
// one picture is written as its name, and with more as a list:
//
//	"artwork": {"poster": "images/poster.jpg", "backdrop": ["images/backdrop.jpg", "images/backdrop-2.jpg"]}
type Pictures []string

// UnmarshalJSON takes a name or a list of names.
func (p *Pictures) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var one string
		if err := json.Unmarshal(b, &one); err != nil {
			return err
		}
		*p = Pictures{one}
		return nil
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("a picture is a name in the archive, or a list of them")
	}
	*p = list
	return nil
}

// MarshalJSON writes one picture as its name, so files with one picture a role read as
// they always have.
func (p Pictures) MarshalJSON() ([]byte, error) {
	if len(p) == 1 {
		return json.Marshal(p[0])
	}
	return json.Marshal([]string(p))
}

// Artwork names the pictures in the archive for each role.
type Artwork map[string]Pictures

// First is the picture shown for a role, "" when it has none.
func (a Artwork) First(role string) string {
	if p := a[role]; len(p) > 0 {
		return p[0]
	}
	return ""
}

func validRole(role string) error {
	for _, r := range ArtworkRoles {
		if r == role {
			return nil
		}
	}
	return fmt.Errorf("%q is not an artwork role: use %s", role, strings.Join(ArtworkRoles, ", "))
}

// SetImage puts a picture in the archive as the one shown for a role (poster, backdrop...),
// in place of the one shown before; other pictures for the role stay. ext is ".jpg", ".png"
// or ".webp".
func (x *Info) SetImage(role, ext string, b []byte) (string, error) {
	if err := validRole(role); err != nil {
		return "", err
	}
	name := x.freeImageName(role, strings.ToLower(ext), true)
	if err := checkImage(name, b); err != nil {
		return "", err
	}
	if x.Artwork == nil {
		x.Artwork = Artwork{}
	}
	p := x.Artwork[role]
	if len(p) > 0 {
		delete(x.Files, p[0])
		p = append(Pictures{name}, p[1:]...)
	} else {
		p = Pictures{name}
	}
	x.putFile(name, b)
	x.Artwork[role] = p
	return name, nil
}

// AddImage adds a picture for a role after the ones it has.
func (x *Info) AddImage(role, ext string, b []byte) (string, error) {
	if err := validRole(role); err != nil {
		return "", err
	}
	if len(x.Artwork[role]) >= MaxPicturesPerRole {
		return "", fmt.Errorf("%s already has %d pictures, the most a file holds", role, MaxPicturesPerRole)
	}
	name := x.freeImageName(role, strings.ToLower(ext), false)
	if err := checkImage(name, b); err != nil {
		return "", err
	}
	if x.Artwork == nil {
		x.Artwork = Artwork{}
	}
	x.putFile(name, b)
	x.Artwork[role] = append(x.Artwork[role], name)
	return name, nil
}

// RemoveImage takes one picture out of a role, or every one when name is "".
func (x *Info) RemoveImage(role, name string) bool {
	p := x.Artwork[role]
	keep := p[:0:0]
	gone := false
	for _, n := range p {
		if name == "" || n == name {
			delete(x.Files, n)
			gone = true
			continue
		}
		keep = append(keep, n)
	}
	if len(keep) == 0 {
		delete(x.Artwork, role)
	} else {
		x.Artwork[role] = keep
	}
	return gone
}

// ShowFirst makes one of a role's pictures the one shown.
func (x *Info) ShowFirst(role, name string) bool {
	p := x.Artwork[role]
	for i, n := range p {
		if n == name {
			out := append(Pictures{n}, p[:i]...)
			x.Artwork[role] = append(out, p[i+1:]...)
			return true
		}
	}
	return false
}

// ImageChange is what a request does to a role's pictures: put a picture as the one shown
// (Add false) or after the others (Add true), show a named one first, or take one or all out.
type ImageChange struct {
	Picture []byte // with Ext; empty when only reordering or removing
	Ext     string
	Add     bool
	First   string // a picture to show first
	Remove  bool
	Name    string // with Remove: the one to take out, "" for every one
}

// ChangeImages applies an ImageChange to a role.
func (x *Info) ChangeImages(role string, c ImageChange) error {
	if err := validRole(role); err != nil {
		return err
	}
	switch {
	case c.Remove:
		x.RemoveImage(role, c.Name)
	case len(c.Picture) > 0 && c.Add:
		_, err := x.AddImage(role, c.Ext, c.Picture)
		return err
	case len(c.Picture) > 0:
		_, err := x.SetImage(role, c.Ext, c.Picture)
		return err
	case c.First != "":
		if !x.ShowFirst(role, c.First) {
			return fmt.Errorf("%s has no picture %q", role, c.First)
		}
	default:
		return fmt.Errorf("send a picture, or say which one to show first")
	}
	return nil
}

func (x *Info) putFile(name string, b []byte) {
	if x.Files == nil {
		x.Files = map[string][]byte{}
	}
	x.Files[name] = b
}

// freeImageName is images/<role><ext> for the first picture, else images/<role>-<n><ext> with
// the lowest n not in use.
func (x *Info) freeImageName(role, ext string, first bool) string {
	inUse := func(n string) bool {
		if _, ok := x.Files[n]; ok {
			return true
		}
		for _, p := range x.Artwork {
			for _, q := range p {
				if q == n {
					return true
				}
			}
		}
		return false
	}
	base := "images/" + role + ext
	if first && (x.Artwork.First(role) == base || !inUse(base)) {
		return base
	}
	for n := 2; ; n++ {
		if c := "images/" + role + "-" + strconv.Itoa(n) + ext; !inUse(c) {
			return c
		}
	}
}
