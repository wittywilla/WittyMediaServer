package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittybluray/wittyblurayplayer"
	"github.com/wittywilla/wittydvd/wittydvdplayer"
	"github.com/wittywilla/wittymediaplayer/engine/internal/disc"
	"github.com/wittywilla/wittymediaplayer/engine/internal/imaging"
	"github.com/wittywilla/wittymediaplayer/engine/internal/modhost"
	"github.com/wittywilla/wittymodule"
)

// Nothing leaves the computer: the helper doesn't even contain network code, and
// nothing here can load a decryption library.
func TestNoNetworkCode(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skip("go list:", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		switch {
		case pkg == "net" || strings.HasPrefix(pkg, "net/"), pkg == "crypto/tls", pkg == "plugin":
			t.Errorf("the helper imports %s", pkg)
		case strings.Contains(pkg, "wittybluray/bdj"):
			t.Errorf("the helper imports %s: this player runs no Java", pkg)
		}
	}
}

// startHelper runs the protocol over pipes, as the window does.
func startHelper(t *testing.T) *wittymodule.Conn {
	t.Helper()
	toHelper, fromTest := io.Pipe()
	toTest, fromHelper := io.Pipe()
	t.Setenv("WMP_DATA", t.TempDir())
	h := &helper{mgr: disc.NewManager("0.1.0")}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = wittymodule.Serve(toHelper, fromHelper, wittymodule.Hello{Module: "wittymediaplayer-engine", Version: "test", API: 1}, h.handle)
		h.closeSession()
		fromHelper.Close()
	}()
	t.Cleanup(func() { fromTest.Close(); <-done })
	c := wittymodule.NewConn(toTest, fromTest)
	if _, err := c.ReadHello(); err != nil {
		t.Fatal(err)
	}
	return c
}

