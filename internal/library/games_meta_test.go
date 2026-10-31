package library

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/gamemeta"
)

// fakeSource answers from a table, and counts how often it's asked.
type fakeSource struct {
	id     string
	fields []gamemeta.KeyField
	answer func(q gamemeta.Query) (*gamemeta.Info, error)
	asked  atomic.Int32
	titles []string
	gaps   bool
}

func (f *fakeSource) About() gamemeta.About {
	return gamemeta.About{ID: f.id, Name: "Fake " + f.id, Site: "https://example.test/" + f.id, Credit: "From " + f.id, License: "Test", Fields: f.fields}
}
func (f *fakeSource) Lookup(ctx context.Context, q gamemeta.Query) (*gamemeta.Info, error) {
	f.asked.Add(1)
	f.titles = append(f.titles, q.Title)
	return f.answer(q)
}
func (f *fakeSource) Search(ctx context.Context, console, title string) ([]gamemeta.Candidate, error) {
	return []gamemeta.Candidate{{Source: f.id, ID: "42", Title: title, Score: 100}}, nil
}
func (f *fakeSource) Fetch(ctx context.Context, console, id string) (*gamemeta.Info, error) {
	in, err := f.answer(gamemeta.Query{Console: console, Title: id})
	if err != nil {
		return nil, err
	}
	in.ID = id
	return in, nil
}
func (f *fakeSource) Test(ctx context.Context) (string, error) { return "fine", nil }

type gapSource struct{ *fakeSource }

func (g gapSource) GapsOnly() bool { return true }

type metaFixture struct {
	g     *GameMeta
	store *Store
	lib   *Library
	root  string
	ctx   context.Context
	src   []gamemeta.Source
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b strings.Builder
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return []byte(b.String())
}

