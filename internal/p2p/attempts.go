// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"slices"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/connectsteps"
)

// Attempt is one device's try at reaching this server from away, as the
// server saw it: so the owner can see that a device tried, and why it was
// refused, in the words the device itself shows (connectsteps). Kept in
// memory only, and never sent anywhere.
type Attempt struct {
	At     int64  `json:"at"`     // Unix seconds, of the latest when Count > 1
	Device string `json:"device"` // the device's connection ID
	Name   string `json:"name,omitempty"`
	Kind   string `json:"kind"` // invite | code | connect
	OK     bool   `json:"ok"`
	Step   string `json:"step,omitempty"` // the step that failed (connectsteps)
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"` // in words
	Path   string `json:"path,omitempty"`   // local | direct | relayed, when it got through
	Count  int    `json:"count"`            // the same try, repeated within a minute
}

const (
	attemptsPerDevice = 12
	attemptDevices    = 100
	attemptRepeat     = time.Minute
)

// attempts is the history, newest last for each device.
type attempts struct {
	mu   sync.Mutex
	by   map[string][]Attempt
	now  func() time.Time
	last map[string]time.Time // when each device last tried, to forget the quietest
}

func newAttempts() *attempts {
	return &attempts{by: map[string][]Attempt{}, last: map[string]time.Time{}, now: time.Now}
}

// note keeps a try. The same result again within a minute adds to the count of
// the one before, so a device retrying in a loop is one line, not twelve.
func (a *attempts) note(t Attempt) {
	if a == nil || t.Device == "" {
		return
	}
	now := a.now()
	t.At, t.Count = now.Unix(), 1
	if len(t.Reason) > 300 {
		t.Reason = t.Reason[:300]
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	list := a.by[t.Device]
	if n := len(list); n > 0 {
		p := &list[n-1]
		if p.Kind == t.Kind && p.OK == t.OK && p.Code == t.Code && now.Unix()-p.At < int64(attemptRepeat/time.Second) {
			p.At, p.Count = t.At, p.Count+1
			if t.Name != "" {
				p.Name = t.Name
			}
			a.last[t.Device] = now
			return
		}
		if t.Name == "" {
			t.Name = p.Name
		}
	}
	if _, had := a.by[t.Device]; !had && len(a.by) >= attemptDevices {
		var oldest string
		var at time.Time
		for d, w := range a.last {
			if oldest == "" || w.Before(at) {
				oldest, at = d, w
			}
		}
		delete(a.by, oldest)
		delete(a.last, oldest)
	}
	list = append(list, t)
	if len(list) > attemptsPerDevice {
		list = slices.Clone(list[len(list)-attemptsPerDevice:])
	}
	a.by[t.Device] = list
	a.last[t.Device] = now
}

// list is a device's tries, newest first, or every device's when device is "".
func (a *attempts) list(device string) []Attempt {
	if a == nil {
		return []Attempt{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []Attempt{}
	for d, l := range a.by {
		if device == "" || d == device {
			out = append(out, l...)
		}
	}
	slices.SortStableFunc(out, func(x, y Attempt) int {
		switch {
		case x.At > y.At:
			return -1
		case x.At < y.At:
			return 1
		}
		return 0
	})
	return out
}

func (a *attempts) forget(device string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	delete(a.by, device)
	delete(a.last, device)
	a.mu.Unlock()
}

// refusal is an attempt that failed for a reason the server gave (an
// EnrolResponse's), at the step that reason belongs to.
func refusal(device, name, kind, reason, words string) Attempt {
	f := connectsteps.Classify(reason, words, connectsteps.SignIn)
	return Attempt{Device: device, Name: name, Kind: kind, Step: f.Step, Code: f.Code, Reason: f.Say}
}

// Attempts are the devices' tries at connecting from away, newest first: one
// device's when device isn't "".
func (n *Node) Attempts(device string) []Attempt { return n.tries.list(device) }

// NoteAttempt keeps a try the server saw outside the node (a sign-in that
// failed over a connection from away, say).
func (n *Node) NoteAttempt(a Attempt) { n.tries.note(a) }

// ForgetAttempts drops a device's history, as removing it does.
func (n *Node) ForgetAttempts(device string) { n.tries.forget(device) }

// enrolledFor is the device an invite was shown for: the device's own key,
// or the one an invite's enrol key spoke for through the relay, once the
// server has checked it could (ok): a stranger can't add to another's history.
func enrolledFor(id string, req EnrolRequest, ok bool) string {
	if req.Device != "" && ok {
		return req.Device
	}
	return id
}

// noteEnrolled keeps the server's answer to an invite or a code.
func (n *Node) noteEnrolled(device, name, kind string, res EnrolResponse) {
	if res.OK {
		n.tries.note(Attempt{Device: device, Name: name, Kind: kind, OK: true})
		return
	}
	n.tries.note(refusal(device, name, kind, res.Reason, res.Error))
}
