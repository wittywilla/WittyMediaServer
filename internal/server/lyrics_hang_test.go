// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/lyrics"
)

// Issue #17: the server "stopped working" while music played with online lyrics, and
// the log had nothing. A browser has six connections to a server. A listener skipping
// through songs with the lyrics open asks for each song's lyrics; if the lyrics service
// is slow, and every look-up waits its turn for minutes, those six are all waiting on
// lyrics, and nothing else the page asks for gets through: not the next song, not a
// page, not the music. From the browser that is the server gone.
func TestSlowLyricsDontStopTheServer(t *testing.T) {
	ts := newTestServer(t)
	_, admin := ts.user(t, "owner", "adult", true, "")
	dir := t.TempDir()
	lib := ts.library(t, "music", dir)
	const songs = 30
	ids := make([]int64, songs)
	for i := range ids {
		song := filepath.Join(dir, fmt.Sprintf("%02d Song.mp3", i))
		if err := os.WriteFile(song, []byte("not really sound"), 0o644); err != nil {
			t.Fatal(err)
		}
		ids[i] = ts.item(t, &library.Item{LibraryID: lib, Kind: "audio", Path: song, Title: fmt.Sprintf("Song %d", i), Artist: "An Artist", Album: "An Album", RuntimeSec: 200}).ID
	}
	// A service that takes the call and never answers.
	hang := make(chan struct{})
	fakeLRCLIB(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hang:
		case <-r.Context().Done():
		}
	})
	t.Cleanup(func() { close(hang) })
	saved := lyrics.RequestTime
	lyrics.RequestTime = 400 * time.Millisecond
	t.Cleanup(func() { lyrics.RequestTime = saved })
	ts.do(t, "PUT", "/api/settings", admin, map[string]string{"lyricsOnline": "1"})

	web := httptest.NewServer(ts.h)
	t.Cleanup(web.Close)
	browser := &http.Client{Transport: &http.Transport{MaxConnsPerHost: 6}} // as a browser has
	t.Cleanup(browser.CloseIdleConnections)
	get := func(path string) (time.Duration, error) {
		req, _ := http.NewRequest("GET", web.URL+path, nil)
		req.Header.Set("Authorization", "Bearer "+admin)
		start := time.Now()
		res, err := browser.Do(req)
		if err != nil {
			return 0, err
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			return 0, fmt.Errorf("%s answered %d", path, res.StatusCode)
		}
		return time.Since(start), nil
	}
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := get(fmt.Sprintf("/api/music/lyrics/%d?fetch=1", id)); err != nil {
				t.Error(err)
			}
		}()
		time.Sleep(5 * time.Millisecond) // skipping, one song after another
	}
	time.Sleep(100 * time.Millisecond)
	took, err := get("/api/status")
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	// Each look-up may take its time once. What mustn't happen is the page waiting for
	// every song's look-up, one after another.
	if limit := 6 * lyrics.RequestTime; took > limit {
		t.Fatalf("the page waited %s for an answer while lyrics were looked up (a look-up may take %s): with the real time limit, that's %s of a server that seems to have stopped",
			took.Round(time.Millisecond), lyrics.RequestTime, (took * time.Duration(saved/lyrics.RequestTime)).Round(time.Second))
	}
}
