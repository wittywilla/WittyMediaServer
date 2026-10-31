package wittyplay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRefusal(t *testing.T) {
	const size = 0x400
	cases := []struct {
		name, console string
		data          []byte
		want          string
	}{
		{"game.3ds", "3ds", fakeCCI(fakeNCCH(false, size)), "encrypted"},
		{"game.3ds", "3ds", fakeCCI(fakeNCCH(true, size)), ""},
		{"game.cci", "3ds", fakeCCI(fakeNCCH(false, size)), "encrypted"},
		{"game.cxi", "3ds", fakeNCCH(false, size), "encrypted"},
		{"game.cxi", "3ds", fakeNCCH(true, size), ""},
		{"game.cia", "3ds", fakeCIA(fakeNCCH(true, size), 1), "encrypted"},
		{"game.cia", "3ds", fakeCIA(scrambled(size), 0), "encrypted"}, // no readable program inside
		{"game.cia", "3ds", fakeCIA(fakeNCCH(true, size), 0), ""},     // the server copies the program out
		{"game.3ds", "3ds", []byte("too short to say"), ""},
		{"game.3dsx", "3ds", []byte("homebrew"), ""},
		{"game.3ds", "nds", fakeCCI(fakeNCCH(false, size)), ""},
	}
	for _, c := range cases {
		got := Refusal(writeTemp(t, c.name, c.data), c.console)
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("%s on %s: %q, want %q", c.name, c.console, got, c.want)
		}
	}
	if Refusal(filepath.Join(t.TempDir(), "missing.3ds"), "3ds") != "" {
		t.Error("a missing file is a different problem")
	}
}
