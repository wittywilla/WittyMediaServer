package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Stream tokens let a native player fetch media it can't add headers to: a
// short-lived, read-only ticket in the address (?st=...) for the media of one
// item. Unlike a session token in an address, a leaked one opens nothing else
// and stops working soon after playback stops; it also ends with the sign-in
// it came from, and every use still goes through the account's access rules.
//
// They live in memory: a restart ends HLS streams and disc sessions too, and
// clients answer a 401 by asking for a new ticket.

const (
	// streamIdle: a ticket nobody has used for this long is gone. Each use
	// extends it, so a long film keeps playing and a pause survives.
	streamIdle = 30 * time.Minute
	// streamMax is the longest a ticket can live, however much it is used.
	streamMax = 12 * time.Hour
	// streamMaxPerUser bounds what one account can keep open; the oldest goes.
	streamMaxPerUser = 32
	streamMaxTotal   = 4096
)

type streamGrant struct {
	session string // the sign-in this ticket came from
	userID  int64
	itemID  int64
	key     string // the item's name in stream IDs (hls.Key)
	idle    time.Time
	hard    time.Time
}

type streamTokens struct {
	mu sync.Mutex
	m  map[string]*streamGrant
}

// mint returns a ticket for an item, reusing the one this sign-in already has
// so that asking repeatedly doesn't pile them up.
func (t *streamTokens) mint(session string, userID int64, it *library.Item) (string, time.Time, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if t.m == nil {
		t.m = map[string]*streamGrant{}
	}
	mine := 0
	var oldest string
	for tok, g := range t.m {
		if now.After(g.idle) || now.After(g.hard) {
			delete(t.m, tok)
			continue
		}
		// Most of its life still ahead: hand out the same ticket.
		if g.session == session && g.itemID == it.ID && g.hard.Sub(now) > streamMax/2 {
			g.idle = now.Add(streamIdle)
			return tok, g.idle, nil
		}
		if g.userID == userID {
			mine++
			if oldest == "" || g.idle.Before(t.m[oldest].idle) {
				oldest = tok
			}
		}
	}
	if mine >= streamMaxPerUser || len(t.m) >= streamMaxTotal {
		if oldest == "" { // the server is full of other people's tickets
			for tok := range t.m {
				oldest = tok
				break
			}
		}
		delete(t.m, oldest)
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", time.Time{}, err
	}
	tok := "st_" + hex.EncodeToString(b)
	g := &streamGrant{session: session, userID: userID, itemID: it.ID, key: hls.Key(it.Path), idle: now.Add(streamIdle), hard: now.Add(streamMax)}
	t.m[tok] = g
	return tok, g.idle, nil
}

// use returns a live ticket and extends it.
func (t *streamTokens) use(tok string) *streamGrant {
	t.mu.Lock()
	defer t.mu.Unlock()
	g := t.m[tok]
	if g == nil {
		return nil
	}
	now := time.Now()
	if now.After(g.idle) || now.After(g.hard) {
		delete(t.m, tok)
		return nil
	}
	g.idle = now.Add(streamIdle)
	if g.idle.After(g.hard) {
		g.idle = g.hard
	}
	c := *g
	return &c
}

// streamRoutes are the only routes a stream token opens, each checking the
// request is for the token's own item. New media routes are added here, and
// never anything that changes data or lists the library.
func (s *Server) streamRoutes(pattern string) func(*http.Request, *streamGrant) bool {
	item := func(r *http.Request, g *streamGrant) bool {
		id, ok := pathInt(r, "id")
		return ok && id == g.itemID
	}
	stream := func(r *http.Request, g *streamGrant) bool {
		_, rest, ok := strings.Cut(r.PathValue("id"), "-") // v-KEY-..., dvd-KEY-..., bd-KEY-...
		return ok && strings.HasPrefix(rest, g.key+"-")
	}
	disc := func(r *http.Request, g *streamGrant) bool {
		sid := r.PathValue("sid")
		if strings.HasPrefix(sid, "bd") {
			sess := s.BD.Get(sid)
			return sess != nil && sess.ItemID == g.itemID
		}
		sess := s.DVD.Get(sid)
		return sess != nil && sess.ItemID == g.itemID
	}
	switch pattern {
	case "GET /api/items/{id}/file", "GET /api/items/{id}/stream", "GET /api/items/{id}/poster", "GET /api/items/{id}/backdrop",
		"GET /api/items/{id}/subtitles/{file}", "GET /api/items/{id}/disc/thumb/{ttn}/{n}":
		return item
	case "GET /api/hls/{id}/playlist.m3u8", "GET /api/hls/{id}/master.m3u8", "GET /api/hls/{id}/seg/{n}", "GET /api/hls/{id}/subs/{file}":
		return stream
	case "GET /api/dvd/{sid}/menu/{serial}/overlay.png", "GET /api/dvd/{sid}/menu/{serial}/highlight/{n}", "GET /api/dvd/{sid}/still/{name}", "GET /api/dvd/{sid}/sub/{name}":
		return disc
	}
	return nil
}

// resolveStream signs a media request in by its stream token, if it has one
// that fits this route and this item.
func (s *Server) resolveStream(r *http.Request) *auth.User {
	tok := r.URL.Query().Get("st")
	if tok == "" {
		return nil
	}
	check := s.streamRoutes(r.Pattern)
	if check == nil {
		return nil
	}
	g := s.stream.use(tok)
	if g == nil || !check(r, g) {
		return nil
	}
	// The ticket ends with the sign-in it came from (signing out, a changed
	// password), and the account's limits are read afresh.
	u, err := s.Auth.Resolve(r.Context(), g.session)
	if err != nil || u.ID != g.userID {
		return nil
	}
	return u
}

// sessionToken is the sign-in behind a request, for tickets made from it.
func (s *Server) sessionToken(r *http.Request, u *auth.User) string {
	for _, tok := range auth.Tokens(r) {
		if c, err := s.Auth.Resolve(r.Context(), tok); err == nil && c.ID == u.ID {
			return tok
		}
	}
	return ""
}

// POST /api/items/{id}/stream-token: a ticket for this item's media. It is
// asked for with a real sign-in, never with another ticket.
func (s *Server) handleStreamToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil {
		writeErr(w, 404, "item not found")
		return
	}
	tok, exp, err := s.stream.mint(s.sessionToken(r, userFrom(r)), userFrom(r).ID, it)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"token": tok, "param": "st", "expiresInSec": int(time.Until(exp).Seconds())})
}

// withStream adds a ticket to a media address.
func withStream(u, tok string) string {
	if tok == "" {
		return u
	}
	if strings.Contains(u, "?") {
		return u + "&st=" + tok
	}
	return u + "?st=" + tok
}
