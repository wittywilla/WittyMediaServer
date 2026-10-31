package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaplayer/engine/internal/disc"
	"github.com/wittywilla/wittymediaplayer/engine/internal/imaging"
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
	h := &helper{}
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
	if _, err := c.Call("open", map[string]any{"path": filepath.Join(dir, "missing.iso")}, nil); err == nil {
		t.Error("a missing file opened")
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
	code := runImage(source, out, strings.NewReader(""), &buf)
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
