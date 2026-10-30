package library

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// Books come from anywhere: whatever is in the archive, reading it must end
// with a book or an error.
func FuzzReadEPUB(f *testing.F) {
	build := func(files map[string]string) []byte {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		for k, v := range files {
			w, _ := zw.Create(k)
			_, _ = w.Write([]byte(v))
		}
		zw.Close()
		return buf.Bytes()
	}
	f.Add(build(map[string]string{
		"META-INF/container.xml": `<container><rootfiles><rootfile full-path="a/b.opf"/></rootfiles></container>`,
		"a/b.opf":                `<package><manifest><item id="c" href="c.xhtml"/><item id="n" href="t.ncx"/></manifest><spine toc="n"><itemref idref="c"/></spine></package>`,
		"a/c.xhtml":              `<html><body>word word</body></html>`,
		"a/t.ncx":                `<ncx><navMap><navPoint><navLabel><text>x</text></navLabel><content src="../../../etc/passwd#x"/></navPoint></navMap></ncx>`,
	}))
	f.Fuzz(func(t *testing.T, b []byte) {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return
		}
		_ = ComicPages(zr)
		info, err := ReadEPUB(zr)
		if err != nil {
			return
		}
		_ = WordCounts(zr, info.Spine)
		_ = SearchBook(zr, info.Spine, "word")
	})
}

// Whatever a feed's XML holds, parsing it ends, and what comes out can't name
// anything but web addresses.
func FuzzParseFeed(f *testing.F) {
	f.Add([]byte(`<rss xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd"><channel><title>t</title><item><title>e</title><enclosure url="https://x.test/a.mp3" type="audio/mpeg"/></item></channel></rss>`))
	f.Add([]byte(`<!DOCTYPE lolz [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;">]><rss><channel><title>&b;</title></channel></rss>`))
	f.Add([]byte(`<!DOCTYPE x [<!ENTITY e SYSTEM "file:///etc/passwd">]><rss><channel><title>&e;</title></channel></rss>`))
	f.Fuzz(func(t *testing.T, b []byte) {
		fd, err := parseFeed(bytes.NewReader(b))
		if err != nil {
			return
		}
		if strings.Contains(fd.Channel.Title, "root:") {
			t.Fatal("an external entity was read")
		}
		for _, e := range fd.Channel.Items {
			_ = plain(e.Description)
			_ = parseDuration(e.Duration)
			_ = parsePub(e.PubDate)
			_ = safeName(e.Title)
		}
	})
}

func TestFeedEntitiesAreNotExpanded(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := `<!DOCTYPE x [<!ENTITY e SYSTEM "file:///` + filepath.ToSlash(secret) + `">]><rss><channel><title>a&e;b</title></channel></rss>`
	fd, err := parseFeed(strings.NewReader(doc))
	if err == nil && strings.Contains(fd.Channel.Title, "SECRET") {
		t.Fatalf("read a local file through an XML entity: %q", fd.Channel.Title)
	}
}
