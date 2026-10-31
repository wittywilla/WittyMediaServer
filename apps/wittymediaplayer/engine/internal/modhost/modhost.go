// Package modhost runs the player's disc modules the way WittyMediaServer runs its own:
// each has a built-in copy that always works, and a newer signed release can replace it
// without a new player. A downloaded version runs as a separate, limited process, is
// checked against its signature every time it starts, and is taken back out by itself
// if it keeps failing.
//
// This package has no network code. Downloading is the updater's job, in a program of
// its own that only runs when the viewer asks.
package modhost

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/wittywilla/wittymodule"
)

// ReleaseKey is the public half of the key that signs this project's releases, the same
// one WittyMediaServer trusts.
const ReleaseKey = "VsPm/CW2lhxXrMSbWZJYwAGtT420DWVEbTB3ub47CHE="

// In a version's folder, next to its files, so they can be checked again.
const (
	SignedList = "SHA256SUMS.txt"
	SignedSig  = "SHA256SUMS.txt.sig"
)

// failLimit is how many calls in a row may fail at the process level (a crash, a hang,
// garbage) before that version is taken back out. A disc a parser refuses isn't one.
const failLimit = 3

// Module is one module the player knows. Only these are accepted.
type Module struct {
	ID      string
	Name    string
	Repo    string
	Builtin string // the built-in copy's version
}

// State is what's remembered about a module.
type State struct {
	Active   string            `json:"active,omitempty"`
	Previous string            `json:"previous,omitempty"`
	Blocked  map[string]string `json:"blocked,omitempty"`
	Failures int               `json:"failures,omitempty"`
	// Note says in plain words what last happened, for Settings.
	Note string `json:"note,omitempty"`
}

// Manager keeps the installed versions of the modules.
type Manager struct {
	Dir     string // <data>/modules
	Host    string // this player's version
	Keys    []ed25519.PublicKey
	Modules []Module
}

// DefaultKeys are the keys that may sign a module.
func DefaultKeys() []ed25519.PublicKey {
	raw, err := base64.StdEncoding.DecodeString(ReleaseKey)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil
	}
	return []ed25519.PublicKey{raw}
}

// DefaultDir is where modules live: the viewer's own data, never the program's folder.
func DefaultDir() string {
	if d := os.Getenv("WMP_DATA"); d != "" {
		return filepath.Join(d, "modules")
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "WittyMediaPlayer", "modules")
}

// Module finds a known module.
func (m *Manager) Module(id string) (Module, bool) {
	for _, x := range m.Modules {
		if x.ID == id {
			return x, true
		}
	}
	return Module{}, false
}

func (m *Manager) moduleDir(id string) string { return filepath.Join(m.Dir, id) }

// VersionDir is where a version of a module is kept.
func (m *Manager) VersionDir(id, version string) string {
	return filepath.Join(m.moduleDir(id), "versions", version)
}

// LoadState reads what's remembered. Nothing remembered is the built-in copy.
func (m *Manager) LoadState(id string) State {
	var s State
	b, err := os.ReadFile(filepath.Join(m.moduleDir(id), "state.json"))
	if err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Blocked == nil {
		s.Blocked = map[string]string{}
	}
	return s
}

// SaveState writes it atomically.
func (m *Manager) SaveState(id string, s State) error {
	dir := m.moduleDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "state-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	tmp.Close()
	return os.Rename(tmp.Name(), filepath.Join(dir, "state.json"))
}

// VerifySums checks a signature against the exact bytes of a checksum list.
func VerifySums(sums, sig []byte, keys []ed25519.PublicKey) error {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return errors.New("its signature is not valid")
	}
	for _, k := range keys {
		if ed25519.Verify(k, sums, raw) {
			return nil
		}
	}
	return errors.New("its signature is not valid")
}

var sumLine = regexp.MustCompile(`^([0-9a-fA-F]{64}) [ *](.+)$`)

// ParseSums reads a checksum list into name and lowercase hex digest.
func ParseSums(b []byte) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		g := sumLine.FindStringSubmatch(line)
		if g == nil {
			return nil, errors.New("its checksum list is unreadable")
		}
		out[g[2]] = strings.ToLower(g[1])
	}
	if len(out) == 0 {
		return nil, errors.New("its checksum list is empty")
	}
	return out, nil
}

// Sum is the SHA-256 of some bytes, as hex.
func Sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
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

// Supports says, in the viewer's words, why a version can't run on this player.
func (m *Manager) Supports(man *wittymodule.Manifest) error {
	err := man.Supports(m.Host, wittymodule.MinAPI, wittymodule.API)
	if err == nil {
		return nil
	}
	// A manifest's "server" here means the player.
	r := strings.NewReplacer("WittyMediaServer", "WittyMediaPlayer", "this server", "this player", "a newer server", "a newer player", "this server's", "this player's")
	return errors.New(r.Replace(err.Error()))
}

