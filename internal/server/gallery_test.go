package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// galleryServer is the whole API on a real database, with no FFmpeg: pictures go
// through everything but making thumbnails.
type galleryServer struct {
	t       *testing.T
	s       *Server
	h       http.Handler
	admin   string
	adult   string
	kid     string
	guest   string
	outside string // an adult who may not open the library
	lib     *library.Library
}

func newGalleryServer(t *testing.T) *galleryServer {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &library.Store{DB: d}
	scanner := library.NewScanner(store, nil, filepath.Join(dir, "cache"), func() string { return "" }, log)
	s := &Server{Cfg: &config.Config{ImagesDir: filepath.Join(dir, "images")}, DB: d, Log: log, Store: store, Scanner: scanner, Auth: &auth.Service{DB: d}}
	ts := &galleryServer{t: t, s: s, h: s.Handler()}
	lib, err := store.CreateLibrary(ctx, "Photos", "photos", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ts.lib = lib
	user := func(name, kind string, admin bool, restrictions string) string {
		if _, err := s.Auth.CreateProfile(ctx, name, "password1", kind, admin, restrictions); err != nil {
			t.Fatal(err)
		}
		tok, _, err := s.Auth.Login(ctx, name, "password1", "test", false)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	ts.admin = user("admin", auth.KindAdult, true, "")
	ts.adult = user("adult", auth.KindAdult, false, "")
	ts.kid = user("kid", auth.KindKids, false, "")
	ts.guest = user("guest", auth.KindGuest, false, "")
	ts.outside = user("outside", auth.KindAdult, false, `{"maxAge":-1,"allowUnrated":true,"libraries":[999]}`)
	return ts
}

func (ts *galleryServer) do(method, path, token string, body any) *httptest.ResponseRecorder {
	ts.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	r := httptest.NewRequest(method, path, rd)
	r.RemoteAddr = "192.168.1.50:4000"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 32, 24)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (ts *galleryServer) upload(token, name string, content []byte) *httptest.ResponseRecorder {
	ts.t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		ts.t.Fatal(err)
	}
	_, _ = fw.Write(content)
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/libraries/"+itoa(ts.lib.ID)+"/upload", &b)
	r.RemoteAddr = "192.168.1.50:4000"
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	return w
}

type uploadReply struct {
	Files []struct {
		Name  string `json:"name"`
		ID    int64  `json:"id"`
		Error string `json:"error"`
	} `json:"files"`
}

func (ts *galleryServer) uploadOK(token, name string) int64 {
	ts.t.Helper()
	w := ts.upload(token, name, tinyJPEG(ts.t))
	var r uploadReply
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if w.Code != 200 || len(r.Files) != 1 || r.Files[0].ID == 0 {
		ts.t.Fatalf("upload %s: %d %s", name, w.Code, w.Body)
	}
	return r.Files[0].ID
}

func TestUploadRules(t *testing.T) {
	ts := newGalleryServer(t)
	id := ts.uploadOK(ts.adult, "picture.jpg")
	// It is now in the library, in the profile's own folder.
	it, err := ts.s.Store.Item(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	rel, _ := filepath.Rel(ts.lib.Paths[0], it.Path)
	if !strings.HasPrefix(filepath.ToSlash(rel), "Uploads/adult/") {
		t.Errorf("saved at %s", rel)
	}
	// A second file of that name doesn't replace the first.
	id2 := ts.uploadOK(ts.adult, "picture.jpg")
	it2, _ := ts.s.Store.Item(context.Background(), id2)
	if it2.Path == it.Path {
		t.Error("an upload replaced a file")
	}
	// A name can't climb out of the library, and only its last part counts.
	id3 := ts.uploadOK(ts.adult, "../../../../escape.jpg")
	it3, _ := ts.s.Store.Item(context.Background(), id3)
	if !ts.s.inLibraries(context.Background(), it3.Path) || filepath.Base(it3.Path) != "escape.jpg" {
		t.Errorf("saved at %s", it3.Path)
	}
	// Files that aren't pictures or videos, whatever they're called.
	for name, content := range map[string][]byte{
		"page.html": []byte("<html><script>alert(1)</script></html>"),
		"fake.jpg":  []byte("<html><script>alert(1)</script></html>"),
		"image.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"run.exe":   []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00\x00\x00"),
		"":          tinyJPEG(t),
	} {
		w := ts.upload(ts.adult, name, content)
		var r uploadReply
		_ = json.Unmarshal(w.Body.Bytes(), &r)
		if len(r.Files) == 1 && r.Files[0].ID != 0 {
			t.Errorf("%q was accepted", name)
		}
	}
	// Who may upload.
	if w := ts.upload(ts.kid, "a.jpg", tinyJPEG(t)); w.Code != 403 {
		t.Errorf("kid: %d", w.Code)
	}
	if w := ts.upload(ts.guest, "a.jpg", tinyJPEG(t)); w.Code != 403 {
		t.Errorf("guest: %d", w.Code)
	}
	if w := ts.upload(ts.outside, "a.jpg", tinyJPEG(t)); w.Code != 404 {
		t.Errorf("an account without this library: %d", w.Code)
	}
	r := httptest.NewRequest("POST", "/api/libraries/"+itoa(ts.lib.ID)+"/upload", nil)
	r.RemoteAddr = "192.168.1.50:4000"
	w := httptest.NewRecorder()
	ts.h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Errorf("no account: %d", w.Code)
	}
	// The owner can turn uploads off.
	_ = ts.s.DB.SetSetting(context.Background(), "uploads", "0")
	if w := ts.upload(ts.adult, "b.jpg", tinyJPEG(t)); w.Code != 403 {
		t.Errorf("uploads off: %d", w.Code)
	}
	_ = ts.s.DB.SetSetting(context.Background(), "uploads", "1")
	// A request larger than the limit stops, leaving nothing half-written.
	_ = ts.s.DB.SetSetting(context.Background(), "uploadMaxMb", "1")
	big := append(tinyJPEG(t), make([]byte, 3<<20)...)
	if w := ts.upload(ts.adult, "big.jpg", big); w.Code == 200 && strings.Contains(w.Body.String(), `"id"`) {
		t.Errorf("an oversized upload was accepted: %d %s", w.Code, w.Body)
	}
	matches, _ := filepath.Glob(filepath.Join(ts.lib.Paths[0], "Uploads", "adult", "*", "big*"))
	if len(matches) != 0 {
		t.Errorf("left behind: %v", matches)
	}
}

