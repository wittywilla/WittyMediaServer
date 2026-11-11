// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

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

	"github.com/wittywilla/wittymediaserver/internal/layout"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymodule"
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
	// Changes is what changed in each version after the running one, newest first.
	Changes []Change `json:"changes,omitempty"`
}

// Status is what Settings shows.
type Status struct {
	Current string `json:"current"`
	Latest  *Info  `json:"latest,omitempty"` // only when newer than Current
	// Withdrawn is why the running version was taken back, by the project's signed list.
	Withdrawn  string `json:"withdrawn,omitempty"`
	CheckedAt  int64  `json:"checkedAt,omitempty"`
	CheckError string `json:"checkError,omitempty"`
	Phase      string `json:"phase"`
	Done       int64  `json:"done,omitempty"` // bytes downloaded
	Total      int64  `json:"total,omitempty"`
	Error      string `json:"error,omitempty"` // why the last install failed
	CanApply   bool   `json:"canApply"`
	Reason     string `json:"reason,omitempty"` // why this copy can't update itself
	Page       string `json:"page"`
	// Method is how an admin updates this copy (MethodApply, MethodInstaller, MethodDocker),
	// or "" when only the release page can help.
	Method    string     `json:"method,omitempty"`
	Installer *Installer `json:"installer,omitempty"` // with MethodInstaller
	Image     string     `json:"image,omitempty"`     // with MethodDocker: the image to pull
}

// Updater checks for, downloads and installs releases.
type Updater struct {
	// Tasks is where an update shows in the activity panel. May be nil.
	Tasks     *tasks.Registry
	task      *tasks.Handle
	Current   string            // the running version
	Dir       string            // the program's top folder
	Protect   []string          // folders and files never written (data, cache, config…)
	Transport http.RoundTripper // internet-only transport for GitHub
	Log       *slog.Logger
	// Restart is called once the new files are in place; it should stop the
	// server and start the new version.
	Restart func()
	// DownloadDir is where an installer is kept for the admin to run. Empty: none is offered.
	DownloadDir string

	mu       sync.Mutex
	rel      *Release
	st       Status
	checked  time.Time
	checking chan struct{}
	busy     bool
	reason   string // cached cannotApply answer
	reasonAt time.Time

	withdrawn     *wittymodule.Withdrawals // the newest signed list seen
	withdrawnFile string                   // where it is kept

	kind       string    // how this copy was installed, read with reason
	dl         Installer // the installer download
	dlBusy     bool
	changes    []Change // what's new, read from the changelog for changesFor
	changesFor string
}

