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
	"strings"
	"time"
)

// WikimediaLogos finds logos through Wikidata (CC0), which says which picture is a
// company's logo, on Wikimedia Commons, where every file states its licence and only
// freely licensed or public domain files are allowed. Only files whose licence
// allows this use are taken. A company's trademark in its logo is a separate
// matter, which the rights notice beside logos says.
type WikimediaLogos struct {
	Client *http.Client
	// Addresses, replaced in tests.
	WikidataAPI, CommonsAPI string
}

const (
	wikidataAPI = "https://www.wikidata.org/w/api.php"
	commonsAPI  = "https://commons.wikimedia.org/w/api.php"
	wdLogoImage = "P154"
	wdIcon      = "P2910"
)

// What an entity's short description has to say to be the right sort of thing, and what
// rules it out. A film called the same as a studio has a picture too, and it isn't a logo.
var logoKindWords = map[string][]string{
	LogoStudio:     {"studio", "company", "production", "producer", "animation", "entertainment", "distributor", "pictures", "film", "television", "conglomerate", "subsidiary", "label"},
	LogoNetwork:    {"network", "television", "channel", "broadcast", "streaming", "cable", "tv", "service", "station"},
	LogoConsole:    {"console", "handheld", "video game", "game system", "gaming", "computer", "system"},
	LogoGameStudio: {"video game", "game", "developer", "publisher", "company", "studio", "entertainment", "software"},
	LogoPlatform:   {"streaming", "video on demand", "service", "platform", "subscription", "television", "website"},
}

var logoNotWords = []string{"album", "song", "single", "given name", "family name", "surname", "disambiguation", "human", "actor", "actress", "singer", "episode", "novel", "book", "character", "painting"}

func (w *WikimediaLogos) client() *http.Client {
	if w.Client != nil {
		return w.Client
	}
	return catalogClient(20 * time.Second)
}

func (w *WikimediaLogos) get(ctx context.Context, base string, q url.Values, out any) error {
	wikiGate.Lock()
	wait := time.Until(wikiGate.next)
	wikiGate.next = time.Now().Add(max(wait, 0) + 200*time.Millisecond)
	wikiGate.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	q.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, "GET", base+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", wikiUA)
	res, err := w.client().Do(req)
	if err != nil {
		return withoutURL(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("wikimedia: %s", res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, maxAnswer)).Decode(out)
}

// Words a company's name often has in a title's credits but not in its own entry.
var logoShorter = regexp.MustCompile(`(?i)\s+(pictures|films?|studios?|entertainment|productions?|animation|television|tv|media|group|company)$`)

// Find lists candidates, best first. Exact marks those whose name is the name asked
// about, which are the only ones used without someone choosing.
func (w *WikimediaLogos) Find(ctx context.Context, kind, name string) ([]LogoCandidate, error) {
	out, err := w.findNamed(ctx, kind, name)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		if short := logoShorter.ReplaceAllString(name, ""); short != name && short != "" {
			return w.findNamed(ctx, kind, short)
		}
	}
	return out, nil
}

func (w *WikimediaLogos) findNamed(ctx context.Context, kind, name string) ([]LogoCandidate, error) {
	apiBase := w.WikidataAPI
	if apiBase == "" {
		apiBase = wikidataAPI
	}
	var search struct {
		Search []struct {
			ID string `json:"id"`
		} `json:"search"`
	}
	if err := w.get(ctx, apiBase, url.Values{"action": {"wbsearchentities"}, "search": {name}, "language": {"en"}, "type": {"item"}, "limit": {"12"}}, &search); err != nil {
		return nil, err
	}
	if len(search.Search) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(search.Search))
	for _, r := range search.Search {
		ids = append(ids, r.ID)
	}
	var ents struct {
		Entities map[string]struct {
			Labels       map[string]struct{ Value string }   `json:"labels"`
			Aliases      map[string][]struct{ Value string } `json:"aliases"`
			Descriptions map[string]struct{ Value string }   `json:"descriptions"`
			Claims       map[string][]struct {
				Rank     string `json:"rank"`
				Mainsnak struct {
					Datavalue struct {
						Value json.RawMessage `json:"value"`
					} `json:"datavalue"`
				} `json:"mainsnak"`
			} `json:"claims"`
		} `json:"entities"`
	}
	if err := w.get(ctx, apiBase, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(ids, "|")}, "props": {"labels|aliases|descriptions|claims"}, "languages": {"en"}}, &ents); err != nil {
		return nil, err
	}
	key := BrandKey(name)
	type pick struct {
		id, label, desc, file string
		exact                 bool
	}
	var picks []pick
	for _, id := range ids {
		e, ok := ents.Entities[id]
		if !ok {
			continue
		}
		desc := strings.ToLower(e.Descriptions["en"].Value)
		if containsAny(desc, logoNotWords...) || (desc != "" && !containsAny(desc, logoKindWords[kind]...)) {
			continue
		}
		var file string
		for _, prop := range []string{wdLogoImage, wdIcon} {
			// The newest logo is usually last; one marked as preferred comes first of all.
			for _, c := range e.Claims[prop] {
				var f string
				if json.Unmarshal(c.Mainsnak.Datavalue.Value, &f) != nil || f == "" {
					continue
				}
				file = f
				if c.Rank == "preferred" {
					break
				}
			}
			if file != "" {
				break
			}
		}
		if file == "" {
			continue
		}
		exact := BrandKey(e.Labels["en"].Value) == key
		for _, a := range e.Aliases["en"] {
			exact = exact || BrandKey(a.Value) == key
		}
		picks = append(picks, pick{id, e.Labels["en"].Value, e.Descriptions["en"].Value, file, exact})
	}
	if len(picks) == 0 {
		return nil, nil
	}
	files := make([]string, 0, len(picks))
	for _, p := range picks {
		files = append(files, "File:"+p.file)
	}
	info, err := w.fileInfo(ctx, files)
	if err != nil {
		return nil, err
	}
	out := []LogoCandidate{}
	for _, p := range picks {
		fi, ok := info[normalFileTitle("File:"+p.file)]
		if !ok {
			continue
		}
		fi.Name, fi.Note, fi.Exact = p.label, p.desc, p.exact
		out = append(out, fi)
	}
	// Exact names first, in the order Wikidata ranked them.
	exact := make([]LogoCandidate, 0, len(out))
	for _, c := range out {
		if c.Exact {
			exact = append(exact, c)
		}
	}
	for _, c := range out {
		if !c.Exact {
			exact = append(exact, c)
		}
	}
	return exact, nil
}

