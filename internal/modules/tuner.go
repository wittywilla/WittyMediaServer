package modules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittytuner"
	"github.com/wittywilla/wittytuner/mpegts"
)

const tunerID = "wittytuner"

// tunerDrivers are the kinds of tuner a helper can run: they only need a network.
// Everything else stays with the built-in copy, whichever version is in use, because a
// helper's limits don't suit it. A custom command and FFmpeg are other programs, which
// a helper may not start, and capture cards and DVB adapters are this machine's devices.
var tunerDrivers = map[string]bool{"network": true, "iptv": true, "stream": true, "backend": true}

const (
	// tunerAnswerTime is how long a helper has to open a channel, and to start anything
	// else. A helper that takes longer is stopped, and it counts against its version.
	tunerAnswerTime = 45 * time.Second
	// tunerListTime is for reading a channel list, which a scan isn't held to.
	tunerListTime = 2 * time.Minute
	maxTunerText  = 1000
	maxLocator    = 8 << 10
	maxChannels   = 100000
	maxFound      = 256
)

// Tuners makes WittyTuner's sources for the server. A source decides at each call
// whether to use a helper, so a new version takes over with no restart, and with the
// built-in copy in use it does just what that copy does.
//
// What is a helper's to say and what isn't: it may say a channel is protected or
// busy, since that only ever refuses something. It can't say a channel is clear. Every
// stream it hands over is watched by this server's own guard, as any other is, and
// the server only believes what that guard sees. A stream for FFmpeg to open itself
// isn't taken from it either: the built-in copy opens and checks it, because FFmpeg's
// options can read files and run other protocols.
type Tuners struct{ M *Manager }

// New makes a source. Its settings are always checked by the built-in copy.
func (t Tuners) New(driver string, cfg wittytuner.Config, env wittytuner.Env) (wittytuner.Source, error) {
	builtin, err := wittytuner.New(driver, cfg, env)
	if err != nil {
		return nil, err
	}
	if t.M == nil || !tunerDrivers[driver] {
		return builtin, nil
	}
	check, ok := builtin.(locatorChecker)
	if !ok {
		return builtin, nil
	}
	return &tunerProxy{Source: builtin, check: check, m: t.M, call: wittytuner.HelperCall{Driver: driver, Name: cfg.Name, Options: cfg.Options}}, nil
}

// Discover looks for tuners: the helper looks over the network, and this machine's own
// devices are looked for here.
func (t Tuners) Discover(ctx context.Context, env wittytuner.Env) []wittytuner.Found {
	if t.M == nil {
		return wittytuner.Discover(ctx, env)
	}
	var wg sync.WaitGroup
	var here []wittytuner.Found
	wg.Add(1)
	go func() { defer wg.Done(); here = wittytuner.DiscoverHere(ctx, env) }()
	var near []wittytuner.Found
	list, cancel := context.WithTimeout(ctx, tunerListTime)
	defer cancel()
	if d := t.M.askTuner(list, "discover", wittytuner.HelperCall{}, nil); d != nil && d.Error == "" {
		near = cleanFound(d.Found)
	} else {
		near = wittytuner.DiscoverNetwork(ctx, env)
	}
	wg.Wait()
	return append(near, here...)
}

// cleanFound keeps what could be on a network. Adding one goes through the built-in
// copy's checks like any other settings.
func cleanFound(in []wittytuner.Found) []wittytuner.Found {
	out := []wittytuner.Found{}
	for _, f := range in {
		if len(out) == maxFound {
			break
		}
		if f.Driver != "network" && f.Driver != "satip" {
			continue
		}
		f.Name, f.Detail = clip(f.Name), clip(f.Detail)
		out = append(out, f)
	}
	return out
}

func clip(s string) string {
	if len(s) > maxTunerText {
		return s[:maxTunerText]
	}
	return s
}

