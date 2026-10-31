// Package modupdate looks for and installs newer signed releases of the player's disc
// modules. It's the only code in the player that uses the network, and it lives in a
// program of its own that runs only when the viewer asks.
package modupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaplayer/engine/internal/modhost"
	"github.com/wittywilla/wittymodule"
)

// Limits on a release, as for the server's modules.
const (
	maxList     = 2 << 20
	maxSums     = 256 << 10
	maxSig      = 4 << 10
	maxManifest = 64 << 10
	maxArchive  = 256 << 20
	maxUnpacked = 2 * maxArchive
	tooMany     = 5 // newer releases looked at, newest first
)

// Client talks to GitHub.
type Client struct {
	Mgr  *modhost.Manager
	HTTP *http.Client
	// API is where releases are listed. Tests point it elsewhere, along with AnyHost.
	API     string
	AnyHost bool
}

// New makes a client for GitHub, over HTTPS only.
func New(m *modhost.Manager) *Client {
	return &Client{Mgr: m, API: "https://api.github.com", HTTP: &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != "https" {
			return errors.New("that download goes somewhere it shouldn't")
		}
		return nil
	}}}
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type ghRelease struct {
	Tag        string    `json:"tag_name"`
	Draft      bool      `json:"draft"`
	Prerelease bool      `json:"prerelease"`
	Assets     []ghAsset `json:"assets"`
}

func (c *Client) allowed(raw string) bool {
	if c.AnyHost {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || h == "api.github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

func (c *Client) get(ctx context.Context, raw string, limit int64) ([]byte, error) {
	if !c.allowed(raw) {
		return nil, errors.New("that address isn't one modules come from")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "WittyMediaPlayer")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("that file is bigger than it should be")
	}
	return b, nil
}

// release is a release whose list and manifest have been verified.
type release struct {
	version   string
	man       *wittymodule.Manifest
	sums, sig []byte
	archive   ghAsset
}

func (c *Client) releases(ctx context.Context, mod modhost.Module) ([]ghRelease, error) {
	b, err := c.get(ctx, c.API+"/repos/"+mod.Repo+"/releases?per_page=50", maxList)
	if err != nil {
		return nil, err
	}
	var all []ghRelease
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, errors.New("GitHub's answer is unreadable")
	}
	return all, nil
}

func asset(g ghRelease, name string) (ghAsset, bool) {
	for _, a := range g.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return ghAsset{}, false
}

// verified fetches a release's small files and believes nothing before checking them.
func (c *Client) verified(ctx context.Context, mod modhost.Module, version string, g ghRelease) (*release, error) {
	sumsA, ok1 := asset(g, wittymodule.SumsName(mod.ID, version))
	sigA, ok2 := asset(g, wittymodule.SumsName(mod.ID, version)+".sig")
	manA, ok3 := asset(g, wittymodule.ManifestName(mod.ID, version))
	arc, ok4 := asset(g, wittymodule.ArchiveName(mod.ID, version, runtime.GOOS, runtime.GOARCH))
	if !ok1 || !ok2 || !ok3 {
		return nil, errors.New("its files are incomplete")
	}
	if !ok4 {
		return nil, errors.New("it has no build for this computer")
	}
	sums, err := c.get(ctx, sumsA.URL, maxSums)
	if err != nil {
		return nil, err
	}
	sig, err := c.get(ctx, sigA.URL, maxSig)
	if err != nil {
		return nil, err
	}
	if err := modhost.VerifySums(sums, sig, c.Mgr.Keys); err != nil {
		return nil, err
	}
	list, err := modhost.ParseSums(sums)
	if err != nil {
		return nil, err
	}
	raw, err := c.get(ctx, manA.URL, maxManifest)
	if err != nil {
		return nil, err
	}
	if list[manA.Name] != modhost.Sum(raw) {
		return nil, errors.New("its manifest doesn't match its signed checksum")
	}
	man, err := wittymodule.ParseManifest(raw)
	if err != nil {
		return nil, err
	}
	// The signature says who made it, and this says it's this version of this module,
	// so a release of one can't be passed off as another, or an old one under a new tag.
	if man.ID != mod.ID || man.Version != version || !strings.EqualFold(man.Repo, mod.Repo) || man.Delivery != wittymodule.DeliveryHelper {
		return nil, fmt.Errorf("its manifest says %s %s", man.ID, man.Version)
	}
	return &release{version: version, man: man, sums: sums, sig: sig, archive: arc}, nil
}

// Offer is what a check found for one module.
type Offer struct {
	ID      string `json:"id"`
	Version string `json:"version,omitempty"`
	Note    string `json:"note,omitempty"`
}

// versionOf reads a module's release tag: <id>-v1.2.3. The engine's own tags are left
// alone, as a server reads those.
func versionOf(id, tag string) (string, bool) {
	v, ok := strings.CutPrefix(tag, id+"-v")
	return v, ok && wittymodule.Valid(v)
}

