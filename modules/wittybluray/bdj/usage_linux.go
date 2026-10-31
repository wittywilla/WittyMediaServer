package bdj

import (
	"bytes"
	"os"
	"strconv"
	"time"
)

// usage reads what a process holds and has used from /proc.
func usage(pid int) (u procUsage, ok bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return u, false
	}
	// The second field is the program's name, in parentheses, and may hold spaces.
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return u, false
	}
	w := bytes.Fields(b[i+1:])
	if len(w) < 22 {
		return u, false
	}
	field := func(n int) uint64 { // numbered as proc(5) numbers them
		v, _ := strconv.ParseUint(string(w[n-3]), 10, 64)
		return v
	}
	const tick = 10 * time.Millisecond // USER_HZ is 100 on every Linux Go runs on
	u.cpu = time.Duration(field(14)+field(15)) * tick
	u.threads = int(field(20))
	u.memory = field(24) * uint64(os.Getpagesize())
	return u, true
}
