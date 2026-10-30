package library

import "testing"

func TestParseName(t *testing.T) {
	cases := []struct {
		in    string
		title string
		year  int
	}{
		{"8707Dr.Seuss.The.Lorax.iso", "Dr Seuss The Lorax", 0},
		{"8707Dr.Seuss.The.Lorax", "Dr Seuss The Lorax", 0}, // the scanner passes names without the extension
		{"The.Matrix.1999.1080p.BluRay.x264.mkv", "The Matrix", 1999},
		{"Some.Movie.Title", "Some Movie Title", 0},
		{"Polar Express (2004)", "Polar Express", 2004},
		{"SHREK_2", "Shrek 2", 0},
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
	if got := legacyTitle("8707Dr.Seuss.The.Lorax"); got != "8707Dr Seuss The" {
		t.Errorf("legacyTitle = %q", got)
	}
	// Names it parsed correctly match the new parse, so nothing changes.
	if got, _ := ParseName("The.Matrix.1999.1080p"); got != legacyTitle("The.Matrix.1999.1080p") {
		t.Errorf("legacy %q differs from %q", legacyTitle("The.Matrix.1999.1080p"), got)
	}
}
