package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymodule"
)

// Limits on a helper. The call limit is generous because a call may read a large
// file from a slow network share.
const (
	helloTimeout = 45 * time.Second // the first run of a new program can be held up by a virus scanner
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
	man      *wittymodule.Manifest
	dir      string
	verify   func() error // checked before every start; nil only for tests
	log      *slog.Logger
	logEnv   func(id string) []string // the owner's log settings, for the helper
	network  bool                     // it may have a network; see mayHave
	programs bool                     // it may start other programs
	devices  bool                     // it may open this machine's devices
	dataDir  string                   // a folder of its own it may write to; "" for none
	writable []string                 // other folders it may write to
	strict   bool                     // refuse to run without its sandbox: a fork, whose only limits these are
	launch   string                   // the program that applies limits before it runs; "" for none
	keep     bool                     // not stopped when idle: it holds state for a job
	// background: a kept helper that reports tasks between calls, and so can't stream.
	background bool
	timeout    time.Duration // for one call; 0 is callTimeout

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.Closer
	conn    *wittymodule.Conn
	release func() // what the sandbox holds for a running helper
	idle    *time.Timer
	stderr  *logging.Tail
	started time.Time
	closed  bool

	notify    func(method string, params json.RawMessage) // hears what the helper says unasked
	listening bool
	reporter  *tasks.Reporter // its task reports; see tasks.go
}

func newProc(man *wittymodule.Manifest, dir string, verify func() error, log *slog.Logger) *proc {
	return &proc{man: man, dir: dir, verify: verify, log: log.With("cat", "modules", "module", man.ID)}
}

// setNotify says who hears what a helper says unasked. The helper already running keeps
// reading from the same pipe, so this works for a process in use as well.
func (p *proc) setNotify(fn func(method string, params json.RawMessage)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.notify = fn
	if p.conn != nil && !p.listening {
		tasksHeard := p.conn.OnNotify
		p.conn.OnNotify = func(method string, params json.RawMessage) {
			if tasksHeard != nil {
				tasksHeard(method, params)
			}
			fn(method, params)
		}
		if !p.background {
			p.conn.Listen()
		}
		p.listening = true
	}
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
	if p.programs {
		// What it starts does the heavy work, and has its own limits.
		env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "TEMP=" + os.TempDir(), "TMP=" + os.TempDir(), "TMPDIR=" + os.TempDir(), "LANG=" + os.Getenv("LANG"),
			"XDG_RUNTIME_DIR=" + os.Getenv("XDG_RUNTIME_DIR"), "DISPLAY=" + os.Getenv("DISPLAY"), "LD_LIBRARY_PATH=" + os.Getenv("LD_LIBRARY_PATH"), "USERPROFILE=" + os.Getenv("USERPROFILE"), "LOCALAPPDATA=" + os.Getenv("LOCALAPPDATA"), "ProgramFiles=" + os.Getenv("ProgramFiles"), "ProgramFiles(x86)=" + os.Getenv("ProgramFiles(x86)"), "JAVA_HOME=" + os.Getenv("JAVA_HOME")}
	}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	if p.dataDir != "" {
		env = append(env, "WITTYMODULE_DATA="+p.dataDir)
	}
	if len(p.writable) > 0 {
		env = append(env, SandboxWriteEnv+"="+strings.Join(p.writable, string(os.PathListSeparator)))
	}
	if p.logEnv != nil {
		env = append(env, p.logEnv(p.man.ID)...)
	}
	return env
}

