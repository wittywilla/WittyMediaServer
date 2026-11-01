package livetv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittytuner"
)

// liveFake is a playlist server whose live playlist grows, and can add a key partway
// through. It has no key to give: the address in the playlist answers 404.
type liveFake struct {
	srv     *httptest.Server
	locked  atomic.Bool
	down    atomic.Bool
	asked   atomic.Int64
	started time.Time
}

func newLiveFake(t *testing.T, segment []byte) *liveFake {
	t.Helper()
	f := &liveFake{started: time.Now()}
	mux := http.NewServeMux()
	mux.HandleFunc("/list.m3u", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1,Live\n%s/live.m3u8\n", f.srv.URL)
	})
	mux.HandleFunc("/live.m3u8", func(w http.ResponseWriter, r *http.Request) {
		f.asked.Add(1)
		if f.down.Load() {
			w.WriteHeader(503)
			return
		}
		seq := int(time.Since(f.started) / time.Second)
		fmt.Fprintf(w, "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:1\n#EXT-X-MEDIA-SEQUENCE:%d\n", seq)
		if f.locked.Load() {
			fmt.Fprint(w, "#EXT-X-KEY:METHOD=SAMPLE-AES,URI=\"/key\"\n")
		}
		for i := range 3 {
			fmt.Fprintf(w, "#EXTINF:1,\nseg%d.ts\n", seq+i)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".ts") {
			_, _ = w.Write(segment)
			return
		}
		w.WriteHeader(404)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func liveChannel(t *testing.T, m *Manager, f *liveFake) *Channel {
	t.Helper()
	tn, err := m.Create(context.Background(), &Tuner{Driver: "iptv", Name: "Live", Options: map[string]string{"playlist": f.srv.URL + "/list.m3u"}})
	if err != nil {
		t.Fatal(err)
	}
	ch, err := m.Channels(context.Background(), tn.ID)
	if err != nil || len(ch) != 1 {
		t.Fatalf("%v %+v", err, ch)
	}
	return ch[0]
}

func refusedIn(t *testing.T, o *Opened, within time.Duration) {
	t.Helper()
	select {
	case <-o.Refused():
	case <-time.After(within):
		t.Fatalf("not refused within %v", within)
	}
}

// A playing stream that adds a key is stopped, and the channel is marked as any other
// encrypted channel is.
func TestKeyAddedPartwayRefusesAPlayingStream(t *testing.T) {
	m := testManager(t)
	f := newLiveFake(t, nil)
	c := liveChannel(t, m, f)
	ctx := context.Background()
	o, err := m.Open(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Release()
	if o.Refusal() != nil {
		t.Fatal("clear so far")
	}
	f.locked.Store(true)
	refusedIn(t, o, 12*time.Second)
	if !wittytuner.IsEncrypted(o.Refusal()) || !strings.Contains(o.Refusal().Error(), wittytuner.EncryptedLabel) {
		t.Errorf("%v", o.Refusal())
	}
	got, err := m.Channel(ctx, c.ID)
	if err != nil || got.Encrypted == "" {
		t.Errorf("not marked: %+v %v", got, err)
	}
	if _, err := m.Open(ctx, c.ID); !wittytuner.IsEncrypted(err) {
		t.Errorf("it opened again: %v", err)
	}
}

// A key already there at opening marks the channel too, on the way to being refused.
func TestKeyAtOpeningMarksTheChannel(t *testing.T) {
	m := testManager(t)
	f := newLiveFake(t, nil)
	c := liveChannel(t, m, f)
	f.locked.Store(true)
	ctx := context.Background()
	if _, err := m.Open(ctx, c.ID); !wittytuner.IsEncrypted(err) {
		t.Fatalf("%v", err)
	}
	if got, err := m.Channel(ctx, c.ID); err != nil || got.Encrypted == "" {
		t.Errorf("%+v %v", got, err)
	}
}

// Encrypted is final, and only that: the playlist being out of reach ends nothing.
func TestWatchGoesOnThroughFailures(t *testing.T) {
	calls := 0
	o := &Opened{Stream: &wittytuner.Stream{}, Channel: &Channel{Name: "Live"}, refused: make(chan struct{})}
	o.note = func(string) {}
	o.Stream.Recheck = func(context.Context) (string, time.Duration, error) {
		calls++
		switch {
		case calls < 4:
			return "", time.Millisecond, errors.New("no network")
		case calls < 6:
			return "", time.Millisecond, nil
		}
		return "its HLS playlist uses DRM", 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	o.watch(ctx, slog.Default())
	if !wittytuner.IsEncrypted(o.Refusal()) || calls != 6 {
		t.Errorf("%v after %d", o.Refusal(), calls)
	}
}

func TestStreamGoesOnWhilePlaylistIsOutOfReach(t *testing.T) {
	m := testManager(t)
	f := newLiveFake(t, nil)
	c := liveChannel(t, m, f)
	o, err := m.Open(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Release()
	f.down.Store(true)
	before := f.asked.Load()
	deadline := time.Now().Add(12 * time.Second)
	for f.asked.Load() == before && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if f.asked.Load() == before {
		t.Fatal("it never looked again")
	}
	if o.Refusal() != nil {
		t.Errorf("%v", o.Refusal())
	}
	if got, err := m.Channel(context.Background(), c.ID); err != nil || got.Encrypted != "" {
		t.Errorf("%+v %v", got, err)
	}
}

// Releasing a stream ends its looking.
func TestReleaseStopsLooking(t *testing.T) {
	m := testManager(t)
	f := newLiveFake(t, nil)
	c := liveChannel(t, m, f)
	o, err := m.Open(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	o.Release()
	time.Sleep(200 * time.Millisecond)
	before := f.asked.Load()
	f.locked.Store(true)
	time.Sleep(6 * time.Second)
	if n := f.asked.Load(); n != before || o.Refusal() != nil {
		t.Errorf("asked %d more times, refused: %v", n-before, o.Refusal())
	}
}

// A recording stops with the viewer's reason, and keeps what was clear until then.
func TestRecordingKeepsWhatWasClearWhenAKeyComes(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("needs FFmpeg")
	}
	seg := filepath.Join(t.TempDir(), "seg.ts")
	gen := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "testsrc=s=64x64:r=5:d=1", "-c:v", "mpeg2video", "-f", "mpegts", seg)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Skipf("can't make a segment: %v %s", err, out)
	}
	segment, err := os.ReadFile(seg)
	if err != nil {
		t.Fatal(err)
	}
	m := testManager(t)
	m.Tools = &ffmpeg.Tools{FFmpeg: bin}
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	finished := make(chan string, 1)
	m.AfterRecording = func(p string) { finished <- p }
	f := newLiveFake(t, segment)
	c := liveChannel(t, m, f)
	ctx := context.Background()
	now := time.Now().Unix()
	rec, err := m.Schedule(ctx, c.ID, 1, "News", "", now, now+120)
	if err != nil {
		t.Fatal(err)
	}
	m.startDue(ctx)
	// Long enough for FFmpeg to have written some of it.
	var path string
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		got, _ := m.recording(ctx, rec.ID)
		if got.Path != "" {
			if st, err := os.Stat(got.Path); err == nil && st.Size() > 0 {
				path = got.Path
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if path == "" {
		t.Fatal("nothing was written while it was clear")
	}
	f.locked.Store(true)
	select {
	case p := <-finished:
		if p != path {
			t.Errorf("told of %s", p)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the recording went on")
	}
	got, err := m.recording(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" || !strings.Contains(got.Error, wittytuner.EncryptedLabel) {
		t.Errorf("%+v", got)
	}
	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		t.Errorf("what was clear is gone: %v", err)
	}
	if ch, err := m.Channel(ctx, c.ID); err != nil || ch.Encrypted == "" {
		t.Errorf("not marked: %+v %v", ch, err)
	}
}
