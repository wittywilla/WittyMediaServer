package diagnose

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// driveHealth reads what the kernel shows anyone about a drive. The drive's own
// health records need administrator rights, so those aren't checked.
func driveHealth(_ context.Context, _ *slog.Logger, f sysinfo.FS, _ string) healthResult {
	dev := sysinfo.BlockDevice(f.Source)
	if dev == "" {
		return healthResult{note: "This isn't a local disk the system can name. Needs administrator rights, not checked."}
	}
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join("/sys/block", dev, rel))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(b))
	}
	r := healthResult{checked: true}
	if m := read("device/model"); m != "" {
		r.lines = append(r.lines, "Drive: "+m)
	}
	switch read("queue/rotational") {
	case "1":
		r.lines = append(r.lines, "Kind: spinning disk")
	case "0":
		r.lines = append(r.lines, "Kind: solid state")
	}
	if st := read("device/state"); st != "" {
		r.lines = append(r.lines, "State: "+st)
		if st != "running" && st != "live" {
			r.bad = true
		}
	}
	matches, _ := filepath.Glob(filepath.Join("/sys/block", dev, "device", "hwmon*", "temp1_input"))
	more, _ := filepath.Glob(filepath.Join("/sys/block", dev, "device", "device", "hwmon*", "temp1_input"))
	for _, p := range append(matches, more...) {
		if b, err := os.ReadFile(p); err == nil {
			if v, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && v > 0 {
				r.lines = append(r.lines, fmt.Sprintf("Temperature: %d °C", v/1000))
				if v/1000 >= 70 {
					r.lines = append(r.lines, "That's hot: check the airflow around this drive")
				}
				break
			}
		}
	}
	r.lines = append(r.lines, "The drive's own health records need administrator rights, not checked")
	return r
}
