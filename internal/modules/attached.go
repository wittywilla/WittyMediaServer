package modules

import (
	"errors"
	"io"
	"runtime"
	"time"

	"github.com/wittywilla/wittymodule"
)

func goos() string { return runtime.GOOS }

// Attached is a helper of its own for one long job, such as an open disc: it keeps its
// state between calls, isn't stopped when idle, and the job's failure doesn't get in
// anyone else's way. It stops when the job closes it, or with the server.
type Attached struct {
	m       *Manager
	id      string
	e       *entry
	p       *proc
	version string
}

// Attach starts nothing yet, and gives a job its own helper for the version in use.
// It returns nil when the built-in copy is in use, which is what the job then uses.
func (m *Manager) Attach(id string, callTimeout time.Duration) *Attached {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil || e.dir == "" || e.man.Delivery != wittymodule.DeliveryHelper || m.closed {
		return nil
	}
	p := m.newProcFor(id, e, e.man, e.dir)
	p.keep, p.timeout = true, callTimeout
	a := &Attached{m: m, id: id, e: e, p: p, version: e.man.Version}
	m.attached[a] = true
	return a
}

// Version is the version this job runs.
func (a *Attached) Version() string { return a.version }

// Call asks the helper one thing. A *wittymodule.CallError is a working helper saying
// no. Any other error means it failed, which counts against the version, and the job
// goes on with its built-in copy; the helper's state is gone with it.
func (a *Attached) Call(method string, params, out any) ([]byte, error) {
	blob, err := a.p.Call(method, params, out)
	var ce *wittymodule.CallError
	if err == nil || errors.As(err, &ce) {
		return blob, err
	}
	a.m.failed(a.id, a.e, a.version, err)
	return nil, err
}

// Stream asks for a stream of bytes: the helper answers one call, and everything it
// writes after that, until it ends or the job closes it, is the stream. The process is
// the stream's own, so a helper keeps no pipe of the server's busy for anyone else.
func (a *Attached) Stream(method string, params, out any) (io.ReadCloser, error) {
	r, err := a.p.Stream(method, params, out)
	if err != nil {
		var ce *wittymodule.CallError
		if !errors.As(err, &ce) {
			a.m.failed(a.id, a.e, a.version, err)
		}
		return nil, err
	}
	return streamCloser{r, a}, nil
}

type streamCloser struct {
	io.Reader
	a *Attached
}

func (s streamCloser) Close() error { s.a.Close(); return nil }

// Close stops the helper.
func (a *Attached) Close() {
	a.m.mu.Lock()
	delete(a.m.attached, a)
	a.m.mu.Unlock()
	a.p.Close()
}
