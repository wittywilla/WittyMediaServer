package modules

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// Limits on a release. The checksum list has a few lines per file per platform,
// and the largest helper planned is well under the archive limit.
const (
	maxSums      = 256 << 10
	maxSig       = 4 << 10
	maxManifest  = 64 << 10
	maxArchive   = 256 << 20
	maxUnpacked  = 2 * maxArchive
	maxNotes     = 64 << 10
	maxCandidate = 5 // how many newer releases are looked at
)

// In a version's folder, next to its files, so they can be checked again.
const (
	signedList = "SHA256SUMS.txt"
	signedSig  = "SHA256SUMS.txt.sig"
)

// release is a release of a module whose list has been verified.
type release struct {
	version string
	archive update.Asset
	sums    update.Asset
	sig     update.Asset
	man     update.Asset
}

// ---------------------------------------------------------------------------
// Checking
// ---------------------------------------------------------------------------

// Check looks for newer releases of every module that can have them, unless the
// last answer is younger than maxAge. Concurrent calls share one look. When the
// network is down the last answer stays, along with why this one failed; nothing
// that is running depends on it.
func (m *Manager) Check(ctx context.Context, maxAge time.Duration) Report {
	m.mu.Lock()
	if !m.checked.IsZero() && time.Since(m.checked) < maxAge {
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
		if updatable(m.mods[id].builtin) {
			ids = append(ids, id)
		}
	}
	m.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var problems []string
	for _, id := range ids {
		if err := m.checkOne(cctx, id); err != nil {
			m.log.Warn("modules: update check failed", "module", id, "err", err)
			problems = append(problems, err.Error())
		}
	}

	m.mu.Lock()
	m.checked = time.Now()
	m.checkErr = strings.Join(problems, " ")
	m.checking = nil
	close(ch)
	m.mu.Unlock()
	return m.Report()
}

func (m *Manager) checkOne(ctx context.Context, id string) error {
	m.mu.Lock()
	e := m.mods[id]
	repo, current, name := e.builtin.Repo, e.man.Version, e.builtin.Name
	if wittymodule.Compare(e.builtin.Version, current) > 0 {
		current = e.builtin.Version
	}
	blocked := e.state.Blocked
	m.mu.Unlock()

	rels, err := m.fetch.Releases(ctx, repo)
	if errors.Is(err, update.ErrNotFound) {
		rels, err = nil, nil // its repository isn't published yet, so there is nothing newer
	}
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	sort.SliceStable(rels, func(i, j int) bool {
		return wittymodule.Compare(strings.TrimPrefix(rels[i].TagName, "v"), strings.TrimPrefix(rels[j].TagName, "v")) > 0
	})

	// A pre-release is only for a server that is one itself, or that already
	// runs that module's pre-release.
	pre := wittymodule.Prerelease(m.cfg.Server) || wittymodule.Prerelease(current)
	var latest, tooNew *Offer
	var rel *release
	var bad error
	looked := 0
	for _, g := range rels {
		ver := strings.TrimPrefix(g.TagName, "v")
		if g.Draft || !wittymodule.Valid(ver) || wittymodule.Compare(ver, current) <= 0 {
			continue
		}
		if (g.Prerelease || wittymodule.Prerelease(ver)) && !pre {
			continue
		}
		if looked++; looked > maxCandidate {
			break
		}
		r := pickAssets(id, ver, g, runtime.GOOS, runtime.GOARCH)
		if r == nil {
			continue // not signed, or nothing for this system: not something to offer
		}
		man, _, err := m.verifiedRelease(ctx, r)
		if err != nil {
			m.log.Warn("modules: ignoring a release", "module", id, "version", ver, "err", err)
			if bad == nil {
				bad = fmt.Errorf("%s %s was ignored: %w", name, ver, err)
			}
			continue
		}
		o := &Offer{Version: ver, MinServer: man.MinServer, MaxServer: man.MaxServer, URL: pageURL(g.HTMLURL), Prerelease: wittymodule.Prerelease(ver), Size: r.archive.Size, Notes: g.Body}
		if len(o.Notes) > maxNotes {
			o.Notes = o.Notes[:maxNotes] + "…"
		}
		if !g.PublishedAt.IsZero() {
			o.Published = g.PublishedAt.UnixMilli()
		}
		if err := man.Supports(m.cfg.Server, wittymodule.MinAPI, wittymodule.API); err != nil {
			if tooNew == nil {
				o.Reason = sentence(err.Error())
				tooNew = o
			}
			continue
		}
		o.Blocked = blocked[ver]
		latest, rel = o, r
		break
	}
	m.mu.Lock()
	e.latest, e.tooNew, e.rel = latest, tooNew, rel
	m.mu.Unlock()
	if latest == nil && tooNew == nil {
		return bad
	}
	return nil
}

func pageURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "github.com") {
		return ""
	}
	return raw
}

// pickAssets finds a release's files for a system, or nil if one is missing.
func pickAssets(id, ver string, g update.GHRelease, goos, goarch string) *release {
	r := &release{version: ver}
	for _, a := range g.Assets {
		as := update.Asset{Name: a.Name, URL: a.URL, Size: a.Size}
		switch a.Name {
		case wittymodule.ArchiveName(id, ver, goos, goarch):
			r.archive = as
		case wittymodule.SumsName(id, ver):
			r.sums = as
		case wittymodule.SumsName(id, ver) + ".sig":
			r.sig = as
		case wittymodule.ManifestName(id, ver):
			r.man = as
		}
	}
	if r.archive.URL == "" || r.sums.URL == "" || r.sig.URL == "" || r.man.URL == "" {
		return nil
	}
	return r
}

type verified struct {
	sums, sig []byte
	list      map[string]string
	manifest  []byte
}

// verifiedRelease fetches a release's signed list and manifest, and checks them.
// Nothing about a release is believed before this.
func (m *Manager) verifiedRelease(ctx context.Context, r *release) (*wittymodule.Manifest, *verified, error) {
	sums, err := m.fetch.Small(ctx, r.sums.URL, maxSums)
	if err != nil {
		return nil, nil, fmt.Errorf("checksums: %w", err)
	}
	sig, err := m.fetch.Small(ctx, r.sig.URL, maxSig)
	if err != nil {
		return nil, nil, fmt.Errorf("signature: %w", err)
	}
	if err := update.VerifySums(sums, sig, m.keys); err != nil {
		return nil, nil, err
	}
	list, err := update.ParseSums(sums)
	if err != nil {
		return nil, nil, err
	}
	raw, err := m.fetch.Small(ctx, r.man.URL, maxManifest)
	if err != nil {
		return nil, nil, fmt.Errorf("manifest: %w", err)
	}
	if want, ok := list[r.man.Name]; !ok || sum(raw) != want {
		return nil, nil, errors.New("its manifest doesn't match its signed checksum")
	}
	man, err := wittymodule.ParseManifest(raw)
	if err != nil {
		return nil, nil, err
	}
	// The signature proves who made the manifest, and this proves it is for this
	// version, so an old release can't be passed off under a new tag.
	if man.Version != r.version || wittymodule.SumsName(man.ID, man.Version) != r.sums.Name {
		return nil, nil, fmt.Errorf("its manifest says %s %s", man.ID, man.Version)
	}
	return man, &verified{sums: sums, sig: sig, list: list, manifest: raw}, nil
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// Installing
// ---------------------------------------------------------------------------

// Install starts installing the release the last check found, in the background.
// Progress is in the Report. An admin may choose a version that was taken out of
// use before; nothing automatic does.
func (m *Manager) Install(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	switch {
	case e == nil:
		return ErrUnknown
	case !updatable(e.builtin):
		return errors.New(e.builtin.Name + " is built into the server and updates with it")
	case e.busy:
		return errors.New("an update of this module is already being installed")
	case m.closed:
		return errors.New("the server is stopping")
	case e.rel == nil:
		return errors.New("no compatible newer version is known; check for updates first")
	}
	e.busy, e.phase, e.lastError, e.done, e.total = true, PhaseDownloading, "", 0, e.rel.archive.Size
	rel := *e.rel
	session := logging.NewID("mod")
	log := m.log.With("session", session, "module", id, "version", rel.version)
	log.Info("modules: installing")
	logging.Go("module install", func() {
		ctx, cancel := context.WithTimeout(logging.WithSession(context.Background(), session), 30*time.Minute)
		defer cancel()
		err := m.install(ctx, id, e, &rel)
		m.mu.Lock()
		defer m.mu.Unlock()
		e.busy = false
		if err != nil {
			e.phase, e.lastError = PhaseFailed, sentence(err.Error())
			log.Error("modules: install failed; nothing changed", "err", err)
			m.prune(id, e)
			return
		}
		e.phase = PhaseIdle
		log.Info("modules: installed")
	}, "cat", "modules", "session", session)
	return nil
}

func (m *Manager) setPhase(e *entry, p string) {
	m.mu.Lock()
	e.phase = p
	m.mu.Unlock()
}

// install does the work. Until the state is saved, the version in use is
// untouched, so a failure at any step, or the power going, changes nothing.
func (m *Manager) install(ctx context.Context, id string, e *entry, r *release) error {
	// 1. The signed list and the manifest, fetched again rather than remembered.
	m.setPhase(e, PhaseVerifying)
	man, v, err := m.verifiedRelease(ctx, r)
	if err != nil {
		return err
	}
	if man.ID != id {
		return fmt.Errorf("the release says it is %s", man.ID)
	}
	if err := man.Supports(m.cfg.Server, wittymodule.MinAPI, wittymodule.API); err != nil {
		return err
	}
	want, ok := v.list[r.archive.Name]
	if !ok {
		return fmt.Errorf("the signed checksum list has no entry for %s", r.archive.Name)
	}

	base := m.modDir(id)
	if err := os.MkdirAll(filepath.Join(base, "versions"), 0o755); err != nil {
		return err
	}
	work, err := os.MkdirTemp(base, ".work-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	// 2. The archive, hashed as it arrives.
	m.setPhase(e, PhaseDownloading)
	archive := filepath.Join(work, r.archive.Name)
	if err := m.download(ctx, e, r.archive, archive, want); err != nil {
		return err
	}

	// 3. Unpack.
	m.setPhase(e, PhaseInstalling)
	stage := filepath.Join(work, "new")
	if err := update.Extract(archive, stage, maxUnpacked); err != nil {
		return fmt.Errorf("unpacking: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, signedList), v.sums, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, signedSig), v.sig, 0o644); err != nil {
		return err
	}
	// 4. Every file is the one that was signed, and there are no others.
	if err := verifyInstalled(stage, id, r.version, m.keys); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(stage, wittymodule.ManifestFile))
	if err != nil {
		return errors.New("the package has no manifest")
	}
	if string(b) != string(v.manifest) {
		return errors.New("the manifest in the package differs from the release's")
	}
	if _, err := os.Stat(filepath.Join(stage, man.EntryFile(runtime.GOOS))); err != nil {
		return fmt.Errorf("the package has no %s", man.EntryFile(runtime.GOOS))
	}

	// 5. It runs here, and it is what it says.
	if err := newProc(man, stage, nil, m.log).Try(); err != nil {
		return fmt.Errorf("this version didn't start on this server, which can happen if the data folder doesn't allow programs to run: %w", err)
	}

	// 6. Into place, and only then remembered.
	dst, err := m.versionDir(id, r.version)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := renameRetry(stage, dst); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	old := e.state
	blocked := map[string]string{}
	for k, val := range old.Blocked {
		if k != r.version {
			blocked[k] = val
		}
	}
	e.state.Previous = old.Active
	if old.Active != "" && blocked[old.Active] != "" {
		e.state.Previous = old.Previous
	}
	e.state.Active, e.state.Blocked = r.version, blocked
	if err := m.saveState(); err != nil {
		e.state = old
		_ = os.RemoveAll(dst)
		return fmt.Errorf("the state could not be saved: %w", err)
	}
	// 7. The next call uses the new version.
	m.resolve(id, e)
	e.latest, e.rel = nil, nil
	m.prune(id, e)
	return nil
}

func (m *Manager) download(ctx context.Context, e *entry, a update.Asset, dst, want string) error {
	res, err := m.fetch.Get(ctx, a.URL, "application/octet-stream")
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer res.Body.Close()
	limit := int64(maxArchive)
	if a.Size > 0 && a.Size < limit {
		limit = a.Size
	}
	if res.ContentLength > limit {
		return errors.New("download: the file is larger than the release says")
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h, progress{m, e}), io.LimitReader(res.Body, limit+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if n > limit {
		return errors.New("download: the file is larger than the release says")
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return fmt.Errorf("the download of %s does not match its signed checksum; it was not installed", a.Name)
	}
	return nil
}

type progress struct {
	m *Manager
	e *entry
}

func (p progress) Write(b []byte) (int, error) {
	p.m.mu.Lock()
	p.e.done += int64(len(b))
	p.m.mu.Unlock()
	return len(b), nil
}

// verifyInstalled checks a version's folder against the signed list kept in it:
// the signature is good, every file the list names for this system is there and
// unchanged, and nothing else is there. It runs again before every start of a
// helper, so a file changed on a shared data folder is refused rather than run.
func verifyInstalled(dir, id, ver string, keys []ed25519.PublicKey) error {
	sums, err := os.ReadFile(filepath.Join(dir, signedList))
	if err != nil {
		return fmt.Errorf("%w: its signed list is missing", errUntrusted)
	}
	sig, err := os.ReadFile(filepath.Join(dir, signedSig))
	if err != nil {
		return fmt.Errorf("%w: its signature is missing", errUntrusted)
	}
	if err := update.VerifySums(sums, sig, keys); err != nil {
		return fmt.Errorf("%w: %w", errUntrusted, err)
	}
	list, err := update.ParseSums(sums)
	if err != nil {
		return fmt.Errorf("%w: %w", errUntrusted, err)
	}
	prefix := wittymodule.TopFolder(id, ver, runtime.GOOS, runtime.GOARCH) + "/"
	want := map[string]string{}
	for name, h := range list {
		if rel, ok := strings.CutPrefix(name, prefix); ok {
			want[rel] = h
		}
	}
	if len(want) == 0 {
		return fmt.Errorf("%w: its signed list has nothing for this system", errUntrusted)
	}
	seen := 0
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() || rel == signedList || rel == signedSig {
			return nil
		}
		h, ok := want[rel]
		if !ok || !d.Type().IsRegular() {
			return fmt.Errorf("%w: %s doesn't belong there", errUntrusted, rel)
		}
		got, err := update.FileSHA256(p)
		if err != nil {
			return err
		}
		if got != h {
			return fmt.Errorf("%w: %s is different", errUntrusted, rel)
		}
		seen++
		return nil
	})
	if err != nil {
		return err
	}
	if seen != len(want) {
		return fmt.Errorf("%w: a file is missing", errUntrusted)
	}
	return nil
}

// renameRetry: on Windows a virus scanner may still hold a new program for a moment.
func renameRetry(from, to string) error {
	var err error
	for i := 0; i < 10; i++ {
		if err = os.Rename(from, to); err == nil || runtime.GOOS != "windows" {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
	return err
}
