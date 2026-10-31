package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Child processes (FFmpeg, runners, helpers) say what went wrong on stderr. Tail
// keeps the start and end of it, whatever they print: the start says what the
// program was doing, the end why it stopped, and the middle isn't worth the
// memory.

const (
	tailHead = 2 << 10
	tailEnd  = 14 << 10
)

// Tail is an io.Writer that keeps a bounded part of what is written to it. It
// never fails, so a child is never blocked or killed by its log.
type Tail struct {
	mu    sync.Mutex
	head  []byte
	end   []byte // grows to twice tailEnd, then keeps its last tailEnd
	total int64
}

// Write keeps what it can.
func (t *Tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := len(b)
	t.total += int64(n)
	if room := tailHead - len(t.head); room > 0 {
		if room > len(b) {
			room = len(b)
		}
		t.head = append(t.head, b[:room]...)
		b = b[room:]
	}
	if len(b) >= tailEnd {
		t.end = append(t.end[:0], b[len(b)-tailEnd:]...)
		return n, nil
	}
	t.end = append(t.end, b...)
	if len(t.end) > 2*tailEnd {
		t.end = append(t.end[:0], t.end[len(t.end)-tailEnd:]...)
	}
	return n, nil
}

// String is what was kept, with a note of what wasn't.
func (t *Tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	end := t.end
	if len(end) > tailEnd {
		end = end[len(end)-tailEnd:]
	}
	kept := int64(len(t.head) + len(end))
	var b strings.Builder
	b.Write(t.head)
	if t.total > kept {
		fmt.Fprintf(&b, "\n…[%d bytes cut]…\n", t.total-kept)
	}
	b.Write(end)
	return strings.TrimSpace(b.String())
}

// Last is up to the last n bytes.
func (t *Tail) Last(n int) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	src := t.end
	if len(src) == 0 {
		src = t.head
	}
	if len(src) > n {
		src = src[len(src)-n:]
	}
	return strings.TrimSpace(string(src))
}

// Empty says whether anything was written.
func (t *Tail) Empty() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.total == 0
}

// Windows' ways for a program to die, as exit codes.
var windowsExits = map[uint32]string{
	0xC0000005: "crashed: it read or wrote memory it doesn't own (access violation)",
	0xC000001D: "crashed: this processor doesn't have an instruction it uses",
	0xC0000017: "ran out of memory",
	0xC00000FD: "crashed: stack overflow",
	0xC0000135: "could not start: a DLL it needs is missing",
	0xC0000139: "could not start: a DLL it needs is the wrong version",
	0xC0000142: "could not start: a DLL failed to load",
	0xC000007B: "could not start: a 32-bit and 64-bit mismatch",
	0xC0000409: "crashed: it detected its own memory being damaged",
	0xC000013A: "stopped by Ctrl+C or a window closing",
	0xC0000374: "crashed: heap corruption",
	0xE06D7363: "crashed: an uncaught C++ exception",
}

// ExitReason says in words why a child stopped: its exit code, the signal that
// ended it, or why it couldn't start.
func ExitReason(err error) string {
	if err == nil {
		return "finished normally"
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ProcessState != nil {
		code := ee.ExitCode()
		if ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus); ok {
			if why, ok := waitReason(ws); ok {
				return why
			}
		}
		if code >= 0 {
			if why, ok := windowsExits[uint32(code)]; ok {
				return fmt.Sprintf("%s (exit code 0x%X)", why, uint32(code))
			}
			if uint32(code) > 0x7FFFFFFF {
				return fmt.Sprintf("crashed (exit code 0x%X)", uint32(code))
			}
			return fmt.Sprintf("exit code %d", code)
		}
		return ee.ProcessState.String()
	}
	if errors.Is(err, exec.ErrNotFound) {
		return "the program was not found"
	}
	if errors.Is(err, context.Canceled) {
		return "stopped on purpose"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "stopped: it took too long"
	}
	return err.Error()
}

// ChildExit logs how a child ended, with what it said and how long it ran. The
// logger brings the category and session. intended means the server stopped it
// on purpose, which is worth only a debug line.
func ChildExit(log *slog.Logger, what string, err error, intended bool, stderr *Tail, started time.Time, attrs ...any) {
	if log == nil {
		return
	}
	lv := slog.LevelWarn
	switch {
	case intended:
		lv = slog.LevelDebug
	case err == nil && stderr != nil && !stderr.Empty():
		lv = slog.LevelInfo // it finished, but had something to say: a damaged file often ends like this
	case err == nil:
		lv = slog.LevelDebug
	}
	args := append([]any{"process", what, "reason", ExitReason(err)}, attrs...)
	if !started.IsZero() {
		args = append(args, "ran", time.Since(started).Round(time.Millisecond).String())
	}
	if stderr != nil && !stderr.Empty() {
		args = append(args, "stderr", stderr.String())
	}
	log.Log(context.Background(), lv, "process ended", args...)
}
