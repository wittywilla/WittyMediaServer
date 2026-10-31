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
	"log/slog"
	"net/http"
	"os/exec"
	"runtime"
	"runtime/debug"
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
	// Checked: the source says for itself whether this channel is protected,
	// so no Encrypted reason means it is clear. Otherwise nobody knows until
	// the channel is first tuned.
	Checked bool `json:"checked,omitempty"`
	// ATSC3: an ATSC 3.0 (NextGen TV) broadcast.
	ATSC3      bool   `json:"atsc3,omitempty"`
	VideoCodec string `json:"videoCodec,omitempty"` // as the source names it: HEVC, MPEG2…
	AudioCodec string `json:"audioCodec,omitempty"` // AC4, AC3…
	// NoRecord is why the device says this channel may not be recorded.
	NoRecord string `json:"noRecord,omitempty"`
}

// Stream is an open channel: Reader for raw MPEG-TS, else Input for FFmpeg.
type Stream struct {
	Reader io.ReadCloser
	Input  []string
	// Guard watches a Reader for scrambling and collects guide data. Set
	// OnEvent on it before reading to receive that data.
	Guard *mpegts.Guard
	// Verified: an Input stream was inspected on opening and found clear.
	Verified bool
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
	return fmt.Sprintf("%s: %s. %s (%s). %s", name, EncryptedLabel, encryptedBecause, e.Why, encryptedPolicy)
}

// What every client shows for an encrypted channel, so that nobody takes it
// for a fault in their setup.
const (
	EncryptedLabel   = "Encrypted: can't be played here"
	encryptedBecause = "The broadcaster protects this channel"
	encryptedPolicy  = "WittyMediaServer does not decrypt or bypass protected broadcasts, so it can't be played or recorded. Nothing is wrong with your setup."
	// EncryptedExplain is the same in one line, for lists.
	EncryptedExplain = encryptedBecause + ". " + encryptedPolicy
)

// IsEncrypted reports whether err means a channel was refused for encryption.
func IsEncrypted(err error) bool {
	var e *EncryptedError
	return errors.As(err, &e) || errors.Is(err, mpegts.ErrScrambled)
}

// ScrambledReason says why a stream stopped by the guard was refused.
const ScrambledReason = "the signal itself is scrambled"

// RefusalMessage turns a stream that a guard stopped into words for a person.
func RefusalMessage(name string, err error) error {
	if errors.Is(err, mpegts.ErrScrambled) {
		return &EncryptedError{Channel: name, Why: ScrambledReason}
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
	// Log is where drivers say what went wrong; nil says nothing. Records are
	// written with the context the host passed in, so its session comes along.
	Log *slog.Logger
}

func (e Env) logger() *slog.Logger {
	if e.Log != nil {
		return e.Log
	}
	return slog.New(slog.DiscardHandler)
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
			d.Note = "Needs " + strings.Join(platformNames(d.Platforms), " or ") + "."
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
		s, err := d.build(cfg, env)
		if err != nil {
			return nil, err
		}
		return checked{s}, nil
	}
	return nil, errors.New("unknown kind of tuner")
}

// checked is the one way a driver reaches the host, so the rule about encrypted
// channels holds for every driver, custom commands included: a channel known to
// be protected is never opened, and raw MPEG-TS always passes the guard.
type checked struct{ Source }

func (c checked) Open(ctx context.Context, ch Channel) (*Stream, error) {
	if ch.Encrypted != "" {
		return nil, &EncryptedError{ch.Name, ch.Encrypted}
	}
	st, err := c.Source.Open(ctx, ch)
	if err != nil {
		return nil, err
	}
	if st.Reader != nil && st.Guard == nil {
		st = guarded(st.Reader)
	}
	return st, nil
}

// GuideHeaders passes on what a source needs for fetching its guide.
func (c checked) GuideHeaders() http.Header {
	if g, ok := c.Source.(interface{ GuideHeaders() http.Header }); ok {
		return g.GuideHeaders()
	}
	return nil
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
	return discover(ctx, env, discoverNetworkTuners, discoverSATIP, discoverDVB, discoverV4L2, discoverCapture)
}

// DiscoverNetwork is the part of Discover that only needs a network.
func DiscoverNetwork(ctx context.Context, env Env) []Found {
	return discover(ctx, env, discoverNetworkTuners, discoverSATIP)
}

// DiscoverHere is the part of Discover that looks at this machine's own devices.
func DiscoverHere(ctx context.Context, env Env) []Found {
	return discover(ctx, env, discoverDVB, discoverV4L2, discoverCapture)
}

func discover(ctx context.Context, env Env, finders ...func(context.Context, Env) []Found) []Found {
	var mu sync.Mutex
	var out []Found
	var wg sync.WaitGroup
	add := func(f func(context.Context, Env) []Found) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// What answers on the network isn't to be trusted to parse cleanly.
			defer func() {
				if p := recover(); p != nil {
					env.logger().ErrorContext(ctx, "tuner discovery crashed", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
				}
			}()
			r := f(ctx, env)
			mu.Lock()
			out = append(out, r...)
			mu.Unlock()
		}()
	}
	for _, f := range finders {
		add(f)
	}
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

// platformNames turns GOOS values into the names people know.
func platformNames(goos []string) []string {
	names := map[string]string{"darwin": "macOS", "linux": "Linux", "windows": "Windows", "freebsd": "FreeBSD"}
	out := make([]string, len(goos))
	for i, g := range goos {
		out[i] = g
		if n, ok := names[g]; ok {
			out[i] = n
		}
	}
	return out
}