// VerifyDir checks a folder against its signed list: every file there and no others,
// and a manifest for this module and version. It runs each time a version starts, so a
// file changed afterwards is refused rather than run.
func (m *Manager) VerifyDir(mod Module, version, dir string) (*wittymodule.Manifest, error) {
	sums, err := os.ReadFile(filepath.Join(dir, SignedList))
	if err != nil {
		return nil, fmt.Errorf("its signed list is missing: %w", err)
	}
	sig, err := os.ReadFile(filepath.Join(dir, SignedSig))
	if err != nil {
		return nil, fmt.Errorf("its signature is missing: %w", err)
	}
	if err := VerifySums(sums, sig, m.Keys); err != nil {
		return nil, err
	}
	list, err := ParseSums(sums)
	if err != nil {
		return nil, err
	}
	top := wittymodule.TopFolder(mod.ID, version, runtime.GOOS, runtime.GOARCH) + "/"
	want := map[string]string{}
	for name, h := range list {
		if strings.HasPrefix(name, top) {
			want[strings.TrimPrefix(name, top)] = h
		}
	}
	if len(want) == 0 {
		return nil, errors.New("it has no files for this computer")
	}
	seen := 0
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() || rel == SignedList || rel == SignedSig {
			return nil
		}
		h, ok := want[rel]
		if !ok {
			return fmt.Errorf("%s isn't in its signed list", rel)
		}
		got, err := fileSum(path)
		if err != nil {
			return err
		}
		if got != h {
			return fmt.Errorf("%s doesn't match its signed checksum", rel)
		}
		seen++
		return nil
	})
	if err != nil {
		return nil, err
	}
	if seen != len(want) {
		return nil, errors.New("some of its files are missing")
	}
	raw, err := os.ReadFile(filepath.Join(dir, wittymodule.ManifestFile))
	if err != nil {
		return nil, err
	}
	man, err := wittymodule.ParseManifest(raw)
	if err != nil {
		return nil, err
	}
	if man.ID != mod.ID || man.Version != version || !strings.EqualFold(man.Repo, mod.Repo) || man.Delivery != wittymodule.DeliveryHelper {
		return nil, fmt.Errorf("its manifest says %s %s", man.ID, man.Version)
	}
	return man, nil
}

// Choice is what plays a module's discs now.
type Choice struct {
	Version string
	Dir     string
	Entry   string // the program to run; empty for the built-in copy
	Note    string // why it isn't what was chosen, in plain words
}

// Choose takes the first of these that passes: the chosen version, the previous one,
// the built-in copy. The built-in copy is also used if it's newer than what's installed.
func (m *Manager) Choose(mod Module) Choice {
	s := m.LoadState(mod.ID)
	note := ""
	for _, v := range []string{s.Active, s.Previous} {
		if v == "" {
			continue
		}
		if why, blocked := s.Blocked[v]; blocked {
			note = fmt.Sprintf("Version %s isn't used: %s", v, why)
			continue
		}
		if wittymodule.Compare(v, mod.Builtin) <= 0 {
			continue
		}
		dir := m.VersionDir(mod.ID, v)
		man, err := m.VerifyDir(mod, v, dir)
		if err == nil {
			err = m.Supports(man)
		}
		if err != nil {
			note = fmt.Sprintf("Version %s isn't used: %s.", v, err)
			continue
		}
		return Choice{Version: v, Dir: dir, Entry: filepath.Join(dir, man.EntryFile(runtime.GOOS)), Note: note}
	}
	return Choice{Version: mod.Builtin, Note: note}
}

// CallOK resets the count of failures in a row.
func (m *Manager) CallOK(mod Module) {
	s := m.LoadState(mod.ID)
	if s.Failures != 0 {
		s.Failures = 0
		_ = m.SaveState(mod.ID, s)
	}
}

// CallFailed counts a process-level failure, and takes the version back out after too
// many in a row. It says whether it did.
func (m *Manager) CallFailed(mod Module, version, reason string) bool {
	s := m.LoadState(mod.ID)
	s.Failures++
	rolled := false
	if s.Failures >= failLimit {
		s.Blocked[version] = reason
		s.Failures = 0
		s.Note = fmt.Sprintf("Version %s kept failing (%s), so it was taken back out.", version, reason)
		if s.Active == version {
			s.Active, s.Previous = s.Previous, ""
		}
		rolled = true
	}
	_ = m.SaveState(mod.ID, s)
	return rolled
}

// Rollback goes back by hand, to the previous version or the built-in copy. The version
// left stays blocked from being chosen by itself, until it's installed again.
func (m *Manager) Rollback(mod Module) (string, error) {
	s := m.LoadState(mod.ID)
	if s.Active == "" {
		return mod.Builtin, errors.New("this module is already using its built-in copy")
	}
	s.Blocked[s.Active] = "you went back from it"
	s.Active, s.Previous = s.Previous, ""
	s.Failures = 0
	s.Note = ""
	if err := m.SaveState(mod.ID, s); err != nil {
		return "", err
	}
	return m.Choose(mod).Version, nil
}

// Installed records a finished install, keeps only the versions that could be used, and
// clears any block on it, since the viewer chose it.
func (m *Manager) Installed(mod Module, version string) error {
	s := m.LoadState(mod.ID)
	if s.Active != version {
		s.Previous, s.Active = s.Active, version
	}
	delete(s.Blocked, version)
	s.Failures = 0
	s.Note = ""
	if err := m.SaveState(mod.ID, s); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(m.moduleDir(mod.ID), "versions"))
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.Name() != s.Active && e.Name() != s.Previous {
			_ = os.RemoveAll(filepath.Join(m.moduleDir(mod.ID), "versions", e.Name()))
		}
	}
	return nil
}

// Status is a module as Settings shows it.
type Status struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Builtin   string `json:"builtin"`
	InUse     string `json:"inUse"`
	IsBuilt   bool   `json:"isBuiltin"`
	CanGoBack bool   `json:"canGoBack"`
	Note      string `json:"note,omitempty"`
}

// Statuses lists every module.
func (m *Manager) Statuses() []Status {
	out := []Status{}
	for _, mod := range m.Modules {
		c := m.Choose(mod)
		s := m.LoadState(mod.ID)
		note := c.Note
		if note == "" {
			note = s.Note
		}
		out = append(out, Status{ID: mod.ID, Name: mod.Name, Builtin: mod.Builtin, InUse: c.Version, IsBuilt: c.Entry == "", CanGoBack: s.Active != "", Note: note})
	}
	return out
}
