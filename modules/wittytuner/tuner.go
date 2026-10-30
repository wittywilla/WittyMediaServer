// Package wittytuner is a common way to reach TV tuners and IPTV: network
// tuners, M3U playlists and single streams, Linux DVB, V4L2 capture, Windows
// and macOS capture devices, SAT>IP, external backends over open protocols and
// a custom command.
//
// A driver lists channels and opens one as either raw MPEG-TS or FFmpeg input
// options. Encrypted and copy-protected channels are detected and refused:
// nothing here decrypts or bypasses anything.
package wittytuner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittytuner/mpegts"
)

// Channel is one channel a source offers.
type Channel struct {
	ID       string `json:"id"` // stable within the source
	Number   string `json:"number"`
	Name     string `json:"name"`
	Group    string `json:"group,omitempty"`
	CallSign string `json:"callsign,omitempty"`
	Locator  string `json:"locator,omitempty"` // how the driver finds it again
	Logo     string `json:"logo,omitempty"`    // an address given by the source
	EPGID    string `json:"epgId,omitempty"`   // XMLTV channel id
	HD       bool   `json:"hd,omitempty"`
	Radio    bool   `json:"radio,omitempty"`
	Program  int    `json:"program,omitempty"` // MPEG program number
	// Encrypted is why this channel cannot be tuned ("" = it can).
	Encrypted string `json:"encrypted,omitempty"`
}

// Stream is an open channel: Reader for raw MPEG-TS, else Input for FFmpeg.
type Stream struct {
	Reader io.ReadCloser
	Input  []string
	// OnEvent, when set by the host before reading, gets guide data found in
	// the stream.
	Guard *mpegts.Guard
}

// Close releases what the stream holds.
func (s *Stream) Close() error {
	if s.Reader != nil {
		return s.Reader.Close()
	}
	return nil
}

// Info describes a configured source.
type Info struct {
	Driver     string
	Name       string
	MaxStreams int    // how many channels it can stream at once; 0 = no known limit
	Detail     string // model, address: shown in Settings
	GuideURL   string // an XMLTV address the source offers
}

// Progress is how far a scan has got.
type Progress struct {
	Percent int    `json:"percent"`
	Found   int    `json:"found"`
	Message string `json:"message,omitempty"`
}

// ScanOptions tune a scan.
type ScanOptions struct {
	Source string // Antenna | Cable: for tuners that ask
}

// Source is one configured tuner.
type Source interface {
	Info() Info
	// Channels is what the source knows now, without tuning anything.
	Channels(ctx context.Context) ([]Channel, error)
	// Scan looks for channels. It may take minutes, and is cancelled by ctx.
	Scan(ctx context.Context, o ScanOptions, progress func(Progress)) ([]Channel, error)
	// Open starts a channel. ErrBusy when no tuner is free, *EncryptedError
	// when the channel is encrypted.
	Open(ctx context.Context, ch Channel) (*Stream, error)
	Close() error
}

// ErrBusy: every tuner of the source is in use.
var ErrBusy = errors.New("all of this device's tuners are in use")

// EncryptedError says a channel was refused because it is encrypted or copy
// protected. It is never decrypted.
type EncryptedError struct {
	Channel string
	Why     string
}

func (e *EncryptedError) Error() string {
	name := e.Channel
	if name == "" {
		name = "This channel"
	}
	return fmt.Sprintf("%s is encrypted or copy protected (%s). WittyMediaServer does not decrypt or bypass protected broadcasts, so it cannot be played or recorded.", name, e.Why)
}

// IsEncrypted reports whether err means a channel was refused for encryption.
func IsEncrypted(err error) bool {
	var e *EncryptedError
	return errors.As(err, &e) || errors.Is(err, mpegts.ErrScrambled)
}

// RefusalMessage turns a stream that a guard stopped into words for a person.
func RefusalMessage(name string, err error) error {
	if errors.Is(err, mpegts.ErrScrambled) {
		return &EncryptedError{Channel: name, Why: "its packets are scrambled"}
	}
	return err
}

// Env is what a driver gets from its host.
type Env struct {
	HTTP *http.Client // for short requests
	// Stream is for long-lived streams: it must have no overall timeout.
	Stream *http.Client
	FFmpeg string
	// Command prepares a process (the host hides console windows and so on).
	Command func(ctx context.Context, name string, args ...string) *exec.Cmd
	// AllowCommands: the owner has allowed the custom command driver.
	AllowCommands bool
}

func (e Env) http() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (e Env) stream() *http.Client {
	if e.Stream != nil {
		return e.Stream
	}
	return &http.Client{}
}

