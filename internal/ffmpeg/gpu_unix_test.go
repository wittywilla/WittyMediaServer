//go:build linux || freebsd

package ffmpeg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Plain files stand in for device nodes: opening one is refused the same way.
func TestFindGPUsIn(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root opens anything")
	}
	dev, sys := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(dev, "dri"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, mode := range map[string]os.FileMode{"renderD128": 0o000, "renderD129": 0o660} {
		if err := os.WriteFile(filepath.Join(dev, "dri", name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(dev, "dri", name), mode); err != nil {
			t.Fatal(err)
		}
	}
	card := filepath.Join(sys, "class", "drm", "renderD129", "device")
	if err := os.MkdirAll(card, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(card, "vendor"), []byte("0x8086\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../../bus/pci/drivers/i915", filepath.Join(card, "driver")); err != nil {
		t.Fatal(err)
	}

	got := findGPUsIn(dev, sys)
	if len(got) != 2 {
		t.Fatalf("found %+v", got)
	}
	locked, open := got[0], got[1]
	if locked.Path != "/dev/dri/renderD128" || locked.Usable || locked.Problem == "" || locked.Mode != "0000" || locked.Group != os.Getgid() {
		t.Errorf("a node it can't open: %+v", locked)
	}
	if !strings.Contains(locked.Problem, "only its owner") {
		t.Errorf("mode 0000 says who may open it: %q", locked.Problem)
	}
	if open.Path != "/dev/dri/renderD129" || !open.Usable || open.Vendor != "intel" || open.Driver != "i915" {
		t.Errorf("an Intel node it can open: %+v", open)
	}
	if d, ok := pickRender(got, "intel"); !ok || d.Path != "/dev/dri/renderD129" {
		t.Errorf("picked %+v", d)
	}
}
