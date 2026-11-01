package bdremote

import (
	"os/exec"
	"strings"
	"testing"
)

// moduleDir is the folder of the source of a module this server requires.
func moduleDir(t testing.TB, id string) string {
	t.Helper()
	dir, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/wittywilla/"+id).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(dir))
}
