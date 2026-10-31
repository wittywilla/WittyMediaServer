package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
)

func newProfile(name string) map[string]any {
	return map[string]any{"name": name, "kind": "gamepad", "controller": "054c:0ce6", "console": "snes", "active": true,
		"map": map[string]any{"0": map[string]any{"b": 5}}}
}

func decodeProfile(t *testing.T, w *httptest.ResponseRecorder) wittyplay.Profile {
	t.Helper()
	var p wittyplay.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
	return p
}

func TestGameProfilesLifecycle(t *testing.T) {
	g := newGameServer(t)
	w := g.do("POST", "/api/game-profiles", g.admin, newProfile("Mine"))
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	p := decodeProfile(t, w)
	if p.ID == 0 || !p.Active || p.Name != "Mine" || p.UpdatedAt == 0 {
		t.Fatalf("%+v", p)
	}
	path := "/api/game-profiles/" + itoa64(p.ID)

	// Copied: a separate profile, not in use.
	w = g.do("POST", path+"/copy", g.admin, nil)
	if w.Code != 201 {
		t.Fatalf("copy: %d %s", w.Code, w.Body.String())
	}
	c := decodeProfile(t, w)
	if c.ID == p.ID || c.Active || c.Name != "Mine copy" || c.Map[0] == nil {
		t.Fatalf("%+v", c)
	}

	// Choosing the copy takes the first out of use: one at a time for one scope.
	if w := g.do("POST", "/api/game-profiles/"+itoa64(c.ID)+"/active", g.admin, map[string]any{"active": true}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if got := decodeProfile(t, g.do("GET", path, g.admin, nil)); got.Active {
		t.Error("two profiles of one scope are active")
	}

	// Edited, with what doesn't belong gone.
	body := newProfile("Renamed")
	body["updatedAt"] = 1
	body["id"] = 12345
	w = g.do("PUT", path, g.admin, body)
	if got := decodeProfile(t, w); w.Code != 200 || got.Name != "Renamed" || got.ID != p.ID || got.UpdatedAt == 1 {
		t.Fatalf("%d %+v", w.Code, got)
	}

	// The list follows, and the controls route offers what's in use.
	var list []wittyplay.Profile
	_ = json.Unmarshal(g.do("GET", "/api/game-profiles?kind=gamepad&console=snes", g.admin, nil).Body.Bytes(), &list)
	if len(list) != 2 {
		t.Fatalf("%+v", list)
	}
	var ctl struct {
		Profiles []wittyplay.Profile `json:"profiles"`
		Targets  []wittyplay.Target  `json:"targets"`
	}
	_ = json.Unmarshal(g.do("GET", "/api/consoles/snes/controls", g.admin, nil).Body.Bytes(), &ctl)
	if len(ctl.Targets) == 0 || len(ctl.Profiles) != 1 || !ctl.Profiles[0].Active {
		t.Fatalf("%+v", ctl)
	}

	// Asking which applies.
	var res struct {
		Profile *wittyplay.Profile        `json:"profile"`
		Map     map[int]wittyplay.Binding `json:"map"`
		Options wittyplay.ProfileOptions  `json:"options"`
	}
	_ = json.Unmarshal(g.do("GET", "/api/game-profiles/resolve?controller=054c:0ce6&console=snes", g.admin, nil).Body.Bytes(), &res)
	if res.Profile == nil || res.Profile.ID != p.ID || *res.Map[0].Button != 5 || res.Options.Deadzone == 0 {
		t.Fatalf("%+v", res)
	}
	res.Profile = nil
	_ = json.Unmarshal(g.do("GET", "/api/game-profiles/resolve?controller=1111:2222&console=snes", g.admin, nil).Body.Bytes(), &res)
	if res.Profile != nil || *res.Map[0].Button != 0 {
		t.Fatalf("with no profile, the default: %+v", res)
	}

	// Deleted.
	if w := g.do("DELETE", path, g.admin, nil); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := g.do("GET", path, g.admin, nil); w.Code != 404 {
		t.Fatalf("%d", w.Code)
	}
	if w := g.do("DELETE", path, g.admin, nil); w.Code != 404 {
		t.Fatalf("%d", w.Code)
	}
}

func TestGameProfilesBelongToTheirOwner(t *testing.T) {
	g := newGameServer(t)
	p := decodeProfile(t, g.do("POST", "/api/game-profiles", g.admin, newProfile("Mine")))
	path := "/api/game-profiles/" + itoa64(p.ID)
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"GET", path, nil},
		{"PUT", path, newProfile("Stolen")},
		{"DELETE", path, nil},
		{"POST", path + "/copy", nil},
		{"POST", path + "/active", map[string]any{"active": false}},
	} {
		if w := g.do(c.method, c.path, g.other, c.body); w.Code != 404 {
			t.Errorf("%s %s by someone else: %d", c.method, c.path, w.Code)
		}
	}
	var list []wittyplay.Profile
	_ = json.Unmarshal(g.do("GET", "/api/game-profiles", g.other, nil).Body.Bytes(), &list)
	if len(list) != 0 {
		t.Errorf("someone else lists %+v", list)
	}
	var f wittyplay.ProfileFile
	_ = json.Unmarshal(g.do("GET", "/api/game-profiles/export?id="+itoa64(p.ID), g.other, nil).Body.Bytes(), &f)
	if len(f.Profiles) != 0 {
		t.Errorf("someone else exports %+v", f)
	}
	if got := decodeProfile(t, g.do("GET", path, g.admin, nil)); got.Name != "Mine" || !got.Active {
		t.Errorf("changed by someone else: %+v", got)
	}
	// And nobody without a sign-in.
	if w := g.do("GET", "/api/game-profiles", "", nil); w.Code != 401 {
		t.Errorf("no sign-in: %d", w.Code)
	}
	if w := g.do("GET", "/api/game-background/image", "", nil); w.Code != 401 {
		t.Errorf("no sign-in: %d", w.Code)
	}
}

