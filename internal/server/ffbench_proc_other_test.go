//go:build !windows

package server

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// The benchmark's view of a process elsewhere: Linux reads /proc, which keeps the peak of
// resident memory (VmHWM); other systems ask ps, and the peak is the most it was seen using.
// A process's last tenth of a second goes unseen once it has ended.

type benchProcHandle = int

func benchListProcs() []benchProcInfo {
	if runtime.GOOS == "linux" {
		return linuxProcs()
	}
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,comm=").Output()
	if err != nil {
		return nil
	}
	var list []benchProcInfo
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		list = append(list, benchProcInfo{pid: pid, ppid: ppid, name: filepath.Base(strings.Join(f[2:], " "))})
	}
	return list
}

func linuxProcs() []benchProcInfo {
	dirs, _ := os.ReadDir("/proc")
	var list []benchProcInfo
	for _, d := range dirs {
		pid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + d.Name() + "/stat")
		if err != nil {
			continue
		}
		s := string(b)
		open, close := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
		if open < 0 || close < open {
			continue
		}
		f := strings.Fields(s[close+1:])
		if len(f) < 2 {
			continue
		}
		ppid, _ := strconv.Atoi(f[1])
		list = append(list, benchProcInfo{pid: pid, ppid: ppid, name: s[open+1 : close]})
	}
	return list
}

func benchReadProc(st *benchProcStat) {
	if runtime.GOOS != "linux" {
		out, err := exec.Command("ps", "-o", "rss=,time=", "-p", strconv.Itoa(st.pid)).Output()
		if f := strings.Fields(string(out)); err == nil && len(f) == 2 {
			kb, _ := strconv.ParseUint(f[0], 10, 64)
			st.peak = max(st.peak, kb*1024)
			st.cpu = max(st.cpu, psTime(f[1]))
		}
		return
	}
	dir := "/proc/" + strconv.Itoa(st.pid)
	if b, err := os.ReadFile(dir + "/stat"); err == nil {
		s := string(b)
		if i := strings.LastIndexByte(s, ')'); i > 0 {
			f := strings.Fields(s[i+1:])
			if len(f) > 12 {
				utime, _ := strconv.ParseFloat(f[11], 64)
				stime, _ := strconv.ParseFloat(f[12], 64)
				st.cpu = max(st.cpu, (utime+stime)/100) // USER_HZ is 100 on every Linux the server runs on
			}
		}
	}
	f, err := os.Open(dir + "/status")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "VmHWM:"); ok {
			kb, _ := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
			st.peak = max(st.peak, kb*1024)
		}
	}
}

// psTime reads ps's [[dd-]hh:]mm:ss[.cc] as seconds.
func psTime(v string) float64 {
	days := 0.0
	if d, rest, ok := strings.Cut(v, "-"); ok {
		days, _ = strconv.ParseFloat(d, 64)
		v = rest
	}
	total := 0.0
	for _, part := range strings.Split(v, ":") {
		n, _ := strconv.ParseFloat(part, 64)
		total = total*60 + n
	}
	return days*86400 + total
}

func benchReleaseProc(*benchProcStat) {}
