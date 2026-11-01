package modules

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wittywilla/wittymodule"
)

// Stores other than this project's: an address an admin added, after a warning, with the key
// its catalogue must be signed with. docs/MODULES.md, "Stores other than this project's".
//
// A store's catalogue is hostile input until its signature passes, and only a claim after that:
// it decides nothing about what runs. A store can offer a fork of a module this server knows,
// which an admin still has to choose, under a fork's limits. It can't replace WittyTuner, and a
// wholly new module it lists is shown as in development and can't be installed.

const (
	// OfficialStore is this project's catalogue, in everything that names a store.
	OfficialStore = "official"
	maxStores     = 16
	storesFile    = "stores.json"
	storesDir     = "stores"
)

// How a third-party store's module relates to this server.
const (
	KindOfficial = "official"       // from this project's catalogue
	KindFork     = "fork"           // another publisher's version of a module this server knows
	KindDev      = "in-development" // a new kind of module, which can't be installed yet
	KindRefused  = "refused"        // it may never be replaced
)

// StoreWarning is what someone agrees to before adding a store. The server says it, so every
// client that offers this says the same thing.
var StoreWarning = []string{
	"Add a store only if you trust whoever runs it. Nobody at this project has reviewed anything in it.",
	"A module is someone's code running on your server. It can read what this server's account can read, which may include your media and this server's database. A web module runs in the browser of whoever uses it, with their access, including yours as an admin.",
	"Whoever runs this store holds its signing key, not this project. A signature only proves a file came from whoever holds that key, not that it's safe. If that key leaks, or its owner turns, this server can't tell.",
	"This server pins the key below now, and refuses the store if the key ever changes. It won't install anything from the store by itself: you choose each version, and each one replaces a module only after its own warning. " + forkLimits + " Encrypted media stays refused whatever a module says.",
	"Check the key's fingerprint with the store's owner, somewhere other than the store itself.",
}

// storeRecord is what is remembered about a store.
type storeRecord struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Key       string   `json:"key"` // pinned when it was added; base64
	Private   bool     `json:"private,omitempty"`
	Enabled   bool     `json:"enabled"`
	AddedAt   int64    `json:"addedAt"`
	AddedBy   string   `json:"addedBy,omitempty"`
	Serial    int64    `json:"serial"` // the highest seen
	Hash      string   `json:"hash,omitempty"`
	CheckedAt int64    `json:"checkedAt,omitempty"`
	Problem   *Failure `json:"problem,omitempty"`
}

type storeEntry struct {
	storeRecord
	cat *wittymodule.Catalog
}

