package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MusicBrainz identifies audio CDs by their table of contents, and the Cover Art
// Archive has their covers. Only the disc ID and the track offsets are sent: no
// names, no audio and nothing about the person or their files. No key is needed.
// Their rules ask for a named client and at most one request a second.

// MusicBrainzAttribution is what is shown wherever their data is used.
const MusicBrainzAttribution = "Album data from MusicBrainz (CC0). Covers from the Cover Art Archive."

const mbUA = "WittyMediaServer/0.5 ( https://github.com/wittywilla/WittyMediaServer )"

// mbGate keeps the whole server to one request a second, however many libraries
// are scanning.
var mbGate = struct {
	sync.Mutex
	next time.Time
}{}

func mbWait(ctx context.Context) error {
	mbGate.Lock()
	now := time.Now()
	at := mbGate.next
	if at.Before(now) {
		at = now
	}
	mbGate.next = at.Add(1100 * time.Millisecond)
	mbGate.Unlock()
	select {
	case <-time.After(time.Until(at)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CDMatch is one release a disc could be.
type CDMatch struct {
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
	Exact    bool          `json:"exact"`
	HasCover bool          `json:"hasCover"`
	Tracks   []CDTrackName `json:"tracks"`
	Group    string        `json:"group,omitempty"`
	URL      string        `json:"url"`
}

// CDTrackName is a track's name from any source.
type CDTrackName struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Artist string `json:"artist,omitempty"`
}

// MusicBrainz is a client of the service. The bases are only set by tests.
type MusicBrainz struct {
	Base, CoverBase string
}

type mbArtistCredit []struct {
	Name       string `json:"name"`
	JoinPhrase string `json:"joinphrase"`
}

func (a mbArtistCredit) text() string {
	var b strings.Builder
	for _, c := range a {
		b.WriteString(c.Name)
		b.WriteString(c.JoinPhrase)
	}
	return b.String()
}

type mbRelease struct {
	ID           string         `json:"id"`
	Title        string         `json:"title"`
	Date         string         `json:"date"`
	Country      string         `json:"country"`
	Barcode      string         `json:"barcode"`
	ArtistCredit mbArtistCredit `json:"artist-credit"`
	LabelInfo    []struct {
		Label *struct {
			Name string `json:"name"`
		} `json:"label"`
	} `json:"label-info"`
	ReleaseGroup *struct {
		ID          string `json:"id"`
		PrimaryType string `json:"primary-type"`
		FirstDate   string `json:"first-release-date"`
	} `json:"release-group"`
	CoverArt struct {
		Front bool `json:"front"`
	} `json:"cover-art-archive"`
	Media []struct {
		Position int    `json:"position"`
		Format   string `json:"format"`
		Title    string `json:"title"`
		Discs    []struct {
			ID string `json:"id"`
		} `json:"discs"`
		TrackCount int `json:"track-count"`
		Tracks     []struct {
			Position     int            `json:"position"`
			Title        string         `json:"title"`
			ArtistCredit mbArtistCredit `json:"artist-credit"`
		} `json:"tracks"`
	} `json:"media"`
}

// ErrMusicBrainzBusy: the service asked us to slow down, or can't be reached.
var ErrMusicBrainzBusy = errors.New("MusicBrainz can't be reached just now")

func (m *MusicBrainz) base() string {
	if m.Base != "" {
		return m.Base
	}
	return "https://musicbrainz.org/ws/2"
}

func (m *MusicBrainz) coverBase() string {
	if m.CoverBase != "" {
		return m.CoverBase
	}
	return "https://coverartarchive.org"
}

// Lookup finds the releases a disc could be: those listing its disc ID, or, when
// none does, those whose track lengths match. An empty answer means nobody knows
// the disc yet.
func (m *MusicBrainz) Lookup(ctx context.Context, discID, toc string, tracks int) ([]CDMatch, error) {
	q := url.Values{"fmt": {"json"}, "cdstubs": {"no"}, "inc": {"artist-credits recordings release-groups labels"}}
	// toc= keeps its + signs, as the service expects.
	u := m.base() + "/discid/" + url.PathEscape(discID) + "?" + q.Encode() + "&toc=" + toc
	if err := mbWait(ctx); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mbUA)
	req.Header.Set("Accept", "application/json")
	res, err := SafeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMusicBrainzBusy, err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return []CDMatch{}, nil
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: it answered %d", ErrMusicBrainzBusy, res.StatusCode)
	}
	var body struct {
		Releases []mbRelease `json:"releases"`
	}
	if err := json.NewDecoder(limitBody(res, 8<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: its answer can't be read", ErrMusicBrainzBusy)
	}
	out := make([]CDMatch, 0, len(body.Releases))
	for i := range body.Releases {
		if match, ok := body.Releases[i].match(discID, tracks); ok {
			out = append(out, match)
		}
	}
	// Exact matches first, and those with a cover before those without.
	rank := func(m *CDMatch) int {
		r := 0
		if !m.Exact {
			r += 2
		}
		if !m.HasCover {
			r++
		}
		return r
	}
	sortStable(out, func(a, b *CDMatch) bool { return rank(a) < rank(b) })
	if len(out) > 25 {
		out = out[:25]
	}
	return out, nil
}

// match reads one release for this disc: which of its discs this is, and the
// names of that disc's tracks.
func (r *mbRelease) match(discID string, tracks int) (CDMatch, bool) {
	if r.ID == "" || !validMBID(r.ID) {
		return CDMatch{}, false
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
		return CDMatch{}, false
	}
	med := &r.Media[pick]
	album := r.ArtistCredit.text()
	m := CDMatch{ReleaseID: r.ID, Title: r.Title, Artist: album, Date: r.Date, Country: r.Country, Barcode: r.Barcode,
		Disc: med.Position, Discs: len(r.Media), Exact: exact, HasCover: r.CoverArt.Front, Tracks: []CDTrackName{},
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
		m.Tracks = append(m.Tracks, CDTrackName{Number: t.Position, Title: t.Title, Artist: artist})
	}
	return m, true
}

// sameAlbum says whether matches all name one album with the same tracks, as the
// many pressings of a disc do, so any of them will do without asking.
func sameAlbum(ms []CDMatch) bool {
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

var mbidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func validMBID(id string) bool { return mbidRe.MatchString(id) }

// Cover fetches a release's front cover at 500 pixels, or says there is none.
func (m *MusicBrainz) Cover(ctx context.Context, releaseID string, size int) ([]byte, error) {
	if !validMBID(releaseID) {
		return nil, errors.New("not a release ID")
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, "GET", m.coverBase()+"/release/"+releaseID+"/front-"+strconv.Itoa(size), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mbUA)
	res, err := SafeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMusicBrainzBusy, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("no cover for this release")
	}
	b, err := io.ReadAll(limitBody(res, 20<<20))
	if err != nil {
		return nil, err
	}
	if !isPicture(b) {
		return nil, errors.New("not a picture")
	}
	return b, nil
}

// Release asks for one release by its ID, for when a person picked it.
func (m *MusicBrainz) Release(ctx context.Context, releaseID, discID string, tracks int) (*CDMatch, error) {
	if !validMBID(releaseID) {
		return nil, errors.New("not a release ID")
	}
	if err := mbWait(ctx); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	q := url.Values{"fmt": {"json"}, "inc": {"artist-credits recordings release-groups labels discids"}}
	req, err := http.NewRequestWithContext(cctx, "GET", m.base()+"/release/"+releaseID+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", mbUA)
	req.Header.Set("Accept", "application/json")
	res, err := SafeClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMusicBrainzBusy, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: it answered %d", ErrMusicBrainzBusy, res.StatusCode)
	}
	var r mbRelease
	if err := json.NewDecoder(limitBody(res, 8<<20)).Decode(&r); err != nil {
		return nil, fmt.Errorf("%w: its answer can't be read", ErrMusicBrainzBusy)
	}
	match, ok := r.match(discID, tracks)
	if !ok {
		return nil, errors.New("that release doesn't have a disc with this many tracks")
	}
	return &match, nil
}