func newMetaFixture(t *testing.T, sources ...gamemeta.Source) *metaFixture {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	store := &Store{DB: d}
	root := t.TempDir()
	ctx := context.Background()
	lib, err := store.CreateLibrary(ctx, "Games", "games", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetLibraryConsole(ctx, lib.ID, "snes"); err != nil {
		t.Fatal(err)
	}
	lib.Console = "snes"
	g := NewGameMeta(store, filepath.Join(dir, "images"), filepath.Join(dir, "cache"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	g.Built = func() []gamemeta.Source { return sources }
	var ids []GameSourceSetting
	for _, s := range sources {
		ids = append(ids, GameSourceSetting{ID: s.About().ID, On: true})
	}
	if err := d.SetSetting(ctx, "gameSources", toJSON(ids)); err != nil {
		t.Fatal(err)
	}
	defaultSourceOrder = append(defaultSourceOrder[:0:0], defaultSourceOrder...)
	for _, s := range sources {
		if !contains(defaultSourceOrder, s.About().ID) {
			defaultSourceOrder = append(defaultSourceOrder, s.About().ID)
			id := s.About().ID
			t.Cleanup(func() {
				for i, x := range defaultSourceOrder {
					if x == id {
						defaultSourceOrder = append(defaultSourceOrder[:i], defaultSourceOrder[i+1:]...)
						break
					}
				}
			})
		}
	}
	return &metaFixture{g: g, store: store, lib: lib, root: root, ctx: ctx, src: sources}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// scanFile makes a game file and has a scan list it.
func (f *metaFixture) scanFile(t *testing.T, name string) *Item {
	t.Helper()
	p := filepath.Join(f.root, name)
	if err := os.WriteFile(p, []byte("a small game: "+name), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := NewScanner(f.store, nil, t.TempDir(), func() string { return "" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	sc.GameMeta = f.g
	if err := sc.scan(f.ctx, f.lib.ID); err != nil {
		t.Fatal(err)
	}
	it, err := f.store.itemByPath(f.ctx, f.lib.ID, p)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func (f *metaFixture) work(t *testing.T) {
	t.Helper()
	f.g.work(f.ctx)
}

func (f *metaFixture) item(t *testing.T, id int64) *Item {
	t.Helper()
	it, err := f.store.Item(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestScanWaitsForNobody(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(gamemeta.Query) (*gamemeta.Info, error) { return nil, gamemeta.ErrNotFound }}
	f := newMetaFixture(t, src)
	it := f.scanFile(t, "Mega Game (USA).sfc")
	if src.asked.Load() != 0 {
		t.Error("a scan asked a source")
	}
	if it.Title != "Mega Game" || it.Tagline != "USA" {
		t.Errorf("%+v", it)
	}
	st := f.g.Status(f.ctx)
	if st.Total != 1 || st.Pending != 1 {
		t.Errorf("%+v", st)
	}
}

func TestDetailsApplied(t *testing.T) {
	esrb, _ := gamemeta.NewAge("ESRB", "T")
	first := &fakeSource{id: "first", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Title: "Mega Game: The Quest (USA)", Method: gamemeta.MethodChecksum, Confidence: 100, Developer: "Maker", Publisher: "Seller",
			Released: "1994-03-02", Players: 2, Genres: []string{"Action"}, Ages: []gamemeta.AgeRating{esrb}, URL: "https://first.test/1"}, nil
	}}
	second := &fakeSource{id: "second", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "2", Title: "Other Title", Method: gamemeta.MethodName, Confidence: 85, Developer: "Ignored", Description: "Words",
			Scores: []gamemeta.Score{{Source: "A Magazine", Kind: "critic", Value: 90}}}, nil
	}}
	f := newMetaFixture(t, first, second)
	it := f.scanFile(t, "Mega Game (USA).sfc")
	f.work(t)
	got := f.item(t, it.ID)
	if got.Title != "Mega Game: The Quest" || got.SortTitle == "" || got.Overview != "Words" || got.Studio != "Maker" || got.Year == nil || *got.Year != 1994 {
		t.Errorf("%+v", got)
	}
	if got.Tagline != "USA" {
		t.Errorf("the file's own tags stay: %q", got.Tagline)
	}
	if got.Rating == nil || *got.Rating != 9 {
		t.Errorf("rating %v", got.Rating)
	}
	if got.ContentRating != "ESRB T" || got.Provider != "game:first" {
		t.Errorf("%q %q", got.ContentRating, got.Provider)
	}
	d, err := f.store.GameDetails(f.ctx, it.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != "done" || d.MatchSource != "first" || d.MatchMethod != gamemeta.MethodChecksum || d.Confidence != 100 || d.Publisher != "Seller" || d.Players != 2 || d.Released != "1994-03-02" {
		t.Errorf("%+v", d)
	}
	if len(d.Scores) != 1 || len(d.Ages) != 1 || len(d.Credits) != 2 || d.Credits[0].URL != "https://first.test/1" {
		t.Errorf("%+v", d)
	}
	// The name a checksum found is what later sources are asked about.
	if second.titles[len(second.titles)-1] != "Mega Game: The Quest" {
		t.Errorf("asked about %v", second.titles)
	}
}

// Parental controls work from the rating, so a game rated above a limit is hidden
// exactly as a film would be, and one with no rating is treated like a film with none.
func TestAgeRatingsReachParentalControls(t *testing.T) {
	ratings := map[string]string{"Teen Game": "T", "Mature Game": "M", "Kids Game": "E"}
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		v, ok := ratings[q.Title]
		if !ok {
			return nil, gamemeta.ErrNotFound
		}
		r, _ := gamemeta.NewAge("ESRB", v)
		return &gamemeta.Info{ID: q.Title, Method: gamemeta.MethodName, Confidence: 92, Ages: []gamemeta.AgeRating{r}}, nil
	}}
	f := newMetaFixture(t, src)
	for name := range ratings {
		f.scanFile(t, name+".sfc")
	}
	f.scanFile(t, "Unknown Game.sfc")
	f.work(t)
	seen := func(a *Access) map[string]bool {
		items, _, err := f.store.Items(f.ctx, ItemQuery{LibraryID: f.lib.ID, Access: a})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, it := range items {
			out[it.Title] = true
		}
		return out
	}
	kids := DefaultAccess("kids") // 7 and under, nothing unrated
	got := seen(&kids)
	if !got["Kids Game"] || got["Teen Game"] || got["Mature Game"] || got["Unknown Game"] {
		t.Errorf("kids see %v", got)
	}
	guest := DefaultAccess("guest") // 13 and under
	got = seen(&guest)
	if !got["Kids Game"] || !got["Teen Game"] || got["Mature Game"] {
		t.Errorf("guests see %v", got)
	}
	got = seen(nil)
	if len(got) != 4 {
		t.Errorf("no limits: %v", got)
	}
	// The same answer for one game as for lists of them.
	all, _, _ := f.store.Items(f.ctx, ItemQuery{LibraryID: f.lib.ID})
	for _, it := range all {
		if want := it.Title == "Kids Game"; kids.AllowsItem(it, nil) != want {
			t.Errorf("AllowsItem(%s) = %v", it.Title, !want)
		}
	}
}

func TestAgeForGames(t *testing.T) {
	for in, want := range map[string]int{"ESRB T": 13, "ESRB M": 17, "esrb:E10+": 10, "PEGI 18": 18, "CERO B": 12, "USK 16": 16, "ACB MA15+": 15, "T": 13, "AO": 18} {
		got := AgeFor(in)
		if got == nil || *got != want {
			t.Errorf("AgeFor(%q) = %v, want %d", in, got, want)
		}
	}
	// A film's ratings still mean what they meant.
	if got := AgeFor("M"); got == nil || *got != 15 {
		t.Errorf("AgeFor(M) = %v", got)
	}
	if got := AgeFor("ESRB RP"); got != nil {
		t.Errorf("rating pending: %v", *got)
	}
}

func TestAnswersAreKept(t *testing.T) {
	a := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Method: gamemeta.MethodName, Confidence: 92, Developer: "From A"}, nil
	}}
	b := &fakeSource{id: "b", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "2", Method: gamemeta.MethodName, Confidence: 92, Developer: "From B"}, nil
	}}
	f := newMetaFixture(t, a, b)
	it := f.scanFile(t, "Mega Game.sfc")
	f.work(t)
	if got := f.item(t, it.ID); got.Studio != "From A" {
		t.Fatalf("%q", got.Studio)
	}
	// A new order: merged again, with nobody asked.
	if err := f.g.SetSourceOrder(f.ctx, []GameSourceSetting{{"b", true}, {"a", true}}); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if got := f.item(t, it.ID); got.Studio != "From B" {
		t.Errorf("reordered: %q", got.Studio)
	}
	if a.asked.Load() != 1 || b.asked.Load() != 1 {
		t.Errorf("asked %d and %d times", a.asked.Load(), b.asked.Load())
	}
	// A rescan of the same file changes nothing.
	f.scanFile(t, "Another Game.sfc")
	f.work(t)
	if a.asked.Load() != 2 {
		t.Errorf("asked %d times", a.asked.Load())
	}
	if got := f.item(t, it.ID); got.Studio != "From B" {
		t.Errorf("after a rescan: %q", got.Studio)
	}
	// A changed file is a new question.
	if err := os.WriteFile(it.Path, []byte("a different version"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(it.Path, time.Now().Add(time.Hour), time.Now().Add(time.Hour))
	f.scanFile(t, "A Third Game.sfc")
	f.work(t)
	if a.asked.Load() != 4 {
		t.Errorf("asked %d times after the file changed", a.asked.Load())
	}
}

// Stopping part way loses nothing, and what's finished isn't asked again.
func TestResumesAfterRestart(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: q.Title, Method: gamemeta.MethodName, Confidence: 92, Developer: "Maker"}, nil
	}}
	f := newMetaFixture(t, src)
	for i := 0; i < gameMetaBatch+5; i++ {
		f.scanFile(t, fmt.Sprintf("Game %03d.sfc", i))
	}
	if n, err := f.g.detailsBatch(f.ctx); err != nil || n != gameMetaBatch {
		t.Fatalf("%d, %v", n, err)
	}
	if st := f.g.Status(f.ctx); st.Pending != 5 {
		t.Errorf("%+v", st)
	}
	// A new worker on the same database: the server has restarted.
	again := NewGameMeta(f.store, f.g.ImagesDir, f.g.CacheDir, f.g.Log)
	again.Built = f.g.Built
	again.work(f.ctx)
	if st := again.Status(f.ctx); st.Pending != 0 {
		t.Errorf("%+v", st)
	}
	if int(src.asked.Load()) != gameMetaBatch+5 {
		t.Errorf("asked %d times", src.asked.Load())
	}
}

