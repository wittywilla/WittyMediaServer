package modules

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/wittywilla/wittymodule"

	"github.com/wittywilla/wittymediaserver/internal/update"
)

// What the repair tools need to know about installed modules, with no Manager and with
// nothing started. A server that isn't running can still be looked at: this only reads
// state.json and the version folders, the same way New does when it starts.

// InstalledState is one module's saved choices.
type InstalledState struct {
	Active   string
	Previous string
	Pin      string
	Removed  bool
	Blocked  map[string]string
	source   *Source
}

// ReadInstalledState reads <dir>/state.json. No file means nothing was ever chosen. An
// error means the file is there and can't be read.
func ReadInstalledState(dir string) (map[string]InstalledState, error) {
	out := map[string]InstalledState{}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var saved map[string]moduleState
	if err := json.Unmarshal(b, &saved); err != nil {
		return nil, err
	}
	for id, s := range saved {
		out[id] = InstalledState{Active: s.Active, Previous: s.Previous, Pin: s.Pin, Removed: s.Removed, Blocked: s.Blocked, source: s.Source}
	}
	return out, nil
}

// What can be wrong with an installed version, for choosing what to say.
const (
	ProblemMissing   = "missing"   // the folder or its manifest has gone
	ProblemSignature = "signature" // the signature or a file doesn't match
	ProblemVersion   = "version"   // it isn't for this server
)

// VersionProblem is why an installed version can't be used.
type VersionProblem struct {
	Kind string
	Err  error
}

func (p *VersionProblem) Error() string { return p.Err.Error() }

// CheckInstalled looks at an installed version as a starting server would. It returns nil
// when the version would be used.
func CheckInstalled(dir, id, ver, server string, st InstalledState) *VersionProblem {
	if !wittymodule.Valid(ver) {
		return &VersionProblem{ProblemMissing, fmt.Errorf("%q is not a version", ver)}
	}
	vdir := filepath.Join(dir, id, "versions", ver)
	b, err := os.ReadFile(filepath.Join(vdir, wittymodule.ManifestFile))
	if err != nil {
		return &VersionProblem{ProblemMissing, errors.New("it is no longer on disk")}
	}
	man, err := wittymodule.ParseManifest(b)
	if err != nil {
		return &VersionProblem{ProblemMissing, err}
	}
	if man.ID != id || man.Version != ver {
		return &VersionProblem{ProblemSignature, fmt.Errorf("its files say %s %s", man.ID, man.Version)}
	}
	keys := update.ModuleKeys()
	if st.source != nil {
		keys = []ed25519.PublicKey{}
		if k, err := parseSourceKey(st.source.Key); err == nil {
			keys = []ed25519.PublicKey{k}
		}
	}
	if err := verifyInstalled(vdir, man, keys); err != nil {
		return &VersionProblem{ProblemSignature, err}
	}
	if err := man.Supports(server, wittymodule.MinAPI, wittymodule.API); err != nil {
		return &VersionProblem{ProblemVersion, err}
	}
	return nil
}

// ReinstallNow asks the publisher for the newest compatible version, installs it and waits.
// It's the one thing here that goes online, and only someone asking for it gets here.
func (m *Manager) ReinstallNow(ctx context.Context, id string) error {
	m.mu.Lock()
	_, known := m.mods[id]
	m.mu.Unlock()
	if !known {
		return ErrUnknown
	}
	if r := m.Check(ctx, 0); r.CheckError != "" {
		return errors.New(r.CheckError)
	}
	if err := m.Install(id); err != nil {
		return err
	}
	for {
		m.mu.Lock()
		e := m.mods[id]
		busy, failed := e.busy, e.lastError
		m.mu.Unlock()
		if !busy {
			if failed != "" {
				return errors.New(failed)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// ForgetVersion takes a version out of use in state.json, with no Manager running: the
// version before it becomes the choice if it's still good, or else the built-in copy.
// A running server keeps this in memory, so it must use Rollback and UseBuiltin instead.
func ForgetVersion(dir, id, ver, server string) (now string, err error) {
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return "", err
	}
	var saved map[string]moduleState
	if err := json.Unmarshal(b, &saved); err != nil {
		return "", err
	}
	s, ok := saved[id]
	if !ok {
		return "", nil
	}
	if s.Pin == ver {
		s.Pin = ""
	}
	if s.Previous == ver {
		s.Previous = ""
	}
	if s.Active == ver {
		s.Active = ""
		s.Chosen = false
		if s.Previous != "" && CheckInstalled(dir, id, s.Previous, server, InstalledState{source: s.Source}) == nil {
			s.Active, s.Previous = s.Previous, ""
		}
	}
	saved[id] = s
	out, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return "", err
	}
	return s.Active, writeAtomic(filepath.Join(dir, "state.json"), out)
}
