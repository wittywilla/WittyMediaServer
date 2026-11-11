// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package mobile is wittyconnect in the shape gomobile can bind for Kotlin
// and Swift: plain strings, numbers and one small interface. What has more
// structure crosses as JSON. scripts/build-wittyconnect-mobile.sh makes the
// Android library and the Apple framework from it; docs/P2P.md says how the
// apps use them.
package mobile

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/wittyconnect"
)

// Listener hears about the connection. It is called from a background thread.
type Listener interface {
	// OnState gets the state as JSON: {"path":"direct","transport":"quic",...}.
	// path is one of connecting, offline, local, direct, relayed.
	OnState(state string)
}

// NewKey makes this device's connection key. Keep it in the system's secure
// storage (Keychain, Keystore): the server knows the device by it.
func NewKey() (string, error) { return wittyconnect.NewKey() }

// DeviceID is the public name of a key: what is sent to POST /api/v1/p2p/devices.
func DeviceID(key string) (string, error) { return wittyconnect.DeviceID(key) }

// ParseInvite reads an invite link (wittymedia://p2p/...) and answers it as
// JSON: {"serverId","secret","name","addrs":[...]}. An error means it isn't one.
func ParseInvite(link string) (string, error) {
	in, err := wittyconnect.ParseInvite(link)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(map[string]any{"serverId": in.ServerID, "secret": in.Secret, "name": in.Name, "addrs": in.Addrs})
	return string(b), err
}

// Session is one running connection.
type Session struct {
	mu sync.Mutex
	c  *wittyconnect.Connector
}

// Start connects. config is JSON:
//
//	{"key":"...","serverId":"12D3KooW...","hints":["/ip4/..."],
//	 "noDht":false,"noRelay":false,"noHolePunch":false,"staticRelays":[]}
//
// It returns once the local address is listening. listener may be nil.
func Start(config string, listener Listener) (*Session, error) {
	var cfg wittyconnect.Config
	if err := json.Unmarshal([]byte(config), &cfg); err != nil {
		return nil, errors.New("the configuration isn't valid JSON")
	}
	if listener != nil {
		cfg.OnState = func(s wittyconnect.State) {
			b, _ := json.Marshal(s)
			listener.OnState(string(b))
		}
	}
	c, err := wittyconnect.Start(cfg)
	if err != nil {
		return nil, err
	}
	return &Session{c: c}, nil
}

func (s *Session) conn() *wittyconnect.Connector {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.c
}

// Port is the port of the local address, http://127.0.0.1:<port> (0 once closed).
func (s *Session) Port() int {
	if c := s.conn(); c != nil {
		return c.Port()
	}
	return 0
}

// Secret is what every request to the local address must carry, in the header
// X-Witty-Connect or a cookie named wittyconnect.
func (s *Session) Secret() string {
	if c := s.conn(); c != nil {
		return c.Secret()
	}
	return ""
}

// DeviceID is this device's ID.
func (s *Session) DeviceID() string {
	if c := s.conn(); c != nil {
		return c.DeviceID()
	}
	return ""
}

// State is the state now, as JSON (see Listener).
func (s *Session) State() string {
	c := s.conn()
	if c == nil {
		return `{"path":"offline"}`
	}
	b, _ := json.Marshal(c.State())
	return string(b)
}

// KnownAddrs is the server's addresses as last learnt, as a JSON list, to
// save and pass as hints next time.
func (s *Session) KnownAddrs() string {
	c := s.conn()
	if c == nil {
		return "[]"
	}
	b, _ := json.Marshal(c.KnownAddrs())
	return string(b)
}

// NetworkChanged tells the session the device moved between networks (call
// it from ConnectivityManager's or NWPathMonitor's callback).
func (s *Session) NetworkChanged() {
	if c := s.conn(); c != nil {
		c.NetworkChanged()
	}
}

// Enrol shows the server an invite's secret. It blocks for up to a minute,
// so call it off the main thread. The answer is JSON:
// {"ok":true,"code":"123456","serverName":"..."} or {"ok":false,"reason":"expired","error":"..."}.
func (s *Session) Enrol(secret, name, platform string) (string, error) {
	c := s.conn()
	if c == nil {
		return "", errors.New("closed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	res, err := c.Enrol(ctx, secret, name, platform)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(res)
	return string(b), nil
}

// Close ends the connection. The session can't be used afterwards.
func (s *Session) Close() {
	s.mu.Lock()
	c := s.c
	s.c = nil
	s.mu.Unlock()
	if c != nil {
		c.Close()
	}
}
