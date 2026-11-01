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

	"github.com/wittywilla/wittymediaserver/internal/tuner"
	"github.com/wittywilla/wittymediaserver/internal/tuner/mpegts"
	"github.com/wittywilla/wittymodule"
)

const tunerID = "wittytuner"

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
	maxDrivers    = 64
)

// Tuners makes WittyTuner's sources for the server. Every question is a helper of its own,
// which leaves when it has answered, so nothing runs while no one is watching and a new
// version takes over with no restart.
//
// What is a helper's to say and what isn't: it may say a channel is protected or busy, since
// that only ever refuses something. It can't say a channel is clear. Every stream it hands
// over is watched by this server's own guard, and the server only believes what that guard
// sees. WittyTuner has no forks for the same reason. What it asks FFmpeg to open is held to
// checkInput, since FFmpeg's options can read files and run other protocols.
type Tuners struct{ M *Manager }

// callFor is a question about a tuner, with what a helper needs told.
func callFor(driver string, cfg tuner.Config, env tuner.Env) tuner.HelperCall {
	return tuner.HelperCall{Driver: driver, Name: cfg.Name, Options: cfg.Options, FFmpeg: env.FFmpeg, AllowCommands: env.AllowCommands}
}

// New makes a source, with its settings checked by the helper.
func (t Tuners) New(ctx context.Context, driver string, cfg tuner.Config, env tuner.Env) (tuner.Source, error) {
	if t.M == nil {
		return nil, (*Manager)(nil).missing(tunerID)
	}
	call := callFor(driver, cfg, env)
	d, err := t.M.askTuner(ctx, "validate", call, nil)
	if err != nil {
		return nil, err
	}
	if d.Error != "" {
		return nil, docError(d, "")
	}
	p := &tunerProxy{m: t.M, call: call, headers: d.GuideHeaders, info: tuner.Info{Driver: driver, Name: cfg.Name}}
	p.setInfo(d.Info)
	return p, nil
}

// Discover looks for tuners, on the network and on this machine.
func (t Tuners) Discover(ctx context.Context, env tuner.Env) ([]tuner.Found, error) {
	if t.M == nil {
		return nil, (*Manager)(nil).missing(tunerID)
	}
	list, cancel := context.WithTimeout(ctx, tunerListTime)
	defer cancel()
	d, err := t.M.askTuner(list, "discover", callFor("", tuner.Config{}, env), nil)
	if err != nil {
		return nil, err
	}
	if d.Error != "" {
		return nil, docError(d, "")
	}
	return cleanFound(d.Found), nil
}

// Drivers lists the kinds of tuner, each answered for this machine.
func (t Tuners) Drivers(ctx context.Context, env tuner.Env) ([]tuner.Driver, error) {
	if t.M == nil {
		return nil, (*Manager)(nil).missing(tunerID)
	}
	d, err := t.M.askTuner(ctx, "drivers", callFor("", tuner.Config{}, env), nil)
	if err != nil {
		return nil, err
	}
	if d.Error != "" {
		return nil, docError(d, "")
	}
	out := d.Drivers
	if len(out) > maxDrivers {
		out = out[:maxDrivers]
	}
	return out, nil
}

// secretsCache keeps the list of drivers from the version in use, since a page listing tuners
// asks about each one and a helper's start is not nothing.
var secretsCache struct {
	sync.Mutex
	version string
	list    []tuner.Driver
}

// SecretFields are a driver's settings that are never shown again once they're saved. A
// caller treats an error as hiding every setting.
func (t Tuners) SecretFields(ctx context.Context, driver string, env tuner.Env) ([]string, error) {
	version := ""
	if t.M != nil {
		version = t.M.VersionInUse(tunerID)
	}
	secretsCache.Lock()
	list := secretsCache.list
	if secretsCache.version != version || version == "" {
		list = nil
	}
	secretsCache.Unlock()
	if list == nil {
		var err error
		if list, err = t.Drivers(ctx, env); err != nil {
			return nil, err
		}
		secretsCache.Lock()
		secretsCache.version, secretsCache.list = version, list
		secretsCache.Unlock()
	}
	var out []string
	for _, d := range list {
		if d.ID == driver {
			for _, f := range d.Fields {
				if f.Kind == "password" {
					out = append(out, f.Key)
				}
			}
		}
	}
	return out, nil
}

