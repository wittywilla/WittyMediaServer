package bdj

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wittywilla/wittybluray"
)

// hostile starts the hostile disc in one of its modes.
func hostile(t *testing.T, mode string, l Limits) (*Host, *wittybluray.BDJObject, string, func() map[string]string, func() string) {
	t.Helper()
	j := javaForTest(t)
	d := discWith(t, "testdata/attack.jar", bdjo("a.AttackXlet", mode))
	obj, err := d.BDJObject("00000")
	if err != nil {
		t.Fatal(err)
	}
	persist := t.TempDir()
	var mu sync.Mutex
	gpr := map[string]string{}
	limit := ""
	h, err := Start(context.Background(), d, Options{Java: j, CacheDir: t.TempDir(), WorkDir: t.TempDir(), PersistDir: persist, Limits: l,
		OnEvent: func(e Event) {
			mu.Lock()
			defer mu.Unlock()
			switch e.Kind {
			case "gpr":
				gpr[e.Args[0]] = e.Args[1]
			case "limit":
				limit = strings.Join(e.Args, " ")
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Close)
	h.StartTitle(0xffff, obj)
	return h, obj, persist, func() map[string]string {
			mu.Lock()
			defer mu.Unlock()
			out := make(map[string]string, len(gpr))
			for k, v := range gpr {
				out[k] = v
			}
			return out
		}, func() string {
			mu.Lock()
			defer mu.Unlock()
			return limit
		}
}

// noLimits leaves a process to Java's own limits.
var noLimits = Limits{Every: time.Hour}

func waitEnd(t *testing.T, h *Host, within time.Duration) {
	t.Helper()
	select {
	case <-h.Done():
	case <-time.After(within):
		t.Fatalf("the process was still running after %v", within)
	}
}

// Whatever a disc's code does to itself, the process ends when it passes a limit
// and the server hears of it.
func TestHostileLimits(t *testing.T) {
	for _, c := range []struct {
		mode   string
		limits Limits
		want   string // in the reason given; "" = Java ends it by itself
	}{
		{"heap", noLimits, ""},
		{"native", Limits{Memory: 500 << 20, Every: 100 * time.Millisecond}, "memory"},
		{"threads", Limits{Threads: 300, Every: 50 * time.Millisecond}, "threads"},
		{"spin", Limits{Cores: 1, Window: 3 * time.Second, Every: 500 * time.Millisecond}, "cores"},
	} {
		t.Run(c.mode, func(t *testing.T) {
			h, _, _, _, limit := hostile(t, c.mode, c.limits)
			waitEnd(t, h, 90*time.Second)
			if got := limit(); c.want != "" && !strings.Contains(got, c.want) {
				t.Errorf("reason %q, want %q", got, c.want)
			}
		})
	}
}

// What only wastes a disc's own time leaves the process running and answering.
func TestHostileFloods(t *testing.T) {
	for _, mode := range []string{"flood-play", "flood-title", "flood-gpr", "flood-log", "flood-sound", "direct", "stack", "hang-paint", "kill-threads", "disk"} {
		t.Run(mode, func(t *testing.T) {
			h, _, persist, gpr, _ := hostile(t, mode, noLimits)
			end := time.Now().Add(20 * time.Second)
			for gpr()[attackDone] == "" { // what flood-gpr goes on to do to this register does not matter

				if time.Now().After(end) {
					t.Fatal("the application never started")
				}
				time.Sleep(10 * time.Millisecond)
			}
			// However much it sends, this side stays responsive: reading the link does not
			// take over the machine and every call here returns at once.
			time.Sleep(2 * time.Second)
			for i := 0; i < 20; i++ {
				start := time.Now()
				h.Send("key 10")
				h.Version()
				h.Pixel(1, 1)
				h.PNG()
				if d := time.Since(start); d > 2*time.Second {
					t.Errorf("a call took %v", d)
				}
			}
			select {
			case <-h.Done():
				t.Error("the process should still be running")
			default:
			}
			if mode == "disk" {
				for end := time.Now().Add(30 * time.Second); gpr()["198"] != "1"; time.Sleep(50 * time.Millisecond) {
					if time.Now().After(end) {
						t.Fatal("the disc's storage never filled up")
					}
				}
				var total int64
				_ = filepath.Walk(persist, func(_ string, fi os.FileInfo, err error) error {
					if err == nil && !fi.IsDir() {
						total += fi.Size()
					}
					return nil
				})
				if total > 65<<20 {
					t.Errorf("the disc stored %d bytes", total)
				}
			}
			// Ending the session ends it, whatever state it left itself in.
			start := time.Now()
			h.Close()
			waitEnd(t, h, 10*time.Second)
			if d := time.Since(start); d > 5*time.Second {
				t.Errorf("closing took %v", d)
			}
		})
	}
}

// An application that never returns from starting holds nothing up.
func TestHostileInit(t *testing.T) {
	h, _, _, _, _ := hostile(t, "hang-init", noLimits)
	time.Sleep(time.Second)
	h.Send("stopapps")
	h.Close()
	waitEnd(t, h, 10*time.Second)
}
