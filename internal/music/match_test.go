package music

import "testing"

func TestKeys(t *testing.T) {
	for _, c := range []struct{ in, title string }{
		{"Lights (Official Music Video)", "lights"},
		{"Zburătorul [HD]", "zburatorul"},
		{"Eat The Sun - Official Video", "eat the sun"},
		{"Rock & Roll (feat. Someone Else)", "rock and roll"},
		{"The Beatles", "the beatles"},
		{"Song ft. A, B", "song"},
		{"Les voyages de l'âme", "les voyages de l ame"},
		{"Autumn (Live)", "autumn live"},
	} {
		if got := TitleKey(c.in); got != c.title {
			t.Errorf("TitleKey(%q) = %q, want %q", c.in, got, c.title)
		}
	}
	for in, want := range map[string]string{"The Beatles": "beatles", "Tora feat. Someone": "tora", "Doc & Lena": "doc and lena", "MYA Symphony Orchestra": "mya symphony orchestra"} {
		if got := ArtistKey(in); got != want {
			t.Errorf("ArtistKey(%q) = %q, want %q", in, got, want)
		}
	}
	if !IsTake("Autumn (Live at the Hall)") || !IsTake("Lights - Acoustic") || IsTake("Lights (Official Video)") {
		t.Error("takes aren't told apart")
	}
}

func TestCodes(t *testing.T) {
	if CleanISRC("us-rc1-76-07839") != "USRC17607839" || CleanISRC("nope") != "" || CleanISRC("12RC17607839") != "" {
		t.Error("ISRCs aren't cleaned")
	}
	if CleanMBID("1A2B3C4D-1111-2222-3333-444455556666") != "1a2b3c4d-1111-2222-3333-444455556666" || CleanMBID("1a2b3c4d") != "" || CleanMBID("zzzzzzzz-1111-2222-3333-444455556666") != "" {
		t.Error("MusicBrainz IDs aren't cleaned")
	}
}

func find(ps []Pair, song, video int64) *Pair {
	for i := range ps {
		if ps[i].SongID == song && ps[i].VideoID == video {
			return &ps[i]
		}
	}
	return nil
}

func TestMatch(t *testing.T) {
	songs := []Recording{
		{ID: 1, Title: "Lights", Artists: []string{"Cinnamon Chasers"}, Seconds: 300},
		{ID: 2, Title: "Autumn", Artists: []string{"Tes Elations"}, Seconds: 200},
		{ID: 3, Title: "Autumn", Artists: []string{"Tes Elations"}, Seconds: 260}, // a second album's copy
		{ID: 4, Title: "Eat the Sun", Artists: []string{"Tora"}, ISRC: "AUABC1600001", Seconds: 240},
		{ID: 5, Title: "Revolution (Live)", Artists: []string{"Fontano"}, Seconds: 250},
		{ID: 6, Title: "Mocassin", Artists: []string{"Razrfish"}, Seconds: 180},
		{ID: 7, Title: "About Life", Artists: []string{"Someofw"}, MBID: "1a2b3c4d-1111-2222-3333-444455556666"},
		{ID: 8, Title: "Tu", Artists: []string{"All In"}},
	}
	videos := []Recording{
		{ID: 101, Title: "Lights (Official Video)", Artists: []string{"Cinnamon Chasers"}, Seconds: 301},
		{ID: 102, Title: "Autumn", Artists: []string{"Tes Elations"}, Seconds: 259},
		{ID: 103, Title: "A different title", Artists: []string{"Someone"}, ISRC: "AUABC1600001", Seconds: 250},
		{ID: 104, Title: "Revolution", Artists: []string{"FONTANO"}, Seconds: 250},
		{ID: 105, Title: "Mocassins", Artists: []string{"Razrfish"}},
		{ID: 106, Title: "About life", Artists: []string{"Somebody"}, MBID: "1a2b3c4d-1111-2222-3333-444455556666"},
		{ID: 107, Title: "Tu", Artists: []string{"All In"}, Seconds: 3600}, // a concert
		{ID: 108, Title: "Lights", Artists: []string{"Cinnamon Chaser"}},
	}
	ps := Match(songs, videos, nil)
	if p := find(ps, 1, 101); p == nil || !p.Sure || p.How != "name" || p.SyncMS == nil || *p.SyncMS != 0 {
		t.Errorf("the same artist and title, with the video's words dropped, isn't tied and lined up: %+v", p)
	}
	for _, s := range []int64{2, 3} {
		if p := find(ps, s, 102); p == nil || p.Sure {
			t.Errorf("two songs that fit are only suggestions: song %d %+v", s, p)
		}
	}
	if p := find(ps, 4, 103); p == nil || !p.Sure || p.How != "isrc" || p.SyncMS != nil {
		t.Errorf("the same ISRC isn't tied (and lengths 10s apart don't line up): %+v", p)
	}
	if p := find(ps, 5, 104); p == nil || p.Sure {
		t.Errorf("a live take is only a suggestion for the studio video: %+v", p)
	}
	if p := find(ps, 6, 105); p == nil || p.Sure || p.How != "close" {
		t.Errorf("a nearly-the-same title is only a suggestion: %+v", p)
	}
	if p := find(ps, 7, 106); p == nil || !p.Sure || p.How != "recording" {
		t.Errorf("the same recording isn't tied: %+v", p)
	}
	if p := find(ps, 8, 107); p != nil {
		t.Errorf("a concert is tied to a song: %+v", p)
	}
	if p := find(ps, 1, 108); p == nil || p.Sure {
		t.Errorf("a nearly-the-same artist is only a suggestion: %+v", p)
	}
	for _, p := range ps {
		if p.Sure && (p.How == "close") {
			t.Errorf("something fuzzy was tied: %+v", p)
		}
	}

	// Untied pairs stay untied.
	ps = Match(songs, videos, func(s, v int64) bool { return s == 1 && v == 101 })
	if find(ps, 1, 101) != nil {
		t.Error("an untied pair came back")
	}
}
