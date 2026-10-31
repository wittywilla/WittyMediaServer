package library

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// The station directory: Radio Browser (radio-browser.info), a free community
// database with an open API. It is contacted only when an administrator searches
// or adds from it, and once a day for stations that came from it, unless the
// "radioDirectory" setting is off. Every answer is treated as hostile: addresses
// are checked, text is cleaned, and sizes and times are limited. A logo is never
// fetched by a viewer's browser, only through the server's artwork cache.

// RadioDirectorySource names the directory on stations that came from it.
const RadioDirectorySource = "radio-browser"

const (
	directoryUserAgent  = "WittyMediaServer/0.5 (internet radio)"
	maxDirectoryAnswer  = 4 << 20
	directoryTimeout    = 8 * time.Second
	directoryTries      = 3
	directorySearchTTL  = 10 * time.Minute
	directoryFacetsTTL  = 12 * time.Hour
	directoryServersTTL = time.Hour
	maxDirectoryCached  = 128
	maxDirectoryResults = 50
	maxDirectoryOffset  = 5000
	directoryDomain     = ".radio-browser.info"
)

// ErrDirectoryUnavailable: no directory server gave a usable answer.
var ErrDirectoryUnavailable = errors.New("the station directory isn't answering; try again later, or add the station by its address")

// DirectoryStation is a station as a directory lists it, already cleaned.
type DirectoryStation struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Homepage    string   `json:"homepage,omitempty"`
	Logo        string   `json:"logo,omitempty"`
	Tags        []string `json:"tags"`
	Country     string   `json:"country,omitempty"`
	CountryCode string   `json:"countryCode,omitempty"`
	Language    string   `json:"language,omitempty"`
	Codec       string   `json:"codec,omitempty"`
	Bitrate     int      `json:"bitrate,omitempty"`
	HLS         bool     `json:"hls"`
	Votes       int      `json:"votes,omitempty"`
	Working     bool     `json:"working"` // the directory's own latest check reached it
	Added       bool     `json:"added"`   // already in the library that was asked about
}

// DirectoryCount is one entry of a browse list, with how many stations it has.
type DirectoryCount struct {
	Name     string `json:"name"`
	Code     string `json:"code,omitempty"`
	Stations int    `json:"stations"`
}

// DirectoryFacets is what the directory can be browsed by.
type DirectoryFacets struct {
	Countries []DirectoryCount `json:"countries"`
	Tags      []DirectoryCount `json:"tags"`
	Languages []DirectoryCount `json:"languages"`
}

// DirectoryQuery is a search of the directory.
type DirectoryQuery struct {
	Text     string
	Tag      string
	Country  string // two-letter code
	Language string
	Limit    int
	Offset   int
}

// directoryStation is the directory's own form, read loosely: only what is used.
type directoryStation struct {
	UUID        string `json:"stationuuid"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Resolved    string `json:"url_resolved"`
	Homepage    string `json:"homepage"`
	Favicon     string `json:"favicon"`
	Tags        string `json:"tags"`
	Country     string `json:"country"`
	CountryCode string `json:"countrycode"`
	Language    string `json:"language"`
	Codec       string `json:"codec"`
	Bitrate     int    `json:"bitrate"`
	HLS         int    `json:"hls"`
	LastCheckOK int    `json:"lastcheckok"`
	Votes       int    `json:"votes"`
}

var (
	directoryIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	countryCodeRe = regexp.MustCompile(`^[A-Za-z]{2}$`)
	hostLabelRe   = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9-]+)*$`)
)

// ValidDirectoryID: the form of a directory's station ID.
func ValidDirectoryID(id string) bool { return directoryIDRe.MatchString(id) }

// publicHost is a first look at an address from a directory: it isn't obviously on
// a private network. What counts is SafeClient, which checks where each connection
// really goes; this only keeps such entries out of the results.
func publicHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return internetAddr(ip)
	}
	if host == "localhost" || !strings.Contains(host, ".") {
		return false
	}
	for _, s := range []string{".localhost", ".local", ".lan", ".home", ".internal", ".intranet", ".corp", ".home.arpa"} {
		if strings.HasSuffix(host, s) {
			return false
		}
	}
	return true
}

