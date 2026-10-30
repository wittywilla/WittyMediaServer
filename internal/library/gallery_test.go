package library

import (
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// galleryStore is a real database with two accounts and a library of a kind.
func galleryStore(t *testing.T, typ string) (*Store, *Library) {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	s := &Store{DB: d}
	for _, name := range []string{"a", "b"} {
		if _, err := d.ExecContext(ctx, `INSERT INTO users(name, password_hash, is_admin, created_at) VALUES(?, '', 0, 0)`, name); err != nil {
			t.Fatal(err)
		}
	}
	lib, err := s.CreateLibrary(ctx, "Lib", typ, []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return s, lib
}

func writeJPEG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
}

func TestPairPhotos(t *testing.T) {
	dir := "base"
	c := []candidate{
		{path: filepath.Join(dir, "a.nef"), kind: "photo"},
		{path: filepath.Join(dir, "a.jpg"), kind: "photo"},
		{path: filepath.Join(dir, "b.heic"), kind: "photo"},
		{path: filepath.Join(dir, "b.mov"), kind: "video", size: 1 << 20},
		{path: filepath.Join(dir, "c.mp4"), kind: "video", size: 1 << 20},
		{path: filepath.Join(dir, "d.jpg"), kind: "photo"},
		{path: filepath.Join(dir, "d.mp4"), kind: "video", size: motionClipMax + 1}, // too long to be a live photo
		{path: filepath.Join(dir, "sub", "a.jpg"), kind: "photo"},
	}
	got := map[string]string{}
	for _, x := range pairPhotos(c) {
		got[filepath.ToSlash(x.path)] = x.motion
	}
	want := []string{"base/a.jpg", "base/b.heic", "base/c.mp4", "base/d.jpg", "base/d.mp4", "base/sub/a.jpg"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("%s is missing from %v", w, got)
		}
	}
	if m := got["base/b.heic"]; filepath.ToSlash(m) != "base/b.mov" {
		t.Errorf("the live photo's clip is %q", m)
	}
}

func TestDateFromName(t *testing.T) {
	cases := map[string]string{
		"IMG_20230412_153045.jpg":        "2023-04-12 15:30:45",
		"2021-12-31 23.59.58.png":        "2021-12-31 23:59:58",
		"Screenshot_20190101-000102.jpg": "2019-01-01 00:01:02",
		"holiday 2020-07-04.jpg":         "2020-07-04 00:00:00",
	}
	for name, want := range cases {
		got, ok := DateFromName(name)
		if !ok || time.Unix(got, 0).UTC().Format("2006-01-02 15:04:05") != want {
			t.Errorf("%s: got %v %v", name, time.Unix(got, 0).UTC(), ok)
		}
	}
	for _, name := range []string{"DSC_0001.jpg", "12345678.jpg", "IMG_20231399.jpg"} {
		if _, ok := DateFromName(name); ok {
			t.Errorf("%s has no date", name)
		}
	}
}

