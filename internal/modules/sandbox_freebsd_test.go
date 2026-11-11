// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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

// Everything the page says about a helper on FreeBSD, tried from inside one: the launcher applies
// the limits and then runs this test again. Each line it prints says what really happened.
func TestFreeBSDHelperLimits(t *testing.T) {
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
		_, err = net.Dial("tcp", os.Getenv("WMS_SANDBOX_ADDR"))
		say("connect", err)
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			l.Close()
		}
		say("listen", err)
		_, err = net.Dial("unix", os.Getenv("WMS_SANDBOX_UNIX"))
		say("unix", err)
		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
		if err == nil {
			unix.Close(fd)
		}
		say("udp", err)
		say("program", exec.Command("/bin/sh", "-c", "exit 0").Run())
		// What it's told it can still do, so the words stay true.
		say("remove", os.Remove(os.Getenv("WMS_SANDBOX_REMOVABLE")))
		if pid, _ := strconv.Atoi(os.Getenv("WMS_SANDBOX_OTHER")); pid > 0 {
			say("signal", unix.Kill(pid, 0))
		}
		// And nothing it can do takes the limits off.
		var r unix.Rlimit
		_ = unix.Getrlimit(unix.RLIMIT_FSIZE, &r)
		r.Cur, r.Max = 1<<40, 1<<40
		say("raise-limit", unix.Setrlimit(unix.RLIMIT_FSIZE, &r))
		println("finished")
		return
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
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
	ul, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer ul.Close()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prog, args := launchWith(self, self, 4*hardMemoryMB, 0, []string{"-test.run=TestFreeBSDHelperLimits$", "-test.v"})
	cmd := exec.Command(prog, args...)
	cmd.Env = append(os.Environ(), "WMS_SANDBOX_CHILD=1", "WMS_SANDBOX_TARGET="+target, "WMS_SANDBOX_REMOVABLE="+removable,
		"WMS_SANDBOX_ADDR="+l.Addr().String(), "WMS_SANDBOX_UNIX="+filepath.Join(dir, "s"), "WMS_SANDBOX_OTHER="+strconv.Itoa(os.Getpid()))
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
	refused := []string{"write-existing", "write-new", "connect", "listen", "unix", "udp"}
	if os.Geteuid() != 0 {
		refused = append(refused, "program") // root isn't held to a limit on programs, and the page says so
	}
	for _, what := range refused {
		if !strings.Contains(text, what+" REFUSED") {
			t.Errorf("%s wasn't refused", what)
		}
	}
	if os.Geteuid() != 0 && !strings.Contains(text, "raise-limit REFUSED") {
		t.Error("an ordinary account raised its own limit")
	}
	// The words on the page say it can do these: if a later FreeBSD stops them, the words can improve.
	if os.Geteuid() != 0 && !strings.Contains(text, "remove ALLOWED") {
		t.Log("removing a file is now refused too, so the words on the page could say so")
	}
	if os.Geteuid() == 0 && !strings.Contains(text, "signal REFUSED") {
		t.Error("in a jail, the server's process should be out of sight")
	}
}

// A helper allowed what it needs keeps it.
func TestFreeBSDHelperKeepsWhatItNeeds(t *testing.T) {
	if os.Getenv("WMS_SANDBOX_CHILD2") == "1" {
		fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
		if err != nil {
			println("NO-SOCKET", err.Error())
		} else {
			unix.Close(fd)
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
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	prog, args := launchWith(self, self, 0, SandboxNetwork|SandboxPrograms|SandboxWriteData, []string{"-test.run=TestFreeBSDHelperKeepsWhatItNeeds$", "-test.v"})
	cmd := exec.Command(prog, args...)
	cmd.Env = append(os.Environ(), "WMS_SANDBOX_CHILD2=1", "WITTYMODULE_DATA="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "finished") || strings.Contains(string(out), "NO-") {
		t.Fatalf("%v\n%s", err, out)
	}
}

// A fork never starts in this launcher, whatever got it this far.
func TestFreeBSDLauncherRefusesAFork(t *testing.T) {
	if code := RunSandboxed([]string{"0", strconv.Itoa(SandboxStrict), "/bin/sh", "-c", "exit 0"}); code == 0 {
		t.Fatal("a fork started")
	}
}
