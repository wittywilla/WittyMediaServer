package modules

import (
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

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	conn    *wittymodule.Conn
	release func() // what the sandbox holds for a running helper
	idle    *time.Timer
	stderr  *tail
	closed  bool
}

func newProc(man *wittymodule.Manifest, dir string, verify func() error, log *slog.Logger) *proc {
	return &proc{man: man, dir: dir, verify: verify, log: log}
}

func (p *proc) path() string { return filepath.Join(p.dir, p.man.EntryFile(runtime.GOOS)) }

// tail keeps the end of what a helper wrote to stderr, for the log.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > 2048 {
		t.buf = append(t.buf[:0], t.buf[len(t.buf)-2048:]...)
	}
	return len(b), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

func (p *proc) env() []string {
	env := []string{"GOMEMLIMIT=" + softMemory, "GOMAXPROCS=2"}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"))
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
		p.log.Info("modules: starting the helper without its sandbox", "module", p.man.ID, "err", err)
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
	release := lockDown(cmd, hardMemoryMB, p.log)
	t := &tail{}
	go func() { _, _ = io.Copy(t, stderr) }()
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
		_ = cmd.Wait()
		release()
		return fmt.Errorf("%w %s", err, t.String())
	}
	p.cmd, p.stdin, p.conn, p.release, p.stderr = cmd, stdin, conn, release, t
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

// halt ends the helper. Closing stdin asks it to leave; it has a moment before
// being killed. Call with p.mu held.
func (p *proc) halt() {
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
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
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
	blob, err := p.conn.Call(method, params, out)
	kill.Stop()
	var ce *wittymodule.CallError
	if err != nil && !errors.As(err, &ce) {
		msg := p.stderr.String()
		_ = cmd.Process.Kill()
		p.halt()
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
		p.halt()
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
	p.halt()
	return nil
}

// Close stops the helper for good.
func (p *proc) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.halt()
}
