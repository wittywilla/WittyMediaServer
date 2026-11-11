// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"
)

// Enrolment is the one thing a device that isn't on the server's list may do:
// show an invite. One request, one answer, each a 4-byte length and that many
// bytes of JSON, and the stream is over.

// EnrolRequest is what the device sends.
type EnrolRequest struct {
	V        int    `json:"v"`
	Secret   string `json:"secret"`
	Name     string `json:"name"`     // what to call the device in the list
	Platform string `json:"platform"` // desktop | android | apple | ...
}

// EnrolResponse is the server's answer.
type EnrolResponse struct {
	OK bool `json:"ok"`
	// Reason says why not, for apps: wrong | expired | used | too-many-tries | off.
	Reason string `json:"reason,omitempty"`
	Error  string `json:"error,omitempty"`
	// Code is a sign-in code to redeem at /api/v1/pair/redeem over the new
	// connection, when the invite was made to sign the device in as well.
	Code       string   `json:"code,omitempty"`
	ServerName string   `json:"serverName,omitempty"`
	Addrs      []string `json:"addrs,omitempty"`
}

const maxEnrolMessage = 4 << 10

func writeMessage(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) > maxEnrolMessage {
		return errors.New("message too long")
	}
	buf := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(buf, uint32(len(b)))
	copy(buf[4:], b)
	_, err = w.Write(buf)
	return err
}

func readMessage(r io.Reader, v any) error {
	var head [4]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(head[:])
	if n == 0 || n > maxEnrolMessage {
		return errors.New("message too long")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// window allows so many tries per key in a stretch of time, for a bounded
// number of keys: when the table is full, new keys are refused, so filling it
// buys a stranger nothing.
type window struct {
	mu    sync.Mutex
	limit int
	span  time.Duration
	max   int
	tries map[string][]time.Time
	now   func() time.Time
}

func newWindow(limit int, span time.Duration, maxKeys int) *window {
	return &window{limit: limit, span: span, max: maxKeys, tries: map[string][]time.Time{}, now: time.Now}
}

// allow counts a try and says whether it may go ahead.
func (w *window) allow(key string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	if len(w.tries) >= w.max {
		for k, list := range w.tries {
			if len(list) == 0 || now.Sub(list[len(list)-1]) >= w.span {
				delete(w.tries, k)
			}
		}
	}
	list, known := w.tries[key]
	if !known && len(w.tries) >= w.max {
		return false
	}
	keep := list[:0]
	for _, t := range list {
		if now.Sub(t) < w.span {
			keep = append(keep, t)
		}
	}
	if len(keep) >= w.limit {
		w.tries[key] = keep
		return false
	}
	w.tries[key] = append(keep, now)
	return true
}
