package library

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Reading a station's stream. Everything a station sends is hostile until it is
// shown otherwise: its playlists, its headers and the song titles inside its
// audio. Every request goes through SafeClient, so a playlist or a redirect can't
// lead the server into the private network.

const (
	maxStationPlaylist = 256 << 10 // a playlist that wraps a stream
	maxHLSPlaylist     = 2 << 20
	maxHLSSegment      = 64 << 20
	maxPlaylistHops    = 4 // a playlist pointing at a playlist...
	stationOpenTimeout = 20 * time.Second
	maxIcyMetaInt      = 1 << 20
	radioUserAgent     = "WittyMediaServer"
)

// icyConn lets old radio servers be understood: they answer "ICY 200 OK" where
// HTTP has "HTTP/1.0 200 OK", which is all that differs. Nothing else is touched.
type icyConn struct {
	net.Conn
	checked bool
	pending []byte
}

func (c *icyConn) Read(p []byte) (int, error) {
	if !c.checked {
		c.checked = true
		head := make([]byte, 4)
		n, err := io.ReadFull(c.Conn, head)
		if n == 0 {
			return 0, err
		}
		c.pending = head[:n]
		if string(head[:n]) == "ICY " {
			c.pending = []byte("HTTP/1.0 ")
		}
	}
	if len(c.pending) > 0 {
		n := copy(p, c.pending)
		c.pending = c.pending[n:]
		return n, nil
	}
	return c.Conn.Read(p)
}

// ErrNotAudio: the address gave something other than sound.
var ErrNotAudio = errors.New("that address isn't an audio stream")

// ErrStationEncrypted: the stream's sound is encrypted, which is never undone here.
var ErrStationEncrypted = errors.New("that stream is encrypted, so it can't be played")

// StationStream is an opened station: audio to relay, or an HLS playlist.
type StationStream struct {
	URL         *url.URL // where it ended up, after playlists and redirects
	ContentType string   // "" for HLS
	MetaInt     int      // bytes of audio between ICY metadata blocks; 0 for none
	Header      http.Header
	HLS         []byte // the playlist, for HLS stations
	Body        io.Reader
	close       func()
}

// Close ends the connection to the station.
func (st *StationStream) Close() { st.close() }

// OpenStation connects to a station, unwrapping .pls and .m3u playlists. It waits
// a limited time for the station to answer; ctx ends the stream afterwards.
func OpenStation(ctx context.Context, raw string) (*StationStream, error) {
	for hop := 0; hop < maxPlaylistHops; hop++ {
		st, next, err := openStationOnce(ctx, raw)
		if err != nil {
			return nil, err
		}
		if st != nil {
			return st, nil
		}
		raw = next
	}
	return nil, errors.New("that station's playlists lead nowhere")
}

func openStationOnce(ctx context.Context, raw string) (*StationStream, string, error) {
	if !ValidStreamURL(raw) {
		return nil, "", ErrNotAudio
	}
	cctx, cancel := context.WithCancel(ctx)
	// Only for the opening: a stream then runs for as long as someone listens.
	timer := time.AfterFunc(stationOpenTimeout, cancel)
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, raw, nil)
	if err != nil {
		timer.Stop()
		cancel()
		return nil, "", ErrNotAudio
	}
	req.Header.Set("User-Agent", radioUserAgent)
	req.Header.Set("Icy-MetaData", "1")
	res, err := SafeClient.Do(req)
	if err != nil {
		timer.Stop()
		cancel()
		return nil, "", fmt.Errorf("can't be reached: %w", err)
	}
	finish := func() {
		timer.Stop()
		res.Body.Close()
		cancel()
	}
	if res.StatusCode != http.StatusOK {
		finish()
		return nil, "", fmt.Errorf("the station answered %d", res.StatusCode)
	}
	br := bufio.NewReaderSize(res.Body, 4096)
	ct := mediaType(res.Header.Get("Content-Type"))
	base := res.Request.URL
	kind := classifyStation(ct, base.Path, br)
	if cctx.Err() != nil {
		finish()
		return nil, "", errors.New("can't be reached: it didn't answer in time")
	}
	switch kind {
	case stationPlaylist:
		body, err := io.ReadAll(io.LimitReader(br, maxHLSPlaylist+1))
		finish()
		if err != nil || len(body) > maxHLSPlaylist {
			return nil, "", errors.New("that station's playlist can't be read")
		}
		if bytes.Contains(body, []byte("#EXT-X-")) {
			return &StationStream{URL: base, Header: res.Header, HLS: body, close: func() {}}, "", nil
		}
		if len(body) > maxStationPlaylist {
			return nil, "", errors.New("that station's playlist is too long")
		}
		for _, s := range ParsePlaylist(string(body)) {
			return nil, s.URL, nil
		}
		// Addresses relative to the playlist.
		for _, line := range strings.Split(string(body), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line[0] == '#' || line[0] == '[' {
				continue
			}
			if u, err := base.Parse(line); err == nil && ValidStreamURL(u.String()) {
				return nil, u.String(), nil
			}
		}
		return nil, "", errors.New("that station's playlist has no stream in it")
	case stationAudio:
		meta := 0
		if v := res.Header.Get("icy-metaint"); v != "" {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil || n <= 0 || n > maxIcyMetaInt {
				finish()
				return nil, "", errors.New("that station's stream can't be read")
			}
			meta = n
		}
		timer.Stop()
		return &StationStream{URL: base, ContentType: ct, MetaInt: meta, Header: res.Header, Body: br, close: func() { res.Body.Close(); cancel() }}, "", nil
	}
	finish()
	return nil, "", ErrNotAudio
}