// A source that says to stop is left alone, for every game, until it says when.
func TestRateLimitedSourceWaits(t *testing.T) {
	limited := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return nil, &gamemeta.RateLimited{Until: time.Now().Add(time.Hour), Why: "too fast"}
	}}
	fine := &fakeSource{id: "b", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Method: gamemeta.MethodName, Confidence: 92, Developer: "Maker"}, nil
	}}
	f := newMetaFixture(t, limited, fine)
	var ids []int64
	for i := 0; i < 5; i++ {
		ids = append(ids, f.scanFile(t, fmt.Sprintf("Game %d.sfc", i)).ID)
	}
	f.work(t)
	if limited.asked.Load() != 1 {
		t.Errorf("a source that said stop was asked %d times", limited.asked.Load())
	}
	if fine.asked.Load() != 5 {
		t.Errorf("the other source was asked %d times", fine.asked.Load())
	}
	// What the other source said is used right away.
	if got := f.item(t, ids[4]); got.Studio != "Maker" {
		t.Errorf("%+v", got)
	}
	st := f.g.Status(f.ctx)
	if st.Pending != 5 || len(st.Paused) != 1 || st.Paused[0].Source != "a" {
		t.Errorf("%+v", st)
	}
	// It stays waiting however many times the worker wakes, without giving up.
	for i := 0; i < 10; i++ {
		f.work(t)
	}
	if limited.asked.Load() != 1 {
		t.Errorf("asked %d times", limited.asked.Load())
	}
	var tries int
	f.store.DB.QueryRow(`SELECT tries FROM game_meta WHERE item_id = ?`, ids[0]).Scan(&tries)
	if tries != 0 {
		t.Errorf("waiting counted as %d failures", tries)
	}
}