func (p *proc) newCmd() *exec.Cmd {
	// Much more address space than the job's limit on Windows, which counts only memory
	// in use: Go sets aside over a gigabyte that it never touches, and a helper at 1 GiB
	// failed to start about one time in six on a busy machine. GOMEMLIMIT keeps real use
	// to softMemory.
	mode := 0
	if p.network {
		mode |= SandboxNetwork
	}
	if p.strict {
		mode |= SandboxStrict
	}
	if p.devices {
		mode |= SandboxDevices
	}
	if p.dataDir != "" {
		mode |= SandboxWriteData
	}
	if p.programs {
		mode |= SandboxPrograms
	}
	memory := int64(4 * hardMemoryMB)
	if p.programs {
		memory = 0 // what it starts counts towards this, and a game can use far more
	}
	prog, args := launchWith(p.launch, p.path(), memory, mode, []string{wittymodule.HelperFlag})
	cmd := exec.Command(prog, args...)
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
	if p.strict && runtime.GOOS == "linux" && p.launch == "" {
		return errors.New("this server can't apply the limits a module from another publisher needs")
	}
	cmd := p.newCmd()
	if p.dataDir != "" {
		if err := os.MkdirAll(p.dataDir, 0o700); err != nil {
			return err
		}
	}
	// Namespaces of its own and a low-integrity token would take away what programs it starts
	// need, such as a graphics card, or a folder to write to. The rest of its limits apply.
	sandboxed := false
	if p.programs || p.devices || p.dataDir != "" {
		hideWindow(cmd)
	} else {
		sandboxed = prepareSandbox(cmd, p.network, p.log)
	}
	if p.strict && sandboxAvailable && !sandboxed {
		return errors.New("this system won't run it with the limits a module from another publisher needs")
	}
	stdin, stdout, stderr, err := pipes(cmd)
	if err != nil {
		return err
	}
	err = cmd.Start()
	if err != nil && sandboxed && p.strict {
		return fmt.Errorf("this system won't run it with the limits a module from another publisher needs: %w", err)
	}
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
	memory := int64(hardMemoryMB)
	if p.programs {
		memory = 0
	}
	release := lockDown(cmd, memory, p.programs, p.log)
	t := &logging.Tail{}
	logging.Go("module stderr", func() {
		o := &helperOutput{tail: t, log: p.log}
		_, _ = io.Copy(o, stderr)
		o.flush()
	}, "cat", "modules")
	conn := wittymodule.NewConn(stdout, stdin)
	p.listening = false

	// Closing our end too: killing a helper doesn't end a child of its that still holds
	// the pipe, and a read on it would never return.
	stop := time.AfterFunc(helloTimeout, func() { _ = cmd.Process.Kill(); _ = stdout.Close() })
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
		if launcherMayFail && p.launch != "" && !p.strict {
			// This way of starting one has never been run on this system. Better a working helper
			// and a page that says it isn't confined than no modules at all.
			p.log.Warn("modules: a helper wouldn't start in its sandbox, so it's starting without one", "version", p.man.Version, "err", err, "said", t.Last(errStderr))
			launcherFailed.Store(true)
			p.launch = ""
			return p.start()
		}
		return fmt.Errorf("%w %s", err, t.Last(errStderr))
	}
	p.hearTasks(conn)
	p.cmd, p.stdin, p.stdout, p.conn, p.release, p.stderr, p.started = cmd, stdin, stdout, conn, release, t, started
	if p.notify != nil {
		// Beside the tasks it reports: what it says goes to each, and they know their own.
		tasksHeard, heard := conn.OnNotify, p.notify
		conn.OnNotify = func(method string, params json.RawMessage) {
			if tasksHeard != nil {
				tasksHeard(method, params)
			}
			heard(method, params)
		}
		if !p.background {
			conn.Listen()
		}
		p.listening = true
	}
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
	p.endTasks(why)
	p.release()
	p.cmd, p.stdin, p.stdout, p.conn, p.release, p.stderr = nil, nil, nil, nil, nil, nil
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
	limit := callTimeout
	if p.timeout > 0 {
		limit = p.timeout
	}
	stdout := p.stdout
	kill := time.AfterFunc(limit, func() { _ = cmd.Process.Kill(); _ = stdout.Close() })
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

// Stream asks for a stream: one answer, and then the helper's output is the stream.
// The process is good for nothing else afterwards, so the job closes it to end it.
func (p *proc) Stream(method string, params, out any) (io.Reader, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == nil {
		if err := p.start(); err != nil {
			return nil, err
		}
	}
	cmd := p.cmd
	limit := callTimeout
	if p.timeout > 0 {
		limit = p.timeout
	}
	stdout := p.stdout
	kill := time.AfterFunc(limit, func() { _ = cmd.Process.Kill(); _ = stdout.Close() })
	r, err := p.conn.Stream(method, params, out)
	kill.Stop()
	var ce *wittymodule.CallError
	if err != nil && !errors.As(err, &ce) {
		_ = cmd.Process.Kill()
		p.halt("failed")
	}
	return r, err
}

// touch restarts the idle clock. Call with p.mu held.
func (p *proc) touch() {
	if p.keep {
		return
	}
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
