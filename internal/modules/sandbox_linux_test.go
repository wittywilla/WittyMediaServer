package modules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The launcher's limits hold after it becomes the program, so they can't be skipped by
// what runs. This test is the program too: the launcher applies them and runs it again.
func TestLauncherStopsWritesAndHoldsMemory(t *testing.T) {
	if os.Getenv("WMS_SANDBOX_CHILD") == "1" {
		target := os.Getenv("WMS_SANDBOX_TARGET")
		if err := os.WriteFile(target, []byte("changed"), 0o644); err == nil {
			println("WROTE")
		} else {
			println("REFUSED", err.Error())
		}
		if _, err := os.Create(target + ".new"); err == nil {
			println("CREATED")
		}
		if b, err := os.ReadFile(target); err != nil || len(b) == 0 {
			println("CANNOT-READ")
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == SandboxFlag {
		os.Exit(RunSandboxed(os.Args[2:]))
	}
	target := filepath.Join(t.TempDir(), "database")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The launcher itself runs in a child of this test binary, the way the server's
	// main does for SandboxFlag.
	if os.Getenv("WMS_SANDBOX_LAUNCH") == "" {
		cmd := exec.Command(exe, "-test.run=TestLauncherStopsWritesAndHoldsMemory")
		cmd.Env = append(os.Environ(), "WMS_SANDBOX_LAUNCH=1", "WMS_SANDBOX_TARGET="+target)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		s := string(out)
		if strings.Contains(s, "SKIP-LANDLOCK") {
			t.Skip("this kernel has no Landlock")
		}
		if strings.Contains(s, "WROTE") || strings.Contains(s, "CREATED") {
			t.Fatalf("it changed a file: %s", s)
		}
		if strings.Contains(s, "CANNOT-READ") {
			t.Fatalf("reading must still work: %s", s)
		}
		if !strings.Contains(s, "REFUSED") {
			t.Fatalf("never got as far as trying: %s", s)
		}
		return
	}
	v, _, e := rawLandlockVersion()
	if e != 0 || v == 0 {
		println("SKIP-LANDLOCK")
		return
	}
	os.Setenv("WMS_SANDBOX_CHILD", "1")
	os.Exit(RunSandboxed([]string{"2048", "0", exe, "-test.run=TestLauncherStopsWritesAndHoldsMemory"}))
}
