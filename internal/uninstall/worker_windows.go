//go:build windows

package uninstall

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// StartWorker copies this program to the temp folder and starts the copy, because Windows
// won't delete a running program, and the copy can delete this one. It doesn't wait: this
// process has to end for its own file to go.
func StartWorker(flags []string, elevate bool) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "wittymediaserver-uninstall-")
	if err != nil {
		return err
	}
	copyTo := filepath.Join(dir, filepath.Base(self))
	if err := copyFile(self, copyTo); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	if elevate {
		return runElevated(copyTo, dir, flags)
	}
	cmd := exec.Command(copyTo, flags...)
	cmd.Dir = dir
	hide(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	return cmd.Process.Release()
}

func runElevated(file, dir string, flags []string) error {
	verb, _ := windows.UTF16PtrFromString("runas")
	exe, _ := windows.UTF16PtrFromString(file)
	quoted := make([]string, len(flags))
	for i, f := range flags {
		quoted[i] = syscall.EscapeArg(f)
	}
	args, _ := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	wd, _ := windows.UTF16PtrFromString(dir)
	if err := windows.ShellExecute(0, verb, exe, args, wd, windows.SW_SHOWNORMAL); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, in); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// RemoveSelfLater clears a worker's temp folder after it has exited. Programs by their full
// path: cmd.exe looks in the current folder first, and anything may be in a temp folder.
func RemoveSelfLater() {
	self, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(self)
	if !strings.HasPrefix(strings.ToLower(filepath.Base(dir)), "wittymediaserver-uninstall-") {
		return
	}
	if cmd := removeLater(dir); cmd != nil {
		_ = cmd.Start()
	}
}

func removeLater(dir string) *exec.Cmd {
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return nil
	}
	cmd := exec.Command(filepath.Join(sys, "cmd.exe"))
	cmd.Dir = os.TempDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW,
		// cmd.exe takes off the first and last quote of what follows /c, so all of it is in quotes.
		CmdLine: `cmd.exe /d /c ""` + filepath.Join(sys, "PING.EXE") + `" -n 2 127.0.0.1 >nul & rmdir /s /q "` + dir + `""`}
	return cmd
}
