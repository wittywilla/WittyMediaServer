package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/libtranscode/tdarr"
	"github.com/wittywilla/wittymediaserver/internal/lyrics"
)

// Lyrics, from the most trusted place to the least: a correction made on this
// server, a file beside the song, the song's own tags, then an online source, which
// only happens if the owner turned it on. What comes from online is kept in the cache
// folder, so it never travels with a backup of the server's data.

func init() {
	settingKeys = append(settingKeys, "lyricsOnline", "lyricsLrclib", "lyricsMirror", "lyricsMusixmatchKey", "musicInfoOnline")
	secretSettings = append(secretSettings, "lyricsMusixmatchKey")
	settingDefaults["lyricsOnline"] = "0"
	settingDefaults["lyricsLrclib"] = "1"
	settingDefaults["musicInfoOnline"] = "0"
	settingChecks["lyricsOnline"] = oneOf("0", "1")
	settingChecks["lyricsLrclib"] = oneOf("0", "1")
	settingChecks["musicInfoOnline"] = oneOf("0", "1")
	settingChecks["lyricsMirror"] = func(v string) bool {
		u, err := url.Parse(v)
		return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil
	}
}

const (
	lyricsNotFoundFor = 14 * 24 * time.Hour // ask again after this long when a source had nothing
	lyricsFailedFor   = 15 * time.Minute    // and after this long when it couldn't be reached
)

// savedLyrics is what's kept for a track, in the cache (from online) or the data
// folder (a correction). Exactly the words someone else wrote: not to be shared.
type savedLyrics struct {
	Provider     string `json:"provider,omitempty"`
	ID           string `json:"id,omitempty"`
	Text         string `json:"text,omitempty"` // LRC, or words alone
	Instrumental bool   `json:"instrumental,omitempty"`
	NotFound     bool   `json:"notFound,omitempty"`
	At           int64  `json:"at"`
	By           string `json:"by,omitempty"`
}

// What the player gets.
type lyricsReply struct {
	ItemID       int64         `json:"itemId"`
	Source       string        `json:"source"` // "", override, sidecar, embedded, lrclib, mirror, musixmatch
	SourceName   string        `json:"sourceName,omitempty"`
	CreditURL    string        `json:"creditUrl,omitempty"`
	Synced       bool          `json:"synced"`
	Lines        []lyrics.Line `json:"lines"`
	Plain        string        `json:"plain"`
	Instrumental bool          `json:"instrumental,omitempty"`
	OffsetMS     int64         `json:"offsetMs"`
	Online       bool          `json:"online"`  // this server may look lyrics up
	CanEdit      bool          `json:"canEdit"` // this person may correct them for everyone
	Tried        bool          `json:"tried"`   // online was asked and had nothing
	Failed       string        `json:"failed,omitempty"`
}

var lyricsNames = map[string][2]string{
	"override":   {"Corrected on this server", ""},
	"sidecar":    {"A file beside the song", ""},
	"embedded":   {"The song's own tags", ""},
	"lrclib":     {"LRCLIB", "https://lrclib.net"},
	"mirror":     {"Your LRCLIB server", ""},
	"musixmatch": {"Musixmatch", "https://www.musixmatch.com"},
}

func (s *Server) lyricsOverridePath(id int64) string {
	return filepath.Join(s.Cfg.DataDir, "lyrics-corrections", strconv.FormatInt(id, 10)+".json")
}

func (s *Server) lyricsCachePath(id int64) string {
	return filepath.Join(s.Cfg.CacheDir, "lyrics", strconv.FormatInt(id, 10)+".json")
}

func readSavedLyrics(path string) *savedLyrics {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 2*lyrics.MaxBytes {
		return nil
	}
	var v savedLyrics
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	return &v
}

