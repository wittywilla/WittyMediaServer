package diagnose

import (
	"context"
	"log/slog"

	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

// driveHealth has nothing to ask on FreeBSD without administrator rights.
func driveHealth(_ context.Context, _ *slog.Logger, _ sysinfo.FS, _ string) healthResult {
	return healthResult{note: "FreeBSD needs administrator rights to say. Not checked."}
}
