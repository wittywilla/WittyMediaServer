package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Standard error is where Go writes a crash, and where a few messages go before the log is
// up. A terminal, the tray app's console.log, systemd's journal and Docker's log all keep
// it. A Windows service, or a launchd job with nowhere set, throw it away: there it goes
// to stderr-<pid>.txt in the crash folder instead, and a run that ends badly has it added
// to its report.

func stderrName(pid int) string { return fmt.Sprintf("stderr-%d.txt", pid) }

// CaptureStderr sends this process's standard error to the crash folder when nothing else
// keeps it. It says where it went, or "" when it was left alone. Call it before the log is
// set up, and give the log io.Discard rather than os.Stderr when it returns a path: the
// log has its own files.
func CaptureStderr(crashDir string) string {
	if !stderrLost() {
		return ""
	}
	if err := os.MkdirAll(crashDir, 0o755); err != nil {
		return ""
	}
	p := filepath.Join(crashDir, stderrName(os.Getpid()))
	f, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return ""
	}
	if err := redirectStderr(f); err != nil {
		f.Close()
		_ = os.Remove(p)
		return ""
	}
	os.Stderr = f
	capturedStderr = f
	return p
}

// capturedStderr is held until a guard takes it, which closes it at a clean stop.
var capturedStderr *os.File

// closeStderr: a clean stop. An empty file isn't worth keeping.
func (g *CrashGuard) closeStderr() {
	f := capturedStderr
	if f == nil || filepath.Dir(f.Name()) != g.dir {
		return
	}
	if st, err := f.Stat(); err == nil && st.Size() == 0 {
		_ = os.Remove(f.Name())
	}
}

// readStderr is the end of what an earlier run wrote to standard error, if it was kept.
func (g *CrashGuard) readStderr(pid int) string {
	b, err := readBounded(filepath.Join(g.dir, stderrName(pid)), 64<<10)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(Redact(string(b)))
}

// removeOldStderr clears what earlier runs left with no marker beside them: a file a
// clean stop couldn't remove because it was still open (Windows), say. Those ended cleanly,
// so there's nothing in them a report needs.
func (g *CrashGuard) removeOldStderr() {
	entries, _ := os.ReadDir(g.dir)
	for _, e := range entries {
		n, ok := strings.CutPrefix(e.Name(), "stderr-")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSuffix(n, ".txt"))
		if err != nil || pid == os.Getpid() || pidAlive(pid) {
			continue
		}
		if _, err := os.Stat(filepath.Join(g.dir, fmt.Sprintf("run-%d.json", pid))); err == nil {
			continue // a run still to be reported
		}
		_ = os.Remove(filepath.Join(g.dir, e.Name()))
	}
}

// removeStderr: an earlier run's file, once it's in a report or wasn't needed.
func (g *CrashGuard) removeStderr(pid int) {
	if pid != os.Getpid() {
		_ = os.Remove(filepath.Join(g.dir, stderrName(pid)))
	}
}
