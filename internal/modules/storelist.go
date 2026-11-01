package modules

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // so a screenshot can be checked
	_ "image/png"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymodule"
)

// What the store's pages show: every module from every store that's on, with what this server
// makes of it. Words from a third-party store are cleaned on the way through; pictures come from
// this server, never from an address in a catalogue.

// Why a version can or can't be installed.
const (
	FitOK        = "ok"
	FitTooOld    = "too-old"
	FitTooNew    = "too-new"
	FitNoBuild   = "no-build"
	FitNone      = "no-version"
	FitLibrary   = "library"
	FitWithdrawn = "withdrawn"
)

// Fit says whether a module can be installed here, and if not, why.
type Fit struct {
	OK   bool   `json:"ok"`
	Kind string `json:"kind"`
	Why  string `json:"why,omitempty"`
}

// StoreItem is one module in the store.
type StoreItem struct {
	Key       string `json:"key"` // store/id
	Store     string `json:"store"`
	StoreName string `json:"storeName"`
	StoreHost string `json:"storeHost"`
	Official  bool   `json:"official"`
	Kind      string `json:"kind"`
	Why       string `json:"why,omitempty"`
	Lookalike string `json:"lookalike,omitempty"` // it has the name of a module from this project

	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Summary     string                 `json:"summary,omitempty"`
	Description string                 `json:"description,omitempty"`
	Group       string                 `json:"group,omitempty"`
	Publisher   *wittymodule.Publisher `json:"publisher,omitempty"`
	Repo        string                 `json:"repo,omitempty"`
	Icon        string                 `json:"icon,omitempty"`
	Screenshots []string               `json:"screenshots,omitempty"`
	Libraries   []string               `json:"libraries,omitempty"`
	Requires    []string               `json:"requires,omitempty"`
	Needs       []string               `json:"needs,omitempty"`
	Core        bool                   `json:"core,omitempty"`
	Updates     string                 `json:"updates,omitempty"`
	Stays       string                 `json:"stays,omitempty"`

	// Of the version the store would offer: the newest that fits, or the newest there is.
	Platforms   []string `json:"platforms,omitempty"`
	Delivery    string   `json:"delivery,omitempty"`
	Version     string   `json:"version,omitempty"`
	Released    string   `json:"released,omitempty"`
	Size        int64    `json:"size,omitempty"`
	License     string   `json:"license,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Changelog   string   `json:"changelog,omitempty"`
	MinServer   string   `json:"minServer,omitempty"`
	MaxServer   string   `json:"maxServer,omitempty"`

	Fit     Fit      `json:"fit"`
	Status  *Status  `json:"status,omitempty"`
	Failure *Failure `json:"failure,omitempty"`
}

// StoreView is everything the store's pages need at once.
type StoreView struct {
	Server      string      `json:"server"`
	API         int         `json:"api"`
	MinAPI      int         `json:"minApi"`
	Policy      string      `json:"policy"`
	MinServer   string      `json:"minServer"`
	Platform    string      `json:"platform"`
	CheckedAt   int64       `json:"checkedAt,omitempty"`
	CheckError  string      `json:"checkError,omitempty"`
	Stores      []StoreInfo `json:"stores"`
	Items       []StoreItem `json:"items"`
	Updates     int         `json:"updates"`
	Run         *Run        `json:"run,omitempty"`
	RepairRoute string      `json:"repairRoute"`
}

// RepairRoute is where the repair tools are, in Settings.
const RepairRoute = "/settings/repair"

func thisPlatform() string { return runtime.GOOS + "-" + runtime.GOARCH }

// listedCatalog is this project's catalogue, and where its relative addresses resolve from.
func (m *Manager) listedCatalog() (*wittymodule.Catalog, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cat, m.catFrom
}

// offer picks the version to show: the newest that fits and hasn't been withdrawn, or failing
// that the newest there is, with why it can't be used.
func (s *Stores) offer(mod *wittymodule.CatalogModule) (*wittymodule.CatalogVersion, Fit) {
	if len(mod.Versions) == 0 {
		return nil, Fit{Kind: FitNone, Why: "No version of " + mod.Name + " is available yet."}
	}
	server := s.m.cfg.Server
	var first Fit
	for i := range mod.Versions {
		v := &mod.Versions[i]
		f := fitOf(mod.Name, v, server)
		if i == 0 {
			first = f
		}
		if f.OK {
			return v, f
		}
	}
	return &mod.Versions[0], first
}

// fitOf says whether one version can be installed on a server of this version, on this system.
func fitOf(name string, v *wittymodule.CatalogVersion, server string) Fit {
	if v.Withdrawn != nil {
		return Fit{Kind: FitWithdrawn, Why: "Its publisher withdrew version " + v.Version + ": " + sentence(cleanText(v.Withdrawn.Reason))}
	}
	// A module built for a server from before this cut-off can't run, whatever range it claims.
	if wittymodule.Compare(v.MinServer, MinModuleServer) < 0 && wittymodule.Compare(server, MinModuleServer) >= 0 {
		return Fit{Kind: FitTooOld, Why: fmt.Sprintf("%s %s was built for a server older than %s, so it can't run here. %s", name, v.Version, MinModuleServer, PolicyWords)}
	}
	if err := v.Supports(name, server, wittymodule.MinAPI, wittymodule.API); err != nil {
		f := Explain(name, err.Error())
		switch f.Kind {
		case FailTooOld:
			return Fit{Kind: FitTooOld, Why: sentence(err.Error()) + " " + PolicyWords}
		case FailTooNew:
			return Fit{Kind: FitTooNew, Why: sentence(err.Error())}
		}
		return Fit{Kind: FitTooNew, Why: sentence(err.Error())}
	}
	for _, p := range v.Permissions {
		if p != wittymodule.PermissionNetwork {
			return Fit{Kind: FitTooNew, Why: fmt.Sprintf("%s %s asks for %q, which this server doesn't know of. A newer server may.", name, v.Version, p)}
		}
	}
	if v.Platform(runtime.GOOS, runtime.GOARCH) == nil {
		return Fit{Kind: FitNoBuild, Why: fmt.Sprintf("%s %s has no build for this kind of computer (%s).", name, v.Version, thisPlatform())}
	}
	return Fit{OK: true, Kind: FitOK}
}

func platformsOf(v *wittymodule.CatalogVersion) []string {
	out := make([]string, 0, len(v.Platforms))
	for p := range v.Platforms {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// mediaURL is where this server serves a catalogue's picture from.
func mediaURL(store, sha string) string { return "/api/modulestore/media/" + store + "/" + sha }

// fillListing copies what a catalogue says of a module, cleaning it when it's a third party's.
func (s *Stores) fillListing(it *StoreItem, mod *wittymodule.CatalogModule, clean func(string) string) {
	it.Name, it.Summary, it.Description = clean(mod.Name), clean(mod.Summary), clean(mod.Description)
	it.Group, it.Repo, it.Libraries, it.Requires, it.Core = mod.Group, mod.Repo, mod.Libraries, mod.Requires, mod.Core
	it.Updates, it.Stays = clean(mod.Updates), clean(mod.Stays)
	it.Needs = nil
	for _, n := range mod.Needs {
		it.Needs = append(it.Needs, clean(n))
	}
	it.Publisher = &wittymodule.Publisher{Name: clean(mod.Publisher.Name), URL: mod.Publisher.URL}
	if mod.Icon != nil {
		it.Icon = mediaURL(it.Store, mod.Icon.SHA256)
	}
	for i := range mod.Screenshots {
		it.Screenshots = append(it.Screenshots, mediaURL(it.Store, mod.Screenshots[i].SHA256))
	}
	v, fit := s.offer(mod)
	it.Fit = fit
	if v == nil {
		return
	}
	it.Version, it.Released, it.License, it.Permissions, it.Delivery = v.Version, v.Released, clean(v.License), v.Permissions, v.Delivery
	it.Changelog, it.MinServer, it.MaxServer = clean(v.Changelog), v.MinServer, v.MaxServer
	it.Platforms = platformsOf(v)
	if f := v.Platform(runtime.GOOS, runtime.GOARCH); f != nil {
		it.Size = f.Size
	}
}

// failureFor is what's wrong with a module now, if anything. A failed install comes first, then
// a version that can't be used.
func failureFor(st *Status) *Failure {
	switch {
	case st == nil:
		return nil
	case st.Phase == PhaseFailed && st.Error != "":
		return Explain(st.Name, st.Error)
	case st.Error != "":
		return Explain(st.Name, st.Error)
	case st.Notice != "":
		return Explain(st.Name, st.Notice)
	}
	return nil
}

// View builds the store.
func (s *Stores) View() StoreView {
	rep := s.m.Report()
	cat, _ := s.m.listedCatalog()
	v := StoreView{Server: rep.Server, API: rep.API, MinAPI: rep.MinAPI, Policy: PolicyWords, MinServer: MinModuleServer, Platform: thisPlatform(), CheckedAt: rep.CheckedAt, CheckError: rep.CheckError,
		Stores: []StoreInfo{}, Items: []StoreItem{}, RepairRoute: RepairRoute, Run: s.CurrentRun(), Updates: s.UpdateCount()}
	official := StoreInfo{ID: OfficialStore, Name: "WittyMediaServer modules", Host: hostOf(s.m.cfg.CatalogURL), URL: s.m.cfg.CatalogURL, Official: true, Enabled: true}
	if cat != nil {
		official.Name, official.Serial, official.Generated, official.Modules = cleanText(cat.Name), cat.Serial, cat.Generated, len(cat.Modules)
	}
	if rep.Catalog != nil && rep.Catalog.Error != "" {
		official.Problem = Explain("The module list", rep.Catalog.Error)
	}
	v.Stores = append(v.Stores, official)
	v.Stores = append(v.Stores, s.List()...)

	names := map[string]string{}
	statuses := map[string]*Status{}
	for i := range rep.Modules {
		st := &rep.Modules[i]
		statuses[st.ID] = st
		names[strings.ToLower(st.Name)] = st.ID
		v.Items = append(v.Items, s.officialItem(st, cat))
	}

	s.mu.Lock()
	entries := append([]*storeEntry(nil), s.list...)
	s.mu.Unlock()
	known := map[string]bool{}
	for _, st := range rep.Modules {
		if st.Delivery != wittymodule.DeliveryBuiltin && st.Delivery != wittymodule.DeliveryLibrary {
			known[st.ID] = true
		}
	}
	for _, e := range entries {
		if !e.Enabled || e.cat == nil {
			continue
		}
		for i := range e.cat.Modules {
			v.Items = append(v.Items, s.thirdPartyItem(e, &e.cat.Modules[i], known, names, statuses))
		}
	}
	return v
}

func (s *Stores) officialItem(st *Status, cat *wittymodule.Catalog) StoreItem {
	it := StoreItem{Key: OfficialStore + "/" + st.ID, Store: OfficialStore, Official: true, Kind: KindOfficial, ID: st.ID, Name: st.Name, Summary: st.Summary, Description: st.Description,
		Group: st.Group, Publisher: st.Publisher, Repo: st.StockRepo, Icon: st.Icon, Libraries: st.Libraries, Requires: st.Requires, Needs: st.Needs, Core: st.Core,
		Updates: st.Updates, Stays: st.Stays, Delivery: st.Delivery, License: st.License, Status: st, Fit: Fit{OK: true, Kind: FitOK}}
	it.StoreName, it.StoreHost = "WittyMediaServer modules", hostOf(s.m.cfg.CatalogURL)
	switch st.Delivery {
	case wittymodule.DeliveryBuiltin:
		it.Kind = "server"
		it.Publisher = nil
		it.Version = st.Version
		return it
	case wittymodule.DeliveryLibrary:
		it.Fit = Fit{Kind: FitLibrary, Why: st.Name + " is a library: it reaches this server inside the modules built on it, so there's nothing to install on its own."}
	}
	if cat != nil {
		if mod := cat.Find(st.ID); mod != nil {
			icon, pub, desc := it.Icon, it.Publisher, it.Description
			was := it.Fit
			s.fillListing(&it, mod, func(x string) string { return x })
			if icon != "" {
				it.Icon = icon // the manager keeps and checks this one
			}
			if pub != nil {
				it.Publisher = pub
			}
			if it.Description == "" {
				it.Description = desc
			}
			if was.Kind == FitLibrary {
				it.Fit = was
			}
		} else if it.Fit.OK {
			it.Fit = Fit{Kind: FitNone, Why: "The list of modules doesn't include " + st.Name + " any more."}
		}
	} else if it.Fit.OK && !st.Installed {
		it.Fit = Fit{Kind: FitNone, Why: "This server hasn't read the list of modules yet. Check for updates, or check its internet connection."}
	}
	it.Failure = failureFor(st)
	return it
}

func (s *Stores) thirdPartyItem(e *storeEntry, mod *wittymodule.CatalogModule, known map[string]bool, names map[string]string, statuses map[string]*Status) StoreItem {
	it := StoreItem{Key: e.ID + "/" + mod.ID, Store: e.ID, StoreName: cleanText(e.cat.Name), StoreHost: hostOf(e.URL), ID: mod.ID}
	s.fillListing(&it, mod, cleanText)
	it.Kind, it.Why = s.kindOf(mod.ID, known)
	if it.Kind != KindFork {
		it.Fit = Fit{Kind: it.Kind, Why: it.Why}
	}
	if owner, ok := names[strings.ToLower(it.Name)]; ok && owner != mod.ID {
		it.Lookalike = owner
	}
	if st := statuses[mod.ID]; st != nil && st.Source != nil && st.Source.Store == e.ID {
		it.Status = st
		it.Failure = failureFor(st)
	}
	return it
}

// PermissionWords says what a permission means, for a person.
func PermissionWords(p string) (name, words string) {
	if p == wittymodule.PermissionNetwork {
		return "The network", "It can connect to other computers, on your network or the internet."
	}
	return cleanText(p), "This server doesn't know what this means, so it won't run a version that asks for it."
}

// ItemDetail is a module's page.
type ItemDetail struct {
	Item        StoreItem       `json:"item"`
	Versions    []VersionInfo   `json:"versions"`
	Plan        []RunItem       `json:"plan"`
	Permissions []PermissionRow `json:"permissions"`
	ForkWarning string          `json:"forkWarning,omitempty"`
	Fingerprint string          `json:"fingerprint,omitempty"`
	Policy      string          `json:"policy"`
	Server      string          `json:"server"`
}

// PermissionRow is one thing a module asks for.
type PermissionRow struct {
	Name  string `json:"name"`
	Words string `json:"words"`
}

// signingKeyOf is the key a store's module must be signed with: its publisher's, when the store
// names one, or the store's own. Either way the admin agreed to the store that said so.
func signingKeyOf(e *storeEntry, mod *wittymodule.CatalogModule) string {
	if mod.Publisher.Key != "" {
		return mod.Publisher.Key
	}
	return e.Key
}

// Detail builds a module's page.
func (s *Stores) Detail(store, id string) (*ItemDetail, error) {
	v := s.View()
	var it *StoreItem
	for i := range v.Items {
		if v.Items[i].Key == store+"/"+id {
			it = &v.Items[i]
			break
		}
	}
	if it == nil {
		return nil, ErrUnknown
	}
	d := &ItemDetail{Item: *it, Versions: []VersionInfo{}, Plan: []RunItem{}, Permissions: []PermissionRow{}, Policy: PolicyWords, Server: v.Server}
	for _, p := range it.Permissions {
		name, words := PermissionWords(p)
		d.Permissions = append(d.Permissions, PermissionRow{name, words})
	}
	if it.Official || (it.Status != nil && it.Status.Source != nil && it.Status.Source.Store == store) {
		if l, err := s.m.Versions(id); err == nil {
			d.Versions = l.Versions
		}
		if !it.Official {
			d.Fingerprint = it.Status.Source.Fingerprint
		}
	} else {
		s.mu.Lock()
		e := s.find(store)
		s.mu.Unlock()
		if e == nil || e.cat == nil {
			return nil, ErrUnknownStore
		}
		mod := e.cat.Find(id)
		if mod == nil {
			return nil, ErrUnknown
		}
		d.Fingerprint = fingerprintOf(signingKeyOf(e, mod))
		for i := range mod.Versions {
			lv := &mod.Versions[i]
			f := fitOf(cleanText(mod.Name), lv, v.Server)
			vi := VersionInfo{Offer: Offer{Version: lv.Version, MinServer: lv.MinServer, MaxServer: lv.MaxServer, Notes: cleanText(lv.Changelog), Prerelease: lv.Prerelease}, Allowed: false, Refused: f.Why}
			if pf := lv.Platform(runtime.GOOS, runtime.GOARCH); pf != nil {
				vi.Size = pf.Size
			}
			d.Versions = append(d.Versions, vi)
		}
		if it.Kind == KindFork {
			d.ForkWarning = ForkWarning
		}
	}
	if it.Official {
		if plan, err := s.InstallPlan(id); err == nil {
			d.Plan = plan
		}
	}
	return d, nil
}

// UseFork makes a store's version of a module the one this server uses: the choice a fork needs,
// with its warning, and then the newest version that fits, which also has its own signature and
// manifest to pass. Without confirm it answers *ErrNeedsConfirm and changes nothing.
func (s *Stores) UseFork(store, id, by string, confirm bool) error {
	s.mu.Lock()
	e := s.find(store)
	s.mu.Unlock()
	switch {
	case e == nil:
		return ErrUnknownStore
	case !e.Enabled:
		return errors.New("that store is turned off")
	case e.cat == nil:
		return errors.New("that store's list of modules hasn't been read")
	case e.Problem != nil && e.Problem.Kind == FailKey:
		return errors.New("the signing key has changed: nothing can be installed from this store")
	}
	mod := e.cat.Find(id)
	if mod == nil {
		return ErrUnknown
	}
	if why := noFork[id]; why != "" {
		return errors.New(why)
	}
	known := s.knownModules()
	if !known[id] {
		return errors.New("new kinds of module are still in development, so this server can't install one yet")
	}
	key := signingKeyOf(e, mod)
	k, err := parseSourceKey(key)
	if err != nil {
		return err
	}
	if !confirm {
		return &ErrNeedsConfirm{Warning: ForkWarning + " The key's fingerprint is " + Fingerprint(k) + "."}
	}
	v, fit := s.offer(mod)
	if v == nil || !fit.OK {
		return errors.New(fit.Why)
	}
	src := &Source{Repo: mod.Repo, Key: base64.StdEncoding.EncodeToString(k), Store: store, AddedAt: time.Now().UnixMilli(), AddedBy: by}
	if err := s.m.useStoreSource(id, src, "a version from "+hostOf(e.URL)+", key "+Fingerprint(k)); err != nil {
		return err
	}
	s.applyFork(store)
	return s.m.InstallVersion(id, v.Version, true)
}

// applyFork tells the manager what a store now offers for the modules that use it.
func (s *Stores) applyFork(store string) {
	s.mu.Lock()
	e := s.find(store)
	s.mu.Unlock()
	if e == nil || e.cat == nil || s.m == nil {
		return
	}
	s.m.applyStoreListing(store, e.cat, e.URL, newStoreFetcher(s.cfg.Transport, e.Private))
}

// The most a picture may be, as in the catalogue's own checks.
const (
	maxMediaIcon = 256 << 10
	maxMediaShot = 2 << 20
)

var mediaHashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Media serves a picture a catalogue lists: only one it lists, only of the kinds a browser can
// be trusted to read as pictures, and checked against its size and hash when it arrives. Kept
// once fetched, so a page never asks anyone else for it.
func (s *Stores) Media(ctx context.Context, store, sha string) (path, contentType string, err error) {
	if !mediaHashRe.MatchString(sha) {
		return "", "", ErrUnknown
	}
	var file *wittymodule.CatalogFile
	var base string
	var limit int64
	var small func(ctx context.Context, raw string, limit int64) ([]byte, error)
	find := func(c *wittymodule.Catalog) {
		for i := range c.Modules {
			mod := &c.Modules[i]
			if mod.Icon != nil && mod.Icon.SHA256 == sha {
				file, limit = mod.Icon, maxMediaIcon
				return
			}
			for j := range mod.Screenshots {
				if mod.Screenshots[j].SHA256 == sha {
					file, limit = &mod.Screenshots[j], maxMediaShot
					return
				}
			}
		}
	}
	folder := filepath.Join(s.cfg.Dir, storesDir, store)
	if store == OfficialStore {
		cat, from := s.m.listedCatalog()
		if cat == nil {
			return "", "", ErrUnknown
		}
		find(cat)
		base, small = from, s.m.small
	} else {
		s.mu.Lock()
		e := s.find(store)
		s.mu.Unlock()
		if e == nil || e.cat == nil || !e.Enabled {
			return "", "", ErrUnknown
		}
		find(e.cat)
		base, small = e.URL, s.fetcher(e).small
	}
	if file == nil {
		return "", "", ErrUnknown
	}
	path = filepath.Join(folder, "media", sha)
	b, err := os.ReadFile(path)
	if err != nil || sum(b) != sha {
		ref, rerr := wittymodule.ResolveCatalogURL(base, file.URL)
		if rerr != nil {
			return "", "", rerr
		}
		if file.Size > limit {
			return "", "", errors.New("the picture is larger than a catalogue may list")
		}
		b, err = small(ctx, ref, file.Size)
		if err != nil {
			return "", "", err
		}
		if int64(len(b)) != file.Size || sum(b) != sha {
			return "", "", errors.New("the picture doesn't match what the catalogue says")
		}
		if err := writeAtomic(path, b); err != nil {
			return "", "", err
		}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 8192 || cfg.Height > 8192 {
		_ = os.Remove(path)
		return "", "", errors.New("that isn't a picture this server will show")
	}
	switch format {
	case "png":
		contentType = "image/png"
	case "jpeg":
		contentType = "image/jpeg"
	default:
		_ = os.Remove(path)
		return "", "", errors.New("that isn't a picture this server will show")
	}
	return path, contentType, nil
}
