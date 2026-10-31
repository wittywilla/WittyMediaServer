package stream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittyplay/ipc"
)

// ProbeOptions test-load one core file.
type ProbeOptions struct {
	Runner    string // wittyplay-host: this is a question for our own runner
	Core      string
	SystemDir string
	WorkDir   string        // an empty folder, the only one the runner may write to
	Timeout   time.Duration // 0 means ProbeTimeout
	MemoryMB  int64         // 0 means 2048
	Log       *slog.Logger
}

// ProbeTimeout is how long a core gets to load. A big core on a slow disk takes
// seconds, and a virus scanner looking at a new library on a busy machine was seen
// taking most of a minute. A hang costs only the wait.
const ProbeTimeout = 2 * time.Minute

// ProbeResult is a successful test-load.
type ProbeResult struct {
	ipc.Probe
	SandboxNote string // a sandbox layer this machine couldn't apply, if any
}

// How a test-load can fail.
const (
	ProbeRefused = "refused" // it loaded far enough to say no, or isn't a core
	ProbeCrashed = "crashed"
	ProbeHung    = "hung"
	ProbeInvalid = "invalid" // it wrote something that isn't the protocol
	ProbeRunner  = "runner"  // our side: the runner couldn't start
)

// ProbeError says, in words for people, why a core failed its test-load.
type ProbeError struct {
	Kind   string
	Reason string
}

func (e *ProbeError) Error() string { return e.Reason }

// Probe test-loads a core in a runner process boxed in exactly as a game's is, and
// reports what it says about itself. Nothing the core does can reach this process:
// a crash is an exit code, a hang is a timeout, and anything it writes is read as
// hostile through the same size limits a game's frames get.
func Probe(ctx context.Context, opt ProbeOptions) (*ProbeResult, error) {
	if opt.Timeout <= 0 {
		opt.Timeout = ProbeTimeout
	}
	if opt.MemoryMB <= 0 {
		opt.MemoryMB = 2048
	}
	if opt.Log == nil {
		opt.Log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opt.Runner == "" {
		return nil, &ProbeError{ProbeRunner, "the game runner (wittyplay-host) is missing from this install, so nothing can be tested"}
	}
	if err := os.MkdirAll(opt.WorkDir, 0o700); err != nil {
		return nil, &ProbeError{ProbeRunner, "a folder to test the core in couldn't be made: " + err.Error()}
	}
	cmd := exec.Command(opt.Runner, "-save", opt.WorkDir, "-memory-mb", strconv.FormatInt(opt.MemoryMB, 10))
	cmd.Dir = opt.WorkDir
	cmd.Env = runnerEnv()
	cmd.WaitDelay = 2 * time.Second // a core's stray thread can't hold the pipes open
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, &ProbeError{ProbeRunner, err.Error()}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, &ProbeError{ProbeRunner, err.Error()}
	}
	stderr := &cappedBuffer{left: 1 << 16}
	cmd.Stderr = stderr
	prepareSandbox(cmd, opt.WorkDir, opt.Log)
	probePriority(cmd)
	if err := cmd.Start(); err != nil {
		return nil, &ProbeError{ProbeRunner, "the game runner couldn't start: " + err.Error()}
	}
	release := lockDown(cmd, opt.MemoryMB, opt.Log) // before any core is loaded
	defer release()

	load, _ := json.Marshal(ipc.Load{Core: opt.Core, System: opt.SystemDir, Save: opt.WorkDir})
	answer := make(chan probeAnswer, 1)
	go func() { answer <- readProbe(stdout) }()
	_ = ipc.Write(stdin, ipc.CmdProbe, load)

	timer := time.NewTimer(opt.Timeout)
	defer timer.Stop()
	var got probeAnswer
	select {
	case got = <-answer:
	case <-timer.C:
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if out := stderr.String(); out != "" {
			opt.Log.Info("a core hung in its test-load", "runner", truncate(out, 2000))
		}
		return nil, &ProbeError{ProbeHung, fmt.Sprintf("It didn't finish loading within %d seconds, so it was stopped.", int(opt.Timeout.Seconds()))}
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, &ProbeError{ProbeRunner, "The test was cancelled."}
	}
	_ = stdin.Close()
	if got.err == nil && got.probe != nil {
		// It has said what it had to say, and may take a moment to close.
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		return &ProbeResult{Probe: *got.probe, SandboxNote: sandboxNote(stderr.String())}, nil
	}
	_ = cmd.Process.Kill()
	waitErr := cmd.Wait()
	if out := stderr.String(); out != "" {
		opt.Log.Info("a core failed its test-load", "runner", truncate(out, 2000))
	}
	if got.err != nil {
		return nil, got.err
	}
	return nil, &ProbeError{ProbeCrashed, "It crashed while loading" + exitDetail(waitErr) + ". The server is fine; only the test process stopped."}
}

