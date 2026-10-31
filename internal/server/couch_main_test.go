package server

import (
	"os"
	"os/exec"
	"testing"

	"github.com/wittywilla/wittycouch/relay/fakehost"
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