func normalFileTitle(t string) string { return strings.ReplaceAll(t, "_", " ") }

var htmlTagsRe = regexp.MustCompile(`<[^>]*>`)

// fileInfo reads Commons' facts about files. Anything whose licence isn't plainly
// one that allows this use is left out.
func (w *WikimediaLogos) fileInfo(ctx context.Context, titles []string) (map[string]LogoCandidate, error) {
	base := w.CommonsAPI
	if base == "" {
		base = commonsAPI
	}
	var res struct {
		Query struct {
			Normalized []struct{ From, To string } `json:"normalized"`
			Pages      map[string]struct {
				Title     string `json:"title"`
				ImageInfo []struct {
					ThumbURL       string `json:"thumburl"`
					DescriptionURL string `json:"descriptionurl"`
					Mime           string `json:"mime"`
					Meta           map[string]struct {
						Value json.RawMessage `json:"value"`
					} `json:"extmetadata"`
				} `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	q := url.Values{"action": {"query"}, "titles": {strings.Join(titles, "|")}, "prop": {"imageinfo"}, "iiprop": {"url|mime|extmetadata"}, "iiurlwidth": {"640"},
		"iiextmetadatafilter": {"LicenseShortName|LicenseUrl|Artist|NonFree|Copyrighted"}}
	if err := w.get(ctx, base, q, &res); err != nil {
		return nil, err
	}
	out := map[string]LogoCandidate{}
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
		artist := strings.TrimSpace(htmlTagsRe.ReplaceAllString(meta("Artist"), ""))
		out[normalFileTitle(p.Title)] = LogoCandidate{Source: "wikimedia", SourceID: strings.TrimPrefix(p.Title, "File:"), ImageURL: ii.ThumbURL,
			PageURL: webURL(ii.DescriptionURL), License: license, LicenseURL: webURL(meta("LicenseUrl")), Credit: clip(artist, 300)}
	}
	return out, nil
}

// FreeLogoLicense: public domain, or a licence that allows reuse, changes (these
// files are recoloured) and commercial use. "No derivatives" and "non-commercial"
// never appear on Commons, and aren't taken if one ever does.
func FreeLogoLicense(license string) bool {
	l := strings.ToLower(license)
	if strings.Contains(l, "-nc") || strings.Contains(l, " nc") || strings.Contains(l, "-nd") || strings.Contains(l, " nd") || strings.Contains(l, "non-free") || strings.Contains(l, "fair use") {
		return false
	}
	return strings.HasPrefix(l, "public domain") || strings.HasPrefix(l, "pd") || strings.HasPrefix(l, "cc0") ||
		strings.HasPrefix(l, "cc by") || strings.HasPrefix(l, "cc-by") || strings.HasPrefix(l, "attribution") || strings.HasPrefix(l, "mit") || strings.HasPrefix(l, "fal")
}

// IsLogoPreview: an address the logo chooser may ask the server to show.
func IsLogoPreview(raw string) bool { return wikimediaHost(raw) }

func wikimediaHost(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "wikimedia.org" || strings.HasSuffix(h, ".wikimedia.org")
}

// Fetch downloads a candidate's picture.
func (w *WikimediaLogos) Fetch(ctx context.Context, c LogoCandidate) ([]byte, error) {
	if w.Client == nil && !wikimediaHost(c.ImageURL) {
		return nil, errors.New("not a Wikimedia address")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.ImageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", wikiUA)
	res, err := w.client().Do(req)
	if err != nil {
		return nil, withoutURL(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("wikimedia: %s", res.Status)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxLogoBytes+1))
	if err != nil {
		return nil, withoutURL(err)
	}
	return b, nil
}
