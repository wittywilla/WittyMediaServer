//go:build windows

package uninstall

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Any account can make a folder in ProgramData before the installer does. As a junction to somewhere
// else, the installer would have given the service full control of wherever it leads.
func TestSecuringRefusesAJunction(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "WittyMediaServer")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Skipf("can't make a junction here: %v %s", err, out)
	}
	if err := refuseLinks(link); err == nil {
		t.Fatal("a junction as the folder itself wasn't noticed")
	}
	if err := SecureServiceData(link); err == nil {
		t.Fatal("it set rules through a junction")
	}
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := refuseLinks(real); err != nil {
		t.Fatalf("an ordinary folder: %v", err)
	}
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(real, "inside"), target).CombinedOutput(); err != nil {
		t.Skipf("can't make a junction here: %v %s", err, out)
	}
	if err := refuseLinks(real); err == nil {
		t.Fatal("a junction inside it wasn't noticed")
	}
}
