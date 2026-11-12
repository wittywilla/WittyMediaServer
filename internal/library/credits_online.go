// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"net/url"
	"strings"
)

// The people each online service names, in the shapes it sends them.

// --- TMDB -------------------------------------------------------------------

// tmdbPersonCredit is one person in a TMDB credits list: a film's or an episode's cast, crew or
// guest stars, or a show's aggregate credits, where one person has several roles or jobs.
type tmdbPersonCredit struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Character   string `json:"character"`
	Order       int    `json:"order"`
	ProfilePath string `json:"profile_path"`
	Department  string `json:"department"`
	Job         string `json:"job"`
	Roles       []struct {
		Character string `json:"character"`
	} `json:"roles"`
	Jobs []struct {
		Job string `json:"job"`
	} `json:"jobs"`
}

// tmdbCredits is TMDB's credits of a film, a show (credits or aggregate_credits) or, with
// guest_stars, an episode.
type tmdbCredits struct {
	Cast       []tmdbPersonCredit `json:"cast"`
	Crew       []tmdbPersonCredit `json:"crew"`
	GuestStars []tmdbPersonCredit `json:"guest_stars"`
}

// tmdbProfile is the address of a person's picture at TMDB, at the size a row of faces needs.
func tmdbProfile(p string) string {
	if p == "" || !strings.HasPrefix(p, "/") {
		return ""
	}
	return tmdbImage + "w185" + p
}

// in turns one TMDB person into credits: one for each part played or job done.
func (p tmdbPersonCredit) in(role string) []CreditIn {
	base := CreditIn{Name: p.Name, IDs: PersonIDs{TMDB: numberID(p.ID)}, Photo: tmdbProfile(p.ProfilePath), Role: role, Order: p.Order}
	var out []CreditIn
	switch role {
	case RoleCrew:
		base.Department = p.Department
		jobs := []string{p.Job}
		if len(p.Jobs) > 0 {
			jobs = jobs[:0]
			for _, j := range p.Jobs {
				jobs = append(jobs, j.Job)
			}
		}
		for _, j := range jobs {
			c := base
			c.Job = j
			out = append(out, c)
		}
	default:
		chars := []string{p.Character}
		if len(p.Roles) > 0 {
			chars = chars[:0]
			for _, r := range p.Roles {
				chars = append(chars, r.Character)
			}
		}
		for _, ch := range chars {
			c := base
			c.Character = ch
			out = append(out, c)
		}
	}
	return out
}

// list is everyone in TMDB's credits: the cast in castRole, then the guest stars and the crew.
func (c *tmdbCredits) list(castRole string) []CreditIn {
	out := []CreditIn{}
	for _, p := range c.Cast {
		out = append(out, p.in(castRole)...)
	}
	for _, p := range c.GuestStars {
		out = append(out, p.in(RoleGuest)...)
	}
	for _, p := range c.Crew {
		out = append(out, p.in(RoleCrew)...)
	}
	return out
}

// directorNames are the film's directors, for the Director field.
func (c *tmdbCredits) directorNames() string {
	var names []string
	for _, p := range c.Crew {
		if p.Job == "Director" {
			names = append(names, p.Name)
		}
	}
	return strings.Join(names, ", ")
}

// tmdbCreator is one of a show's creators, as TMDB lists them apart from its crew.
type tmdbCreator struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	ProfilePath string `json:"profile_path"`
}

// tmdbShowCredits are a show's people: its regular cast over every season (aggregate_credits,
// which also counts the regulars of seasons past), the crew it names as the show's own
// (credits), and its creators.
func tmdbShowCredits(aggregate, current tmdbCredits, creators []tmdbCreator) []CreditIn {
	cast := aggregate.Cast
	if len(cast) == 0 {
		cast = current.Cast
	}
	out := (&tmdbCredits{Cast: cast, Crew: current.Crew}).list(RoleCast)
	for _, c := range creators {
		out = append(out, CreditIn{Name: c.Name, IDs: PersonIDs{TMDB: numberID(c.ID)}, Photo: tmdbProfile(c.ProfilePath), Role: RoleCrew, Department: "Writing", Job: "Creator"})
	}
	return out
}

// MovieCredits reads a film's cast and crew alone, for a film matched before credits existed.
func (t *TMDB) MovieCredits(ctx context.Context, id string) ([]CreditIn, error) {
	var c tmdbCredits
	if err := t.get(ctx, "/movie/"+url.PathEscape(id)+"/credits", url.Values{}, &c); err != nil {
		return nil, err
	}
	return c.list(RoleCast), nil
}

// --- TheTVDB ----------------------------------------------------------------

// tvdbCharacter is one of a series' "characters": TheTVDB's word for every person in it, actor
// or not, each with the part played (name) or the job (peopleType).
type tvdbCharacter struct {
	Name         string     `json:"name"`
	PeopleID     flexString `json:"peopleId"`
	EpisodeID    flexString `json:"episodeId"`
	Type         int        `json:"type"`
	PeopleType   string     `json:"peopleType"`
	PersonName   string     `json:"personName"`
	Image        string     `json:"image"`
	PersonImgURL string     `json:"personImgURL"`
	Sort         int        `json:"sort"`
}

