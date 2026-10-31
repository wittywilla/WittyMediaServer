package modules

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"regexp"
	"strings"
	"time"
)

// A fork of a stock module can take its place: an admin names the fork's repository and
// the public key its publisher signs with, and from then on that module's releases come
// from there and are held to that key, and nothing else about it changes. Nothing
// unsigned ever runs. What changes is who is trusted, which is why it is one explicit
// choice per module, explained first, and why a fork gets less than a stock module.
//
// docs/MODULES.md, "Using someone's fork", has the reasoning.

// Source is where a module's releases come from instead of its own repository.
type Source struct {
	Repo    string `json:"repo"`
	Key     string `json:"key"` // base64 of the publisher's Ed25519 public key
	AddedAt int64  `json:"addedAt"`
	AddedBy string `json:"addedBy,omitempty"`
}

// SourceInfo is what the page shows about a fork in use.
type SourceInfo struct {
	Repo        string `json:"repo"`
	Fingerprint string `json:"fingerprint"` // to compare with what the publisher says
	AddedAt     int64  `json:"addedAt"`
	AddedBy     string `json:"addedBy,omitempty"`
}

// noFork lists modules a fork can't replace, and why, for the page. WittyTuner holds the
// line on encrypted broadcasts, and a fork of it would hold live streams and a network:
// nothing outside it could tell whether it had kept that line, so the server doesn't
// offer the choice. Disc engines can be replaced, because video never passes through them
// and the server checks for itself what they say about encryption.
var noFork = map[string]string{
	"wittytuner": "WittyTuner is what refuses encrypted channels, and it handles live streams with network access, so the server couldn't tell whether a fork still did. Forks of it aren't accepted.",
}

// ForkWarning is what someone agrees to before using a fork. It is said by the server, so
// every client that offers this says the same thing.
const ForkWarning = "Only do this for someone you trust. A module from another publisher is their code running on your server, " +
	"and neither this project nor anyone else has reviewed it. A helper or game runner runs with limits but can read what this server's account can read, " +
	"which may include your media and this server's database. A reader bundle runs in the browser of whoever is reading, with their access, including yours as an admin. " +
	"The signature only proves it came from whoever holds that key, not that it is safe. " +
	"This server won't install updates from a fork by itself, gives it no network access, and still refuses encrypted media whatever the fork says. " +
	"Check the key's fingerprint with the publisher, somewhere other than the place you got the key."

var sourceRepoRe = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)

func parseSourceKey(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("that isn't a public key: it should be the 44 characters `wittymodule keygen` prints")
	}
	return ed25519.PublicKey(b), nil
}

// Fingerprint is a short form of a key that two people can read out to compare.
func Fingerprint(key ed25519.PublicKey) string {
	h := sha256.Sum256(key)
	s := hex.EncodeToString(h[:10])
	var parts []string
	for i := 0; i < len(s); i += 4 {
		parts = append(parts, s[i:i+4])
	}
	return strings.Join(parts, " ")
}

// repo is where this module's releases come from now.
func (e *entry) repo() string {
	if e.state.Source != nil {
		return e.state.Source.Repo
	}
	return e.builtin.Repo
}

// keysFor are the keys this module's releases must be signed with: its fork's key alone,
// or this project's. A fork's key is good for that one module and nothing else. Call with
// m.mu held, or on an entry nothing else has yet.
func (m *Manager) keysFor(e *entry) []ed25519.PublicKey {
	if e.state.Source != nil {
		if k, err := parseSourceKey(e.state.Source.Key); err == nil {
			return []ed25519.PublicKey{k}
		}
		return []ed25519.PublicKey{} // a state file someone edited: nothing passes
	}
	return m.keys
}

func (e *entry) sourceInfo() *SourceInfo {
	s := e.state.Source
	if s == nil {
		return nil
	}
	info := &SourceInfo{Repo: s.Repo, AddedAt: s.AddedAt, AddedBy: s.AddedBy}
	if k, err := parseSourceKey(s.Key); err == nil {
		info.Fingerprint = Fingerprint(k)
	}
	return info
}

// SetSource makes a fork the source of a module. Without confirm it answers
// *ErrNeedsConfirm with the warning, and changes nothing. What was downloaded from the old
// source goes, since it was signed by someone else: the module runs its built-in copy
// until a version of the fork is chosen, and nothing installs on its own.
func (m *Manager) SetSource(id, repo, key, by string, confirm bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if why := noFork[id]; why != "" {
		return errors.New(why)
	}
	if !updatable(e.builtin) {
		return errors.New(e.builtin.Name + " has no releases of its own for a fork to replace")
	}
	if e.busy {
		return errors.New("an update of this module is being installed")
	}
	repo = strings.TrimSpace(repo)
	if !sourceRepoRe.MatchString(repo) {
		return errors.New("name the repository as owner/name, such as someone/WittyDVD")
	}
	k, err := parseSourceKey(key)
	if err != nil {
		return err
	}
	if !confirm {
		return &ErrNeedsConfirm{Warning: ForkWarning + " The key's fingerprint is " + Fingerprint(k) + "."}
	}
	return m.changeSource(id, e, &Source{Repo: repo, Key: base64.StdEncoding.EncodeToString(k), AddedAt: time.Now().UnixMilli(), AddedBy: by}, "fork of "+repo+", key "+Fingerprint(k))
}

// ClearSource goes back to the module's own repository and this project's key.
func (m *Manager) ClearSource(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.mods[id]
	if e == nil {
		return ErrUnknown
	}
	if e.busy {
		return errors.New("an update of this module is being installed")
	}
	if e.state.Source == nil {
		return nil
	}
	return m.changeSource(id, e, nil, "back to "+e.builtin.Repo)
}

// changeSource forgets everything that came from the old source. Call with m.mu held.
func (m *Manager) changeSource(id string, e *entry, s *Source, detail string) error {
	m.loadFeed(id, e)
	old := e.state
	e.state.Source = s
	e.state.Active, e.state.Previous, e.state.Pin, e.state.Chosen, e.state.Blocked, e.state.Withdrawn = "", "", "", false, nil, nil
	e.note("source-changed", "", detail)
	if err := m.saveState(); err != nil {
		e.state = old
		return err
	}
	e.known, e.etag, e.rel, e.latest, e.tooNew, e.lastError = map[string]*knownRelease{}, "", nil, nil, nil, ""
	_ = os.Remove(m.feedPath(id))
	m.resolve(id, e)
	m.prune(id, e)
	return nil
}
