package library

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

// Podcasts: a podcasts library holds subscriptions (RSS feeds). Every
// episode is stored as an audio item whose path is its enclosure URL, so
// progress, access limits, search and the player work as for music. A
// downloaded episode's path becomes the file in the library folder.

// Podcast is one subscription.
type Podcast struct {
	ID           int64  `json:"id"`
	LibraryID    int64  `json:"libraryId"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	Image        string `json:"image"`
	Description  string `json:"description"`
	LastChecked  int64  `json:"lastChecked"`
	AutoDownload int    `json:"autoDownload"` // newest N episodes kept on disk (0 = stream only)
}

// SafeClient fetches feeds and episodes from the internet only: addresses
// on the local network or this machine are refused, so a feed cannot be
// used to reach other devices (server-side request forgery).
var SafeClient = &http.Client{
	Timeout: 0, // episodes can be long; requests carry their own contexts
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{Timeout: 15 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if !internetAddr(net.ParseIP(host)) {
				return fmt.Errorf("refusing to connect to %s (not on the internet)", host)
			}
			return nil
		}}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		MaxIdleConnsPerHost:   4,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 || (req.URL.Scheme != "http" && req.URL.Scheme != "https") {
			return errors.New("too many or unsafe redirects")
		}
		return nil
	},
}

// notInternet are the address ranges that are not the public internet. Some
// lead back to private networks: NAT64 and 6to4 addresses embed an IPv4 one.
var notInternet = func() []netip.Prefix {
	var out []netip.Prefix
	for _, c := range []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24", "192.88.99.0/24",
		"192.168.0.0/16", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "100::/64", "2001::/32", "2001:db8::/32", "64:ff9b:1::/48", "fc00::/7", "fe80::/10", "fec0::/10", "ff00::/8",
	} {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}()

var (
	nat64Prefix = netip.MustParsePrefix("64:ff9b::/96")
	sixToFour   = netip.MustParsePrefix("2002::/16")
)

// internetAddr reports whether an address is on the public internet.
func internetAddr(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if a.Is6() {
		b := a.As16()
		switch {
		case nat64Prefix.Contains(a):
			return internetAddr(net.IP(b[12:16]))
		case sixToFour.Contains(a):
			return internetAddr(net.IP(b[2:6]))
		}
	}
	for _, p := range notInternet {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// catalogClient is for the online catalogues (TMDB, TVmaze...): their fixed
// addresses are on the internet, and a redirect must not lead anywhere else.
func catalogClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: SafeClient.Transport, CheckRedirect: SafeClient.CheckRedirect}
}

// maxAnswer caps what is read from a catalogue's answer.
const maxAnswer = 32 << 20

// webURL keeps a feed's address only when it is a web address: a feed must
// not point the server at a file on this machine.
func webURL(s string) string {
	s = strings.TrimSpace(s)
	if l := strings.ToLower(s); strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://") {
		return s
	}
	return ""
}

type rssFeed struct {
	Channel struct {
		Title       string `xml:"title"`
		Author      string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd author"`
		Description string `xml:"description"`
		// <image><url> (RSS) and <itunes:image href> share a local name, and
		// encoding/xml matches an un-namespaced tag in any namespace, so both
		// are collected here and told apart afterwards.
		Images []struct {
			URL  string `xml:"url"`
			Href string `xml:"href,attr"`
		} `xml:"image"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	Title       string `xml:"title"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Summary     string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd summary"`
	Duration    string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd duration"`
	Episode     string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd episode"`
	Season      string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd season"`
	Explicit    string `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd explicit"`
	Image       struct {
		Href string `xml:"href,attr"`
	} `xml:"http://www.itunes.com/dtds/podcast-1.0.dtd image"`
	Enclosure struct {
		URL  string `xml:"url,attr"`
		Type string `xml:"type,attr"`
	} `xml:"enclosure"`
}

// maxFeedItems caps the episodes taken from one feed: a hostile feed could
// otherwise fill the database with hundreds of thousands of rows.
const maxFeedItems = 5000