// A source that fails is tried again later, a limited number of times.
func TestFailingSourceBacksOff(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) { return nil, errors.New("it fell over") }}
	f := newMetaFixture(t, src)
	it := f.scanFile(t, "Game.sfc")
	f.work(t)
	f.work(t)
	if src.asked.Load() != 1 {
		t.Errorf("asked %d times right away", src.asked.Load())
	}
	var next int64
	f.store.DB.QueryRow(`SELECT next_try FROM game_meta WHERE item_id = ?`, it.ID).Scan(&next)
	if next <= time.Now().Unix() {
		t.Errorf("next try %d", next)
	}
	for i := 0; i < 12; i++ {
		f.store.DB.Exec(`UPDATE game_meta SET next_try = 0 WHERE status = 'pending'`)
		f.work(t)
	}
	var status string
	f.store.DB.QueryRow(`SELECT status FROM game_meta WHERE item_id = ?`, it.ID).Scan(&status)
	if status != "done" {
		t.Errorf("status %q: it should give up in the end", status)
	}
}

func TestGapFillerOnlyAskedForGaps(t *testing.T) {
	full := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		esrb, _ := gamemeta.NewAge("ESRB", "E")
		return &gamemeta.Info{ID: "1", Method: gamemeta.MethodChecksum, Confidence: 100, Description: "x", Poster: "https://a/p.png", Backdrop: "https://a/b.png", Developer: "M", Genres: []string{"G"},
			Ages: []gamemeta.AgeRating{esrb}, Scores: []gamemeta.Score{{Source: "A", Kind: "user", Value: 50}}}, nil
	}}
	tight := gapSource{&fakeSource{id: "b", answer: func(q gamemeta.Query) (*gamemeta.Info, error) { return nil, gamemeta.ErrNotFound }}}
	f := newMetaFixture(t, full, tight)
	f.scanFile(t, "Game.sfc")
	f.g.Pictures = &http.Client{Transport: roundTripErr{}}
	f.work(t)
	if tight.asked.Load() != 0 {
		t.Errorf("asked %d times with nothing missing", tight.asked.Load())
	}
}

type roundTripErr struct{}

func (roundTripErr) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no network here")
}

func TestOwnersWordsStay(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Title: "Source Title", Method: gamemeta.MethodChecksum, Confidence: 100, Description: "Source words", Developer: "Source maker", Released: "1990", Genres: []string{"Source"}}, nil
	}}
	f := newMetaFixture(t, src)
	// From the owner's NFO, beside the game.
	if err := os.WriteFile(filepath.Join(f.root, "Game.nfo"), []byte(`<movie><title>NFO Title</title><plot>NFO plot</plot></movie>`), 0o644); err != nil {
		t.Fatal(err)
	}
	it := f.scanFile(t, "Game.sfc")
	f.work(t)
	got := f.item(t, it.ID)
	if got.Title != "NFO Title" || got.Overview != "NFO plot" {
		t.Errorf("the NFO lost: %+v", got)
	}
	if got.Studio != "Source maker" {
		t.Errorf("gaps should still fill: %+v", got)
	}
	// By hand, on a game with nothing beside it.
	other := f.scanFile(t, "Other.sfc")
	f.work(t)
	if got := f.item(t, other.ID); got.Overview != "Source words" || got.Title != "Source Title" {
		t.Fatalf("%+v", got)
	}
	if err := f.store.MarkGameEdited(f.ctx, other.ID, "overview"); err != nil {
		t.Fatal(err)
	}
	cur := f.item(t, other.ID)
	cur.Overview = "My own words"
	if err := f.store.upsertItem(f.ctx, cur); err != nil {
		t.Fatal(err)
	}
	if err := f.g.RefetchAll(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if got := f.item(t, other.ID); got.Overview != "My own words" {
		t.Errorf("an edit lost: %q", got.Overview)
	}
}