// Whole or not at all: a crash half way through mustn't leave half a file.
func writeSavedLyrics(path string, v *savedLyrics) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// localLyrics reads what's beside and inside the song.
func (s *Server) localLyrics(ctx context.Context, it *library.Item) (lyrics.Doc, string) {
	if it.IsCDTrack() || isWeb(it.Path) || !s.Store.LibraryFile(ctx, it.Path) {
		return lyrics.Doc{}, ""
	}
	if d := s.sidecarLyrics(ctx, it.Path); !d.Empty() {
		return d, "sidecar"
	}
	if f, _, err := s.Store.OpenLibraryFile(ctx, it.Path); err == nil {
		d := lyrics.ReadID3(f)
		f.Close()
		if !d.Empty() {
			return d, "embedded"
		}
	}
	if s.Tools == nil || s.Tools.FFprobe == "" {
		return lyrics.Doc{}, ""
	}
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	p, err := s.Tools.ProbeFile(pctx, it.Path)
	if err != nil {
		return lyrics.Doc{}, ""
	}
	tags := []map[string]string{p.Format.Tags}
	for _, st := range p.Streams {
		if st.CodecType == "audio" {
			tags = append(tags, st.Tags)
		}
	}
	for _, m := range tags {
		for k, v := range m {
			k = strings.ToLower(k)
			if k == "lyrics" || k == "unsyncedlyrics" || k == "unsynced lyrics" || k == "uslt" || strings.HasPrefix(k, "lyrics-") {
				if d := lyrics.Parse(v); !d.Empty() {
					return d, "embedded"
				}
			}
		}
	}
	return lyrics.Doc{}, ""
}

// sidecarLyrics looks for "<song>.lrc" then "<song>.txt", whatever their case. The
// names come from the folder listing, and what's read is what's opened: a file swapped
// for a link after the listing still can't lead out of the library.
func (s *Server) sidecarLyrics(ctx context.Context, songPath string) lyrics.Doc {
	dir := filepath.Dir(songPath)
	base := strings.TrimSuffix(filepath.Base(songPath), filepath.Ext(songPath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return lyrics.Doc{}
	}
	var lrc, txt string
	for i, e := range entries {
		if i > 5000 {
			break
		}
		name := e.Name()
		if !e.Type().IsRegular() || !strings.EqualFold(strings.TrimSuffix(name, filepath.Ext(name)), base) {
			continue
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".lrc", ".elrc":
			lrc = name
		case ".txt":
			txt = name
		}
	}
	for _, name := range []string{lrc, txt} {
		if name == "" {
			continue
		}
		f, st, err := s.Store.OpenLibraryFile(ctx, filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if st.Size() > lyrics.MaxBytes {
			f.Close()
			continue
		}
		b, err := io.ReadAll(io.LimitReader(f, lyrics.MaxBytes))
		f.Close()
		if err != nil {
			continue
		}
		if d := lyrics.Parse(lyrics.Decode(b)); !d.Empty() {
			return d
		}
	}
	return lyrics.Doc{}
}

// lyricsSources lists what may be asked, best first. Nothing is listed until the
// owner has agreed to online lyrics.
func (s *Server) lyricsSources(ctx context.Context) []lyrics.Source {
	if s.DB.GetSetting(ctx, "lyricsOnline", "0") != "1" {
		return nil
	}
	var out []lyrics.Source
	if m := strings.TrimSpace(s.DB.GetSetting(ctx, "lyricsMirror", "")); m != "" {
		// Someone's own copy may well be on the home network, so this one client may
		// reach private addresses: an admin typed the address, nothing else can set it.
		out = append(out, &lyrics.LRCLIB{Base: m, Client: lyricsMirrorClient, Label: "Your LRCLIB server", Provider: "mirror"})
	}
	if key := s.DB.GetSetting(ctx, "lyricsMusixmatchKey", ""); key != "" {
		out = append(out, &lyrics.Musixmatch{Key: key, Client: library.SafeClient})
	}
	if s.DB.GetSetting(ctx, "lyricsLrclib", "1") == "1" {
		out = append(out, &lyrics.LRCLIB{Base: lyricsPublicBase, Client: library.SafeClient, Label: "LRCLIB", Provider: "lrclib"})
	}
	return out
}

// Tests point this at their own server.
var lyricsPublicBase = lyrics.LRCLIBHome

// The owner's own copy may be at home, so this client alone may reach private addresses. It goes
// only where the owner said: a redirect stays on that host, and the link-local addresses, where
// a cloud host's own secrets are, are no place for a lyrics server.
var lyricsMirrorClient = &http.Client{
	Timeout: 12 * time.Second,
	Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second, Control: lyricsMirrorAddress}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || req.URL.Host != via[0].URL.Host {
			return errors.New("a lyrics server's redirect left its own address")
		}
		return nil
	},
}

