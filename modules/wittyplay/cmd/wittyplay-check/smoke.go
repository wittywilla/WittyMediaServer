package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wittywilla/wittyplay/ipc"
)

// smokeCheck is for a core with no game to try. It runs the core in the sandboxed
// runner on a small file of junk: that opens the library, runs retro_init and
// makes the core say what it thinks of the file. A core that refuses it cleanly,
// or takes it, has passed. A crash or a failure to open has not.
func smokeCheck(c *config, j job) (r Result) {
	r = Result{Console: j.console.ID, Core: j.core.id, Outcome: OutcomeOK}
	began := time.Now()
	defer func() { r.Seconds = time.Since(began).Seconds() }()
	work, err := os.MkdirTemp(c.out, "smoke-")
	if err != nil {
		r.Outcome, r.Reason = OutcomeToolError, err.Error()
		return r
	}
	defer removeAll(work)
	ext := ".bin"
	if len(j.console.Extensions) > 0 {
		ext = j.console.Extensions[0]
	}
	junk := filepath.Join(work, "junk"+ext)
	if err := os.WriteFile(junk, make([]byte, 64<<10), 0o644); err != nil {
		r.Outcome, r.Reason = OutcomeToolError, err.Error()
		return r
	}
	saveDir := filepath.Join(work, "save")
	p, err := start(c, saveDir, j.core.id)
	if err != nil {
		r.Outcome, r.Reason = OutcomeToolError, err.Error()
		return r
	}
	load, _ := json.Marshal(ipc.Load{Core: j.core.path, ROM: junk, System: c.systemDir, Save: saveDir})
	_ = p.send(ipc.CmdLoad, load)
	select {
	case err := <-p.readyCh:
		if err == nil { // it took the junk; ask it to leave
			_ = p.send(ipc.CmdQuit, []byte{0})
			r.Reason = "the core opened and accepted a junk file"
		}
	case <-time.After(c.loadTimeout):
		r.Outcome, r.Reason = OutcomeHang, "the core did not answer"
	}
	if !p.waitExit(10 * time.Second) {
		p.kill()
	}
	reason := p.crashReason()
	switch {
	case r.Outcome != OutcomeOK:
	case strings.Contains(reason, "opening core"), strings.Contains(reason, "not a libretro core"), strings.Contains(reason, "API version"):
		r.Outcome, r.Reason = OutcomeLoadFailed, reason
	case strings.HasPrefix(reason, "the core crashed"), strings.HasPrefix(reason, "the runner panicked"):
		r.Outcome, r.Reason = OutcomeCrash, reason
	case r.Reason == "":
		r.Reason = "the core opened and refused a junk file: " + reason
	}
	p.fill(&r)
	return r
}