// FetchFeed downloads and parses a podcast feed (at most 20 MB).
func FetchFeed(ctx context.Context, feedURL string) (*rssFeed, error) {
	u, err := url.Parse(strings.TrimSpace(feedURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("enter the feed's web address (http:// or https://)")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WittyMediaServer/0.4 (podcasts)")
	res, err := SafeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("the feed answered %s", res.Status)
	}
	return parseFeed(res.Body)
}

func parseFeed(r io.Reader) (*rssFeed, error) {
	var f rssFeed
	dec := xml.NewDecoder(io.LimitReader(r, 20<<20))
	dec.Strict = false
	dec.CharsetReader = func(_ string, in io.Reader) (io.Reader, error) { return in, nil } // feeds lie about charsets; UTF-8 in practice
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("not a podcast feed: %w", err)
	}
	if f.Channel.Title == "" {
		return nil, errors.New("not a podcast feed")
	}
	return &f, nil
}

var pubLayouts = []string{time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "2 Jan 2006 15:04:05 -0700", time.RFC3339}

func parsePub(v string) string {
	v = strings.TrimSpace(v)
	for _, l := range pubLayouts {
		if t, err := time.Parse(l, v); err == nil {
			return t.UTC().Format("2006-01-02")
		}
	}
	return ""
}

// "1:02:03", "62:03", "3723" -> seconds
func parseDuration(v string) float64 {
	parts := strings.Split(strings.TrimSpace(v), ":")
	total := 0.0
	for _, p := range parts {
		n, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return 0
		}
		total = total*60 + n
	}
	return total
}

