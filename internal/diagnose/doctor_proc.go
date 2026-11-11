// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package diagnose

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// What's running, and who has a port. The system's own tools answer these, each with a time
// limit. None needs administrator rights, and what they can't say is simply left out.

type procInfo struct {
	pid     int
	exe     string
	cmdline string
}

// procTimeout bounds each tool. A hung system tool must never hang the doctor.
const procTimeout = 15 * time.Second

func toolOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, procTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	return cmd.Output()
}

// parseProcTree reads a /proc folder.
func parseProcTree(root string) []procInfo {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []procInfo
	for _, d := range entries {
		pid, err := strconv.Atoi(d.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		exe, err := os.Readlink(filepath.Join(root, d.Name(), "exe"))
		if err != nil {
			continue // someone else's, or gone
		}
		b, _ := os.ReadFile(filepath.Join(root, d.Name(), "cmdline"))
		out = append(out, procInfo{pid: pid, exe: strings.TrimSuffix(exe, " (deleted)"), cmdline: string(bytes.ReplaceAll(b, []byte{0}, []byte{' '}))})
	}
	return out
}

// parsePS reads what `ps -axo pid=,command=` prints.
func parsePS(b []byte) []procInfo {
	var out []procInfo
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		i := strings.IndexByte(line, ' ')
		if i <= 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:i])
		if err != nil || pid == os.Getpid() {
			continue
		}
		cmd := strings.TrimSpace(line[i+1:])
		exe := cmd
		// A path with spaces can't be told apart here, but the first word does for
		// what's asked of it: is it inside this folder, and is it FFmpeg?
		if j := strings.IndexByte(cmd, ' '); j > 0 {
			exe = cmd[:j]
		}
		out = append(out, procInfo{pid: pid, exe: exe, cmdline: cmd})
	}
	return out
}

// parseCSVLine reads one line of the list PowerShell prints below.
var winLine = regexp.MustCompile(`^(\d+)\t([^\t]*)\t(.*)$`)

func parseWinList(b []byte) []procInfo {
	var out []procInfo
	for _, line := range strings.Split(string(b), "\n") {
		m := winLine.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		pid, err := strconv.Atoi(m[1])
		if err != nil || pid == os.Getpid() || m[2] == "" {
			continue
		}
		out = append(out, procInfo{pid: pid, exe: m[2], cmdline: m[3]})
	}
	return out
}

func listProcesses(ctx context.Context) ([]procInfo, error) {
	switch runtime.GOOS {
	case "linux":
		return parseProcTree("/proc"), nil
	case "windows":
		b, err := toolOutput(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
			`Get-CimInstance Win32_Process -Property ProcessId,ExecutablePath,CommandLine | ForEach-Object { "$($_.ProcessId)`+"`t"+`$($_.ExecutablePath)`+"`t"+`$($_.CommandLine)" }`)
		if err != nil {
			return nil, err
		}
		return parseWinList(b), nil
	}
	b, err := toolOutput(ctx, "ps", "-axo", "pid=,command=")
	if err != nil {
		return nil, err
	}
	return parsePS(b), nil
}

// portOwner says who is listening on a port, as well as the system will say.
func portOwner(ctx context.Context, port int) string {
	p := strconv.Itoa(port)
	switch runtime.GOOS {
	case "windows":
		b, err := toolOutput(ctx, "netstat", "-ano", "-p", "TCP")
		if err != nil {
			return ""
		}
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) >= 5 && strings.HasSuffix(f[1], ":"+p) && strings.EqualFold(f[3], "LISTENING") {
				pid, err := strconv.Atoi(f[4])
				if err != nil {
					return ""
				}
				if list, err := listProcesses(ctx); err == nil {
					for _, x := range list {
						if x.pid == pid {
							return filepath.Base(x.exe) + ", process " + f[4]
						}
					}
				}
				return "process " + f[4]
			}
		}
	case "linux":
		if b, err := toolOutput(ctx, "ss", "-H", "-ltnp", "sport", "=", ":"+p); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				if i := strings.Index(s, "users:"); i >= 0 {
					return s[i:]
				}
			}
		}
	default:
		if b, err := toolOutput(ctx, "lsof", "-nP", "-iTCP:"+p, "-sTCP:LISTEN"); err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			if len(lines) >= 2 {
				if f := strings.Fields(lines[1]); len(f) >= 2 {
					return f[0] + ", process " + f[1]
				}
			}
		}
	}
	return ""
}