// cleanStreamAddress keeps a station address from a directory only if it is a web
// address on the internet, with nothing hidden in it.
func cleanStreamAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if !ValidStreamURL(raw) || strings.ContainsAny(raw, " \t\r\n\\") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || !publicHost(u.Hostname()) {
		return ""
	}
	return raw
}

// cleanHomepage keeps a link that is safe to show and open.
func cleanHomepage(raw string) string {
	raw = cleanStreamAddress(raw)
	if len(raw) > 500 {
		return ""
	}
	return raw
}

// clean makes a directory's entry safe to use; ok is false when it can't be played.
func (d directoryStation) clean() (DirectoryStation, bool) {
	addr := cleanStreamAddress(firstNonEmpty(d.Resolved, d.URL))
	if addr == "" || !ValidDirectoryID(d.UUID) {
		return DirectoryStation{}, false
	}
	u, _ := url.Parse(addr)
	st := DirectoryStation{ID: strings.ToLower(d.UUID), URL: addr, Name: firstNonEmpty(CleanRadioText(d.Name, 200), u.Hostname()), Homepage: cleanHomepage(d.Homepage), Logo: cleanHomepage(d.Favicon),
		Tags: []string{}, Country: CleanRadioText(d.Country, 80), Language: firstListed(d.Language), Codec: CleanRadioText(strings.ToUpper(d.Codec), 16), HLS: d.HLS != 0, Votes: max(d.Votes, 0), Working: d.LastCheckOK != 0}
	if strings.EqualFold(st.Codec, "UNKNOWN") {
		st.Codec = ""
	}
	if countryCodeRe.MatchString(d.CountryCode) {
		st.CountryCode = strings.ToUpper(d.CountryCode)
	}
	if d.Bitrate > 0 && d.Bitrate < 100000 {
		st.Bitrate = d.Bitrate
	}
	seen := map[string]bool{}
	for _, t := range strings.Split(d.Tags, ",") {
		if t = CleanRadioText(t, 40); t != "" && !seen[strings.ToLower(t)] && len(st.Tags) < 8 {
			seen[strings.ToLower(t)] = true
			st.Tags = append(st.Tags, t)
		}
	}
	return st, true
}

// firstListed is the first language of a directory's comma-separated list.
func firstListed(s string) string {
	first, _, _ := strings.Cut(s, ",")
	return CleanRadioText(first, 40)
}

// ---------------------------------------------------------------------------
// Asking the directory
// ---------------------------------------------------------------------------

// RadioDirectoryServers lists the directory's servers, in the order to try them.
// Tests replace it with a local stand-in.
var RadioDirectoryServers = radioDirectoryServers

var directoryCache struct {
	sync.Mutex
	servers   []string
	serversAt time.Time
	facets    *DirectoryFacets
	facetsAt  time.Time
	searches  map[string]directorySearch
}

type directorySearch struct {
	at      time.Time
	results []DirectoryStation
}

