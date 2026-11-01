//go:build linux && (amd64 || arm64)

package hostsandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

// What a taken-over core might try around the plain calls the filter names: another
// numbering of the same calls, a ring that makes them without a call, and the things
// to do to a file that aren't writing to it.
func TestSandboxHasNoWayRound(t *testing.T) {
	if os.Getenv("WITTYPLAY_GAPS_CHILD") == "1" {
		save, outside := os.Getenv("WITTYPLAY_SAVE"), os.Getenv("WITTYPLAY_OUTSIDE")
		if err := Confine(Limits{SaveDir: save}); err != nil {
			t.Fatal(err)
		}
		if err := Apply(Limits{SaveDir: save}); err != nil {
			t.Logf("some layers are unavailable: %v", err)
		}
		if runtime.GOARCH == "amd64" {
			if _, _, e := unix.Syscall(unix.SYS_SOCKET|x32Bit, unix.AF_INET, unix.SOCK_STREAM, 0); e != unix.EPERM {
				t.Errorf("a socket by x32's number: got %v, want EPERM", e)
			}
			if _, _, e := unix.Syscall(unix.SYS_OPENAT|x32Bit, 0, 0, 0); e != unix.EPERM {
				t.Errorf("any x32 call: got %v, want EPERM", e)
			}
		}
		var params [120]byte
		if _, _, e := unix.Syscall(unix.SYS_IO_URING_SETUP, 8, uintptr(unsafe.Pointer(&params[0])), 0); e != unix.EPERM {
			t.Errorf("io_uring: got %v, want EPERM", e)
		}

		// Outside: the file stays as it was, and a core asking gets told no or nothing happens.
		file := filepath.Join(outside, "database")
		if landlockAvailable() {
			if err := os.Truncate(file, 0); !errors.Is(err, unix.EACCES) && !errors.Is(err, unix.EPERM) {
				t.Errorf("truncating outside: got %v", err)
			}
		}
		if err := os.Chmod(file, 0o666); err != nil {
			t.Errorf("chmod is meant to seem to work: %v", err)
		}
		if err := unix.Fchmodat(unix.AT_FDCWD, file, 0o666, 0); err != nil {
			t.Errorf("fchmodat is meant to seem to work: %v", err)
		}
		if f, err := os.Open(file); err == nil {
			if err := f.Chmod(0o666); err != nil {
				t.Errorf("fchmod is meant to seem to work: %v", err)
			}
			f.Close()
		}
		if err := unix.Setxattr(file, "user.wittyplay", []byte("x"), 0); err != nil {
			t.Errorf("setxattr is meant to seem to work: %v", err)
		}
		if _, err := unix.Getxattr(file, "user.wittyplay", nil); err == nil {
			t.Error("setxattr did something")
		}
		st, err := os.Stat(file)
		if err != nil {
			t.Fatal(err)
		}
		if st.Size() != 8 || st.Mode().Perm() != 0o600 {
			t.Errorf("the file outside changed: %d bytes, mode %v", st.Size(), st.Mode())
		}

		// Inside: everything a core does with its own saves still works, setting modes too.
		mine := filepath.Join(save, "game.srm")
		if err := os.WriteFile(mine, []byte("12345678"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(mine, 0o600); err != nil {
			t.Errorf("chmod in the save folder: %v", err)
		}
		f, err := os.OpenFile(mine, os.O_WRONLY|os.O_TRUNC, 0)
		if err != nil {
			t.Fatalf("opening a save to replace it: %v", err)
		}
		if _, err := f.WriteString("12345678"); err != nil {
			t.Error(err)
		}
		if err := f.Truncate(4); err != nil {
			t.Errorf("shortening a save through its file: %v", err)
		}
		f.Close()
		if truncateLimited {
			if err := os.Truncate(mine, 2); err != nil {
				t.Errorf("shortening a save by name: %v", err)
			}
		}
		if err := os.Rename(mine, mine+".bak"); err != nil {
			t.Errorf("renaming a save: %v", err)
		}
		if err := os.Mkdir(filepath.Join(save, "more"), 0o755); err != nil {
			t.Errorf("a folder of its own: %v", err)
		}
		return
	}
	outside := t.TempDir()
	file := filepath.Join(outside, "database")
	if err := os.WriteFile(file, []byte("precious"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestSandboxHasNoWayRound", "-test.v")
	cmd.Env = append(os.Environ(), "WITTYPLAY_GAPS_CHILD=1", "WITTYPLAY_SAVE="+t.TempDir(), "WITTYPLAY_OUTSIDE="+outside)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if b, _ := os.ReadFile(file); string(b) != "precious" {
		t.Fatalf("the file outside holds %q\n%s", b, out)
	}
}
