package bdj

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wittywilla/wittybluray"
)

// The registers the hostile disc reports through (see java/testxlet/a).
const (
	attackDone  = "199"
	attackFirst = 200
)

// A disc's code is untrusted: every way out it tries must fail, and that is
// checked from outside the sandbox as well as from inside it.
func TestSandboxHolds(t *testing.T) {
	j := javaForTest(t)
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.xml")
	if err := os.WriteFile(secret, []byte("<s>TOPSECRET</s>"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(outside, "written.bin")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	var connections atomic.Int32
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			connections.Add(1)
			c.Close()
		}
	}()
	fl, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	free := fl.Addr().(*net.TCPAddr).Port
	fl.Close()
	uri := (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(secret), "/")}).String()
	d := discWith(t, "testdata/attack.jar", bdjo("a.AttackXlet", "probes"))
	obj, err := d.BDJObject("00000")
	if err != nil {
		t.Fatal(err)
	}
	persist := t.TempDir()
	args := []string{secret, target, fmt.Sprint(l.Addr().(*net.TCPAddr).Port), fmt.Sprint(free), outside, uri}
	if err := os.WriteFile(filepath.Join(persist, "args.txt"), []byte(strings.Join(args, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(secret, filepath.Join(persist, "link.xml")) // needs a privilege on Windows
	var mu sync.Mutex
	gpr := map[string]string{}
	probes := map[int]string{}
	h, err := Start(context.Background(), d, Options{Java: j, CacheDir: t.TempDir(), WorkDir: t.TempDir(), PersistDir: persist,
		Log: func(s string) {
			var n int
			if _, err := fmt.Sscanf(s, "probe %d ", &n); err == nil {
				mu.Lock()
				probes[n] = s
				mu.Unlock()
			}
		},
		OnEvent: func(e Event) {
			if e.Kind == "gpr" {
				mu.Lock()
				gpr[e.Args[0]] = e.Args[1]
				mu.Unlock()
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	h.StartTitle(0xffff, obj)
	end := time.Now().Add(60 * time.Second)
	for {
		mu.Lock()
		done := gpr[attackDone] == "1"
		mu.Unlock()
		if done {
			break
		}
		if time.Now().After(end) {
			t.Fatal("the probes did not finish")
		}
		select {
		case <-h.Done():
			t.Fatal("a probe ended the process")
		case <-time.After(20 * time.Millisecond):
		}
	}
	time.Sleep(300 * time.Millisecond) // the log lines arrive on their own pipe
	mu.Lock()
	defer mu.Unlock()
	if len(gpr) < 40 {
		t.Fatalf("only %d probes reported", len(gpr))
	}
	for n := attackFirst; ; n++ {
		v, ok := gpr[fmt.Sprint(n)]
		if !ok {
			break
		}
		if v != "1" {
			t.Errorf("got out: %s", probes[n])
		}
	}
	if n := connections.Load(); n != 0 {
		t.Errorf("the disc reached a local port %d times", n)
	}
	for _, p := range []string{target, target + ".2", target + ".3", target + ".4"} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("the disc wrote %s", p)
		}
	}
	if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", free), time.Second); err == nil {
		c.Close()
		t.Errorf("the disc opened a listening port")
	}
}

var _ = wittybluray.NoButton