func (e Env) command(ctx context.Context, name string, args ...string) *exec.Cmd {
	if e.Command != nil {
		return e.Command(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...)
}

// Config is one tuner's settings.
type Config struct {
	Name    string
	Options map[string]string
}

// Field describes one setting of a driver; forms are drawn from these.
type Field struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // text | url | path | password | select | number | textarea | toggle
	Help     string   `json:"help,omitempty"`
	Default  string   `json:"default,omitempty"`
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"`
}

// Driver describes one kind of tuner.
type Driver struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Status is how well this has been tested: "tested", "mock" (against mock
	// servers, no real device) or "untested".
	Status string `json:"status"`
	// Platforms it can run on; empty = all.
	Platforms []string `json:"platforms,omitempty"`
	Fields    []Field  `json:"fields"`
	Scans     bool     `json:"scans"` // Scan does more than list what is known
	// Planned drivers are listed so people can see they are known about.
	Planned bool `json:"planned,omitempty"`
	// Available is worked out for this machine.
	Available bool   `json:"available"`
	Note      string `json:"note,omitempty"`
	build     func(Config, Env) (Source, error)
}

var (
	regMu   sync.Mutex
	drivers []Driver
)

func register(d Driver) {
	regMu.Lock()
	defer regMu.Unlock()
	drivers = append(drivers, d)
}

// Drivers lists every kind of tuner, marked for what works on this machine.
func Drivers(env Env) []Driver {
	regMu.Lock()
	defer regMu.Unlock()
	out := slices.Clone(drivers)
	for i := range out {
		d := &out[i]
		d.Available = !d.Planned && (len(d.Platforms) == 0 || slices.Contains(d.Platforms, runtime.GOOS))
		if d.ID == "command" && !env.AllowCommands {
			d.Available = false
			d.Note = "Runs a program on the server, so it is off until the server is started with WMS_TUNER_COMMANDS=1."
		}
		if !d.Available && d.Note == "" {
			d.Note = "Needs " + strings.Join(d.Platforms, " or ") + "."
		}
	}
	slices.SortStableFunc(out, func(a, b Driver) int { return rank(a) - rank(b) })
	return out
}

func rank(d Driver) int {
	switch {
	case d.Planned:
		return 2
	case !d.Available:
		return 1
	}
	return 0
}

// New makes a source from its settings. Required settings are checked here.
func New(driver string, cfg Config, env Env) (Source, error) {
	for _, d := range Drivers(env) {
		if d.ID != driver {
			continue
		}
		if !d.Available {
			return nil, fmt.Errorf("%s is not available here: %s", d.Name, d.Note)
		}
		if cfg.Options == nil {
			cfg.Options = map[string]string{}
		}
		for _, f := range d.Fields {
			if f.Required && strings.TrimSpace(cfg.Options[f.Key]) == "" {
				return nil, fmt.Errorf("%s: fill in %s", d.Name, f.Label)
			}
			if cfg.Options[f.Key] == "" && f.Default != "" {
				cfg.Options[f.Key] = f.Default
			}
		}
		if len(cfg.Name) > 200 {
			return nil, errors.New("the name is too long")
		}
		return d.build(cfg, env)
	}
	return nil, errors.New("unknown kind of tuner")
}

// SecretFields lists settings a driver treats as passwords, never sent back to
// a browser.
func SecretFields(driver string) []string {
	regMu.Lock()
	defer regMu.Unlock()
	var out []string
	for _, d := range drivers {
		if d.ID == driver {
			for _, f := range d.Fields {
				if f.Kind == "password" {
					out = append(out, f.Key)
				}
			}
		}
	}
	return out
}

// Found is a tuner discovered on this network or machine, ready to add.
type Found struct {
	Driver  string            `json:"driver"`
	Name    string            `json:"name"`
	Detail  string            `json:"detail,omitempty"`
	Options map[string]string `json:"options"`
}

// Discover looks for tuners: on the local network and among this machine's
// devices. It takes a couple of seconds.
func Discover(ctx context.Context, env Env) []Found {
	var mu sync.Mutex
	var out []Found
	var wg sync.WaitGroup
	add := func(f func(context.Context, Env) []Found) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := f(ctx, env)
			mu.Lock()
			out = append(out, r...)
			mu.Unlock()
		}()
	}
	add(discoverNetworkTuners)
	add(discoverSATIP)
	add(discoverDVB)
	add(discoverV4L2)
	add(discoverCapture)
	wg.Wait()
	return out
}

// noScan is for drivers whose channel list simply is what they know.
func noScan(s Source, ctx context.Context, progress func(Progress)) ([]Channel, error) {
	ch, err := s.Channels(ctx)
	if progress != nil {
		progress(Progress{Percent: 100, Found: len(ch)})
	}
	return ch, err
}

// guarded wraps raw MPEG-TS so an encrypted stream is stopped.
func guarded(r io.ReadCloser) *Stream {
	g := mpegts.NewGuard(r)
	return &Stream{Reader: g, Guard: g}
}
