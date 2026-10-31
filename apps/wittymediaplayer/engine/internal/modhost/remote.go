package modhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/wittywilla/wittymodule"
)

// callLimit is how long one call may take. A slow drive can take a while over a menu,
// so it's generous; a helper that's truly stuck is ended, and that counts against it.
const callLimit = 2 * time.Minute

// Remote is a running helper of one version of a module.
type Remote struct {
	cmd     *exec.Cmd
	conn    *wittymodule.Conn
	stdin   interface{ Close() error }
	release func()
	Version string
}

// Result is a call's answer, kept as it came.
type Result struct {
	JSON json.RawMessage
	Blob []byte
}

// Start runs a helper, limited as far as the system allows, and checks that it says it's
// the version it's supposed to be.
func Start(entry, id, version string) (*Remote, error) {
	cmd := exec.Command(entry, wittymodule.HelperFlag)
	cmd.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot"), "GOMEMLIMIT=1500MiB"}
	prepareSandbox(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("it wouldn't start: %w", err)
	}
	r := &Remote{cmd: cmd, conn: wittymodule.NewConn(stdout, stdin), stdin: stdin, Version: version, release: lockDown(cmd)}
	type hello struct {
		h   *wittymodule.Hello
		err error
	}
	got := make(chan hello, 1)
	go func() {
		h, err := r.conn.ReadHello()
		got <- hello{h, err}
	}()
	select {
	case res := <-got:
		if res.err != nil {
			r.Stop()
			return nil, fmt.Errorf("it didn't start properly: %w", res.err)
		}
		if res.h.Module != id || res.h.Version != version || res.h.API < wittymodule.MinAPI || res.h.API > wittymodule.API {
			r.Stop()
			return nil, fmt.Errorf("it says it is %s %s", res.h.Module, res.h.Version)
		}
	case <-time.After(30 * time.Second):
		r.Stop()
		return nil, errors.New("it didn't start in time")
	}
	return r, nil
}

// Call asks the helper one thing. A *wittymodule.CallError is a plain no, and anything
// else is the helper failing.
func (r *Remote) Call(method string, params json.RawMessage) (Result, error) {
	type answer struct {
		res Result
		err error
	}
	got := make(chan answer, 1)
	go func() {
		var out json.RawMessage
		var p any
		if len(params) > 0 {
			p = params
		}
		blob, err := r.conn.Call(method, p, &out)
		got <- answer{Result{JSON: out, Blob: blob}, err}
	}()
	select {
	case a := <-got:
		return a.res, a.err
	case <-time.After(callLimit):
		r.Stop()
		return Result{}, errors.New("it stopped answering")
	}
}

// Kill ends the helper at once, as a crash would. For tests.
func (r *Remote) Kill() error { return r.cmd.Process.Kill() }

// Stop ends the helper: closing stdin is how it knows to leave.
func (r *Remote) Stop() {
	_ = r.stdin.Close()
	done := make(chan struct{})
	go func() {
		_ = r.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = r.cmd.Process.Kill()
		<-done
	}
	if r.release != nil {
		r.release()
	}
}
