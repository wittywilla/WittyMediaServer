// Command wms-forwarder ships as wms(.exe) in the .zip and .tar.gz packages, only so a copy
// of 0.5.2-alpha or older can update to this one from Settings. Those copies start wms from
// the top folder after an update, or a task or script someone made does; the server is now
// bin/wittymediaserver. It runs that with the same arguments, keeps its stdin, stdout, stderr
// and exit code, and does nothing else.
//
// Installers don't carry it. It can go once 0.5.2-alpha is out of the way.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	name := "wittymediaserver"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	cmd := exec.Command(filepath.Join(filepath.Dir(exe), "bin", name), os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		os.Exit(exit.ExitCode())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
