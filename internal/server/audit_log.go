// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// The server's audit log: what was done to which code runs (module store actions,
// overrides) and what a module asked the server to write down for it (custom cores,
// refused passwords). Kept by the server, so a module can't take its own rows out.

// maxAuditRows is how many rows the audit log keeps; the oldest go first.
const maxAuditRows = 5000

// CoreAudit is one row of the audit log.
type CoreAudit struct {
	At     int64  `json:"at"`
	User   string `json:"user"`
	Action string `json:"action"`
	Core   string `json:"core,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// writeAudit writes down something that happened, and also logs it, so it reaches the
// log files and survives someone clearing the database. module is the module the row
// belongs to ("" for the server's own rows).
func (s *Server) writeAudit(ctx context.Context, u *auth.User, kind, module, action, core, detail string) {
	name := "the server"
	if u != nil {
		name = u.Name
	}
	if len(detail) > 800 {
		detail = detail[:800]
	}
	// Written with its own context: a request that has gone away still happened.
	bg := context.WithoutCancel(ctx)
	if _, err := s.DB.ExecContext(bg, `INSERT INTO game_core_audit(at, user_name, action, core, detail, module) VALUES(?,?,?,?,?,?)`, time.Now().Unix(), name, action, core, detail, module); err != nil {
		s.Log.WarnContext(bg, "could not write an audit row", "cat", auditCat(kind), "err", err)
	}
	if _, err := s.DB.ExecContext(bg, `DELETE FROM game_core_audit WHERE id <= (SELECT MAX(id) FROM game_core_audit) - ?`, maxAuditRows); err != nil {
		s.Log.DebugContext(bg, "could not trim the audit rows", "cat", auditCat(kind), "err", err)
	}
	// In the log too, whatever its levels say: an override or a custom core is what an
	// owner may later need to show was done, and by whom.
	logging.Audit(bg, s.Log, "audit record", "cat", auditCat(kind), "kind", kind, "module", module, "action", action, "by", name, "core", core, "detail", detail)
}

// auditCat is the area an audit row's log line belongs to.
func auditCat(kind string) string {
	if kind == "module" {
		return "modules-store"
	}
	return "games"
}

// coreAudit is one module's rows of the audit log, newest first.
func (s *Server) coreAudit(ctx context.Context, module string, limit int) []CoreAudit {
	out := []CoreAudit{}
	rows, err := s.DB.QueryContext(ctx, `SELECT at, user_name, action, core, detail FROM game_core_audit WHERE module = ? ORDER BY id DESC LIMIT ?`, module, limit)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var a CoreAudit
		if rows.Scan(&a.At, &a.User, &a.Action, &a.Core, &a.Detail) == nil {
			out = append(out, a)
		}
	}
	return out
}
