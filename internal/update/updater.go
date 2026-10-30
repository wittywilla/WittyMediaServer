package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// ExitRestart is the exit code with which the server asks the program that
// started it (the tray app, systemd) to start it again after an update.
const ExitRestart = 75

// Phases of an update, shown in Settings.
const (
	PhaseIdle        = "idle"
	PhaseDownloading = "downloading"
	PhaseVerifying   = "verifying"
	PhaseInstalling  = "installing"
	PhaseRestarting  = "restarting"
	PhaseFailed      = "failed"
)

// Info describes the newest release for this system.
type Info struct {
	Version    string `json:"version"`
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	URL        string `json:"url"`
	Published  int64  `json:"published,omitempty"` // unix ms
	Prerelease bool   `json:"prerelease"`
	Signed     bool   `json:"signed"`
	Size       int64  `json:"size"`
}

// Status is what Settings shows.
type Status struct {
	Current    string `json:"current"`
	Latest     *Info  `json:"latest,omitempty"` // only when newer than Current
	CheckedAt  int64  `json:"checkedAt,omitempty"`
	CheckError string `json:"checkError,omitempty"`
	Phase      string `json:"phase"`
	Done       int64  `json:"done,omitempty"` // bytes downloaded
	Total      int64  `json:"total,omitempty"`
	Error      string `json:"error,omitempty"` // why the last install failed
	CanApply   bool   `json:"canApply"`
	Reason     string `json:"reason,omitempty"` // why this copy can't update itself
	Page       string `json:"page"`
}

// Updater checks for, downloads and installs releases.
type Updater struct {
	Current   string            // the running version
	Dir       string            // the program folder (where wms lives)
	Protect   []string          // folders and files never written (data, cache, config…)
	Transport http.RoundTripper // internet-only transport for GitHub
	Log       *slog.Logger
	// Restart is called once the new files are in place; it should stop the
	// server and start the new version.
	Restart func()

	mu       sync.Mutex
	rel      *Release
	st       Status
	checked  time.Time
	checking chan struct{}
	busy     bool
	reason   string // cached cannotApply answer
	reasonAt time.Time
}

// New prepares an updater for the program in dir.
func New(current, dir string, protect []string, tr http.RoundTripper, log *slog.Logger) *Updater {
	u := &Updater{Current: current, Dir: realPath(dir), Transport: tr, Log: log}
	for _, p := range protect {
		if p != "" {
			u.Protect = append(u.Protect, realPath(p))
		}
	}
	u.st = Status{Current: current, Phase: PhaseIdle, Page: ReleasesPage}
	return u
}

// Status returns the last known state.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	st := u.st
	st.CanApply, st.Reason = false, ""
	if st.Latest != nil {
		if time.Since(u.reasonAt) > 30*time.Second {
			u.reason, u.reasonAt = u.cannotApply(), time.Now()
		}
		if reason := u.reason; reason != "" {
			st.Reason = reason
		} else if !st.Latest.Signed {
			st.Reason = "This release is not signed, so it can't be installed from here. Download it from GitHub."
		} else {
			st.CanApply = true
		}
	}
	return st
}

// Available returns the newer version found by the last check, or "".
func (u *Updater) Available() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.st.Latest == nil {
		return ""
	}
	return u.st.Latest.Version
}

// Check asks GitHub for releases unless the last answer is younger than
// maxAge. Concurrent calls share one request.
func (u *Updater) Check(ctx context.Context, maxAge time.Duration) Status {
	u.mu.Lock()
	if !u.checked.IsZero() && time.Since(u.checked) < maxAge {
		u.mu.Unlock()
		return u.Status()
	}
	if u.checking != nil {
		ch := u.checking
		u.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
		}
		return u.Status()
	}
	ch := make(chan struct{})
	u.checking = ch
	u.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	rels, err := u.fetchReleases(cctx)
	cancel()

	u.mu.Lock()
	u.checked = time.Now()
	u.st.CheckedAt = u.checked.UnixMilli()
	u.checking = nil
	close(ch)
	if err != nil {
		u.st.CheckError = err.Error()
		u.mu.Unlock()
		u.Log.Warn("update check failed", "err", err)
		return u.Status()
	}
	u.st.CheckError = ""
	rel := latest(rels, u.Current, runtime.GOOS, runtime.GOARCH)
	u.rel = rel
	if rel == nil {
		u.st.Latest = nil
	} else {
		u.st.Latest = &Info{Version: rel.Version, Title: rel.Title, Notes: rel.Notes, URL: rel.Page,
			Prerelease: rel.Prerelease, Signed: rel.Signed(), Size: rel.Archive.Size}
		if !rel.Published.IsZero() {
			u.st.Latest.Published = rel.Published.UnixMilli()
		}
	}
	u.mu.Unlock()
	if rel != nil {
		u.Log.Info("update available", "version", rel.Version, "current", u.Current)
	}
	return u.Status()
}

// Run checks at start (after a short pause) and then twice a day while
// enabled() says automatic checks are on.
func (u *Updater) Run(ctx context.Context, enabled func() bool) {
	wait := 2 * time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 12 * time.Hour
		if enabled() && Valid(u.Current) {
			u.Check(ctx, 6*time.Hour)
		}
	}
}