func lyricsMirrorAddress(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() || tdarr.WrapsLinkLocal(ip) {
		return fmt.Errorf("%s can't be a lyrics server", host)
	}
	return nil
}

// One look-up at a time for the whole server: a listener skipping through an album
// shouldn't hammer a free service.
var lyricsAskSlot = make(chan struct{}, 1)

// One look-up per track, however many people ask at once.
var lyricsAsking sync.Map

func lyricsQuery(it *library.Item) lyrics.Query {
	artist := it.Artist
	if artist == "" {
		artist = it.AlbumArtist
	}
	return lyrics.Query{Artist: artist, Title: it.Title, Album: it.Album, DurationSec: it.RuntimeSec}
}

// lyricsSourceKey tells sources apart for lyricsDown: by name, and for LRCLIB by address too,
// so a mirror moved somewhere else is asked straight away.
func lyricsSourceKey(src lyrics.Source) string {
	if l, ok := src.(*lyrics.LRCLIB); ok {
		return src.Name() + "|" + l.Base
	}
	return src.Name()
}

// lyricsSourceDown says a source couldn't be reached less than lyricsFailedFor ago.
func (s *Server) lyricsSourceDown(key string) bool {
	v, ok := s.lyricsDown.Load(key)
	if !ok {
		return false
	}
	if time.Since(v.(time.Time)) < lyricsFailedFor {
		return true
	}
	s.lyricsDown.Delete(key)
	return false
}

// askOnline tries each source in turn and keeps the answer. Only the artist, title,
// album and length go out: never a path, a file name or who's listening. A source that
// couldn't be reached is left alone for lyricsFailedFor, unless again: with one look-up at
// a time for the whole server, a service that's down would make every song wait on it.
func (s *Server) askOnline(ctx context.Context, it *library.Item, again bool) (*savedLyrics, error) {
	sources := s.lyricsSources(ctx)
	if len(sources) == 0 || it.Title == "" {
		return nil, nil
	}
	if _, busy := lyricsAsking.LoadOrStore(it.ID, true); busy {
		return nil, errors.New("still looking")
	}
	defer lyricsAsking.Delete(it.ID)
	select {
	case lyricsAskSlot <- struct{}{}:
		defer func() { <-lyricsAskSlot }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	var failed error
	for _, src := range sources {
		key := lyricsSourceKey(src)
		if !again && s.lyricsSourceDown(key) {
			failed = fmt.Errorf("%s couldn't be reached a few minutes ago, so it isn't being asked for a while", src.Name())
			continue
		}
		f, err := src.Get(ctx, lyricsQuery(it))
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err() // the listener moved on: not the source's fault
			}
			if !errors.Is(err, lyrics.ErrNotFound) {
				failed = err
				s.lyricsDown.Store(key, time.Now())
				s.Log.WarnContext(ctx, "a lyrics source couldn't be reached; leaving it alone for a while", "cat", "library", "source", src.Name(), "for", lyricsFailedFor.String(), "err", err)
			}
			continue
		}
		s.lyricsDown.Delete(key)
		text := f.Synced
		if strings.TrimSpace(text) == "" {
			text = f.Plain
		}
		v := &savedLyrics{Provider: f.Provider, ID: f.ID, Text: text, Instrumental: f.Instrumental, At: time.Now().Unix()}
		if err := writeSavedLyrics(s.lyricsCachePath(it.ID), v); err != nil {
			s.Log.Warn("couldn't keep lyrics", "cat", "library", "item", it.ID, "err", err)
		}
		return v, nil
	}
	if failed != nil {
		return nil, failed // not kept: it may answer next time
	}
	v := &savedLyrics{NotFound: true, At: time.Now().Unix()}
	_ = writeSavedLyrics(s.lyricsCachePath(it.ID), v)
	return v, nil
}

