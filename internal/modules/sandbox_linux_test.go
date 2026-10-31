package modules

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
		if err := os.Truncate(target, 0); err == nil {
			println("WROTE truncate")
		}
		if err := os.Chmod(target, 0o600); err == nil {
			println("WROTE chmod")
		}
		if err := os.Rename(target, target+".moved"); err == nil {
			println("WROTE rename")
		}
		if c, err := net.Dial("unix", os.Getenv("WMS_SANDBOX_SOCKET")); err == nil {
			c.Close()
			println("REACHED a socket file")
		}
		if c, err := net.Dial("tcp", os.Getenv("WMS_SANDBOX_TCP")); err == nil {
			c.Close()
			println("REACHED tcp")
		}
		if _, err := os.Create(target + ".new"); err == nil {
			println("CREATED")
		}
		if b, err := os.ReadFile(target); err != nil || len(b) == 0 {
			println("CANNOT-READ")
		}
		return
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
		sock := filepath.Join(t.TempDir(), "s")
		ul, err := net.Listen("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		defer ul.Close()
		tl, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer tl.Close()
		cmd := exec.Command(exe, "-test.run=TestLauncherStopsWritesAndHoldsMemory")
		cmd.Env = append(os.Environ(), "WMS_SANDBOX_LAUNCH=1", "WMS_SANDBOX_TARGET="+target, "WMS_SANDBOX_SOCKET="+sock, "WMS_SANDBOX_TCP="+tl.Addr().String())
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
		if strings.Contains(s, "REACHED") {
			t.Fatalf("it reached out: %s", s)
		}
		if strings.Contains(s, "CANNOT-READ") {
			t.Fatalf("reading must still work: %s", s)
		}
		if !strings.Contains(s, "REFUSED") {
			t.Fatalf("never got as far as trying: %s", s)
		}
		return
	}
	if landlockABI() < landlockForkABI || seccompArch == 0 {
		println("SKIP-LANDLOCK")
		return
	}
	os.Setenv("WMS_SANDBOX_CHILD", "1")
	os.Exit(RunSandboxed([]string{"2048", strconv.Itoa(SandboxStrict), exe, "-test.run=TestLauncherStopsWritesAndHoldsMemory"}))
}
