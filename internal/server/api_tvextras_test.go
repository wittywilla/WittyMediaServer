package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

func TestTVExtrasNeedTheRightAccount(t *testing.T) {
	h, _, kid, _ := tvServer(t)
	adminOnly := []struct{ method, path string }{
		{"GET", "/api/channels/bumpers"}, {"POST", "/api/channels/bumpers"}, {"POST", "/api/channels/bumpers/upload?name=a.mp4"}, {"DELETE", "/api/channels/bumpers/item/1"},
		{"GET", "/api/channels/discs"}, {"POST", "/api/channels/clip-preview"},
	}
	for _, c := range adminOnly {
		if w := tvCall(h, c.method, c.path, kid, "{}"); w.Code != 403 {
			t.Errorf("%s %s as a Kids profile: %d", c.method, c.path, w.Code)
		}
		if w := tvCall(h, c.method, c.path, "", "{}"); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", c.method, c.path, w.Code)
		}
	}
	signedIn := []struct{ method, path string }{
		{"GET", "/api/guide/search?q=abc"}, {"GET", "/api/tv/favourites"}, {"PUT", "/api/tv/favourites/lib-1"}, {"DELETE", "/api/tv/favourites/lib-1"},
		{"GET", "/api/tv/reminders"}, {"POST", "/api/tv/reminders"}, {"DELETE", "/api/tv/reminders/1"},
	}
	for _, c := range signedIn {
		if w := tvCall(h, c.method, c.path, "", "{}"); w.Code != 401 {
			t.Errorf("%s %s signed out: %d", c.method, c.path, w.Code)
		}
	}
}

// One library with a video in it and a disc, and a channel on the video.
func tvWorld(t *testing.T, s *Server) (lib int64, video, disc int64) {
	t.Helper()
	ctx := context.Background()
	l, err := s.Store.CreateLibrary(ctx, "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	add := func(kind, title string, discJSON any) int64 {
		res, err := s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, disc_json, added_at, updated_at) VALUES(?, ?, ?, ?, ?, 3000, ?, 1, 1)`,
			l.ID, kind, "/nowhere/"+title, title, title, discJSON)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	return l.ID, add("video", "A Made Up Film", nil), add("dvd-iso", "A Made Up Disc", `{"mainTitle":1,"scrambled":true,"titles":[{"number":1,"durationSec":3000}]}`)
}

func TestScenesFromEncryptedDiscsAreRefused(t *testing.T) {
	h, admin, _, s := tvServer(t)
	_, _, disc := tvWorld(t, s)
	body := fmt.Sprintf(`{"name":"Scenes","definition":{"mode":"simple","rules":{"clips":[{"id":"a","item":%d}]},"order":{"mode":"sequential"},"filler":{},"alignMin":0}}`, disc)
	w := tvCall(h, "POST", "/api/channels", admin, body)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "encrypted") {
		t.Fatalf("a scene from an encrypted disc: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), strings.Split(dvdsession.EncryptedRefusal, ":")[0]) {
		t.Errorf("should be the usual refusal: %s", w.Body)
	}
	// The picker lists it, so as to say why it can't be used, and has the words.
	w = tvCall(h, "GET", "/api/channels/discs", admin, "")
	var got struct {
		Discs []struct {
			ID        int64 `json:"id"`
			Scrambled bool  `json:"scrambled"`
		} `json:"discs"`
		Refusal string `json:"refusal"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Discs) != 1 || !got.Discs[0].Scrambled || got.Refusal != dvdsession.EncryptedRefusal {
		t.Fatalf("discs: %s", w.Body)
	}
	// Nor can it be tried.
	w = tvCall(h, "POST", "/api/channels/clip-preview", admin, fmt.Sprintf(`{"item":%d}`, disc))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "encrypted") {
		t.Fatalf("trying a scene from an encrypted disc: %d %s", w.Code, w.Body)
	}
}

