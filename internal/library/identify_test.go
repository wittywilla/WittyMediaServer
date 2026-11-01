package library

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPartNames(t *testing.T) {
	for name, want := range map[string]int{
		"Film CD1": 1, "Film CD2": 2, "Film.cd2": 2, "Film - DVD 2": 2, "Film_disc1": 1, "Film.part2": 2, "Film part1": 1, "Film-pt3": 3,
		"Film (2001) Part 2": 2,
	} {
		_, n, ok := partOf(name)
		if !ok || n != want {
			t.Errorf("%q: part %d, %v", name, n, ok)
		}
	}
	for _, name := range []string{
		"Night Falls Part 2", // a sequel's title, not a part
		"Show S01E01 CD1", "Show 1x02 part1", // an episode split in two is two episodes
		"CD1", "Film", "Part of Me", "Film CD99",
	} {
		if _, n, ok := partOf(name); ok {
			t.Errorf("%q: read as part %d", name, n)
		}
	}
}

func TestStacking(t *testing.T) {
	dir := t.TempDir()
	mk := func(names ...string) []candidate {
		var out []candidate
		for _, n := range names {
			p := filepath.Join(dir, n)
			write(t, p, []byte("x"))
			out = append(out, candidate{path: p, kind: "video"})
		}
		return out
	}
	kept := func(c []candidate) []string {
		var out []string
		for _, x := range c {
			out = append(out, filepath.Base(x.path))
		}
		slices.Sort(out)
		return out
	}
	got := kept(stackParts(mk("A CD1.avi", "A CD2.avi", "A CD3.avi", "Solo CD1.avi", "B CD1.mkv", "B CD3.mkv", "C part1.mkv", "C part2.avi", "Other.mkv")))
	want := []string{"A CD1.avi", "B CD1.mkv", "B CD3.mkv", "C part1.mkv", "C part2.avi", "Other.mkv", "Solo CD1.avi"}
	if !slices.Equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
	parts := StackedParts(filepath.Join(dir, "A CD1.avi"))
	if len(parts) != 3 || filepath.Base(parts[2]) != "A CD3.avi" {
		t.Errorf("parts %v", parts)
	}
	if p := StackedParts(filepath.Join(dir, "A CD2.avi")); len(p) != 1 {
		t.Errorf("a later part is only itself: %v", p)
	}
	if p := StackedParts(filepath.Join(dir, "B CD1.mkv")); len(p) != 1 {
		t.Errorf("a gap: %v", p)
	}
	if p := StackedParts(filepath.Join(dir, "Other.mkv")); len(p) != 1 {
		t.Errorf("an ordinary file: %v", p)
	}
}

func TestFindSubtitles(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Film.mkv")
	write(t, video, []byte("x"))
	for _, n := range []string{"Film.srt", "Film.en.srt", "Film.eng.forced.srt", "Film.es.SDH.ass", "Film.Dutch.vtt", "Film.pt-BR.default.srt", "Film 2.srt", "Other.srt", "Film.en.idx", "Film.en.sub", "Film.sup", "Film.nfo", "Subs/Film.fr.srt", "Subs/German.srt", "Subs/Film/Italian.srt"} {
		write(t, filepath.Join(dir, n), []byte("# VobSub\n"))
	}
	byName := map[string]SubtitleFile{}
	for _, s := range FindSubtitles(video) {
		rel, _ := filepath.Rel(dir, s.Path)
		byName[filepath.ToSlash(rel)] = s
	}
	for _, name := range []string{"Film 2.srt", "Other.srt", "Film.en.sub", "Film.nfo"} {
		if _, ok := byName[name]; ok {
			t.Errorf("%s isn't this film's subtitle", name)
		}
	}
	check := func(name, lang string, forced, sdh, def bool, codec string) {
		t.Helper()
		s, ok := byName[name]
		if !ok {
			t.Errorf("no %s among %v", name, byName)
			return
		}
		if s.Language != lang || s.Forced != forced || s.SDH != sdh || s.Default != def || s.Codec != codec {
			t.Errorf("%s: %+v", name, s)
		}
	}
	check("Film.srt", "", false, false, false, "subrip")
	check("Film.en.srt", "en", false, false, false, "subrip")
	check("Film.eng.forced.srt", "en", true, false, false, "subrip")
	check("Film.es.SDH.ass", "es", false, true, false, "ass")
	check("Film.Dutch.vtt", "nl", false, false, false, "webvtt")
	check("Film.pt-BR.default.srt", "pt", false, false, true, "subrip")
	check("Film.en.idx", "en", false, false, false, "dvd_subtitle")
	check("Film.sup", "", false, false, false, "hdmv_pgs_subtitle")
	check("Subs/Film.fr.srt", "fr", false, false, false, "subrip")
	check("Subs/German.srt", "de", false, false, false, "subrip")
	check("Subs/Film/Italian.srt", "it", false, false, false, "subrip")
}