func TestGalleryAccess(t *testing.T) {
	ts := newGalleryServer(t)
	id := ts.uploadOK(ts.adult, "picture.jpg")
	idp := itoa(id)
	lib := itoa(ts.lib.ID)
	// Without an account nothing answers.
	for _, p := range []string{"/api/photos/months?library=" + lib, "/api/photos/list?library=" + lib, "/api/photos/" + idp, "/api/items/" + idp + "/original", "/api/videos?library=" + lib, "/api/shares", "/api/items/" + idp + "/comments"} {
		if w := ts.do("GET", p, "", nil); w.Code != 401 {
			t.Errorf("%s without an account: %d", p, w.Code)
		}
	}
	// An account without this library sees none of it, not even that it exists.
	for _, p := range []string{"/api/photos/months?library=" + lib, "/api/photos/list?library=" + lib, "/api/photos/" + idp, "/api/items/" + idp + "/original", "/api/items/" + idp + "/image", "/api/items/" + idp + "/comments"} {
		if w := ts.do("GET", p, ts.outside, nil); w.Code != 404 {
			t.Errorf("%s for an account without the library: %d", p, w.Code)
		}
	}
	// A Kids profile doesn't see unrated pictures.
	if w := ts.do("GET", "/api/photos/"+idp, ts.kid, nil); w.Code != 404 {
		t.Errorf("kid sees an unrated picture: %d", w.Code)
	}
	var list struct {
		Photos []library.Photo `json:"photos"`
	}
	w := ts.do("GET", "/api/photos/list?library="+lib, ts.kid, nil)
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Photos) != 0 {
		t.Errorf("kid lists %d pictures", len(list.Photos))
	}
	// The owner sees it, and lists never say where it is on disk.
	w = ts.do("GET", "/api/photos/list?library="+lib, ts.adult, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), ts.lib.Paths[0]) || strings.Contains(w.Body.String(), "path") {
		t.Errorf("list: %d %s", w.Code, w.Body)
	}
	if w := ts.do("GET", "/api/photos/"+idp, ts.adult, nil); w.Code != 200 || strings.Contains(w.Body.String(), `"path"`) {
		t.Errorf("detail: %d %s", w.Code, w.Body)
	}
	// Originals go with a fixed type, in a sandbox.
	w = ts.do("GET", "/api/items/"+idp+"/original", ts.adult, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("original: %d %v", w.Code, w.Header())
	}
	// Admin-only edits.
	if w := ts.do("PUT", "/api/videos/"+idp, ts.adult, map[string]any{}); w.Code != 403 {
		t.Errorf("custom metadata by a non-admin: %d", w.Code)
	}
	// A library of the wrong kind.
	if w := ts.do("GET", "/api/videos/feed?library="+lib, ts.adult, nil); w.Code != 404 {
		t.Errorf("a feed from a photo library: %d", w.Code)
	}
}

