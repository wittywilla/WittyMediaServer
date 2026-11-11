// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/rand"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/play/stream"
)

// Seats: playing one game together from several devices (docs/GAMING-API.md, Seats).
//
// The account that starts a game is its host. The host's own devices watch and play it as they
// always could. Anyone else signed in to this server needs a code from the host: four letters,
// good for ten minutes. Adults, Kids and Guest profiles all join the same way, and only with a
// code. A profile that couldn't open the game itself, because of its parental limits, can't join
// it either. Saves, moments and play time stay the host's: only the host saves, loads, changes
// settings or stops the game. Someone who joins may watch it and send their own seat's controls.

const (
	seatCodeLetters = "BCDFGHJKLMNPQRSTVWXZ" // the TV codes' letters: no vowels, so no words
	seatCodeLen     = 4
	seatCodeLife    = 10 * time.Minute
	seatJoinTries   = 10 // wrong codes one account may try in a window, so codes can't be guessed
	seatJoinWindow  = 10 * time.Minute
	seatsMax        = 8 // players a game can have; seat 0 is the host's
)

// seatHolder is someone who joined a game with a code.
type seatHolder struct {
	UserID int64  `json:"userId"`
	Name   string `json:"name"`
	// Seat is the player this person is, from 1, or -1 when every seat is taken and they can only
	// watch.
	Seat  int   `json:"seat"`
	Since int64 `json:"since"`
}

type seatTable struct {
	host        int64
	hostName    string
	device      string
	started     time.Time
	code        string
	codeExpires time.Time
	holders     []seatHolder
}

// seatBook keeps the hosts, codes and holders of the games running now. It forgets a game when the
// game ends.
type seatBook struct {
	mu     sync.Mutex
	tables map[string]*seatTable // by session ID
	codes  map[string]string     // code → session ID
	tries  map[int64][]time.Time // wrong codes, by account
	now    func() time.Time
}

func (b *seatBook) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

func (b *seatBook) init() {
	if b.tables == nil {
		b.tables, b.codes, b.tries = map[string]*seatTable{}, map[string]string{}, map[int64][]time.Time{}
	}
}

// started notes a new game, its host and the device that started it.
func (b *seatBook) started(sid string, host int64, hostName, device string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.init()
	b.tables[sid] = &seatTable{host: host, hostName: hostName, device: device, started: b.clock()}
}

// ended forgets a game and its code.
func (b *seatBook) ended(sid string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.init()
	if t := b.tables[sid]; t != nil && t.code != "" {
		delete(b.codes, t.code)
	}
	delete(b.tables, sid)
}

// invite makes a new code for a game, in place of any it had. Only its host may.
func (b *seatBook) invite(sid string, host int64) (string, time.Time, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.init()
	t := b.tables[sid]
	if t == nil || t.host != host {
		return "", time.Time{}, false
	}
	if t.code != "" {
		delete(b.codes, t.code)
	}
	code := newSeatCode()
	for b.codes[code] != "" {
		code = newSeatCode()
	}
	t.code, t.codeExpires = code, b.clock().Add(seatCodeLife)
	b.codes[code] = sid
	return code, t.codeExpires, true
}

// normaliseSeatCode accepts a code typed any way: any case, with spaces or a dash.
func normaliseSeatCode(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' {
			return -1
		}
		return r
	}, strings.ToUpper(strings.TrimSpace(s)))
}

// seatJoin is what joining answers.
type seatJoin struct {
	Session string `json:"sid"`
	Seat    int    `json:"seat"`
	Host    bool   `json:"host"` // the code was for one of this account's own games
}

type seatErr string

func (e seatErr) Error() string { return string(e) }

const (
	errSeatCode  = seatErr("That code isn’t right, or it has run out. Ask for a new one.")
	errSeatTries = seatErr("Too many wrong codes. Wait a few minutes, then ask for a new one.")
)

