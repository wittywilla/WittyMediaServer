package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// cdImage writes a generated two-track disc and adds it as an item.
func cdImage(t *testing.T, ts *contractServer) (id int64, pcm []byte) {
	t.Helper()
	dir := t.TempDir()
	pcm = make([]byte, 450*2352)
	for i := range 450 * 588 {
		v := int16(9000 * math.Sin(2*math.Pi*(300*float64(i)/44100+float64(i)*float64(i)/4e9)))
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(v))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(v))
	}
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), pcm, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ts.cds.closeAll)
	cue := filepath.Join(dir, "a.cue")
	sheet := "TITLE \"Generated\"\nFILE \"a.bin\" BINARY\nTRACK 01 AUDIO\nTITLE \"One\"\nINDEX 01 00:00:00\nTRACK 02 AUDIO\nFLAGS PRE\nINDEX 00 00:02:00\nINDEX 01 00:03:00\nINDEX 02 00:04:00\n"
	if err := os.WriteFile(cue, []byte(sheet), 0o644); err != nil {
		t.Fatal(err)
	}
	return ts.item(t, "cd-image", cue), pcm
}

func TestCDRoutes(t *testing.T) {
	ts := newContractServer(t)
	_, session := ts.account(t, "listener", "adult", false)
	id, pcm := cdImage(t, ts)

	w := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d", id), session, nil)
	if w.Code != 200 {
		t.Fatalf("disc: %d %s", w.Code, w.Body.String())
	}
	v := decode[cdView](t, w)
	if v.Format != "cue" || v.DiscID == "" || len(v.Tracks) != 2 || v.Samples != int64(len(pcm)/4) {
		t.Fatalf("disc = %+v", v)
	}
	two := v.Tracks[1]
	if two.Pregap != 150 || two.Start != 225 || two.End != 450 || !two.PreEmphasis || len(two.Indexes) != 1 || v.Tracks[0].Title != "One" {
		t.Fatalf("tracks = %+v", v.Tracks)
	}

	// WAV: the image, byte for byte, with ranges.
	wav := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d/audio?format=wav&deemphasis=0", id), session, nil)
	if wav.Code != 200 || wav.Header().Get("Content-Type") != "audio/wav" || !bytes.Equal(wav.Body.Bytes()[44:], pcm) {
		t.Fatalf("wav: %d %s, %d bytes", wav.Code, wav.Header().Get("Content-Type"), wav.Body.Len())
	}

	// FLAC: a stream, with the program's length in its header and in X-WMS-Samples.
	fl := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d/audio?start=1000&tracks=2,1&loops=2", id), session, nil)
	if fl.Code != 200 || fl.Header().Get("Content-Type") != "audio/flac" || string(fl.Body.Bytes()[:4]) != "fLaC" {
		t.Fatalf("flac: %d %s", fl.Code, fl.Header().Get("Content-Type"))
	}
	wantSamples := (225+150)*2*588 - 1000
	if got := fl.Header().Get("X-WMS-Samples"); got != fmt.Sprint(wantSamples) {
		t.Fatalf("samples = %s, want %d", got, wantSamples)
	}

	// Nonsense is refused rather than guessed at.
	for _, q := range []string{"tracks=9", "tracks=x", "loops=1000", "format=ogg", "start=-1"} {
		if c := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d/audio?%s", id, q), session, nil).Code; c != 400 {
			t.Errorf("%s: %d", q, c)
		}
	}
	// No FFmpeg on this test server: compressed audio isn't offered, and says why.
	if c := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d/audio?format=aac", id), session, nil).Code; c != 503 {
		t.Errorf("aac without FFmpeg: %d", c)
	}

	// Signing in is required, and a ticket opens only its own disc's media.
	if c := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d", id), "", nil).Code; c != 401 {
		t.Errorf("no sign-in: %d", c)
	}
	tok := mintFor(t, ts, session, id)
	if c := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d/audio?format=wav&st=%s", id, tok), "", nil).Code; c != 200 {
		t.Errorf("with a ticket: %d", c)
	}
	other, _ := cdImage(t, ts)
	if c := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d/audio?format=wav&st=%s", other, tok), "", nil).Code; c != 401 {
		t.Errorf("a ticket for another disc: %d", c)
	}
	// Choosing what a disc is belongs to an admin.
	if c := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d/matches", id), session, nil).Code; c != 403 {
		t.Errorf("matches as a listener: %d", c)
	}
	if c := ts.do(t, "POST", fmt.Sprintf("/api/cd/%d/identify", id), session, map[string]string{"releaseId": ""}).Code; c != 403 {
		t.Errorf("identify as a listener: %d", c)
	}

	// Anything else isn't a disc.
	movie := ts.item(t, "video", tempMedia(t, "m.mkv", "x"))
	if c := ts.do(t, "GET", fmt.Sprintf("/api/cd/%d", movie), session, nil).Code; c != 404 {
		t.Errorf("a film as a disc: %d", c)
	}
}

func TestCDTrackPlaysThroughTheOrdinaryAudioRoute(t *testing.T) {
	ts := newContractServer(t)
	_, session := ts.account(t, "listener", "adult", false)
	id, pcm := cdImage(t, ts)
	ctx := context.Background()
	it, err := ts.Store.Item(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ts.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, added_at, updated_at, video_codec, cd_disc) VALUES(?,?,?,?,?,1,1,?,?)`,
		it.LibraryID, "audio", it.Path+"#track=1", "One", "One", "pcm_s16le", id)
	if err != nil {
		t.Fatal(err)
	}
	track, _ := res.LastInsertId()
	w := ts.do(t, "GET", fmt.Sprintf("/api/items/%d/stream", track), session, nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes()[44:], pcm[:150*2352]) {
		t.Fatalf("track stream: %d, %d bytes", w.Code, w.Body.Len())
	}
	// And seconds, as other audio asks for them.
	w = ts.do(t, "GET", fmt.Sprintf("/api/items/%d/stream?start=1", track), session, nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes()[44:], pcm[44100*4:150*2352]) {
		t.Fatalf("track stream from 1 s: %d, %d bytes", w.Code, w.Body.Len())
	}
	// The playback door answers with the disc's stream and its map.
	p := ts.do(t, "POST", fmt.Sprintf("/api/items/%d/playback", id), session, map[string]any{"profile": "web"})
	if p.Code != http.StatusOK {
		t.Fatalf("playback: %d %s", p.Code, p.Body.String())
	}
	b := decode[playbackResponse](t, p)
	if b.Decision != "cd" || b.CDURL == "" || b.URL == "" {
		t.Fatalf("playback = %+v", b)
	}
}
