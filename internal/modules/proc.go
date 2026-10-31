package modules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymodule"
)

// Limits on a helper. The call limit is generous because a call may read a large
// file from a slow network share.
const (
	helloTimeout = 20 * time.Second
	idleTimeout  = time.Minute
	softMemory   = "256MiB"
	hardMemoryMB = 512
)

// callTimeout is a var so tests can shorten it.
var callTimeout = 60 * time.Second

// errUntrusted: a helper's files no longer match their signature.
var errUntrusted = errors.New("files have changed since they were installed")

// proc runs one version of a helper: started on first use, stopped when idle,
// restarted after a failure. It makes one call at a time.
type proc struct {
	man    *wittymodule.Manifest
	dir    string
	verify func() error // checked before every start; nil only for tests
	log    *slog.Logger
	logEnv func(id string) []string // the owner's log settings, for the helper

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	conn    *wittymodule.Conn
	release func() // what the sandbox holds for a running helper
	idle    *time.Timer
	stderr  *logging.Tail
	started time.Time
	closed  bool
}

func newProc(man *wittymodule.Manifest, dir string, verify func() error, log *slog.Logger) *proc {
	return &proc{man: man, dir: dir, verify: verify, log: log.With("cat", "modules", "module", man.ID)}
}

func (p *proc) path() string { return filepath.Join(p.dir, p.man.EntryFile(runtime.GOOS)) }

// errStderr is how much of a helper's stderr goes in an error. The log gets more.
const errStderr = 2048

// maxOutputLine is the most of one line of a helper's stderr the log takes.
const maxOutputLine = 2048

// helperOutput is where a helper's stderr goes: always the bounded Tail, and line
// by line to the log only while someone has asked for that.
type helperOutput struct {
	tail *logging.Tail
	log  *slog.Logger
	line []byte
}

func (o *helperOutput) Write(b []byte) (int, error) {
	n, _ := o.tail.Write(b)
	if !o.log.Enabled(context.Background(), slog.LevelDebug) {
		o.line = o.line[:0]
		return n, nil
	}
	for len(b) > 0 {
		end := bytes.IndexByte(b, '\n')
		part := b
		if end >= 0 {
			part = b[:end]
		}
		if room := maxOutputLine - len(o.line); room < len(part) {
			part = part[:room]
		}
		o.line = append(o.line, part...)
		if end < 0 {
			break
		}
		o.flush()
		b = b[end+1:]
	}
	return n, nil
}

func (o *helperOutput) flush() {
	if line := bytes.TrimRight(o.line, "\r"); len(line) > 0 {
		o.log.Debug("helper says", "topic", "output", "output", string(line))
	}
	o.line = o.line[:0]
}

func (p *proc) env() []string {
	env := []string{"GOMEMLIMIT=" + softMemory, "GOMAXPROCS=2"}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	if p.logEnv != nil {
		env = append(env, p.logEnv(p.man.ID)...)
	}
	return env
}

func (p *proc) newCmd() *exec.Cmd {
	cmd := exec.Command(p.path(), wittymodule.HelperFlag)
	cmd.Dir = p.dir
	cmd.Env = p.env()
	return cmd
}

