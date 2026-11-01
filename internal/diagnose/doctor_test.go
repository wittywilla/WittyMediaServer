package diagnose

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/reset"
)

// A real server's database and folders, in a temporary place, with a small library.
type doctorFixture struct {
	t     *testing.T
	env   *DoctorEnv
	media string
	lib   int64
}

func newDoctorFixture(t *testing.T) *doctorFixture {
	t.Helper()
	base := t.TempDir()
	data := filepath.Join(base, "data")
	media := filepath.Join(base, "media")
	for _, d := range []string{data, media, filepath.Join(data, "cache", "images"), filepath.Join(data, "transcode"), filepath.Join(data, "logs")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := &DoctorEnv{Data: data, Cache: filepath.Join(data, "cache"), Images: filepath.Join(data, "cache", "images"), Transcode: filepath.Join(data, "transcode"),
		LogDir: filepath.Join(data, "logs"), Program: filepath.Join(base, "program"), Version: "0.6.0-alpha"}
	t.Cleanup(env.closeDB)
	d, err := db.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.Now().Unix()
	res, err := d.Exec(`INSERT INTO libraries (name, type, created_at) VALUES ('Films', 'movies', ?)`, now)
	if err != nil {
		t.Fatal(err)
	}
	lib, _ := res.LastInsertId()
	if _, err := d.Exec(`INSERT INTO library_paths (library_id, path) VALUES (?, ?)`, lib, media); err != nil {
		t.Fatal(err)
	}
	return &doctorFixture{t: t, env: env, media: media, lib: lib}
}

// item adds a title. withFile says whether its file exists too.
func (f *doctorFixture) item(name string, withFile bool, set string, args ...any) int64 {
	f.t.Helper()
	p := filepath.Join(f.media, name)
	if withFile {
		if err := os.WriteFile(p, []byte("video"), 0o644); err != nil {
			f.t.Fatal(err)
		}
	}
	d, err := db.Open(f.env.Data)
	if err != nil {
		f.t.Fatal(err)
	}
	defer d.Close()
	now := time.Now().Unix()
	res, err := d.Exec(`INSERT INTO items (library_id, kind, path, title, sort_title, added_at, updated_at) VALUES (?, 'video', ?, ?, ?, ?, ?)`, f.lib, p, name, name, now, now)
	if err != nil {
		f.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if set != "" {
		if _, err := d.Exec(`UPDATE items SET `+set+` WHERE id = ?`, append(args, id)...); err != nil {
			f.t.Fatal(err)
		}
	}
	return id
}

func (f *doctorFixture) run(ids ...string) *DoctorReport {
	f.t.Helper()
	checks, err := Selected(ids)
	if err != nil {
		f.t.Fatal(err)
	}
	return RunDoctor(context.Background(), f.env, checks, nil)
}

func (f *doctorFixture) fixAll(r *DoctorReport) []FixResult {
	f.t.Helper()
	var todo []*Issue
	for i := range r.Issues {
		if r.Issues[i].CanFix() {
			todo = append(todo, &r.Issues[i])
		}
	}
	out := ApplyFixes(context.Background(), f.env, todo, "")
	for _, x := range out {
		if !x.OK {
			f.t.Errorf("fix %s failed: %s", x.Key, x.Error)
		}
	}
	return out
}

func (f *doctorFixture) issue(r *DoctorReport, key string) *Issue {
	f.t.Helper()
	i := r.Find(key)
	if i == nil {
		var keys []string
		for _, x := range r.Issues {
			keys = append(keys, x.Key)
		}
		f.t.Fatalf("no issue %s among %v", key, keys)
	}
	return i
}

func old(t *testing.T, p string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(-age)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorFreshDataFolderIsClean(t *testing.T) {
	f := newDoctorFixture(t)
	f.item("Good Film.mkv", true, "")
	r := f.run("data", "metadata")
	if !r.Healthy() {
		t.Fatalf("a healthy server has problems: %s", r.Text())
	}
}

func TestDoctorNoDatabaseIsANewInstall(t *testing.T) {
	f := newDoctorFixture(t)
	for _, n := range []string{"wms.db", "wms.db-wal", "wms.db-shm"} {
		os.Remove(filepath.Join(f.env.Data, n))
	}
	r := f.run("data", "metadata")
	if !r.Healthy() {
		t.Fatalf("a new install has problems: %s", r.Text())
	}
	f.issue(r, "database:new")
}

func TestDoctorEveryIssueSaysWhatAndHowSafe(t *testing.T) {
	f := newDoctorFixture(t)
	f.item("gone.mkv", false, "poster = 'cache:up-aa.jpg', provider = 'tmdb', provider_id = 'abc'")
	if err := os.WriteFile(filepath.Join(f.env.Data, "reset-request.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := f.run()
	if len(r.Issues) == 0 {
		t.Fatal("nothing found")
	}
	for _, i := range r.Issues {
		if i.What == "" || i.Title == "" || i.Safety == "" || i.Key == "" || i.Area == "" {
			t.Errorf("incomplete: %+v", i)
		}
		if !i.NotChecked && i.Severity != SevInfo && i.Fix == "" {
			t.Errorf("no word on the fix: %+v", i)
		}
		if i.Safety == SafetyManual && i.CanFix() {
			t.Errorf("a manual issue says it can be fixed: %s", i.Key)
		}
	}
	if b, err := json.Marshal(r); err != nil || !strings.Contains(string(b), `"safety"`) {
		t.Fatalf("report as JSON: %v %s", err, b)
	}
	if !strings.Contains(r.Text(), "Fix (") {
		t.Fatalf("report as text doesn't say what a fix does:\n%s", r.Text())
	}
}

func TestDoctorOnlySelects(t *testing.T) {
	if _, err := Selected([]string{"nothing-like-this"}); err == nil {
		t.Fatal("an unknown check was accepted")
	}
	list, err := Selected([]string{"modules", "ffmpeg"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.Area != AreaModules && c.ID != "ffmpeg" {
			t.Errorf("%s was not asked for", c.ID)
		}
	}
	all, _ := Selected(nil)
	if len(all) != len(AllChecks()) {
		t.Fatal("no list should mean every check")
	}
}

// ------------------------------------------------------------ metadata

func TestDoctorOrphanedArtwork(t *testing.T) {
	f := newDoctorFixture(t)
	used := filepath.Join(f.env.Images, "up-used.jpg")
	orphan := filepath.Join(f.env.Images, "up-orphan.jpg")
	recent := filepath.Join(f.env.Images, "up-new.jpg")
	remote := filepath.Join(f.env.Images, "r-0123456789")
	logoDir := filepath.Join(f.env.Images, "logos")
	if err := os.MkdirAll(logoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logo := filepath.Join(logoDir, "up-x.png")
	for _, p := range []string{used, orphan, recent, remote, logo} {
		if err := os.WriteFile(p, []byte("pic"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{used, orphan, remote, logo} {
		old(t, p, 72*time.Hour)
	}
	f.item("Film.mkv", true, "poster = 'cache:up-used.jpg'")
	// Something that holds a reference in a column the check has never heard of counts too.
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	histName := filepath.Join(f.env.Images, "up-history.jpg")
	if err := os.WriteFile(histName, []byte("pic"), 0o644); err != nil {
		t.Fatal(err)
	}
	old(t, histName, 72*time.Hour)
	if _, err := d.Exec(`INSERT INTO settings (key, value) VALUES ('someFutureThing', '{"art":"cache:up-history.jpg"}')`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	r := f.run("artwork-orphans")
	i := f.issue(r, "artwork-orphans")
	if i.Safety != SafetySafe || i.Count != 1 || i.Names[0] != "up-orphan.jpg" {
		t.Fatalf("found: %+v", i)
	}
	res := f.fixAll(r)
	if _, err := os.Stat(orphan); err == nil {
		t.Fatal("the orphan is still there")
	}
	for _, p := range []string{used, recent, remote, logo, histName} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s should have been left alone: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(res[0].Copy, "removed", "cache", "images", "up-orphan.jpg")); err != nil {
		t.Fatalf("it should have gone to the safety copy: %v", err)
	}
	if r := f.run("artwork-orphans"); len(r.Issues) != 0 {
		t.Fatalf("still found: %s", r.Text())
	}
}

func TestDoctorOrphansNotJudgedOnADamagedDatabase(t *testing.T) {
	f := newDoctorFixture(t)
	orphan := filepath.Join(f.env.Images, "up-maybe.jpg")
	if err := os.WriteFile(orphan, []byte("pic"), 0o644); err != nil {
		t.Fatal(err)
	}
	old(t, orphan, 72*time.Hour)
	f.env.damaged = "pretend"
	if iss := checkArtOrphans(context.Background(), f.env); len(iss) != 1 || !iss[0].NotChecked {
		t.Fatalf("got %+v", iss)
	}
}

func TestDoctorBrokenArtworkRefs(t *testing.T) {
	f := newDoctorFixture(t)
	here := filepath.Join(f.env.Images, "thumb-here.jpg")
	if err := os.WriteFile(here, []byte("pic"), 0o644); err != nil {
		t.Fatal(err)
	}
	keep := f.item("A.mkv", true, "poster = 'cache:thumb-here.jpg'")
	gone := f.item("B.mkv", true, "poster = 'cache:up-gone.jpg', backdrop = 'cache:up-gone2.jpg'")
	r := f.run("artwork-refs")
	i := f.issue(r, "artwork-refs")
	if i.Count != 2 || i.Safety != SafetySafe {
		t.Fatalf("found: %+v", i)
	}
	f.fixAll(r)
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var p1, p2, b2 string
	if err := d.QueryRow(`SELECT poster FROM items WHERE id = ?`, keep).Scan(&p1); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT poster, backdrop FROM items WHERE id = ?`, gone).Scan(&p2, &b2); err != nil {
		t.Fatal(err)
	}
	if p1 != "cache:thumb-here.jpg" || p2 != "" || b2 != "" {
		t.Fatalf("after: %q %q %q", p1, p2, b2)
	}
}

func TestDoctorAllArtworkGoneIsAFolderProblem(t *testing.T) {
	f := newDoctorFixture(t)
	for n := range 20 {
		f.item(fmt.Sprintf("T%d.mkv", n), true, fmt.Sprintf("poster = 'cache:up-%d.jpg'", n))
	}
	r := f.run("artwork-refs")
	i := f.issue(r, "artwork-refs:folder")
	if i.CanFix() {
		t.Fatal("clearing every picture's reference because a folder is away would be a disaster")
	}
}

func TestDoctorBrokenMatches(t *testing.T) {
	f := newDoctorFixture(t)
	good := f.item("Good.mkv", true, "provider = 'tmdb', provider_id = '123'")
	custom := f.item("Custom.mkv", true, "provider = 'custom:archive', provider_id = 'lx-2012'")
	noID := f.item("NoId.mkv", true, "provider = 'tmdb', provider_id = ''")
	shape := f.item("Shape.mkv", true, "provider = 'imdb', provider_id = 'abc'")
	lone := f.item("Lone.mkv", true, "provider = '', provider_id = '55'")
	r := f.run("matches")
	i := f.issue(r, "matches")
	if i.Count != 3 || i.Safety != SafetyCareful {
		t.Fatalf("found: %+v", i)
	}
	f.fixAll(r)
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	pid := func(id int64) string {
		var v string
		if err := d.QueryRow(`SELECT IFNULL(provider_id, '') FROM items WHERE id = ?`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if pid(good) != "123" || pid(custom) != "lx-2012" {
		t.Fatal("a good match was touched")
	}
	if pid(noID) != "" || pid(shape) != "" || pid(lone) != "" {
		t.Fatal("a broken match stayed")
	}
}

func TestDoctorDuplicates(t *testing.T) {
	f := newDoctorFixture(t)
	first := f.item("Same.mkv", true, "")
	// The same place written with a doubled separator: the unique index can't see it.
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	res, err := d.Exec(`INSERT INTO items (library_id, kind, path, title, sort_title, added_at, updated_at) VALUES (?, 'video', ?, 'Same', 'Same', ?, ?)`, f.lib, f.media+string(filepath.Separator)+string(filepath.Separator)+"Same.mkv", now, now)
	if err != nil {
		t.Fatal(err)
	}
	second, _ := res.LastInsertId()
	if _, err := d.Exec(`INSERT INTO users (id, name, password_hash, created_at) VALUES (1, 'kim', 'x', ?)`, now); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO progress (user_id, item_id, position_sec, duration_sec, updated_at) VALUES (1, ?, 5, 10, ?)`, second, now); err != nil {
		t.Fatal(err)
	}
	// A show twice under one name is only ever reported.
	for _, p := range []string{"A", "B"} {
		if _, err := d.Exec(`INSERT INTO shows (library_id, path, title, sort_title, added_at, updated_at) VALUES (?, ?, 'My Show', 'My Show', ?, ?)`, f.lib, filepath.Join(f.media, p), now, now); err != nil {
			t.Fatal(err)
		}
	}
	d.Close()
	r := f.run("duplicates")
	if i := f.issue(r, "duplicates:shows"); i.CanFix() {
		t.Fatal("two shows with one name can't be merged without asking")
	}
	f.issue(r, "duplicates:same")
	f.fixAll(r)
	d, err = db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var n int
	_ = d.QueryRow(`SELECT COUNT(*) FROM items WHERE id = ?`, second).Scan(&n)
	if n != 0 {
		t.Fatal("the duplicate is still there")
	}
	_ = d.QueryRow(`SELECT COUNT(*) FROM progress WHERE item_id = ?`, first).Scan(&n)
	if n != 1 {
		t.Fatal("its watch progress should have moved to the one that stays")
	}
}

func TestDoctorInvalidNFOAndJSONNameTheLine(t *testing.T) {
	f := newDoctorFixture(t)
	f.item("Bad.mkv", true, "")
	f.item("Link.mkv", true, "")
	f.item("Fine.mkv", true, "")
	nfo := "<movie>\n<title>Fish & Chips</title>\n</movie>\n"
	if err := os.WriteFile(filepath.Join(f.media, "Bad.nfo"), []byte(nfo), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.media, "Bad.metadata.json"), []byte("{\n  \"title\": \"x\",\n  oops\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.media, "Link.nfo"), []byte("https://example.invalid/film/1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.media, "Fine.nfo"), []byte("<movie><title>Fine</title></movie>\nhttps://example.invalid/film/2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(f.media, "Bad.nfo"))
	r := f.run("nfo-files")
	i := f.issue(r, "nfo-files")
	all := strings.Join(i.Details, "\n")
	if i.Count != 2 || !strings.Contains(all, "Bad.nfo: line 2") || !strings.Contains(all, "Bad.metadata.json: line 3") {
		t.Fatalf("found: %s", all)
	}
	if i.CanFix() {
		t.Fatal("this must never write beside anyone's media")
	}
	after, _ := os.ReadFile(filepath.Join(f.media, "Bad.nfo"))
	if string(before) != string(after) {
		t.Fatal("the file changed")
	}
}

func TestDoctorMissingFilesAndAwayRoots(t *testing.T) {
	f := newDoctorFixture(t)
	f.item("Here.mkv", true, "")
	gone := f.item("Gone.mkv", false, "")
	f.item("Podcast", false, "kind = 'radio', path = 'https://example.invalid/stream'")
	r := f.run("missing-files")
	i := f.issue(r, "missing-files")
	if i.Count != 1 || i.Safety != SafetySafe {
		t.Fatalf("found: %+v", i)
	}
	f.fixAll(r)
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	var missing int
	if err := d.QueryRow(`SELECT missing FROM items WHERE id = ?`, gone).Scan(&missing); err != nil || missing != 1 {
		t.Fatalf("missing = %d, %v", missing, err)
	}
	d.Close()

	// A drive that's away says nothing about its files, and nothing is touched.
	away := f.media + "-away"
	if err := os.Rename(f.media, away); err != nil {
		t.Fatal(err)
	}
	r = f.run("missing-files", "library-folders", "nfo-files")
	if r.Find("missing-files") != nil {
		t.Fatalf("a missing drive made its files look gone: %s", r.Text())
	}
	i = f.issue(r, "library-folders:"+hashOf(f.media))
	if i.Input != "remap" || i.CanFix() {
		t.Fatalf("found: %+v", i)
	}
	// An empty folder is what an unmounted drive often looks like.
	if err := os.Mkdir(f.media, 0o755); err != nil {
		t.Fatal(err)
	}
	if r := f.run("missing-files"); r.Find("missing-files") != nil {
		t.Fatalf("an empty folder made its files look gone: %s", r.Text())
	}
}

func TestDoctorRemap(t *testing.T) {
	f := newDoctorFixture(t)
	a := f.item("Film A.mkv", true, "")
	b := f.item("Film B.mkv", true, "")
	ctx := context.Background()
	poster := filepath.Join(f.media, "poster.jpg")
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`UPDATE items SET poster = ? WHERE id = ?`, poster, a); err != nil {
		t.Fatal(err)
	}
	other := "elsewhere"
	if _, err := d.Exec(`UPDATE items SET overview = ? WHERE id = ?`, other, b); err != nil {
		t.Fatal(err)
	}
	// Starts the same but isn't under it: must not change.
	if _, err := d.Exec(`INSERT INTO settings (key, value) VALUES ('similar', ?)`, f.media+"-extra/file"); err != nil {
		t.Fatal(err)
	}
	d.Close()
	moved := f.media + "-moved"
	if err := os.Rename(f.media, moved); err != nil {
		t.Fatal(err)
	}

	// A folder that isn't the same files is refused.
	strange := filepath.Join(filepath.Dir(moved), "strange")
	if err := os.Mkdir(strange, 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanRemap(ctx, f.env, f.media, strange)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CanApply || plan.Why == "" {
		t.Fatalf("plan: %+v", plan)
	}
	if res, err := ApplyRemap(ctx, f.env, f.media, strange); err != nil || res.Applied {
		t.Fatalf("applied to the wrong folder: %+v %v", res, err)
	}
	// Not a library's folder.
	plan, _ = PlanRemap(ctx, f.env, filepath.Join(filepath.Dir(moved), "nope"), moved)
	if plan.CanApply {
		t.Fatalf("plan: %+v", plan)
	}
	// Into the data folder: no.
	if plan, _ = PlanRemap(ctx, f.env, f.media, f.env.Cache); plan.CanApply {
		t.Fatalf("plan: %+v", plan)
	}
	// The real one. A look first changes nothing.
	plan, err = PlanRemap(ctx, f.env, f.media, moved)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.CanApply || plan.Roots != 1 || plan.Found != 2 || plan.Checked != 2 {
		t.Fatalf("plan: %+v", plan)
	}
	res, err := ApplyRemap(ctx, f.env, f.media, moved)
	if err != nil || !res.Applied || res.Copy == "" {
		t.Fatalf("applied: %+v %v", res, err)
	}
	d, err = db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var root, path, art, similar string
	if err := d.QueryRow(`SELECT path FROM library_paths`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT path, poster FROM items WHERE id = ?`, a).Scan(&path, &art); err != nil {
		t.Fatal(err)
	}
	if err := d.QueryRow(`SELECT value FROM settings WHERE key = 'similar'`).Scan(&similar); err != nil {
		t.Fatal(err)
	}
	if root != moved || path != filepath.Join(moved, "Film A.mkv") || art != filepath.Join(moved, "poster.jpg") {
		t.Fatalf("after: %q %q %q", root, path, art)
	}
	if similar != f.media+"-extra/file" {
		t.Fatalf("a path that merely starts the same was changed: %q", similar)
	}
	if r := f.run("library-folders", "missing-files"); len(r.Issues) != 0 {
		t.Fatalf("after moving: %s", r.Text())
	}
}

func TestRemappedAcrossSystems(t *testing.T) {
	cases := []struct{ v, from, to, want string }{
		{`D:\Media\A\b.mkv`, `D:\Media`, `/mnt/media`, `/mnt/media/A/b.mkv`},
		{`/mnt/media/A/b.mkv`, `/mnt/media/`, `D:\New`, `D:\New\A\b.mkv`},
		{`/mnt/media`, `/mnt/media`, `/x`, `/x`},
	}
	for _, c := range cases {
		if got, ok := remapped(c.v, c.from, c.to); !ok || got != c.want {
			t.Errorf("%q: got %q %v", c.v, got, ok)
		}
	}
	if _, ok := remapped(`/mnt/media2/x`, `/mnt/media`, `/y`); ok {
		t.Error("a sibling folder must not match")
	}
}

// ------------------------------------------------------------ data

func corruptPages(t *testing.T, data string, frac float64, pages int) {
	t.Helper()
	p := filepath.Join(data, "wms.db")
	fh, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	st, _ := fh.Stat()
	const pageSize = 4096
	total := st.Size() / pageSize
	bad := make([]byte, pageSize*pages)
	for i := range bad {
		bad[i] = 0xAB
	}
	if _, err := fh.WriteAt(bad, int64(float64(total)*frac)*pageSize); err != nil {
		t.Fatal(err)
	}
}

func fillAndClose(t *testing.T, f *doctorFixture, n int) {
	t.Helper()
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	for i := range n {
		if _, err := tx.Exec(`INSERT INTO items (library_id, kind, path, title, sort_title, overview, added_at, updated_at) VALUES (?, 'video', ?, ?, ?, ?, ?, ?)`,
			f.lib, filepath.Join(f.media, fmt.Sprintf("f%05d.mkv", i)), fmt.Sprintf("Title %d", i), fmt.Sprintf("Title %d", i), strings.Repeat("words ", 40), now, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	d.Close()
}

func TestDoctorRecoversADamagedDatabase(t *testing.T) {
	f := newDoctorFixture(t)
	fillAndClose(t, f, 6000)
	corruptPages(t, f.env.Data, 0.5, 4)

	r := f.run("data")
	i := f.issue(r, "database:damaged")
	if i.Severity != SevProblem || i.Safety != SafetyCareful || !i.Exclusive {
		t.Fatalf("found: %+v", i)
	}
	// Reading through damage decides nothing about artwork.
	if r := f.run("data", "artwork-orphans"); r.Find("meta:unused artwork") == nil {
		t.Fatalf("expected not checked: %s", r.Text())
	}
	r = f.run("database")
	res := f.fixAll(r)
	if len(res) != 1 || !res[0].OK || res[0].Restart {
		t.Fatalf("fix: %+v", res)
	}
	if !strings.Contains(res[0].Done, "rows were saved") {
		t.Fatalf("it should say what was saved: %s", res[0].Done)
	}
	// The damaged file is kept, as it was.
	kept := filepath.Join(res[0].Copy, "replaced", "wms.db")
	if st, err := os.Stat(kept); err != nil || st.Size() == 0 {
		t.Fatalf("the old file wasn't kept: %v", err)
	}
	// And the new one is whole, is what a server opens, and has nearly everything in it.
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatalf("a server couldn't open the result: %v", err)
	}
	defer d.Close()
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 5000 || n >= 6000 {
		t.Fatalf("%d of 6000 rows saved", n)
	}
	var ok string
	if err := d.QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil || ok != "ok" {
		t.Fatalf("integrity: %s %v", ok, err)
	}
	if r := f.run("data"); !r.Healthy() {
		t.Fatalf("after recovery: %s", r.Text())
	}
	if res := reset.LastRepair(f.env.Data); res == nil || !res.OK {
		t.Fatalf("result: %+v", res)
	}
}

func TestDoctorRecoveryInServerWaitsForNextStart(t *testing.T) {
	f := newDoctorFixture(t)
	fillAndClose(t, f, 3000)
	corruptPages(t, f.env.Data, 0.5, 2)
	f.env.InServer = true
	r := f.run("database")
	res := f.fixAll(r)
	if !res[0].Restart {
		t.Fatalf("it should wait for the next start: %+v", res)
	}
	if reset.PendingRepair(f.env.Data) == nil {
		t.Fatal("nothing is waiting")
	}
	if a := reset.ApplyRepair(f.env.Data); a == nil || !a.OK {
		t.Fatalf("next start: %+v", a)
	}
	d, err := db.Open(f.env.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var ok string
	if err := d.QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil || ok != "ok" {
		t.Fatalf("integrity: %s %v", ok, err)
	}
}

func TestDoctorFileThatIsNoDatabase(t *testing.T) {
	f := newDoctorFixture(t)
	for _, n := range []string{"wms.db-wal", "wms.db-shm"} {
		os.Remove(filepath.Join(f.env.Data, n))
	}
	junk := make([]byte, 8192)
	for i := range junk {
		junk[i] = byte(i)
	}
	if err := os.WriteFile(filepath.Join(f.env.Data, "wms.db"), junk, 0o644); err != nil {
		t.Fatal(err)
	}
	r := f.run("data", "metadata")
	i := f.issue(r, "database:unreadable")
	if !i.CanFix() {
		t.Fatalf("found: %+v", i)
	}
	if r.Find("meta:unused artwork") == nil {
		t.Fatalf("expected metadata checks to say not checked: %s", r.Text())
	}
	// Nothing can be read, so nothing gets replaced.
	var todo []*Issue
	todo = append(todo, i)
	res := ApplyFixes(context.Background(), f.env, todo, "")
	if res[0].OK || !strings.Contains(res[0].Error, "wasn't replaced") {
		t.Fatalf("fix: %+v", res)
	}
	b, _ := os.ReadFile(filepath.Join(f.env.Data, "wms.db"))
	if string(b) != string(junk) {
		t.Fatal("the file changed")
	}
}

func TestDoctorForeignKeysAndIndexes(t *testing.T) {
	f := newDoctorFixture(t)
	id := f.item("Film.mkv", true, "")
	d, err := db.OpenExisting(f.env.Data, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	// A connection pool: one connection that has foreign keys off.
	conn, err := d.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO progress (user_id, item_id, position_sec, duration_sec, updated_at) VALUES (99, ?, 1, 2, 3)`, id); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	d.Close()
	r := f.run("database-links")
	i := f.issue(r, "database-links")
	if i.Count != 1 {
		t.Fatalf("found: %+v", i)
	}
	f.fixAll(r)
	if r := f.run("database-links"); len(r.Issues) != 0 {
		t.Fatalf("after: %s", r.Text())
	}
}

func TestDoctorTempFiles(t *testing.T) {
	f := newDoctorFixture(t)
	stray := filepath.Join(f.env.Data, "wms.db.tmp")
	fresh := filepath.Join(f.env.Data, "other.tmp")
	seg := filepath.Join(f.env.Transcode, "stream-1")
	if err := os.WriteFile(stray, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old(t, stray, 72*time.Hour)
	if err := os.WriteFile(fresh, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(seg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seg, "0.ts"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// With a server up, working files are its own.
	f.env.InServer = true
	r := f.run("temp-files")
	if i := f.issue(r, "temp-files"); i.Count != 1 {
		t.Fatalf("found: %+v", i)
	}
	f.env.InServer = false
	r = f.run("temp-files")
	if i := f.issue(r, "temp-files"); i.Count != 2 {
		t.Fatalf("found: %+v", i)
	}
	res := f.fixAll(r)
	if _, err := os.Stat(stray); err == nil {
		t.Fatal("still there")
	}
	if _, err := os.Stat(filepath.Join(res[0].Copy, "removed", "wms.db.tmp")); err != nil {
		t.Fatalf("not in the safety copy: %v", err)
	}
	if _, err := os.Stat(seg); err == nil {
		t.Fatal("converted video is still there")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("a new temp file may still be in use")
	}
}

func TestDoctorResetStates(t *testing.T) {
	f := newDoctorFixture(t)
	if err := reset.Ask(f.env.Data, reset.Request{Mode: reset.ModeKeepCopy, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// Just asked for: the server is on its way to doing it.
	if r := f.run("reset-state"); len(r.Issues) != 0 {
		t.Fatalf("a request in progress was called stale: %s", r.Text())
	}
	old(t, filepath.Join(f.env.Data, "reset-request.json"), time.Hour)
	r := f.run("reset-state")
	if i := f.issue(r, "reset-state:stale"); i.Safety != SafetyCareful {
		t.Fatalf("found: %+v", i)
	}
	f.fixAll(r)
	if reset.Pending(f.env.Data) != nil {
		t.Fatal("still waiting")
	}

	// Half done: the data moved into a copy.
	copyName := "before-reset-20260101-000000"
	if err := os.Mkdir(filepath.Join(f.env.Data, copyName), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(f.env.Data, "wms.db"), filepath.Join(f.env.Data, copyName, "wms.db")); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"wms.db-wal", "wms.db-shm"} {
		os.Remove(filepath.Join(f.env.Data, n))
	}
	r = f.run("reset-state")
	i := f.issue(r, "reset-state:half")
	if i.Severity != SevProblem || !i.Exclusive {
		t.Fatalf("found: %+v", i)
	}
	f.fixAll(r)
	if p := reset.Pending(f.env.Data); p == nil || p.Mode != reset.ModeRestore || p.Copy != copyName {
		t.Fatalf("waiting: %+v", p)
	}
}

// ------------------------------------------------------------ the safety copy

func TestSafetyCopiesAreKeptAndPruned(t *testing.T) {
	f := newDoctorFixture(t)
	root := BackupsDir(f.env.Data)
	for n := range 7 {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("before-repair-2026010%d-000000", n)), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	mine := filepath.Join(root, "something-of-yours")
	if err := os.Mkdir(mine, 0o750); err != nil {
		t.Fatal(err)
	}
	fx := &FixContext{env: f.env}
	if err := fx.ensureCopy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(fx.copyDir), safetyPrefix) {
		t.Fatalf("copy: %s", fx.copyDir)
	}
	if _, err := os.Stat(filepath.Join(fx.copyDir, "wms.db")); err != nil {
		t.Fatalf("no database copy: %v", err)
	}
	if got := SafetyCopies(f.env.Data); len(got) != safetyKeep {
		t.Fatalf("kept %d: %v", len(got), got)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Fatal("something that isn't a safety copy was removed")
	}
}

func TestSafetyCopyOfADamagedDatabaseIsItsFiles(t *testing.T) {
	f := newDoctorFixture(t)
	fillAndClose(t, f, 1000)
	corruptPages(t, f.env.Data, 0.5, 4)
	fx := &FixContext{env: f.env}
	if err := fx.ensureCopy(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Join(fx.copyDir, "wms.db")); err != nil || st.Size() == 0 {
		t.Fatalf("no copy: %v", err)
	}
}

// ------------------------------------------------------------ modules

func TestDoctorModules(t *testing.T) {
	f := newDoctorFixture(t)
	dir := f.env.modulesDir()
	mod := filepath.Join(dir, "wittydvd")
	if err := os.MkdirAll(filepath.Join(mod, "versions", "1.2.3"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mod, "versions", "1.0.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Active has files that were never signed; a work folder from an update that stopped.
	if err := os.WriteFile(filepath.Join(mod, "versions", "1.2.3", "manifest.json"), []byte(`{"id":"wittydvd","name":"WittyDVD","version":"1.2.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(mod, ".work-123")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	old(t, work, 2*time.Hour)
	old(t, filepath.Join(mod, "versions", "1.0.0"), 2*time.Hour)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"wittydvd":{"active":"1.2.3"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := f.run("modules")
	i := f.issue(r, "module-versions:wittydvd:1.2.3")
	if i.Severity != SevProblem || i.Safety != SafetySafe || !i.Exclusive || len(i.Options) != 1 || !i.Options[0].Network {
		t.Fatalf("found: %+v", i)
	}
	leftover := f.issue(r, "module-updates")
	if leftover.Count != 2 {
		t.Fatalf("leftovers: %+v", leftover)
	}
	f.fixAll(r)
	if _, err := os.Stat(work); err == nil {
		t.Fatal("the work folder is still there")
	}
	if _, err := os.Stat(filepath.Join(mod, "versions", "1.2.3")); err == nil {
		t.Fatal("the bad version is still there")
	}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil || strings.Contains(string(b), `"active": "1.2.3"`) {
		t.Fatalf("state: %s %v", b, err)
	}
	if r := f.run("modules"); !r.Healthy() {
		t.Fatalf("after: %s", r.Text())
	}
}

func TestDoctorCorruptModuleState(t *testing.T) {
	f := newDoctorFixture(t)
	dir := f.env.modulesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"truncated":`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := f.run("modules")
	if i := f.issue(r, "module-state"); i.Safety != SafetySafe {
		t.Fatalf("found: %+v", i)
	}
	res := f.fixAll(r)
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err == nil {
		t.Fatal("still there")
	}
	if _, err := os.Stat(filepath.Join(res[0].Copy, "files", "modules", "state.json")); err != nil {
		t.Fatalf("no safety copy of it: %v", err)
	}
}

func TestDoctorExclusiveFixRefusedWithAServerRunning(t *testing.T) {
	f := newDoctorFixture(t)
	if err := os.MkdirAll(filepath.Join(f.env.Data, "crash"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "linux" {
		t.Skip("uses a child process as the pretend server")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("no sleep here")
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	marker := filepath.Join(f.env.Data, "crash", fmt.Sprintf("run-%d.json", cmd.Process.Pid))
	if err := os.WriteFile(marker, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	i := &Issue{Check: "database", Key: "database:damaged", Safety: SafetyCareful, Exclusive: true, Title: "x"}
	res := ApplyFixes(context.Background(), f.env, []*Issue{i}, "")
	if res[0].OK || !strings.Contains(res[0].Error, "while the server is running") {
		t.Fatalf("result: %+v", res)
	}
}

// ------------------------------------------------------------ install

func TestDoctorFFmpegNotRunnable(t *testing.T) {
	f := newDoctorFixture(t)
	f.env.FFmpeg = filepath.Join(f.env.Data, "no-such-ffmpeg")
	f.env.FFprobe = f.env.FFmpeg
	r := f.run("ffmpeg")
	i := f.issue(r, "ffmpeg")
	if i.Severity != SevProblem || i.CanFix() || len(i.Steps) == 0 {
		t.Fatalf("found: %+v", i)
	}
}

func TestDoctorConfigProblems(t *testing.T) {
	f := newDoctorFixture(t)
	f.env.ConfigError = "wms.json: unknown field \"lisen\""
	r := f.run("config-file")
	if i := f.issue(r, "config-file:bad"); i.Severity != SevProblem || !strings.Contains(i.What, "lisen") {
		t.Fatalf("found: %+v", i)
	}
	f.env.ConfigError = ""
	p := filepath.Join(f.env.Data, "wms.json")
	if err := os.WriteFile(p, []byte(`{"ffmpeg":"no/such/ffmpeg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f.env.ConfigFile = p
	r = f.run("config-file")
	f.issue(r, "config-file:ffmpeg")
}

func TestDoctorFolders(t *testing.T) {
	f := newDoctorFixture(t)
	if err := os.RemoveAll(f.env.Transcode); err != nil {
		t.Fatal(err)
	}
	r := f.run("folders")
	if i := f.issue(r, "folders:missing"); i.Safety != SafetySafe {
		t.Fatalf("found: %+v", i)
	}
	f.fixAll(r)
	if st, err := os.Stat(f.env.Transcode); err != nil || !st.IsDir() {
		t.Fatal("not made")
	}
	notDir := filepath.Join(f.env.Data, "afile")
	if err := os.WriteFile(notDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	f.env.LogDir = notDir
	r = f.run("folders")
	f.issue(r, "folders:log")
}

func TestDoctorPortAndFirewall(t *testing.T) {
	f := newDoctorFixture(t)
	f.env.Listen = "nonsense"
	if r := f.run("port"); r.Find("port:address") == nil {
		t.Fatalf("got: %s", r.Text())
	}
	f.env.Listen = "127.0.0.1:9"
	if r := f.run("firewall"); len(r.Issues) != 0 {
		t.Fatalf("on this machine only, a firewall can't matter: %s", r.Text())
	}
	f.env.Listen = ":9797"
	r := f.run("firewall")
	i := f.issue(r, "firewall")
	if i.CanFix() || len(i.Steps) == 0 || i.Severity != SevInfo {
		t.Fatalf("found: %+v", i)
	}
}

func TestDoctorLayout(t *testing.T) {
	f := newDoctorFixture(t)
	f.env.Program = filepath.Dir(f.env.Data)
	r := f.run("layout")
	f.issue(r, "layout:inside-program")
	f.env.Portable = true
	if r := f.run("layout"); len(r.Issues) != 0 {
		t.Fatalf("portable is meant to be like that: %s", r.Text())
	}
}

func TestOnlyOurProcessesAreRecognised(t *testing.T) {
	dir := filepath.FromSlash("/data/modules")
	list := []procInfo{
		{pid: 10, exe: filepath.FromSlash("/data/modules/wittydvd/versions/1.0.0/helper"), cmdline: "helper"},
		{pid: 11, exe: filepath.FromSlash("/usr/bin/helper"), cmdline: "/data/modules/wittydvd"},
		{pid: 12, exe: filepath.FromSlash("/data/modules2/helper")},
	}
	got := helpersOf(list, dir)
	if len(got) != 1 || got[0].pid != 10 {
		t.Fatalf("helpers: %+v", got)
	}
	trans := filepath.FromSlash("/data/transcode")
	ff := []procInfo{
		{pid: 20, exe: filepath.FromSlash("/usr/bin/ffmpeg"), cmdline: "ffmpeg -i x " + trans + "/s1/out.m3u8"},
		{pid: 21, exe: filepath.FromSlash("/usr/bin/ffmpeg"), cmdline: "ffmpeg -i x /home/me/mine.mkv"},
		{pid: 22, exe: filepath.FromSlash("/usr/bin/vlc"), cmdline: "vlc " + trans + "/s1/out.m3u8"},
	}
	got = ffmpegsOf(ff, trans, "")
	if len(got) != 1 || got[0].pid != 20 {
		t.Fatalf("ffmpegs: %+v", got)
	}
}

func TestLeftoverHelperIsEnded(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("copies sleep and reads /proc")
	}
	f := newDoctorFixture(t)
	dir := filepath.Join(f.env.modulesDir(), "wittydvd", "versions", "1.0.0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep")
	}
	exe := filepath.Join(dir, "helper")
	if err := copyFile(sleep, exe); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(exe, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "30")
	if err := cmd.Start(); err != nil {
		t.Skip("can't run a copy here: ", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() { _ = cmd.Process.Kill() }()
	r := f.run("module-helpers")
	if i := f.issue(r, "module-helpers"); i.Safety != SafetySafe || i.Count != 1 {
		t.Fatalf("found: %+v", i)
	}
	f.fixAll(r)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("it's still running")
	}
}