// StoreInfo is one store, for the page.
type StoreInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Host        string   `json:"host"` // always shown beside the name, which the store chose itself
	URL         string   `json:"url"`
	Official    bool     `json:"official,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
	Private     bool     `json:"private,omitempty"`
	Enabled     bool     `json:"enabled"`
	AddedAt     int64    `json:"addedAt,omitempty"`
	AddedBy     string   `json:"addedBy,omitempty"`
	Serial      int64    `json:"serial,omitempty"`
	Generated   string   `json:"generated,omitempty"`
	CheckedAt   int64    `json:"checkedAt,omitempty"`
	Modules     int      `json:"modules"`
	Problem     *Failure `json:"problem,omitempty"`
	InUse       []string `json:"inUse,omitempty"` // names of modules using this store now
}

// PreviewModule is one module a store would bring.
type PreviewModule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Kind    string `json:"kind"`
	Why     string `json:"why,omitempty"`
}

// StorePreview is what an admin reads before agreeing to add a store.
type StorePreview struct {
	URL         string          `json:"url"`
	Host        string          `json:"host"`
	Name        string          `json:"name"`
	Serial      int64           `json:"serial"`
	Generated   string          `json:"generated"`
	Key         string          `json:"key"`
	Fingerprint string          `json:"fingerprint"`
	KeySource   string          `json:"keySource"` // "pasted", or "offered" when the store itself supplied it
	Private     bool            `json:"private,omitempty"`
	Modules     []PreviewModule `json:"modules"`
	Count       int             `json:"count"`
	Warning     []string        `json:"warning"`
}

// StoresConfig prepares Stores.
type StoresConfig struct {
	Manager      *Manager
	Dir          string            // <data>/modules
	Transport    http.RoundTripper // internet-only
	Log          *slog.Logger
	CheckEnabled func() bool
}

// Stores keeps the third-party stores and runs what the store pages ask for.
type Stores struct {
	cfg StoresConfig
	m   *Manager
	log *slog.Logger

	mu       sync.Mutex
	list     []*storeEntry
	checking bool

	// Watch, if set before anything runs, hears how each run stands: what the activity button
	// shows. It runs on the run's own goroutine, and must not block.
	Watch func(r *Run)

	runMu   sync.Mutex
	current *Run
	last    *Run
}

// NewStores reads the stores this server has. A kept catalogue is checked again, with the key
// that was pinned, since the folder may be a shared one.
func NewStores(cfg StoresConfig) *Stores {
	s := &Stores{cfg: cfg, m: cfg.Manager, log: cfg.Log}
	if cfg.Manager != nil {
		cfg.Manager.autoRun = s.AutoUpdate
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	s.log = s.log.With("cat", "modules")
	b, err := os.ReadFile(s.path())
	if err != nil {
		s.loadRun()
		return s
	}
	var recs []storeRecord
	if err := json.Unmarshal(b, &recs); err != nil {
		s.log.Warn("modules: the list of stores is unreadable", "err", err)
		s.loadRun()
		return s
	}
	for _, r := range recs {
		if _, err := parseSourceKey(r.Key); err != nil || !storeIDRe.MatchString(r.ID) {
			s.log.Warn("modules: ignoring a store in the saved list that can't be right", "store", r.ID)
			continue
		}
		e := &storeEntry{storeRecord: r}
		if c, err := s.readKept(e); err == nil {
			e.cat = c
		} else if !errors.Is(err, os.ErrNotExist) {
			s.log.Warn("modules: a store's kept catalogue is no good; the next check replaces it", "store", r.ID, "err", err)
		}
		s.list = append(s.list, e)
	}
	s.loadRun()
	for _, e := range s.list { // modules using a store get its versions back
		s.applyFork(e.ID)
	}
	return s
}

var storeIDRe = regexp.MustCompile(`^s[0-9a-f]{12}$`)

func storeID(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return "s" + hex.EncodeToString(h[:6])
}

func (s *Stores) path() string            { return filepath.Join(s.cfg.Dir, storesDir, storesFile) }
func (s *Stores) folder(id string) string { return filepath.Join(s.cfg.Dir, storesDir, id) }
func (s *Stores) fetcher(e *storeEntry) *storeFetcher {
	return newStoreFetcher(s.cfg.Transport, e.Private)
}

// save writes the list. Call with s.mu held.
func (s *Stores) save() error {
	recs := make([]storeRecord, 0, len(s.list))
	for _, e := range s.list {
		recs = append(recs, e.storeRecord)
	}
	b, err := json.MarshalIndent(recs, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.path(), b)
}

func (s *Stores) readKept(e *storeEntry) (*wittymodule.Catalog, error) {
	raw, err := os.ReadFile(filepath.Join(s.folder(e.ID), catalogFile))
	if err != nil {
		return nil, err
	}
	sig, err := os.ReadFile(filepath.Join(s.folder(e.ID), catalogSig))
	if err != nil {
		return nil, err
	}
	k, err := parseSourceKey(e.Key)
	if err != nil {
		return nil, err
	}
	c, err := wittymodule.VerifyCatalog(raw, sig, []ed25519.PublicKey{k})
	if err != nil {
		return nil, err
	}
	if c.Serial < e.Serial {
		return nil, fmt.Errorf("it is older than one already seen (%d, and %d was seen)", c.Serial, e.Serial)
	}
	return c, nil
}

// snapshot is a copy of a store as it is now, for reading without the lock.
func (s *Stores) snapshot(id string) *storeEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.find(id)
	if e == nil {
		return nil
	}
	c := *e
	return &c
}

// find looks a store up. Call with s.mu held.
func (s *Stores) find(id string) *storeEntry {
	for _, e := range s.list {
		if e.ID == id {
			return e
		}
	}
	return nil
}

// cleanText takes out what a page could be fooled by even as plain text: characters that
// reorder what's around them, invisible ones, and control codes above the ASCII range. The
// catalogue's own check has already refused the ones below it.
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.Is(unicode.Cf, r), unicode.Is(unicode.Cc, r), unicode.Is(unicode.Co, r):
			return -1
		}
		return r
	}, s)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func fingerprintOf(b64 string) string {
	k, err := parseSourceKey(b64)
	if err != nil {
		return ""
	}
	return Fingerprint(k)
}

// kindOf says how a store's module relates to this server, with the words for it when it can't
// be installed.
func (s *Stores) kindOf(id string, known map[string]bool) (string, string) {
	if why := noFork[id]; why != "" {
		return KindRefused, why
	}
	if known[id] {
		return KindFork, ""
	}
	return KindDev, "New kinds of module are still in development, so this server can't install one yet. It's shown so you can see what the store offers."
}

func (s *Stores) knownModules() map[string]bool {
	known := map[string]bool{}
	for _, st := range s.m.Report().Modules {
		// A part of the server has no releases of its own to replace.
		if st.Delivery != wittymodule.DeliveryBuiltin && st.Delivery != wittymodule.DeliveryLibrary {
			known[st.ID] = true
		}
	}
	return known
}

// Preview fetches a store and says what adding it would mean. Nothing is kept. key is what the
// admin pasted, which may be nothing: the store may offer one beside its catalogue, which is
// only a suggestion the admin has to agree to.
func (s *Stores) Preview(ctx context.Context, raw, key string, private bool) (*StorePreview, error) {
	u, err := checkStoreURL(raw, private)
	if err != nil {
		return nil, err
	}
	raw = u.String()
	f := newStoreFetcher(s.cfg.Transport, private)
	cat, sig, err := fetchCatalogFiles(ctx, f, raw)
	if err != nil {
		return nil, err
	}
	source := "pasted"
	pinned := strings.TrimSpace(key)
	if pinned != "" {
		if _, err := parseSourceKey(pinned); err != nil {
			return nil, err
		}
	} else {
		source = "offered"
		offered, err := f.small(ctx, raw+".key", maxStoreKeyFile)
		if err != nil {
			return nil, &NeedKey{}
		}
		pinned = strings.TrimSpace(string(offered))
		if _, err := parseSourceKey(pinned); err != nil {
			return nil, &NeedKey{}
		}
	}
	k, _ := parseSourceKey(pinned)
	c, err := wittymodule.VerifyCatalog(cat, sig, []ed25519.PublicKey{k})
	if err != nil {
		return nil, err
	}
	known := s.knownModules()
	p := &StorePreview{URL: raw, Host: u.Host, Name: cleanText(c.Name), Serial: c.Serial, Generated: c.Generated, Key: base64.StdEncoding.EncodeToString(k), Fingerprint: Fingerprint(k),
		KeySource: source, Private: private, Count: len(c.Modules), Warning: StoreWarning, Modules: []PreviewModule{}}
	for i := range c.Modules {
		mod := &c.Modules[i]
		kind, why := s.kindOf(mod.ID, known)
		if len(p.Modules) < 50 {
			p.Modules = append(p.Modules, PreviewModule{ID: mod.ID, Name: cleanText(mod.Name), Summary: cleanText(mod.Summary), Kind: kind, Why: why})
		}
	}
	return p, nil
}

// NeedKey: the store has no key beside it, or none that can be read, so the admin has to give it.
type NeedKey struct{}

func (*NeedKey) Error() string {
	return "This store doesn't offer a key to check it with. Ask its owner for their public key and paste it in."
}

// fetchCatalogFiles asks for a catalogue and its signature, with the limits in force. Neither
// is looked at here.
func fetchCatalogFiles(ctx context.Context, f *storeFetcher, raw string) (cat, sig []byte, err error) {
	cat, err = f.small(ctx, raw, wittymodule.MaxCatalog)
	if err != nil {
		return nil, nil, err
	}
	sig, err = f.small(ctx, raw+".sig", maxStoreSig)
	if err != nil {
		if errors.Is(err, errStoreNotFound) {
			return nil, nil, errors.New("this store's catalogue isn't signed: there's no signature beside it")
		}
		return nil, nil, fmt.Errorf("the signature: %w", err)
	}
	return cat, sig, nil
}

// Add keeps a store, pinning the key the admin agreed to. It fetches again rather than believing
// what was previewed, and refuses if the store's answer has changed in between.
func (s *Stores) Add(ctx context.Context, raw, key string, private bool, by string) (*StoreInfo, error) {
	u, err := checkStoreURL(raw, private)
	if err != nil {
		return nil, err
	}
	raw = u.String()
	k, err := parseSourceKey(key)
	if err != nil {
		return nil, err
	}
	if s.m != nil && s.m.cfg.CatalogURL == raw {
		return nil, errors.New("that is this project's own catalogue, which is already here")
	}
	id := storeID(raw)
	s.mu.Lock()
	dup := s.find(id) != nil
	full := len(s.list) >= maxStores
	s.mu.Unlock()
	if dup {
		return nil, errors.New("that store has already been added")
	}
	if full {
		return nil, fmt.Errorf("a server can have at most %d stores", maxStores)
	}
	f := newStoreFetcher(s.cfg.Transport, private)
	cat, sig, err := fetchCatalogFiles(ctx, f, raw)
	if err != nil {
		return nil, err
	}
	c, err := wittymodule.VerifyCatalog(cat, sig, []ed25519.PublicKey{k})
	if err != nil {
		return nil, err
	}
	e := &storeEntry{storeRecord: storeRecord{ID: id, URL: raw, Key: base64.StdEncoding.EncodeToString(k), Private: private, Enabled: true, AddedAt: time.Now().UnixMilli(), AddedBy: by,
		Serial: c.Serial, Hash: sum(cat), CheckedAt: time.Now().UnixMilli()}, cat: c}
	if err := s.keep(e, cat, sig); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.find(id) != nil {
		return nil, errors.New("that store has already been added")
	}
	s.list = append(s.list, e)
	if err := s.save(); err != nil {
		s.list = s.list[:len(s.list)-1]
		return nil, err
	}
	s.log.Warn("modules: a store was added", "store", id, "url", raw, "fingerprint", Fingerprint(k), "by", by)
	info := s.info(e, nil)
	return &info, nil
}

func (s *Stores) keep(e *storeEntry, cat, sig []byte) error {
	if err := writeAtomic(filepath.Join(s.folder(e.ID), catalogFile), cat); err != nil {
		return err
	}
	return writeAtomic(filepath.Join(s.folder(e.ID), catalogSig), sig)
}

// Refresh brings one store up to date. A catalogue that fails any check changes nothing: the
// last good one stays, and the store says why.
func (s *Stores) Refresh(ctx context.Context, id string) error {
	s.mu.Lock()
	e := s.find(id)
	if e == nil {
		s.mu.Unlock()
		return ErrUnknownStore
	}
	rec := e.storeRecord
	s.mu.Unlock()

	problem, c, cat, sig := s.look(ctx, &rec)

	s.mu.Lock()
	e = s.find(id)
	if e == nil {
		s.mu.Unlock()
		return ErrUnknownStore
	}
	e.CheckedAt = time.Now().UnixMilli()
	e.Problem = problem
	if problem == nil && c != nil && sum(cat) != e.Hash {
		if err := s.keep(e, cat, sig); err != nil {
			problem = explainErr("this store", err)
			e.Problem = problem
		} else {
			e.cat, e.Serial, e.Hash = c, c.Serial, sum(cat)
		}
	}
	if err := s.save(); err != nil {
		s.log.Warn("modules: saving the list of stores", "err", err)
	}
	s.mu.Unlock()
	if problem != nil {
		s.log.Warn("modules: a store's check failed", "store", id, "kind", problem.Kind, "detail", problem.Detail)
		return errors.New(problem.Title)
	}
	s.applyFork(id)
	return nil
}

// ErrUnknownStore: no store has that id.
var ErrUnknownStore = errors.New("there is no such store")

// look fetches and checks a store's catalogue. The checks run in this order so that nothing
// from the file is read before its signature passes against the pinned key.
func (s *Stores) look(ctx context.Context, rec *storeRecord) (*Failure, *wittymodule.Catalog, []byte, []byte) {
	f := newStoreFetcher(s.cfg.Transport, rec.Private)
	name := "This store"
	cat, sig, err := fetchCatalogFiles(ctx, f, rec.URL)
	if err != nil {
		return explainErr(name, err), nil, nil, nil
	}
	k, err := parseSourceKey(rec.Key)
	if err != nil {
		return explainErr(name, err), nil, nil, nil
	}
	c, err := wittymodule.VerifyCatalog(cat, sig, []ed25519.PublicKey{k})
	if errors.Is(err, wittymodule.ErrCatalogSignature) {
		// The pinned key doesn't sign this. If the store now offers a different key that does,
		// say so as loudly as possible, and never take that key.
		if offered, oerr := f.small(ctx, rec.URL+".key", maxStoreKeyFile); oerr == nil {
			if nk, kerr := parseSourceKey(strings.TrimSpace(string(offered))); kerr == nil && !nk.Equal(k) {
				if _, verr := wittymodule.VerifyCatalog(cat, sig, []ed25519.PublicKey{nk}); verr == nil {
					return explainErr(name, fmt.Errorf("the signing key has changed: you agreed to %s, and it now offers %s", Fingerprint(k), Fingerprint(nk))), nil, nil, nil
				}
			}
		}
		return explainErr(name, fmt.Errorf("the signing key has changed, or the file was altered: it doesn't check out with the key you agreed to, %s", Fingerprint(k))), nil, nil, nil
	}
	if err != nil {
		return explainErr(name, err), nil, nil, nil
	}
	switch {
	case c.Serial < rec.Serial:
		return &Failure{Kind: FailSignature, Title: "This store went backwards", Detail: fmt.Sprintf("serial %d, and %d was seen", c.Serial, rec.Serial),
			Message: "It sent an older list than the one this server already has. That can mean someone is holding back updates, so the older list was ignored.",
			Try:     []string{"Wait, and check again later.", "If it stays like this, ask the store's owner."}}, nil, nil, nil
	case c.Serial == rec.Serial && rec.Hash != "" && sum(cat) != rec.Hash:
		return &Failure{Kind: FailSignature, Title: "This store changed its list without saying so", Detail: fmt.Sprintf("serial %d, with different contents", c.Serial),
			Message: "It sent a list with the same number as the last one and different contents. A store has to raise that number for every change, so this list was ignored.",
			Try:     []string{"Ask the store's owner."}}, nil, nil, nil
	}
	return nil, c, cat, sig
}

// CheckAll brings every store that's on up to date, one at a time, and says what went wrong.
func (s *Stores) CheckAll(ctx context.Context) []string {
	s.mu.Lock()
	if s.checking {
		s.mu.Unlock()
		return nil
	}
	s.checking = true
	var ids []string
	for _, e := range s.list {
		if e.Enabled {
			ids = append(ids, e.ID)
		}
	}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.checking = false; s.mu.Unlock() }()
	var problems []string
	for _, id := range ids {
		if err := s.Refresh(ctx, id); err != nil {
			problems = append(problems, err.Error())
		}
	}
	return problems
}

// Run looks at the stores after a pause at start and then twice a day, while the owner's
// settings allow looking for updates. A server with no stores asks nobody anything.
func (s *Stores) Run(ctx context.Context) {
	wait := firstCheck
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 12 * time.Hour
		if s.cfg.CheckEnabled == nil || !s.cfg.CheckEnabled() {
			continue
		}
		s.CheckAll(ctx)
	}
}

// SetEnabled turns a store on or off. Off, it isn't fetched or shown in the store, and nothing
// new can be installed from it. What's already installed from it stays, labelled, until it's
// removed or put back to the official version.
func (s *Stores) SetEnabled(id string, on bool, by string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.find(id)
	if e == nil {
		return ErrUnknownStore
	}
	old := e.Enabled
	e.Enabled = on
	if err := s.save(); err != nil {
		e.Enabled = old
		return err
	}
	s.log.Warn("modules: a store was turned on or off", "store", id, "on", on, "by", by)
	return nil
}

// Remove forgets a store and what was kept of it. Any module using it goes back to the official
// version first, so nothing keeps running on trust nobody holds any more.
func (s *Stores) Remove(id, by string) error {
	e := s.snapshot(id)
	if e == nil {
		return ErrUnknownStore
	}
	for _, mid := range s.usersOf(id) {
		if err := s.m.ClearSource(mid); err != nil {
			return fmt.Errorf("%s couldn't go back to the official version, so the store stays: %w", mid, err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.list {
		if x.ID == id {
			s.list = append(s.list[:i:i], s.list[i+1:]...)
			break
		}
	}
	if err := s.save(); err != nil {
		return err
	}
	_ = os.RemoveAll(s.folder(id))
	s.log.Warn("modules: a store was removed", "store", id, "url", e.URL, "by", by)
	return nil
}

// usersOf names the modules whose releases come from a store now.
func (s *Stores) usersOf(id string) []string {
	var out []string
	for _, st := range s.m.Report().Modules {
		if st.Source != nil && st.Source.Store == id {
			out = append(out, st.ID)
		}
	}
	return out
}

// info describes a store. reports is the current report's modules, or nil to ask for it. Call
// with s.mu held.
func (s *Stores) info(e *storeEntry, inUse map[string][]string) StoreInfo {
	i := StoreInfo{ID: e.ID, Host: hostOf(e.URL), URL: e.URL, Fingerprint: fingerprintOf(e.Key), Private: e.Private, Enabled: e.Enabled, AddedAt: e.AddedAt, AddedBy: e.AddedBy,
		Serial: e.Serial, CheckedAt: e.CheckedAt, Problem: e.Problem, InUse: inUse[e.ID]}
	i.Name = i.Host
	if e.cat != nil {
		i.Name, i.Generated, i.Modules = cleanText(e.cat.Name), e.cat.Generated, len(e.cat.Modules)
	}
	return i
}

// List describes every third-party store, in the order they were added.
func (s *Stores) List() []StoreInfo {
	inUse := map[string][]string{}
	for _, st := range s.m.Report().Modules {
		if st.Source != nil && st.Source.Store != "" {
			inUse[st.Source.Store] = append(inUse[st.Source.Store], st.Name)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]StoreInfo, 0, len(s.list))
	for _, e := range s.list {
		out = append(out, s.info(e, inUse))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AddedAt < out[j].AddedAt })
	return out
}
