package library

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"math"
	"net/url"
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

// A rule is typed by an administrator, and the paths it's tried on are whatever files are called.
func FuzzMatchRules(f *testing.F) {
	for _, s := range [][2]string{
		{"{title} ({year})", "Some Film (1999)"}, {"*/{title} [{tmdb}]/*", "a/Some Film [123]/b"}, {"{nope}", "x"}, {"{any}{any}{any}*{title}*{title}", "aaaa"},
		{"(((", "Film [tmdbid-1] {imdb-tt123} (tvdb=9)"}, {"\\\\{edition}", "\xff\xfe"},
	} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, pattern, path string) {
		inTime(t, 5*time.Second, func() {
			if re, err := compileRule(pattern); err == nil {
				applyRule(&MatchRule{}, re, path)
			}
			IDTag(path)
			stripIDTags(path)
			ParseMatchRef(path)
		})
	})
}

// A saved view keeps a query someone wrote, and it's read again whenever the view is opened.
func FuzzBrowseQuery(f *testing.F) {
	for _, s := range []string{"genre=a&genre=b&year=1999&minRating=7.5", "limit=-1&offset=99999999999999999999&seed=x", "%zz=%zz", "decade=1&resolution=4k&sort=random&desc=1", "studio=%00%ff&content=x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		q, err := url.ParseQuery(s)
		if err != nil {
			return
		}
		inTime(t, 5*time.Second, func() {
			ParseBrowseFilter(q)
			if _, err := url.ParseQuery(CleanViewQuery(q).Encode()); err != nil {
				t.Fatalf("a cleaned query can't be read again: %v", err)
			}
			BrandKey(s)
			cleanBrandName(s)
		})
	})
}

// What a metadata source answers is kept and shown, so it's cleaned first.
func FuzzMatchAnswers(f *testing.F) {
	for _, s := range []string{`{"title":"a","year":1999,"genres":["x"],"cast":[{"name":"y"}]}`, `{"title":"\u0000\ud800","overview":"<script>"}`, `{}`, `[]`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var d MatchDetails
		if json.Unmarshal([]byte(s), &d) == nil {
			cleanDetails(&d)
		}
		var c MatchCandidate
		if json.Unmarshal([]byte(s), &c) == nil {
			cleanCandidate(&c)
		}
	})
}

// A logo comes from a source or an upload, and only ever leaves as a new picture.
func FuzzLogoPicture(f *testing.F) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		f.Fatal(err)
	}
	f.Add(b.Bytes())
	f.Add([]byte("<svg xmlns='http://www.w3.org/2000/svg' width='99999' height='99999'/>"))
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Fuzz(func(t *testing.T, in []byte) {
		inTime(t, 20*time.Second, func() { _, _ = makeLogoVariants(in) })
	})
}
