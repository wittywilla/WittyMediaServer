package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Every call the server makes to somewhere else (lyrics, metadata, logos, updates, the
// module store) goes through Calls, which logs how it went and how long it took, and
// keeps a list of the calls still waiting, for a hang's report. Only the host and the
// path are written: a query can hold a key, and what was asked for is in the caller's
// own lines.

// Thresholds for the first answer (the headers): a slow one is worth a line at info, a
// very slow one or a failure at warn. How long the body then takes is reported, and
// judges nothing: a radio stream is meant to last.
var (
	callSlow     = 2 * time.Second
	callVerySlow = 10 * time.Second
)

// inflight is what's being waited for right now.
var inflight struct {
	sync.Mutex
	next  uint64
	calls map[uint64]inflightCall
}

type inflightCall struct {
	kind, what string
	started    time.Time
}

// Begin notes that something slow has started: an outgoing call, a child process. The
// function it returns says it's over. Cheap, and safe to call from anywhere.
func Begin(kind, what string) (end func()) {
	inflight.Lock()
	if inflight.calls == nil {
		inflight.calls = map[uint64]inflightCall{}
	}
	inflight.next++
	id := inflight.next
	if len(inflight.calls) < 4096 {
		inflight.calls[id] = inflightCall{kind, what, time.Now()}
	}
	inflight.Unlock()
	var once atomic.Bool
	return func() {
		if once.CompareAndSwap(false, true) {
			inflight.Lock()
			delete(inflight.calls, id)
			inflight.Unlock()
		}
	}
}

// InFlight lists what's being waited for, longest first.
func InFlight() string {
	inflight.Lock()
	list := make([]inflightCall, 0, len(inflight.calls))
	for _, c := range inflight.calls {
		list = append(list, c)
	}
	inflight.Unlock()
	if len(list) == 0 {
		return "Waiting on nothing outside the server.\n"
	}
	sort.Slice(list, func(i, j int) bool { return list[i].started.Before(list[j].started) })
	var b strings.Builder
	fmt.Fprintf(&b, "Waiting on %d things outside the server, longest first:\n", len(list))
	for i, c := range list {
		if i == 100 {
			fmt.Fprintf(&b, "  …and %d more\n", len(list)-i)
			break
		}
		fmt.Fprintf(&b, "  %s %s, for %s\n", c.kind, c.what, time.Since(c.started).Round(time.Millisecond))
	}
	return b.String()
}

// Calls wraps a client's transport (nil for the default) so that every call through it
// is logged under cat with source saying whose it is ("lyrics", "game metadata").
func Calls(next http.RoundTripper, cat, source string) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	if c, ok := next.(*calls); ok {
		next = c.next
	}
	return &calls{next: next, cat: NormalizeCat(cat), source: source}
}

type calls struct {
	next        http.RoundTripper
	cat, source string
}

// Unwrap is the transport underneath, for tests and for whoever needs its settings.
func (c *calls) Unwrap() http.RoundTripper { return c.next }

func (c *calls) RoundTrip(req *http.Request) (*http.Response, error) {
	what := req.Method + " " + req.URL.Host + req.URL.EscapedPath()
	end := Begin("call", c.source+": "+what)
	started := time.Now()
	res, err := c.next.RoundTrip(req)
	if err != nil {
		end()
		took := time.Since(started)
		c.log(req.Context(), what, 0, -1, took, took, err)
		return nil, err
	}
	answered := time.Since(started)
	// The call is over when its body has been read or closed.
	res.Body = &callBody{ReadCloser: res.Body, done: func(n int64, rerr error) {
		end()
		c.log(req.Context(), what, res.StatusCode, n, answered, time.Since(started), rerr)
	}}
	return res, nil
}

func (c *calls) log(ctx context.Context, what string, status int, n int64, answered, took time.Duration, err error) {
	log := slog.Default()
	lv := slog.LevelDebug
	switch {
	case err != nil && err != io.EOF && ctx.Err() == nil, status >= 500, answered >= callVerySlow:
		lv = slog.LevelWarn
	case answered >= callSlow:
		lv = slog.LevelInfo
	}
	if !log.Enabled(ctx, lv) {
		return
	}
	args := []any{"cat", c.cat, "source", c.source, "call", what, "answeredMs", answered.Milliseconds(), "ms", took.Milliseconds()}
	if status != 0 {
		args = append(args, "status", status)
	}
	if n >= 0 {
		args = append(args, "bytes", n)
	}
	if err != nil && err != io.EOF {
		if ctx.Err() != nil {
			args = append(args, "stopped", "the caller gave up")
		} else {
			args = append(args, "err", err.Error())
		}
	}
	log.Log(ctx, lv, "outgoing call", args...)
}

// callBody reports once, when the body is finished with.
type callBody struct {
	io.ReadCloser
	n    int64
	err  error
	once sync.Once
	done func(int64, error)
}

func (b *callBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.n += int64(n)
	if err != nil {
		b.err = err
		if err == io.EOF {
			b.once.Do(func() { b.done(b.n, nil) })
		}
	}
	return n, err
}

func (b *callBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.done(b.n, b.err) })
	return err
}
