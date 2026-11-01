package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/lyrics"
)

// What a music library's pages ask for beyond lists of albums: its front page, a
// song's radio, artists with their pictures and biographies, a booklet beside an
// album, and the loudness a file was tagged with.

type musicHome struct {
	Hero     []library.Album  `json:"hero"`
	Albums   []library.Album  `json:"albums"`
	Artists  []library.Artist `json:"artists"`
	Songs    []*library.Item  `json:"songs"`
	Recent   []*library.Item  `json:"recent"`
	Continue []*library.Item  `json:"continue"`
	Genres   []string         `json:"genres"`
	Counts   map[string]int   `json:"counts"`
}

// GET /api/music/home?library=: everything the front page shows, in one request.
func (s *Server) handleMusicHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lib := int64(qInt(r, "library", 0))
	u := userFrom(r)
	acc := s.access(r)
	out := musicHome{Counts: map[string]int{}}
	var err error
	fail := func(e error) bool {
		if e != nil {
			writeErr(w, 500, e.Error())
			return true
		}
		return false
	}
	// Last played first, then newest: the front of the shelf is what you came back for.
	if out.Hero, err = s.Store.AlbumsFiltered(ctx, lib, library.AlbumFilter{Sort: "played", UserID: u.ID, Limit: 15}, acc); fail(err) {
		return
	}
	if out.Albums, err = s.Store.AlbumsFiltered(ctx, lib, library.AlbumFilter{Sort: "added", Limit: 30}, acc); fail(err) {
		return
	}
	if out.Artists, err = s.Store.Artists(ctx, lib, acc); fail(err) {
		return
	}
	if len(out.Artists) > 40 {
		out.Artists = out.Artists[:40]
	}
	if out.Songs, err = s.Store.MusicTracks(ctx, library.MusicQuery{LibraryID: lib, UserID: u.ID, View: "played", Limit: 24}, acc); fail(err) {
		return
	}
	if len(out.Songs) < 24 {
		more, err := s.Store.MusicTracks(ctx, library.MusicQuery{LibraryID: lib, UserID: u.ID, Sort: "album", Limit: 48}, acc)
		if fail(err) {
			return
		}
		seen := map[int64]bool{}
		for _, it := range out.Songs {
			seen[it.ID] = true
		}
		for _, it := range more {
			if len(out.Songs) >= 24 {
				break
			}
			if !seen[it.ID] {
				out.Songs = append(out.Songs, it)
			}
		}
	}
	if out.Recent, err = s.Store.MusicTracks(ctx, library.MusicQuery{LibraryID: lib, UserID: u.ID, View: "recent", Limit: 12}, acc); fail(err) {
		return
	}
	if out.Continue, err = s.Store.InProgress(ctx, lib, u.ID, 12, acc); fail(err) {
		return
	}
	if out.Genres, err = s.Store.Genres(ctx, lib, acc); fail(err) {
		return
	}
	albums, artists, songs, liked, err := s.Store.MusicCounts(ctx, lib, u.ID, acc)
	if fail(err) {
		return
	}
	out.Counts["albums"], out.Counts["artists"], out.Counts["songs"], out.Counts["liked"] = albums, artists, songs, liked
	_ = s.Store.AttachProgress(ctx, u.ID, out.Songs)
	_ = s.Store.AttachProgress(ctx, u.ID, out.Recent)
	_ = s.Store.AttachProgress(ctx, u.ID, out.Continue)
	s.forViewer(r, out.Songs...)
	s.forViewer(r, out.Recent...)
	s.forViewer(r, out.Continue...)
	writeJSON(w, 200, out)
}

// GET /api/music/songs?library=&view=&artist=&genre=&search=&sort=&desc=&limit=&offset=
func (s *Server) handleMusicSongs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if v := q.Get("search"); len(v) > 200 {
		writeErr(w, 400, "that search is too long")
		return
	}
	items, err := s.Store.MusicTracks(r.Context(), library.MusicQuery{LibraryID: int64(qInt(r, "library", 0)), UserID: userFrom(r).ID,
		Artist: q.Get("artist"), Genre: q.Get("genre"), Search: q.Get("search"), View: q.Get("view"), Sort: q.Get("sort"), Desc: qBool(r, "desc"),
		Limit: min(qInt(r, "limit", 2000), 5000), Offset: max(qInt(r, "offset", 0), 0)}, s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.Store.AttachProgress(r.Context(), userFrom(r).ID, items)
	writeJSON(w, 200, s.forViewer(r, items...))
}

