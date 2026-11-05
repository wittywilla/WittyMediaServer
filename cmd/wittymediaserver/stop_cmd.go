package main

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// logStart is the first line of every run: how the last one ended, and what started this one.
func logStart(log *slog.Logger, guard *logging.CrashGuard, reports []logging.Crash, stderrFile string) {
	for _, c := range reports {
		if c.Kind != "unclean" { // an unclean end is the line below
			log.Error("the server stopped unexpectedly last time", "report", c.ID, "kind", c.Kind, "version", c.Version, "summary", c.Summary)
		}
	}
	attrs := []any{"cat", "server", "version", version, "pid", os.Getpid()}
	if by := startedBy(); by != "" {
		attrs = append(attrs, "by", by)
	}
	if stderrFile != "" {
		attrs = append(attrs, "stderr", stderrFile)
	}
	prev, ok := guard.Previous()
	switch {
	case !ok:
		log.Info("started", append(attrs, "previous", "none recorded")...)
	case prev.Clean:
		log.Info("started", append(attrs, "previous", prev.Summary())...)
	default:
		log.Error("the last run ended without stopping cleanly", "cat", "server", "previous", prev.Summary(), "report", prev.Report, "lastLog", prev.LastLog)
		log.Info("started", append(attrs, "previous", prev.Summary())...)
	}
}

// startedBy says what started this run, where that's known.
func startedBy() string {
	switch {
	case os.Getenv("WMS_TRAY_RESTART") == "1":
		return "the tray app, after the server stopped unexpectedly"
	case os.Getenv("WMS_RESTARTED") == "1":
		return "the server itself, restarting"
	case asService:
		return "the Windows service manager"
	case os.Getenv("INVOCATION_ID") != "":
		return "systemd"
	}
	return ""
}

// stopOnError is a stop because of an error: said, and kept for the next start.
func stopOnError(log *slog.Logger, what string, err error) int {
	logging.NoteStop(logging.StopFatal, what+": "+err.Error(), "")
	st := logging.StopNoted()
	log.Warn("stopping", "cat", "server", "reason", st.Reason, "why", st.Words())
	return 1
}

// trayReasons are the reasons the tray app may give. Anything else it sends is ignored.
var trayReasons = map[string]bool{
	logging.StopTrayQuit: true, logging.StopTrayStop: true, logging.StopTrayRestart: true, logging.StopTrayClosed: true,
	logging.StopShutdown: true, logging.StopLogoff: true, logging.StopInstaller: true,
}

// readTrayStop reads what the tray app says on the server's standard input until it
// closes: "stop <reason>" before it closes it on purpose. It returns at the end of input.
func readTrayStop(in io.Reader) {
	r := bufio.NewReaderSize(in, 256)
	long := false // in the middle of a line too long to be the tray's
	for {
		line, err := r.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			long = true
			continue
		}
		if !long {
			if reason := trayStopReason(string(line)); reason != "" {
				logging.NoteStop(reason, "said by the tray app", "")
			}
		}
		long = false
		if err != nil {
			return
		}
	}
}

// trayStopReason reads one line from the tray app: a reason it may give, or "".
func trayStopReason(line string) string {
	if reason, ok := strings.CutPrefix(strings.TrimSpace(line), "stop "); ok && trayReasons[reason] {
		return reason
	}
	return ""
}
