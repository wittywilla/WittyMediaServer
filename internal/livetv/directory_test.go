// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	wittytuner "github.com/wittywilla/wittymediaserver/internal/tuner"
	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
)

// fakeDirectory is WittyTuner as the server sees it, for a test: channels, each with the address
// a stream comes from, and each tuner's limit. A .ts address is handed over as MPEG-TS; anything
// else is an address for FFmpeg, an HLS playlist with a look again for keys, as WittyTuner does.
type fakeDirectory struct {
	mu     sync.Mutex
	chans  map[int64]*Channel
	addr   map[int64]string
	limit  map[int64]int
	inUse  map[int64]int
	home   bool
	client *http.Client
	next   int64
	opens  int
}

func newFakeDirectory() *fakeDirectory {
	return &fakeDirectory{chans: map[int64]*Channel{}, addr: map[int64]string{}, limit: map[int64]int{}, inUse: map[int64]int{}, home: true}
}

// add puts a channel on a tuner, in the guide, as clear unless it says otherwise.
func (f *fakeDirectory) add(tunerID int64, c Channel, addr string) *Channel {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	c.ID, c.TunerID, c.TunerEnabled, c.Enabled = f.next, tunerID, true, true
	if c.Security == "" {
		c.Security = SecurityUnknown
	}
	if c.Encrypted != "" {
		c.Security = SecurityEncrypted
	}
	if c.TunerName == "" {
		c.TunerName = fmt.Sprintf("Tuner %d", tunerID)
	}
	f.chans[c.ID] = &c
	f.addr[c.ID] = addr
	cp := c
	return &cp
}

// change changes a channel as WittyTuner's settings would, and has m ask again.
func (f *fakeDirectory) change(m *Manager, id int64, fn func(*Channel)) {
	f.mu.Lock()
	fn(f.chans[id])
	f.mu.Unlock()
	m.ForgetChannels()
}

func (f *fakeDirectory) used(tunerID int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inUse[tunerID]
}

func (f *fakeDirectory) Channels(context.Context) ([]*Channel, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*Channel, 0, len(f.chans))
	for _, c := range f.chans {
		cp := *c
		cp.TunerStreams = f.limit[c.TunerID]
		out = append(out, &cp)
	}
	return out, nil
}

func (f *fakeDirectory) Note(_ context.Context, id int64, why string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.chans[id]
	if c == nil {
		return ErrNotFound
	}
	switch {
	case why != "":
		c.Encrypted, c.Security = why, SecurityEncrypted
	case c.Encrypted == "":
		c.Security = SecurityClear
	}
	return nil
}

func (f *fakeDirectory) mark(id int64, why string) { _ = f.Note(context.Background(), id, why) }

func (f *fakeDirectory) get() *http.Client {
	if f.client != nil {
		return f.client
	}
	return http.DefaultClient
}

// keyIn says why an HLS playlist is encrypted, or "".
func keyIn(body string) string {
	for l := range strings.Lines(body) {
		if strings.HasPrefix(l, "#EXT-X-KEY:") && !strings.Contains(l, "METHOD=NONE") {
			return "its HLS playlist uses DRM"
		}
	}
	return ""
}

func (f *fakeDirectory) look(ctx context.Context, addr string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", addr, nil)
	res, err := f.get().Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return "", errors.New(res.Status)
	}
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return keyIn(string(b)), nil
}

func (f *fakeDirectory) Open(ctx context.Context, req OpenRequest) (*Opening, error) {
	f.mu.Lock()
	c := f.chans[req.Channel]
	if c == nil {
		f.mu.Unlock()
		return nil, ErrNotFound
	}
	if c.Encrypted != "" {
		f.mu.Unlock()
		return nil, &wittytuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
	}
	if l := f.limit[c.TunerID]; l > 0 && f.inUse[c.TunerID] >= l {
		f.mu.Unlock()
		return nil, ErrBusy
	}
	f.inUse[c.TunerID]++
	f.opens++
	addr, id, tunerID := f.addr[c.ID], c.ID, c.TunerID
	f.mu.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			f.mu.Lock()
			f.inUse[tunerID]--
			f.mu.Unlock()
		})
	}
	if addr == "slow" {
		// Some of a stream, then nothing more until it's closed.
		r, w := io.Pipe()
		go func() { _, _ = w.Write(make([]byte, 188*10)) }()
		return &Opening{Stream: &wittytuner.Stream{Reader: &releasing{ReadCloser: r, release: release}}, Channel: id, Home: f.home}, nil
	}
	if strings.HasSuffix(addr, ".ts") || strings.HasSuffix(addr, "/ok") || strings.HasSuffix(addr, "/scrambled") {
		req, _ := http.NewRequestWithContext(ctx, "GET", addr, nil)
		res, err := f.get().Do(req)
		if err != nil {
			release()
			return nil, err
		}
		g := mpegts.NewGuard(&releasing{ReadCloser: res.Body, release: release})
		return &Opening{Stream: &wittytuner.Stream{Reader: g, Guard: g}, Channel: id, Home: f.home}, nil
	}
	st := &wittytuner.Stream{Input: []string{"-i", addr}, Verified: strings.HasSuffix(addr, ".mpd")}
	if strings.HasSuffix(addr, ".m3u8") {
		st.Input = []string{"-protocol_whitelist", "http,https,tcp,tls", "-i", addr}
		why, err := f.look(ctx, addr)
		if err == nil && why != "" {
			release()
			f.mark(id, why)
			return nil, &wittytuner.EncryptedError{Channel: c.Name, Why: why}
		}
		st.Recheck = func(ctx context.Context) (string, time.Duration, error) {
			why, err := f.look(ctx, addr)
			return why, time.Second, err
		}
	}
	st.Done = release
	return &Opening{Stream: st, Channel: id, Home: f.home}, nil
}

// releasing lets a tuner go when its stream closes.
type releasing struct {
	io.ReadCloser
	release func()
}

func (r *releasing) Close() error {
	r.release()
	return r.ReadCloser.Close()
}

// testManager is a manager with a fake WittyTuner, which the test can change.
func testManager(t *testing.T) (*Manager, *fakeDirectory) {
	t.Helper()
	old := directory
	t.Cleanup(func() { SetDirectory(old) })
	f := newFakeDirectory()
	SetDirectory(f)
	return bareManager(t), f
}
