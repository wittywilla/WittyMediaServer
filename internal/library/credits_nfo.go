// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// The people in an NFO file, as Kodi, Jellyfin and Emby write them:
//
//	<actor><name/><role/><order/><thumb/><tmdbid/><imdbid/><type>GuestStar</type></actor>
//	<director>A / B</director> <credits>Writer</credits> <writer/> <producer/> <composer/> ...
//
// Kodi writes one <role> per part played, numbers its actors with <order> (Jellyfin with
// <sortorder>), and joins several names in one tag with " / ". Jellyfin writes most of the crew
// as <actor> with a <type> that says what they did. An episode's actors are its guest stars;
// the series regulars among them are told apart when credits are read, from the show's cast.

type nfoPersonID struct {
	Type string `xml:"type,attr"`
	ID   string `xml:",chardata"`
}

type nfoActor struct {
	Name      string        `xml:"name"`
	Roles     []string      `xml:"role"`
	Order     string        `xml:"order"`
	SortOrder string        `xml:"sortorder"`
	Thumb     string        `xml:"thumb"`
	Type      string        `xml:"type"`
	TMDBID    string        `xml:"tmdbid"`
	IMDBID    string        `xml:"imdbid"`
	TVDBID    string        `xml:"tvdbid"`
	UniqueIDs []nfoPersonID `xml:"uniqueid"`
}

// nfoCrewTag is a crew tag: the name as its text (Kodi), or in a <name> inside it.
type nfoCrewTag struct {
	Text   string `xml:",chardata"`
	Name   string `xml:"name"`
	TMDBID string `xml:"tmdbid,attr"`
	IMDBID string `xml:"imdbid,attr"`
}

// nfoPeople is the part of a movie, tvshow or episodedetails NFO that names people.
type nfoPeople struct {
	Actors              []nfoActor   `xml:"actor"`
	Directors           []nfoCrewTag `xml:"director"`
	Credits             []nfoCrewTag `xml:"credits"`
	Writers             []nfoCrewTag `xml:"writer"`
	Producers           []nfoCrewTag `xml:"producer"`
	Composers           []nfoCrewTag `xml:"composer"`
	Cinematographers    []nfoCrewTag `xml:"cinematographer"`
	Editors             []nfoCrewTag `xml:"editor"`
	ProductionDesigners []nfoCrewTag `xml:"productiondesigner"`
	ArtDirectors        []nfoCrewTag `xml:"artdirector"`
	SetDecorators       []nfoCrewTag `xml:"setdecorator"`
	CostumeDesigners    []nfoCrewTag `xml:"costumedesigner"`
}

// maxNFOPeople bounds what one NFO may name, before the credits' own limit.
const maxNFOPeople = 1000

// crewTags are the crew tags in the order they are read, with the job each means.
func (p *nfoPeople) crewTags() []struct {
	tags []nfoCrewTag
	job  string
} {
	return []struct {
		tags []nfoCrewTag
		job  string
	}{
		{p.Directors, "Director"}, {p.Credits, "Writer"}, {p.Writers, "Writer"}, {p.Producers, "Producer"},
		{p.Composers, "Composer"}, {p.Cinematographers, "Director of Photography"}, {p.Editors, "Editor"},
		{p.ProductionDesigners, "Production Design"}, {p.ArtDirectors, "Art Direction"}, {p.SetDecorators, "Set Decoration"},
		{p.CostumeDesigners, "Costume Design"},
	}
}

// jobInParens is a name with what they did after it: "Ana Lima (Screenplay)".
var jobInParens = regexp.MustCompile(`^(.*?)\s*\(([^()]+)\)\s*$`)

