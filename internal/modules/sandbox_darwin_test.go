package modules

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// Everything the page says about a helper on a Mac, tried from inside one: the launcher starts
// this test again in sandbox-exec. Each line it prints says what really happened. Until this has
// been seen passing on a real Mac as well as GitHub's, forks stay refused (sandboxAvailable).
func TestMacHelperSandbox(t *testing.T) {
	if os.Getenv("WMS_SANDBOX_CHILD") == "1" {
		say := func(what string, err error) {
			if err == nil {
				println(what, "ALLOWED")
			} else {
				println(what, "REFUSED", err.Error())
			}
		}
		target := os.Getenv("WMS_SANDBOX_TARGET")
		// What it has to go on being able to do.
		b, err := os.ReadFile(target)
		say("read", err)
		if string(b) != "original" {
			println("read WRONG")
		}
		// What it shouldn't.
		f, err := os.OpenFile(target, os.O_WRONLY, 0)
		if err == nil {
			_, err = f.WriteAt([]byte("CHANGED"), 0)
			f.Close()
		}
		say("write-existing", err)
		f, err = os.Create(target + ".new")
		if err == nil {
			_, err = f.Write([]byte("x"))
			f.Close()
		}
		say("write-new", err)
		say("remove", os.Remove(os.Getenv("WMS_SANDBOX_REMOVABLE")))
		c, err := net.Dial("tcp", os.Getenv("WMS_SANDBOX_ADDR"))
		if err == nil {
			c.Close()
		}
		say("connect", err)
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			l.Close()
		}
		say("listen", err)
		c, err = net.Dial("unix", os.Getenv("WMS_SANDBOX_UNIX"))
		if err == nil {
			c.Close()
		}
		say("unix", err)
		c, err = net.Dial("udp", os.Getenv("WMS_SANDBOX_ADDR"))
		if err == nil {
			_, err = c.Write([]byte("x"))
			c.Close()
		}
		say("udp", err)
		say("program", exec.Command("/bin/sh", "-c", "exit 0").Run())
		if pid, _ := strconv.Atoi(os.Getenv("WMS_SANDBOX_OTHER")); pid > 0 {
			// A real signal, not 0, which only asks whether it exists. SIGCONT changes nothing here.
			say("signal", unix.Kill(pid, unix.SIGCONT))
		}
		println("finished")
		return
	}
	if !profileWorks() {
		t.Fatal("sandbox-exec refused the profile on a program that does nothing")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	removable := filepath.Join(dir, "removable")
	for _, p := range []string{target, removable} {
		if err := os.WriteFile(p, []byte("original"), 0o666); err != nil {
			t.Fatal(err)
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	// A Unix socket's path has to be short, and TMPDIR on a Mac is long.
	sock, err := os.MkdirTemp("/tmp", "wms-sb-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sock)
	ul, err := net.Listen("unix", filepath.Join(sock, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer ul.Close()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prog, args := launchWith(self, self, 4*hardMemoryMB, 0, []string{"-test.run=TestMacHelperSandbox$", "-test.v"})
	cmd := exec.Command(prog, args...)
	cmd.Env = append(os.Environ(), "WMS_SANDBOX_CHILD=1", "WMS_SANDBOX_TARGET="+target, "WMS_SANDBOX_REMOVABLE="+removable,
		"WMS_SANDBOX_ADDR="+l.Addr().String(), "WMS_SANDBOX_UNIX="+filepath.Join(sock, "s"), "WMS_SANDBOX_OTHER="+strconv.Itoa(os.Getpid()))
	out, err := cmd.CombinedOutput()
	text := string(out)
	t.Logf("as uid %d:\n%s", os.Geteuid(), text)
	if err != nil {
		t.Fatalf("the helper failed: %v\n%s", err, text)
	}
	if !strings.Contains(text, "finished") {
		t.Fatalf("the helper never finished:\n%s", text)
	}
	if b, _ := os.ReadFile(target); string(b) != "original" {
		t.Errorf("the file changed: %q", b)
	}
	if !strings.Contains(text, "read ALLOWED") {
		t.Error("a helper has to be able to read")
	}
	// What the page says it can't do (confinement in sandbox_darwin.go).
	for _, what := range []string{"write-existing", "write-new", "remove", "connect", "listen", "unix", "udp", "program"} {
		if !strings.Contains(text, what+" REFUSED") {
			t.Errorf("%s wasn't refused", what)
		}
	}
	// The profile denies signals to other programs, and on GitHub's Mac runner that didn't hold, so
	// the page doesn't claim it. If a Mac starts refusing it, the words can say so (#139).
	if strings.Contains(text, "signal REFUSED") {
		t.Log("signalling the server is now refused, so the page's words could say so")
	}
}

// A helper allowed what it needs keeps it.
func TestMacHelperKeepsWhatItNeeds(t *testing.T) {
	if os.Getenv("WMS_SANDBOX_CHILD2") == "1" {
		c, err := net.Dial("tcp", os.Getenv("WMS_SANDBOX_ADDR"))
		if err != nil {
			println("NO-NETWORK", err.Error())
		} else {
			c.Close()
		}
		if err := exec.Command("/bin/sh", "-c", "exit 0").Run(); err != nil {
			println("NO-PROGRAM", err.Error())
		}
		if err := os.WriteFile(filepath.Join(os.Getenv("WITTYMODULE_DATA"), "x"), []byte("x"), 0o644); err != nil {
			println("NO-WRITE", err.Error())
		}
		println("finished")
		return
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prog, args := launchWith(self, self, 0, SandboxNetwork|SandboxPrograms|SandboxWriteData, []string{"-test.run=TestMacHelperKeepsWhatItNeeds$", "-test.v"})
	cmd := exec.Command(prog, args...)
	cmd.Env = append(os.Environ(), "WMS_SANDBOX_CHILD2=1", "WITTYMODULE_DATA="+t.TempDir(), "WMS_SANDBOX_ADDR="+l.Addr().String())
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "finished") || strings.Contains(string(out), "NO-") {
		t.Fatalf("%v\n%s", err, out)
	}
}

// A fork never starts in this launcher, whatever got it this far.
func TestMacLauncherRefusesAFork(t *testing.T) {
	if code := RunSandboxed([]string{"0", strconv.Itoa(SandboxStrict), "/bin/sh", "-c", "exit 0"}); code == 0 {
		t.Fatal("a fork started")
	}
}