func TestLikesAndComments(t *testing.T) {
	ts := newGalleryServer(t)
	id := ts.uploadOK(ts.adult, "picture.jpg")
	idp := itoa(id)
	if w := ts.do("PUT", "/api/items/"+idp+"/reaction", ts.adult, map[string]any{"value": 5}); w.Code != 400 {
		t.Errorf("a bad reaction: %d", w.Code)
	}
	if w := ts.do("PUT", "/api/items/"+idp+"/reaction", ts.adult, map[string]any{"value": 1}); w.Code != 200 {
		t.Errorf("like: %d", w.Code)
	}
	if w := ts.do("PUT", "/api/items/"+idp+"/reaction", ts.guest, map[string]any{"value": 1}); w.Code != 403 {
		t.Errorf("a guest liked: %d", w.Code)
	}
	if w := ts.do("POST", "/api/items/"+idp+"/comments", ts.kid, map[string]any{"body": "hi"}); w.Code != 403 {
		t.Errorf("a kid commented: %d", w.Code)
	}
	if w := ts.do("POST", "/api/items/"+idp+"/comments", ts.adult, map[string]any{"body": "   "}); w.Code != 400 {
		t.Errorf("an empty comment: %d", w.Code)
	}
	if w := ts.do("POST", "/api/items/"+idp+"/comments", ts.outside, map[string]any{"body": "hi"}); w.Code != 404 {
		t.Errorf("comment from an account without the library: %d", w.Code)
	}
	w := ts.do("POST", "/api/items/"+idp+"/comments", ts.adult, map[string]any{"body": "<script>alert(1)</script> nice"})
	if w.Code != 201 {
		t.Fatalf("comment: %d %s", w.Code, w.Body)
	}
	var c library.Comment
	_ = json.Unmarshal(w.Body.Bytes(), &c)
	cid := itoa(c.ID)
	if w := ts.do("PUT", "/api/comments/"+cid, ts.outside, map[string]any{"body": "mine now"}); w.Code != 404 {
		t.Errorf("someone else edited a comment: %d", w.Code)
	}
	if w := ts.do("DELETE", "/api/comments/"+cid, ts.kid, nil); w.Code != 404 {
		t.Errorf("someone else deleted a comment: %d", w.Code)
	}
	if w := ts.do("DELETE", "/api/comments/"+cid, ts.admin, nil); w.Code != 200 {
		t.Errorf("an admin deleting a comment: %d", w.Code)
	}
}