// cleanFound keeps what makes sense, and shortens what's shown.
func cleanFound(in []tuner.Found) []tuner.Found {
	out := []tuner.Found{}
	for _, f := range in {
		if len(out) == maxFound {
			break
		}
		// A found tuner goes on the page as a suggestion, and a program to run is never one.
		if f.Driver == "command" {
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

// docError is what a final document that says no means.
func docError(d *tuner.HelperDoc, channel string) error {
	switch d.Code {
	case "encrypted":
		return &tuner.EncryptedError{Channel: channel, Why: clip(d.Error)}
	case "busy":
		return tuner.ErrBusy
	case "unreachable":
		return fmt.Errorf("%w: %s", tuner.ErrUnreachable, clip(d.Error))
	}
	return errors.New(clip(d.Error))
}

// askTuner asks a helper of its own one thing, and reads its answer's documents.
func (m *Manager) askTuner(ctx context.Context, method string, call tuner.HelperCall, progress func(tuner.Progress)) (*tuner.HelperDoc, error) {
	a := m.Attach(tunerID, tunerAnswerTime)
	if a == nil {
		return nil, m.missing(tunerID)
	}
	defer a.Close()
	r, err := a.Stream(method, call, nil)
	if err != nil {
		var ce *wittymodule.CallError
		if errors.As(err, &ce) {
			return &tuner.HelperDoc{Error: ce.Message, Code: ce.Code, Done: true}, nil
		}
		return nil, fmt.Errorf("WittyTuner failed: %w", err)
	}
	stop := context.AfterFunc(ctx, a.Close)
	defer stop()
	dec := json.NewDecoder(io.LimitReader(r, wittymodule.MaxBlob))
	for {
		var d tuner.HelperDoc
		if err := dec.Decode(&d); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			err = fmt.Errorf("it stopped before it finished: %w", err)
			a.failedNow(err)
			return nil, fmt.Errorf("WittyTuner failed: %w", err)
		}
		if d.Progress == nil {
			return &d, nil
		}
		if progress != nil {
			p := *d.Progress
			p.Percent, p.Found, p.Message = min(max(p.Percent, 0), 100), max(p.Found, 0), clip(p.Message)
			progress(p)
		}
	}
}

// VersionInUse is the version of one module in use, or "".
func (m *Manager) VersionInUse(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.mods[id]; e != nil && e.dir != "" {
		return e.man.Version
	}
	return ""
}

// failedNow counts something this job saw against the version.
func (a *Attached) failedNow(err error) { a.m.failed(a.id, a.e, a.version, err) }

// tunerProxy is a tuner whose work a helper does.
type tunerProxy struct {
	m       *Manager
	call    tuner.HelperCall
	headers http.Header

	mu   sync.Mutex
	info tuner.Info
}

// Info is what the helper last said. It has to be quick: it is asked with a lock held.
func (p *tunerProxy) Info() tuner.Info {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.info
}

func (p *tunerProxy) setInfo(i *tuner.Info) {
	if i == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if i.Name != "" {
		p.info.Name = clip(i.Name)
	}
	p.info.Detail = clip(i.Detail)
	p.info.MaxStreams = max(i.MaxStreams, 0)
	// Fetching a guide must stay fetching: a guide source can also be a file on the server,
	// and that's for the owner to choose, not something a device says.
	if u, err := url.Parse(i.GuideURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		p.info.GuideURL = i.GuideURL
	}
}

// GuideHeaders come from the settings, by way of the helper.
func (p *tunerProxy) GuideHeaders() http.Header { return p.headers }

// Close has nothing to do: no process is kept between questions.
func (p *tunerProxy) Close() error { return nil }

// fromDoc takes a final document's channels.
func (p *tunerProxy) fromDoc(d *tuner.HelperDoc) ([]tuner.Channel, error) {
	if d.Error != "" {
		return nil, docError(d, "")
	}
	p.setInfo(d.Info)
	return clean(d.Channels), nil
}

// clean makes a helper's list of channels what the server will keep. Saying a channel is known
// to be clear isn't a helper's to say, so that comes from what a stream shows the server's own guard.
func clean(in []tuner.Channel) []tuner.Channel {
	out := make([]tuner.Channel, 0, len(in))
	for _, c := range in {
		if len(out) == maxChannels {
			break
		}
		if c.ID == "" || len(c.ID) > maxTunerText || len(c.Locator) > maxLocator {
			continue
		}
		if len(c.Logo) > maxLocator || (c.Logo != "" && !strings.HasPrefix(c.Logo, "http://") && !strings.HasPrefix(c.Logo, "https://")) {
			c.Logo = ""
		}
		c.Number, c.Name, c.Group, c.CallSign, c.EPGID = clip(c.Number), clip(c.Name), clip(c.Group), clip(c.CallSign), clip(c.EPGID)
		c.VideoCodec, c.AudioCodec, c.Encrypted, c.NoRecord = clip(c.VideoCodec), clip(c.AudioCodec), clip(c.Encrypted), clip(c.NoRecord)
		out = append(out, c)
	}
	return out
}

// Channels reads the list.
func (p *tunerProxy) Channels(ctx context.Context) ([]tuner.Channel, error) {
	list, cancel := context.WithTimeout(ctx, tunerListTime)
	defer cancel()
	d, err := p.m.askTuner(list, "channels", p.call, nil)
	if err != nil {
		return nil, err
	}
	return p.fromDoc(d)
}

// Scan looks for channels, reporting as it goes.
func (p *tunerProxy) Scan(ctx context.Context, o tuner.ScanOptions, progress func(tuner.Progress)) ([]tuner.Channel, error) {
	call := p.call
	call.Source = o.Source
	d, err := p.m.askTuner(ctx, "scan", call, progress)
	if err != nil {
		return nil, err
	}
	ch, err := p.fromDoc(d)
	if err == nil && progress != nil {
		progress(tuner.Progress{Percent: 100, Found: len(ch)})
	}
	return ch, err
}

// FFmpeg options a helper may ask for: each only says how to read the input, with what
// follows it a value. Anything else is refused, because FFmpeg has options that write files.
var inputOptions = map[string]bool{
	"-user_agent": true, "-headers": true, "-referer": true, "-timeout": true, "-rw_timeout": true, "-reconnect": true, "-reconnect_streamed": true,
	"-reconnect_delay_max": true, "-reconnect_on_network_error": true, "-reconnect_on_http_error": true, "-rtsp_transport": true, "-fflags": true,
	"-analyzeduration": true, "-probesize": true, "-seekable": true, "-multiple_requests": true, "-http_persistent": true, "-tls_verify": true,
	"-allowed_extensions": true, "-protocol_whitelist": true, "-live_start_index": true, "-f": true, "-framerate": true, "-video_size": true,
	"-input_format": true, "-pixel_format": true, "-standard": true, "-thread_queue_size": true, "-channel_layout": true, "-sample_rate": true, "-channels": true,
	"-rtbufsize": true, "-audio_buffer_size": true, "-pkt_size": true, "-buffer_size": true, "-fifo_size": true, "-overrun_nonfatal": true, "-mode": true,
}

// Formats that open this machine's capture devices, whose address is a device's name.
var captureFormats = map[string]bool{"v4l2": true, "alsa": true, "dshow": true, "avfoundation": true}

// Formats that read a stream. FFmpeg has others that make things up or read files.
var streamFormats = map[string]bool{"mpegts": true, "hls": true, "rtsp": true, "rtp": true, "flv": true}

// checkInput refuses what a helper asks FFmpeg to open unless it can only be a stream or a
// capture device: FFmpeg can also read files, chain other protocols and write. It is the
// last thing between a helper with a bad day and this server's own program.
func checkInput(input []string) error {
	addr, capture, limited := "", false, false
	for i := 0; i < len(input); i++ {
		a := input[i]
		if a == "-i" {
			if i+2 != len(input) {
				return errors.New("it gave FFmpeg more after the address")
			}
			addr = input[i+1]
			break
		}
		if !inputOptions[a] || i+1 >= len(input) {
			return fmt.Errorf("it asked for the FFmpeg option %q", a)
		}
		i++
		if a == "-protocol_whitelist" {
			limited = true
		}
		if a == "-protocol_whitelist" && strings.ContainsAny(input[i], "|") {
			return errors.New("it asked for a strange list of protocols")
		}
		if a == "-protocol_whitelist" {
			for _, proto := range strings.Split(input[i], ",") {
				if proto == "file" || proto == "concat" || proto == "subfile" || proto == "pipe" || proto == "data" || proto == "crypto" {
					return fmt.Errorf("it asked FFmpeg to allow %q", proto)
				}
			}
		}
		if a == "-f" {
			capture = captureFormats[input[i]]
			if !capture && !streamFormats[input[i]] {
				return fmt.Errorf("it asked for the FFmpeg format %q", input[i])
			}
		}
	}
	if addr == "" || len(addr) > maxLocator || strings.ContainsAny(addr, "\r\n") {
		return errors.New("it offered no address")
	}
	if capture {
		return nil
	}
	// Without a list FFmpeg would follow a playlist to files and to its own decryption.
	if !limited {
		return errors.New("it didn't limit what FFmpeg may open")
	}
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return errors.New("it offered something other than an address")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "udp", "rtp", "rtsp", "rtsps", "rtmp", "rtmps", "srt":
		return nil
	}
	return fmt.Errorf("it offered a %q address", u.Scheme)
}

// Open starts a channel through a process of its own, so a helper failing ends only
// this stream. See Tuners for what is and isn't believed.
func (p *tunerProxy) Open(ctx context.Context, ch tuner.Channel) (*tuner.Stream, error) {
	if ch.Encrypted != "" {
		return nil, &tuner.EncryptedError{Channel: ch.Name, Why: ch.Encrypted}
	}
	a := p.m.Attach(tunerID, tunerAnswerTime)
	if a == nil {
		return nil, p.m.missing(tunerID)
	}
	call := p.call
	call.Channel = &ch
	var ans tuner.HelperOpen
	r, err := a.Stream("open", call, &ans)
	if err != nil {
		a.Close()
		var ce *wittymodule.CallError
		if errors.As(err, &ce) {
			switch ce.Code {
			case "encrypted":
				return nil, &tuner.EncryptedError{Channel: ch.Name, Why: clip(ce.Message)}
			case "busy":
				return nil, tuner.ErrBusy
			case "unreachable":
				return nil, fmt.Errorf("%w: %s", tuner.ErrUnreachable, clip(ce.Message))
			}
		}
		return nil, err
	}
	if ans.Input != nil {
		a.Close()
		if err := checkInput(ans.Input); err != nil {
			a.failedNow(fmt.Errorf("its stream was refused: %w", err))
			return nil, fmt.Errorf("WittyTuner's stream was refused: %w", err)
		}
		st := &tuner.Stream{Input: ans.Input, Verified: ans.Verified}
		if ans.Recheck {
			st.Recheck = func(ctx context.Context) (string, time.Duration, error) { return p.recheck(ctx, ch) }
		}
		return st, nil
	}
	// The stream lasts only as long as ctx.
	stop := context.AfterFunc(ctx, a.Close)
	g := mpegts.NewGuard(&helperStream{ReadCloser: r, stop: stop})
	return &tuner.Stream{Reader: g, Guard: g}, nil
}

// recheck looks again at a stream FFmpeg is reading that can change while it plays.
func (p *tunerProxy) recheck(ctx context.Context, ch tuner.Channel) (string, time.Duration, error) {
	call := p.call
	call.Channel = &ch
	d, err := p.m.askTuner(ctx, "recheck", call, nil)
	if err != nil {
		return "", 0, err
	}
	if d.Error != "" {
		return "", 0, errors.New(clip(d.Error))
	}
	return clip(d.Why), time.Duration(max(d.Next, 0)) * time.Second, nil
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
