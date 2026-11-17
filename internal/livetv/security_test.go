// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wittytuner "github.com/wittywilla/wittymediaserver/internal/tuner"
)

func securityServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"FriendlyName":"Box","ModelNumber":"M","DeviceID":"1","TunerCount":4}`)
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"GuideNumber":"5.1","GuideName":"WTST","URL":"%[1]s/ok"},
			{"GuideNumber":"105.1","GuideName":"WTST ATSC3","VideoCodec":"HEVC","AudioCodec":"AC4","Tags":"drm","URL":"%[1]s/ok"},
			{"GuideNumber":"7.1","GuideName":"ALONE","Tags":"drm","URL":"%[1]s/ok"},
			{"GuideNumber":"9.1","GuideName":"SNEAKY","URL":"%[1]s/scrambled"},
			{"GuideNumber":"11.1","GuideName":"WCLR3","VideoCodec":"HEVC","AudioCodec":"AC4","URL":"%[1]s/ok"}]`, srv.URL)
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsData(3000, false)) })
	mux.HandleFunc("/scrambled", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsData(3000, true)) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// addBox is securityServer's lineup on a network tuner, as WittyTuner reads it: what the device
// says about each channel's protection and format.
func addBox(f *fakeDirectory, srv *httptest.Server) {
	box := func(c Channel, path string) { c.Driver = "network"; f.add(1, c, srv.URL+path) }
	box(Channel{Number: "5.1", Name: "WTST", Security: SecurityClear}, "/ok")
	box(Channel{Number: "105.1", Name: "WTST ATSC3", ATSC3: true, VideoCodec: "HEVC", AudioCodec: "AC4", Encrypted: "the device says it is protected (DRM)"}, "/ok")
	box(Channel{Number: "7.1", Name: "ALONE", Encrypted: "the device says it is protected (DRM)"}, "/ok")
	box(Channel{Number: "9.1", Name: "SNEAKY", Security: SecurityClear}, "/scrambled")
	box(Channel{Number: "11.1", Name: "WCLR3", ATSC3: true, VideoCodec: "HEVC", AudioCodec: "AC4", Security: SecurityClear}, "/ok")
}

