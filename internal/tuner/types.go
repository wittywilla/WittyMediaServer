// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package tuner is what the server knows of a TV tuner: the shapes WittyTuner answers in, and
// how a server words a refusal. WittyTuner itself is a module, reached through internal/modules.
// These travel as JSON, so a field a newer version adds is simply not read.

package tuner

import (
	"errors"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
)

// HelperOpen is the answer to "open" for a stream FFmpeg should open by itself. It is
// a suggestion, and nothing else: the server opens and checks such a stream itself.
type HelperOpen struct {
	Input []string `json:"input,omitempty"`
	// Verified: it was inspected on opening and found clear. Recheck: it can change while it
	// plays, so it's worth asking again.
	Verified bool `json:"verified,omitempty"`
	Recheck  bool `json:"recheck,omitempty"`
}

// Channel is one channel a source offers.
type Channel struct {
	ID       string `json:"id"` // stable within the source
	Number   string `json:"number"`
	Name     string `json:"name"`
	Group    string `json:"group,omitempty"`
	CallSign string `json:"callsign,omitempty"`
	Locator  string `json:"locator,omitempty"` // how the driver finds it again
	Logo     string `json:"logo,omitempty"`    // an address given by the source
	EPGID    string `json:"epgId,omitempty"`   // XMLTV channel id
	HD       bool   `json:"hd,omitempty"`
	Radio    bool   `json:"radio,omitempty"`
	Program  int    `json:"program,omitempty"` // MPEG program number
	// Encrypted is why this channel cannot be tuned ("" = it can).
	Encrypted string `json:"encrypted,omitempty"`
	// Checked: the source says for itself whether this channel is protected,
	// so no Encrypted reason means it is clear. Otherwise nobody knows until
	// the channel is first tuned.
	Checked bool `json:"checked,omitempty"`
	// ATSC3: an ATSC 3.0 (NextGen TV) broadcast.
	ATSC3      bool   `json:"atsc3,omitempty"`
	VideoCodec string `json:"videoCodec,omitempty"` // as the source names it: HEVC, MPEG2…
	AudioCodec string `json:"audioCodec,omitempty"` // AC4, AC3…
	// NoRecord is why the device says this channel may not be recorded.
	NoRecord string `json:"noRecord,omitempty"`
	// Catch-up: the source keeps CatchupDays of what the channel showed. Catchup is how to
	// ask for it (default, append, shift, flussonic or xc) and CatchupSource the address
	// template, both the driver's own business.
	CatchupDays   int    `json:"catchupDays,omitempty"`
	Catchup       string `json:"catchup,omitempty"`
	CatchupSource string `json:"catchupSource,omitempty"`
	// ArchiveStart and ArchiveEnd are set only when opening: play that stretch from the
	// source's archive instead of what's on now.
	ArchiveStart int64 `json:"archiveStart,omitempty"`
	ArchiveEnd   int64 `json:"archiveEnd,omitempty"`
}

// Info describes a configured source.
type Info struct {
	Driver     string
	Name       string
	MaxStreams int    // how many channels it can stream at once; 0 = no known limit
	Detail     string // model, address: shown in Settings
	GuideURL   string // an XMLTV address the source offers
}

// Progress is how far a scan has got.
type Progress struct {
	Percent int    `json:"percent"`
	Found   int    `json:"found"`
	Message string `json:"message,omitempty"`
}

// ScanOptions tune a scan.
type ScanOptions struct {
	Source string // Antenna | Cable: for tuners that ask
}

// ErrBusy: every tuner of the source is in use.
var ErrBusy = errors.New("all of this device's tuners are in use")

// ErrUnreachable: a connection failed. What's wrapped with it says what's on the network, for the log.
var ErrUnreachable = errors.New("the stream could not be reached")

// EncryptedError says a channel was refused because it is encrypted or copy
// protected. It is never decrypted.
type EncryptedError struct {
	Channel string
	Why     string
}

func (e *EncryptedError) Error() string {
	name := e.Channel
	if name == "" {
		name = "This channel"
	}
	return fmt.Sprintf("%s: %s. %s (%s). %s", name, EncryptedLabel, encryptedBecause, e.Why, encryptedPolicy)
}

// What every client shows for an encrypted channel, so that nobody takes it
// for a fault in their setup.
const (
	EncryptedLabel   = "Encrypted: can't be played here"
	encryptedBecause = "The broadcaster protects this channel"
	encryptedPolicy  = "WittyMediaServer does not decrypt or bypass protected broadcasts, so it can't be played or recorded. Nothing is wrong with your setup."
	// EncryptedExplain is the same in one line, for lists.
	EncryptedExplain = encryptedBecause + ". " + encryptedPolicy
)

// IsEncrypted reports whether err means a channel was refused for encryption.
func IsEncrypted(err error) bool {
	var e *EncryptedError
	return errors.As(err, &e) || errors.Is(err, mpegts.ErrScrambled)
}

// ScrambledReason says why a stream stopped by the guard was refused.
const ScrambledReason = "the signal itself is scrambled"

// RefusalMessage turns a stream that a guard stopped into words for a person.
func RefusalMessage(name string, err error) error {
	if errors.Is(err, mpegts.ErrScrambled) {
		return &EncryptedError{Channel: name, Why: ScrambledReason}
	}
	return err
}

// Field describes one setting of a driver; forms are drawn from these.
type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // text | url | path | password | select | number | textarea | toggle
	Help     string   `json:"help,omitempty"`
	Default  string   `json:"default,omitempty"`
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"`
}

// Driver describes one kind of tuner.
type Driver struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Status is how well this has been tested: "tested", "mock" (against mock
	// servers, no real device) or "untested".
	Status string `json:"status"`
	// Platforms it can run on; empty = all.
	Platforms []string `json:"platforms,omitempty"`
	Fields    []Field  `json:"fields"`
	Scans     bool     `json:"scans"` // Scan does more than list what is known
	// Planned drivers are listed so people can see they are known about.
	Planned bool `json:"planned,omitempty"`
	// Available is worked out for this machine.
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`
}

// Found is a tuner discovered on this network or machine, ready to add.
type Found struct {
	Driver  string            `json:"driver"`
	Name    string            `json:"name"`
	Detail  string            `json:"detail,omitempty"`
	Options map[string]string `json:"options"`
}
