// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// What of pairing outlives a restart (#683, #692). Before, all of it lived only
// in memory: an owner who restarted the server (an update, a setting that
// wanted it) between showing an invite and the phone using it left the phone
// with "That invite isn't right", and nothing on either screen said why.
//
// Invite links are kept, in one setting, until each runs out (fifteen
// minutes), with the same rules after a restart as before it: once, until the
// same deadline, and only while whoever made it stays signed in. What is kept
// of each is its secret's hash, never the secret; who made it and for whom;
// whether it is used and by which device; its enrol key's ID, which is public,
// so it is listed with the server's relays again at once; and the sign-in that
// made it as that sign-in's hash (auth.StoredToken), which proves it is still
// signed in without being a sign-in itself.
//
// Codes are not kept: a short code, or the keys made from it, is worth the code
// itself, and a code that dies with a rare restart is the better trade. What is
// kept instead is only who had one open and until when, so that the device
// typing it is told the server restarted (reason "restarted") rather than that
// the code is wrong, and the person's Connect a device screen shows a new one
// (GET /api/connect/offer). A code a TV showed, typed into the server, lives in
// memory and is offered again to a node started again by a change of settings,
// but not across a restart of the server.
//
// None of it is ever in a backup (secretSettings).

const p2pPendingKey = "p2pPending"

func init() { secretSettings = append(secretSettings, p2pPendingKey) }

// p2pPendingStore is the saving side.
type p2pPendingStore struct {
	mu    sync.Mutex // one save at a time, so what is written last is the latest
	jmu   sync.Mutex
	joins map[string]savedJoin // codes typed into the server, by lookup key: in memory only
	// offered is the node they were last offered to: only that node's word
	// that one is no longer waiting (taken, or given up) drops it.
	offered *p2p.Node
	// codesBefore is who had a code open when the server last stopped, and
	// until when it would have lasted.
	codesBefore map[int64]time.Time
}

type savedInvite struct {
	Hash    string `json:"hash"`
	By      int64  `json:"by"`
	UserID  int64  `json:"userId"`
	Session string `json:"session"`
	Expires int64  `json:"expires"`
	Used    bool   `json:"used,omitempty"`
	UsedBy  string `json:"usedBy,omitempty"`
	Reshows int    `json:"reshows,omitempty"`
	EnrolID string `json:"enrolId,omitempty"`
	Spent   bool   `json:"spent,omitempty"` // the code beside it was used
}

// savedCodes is that someone had a code open, and until when: nothing of the code.
type savedCodes struct {
	By    int64 `json:"by"`
	Until int64 `json:"until"`
}

type savedPending struct {
	V           int           `json:"v"`
	Invites     []savedInvite `json:"invites,omitempty"`
	CodesBefore []savedCodes  `json:"codesBefore,omitempty"`
}

// savedJoin is a code a TV showed, typed into the server, as its node needs it again.
type savedJoin struct {
	Find    string
	Proof   []byte
	Ticket  string // the hash of the invite behind the code
	Expires int64
}

