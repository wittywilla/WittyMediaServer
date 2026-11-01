package bdremote

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

var (
	builtOnce sync.Once
	builtPath string
	builtErr  error
)

// builtHelper is the real helper, built once for all the tests in here.
func builtHelper(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds the real helper")
	}
	builtOnce.Do(func() {
		dir, err := os.MkdirTemp("", "wittybluray-module-*")
		if err != nil {
			builtErr = err
			return
		}
		builtPath = filepath.Join(dir, "wittybluray-module")
		if runtime.GOOS == "windows" {
			builtPath += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", builtPath, "github.com/wittywilla/wittybluray/cmd/wittybluray-module")
		if b, err := cmd.CombinedOutput(); err != nil {
			builtErr = fmt.Errorf("%w\n%s", err, b)
		}
	})
	if builtErr != nil {
		t.Fatal(builtErr)
	}
	return builtPath
}

// running is the real helper in a real process, speaking the real protocol.
type running struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	conn  *wittymodule.Conn
	mu    sync.Mutex
}

func startHelper(t *testing.T) *running {
	t.Helper()
	cmd := exec.Command(builtHelper(t), wittymodule.HelperFlag)
	cmd.Dir = t.TempDir() // as a server runs it: not where the disc is
	cmd.Env = []string{"GOMAXPROCS=2", "SystemRoot=" + os.Getenv("SystemRoot")}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := &running{cmd: cmd, stdin: stdin, conn: wittymodule.NewConn(stdout, stdin)}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	hello, err := r.conn.ReadHello()
	if err != nil {
		t.Fatal(err)
	}
	if hello.Module != moduleID || hello.API != 2 {
		t.Fatalf("hello: %+v", hello)
	}
	return r
}

func (r *running) Call(method string, params, out any) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conn.Call(method, params, out)
}

func (r *running) Close() { r.stdin.Close() }

func TestRealHelperWalksTheDiscAsTheBuiltinCopyDoes(t *testing.T) {
	path := clearDisc(t)
	built := newWorld(t, nil)
	want := walk(t, built.open(path))
	wantLists := built.lists()

	w := newWorld(t, func() process { return startHelper(t) })
	same(t, "the built helper", walk(t, w.open(path)), want)
	sameLists(t, w.lists(), wantLists)
	if w.engine == nil || w.engine.h == nil || len(w.reports) != 0 {
		t.Fatalf("the helper isn't in use any more: %v", w.reports)
	}
}