func TestSubtitlesFolderNeedsToBeThisFilms(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "One.mkv"), []byte("x"))
	write(t, filepath.Join(dir, "Two.mkv"), []byte("x"))
	write(t, filepath.Join(dir, "Subs", "English.srt"), []byte("x"))
	if got := FindSubtitles(filepath.Join(dir, "One.mkv")); len(got) != 0 {
		t.Errorf("two films, one loose subtitle: %v", got)
	}
}

func TestPlaylistFormats(t *testing.T) {
	for format, in := range map[string]string{
		"m3u":  "#EXTM3U\n#EXTINF:100,Song\n01 One.mp3\n\nsub/Two.flac\nhttp://evil.test/x.mp3\n",
		"pls":  "[playlist]\nNumberOfEntries=3\nFile2=sub/Two.flac\nFile1=01 One.mp3\nFile3=http://evil.test/x.mp3\nTitle1=a\n",
		"xspf": `<?xml version="1.0"?><playlist xmlns="http://xspf.org/ns/0/"><trackList><track><location>01 One.mp3</location></track><track><title>x</title><location>sub/Two.flac</location></track><track><location>http://evil.test/x.mp3</location></track></trackList></playlist>`,
		"asx":  `<asx version="3.0"><entry><ref href="01 One.mp3"/></entry><entry><ref href="sub/Two.flac"/></entry><entry><ref href="http://evil.test/x.mp3"/></entry></asx>`,
	} {
		got := ParseEntries(format, []byte(in))
		if len(got) != 3 || got[0] != "01 One.mp3" || got[1] != "sub/Two.flac" {
			t.Errorf("%s: %q", format, got)
		}
	}
}

func TestPlaylistTargets(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator)+"music", "album")
	if got := playlistTarget(dir, "http://evil.test/x.mp3"); got != "" {
		t.Errorf("a web address has no path: %q", got)
	}
	if got := playlistTarget(dir, "rtsp://evil.test/x"); got != "" {
		t.Errorf("nor any other kind: %q", got)
	}
	if got := playlistTarget(dir, "a.mp3"); got != filepath.Join(dir, "a.mp3") {
		t.Errorf("relative: %q", got)
	}
	if got := playlistTarget(dir, `sub\b.mp3`); got != filepath.Join(dir, "sub", "b.mp3") {
		t.Errorf("Windows-style: %q", got)
	}
}

