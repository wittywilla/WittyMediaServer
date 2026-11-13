// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/wittyplayfile"
)

func TestGamePicturesAPI(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, viewer := ts.user(t, "viewer", "adult", false, "")
	root := t.TempDir()
	lib := ts.library(t, "games", root)
	game := ts.item(t, &library.Item{LibraryID: lib, Kind: "game", Path: filepath.Join(root, "Made Up Quest (USA).sfc"), Title: "Made Up Quest", Console: "snes"})
	gid := strconv.FormatInt(game.ID, 10)

	var got struct {
		Server     wittyplayfile.Doc             `json:"server"`
		Games      map[string]wittyplayfile.Look `json:"games"`
		CaseStyles []string                      `json:"caseStyles"`
	}
	w := ts.do(t, "GET", "/api/games/pictures", viewer, nil)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || len(got.Games) != 0 || len(got.CaseStyles) != len(wittyplayfile.CaseStyles) {
		t.Fatalf("empty: %d %s", w.Code, w.Body)
	}

	doc := []byte(`{"wittyplay":1,"default":{"picture":"case"},"consoles":{"ps1":{"picture":"fitted"}},
		"games":[{"match":{"file":"made up quest (usa).sfc"},"case":"generic-box"},{"match":{"title":"Nobody Has This"},"label":"title"}],"x-mine":1}`)
	if w := ts.do(t, "PUT", "/api/games/pictures", viewer, doc); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer set the server's pictures: %d", w.Code)
	}
	w = ts.do(t, "PUT", "/api/games/pictures", admin, doc)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"applied":1`) || !strings.Contains(w.Body.String(), `"unmatched":1`) {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	w = ts.do(t, "GET", "/api/games/pictures", viewer, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Server.Default.Picture != "case" || got.Server.Consoles["ps1"].Picture != "fitted" || got.Games[gid].Case != "generic-box" {
		t.Fatalf("after put: %s", w.Body)
	}
	if !strings.Contains(w.Body.String(), `"x-mine":1`) {
		t.Errorf("an unknown field was lost: %s", w.Body)
	}

	for name, bad := range map[string][]byte{
		"not json":  []byte("nope"),
		"bad value": []byte(`{"wittyplay":1,"default":{"picture":"stretched"}}`),
		"newer":     []byte(`{"wittyplay":9}`),
	} {
		if w := ts.do(t, "PUT", "/api/games/pictures", admin, bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	big := []byte(`{"wittyplay":1,"x":"` + strings.Repeat("y", wittyplayfile.MaxSize) + `"}`)
	if w := ts.do(t, "POST", "/api/games/pictures/check", viewer, big); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("too big: %d", w.Code)
	}

	// One game, by an administrator; an empty look takes it away.
	if w := ts.do(t, "PUT", "/api/games/"+gid+"/picture", viewer, map[string]string{"picture": "filled"}); w.Code != http.StatusForbidden {
		t.Fatalf("viewer: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/games/"+gid+"/picture", admin, map[string]string{"picture": "sideways"}); w.Code != http.StatusBadRequest {
		t.Fatalf("bad look: %d", w.Code)
	}
	w = ts.do(t, "PUT", "/api/games/"+gid+"/picture", admin, map[string]string{"picture": "filled", "case": "cart-super"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"filled"`) {
		t.Fatalf("one game: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/games/99999/picture", admin, map[string]string{"picture": "filled"}); w.Code != 404 {
		t.Fatalf("no such game: %d", w.Code)
	}

	// The server's file reads back as a valid .wittyplay that finds the same game.
	w = ts.do(t, "GET", "/api/games/pictures/file", admin, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != wittyplayfile.MediaType || !strings.Contains(w.Header().Get("Content-Disposition"), ".wittyplay") {
		t.Fatalf("file: %d %v", w.Code, w.Header())
	}
	exported, err := wittyplayfile.Parse(w.Body.Bytes())
	if err != nil {
		t.Fatalf("exported file doesn't read: %v\n%s", err, w.Body)
	}
	if len(exported.Games) != 1 || exported.Games[0].Match.File != "Made Up Quest (USA).sfc" || exported.Games[0].Picture != "filled" || exported.Default.Picture != "case" {
		t.Fatalf("exported: %s", w.Body)
	}
	if w := ts.do(t, "GET", "/api/games/pictures/file", viewer, nil); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer downloaded the server's file: %d", w.Code)
	}

	// Checking a file for someone's own import says which of their games each entry is.
	var check struct {
		Matches [][]int64 `json:"matches"`
	}
	w = ts.do(t, "POST", "/api/games/pictures/check", viewer, []byte(`{"wittyplay":1,"games":[{"match":{"title":"MADE UP QUEST","console":"snes"},"picture":"case"},{"match":{"file":"x.nes"}}]}`))
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &check) != nil || len(check.Matches) != 2 || len(check.Matches[0]) != 1 || check.Matches[0][0] != game.ID || len(check.Matches[1]) != 0 {
		t.Fatalf("check: %d %s", w.Code, w.Body)
	}
}