// The process really dying, not a test saying it did.
func TestRealHelperKilled(t *testing.T) {
	ctx := context.Background()
	path := clearDisc(t)
	var r *running
	w := newWorld(t, func() process { r = startHelper(t); return r })
	s := w.open(path)
	s.Start(ctx)
	if cur := s.Do(ctx, bdsession.Action{Type: "ended"}); cur.Kind != "title" {
		t.Fatalf("%+v", cur)
	}
	if err := r.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	done := make(chan *bdsession.Presentation, 1)
	go func() { done <- s.Do(ctx, bdsession.Action{Type: "menu", PositionSec: 5}) }()
	select {
	case got := <-done:
		if got.Kind == "stop" || got.Reason != restarted {
			t.Fatalf("%+v", got)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the session hung")
	}
	if w.engine.h != nil {
		t.Error("still using the helper")
	}
	if got := s.Do(ctx, bdsession.Action{Type: "ended"}); got.Kind != "title" {
		t.Errorf("after: %+v", got)
	}
}

// The real manager, a signed install, the real limits on a helper: the whole way.
func TestThroughTheManager(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and packs the real helper")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	key := filepath.Join(out, "test.key")
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := os.WriteFile(key, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := func(args ...string) {
		t.Helper()
		cmd := exec.Command("go", append([]string{"run", "./modules/wittymodule/cmd/wittymodule"}, args...)...)
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("wittymodule %v: %v\n%s", args, err, b)
		}
	}
	tool("check", "modules/wittybluray")
	tool("pack", "-out", out, "-targets", runtime.GOOS+"/"+runtime.GOARCH, "modules/wittybluray")
	real, err := wittymodule.ParseManifest(wittybluray.ManifestJSON)
	if err != nil {
		t.Fatal(err)
	}
	tool("sign", "-key", key, filepath.Join(out, wittymodule.SumsName(real.ID, real.Version)))

	gh := &fakeGitHub{files: map[string][]byte{}}
	rel := update.GHRelease{TagName: "v" + real.Version}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == "test.key" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(out, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		gh.files["/r/"+e.Name()] = b
		rel.Assets = append(rel.Assets, update.GHAsset{Name: e.Name(), Size: int64(len(b)), URL: "https://github.com/r/" + e.Name()})
	}
	gh.rels = []update.GHRelease{rel}

	old := bytes.Replace(wittybluray.ManifestJSON, []byte(`"version": "`+real.Version+`"`), []byte(`"version": "0.0.1"`), 1)
	m := modules.New(modules.Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: real.MinServer, Builtins: [][]byte{old}, Keys: []ed25519.PublicKey{pub},
		Fetch: &update.Fetcher{UserAgent: "test", Transport: gh}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer m.Close()

	path := clearDisc(t)
	built := newWorld(t, nil)
	want := walk(t, built.open(path))

	// Before anything is installed, the hook has nothing to offer.
	w := newWorld(t, nil)
	Use(m)
	s := w.open(path)
	if m.Attach(moduleID, time.Second) != nil {
		t.Fatal("a helper with only the built-in copy in use")
	}
	s.Close()

	m.Check(context.Background(), 0)
	if err := m.Install(moduleID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for {
		st := m.Report().Modules[0]
		if st.Phase == modules.PhaseFailed {
			t.Fatalf("the install failed: %+v", st)
		}
		if st.Phase == modules.PhaseIdle && st.Version == real.Version {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the install never finished: %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}

	w = newWorld(t, nil)
	Use(m)
	inner := bdsession.RemoteEngine
	var got *bdsession.Remote
	bdsession.RemoteEngine = func(p string, o wittybluray.NavOptions, l *slog.Logger) *bdsession.Remote {
		got = inner(p, o, l)
		return got
	}
	s = w.open(path)
	if got == nil {
		t.Fatal("the helper didn't open the disc")
	}
	same(t, "the installed helper", walk(t, s), want)
	if e := got.Disc.(*engine); e.h == nil {
		t.Error("the helper was dropped along the way")
	}
	if st := m.Report().Modules[0]; st.LastFail != "" {
		t.Errorf("a failure was counted: %s", st.LastFail)
	}
	s.Close()
}

type fakeGitHub struct {
	files map[string][]byte
	rels  []update.GHRelease
}

func (g *fakeGitHub) RoundTrip(req *http.Request) (*http.Response, error) {
	answer := func(code int, b []byte) (*http.Response, error) {
		return &http.Response{StatusCode: code, Status: fmt.Sprint(code), Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Header: http.Header{}, Request: req}, nil
	}
	if req.URL.Host == "api.github.com" && strings.Contains(req.URL.Path, "/releases/tags/") {
		return answer(404, nil)
	}
	if req.URL.Host == "api.github.com" {
		b, _ := json.Marshal(g.rels)
		return answer(200, b)
	}
	if b, ok := g.files[req.URL.Path]; ok {
		return answer(200, b)
	}
	return answer(404, nil)
}

// TestRealDisc compares the built-in copy and the built helper on a decrypted Blu-ray
// when WMS_TEST_BD points at one: everything the viewer is shown, the bytes of the
// menus' pictures, and what FFmpeg would read. It only ever reads the disc.
func TestRealDisc(t *testing.T) {
	path := os.Getenv("WMS_TEST_BD")
	if path == "" {
		t.Skip("set WMS_TEST_BD to a decrypted Blu-ray image or folder")
	}
	built := newWorld(t, nil)
	wantShown, wantPics := walkRealDisc(t, built.open(path))
	wantLists := built.lists()

	var r *running
	w := newWorld(t, func() process { r = startHelper(t); return r })
	s := w.open(path)
	if w.engine == nil {
		t.Fatal("the helper didn't open the disc")
	}
	start := time.Now()
	shown, pics := walkRealDisc(t, s)
	t.Logf("%d presentations and %d pictures in %v", len(shown), len(pics), time.Since(start))
	same(t, "the built helper", shown, wantShown)
	if len(pics) != len(wantPics) {
		t.Errorf("%d pictures, want %d", len(pics), len(wantPics))
	}
	for name, b := range wantPics {
		if !bytes.Equal(pics[name], b) {
			t.Errorf("picture %s differs (%d bytes, want %d)", name, len(pics[name]), len(b))
		}
	}
	sameLists(t, w.lists(), wantLists)
	if w.engine.h == nil || len(w.reports) != 0 {
		t.Errorf("the helper was dropped: %v", w.reports)
	}
	if t.Failed() {
		return
	}

	// And it dies mid-film, on a real disc.
	if err := r.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	got := s.Do(context.Background(), bdsession.Action{Type: "menu"})
	if got.Reason != restarted && !strings.Contains(got.Reason, "BD-J") {
		t.Errorf("after the crash: %+v", got)
	}
	if got.Kind == "stop" {
		t.Errorf("stopped: %+v", got)
	}
	if w.engine.h != nil {
		t.Error("still using the helper")
	}
}

var overlayKey = regexp.MustCompile(`/menu/(\d+)/overlay\.png$`)

// walkRealDisc goes through a real disc's first play, its menus and its feature.
func walkRealDisc(t *testing.T, s *bdsession.Session) ([]string, map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	var shown []string
	pics := map[string][]byte{}
	see := func(p *bdsession.Presentation) *bdsession.Presentation {
		shown = append(shown, view(t, s, p))
		if p == nil || p.Menu == nil {
			return p
		}
		if m := overlayKey.FindStringSubmatch(p.Menu.OverlayURL); m != nil {
			key, _ := strconv.Atoi(m[1])
			if b, ok := s.OverlayPNG(key); ok {
				pics[fmt.Sprintf("%d overlay", key)] = b
			}
			for _, b := range p.Menu.Buttons {
				if h, ok := s.HighlightPNG(key, b.N); ok {
					pics[fmt.Sprintf("%d highlight %d", key, b.N)] = h
				}
			}
			for i := range p.Menu.InEffect {
				if e, ok := s.EffectPNG(key, i); ok {
					pics[fmt.Sprintf("%d effect %d", key, i)] = e
				}
			}
		}
		for _, b := range p.Menu.Buttons {
			for _, id := range []int{b.SoundSelect, b.SoundPress} {
				if id > 0 {
					if w, ok := s.SoundWAV(id - 1); ok {
						pics[fmt.Sprintf("sound %d", id)] = w
					}
				}
			}
		}
		return p
	}
	cur := see(s.Start(ctx))
	for i := 0; i < 12 && cur.Kind == "menu" && len(cur.Menu.Buttons) == 0; i++ {
		cur = see(s.Do(ctx, bdsession.Action{Type: "ended"}))
	}
	if cur.Kind == "menu" {
		for _, k := range []string{"down", "right", "up", "left"} {
			cur = see(s.Do(ctx, bdsession.Action{Type: "key", Key: k}))
		}
		cur = see(s.Do(ctx, bdsession.Action{Type: "key", Key: "enter"}))
		cur = see(s.Do(ctx, bdsession.Action{Type: "tick"}))
	}
	cur = see(s.OpenMenu(ctx, "root", 0))
	cur = see(s.PlayTitle(ctx, 0, 0))
	if cur.Kind == "title" && cur.Title != nil {
		ttn := cur.Title.TTN
		for n := range cur.Title.Subtitles {
			if n > 3 {
				break
			}
			evs, err := s.Subtitles(ttn, n, 600, 660)
			if err != nil {
				t.Errorf("subtitles %d: %v", n, err)
				continue
			}
			b, _ := json.Marshal(evs)
			shown = append(shown, strings.ReplaceAll(string(b), s.ID, "SESSION"))
			for _, ev := range evs {
				if name := ev.URL[strings.LastIndexByte(ev.URL, '/')+1:]; name != "" {
					if png, ok := s.SubPNG(name); ok {
						pics["subtitle "+name] = png
					}
				}
			}
		}
		see(s.Do(ctx, bdsession.Action{Type: "tick", PositionSec: 100}))
		if len(cur.Title.Audio) > 1 {
			see(s.Do(ctx, bdsession.Action{Type: "audio", Stream: 1, PositionSec: 100}))
		}
		see(s.Do(ctx, bdsession.Action{Type: "subtitle", Stream: 0, On: true}))
		see(s.Do(ctx, bdsession.Action{Type: "menu", PositionSec: 200}))
		see(s.Do(ctx, bdsession.Action{Type: "resume"}))
		for i := 0; i < 3; i++ {
			see(s.Do(ctx, bdsession.Action{Type: "ended"}))
		}
		see(s.PlayTitle(ctx, ttn, 2))
	}
	return shown, pics
}
