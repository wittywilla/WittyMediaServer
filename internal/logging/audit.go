// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package logging

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"time"
)

// AuditKey marks a record that must be kept whatever the levels and the flood
// limit say: a manual override of a module, a device revoked. The file and the
// viewer show it as the attribute audit=yes.
const AuditKey = "audit"

// Audit writes a record that is never silenced: not by an area set to Off, not by
// a module's own level, not by the flood limit. Use it for what an owner may later
// need to prove happened, and who did it ("by"). Secrets are still redacted.
func Audit(ctx context.Context, log *slog.Logger, msg string, args ...any) {
	if log == nil {
		log = slog.Default()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var pcs [1]uintptr
	runtime.Callers(2, pcs[:])
	r := slog.NewRecord(time.Now(), slog.LevelInfo, msg, pcs[0])
	r.Add(AuditKey, "yes")
	r.Add(args...)
	// Straight to the handler: Enabled would ask the area's level, which an audit
	// record doesn't answer to.
	_ = log.Handler().Handle(ctx, r)
}

// IsAudit says whether a record was written with Audit.
func (rec *Record) IsAudit() bool {
	if rec.audit {
		return true
	}
	for _, a := range rec.Attrs {
		if a.K == AuditKey && a.V == "yes" {
			return true
		}
	}
	return false
}

// SessionKind is one kind of work whose records share an ID from start to end, on
// the server, in modules and in the apps, so the viewer can follow one of them.
type SessionKind struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Prefix string   `json:"prefix"` // what NewID puts in front of its IDs, if it makes them
	Cats   []string `json:"cats"`   // the areas whose records with a session belong to it
}

// SessionKinds are what the viewer's "Follow" offers.
var SessionKinds = []SessionKind{
	{"game", "A game", "game", []string{"games", "emulators"}},
	{"party", "A watch party", "party", nil},
	{"rc", "A remote-control command", "rc", nil},
	{"p2p", "A peer-to-peer connection", "p2p", []string{"p2p"}},
	{"sep", "A karaoke separation", "sep", []string{"extras"}},
	{"audit", "Audit records", "", nil},
}

// sessionKind finds a kind by its ID.
func sessionKind(id string) *SessionKind {
	for i := range SessionKinds {
		if SessionKinds[i].ID == id {
			return &SessionKinds[i]
		}
	}
	return nil
}

// belongs says whether a record is part of one of this kind's sessions.
func (k *SessionKind) belongs(rec *Record) bool {
	if k.ID == "audit" {
		return rec.IsAudit()
	}
	if rec.Session == "" {
		return false
	}
	if k.Prefix != "" && strings.HasPrefix(rec.Session, k.Prefix+"-") {
		return true
	}
	for _, c := range k.Cats {
		if rec.Cat == c {
			return true
		}
	}
	return false
}
