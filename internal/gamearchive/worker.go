package gamearchive

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/wittywilla/wittyplay/hostsandbox"
)

// The worker is this same program, started again with workerArg. Doing it in init
// means every program that can ask for one can also be one: the server, the tray
// app and the test binaries, with nothing to remember in any of their mains.
//
// It boxes itself in the way the game runner does (no network, no starting programs,
// writing only to the one folder, memory limited), and reads archives there. The
// server only ever sees the lines it prints, and treats those as untrusted too.
const workerArg = "--wms-archive-worker"

const (
	modeList   = "list"
	modeUnpack = "unpack"
	// workerMemoryMB is enough for a 7z made with the largest settings people use.
	workerMemoryMB = 4096
	// listTimeout is per archive: a header read takes milliseconds, even on a slow disk.
	listTimeout = 60 * time.Second
	maxLine     = 32 << 20
)

// ScanBudget is how much of a compressed tar a scan reads through.
const ScanBudget = int64(64) << 20

func init() {
	if len(os.Args) == 3 && os.Args[1] == workerArg {
		os.Exit(runWorker(os.Args[2]))
	}
}

type listRequest struct {
	Path   string `json:"path"`
	Budget int64  `json:"budget,omitempty"`
}

type listReply struct {
	Listing *Listing `json:"listing,omitempty"`
	Code    string   `json:"code,omitempty"`
}

type unpackRequest struct {
	Path  string `json:"path"`
	Dst   string `json:"dst"`
	Want  []Want `json:"want"`
	Limit int64  `json:"limit"`
}

type unpackReply struct {
	Done    *int64    `json:"done,omitempty"`
	Finish  bool      `json:"finish,omitempty"`
	Written []Written `json:"written,omitempty"`
	Code    string    `json:"code,omitempty"`
}

func runWorker(mode string) int {
	in := bufio.NewReaderSize(os.Stdin, 1<<16)
	out := json.NewEncoder(os.Stdout)
	switch mode {
	case modeList:
		// No folder: it writes nothing at all.
		_ = hostsandbox.Apply(hostsandbox.Limits{MemoryMB: workerMemoryMB})
		confine(workerMemoryMB)
		for {
			line, err := in.ReadBytes('\n')
			if err != nil {
				return 0
			}
			var req listRequest
			if json.Unmarshal(line, &req) != nil {
				return 1
			}
			l, err := listFile(req.Path, req.Budget)
			if err != nil {
				_ = out.Encode(listReply{Code: codeFor(err)})
				continue
			}
			_ = out.Encode(listReply{Listing: l})
		}
	case modeUnpack:
		line, err := in.ReadBytes('\n')
		if err != nil {
			return 1
		}
		var req unpackRequest
		if json.Unmarshal(line, &req) != nil {
			return 1
		}
		_ = hostsandbox.Apply(hostsandbox.Limits{SaveDir: req.Dst, MemoryMB: workerMemoryMB})
		confine(workerMemoryMB)
		// The server keeps this pipe open: its end means nobody is waiting any more,
		// on the platforms with no other way to say so.
		go func() {
			_, _ = io.Copy(io.Discard, in)
			os.Exit(1)
		}()
		written, err := unpackFile(req.Path, req.Dst, req.Want, req.Limit, func(n int64) { _ = out.Encode(unpackReply{Done: &n}) })
		if err != nil {
			_ = out.Encode(unpackReply{Code: codeFor(err)})
			return 1
		}
		_ = out.Encode(unpackReply{Finish: true, Written: written})
		return 0
	}
	return 2
}

func codeFor(err error) string {
	if c := CodeOf(err); c != "" {
		return c
	}
	return ErrUnavailable.Code
}

func errorFor(code string) error {
	if e, ok := errorsByCode[code]; ok {
		return e
	}
	return ErrUnavailable
}

func workerCommand(ctx context.Context, mode string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, exe, workerArg, mode)
	cmd.Env = workerEnv()
	cmd.WaitDelay = 2 * time.Second
	hideWindow(cmd)
	return cmd, nil
}

// errWorkerGone counts as damage, since that's the likely reason and it's all the owner
// can act on, but it isn't an answer about the file: it could be a busy machine.
var errWorkerGone = fmt.Errorf("%w (its reader stopped)", ErrDamaged)

// Transient says an error isn't a lasting fact about the archive, and is worth trying
// again at the next scan.
func Transient(err error) bool {
	return errors.Is(err, errWorkerGone) || errors.Is(err, ErrUnavailable)
}

