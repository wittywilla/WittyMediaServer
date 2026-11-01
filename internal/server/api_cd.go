package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittycd"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Audio CDs: a disc's layout, its audio as one continuous stream in the format a
// player can use, its CD+G pictures, and finding out what it is. The WittyCD
// module does the reading; this is the HTTP around it.

// cdDiscs keeps recently played images open: a sheet is quick to read, but a CHD's
// map isn't, and a player asks for several ranges in a row. Nothing stays open for
// long, and a changed sheet is read afresh.
type cdDiscs struct {
	mu       sync.Mutex
	m        map[string]*cdOpen
	sweeping bool
}

type cdOpen struct {
	d    *wittycd.Disc
	mod  time.Time
	size int64
	refs int
	used time.Time
	gone bool
}

const (
	cdOpenMax  = 8
	cdOpenIdle = 2 * time.Minute
)

// open returns a disc to use until release is called.
func (c *cdDiscs) open(path string) (*wittycd.Disc, func(), error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]*cdOpen{}
	}
	now := time.Now()
	c.closeIdle(now, cdOpenIdle)
	o := c.m[path]
	if o != nil && (!o.mod.Equal(st.ModTime()) || o.size != st.Size()) {
		delete(c.m, path)
		o.gone = true
		if o.refs == 0 {
			o.d.Close()
		}
		o = nil
	}
	if o == nil {
		if len(c.m) >= cdOpenMax {
			for p, x := range c.m {
				if x.refs == 0 {
					x.d.Close()
					delete(c.m, p)
					break
				}
			}
		}
		d, err := wittycd.Open(path)
		if err != nil {
			return nil, nil, err
		}
		o = &cdOpen{d: d, mod: st.ModTime(), size: st.Size()}
		c.m[path] = o
	}
	o.refs++
	o.used = now
	var once sync.Once
	return o.d, func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			o.refs--
			o.used = time.Now()
			if o.gone && o.refs == 0 {
				o.d.Close()
			}
			// An image left open keeps the owner from moving or deleting it on Windows,
			// so it is let go of even if nobody plays anything again.
			if !c.sweeping {
				c.sweeping = true
				time.AfterFunc(cdOpenIdle+time.Second, c.sweep)
			}
		})
	}, nil
}

// closeIdle closes what nobody has used for a while. The lock is held by the caller.
func (c *cdDiscs) closeIdle(now time.Time, idle time.Duration) {
	for p, o := range c.m {
		if o.refs == 0 && now.Sub(o.used) >= idle {
			o.d.Close()
			delete(c.m, p)
		}
	}
}

func (c *cdDiscs) sweep() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeIdle(time.Now(), cdOpenIdle)
	c.sweeping = false
	if len(c.m) > 0 {
		c.sweeping = true
		time.AfterFunc(cdOpenIdle+time.Second, c.sweep)
	}
}

// closeAll lets go of everything nobody is using.
func (c *cdDiscs) closeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeIdle(time.Now(), 0)
}

// cdItem finds a disc item a person may see, and opens it.
func (s *Server) cdItem(w http.ResponseWriter, r *http.Request) (*library.Item, *wittycd.Disc, func(), bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return nil, nil, nil, false
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != library.CDKind || !s.Store.LibraryFile(r.Context(), it.Path) {
		writeErr(w, 404, "disc not found")
		return nil, nil, nil, false
	}
	d, release, err := s.cds.open(it.Path)
	if err != nil {
		writeErr(w, 422, cdErrorText(err))
		return nil, nil, nil, false
	}
	return it, d, release, true
}

// cdErrorText words a failure to open an image for whoever is looking at it,
// without saying where on the server it is.
func cdErrorText(err error) string {
	switch {
	case errors.Is(err, wittycd.ErrNoAudio), errors.Is(err, wittycd.ErrNotCD), errors.Is(err, wittycd.ErrUnsupported):
		return err.Error()
	case errors.Is(err, os.ErrNotExist):
		return "this disc's image is no longer where it was"
	}
	return "this disc's image can't be read: it may be damaged"
}

// cdTrackView is one track as players use it. Positions are in sectors from the
// start of the disc (75 to the second, 588 samples each).
type cdTrackView struct {
	Number int `json:"number"`
	// ItemID is the track's own item in a music library, 0 elsewhere.
	ItemID      int64           `json:"itemId"`
	Title       string          `json:"title"`
	Artist      string          `json:"artist,omitempty"`
	Audio       bool            `json:"audio"`
	Pregap      int             `json:"pregap"`
	Start       int             `json:"start"`
	End         int             `json:"end"`
	Indexes     []wittycd.Index `json:"indexes"`
	PreEmphasis bool            `json:"preEmphasis"`
	ISRC        string          `json:"isrc,omitempty"`
}