// Turning a source off takes away what only it said, and asks nobody anything. Turning it
// back on brings it back the same way.
func TestSwitchingASourceOff(t *testing.T) {
	a := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Method: gamemeta.MethodName, Confidence: 92, Description: "From A", Developer: "Maker A", Released: "1994"}, nil
	}}
	b := &fakeSource{id: "b", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "2", Method: gamemeta.MethodName, Confidence: 92, Developer: "Maker B"}, nil
	}}
	f := newMetaFixture(t, a, b)
	it := f.scanFile(t, "Game.sfc")
	f.work(t)
	if got := f.item(t, it.ID); got.Overview != "From A" || got.Studio != "Maker A" || got.Year == nil {
		t.Fatalf("%+v", got)
	}
	if err := f.g.SetSourceOrder(f.ctx, []GameSourceSetting{{"a", false}, {"b", true}}); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if got := f.item(t, it.ID); got.Overview != "" || got.Studio != "Maker B" || got.Year != nil {
		t.Errorf("off: %+v", got)
	}
	if err := f.g.SetSourceOrder(f.ctx, []GameSourceSetting{{"a", true}, {"b", true}}); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if got := f.item(t, it.ID); got.Overview != "From A" || got.Studio != "Maker A" {
		t.Errorf("on again: %+v", got)
	}
	if a.asked.Load() != 1 || b.asked.Load() != 1 {
		t.Errorf("asked %d and %d times", a.asked.Load(), b.asked.Load())
	}
}