// p2pSavePending writes what is open now. It is called after every change,
// which is a few times for each device that joins.
func (s *Server) p2pSavePending() {
	st := &s.p2p.pending
	st.mu.Lock()
	defer st.mu.Unlock()
	out := s.p2pPendingSnapshot(time.Now())
	raw := ""
	if len(out.Invites)+len(out.CodesBefore) > 0 {
		b, err := json.Marshal(out)
		if err != nil {
			return
		}
		raw = string(b)
	} else if s.DB.GetSetting(context.Background(), p2pPendingKey, "") == "" {
		return // nothing open, and nothing kept: nothing to write
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.DB.SetSetting(ctx, p2pPendingKey, raw); err != nil {
		s.Log.Warn("open invites couldn't be saved; a restart would end them", "cat", "p2p", "err", err)
	}
}

func (s *Server) p2pPendingSnapshot(now time.Time) savedPending {
	out := savedPending{V: 1}
	in := &s.p2p.invites
	in.mu.Lock()
	for _, v := range in.open {
		if now.After(v.expires) {
			continue
		}
		out.Invites = append(out.Invites, savedInvite{Hash: v.hash, By: v.by, UserID: v.userID, Session: auth.StoredToken(v.session),
			Expires: v.expires.Unix(), Used: v.used, UsedBy: v.usedBy, Reshows: v.reshows, EnrolID: v.enrolID,
			Spent: v.spent != nil && v.spent.Load()})
	}
	in.mu.Unlock()
	codes := map[int64]time.Time{}
	co := &s.connect
	co.mu.Lock()
	for _, o := range co.open {
		if now.Before(o.expires) && !o.spent.Load() {
			codes[o.by] = maxTime(codes[o.by], o.expires)
		}
	}
	co.mu.Unlock()
	for by, until := range codes {
		out.CodesBefore = append(out.CodesBefore, savedCodes{By: by, Until: until.Unix()})
	}
	st := &s.p2p.pending
	st.jmu.Lock()
	node := st.offered
	if node != s.p2p.nodeRef.Load() {
		node = nil // a node that hasn't been offered them yet knows nothing of them
	}
	for k, j := range st.joins {
		// Taken by its device, or given up after wrong guesses, on the node that has it.
		if now.Unix() >= j.Expires || (node != nil && !node.JoinWaiting(k)) {
			delete(st.joins, k)
		}
	}
	st.jmu.Unlock()
	return out
}

// p2pLoadPending puts back the invites open before a restart. It runs before
// the node starts, so their enrol keys are on the first list its relays hear.
func (s *Server) p2pLoadPending(ctx context.Context) {
	raw := s.DB.GetSetting(ctx, p2pPendingKey, "")
	if raw == "" {
		return
	}
	var saved savedPending
	if err := json.Unmarshal([]byte(raw), &saved); err != nil || saved.V != 1 {
		s.Log.WarnContext(ctx, "the invites kept from before the restart are unreadable; they are ended", "cat", "p2p")
		_ = s.DB.SetSetting(ctx, p2pPendingKey, "")
		return
	}
	now := time.Now()
	last := time.Time{}
	invites := 0
	in := &s.p2p.invites
	in.mu.Lock()
	if in.open == nil {
		in.open = map[string]*p2pInvite{}
	}
	for _, v := range saved.Invites {
		until := time.Unix(v.Expires, 0)
		if !now.Before(until) || v.Hash == "" || in.open[v.Hash] != nil {
			continue
		}
		// A link whose code was used is used: one way in, after a restart as before it.
		in.open[v.Hash] = &p2pInvite{hash: v.Hash, by: v.By, userID: v.UserID, session: v.Session, expires: until, used: v.Used || v.Spent,
			usedBy: v.UsedBy, reshows: v.Reshows, enrolID: v.EnrolID}
		if !v.Used && !v.Spent {
			invites++
			last = maxTime(last, until)
		}
	}
	in.mu.Unlock()
	st := &s.p2p.pending
	st.jmu.Lock()
	for _, c := range saved.CodesBefore {
		if until := time.Unix(c.Until, 0); now.Before(until) {
			if st.codesBefore == nil {
				st.codesBefore = map[int64]time.Time{}
			}
			st.codesBefore[c.By] = until
		}
	}
	codes := len(st.codesBefore)
	st.jmu.Unlock()
	if !last.IsZero() {
		// Their enrol keys go from the relays when they run out, as they would have without the restart.
		time.AfterFunc(time.Until(last)+time.Second, s.p2pDevicesChanged)
	}
	if invites+codes > 0 {
		s.Log.InfoContext(ctx, "invites made before the restart are open again; codes shown before it have ended", "cat", "p2p", "invites", invites, "codesEnded", codes)
	}
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// p2pCodeEndedByRestart says a code shown before the last restart would still
// have been good: by someone (by > 0), or by anyone (by == 0), as a device
// typing an unknown code can't say whose it is.
func (s *Server) p2pCodeEndedByRestart(by int64) bool {
	st := &s.p2p.pending
	st.jmu.Lock()
	defer st.jmu.Unlock()
	now := time.Now()
	for who, until := range st.codesBefore {
		if !now.Before(until) {
			delete(st.codesBefore, who)
			continue
		}
		if by == 0 || who == by {
			return true
		}
	}
	return false
}

// p2pCodeRestartedWords is what a device typing a code from before a restart is told.
const p2pCodeRestartedWords = "This code expired because the server restarted. Show a new code on the server."

// GET /api/connect/offer: whether the code this person showed is still open,
// and if not, whether a restart ended it: the Connect a device screen then
// shows a new one by itself.
func (s *Server) handleConnectOfferState(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	open := false
	now := time.Now()
	s.connect.mu.Lock()
	for _, o := range s.connect.open {
		if o.by == me.ID && now.Before(o.expires) && !o.spent.Load() {
			open = true
		}
	}
	s.connect.mu.Unlock()
	writeJSON(w, 200, map[string]any{"open": open, "restarted": !open && s.p2pCodeEndedByRestart(me.ID)})
}

// p2pRememberJoin keeps a code typed into node, in memory, for the next node.
func (s *Server) p2pRememberJoin(node *p2p.Node, find string, proof []byte, ticket string, expires time.Time) {
	st := &s.p2p.pending
	st.jmu.Lock()
	defer st.jmu.Unlock()
	if st.joins == nil {
		st.joins = map[string]savedJoin{}
	}
	if st.offered != node {
		st.offered = nil // the node has this one, not the others: they are offered to it on its next start
	}
	st.joins[find] = savedJoin{Find: find, Proof: proof, Ticket: ticket, Expires: expires.Unix()}
}

// applyP2PWithJoins is applyP2P, and then offers the node it leaves running
// the codes typed in before it started: after a change of settings that
// started it again.
func (s *Server) applyP2PWithJoins(ctx context.Context) error {
	err := s.applyP2P(ctx)
	if node := s.p2pNode(); node != nil {
		s.p2pOfferSavedJoins(node)
	}
	return err
}

// p2pOfferSavedJoins offers a node the codes typed in before it, once.
func (s *Server) p2pOfferSavedJoins(node *p2p.Node) {
	st := &s.p2p.pending
	st.jmu.Lock()
	if st.offered == node {
		st.jmu.Unlock()
		return
	}
	st.offered = node
	list := make([]savedJoin, 0, len(st.joins))
	for _, j := range st.joins {
		list = append(list, j)
	}
	st.jmu.Unlock()
	for _, j := range list {
		if err := node.OfferJoinKeys(j.Find, j.Proof, j.Ticket, time.Unix(j.Expires, 0)); err != nil {
			s.Log.Info("a code typed in before peer-to-peer connections started again couldn't be offered again", "cat", "p2p", "err", err)
		}
	}
}

// p2pForgetSavedJoins ends every code typed into the server.
func (s *Server) p2pForgetSavedJoins() {
	st := &s.p2p.pending
	st.jmu.Lock()
	st.joins = nil
	st.jmu.Unlock()
}
