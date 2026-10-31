package stream

import (
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strconv"
)

// Runner is a started runner process, boxed in the way every game's is. Nothing
// has been loaded yet: it waits for a load command on Stdin.
type Runner struct {
	Cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
	box    func()
}

// StartRunner starts one runner with its sandbox on. Sessions start theirs here,
// and so does cmd/wittyplay-check, so what is tested is what the server runs.
func StartRunner(program string, args []string, saveDir string, memoryMB int64, log *slog.Logger) (*Runner, error) {
	all := append(append([]string(nil), args...), "-save", saveDir, "-memory-mb", strconv.FormatInt(memoryMB, 10))
	cmd := exec.Command(program, all...)
	cmd.Dir = saveDir
	cmd.Env = runnerEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	prepareSandbox(cmd, saveDir, log)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting the game runner: %w", err)
	}
	return &Runner{Cmd: cmd, Stdin: stdin, Stdout: stdout, Stderr: stderr, box: lockDown(cmd, memoryMB, log)}, nil
}

// Release frees the sandbox's resources once the runner has gone.
func (r *Runner) Release() {
	if r.box != nil {
		r.box()
	}
}
