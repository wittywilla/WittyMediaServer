package library

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

// folderFixture: a film library, a show library and a folder to export to.
type folderFixture struct {
	st     *Store
	dir    string
	images string
	film   *Item
	second *Item
	show   *Show
	ep     *Item
}

func newFolderFixture(t *testing.T) *folderFixture {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	st := &Store{DB: d}
	ctx := context.Background()
	base := t.TempDir()
	films, err := st.CreateLibrary(ctx, "Films", "movies", []string{filepath.Join(base, "films")})
	if err != nil {
		t.Fatal(err)
	}
	shows, err := st.CreateLibrary(ctx, "TV", "shows", []string{filepath.Join(base, "tv")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateLibrary(ctx, "Songs", "music", []string{filepath.Join(base, "music")}); err != nil {
		t.Fatal(err)
	}
	y, r := 2001, 7.5
	f := &folderFixture{st: st, dir: filepath.Join(t.TempDir(), "metadata"), images: t.TempDir()}
	f.film = &Item{LibraryID: films.ID, Kind: "video", Path: filepath.Join(base, "films", "Harbour Lights (2001).mkv"), Title: "Harbour Lights", SortTitle: "Harbour Lights", Year: &y, Overview: "A plot,\nover two lines.", Tagline: "It's a film", Genres: []string{"Drama", "Comedy"}, Rating: &r, ContentRating: "PG", Poster: "https://example.test/one.jpg"}
	f.second = &Item{LibraryID: films.ID, Kind: "video", Path: filepath.Join(base, "films", "Quiet Pier.mkv"), Title: "Quiet Pier: Part 2?", SortTitle: "Quiet Pier", Poster: "cache:up-abc.jpg"}
	for _, it := range []*Item{f.film, f.second} {
		if err := st.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
	}
	f.show = &Show{LibraryID: shows.ID, Path: filepath.Join(base, "tv", "Lantern Bay"), Title: "Lantern Bay", SortTitle: "Lantern Bay", Network: "Channel Nine"}
	if err := st.UpsertShow(ctx, f.show); err != nil {
		t.Fatal(err)
	}
	s1, e1 := 1, 2
	f.ep = &Item{LibraryID: shows.ID, Kind: "video", Path: filepath.Join(base, "tv", "Lantern Bay", "S01E02.mkv"), Title: "The Second One", SortTitle: "The Second One", ShowID: &f.show.ID, Season: &s1, Episode: &e1}
	if err := st.upsertItem(ctx, f.ep); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *folderFixture) export(t *testing.T) *ExportResult {
	t.Helper()
	res, err := f.st.ExportMetadata(context.Background(), f.dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Problems) != 0 {
		t.Fatalf("export problems: %v", res.Problems)
	}
	return res
}

func (f *folderFixture) preview(t *testing.T) *FolderPreview {
	t.Helper()
	p, err := f.st.PreviewMetadataFolder(context.Background(), f.dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *folderFixture) apply(t *testing.T) *FolderResult {
	t.Helper()
	r, err := f.st.ApplyMetadataFolder(context.Background(), f.dir, FolderApply{User: "owner", ImagesDir: f.images})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *folderFixture) path(rel string) string {
	return filepath.Join(f.dir, filepath.FromSlash(rel))
}

// edit changes a file by replacing text in it.
func (f *folderFixture) edit(t *testing.T, rel, old, new string) {
	t.Helper()
	b, err := os.ReadFile(f.path(rel))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), old) {
		t.Fatalf("%s has no %q in:\n%s", rel, old, b)
	}
	if err := os.WriteFile(f.path(rel), []byte(strings.Replace(string(b), old, new, 1)), 0o640); err != nil {
		t.Fatal(err)
	}
}

func (f *folderFixture) filmFile() string {
	return "Films/Harbour Lights (2001) [item-" + itoa(f.film.ID) + "].json"
}


func TestExportLayout(t *testing.T) {
	f := newFolderFixture(t)
	res := f.export(t)
	if res.Total != 4 || res.Written != 4 {
		t.Fatalf("%+v", res)
	}
	for _, rel := range []string{
		"README.txt",
		f.filmFile(),
		"Films/Quiet Pier Part 2 [item-" + itoa(f.second.ID) + "].json",
		"TV/Lantern Bay [show-" + itoa(f.show.ID) + "]/show.json",
		"TV/Lantern Bay [show-" + itoa(f.show.ID) + "]/S01E02 The Second One [item-" + itoa(f.ep.ID) + "].json",
	} {
		if _, err := os.Stat(f.path(rel)); err != nil {
			t.Errorf("no %s: %v", rel, err)
		}
	}
	// A music library has no file: its tracks aren't what these fields describe.
	if _, err := os.Stat(f.path("Songs")); err == nil {
		t.Error("a folder for a music library")
	}
	b, _ := os.ReadFile(f.path(f.filmFile()))
	for _, want := range []string{`"title": "Harbour Lights"`, `"year": 2001`, `"genres": [`, `"poster": "https://example.test/one.jpg"`, `"target": "item"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("no %q in:\n%s", want, b)
		}
	}
	// A picture of the server's own isn't something a file can name.
	b, _ = os.ReadFile(f.path("Films/Quiet Pier Part 2 [item-" + itoa(f.second.ID) + "].json"))
	if strings.Contains(string(b), "cache:") {
		t.Errorf("an internal reference in a file:\n%s", b)
	}
	if r := f.export(t); r.Written != 0 || r.Unchanged != 4 {
		t.Errorf("a second export wrote files: %+v", r)
	}
}

// What an export writes reads back as no change at all, even with no record of having
// written it: a file copied from a backup is compared by what it says.
func TestExportReadsBackAsNoChange(t *testing.T) {
	f := newFolderFixture(t)
	f.export(t)
	if p := f.preview(t); p.Total != 0 || len(p.Problems) != 0 || p.Unchanged != 4 {
		t.Fatalf("%+v", p)
	}
	if err := os.Remove(f.path(metafolder.StateFile)); err != nil {
		t.Fatal(err)
	}
	if p := f.preview(t); p.Total != 0 || len(p.Problems) != 0 {
		t.Fatalf("with no record: %+v", p)
	}
}

func TestApplyIsPreviewedLockedAndUndone(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	f.export(t)
	f.edit(t, f.filmFile(), `"title": "Harbour Lights"`, `"title": "<script>alert(1)</script>Harbour Lights Revisited"`)
	f.edit(t, f.filmFile(), `"year": 2001`, `"year": 2003`)
	f.edit(t, f.filmFile(), `"poster": "https://example.test/one.jpg"`, `"poster": "https://example.test/two.jpg"`)

	p := f.preview(t)
	if p.Total != 1 || len(p.Changes) != 1 {
		t.Fatalf("%+v", p)
	}
	c := p.Changes[0]
	byField := map[string]FolderFieldChange{}
	for _, x := range c.Fields {
		byField[x.Field] = x
	}
	if byField["title"].New != "Harbour Lights Revisited" || byField["title"].Old != "Harbour Lights" || byField["year"].New != "2003" || byField["poster"].New != "https://example.test/two.jpg" {
		t.Errorf("%+v", byField)
	}
	if byField["year"].Line == 0 {
		t.Error("a change doesn't say its line")
	}
	warned := false
	for _, pr := range p.Problems {
		if pr.Severity == metafolder.SevWarning && strings.Contains(pr.Msg, "taken out") && pr.Line > 0 {
			warned = true
		}
	}
	if !warned {
		t.Errorf("no word about the script: %+v", p.Problems)
	}
	// A preview changes nothing.
	if it, _ := f.st.Item(ctx, f.film.ID); it.Title != "Harbour Lights" {
		t.Fatalf("preview wrote: %q", it.Title)
	}

	r := f.apply(t)
	if r.Applied != 1 || r.Batch == "" || r.Skipped != 0 {
		t.Fatalf("%+v", r)
	}
	it, _ := f.st.Item(ctx, f.film.ID)
	if it.Title != "Harbour Lights Revisited" || it.Year == nil || *it.Year != 2003 || it.Poster != "https://example.test/two.jpg" {
		t.Fatalf("%+v", it)
	}
	if it.SortTitle != "Harbour Lights Revisited" {
		t.Errorf("sorted under %q", it.SortTitle)
	}
	m := f.st.TitleMeta(ctx, TargetItem, f.film.ID)
	if !m.Locks["title"] || !m.Locks["year"] || m.Sources["title"] != SourceYou {
		t.Errorf("not locked: %+v", m)
	}
	hist, err := f.st.History(ctx, 10)
	if err != nil || len(hist) != 1 || hist[0].Batch != r.Batch || hist[0].User != "owner" {
		t.Errorf("history: %+v %v", hist, err)
	}
	// Applied is done with: nothing more to do, and a second apply does nothing.
	if p := f.preview(t); p.Total != 0 {
		t.Errorf("still to do: %+v", p.Changes)
	}
	if r2 := f.apply(t); r2.Applied != 0 {
		t.Errorf("applied twice: %+v", r2)
	}

	if n, err := f.st.Undo(ctx, r.Batch); err != nil || n != 1 {
		t.Fatalf("undo: %d %v", n, err)
	}
	it, _ = f.st.Item(ctx, f.film.ID)
	if it.Title != "Harbour Lights" || *it.Year != 2001 {
		t.Fatalf("not undone: %+v", it)
	}
	if m := f.st.TitleMeta(ctx, TargetItem, f.film.ID); m.Locks["title"] {
		t.Errorf("still locked: %+v", m)
	}
	// With nothing unapplied, an export brings the file back into line.
	f.export(t)
	b, _ := os.ReadFile(f.path(f.filmFile()))
	if !strings.Contains(string(b), `"title": "Harbour Lights"`) {
		t.Errorf("file not brought up to date:\n%s", b)
	}
}

// A bad file is listed with its line, and the rest still go ahead.
func TestBadFileSkipsOnlyItself(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	f.export(t)
	second := "Films/Quiet Pier Part 2 [item-" + itoa(f.second.ID) + "].json"
	f.edit(t, f.filmFile(), `"title": "Harbour Lights",`, `"title": "Harbour Lights"`)
	f.edit(t, second, `"title": "Quiet Pier: Part 2?"`, `"title": "Quiet Pier, Again"`)
	r := f.apply(t)
	if r.Applied != 1 || r.Skipped != 1 {
		t.Fatalf("%+v", r)
	}
	found := false
	for _, p := range r.Problems {
		if p.File == f.filmFile() && p.Severity == metafolder.SevError && p.Line == 7 && strings.Contains(p.Msg, "comma") {
			found = true
		}
	}
	if !found {
		t.Errorf("no problem naming the file and line: %+v", r.Problems)
	}
	if it, _ := f.st.Item(ctx, f.second.ID); it.Title != "Quiet Pier, Again" {
		t.Errorf("good file not applied: %q", it.Title)
	}
	if it, _ := f.st.Item(ctx, f.film.ID); it.Title != "Harbour Lights" {
		t.Errorf("bad file used: %q", it.Title)
	}
	// The bad one is still to be dealt with.
	if p := f.preview(t); !metafolder.HasErrors(p.Problems) {
		t.Errorf("forgotten: %+v", p)
	}
}

// A change made in the app after an export isn't undone by a file nobody touched.
func TestUntouchedFileChangesNothing(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	f.export(t)
	tt, err := f.st.LoadTitle(ctx, TargetItem, f.film.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := tt.EditFields(map[string]string{"title": "Changed In The App"}, true); err != nil {
		t.Fatal(err)
	}
	if err := f.st.SaveTitle(ctx, tt); err != nil {
		t.Fatal(err)
	}
	if p := f.preview(t); p.Total != 0 {
		t.Fatalf("%+v", p.Changes)
	}
	f.apply(t)
	if it, _ := f.st.Item(ctx, f.film.ID); it.Title != "Changed In The App" {
		t.Fatalf("an untouched file undid it: %q", it.Title)
	}
	// An export brings it up to date, in the file it already has.
	r := f.export(t)
	if r.Written != 1 {
		t.Errorf("%+v", r)
	}
	b, err := os.ReadFile(f.path(f.filmFile()))
	if err != nil || !strings.Contains(string(b), "Changed In The App") {
		t.Errorf("%s %v", b, err)
	}
}

func TestExportKeepsEditsNotApplied(t *testing.T) {
	f := newFolderFixture(t)
	f.export(t)
	f.edit(t, f.filmFile(), `"year": 2001`, `"year": 2003`)
	r := f.export(t)
	if r.Kept != 1 || r.Written != 0 {
		t.Errorf("%+v", r)
	}
	b, _ := os.ReadFile(f.path(f.filmFile()))
	if !strings.Contains(string(b), `"year": 2003`) {
		t.Error("an export wrote over someone's edit")
	}
}

func TestFileForAnotherTitle(t *testing.T) {
	f := newFolderFixture(t)
	f.export(t)
	f.edit(t, f.filmFile(), `Harbour Lights (2001).mkv`, `Something Else.mkv`)
	r := f.apply(t)
	if r.Applied != 0 || r.Skipped != 1 || !strings.Contains(r.Problems[0].Msg, "Something Else.mkv") || r.Problems[0].Line != 5 {
		t.Errorf("%+v", r)
	}
	// A number nobody has.
	bad := filepath.Join(f.dir, "Films", "Gone [item-99999].json")
	if err := os.WriteFile(bad, []byte(`{"title":"Gone"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	p := f.preview(t)
	seen := false
	for _, pr := range p.Problems {
		if strings.Contains(pr.File, "Gone") && strings.Contains(pr.Msg, "99999") {
			seen = true
		}
	}
	if !seen {
		t.Errorf("%+v", p.Problems)
	}
}

func TestTwoFilesForOneTitle(t *testing.T) {
	f := newFolderFixture(t)
	f.export(t)
	b, _ := os.ReadFile(f.path(f.filmFile()))
	twin := filepath.Join(f.dir, "Films", "Renamed [item-"+itoa(f.film.ID)+"].json")
	if err := os.WriteFile(twin, []byte(strings.Replace(string(b), "Harbour Lights\"", "Other Name\"", 1)), 0o640); err != nil {
		t.Fatal(err)
	}
	r := f.apply(t)
	if r.Applied != 0 || !metafolder.HasErrors(r.Problems) {
		t.Errorf("%+v", r)
	}
	if it, _ := f.st.Item(context.Background(), f.film.ID); it.Title != "Harbour Lights" {
		t.Errorf("one of them was used: %q", it.Title)
	}
}

func TestPicturesFromTheFolder(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	f.export(t)
	rel := f.filmFile()
	stem := strings.TrimSuffix(rel, ".json")
	// Put a picture beside it, with nothing else changed.
	if err := os.WriteFile(f.path(stem+"-poster.jpg"), jpegForTest, 0o640); err != nil {
		t.Fatal(err)
	}
	p := f.preview(t)
	if p.Total != 1 || p.Changes[0].Fields[0].Field != "poster" || !strings.Contains(p.Changes[0].Fields[0].New, "-poster.jpg") {
		t.Fatalf("%+v", p)
	}
	if entries, _ := os.ReadDir(f.images); len(entries) != 0 {
		t.Errorf("a preview copied a picture")
	}
	r := f.apply(t)
	if r.Applied != 1 {
		t.Fatalf("%+v", r)
	}
	it, _ := f.st.Item(ctx, f.film.ID)
	if !strings.HasPrefix(it.Poster, "cache:up-") {
		t.Fatalf("poster: %q", it.Poster)
	}
	name := strings.TrimPrefix(it.Poster, "cache:")
	if got, err := os.ReadFile(filepath.Join(f.images, name)); err != nil || string(got) != string(jpegForTest) {
		t.Errorf("not in the image cache: %v", err)
	}
	if p := f.preview(t); p.Total != 0 {
		t.Errorf("again: %+v", p.Changes)
	}

	// Named in the file: a fake, traversal, a link. None gets through.
	f.edit(t, rel, `"year": 2001`, `"year": 2001,
  "backdrop": "wide.jpg"`)
	if err := os.WriteFile(f.path("Films/wide.jpg"), []byte("<svg onload=alert(1)>"), 0o640); err != nil {
		t.Fatal(err)
	}
	r = f.apply(t)
	if r.Applied != 0 || !strings.Contains(r.Problems[0].Msg, "isn't a picture") {
		t.Errorf("a fake picture: %+v", r)
	}
	if err := os.Remove(f.path("Films/wide.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(f.path(rel), f.path("Films/wide.jpg")); err == nil {
		r = f.apply(t)
		if r.Applied != 0 || !strings.Contains(r.Problems[0].Msg, "link") {
			t.Errorf("a link: %+v", r)
		}
		_ = os.Remove(f.path("Films/wide.jpg"))
	}
	f.edit(t, rel, `"backdrop": "wide.jpg"`, `"backdrop": "../../secret.jpg"`)
	r = f.apply(t)
	if r.Applied != 0 || !metafolder.HasErrors(r.Problems) || r.Problems[0].Field != "backdrop" {
		t.Errorf("traversal: %+v", r)
	}
	f.edit(t, rel, `"backdrop": "../../secret.jpg"`, `"backdrop": "javascript:alert(1)"`)
	r = f.apply(t)
	if r.Applied != 0 || !metafolder.HasErrors(r.Problems) {
		t.Errorf("javascript: %+v", r)
	}
	if it, _ := f.st.Item(ctx, f.film.ID); it.Backdrop != "" {
		t.Errorf("backdrop: %q", it.Backdrop)
	}
}

func TestLocksAndMatchInAFile(t *testing.T) {
	f := newFolderFixture(t)
	ctx := context.Background()
	f.export(t)
	rel := f.filmFile()
	f.edit(t, rel, `"locked": []`, `"locked": ["overview"],
  "provider": "tmdb",
  "providerId": "42"`)
	if r := f.apply(t); r.Applied != 1 {
		t.Fatalf("%+v", r)
	}
	m := f.st.TitleMeta(ctx, TargetItem, f.film.ID)
	if !m.Locks["overview"] || m.MatchState != MatchManual {
		t.Errorf("%+v", m)
	}
	it, _ := f.st.Item(ctx, f.film.ID)
	if it.Provider != "tmdb" || it.ProviderID != "42" {
		t.Errorf("%+v", it)
	}
	f.edit(t, rel, `"locked": ["overview"]`, `"locked": []`)
	if r := f.apply(t); r.Applied != 1 {
		t.Fatalf("%+v", r)
	}
	if m := f.st.TitleMeta(ctx, TargetItem, f.film.ID); m.Locks["overview"] {
		t.Errorf("still locked: %+v", m)
	}
}

// A field that kind of title doesn't have is a note, and the rest is used.
func TestFieldTheTitleLacks(t *testing.T) {
	f := newFolderFixture(t)
	f.export(t)
	rel := "TV/Lantern Bay [show-" + itoa(f.show.ID) + "]/S01E02 The Second One [item-" + itoa(f.ep.ID) + "].json"
	f.edit(t, rel, `"title": "The Second One"`, `"title": "Fixed", "network": "Elsewhere"`)
	r := f.apply(t)
	if r.Applied != 1 || len(r.Problems) != 1 || r.Problems[0].Severity != metafolder.SevWarning || r.Problems[0].Field != "network" {
		t.Errorf("%+v", r)
	}
}

func TestPlantedLinkStopsWriting(t *testing.T) {
	f := newFolderFixture(t)
	outside := t.TempDir()
	if err := os.MkdirAll(f.dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.dir, "Films")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	res, err := f.st.ExportMetadata(context.Background(), f.dir, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !metafolder.HasErrors(res.Problems) {
		t.Errorf("%+v", res)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("wrote through a link: %v", entries)
	}
}

var jpegForTest = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 60)...)

// ---------------------------------------------------------------- files beside media

func writeTemp(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const planted = `<script>alert(1)</script><img src=x onerror=alert(1)>`

func hasNoMarkup(t *testing.T, what string, values ...string) {
	t.Helper()
	for _, v := range values {
		low := strings.ToLower(v)
		for _, bad := range []string{"<script", "alert(1)", "onerror", "<img", "\x00", "‮"} {
			if strings.Contains(low, bad) {
				t.Errorf("%s kept %q: %q", what, bad, v)
			}
		}
	}
}

func TestNFOIsCleaned(t *testing.T) {
	esc := "&lt;script&gt;alert(1)&lt;/script&gt;&lt;img src=x onerror=alert(1)&gt;"
	var it Item
	ok := applyNFO(&it, writeTemp(t, "a.nfo", `<?xml version="1.0"?>
<movie>
  <title>`+esc+`Film</title>
  <sorttitle>`+esc+`Sort</sorttitle>
  <plot>`+esc+`Plot&#x202E;</plot>
  <tagline>`+esc+`Line</tagline>
  <genre>`+esc+`Drama</genre>
  <mpaa>`+esc+`PG</mpaa>
  <studio>`+esc+`Works</studio>
  <year>99999</year>
  <rating>1000</rating>
  <tmdbid>../../etc/passwd</tmdbid>
  <thumb>javascript:alert(1)</thumb>
  <fanart><thumb>file:///etc/passwd</thumb></fanart>
</movie>`))
	if !ok {
		t.Fatal("not read")
	}
	hasNoMarkup(t, "nfo", it.Title, it.SortTitle, it.Overview, it.Tagline, it.ContentRating, it.Studio, strings.Join(it.Genres, ","))
	if it.Title != "Film" || it.Overview != "Plot" || it.Genres[0] != "Drama" {
		t.Errorf("%+v", it)
	}
	if it.Year != nil || it.Rating != nil || it.ProviderID != "" || it.Poster != "" || it.Backdrop != "" {
		t.Errorf("out of range or unsafe values kept: %+v", it)
	}
	it = Item{}
	if !applyNFO(&it, writeTemp(t, "b.nfo", `<movie><thumb>https://example.test/p.jpg</thumb></movie>`)) || it.Poster != "https://example.test/p.jpg" {
		t.Errorf("a good picture was refused: %q", it.Poster)
	}
}

func TestNFOAttacksAreRefused(t *testing.T) {
	for name, body := range map[string]string{
		"xxe":    `<?xml version="1.0"?><!DOCTYPE movie [<!ENTITY x SYSTEM "file:///etc/passwd">]><movie><title>&x;</title></movie>`,
		"laughs": `<!DOCTYPE l [<!ENTITY a "aaaa"><!ENTITY b "&a;&a;&a;">]><movie><title>&b;</title></movie>`,
		"huge":   `<movie><title>` + strings.Repeat("a", metafolder.MaxNFO+10) + `</title></movie>`,
		"deep":   `<movie>` + strings.Repeat("<a>", 200) + strings.Repeat("</a>", 200) + `</movie>`,
	} {
		var it Item
		if applyNFO(&it, writeTemp(t, name+".nfo", body)) {
			t.Errorf("%s was read: %q", name, it.Title)
		}
		var sh Show
		if applyShowNFO(&sh, writeTemp(t, name+".nfo", strings.ReplaceAll(body, "movie", "tvshow"))) {
			t.Errorf("%s was read as a show", name)
		}
	}
}

func TestShowAndEpisodeNFOAreCleaned(t *testing.T) {
	esc := "&lt;script&gt;alert(1)&lt;/script&gt;"
	var sh Show
	if !applyShowNFO(&sh, writeTemp(t, "tvshow.nfo", `<tvshow><title>`+esc+`Show</title><plot>`+esc+`Plot</plot><genre>`+esc+`Drama</genre><studio>`+esc+`Net</studio><status>`+esc+`On</status><mpaa>`+esc+`PG</mpaa><tvdbid>a b/c</tvdbid></tvshow>`)) {
		t.Fatal("not read")
	}
	hasNoMarkup(t, "show", sh.Title, sh.Overview, sh.Network, sh.Status, sh.ContentRating, strings.Join(sh.Genres, ","))
	if sh.Title != "Show" || sh.ProviderID != "" {
		t.Errorf("%+v", sh)
	}
	var it Item
	if !applyEpisodeNFO(&it, writeTemp(t, "e.nfo", `<episodedetails><title>`+esc+`One</title><plot>`+esc+`Plot</plot><aired>`+esc+`2001-01-01</aired><season>1</season><episode>2</episode><thumb>javascript:alert(1)</thumb></episodedetails><episodedetails><title>Two</title></episodedetails>`)) {
		t.Fatal("not read")
	}
	hasNoMarkup(t, "episode", it.Title, it.Overview, it.Aired)
	if it.Title != "One" || it.Poster != "" || *it.Episode != 2 {
		t.Errorf("%+v", it)
	}
}

func TestSidecarJSONIsCleaned(t *testing.T) {
	dir := t.TempDir()
	side := filepath.Join(dir, "a.metadata.json")
	secret := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(secret, jpegForTest, 0o600); err != nil {
		t.Fatal(err)
	}
	body := `{"title":"` + strings.ReplaceAll(planted, `"`, `\"`) + `Film‮","overview":"<script>alert(1)</script>Plot\u0000","tagline":"<img src=x onerror=alert(1)>Tag","genres":["<script>alert(1)</script>Drama"],"poster":"javascript:alert(1)","backdrop":"../../secret.jpg","provider":"tmdb","providerId":"../x y","year":99999,"rating":99}`
	if err := os.WriteFile(side, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var it Item
	if !applySidecarJSON(&it, side) {
		t.Fatal("not read")
	}
	hasNoMarkup(t, "sidecar", it.Title, it.Overview, it.Tagline, strings.Join(it.Genres, ","))
	if it.Title != "Film" || it.Overview != "Plot" || it.Poster != "" || it.Backdrop != "" || it.Year != nil || it.Rating != nil || it.ProviderID != "" {
		t.Errorf("%+v", it)
	}
	// A link beside the media that leads out isn't a picture of it.
	link := filepath.Join(dir, "linked.jpg")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := os.WriteFile(side, []byte(`{"poster":"linked.jpg"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	it = Item{}
	applySidecarJSON(&it, side)
	if it.Poster != "" {
		t.Errorf("followed a link out: %q", it.Poster)
	}
	// A file that's too big isn't read at all.
	if err := os.WriteFile(side, []byte(`{"title":"`+strings.Repeat("a", metafolder.MaxJSON)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	it = Item{}
	if applySidecarJSON(&it, side) {
		t.Error("a huge sidecar was read")
	}
}

func TestOtherSidecarsAreCleaned(t *testing.T) {
	mv := readMusicVideoNFO(writeTemp(t, "m.nfo", `<musicvideo><title>&lt;script&gt;alert(1)&lt;/script&gt;Song</title><artist>&lt;img src=x onerror=alert(1)&gt;Band</artist><plot>&lt;script&gt;alert(1)&lt;/script&gt;Plot</plot><genre>&lt;script&gt;alert(1)&lt;/script&gt;Rock</genre><tag>&lt;script&gt;alert(1)&lt;/script&gt;Live</tag></musicvideo>`))
	if mv == nil {
		t.Fatal("not read")
	}
	hasNoMarkup(t, "music video", mv.Title, mv.Artist, mv.Plot, strings.Join(mv.Genres, ","), strings.Join(mv.Tags, ","))
	if mv.Title != "Song" || mv.Artist != "Band" {
		t.Errorf("%+v", mv)
	}
	dir := t.TempDir()
	image := filepath.Join(dir, "a.cue")
	if err := os.WriteFile(cdSidecarPath(image), []byte(`{"title":"<script>alert(1)</script>Album","artist":"<img src=x onerror=alert(1)>Band","tracks":[{"title":"<script>alert(1)</script>One","artist":"‮Band"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cd := readCDSidecar(image)
	if cd == nil {
		t.Fatal("not read")
	}
	hasNoMarkup(t, "cd", cd.Title, cd.Artist, cd.Tracks[0].Title, cd.Tracks[0].Artist)
	if cd.Title != "Album" || cd.Tracks[0].Title != "One" {
		t.Errorf("%+v", cd)
	}
	v, _ := readSidecar(writeTemp(t, "v.info.json", `{"title":"<script>alert(1)</script>Clip","description":"<img src=x onerror=alert(1)>Words","uploader":"<script>alert(1)</script>Who","webpage_url":"javascript:alert(1)","tags":["<script>alert(1)</script>Tag"],"comments":[{"author":"<script>alert(1)</script>A","text":"<script>alert(1)</script>B"}],"custom":{"k":"<script>alert(1)</script>V","n":{"m":"<script>alert(1)</script>W"}}}`))
	if v == nil {
		t.Fatal("not read")
	}
	hasNoMarkup(t, "video sidecar", v.Title, v.Description, v.Uploader, v.WebpageURL, strings.Join(v.Tags, ","), v.Comments[0].Author, v.Comments[0].Text, v.Custom["k"].(string), v.Custom["n"].(map[string]any)["m"].(string))
	if v.Title != "Clip" || v.WebpageURL != "" {
		t.Errorf("%+v", v)
	}
}
