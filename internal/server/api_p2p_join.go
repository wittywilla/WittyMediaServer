// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"net/http"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// POST /api/p2p/join {code, userId, password, otp}: the code a device away
// from home is showing, typed in by someone signed in. It is an invite the
// other way round, for a device that can't take a link in: a TV has no
// keyboard worth the name and no camera. The server makes itself findable by
// whoever shows that code (internal/p2p/join.go says how, and why a stranger
// gets nothing from it), puts that device's key on the list, and signs it in
// as this account or a Kids or Guest profile it manages.
//
// What the code is worth is what an invite is worth: once, for fifteen
// minutes, and only while whoever typed it stays signed in. Signing a device
// in as an administrator takes the administrator's password again, as it does
// for a code approved at home.
func (s *Server) handleP2PJoin(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	// A profile kept at home has no use for a way in from away, and isn't given one.
	if !accessFor(me).Can(library.FeatureRemote) {
		writeErr(w, 403, featureRefusal[library.FeatureRemote])
		return
	}
	if !mayPair(me) {
		writeErr(w, 403, "this profile can't sign devices in")
		return
	}
	var b struct {
		Code     string `json:"code"`
		UserID   int64  `json:"userId"`
		Password string `json:"password"`
		OTP      string `json:"otp"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	code, err := p2p.CleanJoinCode(b.Code)
	if err != nil {
		writeErr(w, 400, "That isn't a code a device shows. It has eight letters and digits, like K7QM-2XHD.")
		return
	}
	target, ok := s.pairTarget(r, me, b.UserID)
	if !ok {
		writeErr(w, 403, "you can't sign a device in as that profile")
		return
	}
	node := s.p2pNode()
	if node == nil {
		writeErr(w, 409, "Peer-to-peer connections are off. An administrator can turn them on in Settings, Remote Access.")
		return
	}
	if !node.TakesCodes() {
		writeErr(w, 409, "A device away from home finds this server by its code through the address book, which is turned off here. An administrator can turn it on in Settings, Remote Access; or use an invite link instead.")
		return
	}
	if !s.confirmAdminGrant(w, r, target, b.Password, b.OTP) {
		return
	}
	secret, err := s.p2p.invites.make(me.ID, target.ID, tokenFrom(r))
	if err == nil {
		// The invite behind the code never leaves the server, and still has no place in a log.
		logging.AddSecret(secret)
		err = node.OfferJoin(code, secret)
	}
	switch {
	case errors.Is(err, errPairBusy), errors.Is(err, p2p.ErrJoinBusy):
		w.Header().Set("Retry-After", "60")
		writeErr(w, 429, "Too many devices are waiting to join. Try again in a few minutes.")
		return
	case err != nil:
		writeErr(w, 500, err.Error())
		return
	}
	s.Log.InfoContext(r.Context(), "a device's code was typed in, to join peer to peer", "cat", "auth", "by", me.Name, "for", target.Name)
	writeJSON(w, 200, map[string]any{"ok": true, "expiresInSec": int(p2p.JoinTTL.Seconds()), "user": target})
}
