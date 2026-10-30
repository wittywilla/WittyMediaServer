package stream

import (
	"log/slog"
	"os/exec"
	"strings"
)

func hideWindow(cmd *exec.Cmd) {}

func runnerEnv() []string { return []string{"PATH=/usr/bin:/bin", "HOME=/nonexistent"} }

// prepareSandbox wraps the runner in sandbox-exec, Apple's own tool: no network,
// and writing only to the save folder. Apple has marked the tool deprecated, but
// it still ships, and if it is ever removed the runner keeps its resource limits.
func prepareSandbox(cmd *exec.Cmd, saveDir string, log *slog.Logger) {
	const tool = "/usr/bin/sandbox-exec"
	if _, err := exec.LookPath(tool); err != nil {
		log.Warn("sandbox: sandbox-exec is missing; the runner has only its resource limits")
		return
	}
	quoted := strings.ReplaceAll(strings.ReplaceAll(saveDir, `\`, `\\`), `"`, `\"`)
	profile := `(version 1)(allow default)(deny network*)(deny process-exec*)(deny file-write*)` +
		`(allow file-write* (subpath "` + quoted + `"))(allow file-write-data (literal "/dev/null"))`
	cmd.Args = append([]string{tool, "-p", profile, cmd.Path}, cmd.Args[1:]...)
	cmd.Path = tool
}

func lockDown(cmd *exec.Cmd, memoryMB int64, log *slog.Logger) func() { return func() {} }
