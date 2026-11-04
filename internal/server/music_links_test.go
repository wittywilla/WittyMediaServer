package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

func musicLinksOf(t *testing.T, ts *testServer, token string, id int64) MusicLinks {
	t.Helper()
	w := ts.do(t, "GET", fmt.Sprintf("/api/music/links/%d", id), token, nil)
	if w.Code != 200 {
		t.Fatalf("links of %d: %d %s", id, w.Code, w.Body)
	}
	var out MusicLinks
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMusicVideoLinks(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, kid := ts.user(t, "kid", "kids", false, "")
	music := ts.library(t, "music", t.TempDir())
	videos := ts.library(t, "musicvideos", t.TempDir())
	song := ts.item(t, &library.Item{LibraryID: music, Kind: "audio", Path: filepath.Join(t.TempDir(), "01 Lights.flac"), Title: "Lights", Artist: "Cinnamon Chasers", AlbumArtist: "Cinnamon Chasers", Album: "A Hundred Suns", RuntimeSec: 300, ContentRating: "G"})
	other := ts.item(t, &library.Item{LibraryID: music, Kind: "audio", Path: filepath.Join(t.TempDir(), "02 Autumn.flac"), Title: "Autumn", Artist: "Tes Elations", RuntimeSec: 200})
	video := ts.item(t, &library.Item{LibraryID: videos, Kind: "video", Path: filepath.Join(t.TempDir(), "Cinnamon Chasers - Lights.webm"), Title: "Lights (Official Video)", Studio: "Cinnamon Chasers", RuntimeSec: 301})
	close := ts.item(t, &library.Item{LibraryID: videos, Kind: "video", Path: filepath.Join(t.TempDir(), "Tes Elations - Autumn (Live).webm"), Title: "Autumn (Live)", Studio: "Tes Elations", RuntimeSec: 260})

	w := ts.do(t, "POST", "/api/music/links/match", admin, nil)
	if w.Code != 200 {
		t.Fatalf("match: %d %s", w.Code, w.Body)
	}
	got := musicLinksOf(t, ts, admin, song.ID)
	if got.Role != "song" || len(got.Tied) != 1 || got.Tied[0].VideoID != video.ID || got.Tied[0].SyncMS == nil || *got.Tied[0].SyncMS != 0 {
		t.Fatalf("the song isn't tied to its video and lined up: %+v", got)
	}
	if got.Tied[0].Video == nil || got.Tied[0].Video.Artist != "Cinnamon Chasers" || got.Tied[0].Song.Album != "A Hundred Suns" {
		t.Fatalf("what's at both ends: %+v %+v", got.Tied[0].Song, got.Tied[0].Video)
	}
	if got := musicLinksOf(t, ts, admin, close.ID); got.Role != "video" || len(got.Tied) != 0 || len(got.Suggested) != 1 || got.Suggested[0].SongID != other.ID {
		t.Fatalf("a live take was tied, or not suggested: %+v", got)
	}

	// A profile sees only ties to what it may see: the video has no rating, the song does.
	if got := musicLinksOf(t, ts, kid, song.ID); len(got.Tied) != 0 || len(got.Suggested) != 0 || got.CanEdit {
		t.Fatalf("a kid sees a video they may not watch: %+v", got)
	}
	if w := ts.do(t, "GET", fmt.Sprintf("/api/music/links/%d", video.ID), kid, nil); w.Code != 404 {
		t.Fatalf("a kid can ask about a video they can't see: %d", w.Code)
	}
	for _, r := range [][2]string{{"PUT", "/api/music/links"}, {"POST", "/api/music/links/match"}, {"GET", "/api/music/links"}} {
		if w := ts.do(t, r[0], r[1], kid, map[string]any{"songId": song.ID, "videoId": video.ID}); w.Code != 403 {
			t.Fatalf("%s %s as a kid: %d", r[0], r[1], w.Code)
		}
	}

	// Untied by hand: no match ties them again.
	if w := ts.do(t, "DELETE", fmt.Sprintf("/api/music/links?song=%d&video=%d", song.ID, video.ID), admin, nil); w.Code != 204 {
		t.Fatalf("untie: %d %s", w.Code, w.Body)
	}
	ts.do(t, "POST", "/api/music/links/match", admin, nil)
	if got := musicLinksOf(t, ts, admin, song.ID); len(got.Tied) != 0 {
		t.Fatalf("an untied pair came back: %+v", got)
	}

	// Tied by hand, with an intro of 4 seconds: it stays through a match, and moving the
	// video to another song unties the first.
	syncMS := int64(4000)
	if w := ts.do(t, "PUT", "/api/music/links", admin, map[string]any{"songId": song.ID, "videoId": close.ID, "syncMs": syncMS}); w.Code != 200 {
		t.Fatalf("tie: %d %s", w.Code, w.Body)
	}
	ts.do(t, "POST", "/api/music/links/match", admin, nil)
	got = musicLinksOf(t, ts, admin, close.ID)
	if len(got.Tied) != 1 || got.Tied[0].SongID != song.ID || got.Tied[0].How != "hand" || got.Tied[0].By != "owner" || *got.Tied[0].SyncMS != 4000 || len(got.Suggested) != 0 {
		t.Fatalf("a tie by hand didn't hold: %+v", got)
	}
	if w := ts.do(t, "PUT", "/api/music/links", admin, map[string]any{"songId": other.ID, "videoId": close.ID}); w.Code != 200 {
		t.Fatalf("tie: %d %s", w.Code, w.Body)
	}
	if got := musicLinksOf(t, ts, admin, song.ID); len(got.Tied) != 0 {
		t.Fatalf("a video has two songs: %+v", got)
	}

	// Only a song and a video can be tied.
	if w := ts.do(t, "PUT", "/api/music/links", admin, map[string]any{"songId": video.ID, "videoId": song.ID}); w.Code != 400 {
		t.Fatalf("tied the wrong way round: %d", w.Code)
	}
	if w := ts.do(t, "PUT", "/api/music/links", admin, map[string]any{"songId": song.ID, "videoId": video.ID, "syncMs": 3600000}); w.Code != 400 {
		t.Fatalf("an hour's offset: %d", w.Code)
	}

	// Choosing by hand: videos for a song.
	w = ts.do(t, "GET", fmt.Sprintf("/api/music/links/%d/candidates?q=lights", song.ID), admin, nil)
	var cands []MusicLinkSide
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &cands) != nil || len(cands) != 1 || cands[0].ID != video.ID {
		t.Fatalf("candidates: %d %s", w.Code, w.Body)
	}

	// Removing the video's library takes its links with it.
	if _, err := ts.DB.ExecContext(context.Background(), `DELETE FROM libraries WHERE id = ?`, videos); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = ts.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM music_video_links`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d links outlived their video", n)
	}
}