func TestFavouritesAndRemindersAreEachAccountsOwn(t *testing.T) {
	h, admin, kid, s := tvServer(t)
	_, video, _ := tvWorld(t, s)
	body := fmt.Sprintf(`{"name":"Grown Up","definition":{"mode":"simple","rules":{"items":[%d]},"order":{"mode":"sequential"},"filler":{},"alignMin":0},"visibility":{"mode":"adults"}}`, video)
	w := tvCall(h, "POST", "/api/channels", admin, body)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var c struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	key := fmt.Sprintf("lib-%d", c.ID)

	if w := tvCall(h, "PUT", "/api/tv/favourites/"+key, admin, "{}"); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	// A profile that can't see a channel can't find out about it this way.
	if w := tvCall(h, "PUT", "/api/tv/favourites/"+key, kid, "{}"); w.Code != 404 {
		t.Errorf("favourite of a hidden channel: %d", w.Code)
	}
	if w := tvCall(h, "PUT", "/api/tv/favourites/nonsense", admin, "{}"); w.Code != 404 {
		t.Errorf("favourite of a bad key: %d", w.Code)
	}
	if w := tvCall(h, "GET", "/api/tv/favourites", kid, ""); strings.Contains(w.Body.String(), key) {
		t.Errorf("another account's favourites: %s", w.Body)
	}
	// The guide marks it, and can show only those.
	w = tvCall(h, "GET", "/api/guide?favourites=1&hours=1", admin, "")
	if !strings.Contains(w.Body.String(), `"favourite":true`) || !strings.Contains(w.Body.String(), `"total":1`) {
		t.Errorf("favourites in the guide: %s", w.Body)
	}
	if w := tvCall(h, "DELETE", "/api/tv/favourites/"+key, admin, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w = tvCall(h, "GET", "/api/guide?favourites=1&hours=1", admin, ""); !strings.Contains(w.Body.String(), `"total":0`) {
		t.Errorf("removing a favourite: %s", w.Body)
	}

	start := time.Now().Add(time.Hour).Unix()
	rem := fmt.Sprintf(`{"channel":%q,"title":"A Made Up Film","start":%d,"end":%d}`, key, start, start+600)
	if w := tvCall(h, "POST", "/api/tv/reminders", kid, rem); w.Code != 404 {
		t.Errorf("a reminder on a hidden channel: %d %s", w.Code, w.Body)
	}
	if w := tvCall(h, "POST", "/api/tv/reminders", admin, rem); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := tvCall(h, "GET", "/api/tv/reminders", kid, ""); strings.Contains(w.Body.String(), "Made Up") {
		t.Errorf("another account's reminders: %s", w.Body)
	}
	if w := tvCall(h, "GET", "/api/tv/reminders", admin, ""); !strings.Contains(w.Body.String(), "Made Up") {
		t.Errorf("own reminders: %s", w.Body)
	}
	for _, bad := range []string{
		fmt.Sprintf(`{"channel":%q,"title":"","start":%d,"end":%d}`, key, start, start+1),
		fmt.Sprintf(`{"channel":%q,"title":"x","start":%d,"end":%d}`, key, start, start),
		fmt.Sprintf(`{"channel":%q,"title":"x","start":1000,"end":2000}`, key),
		fmt.Sprintf(`{"channel":%q,"title":%q,"start":%d,"end":%d}`, key, strings.Repeat("x", 301), start, start+1),
	} {
		if w := tvCall(h, "POST", "/api/tv/reminders", admin, bad); w.Code != 400 {
			t.Errorf("reminder %s: %d", bad, w.Code)
		}
	}
	// A search looks through what is coming up, and stays within what can be seen.
	if w := tvCall(h, "GET", "/api/guide/search?q=made&days=2", admin, ""); !strings.Contains(w.Body.String(), "A Made Up Film") {
		t.Errorf("search: %s", w.Body)
	}
	if w := tvCall(h, "GET", "/api/guide/search?q=made&days=2", kid, ""); strings.Contains(w.Body.String(), "A Made Up Film") {
		t.Errorf("search found a channel hidden from this profile: %s", w.Body)
	}
	for _, q := range []string{"q=a", "q=abc&days=99", "q=" + strings.Repeat("a", 101)} {
		if w := tvCall(h, "GET", "/api/guide/search?"+q, admin, ""); w.Code != 400 {
			t.Errorf("search %s: %d", q, w.Code)
		}
	}
}

func TestBumperUploadsAreChecked(t *testing.T) {
	h, admin, _, _ := tvServer(t)
	for name, c := range map[string]struct {
		url, body string
	}{
		"no name":       {"/api/channels/bumpers/upload", "x"},
		"not a video":   {"/api/channels/bumpers/upload?name=a.exe", "x"},
		"up a folder":   {"/api/channels/bumpers/upload?name=../../a.txt", "x"},
		"wrong insides": {"/api/channels/bumpers/upload?name=a.mp4", "this is not a video at all, whatever its name says"},
	} {
		if w := tvCall(h, "POST", c.url, admin, c.body); w.Code != 400 {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
}

// Removing an upload deletes its file, but only ever one in the bumpers folder.
func TestRemovingABumperStaysInItsFolder(t *testing.T) {
	h, admin, _, s := tvServer(t)
	ctx := context.Background()
	lib, err := s.bumperLib(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lib.Type != library.CommercialsType {
		t.Fatalf("library type %q", lib.Type)
	}
	mk := func(path string) int64 {
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, added_at, updated_at) VALUES(?, 'video', ?, 'b', 'b', 5, 1, 1)`, lib.ID, path)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if err := s.Channels.AddBumper(ctx, id, true); err != nil {
			t.Fatal(err)
		}
		return id
	}
	inside := filepath.Join(s.bumperDir(), "mine.mp4")
	outside := filepath.Join(t.TempDir(), "precious.mp4")
	in, out := mk(inside), mk(outside)
	for _, id := range []int64{in, out} {
		if w := tvCall(h, "DELETE", fmt.Sprintf("/api/channels/bumpers/item/%d", id), admin, ""); w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Errorf("an upload's file should go: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("a file outside the bumpers folder must never be deleted: %v", err)
	}
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE id IN (?, ?)`, in, out).Scan(&n)
	if n != 0 {
		t.Errorf("%d items left", n)
	}
	// A second library isn't made for the next upload.
	again, err := s.bumperLib(ctx)
	if err != nil || again.ID != lib.ID {
		t.Errorf("the same library should be used: %v %v", again, err)
	}
}

func TestGuideFiltersByCategoryAndKeepsOldSkins(t *testing.T) {
	h, admin, _, s := tvServer(t)
	_, video, _ := tvWorld(t, s)
	mk := func(name, cats string) {
		body := fmt.Sprintf(`{"name":%q,"categories":%s,"definition":{"mode":"simple","rules":{"items":[%d]},"order":{"mode":"sequential"},"filler":{},"alignMin":0}}`, name, cats, video)
		if w := tvCall(h, "POST", "/api/channels", admin, body); w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	mk("Late Films", `["Late night"]`)
	mk("Anything", `[]`)
	w := tvCall(h, "GET", "/api/guide?group=Late%20night&hours=1", admin, "")
	if !strings.Contains(w.Body.String(), "Late Films") || strings.Contains(w.Body.String(), "Anything") {
		t.Errorf("filter by category: %s", w.Body)
	}
	if !strings.Contains(w.Body.String(), `"groups":["Late night"]`) {
		t.Errorf("the categories should be offered: %s", w.Body)
	}
	if w := tvCall(h, "GET", "/api/guide?channel=nosuch&hours=1", admin, ""); !strings.Contains(w.Body.String(), `"total":0`) {
		t.Errorf("one channel: %s", w.Body)
	}
	if w := tvCall(h, "PUT", "/api/tv/settings", admin, `{"tvSkin":"midnight"}`); w.Code != 200 {
		t.Errorf("midnight colours: %d", w.Code)
	}
	if w := tvCall(h, "PUT", "/api/tv/settings", admin, `{"tvSkin":"neon"}`); w.Code != 400 {
		t.Errorf("unknown colours: %d", w.Code)
	}
	bad := fmt.Sprintf(`{"name":"x","look":{"accent":"url(evil)"},"definition":{"mode":"simple","rules":{"items":[%d]},"order":{"mode":"sequential"},"filler":{},"alignMin":0}}`, video)
	if w := tvCall(h, "POST", "/api/channels", admin, bad); w.Code != 400 {
		t.Errorf("a colour that isn't one: %d", w.Code)
	}
}

// Working out every channel's next two weeks is real work, so only a few run at once.
func TestGuideSearchTakesTurns(t *testing.T) {
	h, admin, _, _ := tvServer(t)
	for range cap(guideSearchSlots) {
		guideSearchSlots <- struct{}{}
	}
	defer func() {
		for range cap(guideSearchSlots) {
			<-guideSearchSlots
		}
	}()
	w := tvCall(h, "GET", "/api/guide/search?q=made", admin, "")
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Errorf("with every place taken: %d %q", w.Code, w.Header().Get("Retry-After"))
	}
}
