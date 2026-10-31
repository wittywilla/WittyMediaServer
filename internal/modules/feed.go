package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// A plugin's own GitHub releases are its feed. GitHub is trusted for nothing except
// delivering files: a release counts only once its signed list and manifest check
// out, and those are kept beside the installs, so the list of versions works
// offline and costs GitHub nothing when it is unchanged.

const (
	maxKnown     = 30               // versions remembered for each plugin
	maxNewChecks = 12               // new releases read in one check; the rest wait for the next
	feedFile     = "feed.json"      // <data>/modules/<id>/feed.json
	checkTimeout = 2 * time.Minute  // for one check of everything
	maxListBytes = int64(256 << 10) // the signed list of withdrawn versions
)

// knownRelease is a release whose signature has been checked.
type knownRelease struct {
	rel        release
	man        *wittymodule.Manifest
	notes, url string
	published  int64
	prerelease bool
}

func (k *knownRelease) offer() *Offer {
	return &Offer{Version: k.rel.version, MinServer: k.man.MinServer, MaxServer: k.man.MaxServer, Notes: k.notes, URL: k.url,
		Published: k.published, Prerelease: k.prerelease || wittymodule.Prerelease(k.rel.version), Size: k.rel.archive.Size}
}

// cachedRelease is how a release is kept on disk: everything needed to check it
// again and offer it, with no need to ask anyone.
type cachedRelease struct {
	Version    string       `json:"version"`
	Notes      string       `json:"notes,omitempty"`
	URL        string       `json:"url,omitempty"`
	Published  int64        `json:"published,omitempty"`
	Prerelease bool         `json:"prerelease,omitempty"`
	Archive    update.Asset `json:"archive"`
	Sums       update.Asset `json:"sums"`
	Sig        update.Asset `json:"sig"`
	Man        update.Asset `json:"man"`
	RawSums    []byte       `json:"rawSums"`
	RawSig     []byte       `json:"rawSig"`
	RawMan     []byte       `json:"rawMan"`
}

type cachedFeed struct {
	ETag     string          `json:"etag,omitempty"`
	Releases []cachedRelease `json:"releases"`
}

func (m *Manager) feedPath(id string) string { return filepath.Join(m.modDir(id), feedFile) }

// loadFeed reads what the last check learned, once. Everything is verified again:
// the folder may be a shared one, and a list of versions is worth as little as a
// release that nobody signed. Call with m.mu held.
func (m *Manager) loadFeed(id string, e *entry) {
	if e.feedLoaded || !updatable(e.builtin) {
		return
	}
	e.feedLoaded = true
	b, err := os.ReadFile(m.feedPath(id))
	if err != nil {
		return
	}
	var c cachedFeed
	if err := json.Unmarshal(b, &c); err != nil {
		m.log.Warn("modules: the list of versions is unreadable; the next check rebuilds it", "module", id, "err", err)
		return
	}
	for i := range c.Releases {
		r := &c.Releases[i]
		rel := release{version: r.Version, archive: r.Archive, sums: r.Sums, sig: r.Sig, man: r.Man}
		man, err := m.checkSigned(id, &rel, m.keysFor(e), e.repo(), r.RawSums, r.RawSig, r.RawMan)
		if err != nil {
			m.log.Warn("modules: ignoring a remembered release", "module", id, "version", r.Version, "err", err)
			continue
		}
		e.known[r.Version] = &knownRelease{rel: rel, man: man, notes: r.Notes, url: r.URL, published: r.Published, prerelease: r.Prerelease}
	}
	if len(e.known) > 0 {
		e.etag = c.ETag // without what it described, the tag would only hide the releases
	}
	m.computeOffers(e)
}

// saveFeed keeps what a check learned. raw holds the signed files of each release.
func (m *Manager) saveFeed(id, etag string, known map[string]*knownRelease, raw map[string]*verified) {
	c := cachedFeed{ETag: etag}
	for ver, k := range known {
		v := raw[ver]
		if v == nil {
			continue
		}
		c.Releases = append(c.Releases, cachedRelease{Version: ver, Notes: k.notes, URL: k.url, Published: k.published, Prerelease: k.prerelease,
			Archive: k.rel.archive, Sums: k.rel.sums, Sig: k.rel.sig, Man: k.rel.man, RawSums: v.sums, RawSig: v.sig, RawMan: v.manifest})
	}
	sort.Slice(c.Releases, func(i, j int) bool { return wittymodule.Compare(c.Releases[i].Version, c.Releases[j].Version) > 0 })
	b, err := json.Marshal(c)
	if err == nil {
		err = writeAtomic(m.feedPath(id), b)
	}
	if err != nil {
		m.log.Warn("modules: keeping the list of versions", "module", id, "err", err)
	}
}

