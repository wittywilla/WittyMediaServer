package server

import (
	"crypto/aes"
	"crypto/cipher"
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/auth"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Internet radio. Anyone with the library listens; administrators add stations,
// by address or from the directory. See docs/COLLECTIONS.md.

// radioLive keeps what is happening right now, in memory: what each station says
// it is playing, and time listened to HLS stations, which have no one long request
// to time. Entries leave when nobody is listening, so it stays small.
type radioLive struct {
	streamsMu sync.Mutex
	streams   int
	perUser   map[int64]int
	mu       sync.Mutex
	titles   map[int64]*radioTitle
	hlsUsers map[[2]int64]*radioHLSUser
	keyOnce  sync.Once
	key      []byte
}

type radioTitle struct {
	title     string
	since     int64
	listeners int       // long relays open now
	seen      time.Time // an HLS request
}

type radioHLSUser struct {
	last    time.Time
	pending float64
}

const (
	radioLiveLinger = 2 * time.Minute // an HLS station with no requests has lost its listeners
	maxRadioLive    = 1024
)

func (l *radioLive) entry(id int64, now time.Time) *radioTitle {
	if l.titles == nil {
		l.titles = map[int64]*radioTitle{}
	}
	if len(l.titles) >= maxRadioLive {
		for k, e := range l.titles {
			if e.listeners == 0 && now.Sub(e.seen) > radioLiveLinger {
				delete(l.titles, k)
			}
		}
	}
	e := l.titles[id]
	if e == nil {
		e = &radioTitle{}
		l.titles[id] = e
	}
	e.seen = now
	return e
}

func (l *radioLive) begin(id int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entry(id, time.Now()).listeners++
}

func (l *radioLive) end(id int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e := l.titles[id]; e != nil {
		if e.listeners--; e.listeners <= 0 {
			delete(l.titles, id)
		}
	}
}

func (l *radioLive) setTitle(id int64, title string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.entry(id, time.Now())
	if e.title != title {
		e.title, e.since = title, time.Now().Unix()
	}
}

func (l *radioLive) now(id int64) library.NowPlaying {
	l.mu.Lock()
	defer l.mu.Unlock()
	e := l.titles[id]
	if e == nil {
		return library.NowPlaying{}
	}
	if e.listeners == 0 && time.Since(e.seen) > radioLiveLinger {
		delete(l.titles, id)
		return library.NowPlaying{}
	}
	n := library.SplitNowPlaying(e.title)
	n.Since, n.Live = e.since, true
	return n
}

// hlsTick notes an HLS request, and gives the seconds to add to this listener's
// record, about one write a minute. first says this is a new listen.
func (l *radioLive) hlsTick(user, id int64) (seconds float64, first bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.entry(id, now)
	if l.hlsUsers == nil {
		l.hlsUsers = map[[2]int64]*radioHLSUser{}
	}
	if len(l.hlsUsers) >= maxRadioLive {
		for k, u := range l.hlsUsers {
			if now.Sub(u.last) > radioLiveLinger {
				delete(l.hlsUsers, k)
			}
		}
	}
	key := [2]int64{user, id}
	u := l.hlsUsers[key]
	if u == nil || now.Sub(u.last) > radioLiveLinger {
		l.hlsUsers[key] = &radioHLSUser{last: now}
		return 0, true
	}
	u.pending += now.Sub(u.last).Seconds()
	u.last = now
	if u.pending >= 60 {
		seconds, u.pending = u.pending, 0
	}
	return seconds, false
}

// sign makes the proof that this server's own playlist gave out an address for a
// station, so the relay can't be asked to fetch anything else. The key lasts until
// a restart, after which a player starts the stream again.
// How many stations this server relays at once, and for one person: each holds a
// connection to the station, and a person may have a few devices going.
const (
	radioStreamsAtOnce  = 32
	radioStreamsPerUser = 4
)

func (l *radioLive) takeStream(user int64) bool {
	l.streamsMu.Lock()
	defer l.streamsMu.Unlock()
	if l.streams >= radioStreamsAtOnce || l.perUser[user] >= radioStreamsPerUser {
		return false
	}
	if l.perUser == nil {
		l.perUser = map[int64]int{}
	}
	l.streams++
	l.perUser[user]++
	return true
}

func (l *radioLive) dropStream(user int64) {
	l.streamsMu.Lock()
	defer l.streamsMu.Unlock()
	l.streams--
	if l.perUser[user]--; l.perUser[user] <= 0 {
		delete(l.perUser, user)
	}
}

func (l *radioLive) cipher() cipher.AEAD {
	l.keyOnce.Do(func() {
		l.key = make([]byte, 32)
		_, _ = rand.Read(l.key)
	})
	block, err := aes.NewCipher(l.key)
	if err != nil {
		panic(err) // the key is always 32 bytes
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return aead
}

// seal is a part of a station as its players see it: only this server can read which
// address it means, and only for that station. A station's address can hold a private
// key, and everyone who may listen isn't someone who may have that. The same address
// seals the same way, so a player sees a part it already has as that part.
func (l *radioLive) seal(id int64, abs string) string {
	aead := l.cipher()
	m := hmac.New(sha256.New, l.key)
	fmt.Fprintf(m, "%d\n%s", id, abs)
	nonce := m.Sum(nil)[:aead.NonceSize()]
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(abs), []byte(strconv.FormatInt(id, 10))))
}

