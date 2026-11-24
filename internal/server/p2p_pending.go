// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// Invites and codes outlive a restart (#683). Before, they lived only in
// memory: an owner who restarted the server (an update, a setting that wanted
// it) between showing an invite and the phone using it left the phone with
// "That invite isn't right", and nothing on either screen said why.
//
// What is kept, in one setting, until each runs out (fifteen minutes):
//
//   - an invite link's hash, never the secret, with who made it, for whom, and
//     whether it is used and by which device; its enrol key's ID, which is
//     public, so it is listed with the server's relays again at once;
//   - a code's lookup key and the key its proofs are made with (not the code),
//     for a code shown on the Connect a device screen, and for a code a device
//     away from home showed and someone typed into the server;
//   - the sign-in that made each, as its hash (auth.StoredToken), which proves
//     it is still signed in without being a sign-in itself.
//
// A code's proof key is worth what the code is, for as long as the code is: it
// is never in a backup (secretSettings), and goes from the setting as soon as
// the code is used or runs out.

const p2pPendingKey = "p2pPending"

func init() { secretSettings = append(secretSettings, p2pPendingKey) }

// p2pPendingStore is the saving side.
type p2pPendingStore struct {
	mu    sync.Mutex // one save at a time, so what is written last is the latest
	jmu   sync.Mutex
	joins map[string]savedJoin // codes typed into the server, by lookup key
	// offered is the node they were last offered to: only that node's word
	// that one is no longer waiting (taken, or given up) drops it.
	offered *p2p.Node
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
	Group   string `json:"group,omitempty"`
	Spent   bool   `json:"spent,omitempty"`
}

type savedOffer struct {
	Find    string `json:"find"`
	Proof   []byte `json:"proof"`
	By      int64  `json:"by"`
	UserID  int64  `json:"userId"`
	Session string `json:"session"`
	Expires int64  `json:"expires"`
	Wrong   int    `json:"wrong,omitempty"`
	Group   string `json:"group,omitempty"`
}

type savedJoin struct {
	Find    string `json:"find"`
	Proof   []byte `json:"proof"`
	Ticket  string `json:"ticket"` // the hash of the invite behind the code
	Expires int64  `json:"expires"`
}

type savedPending struct {
	V       int           `json:"v"`
	Invites []savedInvite `json:"invites,omitempty"`
	Offers  []savedOffer  `json:"offers,omitempty"`
	Joins   []savedJoin   `json:"joins,omitempty"`
}

// p2pSavePending writes what is open now. It is called after every change,
// which is a few times for each device that joins.
func (s *Server) p2pSavePending() {
	st := &s.p2p.pending
	st.mu.Lock()
	defer st.mu.Unlock()
	out := s.p2pPendingSnapshot(time.Now())
	raw := ""
	if len(out.Invites)+len(out.Offers)+len(out.Joins) > 0 {
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
		s.Log.Warn("open invites and codes couldn't be saved; a restart would end them", "cat", "p2p", "err", err)
	}
}

func (s *Server) p2pPendingSnapshot(now time.Time) savedPending {
	out := savedPending{V: 1}
	// A link and the code beside it share what spends them; saved, the pair is named by a number.
	groups := map[*atomic.Bool]string{}
	group := func(b *atomic.Bool) string {
		if b == nil {
			return ""
		}
		if groups[b] == "" {
			groups[b] = strconv.Itoa(len(groups) + 1)
		}
		return groups[b]
	}
	in := &s.p2p.invites
	in.mu.Lock()
	for _, v := range in.open {
		if now.After(v.expires) {
			continue
		}
		out.Invites = append(out.Invites, savedInvite{Hash: v.hash, By: v.by, UserID: v.userID, Session: auth.StoredToken(v.session),
			Expires: v.expires.Unix(), Used: v.used, UsedBy: v.usedBy, Reshows: v.reshows, EnrolID: v.enrolID, Group: group(v.spent),
			Spent: v.spent != nil && v.spent.Load()})
	}
	in.mu.Unlock()
	co := &s.connect
	co.mu.Lock()
	for _, o := range co.open {
		if now.After(o.expires) || o.spent.Load() {
			continue
		}
		out.Offers = append(out.Offers, savedOffer{Find: o.find, Proof: o.proof, By: o.by, UserID: o.userID, Session: auth.StoredToken(o.session),
			Expires: o.expires.Unix(), Wrong: o.wrong, Group: group(o.spent)})
	}
	co.mu.Unlock()
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
			continue
		}
		out.Joins = append(out.Joins, j)
	}
	st.jmu.Unlock()
	return out
}

