// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package metafolder

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTextTakesOutWhatCouldRun(t *testing.T) {
	cases := []struct{ in, want string }{
		{"A plain summary.", "A plain summary."},
		{"<script>alert(1)</script>Hello", "Hello"},
		{"<SCRIPT SRC=x>alert(1)</SCRIPT >Hello", "Hello"},
		{"Hi<script>never closed", "Hi"},
		{"<img src=x onerror=alert(1)>Hello", "Hello"},
		{`<img src="a>b" onerror="alert(1)">Hello`, "Hello"},
		{"<img src=x onerror=alert(1)", ""},
		{"<a href=\"javascript:alert(1)\">Click</a>", "Click"},
		{"<style>body{display:none}</style>Text", "Text"},
		{"<iframe src=//evil.test></iframe>Text", "Text"},
		{"<svg onload=alert(1)><circle/></svg>Text", "Text"},
		{"&lt;script&gt;alert(1)&lt;/script&gt;Text", "Text"},
		{"&amp;lt;script&amp;gt;alert(1)&amp;lt;/script&amp;gt;Text", "Text"},
		{"<scr<script>ipt>alert(1)</script>Text", "ipt>alert(1)Text"},
		{"Line one<br>Line two", "Line one\nLine two"},
		{"Tom &amp; Jerry", "Tom & Jerry"},
		{"I <3 this and 2 < 3", "I <3 this and 2 < 3"},
		{"Na\x00me\x07", "Name"},
		{"evil\u202Egnp.exe", "evilgnp.exe"},
		{"\u2066hidden\u2069 text", "hidden text"},
		{"bad\xffbytes", "badbytes"},
		{"tag\U000E0041 chars", "tag chars"},
		{"<!-- comment --><b>Bold</b>", "Bold"},
	}
	for _, c := range cases {
		if got := Text(c.in, 1000); got != c.want {
			t.Errorf("Text(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := Line("one\ntwo<br>three", 100); got != "one two three" {
		t.Errorf("Line = %q", got)
	}
	if got := Text(strings.Repeat("é", 50), 10); got != strings.Repeat("é", 10) {
		t.Errorf("clip = %q", got)
	}
	if got := List([]string{"Drama", "drama", "<b>Comedy</b>", "", "A|B"}, 10, 60); strings.Join(got, ",") != "Drama,Comedy,A/B" {
		t.Errorf("List = %v", got)
	}
}

func TestURLAcceptsOnlyTheWeb(t *testing.T) {
	for _, u := range []string{"https://example.test/a.jpg", "http://example.test:8080/a.png?x=1"} {
		if _, ok := URL(u); !ok {
			t.Errorf("refused %s", u)
		}
	}
	for _, u := range []string{"javascript:alert(1)", "JaVaScRiPt:alert(1)", "data:text/html,<script>", "file:///etc/passwd", "ftp://x.test/a", "https://user:pass@x.test/a", "//x.test/a", "https://", "https://x.test/a b", `https:\\x.test\a`, "https://x.test/\x00", "java\nscript:alert(1)", "/etc/passwd", ""} {
		if v, ok := URL(u); ok {
			t.Errorf("accepted %q as %q", u, v)
		}
	}
}

func TestFileNameStaysInItsFolder(t *testing.T) {
	if n, ok := FileName("My Film-poster.jpg"); !ok || n != "My Film-poster.jpg" {
		t.Errorf("refused a plain name: %q", n)
	}
	for _, n := range []string{"../poster.jpg", `..\poster.jpg`, "sub/poster.jpg", `sub\poster.jpg`, "/etc/passwd", `C:\x.jpg`, "C:x.jpg", `\\server\share\x.jpg`, ".hidden.jpg", "..", ".", "a:b.jpg", "NUL", "con.jpg", "COM1.png", "x.jpg.", "a\x00.jpg", "a\u202E.jpg", ""} {
		if v, ok := FileName(n); ok {
			t.Errorf("accepted %q as %q", n, v)
		}
	}
}

func TestSafeNameIsLegalEverywhere(t *testing.T) {
	cases := map[string]string{
		"My Film: Part 2?":     "My Film Part 2",
		"AC/DC \\ <live>":      "AC DC",
		"CON":                  "_CON",
		"nul.txt":              "_nul.txt",
		"Trailing dots...":     "Trailing dots",
		"...":                  "Untitled",
		"":                     "Untitled",
		"\u202Egnp.exe":        "gnp.exe",
		"<script>x</script>Hi": "Hi",
	}
	for in, want := range cases {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
	long := SafeName(strings.Repeat("日本語", 100))
	if len(long) > maxName {
		t.Errorf("too long: %d bytes", len(long))
	}
	if !strings.HasSuffix(long, "語") && !strings.HasSuffix(long, "本") && !strings.HasSuffix(long, "日") {
		t.Errorf("cut inside a character: %q", long)
	}
}

func errorsOf(ps []Problem) []Problem {
	var out []Problem
	for _, p := range ps {
		if p.Severity == SevError {
			out = append(out, p)
		}
	}
	return out
}

func TestParseJSON(t *testing.T) {
	d, ps := ParseJSON("Movies/My Film (2001) [item-123].json", []byte(`{
  "wittyMetadata": 1,
  "target": "item",
  "id": 123,
  "path": "/media/My Film (2001).mkv",
  "title": "My Film",
  "year": 2001,
  "overview": "Two\nlines",
  "genres": ["Drama", "drama", "Comedy"],
  "rating": 7.5,
  "poster": "https://example.test/p.jpg",
  "backdrop": "wide.jpg",
  "locked": ["title"],
  "provider": "tmdb",
  "providerId": "12"
}`))
	if len(ps) != 0 {
		t.Fatalf("problems: %v", ps)
	}
	if d.Target != TargetItem || d.ID != 123 || d.Fields["title"] != "My Film" || d.Fields["year"] != "2001" || d.Fields["genres"] != "Drama|Comedy" || d.Fields["rating"] != "7.5" || d.Fields["overview"] != "Two\nlines" {
		t.Errorf("read %+v", d)
	}
	if d.Art["poster"].URL == "" || d.Art["backdrop"].File != "wide.jpg" {
		t.Errorf("art %+v", d.Art)
	}
	if d.Provider != "tmdb" || d.ProviderID != "12" || !d.HasLocked || d.Locked[0] != "title" {
		t.Errorf("controls %+v", d)
	}
	if d.Lines["title"] != 6 {
		t.Errorf("title is on line %d", d.Lines["title"])
	}
}

// A hostile file comes out as plain text or doesn't come out at all, and says where.
func TestParseJSONInjections(t *testing.T) {
	d, ps := ParseJSON("a [item-1].json", []byte(`{
  "title": "<script>alert(1)</script>Film",
  "overview": "<img src=x onerror=alert(1)>Plot\u202E",
  "tagline": "Na\u0000me",
  "genres": ["<b onclick=x>Drama</b>"]
}`))
	if len(errorsOf(ps)) != 0 {
		t.Fatalf("problems: %v", ps)
	}
	if d.Fields["title"] != "Film" || d.Fields["overview"] != "Plot" || d.Fields["tagline"] != "Name" || d.Fields["genres"] != "Drama" {
		t.Errorf("read %+v", d.Fields)
	}
	warned := 0
	for _, p := range ps {
		if p.Severity == SevWarning && p.Line > 0 && strings.Contains(p.Msg, "taken out") {
			warned++
		}
	}
	if warned != 4 {
		t.Errorf("want a warning for each, with its line: %v", ps)
	}
	for _, bad := range []string{`"javascript:alert(1)"`, `"data:image/svg+xml,<svg>"`, `"file:///etc/passwd"`, `"../../etc/passwd"`, `"..\\..\\boot.ini"`, `"/etc/passwd"`, `"C:\\Windows\\win.ini"`, `"\\\\server\\share\\x.jpg"`, `"https://user:pw@x.test/a.jpg"`, `"sub/x.jpg"`} {
		_, ps := ParseJSON("a [item-1].json", []byte(`{
  "poster": `+bad+`
}`))
		errs := errorsOf(ps)
		if len(errs) != 1 || errs[0].Line != 2 || errs[0].Field != "poster" {
			t.Errorf("poster %s: %v", bad, ps)
		}
	}
}

func TestParseJSONErrorsNameTheLine(t *testing.T) {
	cases := []struct {
		name, body string
		line       int
		has        string
	}{
		{"missing comma", "{\n  \"title\": \"A\"\n  \"year\": 2001\n}", 3, "comma"},
		{"trailing comma", "{\n  \"title\": \"A\",\n}", 3, "comma"},
		{"cut short", "{\n  \"title\": \"A\",\n  \"year\": 20", 3, "ends too soon"},
		{"no quotes", "{\n  \"title\": A\n}", 2, "quotes"},
		{"year as text", "{\n  \"title\": \"A\",\n  \"year\": \"soon\"\n}", 3, "a year"},
		{"year out of range", "{\n  \"year\": 99999\n}", 2, "a year"},
		{"rating out of range", "{\n\n  \"rating\": 11\n}", 3, "0 to 10"},
		{"list wrong", "{\n  \"genres\": \"Drama\"\n}", 2, "square brackets"},
		{"empty title", "{\n  \"title\": \"<b></b>\"\n}", 2, "empty"},
		{"twice", "{\n  \"title\": \"A\",\n  \"title\": \"B\"\n}", 3, "twice"},
		{"not an object", "[1,2]", 1, "{"},
		{"nothing", "   ", 1, "empty"},
		{"newer", "{\n  \"wittyMetadata\": 99\n}", 2, "newer"},
		{"wrong id", "{\n  \"id\": 9\n}", 2, "says"},
		{"bad bytes", "{\n  \"title\": \"a\xffb\"\n}", 2, "UTF-8"},
	}
	for _, c := range cases {
		_, ps := ParseJSON("a [item-1].json", []byte(c.body))
		errs := errorsOf(ps)
		if len(errs) == 0 || errs[0].Line != c.line || !strings.Contains(errs[0].Msg, c.has) {
			t.Errorf("%s: got %v, want line %d saying %q", c.name, ps, c.line, c.has)
		}
	}
	// A typo is a warning, with what they probably meant.
	_, ps := ParseJSON("a [item-1].json", []byte("{\n  \"titel\": \"A\"\n}"))
	if len(ps) != 1 || ps[0].Severity != SevWarning || ps[0].Line != 2 || !strings.Contains(ps[0].Msg, `"title"`) {
		t.Errorf("typo: %v", ps)
	}
	// Too big, and too deep, are refused before anything parses them.
	_, ps = ParseJSON("a [item-1].json", []byte(`{"overview":"`+strings.Repeat("a", MaxJSON)+`"}`))
	if len(errorsOf(ps)) != 1 || !strings.Contains(ps[0].Msg, "too big") {
		t.Errorf("huge: %v", ps)
	}
	_, ps = ParseJSON("a [item-1].json", []byte(strings.Repeat("[", 5000)))
	if len(errorsOf(ps)) != 1 || !strings.Contains(ps[0].Msg, "levels") {
		t.Errorf("deep: %v", ps)
	}
	// A byte order mark from Notepad is fine; UTF-16 gets told what to do.
	if _, ps = ParseJSON("a [item-1].json", []byte("\xEF\xBB\xBF{\"title\":\"A\"}")); len(ps) != 0 {
		t.Errorf("BOM: %v", ps)
	}
	if _, ps = ParseJSON("a [item-1].json", []byte("\xFF\xFE{\x00")); len(ps) != 1 || !strings.Contains(ps[0].Msg, "UTF-16") {
		t.Errorf("UTF-16: %v", ps)
	}
}

func TestParseNFO(t *testing.T) {
	d, ps := ParseNFO("a [item-1].nfo", []byte(`<?xml version="1.0"?>
<movie>
  <title>My &amp; Film</title>
  <year>2001</year>
  <plot>&lt;script&gt;alert(1)&lt;/script&gt;A plot</plot>
  <genre>Drama</genre><genre>drama</genre>
  <rating>7.5</rating>
  <thumb>javascript:alert(1)</thumb>
  <thumb>https://example.test/p.jpg</thumb>
  <fanart><thumb>file:///etc/passwd</thumb></fanart>
  <uniqueid type="tmdb">12</uniqueid>
</movie>
http://some.site/trailing/url
`))
	if len(errorsOf(ps)) != 0 {
		t.Fatalf("problems: %v", ps)
	}
	if d.Fields["title"] != "My & Film" || d.Fields["overview"] != "A plot" || d.Fields["genres"] != "Drama" || d.Fields["year"] != "2001" || d.Art["poster"].URL != "https://example.test/p.jpg" || d.Provider != "tmdb" {
		t.Errorf("read %+v %+v", d.Fields, d.Art)
	}
	if _, has := d.Art["backdrop"]; has {
		t.Errorf("a file address got through: %+v", d.Art)
	}
	cases := []struct {
		name, body string
		line       int
		has        string
	}{
		{"xxe", "<?xml version=\"1.0\"?>\n<!DOCTYPE movie [<!ENTITY x SYSTEM \"file:///etc/passwd\">]>\n<movie><title>&x;</title></movie>", 2, "DOCTYPE"},
		{"laughs", "<!DOCTYPE lolz [<!ENTITY a \"aaaa\">]><movie/>", 1, "DOCTYPE"},
		{"mismatched", "<movie>\n<title>A</titel>\n</movie>", 2, "closing tag"},
		{"bare ampersand", "<movie>\n<title>A & B</title>\n</movie>", 2, "&amp;"},
		{"cut", "<movie>\n<title>A</title>\n", 0, "closing tag"},
		{"wrong root", "<album><title>A</title></album>", 1, "<movie>"},
	}
	for _, c := range cases {
		_, ps := ParseNFO("a [item-1].nfo", []byte(c.body))
		errs := errorsOf(ps)
		if len(errs) == 0 || (c.line > 0 && errs[0].Line != c.line) || !strings.Contains(errs[0].Msg, c.has) {
			t.Errorf("%s: got %+v, want line %d saying %q", c.name, ps, c.line, c.has)
		}
	}
	if _, ps := ParseNFO("a.nfo", []byte("<movie>"+strings.Repeat("<a>", 500)+strings.Repeat("</a>", 500)+"</movie>")); len(errorsOf(ps)) == 0 {
		t.Error("nesting 500 levels deep was accepted")
	}
	if _, ps := ParseNFO("a.nfo", []byte(strings.Repeat(" ", MaxNFO+1))); len(errorsOf(ps)) == 0 {
		t.Error("a huge file was accepted")
	}
}

var jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 40)...)

func TestPictures(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if _, ext, err := ReadPicture(write("ok.jpg", jpegBytes)); err != nil || ext != ".jpg" {
		t.Errorf("a real JPEG: %q %v", ext, err)
	}
	if _, ext, err := ReadPicture(write("named-wrong.png", jpegBytes)); err != nil || ext != ".jpg" {
		t.Errorf("a picture is what its bytes say: %q %v", ext, err)
	}
	for name, b := range map[string][]byte{
		"fake.jpg":    []byte("<html><script>alert(1)</script></html>"),
		"vector.svg":  []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
		"svgfake.png": []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`),
		"program.jpg": []byte("MZ\x90\x00"),
	} {
		if _, _, err := ReadPicture(write(name, b)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	big := make([]byte, MaxPicture+1)
	copy(big, jpegBytes)
	if _, _, err := ReadPicture(write("big.jpg", big)); err == nil || !strings.Contains(err.Error(), "too big") {
		t.Errorf("a huge picture: %v", err)
	}
	secret := write("secret.jpg", jpegBytes)
	link := filepath.Join(dir, "link.jpg")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if _, _, err := ReadPicture(link); err == nil {
		t.Error("a link was followed")
	}
}

func TestCheckDir(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("README.txt", "ignored")
	write("Thumbs.db", "ignored")
	write("Movies/Good [item-1].json", `{"title":"Good"}`)
	write("Movies/Broken [item-2].json", "{\n  \"title\": \"Broken\"\n  \"year\": 2001\n}")
	write("Movies/No id.json", `{"title":"Nobody's"}`)
	write("Movies/Dup [item-1].json", `{"title":"Twin"}`)
	write("Movies/Art [item-3].json", "{\n  \"poster\": \"missing.jpg\"\n}")
	write("Movies/fake-poster.jpg", "not a picture")
	write("Movies/note.exe", "MZ")
	write("Movies/vector.svg", "<svg/>")
	write("Shows/A Show [show-5]/show.json", `{"title":"A Show"}`)
	write("Shows/A Show [show-5]/E1 [item-6].nfo", "<episodedetails><title>One</title></episodedetails>")
	ps := CheckDir(root)
	saw := func(file string, line int, has string) {
		t.Helper()
		for _, p := range ps {
			if p.File == file && p.Line == line && strings.Contains(p.Msg, has) {
				return
			}
		}
		t.Errorf("no problem for %s:%d about %q in:\n%v", file, line, has, ps)
	}
	saw("Movies/Broken [item-2].json", 3, "comma")
	saw("Movies/No id.json", 0, "which title")
	saw("Movies/fake-poster.jpg", 0, "isn't a picture")
	saw("Movies/note.exe", 0, "ignored")
	saw("Movies/vector.svg", 0, "SVG")
	saw("Movies/Art [item-3].json", 2, "missing.jpg")
	found := false
	for _, p := range ps {
		if strings.Contains(p.Msg, "same title") {
			found = true
		}
		if strings.Contains(p.File, "README") || strings.Contains(p.File, "Thumbs") || strings.Contains(p.File, "show.json") {
			t.Errorf("a good file was complained about: %v", p)
		}
	}
	if !found {
		t.Errorf("two files for one title weren't noticed: %v", ps)
	}
	// An error always comes before a note.
	if ps[0].Severity != SevError {
		t.Errorf("order: %v", ps)
	}
}

func TestLinksAreNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret [item-9].json")
	if err := os.WriteFile(secret, []byte(`{"title":"From outside"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "linked [item-9].json")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("no symlinks here: %v", err)
		}
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-folder")); err != nil {
		t.Fatal(err)
	}
	var seen []string
	ps := Walk(root, func(e Entry) { seen = append(seen, e.Rel) })
	if len(seen) != 0 {
		t.Errorf("went through a link: %v", seen)
	}
	if len(ps) != 2 {
		t.Errorf("want both links reported: %v", ps)
	}
	if err := WriteAtomic(root, "linked-folder/new.json", []byte("{}")); err == nil {
		t.Error("wrote through a linked folder")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.json")); err == nil {
		t.Error("a file appeared outside")
	}
	// Writing over a planted link replaces the link and leaves its target alone.
	if err := WriteAtomic(root, "linked [item-9].json", []byte(`{"title":"Mine"}`)); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(secret)
	if !strings.Contains(string(b), "From outside") {
		t.Errorf("wrote through a link: %s", b)
	}
}

