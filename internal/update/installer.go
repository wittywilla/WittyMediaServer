package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/layout"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
)

// A copy the server can't replace by itself (the Windows installer for everyone, the macOS, Linux
// and FreeBSD packages) gets the right installer for its system and processor instead: downloaded
// here, checked against the release's signed checksum list, and kept for the admin to run.

// How an admin updates this copy, in Status.Method.
const (
	MethodApply     = "apply"     // Update now: the server replaces its own files
	MethodInstaller = "installer" // Download the installer, then run it
	MethodDocker    = "docker"    // pull the new image and recreate the container
)

// Installer phases.
const (
	InstallerDownloading = "downloading"
	InstallerVerifying   = "verifying"
	InstallerReady       = "ready"
	InstallerFailed      = "failed"
)

// Installer is the installer for this copy and where its download stands.
type Installer struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Phase string `json:"phase,omitempty"`
	Done  int64  `json:"done,omitempty"`
	Error string `json:"error,omitempty"`
	// Once it's checked: where it is on the server's computer, and for packages a package
	// manager installs, the command that installs it.
	Path    string `json:"path,omitempty"`
	Command string `json:"command,omitempty"`
}

// InstallerName is the release file that updates a copy installed as kind, or "" when the release
// has none for this system.
func InstallerName(ver, goos, goarch, kind string) string {
	base := "wittymediaserver-" + ver + "-"
	label := goarch
	if goos == "linux" && goarch == "arm" {
		label = "armv7"
	}
	switch kind {
	case layout.KindMSI:
		if goos == "windows" && (goarch == "amd64" || goarch == "arm64") {
			return base + "windows-" + goarch + ".msi"
		}
	case layout.KindPkg:
		if goos == "darwin" {
			return base + "macos-" + goarch + ".pkg"
		}
	case layout.KindDeb:
		if goos == "linux" {
			return base + "linux-" + label + ".deb"
		}
	case layout.KindRPM:
		if goos == "linux" {
			return base + "linux-" + label + ".rpm"
		}
	case layout.KindFreeBSD:
		if goos == "freebsd" {
			return base + "freebsd-" + goarch + ".pkg"
		}
	default:
		return ArchiveName(ver, goos, goarch)
	}
	return ""
}

// inContainer: Docker, or the image's own marker.
func inContainer() bool { return fileExists("/.dockerenv") || os.Getenv("WMS_DOCKER") == "1" }

// DockerImage is the image a container pulls to update: the newest alpha for someone on one.
func (u *Updater) dockerImage() string {
	if strings.Contains(u.Current, "-") {
		return "ghcr.io/wittywilla/wittymediaserver:alpha"
	}
	return "ghcr.io/wittywilla/wittymediaserver:latest"
}

// installerAsset is the installer for this copy in rel. Call with u.mu held.
func (u *Updater) installerAsset(rel *Release) (Asset, bool) {
	if rel == nil || u.DownloadDir == "" {
		return Asset{}, false
	}
	name := InstallerName(rel.Version, runtime.GOOS, runtime.GOARCH, u.Kind())
	a, ok := rel.Assets[name]
	return a, ok && name != ""
}

// FetchInstaller starts downloading the newest release's installer in the background. Progress
// is in Status().Installer.
func (u *Updater) FetchInstaller() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.rel == nil {
		return errors.New("no newer version is known; check for updates first")
	}
	if !u.rel.Signed() {
		return errors.New("this release is not signed, so its installer can't be checked here")
	}
	a, ok := u.installerAsset(u.rel)
	if !ok {
		return errors.New("this release has no installer for this system")
	}
	if u.dlBusy {
		return errors.New("the installer is already being downloaded")
	}
	if u.dl.Name == a.Name && u.dl.Phase == InstallerReady && fileExists(u.dl.Path) {
		return nil
	}
	u.dlBusy = true
	u.dl = Installer{Name: a.Name, Size: a.Size, Phase: InstallerDownloading}
	rel := *u.rel
	task := u.Tasks.Start(tasks.Spec{Type: tasks.TypeUpdate, Title: "Downloading the installer for " + rel.Version, Phase: "Downloading", Unit: "bytes", AdminOnly: true})
	session := logging.NewID("update")
	log := u.Log.With("session", session, "version", rel.Version, "file", a.Name)
	log.Info("installer download started")
	logging.Go("update-installer", func() {
		ctx, cancel := context.WithTimeout(logging.WithSession(context.Background(), session), time.Hour)
		defer cancel()
		path, err := u.fetchInstaller(ctx, &rel, a, task)
		u.mu.Lock()
		u.dlBusy = false
		if err != nil {
			u.dl.Phase, u.dl.Error = InstallerFailed, err.Error()
			task.Fail("The installer didn't download.", "Try again. Settings, About has the details.", err)
		} else {
			u.dl.Phase, u.dl.Path, u.dl.Command, u.dl.Done = InstallerReady, path, installCommand(u.Kind(), path), a.Size
			task.Result("Downloaded and checked. Settings, About says how to install it.")
			task.Finish(nil)
		}
		u.mu.Unlock()
		if err != nil {
			log.Error("installer download failed", "err", err)
		} else {
			log.Info("installer downloaded and checked", "path", path)
		}
	}, "cat", "update", "session", session)
	return nil
}

