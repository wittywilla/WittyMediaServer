// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"sort"
	"strings"
)

// The people in an NFO the server writes, so another media server reads the same names: the
// cast as <actor> (name, role, order, a picture's web address and the TMDB number), and the
// crew in the tags Kodi and Jellyfin read (<director>, <credits> for the writers, <producer>,
// and the others). What is written is what a page shows: each part from its best source. When
// the server reads the file again it is the NFO's, which names the same people, so nobody is
// doubled.

type nfoActorOut struct {
	Name   string `xml:"name"`
	Role   string `xml:"role,omitempty"`
	Type   string `xml:"type,omitempty"`
	Order  int    `xml:"order"`
	Thumb  string `xml:"thumb,omitempty"`
	TMDBID string `xml:"tmdbid,omitempty"`
}

// nfoCrewOut are the crew tags, in the order they are written.
type nfoCrewOut struct {
	Directors           []string `xml:"director"`
	Credits             []string `xml:"credits"`
	Producers           []string `xml:"producer"`
	Composers           []string `xml:"composer"`
	Cinematographers    []string `xml:"cinematographer"`
	Editors             []string `xml:"editor"`
	ProductionDesigners []string `xml:"productiondesigner"`
	ArtDirectors        []string `xml:"artdirector"`
	SetDecorators       []string `xml:"setdecorator"`
	CostumeDesigners    []string `xml:"costumedesigner"`
}

// Limits on what one NFO lists.
const (
	maxNFOActors  = 250
	maxNFOCrewTag = 50
)

