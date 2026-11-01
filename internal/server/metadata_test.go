package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func metaItem(t *testing.T, ts *testServer, lib int64, title string) *library.Item {
	t.Helper()
	dir := t.TempDir()
	return ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(dir, title+".mkv"), Title: title, Poster: "cache:thumb-x.jpg", Genres: []string{"Comedy"}})
}

// The edit routes are for administrators, and what a person types stays theirs.
func TestEditingATitle(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	lib := ts.library(t, "movies", t.TempDir())
	it := metaItem(t, ts, lib, "A Quiet Pier")
	base := "/api/meta/item/" + strconv.FormatInt(it.ID, 10)
	if w := ts.do(t, "GET", base, kid, nil); w.Code != http.StatusForbidden {
		t.Errorf("a kid reading the editor: %d", w.Code)
	}
	if w := ts.do(t, "PUT", base, kid, map[string]any{"fields": map[string]string{"title": "x"}}); w.Code != http.StatusForbidden {
		t.Errorf("a kid editing: %d", w.Code)
	}
	w := ts.do(t, "PUT", base, admin, map[string]any{"fields": map[string]string{"title": "  My Own Name ", "tags": "Box set|box SET|Rainy day"}})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var r struct {
		Editor struct {
			Name   string `json:"name"`
			Fields map[string]struct {
				Value     string `json:"value"`
				Source    string `json:"source"`
				Locked    bool   `json:"locked"`
				CanRevert bool   `json:"canRevert"`
			} `json:"fields"`
		} `json:"editor"`
		Batch string `json:"batch"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	f := r.Editor.Fields
	if r.Editor.Name != "My Own Name" || f["title"].Source != "you" || !f["title"].Locked || !f["title"].CanRevert {
		t.Errorf("%+v", f["title"])
	}
	if f["tags"].Value != "Box set|Rainy day" {
		t.Errorf("tags: %q", f["tags"].Value)
	}
	if w := ts.do(t, "POST", "/api/meta/undo", admin, map[string]string{"batch": r.Batch}); w.Code != 200 {
		t.Fatalf("undo: %d %s", w.Code, w.Body)
	}
	if got, _ := ts.Store.Item(t.Context(), it.ID); got.Title != "A Quiet Pier" {
		t.Errorf("after undo: %q", got.Title)
	}
	for _, bad := range []map[string]string{{"year": "banana"}, {"rating": "99"}, {"title": ""}, {"nonsense": "x"}, {"poster": "/etc/passwd"}, {"banner": `C:\Windows\win.ini`}} {
		if w := ts.do(t, "PUT", base, admin, map[string]any{"fields": bad}); w.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, w.Code)
		}
	}
	if w := ts.do(t, "GET", "/api/meta/nothing/1", admin, nil); w.Code != 404 {
		t.Errorf("an unknown kind of title: %d", w.Code)
	}
}

// An upload has to be a picture, whatever it calls itself.
func TestArtUploadIsChecked(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	lib := ts.library(t, "movies", t.TempDir())
	it := metaItem(t, ts, lib, "A Quiet Pier")
	base := "/api/meta/item/" + strconv.FormatInt(it.ID, 10) + "/art/"
	for name, body := range map[string][]byte{"svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), "html": []byte("<html><script>alert(1)</script>")} {
		if w := ts.do(t, "PUT", base+"poster", admin, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	m := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	m.SetNRGBA(1, 1, color.NRGBA{1, 2, 3, 255})
	var pic bytes.Buffer
	if err := png.Encode(&pic, m); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "PUT", base+"nonsense", admin, pic.Bytes()); w.Code != http.StatusBadRequest {
		t.Errorf("an unknown kind of art: %d", w.Code)
	}
	if w := ts.do(t, "PUT", base+"poster", admin, pic.Bytes()); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	got, _ := ts.Store.Item(t.Context(), it.ID)
	if !strings.HasPrefix(got.Poster, "cache:up-") {
		t.Errorf("%q", got.Poster)
	}
	if meta := ts.Store.TitleMeta(t.Context(), library.TargetItem, it.ID); !meta.Locks["poster"] {
		t.Error("an upload should lock the poster")
	}
}

// Consent is the owner's, and a viewer can't make the server look names up.
func TestLogoRoutes(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, viewer := ts.user(t, "viewer", "adult", false, "")
	if w := ts.do(t, "PUT", "/api/logos/consent", viewer, map[string]bool{"accepted": true}); w.Code != http.StatusForbidden {
		t.Errorf("a viewer agreeing: %d", w.Code)
	}
	w := ts.do(t, "POST", "/api/logos/lookup", viewer, map[string]any{"logos": []map[string]string{{"kind": "studio", "name": "Lantern Works"}, {"kind": "nonsense", "name": "x"}}})
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	var got struct {
		Logos []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			Has  bool   `json:"has"`
		} `json:"logos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Logos) != 1 || got.Logos[0].ID != 0 || got.Logos[0].Has {
		t.Errorf("a viewer's lookup: %+v", got.Logos)
	}
	if c, _ := ts.Store.LogoCounts(t.Context()); c[library.LogoPending] != 0 {
		t.Errorf("a viewer's lookup added a name: %+v", c)
	}
	many := make([]map[string]string, 201)
	for i := range many {
		many[i] = map[string]string{"kind": "studio", "name": "x"}
	}
	if w := ts.do(t, "POST", "/api/logos/lookup", admin, map[string]any{"logos": many}); w.Code != http.StatusBadRequest {
		t.Errorf("too many names: %d", w.Code)
	}
	w = ts.do(t, "POST", "/api/logos/lookup", admin, map[string]any{"logos": []map[string]string{{"kind": "studio", "name": "Lantern Works"}}})
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Logos) != 1 || got.Logos[0].ID == 0 {
		t.Fatalf("%+v", got.Logos)
	}
	id := strconv.FormatInt(got.Logos[0].ID, 10)
	if w := ts.do(t, "GET", "/api/logos/"+id+"/candidates", admin, nil); w.Code != http.StatusConflict {
		t.Errorf("candidates without agreement: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/logos/preview?src=http://169.254.169.254/latest", admin, nil); w.Code != http.StatusBadRequest {
		t.Errorf("a preview of anything but Wikimedia: %d", w.Code)
	}
	if w := ts.do(t, "GET", "/api/logos/"+id+"/image", viewer, nil); w.Code != 404 {
		t.Errorf("no picture yet: %d", w.Code)
	}
	m := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	m.SetNRGBA(3, 3, color.NRGBA{9, 9, 9, 255})
	var pic bytes.Buffer
	if err := png.Encode(&pic, m); err != nil {
		t.Fatal(err)
	}
	if w := ts.do(t, "PUT", "/api/logos/"+id+"/upload", viewer, pic.Bytes()); w.Code != http.StatusForbidden {
		t.Errorf("a viewer uploading: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/logos/"+id+"/upload", admin, []byte("<svg/>")); w.Code != http.StatusBadRequest {
		t.Errorf("not a picture: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/logos/"+id+"/upload", admin, pic.Bytes()); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	w = ts.do(t, "GET", "/api/logos/"+id+"/image?variant=white", viewer, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("%d %v", w.Code, w.Header())
	}
}

// Listings need every statement accepted, and the words on it are the server's to require.
func TestListingRoutes(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, viewer := ts.user(t, "viewer", "adult", false, "")
	if w := ts.do(t, "PUT", "/api/listings/settings", viewer, map[string]any{"enabled": true}); w.Code != http.StatusForbidden {
		t.Errorf("a viewer: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/listings/settings", admin, map[string]any{"enabled": true, "accepted": []string{"not-streaming"}}); w.Code != http.StatusBadRequest {
		t.Errorf("not everything accepted: %d", w.Code)
	}
	if ts.Store.ListingsEnabled(t.Context()) {
		t.Error("on")
	}
	// Asking for a service with listings off finds nothing, rather than an error or everything.
	lib := ts.library(t, "movies", t.TempDir())
	metaItem(t, ts, lib, "A Quiet Pier")
	w := ts.do(t, "GET", "/api/browse?platform=streamly&library="+strconv.FormatInt(lib, 10), viewer, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"total":0`) {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

// A saved view is its owner's alone.
func TestViewsBelongToTheirOwner(t *testing.T) {
	ts := newTestServer(t)
	_, a := ts.user(t, "one", "adult", false, "")
	_, b := ts.user(t, "two", "adult", false, "")
	lib := ts.library(t, "movies", t.TempDir())
	metaItem(t, ts, lib, "A Quiet Pier")
	w := ts.do(t, "POST", "/api/views", a, map[string]any{"libraryId": lib, "name": "Funny", "query": "genre=Comedy", "pinned": true})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var v library.SavedView
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	id := strconv.FormatInt(v.ID, 10)
	if w := ts.do(t, "GET", "/api/views/"+id+"/titles", b, nil); w.Code != 404 {
		t.Errorf("another account reading it: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/views/"+id, b, map[string]any{"name": "Mine", "query": ""}); w.Code != 404 {
		t.Errorf("another account changing it: %d", w.Code)
	}
	w = ts.do(t, "GET", "/api/views/"+id+"/titles", a, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "A Quiet Pier") {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	w = ts.do(t, "GET", "/api/home", a, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"view-`+id+`"`) {
		t.Errorf("not pinned to Home: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "GET", "/api/home", b, nil); strings.Contains(w.Body.String(), `"view-`+id+`"`) {
		t.Error("it's on someone else's Home")
	}
}

// A title with nothing kept about it yet still answers with lists, as an app expects.
func TestAttachedAnswersWithLists(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	lib := ts.library(t, "movies", t.TempDir())
	it := metaItem(t, ts, lib, "A Quiet Pier")
	w := ts.do(t, "GET", "/api/meta/item/"+strconv.FormatInt(it.ID, 10)+"/attached", admin, nil)
	if w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	for _, null := range []string{`"edition":null`, `"tags":null`, `"studios":null`, `"extras":null`, `"versions":null`} {
		if strings.Contains(w.Body.String(), null) {
			t.Errorf("%s in %s", null, w.Body)
		}
	}
}