func TestGameProfileGameMustBeVisible(t *testing.T) {
	g := newGameServer(t)
	body := newProfile("One game")
	body["game"] = 999999
	if w := g.do("POST", "/api/game-profiles", g.admin, body); w.Code != 400 {
		t.Errorf("%d %s", w.Code, w.Body.String())
	}
	if w := g.do("POST", "/api/game-profiles", g.admin, map[string]any{"name": "x", "kind": "gamepad", "console": "nosuch"}); w.Code != 400 {
		t.Errorf("unknown console: %d", w.Code)
	}
}

func TestGameProfileLimit(t *testing.T) {
	g := newGameServer(t)
	ctx := t.Context()
	for i := 0; i < wittyplay.MaxProfiles; i++ {
		if _, err := g.s.Store.SaveGameProfile(ctx, 1, wittyplay.Profile{Name: "x", Kind: "gamepad"}); err != nil {
			t.Fatal(err)
		}
	}
	if w := g.do("POST", "/api/game-profiles", g.admin, newProfile("One more")); w.Code != 409 {
		t.Errorf("over the limit: %d %s", w.Code, w.Body.String())
	}
	if _, err := g.s.Store.SaveGameProfile(ctx, 1, wittyplay.Profile{Name: "x", Kind: "gamepad"}); err != library.ErrTooManyProfiles {
		t.Errorf("%v", err)
	}
}

