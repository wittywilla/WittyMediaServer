package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/couch/relay/fakehost"
)

// TestMain lets this test program stand in for the Moonlight host program, when a test starts it
// as one. It's the separate program that's replaced, and nothing of the server's.
func TestMain(m *testing.M) {
	if os.Getenv(fakehost.Env) != "" || (len(os.Args) > 1 && os.Args[1] == "--version" && os.Getenv("WMS_TEST_HOST") != "") {
		fakehost.Run()
		return
	}
	os.Exit(m.Run())
}

func fakeHostEnv(c *exec.Cmd) { c.Env = append(c.Env, fakehost.Env+"=ok") }

// fakeHostProgram is a program that runs this test program as the stand-in: WittyCouch starts it from
// its own process, where nothing here can set its surroundings.
func fakeHostProgram(t *testing.T) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in is a shell script")
	}
	p := filepath.Join(t.TempDir(), "wittycouch-host")
	script := "#!/bin/sh\n" + fakehost.Env + "=ok exec '" + self + "' \"$@\"\n"
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
