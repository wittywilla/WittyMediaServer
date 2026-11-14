// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package livetv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/tuner"
	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
)

// Remote is WittyTuner's service, as the module manager reaches it (modules.Tuners): one JSON call
// at a time on its own routes for the server (/tv/...), and an opened channel's MPEG-TS.
type Remote interface {
	Call(ctx context.Context, method, path string, in, out any) error
	Stream(ctx context.Context, slot string) (io.ReadCloser, error)
}

// RemoteDirectory is the Directory WittyTuner's service is.
//
// What is WittyTuner's to say and what isn't: it may say a channel is protected or busy, since that
// only ever refuses something. It can't say a channel is clear. Every stream it hands over is read
// through this server's own guard, and the server only believes what that guard sees. What it asks
// FFmpeg to open is held to CheckInput, since FFmpeg's options can read files and run other
// protocols.
type RemoteDirectory struct {
	R Remote
	// CheckInput refuses FFmpeg input that could be anything but a stream (modules.CheckInput).
	CheckInput func([]string) error
}

const (
	// remoteOpenTime is how long WittyTuner has to open a channel.
	remoteOpenTime = 45 * time.Second
	// remoteListTime is for reading every channel.
	remoteListTime = 30 * time.Second
	maxRemoteText  = 1000
	maxRemoteURL   = 8 << 10
	maxChannels    = 100000
)

func clipText(s string) string {
	if len(s) > maxRemoteText {
		return s[:maxRemoteText]
	}
	return s
}

// remoteErr is a WittyTuner refusal as this server's own errors, and WittyTuner being absent or
// off as errModuleMissing.
func remoteErr(err error) error {
	if err == nil {
		return nil
	}
	var miss interface{ ModuleMissing() bool }
	if errors.As(err, &miss) && miss.ModuleMissing() {
		return ModuleMissing(err)
	}
	var ref interface{ RefusalCode() string }
	if !errors.As(err, &ref) {
		return err
	}
	msg := clipText(err.Error())
	switch ref.RefusalCode() {
	case "encrypted":
		return &tuner.EncryptedError{Why: msg}
	case "busy":
		return tuner.ErrBusy
	case "not-found":
		return ErrNotFound
	case "no-archive":
		return ErrNoArchive
	case "unreachable":
		return fmt.Errorf("%w: %s", tuner.ErrUnreachable, msg)
	}
	return errors.New(msg)
}

// Channels is every channel of every tuner, held to what the guide can take; a logo is only ever a
// web address.
func (d RemoteDirectory) Channels(ctx context.Context) ([]*Channel, error) {
	ctx, cancel := context.WithTimeout(ctx, remoteListTime)
	defer cancel()
	var ans struct {
		Channels []*Channel `json:"channels"`
	}
	if err := d.R.Call(ctx, "GET", "/tv/channels", nil, &ans); err != nil {
		return nil, remoteErr(err)
	}
	out := make([]*Channel, 0, len(ans.Channels))
	for _, c := range ans.Channels {
		if len(out) == maxChannels {
			break
		}
		if c == nil || c.ID <= 0 {
			continue
		}
		if len(c.LogoURL) > maxRemoteURL || (c.LogoURL != "" && !strings.HasPrefix(c.LogoURL, "http://") && !strings.HasPrefix(c.LogoURL, "https://")) {
			c.LogoURL = ""
		}
		c.Number, c.Name, c.Group, c.CallSign = clipText(c.Number), clipText(c.Name), clipText(c.Group), clipText(c.CallSign)
		c.EPGID, c.EPGSet, c.TunerName, c.Driver = clipText(c.EPGID), clipText(c.EPGSet), clipText(c.TunerName), clipText(c.Driver)
		c.VideoCodec, c.AudioCodec, c.Encrypted, c.NoRecord = clipText(c.VideoCodec), clipText(c.AudioCodec), clipText(c.Encrypted), clipText(c.NoRecord)
		c.HealthMsg, c.LogoSet = clipText(c.HealthMsg), clipText(c.LogoSet)
		out = append(out, c)
	}
	return out, nil
}

// Note tells WittyTuner what the server's guard saw a stream show.
func (d RemoteDirectory) Note(ctx context.Context, channelID int64, why string) error {
	in := map[string]any{"channel": channelID}
	if why != "" {
		in["encrypted"] = why
	} else {
		in["clear"] = true
	}
	return remoteErr(d.R.Call(ctx, "POST", "/tv/note", in, nil))
}

// remoteOpened is what WittyTuner says it opened.
type remoteOpened struct {
	Slot     string   `json:"slot"`
	Channel  int64    `json:"channel"`
	Input    []string `json:"input,omitempty"`
	Verified bool     `json:"verified,omitempty"`
	Recheck  bool     `json:"recheck,omitempty"`
	Home     bool     `json:"home,omitempty"`
}

// Open opens a channel through WittyTuner, within its tuner's limit, and hands over the stream:
// MPEG-TS read through this server's own guard, or an address for FFmpeg held to CheckInput.
// Closing the stream lets the tuner go.
func (d RemoteDirectory) Open(ctx context.Context, req OpenRequest) (*Opening, error) {
	octx, cancel := context.WithTimeout(ctx, remoteOpenTime)
	defer cancel()
	var ans remoteOpened
	in := map[string]any{"channel": req.Channel, "archiveStart": req.ArchiveStart, "archiveEnd": req.ArchiveEnd, "backups": req.Backups}
	if err := d.R.Call(octx, "POST", "/tv/open", in, &ans); err != nil {
		return nil, remoteErr(err)
	}
	if ans.Slot == "" {
		return nil, errors.New("WittyTuner opened nothing")
	}
	if ans.Channel == 0 {
		ans.Channel = req.Channel
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			rctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = d.R.Call(rctx, "POST", "/tv/release", map[string]string{"slot": ans.Slot}, nil)
		})
	}
	if ans.Input != nil {
		check := d.CheckInput
		if check == nil {
			check = func([]string) error { return errors.New("there is nothing to check it with") }
		}
		if err := check(ans.Input); err != nil {
			release()
			return nil, fmt.Errorf("WittyTuner's stream was refused: %w", err)
		}
		st := &tuner.Stream{Input: ans.Input, Verified: ans.Verified, Done: release}
		if ans.Recheck {
			slot := ans.Slot
			st.Recheck = func(ctx context.Context) (string, time.Duration, error) {
				var r struct {
					Why  string `json:"why"`
					Next int64  `json:"next"`
				}
				if err := d.R.Call(ctx, "POST", "/tv/recheck", map[string]string{"slot": slot}, &r); err != nil {
					return "", 0, remoteErr(err)
				}
				return clipText(r.Why), time.Duration(max(r.Next, 0)) * time.Second, nil
			}
		}
		return &Opening{Stream: st, Channel: ans.Channel, Home: ans.Home}, nil
	}
	body, err := d.R.Stream(ctx, ans.Slot)
	if err != nil {
		release()
		return nil, remoteErr(err)
	}
	g := mpegts.NewGuard(body)
	return &Opening{Stream: &tuner.Stream{Reader: g, Guard: g, Done: release}, Channel: ans.Channel, Home: ans.Home}, nil
}