// GET /api/music/likes?library=: the ids of this person's liked songs.
func (s *Server) handleMusicLikes(w http.ResponseWriter, r *http.Request) {
	ids, err := s.Store.LikedTracks(r.Context(), userFrom(r).ID, int64(qInt(r, "library", 0)))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ids)
}

// GET /api/music/radio/{id}?limit=: a station from this song, from this library.
func (s *Server) handleMusicRadio(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "audio" {
		writeErr(w, 404, "not found")
		return
	}
	items, err := s.Store.RadioFrom(r.Context(), it, userFrom(r).ID, max(2, min(qInt(r, "limit", 50), 200)), s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, s.forViewer(r, items...))
}

// ---------------------------------------------------------------- loudness

type loudness struct {
	TrackDB   *float64 `json:"trackDb"`
	AlbumDB   *float64 `json:"albumDb"`
	TrackPeak *float64 `json:"trackPeak"`
	AlbumPeak *float64 `json:"albumPeak"`
}

var (
	loudnessMu    sync.Mutex
	loudnessKnown = map[int64]loudness{} // read from each file once; small, and gone with the server
)

var gainRe = regexp.MustCompile(`^\s*([+-]?\d+(?:\.\d+)?)`)

func gainOf(v string) *float64 {
	m := gainRe.FindStringSubmatch(v)
	if m == nil {
		return nil
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil || f < -60 || f > 60 {
		return nil
	}
	return &f
}

// Opus keeps its gain as R128 numbers, in 1/256 dB, aimed 5 dB quieter than ReplayGain.
func r128Of(v string) *float64 {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < -32768 || n > 32767 {
		return nil
	}
	f := float64(n)/256 + 5
	return &f
}

// GET /api/music/loudness/{id}: the volume correction a file is tagged with, if any.
func (s *Server) handleMusicLoudness(w http.ResponseWriter, r *http.Request) {
	it := s.lyricsItem(w, r)
	if it == nil {
		return
	}
	loudnessMu.Lock()
	known, ok := loudnessKnown[it.ID]
	loudnessMu.Unlock()
	if ok {
		writeJSON(w, 200, known)
		return
	}
	var out loudness
	if !it.IsCDTrack() && !isWeb(it.Path) && s.Tools != nil && s.Tools.FFprobe != "" && s.Store.LibraryFile(r.Context(), it.Path) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if p, err := s.Tools.ProbeFile(ctx, it.Path); err == nil {
			tags := []map[string]string{p.Format.Tags}
			for _, st := range p.Streams {
				if st.CodecType == "audio" {
					tags = append(tags, st.Tags)
				}
			}
			for _, m := range tags {
				for k, v := range m {
					switch strings.ToLower(k) {
					case "replaygain_track_gain":
						out.TrackDB = gainOf(v)
					case "replaygain_album_gain":
						out.AlbumDB = gainOf(v)
					case "replaygain_track_peak":
						out.TrackPeak = gainOf(v)
					case "replaygain_album_peak":
						out.AlbumPeak = gainOf(v)
					case "r128_track_gain":
						out.TrackDB = r128Of(v)
					case "r128_album_gain":
						out.AlbumDB = r128Of(v)
					}
				}
			}
		}
	}
	loudnessMu.Lock()
	if len(loudnessKnown) > 20000 {
		loudnessKnown = map[int64]loudness{}
	}
	loudnessKnown[it.ID] = out
	loudnessMu.Unlock()
	writeJSON(w, 200, out)
}

// ---------------------------------------------------------------- booklets

var extraExts = map[string]string{".pdf": "booklet", ".txt": "text", ".md": "text", ".nfo": "text", ".log": ""}

const maxExtraText = 512 << 10

type musicExtra struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // booklet, text
	Size int64  `json:"size"`
}

