// Package licenses says what this server is made of and under which licenses, for
// Settings, About. Libraries come from libraries.json.gz, which internal/licenses/gen
// writes from what's really compiled in, with each license's full text. The rest is
// written here, because nothing can discover it.
package licenses

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"sort"
	"sync"

	"github.com/wittywilla/wittyplay"
)

// The groups, in the order they're shown.
const (
	GroupServer   = "server"
	GroupWeb      = "web"
	GroupPrograms = "programs"
	GroupCores    = "cores"
	GroupData     = "data"
	GroupServices = "services"
)

// A Group is a heading on the list.
type Group struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	About string `json:"about"`
}

// Groups says what each group means, in plain words.
var Groups = []Group{
	{GroupServer, "Built into the server", "Libraries compiled into WittyMediaServer and its helper programs."},
	{GroupWeb, "Built into this web app", "Libraries your browser runs as part of this interface."},
	{GroupPrograms, "Separate programs", "Programs the server starts and talks to, but which aren't part of it, so their licenses don't reach it."},
	{GroupCores, "Game cores", "Never bundled. Each is a separate file that an admin chooses to install, and a separate program runs it. This is who wrote them and what they're under; Settings, Gaming shows which are installed."},
	{GroupData, "Bundled data", "Files that come with the server."},
	{GroupServices, "Online services", "Where information comes from, and the terms it comes with. Each is only asked if it's turned on, or has a key."},
}

// A Library is one generated entry. Text is an index into File.Texts, as many share one.
type Library struct {
	Group   string `json:"g"`
	Name    string `json:"n"`
	URL     string `json:"u"`
	License string `json:"l"`
	Text    int    `json:"t"`
}

// File is the generated data.
type File struct {
	Libraries []Library `json:"libraries"`
	Texts     []string  `json:"texts"`
}

