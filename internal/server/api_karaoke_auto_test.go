// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
	"github.com/wittywilla/wittymediaserver/internal/pace"
	"github.com/wittywilla/wittymediaserver/internal/separator"
)

// No test finds whatever separator the machine running it happens to have.
func init() { separatorDiscover = func(context.Context) *separator.Found { return nil } }

// foundSeparator makes the server "find" a stand-in separator: a script that waits for a
// file called go beside it, says how far along it is, and writes the parts as copies of
// the song. With a file called fail beside it, it fails instead.
func fakeSeparator(t *testing.T, ts *testServer, parts ...string) (release func(), dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in separator is a shell script")
	}
	dir = t.TempDir()
	script := filepath.Join(dir, "demucs")
	body := "#!/bin/sh\nhere=$(dirname \"$0\")\necho ' 50%|#####     |' >&2\ni=0\nwhile [ ! -e \"$here/go\" ] && [ $i -lt 600 ]; do sleep 0.05; i=$((i+1)); done\n" +
		"[ -e \"$here/fail\" ] && { echo 'no memory left' >&2; exit 3; }\nmkdir -p \"$2/htdemucs/song\"\n"
	for _, p := range parts {
		body += "cp \"$1\" \"$2/htdemucs/song/" + p + "\"\n"
	}
	put(t, script, []byte(body))
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	ts.karaoke.mu.Lock()
	ts.karaoke.found = &separator.Found{Name: "Demucs", Version: "4.0.1", Path: script, Words: []string{script, "{input}", "{output}"}}
	ts.karaoke.looked = time.Now()
	ts.karaoke.mu.Unlock()
	return func() { put(t, filepath.Join(dir, "go"), nil) }, dir
}

// karaokeUntil asks until the answer is what's wanted.
func karaokeUntil(t *testing.T, ts *testServer, path, tok string, want func(karaokeAnswer) bool) karaokeAnswer {
	t.Helper()
	var a karaokeAnswer
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if a = decode[karaokeAnswer](t, ts.do(t, "GET", path, tok, nil)); want(a) {
			return a
		}
	}
	t.Fatalf("never got there: %+v", a)
	return a
}

func TestKaraokeAutoRefusals(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	song, video := f.url("/api/media/%d/karaoke", f.song), f.url("/api/media/%d/karaoke", f.video)
	for _, c := range []struct {
		name, method, path, tok string
		want                    int
	}{
		{"signed out", "GET", song, "", 401},
		{"prepare, signed out", "POST", song + "/prepare", "", 401},
		{"library not allowed", "GET", song, f.blocked, 404},
		{"prepare, library not allowed", "POST", song + "/prepare", f.blocked, 404},
		{"no such song", "POST", "/api/media/999999/karaoke/prepare", f.admin, 404},
		{"status, not an administrator", "GET", "/api/karaoke/status", f.adult, 403},
		{"status, signed out", "GET", "/api/karaoke/status", "", 401},
		{"the whole song, library not allowed", "GET", f.url("/api/media/%d/stems/full", f.song), f.blocked, 404},
		{"the whole song of a film", "GET", f.url("/api/media/%d/stems/full", f.video), f.adult, 404},
	} {
		if w := ts.do(t, c.method, c.path, c.tok, nil); w.Code != c.want {
			t.Errorf("%s: %d %s", c.name, w.Code, w.Body)
		}
	}
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", video+"/prepare", f.adult, nil)); a.State != "unavailable" || a.Reason != "not-a-song" || a.Message == "" {
		t.Errorf("a film: %+v", a)
	}
	// Neither FFmpeg nor a separator here: said plainly, with nothing queued.
	a := decode[karaokeAnswer](t, ts.do(t, "POST", song+"/prepare", f.adult, nil))
	if a.State != "unavailable" || a.Reason != "no-ffmpeg" || a.CanImprove || a.PollSec != 0 || a.Stems == nil {
		t.Errorf("nothing to make it with: %+v", a)
	}
	st := decode[map[string]any](t, ts.do(t, "GET", "/api/karaoke/status?look=1", f.admin, nil))
	if sep := st["separator"].(map[string]any); sep["available"] != false || st["auto"] != "auto" || st["cacheMB"] != float64(2048) {
		t.Errorf("status: %v", st)
	}
	// The owner's switch.
	for _, bad := range []map[string]any{{"karaokeAuto": "sometimes"}, {"karaokeCacheMB": "5"}, {"karaokeCacheMB": "lots"}} {
		if w := ts.do(t, "PUT", "/api/settings", f.admin, bad); w.Code != 400 {
			t.Errorf("%v: %d", bad, w.Code)
		}
	}
	fakeSeparator(t, ts, "vocals.wav")
	if w := ts.do(t, "PUT", "/api/settings", f.admin, map[string]any{"karaokeAuto": "never"}); w.Code != 200 {
		t.Fatalf("never: %d %s", w.Code, w.Body)
	}
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", song+"/prepare", f.admin, nil)); a.Reason != "off" || a.CanImprove {
		t.Errorf("turned off: %+v", a)
	}
	if w := ts.do(t, "PUT", "/api/settings", f.admin, map[string]any{"karaokeAuto": "admin"}); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", song+"/prepare", f.adult, nil)); a.Reason != "admin-only" || a.CanImprove {
		t.Errorf("only when an administrator asks: %+v", a)
	}
	if ts.mediaJobTask(f.url("stems-separate:%d", f.song)) != "" {
		t.Error("something was queued for someone who may not ask")
	}
	// What an owner put beside the song is played whatever the switch says.
	put(t, filepath.Join(filepath.Dir(f.song.Path), "01 Song.vocals.flac"), testFLAC)
	put(t, filepath.Join(filepath.Dir(f.song.Path), "01 Song.no_vocals.flac"), testFLAC)
	a = decode[karaokeAnswer](t, ts.do(t, "GET", song, f.adult, nil))
	if a.State != "ready" || a.Quality != "stems" || a.Mix != "parts" || len(a.Stems) != 2 || a.Stems[0].Role != "vocals" || a.Stems[1].ID != "instrumental" || a.Stems[1].Label != "Music" || a.Version == "" {
		t.Fatalf("stems beside the song: %+v", a)
	}
	if w := ts.do(t, "GET", a.Stems[0].URL, f.adult, nil); w.Code != 200 {
		t.Errorf("a listed stem: %d", w.Code)
	}
	// An instrumental alone is blended against the song itself.
	os.Remove(filepath.Join(filepath.Dir(f.song.Path), "01 Song.vocals.flac"))
	a = decode[karaokeAnswer](t, ts.do(t, "GET", song, f.adult, nil))
	if a.Mix != "blend" || a.Quality != "stems" || len(a.Stems) != 2 || a.Stems[0].ID != "full" || a.Stems[0].Role != "full" {
		t.Fatalf("an instrumental alone: %+v", a)
	}
	if w := ts.do(t, "GET", a.Stems[0].URL, f.adult, nil); w.Code != 200 || !strings.HasPrefix(w.Body.String(), "fLaC") {
		t.Errorf("the whole song: %d", w.Code)
	}
}