func (l *radioLive) unseal(id int64, sealed string) (string, bool) {
	aead := l.cipher()
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || len(raw) < aead.NonceSize() {
		return "", false
	}
	abs, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte(strconv.FormatInt(id, 10)))
	return string(abs), err == nil
}

func (s *Server) radioStation(r *http.Request) (*library.Item, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		return nil, false
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "radio" || !library.ValidStreamURL(it.Path) {
		return nil, false
	}
	return it, true
}

// radioForViewer fills in details, and takes the stream address away from anyone
// who can't change it: an address can hold a private key, and a player only needs
// the relay's. Administrators see it, to know what they added.
func (s *Server) radioForViewer(r *http.Request, items ...*library.Item) {
	u := userFrom(r)
	_ = s.Store.AttachRadio(r.Context(), u.ID, items)
	if !u.IsAdmin {
		for _, it := range items {
			it.Path = ""
		}
	}
	for _, it := range items {
		if it.Radio == nil {
			it.Radio = &library.RadioMeta{Status: "unknown"}
		}
	}
}

// GET /api/radio?library=
func (s *Server) handleRadioList(w http.ResponseWriter, r *http.Request) {
	libID := int64(qInt(r, "library", 0))
	if _, err := s.libraryOfType(r, libID, "radio"); err != nil {
		writeLibErr(w, err)
		return
	}
	items, _, err := s.Store.Items(r.Context(), library.ItemQuery{LibraryID: libID, Kind: "radio", Sort: "title", Access: s.access(r), Limit: 5000})
	if err != nil {
		writeLibErr(w, err)
		return
	}
	s.radioForViewer(r, items...)
	writeJSON(w, 200, items)
}

