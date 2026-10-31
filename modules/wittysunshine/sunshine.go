// Package wittysunshine is the advanced option of hosting demanding PC games on the server's own
// hardware, through Sunshine as a separate program that its owner installs. It finds that program.
// Everything that talks to Sunshine is in WittyMoonlight's gamestream package, as it's the same
// protocol as reaching any other PC. See docs/GAME-RIGS.md.
package wittysunshine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	// License is Sunshine's own, read from its repository.
	License = "GPL-3.0"
	// Source is its project.
	Source = "https://github.com/LizardByte/Sunshine"
)

// Find says where Sunshine is installed on this machine, if anywhere it usually goes.
func Find() string {
	var places []string
	switch runtime.GOOS {
	case "windows":
		for _, k := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
			if v := os.Getenv(k); v != "" {
				places = append(places, filepath.Join(v, "Sunshine", "sunshine.exe"))
			}
		}
	case "darwin":
		places = []string{"/Applications/Sunshine.app/Contents/MacOS/sunshine"}
	}
	for _, p := range places {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	if p, err := exec.LookPath("sunshine"); err == nil {
		return p
	}
	return ""
}
