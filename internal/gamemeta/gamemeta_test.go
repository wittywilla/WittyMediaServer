package gamemeta

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Everything here talks to local fakes. No game names, data or art from anywhere real.

func TestCleanName(t *testing.T) {
	for _, c := range []struct{ in, title, tags string }{
		{"Mega Game (USA) (Rev 1) [!]", "Mega Game", "USA, Rev 1"},
		{"Mega Game, The (Europe)", "The Mega Game", "Europe"},
		{"Legend of Stuff, The - Part Two (Japan)", "The Legend of Stuff - Part Two", "Japan"},
		{"plain_name", "plain name", ""},
		{"(USA)", "(USA)", "USA"},
	} {
		title, tags := CleanName(c.in)
		if title != c.title || tags != c.tags {
			t.Errorf("CleanName(%q) = %q, %q", c.in, title, tags)
		}
	}
}

func TestSearchTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Mega Game Disc 1":                 "Mega Game",
		"mega-game-disc1of3-u-scus-94163-": "mega game u",
		"Mega Game (USA)":                  "Mega Game (USA)",
		"Disc":                             "Disc",
	} {
		if got := SearchTitle(in); got != want {
			t.Errorf("SearchTitle(%q) = %q, want %q", in, got, want)
		}
	}
	if s := SerialInName("mega-game-disc1of3-u-scus-94163-.7z"); s != "SCUS-94163" {
		t.Errorf("SerialInName = %q", s)
	}
	if s := SerialInName("Mega Game (USA).7z"); s != "" {
		t.Errorf("SerialInName found %q in a plain name", s)
	}
	if n := DiscNumber("Mega Game Disc 2"); n != 2 {
		t.Errorf("DiscNumber = %d", n)
	}
}

func TestNameScore(t *testing.T) {
	for _, c := range []struct {
		a, b     string
		min, max int
	}{
		{"The Mega Game", "Mega Game, The (USA)", 100, 100},
		{"Mega Game II", "Mega Game 2", 100, 100},
		{"Mega & Game", "Mega and Game", 100, 100},
		{"Pokémon Quest", "Pokemon Quest", 100, 100},
		{"Mega Game 2", "Mega Game 3", 0, 50},
		{"Mega Game", "Mega Game 2", 0, 50},
		{"Mega Game", "Mega Game - The Quest", 70, 79},
		{"Mega Game", "Totally Different Thing", 0, 30},
		{"", "Anything", 0, 0},
	} {
		if s := NameScore(c.a, c.b); s < c.min || s > c.max {
			t.Errorf("NameScore(%q, %q) = %d, want %d..%d", c.a, c.b, s, c.min, c.max)
		}
	}
	// A subtitle left out isn't enough to use without asking: it may be the sequel.
	if s := NameScore("Mega Game", "Mega Game - The Quest"); s >= AcceptScore {
		t.Errorf("a subtitle match scored %d", s)
	}
}

func TestAges(t *testing.T) {
	for _, c := range []struct {
		sys, val string
		want     int // -1: unknown
	}{
		{"ESRB", "E", 0}, {"ESRB", "E10+", 10}, {"ESRB", "T", 13}, {"ESRB", "M", 17}, {"ESRB", "AO", 18}, {"ESRB", "RP", -1},
		{"ESRB", "NOT RATED", -1},
		{"PEGI", "3", 3}, {"PEGI", "PEGI 18", 18}, {"PEGI", "12+", 12},
		{"CERO", "A", 0}, {"CERO", "B", 12}, {"CERO", "Z", 18},
		{"USK", "6", 6}, {"USK", "USK 16", 16},
		{"ACB", "MA15+", 15}, {"ACB", "R18+", 18},
		{"CLASS_IND", "L", 0}, {"Class_Ind", "14", 14},
		{"GRAC", "ALL", 0}, {"GRAC", "Testing", -1},
		{"nonsense", "12", -1},
	} {
		got := AgeFor(c.sys, c.val)
		if (got == nil) != (c.want < 0) || (got != nil && *got != c.want) {
			t.Errorf("AgeFor(%s, %s) = %v, want %d", c.sys, c.val, got, c.want)
		}
	}
	if IsSystem("PG") || !IsSystem("esrb") {
		t.Error("IsSystem")
	}
	a, _ := NewAge("ESRB", "T")
	b, _ := NewAge("PEGI", "16")
	c, _ := NewAge("CERO", "B")
	if s := Strictest([]AgeRating{a, b, c}); s == nil || s.System != "PEGI" {
		t.Errorf("Strictest = %+v", s)
	}
	if Strictest(nil) != nil {
		t.Error("Strictest of nothing")
	}
}

func TestText(t *testing.T) {
	for in, want := range map[string]string{
		`<p>A <b>great</b> game &amp; more</p>`: "A great game & more",
		"<script>alert(1)</script>Plain":        "alert(1)Plain",
		"one<br>two":                            "one\ntwo",
		"bell\x07 and zero​ width":              "bell and zero width",
		"  lots   of\t\tspace  ":                "lots of space",
		`<img src=x onerror=alert(1)>text`:      "text",
		`&lt;script&gt;`:                        "<script>", // React escapes it when shown
	} {
		if got := Text(in); got != want {
			t.Errorf("Text(%q) = %q, want %q", in, got, want)
		}
	}
	if n := len([]rune(Text(strings.Repeat("x", 50000)))); n > maxText+1 {
		t.Errorf("Text kept %d characters", n)
	}
	for in, want := range map[string]string{
		"https://example.com/a.png": "https://example.com/a.png",
		"http://example.com/a.png":  "http://example.com/a.png",
		"javascript:alert(1)":       "",
		"file:///etc/passwd":        "",
		"https://user:pw@host/x":    "",
		"/relative":                 "",
	} {
		if got := WebAddress(in); got != want {
			t.Errorf("WebAddress(%q) = %q", in, got)
		}
	}
}

// ---------------------------------------------------------------- merging