// GET /api/radio/{id}: one station.
func (s *Server) handleRadioGet(w http.ResponseWriter, r *http.Request) {
	it, ok := s.radioStation(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	s.radioForViewer(r, it)
	writeJSON(w, 200, it)
}

// GET /api/radio/{id}/now: what the station says it is playing, from its own
// stream, while someone is listening through this server. Asks nobody else.
func (s *Server) handleRadioNow(w http.ResponseWriter, r *http.Request) {
	it, ok := s.radioStation(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, s.radio.now(it.ID))
}

// POST /api/radio {libraryId, name, url, genre} (admin)
func (s *Server) handleRadioAdd(w http.ResponseWriter, r *http.Request) {
	var b struct {
		LibraryID int64 `json:"libraryId"`
		library.RadioStation
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := s.libraryOfType(r, b.LibraryID, "radio"); err != nil {
		writeLibErr(w, err)
		return
	}
	it, err := s.Store.AddRadioStation(r.Context(), b.LibraryID, b.RadioStation)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 201, it)
}

// POST /api/radio/import {libraryId, playlist} (admin): an M3U or PLS playlist's text.
func (s *Server) handleRadioImport(w http.ResponseWriter, r *http.Request) {
	var b struct {
		LibraryID int64  `json:"libraryId"`
		Playlist  string `json:"playlist"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := s.libraryOfType(r, b.LibraryID, "radio"); err != nil {
		writeLibErr(w, err)
		return
	}
	added := 0
	for _, st := range library.ParsePlaylist(b.Playlist) {
		if _, err := s.Store.AddRadioStation(r.Context(), b.LibraryID, st); err == nil {
			added++
		}
	}
	writeJSON(w, 200, map[string]any{"added": added})
}

// DELETE /api/radio/{id} (admin)
func (s *Server) handleRadioRemove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	if err := s.Store.RemoveRadioStation(r.Context(), id); err != nil {
		writeLibErr(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// POST /api/radio/{id}/check (admin): tries a station now.
func (s *Server) handleRadioCheck(w http.ResponseWriter, r *http.Request) {
	it, ok := s.radioStation(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	_ = s.Store.CheckStation(r.Context(), it) // its result is in the answer
	s.radioForViewer(r, it)
	writeJSON(w, 200, it)
}

// POST /api/radio/check {libraryId} (admin): tries every station of a library, one
// at a time in the background. The stations' status shows how it went.
func (s *Server) handleRadioCheckAll(w http.ResponseWriter, r *http.Request) {
	var b struct {
		LibraryID int64 `json:"libraryId"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := s.libraryOfType(r, b.LibraryID, "radio"); err != nil {
		writeLibErr(w, err)
		return
	}
	items, _, err := s.Store.Items(r.Context(), library.ItemQuery{LibraryID: b.LibraryID, Kind: "radio", Sort: "title", Limit: 5000})
	if err != nil {
		writeLibErr(w, err)
		return
	}
	s.radioChecks.Add(1)
	logging.Go("radio checks", func() {
		defer s.radioChecks.Done()
		s.Store.CheckStations(context.Background(), items, func(msg string, args ...any) {
			s.Log.Info(msg, append([]any{"cat", "library"}, args...)...)
		})
	}, "cat", "library")
	writeJSON(w, 202, map[string]any{"stations": len(items)})
}

// POST /api/radio/{id}/lookup (admin): finds a station added by address in the
// directory, and fills in its details.
func (s *Server) handleRadioLookup(w http.ResponseWriter, r *http.Request) {
	if !s.radioDirectoryOn(w, r) {
		return
	}
	it, ok := s.radioStation(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	if err := s.Store.LookUpRadio(r.Context(), it); err != nil {
		switch {
		case errors.Is(err, library.ErrDirectoryUnavailable):
			writeErr(w, 502, err.Error())
		case errors.Is(err, library.ErrNotFound):
			writeErr(w, 404, "the directory doesn't know this address")
		default:
			writeLibErr(w, err)
		}
		return
	}
	got, err := s.Store.Item(r.Context(), it.ID)
	if err != nil {
		writeLibErr(w, err)
		return
	}
	s.radioForViewer(r, got)
	writeJSON(w, 200, got)
}

// ---------------------------------------------------------------------------
// The directory (administrators; see library/radio_directory.go)
// ---------------------------------------------------------------------------

func (s *Server) radioDirectoryOn(w http.ResponseWriter, r *http.Request) bool {
	if s.DB.GetSetting(r.Context(), "radioDirectory", "1") != "1" {
		writeErr(w, 403, "the station directory is turned off in Settings, Media Indexers and Metadata Agents")
		return false
	}
	return true
}

func writeDirectoryErr(w http.ResponseWriter, err error) {
	if errors.Is(err, library.ErrDirectoryUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		writeErr(w, 502, library.ErrDirectoryUnavailable.Error())
		return
	}
	writeLibErr(w, err)
}

// GET /api/radio/directory/search?q=&tag=&country=&language=&limit=&offset=&library= (admin)
func (s *Server) handleRadioDirectorySearch(w http.ResponseWriter, r *http.Request) {
	if !s.radioDirectoryOn(w, r) {
		return
	}
	q := r.URL.Query()
	found, err := library.DirectorySearch(r.Context(), library.DirectoryQuery{Text: q.Get("q"), Tag: q.Get("tag"), Country: q.Get("country"), Language: q.Get("language"),
		Limit: qInt(r, "limit", 0), Offset: qInt(r, "offset", 0)})
	if err != nil {
		writeDirectoryErr(w, err)
		return
	}
	out := make([]library.DirectoryStation, len(found)) // the saved answer isn't changed
	copy(out, found)
	if libID := int64(qInt(r, "library", 0)); libID != 0 {
		if have, err := s.Store.RadioAddresses(r.Context(), libID); err == nil {
			for i := range out {
				out[i].Added = have[out[i].ID] || have[out[i].URL]
			}
		}
	}
	writeJSON(w, 200, out)
}

// GET /api/radio/directory/browse (admin): countries, genres and languages to browse by.
func (s *Server) handleRadioDirectoryBrowse(w http.ResponseWriter, r *http.Request) {
	if !s.radioDirectoryOn(w, r) {
		return
	}
	f, err := library.DirectoryBrowse(r.Context())
	if err != nil {
		writeDirectoryErr(w, err)
		return
	}
	writeJSON(w, 200, f)
}

// POST /api/radio/directory/add {libraryId, id} (admin)
func (s *Server) handleRadioDirectoryAdd(w http.ResponseWriter, r *http.Request) {
	if !s.radioDirectoryOn(w, r) {
		return
	}
	var b struct {
		LibraryID int64  `json:"libraryId"`
		ID        string `json:"id"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if _, err := s.libraryOfType(r, b.LibraryID, "radio"); err != nil {
		writeLibErr(w, err)
		return
	}
	it, err := s.Store.AddFromDirectory(r.Context(), b.LibraryID, b.ID)
	if err != nil {
		writeDirectoryErr(w, err)
		return
	}
	s.radioForViewer(r, it)
	writeJSON(w, 201, it)
}

// ---------------------------------------------------------------------------
// The relay
// ---------------------------------------------------------------------------

// setRadioHeaders: what comes through the relay is never treated as a page.
func setRadioHeaders(h http.Header, ct string) {
	h.Set("Content-Type", ct)
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	h.Set("X-Content-Type-Options", "nosniff")
}

// GET /api/radio/{id}/stream: relays a station, so listeners on any device get it
// from this server with one sign-in, whatever the station's address looks like. It
// reaches the internet only (never this network), only while someone listens, and
// the type is checked, so a station's address can't serve up a web page. Song titles
// inside the stream are taken out of the audio and shown by /now. An HLS station
// answers with a playlist whose every address leads back here.
func (s *Server) handleRadioStream(w http.ResponseWriter, r *http.Request) {
	it, ok := s.radioStation(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	if !s.radio.takeStream(userFrom(r).ID) {
		w.Header().Set("Retry-After", "30")
		writeErr(w, 503, "too many stations are playing right now. Close one, or try again in a moment")
		return
	}
	defer s.radio.dropStream(userFrom(r).ID)
	bg := context.WithoutCancel(r.Context())
	st, err := library.OpenStation(r.Context(), it.Path)
	if err != nil {
		if r.Context().Err() == nil { // the listener left: says nothing about the station
			s.Log.Info("radio", "cat", "playback", "item", it.ID, "err", err)
			s.Store.RadioResult(bg, it.ID, err)
			writeErr(w, 502, radioErrorText(err))
		}
		return
	}
	defer st.Close()
	answered := func() {
		s.Store.RadioResult(bg, it.ID, nil)
		s.Store.ApplyStationFacts(bg, it, library.FactsOf(st))
	}
	if st.HLS != nil {
		// A player asks for this every few seconds; it's written down once per listen.
		if s.serveRadioPlaylist(w, r, it, st.HLS, st.URL) {
			answered()
		}
		return
	}
	answered()
	u := userFrom(r)
	_ = s.Store.RecordView(bg, u.ID, it.ID, library.ViewEvent{})
	started := time.Now()
	s.radio.begin(it.ID)
	defer s.radio.end(it.ID)
	defer func() { s.Store.AddListening(bg, u.ID, it.ID, time.Since(started).Seconds()) }()

	body := st.Body
	if st.MetaInt > 0 {
		body = library.NewIcyReader(st.Body, st.MetaInt, func(title string) { s.radio.setTitle(it.ID, title) })
	}
	ct := st.ContentType
	if ct == "" || ct == "application/octet-stream" || ct == "binary/octet-stream" {
		ct = "audio/mpeg"
	}
	setRadioHeaders(w.Header(), ct)
	rc := http.NewResponseController(w)
	out := &stallWriter{w: s.capWriter(w, r), rc: rc, limit: maxStreamStall}
	buf := make([]byte, 32<<10)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return
			}
			_ = rc.Flush()
		}
		if rerr != nil {
			if !errors.Is(rerr, io.EOF) && r.Context().Err() == nil {
				s.Log.Info("radio stream ended", "cat", "playback", "item", it.ID, "err", rerr)
			}
			return
		}
	}
}

// radioErrorText is an error in words fit to show, without the station's address.
func radioErrorText(err error) string {
	switch {
	case errors.Is(err, library.ErrNotAudio):
		return "that address isn't an audio stream; if it's a playlist, import it instead"
	case errors.Is(err, library.ErrStationEncrypted):
		return err.Error()
	case strings.HasPrefix(err.Error(), "can't be reached"):
		return "the station can't be reached"
	}
	return library.CleanRadioText(err.Error(), 120)
}

// serveRadioPlaylist writes a station's HLS playlist with its addresses pointing at
// the relay. Like the server's own HLS, the sign-in comes along in the addresses for
// players that had to put it in the first one: a stream token, never the session's.
// It says whether this is the start of someone listening.
func (s *Server) serveRadioPlaylist(w http.ResponseWriter, r *http.Request, it *library.Item, body []byte, base *url.URL) bool {
	suffix := ""
	if st := r.URL.Query().Get("st"); st != "" {
		suffix = "&st=" + url.QueryEscape(st)
	} else if tok := tokenFrom(r); tok != "" && tok == r.URL.Query().Get("token") {
		if !auth.IsStreamToken(tok) {
			tok, _ = auth.StreamToken(tok, fmt.Sprintf("/api/radio/%d/", it.ID), auth.StreamTokenTTL)
		}
		suffix = "&token=" + url.QueryEscape(tok)
	}
	out, title, err := library.RewriteHLS(body, base, func(abs string) string {
		return "hls?u=" + s.radio.seal(it.ID, abs) + suffix
	})
	if err != nil {
		s.Store.RadioResult(context.WithoutCancel(r.Context()), it.ID, err)
		writeErr(w, 502, radioErrorText(err))
		return false
	}
	first := s.noteRadioHLS(r, it, title)
	setRadioHeaders(w.Header(), "application/vnd.apple.mpegurl")
	_, _ = w.Write(out)
	return first
}

func (s *Server) noteRadioHLS(r *http.Request, it *library.Item, title string) bool {
	bg := context.WithoutCancel(r.Context())
	u := userFrom(r)
	secs, first := s.radio.hlsTick(u.ID, it.ID)
	if first {
		_ = s.Store.RecordView(bg, u.ID, it.ID, library.ViewEvent{})
	}
	s.Store.AddListening(bg, u.ID, it.ID, secs)
	if title != "" {
		s.radio.setTitle(it.ID, title)
	}
	return first
}

// GET /api/radio/{id}/hls?u=: a part of an HLS station. Only addresses this
// server's own playlist gave out for this station are fetched.
func (s *Server) handleRadioHLS(w http.ResponseWriter, r *http.Request) {
	it, ok := s.radioStation(r)
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	abs, sealed := s.radio.unseal(it.ID, r.URL.Query().Get("u"))
	if !sealed || !library.ValidStreamURL(abs) {
		writeErr(w, 403, "that isn't part of this station")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, abs, nil)
	if err != nil {
		writeErr(w, 400, "that station's address can't be used")
		return
	}
	req.Header.Set("User-Agent", "WittyMediaServer")
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg) // HLS can address part of a file
	}
	res, err := library.SafeClient.Do(req)
	if err != nil {
		writeErr(w, 502, "the station can't be reached")
		return
	}
	defer res.Body.Close()
	if res.StatusCode != 200 && res.StatusCode != 206 {
		writeErr(w, 502, "the station answered "+strconv.Itoa(res.StatusCode))
		return
	}
	ct := strings.ToLower(strings.TrimSpace(strings.SplitN(res.Header.Get("Content-Type"), ";", 2)[0]))
	br := bufio.NewReaderSize(res.Body, 64<<10)
	head, _ := br.Peek(16)
	if strings.Contains(ct, "mpegurl") || strings.HasPrefix(string(head), "#EXTM3U") {
		body, err := io.ReadAll(io.LimitReader(br, 2<<20+1))
		if err != nil || len(body) > 2<<20 {
			writeErr(w, 502, "that station's playlist can't be read")
			return
		}
		s.serveRadioPlaylist(w, r, it, body, res.Request.URL)
		return
	}
	if !radioSegmentType(ct) || strings.HasPrefix(string(head), "<") {
		writeErr(w, 502, "that address isn't audio")
		return
	}
	title := ""
	if strings.HasPrefix(string(head), "ID3") || (len(head) >= 8 && string(head[4:8]) == "emsg") { // the song is in its first bytes
		tag, _ := br.Peek(64 << 10)
		title = library.SegmentTitle(tag)
	}
	s.noteRadioHLS(r, it, title)
	setRadioHeaders(w.Header(), firstNonEmpty(ct, "application/octet-stream"))
	if cr := res.Header.Get("Content-Range"); cr != "" {
		w.Header().Set("Content-Range", cr)
	}
	w.WriteHeader(res.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(br, 64<<20))
}

// radioSegmentType: what a part of an HLS station may be. The whole of it is checked
// again by whatever plays it; a web page never gets this far.
func radioSegmentType(ct string) bool {
	switch ct {
	case "", "application/octet-stream", "binary/octet-stream", "video/mp2t", "video/mp4", "video/iso.segment", "application/mp4", "application/ogg":
		return true
	}
	return strings.HasPrefix(ct, "audio/")
}

// playbackRadio answers /items/{id}/playback for a station: there's nothing to
// decide, it plays as the station sends it.
func (s *Server) playbackRadio(r *http.Request, it *library.Item, out *playbackResponse) {
	s.radioForViewer(r, it)
	out.Decision, out.Protocol, out.Seek, out.Container = "radio", "stream", "none", it.Radio.Codec
	if it.Radio.HLS {
		out.Protocol = "hls"
	}
	out.ProgressURL = ""
	out.NowPlayingURL = fmt.Sprintf("/api/radio/%d/now", it.ID)
	out.URL = withStream(fmt.Sprintf("/api/radio/%d/stream", it.ID), out.StreamToken)
}
