package hostsandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

// The filter can only be tested on a process that can be thrown away, so this
// test starts itself again and applies the sandbox there.
func TestSandboxRefusesNetworkAndPrograms(t *testing.T) {
	if os.Getenv("WITTYPLAY_SANDBOX_CHILD") == "1" {
		save := os.Getenv("WITTYPLAY_SAVE")
		if err := Confine(Limits{SaveDir: save}); err != nil {
			t.Fatal(err)
		}
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
		if err := Apply(Limits{MemoryMB: limitMB}); err != nil {
			t.Logf("some layers are unavailable: %v", err)
		}
		reserve, err := unix.Mmap(-1, 0, 1<<30, unix.PROT_NONE, unix.MAP_ANON|unix.MAP_PRIVATE)
		if err != nil {
			t.Fatalf("reserving 1 GB of nothing under a %d MB limit: %v", limitMB, err)
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

// The filter must hold on threads that were already running when it went on, as a
// core might end up on any of them.
func TestFilterCoversThreadsAlreadyRunning(t *testing.T) {
	if os.Getenv("WITTYPLAY_THREADS_CHILD") == "1" {
		probe := make(chan func() error)
		go func() {
			runtime.LockOSThread() // this thread is running before the sandbox goes on
			for f := range probe {
				res <- f()
			}
		}()
		check := func() error {
			fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
			if err == nil {
				_ = unix.Close(fd)
			}
			return err
		}
		probe <- check
		if err := <-res; err != nil {
			t.Skipf("sockets don't work here even before the sandbox: %v", err)
		}
		if err := Apply(Limits{}); err != nil {
			t.Logf("some layers are unavailable: %v", err)
		}
		probe <- check
		if err := <-res; !errors.Is(err, unix.EPERM) {
			t.Fatalf("on a thread started earlier, socket: got %v, want EPERM", err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestFilterCoversThreadsAlreadyRunning", "-test.v")
	cmd.Env = append(os.Environ(), "WITTYPLAY_THREADS_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

var res = make(chan error)

// A goroutine can land on any thread, so a socket and a write must be refused from
// threads other than the one that set the limits: one started before they went on,
// and one started after.
func TestLimitsHoldOnOtherThreads(t *testing.T) {
	if os.Getenv("WITTYPLAY_OTHER_THREAD_CHILD") == "1" {
		limits := Limits{SaveDir: os.Getenv("WITTYPLAY_SAVE")}
		if err := Confine(limits); err != nil {
			t.Fatal(err)
		}
		if !landlockAvailable() {
			t.Log("this kernel has no Landlock, so only the socket is checked")
		}
		runtime.LockOSThread()
		self := unix.Gettid()
		write := func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "probe"), []byte("x"), 0o644)
		}
		probe := func(name string, on func(func() error) error) {
			if err := on(func() error {
				_, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
				return err
			}); !errors.Is(err, unix.EPERM) {
				t.Errorf("%s: socket: got %v, want EPERM", name, err)
			}
			if err := on(func() error { return write(limits.SaveDir) }); err != nil {
				t.Errorf("%s: the save folder must stay writable: %v", name, err)
			}
			if landlockAvailable() {
				if err := on(func() error { return write(os.Getenv("WITTYPLAY_OUTSIDE")) }); !errors.Is(err, unix.EACCES) {
					t.Errorf("%s: writing outside the save folder: got %v, want EACCES", name, err)
				}
			}
		}
		before := otherThread(t, self)
		// Writing is limited from the restart on, with Apply still to come.
		if landlockAvailable() {
			if err := before(func() error { return write(os.Getenv("WITTYPLAY_OUTSIDE")) }); !errors.Is(err, unix.EACCES) {
				t.Errorf("before Apply: writing outside the save folder: got %v, want EACCES", err)
			}
		}
		if err := Apply(limits); err != nil {
			t.Logf("some layers are unavailable: %v", err)
		}
		probe("a thread started before", before)
		probe("a thread started after", otherThread(t, self))
		return
	}
	outside := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestLimitsHoldOnOtherThreads", "-test.v")
	cmd.Env = append(os.Environ(), "WITTYPLAY_OTHER_THREAD_CHILD=1", "WITTYPLAY_SAVE="+t.TempDir(), "WITTYPLAY_OUTSIDE="+outside)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(outside, "probe")); err == nil {
		t.Fatalf("a write got out of the save folder\n%s", out)
	}
	t.Logf("%s", out)
}

// otherThread starts a thread that isn't self and runs what it's handed there.
func otherThread(t *testing.T, self int) func(func() error) error {
	t.Helper()
	calls := make(chan func() error)
	results := make(chan error)
	go func() {
		runtime.LockOSThread()
		for f := range calls {
			results <- f()
		}
	}()
	on := func(f func() error) error {
		calls <- f
		return <-results
	}
	if err := on(func() error {
		if tid := unix.Gettid(); tid == self {
			return errors.New("the same thread")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return on
}
