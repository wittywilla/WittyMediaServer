// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/instance"
)

// The test binary is the server too: started with WMS_TEST_RUN_SERVER=1 it runs as one,
// with the arguments after "--".
func TestMain(m *testing.M) {
	if os.Getenv("WMS_TEST_RUN_SERVER") == "1" {
		for i, a := range os.Args {
			if a == "--" {
				os.Exit(run(os.Args[i+1:]))
			}
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

type testServer struct {
	cmd   *exec.Cmd
	out   *syncBuffer
	dir   string
	stdin io.WriteCloser // with -watch-stdin, as the tray app runs it
}

// syncBuffer is the server's output, read while it's still being written.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}
func (b *syncBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startServer runs a server on its own folder, with an empty config file so nothing from
// this machine's own config is used, and never a browser or a dialog.
func startServer(t *testing.T, dir string, port int, extra ...string) *testServer {
	t.Helper()
	conf := filepath.Join(t.TempDir(), "wms.json")
	if err := os.WriteFile(conf, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^$", "--", "serve", "-config", conf, "-data", dir, "-listen", "127.0.0.1:" + strconv.Itoa(port)}, extra...)...)
	cmd.Env = append(os.Environ(), "WMS_TEST_RUN_SERVER=1", "WMS_NO_BROWSER=1", "WMS_NO_DIALOG=1", "WMS_DATA_DIR=", "WMS_LISTEN=", "WMS_CONFIG=", "WMS_RESTARTED=")
	out := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = out, out
	s := &testServer{cmd: cmd, out: out, dir: dir}
	for _, a := range extra {
		if a == "-watch-stdin" {
			in, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			s.stdin = in
		}
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return s
}

// exitWithin waits for a server that's meant to end by itself.
func (s *testServer) exitWithin(t *testing.T, d time.Duration) int {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		if err != nil {
			t.Fatal(err)
		}
		return 0
	case <-time.After(d):
		_ = s.cmd.Process.Kill()
		<-done
		t.Fatalf("still running after %v:\n%s", d, s.out.String())
	}
	return -1
}

func needFFmpeg(t *testing.T) {
	t.Helper()
	if os.Getenv("WMS_FFMPEG") != "" {
		return
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("a server needs FFmpeg to start: none on PATH, and WMS_FFMPEG isn't set")
	}
}

// A second start on a data folder in use says where the running server is and exits 3,
// before it opens anything; another folder on another port runs beside it; ports taken
// by another server or another program are named as such; and a server that's killed
// leaves nothing that stops the next start.
func TestSecondStart(t *testing.T) {
	if testing.Short() {
		t.Skip("starts real servers")
	}
	needFFmpeg(t)
	root := t.TempDir()
	dataA := filepath.Join(root, "a")
	portA := freePort(t)
	first := startServer(t, dataA, portA)
	r := instance.Find(context.Background(), dataA, serverUp)
	if !r.Answering {
		t.Fatalf("the first server didn't come up:\n%s", first.out.String())
	}
	if want := "http://127.0.0.1:" + strconv.Itoa(portA); r.URL != want || r.PID != first.cmd.Process.Pid {
		t.Fatalf("instance.json: %+v, want %s and pid %d", r, want, first.cmd.Process.Pid)
	}

	t.Run("same folder", func(t *testing.T) {
		// Even on another port: it's the folder that counts.
		second := startServer(t, dataA, freePort(t))
		if code := second.exitWithin(t, 60*time.Second); code != instance.ExitRunning {
			t.Fatalf("exit %d, want %d:\n%s", code, instance.ExitRunning, second.out.String())
		}
		if out := second.out.String(); !strings.Contains(out, "already running at "+r.URL) || strings.Contains(out, "listening") {
			t.Fatalf("output:\n%s", out)
		}
	})

	t.Run("another folder and port", func(t *testing.T) {
		dataB := filepath.Join(root, "b")
		other := startServer(t, dataB, freePort(t))
		rb := instance.Find(context.Background(), dataB, serverUp)
		if !rb.Answering || rb.Instance == r.Instance {
			t.Fatalf("second server: %+v\n%s", rb, other.out.String())
		}
		_ = other.cmd.Process.Kill()
	})

	t.Run("port of another server", func(t *testing.T) {
		s := startServer(t, filepath.Join(root, "c"), portA)
		if code := s.exitWithin(t, 60*time.Second); code != 1 {
			t.Fatalf("exit %d:\n%s", code, s.out.String())
		}
		if out := s.out.String(); !strings.Contains(out, "Another WittyMediaServer, with a data folder of its own, is already using port "+strconv.Itoa(portA)) {
			t.Fatalf("output:\n%s", out)
		}
	})

	t.Run("port of another program", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"ok":false}`)) })}
		go func() { _ = srv.Serve(ln) }()
		defer srv.Close()
		s := startServer(t, filepath.Join(root, "d"), ln.Addr().(*net.TCPAddr).Port)
		if code := s.exitWithin(t, 60*time.Second); code != 1 {
			t.Fatalf("exit %d:\n%s", code, s.out.String())
		}
		if out := s.out.String(); !strings.Contains(out, "is in use by another program") {
			t.Fatalf("output:\n%s", out)
		}
	})

	t.Run("killed", func(t *testing.T) {
		if err := first.cmd.Process.Kill(); err != nil { // SIGKILL; TerminateProcess on Windows
			t.Fatal(err)
		}
		_ = first.cmd.Wait()
		again := startServer(t, dataA, freePort(t))
		ra := instance.Find(context.Background(), dataA, serverUp)
		if !ra.Answering || ra.PID != again.cmd.Process.Pid {
			t.Fatalf("after a kill: %+v\n%s", ra, again.out.String())
		}
		_ = again.cmd.Process.Kill()
	})
}

// serverUp is how long a test server gets to start: a busy machine scanning a new test
// program can take minutes over what's a second or two anywhere else.
const serverUp = 3 * time.Minute