// listExtras reads the album's folder: PDFs and text files, leaving out lyrics that
// belong to one song and ripping logs nobody means to read.
func (s *Server) listExtras(ctx context.Context, it *library.Item) []musicExtra {
	out := []musicExtra{}
	if it.IsCDTrack() || isWeb(it.Path) || !s.Store.LibraryFile(ctx, it.Path) {
		return out
	}
	entries, err := os.ReadDir(filepath.Dir(it.Path))
	if err != nil {
		return out
	}
	stems := map[string]bool{}
	for _, e := range entries {
		stems[strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))+"|"+strconv.FormatBool(extraExts[strings.ToLower(filepath.Ext(e.Name()))] != "")] = true
	}
	for i, e := range entries {
		if i > 5000 || len(out) >= 50 {
			break
		}
		name := e.Name()
		kind := extraExts[strings.ToLower(filepath.Ext(name))]
		if kind == "" || !e.Type().IsRegular() || strings.HasPrefix(name, ".") {
			continue
		}
		// "Song.txt" beside "Song.mp3" is that song's words, read by the lyrics.
		if kind == "text" && stems[strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))+"|false"] {
			continue
		}
		st, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, musicExtra{Name: name, Kind: kind, Size: st.Size()})
	}
	return out
}

// GET /api/music/extras/{id}: what there is to read beside a song's album.
func (s *Server) handleMusicExtras(w http.ResponseWriter, r *http.Request) {
	it := s.lyricsItem(w, r)
	if it == nil {
		return
	}
	writeJSON(w, 200, s.listExtras(r.Context(), it))
}

// GET /api/music/extras/{id}/file?name=: one of them. The name has to be one the
// listing gave, so it can't lead anywhere else.
func (s *Server) handleMusicExtraFile(w http.ResponseWriter, r *http.Request) {
	it := s.lyricsItem(w, r)
	if it == nil {
		return
	}
	name := r.URL.Query().Get("name")
	var found *musicExtra
	for _, e := range s.listExtras(r.Context(), it) {
		if e.Name == name {
			found = &e
			break
		}
	}
	if found == nil {
		writeErr(w, 404, "not found")
		return
	}
	path := filepath.Join(filepath.Dir(it.Path), name)
	if found.Kind == "text" {
		f, err := os.Open(path)
		if err != nil {
			writeErr(w, 404, "not found")
			return
		}
		defer f.Close()
		b, err := io.ReadAll(io.LimitReader(f, maxExtraText))
		if err != nil {
			writeErr(w, 500, "couldn't read that")
			return
		}
		writeJSON(w, 200, map[string]string{"text": lyrics.Decode(b)})
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Del("Content-Security-Policy") // the browser's own viewer shows it in a tab; PDFs can't run scripts on this site
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// ---------------------------------------------------------------- artists

// artistFacts is what's kept about an artist from online: the opening of their
// Wikipedia article, its address for the credit, and whether a picture came with it.
type artistFacts struct {
	Name     string `json:"name"`
	Title    string `json:"title,omitempty"`
	Bio      string `json:"bio,omitempty"`
	URL      string `json:"url,omitempty"`
	Picture  bool   `json:"picture,omitempty"`
	At       int64  `json:"at"`
	NotFound bool   `json:"notFound,omitempty"`
}

func (s *Server) artistCache(name, ext string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(name))))
	return filepath.Join(s.Cfg.CacheDir, "artists", hex.EncodeToString(h[:12])+ext)
}

func (s *Server) readArtistFacts(name string) *artistFacts {
	b, err := os.ReadFile(s.artistCache(name, ".json"))
	if err != nil {
		return nil
	}
	var v artistFacts
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	return &v
}

// Tests point this at their own server.
var wikipediaSummary = "https://en.wikipedia.org/api/rest_v1/page/summary/"

// Words that say an article is about a musician, and not someone or something else
// with the same name.
var musicianRe = regexp.MustCompile(`(?i)\b(singer|songwriter|musician|band|rapper|composer|vocalist|guitarist|pianist|drummer|producer|dj|duo|trio|group|orchestra|ensemble|artist|music)\b`)

var artistAsk = make(chan struct{}, 1)