// storedCredits are a title's credits by source, with what is known of each person.
func (s *Store) storedCredits(ctx context.Context, target string, id int64) (map[string][]CreditIn, error) {
	out := map[string][]CreditIn{}
	if id == 0 {
		return out, nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.source, c.role, c.department, c.job, c.character, c.ord, p.name, p.photo, p.tmdb_id, p.imdb_id, p.tvdb_id, p.tvmaze_id
		FROM credits c JOIN people p ON p.id = c.person_id WHERE c.target = ? AND c.target_id = ? ORDER BY c.rowid`, target, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var src string
		var c CreditIn
		if err := rows.Scan(&src, &c.Role, &c.Department, &c.Job, &c.Character, &c.Order, &c.Name, &c.Photo, &c.IDs.TMDB, &c.IDs.IMDB, &c.IDs.TVDB, &c.IDs.TVMaze); err != nil {
			return nil, err
		}
		out[src] = append(out[src], c)
	}
	return out, rows.Err()
}

// withPending puts what was just read over what is stored, source by source.
func withPending(stored map[string][]CreditIn, pending pendingCredits) map[string][]CreditIn {
	for src, list := range pending {
		clean := make([]CreditIn, 0, len(list))
		for _, raw := range list {
			if c, ok := cleanCredit(raw); ok {
				clean = append(clean, c)
			}
		}
		stored[src] = clean
	}
	return stored
}

// bestParts keeps, for each part (the cast, the guests, each department), the best source's list.
func bestParts(by map[string][]CreditIn) map[string][]CreditIn {
	best := map[string]string{}
	for src, list := range by {
		for _, c := range list {
			part := creditPart(c)
			if b, ok := best[part]; !ok || creditSourceRank[src] < creditSourceRank[b] {
				best[part] = src
			}
		}
	}
	out := map[string][]CreditIn{}
	for part, src := range best {
		for _, c := range by[src] {
			if creditPart(c) == part {
				out[part] = append(out[part], c)
			}
		}
	}
	return out
}

func creditPart(c CreditIn) string {
	if c.Role == RoleCrew {
		return c.Department
	}
	return c.Role
}

// nfoActors lists people as <actor>: one each, their parts joined as Kodi writes them.
func nfoActors(list []CreditIn, typ string, skip map[string]bool, out []nfoActorOut) []nfoActorOut {
	sorted := append([]CreditIn(nil), list...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })
	at := map[string]int{}
	for _, c := range sorted {
		k := PersonKey(c.Name)
		if skip[k] {
			continue
		}
		if i, ok := at[k]; ok {
			if c.Character != "" && !containsFold(strings.Split(out[i].Role, " / "), c.Character) {
				out[i].Role = strings.TrimPrefix(out[i].Role+" / "+c.Character, " / ")
			}
			continue
		}
		if len(out) >= maxNFOActors {
			break
		}
		a := nfoActorOut{Name: c.Name, Role: c.Character, Type: typ, Order: len(out), TMDBID: c.IDs.TMDB}
		if isRemote(c.Photo) {
			a.Thumb = c.Photo
		}
		at[k] = len(out)
		out = append(out, a)
	}
	return out
}

// nfoCrewTagOf is the NFO tag a crew job is written in, "" for one no media server reads.
func nfoCrewTagOf(dept, job string) string {
	j := strings.ToLower(job)
	switch dept {
	case "directing":
		if j == "director" || j == "co-director" || j == "series director" {
			return "director"
		}
	case "writing":
		return "credits"
	case "production":
		if strings.Contains(j, "producer") {
			return "producer"
		}
	case "sound":
		if strings.Contains(j, "composer") || j == "music" {
			return "composer"
		}
	case "camera":
		if j == "director of photography" || strings.HasPrefix(j, "cinematograph") {
			return "cinematographer"
		}
	case "editing":
		if j == "editor" {
			return "editor"
		}
	case "art":
		switch {
		case strings.HasPrefix(j, "production design"):
			return "productiondesigner"
		case strings.HasPrefix(j, "art direct"):
			return "artdirector"
		case strings.HasPrefix(j, "set decorat"):
			return "setdecorator"
		}
	case "costume":
		if strings.HasPrefix(j, "costume design") {
			return "costumedesigner"
		}
	}
	return ""
}

// nfoCrew puts each department's people in the tags their jobs are written in.
func nfoCrew(parts map[string][]CreditIn) nfoCrewOut {
	var out nfoCrewOut
	tags := map[string]*[]string{"director": &out.Directors, "credits": &out.Credits, "producer": &out.Producers, "composer": &out.Composers,
		"cinematographer": &out.Cinematographers, "editor": &out.Editors, "productiondesigner": &out.ProductionDesigners,
		"artdirector": &out.ArtDirectors, "setdecorator": &out.SetDecorators, "costumedesigner": &out.CostumeDesigners}
	for _, d := range Departments {
		list := append([]CreditIn(nil), parts[d.Key]...)
		sort.SliceStable(list, func(i, j int) bool { return rankJob(list[i].Job) < rankJob(list[j].Job) })
		for _, c := range list {
			tag := nfoCrewTagOf(d.Key, c.Job)
			if tag == "" {
				continue
			}
			if l := tags[tag]; len(*l) < maxNFOCrewTag && !containsFold(*l, c.Name) {
				*l = append(*l, c.Name)
			}
		}
	}
	return out
}

// nfoPeopleOfItem is who a film's or an episode's NFO names. An episode's lists its show's
// regulars and its own guest stars, as media centres write them, and only its own crew.
func (s *Store) nfoPeopleOfItem(ctx context.Context, it *Item) ([]nfoActorOut, nfoCrewOut, error) {
	stored, err := s.storedCredits(ctx, TargetItem, it.ID)
	if err != nil {
		return nil, nfoCrewOut{}, err
	}
	own := bestParts(withPending(stored, it.credits))
	if it.ShowID == nil {
		return nfoActors(own[RoleCast], "", nil, nil), nfoCrew(own), nil
	}
	showStored, err := s.storedCredits(ctx, TargetShow, *it.ShowID)
	if err != nil {
		return nil, nfoCrewOut{}, err
	}
	show := bestParts(showStored)
	regulars := show[RoleCast]
	if len(own[RoleCast]) > 0 {
		regulars = own[RoleCast]
	}
	actors := nfoActors(regulars, "Actor", nil, nil)
	in := map[string]bool{}
	for _, a := range actors {
		in[PersonKey(a.Name)] = true
	}
	actors = nfoActors(own[RoleGuest], "GuestStar", in, actors)
	return actors, nfoCrew(own), nil
}

// nfoPeopleOfShow is who a tvshow.nfo names.
func (s *Store) nfoPeopleOfShow(ctx context.Context, sh *Show) ([]nfoActorOut, nfoCrewOut, error) {
	stored, err := s.storedCredits(ctx, TargetShow, sh.ID)
	if err != nil {
		return nil, nfoCrewOut{}, err
	}
	parts := bestParts(withPending(stored, sh.credits))
	return nfoActors(parts[RoleCast], "", nil, nil), nfoCrew(parts), nil
}