func TestLockAndChoose(t *testing.T) {
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "auto", Title: q.Title + " found", Method: gamemeta.MethodName, Confidence: 85, Developer: "Maker of " + q.Title}, nil
	}}
	other := &fakeSource{id: "b", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "o", Method: gamemeta.MethodName, Confidence: 85, Publisher: "Seller of " + q.Title}, nil
	}}
	f := newMetaFixture(t, src, other)
	it := f.scanFile(t, "Wrong Name.sfc")
	f.work(t)
	// Choosing: it applies at once, and the other sources are asked by the new name.
	got := f.item(t, it.ID)
	if err := f.g.Choose(f.ctx, got, "a", "Right Game", true); err != nil {
		t.Fatal(err)
	}
	if got := f.item(t, it.ID); got.Title != "Right Game found" {
		t.Errorf("%q", got.Title)
	}
	d, _ := f.store.GameDetails(f.ctx, it.ID)
	if !d.Locked || d.MatchMethod != gamemeta.MethodManual || d.Confidence != 100 {
		t.Errorf("%+v", d)
	}
	f.work(t)
	d, _ = f.store.GameDetails(f.ctx, it.ID)
	if d.Publisher != "Seller of Right Game found" && d.Publisher != "Seller of Right Game" {
		t.Errorf("publisher %q", d.Publisher)
	}
	// Locked: looking up everything again leaves it alone.
	askedBefore := src.asked.Load()
	if err := f.g.RefetchAll(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if src.asked.Load() != askedBefore {
		t.Error("a locked game was looked up")
	}
	// Asking about it directly keeps the choice.
	if err := f.g.Refresh(f.ctx, f.item(t, it.ID)); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	d, _ = f.store.GameDetails(f.ctx, it.ID)
	if d.MatchMethod != gamemeta.MethodManual {
		t.Errorf("%+v", d)
	}
	// No match: everything sources filled in goes, and it's not found again by itself.
	if err := f.g.ClearMatch(f.ctx, f.item(t, it.ID)); err != nil {
		t.Fatal(err)
	}
	got = f.item(t, it.ID)
	if got.Title != "Wrong Name" || got.Studio != "" || got.Provider != "" {
		t.Errorf("%+v", got)
	}
	if err := f.g.RefetchAll(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.work(t)
	if got := f.item(t, it.ID); got.Studio != "" {
		t.Errorf("a game with no match was matched: %+v", got)
	}
}

func TestPictures(t *testing.T) {
	var hits atomic.Int32
	good := pngBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/good.png", "/back.png", "/shot.png":
			w.Write(good)
		case "/page.png":
			w.Write([]byte("<html><script>alert(1)</script></html>"))
		case "/huge.png":
			w.Write(good)
			w.Write(make([]byte, 21<<20))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Method: gamemeta.MethodName, Confidence: 92,
			Poster: srv.URL + "/missing.png", MorePosters: []string{srv.URL + "/page.png", srv.URL + "/huge.png", srv.URL + "/good.png"}, Backdrop: srv.URL + "/back.png",
			Art: []gamemeta.ArtRef{{Ref: srv.URL + "/shot.png", Kind: "screenshot"}, {Ref: srv.URL + "/missing.png", Kind: "title"}}}, nil
	}}
	f := newMetaFixture(t, src)
	f.g.Pictures = srv.Client()
	it := f.scanFile(t, "Game.sfc")
	f.work(t)
	got := f.item(t, it.ID)
	if !strings.HasPrefix(got.Poster, "cache:game-") || !strings.HasPrefix(got.Backdrop, "cache:game-") {
		t.Fatalf("%q, %q", got.Poster, got.Backdrop)
	}
	b, err := os.ReadFile(filepath.Join(f.g.ImagesDir, strings.TrimPrefix(got.Poster, "cache:")))
	if err != nil || string(b) != string(good) {
		t.Errorf("poster on disk: %v", err)
	}
	// Screenshots wait until someone looks.
	before := hits.Load()
	d, _ := f.store.GameDetails(f.ctx, it.ID)
	if len(d.Art) != 2 || strings.Contains(d.Art[0].URL, srv.URL) {
		t.Errorf("%+v", d.Art)
	}
	if hits.Load() != before {
		t.Error("a screenshot was fetched before anyone looked")
	}
	c, err := f.g.GameArtPicture(f.ctx, it.ID, 0)
	if err != nil || !strings.HasPrefix(c, "cache:") {
		t.Errorf("%q, %v", c, err)
	}
	// And once fetched, only once.
	before = hits.Load()
	if _, err := f.g.GameArtPicture(f.ctx, it.ID, 0); err != nil {
		t.Error(err)
	}
	if hits.Load() != before {
		t.Error("fetched twice")
	}
	if _, err := f.g.GameArtPicture(f.ctx, it.ID, 1); !errors.Is(err, errNoPicture) {
		t.Errorf("a missing picture: %v", err)
	}
	if _, err := f.g.GameArtPicture(f.ctx, it.ID, 99); !errors.Is(err, ErrNotFound) {
		t.Errorf("no such number: %v", err)
	}
	// A picture someone already has beside the game stays.
	if err := os.WriteFile(filepath.Join(f.root, "Mine.png"), good, 0o644); err != nil {
		t.Fatal(err)
	}
	mine := f.scanFile(t, "Mine.sfc")
	f.work(t)
	if got := f.item(t, mine.ID); got.Poster != filepath.Join(f.root, "Mine.png") {
		t.Errorf("%q", got.Poster)
	}
}

// Without a test's client, pictures only come from the internet: an answer can't point
// the server at something on the owner's own network.
func TestPicturesCantReachTheLocalNetwork(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write(pngBytes(t)) }))
	defer srv.Close()
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Method: gamemeta.MethodName, Confidence: 92, Poster: srv.URL + "/inside.png", Art: []gamemeta.ArtRef{{Ref: "file:///etc/passwd", Kind: "screenshot"}}}, nil
	}}
	f := newMetaFixture(t, src)
	it := f.scanFile(t, "Game.sfc")
	f.work(t)
	if hits.Load() != 0 {
		t.Error("a local address was fetched")
	}
	if got := f.item(t, it.ID); got.Poster != "" {
		t.Errorf("%q", got.Poster)
	}
	if _, err := f.g.GameArtPicture(f.ctx, it.ID, 0); err == nil {
		t.Error("a file address was fetched")
	}
}