func TestMerge(t *testing.T) {
	esrb, _ := NewAge("ESRB", "T")
	pegi, _ := NewAge("PEGI", "18")
	first := &Info{ID: "1", Title: "Mega Game", Method: MethodChecksum, Confidence: 100, Developer: "First", Poster: "https://a/p.png",
		Scores: []Score{{Source: "A", Kind: "critic", Value: 90}}, Ages: []AgeRating{esrb}}
	second := &Info{ID: "2", Title: "Other", Method: MethodName, Confidence: 85, Developer: "Second", Publisher: "Second Pub", Description: "Words", Poster: "https://b/p.png",
		Scores: []Score{{Source: "A", Kind: "critic", Value: 10}, {Source: "B", Kind: "user", Value: 50}}, Ages: []AgeRating{pegi}, Genres: []string{"Action"}, Players: 2}
	unsure := &Info{ID: "3", Method: MethodName, Confidence: 40, Franchise: "Not this one", Description: "Wrong"}
	m := Merge([]Answer{{"a", first}, {"b", second}, {"c", unsure}})
	if m.Title != "Mega Game" || m.Developer != "First" || m.Publisher != "Second Pub" || m.Description != "Words" || m.Franchise != "" {
		t.Errorf("merged %+v", m)
	}
	if m.MatchSource != "a" || m.MatchMethod != MethodChecksum || m.Confidence != 100 {
		t.Errorf("match %+v", m)
	}
	if len(m.Scores) != 2 || len(m.Ages) != 2 || len(m.Posters) != 2 || m.Posters[0] != "https://a/p.png" {
		t.Errorf("collected %+v", m)
	}
	if r := m.Rating(); r == nil || *r != 9 {
		t.Errorf("rating %v: critics count first", r)
	}
	if c := m.ContentRating(); c != "PEGI 18" {
		t.Errorf("content rating %q: the strictest counts", c)
	}
	if len(m.Used) != 2 {
		t.Errorf("used %v", m.Used)
	}
	// A manual choice comes first, whatever the order.
	chosen := &Info{ID: "9", Title: "Chosen", Method: MethodManual, Confidence: 100}
	if m := Merge([]Answer{{"a", first}, {"b", chosen}}); m.Title != "Chosen" || m.MatchSource != "b" {
		t.Errorf("manual: %+v", m)
	}
	if m := Merge(nil); m.Title != "" || !m.Gaps() || m.Rating() != nil || m.ContentRating() != "" {
		t.Errorf("empty: %+v", m)
	}
}

// ---------------------------------------------------------------- RDB

func packStr(s string) []byte {
	b := []byte(s)
	switch {
	case len(b) < 32:
		return append([]byte{0xa0 | byte(len(b))}, b...)
	default:
		return append([]byte{0xd9, byte(len(b))}, b...)
	}
}

func packBin(b []byte) []byte { return append([]byte{0xc4, byte(len(b))}, b...) }

func packRecord(fields map[string]any) []byte {
	out := []byte{0xde, byte(len(fields) >> 8), byte(len(fields))}
	for k, v := range fields {
		out = append(out, packStr(k)...)
		switch x := v.(type) {
		case string:
			out = append(out, packStr(x)...)
		case int:
			out = append(out, 0xcd, byte(x>>8), byte(x))
		case []byte:
			out = append(out, packBin(x)...)
		}
	}
	return out
}

func crcBytes(c uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, c)
	return b
}

func makeRDB(records ...map[string]any) []byte {
	var body bytes.Buffer
	for _, r := range records {
		body.Write(packRecord(r))
	}
	body.WriteByte(0xc0)
	var out bytes.Buffer
	out.WriteString("RARCHDB\x00")
	offset := make([]byte, 8)
	binary.BigEndian.PutUint64(offset, uint64(16+body.Len()))
	out.Write(offset)
	out.Write(body.Bytes())
	out.Write(append([]byte{0x81}, append(packStr("count"), byte(len(records)))...))
	return out.Bytes()
}

