package diagnose

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

// A junction takes no special rights to make, and following links doesn't see
// through one, so it's checked by itself.
func TestDiagnoseJunctionsThatLeadOut(t *testing.T) {
	top := realDir(t)
	lib, outside := filepath.Join(top, "lib"), filepath.Join(top, "outside")
	inner := filepath.Join(lib, "inner")
	for _, d := range []string{inner, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.iso"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "fine.iso"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	junction := func(link, target string) {
		t.Helper()
		cmd := exec.Command("cmd", "/c", "mklink", "/J", link, target)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("can't make a junction here: %v %s", err, out)
		}
	}
	r := NewRoots([]string{lib})
	out := filepath.Join(lib, "out")
	junction(out, outside)
	for _, p := range []string{out, filepath.Join(out, "secret.iso")} {
		if got, err := r.Resolve(p); !errors.Is(err, ErrOutside) {
			t.Errorf("Resolve(%q) = %q, %v", p, got, err)
		}
	}
	if err := r.CheckInnerLinks(lib); !errors.Is(err, ErrLinkOut) {
		t.Errorf("a junction out inside a disc folder: %v", err)
	}
	// One that stays inside is fine.
	lib2 := filepath.Join(top, "lib2")
	inner2 := filepath.Join(lib2, "inner")
	if err := os.MkdirAll(inner2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner2, "fine.iso"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	junction(filepath.Join(lib2, "alias"), inner2)
	r2 := NewRoots([]string{lib2})
	if _, err := r2.Resolve(filepath.Join(lib2, "alias", "fine.iso")); err != nil {
		t.Errorf("a junction inside: %v", err)
	}
	if err := r2.CheckInnerLinks(lib2); err != nil {
		t.Errorf("a junction inside a disc folder: %v", err)
	}
}
