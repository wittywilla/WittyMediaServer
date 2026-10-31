package sysinfo

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func osRelease() string {
	name := ""
	if f, err := os.Open("/etc/os-release"); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
				name = strings.Trim(v, `"'`)
				break
			}
		}
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		name = strings.TrimSpace(name + " (kernel " + strings.TrimSpace(string(b)) + ")")
	}
	return name
}

func cpuName() string {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "model name", "Model", "Hardware":
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func memory() (total, free int64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var kb int64
		if n, _ := fmt.Sscanf(sc.Text(), "MemTotal: %d kB", &kb); n == 1 {
			total = kb >> 10
		}
		if n, _ := fmt.Sscanf(sc.Text(), "MemAvailable: %d kB", &kb); n == 1 {
			free = kb >> 10
		}
	}
	return total, free
}

var gpuVendors = map[string]string{"0x10de": "NVIDIA", "0x1002": "AMD", "0x8086": "Intel", "0x14e4": "Broadcom", "0x1af4": "virtual"}

// gpus lists what the kernel sees: names need a database we don't have, but the
// driver and the numbers tell enough.
func gpus() []string {
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]")
	var out []string
	for _, c := range cards {
		vendor, _ := os.ReadFile(filepath.Join(c, "device", "vendor"))
		device, _ := os.ReadFile(filepath.Join(c, "device", "device"))
		driver, _ := os.Readlink(filepath.Join(c, "device", "driver"))
		v := strings.TrimSpace(string(vendor))
		name := gpuVendors[v]
		if name == "" {
			name = v
		}
		s := strings.TrimSpace(name + " " + strings.TrimSpace(string(device)))
		if driver != "" {
			s += " (driver " + filepath.Base(driver) + ")"
		}
		if s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
