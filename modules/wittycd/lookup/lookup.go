// Package lookup finds out what an audio CD is, from MusicBrainz and the Cover Art
// Archive, using nothing but its table of contents: no names, no audio, and nothing
// about the person or their files. No key is needed.
//
// It needs no image and no server. A program reading a real drive can fill in a
// wittycd.TOC from the drive's own table of contents and ask the same way.
//
//	matches, err := (&lookup.Client{UserAgent: "MyPlayer/1.0 ( https://example.org )"}).Find(ctx, toc)
//
// The services ask for a client that names itself and for at most one request a
// second. Every Client in a program shares one gate, so that holds however many
// there are. Their data is CC0; cover pictures belong to their owners. Show
// Attribution wherever their data is used.
package lookup

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittycd"
)

// Attribution is what to show wherever their data is used.
const Attribution = "Album data from MusicBrainz (CC0). Covers from the Cover Art Archive."

const defaultUserAgent = "WittyCD/0.1 ( https://github.com/wittywilla/WittyCD )"

// ErrBusy says the service asked to slow down or can't be reached. Try again later.
var ErrBusy = errors.New("MusicBrainz can't be reached just now")

// gate keeps a whole program to one request a second.
var gate = struct {
	sync.Mutex
	next time.Time
}{}

func wait(ctx context.Context) error {
	gate.Lock()
	now := time.Now()
	at := gate.next
	if at.Before(now) {
		at = now
	}
	gate.next = at.Add(1100 * time.Millisecond)
	gate.Unlock()
	select {
	case <-time.After(time.Until(at)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Match is one release a disc could be.
type Match struct {
	ReleaseID string `json:"releaseId"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Date      string `json:"date,omitempty"`
	Country   string `json:"country,omitempty"`
	Barcode   string `json:"barcode,omitempty"`
	Label     string `json:"label,omitempty"`
	// Disc is which disc of the release this one is, of Discs.
	Disc  int `json:"disc"`
	Discs int `json:"discs"`
	// Exact: the release lists this very disc ID. Otherwise its track lengths are
	// only close, so it could be another pressing, or a different album.
	Exact    bool        `json:"exact"`
	HasCover bool        `json:"hasCover"`
	Tracks   []TrackName `json:"tracks"`
	Group    string      `json:"group,omitempty"`
	URL      string      `json:"url"`
}

// Year is the year of a match, or 0 when it doesn't say.
func (m *Match) Year() int {
	if len(m.Date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(m.Date[:4])
	if err != nil || y < 1000 {
		return 0
	}
	return y
}

// TrackName is a track's name. Artist is empty when it is the album's.
type TrackName struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Artist string `json:"artist,omitempty"`
}

// SameAlbum says whether matches all name one album with the same tracks, as the many
// pressings of a disc do, so any of them will do without asking anyone.
func SameAlbum(ms []Match) bool {
	if len(ms) == 0 {
		return false
	}
	norm := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	for i := range ms {
		if !ms[i].Exact || norm(ms[i].Title) != norm(ms[0].Title) || norm(ms[i].Artist) != norm(ms[0].Artist) || len(ms[i].Tracks) != len(ms[0].Tracks) {
			return false
		}
		for j := range ms[i].Tracks {
			if norm(ms[i].Tracks[j].Title) != norm(ms[0].Tracks[j].Title) {
				return false
			}
		}
	}
	return true
}

// Client asks the services. The zero value works.
type Client struct {
	// HTTP is the client to use; nil means one with no special limits. A server
	// should give one that can't be redirected onto its own network.
	HTTP *http.Client
	// UserAgent names your program, with a way to contact you, as the service asks:
	// "MyPlayer/1.0 ( https://example.org )".
	UserAgent string
	// Base and CoverBase replace the services' addresses in tests.
	Base, CoverBase string
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return defaultUserAgent
}

func (c *Client) base() string {
	if c.Base != "" {
		return c.Base
	}
	return "https://musicbrainz.org/ws/2"
}

func (c *Client) coverBase() string {
	if c.CoverBase != "" {
		return c.CoverBase
	}
	return "https://coverartarchive.org"
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
	Barcode      string       `json:"barcode"`
	ArtistCredit artistCredit `json:"artist-credit"`
	LabelInfo    []struct {
		Label *struct {
			Name string `json:"name"`
		} `json:"label"`
	} `json:"label-info"`
	ReleaseGroup *struct {
		PrimaryType string `json:"primary-type"`
		FirstDate   string `json:"first-release-date"`
	} `json:"release-group"`
	CoverArt struct {
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

func (c *Client) get(ctx context.Context, u string, into any) (bool, error) {
	if err := wait(ctx); err != nil {
		return false, err
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", u, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", c.userAgent())
	req.Header.Set("Accept", "application/json")
	res, err := c.http().Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrBusy, err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return false, nil
	case res.StatusCode != http.StatusOK:
		return false, fmt.Errorf("%w: it answered %d", ErrBusy, res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(into); err != nil {
		return false, fmt.Errorf("%w: its answer can't be read", ErrBusy)
	}
	return true, nil
}

// Find returns the releases a disc could be, the likeliest first: those listing its
// disc ID, or, when none does, those whose track lengths match. An empty answer means
// nobody knows the disc yet.
func (c *Client) Find(ctx context.Context, toc wittycd.TOC) ([]Match, error) {
	id := toc.MusicBrainzID()
	q := url.Values{"fmt": {"json"}, "cdstubs": {"no"}, "inc": {"artist-credits recordings release-groups labels"}}
	// toc= keeps its + signs, as the service expects.
	u := c.base() + "/discid/" + url.PathEscape(id) + "?" + q.Encode() + "&toc=" + toc.LookupString()
	var body struct {
		Releases []release `json:"releases"`
	}
	found, err := c.get(ctx, u, &body)
	if err != nil {
		return nil, err
	}
	out := []Match{}
	if !found {
		return out, nil
	}
	for i := range body.Releases {
		if m, ok := body.Releases[i].match(id, len(toc.Offsets)); ok {
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

// Release asks for one release by its ID, for when a person picked it.
func (c *Client) Release(ctx context.Context, releaseID string, toc wittycd.TOC) (*Match, error) {
	if !ValidID(releaseID) {
		return nil, errors.New("not a release ID")
	}
	q := url.Values{"fmt": {"json"}, "inc": {"artist-credits recordings release-groups labels discids"}}
	var r release
	found, err := c.get(ctx, c.base()+"/release/"+releaseID+"?"+q.Encode(), &r)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("MusicBrainz has no such release")
	}
	m, ok := r.match(toc.MusicBrainzID(), len(toc.Offsets))
	if !ok {
		return nil, errors.New("that release doesn't have a disc with this many tracks")
	}
	return &m, nil
}

// match reads one release for this disc: which of its discs this is, and the names
// of that disc's tracks.
func (r *release) match(discID string, tracks int) (Match, bool) {
	if !ValidID(r.ID) {
		return Match{}, false
	}
	pick := -1
	exact := false
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
	m := Match{ReleaseID: r.ID, Title: r.Title, Artist: album, Date: r.Date, Country: r.Country, Barcode: r.Barcode,
		Disc: med.Position, Discs: len(r.Media), Exact: exact, HasCover: r.CoverArt.Front, Tracks: []TrackName{},
		URL: "https://musicbrainz.org/release/" + r.ID}
	if m.Disc == 0 {
		m.Disc = pick + 1
	}
	if r.ReleaseGroup != nil {
		m.Group = r.ReleaseGroup.PrimaryType
		if m.Date == "" {
			m.Date = r.ReleaseGroup.FirstDate
		}
	}
	if len(r.LabelInfo) > 0 && r.LabelInfo[0].Label != nil {
		m.Label = r.LabelInfo[0].Label.Name
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

var idRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ValidID says whether this is a MusicBrainz release ID.
func ValidID(id string) bool { return idRe.MatchString(id) }

// Cover fetches a release's front cover, 250, 500 or 1200 pixels across.
func (c *Client) Cover(ctx context.Context, releaseID string, size int) ([]byte, error) {
	if !ValidID(releaseID) {
		return nil, errors.New("not a release ID")
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", c.coverBase()+"/release/"+releaseID+"/front-"+strconv.Itoa(size), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent())
	res, err := c.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBusy, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("no cover for this release")
	}
	return io.ReadAll(io.LimitReader(res.Body, 20<<20))
}