func TestKaraokeAutoSeparates(t *testing.T) {
	f := newExtrasFixture(t)
	ts := f.ts
	release, dir := fakeSeparator(t, ts, "vocals.flac", "drums.flac", "bass.flac", "other.flac", "notes.txt")
	song := f.url("/api/media/%d/karaoke", f.song)
	if a := decode[karaokeAnswer](t, ts.do(t, "GET", song, f.adult, nil)); a.State != "unavailable" || a.Reason != "not-made" || !a.CanImprove {
		t.Fatalf("before anyone asked: %+v", a)
	}
	// Anyone who may play the song may ask; asking twice queues one.
	a := decode[karaokeAnswer](t, ts.do(t, "POST", song+"/prepare", f.adult, nil))
	if a.State != "preparing" || a.PollSec == 0 || a.CanImprove {
		t.Fatalf("asked: %+v", a)
	}
	task := ts.mediaJobTask(f.url("stems-separate:%d", f.song))
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", song+"/prepare", f.admin, nil)); a.State != "preparing" || ts.mediaJobTask(f.url("stems-separate:%d", f.song)) != task || task == "" {
		t.Fatalf("asked again: %+v", a)
	}
	// What the separator says about how far it is, is passed on.
	karaokeUntil(t, ts, song, f.adult, func(a karaokeAnswer) bool { return a.Progress == 50 })
	release()
	a = karaokeUntil(t, ts, song, f.adult, func(a karaokeAnswer) bool { return a.State == "ready" })
	if a.Quality != "stems" || a.Mix != "parts" || len(a.Stems) != 4 || a.Stems[0].ID != "vocals" || a.Stems[3].ID != "other" || a.Improving || a.CanImprove || a.PollSec != 0 {
		t.Fatalf("separated: %+v", a)
	}
	for _, st := range a.Stems {
		if w := ts.do(t, "GET", st.URL, f.adult, nil); w.Code != 200 || st.DefaultGain != 1 || st.Label == "" {
			t.Errorf("%s: %d %+v", st.ID, w.Code, st)
		}
	}
	// Asking again makes nothing more.
	if decode[karaokeAnswer](t, ts.do(t, "POST", song+"/prepare", f.adult, nil)); ts.mediaJobTask(f.url("stems-separate:%d", f.song)) != "" {
		t.Error("made again what was there")
	}
	st := decode[map[string]any](t, ts.do(t, "GET", "/api/karaoke/status", f.admin, nil))
	if sep := st["separator"].(map[string]any); sep["name"] != "Demucs" || sep["version"] != "4.0.1" || sep["source"] != "found" || st["songs"] != float64(1) {
		t.Errorf("status: %v", st)
	}
	if got := decode[stemsAnswer](t, ts.do(t, "GET", f.url("/api/media/%d/stems", f.song), f.adult, nil)); got.Separation.Best != "separator" || got.Separation.Separator != "Demucs" {
		t.Errorf("the listing's separation: %+v", got.Separation)
	}

	// A second song, for the limits.
	other := ts.item(t, &library.Item{LibraryID: f.song.LibraryID, Kind: "audio", Path: filepath.Join(filepath.Dir(f.song.Path), "02 Other.flac"), Title: "Other"})
	put(t, other.Path, testFLAC)
	second := fmt.Sprintf("/api/media/%d/karaoke", other.ID)
	key := fmt.Sprintf("stems-separate:%d", other.ID)
	// A tiny machine would take hours: it isn't started, and nobody is told it can be.
	restore := pace.Force(pace.Tiny)
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", second+"/prepare", f.admin, nil)); a.State != "unavailable" || a.CanImprove || ts.mediaJobTask(key) != "" {
		t.Errorf("a tiny machine: %+v", a)
	}
	restore()
	// A small machine that is serving someone waits.
	restore = pace.Force(pace.Small)
	karaokeBusy = func() bool { return true }
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", second+"/prepare", f.adult, nil)); a.Reason != "busy" || !a.CanImprove || ts.mediaJobTask(key) != "" {
		t.Errorf("a busy machine: %+v", a)
	}
	karaokeBusy = pace.Busy
	restore()
	// An account that has asked for its share this hour waits too; an administrator doesn't.
	sam, samTok := ts.user(t, "sam", "adult", false, "")
	for i := 0; i < karaokeRealPerHour; i++ {
		ts.karaoke.allow("real", sam.ID, karaokeRealPerHour)
	}
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", second+"/prepare", samTok, nil)); a.Reason != "rate-limited" || ts.mediaJobTask(key) != "" {
		t.Errorf("over the limit: %+v", a)
	}
	// A separator that fails: said plainly, and not tried again by itself.
	put(t, filepath.Join(dir, "fail"), nil)
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", second+"/prepare", f.admin, nil)); a.State != "preparing" {
		t.Fatalf("an administrator asks: %+v", a)
	}
	a = karaokeUntil(t, ts, second, f.adult, func(a karaokeAnswer) bool { return a.State == "unavailable" })
	if a.Reason != "failed" || a.CanImprove {
		t.Errorf("after failing: %+v", a)
	}
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", second+"/prepare", f.admin, nil)); a.State != "unavailable" || ts.mediaJobTask(key) != "" {
		t.Errorf("tried again straight away: %+v", a)
	}
}