// join takes a code from someone signed in and gives them a seat in its game: the lowest one
// free, or watching only when every seat is taken.
func (b *seatBook) join(code string, user int64, name string) (seatJoin, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.init()
	now := b.clock()
	recent := slices.DeleteFunc(b.tries[user], func(t time.Time) bool { return now.Sub(t) > seatJoinWindow })
	b.tries[user] = recent
	if len(recent) >= seatJoinTries {
		return seatJoin{}, errSeatTries
	}
	sid := b.codes[normaliseSeatCode(code)]
	t := b.tables[sid]
	if t == nil || now.After(t.codeExpires) {
		b.tries[user] = append(recent, now)
		return seatJoin{}, errSeatCode
	}
	if t.host == user {
		return seatJoin{Session: sid, Seat: 0, Host: true}, nil
	}
	for _, h := range t.holders {
		if h.UserID == user {
			return seatJoin{Session: sid, Seat: h.Seat}, nil
		}
	}
	seat := -1
	for n := 1; n < seatsMax; n++ {
		if !slices.ContainsFunc(t.holders, func(h seatHolder) bool { return h.Seat == n }) {
			seat = n
			break
		}
	}
	t.holders = append(t.holders, seatHolder{UserID: user, Name: name, Seat: seat, Since: now.Unix()})
	return seatJoin{Session: sid, Seat: seat}, nil
}

// holds says whether someone joined a game with its code.
func (b *seatBook) holds(sid string, user int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.tables[sid]
	return t != nil && slices.ContainsFunc(t.holders, func(h seatHolder) bool { return h.UserID == user })
}

// seatOf is the seat someone joined a game in, as stream version 2 numbers players: 1 to 7, the
// host being 0, or -1 for watching only. False when they didn't join it with a code.
func (b *seatBook) seatOf(sid string, user int64) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if t := b.tables[sid]; t != nil {
		for _, h := range t.holders {
			if h.UserID == user {
				return h.Seat, true
			}
		}
	}
	return 0, false
}

// joinedSeats is what a joined device's version 2 hello asks for: the seat its code gave it, or
// none when it can only watch. Its own choice would be any player nobody holds.
func joinedSeats(seat int) []int {
	if seat < 0 {
		return []int{}
	}
	return []int{seat}
}

// leave frees someone's seat. The host may free anyone's, and anyone may leave.
func (b *seatBook) leave(sid string, by, user int64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.tables[sid]
	if t == nil || (by != t.host && by != user) {
		return false
	}
	before := len(t.holders)
	t.holders = slices.DeleteFunc(t.holders, func(h seatHolder) bool { return h.UserID == user })
	return len(t.holders) < before
}

// seatView is a game's seats as its host and holders see them.
type seatView struct {
	Host      seatHost     `json:"host"`
	Seats     []seatHolder `json:"seats"`
	Code      string       `json:"code,omitempty"`      // for the host, while it lasts
	ExpiresAt int64        `json:"expiresAt,omitempty"` // when the code runs out
}

type seatHost struct {
	Name      string `json:"name"`
	Device    string `json:"device,omitempty"`
	StartedAt int64  `json:"startedAt"`
}

func (b *seatBook) view(sid string, forHost bool) (seatView, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	t := b.tables[sid]
	if t == nil {
		return seatView{}, false
	}
	v := seatView{Host: seatHost{Name: t.hostName, Device: t.device, StartedAt: t.started.Unix()}, Seats: slices.Clone(t.holders)}
	if v.Seats == nil {
		v.Seats = []seatHolder{}
	}
	if forHost && t.code != "" && b.clock().Before(t.codeExpires) {
		v.Code, v.ExpiresAt = t.code, t.codeExpires.Unix()
	}
	return v, true
}

func newSeatCode() string {
	var sb strings.Builder
	n := big.NewInt(int64(len(seatCodeLetters)))
	for range seatCodeLen {
		i, err := rand.Int(rand.Reader, n)
		if err != nil {
			panic(err) // crypto/rand doesn't fail on any system this runs on
		}
		sb.WriteByte(seatCodeLetters[i.Int64()])
	}
	return sb.String()
}

// ---------------------------------------------------------------- routes

func (s *Server) seatRoutes(m routes) {
	a := s.requireUser
	m.Handle("GET /api/games/playing", a(s.handleGamesPlaying))
	m.Handle("POST /api/play/join", a(s.handlePlayJoin))
	m.Handle("POST /api/play/{sid}/invite", a(s.handlePlayInvite))
	m.Handle("GET /api/play/{sid}/seats", a(s.handlePlaySeats))
	m.Handle("DELETE /api/play/{sid}/seats/{user}", a(s.handlePlaySeatLeave))
}