// fakeLogos is a logo source that serves made-up pictures from a test server: nothing real.
type fakeLogos struct {
	url  string
	asks int
}

func (f *fakeLogos) Find(ctx context.Context, kind, name string) ([]library.LogoCandidate, error) {
	f.asks++
	if kind != library.LogoConsole {
		return nil, nil
	}
	return []library.LogoCandidate{{Source: "test", SourceID: name, Name: name, ImageURL: f.url + "/logo.png", PageURL: f.url + "/page",
		License: "Public domain (made up for a test)", Credit: "Nobody", Exact: true}}, nil
}

func (f *fakeLogos) Fetch(ctx context.Context, c library.LogoCandidate) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", c.ImageURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var b bytes.Buffer
	_, err = b.ReadFrom(resp.Body)
	return b.Bytes(), err
}

func TestBrandLogos(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 60, 20))
	for y := 4; y < 16; y++ {
		for x := 6; x < 54; x++ {
			m.SetNRGBA(x, y, color.NRGBA{30, 60, 200, 255})
		}
	}
	var pic bytes.Buffer
	if err := png.Encode(&pic, m); err != nil {
		t.Fatal(err)
	}
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pic.Bytes())
	}))
	defer src.Close()

	ts := newTestServer(t)
	fake := &fakeLogos{url: src.URL}
	ts.logoOnce.Do(func() {
		ts.logoSvc = &library.Logos{Store: ts.Store, Dir: filepath.Join(ts.Cfg.ImagesDir, "logos"), Source: fake, Log: ts.Log, Gap: time.Millisecond}
	})
	_, admin := ts.user(t, "admin", "adult", true, "")
	_, viewer := ts.user(t, "viewer", "adult", false, "")

	type state struct {
		Enabled bool                 `json:"enabled"`
		Logos   map[string]brandLogo `json:"logos"`
		Warning []string             `json:"warning"`
		Pending int                  `json:"pending"`
		AppWide string               `json:"appWide"`
	}
	read := func(tok string) state {
		t.Helper()
		var s state
		w := ts.do(t, "GET", "/api/games/brand-logos", tok, nil)
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &s) != nil {
			t.Fatalf("read: %d %s", w.Code, w.Body)
		}
		return s
	}
	// Off by default, and nothing is asked for.
	if s := read(viewer); s.Enabled || len(s.Logos) != 0 || len(s.Warning) == 0 {
		t.Fatalf("default: %+v", s)
	}
	if w := ts.do(t, "PUT", "/api/games/brand-logos", viewer, map[string]bool{"enabled": true, "accepted": true}); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer turned them on: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/games/brand-logos", admin, map[string]bool{"enabled": true}); w.Code != http.StatusBadRequest {
		t.Fatalf("on without confirming the warning: %d", w.Code)
	}
	if fake.asks != 0 {
		t.Fatal("a logo was looked for before anyone agreed")
	}
	studio, err := ts.Store.EnsureLogo(context.Background(), library.LogoGameStudio, "Made Up Studio")
	if err != nil {
		t.Fatal(err)
	}
	studioURL := "/api/logos/" + strconv.FormatInt(studio.ID, 10)
	if w := ts.do(t, "GET", studioURL+"/candidates", admin, nil); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"brandKind":true`) || !strings.Contains(w.Body.String(), `"brandLogos":false`) {
		t.Fatalf("a search before agreeing: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", "/api/games/brand-logos", admin, map[string]bool{"enabled": true, "accepted": true}); w.Code != 200 {
		t.Fatalf("on: %d %s", w.Code, w.Body)
	}
	var s state
	for deadline := time.Now().Add(20 * time.Second); ; {
		s = read(admin)
		if s.Pending == 0 && len(s.Logos) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no logos came: %+v", s)
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // the service's last pause
	snes, ok := s.Logos["snes"]
	if !ok || snes.License == "" || snes.Source != "test" || snes.Credit != "Nobody" || s.AppWide == "yes" {
		t.Fatalf("snes: %+v", s)
	}
	// The app-wide agreement wasn't given, so only consoles were looked for.
	if c := ts.Store.LogoConsent(context.Background()); c == "yes" {
		t.Fatal("the app-wide agreement changed")
	}
	// The picture comes from this server.
	if w := ts.do(t, "GET", "/api/logos/"+strconv.FormatInt(snes.ID, 10)+"/image?variant=white", viewer, nil); w.Code != 200 {
		t.Fatalf("image: %d", w.Code)
	}
	if v := read(viewer); !v.Enabled || len(v.Logos) == 0 || v.AppWide != "" {
		t.Fatalf("viewer sees: %+v", v)
	}
	// The logo chooser offers what was downloaded, for another name, and can search for a studio's.
	other, err := ts.Store.EnsureLogo(context.Background(), library.LogoConsole, "Made Up Console")
	if err != nil {
		t.Fatal(err)
	}
	otherURL := "/api/logos/" + strconv.FormatInt(other.ID, 10)
	var cands struct {
		Downloaded []library.Logo `json:"downloaded"`
		BrandLogos bool           `json:"brandLogos"`
	}
	w0 := ts.do(t, "GET", otherURL+"/candidates", admin, nil)
	if w0.Code != 200 || json.Unmarshal(w0.Body.Bytes(), &cands) != nil || !cands.BrandLogos || len(cands.Downloaded) == 0 {
		t.Fatalf("candidates: %d %s", w0.Code, w0.Body)
	}
	if w := ts.do(t, "GET", studioURL+"/candidates", admin, nil); w.Code != 200 {
		t.Fatalf("a studio's search under WittyPlay's agreement: %d %s", w.Code, w.Body)
	}
	if w := ts.do(t, "PUT", otherURL, admin, map[string]any{"copyFrom": studio.ID}); w.Code == 200 {
		t.Fatal("a logo of another kind was copied")
	}
	w1 := ts.do(t, "PUT", otherURL, admin, map[string]any{"copyFrom": snes.ID})
	if w1.Code != 200 || !strings.Contains(w1.Body.String(), `"has":true`) || !strings.Contains(w1.Body.String(), `"credit":"Nobody"`) {
		t.Fatalf("copy: %d %s", w1.Code, w1.Body)
	}
	// Removed: every downloaded console logo goes, and they're off.
	if w := ts.do(t, "DELETE", "/api/games/brand-logos", viewer, nil); w.Code != http.StatusForbidden {
		t.Fatalf("a viewer removed them: %d", w.Code)
	}
	w := ts.do(t, "DELETE", "/api/games/brand-logos", admin, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"removed":0`) {
		t.Fatalf("remove: %d %s", w.Code, w.Body)
	}
	if v := read(admin); v.Enabled || len(v.Logos) != 0 {
		t.Fatalf("after removing: %+v", v)
	}
	if w := ts.do(t, "GET", "/api/logos/"+strconv.FormatInt(snes.ID, 10)+"/image?variant=white", viewer, nil); w.Code == 200 {
		t.Fatal("a removed logo is still served")
	}
}