func TestKaraokeLimitsAreBounded(t *testing.T) {
	var k karaokeAuto
	for i := 0; i < karaokeRealPerHour; i++ {
		if !k.allow("real", 7, karaokeRealPerHour) {
			t.Fatalf("ask %d refused", i)
		}
	}
	if k.allow("real", 7, karaokeRealPerHour) || !k.allow("real", 8, karaokeRealPerHour) || !k.allow("approx", 7, karaokeApproxPerHour) {
		t.Error("the limit is per account and per kind")
	}
	for i := int64(0); i < 3*maxKaraokeNotes; i++ {
		k.allow("real", 100+i, 1)
		k.fail("real", i, "failed")
	}
	if len(k.asks) > maxKaraokeNotes || len(k.failed) > maxKaraokeNotes {
		t.Errorf("%d accounts and %d failures remembered", len(k.asks), len(k.failed))
	}
	var got []int
	p := &percentWatcher{last: -1, say: func(n int) { got = append(got, n) }}
	for _, chunk := range []string{"  3%|▎  | 1/40\r 1", "2%|█ | 5/40\r 12%| again\r100%|done 250% 1234% %"} {
		_, _ = p.Write([]byte(chunk))
	}
	if fmt.Sprint(got) != "[3 12 100]" {
		t.Errorf("progress read as %v", got)
	}
}