func TestSVGIsDefused(t *testing.T) {
	attacks := []string{
		`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><rect width="1" height="1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script><rect width="1" height="1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><a href="javascript:alert(1)"><rect width="1" height="1"/></a></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><image href="http://evil.test/x.png"/><rect width="1" height="1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="http://evil.test/x.svg#a"/><rect width="1" height="1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><iframe src="http://evil.test"></iframe></foreignObject><rect width="1" height="1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><style>@import url(http://evil.test/x.css);</style><rect width="1" height="1"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><rect width="1" height="1" fill="url(http://evil.test/x)" style="background:url(http://evil.test)"/></svg>`,
		`<svg xmlns="http://www.w3.org/2000/svg"><animate attributeName="href" to="javascript:alert(1)"/><set attributeName="onmouseover" to="alert(1)"/><rect width="1" height="1"/></svg>`,
		`<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg xmlns="http://www.w3.org/2000/svg"><text>&x;</text><rect width="1" height="1"/></svg>`,
	}
	for _, a := range attacks {
		got, err := SanitizeSVG([]byte(a))
		if err != nil {
			t.Errorf("%s: %v", a, err)
			continue
		}
		s := string(got)
		for _, bad := range []string{"script", "onload", "javascript", "evil.test", "iframe", "foreignObject", "<style", "<animate", "<set", "passwd", "style="} {
			if strings.Contains(s, bad) {
				t.Errorf("%q in %s", bad, s)
			}
		}
		if !strings.Contains(s, "<rect") {
			t.Errorf("the picture is gone: %s", s)
		}
	}
	keep := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><defs><linearGradient id="g"><stop offset="0" stop-color="red"/></linearGradient></defs><rect width="10" height="10" fill="url(#g)"/><use href="#g"/></svg>`
	got, err := SanitizeSVG([]byte(keep))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`fill="url(#g)"`, `viewbox="0 0 10 10"`, "<lineargradient", `href="#g"`} {
		if !strings.Contains(strings.ToLower(string(got)), want) {
			t.Errorf("no %q in %s", want, got)
		}
	}
	if _, err := SanitizeSVG([]byte("<html><body>not a picture</body></html>")); err == nil {
		t.Error("a web page isn't a picture")
	}
	if _, err := SanitizeSVG([]byte(strings.Repeat("<svg>", 100000))); err == nil {
		t.Error("nesting without end should be refused")
	}
}

func FuzzSanitizeSVG(f *testing.F) {
	f.Add([]byte(`<svg><script>x</script><rect onload="x"/></svg>`))
	f.Add([]byte("\x1F\x8B\x08"))
	f.Fuzz(func(t *testing.T, b []byte) {
		out, err := SanitizeSVG(b)
		if err == nil && strings.Contains(strings.ToLower(string(out)), "<script") {
			t.Errorf("a script got through: %s", out)
		}
	})
}

func TestEmbeddedMotion(t *testing.T) {
	dir := t.TempDir()
	clip := append([]byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"), make([]byte, 500)...)
	photo := append([]byte("\xFF\xD8\xFF\xFE\x00\x14GCamera:MotionPhoto\xFF\xD9"), clip...)
	p := filepath.Join(dir, "m.jpg")
	write(t, p, photo)
	got := embeddedMotion(p, int64(len(photo)))
	off, length, ok := ParseEmbeddedMotion(got)
	if !ok || off != int64(len(photo)-len(clip)) || length != int64(len(clip)) {
		t.Errorf("%q: %d %d %v", got, off, length, ok)
	}
	plain := filepath.Join(dir, "plain.jpg")
	write(t, plain, append([]byte("\xFF\xD8\xFF\xD9"), clip...))
	if got := embeddedMotion(plain, int64(len(clip)+4)); got != "" {
		t.Errorf("no mark, no read: %q", got)
	}
	if _, _, ok := ParseEmbeddedMotion("/some/path.mov"); ok {
		t.Error("a path isn't a place in a file")
	}
}

func TestDiscoverChecksNames(t *testing.T) {
	dir := t.TempDir()
	ts := make([]byte, 188*6)
	for i := 0; i < len(ts); i += 188 {
		ts[i] = 0x47
	}
	write(t, filepath.Join(dir, "real.ts"), ts)
	write(t, filepath.Join(dir, "code.ts"), []byte("export const x: number = 1\n"))
	write(t, filepath.Join(dir, "unnamed"), append([]byte("\x1A\x45\xDF\xA3\x01\x42\x82\x88matroska"), make([]byte, 1024)...))
	write(t, filepath.Join(dir, "notes"), []byte(strings.Repeat("just some words ", 100)))
	write(t, filepath.Join(dir, "half.mkv.part"), append([]byte("\x1A\x45\xDF\xA3"), make([]byte, 1024)...))
	c, err := discoverClips(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, x := range c {
		names = append(names, filepath.Base(x.path))
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"real.ts", "unnamed"}) {
		t.Errorf("found %v", names)
	}
}

func TestBooksLeaveOutNotes(t *testing.T) {
	dir := t.TempDir()
	long := []byte(strings.Repeat("A sentence of a book, going on. ", 200))
	write(t, filepath.Join(dir, "A Book.txt"), long)
	write(t, filepath.Join(dir, "readme.txt"), long)
	write(t, filepath.Join(dir, "short.txt"), []byte("not much"))
	write(t, filepath.Join(dir, "Both.txt"), long)
	write(t, filepath.Join(dir, "Both.epub"), []byte("PK\x03\x04"))
	write(t, filepath.Join(dir, "Notes.md"), long)
	c, err := discoverBooks(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, x := range c {
		names = append(names, filepath.Base(x.path))
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"A Book.txt", "Both.epub"}) {
		t.Errorf("found %v", names)
	}
}

func TestTitlesKeepWordsThatLookLikeExtensions(t *testing.T) {
	for name, want := range map[string]string{"The.Nut": "The Nut", "Mr.Doc": "Mr Doc", "Far.Med": "Far Med", "A.Film.mkv": "A Film", "Long.Title.iso": "Long Title"} {
		if got, _ := ParseName(name); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
}

func TestWhatPlaysDirect(t *testing.T) {
	disc := int64(1)
	for _, c := range []struct {
		name string
		it   Item
		want bool
	}{
		{"mp3", Item{Container: "mp3", Path: "/a.mp3", VideoCodec: "mp3"}, true},
		{"m4a", Item{Container: "mov", Path: "/a.m4a", VideoCodec: "aac"}, true},
		{"alac", Item{Container: "mov", Path: "/a.m4a", VideoCodec: "alac"}, false},
		{"flac in matroska", Item{Container: "matroska", Path: "/a.mka", VideoCodec: "flac"}, false},
		{"opus in weba", Item{Container: "matroska", Path: "/a.weba", VideoCodec: "opus"}, true},
		{"a CD track", Item{Container: "cue", Path: "/a.cue#track=1", VideoCodec: "pcm_s16le", CDDisc: &disc}, true},
		{"a podcast online", Item{Path: "https://example.test/ep.mp3?x=1", VideoCodec: "mp3"}, true},
		{"nothing known", Item{Path: "/a.ape", VideoCodec: "ape"}, false},
	} {
		if got := audioPlaysDirect(&c.it); got != c.want {
			t.Errorf("%s: %v", c.name, got)
		}
	}
}