// askTuner asks a helper of its own one thing, and reads its answer's documents. It
// returns nil when the built-in copy should answer: there is no helper in use, or it failed.
func (m *Manager) askTuner(ctx context.Context, method string, call wittytuner.HelperCall, progress func(wittytuner.Progress)) *wittytuner.HelperDoc {
	a := m.Attach(tunerID, tunerAnswerTime)
	if a == nil {
		return nil
	}
	defer a.Close()
	r, err := a.Stream(method, call, nil)
	if err != nil {
		return nil
	}
	stop := context.AfterFunc(ctx, a.Close)
	defer stop()
	dec := json.NewDecoder(io.LimitReader(r, wittymodule.MaxBlob))
	for {
		var d wittytuner.HelperDoc
		if err := dec.Decode(&d); err != nil {
			if ctx.Err() == nil {
				a.failedNow(fmt.Errorf("it stopped before it finished: %w", err))
			}
			return nil
		}
		if d.Progress == nil {
			return &d
		}
		if progress != nil {
			p := *d.Progress
			p.Percent, p.Found, p.Message = min(max(p.Percent, 0), 100), max(p.Found, 0), clip(p.Message)
			progress(p)
		}
	}
}

// failedNow counts something this job saw against the version.
func (a *Attached) failedNow(err error) { a.m.failed(a.id, a.e, a.version, err) }

type locatorChecker interface{ CheckLocator(string) error }

// tunerProxy is a tuner whose work a helper does when one is in use, with the built-in
// copy behind it for everything a helper can't do, or fails at.
type tunerProxy struct {
	wittytuner.Source
	check locatorChecker
	m     *Manager
	call  wittytuner.HelperCall

	mu   sync.Mutex
	info *wittytuner.Info // what a helper last learned from the device
}

// Info is what the built-in copy knows from its settings, with what a helper has
// learned from the device. It has to be quick: it is asked with a lock held.
func (p *tunerProxy) Info() wittytuner.Info {
	i := p.Source.Info()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.info != nil {
		if p.info.Detail != "" {
			i.Detail = clip(p.info.Detail)
		}
		if p.info.MaxStreams > 0 {
			i.MaxStreams = p.info.MaxStreams
		}
		// Only a playlist finds its guide address as it goes. Fetching it must stay
		// fetching: a guide source can also be a file on the server.
		if u, err := url.Parse(p.info.GuideURL); i.GuideURL == "" && err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && p.call.Driver == "iptv" {
			i.GuideURL = p.info.GuideURL
		}
	}
	return i
}

// GuideHeaders are the built-in copy's: they come from the settings.
func (p *tunerProxy) GuideHeaders() http.Header {
	if g, ok := p.Source.(interface{ GuideHeaders() http.Header }); ok {
		return g.GuideHeaders()
	}
	return nil
}

// fromDoc takes a final document's channels, or nil when the built-in copy should
// answer. An empty list would take the owner's channels away, so it needs the built-in
// copy's word.
func (p *tunerProxy) fromDoc(d *wittytuner.HelperDoc) []wittytuner.Channel {
	if d == nil || d.Error != "" {
		return nil
	}
	list := p.clean(d.Channels)
	if len(list) == 0 {
		return nil
	}
	if d.Info != nil {
		p.mu.Lock()
		p.info = d.Info
		p.mu.Unlock()
	}
	return list
}

// clean makes a helper's list of channels what the server will keep. Saying a channel
// is known to be clear isn't a helper's to say, so that comes from what a stream shows
// the server's own guard. An address the built-in copy wouldn't use goes.
func (p *tunerProxy) clean(in []wittytuner.Channel) []wittytuner.Channel {
	out := make([]wittytuner.Channel, 0, len(in))
	for _, c := range in {
		if len(out) == maxChannels {
			break
		}
		if c.ID == "" || len(c.ID) > maxTunerText || len(c.Locator) > maxLocator || p.check.CheckLocator(c.Locator) != nil {
			continue
		}
		if c.Logo != "" && !strings.HasPrefix(c.Logo, "http://") && !strings.HasPrefix(c.Logo, "https://") {
			c.Logo = ""
		}
		c.Number, c.Name, c.Group, c.CallSign, c.EPGID = clip(c.Number), clip(c.Name), clip(c.Group), clip(c.CallSign), clip(c.EPGID)
		c.VideoCodec, c.AudioCodec, c.Encrypted, c.NoRecord = clip(c.VideoCodec), clip(c.AudioCodec), clip(c.Encrypted), clip(c.NoRecord)
		if len(c.Logo) > maxLocator {
			c.Logo = ""
		}
		c.Checked = false
		out = append(out, c)
	}
	return out
}

