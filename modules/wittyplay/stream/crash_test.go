package stream

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/wittywilla/wittyplay/ipc"
)

// fakeRunner is this test program started again as a runner: a stand-in for a core
// that crashes, which no test can ship.
func TestFakeRunner(t *testing.T) {
	mode := ""
	if flag.NArg() > 0 {
		mode = flag.Arg(0)
	}
	if mode == "" {
		t.Skip("only runs when a test starts it as a runner")
	}
	crash := func() {
		fmt.Fprintln(os.Stderr, "lots of chatter from the core")
		fmt.Fprintln(os.Stderr, "Exception 0xc0000005 0x0 0xc0 0x7ffae35bc9ab")
		os.Exit(2)
	}
	rd := ipc.NewReader(os.Stdin)
	if _, _, err := rd.Next(); err != nil { // the load command
		os.Exit(1)
	}
	if mode == "crash-loading" {
		crash()
	}
	ready, _ := json.Marshal(ipc.Ready{Name: "fake", Width: 16, Height: 16, FPS: 60, SampleRate: 44100})
	_ = ipc.Write(os.Stdout, ipc.MsgReady, ready)
	for {
		c, _, err := rd.Next()
		if err != nil {
			os.Exit(0)
		}
		if c == ipc.CmdQuit {
			os.Exit(0)
		}
		if c == ipc.CmdPause && mode == "crash-playing" {
			crash()
		}
	}
}

func fakeRunnerManager(t *testing.T, mode string) *Manager {
	t.Helper()
	self, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return NewManager(Config{Runner: self, RunnerArgs: []string{"-test.run=^TestFakeRunner$", "--", mode}, FFmpeg: "ffmpeg-is-not-needed"})
}

func TestCrashWhileLoadingSaysSoPlainly(t *testing.T) {
	m := fakeRunnerManager(t, "crash-loading")
	_, err := m.Start(Options{UserID: 1, ItemID: 7, Title: "Test", Core: "fake", ROM: "fake", SystemDir: t.TempDir(), SaveDir: t.TempDir()})
	if err == nil {
		t.Fatal("a crash while loading must fail the start")
	}
	if !strings.Contains(err.Error(), "emulator crashed") || strings.Contains(err.Error(), "0xc0000005") {
		t.Fatalf("message: %q", err)
	}
	if m.Count() != 0 {
		t.Fatal("no session should remain")
	}
}

func TestCrashWhilePlayingTellsTheViewer(t *testing.T) {
	m := fakeRunnerManager(t, "crash-playing")
	s, err := m.Start(Options{UserID: 1, ItemID: 7, Title: "Test", Core: "fake", ROM: "fake", SystemDir: t.TempDir(), SaveDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.ServeWS(w, r, "jpeg") }))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	for {
		typ, b, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("connection ended without saying why: %v", err)
		}
		if typ != websocket.MessageText {
			continue
		}
		var e ipc.Event
		if json.Unmarshal(b, &e) == nil && e.Type == "ended" {
			if !strings.Contains(e.Text, "emulator crashed") {
				t.Fatalf("message: %q", e.Text)
			}
			return
		}
	}
}

func TestDescribeExit(t *testing.T) {
	for in, want := range map[string]string{
		"Exception 0xc0000005 0x0":            "crashed",
		"signal arrived during external code": "crashed",
		"Exception 0xc000001d illegal":        "this processor",
		"fatal error: runtime: out of memory": "out of memory",
		"mmap: cannot allocate memory":        "out of memory",
		"":                                    "crashed",
	} {
		if got := describeExit(in); !strings.Contains(got, want) {
			t.Errorf("%q: %q", in, got)
		}
	}
}