// people turns an NFO's people into credits. episode: its actors are guest stars. photos finds
// a picture in an .actors folder, which is used before the NFO's <thumb>.
func (p *nfoPeople) people(episode bool, photos *actorPhotos) []CreditIn {
	var out []CreditIn
	for i, a := range p.Actors {
		if len(out) >= maxNFOPeople {
			break
		}
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		ids := PersonIDs{TMDB: strings.TrimSpace(a.TMDBID), IMDB: strings.TrimSpace(a.IMDBID), TVDB: strings.TrimSpace(a.TVDBID)}
		for _, u := range a.UniqueIDs {
			v := strings.TrimSpace(u.ID)
			switch strings.ToLower(strings.TrimSpace(u.Type)) {
			case "tmdb":
				ids.TMDB = v
			case "imdb":
				ids.IMDB = v
			case "tvdb":
				ids.TVDB = v
			case "tvmaze":
				ids.TVMaze = v
			}
		}
		// The owner's own picture beside the media comes before one on the web.
		photo := photos.find(name)
		if photo == "" {
			photo = firstWebAddress([]string{a.Thumb})
		}
		order := i
		for _, o := range []string{a.Order, a.SortOrder} {
			if n, err := strconv.Atoi(strings.TrimSpace(o)); err == nil && n >= 0 {
				order = n
				break
			}
		}
		role, job := actorRole(a.Type, episode)
		if role == RoleCrew {
			out = append(out, CreditIn{Name: name, IDs: ids, Photo: photo, Role: RoleCrew, Job: job, Order: order})
			continue
		}
		roles := a.Roles
		if len(roles) == 0 {
			roles = []string{""}
		}
		for _, r := range roles {
			out = append(out, CreditIn{Name: name, IDs: ids, Photo: photo, Role: role, Character: strings.TrimSpace(r), Order: order})
		}
	}
	for _, t := range p.crewTags() {
		for _, tag := range t.tags {
			text := strings.TrimSpace(tag.Name)
			if text == "" {
				text = strings.TrimSpace(tag.Text)
			}
			for _, name := range splitNames(text) {
				if len(out) >= maxNFOPeople {
					return out
				}
				job := t.job
				if m := jobInParens.FindStringSubmatch(name); m != nil && strings.TrimSpace(m[1]) != "" {
					name, job = strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
				}
				out = append(out, CreditIn{Name: name, IDs: PersonIDs{TMDB: strings.TrimSpace(tag.TMDBID), IMDB: strings.TrimSpace(tag.IMDBID)},
					Photo: photos.find(name), Role: RoleCrew, Job: job})
			}
		}
	}
	return out
}

// actorRole is what an <actor> with this <type> did: Jellyfin names the crew this way too.
func actorRole(typ string, episode bool) (role, job string) {
	t := strings.ToLower(strings.Join(strings.Fields(typ), ""))
	switch t {
	case "", "actor", "voice", "voiceactor":
		if episode {
			return RoleGuest, ""
		}
		return RoleCast, ""
	case "gueststar", "guest":
		return RoleGuest, ""
	}
	// Director, Writer, Producer, Composer, Conductor, Lyricist, Editor ...
	return RoleCrew, strings.TrimSpace(typ)
}

// actorPhotos finds actors' pictures in an .actors folder, as Kodi keeps them
// (".actors/Ana_Lima.jpg"). The folder is read once, and only plain files in it count.
type actorPhotos struct {
	dirs  []string
	files map[string]string // lower-case name without extension -> whole path
	read  bool
}

// maxActorPhotos bounds how many pictures of one folder are kept in mind.
const maxActorPhotos = 5000

func newActorPhotos(dirs ...string) *actorPhotos { return &actorPhotos{dirs: dirs} }

func (a *actorPhotos) find(name string) string {
	if a == nil || name == "" {
		return ""
	}
	if !a.read {
		a.read = true
		a.files = map[string]string{}
		for _, dir := range a.dirs {
			if dir == "" || !filepath.IsAbs(dir) {
				continue // a picture is a whole path in a library, never a relative one
			}
			folder := filepath.Join(dir, ".actors")
			entries, err := os.ReadDir(folder)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if len(a.files) >= maxActorPhotos {
					break
				}
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if !e.Type().IsRegular() || (ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp") {
					continue
				}
				k := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
				if _, ok := a.files[k]; !ok {
					a.files[k] = filepath.Join(folder, e.Name())
				}
			}
		}
	}
	return a.files[strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), " ", "_"))]
}

// addActorPhotos gives people without a picture one from an .actors folder.
func addActorPhotos(list []CreditIn, photos *actorPhotos) {
	for i := range list {
		if list[i].Photo == "" {
			list[i].Photo = photos.find(list[i].Name)
		}
	}
}