func TestGameProfileExportImport(t *testing.T) {
	g := newGameServer(t)
	p := decodeProfile(t, g.do("POST", "/api/game-profiles", g.admin, newProfile("Exported")))
	w := g.do("GET", "/api/game-profiles/export", g.admin, nil)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	file := w.Body.Bytes()

	// Into another account, where nothing else is in use: it takes effect.
	imp := func(tok string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/game-profiles/import", bytes.NewReader(body))
		req.RemoteAddr = "192.168.1.50:4000"
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		g.h.ServeHTTP(rec, req)
		return rec
	}
	var res struct {
		Imported []wittyplay.Profile `json:"imported"`
		Rejected []struct {
			Index int    `json:"index"`
			Error string `json:"error"`
		} `json:"rejected"`
		Notes []string `json:"notes"`
	}
	w = imp(g.other, file)
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || w.Code != 200 || len(res.Imported) != 1 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	got := res.Imported[0]
	if got.ID == p.ID || got.Name != "Exported" || !got.Active || got.Map[0] == nil {
		t.Fatalf("%+v", got)
	}

	// Back into the first, where one is in use: it stays out of the way.
	res.Imported = nil
	_ = json.Unmarshal(imp(g.admin, file).Body.Bytes(), &res)
	if len(res.Imported) != 1 || res.Imported[0].Active {
		t.Fatalf("%+v", res.Imported)
	}
	if !decodeProfile(t, g.do("GET", "/api/game-profiles/"+itoa64(p.ID), g.admin, nil)).Active {
		t.Error("importing replaced what was in use")
	}

	// A hostile file: its good part is kept, each bad one says why, and forged fields go.
	hostile := `{"format":"wittyplay-controller-profiles","version":1,"profiles":[
		{"name":"Good","kind":"gamepad","id":1,"gameTitle":"<script>"},
		{"name":"Bad button","kind":"gamepad","map":{"99":{"b":1}}},
		{"name":"<img src=x onerror=alert(1)>","kind":"gamepad","controller":"../../../etc/passwd"},
		{"name":"Someone else's game","kind":"gamepad","console":"snes","game":424242,"gameTitle":"Missing"}
	]}`
	res.Imported, res.Rejected, res.Notes = nil, nil, nil
	w = imp(g.other, []byte(hostile))
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if len(res.Imported) != 2 || len(res.Rejected) != 2 || len(res.Notes) != 1 {
		t.Fatalf("%s", w.Body.String())
	}
	for _, p := range res.Imported {
		if p.Game != 0 || p.GameTitle != "" || p.ID == 1 {
			t.Errorf("%+v", p)
		}
	}

	for why, body := range map[string][]byte{
		"not a file of ours": []byte(`{"hello":1}`),
		"not JSON":           []byte("not json"),
		"too large":          bytes.Repeat([]byte(" "), wittyplay.MaxImportSize+1),
	} {
		if w := imp(g.other, body); w.Code != 400 && w.Code != 413 {
			t.Errorf("%s: %d", why, w.Code)
		}
	}
}

// ---- backgrounds ----

func testPicture(w, h int) image.Image {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.Set(x, y, color.RGBA{uint8(x), uint8(y), 80, 255})
		}
	}
	return m
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, testPicture(w, h)); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, testPicture(w, h), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// jpegWithExif puts an EXIF block in a JPEG: orientation, and a secret to be dropped.
func jpegWithExif(t *testing.T, w, h int, orientation byte) []byte {
	t.Helper()
	src := jpegBytes(t, w, h)
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, orientation, 0, 0, 0, 0, 0, 0, 0}
	tiff = append(tiff, []byte("SECRET-GPS-AND-SERIAL")...)
	body := append([]byte("Exif\x00\x00"), tiff...)
	seg := append([]byte{0xFF, 0xE1, byte((len(body) + 2) >> 8), byte(len(body) + 2)}, body...)
	return append(append(append([]byte{}, src[:2]...), seg...), src[2:]...)
}

func (g *gameServer) upload(tok string, body []byte, ctype string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("PUT", "/api/game-background/image", bytes.NewReader(body))
	req.RemoteAddr = "192.168.1.50:4000"
	req.Header.Set("Authorization", "Bearer "+tok)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	rec := httptest.NewRecorder()
	g.h.ServeHTTP(rec, req)
	return rec
}

