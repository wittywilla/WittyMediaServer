package server

import (
	"net/http"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// Whether a channel can be recorded is decided here, once, for every client.
//
// Only a channel from a tuner can be recorded, never one made from the library:
// what a library channel plays is already the owner's own files. The tuner
// channel must also be clear (not encrypted), not marked by its device or its
// owner as not to be recorded, and the profile asking must be one that may use
// recordings.
//
// Every response that describes a channel sends canRecord from recordRefusal,
// always as true or false, and POST /api/recordings and POST /api/recording-rules
// refuse whatever it refuses. A client offers Record exactly when canRecord is
// true, and the server takes exactly what it offered. See docs/CLIENT-API.md.

// refusal is why a channel can't be recorded, as the answer to send.
type refusal struct {
	status int
	why    string
}

const libraryNotRecorded = "library channels can't be recorded: what they play is already in your library"

// mayRecord: recordings are for accounts that may use tuner channels.
func (s *Server) mayRecord(r *http.Request) bool {
	u := userFrom(r)
	return u != nil && (u.IsAdmin || s.access(r).CanWatchChannels("tuner"))
}

// recordRefusal is the one rule. c is a tuner channel; nil is a library channel.
// It says nil when whoever is asking may record c.
func (s *Server) recordRefusal(r *http.Request, c *livetv.Channel) *refusal {
	switch {
	case c == nil:
		return &refusal{400, libraryNotRecorded}
	case !s.mayRecord(r):
		return &refusal{403, "this profile cannot use recordings"}
	case c.Encrypted != "":
		return &refusal{403, (&tuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}).Error()}
	case c.NoRecord != "":
		return &refusal{403, c.Name + " can't be recorded: " + c.NoRecord}
	}
	return nil
}

// canRecord is the canRecord every response sends for a channel.
func (s *Server) canRecord(r *http.Request, c *livetv.Channel) bool {
	return s.recordRefusal(r, c) == nil
}

// canRecordKey is canRecord for a channel known by its key (lib-5, tun-12).
func (s *Server) canRecordKey(r *http.Request, key string) bool {
	kind, id, ok := parseKey(key)
	if !ok || kind != "tun" || s.TV == nil {
		return false
	}
	c, err := s.TV.Channel(r.Context(), id)
	return err == nil && c.Enabled && s.canRecord(r, c)
}

// recordableChannel is the channel a recording or a series rule is asked for,
// refused with the rule's own words when it can't be recorded.
func (s *Server) recordableChannel(w http.ResponseWriter, r *http.Request, key string) (int64, bool) {
	kind, id, ok := parseKey(key)
	if !ok {
		writeErr(w, 400, "bad channel")
		return 0, false
	}
	refuse := func(no *refusal) (int64, bool) {
		writeErr(w, no.status, no.why)
		return 0, false
	}
	if kind != "tun" {
		return refuse(s.recordRefusal(r, nil))
	}
	// Before the channel is looked up, so that a profile without Live TV learns nothing about it.
	if !s.mayRecord(r) {
		return refuse(&refusal{403, "this profile cannot use recordings"})
	}
	if !s.canWatchTuner(r, id) {
		writeErr(w, 404, "channel not found")
		return 0, false
	}
	c, err := s.TV.Channel(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "channel not found")
		return 0, false
	}
	if no := s.recordRefusal(r, c); no != nil {
		return refuse(no)
	}
	return id, true
}

// withRecord adds canRecord to a clear alternative, for the profile asking.
func (s *Server) withRecord(r *http.Request, alt *livetv.Alternative, byID map[int64]*livetv.Channel) *livetv.Alternative {
	if alt == nil {
		return nil
	}
	if _, id, ok := parseKey(alt.Key); ok {
		alt.CanRecord = byID[id] != nil && s.canRecord(r, byID[id])
	}
	return alt
}