// lookUpArtist asks Wikipedia about one name. It tells Wikipedia the name and nothing
// else, and only runs when the owner has turned online music information on.
func (s *Server) lookUpArtist(ctx context.Context, name string) *artistFacts {
	if s.DB.GetSetting(ctx, "musicInfoOnline", "0") != "1" || strings.TrimSpace(name) == "" {
		return nil
	}
	select {
	case artistAsk <- struct{}{}:
		defer func() { <-artistAsk }()
	case <-ctx.Done():
		return nil
	}
	if v := s.readArtistFacts(name); v != nil { // someone else asked while this waited
		return v
	}
	type summary struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Extract     string `json:"extract"`
		Thumbnail   struct {
			Source string `json:"source"`
		} `json:"thumbnail"`
		ContentURLs struct {
			Desktop struct {
				Page string `json:"page"`
			} `json:"desktop"`
		} `json:"content_urls"`
	}
	facts := &artistFacts{Name: name, At: time.Now().Unix(), NotFound: true}
	for _, page := range []string{name, name + " (band)", name + " (singer)", name + " (musician)", name + " (rapper)"} {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		req, err := http.NewRequestWithContext(cctx, "GET", wikipediaSummary+url.PathEscape(strings.ReplaceAll(page, " ", "_")), nil)
		if err != nil {
			cancel()
			return nil
		}
		req.Header.Set("User-Agent", lyrics.UserAgent)
		res, err := library.SafeClient.Do(req)
		if err != nil {
			cancel()
			return nil // not kept: it may answer next time
		}
		var v summary
		if res.StatusCode == 200 {
			_ = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&v)
		}
		res.Body.Close()
		cancel()
		if v.Type != "standard" || !musicianRe.MatchString(v.Description+" "+v.Extract) {
			continue
		}
		facts = &artistFacts{Name: name, Title: v.Title, Bio: v.Extract, URL: v.ContentURLs.Desktop.Page, At: time.Now().Unix()}
		if src := v.Thumbnail.Source; src != "" {
			facts.Picture = s.saveArtistPicture(ctx, name, src)
		}
		break
	}
	if err := os.MkdirAll(filepath.Dir(s.artistCache(name, "")), 0o750); err == nil {
		if b, err := json.Marshal(facts); err == nil {
			_ = os.WriteFile(s.artistCache(name, ".json"), b, 0o640)
		}
	}
	return facts
}

// saveArtistPicture keeps the article's picture, from Wikimedia's own servers only.
func (s *Server) saveArtistPicture(ctx context.Context, name, src string) bool {
	u, err := url.Parse(src)
	if err != nil || u.Scheme != "https" || !(u.Host == "upload.wikimedia.org" || wikipediaSummary[:5] == "http:") {
		return false
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", src, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", lyrics.UserAgent)
	res, err := library.SafeClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	ct := res.Header.Get("Content-Type")
	if res.StatusCode != 200 || !(strings.HasPrefix(ct, "image/jpeg") || strings.HasPrefix(ct, "image/png") || strings.HasPrefix(ct, "image/webp")) {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(s.artistCache(name, "")), 0o750); err != nil {
		return false
	}
	tmp := s.artistCache(name, ".tmp")
	f, err := os.Create(tmp)
	if err != nil {
		return false
	}
	_, err = io.Copy(f, io.LimitReader(res.Body, 8<<20))
	f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return false
	}
	return os.Rename(tmp, s.artistCache(name, ".img")) == nil
}

// Pictures of an artist that can sit in their folder.
var artistPictures = []string{"artist.jpg", "artist.jpeg", "artist.png", "artist.webp"}

// localArtistPicture looks in the artist's folder and the album's for a picture named
// for that purpose. The folders come from a song's own path, checked to be in a library.
func (s *Server) localArtistPicture(ctx context.Context, songPath string) string {
	if isWeb(songPath) {
		return ""
	}
	dir := filepath.Dir(songPath)
	for _, d := range []string{filepath.Dir(dir), dir} {
		for _, name := range artistPictures {
			p := filepath.Join(d, name)
			if s.Store.LibraryFile(ctx, p) {
				return p
			}
		}
	}
	return ""
}

var artistRefresh = make(chan string, 64) // names to look up when there's a moment

var artistRefreshOnce sync.Once

func (s *Server) queueArtistLookup(name string) {
	artistRefreshOnce.Do(func() {
		go func() {
			for n := range artistRefresh {
				ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
				s.lookUpArtist(ctx, n)
				cancel()
			}
		}()
	})
	select {
	case artistRefresh <- name:
	default: // busy; the next visit asks again
	}
}