// Check looks for the newest release that fits this player. A newer one that doesn't fit
// is reported, so the reason can be seen.
func (c *Client) Check(ctx context.Context, mod modhost.Module) Offer {
	out := Offer{ID: mod.ID}
	all, err := c.releases(ctx, mod)
	if err != nil {
		out.Note = "Couldn't look for updates: " + err.Error()
		return out
	}
	now := c.Mgr.Choose(mod).Version
	type candidate struct {
		v string
		g ghRelease
	}
	var list []candidate
	for _, g := range all {
		v, ok := versionOf(mod.ID, g.Tag)
		// A pre-release only goes to a player that's one itself, or already has one.
		if !ok || g.Draft || wittymodule.Compare(v, now) <= 0 || (wittymodule.Prerelease(v) && !wittymodule.Prerelease(c.Mgr.Host) && !wittymodule.Prerelease(now)) {
			continue
		}
		list = append(list, candidate{v, g})
	}
	sort.Slice(list, func(i, j int) bool { return wittymodule.Compare(list[i].v, list[j].v) > 0 })
	blocked := c.Mgr.LoadState(mod.ID).Blocked
	for i, cand := range list {
		if i >= tooMany {
			break
		}
		r, err := c.verified(ctx, mod, cand.v, cand.g)
		if err != nil {
			if out.Note == "" {
				out.Note = fmt.Sprintf("Version %s can't be used: %s.", cand.v, err)
			}
			continue
		}
		if err := c.Mgr.Supports(r.man); err != nil {
			if out.Note == "" {
				out.Note = err.Error() + "."
			}
			continue
		}
		if why, no := blocked[cand.v]; no {
			if out.Note == "" {
				out.Note = fmt.Sprintf("Version %s isn't offered: %s.", cand.v, why)
			}
			continue
		}
		out.Version = cand.v
		out.Note = ""
		return out
	}
	return out
}

// Install fetches a version and makes it the one in use. Each step leaves what's in use
// untouched if it fails.
func (c *Client) Install(ctx context.Context, mod modhost.Module, version string) error {
	if !wittymodule.Valid(version) {
		return errors.New("that isn't a version")
	}
	all, err := c.releases(ctx, mod)
	if err != nil {
		return err
	}
	var g *ghRelease
	for i := range all {
		if v, ok := versionOf(mod.ID, all[i].Tag); ok && v == version {
			g = &all[i]
		}
	}
	if g == nil {
		return fmt.Errorf("there's no release of version %s", version)
	}
	// 1. The signature, then the manifest and whether it fits this player.
	r, err := c.verified(ctx, mod, version, *g)
	if err != nil {
		return fmt.Errorf("version %s can't be used: %w", version, err)
	}
	if err := c.Mgr.Supports(r.man); err != nil {
		return err
	}
	list, err := modhost.ParseSums(r.sums)
	if err != nil {
		return err
	}
	// 2. The archive, which must match the signed list.
	if r.archive.Size > maxArchive {
		return errors.New("that release is bigger than it should be")
	}
	arc, err := c.get(ctx, r.archive.URL, maxArchive)
	if err != nil {
		return err
	}
	if list[r.archive.Name] != modhost.Sum(arc) {
		return errors.New("the download doesn't match its signed checksum")
	}
	versions := filepath.Dir(c.Mgr.VersionDir(mod.ID, version))
	if err := os.MkdirAll(versions, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(versions, ".install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	// 3. Unpacked with no links, and nothing climbing out of its folder.
	top := wittymodule.TopFolder(mod.ID, version, runtime.GOOS, runtime.GOARCH)
	if err := unpack(arc, top, stage); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, modhost.SignedList), r.sums, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, modhost.SignedSig), r.sig, 0o644); err != nil {
		return err
	}
	// 4. Every file against the signed list, with nothing extra and nothing missing.
	man, err := c.Mgr.VerifyDir(mod, version, stage)
	if err != nil {
		return err
	}
	// 5. It has to start and say it's what it's supposed to be.
	rem, err := modhost.Start(filepath.Join(stage, man.EntryFile(runtime.GOOS)), mod.ID, version)
	if err != nil {
		return fmt.Errorf("version %s: %w", version, err)
	}
	rem.Stop()
	// 6. Moved into place, then remembered.
	dest := c.Mgr.VersionDir(mod.ID, version)
	_ = os.RemoveAll(dest)
	if err := renameSoon(stage, dest); err != nil {
		return err
	}
	return c.Mgr.Installed(mod, version)
}

// renameSoon allows for a program that has only just run: on Windows, the system or a
// virus scanner can hold on to its folder for a moment after it ends.
func renameSoon(from, to string) error {
	var err error
	for i := 0; i < 50; i++ {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// safeName takes a path in an archive to one under its top folder, or refuses it.
func safeName(name, top string) (string, error) {
	name = strings.ReplaceAll(name, `\`, "/")
	rel, ok := strings.CutPrefix(name, top+"/")
	if !ok || rel == "" {
		return "", fmt.Errorf("%q isn't where it should be in the release", name)
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." || part == "." || part == "" || strings.Contains(part, ":") {
			return "", fmt.Errorf("%q is an unsafe name in the release", name)
		}
	}
	return rel, nil
}

func unpack(arc []byte, top, dest string) error {
	if len(arc) > 2 && arc[0] == 'P' && arc[1] == 'K' {
		return unzip(arc, top, dest)
	}
	return untar(arc, top, dest)
}

func writeFile(dest, rel string, r io.Reader, left *int64) error {
	p := filepath.Join(dest, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, io.LimitReader(r, *left+1))
	if err != nil {
		return err
	}
	if *left -= n; *left < 0 {
		return errors.New("that release unpacks to more than it should")
	}
	return nil
}

func unzip(arc []byte, top, dest string) error {
	z, err := zip.NewReader(bytes.NewReader(arc), int64(len(arc)))
	if err != nil {
		return err
	}
	left := int64(maxUnpacked)
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return errors.New("links aren't allowed in a release")
		}
		rel, err := safeName(f.Name, top)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFile(dest, rel, rc, &left)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untar(arc []byte, top, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(arc))
	if err != nil {
		return err
	}
	defer gz.Close()
	t := tar.NewReader(gz)
	left := int64(maxUnpacked)
	for {
		h, err := t.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeReg:
		default:
			return errors.New("links aren't allowed in a release")
		}
		rel, err := safeName(h.Name, top)
		if err != nil {
			return err
		}
		if err := writeFile(dest, rel, t, &left); err != nil {
			return err
		}
	}
}