func TestParseRDB(t *testing.T) {
	b := makeRDB(
		map[string]any{"name": "Mega Game (USA)", "crc": crcBytes(0xDEADBEEF), "serial": []byte("SLUS-00001\r"), "users": 2, "releaseyear": 1994, "releasemonth": 3,
			"developer": "A Studio", "genre": "Action", "esrb_rating": "T", "edge_rating": 8, "famitsu_rating": 36, "unknown_field": "skip me"},
		map[string]any{"crc": crcBytes(0x11111111), "genre": "Sports"}, // merged in with no name
	)
	var got []*rdbRecord
	if err := parseRDB(b, func(r *rdbRecord) { c := *r; got = append(got, &c) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d records", len(got))
	}
	r := got[0]
	if r.Name != "Mega Game (USA)" || !r.HasCRC || r.CRC != 0xDEADBEEF || r.Serial != "SLUS-00001" || r.Users != 2 || r.ReleaseYear != 1994 || r.ESRB != "T" || r.Edge != 8 || r.Famitsu != 36 {
		t.Errorf("record %+v", r)
	}
}

// parseRDB takes what comes off the internet: nothing in a file may crash it, or make
// it allocate according to a length the file claims.
func TestParseRDBHostile(t *testing.T) {
	for name, b := range map[string][]byte{
		"empty":       {},
		"short":       []byte("RARCHDB\x00"),
		"wrong magic": append([]byte("NOTADATA"), make([]byte, 16)...),
		"offset past": append([]byte("RARCHDB\x00\xff\xff\xff\xff\xff\xff\xff\xff"), 0x80),
		"offset low":  []byte("RARCHDB\x00\x00\x00\x00\x00\x00\x00\x00\x01"),
	} {
		if err := parseRDB(b, func(*rdbRecord) {}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	good := makeRDB(map[string]any{"name": "Mega Game", "crc": crcBytes(1), "users": 2})
	// Cut anywhere, and with any byte changed: an error at most.
	for i := range good {
		_ = parseRDB(good[:i], func(*rdbRecord) {})
		mod := append([]byte{}, good...)
		mod[i] ^= 0xff
		_ = parseRDB(mod, func(*rdbRecord) {})
	}
	// Claims of enormous sizes.
	head := func(body ...byte) []byte {
		out := []byte("RARCHDB\x00")
		o := make([]byte, 8)
		binary.BigEndian.PutUint64(o, uint64(16+len(body)))
		return append(append(out, o...), body...)
	}
	for _, b := range [][]byte{
		head(0xdf, 0xff, 0xff, 0xff, 0xff),       // a map of 4 billion
		head(0xdd, 0xff, 0xff, 0xff, 0xff),       // an array of 4 billion
		head(0xdb, 0xff, 0xff, 0xff, 0xff),       // a string of 4 gigabytes
		head(0xc6, 0xff, 0xff, 0xff, 0xff),       // the same in binary
		head(bytes.Repeat([]byte{0x91}, 200)...), // deep nesting
	} {
		_ = parseRDB(b, func(*rdbRecord) {})
	}
}

func rdbServer(t *testing.T, hits *atomic.Int32, files map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		name, _ := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), "/"))
		b, ok := files[strings.TrimSuffix(name, ".rdb")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(304)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func snesFile() map[string][]byte {
	return map[string][]byte{"Nintendo - Super Nintendo Entertainment System": makeRDB(
		map[string]any{"name": "Mega Game (Japan)", "crc": crcBytes(0xAAAA0001), "serial": []byte("SHVC-XX")},
		map[string]any{"name": "Mega Game (USA)", "crc": crcBytes(0xAAAA0002), "developer": "A Studio", "publisher": "A Publisher", "genre": "Action", "users": 2, "releaseyear": 1994, "esrb_rating": "T"},
		map[string]any{"name": "Mega Game (Europe)", "crc": crcBytes(0xAAAA0003)},
		map[string]any{"name": "Mega Game 2 (USA)", "crc": crcBytes(0xBBBB0001)},
		map[string]any{"name": "Other Game & Friends (USA)", "crc": crcBytes(0xCCCC0001), "edge_rating": 9},
		map[string]any{"name": "Disc Game (USA) (Disc 1)", "serial": []byte("SLUS-00123"), "crc": crcBytes(0xDDDD0001)},
		map[string]any{"name": "Quest of Long Names Part Two (USA)", "crc": crcBytes(0xEEEE0001)},
	)}
}

func TestLibretroMatching(t *testing.T) {
	var hits atomic.Int32
	srv := rdbServer(t, &hits, snesFile())
	src := &Libretro{HTTP: srv.Client(), Dir: t.TempDir(), Base: srv.URL}
	ctx := context.Background()
	check := func(q Query, wantName, method string, conf int) {
		t.Helper()
		in, err := src.Lookup(ctx, q)
		if wantName == "" {
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("%+v: %v, %+v", q, err, in)
			}
			return
		}
		if err != nil {
			t.Fatalf("%+v: %v", q, err)
		}
		if in.Title != wantName || in.Method != method || in.Confidence != conf {
			t.Errorf("%+v: %s, %s, %d", q, in.Title, in.Method, in.Confidence)
		}
	}
	q := func(title, tags string, crcs ...uint32) Query {
		return Query{Console: "snes", Title: title, Tags: tags, CRCs: crcs}
	}
	// The checksum beats the name.
	check(q("Totally Misnamed", "", 0xBBBB0001), "Mega Game 2 (USA)", MethodChecksum, 100)
	check(q("Mega Game", "", 1, 0xAAAA0003), "Mega Game (Europe)", MethodChecksum, 100)
	// By name: the file's own region if it says, and otherwise the most useful.
	check(q("Mega Game", ""), "Mega Game (USA)", MethodName, 92)
	check(q("Mega Game", "Japan"), "Mega Game (Japan)", MethodName, 92)
	check(q("Other Game and Friends", ""), "Other Game & Friends (USA)", MethodName, 92)
	// Close, but not the same word for word.
	in, err := src.Lookup(ctx, q("Quest of Long Name Part Two", ""))
	if err != nil || in.Confidence < 72 || in.Confidence > 91 {
		t.Errorf("close match: %v, %+v", err, in)
	}
	// Numbers matter, and strangers aren't guessed at.
	check(q("Mega Game 9", ""), "", "", 0)
	check(q("Nothing Like Anything", ""), "", "", 0)
	// A serial, however it's written; a disc in a set.
	check(Query{Console: "snes", Title: "x", Serial: "slus_001.23"}, "Disc Game (USA) (Disc 1)", MethodSerial, 98)
	// A record with no facts borrows from its other releases.
	in, err = src.Lookup(ctx, q("x", "", 0xAAAA0003))
	if err != nil || in.Developer != "A Studio" || in.Publisher != "A Publisher" || in.Players != 2 || in.Released != "1994" || len(in.Ages) != 1 {
		t.Errorf("gaps: %v, %+v", err, in)
	}
	// A console nobody has a list for.
	if _, err := src.Lookup(ctx, Query{Console: "pc", Title: "Mega Game"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("pc: %v", err)
	}
	// Choosing by hand.
	in, err = src.Fetch(ctx, "snes", "Mega Game (Europe)")
	if err != nil || in.Method != MethodManual {
		t.Errorf("Fetch: %v, %+v", err, in)
	}
	list, err := src.Search(ctx, "snes", "mega game")
	if err != nil || len(list) < 3 || list[0].Score != 100 {
		t.Errorf("Search: %v, %+v", err, list)
	}
}

func TestLibretroDownloadsOnce(t *testing.T) {
	var hits atomic.Int32
	dir := t.TempDir()
	srv := rdbServer(t, &hits, snesFile())
	first := &Libretro{HTTP: srv.Client(), Dir: dir, Base: srv.URL}
	ctx := context.Background()
	if err := first.Prepare(ctx, "snes", nil); err != nil {
		t.Fatal(err)
	}
	asked := hits.Load()
	if asked == 0 {
		t.Fatal("nothing downloaded")
	}
	// Lookups cost nothing further, and neither does a restart.
	if _, err := first.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"}); err != nil {
		t.Fatal(err)
	}
	second := &Libretro{HTTP: srv.Client(), Dir: dir, Base: srv.URL}
	if _, err := second.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != asked {
		t.Errorf("%d more requests", hits.Load()-asked)
	}
	// An old list is checked, and a 304 keeps it. The host being down keeps it too.
	p := filepath.Join(dir, "Nintendo - Super Nintendo Entertainment System.rdb")
	old := time.Now().Add(-2 * rdbFresh)
	os.Chtimes(p, old, old)
	third := &Libretro{HTTP: srv.Client(), Dir: dir, Base: srv.URL}
	if _, err := third.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"}); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); time.Since(st.ModTime()) > time.Hour {
		t.Error("the check didn't count as fresh")
	}
	os.Chtimes(p, old, old)
	srv.Close()
	fourth := &Libretro{HTTP: srv.Client(), Dir: dir, Base: srv.URL}
	if _, err := fourth.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"}); err != nil {
		t.Errorf("with the host down: %v", err)
	}
}

func TestLibretroRefusesBadFile(t *testing.T) {
	var hits atomic.Int32
	srv := rdbServer(t, &hits, map[string][]byte{"Nintendo - Super Nintendo Entertainment System": []byte("<html>nope</html>")})
	dir := t.TempDir()
	src := &Libretro{HTTP: srv.Client(), Dir: dir, Base: srv.URL}
	if _, err := src.Lookup(context.Background(), Query{Console: "snes", Title: "Mega Game"}); err == nil {
		t.Error("no error")
	}
	if _, err := os.Stat(filepath.Join(dir, "Nintendo - Super Nintendo Entertainment System.rdb")); err == nil {
		t.Error("kept it")
	}
}

func TestLibretroArt(t *testing.T) {
	var hits atomic.Int32
	srv := rdbServer(t, &hits, map[string][]byte{"Nintendo - Super Nintendo Entertainment System": makeRDB(
		map[string]any{"name": `Odd: Name & "More"? (USA)`, "crc": crcBytes(1)},
		map[string]any{"name": `Odd: Name & "More"? (Japan)`, "crc": crcBytes(2)},
	)})
	db := &Libretro{HTTP: srv.Client(), Dir: t.TempDir(), Base: srv.URL}
	art := &LibretroArt{DB: db, Base: "https://pics.test"}
	in, err := art.Lookup(context.Background(), Query{Console: "snes", Title: "x", CRCs: []uint32{1}})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://pics.test/Nintendo%20-%20Super%20Nintendo%20Entertainment%20System/Named_Boxarts/Odd_%20Name%20_%20_More__%20%28USA%29.png"
	if in.Poster != want {
		t.Errorf("poster\n got %s\nwant %s", in.Poster, want)
	}
	if len(in.MorePosters) != 2 || !strings.Contains(in.MorePosters[0], "Japan") {
		t.Errorf("other releases: %v", in.MorePosters)
	}
	if len(in.Art) != 2 || in.Art[1].Kind != "title" || in.Title != "" {
		t.Errorf("%+v", in)
	}
}

