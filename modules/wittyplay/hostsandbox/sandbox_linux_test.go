package hostsandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// The filter can only be tested on a process that can be thrown away, so this
// test starts itself again and applies the sandbox there.
func TestSandboxRefusesNetworkAndPrograms(t *testing.T) {
	if os.Getenv("WITTYPLAY_SANDBOX_CHILD") == "1" {
		save := os.Getenv("WITTYPLAY_SAVE")
		if err := Apply(Limits{SaveDir: save, MemoryMB: 4096}); err != nil {
			t.Logf("some layers are unavailable: %v", err)
		}
		if _, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0); !errors.Is(err, unix.EPERM) {
			t.Fatalf("socket: got %v, want EPERM", err)
		}
		if err := exec.Command("/bin/sh", "-c", "true").Run(); err == nil {
			t.Fatal("starting a program should fail")
		}
		if err := os.WriteFile(filepath.Join(save, "ok"), []byte("x"), 0o644); err != nil {
			t.Fatalf("the save folder must stay writable: %v", err)
		}
		if landlockAvailable() {
			if err := os.WriteFile(filepath.Join(os.TempDir(), "wittyplay-sandbox-escape"), []byte("x"), 0o644); err == nil {
				t.Fatal("writing outside the save folder should fail")
			}
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestSandboxRefusesNetworkAndPrograms", "-test.v")
	cmd.Env = append(os.Environ(), "WITTYPLAY_SANDBOX_CHILD=1", "WITTYPLAY_SAVE="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Logf("%s", out)
}

func landlockAvailable() bool {
	// Version query: flag 1 returns the ABI version instead of a ruleset.
	v, _, e := unix.Syscall(sysLandlockCreateRuleset, 0, 0, 1)
	return e == 0 && v > 0
}

// Real cores hold little and reserve a lot. Reservations must fit, and memory
// actually used must hit the limit.
func TestMemoryLimitCountsUseNotReservations(t *testing.T) {
	if os.Getenv("WITTYPLAY_MEMORY_CHILD") == "1" {
		const limitMB = 256
		if err := Apply(Limits{SaveDir: t.TempDir(), MemoryMB: limitMB}); err != nil {
			t.Logf("some layers are unavailable: %v", err)
		}
		reserve, err := unix.Mmap(-1, 0, 2<<30, unix.PROT_NONE, unix.MAP_ANON|unix.MAP_PRIVATE)
		if err != nil {
			t.Fatalf("reserving 2 GB of nothing under a %d MB limit: %v", limitMB, err)
		}
		_ = unix.Munmap(reserve)
		if b, err := unix.Mmap(-1, 0, (limitMB+64)<<20, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANON|unix.MAP_PRIVATE); err == nil {
			_ = unix.Munmap(b)
			t.Fatal("writable memory over the limit should be refused")
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestMemoryLimitCountsUseNotReservations", "-test.v")
	cmd.Env = append(os.Environ(), "WITTYPLAY_MEMORY_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}
