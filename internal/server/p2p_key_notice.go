// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"github.com/wittywilla/wittymediaserver/internal/notify"
)

// A server that has devices on its peer-to-peer list but no connection key
// makes a new key, and so has a new connection ID, which none of those devices
// knows. It happens after restoring a backup made without a password: the key
// is the server's private key, and such a backup never holds it (whoever had
// it could pass for this server to every device). Before, nothing said so: the
// list still looked paired, and every device away from home simply failed
// (audit F8, #746).
//
// Now the moment is kept (p2pKeyNewSetting), the administrators are told, the
// Remote Access page says so until it no longer matters, and each device that
// hasn't connected since is shown as waiting for a new invite. A device that
// connects again, having learnt the new ID at home or by a new invite, is
// back to normal by itself; one removed from the list is gone from the count.

const p2pKeyNewSetting = "p2pKeyNew"

// p2pKeyNew is when the server last had to make a new connection key while
// devices were on its list.
type p2pKeyNew struct {
	At      int64 `json:"at"`
	Devices int   `json:"devices"` // how many were on the list then
}

// p2pKeyNotice is what the Remote Access page shows about it.
type p2pKeyNotice struct {
	At       int64  `json:"at"`
	Reinvite int    `json:"reinvite"` // devices still waiting for a new invite
	Words    string `json:"words"`
}

func (s *Server) p2pKeyNewAt(ctx context.Context) (p2pKeyNew, bool) {
	raw := s.DB.GetSetting(ctx, p2pKeyNewSetting, "")
	if raw == "" {
		return p2pKeyNew{}, false
	}
	var k p2pKeyNew
	if err := json.Unmarshal([]byte(raw), &k); err != nil || k.At <= 0 {
		return p2pKeyNew{}, false
	}
	return k, true
}

// p2pNoteNewKey is called when a new connection key has just been made. With
// devices on the list, it keeps the moment and tells the administrators.
func (s *Server) p2pNoteNewKey(ctx context.Context) {
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM p2p_devices`).Scan(&n); err != nil || n == 0 {
		return // a first key: nobody knew the old one
	}
	k := p2pKeyNew{At: time.Now().Unix(), Devices: n}
	raw, _ := json.Marshal(k)
	if err := s.DB.SetSetting(ctx, p2pKeyNewSetting, string(raw)); err != nil {
		s.Log.WarnContext(ctx, "couldn't keep when this server's connection ID changed", "cat", "p2p", "err", err)
	}
	s.Log.WarnContext(ctx, "this server had no peer-to-peer key, though devices are on its list (a backup made without a password was restored?): it has a new connection ID, and those devices must be invited again", "cat", "p2p", "devices", n)
	if s.notes != nil {
		s.notes.Send(ctx, s.noteAdmins(ctx), notify.Note{Kind: notify.Server,
			Title: i18n.T("This server has a new connection ID"),
			Body: i18n.F("The backup it was restored from was made without a password, which can't keep it. {n} devices that connect from away must be invited again.",
				"n", strconv.Itoa(n)),
			Link: "/settings/remote"}, "p2pkey:"+strconv.FormatInt(k.At, 10))
	}
}

// p2pNeedsReinvite says a device hasn't connected since the server's key last
// changed under it.
func p2pNeedsReinvite(d p2pDevice, k p2pKeyNew, have bool) bool {
	return have && d.Created < k.At && d.LastSeen < k.At
}

// p2pReinviteWords is what a device waiting for a new invite says, in the list.
func p2pReinviteWords() string {
	return i18n.T("This server has a new connection ID since a backup made without a password was restored. Invite this device again.")
}

// p2pKeyNoticeFor is the page's notice, or nil once no device waits any more,
// when what was kept is let go of.
func (s *Server) p2pKeyNoticeFor(ctx context.Context, devices []p2pDevice) *p2pKeyNotice {
	k, have := s.p2pKeyNewAt(ctx)
	if !have {
		return nil
	}
	n := 0
	for _, d := range devices {
		if p2pNeedsReinvite(d, k, true) {
			n++
		}
	}
	if n == 0 {
		_ = s.DB.SetSetting(ctx, p2pKeyNewSetting, "")
		return nil
	}
	return &p2pKeyNotice{At: k.At, Reinvite: n,
		Words: i18n.F("This server has a new connection ID, because the backup it was restored from was made without a password. {n} devices that connect from away must be invited again: they show as waiting below.", "n", strconv.Itoa(n))}
}

// p2pBackupKeyNote is the backup screen's line while connections from away
// are on: a backup made without a password can't keep the connection ID.
func (s *Server) p2pBackupKeyNote(ctx context.Context) string {
	if !s.loadP2P(ctx).Enabled {
		return ""
	}
	return i18n.T("Without a password, a backup can't keep this server's connection ID: after restoring it, devices that connect from away must be invited again.")
}
