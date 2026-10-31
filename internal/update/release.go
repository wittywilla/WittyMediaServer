package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// Repo is where releases are published.
	Repo = "wittywilla/WittyMediaServer"
	// ReleasesPage is shown when a release can't be installed from the app.
	ReleasesPage = "https://github.com/" + Repo + "/releases"
	apiURL       = "https://api.github.com/repos/" + Repo + "/releases?per_page=20"

	maxFeedBytes  = 2 << 20
	maxSumsBytes  = 256 << 10
	maxSigBytes   = 4 << 10
	maxArchive    = 1 << 30 // the largest package is a few hundred MB
	maxNotesBytes = 64 << 10
)

// Overrides for development builds made with -tags updatetest (see
// devfeed_updatetest.go). They are never set in release builds.
var (
	devKeys       []string              // extra trusted public keys
	devFeedURL    string                // replaces the GitHub releases API
	devModuleFeed string                // replaces it for plugins: <this>/<Repository>.json
	devAllowURL   func(u *url.URL) bool // extra hosts downloads may come from
	devTransport  http.RoundTripper     // replaces the internet-only transport
)

// Asset is one file attached to a release.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// Release is a published version with the files this system needs.
type Release struct {
	Version    string
	Title      string
	Notes      string
	Page       string
	Published  time.Time
	Prerelease bool
	Archive    Asset // the package for this OS and architecture
	Sums       Asset // wittymediaserver-<version>-SHA256SUMS.txt
	Sig        Asset // its .sig
}

// Signed: the release carries a checksum list and its signature.
func (r *Release) Signed() bool { return r.Sums.URL != "" && r.Sig.URL != "" }

type ghRelease = GHRelease

// GHRelease is one entry of GitHub's list of a repository's releases.
type GHRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// ArchiveName is the release package for a platform, as built by
// scripts/build-release.sh.
func ArchiveName(ver, goos, goarch string) string {
	label := goarch
	if goos == "linux" && goarch == "arm" {
		label = "armv7"
	}
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}
	return "wittymediaserver-" + ver + "-" + goos + "-" + label + ext
}

// SumsName is a release's checksum list.
func SumsName(ver string) string { return "wittymediaserver-" + ver + "-SHA256SUMS.txt" }

// allowedURL: downloads come from GitHub over HTTPS only.
func allowedURL(u *url.URL) bool {
	if devAllowURL != nil && devAllowURL(u) {
		return true
	}
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "github.com", "api.github.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com":
		return true
	}
	return false
}

// latest picks the newest release above current that has a package for
// goos/goarch. Drafts and unreadable versions are skipped.
func latest(rels []ghRelease, current, goos, goarch string) *Release {
	var best *Release
	for _, g := range rels {
		ver := strings.TrimPrefix(g.TagName, "v")
		if g.Draft || !Valid(ver) || Compare(ver, current) <= 0 || (best != nil && Compare(ver, best.Version) <= 0) {
			continue
		}
		r := &Release{Version: ver, Title: g.Name, Notes: g.Body, Page: g.HTMLURL, Published: g.PublishedAt, Prerelease: g.Prerelease}
		want := ArchiveName(ver, goos, goarch)
		for _, a := range g.Assets {
			as := Asset{Name: a.Name, URL: a.URL, Size: a.Size}
			switch a.Name {
			case want:
				r.Archive = as
			case SumsName(ver):
				r.Sums = as
			case SumsName(ver) + ".sig":
				r.Sig = as
			}
		}
		if r.Archive.URL == "" {
			continue
		}
		if len(r.Notes) > maxNotesBytes {
			r.Notes = r.Notes[:maxNotesBytes] + "…"
		}
		if r.Title == "" {
			r.Title = "WittyMediaServer " + ver
		}
		if u, err := url.Parse(r.Page); err != nil || !allowedURL(u) {
			r.Page = ReleasesPage
		}
		best = r
	}
	return best
}

// ErrNotFound: GitHub has no such repository or file.
var ErrNotFound = errors.New("GitHub answered 404 Not Found")

// Fetcher downloads from GitHub, and from nowhere else. Plugins' releases use
// it too (internal/modules), so everything this server installs comes the same
// way.
type Fetcher struct {
	UserAgent string
	Transport http.RoundTripper
}

func (f *Fetcher) allowed(u *url.URL) bool {
	return allowedURL(u)
}

// client builds an HTTP client that only follows redirects to allowed hosts.
func (f *Fetcher) client() *http.Client {
	tr := f.Transport
	if devTransport != nil {
		tr = devTransport
	}
	return &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !f.allowed(req.URL) {
				return errors.New("refusing a redirect away from GitHub")
			}
			return nil
		},
	}
}

// Get asks for an address; the caller closes the answer's body.
func (f *Fetcher) Get(ctx context.Context, raw string, accept string) (*http.Response, error) {
	u, err := url.Parse(raw)
	if err != nil || !f.allowed(u) {
		return nil, fmt.Errorf("refusing to download from %q (only GitHub release files are allowed)", raw)
	}
	req, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	res, err := f.client().Do(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		if res.StatusCode == 403 || res.StatusCode == 429 {
			return nil, errors.New("GitHub is limiting requests from this network; try again in an hour")
		}
		if res.StatusCode == 404 {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("GitHub answered %s", res.Status)
	}
	return res, nil
}

// Small downloads a small file completely, refusing anything larger than
// limit.
func (f *Fetcher) Small(ctx context.Context, raw string, limit int64) ([]byte, error) {
	res, err := f.Get(ctx, raw, "")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s is larger than expected", raw)
	}
	return b, nil
}

// Releases lists a repository's releases, newest first as GitHub gives them.
func (f *Fetcher) Releases(ctx context.Context, repo string) ([]GHRelease, error) {
	feed := "https://api.github.com/repos/" + repo + "/releases?per_page=20"
	if devModuleFeed != "" {
		feed = devModuleFeed + "/" + repo[strings.LastIndexByte(repo, '/')+1:] + ".json"
	}
	return f.feed(ctx, feed)
}

func (f *Fetcher) feed(ctx context.Context, feed string) ([]GHRelease, error) {
	res, err := f.Get(ctx, feed, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var rels []GHRelease
	if err := json.NewDecoder(io.LimitReader(res.Body, maxFeedBytes)).Decode(&rels); err != nil {
		return nil, fmt.Errorf("unreadable release list: %w", err)
	}
	return rels, nil
}

func (u *Updater) fetcher() *Fetcher {
	return &Fetcher{UserAgent: "WittyMediaServer/" + u.Current, Transport: u.Transport}
}

func (u *Updater) get(ctx context.Context, raw string, accept string) (*http.Response, error) {
	return u.fetcher().Get(ctx, raw, accept)
}

func (u *Updater) fetchSmall(ctx context.Context, raw string, limit int64) ([]byte, error) {
	return u.fetcher().Small(ctx, raw, limit)
}

func (u *Updater) fetchReleases(ctx context.Context) ([]ghRelease, error) {
	feed := apiURL
	if devFeedURL != "" {
		feed = devFeedURL
	}
	return u.fetcher().feed(ctx, feed)
}
