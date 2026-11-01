package server

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// An administrator's web account can't run programs on the machine. The Java setting used to be
// a way to: name any program, and the status page ran it.
func TestWebSettingsCantNameAProgram(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	evil := filepath.Join(dir, "evil.sh")
	script := "#!/bin/sh\ntouch " + marker + "\necho secret-from-a-program >&2\nexit 3\n"
	if err := os.WriteFile(evil, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if w := ts.do(t, "PUT", "/api/settings", tok, map[string]string{"javaPath": evil}); w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := ts.DB.GetSetting(ctx, "javaPath", ""); got != "" {
		t.Errorf("a web request set the program: %q", got)
	}
	// One saved by an older version, off and on.
	if err := ts.DB.SetSetting(ctx, "javaPath", evil); err != nil {
		t.Fatal(err)
	}
	for _, on := range []string{"0", "1"} {
		if err := ts.DB.SetSetting(ctx, "bdjEnabled", on); err != nil {
			t.Fatal(err)
		}
		w := ts.do(t, "GET", "/api/bdj/status", tok, nil)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("bdjEnabled=%s: the program ran", on)
		}
		if strings.Contains(w.Body.String(), "evil.sh") || strings.Contains(w.Body.String(), "secret-from-a-program") {
			t.Errorf("bdjEnabled=%s: %s", on, w.Body)
		}
	}
	if strings.Contains(ts.do(t, "GET", "/api/settings", tok, nil).Body.String(), "evil.sh") {
		t.Error("the old value is still shown as a setting")
	}
}

// What the owner named in the config file is used while BD-J is on, and nothing is started while
// it's off. What a program prints never reaches the page.
func TestJavaFromTheConfigFile(t *testing.T) {
	modtest.Need(t, "wittybluray")
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	ts := newTestServer(t)
	_, tok := ts.user(t, "owner", "adult", true, "")
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	java := filepath.Join(dir, "java")
	script := "#!/bin/sh\ntouch " + marker + "\necho secret-from-a-program >&2\nexit 3\n"
	if err := os.WriteFile(java, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ts.Cfg.JavaPath = java
	if err := ts.DB.SetSetting(context.Background(), "bdjEnabled", "0"); err != nil {
		t.Fatal(err)
	}
	w := ts.do(t, "GET", "/api/bdj/status", tok, nil)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("it was started with BD-J off")
	}
	if !strings.Contains(w.Body.String(), `"path"`) {
		t.Errorf("what it would use isn't shown: %s", w.Body)
	}
	if err := ts.DB.SetSetting(context.Background(), "bdjEnabled", "1"); err != nil {
		t.Fatal(err)
	}
	w = ts.do(t, "GET", "/api/bdj/status", tok, nil)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("not tried with BD-J on: %s", w.Body)
	}
	if strings.Contains(w.Body.String(), "secret-from-a-program") || !strings.Contains(w.Body.String(), `"error"`) {
		t.Errorf("%s", w.Body)
	}
}