// p2pLoadPending puts back what was open before a restart. It runs before the
// node starts, so the invites' enrol keys are on the first list its relays hear.
func (s *Server) p2pLoadPending(ctx context.Context) {
	raw := s.DB.GetSetting(ctx, p2pPendingKey, "")
	if raw == "" {
		return
	}
	var saved savedPending
	if err := json.Unmarshal([]byte(raw), &saved); err != nil || saved.V != 1 {
		s.Log.WarnContext(ctx, "the invites and codes kept from before the restart are unreadable; they are ended", "cat", "p2p")
		_ = s.DB.SetSetting(ctx, p2pPendingKey, "")
		return
	}
	now := time.Now()
	groups := map[string]*atomic.Bool{}
	group := func(name string) *atomic.Bool {
		if name == "" {
			return nil
		}
		if groups[name] == nil {
			groups[name] = &atomic.Bool{}
		}
		return groups[name]
	}
	last := time.Time{}
	invites, offers, joins := 0, 0, 0
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
		spent := group(v.Group)
		if spent != nil && v.Spent {
			spent.Store(true)
		}
		in.open[v.Hash] = &p2pInvite{hash: v.Hash, by: v.By, userID: v.UserID, session: v.Session, expires: until, used: v.Used,
			usedBy: v.UsedBy, reshows: v.Reshows, enrolID: v.EnrolID, spent: spent}
		if !v.Used {
			invites++
			last = maxTime(last, until)
		}
	}
	in.mu.Unlock()
	co := &s.connect
	co.mu.Lock()
	if co.open == nil {
		co.open = map[string]*connectOffer{}
	}
	for _, o := range saved.Offers {
		until := time.Unix(o.Expires, 0)
		spent := group(o.Group)
		if spent == nil {
			spent = &atomic.Bool{}
		}
		if !now.Before(until) || spent.Load() || len(o.Find) != 64 || len(o.Proof) == 0 || co.open[o.Find] != nil {
			continue
		}
		co.open[o.Find] = &connectOffer{find: o.Find, proof: o.Proof, by: o.By, userID: o.UserID, session: o.Session, expires: until,
			wrong: o.Wrong, asks: map[string]time.Time{}, spent: spent}
		offers++
	}
	co.mu.Unlock()
	st := &s.p2p.pending
	st.jmu.Lock()
	if st.joins == nil {
		st.joins = map[string]savedJoin{}
	}
	for _, j := range saved.Joins {
		if now.Unix() < j.Expires && j.Find != "" && len(j.Proof) > 0 {
			st.joins[j.Find] = j
			joins++
		}
	}
	st.jmu.Unlock()
	if !last.IsZero() {
		// Their enrol keys go from the relays when they run out, as they would have without the restart.
		time.AfterFunc(time.Until(last)+time.Second, s.p2pDevicesChanged)
	}
	if invites+offers+joins > 0 {
		s.Log.InfoContext(ctx, "invites and codes made before the restart are open again", "cat", "p2p", "invites", invites, "codes", offers, "deviceCodes", joins)
	}
}

func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// p2pRememberJoin keeps a code typed into node for a restart.
func (s *Server) p2pRememberJoin(node *p2p.Node, find string, proof []byte, ticket string, expires time.Time) {
	st := &s.p2p.pending
	st.jmu.Lock()
	if st.joins == nil {
		st.joins = map[string]savedJoin{}
	}
	if st.offered != node {
		st.offered = nil // the node has this one, not the others: they are offered to it on its next start
	}
	st.joins[find] = savedJoin{Find: find, Proof: proof, Ticket: ticket, Expires: expires.Unix()}
	st.jmu.Unlock()
	s.p2pSavePending()
}

// applyP2PWithJoins is applyP2P, and then offers the node it leaves running
// the codes typed in before it started: after a restart, or a change of
// settings that started it again.
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
			s.Log.Info("a code typed in before the restart couldn't be offered again", "cat", "p2p", "err", err)
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