// Subscribe adds a feed to a podcasts library and loads its episodes.
func (s *Scanner) Subscribe(ctx context.Context, libraryID int64, feedURL string) (*Podcast, error) {
	f, err := FetchFeed(ctx, feedURL)
	if err != nil {
		return nil, err
	}
	res, err := s.Store.DB.ExecContext(ctx, `INSERT INTO podcasts(library_id, url, title, created_at) VALUES(?,?,?,?)`, libraryID, strings.TrimSpace(feedURL), f.Channel.Title, time.Now().Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("you already follow this podcast")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	p, err := s.Store.Podcast(ctx, id)
	if err != nil {
		return nil, err
	}
	return p, s.applyFeed(ctx, p, f)
}

// RefreshPodcast reloads one feed.
func (s *Scanner) RefreshPodcast(ctx context.Context, p *Podcast) error {
	f, err := FetchFeed(ctx, p.URL)
	if err != nil {
		return err
	}
	return s.applyFeed(ctx, p, f)
}

func (s *Scanner) applyFeed(ctx context.Context, p *Podcast, f *rssFeed) error {
	ch := f.Channel
	image := ""
	for _, im := range ch.Images { // the iTunes image is usually the larger one
		if webURL(im.Href) != "" {
			image = im.Href
			break
		}
		if image == "" {
			image = webURL(im.URL)
		}
	}
	// Episodes already stored, in one read: a library can hold many thousands.
	known, err := s.Store.podcastEpisodeIDs(ctx, p.LibraryID)
	if err != nil {
		return err
	}
	desc := plain(ch.Description)
	if _, err := s.Store.DB.ExecContext(ctx, `UPDATE podcasts SET title=?, author=?, image=?, description=?, last_checked=? WHERE id=?`,
		ch.Title, ch.Author, image, desc, time.Now().Unix(), p.ID); err != nil {
		return err
	}
	p.Title, p.Author, p.Image, p.Description = ch.Title, ch.Author, image, desc
	if len(ch.Items) > maxFeedItems {
		ch.Items = ch.Items[:maxFeedItems] // feeds list the newest first
	}
	for _, e := range ch.Items {
		if webURL(e.Enclosure.URL) == "" {
			continue // no episode, or not on the web
		}
		guid := firstNonEmpty(e.GUID, e.Enclosure.URL)
		if known[guid] || known[e.Enclosure.URL] {
			continue // already known (it may be downloaded: keep its file path)
		}
		known[guid] = true
		it := &Item{LibraryID: p.LibraryID, Kind: "audio", Path: e.Enclosure.URL, Title: strings.TrimSpace(e.Title), Album: p.Title, AlbumArtist: p.Author, Artist: p.Author,
			Overview: plain(firstNonEmpty(e.Summary, e.Description)), Aired: parsePub(e.PubDate), RuntimeSec: parseDuration(e.Duration), Poster: firstNonEmpty(webURL(e.Image.Href), image),
			Provider: "podcast", ProviderID: firstNonEmpty(e.GUID, e.Enclosure.URL), VideoCodec: codecForMime(e.Enclosure.Type, e.Enclosure.URL)}
		it.SortTitle = SortTitle(it.Title)
		it.Year = yearOf(it.Aired)
		it.Track, it.DiscNo = num(e.Episode), num(e.Season)
		if strings.EqualFold(e.Explicit, "true") || strings.EqualFold(e.Explicit, "yes") || strings.EqualFold(e.Explicit, "explicit") {
			it.ContentRating = "TV-MA" // explicit episodes are hidden from Kids and Guest profiles
		}
		if err := s.Store.upsertItem(ctx, it); err != nil {
			// One bad entry (the same file listed twice) must not hide the rest.
			s.Log.Info("podcast episode skipped", "cat", "library", "podcast", p.Title, "episode", it.Title, "err", err)
		}
	}
	if p.AutoDownload > 0 {
		s.autoDownload(ctx, p)
	}
	return nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// codecForMime guesses the codec of an episode so the player knows whether
// the browser can seek it directly (nearly all podcasts are MP3 or AAC).
func codecForMime(mime, u string) string {
	m := strings.ToLower(mime)
	switch {
	case strings.Contains(m, "mpeg") || strings.HasSuffix(strings.ToLower(strings.Split(u, "?")[0]), ".mp3"):
		return "mp3"
	case strings.Contains(m, "mp4") || strings.Contains(m, "m4a") || strings.Contains(m, "aac"):
		return "aac"
	case strings.Contains(m, "ogg") || strings.Contains(m, "opus"):
		return "opus"
	}
	return "mp3"
}

// podcastEpisodeIDs lists what identifies the podcast episodes of a library:
// their GUIDs and web addresses. A feed that renames itself keeps its episodes.
func (s *Store) podcastEpisodeIDs(ctx context.Context, libraryID int64) (map[string]bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT IFNULL(provider_id, ''), path FROM items WHERE library_id = ? AND provider = 'podcast'`, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		out[id], out[path] = true, true
	}
	return out, rows.Err()
}

// Podcast loads one subscription.
func (s *Store) Podcast(ctx context.Context, id int64) (*Podcast, error) {
	var p Podcast
	err := s.DB.QueryRowContext(ctx, `SELECT id, library_id, url, title, IFNULL(author, ''), IFNULL(image, ''), IFNULL(description, ''), IFNULL(last_checked, 0), auto_download FROM podcasts WHERE id = ?`, id).
		Scan(&p.ID, &p.LibraryID, &p.URL, &p.Title, &p.Author, &p.Image, &p.Description, &p.LastChecked, &p.AutoDownload)
	if err != nil {
		return nil, ErrNotFound
	}
	return &p, nil
}

// Podcasts lists the subscriptions of a library (0 = all).
func (s *Store) Podcasts(ctx context.Context, libraryID int64) ([]Podcast, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, library_id, url, title, IFNULL(author, ''), IFNULL(image, ''), IFNULL(description, ''), IFNULL(last_checked, 0), auto_download
		FROM podcasts WHERE ? = 0 OR library_id = ? ORDER BY title COLLATE NOCASE`, libraryID, libraryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Podcast{}
	for rows.Next() {
		var p Podcast
		if err := rows.Scan(&p.ID, &p.LibraryID, &p.URL, &p.Title, &p.Author, &p.Image, &p.Description, &p.LastChecked, &p.AutoDownload); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Unsubscribe removes a podcast and its episodes that were not downloaded
// (downloaded files stay on disk and in the library).
func (s *Store) Unsubscribe(ctx context.Context, id int64) error {
	p, err := s.Podcast(ctx, id)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM items WHERE library_id = ? AND provider = 'podcast' AND IFNULL(album, '') = ? AND (path LIKE 'http://%' OR path LIKE 'https://%')`, p.LibraryID, p.Title); err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM podcasts WHERE id = ?`, id)
	return err
}

var unsafeName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

func safeName(s string) string {
	s = strings.TrimSpace(unsafeName.ReplaceAllString(s, " "))
	s = strings.TrimRight(s, ". ")
	for len(s) > 120 { // cut between characters, not inside one
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	s = strings.TrimRight(s, ". ")
	if s == "" {
		s = "Episode"
	}
	// Names Windows keeps for devices, with or without an extension.
	if !filepath.IsLocal(s+".mp3") || reservedNameRe.MatchString(s) {
		s = "_" + s
	}
	return s
}


// Download saves an episode into the library's first folder:
// <folder>/<podcast>/<date> <title>.<ext>, and the item then plays from disk.
func (s *Scanner) Download(ctx context.Context, it *Item) error {
	if !strings.HasPrefix(it.Path, "http://") && !strings.HasPrefix(it.Path, "https://") {
		return nil // already on disk
	}
	lib, err := s.Store.Library(ctx, it.LibraryID)
	if err != nil || len(lib.Paths) == 0 {
		return errors.New("to download episodes, give this podcasts library a folder in Settings, Libraries")
	}
	ext := strings.ToLower(filepath.Ext(strings.Split(it.Path, "?")[0]))
	if !audioExts[ext] {
		ext = map[string]string{"aac": ".m4a", "opus": ".opus"}[it.VideoCodec]
		if ext == "" {
			ext = ".mp3"
		}
	}
	dir := filepath.Join(lib.Paths[0], safeName(it.Album))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, safeName(strings.TrimSpace(it.Aired+" "+it.Title))+ext)
	req, err := http.NewRequestWithContext(ctx, "GET", it.Path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "WittyMediaServer/0.4 (podcasts)")
	res, err := SafeClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", res.Status)
	}
	f, err := os.CreateTemp(dir, ".wms-*.part")
	if err != nil {
		return err
	}
	tmp := f.Name()
	n, err := io.Copy(f, io.LimitReader(res.Body, 4<<30))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	// A feed chooses the name: it must not replace a file already there.
	dst = freeName(dst)
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	st, _ := os.Stat(dst)
	it.Path, it.SizeBytes = dst, n
	if st != nil {
		it.MTime = st.ModTime().Unix()
	}
	return s.Store.upsertItem(ctx, it)
}

// freeName is path, or path with a number added when something has that name.
func freeName(path string) string {
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		if _, err := os.Lstat(path); err != nil {
			return path
		}
		path = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
}

// autoDownload keeps the newest N episodes of a podcast on disk.
func (s *Scanner) autoDownload(ctx context.Context, p *Podcast) {
	items, _, err := s.Store.Items(ctx, ItemQuery{LibraryID: p.LibraryID, Kind: "audio", Album: &p.Title, Sort: "aired", Desc: true, Limit: p.AutoDownload})
	if err != nil {
		return
	}
	for _, it := range items {
		if err := s.Download(ctx, it); err != nil {
			s.Log.Info("podcast download failed", "cat", "library", "episode", it.Title, "err", err)
		}
	}
}

// RefreshPodcasts reloads every feed (run periodically).
func (s *Scanner) RefreshPodcasts(ctx context.Context) {
	all, err := s.Store.Podcasts(ctx, 0)
	if err != nil {
		return
	}
	for i := range all {
		if err := s.RefreshPodcast(ctx, &all[i]); err != nil {
			s.Log.Info("podcast refresh failed", "cat", "library", "podcast", all[i].Title, "err", err)
		}
	}
}