// noteGameStarted keeps who started a game and on what, and forgets it when the game ends.
func (s *Server) noteGameStarted(sess *stream.Session, host int64, hostName, device string) {
	g := s.games()
	g.seats.started(sess.ID(), host, hostName, device)
	sid, done := sess.ID(), sess.Done()
	logging.Go("game seats", func() { <-done; g.seats.ended(sid) }, "cat", "games", "session", sid)
}

// watchSession finds a session this account may watch and play: its own, or one it joined with a
// code. Everything else about a session stays with its host (session).
func (s *Server) watchSession(r *http.Request) *stream.Session {
	if sess := s.session(r); sess != nil {
		return sess
	}
	if s.gameSvc == nil {
		return nil
	}
	sess := s.gameSvc.mgr.Get(r.PathValue("sid"))
	if sess == nil || !s.gameSvc.seats.holds(sess.ID(), userFrom(r).ID) {
		return nil
	}
	return sess
}

// handleGamesPlaying says what this account is playing now, and where: what a device needs to
// offer "Continue here".
func (s *Server) handleGamesPlaying(w http.ResponseWriter, r *http.Request) {
	if s.gameSvc == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	sess := s.gameSvc.mgr.ForUser(userFrom(r).ID)
	if sess == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	info := sess.Info()
	out := map[string]any{"id": info.ID, "itemId": info.ItemID, "title": info.Title, "console": info.Console}
	if v, ok := s.gameSvc.seats.view(sess.ID(), true); ok {
		out["device"], out["startedAt"], out["seats"] = v.Host.Device, v.Host.StartedAt, v.Seats
	}
	writeJSON(w, 200, out)
}

func (s *Server) handlePlayInvite(w http.ResponseWriter, r *http.Request) {
	sess := s.session(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	code, expires, ok := s.games().seats.invite(sess.ID(), userFrom(r).ID)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, map[string]any{"code": code, "expiresAt": expires.Unix()})
}

func (s *Server) handlePlayJoin(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Code string `json:"code"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if s.gameSvc == nil {
		writeErr(w, 404, errSeatCode.Error())
		return
	}
	u := userFrom(r)
	name := u.DisplayName
	if name == "" {
		name = u.Name
	}
	j, err := s.gameSvc.seats.join(b.Code, u.ID, name)
	if err == errSeatTries {
		w.Header().Set("Retry-After", "600")
		writeErr(w, 429, err.Error())
		return
	}
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	sess := s.gameSvc.mgr.Get(j.Session)
	if sess == nil {
		s.gameSvc.seats.ended(j.Session)
		writeErr(w, 404, errSeatCode.Error())
		return
	}
	info := sess.Info()
	// Someone whose limits wouldn't let them open this game can't join it either.
	if !j.Host {
		if _, err := s.visibleItem(r, info.ItemID); err != nil {
			s.gameSvc.seats.leave(j.Session, u.ID, u.ID)
			writeErr(w, 403, "This game isn’t one this profile can play.")
			return
		}
	}
	host := ""
	if v, ok := s.gameSvc.seats.view(j.Session, false); ok {
		host = v.Host.Name
	}
	writeJSON(w, 200, map[string]any{"sid": j.Session, "seat": j.Seat, "itemId": info.ItemID, "title": info.Title, "console": info.Console, "host": host, "yours": j.Host})
}

func (s *Server) handlePlaySeats(w http.ResponseWriter, r *http.Request) {
	sess := s.watchSession(r)
	if sess == nil {
		writeErr(w, 404, "not found")
		return
	}
	v, ok := s.gameSvc.seats.view(sess.ID(), sess.UserID() == userFrom(r).ID)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, v)
}

func (s *Server) handlePlaySeatLeave(w http.ResponseWriter, r *http.Request) {
	sess := s.watchSession(r)
	user, ok := pathInt(r, "user")
	if sess == nil || !ok {
		writeErr(w, 404, "not found")
		return
	}
	if !s.gameSvc.seats.leave(sess.ID(), userFrom(r).ID, user) {
		writeErr(w, 404, "not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
