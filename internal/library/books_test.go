package library

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestParseComicName(t *testing.T) {
	cases := []struct{ in, series, num string }{
		{"Mashle - Magic and Muscles 145 (2023) (Digital) (anadius)", "Mashle - Magic and Muscles", "145"},
		{"One Piece v03", "One Piece", "03"},
		{"Saga #12", "Saga", "12"},
		{"Moby Dick", "Moby Dick", ""},
	}
	for _, c := range cases {
		s, n, _ := parseComicName(c.in)
		if s != c.series || n != c.num {
			t.Errorf("%q -> %q %q, want %q %q", c.in, s, n, c.series, c.num)
		}
	}
	if _, _, y := parseComicName("Mashle 145 (2023)"); y == nil || *y != 2023 {
		t.Error("year not read")
	}
}

func TestNaturalOrder(t *testing.T) {
	p := []string{"p10.jpg", "p2.jpg", "p1.jpg", "P3.jpg"}
	sort.Slice(p, func(i, j int) bool { return naturalLess(p[i], p[j]) })
	if p[0] != "p1.jpg" || p[1] != "p2.jpg" || p[2] != "P3.jpg" || p[3] != "p10.jpg" {
		t.Errorf("order: %v", p)
	}
}

// A digital-library download: one comic in three formats plus scans, OCR
// and a DRM-locked copy must become exactly one book, read from the CBZ.
func TestDiscoverBooksGroupsFormats(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Mashle (Manga)")
	_ = os.MkdirAll(filepath.Join(dir, "x_daisy"), 0o755)
	stem := "Mashle - Magic and Muscles 145 (2023) (Digital) (anadius)"
	for _, n := range []string{stem + ".cbz", stem + ".epub", stem + ".pdf", stem + "_lcp.epub", stem + "_jp2.zip", stem + "_djvu.txt",
		"takhbfzz_meta.xml", "__ia_thumb.jpg", "x_daisy/book.opf"} {
		_ = os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644)
	}
	_ = os.WriteFile(filepath.Join(root, "MolbyDick.epub"), []byte("x"), 0o644)
	got, err := discoverBooks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || filepath.Base(got[0].path) != stem+".cbz" || filepath.Base(got[1].path) != "MolbyDick.epub" {
		var names []string
		for _, c := range got {
			names = append(names, filepath.Base(c.path))
		}
		t.Fatalf("books: %v", names)
	}
}