func TestSafeFileName(t *testing.T) {
	cases := map[string]string{
		"photo.jpg":           "photo.jpg",
		"../../etc/passwd":    "passwd",
		`..\..\windows\x.jpg`: "x.jpg",
		"C:\\temp\\y.png":     "y.png",
		".hidden.jpg":         "hidden.jpg",
		"bad<>:\"|?*name.jpg": "bad_______name.jpg",
		"con.jpg":             "",
		"..":                  "",
		"  .  ":               "",
		"tab\tname\x00.jpg":   "tab_name_.jpg",
		"trailing.jpg. ":      "trailing.jpg",
	}
	for in, want := range cases {
		if got := SafeFileName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	long := SafeFileName(strings.Repeat("é", 300) + ".jpg")
	if len(long) > 180 || !strings.HasSuffix(long, ".jpg") || strings.ContainsRune(long, '\uFFFD') {
		t.Errorf("long name became %q", long)
	}
}

func TestSniffMedia(t *testing.T) {
	yes := [][]byte{
		[]byte("\xFF\xD8\xFF\xE0xxxxxxxxxxxx"), []byte("\x89PNG\r\n\x1a\nxxxxxxxx"), []byte("GIF89axxxxxxxxxx"),
		[]byte("RIFFxxxxWEBPxxxx"), []byte("\x00\x00\x00\x18ftypheicxxxxxxxx"), []byte("II*\x00xxxxxxxxxxxx"), []byte("\x1A\x45\xDF\xA3xxxxxxxxxxxx"),
	}
	for _, b := range yes {
		if !SniffMedia(b) {
			t.Errorf("%q should pass", b[:8])
		}
	}
	no := []string{"<html><script>alert(1)</script></html>", "<svg xmlns='http://www.w3.org/2000/svg'/>", "#!/bin/sh\nrm -rf /\n", "MZ\x90\x00xxxxxxxxxxxx", "short"}
	for _, b := range no {
		if SniffMedia([]byte(b)) {
			t.Errorf("%q should not pass", b)
		}
	}
}

func TestPhotosEndToEnd(t *testing.T) {
	s, lib := galleryStore(t, "photos")
	ctx := context.Background()
	sc := &Scanner{Store: s, Log: discardLogger()}
	root := lib.Paths[0]
	for i, p := range []string{"2023/IMG_20230412_120000.jpg", "2023/IMG_20230501_120000.jpg", "2020/IMG_20200101_120000.jpg", "IMG_20230413_120000.jpg"} {
		full := filepath.Join(root, filepath.FromSlash(p))
		writeJPEG(t, full, 40+i, 30)
		st, _ := os.Stat(full)
		if err := sc.indexPhoto(ctx, lib, candidate{path: full, kind: "photo", size: st.Size(), mtime: st.ModTime().Unix()}); err != nil {
			t.Fatal(err)
		}
	}
	q := PhotoQuery{LibraryID: lib.ID, UserID: 1}
	months, err := s.PhotoMonths(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	want := []PhotoMonth{{202305, 1}, {202304, 2}, {202001, 1}}
	if len(months) != len(want) {
		t.Fatalf("months: %+v", months)
	}
	for i := range want {
		if months[i] != want[i] {
			t.Errorf("month %d: %+v, want %+v", i, months[i], want[i])
		}
	}
	// Newest first, and paging by cursor visits each exactly once.
	seen := map[int64]bool{}
	var last int64 = 1 << 62
	cursor := ""
	for pages := 0; ; pages++ {
		q.Limit, q.Cursor = 2, cursor
		page, next, err := s.Photos(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range page {
			if seen[p.ID] {
				t.Errorf("%d came twice", p.ID)
			}
			if p.TakenAt > last {
				t.Errorf("out of order at %d", p.ID)
			}
			seen[p.ID], last = true, p.TakenAt
		}
		if next == "" {
			break
		}
		cursor = next
		if pages > 5 {
			t.Fatal("paging doesn't end")
		}
	}
	if len(seen) != 4 {
		t.Errorf("paged %d photos", len(seen))
	}
	// Folders.
	f, err := s.PhotoFolders(ctx, PhotoQuery{LibraryID: lib.ID})
	if err != nil || len(f) != 3 {
		t.Fatalf("folders %v %v", f, err)
	}
	folder := "2023"
	page, _, _ := s.Photos(ctx, PhotoQuery{LibraryID: lib.ID, Folder: &folder})
	if len(page) != 2 {
		t.Errorf("folder has %d", len(page))
	}
	// Albums belong to their owner until shared, and only list what the
	// account may see.
	album, err := s.CreatePhotoAlbum(ctx, lib.ID, 1, "Trip", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PhotoAlbumByID(ctx, album.ID, 2, false); err == nil {
		t.Error("another account can see a private album")
	}
	ids := []int64{page[0].ID, page[1].ID, 99999}
	if n, err := s.AddToPhotoAlbum(ctx, album, ids); err != nil || n != 2 {
		t.Errorf("added %d, %v (a missing picture must be skipped)", n, err)
	}
	blocked := &Access{MaxAge: NoAgeLimit, AllowUnrated: true, BlockItems: []int64{page[0].ID}}
	l, err := s.PhotoAlbums(ctx, lib.ID, 1, blocked)
	if err != nil || len(l) != 1 || l[0].Count != 1 {
		t.Errorf("albums %+v %v", l, err)
	}
	if err := s.UpdatePhotoAlbum(ctx, album.ID, "Trip", "", true, nil); err != nil {
		t.Fatal(err)
	}
	if l, _ := s.PhotoAlbums(ctx, lib.ID, 2, nil); len(l) != 1 {
		t.Errorf("a shared album should be listed for others: %+v", l)
	}
	// A Kids profile with default limits sees no unrated pictures.
	kids := DefaultAccess("kids")
	if p, _, _ := s.Photos(ctx, PhotoQuery{LibraryID: lib.ID, Access: &kids}); len(p) != 0 {
		t.Errorf("kids see %d unrated photos", len(p))
	}
	// Favourites are per account.
	if err := s.SetReaction(ctx, 1, page[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	if p, _, _ := s.Photos(ctx, PhotoQuery{LibraryID: lib.ID, UserID: 1, Favorites: true}); len(p) != 1 || !p[0].Favorite {
		t.Errorf("favourites: %+v", p)
	}
	if p, _, _ := s.Photos(ctx, PhotoQuery{LibraryID: lib.ID, UserID: 2, Favorites: true}); len(p) != 0 {
		t.Errorf("another account's favourites: %+v", p)
	}
}

func TestPhotoClusters(t *testing.T) {
	s, lib := galleryStore(t, "photos")
	ctx := context.Background()
	add := func(name string, la, lo float64) {
		it := &Item{LibraryID: lib.ID, Kind: "photo", Path: filepath.Join(lib.Paths[0], name), Title: name, SortTitle: name}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		if err := s.savePhotoMeta(ctx, it, &photoMeta{TakenAt: 1, Lat: &la, Lon: &lo}); err != nil {
			t.Fatal(err)
		}
	}
	add("a", 52.37, 4.89)
	add("b", 52.38, 4.90)
	add("c", -33.85, 151.21)
	far, err := s.PhotoClusters(ctx, PhotoQuery{LibraryID: lib.ID}, 1)
	if err != nil || len(far) != 2 {
		t.Fatalf("zoomed out: %+v %v", far, err)
	}
	near, err := s.PhotoClusters(ctx, PhotoQuery{LibraryID: lib.ID}, 14)
	if err != nil || len(near) != 3 {
		t.Fatalf("zoomed in: %+v %v", near, err)
	}
	b := [4]float64{52, 4, 53, 5}
	if p, _, _ := s.Photos(ctx, PhotoQuery{LibraryID: lib.ID, BBox: &b}); len(p) != 2 {
		t.Errorf("in the box: %+v", p)
	}
}

func TestComments(t *testing.T) {
	s, lib := galleryStore(t, "channels")
	ctx := context.Background()
	it := &Item{LibraryID: lib.ID, Kind: "video", Path: "/v/a.mp4", Title: "A", SortTitle: "A"}
	other := &Item{LibraryID: lib.ID, Kind: "video", Path: "/v/b.mp4", Title: "B", SortTitle: "B"}
	for _, x := range []*Item{it, other} {
		if err := s.upsertItem(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	if got := CleanComment("  hi\r\nthere\x07 "); got != "hi\nthere" {
		t.Errorf("cleaned to %q", got)
	}
	if CleanComment("   ") != "" || CleanComment(strings.Repeat("x", MaxCommentRunes+1)) != "" {
		t.Error("empty and overlong comments must be refused")
	}
	c, err := s.AddComment(ctx, 1, it.ID, nil, "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddComment(ctx, 2, it.ID, &c.ID, "a reply"); err != nil {
		t.Fatal(err)
	}
	// A reply can't point at a comment on something else.
	if _, err := s.AddComment(ctx, 2, other.ID, &c.ID, "elsewhere"); err == nil {
		t.Error("a reply to another item's comment was accepted")
	}
	likes, when := int64(5), 1.7e9
	if err := s.importComments(ctx, it.ID, []sideComment{{Author: "someone", Text: "from the archive", Timestamp: &when, LikeCount: &likes}}); err != nil {
		t.Fatal(err)
	}
	list, total, err := s.Comments(ctx, it.ID, 0, 0)
	if err != nil || total != 3 {
		t.Fatalf("%d comments, %v", total, err)
	}
	if list[len(list)-1].Author != "someone" || !list[len(list)-1].Imported {
		t.Errorf("imported comments come last with their name: %+v", list)
	}
	// Scanning again replaces imported comments without touching others.
	if err := s.importComments(ctx, it.ID, []sideComment{{Author: "x", Text: "again"}}); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := s.Comments(ctx, it.ID, 0, 0); total != 3 {
		t.Errorf("%d comments after a rescan", total)
	}
	owner, on, err := s.CommentOwner(ctx, c.ID)
	if err != nil || owner != 1 || on != it.ID {
		t.Errorf("owner %d on %d: %v", owner, on, err)
	}
	if err := s.DeleteComment(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := s.Comments(ctx, it.ID, 0, 0); total != 1 {
		t.Errorf("deleting a comment should remove its replies: %d left", total)
	}
}

func TestShareLinks(t *testing.T) {
	s, _ := galleryStore(t, "photos")
	ctx := context.Background()
	past, future := db.Now()-10, db.Now()+1000
	l, err := s.CreateShare(ctx, ShareLink{UserID: 1, Kind: "item", TargetID: 1, Title: "x", ExpiresAt: &future})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Token) < 30 {
		t.Fatalf("a weak token: %q", l.Token)
	}
	var stored string
	if err := s.DB.QueryRowContext(ctx, `SELECT token_hash FROM share_links WHERE id = ?`, l.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, l.Token) {
		t.Error("the secret itself must not be stored")
	}
	if got, err := s.ShareByToken(ctx, l.Token); err != nil || got.ID != l.ID {
		t.Errorf("open: %+v %v", got, err)
	}
	if _, err := s.ShareByToken(ctx, l.Token+"x"); err == nil {
		t.Error("a wrong token opened a link")
	}
	if _, err := s.ShareByToken(ctx, ""); err == nil {
		t.Error("an empty token opened a link")
	}
	old, err := s.CreateShare(ctx, ShareLink{UserID: 1, Kind: "item", TargetID: 1, Title: "y", ExpiresAt: &past})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ShareByToken(ctx, old.Token); err == nil {
		t.Error("an expired link opened")
	}
	if list, _ := s.Shares(ctx, 2, false); len(list) != 0 {
		t.Errorf("another account sees %d links", len(list))
	}
	if err := s.DeleteShare(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ShareByToken(ctx, l.Token); err == nil {
		t.Error("an ended link opened")
	}
}

func TestSidecars(t *testing.T) {
	dir := t.TempDir()
	v := filepath.Join(dir, "My video [abcdef123].mp4")
	write := func(p string, x any) {
		b, _ := json.Marshal(x)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(strings.TrimSuffix(v, ".mp4")+".info.json", map[string]any{"title": "From archive", "uploader": "Maker", "upload_date": "20240531", "tags": []string{"a", "b"}, "view_count": 12, "extractor_key": "SomeSite", "webpage_url": "https://example.com/x"})
	side, _ := sidecarFor(v)
	if side == nil || side.Title != "From archive" || side.Uploader != "Maker" || uploadDate(side.UploadDate) != "2024-05-31" {
		t.Fatalf("archive sidecar: %+v", side)
	}
	// The owner's own file wins, field by field.
	write(v+".metadata.json", map[string]any{"title": "Mine", "platform": "Elsewhere", "tags": []string{"c"}, "custom": map[string]any{"Recorded by": "Sam"}})
	side, _ = sidecarFor(v)
	if side.Title != "Mine" || side.Platform != "Elsewhere" || side.Uploader != "Maker" || len(side.Tags) != 1 || side.Custom["Recorded by"] != "Sam" {
		t.Errorf("merged: %+v", side)
	}
	if got := cleanTitle("My video [abcdef123]"); got != "My video" {
		t.Errorf("clean title %q", got)
	}
	if got := cleanTitle("some_video_name"); got != "some video name" {
		t.Errorf("clean title %q", got)
	}
	if safeWebURL("javascript:alert(1)") != "" || safeWebURL("data:text/html,x") != "" || safeWebURL("https://a.example/") == "" {
		t.Error("only web addresses may become links")
	}
	if got := platformName("SomeSiteTab", ""); got != "SomeSite" {
		t.Errorf("platform %q", got)
	}
	if got := platformName("generic", "GenericThing"); got != "GenericThing" {
		t.Errorf("platform %q", got)
	}
	tags := cleanTags([]string{"#Cats", "cats", " dogs|x ", ""})
	if len(tags) != 2 || tags[0] != "Cats" || tags[1] != "dogs x" {
		t.Errorf("tags %v", tags)
	}
}

func TestCreatorFromPath(t *testing.T) {
	root := filepath.FromSlash("/lib/a")
	lib := &Library{Paths: []string{root}}
	who, list, dir := creatorFromPath(lib, filepath.Join(root, "Maker", "Series", "v.mp4"))
	if who != "Maker" || list != "Series" || dir != filepath.Join(root, "Maker") {
		t.Errorf("%q %q %q", who, list, dir)
	}
	if who, list, _ := creatorFromPath(lib, filepath.Join(root, "v.mp4")); who != "" || list != "" {
		t.Errorf("a file at the top has no creator: %q %q", who, list)
	}
}

// One person: a video with a sidecar and one without land on the same creator,
// in either order.
func TestCreatorsMerge(t *testing.T) {
	s, lib := galleryStore(t, "channels")
	ctx := context.Background()
	cs := newCreatorScan()
	folder := filepath.Join(lib.Paths[0], "Maker")
	a, err := s.ensureCreator(ctx, lib.ID, creatorRow{Name: "Maker", Folder: folder}, cs)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.ensureCreator(ctx, lib.ID, creatorRow{Key: "uc123", Name: "Maker", Folder: folder}, cs)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("two creators for one person: %d, %d", a, b)
	}
	c, err := s.ensureCreator(ctx, lib.ID, creatorRow{Name: "Someone else"}, cs)
	if err != nil || c == a {
		t.Errorf("a different creator: %d %v", c, err)
	}
}

func TestVideosAndEdits(t *testing.T) {
	s, lib := galleryStore(t, "channels")
	ctx := context.Background()
	cs := newCreatorScan()
	maker, err := s.ensureCreator(ctx, lib.ID, creatorRow{Name: "Maker"}, cs)
	if err != nil {
		t.Fatal(err)
	}
	add := func(name, date string, tags []string, vertical bool, rating string) *Item {
		it := &Item{LibraryID: lib.ID, Kind: "video", Path: "/v/" + name, Title: name, SortTitle: name, ContentRating: rating}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		if err := s.saveVideoMeta(ctx, it, &videoMeta{CreatorID: maker, UploadDate: date, Tags: tags, Vertical: vertical, OrigViews: 10}); err != nil {
			t.Fatal(err)
		}
		return it
	}
	a := add("old", "2020-01-01", []string{"x"}, false, "PG")
	b := add("new", "2024-01-01", []string{"y"}, false, "R")
	c := add("tall", "2022-01-01", []string{"x", "y"}, true, "G")
	list := func(q VideoQuery) []int64 {
		t.Helper()
		q.LibraryID, q.UserID = lib.ID, 1
		v, _, err := s.Videos(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		out := []int64{}
		for _, x := range v {
			out = append(out, x.ID)
		}
		return out
	}
	eq := func(got []int64, want ...int64) {
		t.Helper()
		if len(got) != len(want) {
			t.Errorf("got %v, want %v", got, want)
			return
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("got %v, want %v", got, want)
				return
			}
		}
	}
	eq(list(VideoQuery{}), b.ID, c.ID, a.ID)
	eq(list(VideoQuery{Sort: "oldest"}), a.ID, c.ID, b.ID)
	eq(list(VideoQuery{Tag: "x"}), c.ID, a.ID)
	yes := true
	eq(list(VideoQuery{Vertical: &yes}), c.ID)
	eq(list(VideoQuery{Search: "tall"}), c.ID)
	eq(list(VideoQuery{Subscribed: true}))
	if err := s.Subscribe(ctx, 1, maker, true); err != nil {
		t.Fatal(err)
	}
	eq(list(VideoQuery{Subscribed: true}), b.ID, c.ID, a.ID)
	// Profile limits.
	kids := DefaultAccess("kids")
	eq(list(VideoQuery{Access: &kids}), c.ID)
	// What was watched, newest first.
	_ = s.RecordView(ctx, 1, a.ID, ViewEvent{Seconds: 5})
	_, _ = s.DB.ExecContext(ctx, `UPDATE item_stats SET last_at = last_at - 100 WHERE item_id = ?`, a.ID)
	_ = s.RecordView(ctx, 1, b.ID, ViewEvent{Seconds: 5, Completed: true})
	eq(list(VideoQuery{History: true}), b.ID, a.ID)
	// Likes count for everyone, reaction is this account's.
	_ = s.SetReaction(ctx, 2, a.ID, 1)
	v, _, _ := s.Videos(ctx, VideoQuery{LibraryID: lib.ID, UserID: 1, IDs: []int64{a.ID}})
	if v[0].Likes != 1 || v[0].Reaction != 0 || v[0].Views != 1 {
		t.Errorf("card: %+v", v[0])
	}
	// Custom metadata is kept and checked.
	platform, date := "Somewhere", "2021-02-03"
	if err := s.EditVideo(ctx, a, VideoEdit{Platform: &platform, Date: &date, Custom: map[string]any{"k": "v"}}); err != nil {
		t.Fatal(err)
	}
	m, err := s.videoMetaOf(ctx, a.ID)
	if err != nil || m.Platform != "Somewhere" || !m.Edited || !strings.Contains(m.Custom, `"k"`) || m.UploadDate != "2021-02-03" {
		t.Errorf("after edit: %+v %v", m, err)
	}
	bad := "javascript:alert(1)"
	if err := s.EditVideo(ctx, a, VideoEdit{SourceURL: &bad}); err == nil {
		t.Error("a script address was accepted as a source")
	}
	worse := "not a date"
	if err := s.EditVideo(ctx, a, VideoEdit{Date: &worse}); err == nil {
		t.Error("a bad date was accepted")
	}
	// Creators with nothing the account may see stay hidden.
	cq := CreatorQuery{LibraryID: lib.ID, UserID: 1, Access: &Access{MaxAge: NoAgeLimit, AllowUnrated: true, BlockItems: []int64{a.ID, b.ID, c.ID}}}
	if l, err := s.Creators(ctx, cq); err != nil || len(l) != 0 {
		t.Errorf("creators: %+v %v", l, err)
	}
	cq.Access = nil
	if l, err := s.Creators(ctx, cq); err != nil || len(l) != 1 || !l[0].Subscribed || l[0].Videos != 3 {
		t.Errorf("creators: %+v %v", l, err)
	}
	// Related: not itself, within the library.
	rel, err := s.RelatedVideos(ctx, VideoQuery{UserID: 1}, a, 10)
	if err != nil || len(rel) != 2 {
		t.Fatalf("related: %+v %v", rel, err)
	}
	for _, r := range rel {
		if r.ID == a.ID {
			t.Error("a video is related to itself")
		}
	}
}

func TestRankFeed(t *testing.T) {
	const now = int64(1_800_000_000)
	day := int64(86400)
	cands := []feedCandidate{
		{ID: 1, Creator: 10, CreatorN: "Cats", Tags: []string{"cats"}, Added: now - 100*day},
		{ID: 2, Creator: 20, CreatorN: "Bakes", Tags: []string{"food"}, Added: now - 100*day},
		{ID: 3, Creator: 30, CreatorN: "Run", Tags: []string{"running"}, Added: now - 100*day},
		{ID: 4, Creator: 10, CreatorN: "Cats", Tags: []string{"cats"}, Added: now - 100*day},
		{ID: 5, Creator: 40, CreatorN: "Other", Tags: []string{"other"}, Added: now - 100*day, Reaction: -1},
		{ID: 6, Creator: 50, CreatorN: "Seen", Tags: []string{"cats"}, Added: now - 100*day, Plays: 3, Completes: 3, LastAt: now - day},
	}
	likesCats := buildTaste([]tasteEvent{{Tags: []string{"cats"}, Creator: 10, Weight: tasteLike, At: now - day}, {Tags: []string{"cats"}, Creator: 10, Weight: tasteLike, At: now - day}}, now)
	ranked := rankFeed(cands, likesCats, 42, now)
	if ranked[0].Creator != 10 {
		t.Errorf("a cat lover should start with cats: %+v", ranked[0])
	}
	for _, r := range ranked {
		if r.ID == 5 {
			t.Error("a video the account disliked must be left out")
		}
	}
	if ranked[len(ranked)-1].ID != 6 {
		t.Errorf("seen videos should sink to the end, got %d last", ranked[len(ranked)-1].ID)
	}
	if ranked[0].Reason == "" {
		t.Error("a recommendation should say why")
	}
	// The same seed gives the same order; pages stay stable.
	again := rankFeed(cands, likesCats, 42, now)
	for i := range ranked {
		if ranked[i].ID != again[i].ID {
			t.Fatalf("order changed at %d", i)
		}
	}
	// Someone new still gets everything but what they disliked, with no taste needed.
	cold := rankFeed(cands, buildTaste(nil, now), 7, now)
	if len(cold) != 5 {
		t.Errorf("cold start: %d videos", len(cold))
	}
	// Old likes fade.
	old := buildTaste([]tasteEvent{{Tags: []string{"cats"}, Weight: 1, At: now - 365*day}}, now)
	newer := buildTaste([]tasteEvent{{Tags: []string{"cats"}, Weight: 1, At: now - day}}, now)
	if old.tags["cats"] >= newer.tags["cats"] {
		t.Errorf("old taste %f isn't fainter than new %f", old.tags["cats"], newer.tags["cats"])
	}
	// One creator doesn't fill the screen when others are there to show.
	var many []feedCandidate
	for i := int64(1); i <= 6; i++ {
		many = append(many, feedCandidate{ID: i, Creator: 1, CreatorN: "A", Tags: []string{"x"}, Added: now})
	}
	many = append(many, feedCandidate{ID: 7, Creator: 2, CreatorN: "B", Added: now - 400*day}, feedCandidate{ID: 8, Creator: 3, CreatorN: "C", Added: now - 400*day})
	mixed := rankFeed(many, buildTaste([]tasteEvent{{Tags: []string{"x"}, Creator: 1, Weight: 9, At: now}}, now), 1, now)
	for i := 0; i+1 < 4; i++ {
		if mixed[i].Creator == mixed[i+1].Creator {
			t.Errorf("same creator twice at %d: %d, %d", i, mixed[i].ID, mixed[i+1].ID)
		}
	}
	if len(mixed) != 8 {
		t.Errorf("diversifying lost videos: %d", len(mixed))
	}
}

func TestForYouPagesStable(t *testing.T) {
	s, lib := galleryStore(t, "shorts")
	ctx := context.Background()
	cs := newCreatorScan()
	for i := 0; i < 30; i++ {
		id, err := s.ensureCreator(ctx, lib.ID, creatorRow{Name: string(rune('A' + i%3))}, cs)
		if err != nil {
			t.Fatal(err)
		}
		it := &Item{LibraryID: lib.ID, Kind: "video", Path: filepath.Join("v", string(rune('a'+i))+".mp4"), Title: "v", SortTitle: "v"}
		if err := s.upsertItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		if err := s.saveVideoMeta(ctx, it, &videoMeta{CreatorID: id, Tags: []string{"t"}}); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[int64]bool{}
	for offset := 0; ; offset += 10 {
		page, more, err := s.ForYou(ctx, VideoQuery{LibraryID: lib.ID, UserID: 1, Limit: 10, Offset: offset}, 99)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range page {
			if seen[v.ID] {
				t.Errorf("%d came twice", v.ID)
			}
			seen[v.ID] = true
		}
		if !more {
			break
		}
	}
	if len(seen) != 30 {
		t.Errorf("saw %d of 30", len(seen))
	}
	// Someone else's taste doesn't reach this account; their dislikes stay theirs.
	page, _, _ := s.ForYou(ctx, VideoQuery{LibraryID: lib.ID, UserID: 1, Limit: 1}, 99)
	if err := s.SetReaction(ctx, 2, page[0].ID, -1); err != nil {
		t.Fatal(err)
	}
	again, _, _ := s.ForYou(ctx, VideoQuery{LibraryID: lib.ID, UserID: 1, Limit: 1}, 99)
	if again[0].ID != page[0].ID {
		t.Error("another account's dislike changed this account's feed")
	}
	kids := DefaultAccess("kids")
	if p, _, _ := s.ForYou(ctx, VideoQuery{LibraryID: lib.ID, UserID: 1, Access: &kids, Limit: 10}, 99); len(p) != 0 {
		t.Errorf("kids' feed has %d unrated videos", len(p))
	}
}

func TestThumbWidthsAndShrink(t *testing.T) {
	for w, want := range map[int]int{1: 160, 160: 160, 161: 320, 2000: 2560, 99999: 2560} {
		if got := SnapThumbWidth(w); got != want {
			t.Errorf("%d: got %d", w, got)
		}
	}
	for _, c := range []struct {
		name          string
		long, w, want int
	}{{"a.jpg", 6000, 480, 3}, {"a.jpg", 1600, 480, 1}, {"a.jpg", 900, 480, 0}, {"a.png", 6000, 480, 0}, {"a.jpg", 0, 480, 0}, {"a.JPEG", 2000, 480, 2}} {
		if got := jpegShrink(c.name, c.long, c.w); got != c.want {
			t.Errorf("%s %d->%d: got %d, want %d", c.name, c.long, c.w, got, c.want)
		}
	}
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