// readCachedRaw brings back the signed files of remembered releases, so a check
// that learns one new version doesn't lose the rest.
func (m *Manager) readCachedRaw(id string) map[string]*verified {
	out := map[string]*verified{}
	b, err := os.ReadFile(m.feedPath(id))
	if err != nil {
		return out
	}
	var c cachedFeed
	if json.Unmarshal(b, &c) != nil {
		return out
	}
	for i := range c.Releases {
		r := &c.Releases[i]
		out[r.Version] = &verified{sums: r.RawSums, sig: r.RawSig, manifest: r.RawMan}
	}
	return out
}

// ---------------------------------------------------------------------------
// Checking
// ---------------------------------------------------------------------------

// Check looks for releases of every module that can have them, unless the last
// answer is younger than maxAge. Concurrent calls share one look. When the network
// is down, or GitHub asks this network to wait, the last answer stays, along with
// why this one failed; nothing that is running depends on it.
func (m *Manager) Check(ctx context.Context, maxAge time.Duration) Report {
	m.mu.Lock()
	if !m.checked.IsZero() && time.Since(m.checked) < maxAge {
		m.mu.Unlock()
		return m.Report()
	}
	if time.Now().Before(m.holdUntil) { // asking anyway would only make the wait longer
		m.mu.Unlock()
		return m.Report()
	}
	if m.checking != nil {
		ch := m.checking
		m.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return m.Report()
	}
	ch := make(chan struct{})
	m.checking = ch
	var ids []string
	for _, id := range m.order {
		// A module that was taken off this server has no reason to ask anyone anything.
		if updatable(m.mods[id].builtin) && !m.mods[id].state.Removed {
			m.loadFeed(id, m.mods[id])
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	var problems []string
	var hold time.Time
	for _, id := range ids {
		err := m.checkOne(cctx, id)
		if err == nil {
			continue
		}
		var limited *update.RateLimited
		if errors.As(err, &limited) { // the same answer waits for every other plugin
			hold = limited.Until
			problems = []string{limited.Error() + "."}
			m.log.Warn("modules: GitHub is limiting requests", "until", limited.Until)
			break
		}
		m.log.Warn("modules: update check failed", "module", id, "err", err)
		problems = append(problems, sentence(err.Error()))
	}

	m.mu.Lock()
	m.checked = time.Now()
	m.checkErr = strings.Join(problems, " ")
	m.holdUntil = hold
	m.checking = nil
	close(ch)
	m.mu.Unlock()
	return m.Report()
}

// checkOne brings one module's list of versions up to date.
func (m *Manager) checkOne(ctx context.Context, id string) error {
	m.mu.Lock()
	e := m.mods[id]
	repo, name, etag := e.repo(), e.builtin.Name, e.etag
	m.mu.Unlock()

	rels, newEtag, unchanged, err := m.fetch.ReleasesSince(ctx, repo, etag)
	if errors.Is(err, update.ErrNotFound) {
		rels, newEtag, err = nil, "", nil // its repository isn't published yet, so there is nothing newer
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if unchanged {
		m.mu.Lock()
		m.computeOffers(e)
		m.mu.Unlock()
		return nil
	}

	var withdrawals *update.GHRelease
	versions := rels[:0:0]
	for i := range rels {
		g := &rels[i]
		if g.TagName == wittymodule.WithdrawalsTag {
			withdrawals = g
			continue
		}
		if !g.Draft && wittymodule.Valid(strings.TrimPrefix(g.TagName, "v")) {
			versions = append(versions, *g)
		}
	}
	sort.SliceStable(versions, func(i, j int) bool {
		return wittymodule.Compare(strings.TrimPrefix(versions[i].TagName, "v"), strings.TrimPrefix(versions[j].TagName, "v")) > 0
	})
	if len(versions) > maxKnown {
		versions = versions[:maxKnown]
	}

	// The list of withdrawn versions first, so nothing it names gets offered. Its release
	// is an old one, so a list of the newest may not reach it: asked for by name then.
	var withdrawErr error
	if withdrawals == nil {
		withdrawals, withdrawErr = m.fetch.ReleaseByTag(ctx, repo, wittymodule.WithdrawalsTag)
		if errors.Is(withdrawErr, update.ErrNotFound) {
			withdrawals, withdrawErr = nil, nil
		}
	}
	if withdrawErr == nil {
		withdrawErr = m.readWithdrawals(ctx, id, e, withdrawals)
	}

	m.mu.Lock()
	had := e.known
	m.mu.Unlock()
	raw := m.readCachedRaw(id)
	known := map[string]*knownRelease{}
	var bad error
	read := 0
	for _, g := range versions {
		ver := strings.TrimPrefix(g.TagName, "v")
		r := m.pickFor(e, id, ver, g)
		if r == nil {
			continue // not signed, or nothing for this system: not something to offer
		}
		// Signed files never change, so a release that still has the same files needs
		// no second look. A release whose files changed gets one.
		old := had[ver]
		var man *wittymodule.Manifest
		if old != nil && old.rel == *r && raw[ver] != nil {
			man = old.man
		} else {
			// A release that failed isn't fetched again, and running out of checks only
			// stops new ones: what is already known stays, so fake tags can't hide it.
			if e.badFiles[r.sums.URL] {
				continue
			}
			if read++; read > maxNewChecks {
				newEtag = "" // come back for the rest
				continue
			}
			var v *verified
			var verr error
			man, v, verr = m.verifiedRelease(ctx, id, e, r)
			if verr != nil {
				m.log.Warn("modules: ignoring a release", "module", id, "version", ver, "err", verr)
				if bad == nil {
					bad = fmt.Errorf("%s %s was ignored: %w", name, ver, verr)
				}
				var limited *update.RateLimited
				if !errors.As(verr, &limited) && ctx.Err() == nil {
					m.mu.Lock()
					if e.badFiles == nil {
						e.badFiles = map[string]bool{}
					}
					e.badFiles[r.sums.URL] = true
					m.mu.Unlock()
				}
				continue
			}
			raw[ver] = v
		}
		k := &knownRelease{rel: *r, man: man, url: pageURL(g.HTMLURL), prerelease: g.Prerelease, notes: trimNotes(g.Body)}
		if !g.PublishedAt.IsZero() {
			k.published = g.PublishedAt.UnixMilli()
		}
		known[ver] = k
	}
	m.saveFeed(id, newEtag, known, raw)

	m.mu.Lock()
	e.known, e.etag = known, newEtag
	m.computeOffers(e)
	found := e.latest != nil || e.tooNew != nil
	m.mu.Unlock()
	if withdrawErr != nil {
		return fmt.Errorf("%s: %w", name, withdrawErr)
	}
	if !found {
		return bad
	}
	return nil
}

func trimNotes(s string) string {
	if len(s) > maxNotes {
		return s[:maxNotes] + "…"
	}
	return s
}

// pickFor finds a release's files for this system, or nil if one is missing.
func (m *Manager) pickFor(e *entry, id, ver string, g update.GHRelease) *release {
	if e.builtin.Delivery == wittymodule.DeliveryWeb {
		return pickAssets(wittymodule.WebArchiveName(id, ver), id, ver, g)
	}
	return pickAssets(wittymodule.ArchiveName(id, ver, runtime.GOOS, runtime.GOARCH), id, ver, g)
}

// computeOffers works out what to offer from what is known: the newest version that
// fits and hasn't been withdrawn, and the newest that doesn't fit, which is reported
// so the reason is visible. Call with m.mu held.
func (m *Manager) computeOffers(e *entry) {
	current := e.man.Version
	if wittymodule.Compare(e.builtin.Version, current) > 0 && !e.state.Chosen {
		current = e.builtin.Version
	}
	// A pre-release is only for a server that is one itself, or that already runs
	// that module's pre-release.
	pre := wittymodule.Prerelease(m.cfg.Server) || wittymodule.Prerelease(current)
	vers := make([]string, 0, len(e.known))
	for v := range e.known {
		vers = append(vers, v)
	}
	sort.Slice(vers, func(i, j int) bool { return wittymodule.Compare(vers[i], vers[j]) > 0 })
	var latest, tooNew *Offer
	var rel *release
	for _, ver := range vers {
		k := e.known[ver]
		if wittymodule.Compare(ver, current) <= 0 {
			break
		}
		if (k.prerelease || wittymodule.Prerelease(ver)) && !pre {
			continue
		}
		if e.state.Withdrawn.Find(ver) != nil {
			continue
		}
		o := k.offer()
		if err := k.man.Supports(m.cfg.Server, wittymodule.MinAPI, wittymodule.API); err != nil {
			if tooNew == nil {
				o.Reason = sentence(err.Error())
				tooNew = o
			}
			continue
		}
		if r := protocolRefusal(e, k.man); r != "" {
			if tooNew == nil {
				o.Reason = r
				tooNew = o
			}
			continue
		}
		if r := dataRefusal(e.builtin.Name, k.man, e.state.DataHigh); r != "" {
			continue
		}
		o.Blocked = e.state.Blocked[ver]
		latest, rel = o, &k.rel
		break
	}
	e.latest, e.tooNew, e.rel = latest, tooNew, rel
}

// readWithdrawals fetches the plugin's signed list, if it has one, and takes it only
// if it is newer than the one already kept. A list that is missing or stale changes
// nothing: withdrawing a version sticks even if someone later hides the list.
func (m *Manager) readWithdrawals(ctx context.Context, id string, e *entry, g *update.GHRelease) error {
	if g == nil {
		return nil
	}
	name := wittymodule.WithdrawalsName(id)
	var listURL, sigURL string
	for _, a := range g.Assets {
		switch a.Name {
		case name:
			listURL = a.URL
		case name + ".sig":
			sigURL = a.URL
		}
	}
	if listURL == "" || sigURL == "" {
		return nil
	}
	list, err := m.fetch.Small(ctx, listURL, maxListBytes)
	if err != nil {
		return fmt.Errorf("the list of withdrawn versions: %w", err)
	}
	sig, err := m.fetch.Small(ctx, sigURL, maxSig)
	if err != nil {
		return fmt.Errorf("the list of withdrawn versions: %w", err)
	}
	m.mu.Lock()
	keys := m.keysFor(e)
	m.mu.Unlock()
	if err := update.VerifySums(list, sig, keys); err != nil {
		return fmt.Errorf("the list of withdrawn versions was ignored: %w", err)
	}
	w, err := wittymodule.ParseWithdrawals(list)
	if err != nil {
		return err
	}
	if w.Module != id {
		return fmt.Errorf("the list of withdrawn versions is for %s", w.Module)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.state.Withdrawn != nil && w.Serial <= e.state.Withdrawn.Serial {
		return nil
	}
	inUse := e.dir != ""
	ver := e.man.Version
	e.state.Withdrawn = w
	if v := w.Find(ver); v != nil && inUse {
		m.log.Warn("modules: the version in use was withdrawn", "module", id, "version", ver, "severity", v.Severity, "reason", v.Reason)
		e.note("withdrawn", ver, v.Severity+": "+v.Reason)
		if v.Severity == wittymodule.SeveritySecurity {
			m.resolve(id, e)
		}
	}
	if err := m.saveState(); err != nil {
		m.log.Warn("modules: saving the state", "err", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Choosing a version
// ---------------------------------------------------------------------------

// VersionInfo is one version, explained: what it is, and whether it can be chosen.
type VersionInfo struct {
	Offer
	Builtin   bool                   `json:"builtin,omitempty"`   // the copy compiled into this server
	InUse     bool                   `json:"inUse,omitempty"`     // the version running now
	Installed bool                   `json:"installed,omitempty"` // on disk, so switching needs no download
	Older     bool                   `json:"older,omitempty"`     // older than the version in use
	Allowed   bool                   `json:"allowed"`             // it can be chosen
	Refused   string                 `json:"refused,omitempty"`   // why it can't, in words for a person
	Warning   string                 `json:"warning,omitempty"`   // what to know first; choosing it needs confirming
	Withdrawn *wittymodule.Withdrawn `json:"withdrawn,omitempty"`
}

// VersionList is everything known about a module's versions.
type VersionList struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Repo      string        `json:"repo"`
	InUse     string        `json:"inUse"`
	Pin       string        `json:"pin,omitempty"`
	CheckedAt int64         `json:"checkedAt,omitempty"`
	Versions  []VersionInfo `json:"versions"`
}

// judge decides whether a version may be chosen. It says everything it can before
// anything is downloaded. Call with m.mu held.
func (m *Manager) judge(e *entry, man *wittymodule.Manifest) (refused, warning string) {
	ver := man.Version
	if err := man.Supports(m.cfg.Server, wittymodule.MinAPI, wittymodule.API); err != nil {
		return sentence(err.Error()), ""
	}
	if man.Delivery != e.builtin.Delivery {
		return fmt.Sprintf("It is delivered as %q, and this server expects %q.", man.Delivery, e.builtin.Delivery), ""
	}
	if r := protocolRefusal(e, man); r != "" {
		return r, ""
	}
	w := e.state.Withdrawn.Find(ver)
	if w != nil && w.Severity == wittymodule.SeveritySecurity {
		return "It was withdrawn for security reasons: " + sentence(w.Reason), ""
	}
	if r := dataRefusal(e.builtin.Name, man, e.state.DataHigh); r != "" {
		return r, ""
	}
	var warn []string
	if w != nil {
		warn = append(warn, "It was withdrawn: "+sentence(w.Reason))
	}
	if b := e.state.Blocked[ver]; b != "" {
		warn = append(warn, "It was taken out of use before: "+sentence(b))
	}
	if wittymodule.Compare(ver, e.man.Version) < 0 {
		warn = append(warn, "It is older than the version in use, so it may lack fixes, including security fixes. It will be pinned, so automatic updates leave it alone until you go back to the latest.")
	}
	if wittymodule.Prerelease(ver) && !wittymodule.Prerelease(m.cfg.Server) {
		warn = append(warn, "It is a pre-release, and this server is not.")
	}
	return "", strings.Join(warn, " ")
}

// Versions lists what is known, without going online: the last check's answer, or an
// empty list and the built-in copy until there has been one. Incompatible versions
// are there too, explained, rather than hidden.
func (m *Manager) Versions(id string) (VersionList, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return VersionList{}, ErrUnknown
	}
	m.loadFeed(id, e)
	l := VersionList{ID: id, Name: e.builtin.Name, Repo: e.repo(), InUse: e.man.Version, Pin: e.state.Pin, Versions: []VersionInfo{}}
	if !m.checked.IsZero() {
		l.CheckedAt = m.checked.UnixMilli()
	}
	onDisk := map[string]bool{}
	if ds, err := os.ReadDir(filepath.Join(m.modDir(id), "versions")); err == nil {
		for _, d := range ds {
			if d.IsDir() {
				onDisk[d.Name()] = true
			}
		}
	}
	seen := map[string]bool{}
	for ver, k := range e.known {
		seen[ver] = true
		v := VersionInfo{Offer: *k.offer(), Installed: onDisk[ver], InUse: e.dir != "" && e.man.Version == ver, Older: wittymodule.Compare(ver, e.man.Version) < 0}
		v.Blocked = e.state.Blocked[ver]
		v.Withdrawn = e.state.Withdrawn.Find(ver)
		if !v.InUse {
			v.Refused, v.Warning = m.judge(e, k.man)
			v.Allowed = v.Refused == ""
		}
		// The same number from a fork is someone else's code, not the built-in copy.
		if ver == e.builtin.Version && e.state.Source == nil {
			v.Builtin = true
		}
		l.Versions = append(l.Versions, v)
	}
	// What is installed may have left the list of releases; the signature was checked
	// when it was installed, and again when it starts.
	for ver := range onDisk {
		if seen[ver] || !wittymodule.Valid(ver) {
			continue
		}
		if man, _, err := m.usable(id, e, ver); err == nil {
			seen[ver] = true
			v := VersionInfo{Offer: Offer{Version: ver, MinServer: man.MinServer, MaxServer: man.MaxServer, Prerelease: wittymodule.Prerelease(ver), Blocked: e.state.Blocked[ver]},
				Installed: true, InUse: e.dir != "" && e.man.Version == ver, Older: wittymodule.Compare(ver, e.man.Version) < 0, Withdrawn: e.state.Withdrawn.Find(ver)}
			if !v.InUse {
				v.Refused, v.Warning = m.judge(e, man)
				v.Allowed = v.Refused == ""
			}
			l.Versions = append(l.Versions, v)
		}
	}
	// The built-in copy can always be chosen.
	if !seen[e.builtin.Version] || e.state.Source != nil {
		l.Versions = append(l.Versions, VersionInfo{Offer: Offer{Version: e.builtin.Version, MinServer: e.builtin.MinServer, MaxServer: e.builtin.MaxServer},
			Builtin: true, InUse: e.dir == "", Allowed: e.dir != "", Older: wittymodule.Compare(e.builtin.Version, e.man.Version) < 0})
	} else {
		for i := range l.Versions {
			if v := &l.Versions[i]; v.Builtin {
				v.InUse, v.Allowed, v.Refused, v.Warning = v.InUse || e.dir == "", e.dir != "", "", ""
			}
		}
	}
	sort.Slice(l.Versions, func(i, j int) bool { return wittymodule.Compare(l.Versions[i].Version, l.Versions[j].Version) > 0 })
	return l, nil
}