// A Lister reads archive headers one after another through one worker, so a scan of
// thousands costs one process and not thousands. A crash or a hang costs only the
// archive that caused it: that one is reported as damaged, and the next gets a new
// worker. Not for use by several goroutines at once.
type Lister struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	lines   chan []byte
	timeout time.Duration
}

// NewLister starts nothing until the first archive.
func NewLister() *Lister { return &Lister{timeout: listTimeout} }

func (l *Lister) start() error {
	cmd, err := workerCommand(context.Background(), modeList)
	if err != nil {
		return err
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	lines := make(chan []byte, 1)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 0, 1<<16), maxLine)
		for sc.Scan() {
			lines <- append([]byte(nil), sc.Bytes()...)
		}
	}()
	l.cmd, l.in, l.lines = cmd, in, lines
	return nil
}

// Close ends the worker.
func (l *Lister) Close() {
	if l.cmd == nil {
		return
	}
	_ = l.in.Close()
	// Wait may only run once the output has been read to its end.
	t := time.AfterFunc(2*time.Second, func() { _ = l.cmd.Process.Kill() })
	defer t.Stop()
	for range l.lines {
	}
	_ = l.cmd.Wait()
	l.cmd = nil
}

// kill is for a worker that's stopped answering, or has already died.
func (l *Lister) kill() {
	if l.cmd == nil {
		return
	}
	_ = l.cmd.Process.Kill()
	_ = l.in.Close()
	for range l.lines {
	}
	_ = l.cmd.Wait()
	l.cmd = nil
}

// List reads one archive's headers. budget is as for ScanBudget; 0 reads everything.
func (l *Lister) List(path string, budget int64) (*Listing, error) {
	if l.cmd == nil {
		if err := l.start(); err != nil {
			return nil, ErrUnavailable
		}
	}
	b, _ := json.Marshal(listRequest{Path: path, Budget: budget})
	if _, err := l.in.Write(append(b, '\n')); err != nil {
		l.kill()
		return nil, errWorkerGone
	}
	timer := time.NewTimer(l.timeout)
	defer timer.Stop()
	select {
	case line, ok := <-l.lines:
		if !ok {
			l.kill() // it died on this archive
			return nil, errWorkerGone
		}
		var r listReply
		if json.Unmarshal(line, &r) != nil {
			l.kill()
			return nil, errWorkerGone
		}
		if r.Code != "" {
			return nil, errorFor(r.Code)
		}
		if r.Listing == nil {
			return nil, ErrDamaged
		}
		if err := r.Listing.check(); err != nil {
			return nil, err
		}
		return r.Listing, nil
	case <-timer.C:
		l.kill()
		return nil, errWorkerGone
	}
}

// check applies the rules again on this side of the pipe.
func (l *Listing) check() error {
	if len(l.Entries) > MaxEntries {
		return ErrTooMany
	}
	for _, e := range l.Entries {
		if n, err := cleanName(e.Name); err != nil || n != e.Name {
			return ErrUnsafe
		}
	}
	return nil
}

// List reads one archive's headers, all of them, in a worker of its own.
func List(path string) (*Listing, error) {
	l := NewLister()
	defer l.Close()
	l.timeout = 10 * time.Minute // a whole .tar.gz, on the slowest hardware
	return l.List(path, 0)
}

// unpackIn runs one unpack in a worker. Cancelling ctx kills it.
func unpackIn(ctx context.Context, req unpackRequest, progress func(int64)) ([]Written, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd, err := workerCommand(ctx, modeUnpack)
	if err != nil {
		return nil, ErrUnavailable
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, ErrUnavailable
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, ErrUnavailable
	}
	if err := cmd.Start(); err != nil {
		return nil, ErrUnavailable
	}
	var once sync.Once
	closeIn := func() { once.Do(func() { _ = in.Close() }) }
	defer closeIn()
	b, _ := json.Marshal(req)
	if _, err := in.Write(append(b, '\n')); err != nil {
		cancel()
		_ = cmd.Wait()
		return nil, ErrDamaged
	}
	var written []Written
	finished := false
	code := ""
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 1<<16), maxLine)
	for sc.Scan() {
		var r unpackReply
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		switch {
		case r.Done != nil:
			if progress != nil {
				progress(*r.Done)
			}
		case r.Code != "":
			code = r.Code
		case r.Finish:
			finished, written = true, r.Written
		}
	}
	closeIn()
	werr := cmd.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if code != "" {
		return nil, errorFor(code)
	}
	if !finished || werr != nil {
		// It died, most likely asking for more memory than it may have.
		return nil, ErrDamaged
	}
	return written, nil
}
