// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// An artist card opens /artist/{library}?name=: the server has a page for it, with the
// artist's albums and songs, within the profile's limits.
func TestSDUIArtistPage(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	music := ts.library(t, "music", root)
	n := 0
	song := func(artist, album, rating string, year int) {
		n++
		y := year
		ts.item(t, &library.Item{LibraryID: music, Kind: "audio", Path: filepath.Join(root, fmt.Sprintf("%d.flac", n)), Title: fmt.Sprintf("Song %02d", n),
			Artist: artist, AlbumArtist: artist, Album: album, Year: &y, RuntimeSec: 200, ContentRating: rating})
	}
	song("The Examples", "First", "G", 2001)
	song("The Examples", "First", "G", 2001)
	song("The Examples", "Second", "R", 2004)
	song("Hidden Band", "Loud", "R", 2010)
	ts.user(t, "boss", "adult", true, "")
	_, adult := ts.user(t, "bob", "adult", false, "")
	_, kid := ts.user(t, "kid", "kids", false, "")

	artistPage := func(name string) string {
		return fmt.Sprintf("/api/sdui/pages/detail?formFactor=phone&library=%d&artist=%s", music, url.QueryEscape(name))
	}
	cases := []struct {
		name, tok, artist string
		code              int
		albums, songs     int
	}{
		{"an adult sees every album and song", adult, "The Examples", 200, 2, 3},
		{"a kid sees what is within its limits", kid, "The Examples", 200, 1, 2},
		{"an artist with nothing the kid may hear", kid, "Hidden Band", 404, 0, 0},
		{"an artist who isn't there", adult, "Nobody", 404, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := ts.do(t, "GET", artistPage(tc.artist), tc.tok, nil)
			if w.Code != tc.code {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if tc.code != 200 {
				return
			}
			doc := sduiDoc(t, w)
			want := fmt.Sprintf("/artist/%d?name=%s", music, url.QueryEscape(tc.artist))
			if doc.Page.ID != "detail-artist" || doc.Page.Title != tc.artist || doc.Page.Route != want {
				t.Errorf("page %+v, want detail-artist at %s", doc.Page, want)
			}
			if h := findNode(&doc.Root, "header"); h == nil || h.Props["title"] != tc.artist {
				t.Errorf("header %+v", h)
			}
			if a := findNode(&doc.Root, "albums"); a == nil || len(a.Children) != tc.albums {
				t.Errorf("albums: %+v", a)
			}
			if s := findNode(&doc.Root, "tracks"); s == nil || len(s.Children) != tc.songs {
				t.Errorf("songs: %+v", s)
			}
		})
	}

	// The artists row's cards open exactly that page.
	lib := sduiDoc(t, ts.do(t, "GET", fmt.Sprintf("/api/sdui/pages/library?formFactor=phone&library=%d&tab=artists", music), adult, nil))
	found := false
	var walk func(n *sdui.Node)
	walk = func(n *sdui.Node) {
		if n.Action != nil && n.Action.To == fmt.Sprintf("/artist/%d?name=The+Examples", music) {
			found = true
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&lib.Root)
	if !found {
		t.Error("no artist card opens the artist's page")
	}
}