// ---------------------------------------------------------------- files

func TestIdentifyCartridge(t *testing.T) {
	dir := t.TempDir()
	rom := bytes.Repeat([]byte("cartridge"), 4000)
	plain := filepath.Join(dir, "plain.gba")
	os.WriteFile(plain, rom, 0o644)
	var x Identifier
	defer x.Close()
	id := x.Identify(plain, "gba")
	if len(id.CRCs) != 1 || id.CRCs[0] != crc32.ChecksumIEEE(rom) || id.Size != int64(len(rom)) {
		t.Errorf("plain: %+v", id)
	}
	// Copiers' headers: both checksums, as lists hold both kinds.
	nes := filepath.Join(dir, "headered.nes")
	headered := append(append([]byte("NES\x1a"), make([]byte, 12)...), rom...)
	os.WriteFile(nes, headered, 0o644)
	id = x.Identify(nes, "nes")
	if len(id.CRCs) != 2 || id.CRCs[0] != crc32.ChecksumIEEE(headered) || id.CRCs[1] != crc32.ChecksumIEEE(rom) {
		t.Errorf("nes: %+v", id)
	}
	smc := filepath.Join(dir, "copier.smc")
	rom2 := bytes.Repeat([]byte("x"), 1024*8)
	withHeader := append(make([]byte, 512), rom2...)
	os.WriteFile(smc, withHeader, 0o644)
	id = x.Identify(smc, "snes")
	if len(id.CRCs) != 2 || id.CRCs[1] != crc32.ChecksumIEEE(rom2) {
		t.Errorf("smc: %+v", id)
	}
	// What can't be read says nothing.
	if id := x.Identify(filepath.Join(dir, "missing.gba"), "gba"); len(id.CRCs) != 0 {
		t.Errorf("missing: %+v", id)
	}
}

func TestIdentifyZipReadsOnlyHeaders(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "game.zip")
	f, _ := os.Create(p)
	z := zip.NewWriter(f)
	rom := []byte("the cartridge itself, several words long")
	w, _ := z.Create("readme.txt")
	w.Write([]byte("hello"))
	w, _ = z.Create("game.sfc")
	w.Write(rom)
	z.Close()
	f.Close()
	var x Identifier
	defer x.Close()
	id := x.Identify(p, "snes")
	if len(id.CRCs) != 1 || id.CRCs[0] != crc32.ChecksumIEEE(rom) || id.Size != int64(len(rom)) {
		t.Errorf("%+v", id)
	}
}

func TestIdentifySevenZip(t *testing.T) {
	// A file the archive tests made, with one game in it.
	p := filepath.Join("..", "gamearchive", "testdata", "cart.7z")
	if _, err := os.Stat(p); err != nil {
		t.Skip("the archive tests' file isn't here")
	}
	var x Identifier
	defer x.Close()
	if id := x.Identify(p, "snes"); len(id.CRCs) == 0 {
		t.Errorf("no checksum from a .7z: %+v", id)
	}
}

// makeISO builds the least of a disc image: a volume descriptor, a root folder, one file.
func makeISO(sector, offset int, name string, content []byte) []byte {
	const lba = 24
	out := make([]byte, (lba+4)*sector)
	put := func(s int, b []byte) { copy(out[s*sector+offset:], b) }
	pvd := make([]byte, 2048)
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	binary.LittleEndian.PutUint32(pvd[156+2:], lba)
	binary.LittleEndian.PutUint32(pvd[156+10:], 2048)
	put(16, pvd)
	dir := make([]byte, 2048)
	rec := make([]byte, 33+len(name))
	rec[0] = byte(len(rec))
	binary.LittleEndian.PutUint32(rec[2:], lba+1)
	binary.LittleEndian.PutUint32(rec[10:], uint32(len(content)))
	rec[32] = byte(len(name))
	copy(rec[33:], name)
	copy(dir, rec)
	put(lba, dir)
	put(lba+1, content)
	return out
}

func TestDiscSerials(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		os.WriteFile(p, b, 0o644)
		return p
	}
	cnf := []byte("BOOT = cdrom:\\SLUS_005.94;1\r\nTCB = 4\r\n")
	cases := []struct {
		name    string
		console string
		data    []byte
		want    string
	}{
		{"ps1-cooked.iso", "ps1", makeISO(2048, 0, "SYSTEM.CNF;1", cnf), "SLUS-00594"},
		{"ps1-raw.bin", "ps1", makeISO(2352, 24, "SYSTEM.CNF;1", cnf), "SLUS-00594"},
		{"ps2.iso", "ps2", makeISO(2048, 0, "SYSTEM.CNF;1", []byte("BOOT2 = cdrom0:\\SCES_123.45;1\r\n")), "SCES-12345"},
		{"psp.iso", "psp", makeISO(2048, 0, "UMD_DATA.BIN;1", []byte("ULUS-10041|abc|")), "ULUS-10041"},
		{"noboot.iso", "ps1", makeISO(2048, 0, "SYSTEM.CNF;1", []byte("BOOT = cdrom:\\PSX.EXE;1\r\n")), ""},
		{"junk.iso", "ps1", bytes.Repeat([]byte{0xff}, 100000), ""},
		{"short.bin", "ps1", []byte("short"), ""},
	}
	var x Identifier
	defer x.Close()
	for _, c := range cases {
		if id := x.Identify(write(c.name, c.data), c.console); id.Serial != c.want {
			t.Errorf("%s: %q, want %q", c.name, id.Serial, c.want)
		}
	}
	// GameCube: six characters, and the mark that says it's a GameCube disc.
	gc := make([]byte, 4096)
	copy(gc, "GMGE01")
	binary.BigEndian.PutUint32(gc[0x1c:], 0xc2339f3d)
	if id := x.Identify(write("gc.iso", gc), "gc"); id.Serial != "GMGE01" {
		t.Errorf("gc: %q", id.Serial)
	}
	notGC := make([]byte, 4096)
	copy(notGC, "GMGE01")
	if id := x.Identify(write("notgc.iso", notGC), "gc"); id.Serial != "" {
		t.Errorf("not a gc disc: %q", id.Serial)
	}
	// Sega: raw sectors, with the system's name 16 bytes in.
	saturn := make([]byte, 4096)
	copy(saturn[0x10:], "SEGA SEGASATURN ")
	copy(saturn[0x30:], "MK-81022 ")
	if id := x.Identify(write("saturn.bin", saturn), "saturn"); id.Serial != "MK-81022" {
		t.Errorf("saturn: %q", id.Serial)
	}
	cd := make([]byte, 4096)
	copy(cd[0x10:], "SEGADISCSYSTEM")
	copy(cd[0x193:], "T-12345    ")
	if id := x.Identify(write("segacd.bin", cd), "segacd"); id.Serial != "T-12345" {
		t.Errorf("segacd: %q", id.Serial)
	}
	// A cue sheet's first track, staying in its own folder.
	track := write("Disc Game (Track 01).bin", makeISO(2352, 24, "SYSTEM.CNF;1", cnf))
	cue := write("Disc Game.cue", []byte(`FILE "`+filepath.Base(track)+`" BINARY`+"\n  TRACK 01 MODE2/2352\n"))
	if id := x.Identify(cue, "ps1"); id.Serial != "SLUS-00594" {
		t.Errorf("cue: %+v", id)
	}
	escape := write("escape.cue", []byte(`FILE "../../secret.bin" BINARY`+"\n"))
	if id := x.Identify(escape, "ps1"); id.Serial != "" || len(id.CRCs) != 0 {
		t.Errorf("a cue reached outside its folder: %+v", id)
	}
}

