// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build !windows

package uninstall

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type realSystem struct{}

func hide(cmd *exec.Cmd) {}

func (realSystem) Run(name string, args ...string) (string, error) { return runProgram(name, args...) }

func isAdmin() bool { return os.Geteuid() == 0 }

func (realSystem) RegKeyExists(string, string) bool               { return false }
func (realSystem) RegValue(string, string, string) (string, bool) { return "", false }
func (realSystem) RegDeleteKey(string, string) error              { return errNotWindows }
func (realSystem) RegDeleteValue(string, string, string) error    { return errNotWindows }

// programsIn lists what's running from inside dir, using /proc where there is one and ps
// where there isn't.
func programsIn(dir string) []int {
	self := os.Getpid()
	var out []int
	dir = filepath.Clean(dir) + string(filepath.Separator)
	if entries, err := os.ReadDir("/proc"); err == nil && len(entries) > 0 {
		for _, e := range entries {
			pid, err := strconv.Atoi(e.Name())
			if err != nil || pid == self {
				continue
			}
			if exe, err := os.Readlink("/proc/" + e.Name() + "/exe"); err == nil && strings.HasPrefix(exe, dir) {
				out = append(out, pid)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	b, err := exec.Command("ps", "-axo", "pid=,comm=").Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		pidText, comm, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		if pid, err := strconv.Atoi(pidText); err == nil && pid != self && strings.HasPrefix(strings.TrimSpace(comm), dir) {
			out = append(out, pid)
		}
	}
	return out
}

func (realSystem) StopProgramsIn(dir string) {
	for _, pid := range programsIn(dir) {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		if len(programsIn(dir)) == 0 {
			return
		}
	}
	for _, pid := range programsIn(dir) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}
