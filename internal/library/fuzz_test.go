package library

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// What goes wrong in a parser of a hostile file isn't only a crash: an answer that takes minutes
// holds a scan or a request just as well. These fail on that too.
func inTime(t *testing.T, limit time.Duration, f func()) {
	t.Helper()
	start := time.Now()
	f()
	if d := time.Since(start); d > limit {
		t.Fatalf("took %v", d)
	}
}

func FuzzNames(f *testing.F) {
	for _, s := range []string{"Some Film (1999).mkv", "Show.S01E02.Title.mkv", "[Group] Show - 012 [1080p].mkv", "Comic v02 #5 (2004).cbz", "..\\..\\x.jpg", "CON.txt", "a\x00b", "1x01", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		inTime(t, 2*time.Second, func() {
			parseComicName(s)
			ParseName(s)
			ParseEpisode("/lib", "/lib/"+s)
			plain(s)
			naturalLess(s, "a10b")
		})
		for name, out := range map[string]string{"SafeFileName": SafeFileName(s), "safeName": safeName(s)} {
			if out != "" && !filepath.IsLocal(out) {
				t.Fatalf("%s(%q) = %q: not a plain name", name, s, out)
			}
		}
	})
}

func FuzzParseDuration(f *testing.F) {
	for _, s := range []string{"1:02:03", "Inf", "NaN", "1e308:1e308:1e308", "-5", "99999999999999999999", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if d := parseDuration(s); math.IsNaN(d) || math.IsInf(d, 0) || d < 0 {
			t.Fatalf("parseDuration(%q) = %v", s, d)
		}
	})
}

func FuzzArtRef(f *testing.F) {
	for _, s := range []string{"poster.jpg", "../x.jpg", "..\\x.jpg", "/etc/passwd", "C:foo.jpg", "\\\\host\\share\\x.jpg", "https://h/x.jpg", "a/../../b.jpg"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		dir := filepath.FromSlash("/lib/show")
		out := artRef(s, dir)
		if out == "" || isWebAddress(out) {
			return
		}
		rel, err := filepath.Rel(dir, out)
		if err != nil || !filepath.IsLocal(rel) {
			t.Fatalf("artRef(%q) = %q: leaves the folder", s, out)
		}
	})
}

func isWebAddress(s string) bool {
	return len(s) > 7 && (s[:7] == "http://" || (len(s) > 8 && s[:8] == "https://"))
}

func FuzzAgentAnswer(f *testing.F) {
	f.Add([]byte(`{"title":"x","year":1999,"overview":"y","poster":"https://h/p.jpg"}`))
	f.Add([]byte(`{"rating":1e999}`))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = parseAgentAnswer(b) })
}

func FuzzSniffMedia(f *testing.F) {
	f.Add([]byte("FUJIFILMCCD-RAW"))
	f.Add([]byte("\x00\x00\x00\x18ftypmp42"))
	f.Fuzz(func(t *testing.T, b []byte) { SniffMedia(b) })
}

// Files beside the media, all reached by a scan with nobody opening anything.
func FuzzSidecars(f *testing.F) {
	for _, s := range []string{
		`<movie><title>x</title><year>1999</year><rating>Inf</rating><uniqueid type="tvdb">1</uniqueid></movie>`,
		`<tvshow><title>x</title><studio>y</studio><genre>z</genre></tvshow>`,
		`<episodedetails><title>x</title><season>1</season><episode>2</episode></episodedetails>`,
		`{"title":"x","poster":"../y.jpg","rating":1e999,"custom":{"a":[{},{}]}}`,
		`{"comments":[{"text":"x","like_count":1}],"tags":["a"]}`,
		`<!DOCTYPE x [<!ENTITY a "aaaa"><!ENTITY b "&a;&a;">]><movie><title>&b;</title></movie>`,
		`FILE "a.bin" BINARY` + "\n",
		"",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		p := filepath.Join(t.TempDir(), "side")
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		inTime(t, 5*time.Second, func() {
			applyNFO(&Item{}, p)
			applySidecarJSON(&Item{}, p)
			applyShowNFO(&Show{}, p)
			applyEpisodeNFO(&Item{}, p)
			readSidecar(p)
			readMusicVideoNFO(p)
			referencedFiles(p)
		})
	})
}