type probeAnswer struct {
	probe *ipc.Probe
	err   *ProbeError
}

// readProbe waits for the one message that matters. The reader's own limits apply,
// so a core that writes junk to its output fails here rather than confusing anything.
func readProbe(r io.Reader) probeAnswer {
	rd := ipc.NewReader(r)
	for {
		t, p, err := rd.Next()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, os.ErrClosed) {
				return probeAnswer{}
			}
			return probeAnswer{err: &ProbeError{ProbeInvalid, "It wrote something the runner's protocol doesn't allow, so it was stopped."}}
		}
		switch t {
		case ipc.MsgProbe:
			var out ipc.Probe
			if err := json.Unmarshal(p, &out); err != nil {
				return probeAnswer{err: &ProbeError{ProbeInvalid, "Its answer couldn't be read, so it was stopped."}}
			}
			return probeAnswer{probe: &out}
		case ipc.MsgError:
			return probeAnswer{err: &ProbeError{ProbeRefused, explainLoadError(string(p))}}
		}
	}
}

// explainLoadError turns what a system loader says into what it means here.
func explainLoadError(msg string) string {
	if len(msg) > 400 {
		msg = msg[:400]
	}
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "is not a libretro core"):
		return "This isn't a libretro core: " + msg
	case strings.Contains(lower, "api version"):
		return "It's built for a libretro version this server doesn't speak: " + msg
	case strings.Contains(lower, "not a valid win32"), strings.Contains(lower, "wrong elf class"), strings.Contains(lower, "invalid elf header"),
		strings.Contains(lower, "mach-o"), strings.Contains(lower, "cannot open shared object file: no such file") && strings.Contains(lower, "opening core"):
		return "It couldn't be opened on this server. It may be built for another kind of computer, or it may need a library this server doesn't have: " + msg
	case strings.Contains(lower, "specified module could not be found"), strings.Contains(lower, "cannot open shared object"):
		return "It needs another library that this server doesn't have: " + msg
	case strings.Contains(lower, "failed to map segment"), strings.Contains(lower, "operation not permitted"):
		return "The system wouldn't let it load. The data folder may be on a drive that doesn't allow running programs: " + msg
	}
	return "It couldn't be loaded: " + msg
}

// exitDetail names the way a process ended, tidily. Windows reports a crash as a
// status code that only makes sense in hex.
func exitDetail(err error) string {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return ""
	}
	if code := ee.ExitCode(); code > 0xffff {
		return fmt.Sprintf(" (status 0x%x)", uint32(code))
	}
	return " (" + ee.String() + ")"
}

// sandboxNote finds the line wittyplay-host writes when a layer couldn't apply.
func sandboxNote(stderr string) string {
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "sandbox: ") {
			if len(line) > 300 {
				line = line[:300]
			}
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// cappedBuffer keeps the start of what a process writes and drops the rest.
type cappedBuffer struct {
	b    []byte
	left int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if n := min(len(p), c.left); n > 0 {
		c.b = append(c.b, p[:n]...)
		c.left -= n
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return string(c.b) }

// Failure says why a game ended by itself, or "" when it was simply stopped.
func (s *Session) Failure() string {
	if v := s.exitErr.Load(); v != nil {
		return v.(string)
	}
	return ""
}

// UsingCore counts the games running with this core file. On Windows a loaded
// file can't be deleted, and nobody wants a game pulled out from under them.
func (m *Manager) UsingCore(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, s := range m.sessions {
		if s.opt.Core == path {
			n++
		}
	}
	return n
}