func TestAlbums(t *testing.T) {
	ts := newGalleryServer(t)
	id := ts.uploadOK(ts.adult, "picture.jpg")
	lib := ts.lib.ID
	if w := ts.do("POST", "/api/photos/albums", ts.adult, map[string]any{"libraryId": lib, "name": "  "}); w.Code != 400 {
		t.Errorf("an album without a name: %d", w.Code)
	}
	if w := ts.do("POST", "/api/photos/albums", ts.kid, map[string]any{"libraryId": lib, "name": "x"}); w.Code != 403 {
		t.Errorf("a kid made an album: %d", w.Code)
	}
	if w := ts.do("POST", "/api/photos/albums", ts.outside, map[string]any{"libraryId": lib, "name": "x"}); w.Code != 404 {
		t.Errorf("an album in a library the account can't open: %d", w.Code)
	}
	w := ts.do("POST", "/api/photos/albums", ts.adult, map[string]any{"libraryId": lib, "name": "Trip"})
	if w.Code != 201 {
		t.Fatalf("album: %d %s", w.Code, w.Body)
	}
	var a library.PhotoAlbum
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	aid := itoa(a.ID)
	if w := ts.do("POST", "/api/photos/albums/"+aid+"/items", ts.adult, map[string]any{"ids": []int64{id}}); w.Code != 200 || !strings.Contains(w.Body.String(), `"added":1`) {
		t.Errorf("add: %d %s", w.Code, w.Body)
	}
	// Someone else's private album is invisible to them.
	if w := ts.do("POST", "/api/photos/albums/"+aid+"/items", ts.guest, map[string]any{"ids": []int64{id}}); w.Code != 404 {
		t.Errorf("another account changed an album: %d", w.Code)
	}
	if w := ts.do("GET", "/api/photos/list?library="+itoa(lib)+"&album="+aid, ts.guest, nil); w.Code != 404 {
		t.Errorf("another account opened a private album: %d", w.Code)
	}
	if w := ts.do("GET", "/api/photos/list?library="+itoa(lib)+"&album="+aid, ts.adult, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "picture") {
		t.Errorf("album list: %d %s", w.Code, w.Body)
	}
	if w := ts.do("DELETE", "/api/photos/albums/"+aid, ts.guest, nil); w.Code != 404 {
		t.Errorf("another account deleted an album: %d", w.Code)
	}
	if w := ts.do("DELETE", "/api/photos/albums/"+aid, ts.adult, nil); w.Code != 200 {
		t.Errorf("delete: %d", w.Code)
	}
	if _, err := ts.s.Store.Item(context.Background(), id); err != nil {
		t.Error("deleting an album must leave its pictures")
	}
}

