// Package cdlookup asks MusicBrainz and the Cover Art Archive what a CD is. Only the disc's
// table of contents is sent: no audio, no names and no file paths. It runs only when the
// viewer asks, in the updater program, the only part of the player with network code.
package cdlookup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const userAgent = "WittyMediaPlayer/0.1 ( https://github.com/wittywilla/WittyMediaPlayer )"

// Client talks to the services. The bases are only set by tests.
type Client struct {
	HTTP            *http.Client
	Base, CoverBase string
}

// New makes a client.
func New() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Second}, Base: "https://musicbrainz.org/ws/2", CoverBase: "https://coverartarchive.org"}
}

// Match is one release a disc could be.
type Match struct {
	ReleaseID string `json:"releaseId"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Date      string `json:"date,omitempty"`
	Country   string `json:"country,omitempty"`
	Disc      int    `json:"disc"`
	Discs     int    `json:"discs"`
	// Exact: the release lists this very disc. Otherwise its track count only fits.
	Exact    bool        `json:"exact"`
	HasCover bool        `json:"hasCover"`
	Tracks   []TrackName `json:"tracks"`
}

// TrackName is what a track is called.
type TrackName struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Artist string `json:"artist,omitempty"`
}

type artistCredit []struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
}

func (a artistCredit) text() string {
	var b strings.Builder
	for _, c := range a {
		b.WriteString(c.Name)
		b.WriteString(c.JoinPhrase)
	}
	return b.String()
}

type release struct {
	ID           string       `json:"id"`
	Title        string       `json:"title"`
	Date         string       `json:"date"`
	Country      string       `json:"country"`
	ArtistCredit artistCredit `json:"artist-credit"`
	CoverArt     struct {
		Front bool `json:"front"`
	} `json:"cover-art-archive"`
	Media []struct {
		Position int `json:"position"`
		Discs    []struct {
			ID string `json:"id"`
		} `json:"discs"`
		TrackCount int `json:"track-count"`
		Tracks     []struct {
			Position     int          `json:"position"`
			Title        string       `json:"title"`
			ArtistCredit artistCredit `json:"artist-credit"`
		} `json:"tracks"`
	} `json:"media"`
}

var idRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// discIDRe is what a disc ID and a table of contents can be, so neither can carry anything else.
var (
	discIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	tocRe    = regexp.MustCompile(`^[0-9+]+$`)
)

// ErrBusy is the service asking to slow down, or out of reach.
var ErrBusy = errors.New("MusicBrainz can't be reached just now")

func (c *Client) get(ctx context.Context, u, accept string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, limit))
	return b, res.StatusCode, err
}

// Lookup finds the releases a disc could be, exact matches first. An empty answer means
// nobody knows the disc yet.
func (c *Client) Lookup(ctx context.Context, discID, toc string, tracks int) ([]Match, error) {
	if !discIDRe.MatchString(discID) || len(toc) > 1024 || !tocRe.MatchString(toc) {
		return nil, errors.New("that isn't a disc's table of contents")
	}
	q := url.Values{"fmt": {"json"}, "cdstubs": {"no"}, "inc": {"artist-credits recordings"}}
	b, status, err := c.get(ctx, c.Base+"/discid/"+url.PathEscape(discID)+"?"+q.Encode()+"&toc="+toc, "application/json", 8<<20)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBusy, err)
	}
	switch status {
	case http.StatusOK:
	case http.StatusNotFound:
		return []Match{}, nil
	default:
		return nil, fmt.Errorf("%w: it answered %d", ErrBusy, status)
	}
	var body struct {
		Releases []release `json:"releases"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return nil, fmt.Errorf("%w: its answer can't be read", ErrBusy)
	}
	out := []Match{}
	for i := range body.Releases {
		if m, ok := body.Releases[i].match(discID, tracks); ok {
			out = append(out, m)
		}
	}
	rank := func(m *Match) int {
		r := 0
		if !m.Exact {
			r += 2
		}
		if !m.HasCover {
			r++
		}
		return r
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(&out[i]) < rank(&out[j]) })
	if len(out) > 25 {
		out = out[:25]
	}
	return out, nil
}

func (r *release) match(discID string, tracks int) (Match, bool) {
	if !idRe.MatchString(r.ID) {
		return Match{}, false
	}
	pick, exact := -1, false
	for i := range r.Media {
		for _, d := range r.Media[i].Discs {
			if d.ID == discID {
				pick, exact = i, true
			}
		}
		if pick >= 0 {
			break
		}
	}
	if pick < 0 {
		for i := range r.Media {
			if r.Media[i].TrackCount == tracks || len(r.Media[i].Tracks) == tracks {
				pick = i
				break
			}
		}
	}
	if pick < 0 {
		return Match{}, false
	}
	med := &r.Media[pick]
	album := r.ArtistCredit.text()
	m := Match{ReleaseID: r.ID, Title: r.Title, Artist: album, Date: r.Date, Country: r.Country, Disc: med.Position, Discs: len(r.Media), Exact: exact, HasCover: r.CoverArt.Front, Tracks: []TrackName{}}
	if m.Disc == 0 {
		m.Disc = pick + 1
	}
	for _, t := range med.Tracks {
		artist := t.ArtistCredit.text()
		if artist == album {
			artist = ""
		}
		m.Tracks = append(m.Tracks, TrackName{Number: t.Position, Title: t.Title, Artist: artist})
	}
	return m, true
}

// Cover fetches a release's front cover, or says there is none.
func (c *Client) Cover(ctx context.Context, releaseID string) ([]byte, error) {
	if !idRe.MatchString(releaseID) {
		return nil, errors.New("that isn't a release")
	}
	b, status, err := c.get(ctx, c.CoverBase+"/release/"+releaseID+"/front-500", "image/*", 16<<20)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBusy, err)
	}
	if status != http.StatusOK {
		return nil, errors.New("this release has no cover to fetch")
	}
	return b, nil
}
