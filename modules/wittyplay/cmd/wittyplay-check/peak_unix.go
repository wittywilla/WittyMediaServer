//go:build !windows

package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// peakMB is the most memory a running process has held, from /proc where there is
// one, and 0 elsewhere.
func peakMB(pid int) float64 {
	f, err := os.Open("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "VmHWM:"); ok {
			kb, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "kB")))
			return float64(kb) / 1024
		}
	}
	return 0
}
