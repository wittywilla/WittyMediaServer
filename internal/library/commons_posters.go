package library

import (
	"context"
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Posters of films from Wikimedia Commons, for a server with no TMDB key. Wikidata names a film's poster
// file; Commons holds only freely licensed files, and says for each one its licence and author, which are
// credited wherever this server shows where its pictures came from (GET /api/artwork/credits). Most
// posters of recent films aren't free and so aren't there: this finds the ones that are.

const (
	wdFilmPoster = "P3383" // film poster: a Commons file
	wdImage      = "P18"   // image: a Commons file, often a still, sometimes the poster
)

// The width of the thumbnail asked for: a poster card's largest.
const commonsPosterWidth = 500

func (s wdSnak) str() string {
	var v string
	_ = json.Unmarshal(s.Datavalue.Value, &v)
	return v
}

// posterFile is the Commons file of a film's poster: its film poster (P3383), or else its image (P18) when
// that's plainly a poster or key art by its name. A still from the film isn't a poster.
func posterFile(e wdEntity) string {
	for _, c := range e.Claims[wdFilmPoster] {
		if f := c.Mainsnak.str(); f != "" {
			return f
		}
	}
	for _, c := range e.Claims[wdImage] {
		f := c.Mainsnak.str()
		if l := strings.ToLower(f); containsAny(l, "poster", "key art", "keyart", "key_art") {
			return f
		}
	}
	return ""
}

// CommonsPicture is a picture on Wikimedia Commons, with what its licence asks to be said about it.
type CommonsPicture struct {
	URL        string `json:"-"` // the thumbnail, on upload.wikimedia.org
	Page       string `json:"page"`
	License    string `json:"license"`
	LicenseURL string `json:"licenseUrl,omitempty"`
	Credit     string `json:"credit,omitempty"`
}

func (w *Wikidata) commons() string {
	if w.CommonsAPI != "" {
		return w.CommonsAPI
	}
	return commonsAPI
}

func commonsKey(name string) string {
	return strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(name), "File:"), "_", " ")
}

// commonsPictures reads Commons' facts about files, by name ("Poster.jpg"), with a thumbnail width wide.
// A file whose licence isn't plainly free, or that isn't on Wikimedia's own servers, is left out.
func (w *Wikidata) commonsPictures(ctx context.Context, files []string, width int) (map[string]CommonsPicture, error) {
	out := map[string]CommonsPicture{}
	for len(files) > 0 {
		n := min(len(files), 50) // Commons answers about 50 at a time
		batch := files[:n]
		files = files[n:]
		titles := make([]string, len(batch))
		for i, f := range batch {
			titles[i] = "File:" + commonsKey(f)
		}
		var res struct {
			Query struct {
				Normalized []struct{ From, To string } `json:"normalized"`
				Pages      map[string]struct {
					Title     string `json:"title"`
					ImageInfo []struct {
						ThumbURL       string `json:"thumburl"`
						DescriptionURL string `json:"descriptionurl"`
						Meta           map[string]struct {
							Value json.RawMessage `json:"value"`
						} `json:"extmetadata"`
					} `json:"imageinfo"`
				} `json:"pages"`
			} `json:"query"`
		}
		q := url.Values{"action": {"query"}, "titles": {strings.Join(titles, "|")}, "prop": {"imageinfo"}, "iiprop": {"url|extmetadata"},
			"iiurlwidth": {strconv.Itoa(width)}, "iiextmetadatafilter": {"LicenseShortName|LicenseUrl|Artist|NonFree"}, "format": {"json"}}
		if err := w.get(ctx, w.commons()+"?"+q.Encode(), &res); err != nil {
			return out, err
		}
		asked := map[string]string{} // what Commons calls a file -> the name it was asked by
		for _, f := range batch {
			asked[commonsKey(f)] = commonsKey(f)
		}
		for _, nm := range res.Query.Normalized {
			if a, ok := asked[commonsKey(nm.From)]; ok {
				asked[commonsKey(nm.To)] = a
			}
		}
		for _, p := range res.Query.Pages {
			if len(p.ImageInfo) == 0 {
				continue
			}
			ii := p.ImageInfo[0]
			meta := func(k string) string {
				var s string
				_ = json.Unmarshal(ii.Meta[k].Value, &s)
				return strings.TrimSpace(s)
			}
			license := meta("LicenseShortName")
			if strings.EqualFold(meta("NonFree"), "true") || !FreeLogoLicense(license) || !wikimediaHost(ii.ThumbURL) {
				continue
			}
			name, ok := asked[commonsKey(p.Title)]
			if !ok {
				continue
			}
			out[name] = CommonsPicture{URL: plainAddress(ii.ThumbURL), Page: webURL(ii.DescriptionURL), License: license, LicenseURL: webURL(meta("LicenseUrl")),
				Credit: commonsCredit(meta("Artist"))}
		}
	}
	return out, nil
}

// plainAddress is a picture's address without what Commons adds for its statistics ("?utm_source=…"), so one
// file always has one address.
func plainAddress(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.RawQuery, u.Fragment = "", ""
	return u.String()
}

// A part of Commons' author field written for machines and hidden from people: "Unknown author<span
// style="display: none;">Unknown author</span>".
var hiddenSpanRe = regexp.MustCompile(`(?is)<span[^>]*display:\s*none[^>]*>.*?</span>`)

// commonsCredit is Commons' author field as words.
func commonsCredit(artist string) string {
	s := htmlTagsRe.ReplaceAllString(hiddenSpanRe.ReplaceAllString(artist, ""), "")
	return clip(strings.Join(strings.Fields(html.UnescapeString(s)), " "), 300)
}

