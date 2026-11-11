// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package wittyremote is the Remote Protocol: one WittyMedia client controlling another
// through the server. A client that can be controlled announces itself as a device, keeps
// a channel open for commands, and reports what it is playing. docs/REMOTE-PROTOCOL.md is
// the wire format; this file is its messages, and hub.go who is here and what is waiting.
//
// Nothing here knows about HTTP, accounts' limits or the library: internal/server decides
// who may control what, and hands over only what is allowed.
package wittyremote

import (
	"errors"
	"math"
	"slices"
	"strings"
	"unicode/utf8"
)

// Version is the protocol version this server speaks, and MinVersion the oldest it still
// accepts from a client. Within a version, messages only gain fields.
const (
	Version    = 1
	MinVersion = 1
)

// Kinds of device. A client that is none of these says "other".
var Kinds = []string{"web", "tv", "phone", "tablet", "desktop", "other"}

// Features a device can say it has. One it doesn't name is never asked of it.
const (
	FeatureSeek      = "seek"
	FeatureVolume    = "volume"
	FeatureQueue     = "queue"
	FeatureAudio     = "audio"
	FeatureSubtitles = "subtitles"
	FeatureNavigate  = "navigate"
	FeatureRate      = "rate"
)

// Features is every feature this version knows.
var Features = []string{FeatureSeek, FeatureVolume, FeatureQueue, FeatureAudio, FeatureSubtitles, FeatureNavigate, FeatureRate}

// MediaKinds are the kinds of item a device can say it plays: the library's own item kinds.
var MediaKinds = []string{"video", "audio", "dvd-iso", "dvd-folder", "bluray-iso", "bluray-folder", "cd-image", "book", "radio", "game", "photo"}

// Command types.
const (
	CmdPlay     = "play"
	CmdPause    = "pause"
	CmdResume   = "resume"
	CmdStop     = "stop"
	CmdSeek     = "seek"
	CmdNext     = "next"
	CmdPrevious = "previous"
	CmdVolume   = "volume"
	CmdMute     = "mute"
	CmdAudio    = "audio"
	CmdSubtitle = "subtitle"
	CmdRate     = "rate"
	CmdNavigate = "navigate"
	// CmdCast is only ever sent by a controller: the server turns it into a play for the
	// device it is sent to, and a stop for the one named in From.
	CmdCast = "cast"
)

// NavigateKeys are the keys a TV interface is driven with.
var NavigateKeys = []string{"up", "down", "left", "right", "select", "back", "home", "menu", "playpause"}

// Playback states a device reports.
var Statuses = []string{"idle", "playing", "paused", "buffering"}

// What bounds a message. A device or a controller can't make the server hold more.
const (
	MaxNameLen    = 60
	MaxQueueItems = 200
	MaxTracks     = 64
	maxLabelLen   = 80
	maxIDLen      = 64
)

// Capabilities is what a device says it can do, and after Negotiate what the server will
// ask of it.
type Capabilities struct {
	MediaKinds []string `json:"mediaKinds"`
	Features   []string `json:"features"`
}

// Negotiate keeps what both sides know. A newer client's features this server has never
// heard of are dropped, so nothing is ever sent that neither side can describe.
func (c Capabilities) Negotiate() Capabilities {
	out := Capabilities{MediaKinds: []string{}, Features: []string{}}
	for _, k := range MediaKinds {
		if slices.Contains(c.MediaKinds, k) {
			out.MediaKinds = append(out.MediaKinds, k)
		}
	}
	for _, f := range Features {
		if slices.Contains(c.Features, f) {
			out.Features = append(out.Features, f)
		}
	}
	return out
}

// Has reports whether a feature was agreed.
func (c Capabilities) Has(feature string) bool { return slices.Contains(c.Features, feature) }

// Plays reports whether a kind of item was agreed.
func (c Capabilities) Plays(kind string) bool { return slices.Contains(c.MediaKinds, kind) }

// Sender says who asked, for the device to show ("Sam is playing this on the TV").
type Sender struct {
	UserName string `json:"userName"`
	DeviceID string `json:"deviceId,omitempty"`
}

