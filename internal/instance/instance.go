// Package instance keeps to one server per data folder. The first start takes a lock
// that the system lets go of when the process ends, however it ends, so a crash never
// leaves anything behind to block the next start. A second start finds the first one
// through instance.json and its health check, and says so or asks what to do.
package instance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// LockFile is held by the running server for as long as it runs. It's never removed:
	// removing a lock file is how two processes end up each holding one.
	LockFile = "instance.lock"
	// InfoFile says where the running server is. No secrets: it's only how a second
	// start finds the first.
	InfoFile = "instance.json"
	// ExitRunning is the exit code of a start that found its data folder in use. Service
	// managers are told not to restart on it.
	ExitRunning = 3
	// FirstOtherPort is where the search for a second server's port starts.
	FirstOtherPort = 9798
)

var (
	// ErrBusy: another process holds the data folder.
	ErrBusy = errors.New("another server is using this data folder")
	// ErrUnsupported: the folder's file system can't lock (some network shares and
	// container file sharing). The server starts anyway, unguarded.
	ErrUnsupported = errors.New("this folder can't be locked")
)

// Info is what instance.json holds.
type Info struct {
	PID      int       `json:"pid"`
	Started  time.Time `json:"started"`
	Version  string    `json:"version"`
	URLs     []string  `json:"urls"`
	Instance string    `json:"instance"`
	// Service: started by a service manager, which the tray app attaches to rather than asking.
	Service bool `json:"service,omitempty"`
}

// Lock is this process's hold on a data folder.
type Lock struct {
	dir string
	id  string
	f   *os.File
}

// Acquire takes the data folder for this process. ErrBusy means another one has it.
func Acquire(dir string) (*Lock, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, LockFile), os.O_RDWR|os.O_CREATE, 0o640)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{dir: dir, id: newID(), f: f}, nil
}

// AcquireWait is Acquire for a start that follows a restart: the process before this
// one may still be closing, so a busy folder gets up to wait.
func AcquireWait(dir string, wait time.Duration) (*Lock, error) {
	deadline := time.Now().Add(wait)
	for {
		l, err := Acquire(dir)
		if !errors.Is(err, ErrBusy) || time.Now().After(deadline) {
			return l, err
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// Held says another process holds dir, for tools that mustn't change its files under
// a running server.
func Held(dir string) bool {
	f, err := os.OpenFile(filepath.Join(dir, LockFile), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return errors.Is(err, ErrBusy)
	}
	unlockFile(f)
	return false
}

// ID is this run's own random id, which /api/health gives out so that a second start
// knows it's talking to this server and not some other program on the port.
func (l *Lock) ID() string {
	if l == nil {
		return ""
	}
	return l.id
}

// Publish writes instance.json once the server is listening. Its Instance is this lock's.
func (l *Lock) Publish(info Info) error {
	if l == nil {
		return nil
	}
	info.Instance = l.id
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	p := filepath.Join(l.dir, InfoFile)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	for i := 0; ; i++ {
		err = os.Rename(tmp, p)
		if err == nil || i == 10 {
			break
		}
		time.Sleep(50 * time.Millisecond) // a second start reading it at this moment, on Windows
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// Release lets go of the folder, at the very end of a run.
func (l *Lock) Release() {
	if l == nil || l.f == nil {
		return
	}
	if info, err := Read(l.dir); err == nil && info.Instance == l.id {
		_ = os.Remove(filepath.Join(l.dir, InfoFile))
	}
	unlockFile(l.f)
	_ = l.f.Close()
	l.f = nil
}

// Read reads instance.json.
func Read(dir string) (Info, error) {
	var info Info
	b, err := os.ReadFile(filepath.Join(dir, InfoFile))
	if err != nil {
		return info, err
	}
	if len(b) > 64<<10 {
		return info, errors.New("instance.json is too large")
	}
	err = json.Unmarshal(b, &info)
	return info, err
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Running is what a second start found.
type Running struct {
	Info
	// URL answered the health check with this server's id.
	URL string
	// Answering: it did, within the time given. Otherwise it's starting, or hung.
	Answering bool
}

// Find looks for the server holding dir, for up to wait. It returns what it knows even
// when that server doesn't answer.
func Find(ctx context.Context, dir string, wait time.Duration) Running {
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	var r Running
	for {
		if info, err := Read(dir); err == nil {
			r.Info = info
			for _, u := range info.URLs {
				if id, _ := HealthID(ctx, u); id != "" && id == info.Instance {
					r.URL, r.Answering = u, true
					return r
				}
			}
		}
		select {
		case <-ctx.Done():
			return r
		case <-time.After(300 * time.Millisecond):
		}
	}
}

var client = &http.Client{
	Timeout:   time.Second,
	Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true},
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// HealthID asks a server at base for its instance id. A WittyMediaServer older than this
// answers with an empty one; ok says something answered as a WittyMediaServer does.
func HealthID(ctx context.Context, base string) (id string, ok bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(base, "/")+"/api/health", nil)
	if err != nil {
		return "", false
	}
	res, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", false
	}
	var h struct {
		OK       bool   `json:"ok"`
		Instance string `json:"instance"`
	}
	dec := json.NewDecoder(io.LimitReader(res.Body, 4<<10))
	if err := dec.Decode(&h); err != nil || !h.OK {
		return "", false
	}
	return h.Instance, true
}

// Line is the one line a start without a desktop prints before it exits.
func Line(r Running, dir string) string {
	switch {
	case r.Answering:
		return fmt.Sprintf("WittyMediaServer is already running at %s (process %d), using the data folder %s.", r.URL, r.PID, dir)
	case r.PID != 0:
		return fmt.Sprintf("WittyMediaServer is starting or not answering (process %d), using the data folder %s. Try again in a moment.", r.PID, dir)
	}
	return fmt.Sprintf("Another WittyMediaServer is using the data folder %s and hasn't said where it is yet. Try again in a moment.", dir)
}

// FreePort finds the first port from `from` that host can listen on, skipping those in
// skip. It returns 0 when none of the next hundred are free.
func FreePort(host string, from int, skip ...int) int {
next:
	for p := from; p < from+100 && p < 65536; p++ {
		for _, s := range skip {
			if p == s {
				continue next
			}
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return p
	}
	return 0
}

// SuggestFolder is the folder offered for a second server: beside dir, named after it,
// and not in use by a server already.
func SuggestFolder(dir string) string {
	dir = filepath.Clean(dir)
	for i := 2; i < 100; i++ {
		c := fmt.Sprintf("%s-%d", dir, i)
		if !Held(c) {
			return c
		}
	}
	return dir + "-new"
}

// SameFolder says two paths are one folder.
func SameFolder(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	sa, ea := os.Stat(a)
	sb, eb := os.Stat(b)
	if ea == nil && eb == nil {
		return os.SameFile(sa, sb)
	}
	aa, _ := filepath.Abs(a)
	ab, _ := filepath.Abs(b)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(aa, ab)
	}
	return aa == ab
}