func TestPublicLinks(t *testing.T) {
	ts := newGalleryServer(t)
	id := ts.uploadOK(ts.adult, "picture.jpg")
	other := ts.uploadOK(ts.adult, "other.jpg")
	if w := ts.do("POST", "/api/shares", ts.kid, map[string]any{"kind": "item", "id": id}); w.Code != 403 {
		t.Errorf("a kid made a link: %d", w.Code)
	}
	if w := ts.do("POST", "/api/shares", ts.outside, map[string]any{"kind": "item", "id": id}); w.Code != 404 {
		t.Errorf("a link to something the account can't see: %d", w.Code)
	}
	w := ts.do("POST", "/api/shares", ts.adult, map[string]any{"kind": "item", "id": id, "days": 7})
	if w.Code != 201 {
		t.Fatalf("link: %d %s", w.Code, w.Body)
	}
	var l library.ShareLink
	_ = json.Unmarshal(w.Body.Bytes(), &l)
	// Without an account: what it names, and nothing else.
	w = ts.do("GET", "/api/share/"+l.Token, "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "picture") {
		t.Fatalf("open: %d %s", w.Code, w.Body)
	}
	for _, leak := range []string{ts.lib.Paths[0], `"path"`, `"lat"`, "other"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Errorf("the link gives away %s: %s", leak, w.Body)
		}
	}
	if w := ts.do("GET", "/api/share/"+l.Token+"/image/"+itoa(other), "", nil); w.Code != 404 {
		t.Errorf("a link opened another picture: %d", w.Code)
	}
	if w := ts.do("GET", "/api/share/"+l.Token+"/original/"+itoa(id), "", nil); w.Code != 404 {
		t.Errorf("originals without permission: %d", w.Code)
	}
	if w := ts.do("GET", "/api/share/wrong-token", "", nil); w.Code != 404 {
		t.Errorf("a wrong token: %d", w.Code)
	}
	if w := ts.do("GET", "/api/items/"+itoa(id)+"/original", l.Token, nil); w.Code != 401 {
		t.Errorf("a link's secret worked as a login: %d", w.Code)
	}
	// Only its maker or an admin can end it.
	if w := ts.do("DELETE", "/api/shares/"+itoa(l.ID), ts.guest, nil); w.Code != 404 {
		t.Errorf("another account ended a link: %d", w.Code)
	}
	// A link stops when its maker loses the picture, or the owner turns links off.
	restricted := `{"maxAge":-1,"allowUnrated":true,"blockItems":[` + itoa(id) + `]}`
	_, _ = ts.s.DB.ExecContext(context.Background(), `UPDATE users SET restrictions = ? WHERE name = 'adult'`, restricted)
	if w := ts.do("GET", "/api/share/"+l.Token, "", nil); w.Code != 404 {
		t.Errorf("a link outlived its maker's access: %d", w.Code)
	}
	_, _ = ts.s.DB.ExecContext(context.Background(), `UPDATE users SET restrictions = '' WHERE name = 'adult'`)
	_ = ts.s.DB.SetSetting(context.Background(), "shareLinks", "0")
	if w := ts.do("GET", "/api/share/"+l.Token, "", nil); w.Code != 404 {
		t.Errorf("a link worked with links off: %d", w.Code)
	}
	_ = ts.s.DB.SetSetting(context.Background(), "shareLinks", "1")
	if w := ts.do("DELETE", "/api/shares/"+itoa(l.ID), ts.adult, nil); w.Code != 200 {
		t.Errorf("end: %d", w.Code)
	}
	if w := ts.do("GET", "/api/share/"+l.Token, "", nil); w.Code != 404 {
		t.Errorf("an ended link opened: %d", w.Code)
	}
	if got := redactPath("/api/share/abcDEF123_-x/image/4"); strings.Contains(got, "abcDEF") {
		t.Errorf("a link's secret in the log: %s", got)
	}
	if got := redactPath("/s/abcDEF123"); strings.Contains(got, "abcDEF") {
		t.Errorf("a link's secret in the log: %s", got)
	}
}

func TestMapTilesSetting(t *testing.T) {
	for v, want := range map[string]bool{"": true, "https://tiles.example/{z}/{x}/{y}.png": true, "http://tiles.example/{z}/{x}/{y}.png": false, "https://tiles.example/map.png": false, "javascript:alert(1)//{z}{x}{y}": false, `https://a/{z}/{x}/{y}"onerror=`: false} {
		if validMapTiles(v) != want {
			t.Errorf("%q: want %v", v, want)
		}
	}
}

func TestOriginalType(t *testing.T) {
	if originalType("x.html") != "application/octet-stream" || originalType("x.svg") != "application/octet-stream" {
		t.Error("a file must never be served as something a browser would run")
	}
	if originalType("x.JPG") != "image/jpeg" {
		t.Error("case")
	}
}
