// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Where lyrics can come from online. Nothing here runs unless the owner has turned
// it on: the server decides that, and this package only knows how to ask.

// Query is everything a source is told about a track, and all it is ever told.
type Query struct {
	Artist, Title, Album string
	DurationSec          float64
}

// Found is lyrics from a source. Instrumental means the source says there are no
// words, which is an answer and not a failure.
type Found struct {
	Provider     string  `json:"provider"`
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Artist       string  `json:"artist"`
	Album        string  `json:"album,omitempty"`
	DurationSec  float64 `json:"durationSec,omitempty"`
	Synced       string  `json:"-"`
	Plain        string  `json:"-"`
	HasSynced    bool    `json:"hasSynced"`
	Instrumental bool    `json:"instrumental,omitempty"`
}

// ErrNotFound: the source answered, and has nothing for this track.
var ErrNotFound = errors.New("no lyrics found")

// ErrNeedsPlan: a keyed source refused the key or its plan doesn't include lyrics.
var ErrNeedsPlan = errors.New("that key was refused, or its plan doesn't include lyrics")

// Source is one place to ask.
type Source interface {
	Name() string
	// Get finds the best match for a track, or ErrNotFound.
	Get(ctx context.Context, q Query) (*Found, error)
	// Search lists what might match, for someone choosing by hand.
	Search(ctx context.Context, q Query, text string) ([]Found, error)
	// ByID fetches one result of Search.
	ByID(ctx context.Context, id string) (*Found, error)
}

// UserAgent says who is asking, as LRCLIB's documentation asks of apps.
const UserAgent = "WittyMediaServer (https://github.com/wittywilla/WittyMediaServer)"

// RequestTime is how long one request to a source may take. A variable for tests.
var RequestTime = 12 * time.Second

func getJSON(ctx context.Context, c *http.Client, address string, out any) (int, error) {
	cctx, cancel := context.WithTimeout(ctx, RequestTime)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", address, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := c.Do(req)
	if err != nil {
		// The error would hold the address, which can hold a key.
		return 0, errors.New("the lyrics source did not answer")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
		return res.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2*MaxBytes+1<<16)).Decode(out); err != nil {
		return res.StatusCode, errors.New("the lyrics source sent something unreadable")
	}
	return res.StatusCode, nil
}

// LRCLIB: a free and open database of timed lyrics, with no key and no account.
// Base can point at a copy someone runs themselves.
type LRCLIB struct {
	Base     string
	Client   *http.Client
	Label    string
	Provider string // how its answers are marked: "lrclib", or "mirror" for someone's own copy
}

// LRCLIBHome is the public service.
const LRCLIBHome = "https://lrclib.net"

func (l *LRCLIB) Name() string { return l.Label }

type lrclibRecord struct {
	ID           int64   `json:"id"`
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func (r lrclibRecord) found(provider string) Found {
	return Found{Provider: provider, ID: strconv.FormatInt(r.ID, 10), Title: r.TrackName, Artist: r.ArtistName, Album: r.AlbumName, DurationSec: r.Duration,
		Synced: r.SyncedLyrics, Plain: r.PlainLyrics, HasSynced: strings.TrimSpace(r.SyncedLyrics) != "", Instrumental: r.Instrumental}
}

func (l *LRCLIB) Get(ctx context.Context, q Query) (*Found, error) {
	v := url.Values{"artist_name": {q.Artist}, "track_name": {q.Title}}
	if q.Album != "" {
		v.Set("album_name", q.Album)
	}
	if q.DurationSec > 0 {
		v.Set("duration", strconv.Itoa(int(math.Round(q.DurationSec))))
	}
	var r lrclibRecord
	code, err := getJSON(ctx, l.Client, strings.TrimRight(l.Base, "/")+"/api/get?"+v.Encode(), &r)
	if err != nil {
		return nil, err
	}
	if code == 200 {
		f := r.found(l.Provider)
		return &f, nil
	}
	if code != 404 {
		return nil, fmt.Errorf("the lyrics source answered %d", code)
	}
	// The exact look-up wants the album and length to match. Tags often differ, so
	// fall back on a search, keeping only a result of about the same length: a live
	// version or a remix has the wrong timing even with the right words.
	list, err := l.Search(ctx, q, "")
	if err != nil {
		return nil, err
	}
	if best := BestMatch(list, q); best != nil {
		return best, nil
	}
	return nil, ErrNotFound
}

func (l *LRCLIB) Search(ctx context.Context, q Query, text string) ([]Found, error) {
	v := url.Values{}
	if strings.TrimSpace(text) != "" {
		v.Set("q", text)
	} else {
		v.Set("track_name", q.Title)
		if q.Artist != "" {
			v.Set("artist_name", q.Artist)
		}
	}
	var rows []lrclibRecord
	code, err := getJSON(ctx, l.Client, strings.TrimRight(l.Base, "/")+"/api/search?"+v.Encode(), &rows)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("the lyrics source answered %d", code)
	}
	if len(rows) > 30 {
		rows = rows[:30]
	}
	out := make([]Found, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.found(l.Provider))
	}
	return out, nil
}

func (l *LRCLIB) ByID(ctx context.Context, id string) (*Found, error) {
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return nil, ErrNotFound
	}
	var r lrclibRecord
	code, err := getJSON(ctx, l.Client, strings.TrimRight(l.Base, "/")+"/api/get/"+id, &r)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, ErrNotFound
	}
	f := r.found(l.Provider)
	return &f, nil
}