// Command is one thing a device is asked to do. Which fields matter depends on Type;
// docs/REMOTE-PROTOCOL.md lists them. Pointers tell "not said" from zero.
type Command struct {
	V    int    `json:"v"`
	ID   string `json:"id,omitempty"`
	Type string `json:"type"`

	// play
	ItemID     int64   `json:"itemId,omitempty"`
	Queue      []int64 `json:"queue,omitempty"`
	QueueIndex int     `json:"queueIndex,omitempty"`
	// play and seek: where, in seconds from the start.
	PositionSec *float64 `json:"positionSec,omitempty"`
	// seek: or this far from where it is now.
	BySec *float64 `json:"bySec,omitempty"`
	// play, audio, subtitle: a track's id as the device's own state lists it. "off" for no
	// subtitles.
	Audio    *string `json:"audio,omitempty"`
	Subtitle *string `json:"subtitle,omitempty"`
	// volume: 0 to 1. mute: on or off.
	Level *float64 `json:"level,omitempty"`
	Muted *bool    `json:"muted,omitempty"`
	// rate: 1 is normal speed.
	Rate *float64 `json:"rate,omitempty"`
	// navigate
	Key string `json:"key,omitempty"`
	// cast: the device to take over from.
	From string `json:"from,omitempty"`

	// Filled in by the server on the way to the device.
	Sender *Sender `json:"sender,omitempty"`
	SentAt int64   `json:"sentAt,omitempty"` // server time, ms
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

// Check says what is wrong with a command a controller sent, or nil.
func (c *Command) Check() error {
	if c.V != 0 && c.V < MinVersion {
		return errors.New("that protocol version is too old for this server")
	}
	for _, f := range []*float64{c.PositionSec, c.BySec, c.Level, c.Rate} {
		if f != nil && !finite(*f) {
			return errors.New("numbers must be finite")
		}
	}
	for _, s := range []*string{c.Audio, c.Subtitle} {
		if s != nil && (len(*s) > maxIDLen || !utf8.ValidString(*s)) {
			return errors.New("a track id is too long")
		}
	}
	if c.PositionSec != nil && *c.PositionSec < 0 {
		return errors.New("positionSec can't be negative")
	}
	switch c.Type {
	case CmdPlay:
		if c.ItemID <= 0 && len(c.Queue) == 0 {
			return errors.New("play needs itemId or queue")
		}
		if len(c.Queue) > MaxQueueItems {
			return errors.New("the queue is too long")
		}
		for _, id := range c.Queue {
			if id <= 0 {
				return errors.New("the queue holds something that isn't an item")
			}
		}
		if len(c.Queue) > 0 {
			if c.QueueIndex < 0 || c.QueueIndex >= len(c.Queue) {
				return errors.New("queueIndex is outside the queue")
			}
			if c.ItemID != 0 && c.ItemID != c.Queue[c.QueueIndex] {
				return errors.New("itemId isn't the queue's item at queueIndex")
			}
		} else if c.QueueIndex != 0 {
			return errors.New("queueIndex needs a queue")
		}
	case CmdSeek:
		if (c.PositionSec == nil) == (c.BySec == nil) {
			return errors.New("seek needs positionSec or bySec, not both")
		}
	case CmdVolume:
		if c.Level == nil || *c.Level < 0 || *c.Level > 1 {
			return errors.New("volume needs level, from 0 to 1")
		}
	case CmdMute:
		if c.Muted == nil {
			return errors.New("mute needs muted")
		}
	case CmdAudio:
		if c.Audio == nil || *c.Audio == "" {
			return errors.New("audio needs a track id")
		}
	case CmdSubtitle:
		if c.Subtitle == nil || *c.Subtitle == "" {
			return errors.New(`subtitle needs a track id, or "off"`)
		}
	case CmdRate:
		if c.Rate == nil || *c.Rate < 0.25 || *c.Rate > 4 {
			return errors.New("rate needs rate, from 0.25 to 4")
		}
	case CmdNavigate:
		if !slices.Contains(NavigateKeys, c.Key) {
			return errors.New("navigate needs a key: " + strings.Join(NavigateKeys, ", "))
		}
	case CmdCast:
		if c.From == "" || len(c.From) > maxIDLen {
			return errors.New("cast needs from: the device to take over from")
		}
	case CmdPause, CmdResume, CmdStop, CmdNext, CmdPrevious:
	default:
		return errors.New("unknown command type")
	}
	return nil
}

// Needs is the feature a device must have agreed for this command, "" for one every
// device obeys.
func (c *Command) Needs() string {
	switch c.Type {
	case CmdSeek:
		return FeatureSeek
	case CmdNext, CmdPrevious:
		return FeatureQueue
	case CmdVolume, CmdMute:
		return FeatureVolume
	case CmdAudio:
		return FeatureAudio
	case CmdSubtitle:
		return FeatureSubtitles
	case CmdRate:
		return FeatureRate
	case CmdNavigate:
		return FeatureNavigate
	case CmdPlay:
		if len(c.Queue) > 1 {
			return FeatureQueue
		}
	}
	return ""
}

// Items is every item a play command names.
func (c *Command) Items() []int64 {
	if len(c.Queue) > 0 {
		return c.Queue
	}
	if c.ItemID > 0 {
		return []int64{c.ItemID}
	}
	return nil
}

// Track is one sound or subtitle track a device offers for what it is playing.
type Track struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
	Lang  string `json:"lang,omitempty"`
}