// start runs the helper and checks that it is what its manifest says. Call with
// p.mu held.
func (p *proc) start() error {
	if p.closed {
		return errors.New("this version is no longer in use")
	}
	if p.verify != nil {
		if err := p.verify(); err != nil {
			return err
		}
	}
	cmd := p.newCmd()
	sandboxed := prepareSandbox(cmd, p.log)
	stdin, stdout, stderr, err := pipes(cmd)
	if err != nil {
		return err
	}
	err = cmd.Start()
	if err != nil && sandboxed {
		// Containers and some systems refuse what the sandbox asks for. The
		// helper is signed, and its other limits still apply.
		p.log.Info("modules: starting the helper without its sandbox", "err", err)
		cmd = p.newCmd()
		hideWindow(cmd)
		if stdin, stdout, stderr, err = pipes(cmd); err != nil {
			return err
		}
		err = cmd.Start()
	}
	if err != nil {
		return fmt.Errorf("could not start: %w", err)
	}
	started := time.Now()
	release := lockDown(cmd, hardMemoryMB, p.log)
	t := &logging.Tail{}
	logging.Go("module stderr", func() {
		o := &helperOutput{tail: t, log: p.log}
		_, _ = io.Copy(o, stderr)
		o.flush()
	}, "cat", "modules")
	conn := wittymodule.NewConn(stdout, stdin)

	stop := time.AfterFunc(helloTimeout, func() { _ = cmd.Process.Kill() })
	hello, err := conn.ReadHello()
	stop.Stop()
	if err == nil {
		switch {
		case hello.Module != p.man.ID:
			err = fmt.Errorf("it says it is %q, not %s", hello.Module, p.man.ID)
		case hello.Version != p.man.Version:
			err = fmt.Errorf("it says it is version %q, not %s", hello.Version, p.man.Version)
		case hello.API != p.man.API:
			err = fmt.Errorf("it says it uses module API %d, not %d", hello.API, p.man.API)
		}
	}
	if err != nil {
		stdin.Close()
		_ = cmd.Process.Kill()
		logging.ChildExit(p.log, "module helper", cmd.Wait(), false, t, started, "topic", "lifecycle", "why", "failed to start", "version", p.man.Version)
		release()
		return fmt.Errorf("%w %s", err, t.Last(errStderr))
	}
	p.cmd, p.stdin, p.conn, p.release, p.stderr, p.started = cmd, stdin, conn, release, t, started
	p.log.Debug("helper started", "topic", "lifecycle", "version", p.man.Version, "sandboxed", sandboxed)
	return nil
}

func pipes(cmd *exec.Cmd) (io.WriteCloser, io.ReadCloser, io.ReadCloser, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	return stdin, stdout, stderr, nil
}

// halt ends the helper, for the reason given. Closing stdin asks it to leave; it
// has a moment before being killed. Call with p.mu held.
func (p *proc) halt(why string) {
	if p.idle != nil {
		p.idle.Stop()
		p.idle = nil
	}
	if p.cmd == nil {
		return
	}
	cmd := p.cmd
	p.stdin.Close()
	done := make(chan struct{})
	var werr error
	logging.Go("module wait", func() { werr = cmd.Wait(); close(done) }, "cat", "modules")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	logging.ChildExit(p.log, "module helper", werr, false, p.stderr, p.started, "topic", "lifecycle", "why", why, "version", p.man.Version)
	p.release()
	p.cmd, p.stdin, p.conn, p.release, p.stderr = nil, nil, nil, nil, nil
}

// Call asks the helper one thing. A *wittymodule.CallError is a working helper
// saying no; any other error means it failed, and it is restarted on the next
// call.
func (p *proc) Call(method string, params, out any) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		if err := p.start(); err != nil {
			return nil, err
		}
	}
	cmd := p.cmd
	kill := time.AfterFunc(callTimeout, func() { _ = cmd.Process.Kill() })
	began := time.Now()
	blob, err := p.conn.Call(method, params, out)
	kill.Stop()
	if p.log.Enabled(context.Background(), slog.LevelDebug) {
		p.log.Debug("module call", "topic", "calls", "method", method, "took", time.Since(began).String(), "err", err)
	}
	var ce *wittymodule.CallError
	if err != nil && !errors.As(err, &ce) {
		msg := p.stderr.Last(errStderr)
		p.log.Warn("module call failed", "version", p.man.Version, "method", method, "err", err)
		_ = cmd.Process.Kill()
		p.halt("failed") // which logs how it ended
		if msg != "" {
			return nil, fmt.Errorf("%w (%s)", err, msg)
		}
		return nil, err
	}
	p.touch()
	return blob, err
}

// touch restarts the idle clock. Call with p.mu held.
func (p *proc) touch() {
	if p.idle != nil {
		p.idle.Stop()
	}
	p.idle = time.AfterFunc(idleTimeout, func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.halt("idle")
	})
}

// Try starts the helper once and stops it: the last check before a version is
// put to use.
func (p *proc) Try() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.start(); err != nil {
		return err
	}
	p.halt("tried")
	return nil
}

// Close stops the helper for good.
func (p *proc) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.halt("closed")
}