// Channels reads the list through a helper, and the built-in copy has the last word
// if that didn't work: the helper's limits may be why.
func (p *tunerProxy) Channels(ctx context.Context) ([]wittytuner.Channel, error) {
	list, cancel := context.WithTimeout(ctx, tunerListTime)
	defer cancel()
	if ch := p.fromDoc(p.m.askTuner(list, "channels", p.call, nil)); ch != nil {
		return ch, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.Source.Channels(ctx)
}

// Scan looks for channels through a helper, which reports as it goes.
func (p *tunerProxy) Scan(ctx context.Context, o wittytuner.ScanOptions, progress func(wittytuner.Progress)) ([]wittytuner.Channel, error) {
	call := p.call
	call.Source = o.Source
	if ch := p.fromDoc(p.m.askTuner(ctx, "scan", call, progress)); ch != nil {
		if progress != nil {
			progress(wittytuner.Progress{Percent: 100, Found: len(ch)})
		}
		return ch, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.Source.Scan(ctx, o, progress)
}

// helperCanOpen: addresses other than these take FFmpeg, and a helper runs no programs.
func helperCanOpen(locator string) bool {
	if locator == "" {
		return true
	}
	u, err := url.Parse(locator)
	return err == nil && (strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https") || u.Host == "")
}

// plainInputURL takes the address out of what a helper offered for FFmpeg to open, and
// refuses anything but a plain address of a kind that can only be a stream.
func plainInputURL(input []string) (string, error) {
	if len(input) != 2 || input[0] != "-i" {
		return "", errors.New("it offered FFmpeg options as well as an address")
	}
	raw := input[1]
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || strings.ContainsAny(raw, " \t\r\n|") {
		return "", errors.New("it offered something other than a plain address")
	}
	switch u.Scheme {
	case "http", "https", "udp", "rtp":
		return raw, nil
	}
	return "", fmt.Errorf("it offered a %q address", u.Scheme)
}

// Open starts a channel through a process of its own, so a helper failing ends only
// this stream. See Tuners for what is and isn't believed.
func (p *tunerProxy) Open(ctx context.Context, ch wittytuner.Channel) (*wittytuner.Stream, error) {
	if ch.Encrypted != "" {
		return nil, &wittytuner.EncryptedError{Channel: ch.Name, Why: ch.Encrypted}
	}
	if !helperCanOpen(ch.Locator) || p.check.CheckLocator(ch.Locator) != nil {
		return p.Source.Open(ctx, ch)
	}
	a := p.m.Attach(tunerID, tunerAnswerTime)
	if a == nil {
		return p.Source.Open(ctx, ch)
	}
	call := p.call
	call.Channel = &ch
	var ans wittytuner.HelperOpen
	r, err := a.Stream("open", call, &ans)
	if err != nil {
		a.Close()
		var ce *wittymodule.CallError
		if errors.As(err, &ce) {
			switch ce.Code {
			case "encrypted":
				return nil, &wittytuner.EncryptedError{Channel: ch.Name, Why: clip(ce.Message)}
			case "busy":
				return nil, wittytuner.ErrBusy
			}
		}
		return p.Source.Open(ctx, ch)
	}
	if ans.Input != nil {
		a.Close()
		addr, err := plainInputURL(ans.Input)
		if err == nil {
			err = p.check.CheckLocator(addr)
		}
		if err != nil {
			a.failedNow(fmt.Errorf("its stream was refused: %w", err))
			return p.Source.Open(ctx, ch)
		}
		ch.Locator = addr
		return p.Source.Open(ctx, ch)
	}
	// As with the built-in copy, the stream lasts only as long as ctx.
	stop := context.AfterFunc(ctx, a.Close)
	g := mpegts.NewGuard(&helperStream{ReadCloser: r, stop: stop})
	return &wittytuner.Stream{Reader: g, Guard: g}, nil
}

// helperStream ends a helper's stream, and what would have ended it.
type helperStream struct {
	io.ReadCloser
	stop func() bool
}

func (h *helperStream) Close() error {
	h.stop()
	return h.ReadCloser.Close()
}