func TestGameBackgroundUploadRefusals(t *testing.T) {
	g := newGameServer(t)
	var gifBuf bytes.Buffer
	pal := image.NewPaletted(image.Rect(0, 0, 32, 32), color.Palette{color.Black, color.White})
	if err := gif.Encode(&gifBuf, pal, nil); err != nil {
		t.Fatal(err)
	}
	good := pngBytes(t, 64, 64)
	cases := []struct {
		why   string
		body  []byte
		ctype string
	}{
		{"SVG, even claiming to be PNG", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), "image/png"},
		{"HTML", []byte("<html><script>alert(document.cookie)</script></html>"), "image/jpeg"},
		{"nothing", nil, "image/png"},
		{"garbage", bytes.Repeat([]byte{0xde, 0xad}, 512), "image/webp"},
		{"GIF", gifBuf.Bytes(), "image/png"},
		{"a PNG header and nothing else", good[:40], "image/png"},
		{"a cut-off PNG", good[:len(good)-30], "image/png"},
		{"a JPEG header alone", jpegBytes(t, 64, 64)[:30], "image/jpeg"},
		{"too small", pngBytes(t, 4, 4), "image/png"},
		{"a bomb: small file, huge picture", pngBomb(t, 8000, 8000), "image/png"},
		{"too wide", pngBomb(t, 9000, 10), "image/png"},
		{"too large a file", bytes.Repeat([]byte{0}, maxBackgroundBytes+1), "image/png"},
		{"a RIFF that isn't WebP", append([]byte("RIFF\x10\x00\x00\x00WAVEfmt "), make([]byte, 64)...), "image/webp"},
		{"a WebP with nothing in it", append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), make([]byte, 16)...), "image/webp"},
	}
	for _, c := range cases {
		if w := g.upload(g.admin, c.body, c.ctype); w.Code != 400 && w.Code != 413 {
			t.Errorf("%s: %d %s", c.why, w.Code, w.Body.String())
		}
	}
	var bg library.GameBackground
	_ = json.Unmarshal(g.do("GET", "/api/game-background", g.admin, nil).Body.Bytes(), &bg)
	if bg.Version != 0 {
		t.Errorf("something got stored: %+v", bg)
	}
	if entries, _ := os.ReadDir(filepath.Join(g.s.Cfg.DataDir, "gaming", "backgrounds")); len(entries) != 0 {
		t.Errorf("files were left: %v", entries)
	}
}

// pngBomb is a real PNG header claiming a size, with nothing behind it.
func pngBomb(t *testing.T, w, h int) []byte {
	t.Helper()
	cfg := pngBytes(t, 16, 16)
	out := append([]byte{}, cfg...)
	out[16], out[17], out[18], out[19] = byte(w>>24), byte(w>>16), byte(w>>8), byte(w)
	out[20], out[21], out[22], out[23] = byte(h>>24), byte(h>>16), byte(h>>8), byte(h)
	// The checksum on this chunk is now wrong, which is a decoder's problem to find out.
	return out
}

