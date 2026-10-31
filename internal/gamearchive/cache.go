package gamearchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wittywilla/wittyplay"
)

// The cache: games unpacked to play, kept so the next start is immediate. The oldest
// played go first when it's full. A game's folder holds nothing but the game, and
// appears under its final name in one rename, after every file is on disk. Anything
// a crash leaves behind is under a temporary name, and is never used.

const (
	stateReady     = "ready"
	stateWaiting   = "waiting"
	stateReading   = "reading"
	stateUnpacking = "unpacking"
	stateFailed    = "failed"

	manifestName = "manifest.json"
	gameDirName  = "game"
	tmpPrefix    = ".tmp-"

	// maxJobs: unpacking is the heaviest thing a low-end server does for a game, and
	// any signed-in account can ask for it. More wait their turn.
	maxJobs = 2
	// failureMemory: pollers need to be able to read an error, and pressing Play
	// again shouldn't unpack a bad archive again straight away.
	failureMemory = 30 * time.Second
	// freeReserve is left free on the disk, whatever the cache's limit.
	freeReserve = int64(512) << 20
)

// abandonAfter: nobody has asked about an unpack for this long, so stop it.
var abandonAfter = 2 * time.Minute

// Status says how preparing a game is going.
type Status struct {
	State string `json:"state"` // ready, waiting, reading, unpacking or failed
	Done  int64  `json:"done"`  // bytes written so far
	Total int64  `json:"total"` // bytes to write, as far as the archive says
	Error string `json:"error,omitempty"`
}

type manifestFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type manifest struct {
	Source string         `json:"source"`
	Entry  string         `json:"entry"`
	Size   int64          `json:"size"`
	Files  []manifestFile `json:"files"`
}

// Cache unpacks games on demand. Limit is its size in bytes, asked for each time so
// that a setting takes effect at once.
type Cache struct {
	Dir   string
	Limit func() int64

	mu     sync.Mutex
	jobs   map[string]*job
	failed map[string]failure
	leases map[string]int
	slots  chan struct{}
	once   sync.Once
	closed bool
	stop   context.CancelFunc
	ctx    context.Context
	builds atomic.Int64 // how many unpacks have really run
}

type failure struct {
	err error
	at  time.Time
}

type job struct {
	key     string
	path    string
	console *wittyplay.Console
	done    chan struct{}
	err     error

	mu       sync.Mutex
	state    string
	written  int64
	total    int64
	waiters  int
	lastSeen time.Time
}

func (j *job) status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Status{State: j.state, Done: j.written, Total: j.total}
}

func (j *job) set(f func()) {
	j.mu.Lock()
	defer j.mu.Unlock()
	f()
}

// init clears what a crash left behind. No job of this process can be running yet.
func (c *Cache) init() {
	c.once.Do(func() {
		c.jobs, c.failed, c.leases = map[string]*job{}, map[string]failure{}, map[string]int{}
		c.slots = make(chan struct{}, maxJobs)
		c.ctx, c.stop = context.WithCancel(context.Background())
		_ = os.MkdirAll(c.Dir, 0o700)
		entries, _ := os.ReadDir(c.Dir)
		for _, e := range entries {
			_, err := os.Stat(filepath.Join(c.Dir, e.Name(), manifestName))
			if strings.HasPrefix(e.Name(), tmpPrefix) || err != nil {
				_ = os.RemoveAll(filepath.Join(c.Dir, e.Name()))
			}
		}
	})
}

// Close stops what's unpacking. What's finished stays for next time.
func (c *Cache) Close() {
	c.init()
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.stop()
}

func keyOf(p string, console *wittyplay.Console) (string, error) {
	st, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(p + "\x00" + strconv.FormatInt(st.Size(), 10) + "\x00" + strconv.FormatInt(st.ModTime().UnixNano(), 10) + "\x00" + console.ID))
	return hex.EncodeToString(h[:16]), nil
}

