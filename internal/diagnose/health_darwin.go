package diagnose

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// driveHealth asks macOS what it knows about a drive, which takes no special
// rights.
func driveHealth(ctx context.Context, log *slog.Logger, f sysinfo.FS, _ string) healthResult {
	if f.Mount == "" {
		return healthResult{note: "macOS can't say which drive this is. Not checked."}
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "diskutil", "info", f.Mount)
	var out, stderr logging.Tail
	cmd.Stdout, cmd.Stderr = &out, &stderr
	started := time.Now()
	err := cmd.Run()
	logging.ChildExit(log, "drive health", err, ctx.Err() != nil, &stderr, started)
	if err != nil {
		return healthResult{note: "macOS didn't say. Not checked."}
	}
	r := healthResult{checked: true}
	for _, line := range strings.Split(out.String(), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "Device / Media Name":
			r.lines = append(r.lines, "Drive: "+v)
		case "Protocol":
			r.lines = append(r.lines, "Connected by: "+v)
		case "Solid State":
			if v == "Yes" {
				r.lines = append(r.lines, "Kind: solid state")
			} else {
				r.lines = append(r.lines, "Kind: spinning disk")
			}
		case "SMART Status":
			r.lines = append(r.lines, "Health: "+v)
			if strings.Contains(strings.ToLower(v), "fail") {
				r.bad = true
			}
		}
	}
	return r
}