// cdView is a disc for a player.
type cdView struct {
	ID          int64   `json:"id"`
	LibraryID   int64   `json:"libraryId"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Year        *int    `json:"year"`
	Poster      string  `json:"poster"`
	Format      string  `json:"format"`
	DiscID      string  `json:"discId"`
	CDDBID      string  `json:"cddbId,omitempty"`
	TOC         string  `json:"toc"`
	SizeBytes   int64   `json:"sizeBytes"`
	Sessions    int     `json:"sessions"`
	Samples     int64   `json:"samples"` // of the whole-disc stream
	DurationSec float64 `json:"durationSec"`
	// PositionSec is this person's place on the disc, counted from sector 0.
	PositionSec float64       `json:"positionSec"`
	Tracks      []cdTrackView `json:"tracks"`
	HiddenTrack bool          `json:"hiddenTrack"`
	HasGraphics bool          `json:"hasGraphics"`
	// Problem: dts or noise when the audio wouldn't sound like music, with a
	// warning that says why. A player asks before playing it.
	Problem  string            `json:"problem,omitempty"`
	Warnings []wittycd.Warning `json:"warnings"`
	// Formats that can be asked of /cd/{id}/audio, best first. Suggested is the one
	// to start with for this person's connection and limits.
	Formats   []string `json:"formats"`
	Suggested string   `json:"suggested"`
	// Lookup and Source: see library.CDInfo.
	Lookup       string `json:"lookup,omitempty"`
	Matches      int    `json:"matches,omitempty"`
	Source       string `json:"source,omitempty"`
	ReleaseID    string `json:"releaseId,omitempty"`
	Attribution  string `json:"attribution,omitempty"`
	InsertEffect bool   `json:"insertEffect"`
	InsertSound  bool   `json:"insertSound"`
}

func (s *Server) cdViewOf(r *http.Request, it *library.Item, d *wittycd.Disc) (*cdView, error) {
	toc := d.TOC()
	st, err := d.NewStream(nil, 1, false)
	if err != nil {
		return nil, err
	}
	var cd *library.CDInfo
	if it.Disc != nil {
		cd = it.Disc.CD
	}
	names := map[int]library.CDTrackName{}
	v := &cdView{ID: it.ID, LibraryID: it.LibraryID, Title: it.Title, Artist: it.Artist, Year: it.Year, Poster: it.Poster, Format: d.Format,
		DiscID: toc.MusicBrainzID(), CDDBID: toc.CDDBID(), TOC: toc.LookupString(), Sessions: d.Sessions, Samples: st.Samples(), DurationSec: d.DurationSec(),
		Tracks: []cdTrackView{}, HiddenTrack: d.HiddenTrack, HasGraphics: d.HasGraphics, Problem: d.Problem, Warnings: d.Warnings,
		Formats: []string{"flac", "wav"}, Suggested: "flac"}
	if !userFrom(r).IsAdmin {
		v.Poster = artForViewer(v.Poster)
	}
	if v.Warnings == nil {
		v.Warnings = []wittycd.Warning{}
	}
	v.SizeBytes = d.TotalBytes()
	if s.Tools != nil && s.Tools.FFmpeg != "" {
		v.Formats = append(v.Formats, "aac")
	}
	if cd != nil {
		for _, n := range cd.Tracks {
			names[n.Number] = n
		}
		v.Lookup, v.Matches, v.Source, v.ReleaseID = cd.Lookup, cd.Matches, cd.Source, cd.ReleaseID
		if cd.Source == "musicbrainz" || it.Provider == "musicbrainz" {
			v.Attribution = library.MusicBrainzAttribution
		}
	}
	// Someone away from home with a limit under what lossless needs starts lossy.
	if _, rate := s.remoteCaps(r); rate != "" {
		if limit := ffmpeg.ParseRate(rate); limit > 0 && limit < 1_100_000 && strings.Contains(strings.Join(v.Formats, ","), "aac") {
			v.Suggested = "aac"
		}
	}
	items := s.Store.CDTrackItems(r.Context(), it.ID)
	for i := range d.Tracks {
		t := &d.Tracks[i]
		n := names[t.Number]
		title := n.Title
		if title == "" {
			title = t.Text.Title
		}
		if title == "" && !t.Audio {
			title = "Data track"
		}
		if title == "" {
			title = fmt.Sprintf("Track %02d", t.Number)
		}
		artist := n.Artist
		if artist == "" {
			artist = t.Text.Performer
		}
		if artist == it.Artist {
			artist = ""
		}
		tv := cdTrackView{Number: t.Number, ItemID: items[t.Number], Title: title, Artist: artist, Audio: t.Audio, Pregap: t.Pregap, Start: t.Start, End: t.End,
			Indexes: t.Indexes, PreEmphasis: t.PreEmphasis, ISRC: t.ISRC}
		if tv.Indexes == nil {
			tv.Indexes = []wittycd.Index{}
		}
		v.Tracks = append(v.Tracks, tv)
	}
	if err := s.Store.AttachProgress(r.Context(), userFrom(r).ID, []*library.Item{it}); err == nil && it.Progress != nil {
		v.PositionSec = it.Progress.ResumeSec
	}
	v.InsertEffect = it.InsertFX == "on" || (it.InsertFX == "" && s.DB.GetSetting(r.Context(), "insertFx", "1") == "1")
	v.InsertSound = s.DB.GetSetting(r.Context(), "insertSound", "1") == "1"
	return v, nil
}

// GET /api/cd/{id}
func (s *Server) handleCD(w http.ResponseWriter, r *http.Request) {
	it, d, release, ok := s.cdItem(w, r)
	if !ok {
		return
	}
	defer release()
	v, err := s.cdViewOf(r, it, d)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, v)
}

// cdSlots: lossless streams at once. The encoder is light, but one runs per
// listener, and any signed-in account can start one.
var cdSlots = make(chan struct{}, 16)

// GET /api/cd/{id}/audio?format=flac|wav|aac&tracks=3,7,1&loops=2&start=SAMPLE&deemphasis=0
func (s *Server) handleCDAudio(w http.ResponseWriter, r *http.Request) {
	it, d, release, ok := s.cdItem(w, r)
	if !ok {
		return
	}
	defer release()
	q := r.URL.Query()
	var tracks []int
	if v := q.Get("tracks"); v != "" {
		for _, p := range strings.Split(v, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				writeErr(w, 400, "tracks is a list of track numbers")
				return
			}
			tracks = append(tracks, n)
		}
	}
	loops := qInt(r, "loops", 1)
	s.serveCDStream(w, r, it, d, tracks, loops)
}

// serveCDStream sends a program in the format asked for.
func (s *Server) serveCDStream(w http.ResponseWriter, r *http.Request, it *library.Item, d *wittycd.Disc, tracks []int, loops int) {
	q := r.URL.Query()
	st, err := d.NewStream(tracks, loops, q.Get("deemphasis") != "0")
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var start int64
	if v := q.Get("start"); v != "" {
		if start, err = strconv.ParseInt(v, 10, 64); err != nil || start < 0 {
			writeErr(w, 400, "start is a sample number")
			return
		}
	}
	if start > st.Samples() {
		start = st.Samples()
	}
	w = s.capWriter(w, r)
	w.Header().Set("X-WMS-Samples", strconv.FormatInt(st.Samples()-start, 10))
	switch q.Get("format") {
	case "wav":
		// Plain samples with byte ranges: no work beyond reading the image, and the
		// player seeks by itself.
		wav := st.WAV(start)
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Cache-Control", "private, max-age=3600")
		http.ServeContent(w, r, "", time.Time{}, io.NewSectionReader(wav, 0, wav.Size()))
	case "aac":
		if s.Tools == nil || s.Tools.FFmpeg == "" {
			writeErr(w, 503, "compressed audio needs FFmpeg, which this server can't find")
			return
		}
		select {
		case s.audioSlots <- struct{}{}:
			defer func() { <-s.audioSlots }()
		default:
			w.Header().Set("Retry-After", "5")
			writeErr(w, 503, "the server is busy with other streams")
			return
		}
		w.Header().Set("Content-Type", "audio/aac")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-WMS-Transcoded", "1")
		if r.Method == http.MethodHead {
			return
		}
		cmd := ffmpeg.Command(r.Context(), s.Tools.FFmpeg, "-hide_banner", "-loglevel", "error", "-nostdin", "-f", "s16le", "-ar", "44100", "-ac", "2", "-i", "pipe:0",
			"-c:a", "aac", "-b:a", "192k", "-f", "adts", "pipe:1")
		cmd.Stdin = io.NewSectionReader(st, start*wittycd.SampleBytes, st.Size()-start*wittycd.SampleBytes)
		cmd.Stdout = &stallWriter{w: w, rc: http.NewResponseController(w), limit: maxStreamStall}
		s.Log.Info("cd stream", "cat", "playback", "item", it.ID, "format", "aac", "start", start)
		_ = cmd.Run()
	case "", "flac":
		select {
		case cdSlots <- struct{}{}:
			defer func() { <-cdSlots }()
		default:
			w.Header().Set("Retry-After", "5")
			writeErr(w, 503, "the server is busy with other streams")
			return
		}
		w.Header().Set("Content-Type", "audio/flac")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Accept-Ranges", "none")
		if r.Method == http.MethodHead {
			return
		}
		s.Log.Info("cd stream", "cat", "playback", "item", it.ID, "format", "flac", "start", start)
		_ = st.WriteFLAC(&stallWriter{w: w, rc: http.NewResponseController(w), limit: maxStreamStall}, start)
	default:
		writeErr(w, 400, "format is flac, wav or aac")
	}
}

// streamCDTrack plays one track of a disc through the ordinary audio route, for
// players that know nothing about discs. It sends plain samples with byte ranges,
// so its VideoCodec (pcm_s16le) tells them the truth; ?transcode=1 sends AAC as for
// any other audio.
func (s *Server) streamCDTrack(w http.ResponseWriter, r *http.Request, it *library.Item) {
	image, n, ok := library.CDTrackNumber(it.Path)
	if !ok || !s.Store.LibraryFile(r.Context(), image) {
		writeErr(w, 404, "not found")
		return
	}
	d, release, err := s.cds.open(image)
	if err != nil {
		writeErr(w, 422, cdErrorText(err))
		return
	}
	defer release()
	// The same parameters as other audio: start in seconds, transcode.
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "wav"
	}
	if qBool(r, "transcode") {
		format = "aac"
	}
	q.Set("format", format)
	if v := q.Get("start"); v != "" {
		sec, err := strconv.ParseFloat(v, 64)
		if err != nil || sec < 0 {
			q.Del("start")
		} else {
			q.Set("start", strconv.FormatInt(int64(sec*wittycd.SampleRate), 10))
		}
	}
	r2 := r.Clone(r.Context())
	r2.URL.RawQuery = q.Encode()
	s.serveCDStream(w, r2, it, d, []int{n}, 1)
}

// GET /api/cd/{id}/graphics?from=SECTOR&sectors=N: CD+G packets, 96 bytes a sector.
func (s *Server) handleCDGraphics(w http.ResponseWriter, r *http.Request) {
	_, d, release, ok := s.cdItem(w, r)
	if !ok {
		return
	}
	defer release()
	from, sectors := qInt(r, "from", 0), qInt(r, "sectors", 7500)
	if from < 0 || sectors < 1 || sectors > 45000 {
		writeErr(w, 400, "from is a sector, sectors at most 45000")
		return
	}
	b, err := d.ReadGraphics(from, sectors)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(b)
}

// GET /api/cd/{id}/matches: what this disc could be. Sends only the table of
// contents to MusicBrainz.
func (s *Server) handleCDMatches(w http.ResponseWriter, r *http.Request) {
	it, d, release, ok := s.cdItem(w, r)
	if !ok {
		return
	}
	defer release()
	ms, err := s.Scanner.CDLookup(r.Context(), d)
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"itemId": it.ID, "discId": d.TOC().MusicBrainzID(), "matches": ms, "attribution": library.MusicBrainzAttribution})
}

// GET /api/cd-covers/{release}: a release's cover, small, for choosing between
// matches, so that a browser needn't ask another site for it.
func (s *Server) handleCDCover(w http.ResponseWriter, r *http.Request) {
	b, err := s.Scanner.CDCoverPreview(r.Context(), r.PathValue("release"))
	if err != nil {
		writeErr(w, 404, "no cover")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(b)
}

// POST /api/cd/{id}/identify {"releaseId": "..."}: use this release's names and
// cover. {"releaseId": ""} goes back to the disc's own names on the next scan.
func (s *Server) handleCDIdentify(w http.ResponseWriter, r *http.Request) {
	it, d, release, ok := s.cdItem(w, r)
	if !ok {
		return
	}
	defer release()
	var b struct {
		ReleaseID string `json:"releaseId"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, "expected a JSON body")
		return
	}
	lib, err := s.Store.Library(r.Context(), it.LibraryID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if b.ReleaseID == "" {
		if err := s.Scanner.ForgetCDMatch(r.Context(), lib, it, d); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	} else {
		m, err := s.Scanner.CDRelease(r.Context(), d, b.ReleaseID)
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		if err := s.Scanner.ApplyCDMatch(r.Context(), lib, it, d, m); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
	}
	v, err := s.cdViewOf(r, it, d)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, v)
}