func TestSerialForms(t *testing.T) {
	var hits atomic.Int32
	srv := rdbServer(t, &hits, map[string][]byte{"Nintendo - GameCube": makeRDB(
		map[string]any{"name": "Redump Style (USA)", "serial": []byte("DL-DOL-GMGE-USA"), "crc": crcBytes(1)},
		map[string]any{"name": "Set Game (USA) (Disc 2)", "serial": []byte("GSTE01-1"), "crc": crcBytes(2)},
	)})
	src := &Libretro{HTTP: srv.Client(), Dir: t.TempDir(), Base: srv.URL}
	in, err := src.Lookup(context.Background(), Query{Console: "gc", Title: "x", Serial: "GMGE01"})
	if err != nil || in.Title != "Redump Style (USA)" {
		t.Errorf("%v, %+v", err, in)
	}
	in, err = src.Lookup(context.Background(), Query{Console: "gc", Title: "x", Serial: "GSTE01", Tags: "Disc 2"})
	if err != nil || in.Title != "Set Game (USA) (Disc 2)" {
		t.Errorf("%v, %+v", err, in)
	}
}

// ---------------------------------------------------------------- sources with keys

// leak fails if a secret turns up in an error, an answer that's stored, or anything a
// browser would see.
func leak(t *testing.T, what, text string, secrets ...string) {
	t.Helper()
	for _, s := range secrets {
		if s != "" && strings.Contains(text, s) {
			t.Errorf("%s holds a secret: %s", what, text)
		}
	}
}

func TestIGDB(t *testing.T) {
	var tokens, posts atomic.Int32
	var expired atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("client_secret") != "the-secret" {
			http.Error(w, "bad", 403)
			return
		}
		tokens.Add(1)
		fmt.Fprintf(w, `{"access_token":"tok%d","expires_in":5000000}`, tokens.Load())
	})
	mux.HandleFunc("/v4/games", func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		if r.Header.Get("Client-ID") != "the-id" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tok") {
			http.Error(w, "no", 401)
			return
		}
		if expired.Swap(false) {
			http.Error(w, "expired", 401)
			return
		}
		fmt.Fprint(w, `[{"id":7,"name":"Mega Game","url":"https://www.igdb.com/games/mega-game","summary":"<b>Words</b> about it","first_release_date":774662400,
			"aggregated_rating":88.5,"aggregated_rating_count":4,"rating":71,"rating_count":200,"cover":{"image_id":"cov1"},"screenshots":[{"image_id":"s1"},{"image_id":"s2"}],"artworks":[{"image_id":"a1"}],
			"genres":[{"name":"Platform"},{"name":"Adventure"}],"franchises":[{"name":"Mega"}],
			"involved_companies":[{"company":{"name":"Maker"},"developer":true},{"company":{"name":"Seller"},"publisher":true}],
			"multiplayer_modes":[{"offlinemax":4}],
			"age_ratings":[{"organization":{"name":"ESRB"},"rating_category":{"rating":"T"}},{"organization":{"name":"PEGI"},"rating_category":{"rating":"12"}},{"organization":{"name":"CERO"},"rating_category":{"rating":"B"}},{"organization":{"name":"USK"},"rating_category":{"rating":"RP"}}]},
			{"id":8,"name":"Mega Game 2"}]`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	src := &IGDB{ClientID: "the-id", Secret: "the-secret", HTTP: srv.Client(), TokenURL: srv.URL + "/token", Base: srv.URL + "/v4", ImageBase: "https://img.test"}
	ctx := context.Background()
	in, err := src.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"})
	if err != nil {
		t.Fatal(err)
	}
	if in.ID != "7" || in.Description != "Words about it" || in.Released != "1994-07-20" || in.Developer != "Maker" || in.Publisher != "Seller" || in.Players != 4 || in.Franchise != "Mega" {
		t.Errorf("%+v", in)
	}
	if in.Poster != "https://img.test/t_cover_big_2x/cov1.jpg" || in.Backdrop != "https://img.test/t_1080p/a1.jpg" || len(in.Art) != 3 {
		t.Errorf("art %+v", in)
	}
	if len(in.Scores) != 2 || in.Scores[0].Kind != "critic" || in.Scores[0].Value != 88.5 || in.Scores[1].Kind != "user" {
		t.Errorf("scores %+v", in.Scores)
	}
	if len(in.Ages) != 3 {
		t.Errorf("ages %+v: pending ratings aren't ratings", in.Ages)
	}
	if in.Method != MethodName || in.Confidence != 92 {
		t.Errorf("%s %d", in.Method, in.Confidence)
	}
	// One token, reused.
	if _, err := src.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"}); err != nil {
		t.Fatal(err)
	}
	if tokens.Load() != 1 {
		t.Errorf("%d tokens", tokens.Load())
	}
	// A token that stops working is replaced, once.
	expired.Store(true)
	if _, err := src.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"}); err != nil {
		t.Errorf("after expiry: %v", err)
	}
	if tokens.Load() != 2 {
		t.Errorf("%d tokens after expiry", tokens.Load())
	}
	if _, err := src.Lookup(ctx, Query{Console: "snes", Title: "Something Else"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("no match: %v", err)
	}
	if msg, err := src.Test(ctx); err != nil || msg == "" {
		t.Errorf("Test: %q, %v", msg, err)
	}
	bad := &IGDB{ClientID: "the-id", Secret: "wrong", HTTP: srv.Client(), TokenURL: srv.URL + "/token", Base: srv.URL + "/v4"}
	_, err = bad.Test(ctx)
	if !errors.Is(err, ErrKey) {
		t.Errorf("bad key: %v", err)
	}
	if none := (&IGDB{HTTP: srv.Client()}); true {
		if _, err := none.Test(ctx); !errors.Is(err, ErrNotSet) {
			t.Errorf("no key: %v", err)
		}
	}
}

