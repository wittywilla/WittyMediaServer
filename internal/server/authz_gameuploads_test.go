package server

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/auth"
)

// Guest profiles only look, and Kids profiles can't feed bytes to a game core except their own saves.
func TestGuestsAndKidsAndGameUploads(t *testing.T) {
	g := newGameServer(t)
	item := runnerGameItem(t, g)
	ctx := context.Background()
	if _, err := g.s.DB.Exec(`UPDATE items SET content_rating = 'G' WHERE id = ?`, item); err != nil {
		t.Fatal(err)
	}
	profile := func(name, kind string) string {
		if _, err := g.s.Auth.CreateProfile(ctx, name, "1234", kind, false, ""); err != nil {
			t.Fatal(err)
		}
		tok, _, err := g.s.Auth.Login(ctx, name, "1234", "t", false)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	guest, kid := profile("Visitor", auth.KindGuest), profile("Kiddo", auth.KindKids)
	var pic bytes.Buffer
	if err := png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	send := func(tok, method, path string, body []byte) int {
		r := httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Host = "127.0.0.1:9797"
		r.RemoteAddr = "127.0.0.1:1"
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		g.h.ServeHTTP(w, r)
		return w.Code
	}
	state := []byte("bytes for a core to read")
	patch := []byte("PATCH\x00\x00\x10\x00\x01\x41EOF")
	cases := []struct {
		name, method, path string
		body               []byte
		kids               bool // a Kids profile may do this too
	}{
		{"state", "PUT", fmt.Sprintf("/api/games/%d/saves/1/file", item), state, true},
		{"battery", "PUT", fmt.Sprintf("/api/games/%d/battery", item), state, true},
		{"patch", "POST", fmt.Sprintf("/api/games/%d/patches?name=x", item), patch, false},
		{"bezel", "POST", "/api/game-bezels?name=x", pic.Bytes(), true},
		{"background", "PUT", "/api/game-background/image", pic.Bytes(), true},
	}
	for _, c := range cases {
		if got := send(guest, c.method, c.path, c.body); got != 403 {
			t.Errorf("Guest, %s: %d", c.name, got)
		}
		got := send(kid, c.method, c.path, c.body)
		if c.kids && got == 403 || !c.kids && got != 403 {
			t.Errorf("Kids, %s: %d", c.name, got)
		}
		if got := send(g.other, c.method, c.path, c.body); got == 403 {
			t.Errorf("an adult, %s: %d", c.name, got)
		}
	}
}
