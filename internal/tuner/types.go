// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package tuner is what the server knows of a TV tuner's stream: a stream WittyTuner handed over,
// and how a server words a refusal. WittyTuner itself is a module, reached through internal/modules;
// its tuners, channels and guide sources are its own.

package tuner

import (
	"errors"
	"fmt"

	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
)

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
