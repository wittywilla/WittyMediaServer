package sysinfo

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// ReadCPU gives the whole machine and each core.
func ReadCPU() (all CPUTicks, cores []CPUTicks, ok bool) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return all, nil, false
	}
	for line := range bytes.Lines(b) {
		if !bytes.HasPrefix(line, []byte("cpu")) {
			break // the cpu lines come first
		}
		f := strings.Fields(string(line))
		if len(f) < 5 {
			continue
		}
		var t CPUTicks
		for i, v := range f[1:] {
			if i >= 8 { // guest time is counted in user time already
				break
			}
			n, _ := strconv.ParseUint(v, 10, 64)
			t.Total += n
			if i != 3 && i != 4 { // idle and iowait
				t.Busy += n
			}
		}
		if f[0] == "cpu" {
			all, ok = t, true
		} else {
			cores = append(cores, t)
		}
	}
	return all, cores, ok
}

// ReadMemory gives the machine's memory.
func ReadMemory() (MemoryUse, bool) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return MemoryUse{}, false
	}
	defer f.Close()
	var m MemoryUse
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		n, _ := strconv.ParseUint(fields[0], 10, 64)
		n *= 1024
		switch name {
		case "MemTotal":
			m.Total = n
		case "MemAvailable":
			m.Available = n
		case "SwapTotal":
			m.SwapTotal = n
		case "SwapFree":
			m.SwapFree = n
		}
	}
	return m, m.Total > 0
}

// ReadNetwork gives each interface, leaving out the loopback.
func ReadNetwork() []NetCounters {
	b, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil
	}
	var out []NetCounters
	for line := range bytes.Lines(b) {
		name, rest, ok := strings.Cut(string(line), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		f := strings.Fields(rest)
		if name == "lo" || len(f) < 16 {
			continue
		}
		n := func(i int) uint64 { v, _ := strconv.ParseUint(f[i], 10, 64); return v }
		c := NetCounters{Name: name, RxBytes: n(0), RxErrs: n(2), TxBytes: n(8), TxErrs: n(10)}
		if st, err := os.ReadFile("/sys/class/net/" + name + "/operstate"); err == nil {
			s := strings.TrimSpace(string(st))
			c.Up = s == "up" || s == "unknown"
		}
		if sp, err := os.ReadFile("/sys/class/net/" + name + "/speed"); err == nil {
			if v, err := strconv.ParseInt(strings.TrimSpace(string(sp)), 10, 64); err == nil && v > 0 {
				c.SpeedMbps = v
			}
		}
		out = append(out, c)
	}
	return out
}

// ReadDiskIO gives each whole disk.
func ReadDiskIO() []DiskCounters {
	b, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil
	}
	var out []DiskCounters
	for line := range bytes.Lines(b) {
		f := strings.Fields(string(line))
		if len(f) < 13 {
			continue
		}
		name := f[2]
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "zram") {
			continue
		}
		// A partition has a "partition" file; a whole disk doesn't. Counting both counts twice.
		if _, err := os.Stat("/sys/class/block/" + name + "/partition"); err == nil {
			continue
		}
		n := func(i int) uint64 { v, _ := strconv.ParseUint(f[i], 10, 64); return v }
		out = append(out, DiskCounters{Name: name, ReadBytes: n(5) * 512, WriteBytes: n(9) * 512, BusyMs: n(12)})
	}
	return out
}

// ReadVolumes gives each mounted file system that holds files of its own.
func ReadVolumes() []Volume {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	skip := map[string]bool{"proc": true, "sysfs": true, "devtmpfs": true, "devpts": true, "tmpfs": true, "cgroup": true, "cgroup2": true, "securityfs": true, "pstore": true,
		"debugfs": true, "tracefs": true, "bpf": true, "mqueue": true, "hugetlbfs": true, "configfs": true, "fusectl": true, "binfmt_misc": true, "autofs": true, "overlay": true,
		"squashfs": true, "nsfs": true, "rpc_pipefs": true, "ramfs": true, "efivarfs": true}
	seen := map[string]bool{}
	var out []Volume
	for line := range bytes.Lines(b) {
		m, ok := ParseMountLine(strings.TrimRight(string(line), "\n"))
		if !ok || skip[m.Type] || seen[m.Source+m.Type] {
			continue
		}
		var st syscall.Statfs_t
		if syscall.Statfs(m.Mount, &st) != nil || st.Blocks == 0 {
			continue
		}
		seen[m.Source+m.Type] = true
		kind := "fixed"
		if networkTypes[m.Type] {
			kind = "network"
		}
		bs := uint64(st.Bsize)
		out = append(out, Volume{Mount: m.Mount, Device: filepath.Base(m.Source), Type: m.Type, Kind: kind, Total: uint64(st.Blocks) * bs, Free: uint64(st.Bavail) * bs})
	}
	return out
}

// ReadTemperatures gives what the kernel's sensors say.
func ReadTemperatures() []Temperature {
	var out []Temperature
	hw, _ := filepath.Glob("/sys/class/hwmon/hwmon*/temp*_input")
	for _, p := range hw {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil || v <= 0 {
			continue
		}
		dir := filepath.Dir(p)
		name := ""
		if n, err := os.ReadFile(filepath.Join(dir, "name")); err == nil {
			name = strings.TrimSpace(string(n))
		}
		if l, err := os.ReadFile(strings.TrimSuffix(p, "_input") + "_label"); err == nil {
			name += " " + strings.TrimSpace(string(l))
		}
		out = append(out, Temperature{Name: strings.TrimSpace(name), Celsius: v / 1000})
	}
	if len(out) == 0 {
		zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
		for _, p := range zones {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			if err != nil || v <= 0 {
				continue
			}
			name := filepath.Base(filepath.Dir(p))
			if t, err := os.ReadFile(filepath.Join(filepath.Dir(p), "type")); err == nil {
				name = strings.TrimSpace(string(t))
			}
			out = append(out, Temperature{Name: name, Celsius: v / 1000})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ReadProcess gives this program's own use.
func ReadProcess() (ProcessUse, bool) {
	var ru syscall.Rusage
	var p ProcessUse
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) == nil {
		p.CPUSec = float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6 + float64(ru.Stime.Sec) + float64(ru.Stime.Usec)/1e6
	}
	if b, err := os.ReadFile("/proc/self/statm"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 1 {
			n, _ := strconv.ParseUint(f[1], 10, 64)
			p.RSS = n * uint64(os.Getpagesize())
		}
	}
	if b, err := os.ReadFile("/proc/self/stat"); err == nil {
		if i := bytes.LastIndexByte(b, ')'); i > 0 {
			if f := strings.Fields(string(b[i+1:])); len(f) > 17 {
				p.Threads, _ = strconv.Atoi(f[17])
			}
		}
	}
	if e, err := os.ReadDir("/proc/self/fd"); err == nil {
		p.Handles = len(e)
	}
	return p, true
}