func (u *Updater) fetchInstaller(ctx context.Context, rel *Release, a Asset, task *tasks.Handle) (string, error) {
	phase := func(p, words string) {
		u.mu.Lock()
		u.dl.Phase = p
		u.mu.Unlock()
		task.Phase(words)
	}
	// The checksum list and its signature come first, as for an update.
	sums, err := u.fetchSmall(ctx, rel.Sums.URL, maxSumsBytes)
	if err != nil {
		return "", fmt.Errorf("checksums: %w", err)
	}
	sig, err := u.fetchSmall(ctx, rel.Sig.URL, maxSigBytes)
	if err != nil {
		return "", fmt.Errorf("signature: %w", err)
	}
	phase(InstallerVerifying, "Checking its signature")
	if err := VerifySums(sums, sig, TrustedKeys()); err != nil {
		return "", err
	}
	list, err := ParseSums(sums)
	if err != nil {
		return "", err
	}
	want, ok := list[a.Name]
	if !ok {
		return "", fmt.Errorf("the signed checksum list has no entry for %s", a.Name)
	}
	if err := os.MkdirAll(u.DownloadDir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(u.DownloadDir, a.Name)
	// One already here from an earlier try counts only if it still matches.
	if got, err := hashFile(dst); err == nil && got == want {
		u.dropOtherDownloads(a.Name)
		return dst, nil
	}
	phase(InstallerDownloading, "Downloading")
	part := dst + ".part"
	defer os.Remove(part)
	err = u.download(ctx, a, part, want, func(n int64) {
		u.mu.Lock()
		u.dl.Done += n
		done := u.dl.Done
		u.mu.Unlock()
		task.Progress(done, a.Size)
	}, func() { phase(InstallerVerifying, "Checking it against the signed checksum") })
	if err != nil {
		return "", err
	}
	if err := os.Rename(part, dst); err != nil {
		return "", err
	}
	u.dropOtherDownloads(a.Name)
	return dst, nil
}

// dropOtherDownloads removes installers of other versions, so only one is ever kept.
func (u *Updater) dropOtherDownloads(keep string) {
	ents, _ := os.ReadDir(u.DownloadDir)
	for _, e := range ents {
		if !e.IsDir() && e.Name() != keep {
			_ = os.Remove(filepath.Join(u.DownloadDir, e.Name()))
		}
	}
}

// CleanDownloads, at start, removes installers that are no longer newer than what runs.
func (u *Updater) CleanDownloads() {
	if u.DownloadDir == "" {
		return
	}
	ents, _ := os.ReadDir(u.DownloadDir)
	for _, e := range ents {
		if ver := installerVersion(e.Name()); e.IsDir() || ver == "" || Compare(ver, u.Current) <= 0 {
			_ = os.RemoveAll(filepath.Join(u.DownloadDir, e.Name()))
		}
	}
}

// installerVersion reads the version out of a release file's name, or "".
func installerVersion(name string) string {
	rest, ok := strings.CutPrefix(name, "wittymediaserver-")
	if !ok || strings.HasSuffix(name, ".part") {
		return ""
	}
	for _, sys := range []string{"-windows-", "-macos-", "-darwin-", "-linux-", "-freebsd-"} {
		if i := strings.Index(rest, sys); i > 0 && Valid(rest[:i]) {
			return rest[:i]
		}
	}
	return ""
}

// InstallerFile is the checked installer, once it's ready.
func (u *Updater) InstallerFile() (path, name string, ok bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.dl.Phase != InstallerReady || !fileExists(u.dl.Path) {
		return "", "", false
	}
	return u.dl.Path, u.dl.Name, true
}

// installCommand is what installs a package the package manager owns. The server never runs it:
// it needs the computer's administrator.
func installCommand(kind, path string) string {
	switch kind {
	case layout.KindDeb:
		return "sudo apt install " + shellQuote(path)
	case layout.KindRPM:
		return "sudo dnf install " + shellQuote(path)
	case layout.KindFreeBSD:
		return "pkg add -f " + shellQuote(path)
	case layout.KindPkg:
		return "sudo installer -pkg " + shellQuote(path) + " -target /"
	}
	return ""
}

func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+:", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("the file is larger than expected")
	}
	return b, nil
}