type stationKind int

const (
	stationRefused stationKind = iota
	stationAudio
	stationPlaylist
)

func mediaType(ct string) string {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))
	}
	return mt
}

// classifyStation decides what a station's answer is. A declared audio type is
// believed; a type that says nothing is checked against the first bytes.
func classifyStation(ct, path string, br *bufio.Reader) stationKind {
	switch ct {
	case "application/vnd.apple.mpegurl", "application/x-mpegurl", "audio/mpegurl", "audio/x-mpegurl", "audio/x-scpls", "audio/scpls", "application/pls+xml", "application/x-scpls":
		return stationPlaylist
	}
	if ct != "application/octet-stream" && ct != "binary/octet-stream" && ct != "" && !strings.HasPrefix(ct, "text/") {
		if RadioAudioType(ct) {
			return stationAudio
		}
		return stationRefused
	}
	head, _ := br.Peek(512)
	text := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(string(head), "\xef\xbb\xbf")))
	switch {
	case strings.HasPrefix(text, "#extm3u"), strings.HasPrefix(text, "[playlist]"), strings.HasPrefix(text, "http://"), strings.HasPrefix(text, "https://"):
		return stationPlaylist
	case strings.HasPrefix(ct, "text/"), strings.HasPrefix(text, "<"):
		return stationRefused
	}
	ext := strings.ToLower(path)
	if strings.HasSuffix(ext, ".m3u") || strings.HasSuffix(ext, ".m3u8") || strings.HasSuffix(ext, ".pls") {
		return stationPlaylist
	}
	return stationAudio
}

// RadioAudioType: a type that carries sound. Anything else isn't relayed, so a
// station's address can't serve up a web page from this site.
func RadioAudioType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "audio/") || strings.HasPrefix(ct, "application/ogg") || strings.HasPrefix(ct, "video/mp2t") || ct == "application/octet-stream" || ct == ""
}

// ---------------------------------------------------------------------------
// ICY metadata
// ---------------------------------------------------------------------------

// IcyReader gives a stream's audio with the metadata blocks taken out, and hands
// each block's title to onTitle. A block is at most 4080 bytes (its length byte
// counts sixteens), so a station can't make this hold more than that.
type IcyReader struct {
	r       io.Reader
	metaInt int
	left    int
	onTitle func(string)
	block   [255 * 16]byte
}

// NewIcyReader reads r, whose metadata comes every metaInt bytes.
func NewIcyReader(r io.Reader, metaInt int, onTitle func(string)) *IcyReader {
	return &IcyReader{r: r, metaInt: metaInt, left: metaInt, onTitle: onTitle}
}