func TestIGDBEscapesNames(t *testing.T) {
	if got := escapeApicalypse(`a"; fields *; \`); got != `a\"; fields *; \\` {
		t.Errorf("%s", got)
	}
}

func TestRAWG(t *testing.T) {
	var last atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		last.Store(r.URL.String())
		if r.URL.Query().Get("key") != "rawg-key" {
			http.Error(w, "no", 401)
			return
		}
		switch r.URL.Path {
		case "/games":
			fmt.Fprint(w, `{"results":[{"id":5,"slug":"mega-game","name":"Mega Game","released":"1994-03-02","background_image":"https://img.test/b.jpg","short_screenshots":[{"image":"https://img.test/b.jpg"},{"image":"https://img.test/s.jpg"}]}]}`)
		case "/games/5":
			fmt.Fprint(w, `{"id":5,"slug":"mega-game","name":"Mega Game","released":"1994-03-02","background_image":"https://img.test/b.jpg","description":"<p>Hello</p>","description_raw":"Hello",
				"metacritic":81,"metacritic_url":"https://mc.test/x","rating":4.0,"rating_top":5,"ratings_count":10,"genres":[{"name":"Action"}],"developers":[{"name":"Maker"}],"publishers":[{"name":"Seller"}],"esrb_rating":{"name":"Everyone 10+"}}`)
		case "/platforms":
			fmt.Fprint(w, `{"count":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	src := &RAWG{Key: "rawg-key", HTTP: srv.Client(), Base: srv.URL}
	ctx := context.Background()
	in, err := src.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"})
	if err != nil {
		t.Fatal(err)
	}
	if in.Description != "Hello" || in.Developer != "Maker" || in.Publisher != "Seller" || in.URL != "https://rawg.io/games/mega-game" || in.Poster != "" || in.Backdrop != "https://img.test/b.jpg" {
		t.Errorf("%+v", in)
	}
	if len(in.Scores) != 2 || in.Scores[0].Source != "Metacritic" || in.Scores[0].Value != 81 || in.Scores[1].Value != 80 {
		t.Errorf("scores %+v", in.Scores)
	}
	if len(in.Ages) != 1 || in.Ages[0].Value != "E10+" {
		t.Errorf("ages %+v", in.Ages)
	}
	if len(in.Art) != 1 || in.Art[0].Ref != "https://img.test/s.jpg" {
		t.Errorf("art %+v", in.Art)
	}
	// The allowance is respected: a spent meter stops requests before they're made.
	var spent int
	src.SetMeter(func(n int) error {
		spent += n
		if spent > 1 {
			return &RateLimited{Until: time.Now().Add(time.Hour), Why: "spent"}
		}
		return nil
	})
	_, err = src.Lookup(ctx, Query{Console: "snes", Title: "Mega Game"})
	var rl *RateLimited
	if !errors.As(err, &rl) {
		t.Errorf("past the allowance: %v", err)
	}
}

// A key in an address must not reach an error, however the request fails.
func TestKeysStayOutOfErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", 500) }))
	defer srv.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	ctx := context.Background()
	for _, base := range []string{srv.URL, closedURL} {
		for name, src := range map[string]Source{
			"rawg": &RAWG{Key: "SECRETKEY", HTTP: srv.Client(), Base: base},
			"tgdb": &TGDB{Key: "SECRETKEY", HTTP: srv.Client(), Base: base},
			"ss":   &ScreenScraper{User: "SECRETUSER", Password: "SECRETPASS", DevUser: "SECRETDEV", DevPass: "SECRETDEVPASS", HTTP: srv.Client(), Base: base},
		} {
			_, err := src.Lookup(ctx, Query{Console: "snes", Title: "Mega Game", CRCs: []uint32{1}})
			if err == nil {
				t.Errorf("%s: no error", name)
				continue
			}
			leak(t, name+" error", err.Error(), "SECRETKEY", "SECRETUSER", "SECRETPASS", "SECRETDEV", "SECRETDEVPASS", "apikey", "devpassword")
			_, err = src.Test(ctx)
			if err != nil {
				leak(t, name+" test error", err.Error(), "SECRETKEY", "SECRETUSER", "SECRETPASS", "SECRETDEV", "SECRETDEVPASS")
			}
		}
	}
}

func TestRateLimitsAreObeyed(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "600")
		w.WriteHeader(429)
	}))
	defer srv.Close()
	src := &RAWG{Key: "k", HTTP: srv.Client(), Base: srv.URL}
	ctx := context.Background()
	_, err := src.Lookup(ctx, Query{Title: "A"})
	var rl *RateLimited
	if !errors.As(err, &rl) || time.Until(rl.Until) < 9*time.Minute {
		t.Fatalf("%v", err)
	}
	// It isn't asked again until then.
	if _, err := src.Lookup(ctx, Query{Title: "B"}); !errors.As(err, &rl) {
		t.Errorf("%v", err)
	}
	if hits.Load() != 1 {
		t.Errorf("%d requests", hits.Load())
	}
}

func TestPacing(t *testing.T) {
	g := newGate(40 * time.Millisecond)
	began := time.Now()
	for i := 0; i < 4; i++ {
		if err := g.wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if d := time.Since(began); d < 110*time.Millisecond {
		t.Errorf("four requests in %v", d)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g.gap = time.Hour
	g.wait(context.Background())
	if err := g.wait(ctx); err == nil {
		t.Error("a cancelled wait carried on")
	}
}

func TestTGDB(t *testing.T) {
	remaining := 3000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != "tg-key" {
			http.Error(w, "no", 403)
			return
		}
		tail := fmt.Sprintf(`"remaining_monthly_allowance":%d,"extra_allowance":0,"allowance_refresh_timer":3600}`, remaining)
		switch r.URL.Path {
		case "/v1/Games/ByGameHash":
			if r.URL.Query().Get("hash") == "0000002a" && r.URL.Query().Get("filter[type]") == "crc" {
				fmt.Fprint(w, `{"code":200,"data":{"games":[{"id":3,"game_title":"Mega Game","release_date":"1994-01-01","overview":"Hello","players":2,"rating":"T - Teen","developers":[1],"publishers":[2],"genres":[3]}]},
				"include":{"boxart":{"base_url":{"original":"https://img.test/"},"data":{"3":[{"type":"boxart","side":"front","filename":"f.jpg"},{"type":"fanart","filename":"fa.jpg"},{"type":"screenshot","filename":"s.jpg"}]}}},`+tail)
				return
			}
			fmt.Fprint(w, `{"code":200,"data":{"games":[]},`+tail)
		case "/v1.1/Games/ByGameName":
			fmt.Fprint(w, `{"code":200,"data":{"games":[{"id":4,"game_title":"Other Name","alternates":["Plain Game"]}]},`+tail)
		case "/v1/Developers":
			fmt.Fprint(w, `{"code":200,"data":{"developers":{"1":{"name":"Maker"}}},`+tail)
		case "/v1/Publishers":
			fmt.Fprint(w, `{"code":200,"data":{"publishers":{"2":{"name":"Seller"}}},`+tail)
		case "/v1/Genres":
			fmt.Fprint(w, `{"code":200,"data":{"genres":{"3":{"name":"Action"}}},`+tail)
		case "/v1/API/Limit":
			fmt.Fprint(w, `{"remaining_monthly_allowance":12,"extra_allowance":3}`)
		}
	}))
	defer srv.Close()
	src := &TGDB{Key: "tg-key", HTTP: srv.Client(), Base: srv.URL}
	ctx := context.Background()
	in, err := src.Lookup(ctx, Query{Console: "snes", Title: "x", CRCs: []uint32{42}})
	if err != nil {
		t.Fatal(err)
	}
	if in.Method != MethodChecksum || in.Developer != "Maker" || in.Publisher != "Seller" || in.Poster != "https://img.test/f.jpg" || in.Backdrop != "https://img.test/fa.jpg" || len(in.Art) != 1 || in.Players != 2 || len(in.Ages) != 1 {
		t.Errorf("%+v", in)
	}
	// Another name it goes by counts.
	in, err = src.Lookup(ctx, Query{Console: "snes", Title: "Plain Game", CRCs: []uint32{1}})
	if err != nil || in.Method != MethodName || in.ID != "4" {
		t.Errorf("%v, %+v", err, in)
	}
	if msg, err := src.Test(ctx); err != nil || !strings.Contains(msg, "15") {
		t.Errorf("Test: %q, %v", msg, err)
	}
	// Running low on the allowance: stop, and leave the rest for the person's own use.
	remaining = tgdbFloor - 1
	if _, err := src.Lookup(ctx, Query{Console: "snes", Title: "Plain Game"}); err != nil {
		t.Errorf("the request that found out: %v", err)
	}
	var rl *RateLimited
	if _, err := src.Lookup(ctx, Query{Console: "snes", Title: "Plain Game"}); !errors.As(err, &rl) {
		t.Errorf("after the floor: %v", err)
	}
}

func TestScreenScraper(t *testing.T) {
	var bad atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("devid") != "dev" || bad.Load() {
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/jeuInfos.php":
			if q.Get("crc") != "0000002A" && q.Get("gameid") == "" {
				w.WriteHeader(404)
				return
			}
			fmt.Fprint(w, `{"response":{"jeu":{"id":"99","systeme":{"id":"4"},"noms":[{"region":"jp","text":"Nihon Name"},{"region":"us","text":"Mega Game"}],"synopsis":[{"langue":"fr","text":"Bonjour"},{"langue":"en","text":"<i>Hello</i>"}],
				"dates":[{"region":"us","text":"1994-03-02"}],"editeur":{"text":"Seller"},"developpeur":{"text":"Maker"},"joueurs":{"text":"1-4"},"note":{"text":"16"},
				"classifications":[{"type":"ESRB","text":"T"},{"type":"SS","text":"nope"}],
				"genres":[{"noms":[{"langue":"en","text":"Action"}]}],
				"medias":[{"type":"box-2D","region":"us","parent":"jeu"},{"type":"ss","parent":"jeu"},{"type":"sstitle","parent":"jeu"},{"type":"fanart","parent":"jeu"}]}}}`)
		case "/jeuRecherche.php":
			fmt.Fprint(w, `{"response":{"jeux":[{"id":"5","noms":[{"region":"us","text":"By Name"}]}]}}`)
		case "/ssuserInfos.php":
			fmt.Fprint(w, `{"response":{"ssuser":{"maxrequestsperday":"20000"}}}`)
		}
	}))
	defer srv.Close()
	src := &ScreenScraper{User: "me", Password: "pw", DevUser: "dev", DevPass: "devpw", HTTP: srv.Client(), Base: srv.URL}
	ctx := context.Background()
	in, err := src.Lookup(ctx, Query{Console: "snes", Title: "whatever", CRCs: []uint32{42}, Size: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if in.Method != MethodChecksum || in.Title != "Mega Game" || in.Description != "Hello" || in.Players != 4 || in.Released != "1994-03-02" {
		t.Errorf("%+v", in)
	}
	if len(in.Scores) != 1 || in.Scores[0].Value != 80 || len(in.Ages) != 1 {
		t.Errorf("%+v %+v", in.Scores, in.Ages)
	}
	// Pictures are references. Nothing that gets stored holds a login.
	stored, _ := json.Marshal(in)
	leak(t, "what's stored", string(stored), "devpw", "pw\"", "devid", "sspassword", "ssid=")
	if in.Poster != "ss:4:99:box-2D(us)" {
		t.Errorf("poster %q", in.Poster)
	}
	u, err := src.PictureURL(in.Poster)
	if err != nil || !strings.Contains(u, "devpassword=devpw") || !strings.Contains(u, "media=box-2D%28us%29") {
		t.Errorf("%q, %v", u, err)
	}
	// By name, when the checksum isn't known.
	in, err = src.Lookup(ctx, Query{Console: "snes", Title: "By Name", CRCs: []uint32{1}})
	if err != nil || in.Method != MethodName {
		t.Errorf("%v, %+v", err, in)
	}
	// Candidates show no pictures, as their addresses would hold logins.
	list, err := src.Search(ctx, "snes", "By Name")
	if err != nil || len(list) != 1 || list[0].Poster != "" {
		t.Errorf("%v, %+v", err, list)
	}
	if msg, err := src.Test(ctx); err != nil || !strings.Contains(msg, "20000") {
		t.Errorf("%q, %v", msg, err)
	}
	bad.Store(true)
	if _, err := src.Test(ctx); !errors.Is(err, ErrKey) {
		t.Errorf("%v", err)
	}
}

func TestScreenScraperNeedsADeveloperID(t *testing.T) {
	src := &ScreenScraper{User: "me", Password: "pw"}
	a := src.About()
	if a.Unavailable == "" {
		t.Error("no reason given")
	}
	if _, err := src.Lookup(context.Background(), Query{Title: "x"}); !errors.Is(err, ErrNotSet) {
		t.Errorf("%v", err)
	}
	DevID, DevPassword = "built", "in"
	defer func() { DevID, DevPassword = "", "" }()
	if a := (&ScreenScraper{}).About(); a.Unavailable != "" {
		t.Errorf("a release has one built in: %s", a.Unavailable)
	}
}

// ---------------------------------------------------------------- Wikidata

func TestWikidataAsksInBulk(t *testing.T) {
	var sparql, entities, wiki atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/sparql", func(w http.ResponseWriter, r *http.Request) {
		sparql.Add(1)
		r.ParseForm()
		if !strings.Contains(r.Form.Get("query"), "wd:Q183259") || strings.Contains(r.Form.Get("query"), "wikibase:label") {
			t.Errorf("query: %s", r.Form.Get("query"))
		}
		rows := []string{}
		for i := 1; i <= 60; i++ {
			rows = append(rows, fmt.Sprintf(`{"game":{"value":"http://www.wikidata.org/entity/Q%d"},"gameLabel":{"value":"Game Number %d"},"wiki":{"value":"Game Number %d (video game)"}}`, 1000+i, i, i))
		}
		fmt.Fprintf(w, `{"results":{"bindings":[%s]}}`, strings.Join(rows, ","))
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("maxlag") != "5" {
			t.Errorf("no maxlag: %s", r.URL)
		}
		if q.Get("action") != "wbgetentities" {
			t.Errorf("unexpected: %s", r.URL)
		}
		ids := strings.Split(q.Get("ids"), "|")
		if len(ids) > 50 {
			t.Errorf("%d ids in one request", len(ids))
		}
		if !strings.Contains(q.Get("props"), "claims") {
			fmt.Fprint(w, `{"entities":{"Q500":{"id":"Q500","labels":{"en":{"value":"A Studio"}}},"Q501":{"id":"Q501","labels":{"mul":{"value":"platform video game"}}},"Q502":{"id":"Q502","labels":{"en":{"value":"Teen"}}},"Q503":{"id":"Q503","labels":{"en":{"value":"PEGI 12"}}},"Q504":{"id":"Q504","labels":{"en":{"value":"A Magazine"}}}}}`)
			return
		}
		entities.Add(1)
		out := map[string]any{}
		for _, id := range ids {
			n := strings.TrimPrefix(id, "Q")
			out[id] = json.RawMessage(fmt.Sprintf(`{"id":%q,"labels":{"mul":{"value":"Game Number"}},"sitelinks":{"enwiki":{"title":"Game Number %s (video game)"}},"claims":{
				"P577":[{"rank":"normal","mainsnak":{"datavalue":{"value":{"time":"+1994-03-00T00:00:00Z"}}}}],
				"P178":[{"rank":"normal","mainsnak":{"datavalue":{"value":{"id":"Q500"}}}}],
				"P136":[{"rank":"normal","mainsnak":{"datavalue":{"value":{"id":"Q501"}}}}],
				"P852":[{"rank":"normal","mainsnak":{"datavalue":{"value":{"id":"Q502"}}}}],
				"P908":[{"rank":"deprecated","mainsnak":{"datavalue":{"value":{"id":"Q999"}}}},{"rank":"normal","mainsnak":{"datavalue":{"value":{"id":"Q503"}}}}],
				"P444":[{"rank":"normal","mainsnak":{"datavalue":{"value":"8/10"}},"qualifiers":{"P447":[{"datavalue":{"value":{"id":"Q504"}}}]}},{"rank":"normal","mainsnak":{"datavalue":{"value":"A+"}},"qualifiers":{"P447":[{"datavalue":{"value":{"id":"Q504"}}}]}}],
				"P404":[{"rank":"normal","mainsnak":{"datavalue":{"value":{"id":"Q208189"}}}}]}}`, id, strings.TrimPrefix(n, "10")))
		}
		json.NewEncoder(w).Encode(map[string]any{"entities": out})
	})
	mux.HandleFunc("/wiki", func(w http.ResponseWriter, r *http.Request) {
		wiki.Add(1)
		titles := strings.Split(r.URL.Query().Get("titles"), "|")
		if len(titles) > 20 {
			t.Errorf("%d titles in one request", len(titles))
		}
		pages := []map[string]string{}
		for _, title := range titles {
			pages = append(pages, map[string]string{"title": title, "extract": "<b>Summary</b> of " + title})
		}
		json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": pages}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	dir := t.TempDir()
	src := &WikidataSource{HTTP: srv.Client(), Dir: dir, Query: srv.URL + "/sparql", API: srv.URL + "/api", Wikipedia: srv.URL + "/wiki"}
	src.init()
	src.c.gate.gap = time.Millisecond
	ctx := context.Background()
	var qs []Query
	for i := 1; i <= 50; i++ {
		qs = append(qs, Query{Console: "snes", Title: fmt.Sprintf("Game Number %d", i)})
	}
	if err := src.Prepare(ctx, "snes", qs); err != nil {
		t.Fatal(err)
	}
	for _, q := range qs {
		if _, err := src.Lookup(ctx, q); err != nil {
			t.Fatalf("%s: %v", q.Title, err)
		}
	}
	// 50 games, one list, one batch of facts, three of summaries.
	if sparql.Load() != 1 || entities.Load() != 1 || wiki.Load() != 3 {
		t.Errorf("list %d, facts %d, summaries %d", sparql.Load(), entities.Load(), wiki.Load())
	}
	in, err := src.Lookup(ctx, qs[0])
	if err != nil {
		t.Fatal(err)
	}
	if in.Released != "1994-03" || in.Developer != "A Studio" || len(in.Genres) != 1 || in.Genres[0] != "Platform" || in.Players != 1 {
		t.Errorf("%+v", in)
	}
	if in.Method != MethodName || in.Confidence != 92 || !strings.HasPrefix(in.Description, "Summary of") {
		t.Errorf("%+v", in)
	}
	if len(in.Ages) != 2 || in.Ages[0].Value != "T" || in.Ages[1].Value != "12" {
		t.Errorf("ages %+v", in.Ages)
	}
	if len(in.Scores) != 1 || in.Scores[0].Value != 80 || in.Scores[0].Source != "A Magazine" {
		t.Errorf("scores %+v", in.Scores)
	}
	// A restart uses the list on disk.
	again := &WikidataSource{HTTP: srv.Client(), Dir: dir, Query: srv.URL + "/sparql", API: srv.URL + "/api", Wikipedia: srv.URL + "/wiki"}
	if _, err := again.getList(ctx, "snes"); err != nil {
		t.Fatal(err)
	}
	if sparql.Load() != 1 {
		t.Errorf("%d list queries", sparql.Load())
	}
}

func TestWikidataBusy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":{"code":"maxlag","info":"Waiting"}}`)
	}))
	defer srv.Close()
	src := &WikidataSource{HTTP: srv.Client(), Dir: t.TempDir(), API: srv.URL, Wikipedia: srv.URL, Query: srv.URL}
	src.init()
	_, err := src.searchNames(context.Background(), "x")
	var rl *RateLimited
	if !errors.As(err, &rl) {
		t.Errorf("%v", err)
	}
}

func TestParseReview(t *testing.T) {
	for in, want := range map[string]float64{"8/10": 80, "92/100": 92, "37/40": 92.5, "85%": 85, "A+": 0, "11/10": 0, "4 stars": 0, "": 0} {
		if got := parseReview(in); got != want {
			t.Errorf("parseReview(%q) = %v", in, got)
		}
	}
}

func TestEveryConsoleCanBeNamed(t *testing.T) {
	for console := range rdbNames {
		if !PlatformKnown(console) {
			t.Errorf("%s has a list but no platform numbers", console)
		}
	}
}