// cannotApply says why this copy can't replace its own files, or "".
// Call with u.mu held.
func (u *Updater) cannotApply() string {
	switch {
	case u.Restart == nil:
		return "This server can't restart itself."
	case runtime.GOOS == "darwin":
		return "On a Mac, download the new version and replace the app."
	case fileExists("/.dockerenv") || os.Getenv("WMS_DOCKER") == "1":
		return "This server runs in a container: pull the new image and recreate the container."
	}
	exe := "wms"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if !fileExists(filepath.Join(u.Dir, exe)) || !fileExists(filepath.Join(u.Dir, "LICENSE")) {
		return "This copy wasn't installed from a release package, so it can't replace its own files."
	}
	for _, p := range u.Protect {
		if within(u.Dir, p) {
			return "The program folder is also the data folder, so the update can't safely replace files there."
		}
	}
	if !writable(u.Dir) {
		if runtime.GOOS == "windows" {
			return "WittyMediaServer can't write to its program folder (" + u.Dir + "). Install it with the Windows setup, which installs for your account and updates itself, or run install.ps1 again as administrator."
		}
		return "WittyMediaServer can't write to its program folder (" + u.Dir + "). Update it with install.sh as root."
	}
	return ""
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".wms-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	return os.Remove(name) == nil
}

// Apply starts installing the newest release in the background. Progress
// is in Status; on success Restart is called.
func (u *Updater) Apply() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busy {
		return errors.New("an update is already being installed")
	}
	if u.rel == nil {
		return errors.New("no newer version is known; check for updates first")
	}
	if r := u.cannotApply(); r != "" {
		return errors.New(r)
	}
	if !u.rel.Signed() {
		return errors.New("this release is not signed, so it can't be installed from here")
	}
	u.busy = true
	u.st.Phase, u.st.Error, u.st.Done, u.st.Total = PhaseDownloading, "", 0, u.rel.Archive.Size
	rel := *u.rel
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		err := u.install(ctx, &rel)
		u.mu.Lock()
		u.busy = false
		if err != nil {
			u.st.Phase, u.st.Error = PhaseFailed, err.Error()
		} else {
			u.st.Phase = PhaseRestarting
		}
		u.mu.Unlock()
		if err != nil {
			u.Log.Error("update failed", "version", rel.Version, "err", err)
			return
		}
		u.Log.Info("update installed; restarting", "version", rel.Version)
		// Let Settings see "restarting" before the server goes away.
		time.Sleep(1500 * time.Millisecond)
		u.Restart()
	}()
	return nil
}

func (u *Updater) phase(p string) {
	u.mu.Lock()
	u.st.Phase = p
	u.mu.Unlock()
}

func (u *Updater) install(ctx context.Context, rel *Release) error {
	stage := filepath.Join(u.Dir, StageDir)
	if st, err := os.Lstat(stage); err == nil && !st.IsDir() {
		return fmt.Errorf("%s is in the way", stage)
	}
	// Leftovers of an earlier try (not the list of files to remove).
	work := filepath.Join(stage, "work")
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(work)

	// 1. The checksum list and its signature, checked before anything else.
	sums, err := u.fetchSmall(ctx, rel.Sums.URL, maxSumsBytes)
	if err != nil {
		return fmt.Errorf("checksums: %w", err)
	}
	sig, err := u.fetchSmall(ctx, rel.Sig.URL, maxSigBytes)
	if err != nil {
		return fmt.Errorf("signature: %w", err)
	}
	u.phase(PhaseVerifying)
	if err := VerifySums(sums, sig, TrustedKeys()); err != nil {
		return err
	}
	list, err := ParseSums(sums)
	if err != nil {
		return err
	}
	want, ok := list[rel.Archive.Name]
	if !ok {
		return fmt.Errorf("the signed checksum list has no entry for %s", rel.Archive.Name)
	}

	// 2. The package, hashed while it downloads.
	u.phase(PhaseDownloading)
	archive := filepath.Join(work, rel.Archive.Name)
	if err := u.download(ctx, rel.Archive, archive, want); err != nil {
		return err
	}

	// 3. Unpack next to the program and swap the files in.
	u.phase(PhaseInstalling)
	newDir := filepath.Join(work, "new")
	if err := Extract(archive, newDir, 2*maxArchive); err != nil {
		return fmt.Errorf("unpacking: %w", err)
	}
	_ = os.Remove(archive)
	exe := "wms"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if !fileExists(filepath.Join(newDir, exe)) {
		return fmt.Errorf("the package has no %s", exe)
	}
	olds, err := Swap(u.Dir, newDir, u.Protect)
	if err != nil {
		return err
	}
	if err := RecordOld(u.Dir, olds); err != nil {
		u.Log.Warn("update: could not list the old files to remove", "err", err)
	}
	noteInstalled(u.Dir, rel.Version)
	return nil
}

// download saves an asset to dst and checks its SHA-256 against want.
func (u *Updater) download(ctx context.Context, a Asset, dst, want string) error {
	res, err := u.get(ctx, a.URL, "application/octet-stream")
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
	pw := &progress{u: u}
	n, err := io.Copy(io.MultiWriter(f, h, pw), io.LimitReader(res.Body, limit+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if n > limit {
		return errors.New("download: the file is larger than the release says")
	}
	u.phase(PhaseVerifying)
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("the download of %s does not match its signed checksum; it was not installed", a.Name)
	}
	return nil
}

type progress struct{ u *Updater }

func (p *progress) Write(b []byte) (int, error) {
	p.u.mu.Lock()
	p.u.st.Done += int64(len(b))
	p.u.mu.Unlock()
	return len(b), nil
}