// GET /api/music/artist-photo?library=&name=: their picture, or a cover of theirs.
func (s *Server) handleArtistPhoto(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.URL.Query().Get("name")
	lib := int64(qInt(r, "library", 0))
	if len(name) > 300 {
		writeErr(w, 400, "that name is too long")
		return
	}
	tracks, err := s.Store.MusicTracks(ctx, library.MusicQuery{LibraryID: lib, Artist: name, Sort: "album", Limit: 1}, s.access(r))
	if err != nil || len(tracks) == 0 {
		writeErr(w, 404, "not found")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if p := s.localArtistPicture(ctx, tracks[0].Path); p != "" {
		w.Header().Set("Cache-Control", "private, max-age=86400")
		http.ServeFile(w, r, p)
		return
	}
	facts := s.readArtistFacts(name)
	if facts != nil && facts.Picture {
		if st, err := os.Stat(s.artistCache(name, ".img")); err == nil && st.Mode().IsRegular() {
			f, err := os.Open(s.artistCache(name, ".img"))
			if err == nil {
				defer f.Close()
				head := make([]byte, 512)
				n, _ := f.Read(head)
				ct := http.DetectContentType(head[:n])
				if !strings.HasPrefix(ct, "image/") {
					writeErr(w, 404, "not found")
					return
				}
				_, _ = f.Seek(0, io.SeekStart)
				w.Header().Set("Content-Type", ct)
				w.Header().Set("Cache-Control", "private, max-age=604800")
				http.ServeContent(w, r, "", st.ModTime(), f)
				return
			}
		}
	}
	if facts == nil && name != "" {
		s.queueArtistLookup(name) // for next time; this picture shouldn't wait on it
	}
	// Their first album's cover stands in, and isn't kept for long so a photo can replace it.
	cover, err := s.Store.AlbumsFiltered(ctx, lib, library.AlbumFilter{Artist: name, Limit: 1}, s.access(r))
	if err != nil || len(cover) == 0 || cover[0].CoverItem == 0 {
		writeErr(w, 404, "no picture")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=600")
	q := url.Values{}
	if t := r.URL.Query().Get("token"); t != "" {
		q.Set("token", t)
	}
	to := "/api/items/" + strconv.FormatInt(cover[0].CoverItem, 10) + "/poster"
	if len(q) > 0 {
		to += "?" + q.Encode()
	}
	http.Redirect(w, r, to, http.StatusFound)
}

// GET /api/music/artist?library=&name=: an artist's page.
func (s *Server) handleMusicArtist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	name := q.Get("name")
	lib := int64(qInt(r, "library", 0))
	if len(name) > 300 {
		writeErr(w, 400, "that name is too long")
		return
	}
	acc := s.access(r)
	u := userFrom(r)
	albums, err := s.Store.AlbumsFiltered(ctx, lib, library.AlbumFilter{Artist: name, Sort: "year"}, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	top, err := s.Store.MusicTracks(ctx, library.MusicQuery{LibraryID: lib, UserID: u.ID, Artist: name, View: "played", Limit: 10}, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	all, err := s.Store.MusicTracks(ctx, library.MusicQuery{LibraryID: lib, UserID: u.ID, Artist: name, Sort: "album", Limit: 500}, acc)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if len(albums) == 0 && len(all) == 0 {
		writeErr(w, 404, "not found")
		return
	}
	facts := s.readArtistFacts(name)
	stale := facts != nil && facts.NotFound && time.Since(time.Unix(facts.At, 0)) > 30*24*time.Hour
	if facts == nil || stale {
		if f := s.lookUpArtist(ctx, name); f != nil {
			facts = f
		}
	}
	genres := map[string]bool{}
	genreList := []string{}
	for _, it := range all {
		for _, g := range it.Genres {
			if !genres[g] {
				genres[g] = true
				genreList = append(genreList, g)
			}
		}
	}
	out := map[string]any{"name": name, "albums": albums, "tracks": s.forViewer(r, all...), "top": s.forViewer(r, top...), "genres": genreList,
		"online": s.DB.GetSetting(ctx, "musicInfoOnline", "0") == "1"}
	if facts != nil && !facts.NotFound {
		out["bio"], out["bioUrl"], out["bioTitle"] = facts.Bio, facts.URL, facts.Title
	}
	writeJSON(w, 200, out)
}