func channelNamed(t *testing.T, list []*Channel, name string) *Channel {
	t.Helper()
	for _, c := range list {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no channel %s", name)
	return nil
}

func TestSecurityStatusAndAlternatives(t *testing.T) {
	m, f := testManager(t)
	srv := securityServer(t)
	ctx := context.Background()
	addBox(f, srv)
	list, _ := m.GuideChannels(ctx)
	a3, plain, alone := channelNamed(t, list, "WTST ATSC3"), channelNamed(t, list, "WTST"), channelNamed(t, list, "ALONE")
	if a3.Security != SecurityEncrypted || !a3.ATSC3 || plain.Security != SecurityClear || plain.ATSC3 {
		t.Fatalf("%+v\n%+v", a3, plain)
	}
	alts := Alternatives(list, nil)
	if got := alts[a3.ID]; got == nil || got.Number != "5.1" || got.Key != fmt.Sprintf("tun-%d", plain.ID) {
		t.Fatalf("the same station on ATSC 1.0 should be offered: %+v", got)
	}
	if alts[alone.ID] != nil {
		t.Fatalf("a station with no clear version gets no offer: %+v", alts[alone.ID])
	}
	if alts[plain.ID] != nil {
		t.Fatal("a clear channel needs no alternative")
	}
	// Silent channels get an offer too, and are not offered themselves.
	silent := channelNamed(t, list, "WCLR3")
	withSilent := Alternatives(list, func(c *Channel) bool { return c.ID == silent.ID })
	if withSilent[silent.ID] != nil {
		t.Fatal("WCLR3 has no other version")
	}
	// The encrypted one is never opened, and never reaches the device.
	if _, err := m.Open(ctx, a3.ID); !wittytuner.IsEncrypted(err) {
		t.Fatalf("%v", err)
	}
	if !strings.Contains((&wittytuner.EncryptedError{Channel: "X", Why: "y"}).Error(), wittytuner.EncryptedLabel) {
		t.Fatal("the refusal uses the label")
	}
}

func TestUnencryptedATSC3Plays(t *testing.T) {
	m, f := testManager(t)
	srv := securityServer(t)
	ctx := context.Background()
	addBox(f, srv)
	list, _ := m.GuideChannels(ctx)
	c := channelNamed(t, list, "WCLR3")
	if !c.ATSC3 || c.Security != SecurityClear || c.VideoCodec != "HEVC" || c.AudioCodec != "AC4" {
		t.Fatalf("%+v", c)
	}
	o, err := m.Open(ctx, c.ID)
	if err != nil {
		t.Fatalf("unencrypted ATSC 3.0 is an ordinary channel: %v", err)
	}
	o.Release()
}

func TestStreamShowsWhatItIs(t *testing.T) {
	m, f := testManager(t)
	srv := securityServer(t)
	ctx := context.Background()
	addBox(f, srv)
	list, _ := m.GuideChannels(ctx)
	c := channelNamed(t, list, "SNEAKY")
	if c.Security != SecurityClear {
		t.Fatalf("the device said nothing against it: %+v", c)
	}
	o, err := m.Open(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 188*64)
	for {
		if _, err := o.Stream.Reader.Read(buf); err != nil {
			if !wittytuner.IsEncrypted(err) {
				t.Fatalf("the guard should stop it: %v", err)
			}
			break
		}
	}
	o.Release()
	got, _ := m.Channel(ctx, c.ID)
	if got.Security != SecurityEncrypted || got.Encrypted != wittytuner.ScrambledReason {
		t.Fatalf("WittyTuner should have been told, and it labelled: %+v", got)
	}
	if _, err := m.Open(ctx, c.ID); !wittytuner.IsEncrypted(err) {
		t.Fatalf("now refused before tuning: %v", err)
	}
	// Check again (in WittyTuner) only forgets: the guard still stands in the way.
	f.change(m, c.ID, func(c *Channel) { c.Encrypted, c.Security = "", SecurityUnknown })
	o, err = m.Open(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	var last error
	for last == nil {
		_, last = o.Stream.Reader.Read(buf)
	}
	if !wittytuner.IsEncrypted(last) {
		t.Fatalf("Check again must not get past the guard: %v", last)
	}
	o.Release()
}

func TestPlaylistChannelIsCheckedByPlaying(t *testing.T) {
	m, f := testManager(t)
	streams := securityServer(t)
	ctx := context.Background()
	ch := []*Channel{f.add(1, Channel{Number: "1", Name: "Long", Driver: "iptv"}, streams.URL+"/ok")}
	if ch[0].Security != SecurityUnknown {
		t.Fatalf("a playlist doesn't say: %+v", ch[0])
	}
	o, err := m.Open(ctx, ch[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 188*64)
	for {
		if _, err := o.Stream.Reader.Read(buf); err != nil {
			break
		}
	}
	o.Release()
	got, _ := m.Channel(ctx, ch[0].ID)
	if got.Security != SecurityClear {
		t.Fatalf("%+v", got)
	}
}

func TestRecordingRestrictionsAreHonoured(t *testing.T) {
	m, f := testManager(t)
	// The folder is set before anything is scheduled: the background loop may start the recording as soon as it's
	// due, and reads it then.
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	srv := iptvServer(t)
	_, ch := addIPTV(t, f, srv)
	ctx := context.Background()
	noRecord := func(why string) { f.change(m, ch[0].ID, func(c *Channel) { c.NoRecord = why }) }
	noRecord("you marked it as not to be recorded")
	now := time.Now().Unix()
	if _, err := m.Schedule(ctx, ch[0].ID, 1, "x", "", now, now+60); err == nil || !strings.Contains(err.Error(), "can't be recorded") {
		t.Fatalf("%v", err)
	}
	if _, err := m.AddRule(ctx, ch[0].ID, 1, "x"); err == nil {
		t.Fatal("nor as a series")
	}
	// Scheduled first, restricted after: it must not start.
	noRecord("")
	r, err := m.Schedule(ctx, ch[0].ID, 1, "x", "", now, now+60)
	if err != nil {
		t.Fatal(err)
	}
	noRecord("you marked it as not to be recorded")
	full, _ := m.recording(ctx, r.ID)
	m.record(full)
	got, _ := m.recording(ctx, r.ID)
	if got.Status != "failed" || got.Path != "" {
		t.Fatalf("%+v", got)
	}
}