func TestSavingBesideTheGame(t *testing.T) {
	good := pngBytes(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(good) }))
	defer srv.Close()
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Method: gamemeta.MethodName, Confidence: 92, Description: "Words", Poster: srv.URL + "/p.png"}, nil
	}}
	listFiles := func(root string) []string {
		var out []string
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			out = append(out, e.Name())
		}
		return out
	}
	// Off: nothing appears next to the games, whatever else happens.
	f := newMetaFixture(t, src)
	f.g.Pictures = srv.Client()
	f.g.SaveMeta = func() bool { return false }
	saves := 0
	f.g.Save = func(context.Context, *Item) error { saves++; return nil }
	f.scanFile(t, "Game.sfc")
	f.work(t)
	if files := listFiles(f.root); len(files) != 1 || saves != 0 {
		t.Errorf("wrote %v, %d saves", files, saves)
	}
	// On: the owner's asked for it, under names that belong to this one game.
	g := newMetaFixture(t, src)
	sc := NewScanner(g.store, nil, t.TempDir(), func() string { return "" }, g.g.Log)
	sc.ImagesDir = g.g.ImagesDir
	g.g.Pictures = srv.Client()
	g.g.SaveMeta = func() bool { return true }
	g.g.Save = sc.SaveItemMetadata
	g.scanFile(t, "Game.sfc")
	g.work(t)
	files := strings.Join(listFiles(g.root), "|")
	if !strings.Contains(files, "Game.nfo") {
		t.Errorf("wrote %s", files)
	}
	if strings.Contains(files, "|poster.jpg") || strings.HasPrefix(files, "poster.jpg") {
		t.Errorf("a folder's worth of games must not share a poster: %s", files)
	}
}

func TestArtPathsForGames(t *testing.T) {
	poster, back, nfo := ArtPaths(&Item{Kind: "game", Path: filepath.Join("g", "Mega Game (USA).7z")})
	if filepath.Base(poster) != "Mega Game (USA)-poster.jpg" || filepath.Base(back) != "Mega Game (USA)-fanart.jpg" || filepath.Base(nfo) != "Mega Game (USA).nfo" {
		t.Errorf("%s %s %s", poster, back, nfo)
	}
}