// commonsPoster is the thumbnail of a film's poster on Commons, or "" when it has none that's free.
func (w *Wikidata) commonsPoster(ctx context.Context, e wdEntity) string {
	f := posterFile(e)
	if f == "" {
		return ""
	}
	pics, err := w.commonsPictures(ctx, []string{f}, commonsPosterWidth)
	if err != nil {
		return ""
	}
	return pics[commonsKey(f)].URL
}

// WikidataPoster is the free poster from Commons of the film with this Wikidata id, or "".
func WikidataPoster(ctx context.Context, w *Wikidata, id string) string {
	if !wikiIDRe.MatchString(id) {
		return ""
	}
	ents, err := w.entities(ctx, []string{id}, "claims")
	if err != nil {
		return ""
	}
	e, ok := ents[id]
	if !ok {
		return ""
	}
	return w.commonsPoster(ctx, e)
}

// commonsPosters is the same for several films at once, by their ids.
func (w *Wikidata) commonsPosters(ctx context.Context, ents map[string]wdEntity) map[string]string {
	files := map[string]string{}
	var names []string
	for id, e := range ents {
		if f := posterFile(e); f != "" {
			files[id] = commonsKey(f)
			names = append(names, f)
		}
	}
	out := map[string]string{}
	if len(names) == 0 {
		return out
	}
	pics, _ := w.commonsPictures(ctx, names, commonsPosterWidth)
	for id, f := range files {
		if p, ok := pics[f]; ok {
			out[id] = p.URL
		}
	}
	return out
}

// CommonsFileOf is the name of the Commons file a picture's address shows, or "" for any other address.
// "https://upload.wikimedia.org/wikipedia/commons/thumb/a/ab/A_poster.jpg/500px-A_poster.jpg" is "A poster.jpg",
// and so is the same on thumb.wikimedia.org, where Commons now puts some thumbnails.
func CommonsFileOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	if h := strings.ToLower(u.Hostname()); h != "upload.wikimedia.org" && h != "thumb.wikimedia.org" {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) < 5 || parts[0] != "wikipedia" || parts[1] != "commons" {
		return ""
	}
	i := 2
	if parts[2] == "thumb" {
		i = 3
	}
	if len(parts) < i+3 || len(parts[i]) != 1 || len(parts[i+1]) != 2 {
		return ""
	}
	name, err := url.PathUnescape(parts[i+2])
	if err != nil {
		return ""
	}
	return commonsKey(name)
}

// What Commons said about files, kept a while: the credits are read far more often than files change.
var commonsCache = struct {
	sync.Mutex
	at   map[string]time.Time
	pics map[string]CommonsPicture
}{at: map[string]time.Time{}, pics: map[string]CommonsPicture{}}

const commonsCacheFor = 6 * time.Hour

// CommonsCredits says, for each address that is a picture on Commons, where it came from and under what
// licence. Addresses of anything else, and files Commons no longer has as free, are left out.
func CommonsCredits(ctx context.Context, w *Wikidata, addresses []string) map[string]CommonsPicture {
	byFile := map[string][]string{}
	for _, a := range addresses {
		if f := CommonsFileOf(a); f != "" {
			byFile[f] = append(byFile[f], a)
		}
	}
	out := map[string]CommonsPicture{}
	var ask []string
	commonsCache.Lock()
	for f, as := range byFile {
		if t, ok := commonsCache.at[f]; ok && time.Since(t) < commonsCacheFor {
			if p, ok := commonsCache.pics[f]; ok {
				for _, a := range as {
					out[a] = p
				}
			}
			continue
		}
		ask = append(ask, f)
	}
	commonsCache.Unlock()
	if len(ask) == 0 {
		return out
	}
	sort.Strings(ask)
	pics, err := w.commonsPictures(ctx, ask, commonsPosterWidth)
	commonsCache.Lock()
	defer commonsCache.Unlock()
	for _, f := range ask {
		p, ok := pics[f]
		if ok || err == nil {
			// Remembered either way once Commons answered: a file it no longer has as free stays uncredited
			// rather than being asked about on every visit.
			commonsCache.at[f] = time.Now()
			if ok {
				commonsCache.pics[f] = p
			} else {
				delete(commonsCache.pics, f)
			}
		}
		if ok {
			for _, a := range byFile[f] {
				out[a] = p
			}
		}
	}
	return out
}

// CommonsArt is a title whose poster or backdrop is a picture from Commons.
type CommonsArt struct {
	Target   string
	ID       int64
	Title    string
	Poster   string
	Backdrop string
}

// CommonsArtwork lists the titles an account may see whose poster or backdrop comes from Commons.
func (s *Store) CommonsArtwork(ctx context.Context, acc *Access) ([]CommonsArt, error) {
	const commons = "https://%.wikimedia.org/wikipedia/commons/%"
	ic, iargs := acc.ItemClause()
	sc, sargs := acc.ShowClause()
	args := append(append([]any{commons, commons}, iargs...), commons, commons)
	args = append(args, sargs...)
	rows, err := s.DB.QueryContext(ctx, `SELECT 'item', i.id, i.title, IFNULL(i.poster,''), IFNULL(i.backdrop,'') FROM items i
		WHERE (i.poster LIKE ? OR i.backdrop LIKE ?) AND `+ic+`
		UNION ALL SELECT 'show', s.id, s.title, IFNULL(s.poster,''), IFNULL(s.backdrop,'') FROM shows s
		WHERE (s.poster LIKE ? OR s.backdrop LIKE ?) AND `+sc+` ORDER BY 3 LIMIT 2000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommonsArt
	for rows.Next() {
		var a CommonsArt
		if err := rows.Scan(&a.Target, &a.ID, &a.Title, &a.Poster, &a.Backdrop); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
