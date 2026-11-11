// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package relay

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os/exec"
	"time"

	"github.com/wittywilla/wittymodule/service"
)

// Written by hand: what holds functions or channels, or lives in the module and is reached by name.

const module = "wittycouch"

// Options start a stream.
type Options struct {
	Program  string
	Prepare  func(*exec.Cmd) `json:"-"` // the module prepares what it starts
	Start    Start
	Log      *slog.Logger `json:"-"`
	STUN     []string
	PublicIP []string
	UDPPort  int // one port for every stream, when set, for routers and containers
	// OnEnd is told once, when the stream has ended for any reason.
	OnEnd func(reason string) `json:"-"`
}

// Session is a stream, in the module.
type Session struct {
	Sid string `json:"id"`

	ended chan struct{}
}

// New starts a stream.
func New(ctx context.Context, opts Options) (*Session, error) {
	var s *Session
	if err := service.Call(ctx, module, "relay.New", nil, nil, []any{opts}, []any{&s}); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("the stream didn't start")
	}
	s.ended = make(chan struct{})
	go s.wait(opts.OnEnd)
	return s, nil
}

// wait asks until the stream has ended. The service going away ends it too.
func (s *Session) wait(onEnd func(string)) {
	reason := ""
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		var a struct {
			Over   bool   `json:"over"`
			Reason string `json:"reason"`
		}
		err := service.Call(ctx, module, "relay.Session.Wait", nil, nil, []any{s.Sid}, []any{&a})
		cancel()
		if a.Over {
			reason = a.Reason
			break
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			reason = "the module stopped"
			break
		}
	}
	close(s.ended)
	if onEnd != nil {
		onEnd(reason)
	}
}

// Ended closes when the stream has ended.
func (s *Session) Ended() <-chan struct{} { return s.ended }

// CheckProgram says whether the program a stream needs runs on this machine.
func CheckProgram(ctx context.Context, program string, _ func(*exec.Cmd)) error {
	return service.Call(ctx, module, "relay.CheckProgram", nil, nil, []any{program}, nil)
}

// MarshalJSON names a session to the module, by the name it gave.
func (s *Session) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID string `json:"id"`
	}{s.Sid})
}