func (i *IcyReader) Read(p []byte) (int, error) {
	if i.left == 0 {
		var size [1]byte
		if _, err := io.ReadFull(i.r, size[:]); err != nil {
			return 0, err
		}
		if n := int(size[0]) * 16; n > 0 {
			if _, err := io.ReadFull(i.r, i.block[:n]); err != nil {
				return 0, err
			}
			if title, ok := ParseStreamTitle(i.block[:n]); ok && i.onTitle != nil {
				i.onTitle(title)
			}
		}
		i.left = i.metaInt
	}
	if len(p) > i.left {
		p = p[:i.left]
	}
	n, err := i.r.Read(p)
	i.left -= n
	return n, err
}

// ParseStreamTitle finds StreamTitle='...'; in an ICY metadata block. ok is false
// when the block doesn't mention the title (it may carry only other fields).
func ParseStreamTitle(block []byte) (string, bool) {
	block = bytes.TrimRight(block, "\x00")
	const key = "StreamTitle='"
	i := bytes.Index(block, []byte(key))
	if i < 0 {
		return "", false
	}
	rest := block[i+len(key):]
	// The title may itself hold quotes; what ends it is a quote and a semicolon.
	if j := bytes.Index(rest, []byte("';")); j >= 0 {
		rest = rest[:j]
	} else {
		rest = bytes.TrimSuffix(rest, []byte("'"))
	}
	return CleanRadioText(decodeLoose(rest), 200), true
}

// decodeLoose reads text that may be UTF-8 or the older single-byte kind.
func decodeLoose(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}

// CleanRadioText makes text from a station or a directory fit to store and show:
// no markup, no control or direction-changing characters, one line, limited length.
// The interface escapes what it shows as well; this is so nothing odd is kept.
func CleanRadioText(s string, limit int) string {
	s = strings.ToValidUTF8(s, "")
	s = plain(s)
	var b strings.Builder
	space, n := true, 0 // true at the start, so leading spaces go
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			if !space {
				b.WriteByte(' ')
				n++
			}
			space = true
			continue
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == utf8.RuneError:
			continue
		}
		if n >= limit {
			break
		}
		b.WriteRune(r)
		n++
		space = false
	}
	return strings.TrimSpace(b.String())
}

// NowPlaying is what a station says it is playing.
type NowPlaying struct {
	Title  string `json:"title"`            // the whole line, as the station sent it
	Artist string `json:"artist,omitempty"` // when the line reads "Artist - Track"
	Track  string `json:"track,omitempty"`
	Since  int64  `json:"since,omitempty"` // when this line first appeared
	Live   bool   `json:"live"`            // someone is listening through this server now
}

// SplitNowPlaying fills in the artist and track from a line like "Artist - Track".
func SplitNowPlaying(title string) NowPlaying {
	n := NowPlaying{Title: title}
	if a, t, ok := strings.Cut(title, " - "); ok && strings.TrimSpace(a) != "" && strings.TrimSpace(t) != "" {
		n.Artist, n.Track = strings.TrimSpace(a), strings.TrimSpace(t)
	}
	return n
}

// ---------------------------------------------------------------------------
// HLS
// ---------------------------------------------------------------------------

var (
	hlsURIAttr  = regexp.MustCompile(`URI="([^"]*)"`)
	hlsCodecs   = regexp.MustCompile(`CODECS="([^"]*)"`)
	hlsMethod   = regexp.MustCompile(`METHOD=([A-Za-z0-9-]+)`)
	extinfTitle = regexp.MustCompile(`(?i)(?:^|[,\s])title="([^"]*)"`)
)

// videoCodec: a codec name in an HLS playlist that isn't sound.
func videoCodec(codecs string) bool {
	for _, c := range strings.Split(strings.ToLower(codecs), ",") {
		c = strings.TrimSpace(c)
		for _, v := range []string{"avc", "hvc", "hev", "vp8", "vp9", "vp09", "av01", "mp4v", "dvh", "dva"} {
			if strings.HasPrefix(c, v) {
				return true
			}
		}
	}
	return false
}