func TestDroppedPicture(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"My Film [item-1]-poster.jpg", "backdrop.png"} {
		if err := os.WriteFile(filepath.Join(dir, n), jpegBytes, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := DroppedPicture(dir, "My Film [item-1].json", "poster"); got != "My Film [item-1]-poster.jpg" {
		t.Errorf("film's poster: %q", got)
	}
	if got := DroppedPicture(dir, "show.json", "backdrop"); got != "backdrop.png" {
		t.Errorf("show's backdrop: %q", got)
	}
	if got := DroppedPicture(dir, "My Film [item-1].json", "backdrop"); got != "" {
		t.Errorf("another's picture: %q", got)
	}
}

// Whatever comes in, parsing ends, and what comes out holds no markup.
func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"title":"<script>x</script>"}`))
	f.Add([]byte(`<movie><title>a</title></movie>`))
	f.Add([]byte(`<!DOCTYPE a [<!ENTITY x "y">]><movie/>`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<16 {
			return
		}
		start := time.Now()
		defer func() {
			if took := time.Since(start); took > 2*time.Second {
				t.Errorf("%d bytes took %v", len(b), took)
			}
		}()
		for _, parse := range []func(string, []byte) (*Doc, []Problem){ParseJSON, ParseNFO} {
			d, _ := parse("a [item-1].json", b)
			for k, v := range d.Fields {
				if strings.Contains(strings.ToLower(v), "<script") {
					t.Errorf("%s kept markup: %q", k, v)
				}
			}
		}
	})
}