// Prepare starts unpacking if it isn't running and isn't done, and says how it's
// going. It never waits, so it's what a client polls.
func (c *Cache) Prepare(p string, console *wittyplay.Console) Status {
	c.init()
	key, err := keyOf(p, console)
	if err != nil {
		return Status{State: stateFailed, Error: "the game file is missing"}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if j := c.jobs[key]; j != nil {
		j.set(func() { j.lastSeen = time.Now() })
		return j.status()
	}
	if f, ok := c.failed[key]; ok && time.Since(f.at) < failureMemory {
		return Status{State: stateFailed, Error: f.err.Error()}
	}
	if m := c.ready(key); m != nil {
		return Status{State: stateReady, Done: m.Size, Total: m.Size}
	}
	if c.closed {
		return Status{State: stateFailed, Error: "the server is stopping"}
	}
	return c.start(key, p, console).status()
}

// Acquire waits until the game is unpacked and gives the file a core should open.
// Until release is called, the game can't be evicted. Giving up on ctx doesn't stop
// the unpack, since someone else may be waiting for it, or may come back.
func (c *Cache) Acquire(ctx context.Context, p string, console *wittyplay.Console) (rom string, release func(), err error) {
	c.init()
	key, err := keyOf(p, console)
	if err != nil {
		return "", nil, errors.New("the game file is missing")
	}
	// A game can be evicted just after it finishes, if the cache is that busy.
	for range 3 {
		c.mu.Lock()
		if m := c.ready(key); m != nil {
			c.leases[key]++
			c.mu.Unlock()
			now := time.Now()
			_ = os.Chtimes(filepath.Join(c.Dir, key, manifestName), now, now)
			var once sync.Once
			return filepath.Join(c.Dir, key, gameDirName, filepath.FromSlash(m.Entry)), func() { once.Do(func() { c.release(key) }) }, nil
		}
		j := c.jobs[key]
		if j == nil {
			if f, ok := c.failed[key]; ok && time.Since(f.at) < failureMemory {
				c.mu.Unlock()
				return "", nil, f.err
			}
			if c.closed {
				c.mu.Unlock()
				return "", nil, errors.New("the server is stopping")
			}
			j = c.start(key, p, console)
		}
		j.set(func() { j.waiters++ })
		c.mu.Unlock()
		select {
		case <-j.done:
			j.set(func() { j.waiters-- })
			if j.err != nil {
				return "", nil, j.err
			}
		case <-ctx.Done():
			j.set(func() { j.waiters--; j.lastSeen = time.Now() })
			return "", nil, ctx.Err()
		}
	}
	return "", nil, errors.New("the unpacked games folder is too busy; try again")
}

func (c *Cache) release(key string) {
	c.mu.Lock()
	c.leases[key]--
	if c.leases[key] <= 0 {
		delete(c.leases, key)
	}
	c.mu.Unlock()
	c.makeRoom(0, "")
}

// ready reads a finished game's manifest and checks every file is still there. With
// the lock held. A folder that fails is removed, and unpacked again.
func (c *Cache) ready(key string) *manifest {
	dir := filepath.Join(c.Dir, key)
	b, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		if c.leases[key] == 0 {
			_ = os.RemoveAll(dir) // an eviction a crash cut short
		}
		return nil
	}
	var m manifest
	ok := json.Unmarshal(b, &m) == nil && m.Entry != ""
	if ok {
		for _, f := range m.Files {
			st, err := os.Lstat(filepath.Join(dir, gameDirName, filepath.FromSlash(f.Name)))
			if err != nil || !st.Mode().IsRegular() || st.Size() != f.Size {
				ok = false
				break
			}
		}
	}
	if !ok {
		if c.leases[key] == 0 {
			_ = os.RemoveAll(dir)
		}
		return nil
	}
	return &m
}

// start begins a job. With the lock held.
func (c *Cache) start(key, p string, console *wittyplay.Console) *job {
	delete(c.failed, key)
	j := &job{key: key, path: p, console: console, done: make(chan struct{}), state: stateWaiting, lastSeen: time.Now()}
	c.jobs[key] = j
	go c.run(j)
	return j
}

func (c *Cache) run(j *job) {
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()
	watch := make(chan struct{})
	defer close(watch)
	go func() {
		t := time.NewTicker(abandonAfter / 8)
		defer t.Stop()
		for {
			select {
			case <-watch:
				return
			case <-t.C:
				j.mu.Lock()
				gone := j.waiters == 0 && time.Since(j.lastSeen) > abandonAfter
				j.mu.Unlock()
				if gone {
					cancel()
					return
				}
			}
		}
	}()
	err := func() error {
		select {
		case c.slots <- struct{}{}:
			defer func() { <-c.slots }()
		case <-ctx.Done():
			return ctx.Err()
		}
		return c.build(ctx, j)
	}()
	c.mu.Lock()
	delete(c.jobs, j.key)
	if err != nil && !errors.Is(err, context.Canceled) {
		c.failed[j.key] = failure{err: err, at: time.Now()}
	}
	for k, f := range c.failed {
		if time.Since(f.at) > failureMemory {
			delete(c.failed, k)
		}
	}
	c.mu.Unlock()
	j.err = err
	if errors.Is(err, context.Canceled) {
		j.err = errors.New("unpacking this game was stopped")
	}
	close(j.done)
}