// A Component is one line on the list.
type Component struct {
	ID      int    `json:"id"`
	Group   string `json:"group"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	License string `json:"license"`
	URL     string `json:"url"`
	// Note says anything more a reader needs, such as an attribution a service asks for.
	Note string `json:"note,omitempty"`
	// HasText: the license's full text is available from this server.
	HasText bool `json:"hasText"`
}

//go:embed libraries.json.gz
var librariesGz []byte

// written is everything that isn't discovered. A link, where the text is long and the
// program isn't part of this one.
var written = []Component{
	{Group: GroupPrograms, Name: "FFmpeg", License: "GPL-2.0-or-later", URL: "https://ffmpeg.org/legal.html",
		Note: "Bundled with release downloads as bin/ffmpeg and bin/ffprobe, to convert video. Those builds include libx264 (GPL-2.0-or-later), LAME (LGPL-2.1) and Opus (BSD-3-Clause). Source: ffmpeg.org. Set WMS_FFMPEG to use a different build."},
	{Group: GroupPrograms, Name: "RetroArch", License: "GPL-3.0", URL: "https://github.com/libretro/RetroArch",
		Note: "Not bundled or run. If it's installed, its folder of game cores is read."},
	{Group: GroupPrograms, Name: "Moonlight", License: "GPL-3.0", URL: "https://github.com/moonlight-stream",
		Note: "WittyCouch Host, a separate program, is built on its code; its source is offered beside each release. Moonlight's own apps aren't bundled, and are only linked."},
	{Group: GroupPrograms, Name: "Sunshine", License: "GPL-3.0", URL: "https://github.com/LizardByte/Sunshine",
		Note: "Not bundled. WittyRigs uses a copy that someone installs themselves, and WittyCouch connects to it on a PC."},
	{Group: GroupData, Name: "Natural Earth 1:110m land, via world-atlas", License: "Public domain (world-atlas: ISC)", URL: "https://www.naturalearthdata.com",
		Note: "The outline of the world on the photo map."},
	{Group: GroupServices, Name: "The Movie Database (TMDB)", License: "TMDB's terms", URL: "https://www.themoviedb.org/terms-of-use",
		Note: "Used with your own key. This product uses the TMDB API but is not endorsed or certified by TMDB."},
	{Group: GroupServices, Name: "TheTVDB", License: "TheTVDB's terms", URL: "https://thetvdb.com",
		Note: "Used with your own key. TV information and images are provided by TheTVDB."},
	{Group: GroupServices, Name: "TVmaze", License: "CC BY-SA 4.0", URL: "https://www.tvmaze.com/api", Note: "TV information, without keys."},
	{Group: GroupServices, Name: "Wikidata", License: "CC0", URL: "https://www.wikidata.org", Note: "Facts about movies and games."},
	{Group: GroupServices, Name: "Wikipedia", License: "CC BY-SA 4.0", URL: "https://en.wikipedia.org/wiki/Wikipedia:Text_of_the_Creative_Commons_Attribution-ShareAlike_4.0_International_License",
		Note: "Plot summaries, credited where they're shown."},
	{Group: GroupServices, Name: "AniList", License: "AniList's terms", URL: "https://anilist.co/terms", Note: "Anime information."},
	{Group: GroupServices, Name: "Apple Podcasts search", License: "Apple's terms", URL: "https://www.apple.com/legal/internet-services/itunes/", Note: "Finding podcasts to add."},
	{Group: GroupServices, Name: "Free Dictionary API", License: "The service's terms", URL: "https://dictionaryapi.dev", Note: "Looking up a word while reading."},
	{Group: GroupServices, Name: "libretro database", License: "CC BY-SA 4.0", URL: "https://github.com/libretro/libretro-database", Note: "Recognising game files, on this server."},
	{Group: GroupServices, Name: "libretro thumbnails", License: "None stated; pictures belong to their publishers", URL: "https://github.com/libretro-thumbnails/libretro-thumbnails", Note: "Box art and screenshots for games, kept on this server."},
	{Group: GroupServices, Name: "IGDB", License: "IGDB's terms: non-commercial use", URL: "https://www.igdb.com", Note: "Used with your own key. Game details and scores."},
	{Group: GroupServices, Name: "RAWG", License: "RAWG's API terms", URL: "https://rawg.io/apidocs", Note: "Used with your own key. Game details, with a link back to RAWG."},
	{Group: GroupServices, Name: "ScreenScraper", License: "ScreenScraper's terms", URL: "https://www.screenscraper.fr", Note: "Used with your own account. Recognising games and their art."},
	{Group: GroupServices, Name: "TheGamesDB", License: "No written terms", URL: "https://thegamesdb.net", Note: "Used with your own key. Game details and art, credited."},
	{Group: GroupServices, Name: "MusicBrainz", License: "CC0 (core data)", URL: "https://musicbrainz.org/doc/About/Data_License", Note: "Album and track names for audio CD images."},
	{Group: GroupServices, Name: "Cover Art Archive", License: "Each picture's own", URL: "https://coverartarchive.org", Note: "Album covers for audio CD images."},
	{Group: GroupServices, Name: "Radio Browser", License: "Public domain data", URL: "https://www.radio-browser.info", Note: "Finding internet radio stations. The stations themselves belong to whoever runs them."},
	{Group: GroupServices, Name: "libretro build server", License: "Each core's own", URL: "https://buildbot.libretro.com", Note: "Where a game core comes from, when an admin installs one."},
}

var (
	once  sync.Once
	list  []Component
	texts []string
	which []int
)

func load() {
	once.Do(func() {
		var f File
		z, err := gzip.NewReader(bytes.NewReader(librariesGz))
		if err == nil {
			err = json.NewDecoder(z).Decode(&f)
		}
		if err != nil {
			panic("licenses: the data built into this program can't be read: " + err.Error())
		}
		texts = f.Texts
		order := map[string]int{}
		for i, g := range Groups {
			order[g.ID] = i
		}
		for _, l := range f.Libraries {
			list = append(list, Component{Group: l.Group, Name: l.Name, License: l.License, URL: l.URL, HasText: true})
			which = append(which, l.Text)
		}
		more := append(append([]Component{}, written...), coreComponents()...)
		for _, c := range more {
			list = append(list, c)
			which = append(which, -1)
		}
		ids := make([]int, len(list))
		for i := range ids {
			ids[i] = i
		}
		sort.SliceStable(ids, func(i, j int) bool { return order[list[ids[i]].Group] < order[list[ids[j]].Group] })
		sorted, sortedWhich := make([]Component, len(list)), make([]int, len(list))
		for to, from := range ids {
			sorted[to], sortedWhich[to] = list[from], which[from]
			sorted[to].ID = to
		}
		list, which = sorted, sortedWhich
	})
}

// coreComponents lists the cores this server offers, from the same table that offers
// them, so the two can't disagree.
func coreComponents() []Component {
	seen := map[string]bool{}
	var out []Component
	for i := range wittyplay.Consoles {
		for _, c := range wittyplay.Consoles[i].Cores {
			if seen[c.ID] {
				continue
			}
			seen[c.ID] = true
			out = append(out, Component{Group: GroupCores, Name: c.Name, License: c.License, URL: "https://github.com/libretro"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// All is every component, grouped, in the order Groups lists. Callers mustn't change it.
func All() []Component {
	load()
	return list
}

// Text is a component's full license text, if it has one here.
func Text(id int) (string, bool) {
	load()
	if id < 0 || id >= len(list) || which[id] < 0 {
		return "", false
	}
	return texts[which[id]], true
}
