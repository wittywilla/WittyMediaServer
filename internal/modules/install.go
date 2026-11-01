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
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

// Limits on a release. The checksum list has a few lines per file per platform,
// and the largest helper planned is well under the archive limit.
const (
	maxSums     = 256 << 10
	maxSig      = 4 << 10
	maxManifest = 64 << 10
	maxArchive  = 256 << 20
	maxUnpacked = 2 * maxArchive
	maxNotes    = 16 << 10
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

func pageURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "github.com") {
		return ""
	}
	return raw
}

// pickAssets finds a release's files, or nil if one is missing.
func pickAssets(archive, id, ver string, g update.GHRelease) *release {
	r := &release{version: ver}
	for _, a := range g.Assets {
		as := update.Asset{Name: a.Name, URL: a.URL, Size: a.Size, Updated: a.Updated}
		switch a.Name {
		case archive:
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
func (m *Manager) verifiedRelease(ctx context.Context, id string, e *entry, r *release) (*wittymodule.Manifest, *verified, error) {
	sums, err := m.fetch.Small(ctx, r.sums.URL, maxSums)
	if err != nil {
		return nil, nil, fmt.Errorf("checksums: %w", err)
	}
	sig, err := m.fetch.Small(ctx, r.sig.URL, maxSig)
	if err != nil {
		return nil, nil, fmt.Errorf("signature: %w", err)
	}
	m.mu.Lock()
	keys, repo := m.keysFor(e), e.repo()
	m.mu.Unlock()
	if err := update.VerifySums(sums, sig, keys); err != nil {
		return nil, nil, err
	}
	raw, err := m.fetch.Small(ctx, r.man.URL, maxManifest)
	if err != nil {
		return nil, nil, fmt.Errorf("manifest: %w", err)
	}
	man, err := m.checkSigned(id, r, keys, repo, sums, sig, raw)
	if err != nil {
		return nil, nil, err
	}
	list, err := update.ParseSums(sums)
	if err != nil {
		return nil, nil, err
	}
	return man, &verified{sums: sums, sig: sig, list: list, manifest: raw}, nil
}

// checkSigned checks a release's three small files against each other and against
// this module. It needs no network, so it also checks what was remembered.
func (m *Manager) checkSigned(id string, r *release, keys []ed25519.PublicKey, repo string, sums, sig, raw []byte) (*wittymodule.Manifest, error) {
	if err := update.VerifySums(sums, sig, keys); err != nil {
		return nil, err
	}
	list, err := update.ParseSums(sums)
	if err != nil {
		return nil, err
	}
	if want, ok := list[r.man.Name]; !ok || sum(raw) != want {
		return nil, errors.New("its manifest doesn't match its signed checksum")
	}
	man, err := wittymodule.ParseManifest(raw)
	if err != nil {
		return nil, err
	}
	// The signature proves who made the manifest, and this proves it is for this
	// version of this plugin, so a release of one can't be passed off as another, or
	// an old one under a new tag.
	if man.ID != id || man.Version != r.version || wittymodule.SumsName(man.ID, man.Version) != r.sums.Name {
		return nil, fmt.Errorf("its manifest says %s %s", man.ID, man.Version)
	}
	if !strings.EqualFold(man.Repo, repo) {
		return nil, fmt.Errorf("its manifest says it belongs to %s", man.Repo)
	}
	if err := man.ValidateRelease(); err != nil {
		return nil, err
	}
	return man, nil
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// Installing
// ---------------------------------------------------------------------------

// ErrNeedsConfirm is what choosing a version needs before it goes ahead: someone
// has read the warning.
type ErrNeedsConfirm struct{ Warning string }

func (e *ErrNeedsConfirm) Error() string { return e.Warning }

// Install starts installing the newest release the last check found, in the
// background. Progress is in the Report. An admin may choose a version that was
// taken out of use before; nothing automatic does.
func (m *Manager) Install(id string) error {
	m.mu.Lock()
	e := m.mods[id]
	if e == nil {
		m.mu.Unlock()
		return ErrUnknown
	}
	if !updatable(e.builtin) {
		m.mu.Unlock()
		return errors.New(e.builtin.Name + " is built into the server and updates with it")
	}
	m.loadFeed(id, e)
	m.computeOffers(e)
	rel := e.rel
	m.mu.Unlock()
	if rel == nil {
		return errors.New("no compatible newer version is known; check for updates first")
	}
	return m.InstallVersion(id, rel.version, true)
}

// InstallVersion starts moving a module to a given version, newer or older. Without
// confirm, a choice with a warning is refused with *ErrNeedsConfirm. A version already
// on disk is switched to with no download, so going back works offline.
func (m *Manager) InstallVersion(id, ver string, confirm bool) error {
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
	case e.state.Removed:
		return errors.New(e.builtin.Name + " isn't on this server; add it first")
	case !wittymodule.Valid(ver):
		return fmt.Errorf("%q is not a version", ver)
	}
	m.loadFeed(id, e)
	if e.dir != "" && e.man.Version == ver {
		return errors.New("that version is already in use")
	}
	k := e.known[ver]
	local, _, localErr := m.usable(id, e, ver)
	var man *wittymodule.Manifest
	switch {
	case localErr == nil:
		man = local
	case k != nil:
		man = k.man
	default:
		if ver == e.builtin.Version {
			return errors.New("that is the built-in copy; choose it with use-built-in")
		}
		if errors.Is(localErr, errUntrusted) {
			return localErr
		}
		return errors.New("that version isn't known; check for updates first")
	}
	refused, warning := m.judge(e, man)
	if refused != "" {
		return errors.New(refused)
	}
	if warning != "" && !confirm {
		return &ErrNeedsConfirm{Warning: warning}
	}
	older := wittymodule.Compare(ver, e.man.Version) < 0
	if localErr == nil {
		return m.activate(id, e, man, older)
	}
	e.busy, e.phase, e.lastError, e.done, e.total = true, PhaseDownloading, "", 0, k.rel.archive.Size
	rel := k.rel
	session := logging.NewID("mod")
	log := m.log.With("session", session, "module", id, "version", rel.version)
	log.Info("modules: installing")
	logging.Go("module install", func() {
		ctx, cancel := context.WithTimeout(logging.WithSession(context.Background(), session), 30*time.Minute)
		defer cancel()
		err := m.install(ctx, id, e, &rel, older)
		m.mu.Lock()
		defer m.mu.Unlock()
		e.busy = false
		if err != nil {
			e.phase, e.lastError = PhaseFailed, sentence(err.Error())
			e.note("failed", rel.version, "the install failed: "+err.Error())
			if serr := m.saveState(); serr != nil {
				m.log.Warn("modules: saving the state", "err", serr)
			}
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
func (m *Manager) install(ctx context.Context, id string, e *entry, r *release, older bool) error {
	// 1. The signed list and the manifest, fetched again rather than remembered.
	m.setPhase(e, PhaseVerifying)
	man, v, err := m.verifiedRelease(ctx, id, e, r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	refused, _ := m.judge(e, man)
	m.mu.Unlock()
	if refused != "" {
		return errors.New(refused)
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
	m.mu.Lock()
	keys := m.keysFor(e)
	m.mu.Unlock()
	if err := verifyInstalled(stage, man, keys); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(stage, wittymodule.ManifestFile))
	if err != nil {
		return errors.New("the package has no manifest")
	}
	if string(b) != string(v.manifest) {
		return errors.New("the manifest in the package differs from the release's")
	}

	// 5. It runs here, and it is what it says. A bundle runs in browsers, where the
	// server can only check it is all there.
	if man.PerPlatform() {
		if _, err := os.Stat(filepath.Join(stage, man.EntryFile(runtime.GOOS))); err != nil {
			return fmt.Errorf("the package has no %s", man.EntryFile(runtime.GOOS))
		}
		// With the limits it would have in use: this is the first time its code runs.
		m.mu.Lock()
		p := m.newProcFor(id, e, man, stage)
		m.mu.Unlock()
		p.verify = nil
		if err := p.Try(); err != nil {
			return fmt.Errorf("this version didn't start on this server, which can happen if the data folder doesn't allow programs to run: %w", err)
		}
	} else {
		if _, err := os.Stat(filepath.Join(stage, man.Entry)); err != nil {
			return fmt.Errorf("the package has no %s", man.Entry)
		}
		if man.Style != "" {
			if _, err := os.Stat(filepath.Join(stage, man.Style)); err != nil {
				return fmt.Errorf("the package has no %s", man.Style)
			}
		}
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
	if err := m.activate(id, e, man, older); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	return nil
}

// activate makes an installed version the one in use: remembered first, and only
// then used by the next call, with no restart. An older version is pinned, or the
// next automatic update would undo the choice. Call with m.mu held.
func (m *Manager) activate(id string, e *entry, man *wittymodule.Manifest, older bool) error {
	old := e.state
	blocked := map[string]string{}
	for k, val := range old.Blocked {
		if k != man.Version {
			blocked[k] = val
		}
	}
	e.state.Previous = old.Active
	if old.Active != "" && blocked[old.Active] != "" {
		e.state.Previous = old.Previous
	}
	if e.state.Previous == man.Version {
		e.state.Previous = ""
	}
	e.state.Active, e.state.Blocked = man.Version, blocked
	e.state.Chosen = wittymodule.Compare(man.Version, e.builtin.Version) <= 0 || older
	if older {
		e.state.Pin = man.Version
	} else if old.Pin != "" {
		e.state.Pin = man.Version // chosen by hand while pinned: stay on the choice
	}
	kind := "installed"
	if older {
		kind = "downgraded"
	}
	e.note(kind, man.Version, "from "+e.man.Version)
	if err := m.saveState(); err != nil {
		e.state = old
		return fmt.Errorf("the state could not be saved: %w", err)
	}
	// 7. The next call uses the new version.
	m.resolve(id, e)
	m.computeOffers(e)
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

// signedFiles reads a version's signed list: name inside the version → hash, for this
// system's files only. It proves the signature first.
func signedFiles(dir string, man *wittymodule.Manifest, keys []ed25519.PublicKey) (map[string]string, error) {
	sums, err := os.ReadFile(filepath.Join(dir, signedList))
	if err != nil {
		return nil, fmt.Errorf("%w: its signed list is missing", errUntrusted)
	}
	sig, err := os.ReadFile(filepath.Join(dir, signedSig))
	if err != nil {
		return nil, fmt.Errorf("%w: its signature is missing", errUntrusted)
	}
	if err := update.VerifySums(sums, sig, keys); err != nil {
		return nil, fmt.Errorf("%w: %w", errUntrusted, err)
	}
	list, err := update.ParseSums(sums)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUntrusted, err)
	}
	prefix := man.Top(runtime.GOOS, runtime.GOARCH) + "/"
	want := map[string]string{}
	for name, h := range list {
		if rel, ok := strings.CutPrefix(name, prefix); ok {
			want[rel] = h
		}
	}
	if len(want) == 0 {
		return nil, fmt.Errorf("%w: its signed list has nothing for this system", errUntrusted)
	}
	return want, nil
}

// verifyInstalled checks a version's folder against the signed list kept in it:
// the signature is good, every file the list names for this system is there and
// unchanged, and nothing else is there. It runs again before every start of a
// helper, so a file changed on a shared data folder is refused rather than run.
func verifyInstalled(dir string, man *wittymodule.Manifest, keys []ed25519.PublicKey) error {
	want, err := signedFiles(dir, man, keys)
	if err != nil {
		return err
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