// New prepares an updater for the program in dir.
func New(current, dir string, protect []string, tr http.RoundTripper, log *slog.Logger) *Updater {
	u := &Updater{Current: current, Dir: realPath(dir), Transport: tr, Log: log.With("cat", "update")}
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
	st.Withdrawn = u.withdrawnReason(u.Current)
	st.CanApply, st.Reason, st.Method, st.Installer, st.Image = false, "", "", nil, ""
	if st.Latest != nil {
		if time.Since(u.reasonAt) > 30*time.Second {
			u.reason, u.reasonAt = u.cannotApply(), time.Now()
		}
		switch reason := u.reason; {
		case inContainer():
			st.Reason, st.Method, st.Image = reason, MethodDocker, u.dockerImage()
		case !st.Latest.Signed:
			st.Reason = "This release is not signed, so it can't be installed from here. Download it from GitHub."
		case reason == "":
			st.CanApply, st.Method = true, MethodApply
		default:
			st.Reason = reason
			if a, ok := u.installerAsset(u.rel); ok {
				in := Installer{Name: a.Name, Size: a.Size}
				if u.dl.Name == a.Name {
					in = u.dl
				}
				st.Method, st.Installer = MethodInstaller, &in
			}
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

	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	rels, err := u.fetchReleases(cctx)
	var rel *Release
	var changes []Change
	if err == nil {
		if werr := u.readWithdrawals(cctx, rels); werr != nil {
			u.Log.Warn("the list of withdrawn versions could not be read", "err", werr)
		}
		u.mu.Lock()
		rel = latest(rels, u.Current, runtime.GOOS, runtime.GOARCH, u.withdrawn)
		cached := rel != nil && u.changesFor == rel.Version
		changes = u.changes
		u.mu.Unlock()
		// The changelog is read once for each new version found, not at every check.
		if rel != nil && !cached {
			var fromLog bool
			if changes, fromLog = u.readChanges(cctx, rels, rel); fromLog {
				u.mu.Lock()
				u.changes, u.changesFor = changes, rel.Version
				u.mu.Unlock()
			}
		}
	}
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
	u.rel = rel
	if rel == nil {
		u.st.Latest = nil
	} else {
		u.st.Latest = &Info{Version: rel.Version, Title: rel.Title, Notes: rel.Notes, URL: rel.Page,
			Prerelease: rel.Prerelease, Signed: rel.Signed(), Size: rel.Archive.Size, Changes: changes}
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
	case inContainer():
		return "This server runs in a container: pull the new image and recreate the container."
	case runtime.GOOS == "darwin":
		return "On a Mac, the new version comes as an installer: download it, then open it. Your library and settings are kept."
	}
	switch u.Kind() {
	case layout.KindDeb:
		return "This copy was installed with a .deb package, which your package manager looks after. Download the new one here, then install it with apt as the computer's administrator. Your library and settings are kept."
	case layout.KindRPM:
		return "This copy was installed with an .rpm package, which your package manager looks after. Download the new one here, then install it with dnf as the computer's administrator. Your library and settings are kept."
	case layout.KindFreeBSD:
		return "This copy was installed with a FreeBSD package, which pkg looks after. Download the new one here, then install it with pkg add as root. Your library and settings are kept."
	case layout.KindPkg:
		return "On a Mac, the new version comes as an installer: download it, then open it. Your library and settings are kept."
	case layout.KindAppImage:
		return "This copy is an AppImage: one file that can't change itself. Download the new one here, make it executable and run it in place of this one. Your library and settings are kept."
	case layout.KindFlatpak:
		return "This copy is a Flatpak, which flatpak looks after. Download the new one here, then install it with flatpak install. Your library and settings are kept."
	case layout.KindSnap:
		return "This copy is a Snap, which snap looks after. Download the new one here, then install it with snap install as the computer's administrator. Your library and settings are kept."
	case layout.KindMSI, layout.KindArchive:
	default:
		return "This copy wasn't installed from a release package, so it can't replace its own files."
	}
	if !fileExists(filepath.Join(u.Dir, "bin", layout.Exe(layout.ServerCommand))) {
		return "This copy wasn't installed from a release package, so it can't replace its own files."
	}
	for _, p := range u.Protect {
		if within(u.Dir, p) {
			return "The program folder is also the data folder, so the update can't safely replace files there."
		}
	}
	if !writable(u.Dir) {
		if runtime.GOOS == "windows" {
			return "Windows protects the program folder (" + u.Dir + "), so this copy can't replace its own files. Download the new installer and run it: it updates in place and keeps your library and settings."
		}
		return "WittyMediaServer can't write to its program folder (" + u.Dir + "). Unpack the new package and run its install.sh as root."
	}
	return ""
}

// Kind says how this copy was installed.
func (u *Updater) Kind() string {
	return layout.Find(filepath.Join(u.Dir, layout.Exe(layout.DesktopApp))).Kind
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
	u.task = u.Tasks.Start(tasks.Spec{Type: tasks.TypeUpdate, Title: "Updating the server to " + rel.Version, Phase: "Downloading", Unit: "bytes", AdminOnly: true})
	session := logging.NewID("update")
	log := u.Log.With("session", session, "version", rel.Version)
	log.Info("update started", "current", u.Current)
	logging.Go("update", func() {
		ctx, cancel := context.WithTimeout(logging.WithSession(context.Background(), session), time.Hour)
		defer cancel()
		err := func() (err error) {
			// A fault ends it as failed: otherwise it stays busy, and no try can follow.
			defer func() {
				if p := recover(); p != nil {
					logging.ReportPanic("update", p, "cat", "update", "session", session)
					err = fmt.Errorf("the update stopped on a fault in the server (%v)", p)
				}
			}()
			return u.install(ctx, &rel)
		}()
		u.mu.Lock()
		u.busy = false
		if err != nil {
			u.st.Phase, u.st.Error = PhaseFailed, err.Error()
			u.task.Fail("The update didn't install, and nothing changed.", "Try again. Settings, About has the details.", err)
		} else {
			u.st.Phase = PhaseRestarting
			u.task.Result("Installed. The server is restarting.")
			u.task.Finish(nil)
		}
		u.mu.Unlock()
		if err != nil {
			log.Error("update failed", "err", err)
			return
		}
		log.Info("update installed; restarting")
		// Let Settings see "restarting" before the server goes away.
		time.Sleep(1500 * time.Millisecond)
		u.Restart()
	}, "cat", "update", "session", session)
	return nil
}

func (u *Updater) phase(p string) {
	u.mu.Lock()
	u.st.Phase = p
	u.task.Phase(phaseWords[p])
	u.mu.Unlock()
}

// phaseWords are the update's steps, for the activity panel.
var phaseWords = map[string]string{PhaseDownloading: "Downloading", PhaseVerifying: "Checking its signature", PhaseInstalling: "Installing"}

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
	err = u.download(ctx, rel.Archive, archive, want, func(n int64) {
		u.mu.Lock()
		u.st.Done += n
		u.task.Progress(u.st.Done, u.st.Total)
		u.mu.Unlock()
	}, func() { u.phase(PhaseVerifying) })
	if err != nil {
		return err
	}

	// 3. Unpack next to the program and swap the files in.
	u.phase(PhaseInstalling)
	newDir := filepath.Join(work, "new")
	if err := Extract(archive, newDir, 2*maxArchive); err != nil {
		return fmt.Errorf("unpacking: %w", err)
	}
	_ = os.Remove(archive)
	// Packages up to 0.5.2-alpha had their server in the top folder, so an update to one
	// of those is refused rather than leaving two servers side by side.
	exe := filepath.Join("bin", layout.Exe(layout.ServerCommand))
	if !fileExists(filepath.Join(newDir, exe)) {
		return fmt.Errorf("the package has no %s", exe)
	}
	olds, err := Swap(u.Dir, newDir, u.Protect)
	if err != nil {
		return err
	}
	if err := RecordOld(u.Dir, olds); err != nil {
		u.Log.WarnContext(ctx, "update: could not list the old files to remove", "err", err)
	}
	noteInstalled(u.Dir, rel.Version)
	return nil
}

// download saves an asset to dst and checks its SHA-256 against want, telling got how much has
// come, and verifying when it's all there.
func (u *Updater) download(ctx context.Context, a Asset, dst, want string, got func(n int64), verifying func()) error {
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
	n, err := io.Copy(io.MultiWriter(f, h, progress(got)), io.LimitReader(res.Body, limit+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if n > limit {
		return errors.New("download: the file is larger than the release says")
	}
	verifying()
	if sum := hex.EncodeToString(h.Sum(nil)); sum != want {
		return fmt.Errorf("the download of %s does not match its signed checksum; it was not installed", a.Name)
	}
	return nil
}

type progress func(n int64)

func (p progress) Write(b []byte) (int, error) {
	p(int64(len(b)))
	return len(b), nil
}