func TestGameBackgroundIsCleanedAndPrivate(t *testing.T) {
	g := newGameServer(t)
	// A sideways photo with something private in it, and something after its end.
	upload := append(jpegWithExif(t, 80, 40, 6), []byte("<script>alert(1)</script>")...)
	w := g.upload(g.admin, upload, "application/x-anything")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var bg library.GameBackground
	_ = json.Unmarshal(w.Body.Bytes(), &bg)
	if bg.Version == 0 || bg.Fit != "cover" {
		t.Fatalf("%+v", bg)
	}

	w = g.do("GET", "/api/game-background/image?v=1", g.admin, nil)
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	h := w.Header()
	if h.Get("Content-Type") != "image/jpeg" || h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") || !strings.Contains(h.Get("Cache-Control"), "private") {
		t.Errorf("headers: %v", h)
	}
	served := w.Body.Bytes()
	if bytes.Contains(served, []byte("SECRET")) || bytes.Contains(served, []byte("Exif")) || bytes.Contains(served, []byte("<script>")) {
		t.Error("what the file carried survived")
	}
	m, format, err := image.Decode(bytes.NewReader(served))
	if err != nil || format != "jpeg" {
		t.Fatalf("%v %s", err, format)
	}
	if m.Bounds().Dx() != 40 || m.Bounds().Dy() != 80 {
		t.Errorf("the photo wasn't turned upright: %v", m.Bounds())
	}

	// Someone else has none, can't reach this one, and can't remove it.
	if w := g.do("GET", "/api/game-background/image", g.other, nil); w.Code != 404 {
		t.Errorf("someone else's picture: %d", w.Code)
	}
	if w := g.do("DELETE", "/api/game-background", g.other, nil); w.Code != 200 {
		t.Errorf("%d", w.Code)
	}
	if w := g.do("GET", "/api/game-background/image", g.admin, nil); w.Code != 200 {
		t.Errorf("removed by someone else: %d", w.Code)
	}
	// Their own upload doesn't replace it either.
	if w := g.upload(g.other, pngBytes(t, 32, 32), "image/png"); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	again := g.do("GET", "/api/game-background/image", g.admin, nil).Body.Bytes()
	if !bytes.Equal(again, served) {
		t.Error("one person's upload replaced another's")
	}

	// Position, fit and dimming, in range only, and never which picture it is.
	w = g.do("PUT", "/api/game-background", g.admin, map[string]any{"fit": "contain", "x": 10, "y": 90, "dim": 70, "version": 1})
	var set library.GameBackground
	_ = json.Unmarshal(w.Body.Bytes(), &set)
	if w.Code != 200 || set.Fit != "contain" || set.Dim != 70 || set.Version != bg.Version {
		t.Fatalf("%d %+v", w.Code, set)
	}
	for _, bad := range []map[string]any{
		{"fit": "url(javascript:alert(1))", "x": 0, "y": 0, "dim": 0},
		{"fit": "cover", "x": 200, "y": 0, "dim": 0},
		{"fit": "cover", "x": 0, "y": -1, "dim": 0},
		{"fit": "cover", "x": 0, "y": 0, "dim": 100},
	} {
		if w := g.do("PUT", "/api/game-background", g.admin, bad); w.Code != 400 {
			t.Errorf("%v: %d", bad, w.Code)
		}
	}

	// Removed, and gone from disk.
	if w := g.do("DELETE", "/api/game-background", g.admin, nil); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := g.do("GET", "/api/game-background/image", g.admin, nil); w.Code != 404 {
		t.Errorf("%d", w.Code)
	}
	if _, err := os.Stat(g.s.backgroundPath(1)); !os.IsNotExist(err) {
		t.Errorf("still on disk: %v", err)
	}
}

func TestGameBackgroundIsShrunkAndFlattened(t *testing.T) {
	// Big pictures are kept small, and transparency doesn't reach the JPEG as junk.
	m := image.NewNRGBA(image.Rect(0, 0, 3000, 1500))
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	out, err := cleanBackground(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds().Dx() != backgroundOutSide || got.Bounds().Dy() != backgroundOutSide/2 {
		t.Errorf("%v", got.Bounds())
	}
	if r, g2, bl, _ := got.At(10, 10).RGBA(); r>>8 > 2 || g2>>8 > 2 || bl>>8 > 2 {
		t.Errorf("transparent should become black: %d %d %d", r>>8, g2>>8, bl>>8)
	}
}

func TestRemovingAnAccountRemovesItsBackground(t *testing.T) {
	g := newGameServer(t)
	if w := g.upload(g.other, pngBytes(t, 32, 32), "image/png"); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if entries, _ := os.ReadDir(filepath.Join(g.s.Cfg.DataDir, "gaming", "backgrounds")); len(entries) != 1 {
		t.Fatalf("%v", entries)
	}
	if w := g.do(http.MethodDelete, "/api/users/2", g.admin, nil); w.Code != 200 { // the account made second
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if entries, _ := os.ReadDir(filepath.Join(g.s.Cfg.DataDir, "gaming", "backgrounds")); len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
}