// Every module's built-in copy works with this player's version, as a test rather than a
// surprise: a player past a module's range would still use it, but say nothing is wrong.
func TestModulesFitThisPlayer(t *testing.T) {
	m := disc.NewManager(Version)
	if len(m.Modules) != 2 {
		t.Fatalf("%+v", m.Modules)
	}
	for _, raw := range [][]byte{wittydvdplayer.Manifest, wittyblurayplayer.Manifest} {
		man, err := wittymodule.ParseManifest(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Supports(man); err != nil {
			t.Errorf("%s: %v", man.ID, err)
		}
	}
}

// The real helpers, signed and installed as a release would be, play a real disc through
// a process of their own, and leave the window's answers as they were.
func TestRealHelperPlaysADisc(t *testing.T) {
	cases := []struct {
		env, kind, id, entry, pkg string
		raw                       []byte
	}{
		{"WITTY_TEST_DVD", disc.KindDVD, disc.ModuleDVD, "wittydvd-player", "github.com/wittywilla/wittydvd/wittydvdplayer/cmd/wittydvd-player", wittydvdplayer.Manifest},
		{"WITTY_TEST_BD", disc.KindBluray, disc.ModuleBluray, "wittybluray-player", "github.com/wittywilla/wittybluray/wittyblurayplayer/cmd/wittybluray-player", wittyblurayplayer.Manifest},
	}
	for _, c := range cases {
		path := os.Getenv(c.env)
		if path == "" {
			t.Logf("%s is not set", c.env)
			continue
		}
		pub, priv, _ := ed25519.GenerateKey(nil)
		data := t.TempDir()
		t.Setenv("WMP_DATA", data)
		m := disc.NewManager("0.1.0")
		m.Keys = []ed25519.PublicKey{pub}
		man, _ := wittymodule.ParseManifest(c.raw)
		mod, _ := m.Module(c.id)
		// Older built in, so the installed one is the newer.
		for i := range m.Modules {
			if m.Modules[i].ID == c.id {
				m.Modules[i].Builtin = "0.0.1"
			}
		}
		mod.Builtin = "0.0.1"
		dir := m.VersionDir(c.id, man.Version)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		exe := filepath.Join(dir, man.EntryFile(runtime.GOOS))
		if out, err := exec.Command("go", "build", "-o", exe, c.pkg).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", out, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "module.json"), c.raw, 0o644); err != nil {
			t.Fatal(err)
		}
		top := wittymodule.TopFolder(c.id, man.Version, runtime.GOOS, runtime.GOARCH)
		var list strings.Builder
		for _, name := range []string{man.EntryFile(runtime.GOOS), "module.json"} {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&list, "%s  %s/%s\n", modhost.Sum(b), top, name)
		}
		sums := []byte(list.String())
		_ = os.WriteFile(filepath.Join(dir, modhost.SignedList), sums, 0o644)
		_ = os.WriteFile(filepath.Join(dir, modhost.SignedSig), []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sums))+"\n"), 0o644)
		if err := m.Installed(mod, man.Version); err != nil {
			t.Fatal(err)
		}
		if ch := m.Choose(mod); ch.Entry == "" {
			t.Fatalf("not chosen: %+v", ch)
		}
		h := &helper{mgr: m}
		res, _, err := h.handle("open", mustJSON(t, map[string]any{"path": path, "prefs": disc.Prefs{MenuLanguage: "en"}}))
		if err != nil {
			t.Fatal(err)
		}
		if h.remote == nil {
			t.Fatal("the built-in copy was used")
		}
		b, _ := json.Marshal(res)
		var reply openReply
		if err := json.Unmarshal(b, &reply); err != nil || reply.Kind != c.kind || reply.Info == nil || reply.Presentation == nil {
			t.Fatalf("%v %s", err, b)
		}
		res, _, err = h.handle("title", mustJSON(t, map[string]any{"ttn": reply.Info.Main}))
		if err != nil {
			t.Fatal(err)
		}
		b, _ = json.Marshal(res)
		var p disc.Presentation
		if err := json.Unmarshal(b, &p); err != nil || p.Media == nil || len(p.Media.Extents) == 0 {
			t.Fatalf("%v %s", err, b)
		}
		// A helper dying costs that session, and the next open still works.
		_ = h.remote.Kill()
		if _, _, err := h.handle("action", mustJSON(t, map[string]any{"type": "tick"})); err == nil || !strings.Contains(err.Error(), "stopped unexpectedly") {
			t.Errorf("after a crash: %v", err)
		}
		if st := m.LoadState(c.id); st.Failures != 1 {
			t.Errorf("failures %d", st.Failures)
		}
		if _, _, err := h.handle("open", mustJSON(t, map[string]any{"path": path})); err != nil {
			t.Errorf("opening again: %v", err)
		}
		h.closeSession()
	}
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestHelperAnswersPlainly(t *testing.T) {
	c := startHelper(t)
	var list []any
	if _, err := c.Call("drives", nil, &list); err != nil {
		t.Fatalf("listing drives: %v", err)
	}
	dir := t.TempDir()
	movie := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(movie, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var reply openReply
	if _, err := c.Call("open", map[string]any{"path": movie}, &reply); err != nil || reply.Kind != disc.KindFile {
		t.Errorf("a plain file: %+v, %v", reply, err)
	}
	// A folder with no disc in it is refused with its reason, and the helper lives.
	_, err := c.Call("open", map[string]any{"path": dir}, nil)
	ce, ok := err.(*wittymodule.CallError)
	if !ok || ce.Code != disc.CodeNotVideo {
		t.Errorf("an empty folder: %v", err)
	}
	if _, err := c.Call("action", map[string]any{"type": "tick"}, nil); err == nil || !strings.Contains(err.Error(), "no disc") {
		t.Errorf("an action with no disc: %v", err)
	}
	if _, err := c.Call("open", map[string]any{"path": filepath.Join(dir, "missing.iso")}, nil); err == nil || !strings.Contains(err.Error(), "can't be found") {
		t.Errorf("a missing file says: %v", err)
	}
}

func TestHelperPlaysARealDisc(t *testing.T) {
	path := os.Getenv("WITTY_TEST_DVD")
	if path == "" {
		t.Skip("WITTY_TEST_DVD is not set")
	}
	c := startHelper(t)
	var reply openReply
	if _, err := c.Call("open", map[string]any{"path": path, "prefs": disc.Prefs{MenuLanguage: "en"}}, &reply); err != nil {
		t.Fatal(err)
	}
	if reply.Kind != disc.KindDVD || reply.Info == nil || reply.Presentation == nil || reply.Presentation.Media == nil {
		t.Fatalf("%+v", reply)
	}
	var p disc.Presentation
	if _, err := c.Call("title", map[string]any{"ttn": reply.Info.Main}, &p); err != nil || p.Kind == "stop" {
		t.Fatalf("playing the main title: %+v, %v", p, err)
	}
	// The message for a long film must fit the protocol's line limit.
	b, _ := json.Marshal(p)
	if len(b) >= wittymodule.MaxLine {
		t.Errorf("a presentation is %d bytes", len(b))
	}
}

func imageReports(t *testing.T, source, out string) ([]imaging.Progress, int) {
	t.Helper()
	var buf bytes.Buffer
	code := runImage(source, out, nil, &buf)
	var reports []imaging.Progress
	sc := bufio.NewScanner(&buf)
	for sc.Scan() {
		var p imaging.Progress
		if err := json.Unmarshal(sc.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		reports = append(reports, p)
	}
	return reports, code
}

func TestImageOfADataDisc(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "data.iso")
	want := bytes.Repeat([]byte("sector.."), 2048)
	if err := os.WriteFile(in, want, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "copy.img")
	r, code := imageReports(t, in, out)
	if code != 0 || r[len(r)-1].Event != "done" || r[len(r)-1].Kind != disc.KindData {
		t.Fatalf("%d %+v", code, r)
	}
	if got, _ := os.ReadFile(out); !bytes.Equal(got, want) {
		t.Error("the copy differs")
	}
	// It never overwrites, and says so.
	r, code = imageReports(t, in, out)
	if code == 0 || !strings.Contains(r[0].Message, "never overwrites") {
		t.Errorf("%d %+v", code, r)
	}
}

// Closing stdin stops a copy, for the window's Stop button.
func TestImageStopsWhenAskedTo(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "data.iso")
	if err := os.WriteFile(in, make([]byte, 64<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "copy.iso")
	var buf bytes.Buffer
	if code := runImage(in, out, strings.NewReader(""), &buf); code == 0 && !strings.Contains(buf.String(), `"done"`) {
		t.Fatalf("exit 0 with %s", buf.String())
	}
	if strings.Contains(buf.String(), "cancelled") {
		for _, p := range []string{out, out + ".part"} {
			if _, err := os.Stat(p); err == nil {
				t.Errorf("%s was left behind", filepath.Base(p))
			}
		}
	}
}

func TestImageRefusals(t *testing.T) {
	dir := t.TempDir()
	movie := filepath.Join(dir, "movie.mkv")
	if err := os.WriteFile(movie, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, code := imageReports(t, movie, filepath.Join(dir, "out.iso"))
	if code == 0 || len(r) != 1 || r[0].Event != "error" || !strings.Contains(r[0].Message, "Only discs") {
		t.Errorf("imaging a plain file: %d %+v", code, r)
	}
	r, code = imageReports(t, filepath.Join(dir, "missing.iso"), filepath.Join(dir, "out.iso"))
	if code == 0 || len(r) != 1 || r[0].Event != "error" {
		t.Errorf("imaging nothing: %d %+v", code, r)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.iso")); err == nil {
		t.Error("a refused copy left a file")
	}
}
