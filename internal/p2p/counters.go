// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package p2p

import "sync/atomic"

// Counters are what the server's node has done since it started, for the
// status page and the diagnostics: enough to tell "nobody tried" from "every
// try was refused" from "it all went through a relay".
type Counters struct {
	StreamsDirect    int64 `json:"streamsDirect"`  // HTTP connections over a direct path
	StreamsRelayed   int64 `json:"streamsRelayed"` // HTTP connections through a relay
	RefusedNotListed int64 `json:"refusedNotListed"`
	RefusedTooMany   int64 `json:"refusedTooMany"`
	Enrols           int64 `json:"enrols"`        // invites shown
	EnrolsRefused    int64 `json:"enrolsRefused"` // over the limits
	PathChanges      int64 `json:"pathChanges"`   // a device moving between direct and relayed
	RelayedBytesSent int64 `json:"relayedBytesSent"`
	Registrations    int64 `json:"registrations"`       // with the owner's WittyRelays
	RegistrationsBad int64 `json:"registrationsFailed"` // refused or unanswered
}

type counters struct {
	streamsDirect, streamsRelayed, refusedNotListed, refusedTooMany atomic.Int64
	enrols, enrolsRefused, pathChanges, relayedBytes                atomic.Int64
	registrations, registrationsBad                                 atomic.Int64
}

func (c *counters) snapshot() Counters {
	return Counters{
		StreamsDirect: c.streamsDirect.Load(), StreamsRelayed: c.streamsRelayed.Load(),
		RefusedNotListed: c.refusedNotListed.Load(), RefusedTooMany: c.refusedTooMany.Load(),
		Enrols: c.enrols.Load(), EnrolsRefused: c.enrolsRefused.Load(), PathChanges: c.pathChanges.Load(),
		RelayedBytesSent: c.relayedBytes.Load(), Registrations: c.registrations.Load(), RegistrationsBad: c.registrationsBad.Load(),
	}
}