// RewriteHLS turns a station's HLS playlist into one that leads back through this
// server: every address goes through link, so a player never contacts the station
// itself and the relay can check each answer. Video variants are left out, and an
// encrypted stream is refused, not unlocked. title is the song the playlist names, if any.
func RewriteHLS(body []byte, base *url.URL, link func(abs string) string) (out []byte, title string, err error) {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	resolve := func(ref string) (string, bool) {
		u, err := base.Parse(strings.TrimSpace(ref))
		if err != nil || !ValidStreamURL(u.String()) {
			return "", false
		}
		return link(u.String()), true
	}
	// A variant is its #EXT-X-STREAM-INF line and the address after it.
	var variants, audioOnly int
	for _, line := range lines {
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF") {
			variants++
			m := hlsCodecs.FindStringSubmatch(line)
			if !strings.Contains(line, "RESOLUTION=") && (m == nil || !videoCodec(m[1])) {
				audioOnly++
			}
		}
	}
	if variants > 0 && audioOnly == 0 {
		return nil, "", ErrNotAudio
	}
	var b bytes.Buffer
	skipNext := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#EXT-X-KEY"), strings.HasPrefix(line, "#EXT-X-SESSION-KEY"):
			if m := hlsMethod.FindStringSubmatch(line); m == nil || !strings.EqualFold(m[1], "NONE") {
				return nil, "", ErrStationEncrypted
			}
			continue
		case strings.HasPrefix(line, "#EXT-X-I-FRAME-STREAM-INF"), strings.HasPrefix(line, "#EXT-X-MEDIA") && strings.Contains(line, "TYPE=VIDEO"):
			continue
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF"):
			m := hlsCodecs.FindStringSubmatch(line)
			if strings.Contains(line, "RESOLUTION=") || (m != nil && videoCodec(m[1])) {
				skipNext = true
				continue
			}
		case strings.HasPrefix(line, "#EXTINF:"):
			if t := extinfSong(line); t != "" {
				title = t
			}
		}
		if strings.HasPrefix(line, "#") {
			if hlsURIAttr.MatchString(line) {
				bad := false
				line = hlsURIAttr.ReplaceAllStringFunc(line, func(m string) string {
					link, ok := resolve(m[len(`URI="`) : len(m)-1])
					bad = bad || !ok
					return `URI="` + link + `"`
				})
				if bad {
					continue
				}
			}
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		if link, ok := resolve(line); ok {
			b.WriteString(link)
			b.WriteByte('\n')
		}
	}
	if !bytes.HasPrefix(b.Bytes(), []byte("#EXTM3U")) {
		return nil, "", ErrNotAudio
	}
	return b.Bytes(), title, nil
}

// extinfSong finds a song's name in an #EXTINF line: title="..." where a station
// labels it, otherwise the free text after the comma.
func extinfSong(line string) string {
	_, rest, ok := strings.Cut(line, ",")
	if !ok {
		return ""
	}
	if m := extinfTitle.FindStringSubmatch(rest); m != nil {
		return CleanRadioText(m[1], 200)
	}
	rest = strings.TrimSpace(rest)
	if strings.Contains(rest, `="`) || strings.EqualFold(rest, "no desc") {
		return ""
	}
	// A segment's number or file name says nothing about the song.
	for _, r := range rest {
		if unicode.IsLetter(r) {
			return CleanRadioText(rest, 200)
		}
	}
	return ""
}

// ID3Title reads the song from an ID3v2 tag at the start of a packed audio HLS
// segment: "Artist - Track", or whichever of them is there. Sizes come from the
// station and are all checked against the bytes actually given.
func ID3Title(b []byte) string {
	if len(b) < 10 || string(b[:3]) != "ID3" || (b[3] != 3 && b[3] != 4) {
		return ""
	}
	size := syncSafe(b[6:10])
	end := min(10+size, len(b))
	var title, artist string
	for pos := 10; pos+10 <= end; {
		id := string(b[pos : pos+4])
		if id[0] == 0 {
			break
		}
		n := 0
		if b[3] == 4 {
			n = syncSafe(b[pos+4 : pos+8])
		} else {
			n = int(b[pos+4])<<24 | int(b[pos+5])<<16 | int(b[pos+6])<<8 | int(b[pos+7])
		}
		pos += 10
		if n <= 0 || n > end-pos {
			break
		}
		switch id {
		case "TIT2":
			title = id3Text(b[pos : pos+n])
		case "TPE1":
			artist = id3Text(b[pos : pos+n])
		}
		pos += n
	}
	switch {
	case artist != "" && title != "":
		return CleanRadioText(artist+" - "+title, 200)
	case title != "":
		return CleanRadioText(title, 200)
	}
	return CleanRadioText(artist, 200)
}