func (s *Server) lyricsOffset(ctx context.Context, userID, itemID int64) int64 {
	var ms int64
	_ = s.DB.QueryRowContext(ctx, `SELECT offset_ms FROM lyrics_offsets WHERE user_id = ? AND item_id = ?`, userID, itemID).Scan(&ms)
	return ms
}

func (s *Server) lyricsItem(w http.ResponseWriter, r *http.Request) *library.Item {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "audio" {
		writeErr(w, 404, "not found")
		return nil
	}
	return it
}

func fillLyrics(out *lyricsReply, d lyrics.Doc, source string) {
	out.Source = source
	if n, ok := lyricsNames[source]; ok {
		out.SourceName, out.CreditURL = n[0], n[1]
	}
	out.Synced, out.Lines, out.Plain = d.Synced(), d.Timed, d.Plain
	if out.Lines == nil {
		out.Lines = []lyrics.Line{}
	}
}

// GET /api/music/lyrics/{id}?fetch=1. Asks online only with fetch=1, which the
// player sends when someone opens the lyrics, not for every track that plays.
func (s *Server) handleLyrics(w http.ResponseWriter, r *http.Request) {
	it := s.lyricsItem(w, r)
	if it == nil {
		return
	}
	ctx := r.Context()
	u := userFrom(r)
	out := lyricsReply{ItemID: it.ID, Lines: []lyrics.Line{}, CanEdit: u.IsAdmin, OffsetMS: s.lyricsOffset(ctx, u.ID, it.ID)}
	lib, _ := s.Store.Library(ctx, it.LibraryID)
	music := lib != nil && lib.Type == "music"
	out.Online = music && len(s.lyricsSources(ctx)) > 0
	if o := readSavedLyrics(s.lyricsOverridePath(it.ID)); o != nil {
		out.Instrumental = o.Instrumental
		fillLyrics(&out, lyrics.Parse(o.Text), "override")
		writeJSON(w, 200, out)
		return
	}
	if d, src := s.localLyrics(ctx, it); !d.Empty() {
		fillLyrics(&out, d, src)
		writeJSON(w, 200, out)
		return
	}
	saved := readSavedLyrics(s.lyricsCachePath(it.ID))
	stale := saved != nil && saved.NotFound && time.Since(time.Unix(saved.At, 0)) > lyricsNotFoundFor
	if music && qBool(r, "fetch") && out.Online && (saved == nil || stale || qBool(r, "again")) {
		fresh, err := s.askOnline(ctx, it, qBool(r, "again"))
		if err != nil {
			out.Failed = err.Error()
		} else if fresh != nil {
			saved = fresh
		}
	}
	// Turned off since: what's already here stays on this server, but isn't shown as
	// if online were still on, and goes away with "clear cached lyrics".
	if saved != nil {
		if saved.NotFound {
			out.Tried = true
		} else {
			out.Instrumental = saved.Instrumental
			fillLyrics(&out, lyrics.Parse(saved.Text), saved.Provider)
		}
	}
	writeJSON(w, 200, out)
}

