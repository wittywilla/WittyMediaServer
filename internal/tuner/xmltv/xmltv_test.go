// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package xmltv

import (
	"fmt"
	"strings"
	"testing"
)

const guide = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
<channel id="harbour.test"><display-name>Harbour Network</display-name></channel>
<programme start="20261112193000 +0000" stop="20261112203000 +0000" channel="harbour.test">
  <title>The Quiet Orchard</title>
  <sub-title>The Ledger</sub-title>
  <desc>The orchard's last tenant must decide before the frost.</desc>
  <credits>
    <director>Ana Ruiz</director>
    <actor role="Nell">Mara Voss</actor>
    <actor role="Dr. Hale" guest="yes">Jon Ellis</actor>
    <writer>Lee Park</writer>
    <presenter>Kim Ode</presenter>
    <unknownrole>Nobody</unknownrole>
  </credits>
  <date>2024</date>
  <category>Drama</category>
  <category>Mystery</category>
  <keyword>orchard</keyword>
  <language>English</language>
  <orig-language>English</orig-language>
  <length units="hours">1</length>
  <country>NZ</country>
  <episode-num system="xmltv_ns">1.3.</episode-num>
  <episode-num system="imdb.com">title/tt0123456/</episode-num>
  <episode-num system="themoviedb.org">tv/4242/season/2</episode-num>
  <video><quality>HDTV</quality></video>
  <audio><stereo>dolby digital</stereo></audio>
  <new/>
  <subtitles type="teletext"><language>EN</language></subtitles>
  <subtitles type="onscreen"/>
  <subtitles type="deaf-signed"/>
  <rating system="VCHIP"><value>TV-PG</value></rating>
  <star-rating system="IMDB"><value>8.1/10</value></star-rating>
</programme>
<programme start="20261112203000 +0000" stop="20261112210000 +0000" channel="harbour.test"><title>Weather</title></programme>
</tv>`

func read(t *testing.T, src string) []Programme {
	t.Helper()
	var out []Programme
	if err := Parse(strings.NewReader(src), nil, func(p Programme) bool { out = append(out, p); return true }); err != nil {
		t.Fatal(err)
	}
	return out
}

// Everything the More info pop-up shows that a guide can say, read from one programme.
func TestProgrammeDetails(t *testing.T) {
	list := read(t, guide)
	if len(list) != 2 {
		t.Fatalf("%d programmes", len(list))
	}
	p, d := list[0], list[0].Details
	if p.Title != "The Quiet Orchard" || p.Sub != "The Ledger" || p.Category != "Drama" || p.Rating != "TV-PG" || *p.Season != 2 || *p.Episode != 4 {
		t.Fatalf("the grid's line: %+v", p)
	}
	want := []Credit{{Role: "director", Name: "Ana Ruiz"}, {Role: "actor", Name: "Mara Voss", Character: "Nell"},
		{Role: "actor", Name: "Jon Ellis", Character: "Dr. Hale", Guest: true}, {Role: "writer", Name: "Lee Park"}, {Role: "presenter", Name: "Kim Ode"}}
	if fmt.Sprint(d.Credits) != fmt.Sprint(want) {
		t.Errorf("credits %+v", d.Credits)
	}
	if d.Year != 2024 || fmt.Sprint(d.Categories) != "[Mystery]" || fmt.Sprint(d.Keywords) != "[orchard]" || d.Language != "English" ||
		d.OrigLanguage != "English" || d.Country != "NZ" || d.LengthMin != 60 {
		t.Errorf("facts %+v", d)
	}
	if d.IMDb != "tt0123456" || d.TMDb != "tv/4242" {
		t.Errorf("ids %q %q", d.IMDb, d.TMDb)
	}
	if !d.HD || d.Audio != "dolby digital" || !d.New || d.Premiere || d.Repeat || !d.SignLanguage || fmt.Sprint(d.Subtitles) != "[EN onscreen]" {
		t.Errorf("picture, sound, subtitles %+v", d)
	}
	if len(d.Stars) != 1 || d.Stars[0] != (Star{Value: "8.1/10", System: "IMDB"}) {
		t.Errorf("stars %+v", d.Stars)
	}
	if w := list[1].Details; w.Year != 0 || len(w.Credits) != 0 {
		t.Errorf("a programme that says nothing more: %+v", w)
	}
}

// A guide that says a great deal about one programme still costs little to keep.
func TestProgrammeDetailsAreBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<tv><programme start="20261112193000" stop="20261112203000" channel="x"><title>Big</title><credits>`)
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&b, `<actor role="%s">Actor %d</actor>`, strings.Repeat("r", 500), i)
	}
	b.WriteString(`</credits>`)
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, `<category>C%d</category><keyword>K%d</keyword>`, i, i)
	}
	b.WriteString(`</programme></tv>`)
	d := read(t, b.String())[0].Details
	if len(d.Credits) != maxCredits || len([]rune(d.Credits[0].Character)) != maxDetail || len(d.Categories) != maxListItems || len(d.Keywords) != maxListItems {
		t.Fatalf("%d credits, part %d long, %d categories, %d keywords", len(d.Credits), len(d.Credits[0].Character), len(d.Categories), len(d.Keywords))
	}
}

func TestTitleIDs(t *testing.T) {
	for in, want := range map[string]string{"tt0123456": "tt0123456", "https://www.imdb.com/title/tt7654321/": "tt7654321", "tt": "", "nm0000001": ""} {
		if got := imdbID(in); got != want {
			t.Errorf("imdb %q: %q", in, got)
		}
	}
	for in, want := range map[string]string{"movie/603": "movie/603", "/tv/1399/season/1": "tv/1399", "person/3": "", "movie/abc": ""} {
		if got := tmdbID(in); got != want {
			t.Errorf("tmdb %q: %q", in, got)
		}
	}
}