// BestMatch picks the result nearest in length that has words, and prefers timed
// ones. Nothing within a few seconds means nothing fits.
func BestMatch(list []Found, q Query) *Found {
	var best *Found
	score := math.Inf(1)
	for i := range list {
		f := &list[i]
		if !f.HasSynced && strings.TrimSpace(f.Plain) == "" && !f.Instrumental {
			continue
		}
		d := 0.0
		if q.DurationSec > 0 && f.DurationSec > 0 {
			d = math.Abs(q.DurationSec - f.DurationSec)
			if d > 4 {
				continue
			}
		}
		if !f.HasSynced {
			d += 100 // words without times, only if nothing better
		}
		if d < score {
			best, score = f, d
		}
	}
	return best
}

// Musixmatch is the optional keyed source. Lyrics need one of its paid plans; the
// free plan gets a refusal, which comes back as ErrNeedsPlan. Timed lyrics come from
// matcher.subtitle.get, and words alone from matcher.lyrics.get.
type Musixmatch struct {
	Key    string
	Base   string
	Client *http.Client
}

// MusixmatchHome is the public service.
const MusixmatchHome = "https://api.musixmatch.com/ws/1.1"

func (m *Musixmatch) Name() string { return "Musixmatch" }

func (m *Musixmatch) base() string {
	if m.Base != "" {
		return strings.TrimRight(m.Base, "/")
	}
	return MusixmatchHome
}

type mxmEnvelope struct {
	Message struct {
		Header struct {
			StatusCode int `json:"status_code"`
		} `json:"header"`
		Body json.RawMessage `json:"body"`
	} `json:"message"`
}

func (m *Musixmatch) call(ctx context.Context, path string, q Query, into any) error {
	v := url.Values{"apikey": {m.Key}, "q_track": {q.Title}, "q_artist": {q.Artist}, "format": {"json"}}
	if q.DurationSec > 0 {
		v.Set("f_subtitle_length", strconv.Itoa(int(math.Round(q.DurationSec))))
		v.Set("f_subtitle_length_max_deviation", "4")
	}
	var env mxmEnvelope
	code, err := getJSON(ctx, m.Client, m.base()+"/"+path+"?"+v.Encode(), &env)
	if err != nil {
		return err
	}
	if code != 200 {
		return fmt.Errorf("the lyrics source answered %d", code)
	}
	switch env.Message.Header.StatusCode {
	case 200:
		return json.Unmarshal(env.Message.Body, into)
	case 404:
		return ErrNotFound
	case 401, 402, 403:
		return ErrNeedsPlan
	}
	return fmt.Errorf("the lyrics source answered %d", env.Message.Header.StatusCode)
}

func (m *Musixmatch) Get(ctx context.Context, q Query) (*Found, error) {
	var sub struct {
		Subtitle struct {
			Body string `json:"subtitle_body"`
		} `json:"subtitle"`
	}
	f := &Found{Provider: "musixmatch", Title: q.Title, Artist: q.Artist, Album: q.Album, DurationSec: q.DurationSec}
	err := m.call(ctx, "matcher.subtitle.get", q, &sub)
	if err == nil && strings.TrimSpace(sub.Subtitle.Body) != "" {
		f.Synced, f.HasSynced = sub.Subtitle.Body, true
		return f, nil
	}
	if err != nil && !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrNeedsPlan) {
		return nil, err
	}
	var ly struct {
		Lyrics struct {
			Body         string `json:"lyrics_body"`
			Instrumental int    `json:"instrumental"`
		} `json:"lyrics"`
	}
	if e := m.call(ctx, "matcher.lyrics.get", q, &ly); e != nil {
		return nil, e
	}
	f.Plain, f.Instrumental = ly.Lyrics.Body, ly.Lyrics.Instrumental == 1
	if strings.TrimSpace(f.Plain) == "" && !f.Instrumental {
		return nil, ErrNotFound
	}
	return f, nil
}

// Search has only the one answer: this source matches one track at a time. What it searched
// for goes in the result's id, because this source has no ids of its own to ask by again.
func (m *Musixmatch) Search(ctx context.Context, q Query, _ string) ([]Found, error) {
	f, err := m.Get(ctx, q)
	if errors.Is(err, ErrNotFound) {
		return []Found{}, nil
	}
	if err != nil {
		return nil, err
	}
	f.ID = url.Values{"artist": {q.Artist}, "title": {q.Title}, "length": {strconv.Itoa(int(math.Round(q.DurationSec)))}}.Encode()
	return []Found{*f}, nil
}

// ByID gets the same match again, from the id Search gave it.
func (m *Musixmatch) ByID(ctx context.Context, id string) (*Found, error) {
	v, err := url.ParseQuery(id)
	if err != nil || len(id) > 2048 || v.Get("title") == "" {
		return nil, ErrNotFound
	}
	length, _ := strconv.Atoi(v.Get("length"))
	f, err := m.Get(ctx, Query{Artist: v.Get("artist"), Title: v.Get("title"), DurationSec: float64(length)})
	if err != nil {
		return nil, err
	}
	f.ID = id
	return f, nil
}

// Provider is how this source's answers are marked.
func (m *Musixmatch) Provider() string { return "musixmatch" }
