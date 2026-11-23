// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// A module's service that dies in the middle of a page's call is started again the whole way,
// with where its host API is: before, the call path started a bare process that answered every
// source with "this server has no host API".
func TestAServiceKilledMidCallIsStartedTheWholeWayAgain(t *testing.T) {
	w := newWorld(t)
	w.publishWith("0.4.0", manifestJSON("0.4.0", "0.4.0", "0.5.x", API, "service", "slow-items"), map[string][]byte{sdui.ModuleFile: []byte(photosUI)})
	m := w.manager()
	defer m.Close()
	m.SetHost("http://127.0.0.1:9")
	check(t, m)
	installWait(t, m)
	req := ItemsRequest{Source: map[string]any{"type": "wittyphotos.picks"}, Limit: 12}
	ask := func() (string, error) {
		got, err := m.UIItems("wittyphotos", req)
		if err != nil || len(got.Items) == 0 {
			return "", err
		}
		return got.Items[0].Title, nil
	}
	if title, err := ask(); err != nil || !strings.HasPrefix(title, "host http://127.0.0.1:9") {
		t.Fatalf("first: %q %v", title, err)
	}
	// Killed while it answers.
	m.svcMu.Lock()
	s := m.services["wittyphotos"]
	m.svcMu.Unlock()
	if s == nil {
		t.Fatal("no service running")
	}
	s.a.p.mu.Lock()
	proc := s.a.p.cmd.Process
	s.a.p.mu.Unlock()
	time.AfterFunc(300*time.Millisecond, func() { _ = proc.Kill() })
	if _, err := ask(); err == nil {
		t.Fatal("a call whose helper was killed under it should fail")
	}
	// The next one starts it again, the whole way.
	for i := range 3 {
		title, err := ask()
		if err != nil || !strings.HasPrefix(title, "host http://127.0.0.1:9") {
			t.Fatalf("after the restart (%d): %q %v", i, title, err)
		}
	}
}

// A service whose process has gone is not started again bare by a call or by the check that it's
// there: errHelperGone, and the owner starts it.
func TestAGoneServiceHelperIsNotStartedBare(t *testing.T) {
	p := &proc{once: true, ran: true}
	if _, err := p.Call("sdui.items", nil, nil); err != errHelperGone {
		t.Fatalf("a call: %v", err)
	}
	if _, err := p.Stream("stream", nil, nil); err != errHelperGone {
		t.Fatalf("a stream: %v", err)
	}
}

// A helper that dies says why on stderr, and that reaches the log, however it ended: before,
// Wait could close the pipe before its last lines were read.
func TestAHelpersLastWordsAreLogged(t *testing.T) {
	logs := &syncBuffer{}
	w := newWorld(t)
	w.logs = logs
	w.publish("0.2.0", manifestJSON("0.2.0", "0.4.0", "0.5.x", MinAPI, "say-and-crash"))
	m := w.manager()
	defer m.Close()
	check(t, m)
	installWait(t, m)
	a := m.Attach("wittyphotos", 0)
	if a == nil {
		t.Fatal("no helper")
	}
	defer a.Close()
	_, err := a.Call("read", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "title table is shorter") {
		t.Errorf("the call's error doesn't say why: %v", err)
	}
	if got := logs.String(); !strings.Contains(got, "title table is shorter") || !strings.Contains(got, "process ended") {
		t.Errorf("the log doesn't say why it ended:\n%s", got)
	}
}
