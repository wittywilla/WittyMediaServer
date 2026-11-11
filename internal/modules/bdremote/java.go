// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package bdremote

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/bdsession"
	"github.com/wittywilla/wittymediaserver/internal/bluray"
)

// A disc's Java programs run beside its navigator in the same helper: the helper starts Java,
// and tells this side what the programs did as they do it. What it says is untrusted like the
// rest: a session already treats what's in an event as coming from a disc.

// javaFrame limits a picture: 1920x1080 as a PNG is far less, and anything more is a mistake.
const maxJavaFrame = 32 << 20

// listener is the part of a helper that can say things unasked.
type listener interface {
	OnNotify(func(method string, params json.RawMessage))
}

// javaHost is Java in a helper, as a session uses it.
type javaHost struct {
	e    *engine
	done chan struct{}
	once sync.Once

	mu      sync.Mutex
	version int
	changes int
	waiters []chan struct{}
	png     []byte
	pngVer  int
}

// StartJava starts Java in the helper. An error means the disc plays from its playlists.
func (e *engine) StartJava(o bdsession.JavaOptions) (bdsession.JavaHost, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return nil, errGone
	}
	l, ok := e.h.(listener)
	if !ok {
		return nil, fmt.Errorf("this helper can't run Java")
	}
	host := &javaHost{e: e, done: make(chan struct{})}
	l.OnNotify(func(method string, params json.RawMessage) { host.heard(method, params, o) })
	if _, err := e.h.Call("javastart", bluray.JavaStart{Java: o.Java, Session: o.Session}, nil); err != nil {
		if !answeredNo(err) {
			e.fail(err)
		}
		return nil, err
	}
	return host, nil
}

// heard takes what the helper said. It runs while the pipe is being read, so it must not
// call the helper: a session's handling happens on its own goroutine, as OnEvent promises.
func (h *javaHost) heard(method string, params json.RawMessage, o bdsession.JavaOptions) {
	switch method {
	case bluray.NotifyJavaEvent:
		var ev bdsession.JavaEvent
		if json.Unmarshal(params, &ev) == nil && o.OnEvent != nil && len(ev.Kind) < 64 && len(ev.Args) < 256 {
			o.OnEvent(ev)
		}
	case bluray.NotifyJavaChange:
		var c bluray.JavaChange
		if json.Unmarshal(params, &c) == nil {
			h.mu.Lock()
			if c.Version > h.version {
				h.version = c.Version
			}
			h.changed()
			h.mu.Unlock()
		}
	case bluray.NotifyJavaLog:
		var line string
		if json.Unmarshal(params, &line) == nil && o.Log != nil {
			if len(line) > 2048 {
				line = line[:2048]
			}
			o.Log(line)
		}
	case bluray.NotifyJavaExit:
		var x bluray.JavaExit
		_ = json.Unmarshal(params, &x)
		if o.OnExit != nil {
			var err error
			if x.Error != "" {
				err = fmt.Errorf("%s", x.Error)
			}
			o.OnExit(err, x.Stopped)
		}
		h.once.Do(func() { close(h.done) })
	}
}

// changed wakes whoever is waiting. Call with h.mu held.
func (h *javaHost) changed() {
	h.changes++
	for _, w := range h.waiters {
		close(w)
	}
	h.waiters = nil
}

// Version is the newest picture's number.
func (h *javaHost) Version() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.version
}

// Kick says something changed other than the picture.
func (h *javaHost) Kick() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.changed()
}

// WaitChange waits for something to change after since, for as long as max.
func (h *javaHost) WaitChange(ctx context.Context, since int, max time.Duration) int {
	t := time.NewTimer(max)
	defer t.Stop()
	for {
		h.mu.Lock()
		if h.changes != since {
			c := h.changes
			h.mu.Unlock()
			return c
		}
		w := make(chan struct{})
		h.waiters = append(h.waiters, w)
		h.mu.Unlock()
		select {
		case <-w:
		case <-ctx.Done():
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.changes
		case <-t.C:
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.changes
		case <-h.done:
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.changes
		}
	}
}

// PNG is the newest picture. Each is asked for once.
func (h *javaHost) PNG() ([]byte, int) {
	h.mu.Lock()
	if h.png != nil && h.pngVer == h.version {
		defer h.mu.Unlock()
		return h.png, h.pngVer
	}
	h.mu.Unlock()
	e := h.e
	e.mu.Lock()
	var b []byte
	var c bluray.JavaChange
	var err error
	if e.h != nil {
		b, err = e.h.Call("javaframe", bluray.Params{}, &c)
	}
	e.mu.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil || len(b) == 0 || len(b) > maxJavaFrame {
		return h.png, h.pngVer
	}
	h.png, h.pngVer = b, c.Version
	if c.Version > h.version {
		h.version = c.Version
	}
	return h.png, h.pngVer
}

// tell says something to the programs. A helper that has gone has nobody to tell.
func (h *javaHost) tell(method string, p any) {
	e := h.e
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.h == nil {
		return
	}
	if _, err := e.h.Call(method, p, nil); err != nil && !answeredNo(err) {
		e.fail(err)
	}
}

// Send passes a line to the programs.
func (h *javaHost) Send(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if strings.ContainsAny(line, "\r\n") {
		return
	}
	h.tell("javasend", bluray.JavaLine{Line: line})
}

// Sync gives the programs the navigator's registers, which the helper has already.
func (h *javaHost) Sync(*bluray.Registers) { h.tell("javasync", bluray.Params{}) }

// StartTitle starts a title's applications.
func (h *javaHost) StartTitle(title int, o *bluray.BDJObject) {
	h.tell("javatitle", bluray.Params{N: title, Name: o.Name})
}

// Done closes when the process has ended.
func (h *javaHost) Done() <-chan struct{} { return h.done }

// Close ends the process.
func (h *javaHost) Close() {
	h.tell("javaclose", bluray.Params{})
	h.once.Do(func() { close(h.done) })
}

// Probe asks a helper of its own whether this machine's Java can run a disc's programs.
func probe(p process, path string) (version, where string, err error) {
	defer p.Close()
	var a bluray.JavaProbe
	if _, err := p.Call("javaprobe", bluray.JavaStart{Java: path}, &a); err != nil {
		return "", "", err
	}
	if a.CantRun != "" {
		return a.Version, a.Path, fmt.Errorf("%w: %s", bdsession.ErrJavaCantRun, a.CantRun)
	}
	return a.Version, a.Path, nil
}

// find asks a helper of its own which Java would be used. That doesn't run it.
func find(p process, path string) (string, error) {
	defer p.Close()
	var a bluray.JavaProbe
	if _, err := p.Call("javafind", bluray.JavaStart{Java: path}, &a); err != nil {
		return "", err
	}
	return a.Path, nil
}
