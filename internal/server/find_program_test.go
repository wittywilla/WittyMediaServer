package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/config"
)

// A release finds its programs beside it. The folder it was started from is another
// matter: that's anyone's, and only a development build looks there.
func TestFindProgramLooksInBuildOnlyForDevelopment(t *testing.T) {
	name := "wms-find-program-test"
	file := name
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".build", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".build", file), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	release := &Server{Cfg: &config.Config{}}
	if got := release.findProgram(name); got != "" {
		t.Errorf("a release ran %s from the folder it was started in", got)
	}
	dev := &Server{Cfg: &config.Config{Dev: true}}
	if got := dev.findProgram(name); got == "" {
		t.Error("a development build didn't look in .build")
	}
	beside := filepath.Join(config.ProgramDir(), file)
	if err := os.WriteFile(beside, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(beside)
	if got := release.findProgram(name); got != beside {
		t.Errorf("a release didn't find the program beside it: %q", got)
	}
}