// PUT /api/music/lyrics/{id}/offset {offsetMs}: each person's own.
func (s *Server) handleLyricsOffset(w http.ResponseWriter, r *http.Request) {
	it := s.lyricsItem(w, r)
	if it == nil {
		return
	}
	var in struct {
		OffsetMS int64 `json:"offsetMs"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.OffsetMS < -120000 || in.OffsetMS > 120000 {
		writeErr(w, 400, "that's more than two minutes out")
		return
	}
	var err error
	if in.OffsetMS == 0 {
		_, err = s.DB.ExecContext(r.Context(), `DELETE FROM lyrics_offsets WHERE user_id = ? AND item_id = ?`, userFrom(r).ID, it.ID)
	} else {
		_, err = s.DB.ExecContext(r.Context(), `INSERT INTO lyrics_offsets(user_id, item_id, offset_ms) VALUES(?,?,?)
			ON CONFLICT(user_id, item_id) DO UPDATE SET offset_ms = excluded.offset_ms`, userFrom(r).ID, it.ID, in.OffsetMS)
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"offsetMs": in.OffsetMS})
}

// POST /api/music/lyrics/{id}/search {q}: what might match, to choose from.
func (s *Server) handleLyricsSearch(w http.ResponseWriter, r *http.Request) {
	it := s.lyricsItem(w, r)
	if it == nil {
		return
	}
	var in struct {
		Q string `json:"q"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(in.Q) > 300 {
		writeErr(w, 400, "that search is too long")
		return
	}
	sources := s.lyricsSources(r.Context())
	if len(sources) == 0 {
		writeErr(w, 409, "online lyrics are turned off on this server")
		return
	}
	out := []lyrics.Found{}
	var failed error
	for _, src := range sources {
		list, err := src.Search(r.Context(), lyricsQuery(it), in.Q)
		if err != nil {
			failed = err
			continue
		}
		out = append(out, list...)
	}
	if len(out) == 0 && failed != nil {
		writeErr(w, 502, failed.Error())
		return
	}
	writeJSON(w, 200, out)
}

// PUT /api/music/lyrics/{id} (admin): corrects a track for everyone on this server.
// {text} is LRC or plain words; {instrumental}; or {provider, id} takes a search
// result. Nothing is written beside the music.
func (s *Server) handleLyricsSave(w http.ResponseWriter, r *http.Request) {
	it := s.lyricsItem(w, r)
	if it == nil {
		return
	}
	var in struct {
		Text         *string `json:"text"`
		Instrumental bool    `json:"instrumental"`
		Provider     string  `json:"provider"`
		ID           string  `json:"id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2*lyrics.MaxBytes)
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "expected lyrics of at most 256 KB")
		return
	}
	v := &savedLyrics{At: time.Now().Unix(), By: userFrom(r).Name}
	switch {
	case in.Provider != "":
		for _, src := range s.lyricsSources(r.Context()) {
			provider := ""
			switch l := src.(type) {
			case *lyrics.LRCLIB:
				provider = l.Provider
			case *lyrics.Musixmatch:
				provider = l.Provider()
			}
			if provider != in.Provider {
				continue
			}
			f, err := src.ByID(r.Context(), in.ID)
			if err != nil {
				writeErr(w, 502, err.Error())
				return
			}
			text := f.Synced
			if strings.TrimSpace(text) == "" {
				text = f.Plain
			}
			v.Provider, v.ID, v.Text, v.Instrumental = f.Provider, f.ID, text, f.Instrumental
		}
		if v.Provider == "" {
			writeErr(w, 409, "that source isn't turned on")
			return
		}
		// A choice from online is still someone else's words: it goes in the cache,
		// where online answers live, and replaces whichever answer was there.
		if err := writeSavedLyrics(s.lyricsCachePath(it.ID), v); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		_ = os.Remove(s.lyricsOverridePath(it.ID))
		writeJSON(w, 200, map[string]bool{"saved": true})
		return
	case in.Instrumental:
		v.Instrumental = true
	case in.Text != nil:
		if len(*in.Text) > lyrics.MaxBytes {
			writeErr(w, 400, "expected lyrics of at most 256 KB")
			return
		}
		if lyrics.Parse(*in.Text).Empty() {
			writeErr(w, 400, "there are no words in that")
			return
		}
		v.Text = *in.Text
	default:
		writeErr(w, 400, "nothing to save")
		return
	}
	if err := writeSavedLyrics(s.lyricsOverridePath(it.ID), v); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"saved": true})
}

// DELETE /api/music/lyrics/{id} (admin): forgets the correction and any online answer.
func (s *Server) handleLyricsForget(w http.ResponseWriter, r *http.Request) {
	it := s.lyricsItem(w, r)
	if it == nil {
		return
	}
	_ = os.Remove(s.lyricsOverridePath(it.ID))
	_ = os.Remove(s.lyricsCachePath(it.ID))
	w.WriteHeader(204)
}

// DELETE /api/music/lyrics-cache (admin): forgets everything fetched from online.
func (s *Server) handleLyricsClearCache(w http.ResponseWriter, _ *http.Request) {
	if err := os.RemoveAll(filepath.Join(s.Cfg.CacheDir, "lyrics")); err != nil {
		writeErr(w, 500, "couldn't clear them")
		return
	}
	w.WriteHeader(204)
}