func (c *Cache) build(ctx context.Context, j *job) error {
	c.builds.Add(1)
	j.set(func() { j.state = stateReading })
	l, err := List(j.path)
	if err != nil {
		return err
	}
	plan, err := Choose(l, j.console, filepath.Base(j.path))
	if errors.Is(err, ErrNoGame) {
		return errors.New("this archive holds no game for " + j.console.Name)
	}
	if err != nil {
		return err
	}
	limit := c.Limit()
	if plan.Size > limit {
		return fmt.Errorf("this game needs %.1f GB unpacked, and the space for unpacked games is %.1f GB. An admin can raise that in Settings, Gaming", gib(plan.Size), gib(limit))
	}
	c.makeRoom(plan.Size, j.key)
	if free := freeBytes(c.Dir); free >= 0 && free-freeReserve < plan.Size {
		return ErrNoSpace
	}
	tmp, err := os.MkdirTemp(c.Dir, tmpPrefix+j.key+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp) // nothing to remove once it's been renamed
	game := filepath.Join(tmp, gameDirName)
	if err := os.Mkdir(game, 0o700); err != nil {
		return err
	}
	j.set(func() { j.state, j.total = stateUnpacking, plan.Size })
	written, err := unpackIn(ctx, unpackRequest{Path: j.path, Dst: game, Want: plan.Want, Limit: limit}, func(n int64) {
		j.set(func() { j.written = n })
	})
	if err != nil {
		return err
	}
	return c.finish(j, plan, tmp, game, written)
}

// finish checks what arrived, makes the game ready to open and puts it in place.
func (c *Cache) finish(j *job, plan *Plan, tmp, game string, written []Written) error {
	if len(written) != len(plan.Want) {
		return ErrDamaged
	}
	var names []string
	var size int64
	for _, w := range written {
		if n, err := cleanName(w.Out); err != nil || n != w.Out {
			return ErrUnsafe
		}
		st, err := os.Lstat(filepath.Join(game, filepath.FromSlash(w.Out)))
		if err != nil || !st.Mode().IsRegular() {
			return ErrDamaged
		}
		names = append(names, w.Out)
		size += st.Size()
	}
	if plan.Discs != nil {
		if err := writePlaylist(game, plan.Entry, plan.Discs); err != nil {
			return err
		}
		names = append(names, plan.Entry)
	}
	if err := checkReferences(game, plan.Entry, names); err != nil {
		return err
	}
	m := manifest{Source: j.path, Entry: plan.Entry}
	for _, n := range names {
		st, err := os.Lstat(filepath.Join(game, filepath.FromSlash(n)))
		if err != nil {
			return err
		}
		m.Files = append(m.Files, manifestFile{Name: n, Size: st.Size()})
	}
	m.Size = size
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := writeSynced(filepath.Join(tmp, manifestName), b); err != nil {
		return err
	}
	if err := renameRetry(tmp, filepath.Join(c.Dir, j.key)); err != nil {
		return err
	}
	j.set(func() { j.state, j.written = stateReady, size })
	return nil
}

func writeSynced(p string, b []byte) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// renameRetry: on Windows a virus scanner often has a new file open for a moment.
func renameRetry(from, to string) error {
	var err error
	for i := range 20 {
		if err = os.Rename(from, to); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 50 * time.Millisecond)
	}
	return err
}

// makeRoom evicts the longest-unplayed games until need more bytes fit under the
// limit. A game being played stays, even if that leaves the cache over for a while.
func (c *Cache) makeRoom(need int64, keep string) {
	limit := c.Limit()
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return
	}
	type folder struct {
		key  string
		size int64
		used time.Time
	}
	var all []folder
	var total int64
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), tmpPrefix) {
			continue
		}
		p := filepath.Join(c.Dir, e.Name(), manifestName)
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		var m manifest
		if b, err := os.ReadFile(p); err != nil || json.Unmarshal(b, &m) != nil {
			continue
		}
		all = append(all, folder{e.Name(), m.Size, st.ModTime()})
		total += m.Size
	}
	if total+need <= limit {
		return
	}
	sort.Slice(all, func(i, j int) bool { return all[i].used.Before(all[j].used) })
	for _, f := range all {
		if total+need <= limit {
			return
		}
		c.mu.Lock()
		// Taken out of use under the lock, so ready can't hand it to anyone from here.
		busy := f.key == keep || c.leases[f.key] > 0 || c.jobs[f.key] != nil
		if !busy {
			_ = os.Remove(filepath.Join(c.Dir, f.key, manifestName))
		}
		c.mu.Unlock()
		if !busy {
			_ = os.RemoveAll(filepath.Join(c.Dir, f.key))
			total -= f.size
		}
	}
}

// Used is how much the unpacked games hold, for the settings page.
func (c *Cache) Used() int64 {
	c.init()
	var total int64
	_ = filepath.WalkDir(c.Dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if st, err := d.Info(); err == nil {
				total += st.Size()
			}
		}
		return nil
	})
	return total
}

func gib(n int64) float64 { return float64(n) / (1 << 30) }