// SegmentTitle finds the song at the start of an HLS segment: an ID3 tag in packed
// audio, or in the first emsg box of fragmented MP4. MPEG-TS segments carry theirs
// split across packets, and aren't read.
func SegmentTitle(head []byte) string {
	if bytes.HasPrefix(head, []byte("ID3")) {
		return ID3Title(head)
	}
	if len(head) < 8 || string(head[4:8]) != "emsg" {
		return ""
	}
	size := int(binary.BigEndian.Uint32(head[:4]))
	if size < 8 || size > len(head) {
		size = len(head)
	}
	box := head[:size]
	if i := bytes.Index(box, []byte("ID3")); i >= 0 {
		return ID3Title(box[i:])
	}
	return ""
}

func syncSafe(b []byte) int {
	return int(b[0]&0x7f)<<21 | int(b[1]&0x7f)<<14 | int(b[2]&0x7f)<<7 | int(b[3]&0x7f)
}

// id3Text decodes a text frame: one byte says the encoding.
func id3Text(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	body := b[1:]
	switch b[0] {
	case 1, 2: // UTF-16, with a byte-order mark (1) or big-endian (2)
		big := b[0] == 2
		if len(body) >= 2 && body[0] == 0xff && body[1] == 0xfe {
			big, body = false, body[2:]
		} else if len(body) >= 2 && body[0] == 0xfe && body[1] == 0xff {
			big, body = true, body[2:]
		}
		var r []rune
		for i := 0; i+1 < len(body); i += 2 {
			c := rune(body[i])<<8 | rune(body[i+1])
			if !big {
				c = rune(body[i+1])<<8 | rune(body[i])
			}
			if c == 0 {
				break
			}
			r = append(r, c)
		}
		return string(r)
	}
	if i := bytes.IndexByte(body, 0); i >= 0 {
		body = body[:i]
	}
	return decodeLoose(body)
}

// ---------------------------------------------------------------------------
// Looking at a station
// ---------------------------------------------------------------------------

// StationFacts is what a station says about itself in its answer.
type StationFacts struct {
	Name, Genre, Description, Homepage, Codec string
	Bitrate                                   int
	HLS                                       bool
}

// FactsOf reads an opened station's headers. They are the station's own words, so
// they only fill gaps and are cleaned like anything else from outside.
func FactsOf(st *StationStream) StationFacts {
	h := st.Header
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := h.Get(k); v != "" {
				return v
			}
		}
		return ""
	}
	f := StationFacts{HLS: st.HLS != nil,
		Name:        CleanRadioText(first("icy-name", "ice-name"), 200),
		Genre:       CleanRadioText(first("icy-genre", "ice-genre"), 80),
		Description: CleanRadioText(first("icy-description", "ice-description"), 500),
		Codec:       codecOfType(st.ContentType),
	}
	if u := cleanHomepage(first("icy-url", "ice-url")); u != "" {
		f.Homepage = u
	}
	// icy-br is sometimes "128,128".
	br, _, _ := strings.Cut(first("icy-br", "ice-bitrate"), ",")
	if n, err := strconv.Atoi(strings.TrimSpace(br)); err == nil && n > 0 && n < 100000 {
		f.Bitrate = n
	}
	return f
}

func codecOfType(ct string) string {
	switch {
	case strings.Contains(ct, "mpeg") && !strings.Contains(ct, "url"):
		return "MP3"
	case strings.Contains(ct, "aac"), strings.Contains(ct, "mp4"):
		return "AAC"
	case strings.Contains(ct, "opus"):
		return "Opus"
	case strings.Contains(ct, "ogg"):
		return "Ogg"
	case strings.Contains(ct, "flac"):
		return "FLAC"
	}
	return ""
}

// ProbeStation opens a station and waits for some sound, then hangs up.
func ProbeStation(ctx context.Context, raw string) (StationFacts, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	st, err := OpenStation(ctx, raw)
	if err != nil {
		return StationFacts{}, err
	}
	defer st.Close()
	if st.HLS != nil {
		if _, _, err := RewriteHLS(st.HLS, st.URL, func(abs string) string { return abs }); err != nil {
			return StationFacts{}, err
		}
		return FactsOf(st), nil
	}
	if _, err := io.ReadFull(st.Body, make([]byte, 4096)); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return StationFacts{}, errors.New("the station sends no sound")
	}
	return FactsOf(st), nil
}