func TestFiltersAndFacets(t *testing.T) {
	info := map[string]*gamemeta.Info{
		"Old Game":  {Released: "1991", Genres: []string{"Action", "Platform"}, Players: 1, Scores: []gamemeta.Score{{Source: "A", Kind: "critic", Value: 90}}},
		"Late Game": {Released: "1999-05", Genres: []string{"Action"}, Players: 4, Scores: []gamemeta.Score{{Source: "A", Kind: "critic", Value: 70}}},
		"New Game":  {Released: "2004", Genres: []string{"Puzzle"}, Players: 2},
	}
	src := &fakeSource{id: "a", answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		in, ok := info[q.Title]
		if !ok {
			return nil, gamemeta.ErrNotFound
		}
		cp := *in
		cp.ID, cp.Method, cp.Confidence = q.Title, gamemeta.MethodName, 92
		return &cp, nil
	}}
	f := newMetaFixture(t, src)
	for name := range info {
		f.scanFile(t, name+".sfc")
	}
	f.scanFile(t, "Unknown Game.sfc")
	f.work(t)
	titles := func(q ItemQuery) []string {
		q.LibraryID = f.lib.ID
		items, total, err := f.store.Items(f.ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, it := range items {
			out = append(out, it.Title)
		}
		if total != len(out) {
			t.Errorf("total %d for %d", total, len(out))
		}
		return out
	}
	if got := titles(ItemQuery{MinRating: 8}); !equal(got, []string{"Old Game"}) {
		t.Errorf("rating: %v", got)
	}
	if got := titles(ItemQuery{Decade: 1990, Sort: "year"}); !equal(got, []string{"Old Game", "Late Game"}) {
		t.Errorf("decade: %v", got)
	}
	if got := titles(ItemQuery{Players: 2, Sort: "title"}); !equal(got, []string{"Late Game", "New Game"}) {
		t.Errorf("players: %v", got)
	}
	if got := titles(ItemQuery{Genre: "Action", Sort: "rating", Desc: true}); !equal(got, []string{"Old Game", "Late Game"}) {
		t.Errorf("genre, by rating: %v", got)
	}
	fc, err := f.store.GameFacets(f.ctx, f.lib.ID, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(fc.Genres) != 3 || fc.Genres[0].Value != "Action" || fc.Genres[0].Count != 2 {
		t.Errorf("genres %+v", fc.Genres)
	}
	if len(fc.Decades) != 2 || fc.Decades[0].Value != "1990" || fc.Decades[0].Count != 2 {
		t.Errorf("decades %+v", fc.Decades)
	}
	if len(fc.Players) != 3 || fc.Rated != 2 {
		t.Errorf("%+v", fc)
	}
	// A profile only counts what it can see.
	limited := Access{MaxAge: NoAgeLimit, BlockGenres: []string{"Action"}, AllowUnrated: true, LiveTV: true, LibraryChannels: true, TunerChannels: true}
	fc, err = f.store.GameFacets(f.ctx, f.lib.ID, "", &limited)
	if err != nil {
		t.Fatal(err)
	}
	if fc.Rated != 0 {
		t.Errorf("hidden games counted: %+v", fc)
	}
	for _, g := range fc.Genres {
		if g.Value == "Action" {
			t.Errorf("a blocked genre listed: %+v", fc.Genres)
		}
	}
}

func TestSourceOrderSetting(t *testing.T) {
	got := GameSourceOrder(`[{"id":"wikidata","on":false},{"id":"nonsense","on":true},{"id":"igdb","on":true},{"id":"igdb","on":false}]`, "1")
	if got[0].ID != "wikidata" || got[0].On || got[1].ID != "igdb" || !got[1].On || len(got) != len(defaultSourceOrder) {
		t.Errorf("%+v", got)
	}
	for _, s := range GameSourceOrder("", "0") {
		if s.ID == "wikidata" && s.On {
			t.Error("someone who turned Wikidata off for films hasn't asked for it here")
		}
	}
	if len(GameSourceOrder("not json", "1")) != len(defaultSourceOrder) {
		t.Error("a broken setting")
	}
}

func TestSourceViewsAndTest(t *testing.T) {
	src := &fakeSource{id: "keyed", fields: []gamemeta.KeyField{{Setting: "rawgKey", Label: "API key"}}, answer: func(q gamemeta.Query) (*gamemeta.Info, error) {
		return &gamemeta.Info{ID: "1", Method: gamemeta.MethodName, Confidence: 92}, nil
	}}
	f := newMetaFixture(t, src)
	views := f.g.SourceViews(f.ctx)
	if len(views) != 1 || views[0].Ready || len(views[0].Saved) != 0 {
		t.Errorf("%+v", views)
	}
	// With no key it isn't used, however it's switched.
	it := f.scanFile(t, "Game.sfc")
	f.work(t)
	if src.asked.Load() != 0 {
		t.Error("asked with no key")
	}
	if msg, err := f.g.TestSource(f.ctx, "keyed", nil); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Errorf("%q, %v", msg, err)
	}
	// A key arrives: only the new source is asked, about games already looked up.
	f.store.DB.SetSetting(f.ctx, "rawgKey", "abc")
	f.g.KeysChanged(f.ctx)
	f.work(t)
	if src.asked.Load() != 1 {
		t.Errorf("asked %d times", src.asked.Load())
	}
	if d, _ := f.store.GameDetails(f.ctx, it.ID); d.MatchSource != "keyed" {
		t.Errorf("%+v", d)
	}
	views = f.g.SourceViews(f.ctx)
	if !views[0].Ready || len(views[0].Saved) != 1 || views[0].Saved[0] != "rawgKey" {
		t.Errorf("%+v", views)
	}
	if msg, err := f.g.TestSource(f.ctx, "keyed", nil); err != nil || msg != "fine" {
		t.Errorf("%q, %v", msg, err)
	}
	if _, err := f.g.TestSource(f.ctx, "nothing", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("%v", err)
	}
}

func TestMeterKeepsCountAcrossRestarts(t *testing.T) {
	f := newMetaFixture(t)
	m := &gamemeta.RAWG{Key: "k"}
	limit, _ := m.Allowance()
	spend := f.g.meterFor("rawg", m)
	if err := spend(limit); err != nil {
		t.Fatal(err)
	}
	spend = NewGameMeta(f.store, "", "", f.g.Log).meterFor("rawg", m)
	var rl *gamemeta.RateLimited
	if err := spend(1); !errors.As(err, &rl) || !rl.Until.After(time.Now()) {
		t.Errorf("%v", err)
	}
	// Next month, it starts again.
	f.store.DB.SetSetting(f.ctx, "gameSourceUse.rawg", toJSON(meterState{Period: "2000-01", Used: limit}))
	if err := spend(1); err != nil {
		t.Errorf("%v", err)
	}
}
