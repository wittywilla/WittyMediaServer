// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package uninstall

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type realSystem struct{}

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

func (realSystem) Run(name string, args ...string) (string, error) { return runProgram(name, args...) }

func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func hive(name string) registry.Key {
	if name == "HKLM" {
		return registry.LOCAL_MACHINE
	}
	return registry.CURRENT_USER
}

func (realSystem) RegKeyExists(h, key string) bool {
	k, err := registry.OpenKey(hive(h), key, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

func (realSystem) RegValue(h, key, name string) (string, bool) {
	k, err := registry.OpenKey(hive(h), key, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return "", false
	}
	return v, true
}

func (realSystem) RegDeleteKey(h, key string) error {
	err := registry.DeleteKey(hive(h), key)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func (realSystem) RegDeleteValue(h, key, name string) error {
	k, err := registry.OpenKey(hive(h), key, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer k.Close()
	err = k.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

// longPath gives a path's full, long form in lower case for comparing (Windows may name the
// same folder C:\Users\LONGNA~1).
func longPath(p string) string {
	p = filepath.Clean(p)
	if u, err := windows.UTF16PtrFromString(p); err == nil {
		buf := make([]uint16, windows.MAX_LONG_PATH)
		if n, err := windows.GetLongPathName(u, &buf[0], uint32(len(buf))); err == nil && n > 0 && int(n) < len(buf) {
			p = windows.UTF16ToString(buf[:n])
		}
	}
	return strings.ToLower(p)
}

func imagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	return longPath(windows.UTF16ToString(buf[:n]))
}

// running lists the processes whose program is inside dir, but not this one.
func running(dir string) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	prefix := longPath(dir) + `\`
	var out []uint32
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID == uint32(os.Getpid()) || e.ProcessID == 0 {
			continue
		}
		if strings.HasPrefix(imagePath(e.ProcessID), prefix) {
			out = append(out, e.ProcessID)
		}
	}
	return out
}

func kill(pid uint32) {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return
	}
	_ = windows.TerminateProcess(h, 1)
	windows.CloseHandle(h)
}

// StopProgramsIn ends the tray app first: that closes the server's control pipe, so the
// server shuts down cleanly by itself. Whatever still runs after a while is ended.
func (realSystem) StopProgramsIn(dir string) {
	for _, pid := range running(dir) {
		if strings.EqualFold(filepath.Base(imagePath(pid)), "wittymediaserver.exe") && filepath.Base(filepath.Dir(imagePath(pid))) != "bin" {
			kill(pid)
		}
	}
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(300 * time.Millisecond) {
		if len(running(dir)) == 0 {
			return
		}
	}
	for _, pid := range running(dir) {
		kill(pid)
	}
	time.Sleep(time.Second)
}
