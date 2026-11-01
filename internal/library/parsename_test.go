package library

import "testing"

func TestParseName(t *testing.T) {
	cases := []struct {
		in    string
		title string
		year  int
	}{
		{"8707Dr.Story.The.Treeline.iso", "Dr Story The Treeline", 0},
		{"8707Dr.Story.The.Treeline", "Dr Story The Treeline", 0}, // the scanner passes names without the extension
		{"The.Matrix.1999.1080p.BluRay.x264.mkv", "The Matrix", 1999},
		{"Some.Movie.Title", "Some Movie Title", 0},
		{"Polar Express (2004)", "Polar Express", 2004},
		{"SWAMPTALE_2", "Swamptale 2", 0},
		{"1917.2019.mkv", "1917", 2019},
	}
	for _, c := range cases {
		title, year := ParseName(c.in)
		y := 0
		if year != nil {
			y = *year
		}
		if title != c.title || y != c.year {
			t.Errorf("ParseName(%q) = %q, %d; want %q, %d", c.in, title, y, c.title, c.year)
		}
	}
}

func TestLegacyTitle(t *testing.T) {
	// The earlier parse dropped the last dotted word; such titles are fixed.
	if got := legacyTitle("8707Dr.Story.The.Treeline"); got != "8707Dr Story The" {
		t.Errorf("legacyTitle = %q", got)
	}
	// Names it parsed correctly match the new parse, so nothing changes.
	if got, _ := ParseName("The.Matrix.1999.1080p"); got != legacyTitle("The.Matrix.1999.1080p") {
		t.Errorf("legacy %q differs from %q", legacyTitle("The.Matrix.1999.1080p"), got)
	}
}

func TestWebURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://example.com/a.mp3": "https://example.com/a.mp3",
		" HTTP://x/y ":              "HTTP://x/y",
		`C:\Users\me\secret.txt`:    "",
		"file:///etc/passwd":        "",
		"/etc/passwd":               "",
	} {
		if got := webURL(in); got != want {
			t.Errorf("webURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikeTVDisc(t *testing.T) {
	for in, want := range map[string]bool{
		"Friends Season 1 Disc 2": true, "THE_OFFICE_S2D1": true, "Doctor Who Series 4": true,
		"Planet Earth Complete Box Set": true, "Dr Story The Treeline": false, "Swamptale 2": false, "THE POLAR EXPRESS 3D": false,
	} {
		if got := LooksLikeTVDisc(in); got != want {
			t.Errorf("LooksLikeTVDisc(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestGuessDiscContent(t *testing.T) {
	for _, c := range []struct{ lib, title, label, want string }{
		{"movies", "Swamptale 2", "SWAMPTALE_2", "movie"},
		{"discs", "Friends Season 3 Disc 1", "FRIENDS_S3D1", "tv"},
		{"discs", "The Treeline", "THE_TREELINE", "movie"},
		{"discs", "Planet Earth", "PLANET_EARTH_S1D2", "tv"},
		{"shows", "Disc 1", "DISC_1", "tv"},
		{"anime", "Spirited", "", "tv"},
	} {
		if got := GuessDiscContent(c.lib, c.title, c.label); got != c.want {
			t.Errorf("GuessDiscContent(%q, %q, %q) = %q, want %q", c.lib, c.title, c.label, got, c.want)
		}
	}
}
