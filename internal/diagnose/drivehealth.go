package diagnose

import (
	"context"
	"log/slog"

	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// DriveHealth is what a drive says about itself, for the statistics.
type DriveHealth struct {
	Checked bool     // the system answered
	Bad     bool     // and what it said is a worry
	Lines   []string // what it said, in words
	Note    string   // why it wasn't checked
}

// ReadDriveHealth asks the system about the drive holding a folder. On some systems that
// means starting a program, which can take seconds, so this is for now and then. A drive on
// another machine is that machine's to report.
func ReadDriveHealth(ctx context.Context, log *slog.Logger, path string) DriveHealth {
	f, err := sysinfo.FSInfo(path)
	if err != nil {
		return DriveHealth{Note: "The system couldn't say which drive this is."}
	}
	if f.Network() {
		return DriveHealth{Note: "This drive is on another machine, so it's that machine's to report."}
	}
	r := driveHealth(ctx, log, f, path)
	return DriveHealth{Checked: r.checked, Bad: r.bad, Lines: r.lines, Note: r.note}
}