// radioDirectoryServers finds the servers as the directory asks: its DNS names
// them, and clients pick at random. Only names under its own domain are used.
func radioDirectoryServers(ctx context.Context) []string {
	directoryCache.Lock()
	cached, at := directoryCache.servers, directoryCache.serversAt
	directoryCache.Unlock()
	if len(cached) == 0 || time.Since(at) > directoryServersTTL {
		lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		found := []string{}
		if _, srv, err := net.DefaultResolver.LookupSRV(lctx, "api", "tcp", "radio-browser.info"); err == nil {
			for _, r := range srv {
				name := strings.ToLower(strings.TrimSuffix(r.Target, "."))
				if strings.HasSuffix(name, directoryDomain) && hostLabelRe.MatchString(name) {
					found = append(found, "https://"+name)
				}
			}
		}
		if len(found) == 0 {
			found = []string{"https://all.api.radio-browser.info"}
		}
		directoryCache.Lock()
		directoryCache.servers, directoryCache.serversAt = found, time.Now()
		directoryCache.Unlock()
		cached = found
	}
	out := append([]string(nil), cached...)
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// ResetRadioDirectoryCache forgets the directory's answers (for tests).
func ResetRadioDirectoryCache() {
	directoryCache.Lock()
	defer directoryCache.Unlock()
	directoryCache.servers, directoryCache.facets, directoryCache.searches = nil, nil, nil
}

// directoryGet asks the directory, moving on to another server when one fails.
func directoryGet(ctx context.Context, path string, q url.Values, out any) error {
	servers := RadioDirectoryServers(ctx)
	if len(servers) > directoryTries {
		servers = servers[:directoryTries]
	}
	target := path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	for _, base := range servers {
		if err := directoryAsk(ctx, base+target, out); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return ErrDirectoryUnavailable
}

func directoryAsk(ctx context.Context, addr string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, directoryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", directoryUserAgent)
	req.Header.Set("Accept", "application/json")
	res, err := SafeClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.New("the directory answered " + strconv.Itoa(res.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxDirectoryAnswer+1))
	if err != nil {
		return err
	}
	if len(body) > maxDirectoryAnswer {
		return errors.New("the directory's answer is too long")
	}
	return json.Unmarshal(body, out)
}

// DirectorySearch searches the directory. Answers are kept for a few minutes, so
// typing and paging back don't ask it the same thing twice.
func DirectorySearch(ctx context.Context, q DirectoryQuery) ([]DirectoryStation, error) {
	q.Text, q.Tag, q.Language = CleanRadioText(q.Text, 100), CleanRadioText(q.Tag, 60), CleanRadioText(q.Language, 60)
	if q.Country != "" && !countryCodeRe.MatchString(q.Country) {
		return nil, ErrBadInput("that isn't a country code")
	}
	if q.Limit <= 0 || q.Limit > maxDirectoryResults {
		q.Limit = 30
	}
	q.Offset = min(max(q.Offset, 0), maxDirectoryOffset)
	key := strings.Join([]string{q.Text, q.Tag, strings.ToUpper(q.Country), q.Language, strconv.Itoa(q.Limit), strconv.Itoa(q.Offset)}, "\x00")
	directoryCache.Lock()
	if c, ok := directoryCache.searches[key]; ok && time.Since(c.at) < directorySearchTTL {
		directoryCache.Unlock()
		return c.results, nil
	}
	directoryCache.Unlock()

	results, err := directoryFind(ctx, q, q.Text)
	if err != nil {
		return nil, err
	}
	// A word that names few stations may be a genre: "jazz" matches the tag too.
	if q.Text != "" && q.Tag == "" && q.Offset == 0 && len(results) < 10 {
		byTag := q
		byTag.Tag = q.Text
		if more, err := directoryFind(ctx, byTag, ""); err == nil {
			results = append(results, more...)
		}
	}
	results = uniqueStations(results, q.Limit)

	directoryCache.Lock()
	defer directoryCache.Unlock()
	if directoryCache.searches == nil || len(directoryCache.searches) >= maxDirectoryCached {
		directoryCache.searches = map[string]directorySearch{}
	}
	directoryCache.searches[key] = directorySearch{time.Now(), results}
	return results, nil
}

func directoryFind(ctx context.Context, q DirectoryQuery, name string) ([]DirectoryStation, error) {
	v := url.Values{"hidebroken": {"true"}, "order": {"clickcount"}, "reverse": {"true"}, "limit": {strconv.Itoa(q.Limit)}, "offset": {strconv.Itoa(q.Offset)}}
	if name != "" {
		v.Set("name", name)
	}
	if q.Tag != "" {
		v.Set("tag", q.Tag)
	}
	if q.Country != "" {
		v.Set("countrycode", strings.ToUpper(q.Country))
	}
	if q.Language != "" {
		v.Set("language", q.Language)
	}
	var raw []directoryStation
	if err := directoryGet(ctx, "/json/stations/search", v, &raw); err != nil {
		return nil, err
	}
	return cleanStations(raw), nil
}

func cleanStations(raw []directoryStation) []DirectoryStation {
	out := make([]DirectoryStation, 0, min(len(raw), maxDirectoryResults))
	for _, d := range raw {
		if len(out) == maxDirectoryResults {
			break
		}
		if st, ok := d.clean(); ok {
			out = append(out, st)
		}
	}
	return out
}

func uniqueStations(in []DirectoryStation, limit int) []DirectoryStation {
	seen := map[string]bool{}
	out := make([]DirectoryStation, 0, len(in))
	for _, st := range in {
		if !seen[st.ID] && !seen[st.URL] && len(out) < limit {
			seen[st.ID], seen[st.URL] = true, true
			out = append(out, st)
		}
	}
	return out
}

// DirectoryByID reads stations by their IDs; ones the directory no longer has are
// simply not in the answer.
func DirectoryByID(ctx context.Context, ids []string) ([]DirectoryStation, error) {
	for _, id := range ids {
		if !ValidDirectoryID(id) {
			return nil, ErrBadInput("that isn't a station in the directory")
		}
	}
	var raw []directoryStation
	if err := directoryGet(ctx, "/json/stations/byuuid", url.Values{"uuids": {strings.Join(ids, ",")}}, &raw); err != nil {
		return nil, err
	}
	return cleanStations(raw), nil
}

// directoryByURL finds a station by its exact address.
func directoryByURL(ctx context.Context, addr string) ([]DirectoryStation, error) {
	var raw []directoryStation
	if err := directoryGet(ctx, "/json/stations/byurl", url.Values{"url": {addr}}, &raw); err != nil {
		return nil, err
	}
	return cleanStations(raw), nil
}

// directoryClick tells the directory a station was chosen, which it asks of apps,
// so its popular lists mean something. It is counted once, when a station is
// added; what anyone listens to afterwards isn't reported.
func directoryClick(id string) {
	go func() {
		defer logging.Recover("radio directory click", "cat", "library")
		ctx, cancel := context.WithTimeout(context.Background(), directoryTimeout)
		defer cancel()
		var ignored struct{}
		_ = directoryGet(ctx, "/json/url/"+id, nil, &ignored)
	}()
}

// DirectoryBrowse gives what the directory can be browsed by, kept for hours:
// these lists barely change.
func DirectoryBrowse(ctx context.Context) (*DirectoryFacets, error) {
	directoryCache.Lock()
	if f, at := directoryCache.facets, directoryCache.facetsAt; f != nil && time.Since(at) < directoryFacetsTTL {
		directoryCache.Unlock()
		return f, nil
	}
	directoryCache.Unlock()
	type counted struct {
		Name     string `json:"name"`
		Code     string `json:"iso_3166_1"`
		Stations int    `json:"stationcount"`
	}
	list := func(path string, limit int) ([]counted, error) {
		var raw []counted
		err := directoryGet(ctx, path, url.Values{"hidebroken": {"true"}, "order": {"stationcount"}, "reverse": {"true"}, "limit": {strconv.Itoa(limit)}}, &raw)
		return raw, err
	}
	clean := func(raw []counted, withCode bool, limit int) []DirectoryCount {
		out := []DirectoryCount{}
		for _, c := range raw {
			name := CleanRadioText(c.Name, 40)
			if name == "" || c.Stations < 1 || len(out) == limit {
				continue
			}
			e := DirectoryCount{Name: name, Stations: c.Stations}
			if withCode {
				if !countryCodeRe.MatchString(c.Code) {
					continue
				}
				e.Code = strings.ToUpper(c.Code)
			}
			out = append(out, e)
		}
		return out
	}
	countries, err := list("/json/countries", 250)
	if err != nil {
		return nil, err
	}
	tags, err := list("/json/tags", 80)
	if err != nil {
		return nil, err
	}
	langs, err := list("/json/languages", 80)
	if err != nil {
		return nil, err
	}
	f := &DirectoryFacets{Countries: clean(countries, true, 250), Tags: clean(tags, false, 60), Languages: clean(langs, false, 60)}
	sort.Slice(f.Countries, func(i, j int) bool { return f.Countries[i].Name < f.Countries[j].Name })
	directoryCache.Lock()
	directoryCache.facets, directoryCache.facetsAt = f, time.Now()
	directoryCache.Unlock()
	return f, nil
}
