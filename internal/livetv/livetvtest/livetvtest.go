// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package livetvtest is WittyTuner as the server's live TV sees it (livetv.Directory), for tests
// of other packages: channels a test puts there, each with the address its stream comes from. A
// .ts address is handed over as MPEG-TS through the server's guard; anything else is an address
// for FFmpeg. The tuners, their channels and the guide sources are WittyTuner's own, so a test of
// the server's guide, recordings or sets starts here instead of in a database.
package livetvtest

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
)

// Directory is a WittyTuner with channels a test chose.
type Directory struct {
	mu    sync.Mutex
	chans map[int64]*livetv.Channel
	addr  map[int64]string
	limit map[int64]int
	inUse map[int64]int
	// Home is what every opened stream says of where it may lead (livetv.Opening.Home).
	Home bool
	// Opens counts the channels opened.
	Opens int
}

// Use puts a new Directory in place for the rest of the test.
func Use(t testing.TB) *Directory {
	t.Helper()
	d := &Directory{chans: map[int64]*livetv.Channel{}, addr: map[int64]string{}, limit: map[int64]int{}, inUse: map[int64]int{}, Home: true}
	livetv.SetDirectory(d)
	t.Cleanup(func() { livetv.SetDirectory(nil) })
	return d
}

// Add puts a channel on its tuner (c.TunerID, 1 if none), with the address its stream comes
// from. Its tuner is on; its own Enabled is as given. Security follows Encrypted when not set.
func (d *Directory) Add(c livetv.Channel, addr string) *livetv.Channel {
	d.mu.Lock()
	defer d.mu.Unlock()
	if c.ID == 0 {
		c.ID = int64(len(d.chans) + 1)
		for d.chans[c.ID] != nil {
			c.ID++
		}
	}
	if c.TunerID == 0 {
		c.TunerID = 1
	}
	if c.TunerName == "" {
		c.TunerName = "Test"
	}
	c.TunerEnabled = true
	switch {
	case c.Encrypted != "":
		c.Security = livetv.SecurityEncrypted
	case c.Security == "":
		c.Security = livetv.SecurityUnknown
	}
	d.chans[c.ID] = &c
	d.addr[c.ID] = addr
	cp := c
	return &cp
}

// Change changes a channel as WittyTuner's settings would, and has m ask again.
func (d *Directory) Change(m *livetv.Manager, id int64, fn func(*livetv.Channel)) {
	d.mu.Lock()
	if c := d.chans[id]; c != nil {
		fn(c)
	}
	d.mu.Unlock()
	if m != nil {
		m.ForgetChannels()
	}
}

// Limit sets how many streams a tuner allows at once.
func (d *Directory) Limit(tunerID int64, n int) {
	d.mu.Lock()
	d.limit[tunerID] = n
	d.mu.Unlock()
}

// InUse is how many streams a tuner has open.
func (d *Directory) InUse(tunerID int64) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.inUse[tunerID]
}

// Channels is every channel.
func (d *Directory) Channels(context.Context) ([]*livetv.Channel, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]*livetv.Channel, 0, len(d.chans))
	for _, c := range d.chans {
		cp := *c
		cp.LogoURL = c.LogoURL
		cp.TunerStreams = d.limit[c.TunerID]
		out = append(out, &cp)
	}
	return out, nil
}

// Note keeps what the server's guard saw.
func (d *Directory) Note(_ context.Context, id int64, why string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := d.chans[id]
	if c == nil {
		return livetv.ErrNotFound
	}
	switch {
	case why != "":
		c.Encrypted, c.Security = why, livetv.SecurityEncrypted
	case c.Encrypted == "":
		c.Security = livetv.SecurityClear
	}
	return nil
}

// Open opens a channel within its tuner's limit.
func (d *Directory) Open(ctx context.Context, req livetv.OpenRequest) (*livetv.Opening, error) {
	d.mu.Lock()
	c := d.chans[req.Channel]
	if c == nil {
		d.mu.Unlock()
		return nil, livetv.ErrNotFound
	}
	if c.Encrypted != "" {
		d.mu.Unlock()
		return nil, &tuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
	}
	if l := d.limit[c.TunerID]; l > 0 && d.inUse[c.TunerID] >= l {
		d.mu.Unlock()
		return nil, tuner.ErrBusy
	}
	d.inUse[c.TunerID]++
	d.Opens++
	addr, id, tunerID := d.addr[c.ID], c.ID, c.TunerID
	d.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			d.mu.Lock()
			d.inUse[tunerID]--
			d.mu.Unlock()
		})
	}
	if strings.HasSuffix(addr, ".ts") {
		r, err := http.NewRequestWithContext(ctx, "GET", addr, nil)
		if err != nil {
			release()
			return nil, err
		}
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			release()
			return nil, err
		}
		g := mpegts.NewGuard(res.Body)
		return &livetv.Opening{Stream: &tuner.Stream{Reader: g, Guard: g, Done: release}, Channel: id, Home: d.Home}, nil
	}
	return &livetv.Opening{Stream: &tuner.Stream{Input: []string{"-protocol_whitelist", "http,https,tcp,tls", "-i", addr}, Done: release}, Channel: id, Home: d.Home}, nil
}

var _ io.Reader = (*mpegts.Guard)(nil)