// With a real FFmpeg: the quick version is made at once and played while the separator
// works, and the real parts take its place when they're done.
func TestKaraokeAutoUpgradesFromApproximate(t *testing.T) {
	tools, err := ffmpeg.Locate("", "")
	if err != nil {
		t.Skip("no FFmpeg here")
	}
	ts := newTestServerWith(t, tools)
	root := t.TempDir()
	_, adult := ts.user(t, "alex", "adult", false, "")
	music := ts.library(t, "music", filepath.Join(root, "music"))
	const rate = 44100
	n := rate * 2
	left, right, mono := make([]float64, n), make([]float64, n), make([]float64, n)
	for i := range left {
		ti := float64(i) / rate
		mono[i] = math.Sin(2 * math.Pi * 1000 * ti)
		left[i], right[i] = mono[i]+math.Sin(2*math.Pi*400*ti), mono[i]+math.Sin(2*math.Pi*600*ti)
	}
	songPath, monoPath := filepath.Join(root, "music", "Album", "Song.wav"), filepath.Join(root, "music", "Album", "Mono.wav")
	put(t, songPath, stereoWAV(rate, left, right))
	put(t, monoPath, wav(rate, s16(mono)))
	song := ts.item(t, &library.Item{LibraryID: music, Kind: "audio", Path: songPath, Title: "Song"})
	monoSong := ts.item(t, &library.Item{LibraryID: music, Kind: "audio", Path: monoPath, Title: "Mono"})

	// No separator: a mono song has nothing for the quick method, and that's remembered.
	monoURL := fmt.Sprintf("/api/media/%d/karaoke", monoSong.ID)
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", monoURL+"/prepare", adult, nil)); a.State != "preparing" {
		t.Fatalf("mono, asked: %+v", a)
	}
	if a := karaokeUntil(t, ts, monoURL, adult, func(a karaokeAnswer) bool { return a.State != "preparing" }); a.Reason != "mono" {
		t.Errorf("a mono song: %+v", a)
	}
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", monoURL+"/prepare", adult, nil)); a.State != "unavailable" || a.Reason != "mono" {
		t.Errorf("mono, asked again: %+v", a)
	}

	release, _ := fakeSeparator(t, ts, "vocals.wav", "no_vocals.wav")
	url := fmt.Sprintf("/api/media/%d/karaoke", song.ID)
	if a := decode[karaokeAnswer](t, ts.do(t, "POST", url+"/prepare", adult, nil)); a.State != "preparing" {
		t.Fatalf("asked: %+v", a)
	}
	quick := karaokeUntil(t, ts, url, adult, func(a karaokeAnswer) bool { return a.State == "ready" })
	if quick.Quality != "approximate" || quick.Mix != "blend" || len(quick.Stems) != 2 || quick.Stems[0].ID != "full" || quick.Stems[1].ID != "instrumental" || !quick.Improving || quick.PollSec == 0 {
		t.Fatalf("the quick version: %+v", quick)
	}
	sizes := map[string]int{}
	for _, st := range quick.Stems {
		w := ts.do(t, "GET", st.URL, adult, nil)
		if w.Code != 200 || w.Header().Get("Content-Type") != "audio/mp4" || st.MIME != "audio/mp4" {
			t.Errorf("%s: %d %s", st.ID, w.Code, w.Header().Get("Content-Type"))
		}
		sizes[st.ID] = w.Body.Len()
	}
	if sizes["full"] < 1000 || sizes["instrumental"] < 1000 {
		t.Errorf("sizes: %v", sizes)
	}
	release()
	real := karaokeUntil(t, ts, url, adult, func(a karaokeAnswer) bool { return a.Quality == "stems" })
	if real.State != "ready" || real.Mix != "parts" || len(real.Stems) != 2 || real.Version == quick.Version || real.Improving || real.PollSec != 0 {
		t.Fatalf("the real version: %+v", real)
	}
	// The WAVs the separator wrote were made small, and the quick pair is gone.
	for _, st := range real.Stems {
		if st.MIME != "audio/mp4" {
			t.Errorf("%s wasn't compacted: %s", st.ID, st.MIME)
		}
	}
	dir := filepath.Join(ts.extrasRoot(), mediaextras.Key(song.ID, song.Path))
	if left, _ := os.ReadDir(filepath.Join(dir, mediaextras.ApproxDir)); len(left) != 0 {
		t.Errorf("the quick pair is still there: %v", left)
	}
	if used, songs := mediaextras.KaraokeUsage(ts.extrasRoot()); songs != 1 || used == 0 {
		t.Errorf("usage: %d bytes, %d songs", used, songs)
	}
	// Over the cap, what was made for a song nobody is singing goes; asking makes it again.
	if n, _ := mediaextras.EvictKaraoke(ts.extrasRoot(), 0, ""); n != 1 {
		t.Fatalf("evicted %d", n)
	}
	if a := decode[karaokeAnswer](t, ts.do(t, "GET", url, adult, nil)); a.State != "unavailable" || a.Reason != "not-made" || !a.CanImprove {
		t.Errorf("after eviction: %+v", a)
	}
}
