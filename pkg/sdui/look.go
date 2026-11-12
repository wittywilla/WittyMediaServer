// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package sdui

import "time"

// A look is an experience and a theme, chosen separately (docs/plans/DESIGN-2.md section 8).
// The choice order for each: what an administrator requires, then the person's own, then the
// server's default, then (for the theme) the experience's own default. The server works it
// out and sends the finished answer, with who required what, so every client behaves alike.

// Requirement is a look an administrator requires: an experience, a theme or both. A part
// left empty is not required.
type Requirement struct {
	Experience string `json:"experience,omitempty"`
	Theme      string `json:"theme,omitempty"`
	// By is the administrator who set it, At when.
	By string    `json:"by,omitempty"`
	At time.Time `json:"at,omitzero"`
}

// RequiredPolicy is what is required of whom: of everyone, of every kids profile, and of
// named profiles (by profile ID). The most specific wins, part by part: a profile's own
// requirement over the kids' over everyone's.
type RequiredPolicy struct {
	Everyone *Requirement           `json:"everyone,omitempty"`
	Kids     *Requirement           `json:"kids,omitempty"`
	Profiles map[string]Requirement `json:"profiles,omitempty"`
}

// RequiredPart is one required value as a client is told it: the value, who required it
// and when, and for whom (profile, kids or everyone).
type RequiredPart struct {
	Value string    `json:"value"`
	By    string    `json:"by,omitempty"`
	At    time.Time `json:"at,omitzero"`
	Scope string    `json:"scope"`
}

// Resolve is the required experience and theme of a profile, nil for a part nobody
// requires.
func (p *RequiredPolicy) Resolve(profileID string, kids bool) (experience, theme *RequiredPart) {
	if p == nil {
		return nil, nil
	}
	type layer struct {
		scope string
		r     *Requirement
	}
	var layers []layer
	if r, ok := p.Profiles[profileID]; ok {
		layers = append(layers, layer{"profile", &r})
	}
	if kids && p.Kids != nil {
		layers = append(layers, layer{"kids", p.Kids})
	}
	if p.Everyone != nil {
		layers = append(layers, layer{"everyone", p.Everyone})
	}
	for _, l := range layers {
		if experience == nil && l.r.Experience != "" {
			experience = &RequiredPart{Value: l.r.Experience, By: l.r.By, At: l.r.At, Scope: l.scope}
		}
		if theme == nil && l.r.Theme != "" {
			theme = &RequiredPart{Value: l.r.Theme, By: l.r.By, At: l.r.At, Scope: l.scope}
		}
	}
	return experience, theme
}

// Validate checks a requirement's experience (the theme is the server's to check: it knows
// the installed themes).
func (r *Requirement) validate(v *validator, path string) {
	if r.Experience != "" && ExperienceByID(r.Experience) == nil {
		v.err(path+"/experience", suggest(r.Experience, ExperienceIDs()), "%q is not an experience", r.Experience)
	}
}

// ValidateRequired checks the requirements' experiences.
func ValidateRequired(p *RequiredPolicy) []Issue {
	v := &validator{page: &Page{Kind: "custom"}}
	if p == nil {
		return nil
	}
	if p.Everyone != nil {
		p.Everyone.validate(v, "/everyone")
	}
	if p.Kids != nil {
		p.Kids.validate(v, "/kids")
	}
	for id, r := range p.Profiles {
		r.validate(v, "/profiles/"+id)
	}
	return v.issues
}

// RequiredThemes lists every theme a policy requires, for the server to check against what
// is installed.
func (p *RequiredPolicy) RequiredThemes() []string {
	if p == nil {
		return nil
	}
	var out []string
	add := func(r *Requirement) {
		if r != nil && r.Theme != "" {
			out = append(out, r.Theme)
		}
	}
	add(p.Everyone)
	add(p.Kids)
	for _, r := range p.Profiles {
		r := r
		add(&r)
	}
	return out
}
