package server

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyplay"
)

// raw sends a body as it is, for the endpoints that take a file.
func (g *gameServer) raw(method, path, tok string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.RemoteAddr = "192.168.1.50:4000"
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	g.h.ServeHTTP(w, req)
	return w
}

// aGame adds a game of the built-in test console that everyone may see.
func (g *gameServer) aGame(t *testing.T, name string) int64 {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("a test game"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := g.do("POST", "/api/libraries", g.admin, map[string]any{"name": "Test", "type": "games", "console": wittyplay.TestConsole, "paths": []string{dir}})
	if w.Code != 201 {
		t.Fatalf("library: %d %s", w.Code, w.Body)
	}
	var lib library.Library
	jsonBody(t, w, &lib)
	for i := 0; i < 200; i++ {
		var r struct {
			Items []*library.Item `json:"items"`
		}
		jsonBody(t, g.do("GET", "/api/items?kind=game&library="+itoa64(lib.ID), g.admin, nil), &r)
		if len(r.Items) > 0 {
			return r.Items[0].ID
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the game wasn't found")
	return 0
}

func jsonBody(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("%v: %s", err, w.Body.String())
	}
}

type settingsResp struct {
	Resolved wittyplay.Resolved          `json:"resolved"`
	Layers   map[string]*wittyplay.Layer `json:"layers"`
	Limits   wittyplay.Limits            `json:"limits"`
	Ignored  []wittyplay.Issue           `json:"ignored"`
}

func (g *gameServer) settings(t *testing.T, tok string, item int64) settingsResp {
	t.Helper()
	w := g.do("GET", "/api/games/settings?item="+strconv.FormatInt(item, 10), tok, nil)
	if w.Code != 200 {
		t.Fatalf("reading settings: %d %s", w.Code, w.Body)
	}
	var s settingsResp
	jsonBody(t, w, &s)
	return s
}

func TestGameSettingsFollowTheirLevels(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	put := func(tok, level string, values map[string]any) *httptest.ResponseRecorder {
		return g.do("PUT", "/api/games/settings", tok, map[string]any{"level": level, "item": id, "settings": values})
	}
	if w := put(g.other, "person", map[string]any{"display.scanlines": 0.2}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := put(g.other, "console", map[string]any{"display.scanlines": 0.3}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := put(g.other, "game", map[string]any{"display.scanlines": 0.4, "made.up": 1, "display.curve": 42}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	} else {
		var s settingsResp
		jsonBody(t, w, &s)
		if len(s.Ignored) != 2 {
			t.Errorf("what wasn't used should be listed: %+v", s.Ignored)
		}
	}
	s := g.settings(t, g.other, id)
	if s.Resolved.Values["display.scanlines"] != 0.4 || s.Resolved.Sources["display.scanlines"] != wittyplay.LevelGame {
		t.Errorf("got %v from %v", s.Resolved.Values["display.scanlines"], s.Resolved.Sources["display.scanlines"])
	}
	// Taking the game's away shows the console's.
	if w := put(g.other, "game", map[string]any{"display.scanlines": nil}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if v := g.settings(t, g.other, id).Resolved.Values["display.scanlines"]; v != 0.3 {
		t.Errorf("after removing it: %v", v)
	}
	// Nothing of one person's is another's.
	if v := g.settings(t, g.kid, id).Resolved.Values["display.scanlines"]; v != 0.0 {
		t.Errorf("someone else's setting came through: %v", v)
	}
	// Resetting clears a level.
	if w := g.do("PUT", "/api/games/settings", g.other, map[string]any{"level": "console", "item": id, "reset": true}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if v := g.settings(t, g.other, id).Resolved.Values["display.scanlines"]; v != 0.2 {
		t.Errorf("after reset: %v", v)
	}
}

func TestOnlyAnAdminSetsWhatEveryoneStartsWith(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	if w := g.do("PUT", "/api/games/settings", g.other, map[string]any{"level": "server", "settings": map[string]any{"display.scanlines": 0.9}}); w.Code != 403 {
		t.Fatalf("a person set a server default: %d", w.Code)
	}
	if w := g.do("PUT", "/api/games/settings", g.admin, map[string]any{"level": "server", "settings": map[string]any{"display.scanlines": 0.5, "osd.on": true}}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	s := g.settings(t, g.other, id)
	if s.Resolved.Values["display.scanlines"] != 0.5 || s.Resolved.Sources["display.scanlines"] != wittyplay.LevelServer {
		t.Fatalf("got %v from %v", s.Resolved.Values["display.scanlines"], s.Resolved.Sources["display.scanlines"])
	}
	// And anyone can still choose otherwise for themselves.
	if w := g.do("PUT", "/api/games/settings", g.other, map[string]any{"level": "person", "settings": map[string]any{"display.scanlines": 0.1}}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if v := g.settings(t, g.other, id).Resolved.Values["display.scanlines"]; v != 0.1 {
		t.Fatalf("got %v", v)
	}
	// The stored defaults aren't reachable through the plain settings route.
	w := g.do("GET", "/api/settings", g.admin, nil)
	if strings.Contains(w.Body.String(), gameDefaultsKey) {
		t.Error("the defaults leak through /api/settings")
	}
}

func TestASettingsFileGoesThroughTheAPI(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	w := g.do("POST", "/api/games/settings/validate", g.other, map[string]any{"item": id, "document": "{\n \"settings\": {\n  \"display.curve\": 42\n }\n}"})
	var v struct {
		OK     bool                   `json:"ok"`
		Report wittyplay.CustomReport `json:"report"`
	}
	jsonBody(t, w, &v)
	if v.OK || len(v.Report.Errors) != 1 || v.Report.Errors[0].Line != 3 {
		t.Fatalf("%+v", v)
	}
	// A bad file changes nothing.
	if w := g.do("PUT", "/api/games/settings", g.other, map[string]any{"level": "game", "item": id, "settings": map[string]any{"display.curve": 0.5}}); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := g.do("PUT", "/api/games/settings", g.other, map[string]any{"level": "game", "item": id, "document": `{"settings":{"display.curve":42}}`}); w.Code != 422 {
		t.Fatalf("a bad file was saved: %d %s", w.Code, w.Body)
	}
	if v := g.settings(t, g.other, id).Resolved.Values["display.curve"]; v != 0.5 {
		t.Fatalf("a bad file changed something: %v", v)
	}
	// A good one replaces the level.
	w = g.do("PUT", "/api/games/settings", g.other, map[string]any{"level": "game", "item": id, "document": `{"wittyplay":1,"settings":{"display.glow":0.3},"coreOptions":{"test_colours":"inverted","no_such":"x"},"exec":"rm -rf /"}`})
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var s settingsResp
	jsonBody(t, w, &s)
	if s.Resolved.Values["display.glow"] != 0.3 || s.Resolved.Values["display.curve"] != 0.0 {
		t.Fatalf("got %+v", s.Resolved.Values)
	}
	ignored := ""
	for _, i := range s.Ignored {
		ignored += i.Path + " "
	}
	if !strings.Contains(ignored, "exec") {
		t.Fatalf("what was ignored wasn't said: %v", s.Ignored)
	}
	// Too large is refused without being read.
	huge := strings.Repeat(" ", wittyplay.MaxCustomSize*3)
	if w := g.do("POST", "/api/games/settings/validate", g.other, map[string]any{"item": id, "document": huge}); w.Code == 200 {
		t.Fatalf("an oversized file was read: %d", w.Code)
	}
}

func TestNobodyReachesSomeoneElsesGameData(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	item := "/api/games/" + strconv.FormatInt(id, 10)

	// A state in slot 1 belonging to one person.
	state := []byte("the first person's state")
	if w := g.raw("PUT", item+"/saves/1/file", g.other, state); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := g.do("GET", item+"/saves", g.kid, nil); strings.Contains(w.Body.String(), `"slot":1`) {
		t.Errorf("another person sees the slot: %s", w.Body)
	}
	if w := g.raw("GET", item+"/saves/1/file", g.kid, nil); w.Code != 404 {
		t.Errorf("another person read the state: %d", w.Code)
	}
	if w := g.raw("DELETE", item+"/saves/1", g.kid, nil); w.Code != 404 {
		t.Errorf("another person deleted the state: %d", w.Code)
	}
	if w := g.raw("GET", item+"/saves/1/file", g.other, nil); w.Code != 200 || !bytes.HasSuffix(w.Body.Bytes(), state) {
		t.Errorf("its owner lost it: %d", w.Code)
	}
	// And their own import goes to their own place.
	if w := g.raw("PUT", item+"/saves/1/file", g.kid, []byte("mine")); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := g.raw("GET", item+"/saves/1/file", g.other, nil); !bytes.HasSuffix(w.Body.Bytes(), state) {
		t.Error("an import replaced someone else's state")
	}

	// Battery saves.
	if w := g.raw("PUT", item+"/battery", g.other, []byte("save data")); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := g.raw("GET", item+"/battery", g.kid, nil); w.Code != 404 {
		t.Errorf("another person read a battery save: %d", w.Code)
	}

	// Cheats, notes and favourites.
	if w := g.do("PUT", item+"/cheats", g.other, map[string]any{"cheats": []map[string]any{{"label": "x", "code": "ABC", "enabled": true}}}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := g.do("GET", item+"/cheats", g.kid, nil); strings.Contains(w.Body.String(), "ABC") {
		t.Error("another person read the cheats")
	}
	if w := g.do("PUT", item+"/meta", g.other, map[string]any{"favourite": true, "note": "private"}); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := g.do("GET", item+"/meta", g.kid, nil); strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), `"favourite":true`) {
		t.Error("another person read the note")
	}
	if w := g.do("GET", "/api/games/recent?favourites=1", g.kid, nil); strings.Contains(w.Body.String(), "one.wittytest") {
		t.Error("another person's favourites showed")
	}
	if w := g.do("GET", "/api/games/recent?favourites=1", g.other, nil); !strings.Contains(w.Body.String(), "one.wittytest") {
		t.Errorf("their own favourites didn't show: %s", w.Body)
	}

	// Patches.
	p := append(append([]byte("PATCH"), 0, 0, 0, 0, 1, 'x'), "EOF"...)
	w := g.raw("POST", item+"/patches?name="+"../../evil.ips", g.other, p)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var list []library.GamePatch
	jsonBody(t, w, &list)
	if len(list) != 1 || strings.ContainsAny(list[0].Name, `/\`) {
		t.Fatalf("name: %+v", list)
	}
	pid := strconv.FormatInt(list[0].ID, 10)
	if w := g.do("GET", item+"/patches", g.kid, nil); strings.Contains(w.Body.String(), "evil") {
		t.Error("another person sees the patch")
	}
	if w := g.do("PUT", item+"/patches/"+pid, g.kid, map[string]any{"enabled": false}); w.Code != 404 {
		t.Errorf("another person changed the patch: %d", w.Code)
	}
	if w := g.raw("DELETE", item+"/patches/"+pid, g.kid, nil); w.Code != 404 {
		t.Errorf("another person removed the patch: %d", w.Code)
	}

	// Images.
	w = g.raw("POST", "/api/game-bezels?name=mine.png", g.other, aPNG(t, 8, 8))
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var added struct {
		ID int64 `json:"id"`
	}
	jsonBody(t, w, &added)
	bid := strconv.FormatInt(added.ID, 10)
	if w := g.raw("GET", "/api/game-bezels/"+bid, g.kid, nil); w.Code != 404 {
		t.Errorf("another person read an image: %d", w.Code)
	}
	if w := g.raw("DELETE", "/api/game-bezels/"+bid, g.kid, nil); w.Code != 404 {
		t.Errorf("another person removed an image: %d", w.Code)
	}
	if w := g.raw("GET", "/api/game-bezels/"+bid, g.other, nil); w.Code != 200 {
		t.Errorf("its owner lost it: %d", w.Code)
	}
}

// A page or an app can't add a header to an <img> or a download link, so a sign-in in the
// address opens these, and only these, and only for reading.
func TestASignInInTheAddressOpensOnlyPicturesAndDownloads(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	item := "/api/games/" + strconv.FormatInt(id, 10)
	if w := g.raw("PUT", item+"/saves/1/file", g.other, []byte("a state")); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	if w := g.raw("PUT", item+"/battery", g.other, []byte("a save")); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	}
	w := g.raw("POST", "/api/game-bezels?name=a.png", g.other, aPNG(t, 4, 4))
	if w.Code != 201 {
		t.Fatalf("%d", w.Code)
	}
	var added struct {
		ID int64 `json:"id"`
	}
	jsonBody(t, w, &added)
	q := "?token=" + g.other
	for _, path := range []string{item + "/saves/1/file", item + "/battery", "/api/game-bezels/" + strconv.FormatInt(added.ID, 10)} {
		if w := g.raw("GET", path+q, "", nil); w.Code != 200 {
			t.Errorf("GET %s with a token in the address: %d", path, w.Code)
		}
		// A wrong one opens nothing, and nor does another person's.
		if w := g.raw("GET", path+"?token=nonsense", "", nil); w.Code != 401 {
			t.Errorf("GET %s with a made up token: %d", path, w.Code)
		}
	}
	if w := g.raw("GET", item+"/saves/1/file?token="+g.kid, "", nil); w.Code != 404 {
		t.Errorf("another person's token opened a save: %d", w.Code)
	}
	for _, c := range []struct{ method, path string }{
		{"PUT", item + "/saves/1/file"}, {"PUT", item + "/battery"}, {"DELETE", item + "/saves/1"},
		{"DELETE", "/api/game-bezels/" + strconv.FormatInt(added.ID, 10)}, {"GET", item + "/saves"}, {"GET", item + "/meta"},
		{"GET", item + "/cheats"}, {"GET", "/api/games/settings"}, {"PUT", "/api/games/settings"},
	} {
		if w := g.raw(c.method, c.path+q, "", []byte("{}")); w.Code != 401 {
			t.Errorf("%s %s with a token in the address: %d, want 401", c.method, c.path, w.Code)
		}
	}
}

func TestSlotNumbersOnlyEverNameTheirOwnFile(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	item := "/api/games/" + strconv.FormatInt(id, 10)
	for _, bad := range []string{"0", "-1", "100", "101", "102", "99999999999", "..%2Fx", "abc", "1.5"} {
		if w := g.raw("PUT", item+"/saves/"+bad+"/file", g.other, []byte("x")); w.Code < 400 {
			t.Errorf("imported into slot %s: %d", bad, w.Code)
		}
	}
	for _, good := range []string{"1", "99"} {
		if w := g.raw("PUT", item+"/saves/"+good+"/file", g.other, []byte("x")); w.Code != 200 {
			t.Errorf("slot %s: %d %s", good, w.Code, w.Body)
		}
	}
}

func aPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{byte(x), byte(y), 80, 200})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// A person's image is hostile: only pixels that decode get through, in a file we made.
func TestImagesAreDecodedAndRemade(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	good := aPNG(t, 16, 9)
	// Something extra hiding after the image's end must not survive.
	withTrailer := append(append([]byte{}, good...), []byte("<script>alert(1)</script>")...)
	w := g.raw("POST", "/api/game-bezels?name=a.png", g.other, withTrailer)
	if w.Code != 201 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var added struct {
		ID     int64               `json:"id"`
		Bezels []library.GameBezel `json:"bezels"`
	}
	jsonBody(t, w, &added)
	if added.Bezels[0].Width != 16 || added.Bezels[0].Height != 9 {
		t.Errorf("size: %+v", added.Bezels[0])
	}
	got := g.raw("GET", "/api/game-bezels/"+strconv.FormatInt(added.ID, 10), g.other, nil)
	if got.Code != 200 || got.Header().Get("Content-Type") != "image/png" || got.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("served as %d %v", got.Code, got.Header())
	}
	if bytes.Contains(got.Body.Bytes(), []byte("<script")) {
		t.Error("what was hiding in the file got through")
	}
	if _, err := png.Decode(bytes.NewReader(got.Body.Bytes())); err != nil {
		t.Errorf("what's served isn't a PNG: %v", err)
	}

	hostile := map[string][]byte{
		"svg":         []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"html":        []byte(`<!doctype html><script>alert(1)</script>`),
		"text":        []byte("hello"),
		"empty":       {},
		"cut in half": good[:len(good)/2],
		"gif":         []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00\x00\x00\x00\xff\xff\xff!\xf9\x04\x01\x00\x00\x00\x00,\x00\x00\x00\x00\x01\x00\x01\x00\x00\x02\x02D\x01\x00;"),
	}
	for name, b := range hostile {
		if w := g.raw("POST", "/api/game-bezels?name=x.svg", g.other, b); w.Code < 400 {
			t.Errorf("%s was accepted: %d", name, w.Code)
		}
	}
	// A tiny file claiming enormous pixels is refused from its header alone.
	var bomb bytes.Buffer
	bomb.Write([]byte("\x89PNG\r\n\x1a\n"))
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr, 60000)
	binary.BigEndian.PutUint32(ihdr[4:], 60000)
	ihdr[8], ihdr[9] = 8, 6
	_ = binary.Write(&bomb, binary.BigEndian, uint32(13))
	bomb.WriteString("IHDR")
	bomb.Write(ihdr)
	_ = binary.Write(&bomb, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("IHDR"), ihdr...)))
	if w := g.raw("POST", "/api/game-bezels?name=bomb.png", g.other, bomb.Bytes()); w.Code != 422 {
		t.Errorf("a decompression bomb got: %d %s", w.Code, w.Body)
	}
	// Too large to even read.
	if w := g.raw("POST", "/api/game-bezels?name=big.png", g.other, make([]byte, maxBezelUpload+1)); w.Code != 413 {
		t.Errorf("oversized got: %d", w.Code)
	}
	// There's a limit on how many a person keeps.
	for i := 0; i < maxBezels; i++ {
		g.raw("POST", "/api/game-bezels?name=n.png", g.other, good)
	}
	if w := g.raw("POST", "/api/game-bezels?name=n.png", g.other, good); w.Code != 409 {
		t.Errorf("too many images got: %d", w.Code)
	}
}

func TestStateFilesAreCheckedComingIn(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	item := "/api/games/" + strconv.FormatInt(id, 10)
	state := []byte("some machine state")
	if w := g.raw("PUT", item+"/saves/3/file", g.other, state); w.Code != 200 {
		t.Fatalf("%d", w.Code)
	} else if !strings.Contains(w.Body.String(), "no header") {
		t.Errorf("a plain file should come with a warning: %s", w.Body)
	}
	file := g.raw("GET", item+"/saves/3/file", g.other, nil).Body.Bytes()
	if !bytes.HasPrefix(file, stateFileMagic) {
		t.Fatal("an export has no header")
	}
	// Its own export goes back in with no warning, and arrives the same.
	w := g.raw("PUT", item+"/saves/4/file", g.other, file)
	if w.Code != 200 || strings.Contains(w.Body.String(), "no header") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if again := g.raw("GET", item+"/saves/4/file", g.other, nil).Body.Bytes(); !bytes.HasSuffix(again, state) {
		t.Fatal("it didn't come back the same")
	}
	// Damage is noticed.
	damaged := append([]byte{}, file...)
	damaged[len(damaged)-1] ^= 1
	if w := g.raw("PUT", item+"/saves/5/file", g.other, damaged); w.Code != 422 {
		t.Errorf("a damaged file got: %d", w.Code)
	}
	if w := g.raw("PUT", item+"/saves/5/file", g.other, file[:len(file)-3]); w.Code != 422 {
		t.Errorf("a cut file got: %d", w.Code)
	}
	// A huge header length can't reach past the file.
	bad := append(append([]byte{}, stateFileMagic...), 0xff, 0xff, 0xff, 0xff)
	if w := g.raw("PUT", item+"/saves/5/file", g.other, bad); w.Code != 422 {
		t.Errorf("a bad header got: %d", w.Code)
	}
	if w := g.raw("PUT", item+"/saves/5/file", g.other, nil); w.Code != 422 {
		t.Errorf("an empty file got: %d", w.Code)
	}
}

func TestPlayCountsAndRecentGames(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	ctx := t.Context()
	var uid int64
	if err := g.s.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE name = 'other'`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := g.s.Store.NoteGamePlayed(ctx, uid, id); err != nil {
			t.Fatal(err)
		}
	}
	w := g.do("GET", "/api/games/"+strconv.FormatInt(id, 10)+"/meta", g.other, nil)
	var m library.GameMeta
	jsonBody(t, w, &m)
	if m.Plays != 3 || m.LastPlayed == 0 {
		t.Fatalf("%+v", m)
	}
	w = g.do("GET", "/api/games/recent", g.other, nil)
	if !strings.Contains(w.Body.String(), `"plays":3`) {
		t.Fatalf("%s", w.Body)
	}
}

func TestPersonalNotesHaveALimit(t *testing.T) {
	g := newGameServer(t)
	defer g.s.StopGames()
	id := g.aGame(t, "one.wittytest")
	item := "/api/games/" + strconv.FormatInt(id, 10)
	if w := g.do("PUT", item+"/meta", g.other, map[string]any{"note": strings.Repeat("x", maxGameNote+1)}); w.Code != 400 {
		t.Fatalf("got %d", w.Code)
	}
	if w := g.do("PUT", item+"/cheats", g.other, map[string]any{"cheats": []map[string]any{{"code": "<script>"}}}); w.Code != 400 {
		t.Fatalf("got %d", w.Code)
	}
}
