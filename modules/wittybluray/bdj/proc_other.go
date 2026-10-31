//go:build !windows

package bdj

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func hide(c *exec.Cmd) {}

// javaCommand runs Java slightly niced where nice exists, so a disc's code gives
// way to the server without stalling beside a busy FFmpeg. nice replaces itself
// with Java: the process is the same one.
func javaCommand(path string, args ...string) *exec.Cmd {
	if n, err := exec.LookPath("nice"); err == nil {
		return exec.Command(n, append([]string{"-n", "5", path}, args...)...)
	}
	return exec.Command(path, args...)
}

// usagePS asks ps, for systems without a /proc to read. It knows no thread
// count; the runtime watches that itself as well.
func usagePS(pid int) (u procUsage, ok bool) {
	out, err := exec.Command("ps", "-o", "rss=", "-o", "time=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return u, false
	}
	w := strings.Fields(string(out))
	if len(w) != 2 {
		return u, false
	}
	kb, err := strconv.ParseUint(w[0], 10, 64)
	if err != nil {
		return u, false
	}
	u.memory = kb * 1024
	// [dd-]hh:mm:ss[.cc], or mm:ss.cc
	t := w[1]
	days := 0
	if d, rest, found := strings.Cut(t, "-"); found {
		days, _ = strconv.Atoi(d)
		t = rest
	}
	var secs float64
	for _, p := range strings.Split(t, ":") {
		f, _ := strconv.ParseFloat(p, 64)
		secs = secs*60 + f
	}
	u.cpu = time.Duration((float64(days)*86400 + secs) * float64(time.Second))
	return u, true
}
