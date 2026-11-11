// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"

	wittytuner "github.com/wittywilla/wittymediaserver/internal/tuner"
	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
)

// setupEngine is WittyTuner for the setup's tests: a source per "behave" setting.
type setupEngine struct{ fakeEngine }

func (e *setupEngine) New(_ context.Context, driver string, c wittytuner.Config, _ wittytuner.Env) (wittytuner.Source, error) {
	if c.Options["address"] == "" && driver == "network" {
		return nil, errors.New("fill in the device's address")
	}
	return &setupSource{driver: driver, behave: c.Options["behave"]}, nil
}

type setupSource struct{ driver, behave string }

func (s *setupSource) Info() wittytuner.Info {
	i := wittytuner.Info{Driver: s.driver, Name: "Box", Detail: "Test box TB-2 at http://192.0.2.1", MaxStreams: 2}
	if s.behave == "own guide" {
		i.GuideURL = "http://192.0.2.1/guide.xml"
	}
	return i
}

func (s *setupSource) Channels(context.Context) ([]wittytuner.Channel, error) {
	switch s.behave {
	case "unreachable":
		return nil, fmt.Errorf("no tuner answered: %w", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")})
	case "web page":
		return nil, errors.New("no tuner answered at http://192.0.2.1: the device answered 404 Not Found")
	case "all encrypted":
		return []wittytuner.Channel{{ID: "1", Number: "1", Name: "Locked", Encrypted: "the device marks it as copy protected", Checked: true}}, nil
	}
	return []wittytuner.Channel{
		{ID: "2", Number: "2", Name: "Locked", Encrypted: "the device marks it as copy protected", Checked: true},
		{ID: "3", Number: "3", Name: "Radio", Radio: true},
		{ID: "4", Number: "4", Name: "News"},
		{ID: "5", Number: "5", Name: "Scrambled"},
		{ID: "6", Number: "6", Name: "Silent"},
	}, nil
}

func (s *setupSource) Scan(ctx context.Context, _ wittytuner.ScanOptions, _ func(wittytuner.Progress)) ([]wittytuner.Channel, error) {
	return s.Channels(ctx)
}

func (s *setupSource) Open(_ context.Context, ch wittytuner.Channel) (*wittytuner.Stream, error) {
	var data []byte
	switch ch.Name {
	case "News":
		data = tsData(3000, false)
	case "Scrambled":
		data = tsData(3000, true)
	case "Silent":
	default:
		return nil, errors.New("the tuner could not tune this channel: the signal may be too weak")
	}
	g := mpegts.NewGuard(io.NopCloser(bytes.NewReader(data)))
	return &wittytuner.Stream{Reader: g, Guard: g}, nil
}

func (s *setupSource) Close() error { return nil }

func setupManager(t *testing.T) *Manager {
	old := engine
	t.Cleanup(func() { SetEngine(old) })
	SetEngine(&setupEngine{})
	return bareManager(t)
}

// A check says what the source is and what it has, and keeps nothing.
func TestCheckSaysWhatItFound(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()
	res, err := m.Check(ctx, "network", "", map[string]string{"address": "192.0.2.1", "behave": "own guide"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Channels != 5 || res.Encrypted != 1 || res.MaxStreams != 2 || !strings.Contains(res.Detail, "TB-2") || !res.OwnGuide {
		t.Fatalf("%+v", res)
	}
	if strings.Join(res.Sample, ",") != "Radio,News,Scrambled,Silent" {
		t.Fatalf("encrypted channels aren't offered as a sample: %v", res.Sample)
	}
	if list, _ := m.Tuners(ctx); len(list) != 0 {
		t.Fatalf("a check must not save anything: %+v", list)
	}
}

// Every way a check fails has its own words, and none of them carries an address.
func TestCheckProblems(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()
	for _, c := range []struct{ driver, behave, address, kind string }{
		{"network", "", "", "settings"},
		{"network", "unreachable", "192.0.2.1", "unreachable"},
		{"iptv", "unreachable", "x", "unreachable"},
		{"network", "web page", "192.0.2.1", "not-a-tuner"},
		{"iptv", "web page", "x", "unreadable"},
	} {
		_, err := m.Check(ctx, c.driver, "", map[string]string{"address": c.address, "behave": c.behave})
		if err == nil {
			t.Fatalf("%+v: no error", c)
		}
		p := CheckProblem(c.driver, err)
		if p.Kind != c.kind || p.Message == "" || strings.Contains(p.Message+p.Hint, "://") {
			t.Fatalf("%+v: %+v", c, p)
		}
	}
	if p := CheckProblem("network", context.DeadlineExceeded); p.Kind != "unreachable" {
		t.Fatalf("%+v", p)
	}
}

func addSetupTuner(t *testing.T, m *Manager, behave string) *Tuner {
	t.Helper()
	tn, err := m.Create(context.Background(), &Tuner{Driver: "network", Name: "Box", Options: map[string]string{"address": "192.0.2.1", "behave": behave}})
	if err != nil {
		t.Fatal(err)
	}
	return tn
}

func setupChannel(t *testing.T, m *Manager, tunerID int64, name string) *Channel {
	t.Helper()
	list, _ := m.Channels(context.Background(), tunerID)
	for _, c := range list {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no channel %q", name)
	return nil
}

// Trying a channel picks a clear TV channel by itself, tunes it, and says it played.
func TestTestChannelPlays(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()
	tn := addSetupTuner(t, m, "")
	res, err := m.TestChannel(ctx, tn.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Without FFmpeg there's nothing to say what's in it, but the guard has seen it's clear.
	if !res.OK || res.Problem != nil || res.Channel == nil || res.Channel.Name != "News" || !res.Clear {
		t.Fatalf("%+v %+v", res, res.Problem)
	}
	if c := setupChannel(t, m, tn.ID, "News"); c.Security != SecurityClear {
		t.Fatalf("what the guard saw is kept: %+v", c)
	}
	m.mu.Lock()
	n := m.inUse[tn.ID]
	m.mu.Unlock()
	if n != 0 {
		t.Fatalf("the tuner is given back: %d in use", n)
	}
}

// An encrypted channel stays refused, whether it's known to be or the stream shows it.
func TestTestChannelEncrypted(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()
	tn := addSetupTuner(t, m, "")
	for _, name := range []string{"Locked", "Scrambled"} {
		c := setupChannel(t, m, tn.ID, name)
		res, err := m.TestChannel(ctx, tn.ID, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if res.OK || res.Problem == nil || res.Problem.Kind != "encrypted" || !strings.Contains(res.Problem.Message, wittytuner.EncryptedLabel) {
			t.Fatalf("%s: %+v %+v", name, res, res.Problem)
		}
	}
	if c := setupChannel(t, m, tn.ID, "Scrambled"); c.Security != SecurityEncrypted {
		t.Fatalf("a scrambled stream is remembered as encrypted: %+v", c)
	}
	// Once every channel is encrypted, there's nothing to try, and it says why.
	tn2 := addSetupTuner(t, m, "all encrypted")
	res, err := m.TestChannel(ctx, tn2.ID, 0)
	if err != nil || res.Problem == nil || res.Problem.Kind != "encrypted" || res.Channel != nil {
		t.Fatalf("%+v %v", res, err)
	}
}

// A channel that sends nothing, or won't tune, says so in words, and a channel of another tuner isn't found.
func TestTestChannelFailures(t *testing.T) {
	m := setupManager(t)
	ctx := context.Background()
	tn := addSetupTuner(t, m, "")
	res, err := m.TestChannel(ctx, tn.ID, setupChannel(t, m, tn.ID, "Silent").ID)
	if err != nil || res.OK || res.Problem.Kind != "no-signal" {
		t.Fatalf("%+v %v", res, err)
	}
	res, err = m.TestChannel(ctx, tn.ID, setupChannel(t, m, tn.ID, "Radio").ID)
	if err != nil || res.OK || res.Problem.Kind != "other" || !strings.Contains(res.Problem.Message, "signal may be too weak") {
		t.Fatalf("%+v %v", res, err)
	}
	other := addSetupTuner(t, m, "all encrypted")
	if _, err := m.TestChannel(ctx, other.ID, setupChannel(t, m, tn.ID, "News").ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("%v", err)
	}
}
