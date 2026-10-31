package diagnose

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// driveHealth asks Windows what the drive says about itself. This needs no
// administrator rights on most systems; where it does, the answer is "not checked".
func driveHealth(ctx context.Context, log *slog.Logger, f sysinfo.FS, path string) healthResult {
	if f.UNC || len(path) < 2 || path[1] != ':' {
		return healthResult{note: "Windows can't say which drive this is. Needs administrator rights, not checked."}
	}
	letter := strings.ToUpper(path[:1])
	if letter[0] < 'A' || letter[0] > 'Z' {
		return healthResult{note: "Windows can't say which drive this is. Needs administrator rights, not checked."}
	}
	// Only the drive letter, checked just above, is put into this script.
	script := `$ErrorActionPreference='Stop';$n=(Get-Partition -DriveLetter ` + letter + ` | Get-Disk).Number;` +
		`Get-PhysicalDisk | Where-Object {$_.DeviceId -eq "$n"} | ForEach-Object {"$($_.FriendlyName)` + "`t" + `$($_.MediaType)` + "`t" + `$($_.HealthStatus)` + "`t" + `$($_.OperationalStatus)` + "`t" + `$($_.BusType)"}`
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	var out, stderr logging.Tail
	cmd.Stdout, cmd.Stderr = &out, &stderr
	started := time.Now()
	err := cmd.Run()
	logging.ChildExit(log, "drive health", err, ctx.Err() != nil, &stderr, started)
	if err != nil {
		return healthResult{note: "Windows didn't say: it needs administrator rights, not checked."}
	}
	return parseWindowsHealth(out.String())
}

// parseWindowsHealth reads the script's tab-separated line: name, media type,
// health, operational status, bus.
func parseWindowsHealth(text string) healthResult {
	r := healthResult{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) < 5 {
			continue
		}
		r.checked = true
		name, media, health, op, bus := parts[0], parts[1], parts[2], parts[3], parts[4]
		r.lines = append(r.lines, "Drive: "+name, "Kind: "+media+" on "+bus, "Health: "+health, "Status: "+op)
		if h := strings.ToLower(health); h != "" && h != "healthy" && h != "0" {
			r.bad = true
		}
		if o := strings.ToLower(op); o != "" && !strings.Contains(o, "ok") && o != "online" && o != "2" {
			r.bad = true
		}
		break
	}
	if !r.checked {
		r.note = "Windows has nothing to say about this drive: it's common for USB drives, and needs administrator rights for more. Not checked."
	}
	return r
}
