package bdj

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// The calls real discs make, as they arrive after the sandbox has rewritten the
// class: each check reports through a register (see java/testxlet/p).
func TestPlatformCalls(t *testing.T) {
	j := javaForTest(t)
	d := discWith(t, "testdata/platform.jar", bdjo("p.PlatformXlet"))
	obj, err := d.BDJObject("00000")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	gpr := map[string]string{}
	checks := map[int]string{}
	var failed []string
	h, err := Start(context.Background(), d, Options{Java: j, CacheDir: t.TempDir(), WorkDir: t.TempDir(), PersistDir: t.TempDir(),
		Log: func(s string) {
			var n int
			if _, err := fmt.Sscanf(s, "check %d ", &n); err == nil {
				mu.Lock()
				checks[n] = s
				mu.Unlock()
			}
		},
		OnEvent: func(e Event) {
			mu.Lock()
			defer mu.Unlock()
			switch e.Kind {
			case "gpr":
				gpr[e.Args[0]] = e.Args[1]
			case "appfailed":
				failed = append(failed, e.Args...)
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	h.StartTitle(0xffff, obj)
	got := func(k string) string {
		mu.Lock()
		defer mu.Unlock()
		if len(failed) > 0 {
			t.Fatalf("the application failed: %v", failed)
		}
		return gpr[k]
	}
	for end := time.Now().Add(60 * time.Second); got("299") != "1"; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("the checks did not finish")
		}
	}
	time.Sleep(300 * time.Millisecond) // the log lines arrive on their own pipe
	mu.Lock()
	for n := 300; ; n++ {
		v, ok := gpr[fmt.Sprint(n)]
		if !ok {
			if n < 320 {
				t.Errorf("only %d checks ran", n-300)
			}
			break
		}
		if v != "1" {
			t.Errorf("failed: %s", checks[n])
		}
	}
	mu.Unlock()
	// What it drew straight to the screen reaches the picture without a repaint.
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if r, g, b, a := h.Pixel(120, 120); r == 255 && g == 0 && b == 255 && a == 255 {
			break
		}
		if time.Now().After(end) {
			t.Fatal("direct drawing did not reach the picture")
		}
	}
	// Ending the title ends the application, and takes its picture away.
	h.Send("stopapps")
	for end := time.Now().Add(10 * time.Second); got("298") != "1"; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("destroyXlet was not called")
		}
	}
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, _, _, a := h.Pixel(120, 120); a == 0 {
			break
		}
		if time.Now().After(end) {
			t.Fatal("the ended application's picture stayed on screen")
		}
	}
}

// The diagnostic names what a disc reaches for and the sandbox will not give it.
func TestRefusedClasses(t *testing.T) {
	j := javaForTest(t)
	jar, err := os.ReadFile("testdata/attack.jar")
	if err != nil {
		t.Skip("test application not built")
	}
	names, err := JDKClasses(jar)
	if err != nil {
		t.Fatal(err)
	}
	refused, err := j.Refused(context.Background(), t.TempDir(), names)
	if err != nil {
		t.Fatal(err)
	}
	has := func(c string) bool { return slices.Contains(refused, c) }
	if !slices.Contains(names, "java/lang/ProcessBuilder") || !has("java/lang/ProcessBuilder") || !has("java/rmi/Naming") {
		t.Errorf("should be refused: %v", refused)
	}
	if has("java/lang/String") || has("java/io/File") || has("javax/xml/parsers/SAXParser") {
		t.Errorf("should be allowed or replaced: %v", refused)
	}
}
