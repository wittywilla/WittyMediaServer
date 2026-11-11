// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Someone holding a stream's address who may not see what it plays gets nothing.
func TestStreamsFollowTheAccountsLimits(t *testing.T) {
	ts := newTestServer(t)
	root := t.TempDir()
	lib := ts.library(t, "movies", root)
	it := ts.item(t, &library.Item{LibraryID: lib, Kind: "video", Path: filepath.Join(root, "a.mkv"), Title: "A", ContentRating: "R"})
	ts.user(t, "boss", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	_, adult := ts.user(t, "bob", "adult", false, "")
	const id = "v-aaaaaaaaaaaaaaaaaaaaaaaa-n111111111111"
	ts.HLS.Ensure(hls.Spec{ID: id, ItemID: it.ID, Source: it.Path, InputArgs: []string{"-i", it.Path}, Duration: 60, AudioIndex: -1, SubtitleIndex: -1, AudioCodec: "aac"})
	for _, u := range []string{"playlist.m3u8", "master.m3u8", "seg/0", "subs/0.m3u8"} {
		if w := ts.do(t, "GET", "/api/hls/"+id+"/"+u, kid, nil); w.Code != 404 {
			t.Errorf("%s for a Kids profile: %d %.100s", u, w.Code, w.Body)
		}
	}
	if w := ts.do(t, "GET", "/api/hls/"+id+"/playlist.m3u8", adult, nil); w.Code != 200 {
		t.Errorf("an adult with no limits: %d %.100s", w.Code, w.Body)
	}
	// A stream nobody recorded an item for is refused to anyone with limits.
	const other = "v-bbbbbbbbbbbbbbbbbbbbbbbb-n111111111111"
	ts.HLS.Ensure(hls.Spec{ID: other, Source: "/x", InputArgs: []string{"-i", "/x"}, Duration: 60, AudioIndex: -1, SubtitleIndex: -1, AudioCodec: "aac"})
	if w := ts.do(t, "GET", "/api/hls/"+other+"/playlist.m3u8", kid, nil); w.Code != 404 {
		t.Errorf("no item: %d", w.Code)
	}
}