// State is what a device is doing, as it last said.
type State struct {
	V           int      `json:"v"`
	Status      string   `json:"status"`
	ItemID      int64    `json:"itemId,omitempty"`
	PositionSec float64  `json:"positionSec"`
	DurationSec float64  `json:"durationSec"`
	Rate        float64  `json:"rate"`
	Volume      *float64 `json:"volume,omitempty"`
	Muted       bool     `json:"muted"`
	Audio       string   `json:"audio,omitempty"`
	Subtitle    string   `json:"subtitle,omitempty"`
	AudioTracks []Track  `json:"audioTracks,omitempty"`
	SubTracks   []Track  `json:"subtitleTracks,omitempty"`
	Queue       []int64  `json:"queue,omitempty"`
	QueueIndex  int      `json:"queueIndex,omitempty"`
	// Screen: where in its interface the device is, when it isn't playing ("home", "player").
	Screen string `json:"screen,omitempty"`
}

func clampNum(f, lo, hi float64) float64 {
	if !finite(f) || f < lo {
		return lo
	}
	if f > hi {
		return hi
	}
	return f
}

// Clip cuts s to at most n bytes without splitting a character, and drops what isn't text.
func Clip(s string, n int) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	for len(s) > n {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

func cleanTracks(in []Track) []Track {
	if len(in) > MaxTracks {
		in = in[:MaxTracks]
	}
	out := make([]Track, 0, len(in))
	for _, t := range in {
		if t.ID = Clip(t.ID, maxIDLen); t.ID == "" {
			continue
		}
		out = append(out, Track{ID: t.ID, Label: Clip(t.Label, maxLabelLen), Lang: Clip(t.Lang, 16)})
	}
	return out
}

// Clean makes a state a device sent safe to keep and pass on: known status, finite
// numbers in range, short strings, bounded lists. A device is never refused for a slightly
// wrong report; it is tidied.
func (s State) Clean() State {
	out := State{V: Version, Status: s.Status, Muted: s.Muted}
	if !slices.Contains(Statuses, out.Status) {
		out.Status = "idle"
	}
	if s.Volume != nil {
		v := clampNum(*s.Volume, 0, 1)
		out.Volume = &v
	}
	out.Screen = Clip(s.Screen, 40)
	if out.Status == "idle" || s.ItemID <= 0 {
		out.Status = "idle"
		out.Rate = 1
		return out
	}
	const day = 24 * 3600 * 30
	out.ItemID = s.ItemID
	out.PositionSec = clampNum(s.PositionSec, 0, day)
	out.DurationSec = clampNum(s.DurationSec, 0, day)
	out.Rate = clampNum(s.Rate, 0, 16)
	if out.Rate == 0 {
		out.Rate = 1
	}
	out.Audio, out.Subtitle = Clip(s.Audio, maxIDLen), Clip(s.Subtitle, maxIDLen)
	out.AudioTracks, out.SubTracks = cleanTracks(s.AudioTracks), cleanTracks(s.SubTracks)
	if n := len(s.Queue); n > 0 && n <= MaxQueueItems && s.QueueIndex >= 0 && s.QueueIndex < n {
		ok := true
		for _, id := range s.Queue {
			ok = ok && id > 0
		}
		if ok {
			out.Queue, out.QueueIndex = slices.Clone(s.Queue), s.QueueIndex
		}
	}
	return out
}