// tvdbPeopleTypes are TheTVDB's people type numbers, for a record without the name.
var tvdbPeopleTypes = map[int]string{1: "Director", 2: "Writer", 3: "Actor", 4: "Guest Star", 5: "Crew", 6: "Creator", 7: "Producer",
	8: "Showrunner", 9: "Musical Guest", 10: "Host", 11: "Executive Producer"}

// tvdbPeople sorts a series' characters into the show's people and, by episode id, the guest
// stars and crew of its episodes. episodes is never nil: a series read names who is in which
// episode, even when that's nobody.
func tvdbPeople(chars []tvdbCharacter) (show []CreditIn, episodes map[string][]CreditIn) {
	show, episodes = []CreditIn{}, map[string][]CreditIn{}
	var orders []int
	for i, c := range chars {
		if i == maxCreditsPerTitle*2 {
			break
		}
		typ := strings.TrimSpace(c.PeopleType)
		if typ == "" {
			typ = tvdbPeopleTypes[c.Type]
		}
		ep := strings.TrimSpace(string(c.EpisodeID))
		if ep == "0" {
			ep = ""
		}
		photo := webURL(c.PersonImgURL)
		in := CreditIn{Name: c.PersonName, IDs: PersonIDs{TVDB: strings.TrimSpace(string(c.PeopleID))}, Photo: photo, Order: c.Sort}
		switch strings.ToLower(strings.ReplaceAll(typ, " ", "")) {
		case "actor":
			in.Role, in.Character = RoleCast, c.Name
		case "host":
			in.Role, in.Character = RoleCast, c.Name
			if in.Character == "" {
				in.Character = "Host"
			}
		case "gueststar", "musicalguest":
			in.Role, in.Character = RoleGuest, c.Name
		case "":
			continue
		default:
			in.Role, in.Job = RoleCrew, typ
		}
		switch {
		case ep != "":
			if in.Role == RoleCast {
				in.Role = RoleGuest // an actor named for one episode is in that episode
			}
			episodes[ep] = append(episodes[ep], in)
		case in.Role == RoleGuest:
			// A guest star of no episode in particular has nowhere to be shown.
		default:
			if in.Role == RoleCast {
				orders = append(orders, c.Sort)
			}
			show = append(show, in)
		}
	}
	// The cast in TheTVDB's order: its sort numbers, or the order it lists them in.
	var cast []CreditIn
	for _, c := range show {
		if c.Role == RoleCast {
			cast = append(cast, c)
		}
	}
	castOrder(cast, orders)
	n := 0
	for i := range show {
		if show[i].Role == RoleCast {
			show[i].Order = cast[n].Order
			n++
		}
	}
	return show, episodes
}

// --- TVmaze -----------------------------------------------------------------

type mazePerson struct {
	ID    int        `json:"id"`
	Name  string     `json:"name"`
	Image *mazeImage `json:"image"`
}

// mazeEmbedded is what /shows/:id?embed[]=cast&embed[]=crew adds to a show.
type mazeEmbedded struct {
	Cast []struct {
		Person    mazePerson `json:"person"`
		Character struct {
			Name string `json:"name"`
		} `json:"character"`
	} `json:"cast"`
	Crew []struct {
		Type   string     `json:"type"`
		Person mazePerson `json:"person"`
	} `json:"crew"`
}

// mazeShowQuery asks TVmaze for a show with its cast and crew in the same answer.
var mazeShowQuery = "?" + url.Values{"embed[]": {"cast", "crew"}}.Encode()

func (p mazePerson) credit() CreditIn {
	c := CreditIn{Name: p.Name, IDs: PersonIDs{TVMaze: numberID(p.ID)}}
	if p.Image != nil {
		c.Photo = webURL(p.Image.Medium)
		if c.Photo == "" {
			c.Photo = webURL(p.Image.Original)
		}
	}
	return c
}

// people is a TVmaze show's cast, in TVmaze's order, and its crew by what TVmaze calls each job
// ("Creator", "Executive Producer", "Director of Photography").
func (e *mazeEmbedded) people() []CreditIn {
	out := []CreditIn{}
	for i, c := range e.Cast {
		in := c.Person.credit()
		in.Role, in.Character, in.Order = RoleCast, c.Character.Name, i
		out = append(out, in)
	}
	for _, c := range e.Crew {
		in := c.Person.credit()
		in.Role, in.Job = RoleCrew, c.Type
		out = append(out, in)
	}
	return out
}

// ShowCredits reads a TVmaze show's cast and crew alone, for a show matched before credits
// existed.
func (t *TVMaze) ShowCredits(ctx context.Context, id string) ([]CreditIn, error) {
	var sh struct {
		Embedded mazeEmbedded `json:"_embedded"`
	}
	if err := t.get(ctx, "/shows/"+url.PathEscape(id)+mazeShowQuery, &sh); err != nil {
		return nil, err
	}
	return sh.Embedded.people(), nil
}

// tvdbSeriesPeople reads a TheTVDB series' people alone, for a show matched before credits
// existed.
func (t *TVDB) seriesPeople(ctx context.Context, id string) ([]CreditIn, map[string][]CreditIn, error) {
	var out struct {
		Data struct {
			Characters []tvdbCharacter `json:"characters"`
		} `json:"data"`
	}
	if err := t.get(ctx, "/series/"+url.PathEscape(id)+"/extended", url.Values{"short": {"false"}}, &out); err != nil {
		return nil, nil, err
	}
	show, eps := tvdbPeople(out.Data.Characters)
	return show, eps, nil
}
