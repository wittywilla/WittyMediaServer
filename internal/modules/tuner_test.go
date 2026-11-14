// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
	wittytuner "github.com/wittywilla/wittymediaserver/internal/tuner"
	"github.com/wittywilla/wittymodule"
)

func tunerManifest(ver string) []byte {
	b, _ := json.Marshal(wittymodule.Manifest{ID: tunerID, Name: "WittyTuner", Version: ver, Repo: "wittywilla/WittyTuner", Delivery: wittymodule.DeliveryHelper,
		Entry: "wittytuner-module", API: MinAPI, MinServer: "0.1.0", MaxServer: "0.x", Permissions: []string{wittymodule.PermissionNetwork}})
	return b
}

// fakeTuners has a version of WittyTuner in use whose service's channels behave as
// testdata/faketuner says.
func fakeTuners(t *testing.T) (Tuners, *Manager) {
	t.Helper()
	w := newWorld(t)
	m := withInstalled(t, w, tunerManifest("0.1.0"), tunerManifest("0.2.0"), programBytes(t, "./testdata/faketuner"), nil)
	m.UseServices()
	t.Cleanup(m.Close)
	return Tuners{m}, m
}

func open(t *testing.T, tn Tuners, id int64) (*livetv.Opening, error) {
	t.Helper()
	return dir(tn).Open(context.Background(), livetv.OpenRequest{Channel: id})
}

// dir is live TV's view of WittyTuner, as the server sets it up.
func dir(tn Tuners) livetv.RemoteDirectory {
	return livetv.RemoteDirectory{R: tn, CheckInput: CheckInput}
}

// However WittyTuner describes a stream, the server's own guard reads it and refuses it.
func TestTunerScrambledIsRefused(t *testing.T) {
	tn, _ := fakeTuners(t)
	o, err := open(t, tn, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Stream.Close()
	if o.Stream.Guard == nil {
		t.Fatal("not through the server's guard")
	}
	_, err = io.Copy(io.Discard, o.Stream.Reader)
	if !wittytuner.IsEncrypted(err) {
		t.Fatalf("%v", err)
	}
	clear, err := open(t, tn, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer clear.Stream.Close()
	if n, err := io.Copy(io.Discard, clear.Stream.Reader); err != nil || n != 2000*188 || !clear.Home || clear.Channel != 1 {
		t.Fatalf("%d %v %+v", n, err, clear)
	}
}

// What WittyTuner asks FFmpeg to open is held to checkInput, and the tuner is let go.
func TestTunerUnsafeInputIsRefused(t *testing.T) {
	tn, _ := fakeTuners(t)
	if _, err := open(t, tn, 3); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("%v", err)
	}
	o, err := open(t, tn, 4)
	if err != nil || o.Stream.Recheck == nil || o.Stream.Reader != nil {
		t.Fatalf("%+v %v", o, err)
	}
	defer o.Stream.Close()
	why, next, err := o.Stream.Recheck(context.Background())
	if err != nil || why == "" || next.Seconds() != 1 {
		t.Fatalf("%q %v %v", why, next, err)
	}
}

// Its refusals are the server's own errors.
func TestTunerRefusals(t *testing.T) {
	tn, _ := fakeTuners(t)
	if _, err := open(t, tn, 5); !errors.Is(err, wittytuner.ErrBusy) {
		t.Fatalf("%v", err)
	}
	if _, err := open(t, tn, 6); !wittytuner.IsEncrypted(err) {
		t.Fatalf("%v", err)
	}
	if _, err := open(t, tn, 99); !errors.Is(err, livetv.ErrNotFound) {
		t.Fatalf("%v", err)
	}
}

// Closing a stream lets its tuner go.
func TestTunerCloseLetsGo(t *testing.T) {
	tn, _ := fakeTuners(t)
	o, err := open(t, tn, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := open(t, tn, 7); !errors.Is(err, wittytuner.ErrBusy) {
		t.Fatalf("a second stream: %v", err)
	}
	o.Stream.Close()
	o.Stream.Close() // twice is harmless
	again, err := open(t, tn, 7)
	if err != nil {
		t.Fatalf("after closing: %v", err)
	}
	again.Stream.Close()
}

// What it lists is held to what the guide can take.
func TestTunerChannelsAreChecked(t *testing.T) {
	tn, _ := fakeTuners(t)
	list, err := dir(tn).Channels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("%d channels: a channel with no id is left out", len(list))
	}
	if list[0].LogoURL != "https://logos.test/1.png" || list[0].Driver != "network" || !list[0].TunerEnabled {
		t.Fatalf("%+v", list[0])
	}
	if list[1].LogoURL != "" || len(list[1].Name) > 1000 {
		t.Fatalf("a file as a logo, or a long name: %q %d", list[1].LogoURL, len(list[1].Name))
	}
	if err := dir(tn).Note(context.Background(), 1, ""); err != nil {
		t.Fatal(err)
	}
}

// Without WittyTuner, the guide's live TV says so, and goes on without tuner channels.
func TestTunerMissing(t *testing.T) {
	if _, err := dir(Tuners{}).Channels(context.Background()); err == nil {
		t.Fatal("no WittyTuner, and channels")
	}
}

func TestCheckInput(t *testing.T) {
	list := []string{"-protocol_whitelist", "http,https,tcp,tls,udp,rtp,rtsp"}
	with := func(in ...string) []string { return append(append([]string{}, list...), in...) }
	for _, in := range [][]string{with("-i", "http://a.test/x"), with("-i", "https://a.test/x?y=1"), with("-i", "udp://239.0.0.1:1234"), with("-i", "rtp://239.0.0.1:1234"),
		with("-i", "rtsp://a.test/x"), with("-user_agent", "a", "-i", "http://a.test/x"), {"-f", "v4l2", "-i", "/dev/video0"}, {"-f", "dshow", "-i", "video=Card"}} {
		if err := CheckInput(in); err != nil {
			t.Errorf("%v: %v", in, err)
		}
	}
	for _, in := range [][]string{nil, {}, {"-i"}, {"http://a.test/x"}, {"-i", ""},
		{"-f", "lavfi", "-i", "http://a.test/x"}, {"-i", "http://a.test/x", "-i", "file:///etc/passwd"},
		{"-i", "file:///etc/passwd"}, {"-i", "/etc/passwd"}, {"-i", "concat:http://a/1|file:///x"},
		{"-i", "crypto+http://a.test/x"}, {"-i", "hls+http://a.test/x"}, {"-i", "subfile,,start,0,end,0,,:/etc/passwd"},
		{"-i", "tcp://a.test:1"}, {"-protocol_whitelist", "file,http", "-i", "http://a.test/x"}, {"-y", "/tmp/x", "-i", "http://a.test/x"}, {"-i", "http://a.test/x", "/tmp/out"},
		{"-i", "http://a.test/x"}, {"-protocol_whitelist", "crypto,http", "-i", "http://a.test/x"}, with("-i", "tcp://a.test:1"), with("-i", "/etc/passwd"), with("-f", "lavfi", "-i", "http://a.test/x")} {
		if err := CheckInput(in); err == nil {
			t.Errorf("%v passed", in)
		}
	}
}
