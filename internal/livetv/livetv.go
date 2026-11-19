// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package livetv is the server's half of live TV from tuners: the guide data from XMLTV and from
// the broadcasts themselves, opening channels through WittyTuner and guarding every stream, and
// recording. The tuners, their channels and the guide sources are WittyTuner's own (module API 5):
// the server reaches them through a Directory, and keeps only what the guide, recordings,
// favourites and sets need.
package livetv

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// Channel is a tuner's channel as the guide and the settings see it. Number
// and Name are what the viewer sees: the owner's choice, else the source's.
type Channel struct {
	ID        int64  `json:"id"`
	TunerID   int64  `json:"tunerId"`
	SourceID  string `json:"sourceId"`
	Number    string `json:"number"`
	Name      string `json:"name"`
	Group     string `json:"group,omitempty"`
	CallSign  string `json:"callsign,omitempty"`
	Logo      string `json:"-"` // the source's address, never sent to clients: see handleTunerLogo (from WittyTuner's logoUrl)
	HD        bool   `json:"hd,omitempty"`
	Radio     bool   `json:"radio,omitempty"`
	Encrypted string `json:"encrypted,omitempty"`
	// Security is encrypted, clear or unknown (nobody can tell until it is tuned).
	Security   string `json:"security"`
	ATSC3      bool   `json:"atsc3,omitempty"`
	VideoCodec string `json:"videoCodec,omitempty"`
	AudioCodec string `json:"audioCodec,omitempty"`
	// NoRecord is why this channel may not be recorded: the device said so, or
	// the owner did from what the device's documentation says.
	NoRecord  string `json:"noRecord,omitempty"`
	Program   int    `json:"program,omitempty"`
	Enabled   bool   `json:"enabled"`
	EPGID     string `json:"epgId,omitempty"`
	NumberSet string `json:"numberSet,omitempty"`
	NameSet   string `json:"nameSet,omitempty"`
	EPGSet    string `json:"epgSet,omitempty"`
	// LogoSet is what a person chose for its logo: "" to find it by name, "none", or
	// "kind|name" in the logo service.
	LogoSet string `json:"logoSet,omitempty"`
	// CatchupDays: the source keeps this many days of what the channel showed, to be
	// watched from the start. catchup and catchupSource say how to ask for it.
	CatchupDays int `json:"catchupDays,omitempty"`
	// Health is what the last check or tune found: HealthOK, HealthFailing, or "" for not yet.
	Health    string `json:"health,omitempty"`
	HealthAt  int64  `json:"healthAt,omitempty"`
	HealthMsg string `json:"healthMsg,omitempty"`
	// MergedInto: this channel is a backup of that one, hidden from the guide and tried
	// when that one won't open. Backups counts a channel's own backups (settings only).
	MergedInto int64 `json:"mergedInto,omitempty"`
	Backups    int   `json:"backups,omitempty"`
	// The tuner's, as WittyTuner says: its name and kind, and whether it's on.
	TunerName    string `json:"tunerName,omitempty"`
	Driver       string `json:"driver,omitempty"`
	TunerEnabled bool   `json:"tunerEnabled,omitempty"`
	// TunerStreams is how many channels the tuner can stream at once; 0 for no known limit.
	TunerStreams int `json:"tunerStreams,omitempty"`
	// LogoURL is the source's own address for the channel's logo (Logo), as WittyTuner sends it.
	LogoURL string `json:"logoUrl,omitempty"`
	// GuideShift moves the channel's guide by this many minutes (a playlist's tvg-shift): a "+1"
	// channel shows its guide id's programmes an hour later.
	GuideShift int `json:"guideShift,omitempty"`
}

// Directory is WittyTuner, as the server's live TV reaches it: every channel of every tuner, and
// opening one within its tuner's limit. It is set once, before the server starts serving.
type Directory interface {
	// Channels is every channel of every tuner.
	Channels(ctx context.Context) ([]*Channel, error)
	// Open opens a channel (with its backups, when backups is true), or a stretch of its archive.
	// The stream's Close lets it go. The answer's channel is the one that opened.
	Open(ctx context.Context, req OpenRequest) (*Opening, error)
	// Note keeps what the server's own guard saw a stream show: why it is encrypted, or "" for clear.
	Note(ctx context.Context, channelID int64, encryptedWhy string) error
}

// OpenRequest is a channel to open.
type OpenRequest struct {
	Channel                  int64
	ArchiveStart, ArchiveEnd int64
	Backups                  bool
}

// Opening is an opened stream: what WittyTuner handed over, which channel it is, and whether what
// it names may lead to the home network (it doesn't, for a playlist from the internet).
type Opening struct {
	Stream  *tuner.Stream
	Channel int64
	Home    bool
}

// ErrNoEngine: WittyTuner isn't on this server, or isn't on.
var ErrNoEngine = errors.New("WittyTuner isn't on this server")

var directory Directory

// SetDirectory says how to reach WittyTuner. It is set once, before the server starts serving.
func SetDirectory(d Directory) { directory = d }

// Manager owns the running sources.
type Manager struct {
	// Tasks is where guide updates, channel scans and recordings show in the activity
	// panel. May be nil.
	Tasks *tasks.Registry
	DB    *db.DB
	Tools *ffmpeg.Tools
	Log   *slog.Logger

	internet *http.Client // what internetClient gives, for a test

	mu               sync.Mutex
	viewers          map[int64]func() // live sessions by channel, for stopping one when a recording needs its tuner
	recordingCancels map[int64]context.CancelFunc
	dvrOnce          sync.Once
	epgOnce          sync.Once
	// WittyTuner's channels, kept for a moment: the guide asks for them on every page.
	chMu   sync.Mutex
	chList []*Channel
	chByID map[int64]*Channel
	chAt   time.Time
	// Stop ends everything at shutdown.
	stopCtx context.Context
	stop    context.CancelFunc
	// Work the manager started on its own, which Close ends and waits for: see background.
	bgMu      sync.Mutex
	bgWork    sync.WaitGroup
	bgRunning map[string]int // by name, to say what Close couldn't wait for
	bgClosed  bool
	closeOnce sync.Once
	// AfterRecording is told when a recording has finished (to rescan a library).
	AfterRecording func(path string)
	// Folder answers where recordings go.
	Folder func() string
}

// New makes a manager. Background work starts only when something needs it.
func New(database *db.DB, tools *ffmpeg.Tools, log *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{DB: database, Tools: tools, Log: log.With("cat", "tv"), viewers: map[int64]func(){}, stopCtx: ctx, stop: cancel}
}

// Start runs the guide's tidying and the recording scheduler, if there is anything for them to
// do. Called at startup, when a guide arrives and whenever a recording is scheduled.
func (m *Manager) Start() {
	var n int
	_ = m.DB.QueryRow(`SELECT (SELECT COUNT(*) FROM recordings) + (SELECT COUNT(*) FROM recording_rules) + (SELECT COUNT(*) FROM epg_channels)`).Scan(&n)
	if n == 0 {
		return
	}
	m.epgOnce.Do(func() { m.loop("guide tidying", m.epgLoop, "cat", "tv") })
	m.dvrOnce.Do(func() {
		m.failInterrupted()
		m.loop("recording scheduler", m.dvrLoop, "cat", "tv")
	})
}

// tunerLog is for what a tuner did, as opposed to what the guide or a recording did.
func (m *Manager) tunerLog() *slog.Logger { return m.Log.With("module", "wittytuner") }

var addressInText = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"']+`)

// PublicError is what people may be told about a tuner or stream that failed. Connection errors say
// what's on the network, and hold the address, which can hold a login. Anything else that quotes
// an address, as a helper program's errors can, has it taken out.
func PublicError(err error) string {
	if err == nil {
		return ""
	}
	var ue *url.Error
	var op *net.OpError
	var dns *net.DNSError
	var pe *fs.PathError
	if errors.As(err, &ue) || errors.As(err, &op) || errors.As(err, &dns) || errors.Is(err, tuner.ErrUnreachable) {
		return "the stream could not be reached"
	}
	if errors.As(err, &pe) {
		return "the recording could not be written to its folder"
	}
	if t := ffmpegFailure(err.Error()); t != "" {
		return t
	}
	return addressInText.ReplaceAllString(err.Error(), "(address)")
}

// ffmpegFailure puts what FFmpeg said as a stream failed into words for a person: its own lines are
// for the log. "" when the error isn't FFmpeg's.
func ffmpegFailure(msg string) string {
	if !strings.HasPrefix(msg, "exit status") && !strings.Contains(msg, "Error opening input") {
		return ""
	}
	l := strings.ToLower(msg)
	switch {
	case strings.Contains(l, "404 not found"), strings.Contains(l, "410 gone"):
		return "the channel's stream isn't there any more (404). Its playlist may be out of date: read it again in Settings, Live TV & tuners"
	case strings.Contains(l, "403 forbidden"), strings.Contains(l, "401 unauthorized"):
		return "the channel's stream refused this server (403). Its link may have expired, so reading the playlist again can help, or the provider may want a particular user agent or referrer"
	case strings.Contains(l, "server returned 5"):
		return "the provider's server had a problem. Try again in a moment"
	case strings.Contains(l, "connection refused"), strings.Contains(l, "timed out"), strings.Contains(l, "connection reset"), strings.Contains(l, "network is unreachable"):
		return "the stream could not be reached"
	case strings.Contains(l, "invalid data found"):
		return "the channel sent something that isn't video this server can read"
	}
	return "the channel's stream stopped and couldn't be started again. Try again in a moment, or try another channel"
}

// internetClient reaches the internet and nothing else, checking each connection as it is made.
func (m *Manager) internetClient() *http.Client {
	if m.internet != nil {
		return m.internet
	}
	return library.SafeClient
}

// What is known about a channel's protection.
const (
	SecurityClear     = "clear"
	SecurityEncrypted = "encrypted"
	SecurityUnknown   = "unknown"
)

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// GuideKey is the id that links a channel to guide data.
func (c *Channel) GuideKey() string {
	k := c.guideBase()
	if k != "" && c.GuideShift != 0 {
		return shiftedKey(k, c.GuideShift)
	}
	return k
}

// guideBase is the guide's own id for the channel, before any shift.
func (c *Channel) guideBase() string { return firstNonEmpty(c.EPGSet, c.EPGID) }

// shiftedKey is where a shifted channel's programmes are kept: its guide id's, moved.
func shiftedKey(base string, minutes int) string { return base + "@" + strconv.Itoa(minutes) }

func sortChannels(l []*Channel) {
	sort.SliceStable(l, func(i, j int) bool {
		a, ea := strconv.ParseFloat(l[i].Number, 64)
		b, eb := strconv.ParseFloat(l[j].Number, 64)
		switch {
		case ea == nil && eb == nil:
			return a < b
		case ea == nil:
			return true
		case eb == nil:
			return false
		}
		return l[i].Number < l[j].Number
	})
}

// channelsLife is how long WittyTuner's channels are kept before they're asked for again.
const channelsLife = 2 * time.Second

// allChannels is every channel of every tuner, from WittyTuner: none when it isn't there.
func (m *Manager) allChannels(ctx context.Context, fresh bool) ([]*Channel, map[int64]*Channel, error) {
	m.chMu.Lock()
	defer m.chMu.Unlock()
	if !fresh && m.chByID != nil && time.Since(m.chAt) < channelsLife {
		return m.chList, m.chByID, nil
	}
	if directory == nil {
		return nil, map[int64]*Channel{}, ErrNoEngine
	}
	list, err := directory.Channels(ctx)
	if err != nil {
		return nil, map[int64]*Channel{}, err
	}
	by := make(map[int64]*Channel, len(list))
	for _, c := range list {
		c.Logo = c.LogoURL
		by[c.ID] = c
	}
	m.chList, m.chByID, m.chAt = list, by, time.Now()
	return list, by, nil
}

// ForgetChannels has the next question about channels go to WittyTuner, for a change made there.
func (m *Manager) ForgetChannels() {
	m.chMu.Lock()
	m.chAt = time.Time{}
	m.chMu.Unlock()
}

// copyChannel is a channel the caller may change.
func copyChannel(c *Channel) *Channel {
	cp := *c
	return &cp
}

// AllChannels lists every channel of every tuner, enabled or not, for recordings and sets.
func (m *Manager) AllChannels(ctx context.Context) ([]*Channel, error) {
	list, _, err := m.allChannels(ctx, false)
	out := make([]*Channel, 0, len(list))
	for _, c := range list {
		out = append(out, copyChannel(c))
	}
	return out, err
}

// GuideChannels lists every channel that can appear in the guide, across
// enabled tuners. Encrypted channels stay, so people can see why they do not play.
func (m *Manager) GuideChannels(ctx context.Context) ([]*Channel, error) {
	list, _, err := m.allChannels(ctx, false)
	if err != nil {
		if errors.Is(err, ErrNoEngine) || errors.Is(err, errModuleMissing) {
			return []*Channel{}, nil
		}
		return nil, err
	}
	out := []*Channel{}
	for _, c := range list {
		if c.Enabled && c.TunerEnabled && c.MergedInto == 0 {
			out = append(out, copyChannel(c))
		}
	}
	return out, nil
}

// errModuleMissing is what a Directory says when WittyTuner is absent or off.
var errModuleMissing = errors.New("WittyTuner isn't installed and on")

// ModuleMissing marks an error as WittyTuner being absent or off.
func ModuleMissing(err error) error { return fmt.Errorf("%w: %w", errModuleMissing, err) }

// Channel returns one channel.
func (m *Manager) Channel(ctx context.Context, id int64) (*Channel, error) {
	_, by, err := m.allChannels(ctx, false)
	c := by[id]
	if c == nil && err == nil {
		// A channel just added: ask again rather than wait.
		_, by, err = m.allChannels(ctx, true)
		c = by[id]
	}
	if c == nil {
		if err != nil && !errors.Is(err, ErrNoEngine) && !errors.Is(err, errModuleMissing) {
			return nil, err
		}
		return nil, ErrNotFound
	}
	return copyChannel(c), nil
}

// ErrNotFound: no such channel or recording.
var ErrNotFound = errors.New("not found")

// What a channel's last check or tune found, as WittyTuner keeps it.
const (
	HealthOK      = "ok"
	HealthFailing = "failing"
)

// --------------------------------------------------------------------------
// Opening streams
// --------------------------------------------------------------------------

// ErrBusy: every tuner of a source is in use.
var ErrBusy = tuner.ErrBusy

// Opened is a stream with the tuner slot it holds (WittyTuner's, let go when the stream closes).
type Opened struct {
	Stream  *tuner.Stream
	Channel *Channel
	note    func(encryptedWhy string)
	once    sync.Once
	// A stream that was clear on opening can turn out not to be: see watch.
	stopWatch  context.CancelFunc
	gate       *hlsGate // what FFmpeg reads a live stream through
	refused    chan struct{}
	refuseOnce sync.Once
	refusal    error
}

// Refused is closed when a stream that was playing was found to be encrypted. Whatever
// is using it stops, and Refusal says why. nil for a stream that can't be looked at again.
func (o *Opened) Refused() <-chan struct{} { return o.refused }

// Refusal is why the stream was refused, or nil.
func (o *Opened) Refusal() error {
	select {
	case <-o.refused:
		return o.refusal
	default:
		return nil
	}
}

func (o *Opened) refuse(why string) {
	o.refuseOnce.Do(func() {
		o.refusal = &tuner.EncryptedError{Channel: o.Channel.Name, Why: why}
		o.note(why)
		close(o.refused)
	})
}

// CheckNow looks at the stream again straight away, for when it has just stopped by
// itself, and says if it was refused.
func (o *Opened) CheckNow(ctx context.Context) error {
	if o.Stream.Recheck != nil && o.refused != nil {
		if why, _, err := o.Stream.Recheck(ctx); err == nil && why != "" {
			o.refuse(why)
		}
	}
	return o.Refusal()
}

// watch looks at a playing stream again for as long as it plays. Encrypted is final; not
// being able to find out says nothing, so a clear stream goes on through a network failure.
func (o *Opened) watch(ctx context.Context, log *slog.Logger) {
	for {
		why, next, err := o.Stream.Recheck(ctx)
		if ctx.Err() != nil {
			return
		}
		if why != "" {
			log.WarnContext(ctx, "a playing stream turned out to be encrypted; stopping it", "why", why)
			o.refuse(why)
			return
		}
		if err != nil {
			log.DebugContext(ctx, "could not look at a stream's playlist again", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(max(next, time.Millisecond)):
		}
	}
}

// Release frees the tuner, and remembers what the stream showed about itself.
func (o *Opened) Release() {
	o.once.Do(func() {
		if o.stopWatch != nil {
			o.stopWatch()
		}
		if g := o.Stream.Guard; g != nil {
			switch {
			case g.Fatal() != nil:
				o.note(tuner.ScrambledReason)
			case g.Verified():
				o.note("")
			}
		}
		_ = o.Stream.Close()
		o.gate.Close()
	})
}

// Open starts a channel, within its tuner's limit, through WittyTuner. Guide data found in the
// stream is stored as it goes by. When the channel won't open and has backups (the same station
// from other sources), WittyTuner tries them in turn; the one that opened is the answer's Channel.
func (m *Manager) Open(ctx context.Context, channelID int64) (*Opened, error) {
	return m.openWith(ctx, OpenRequest{Channel: channelID, Backups: true})
}

// ErrNoArchive: what was asked for isn't in the source's archive.
var ErrNoArchive = errors.New("that isn't in this channel's archive")

// OpenArchive plays a stretch of what a channel showed, from its source's archive (catch-up),
// within the same limits and checks as watching it live. Backups aren't tried: another
// source's archive needn't hold the same programme at the same time.
func (m *Manager) OpenArchive(ctx context.Context, channelID, start, end int64) (*Opened, error) {
	c, err := m.Channel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if c.CatchupDays <= 0 || start <= 0 || start > now || start < now-int64(c.CatchupDays)*86400 || (end != 0 && end <= start) {
		return nil, ErrNoArchive
	}
	return m.openWith(ctx, OpenRequest{Channel: channelID, ArchiveStart: start, ArchiveEnd: end})
}

func (m *Manager) openWith(ctx context.Context, req OpenRequest) (*Opened, error) {
	c, err := m.Channel(ctx, req.Channel)
	if err != nil {
		return nil, err
	}
	if c.Encrypted != "" {
		return nil, &tuner.EncryptedError{Channel: c.Name, Why: c.Encrypted}
	}
	if directory == nil {
		return nil, ErrNoEngine
	}
	got, err := directory.Open(ctx, req)
	if err != nil {
		if tuner.IsEncrypted(err) {
			m.ForgetChannels() // WittyTuner has marked it
		}
		m.noteOpenFailure(ctx, c, err)
		return nil, err
	}
	st := got.Stream
	if got.Channel != c.ID {
		if b, berr := m.Channel(ctx, got.Channel); berr == nil {
			c = b
		}
	}
	if st.Guard != nil {
		st.Guard.OnEvent = m.broadcastEvents(c)
	}
	m.Log.DebugContext(ctx, "channel opened", "module", "wittytuner", "topic", "tuner", "tuner", c.TunerID, "channel", c.Name)
	id := c.ID
	note := func(why string) { m.noteStream(id, why) }
	if st.Verified {
		note("")
	}
	o := &Opened{Stream: st, Channel: c, note: note}
	home := got.Home
	addr, gated := gateInput(st)
	// What FFmpeg would read by itself, a DASH manifest being the one kind left, could lead it
	// anywhere. From a list on the internet, that isn't on offer.
	if !home && !gated && st.Reader == nil {
		_ = st.Close()
		return nil, errors.New("this channel is a kind of stream that can't be played from a playlist on the internet. To use it, keep the playlist on your home network, or add it as a single stream")
	}
	if gated {
		g, err := newHLSGate(addr, st.Input, home, m.tunerLog(), o.refuse)
		if err != nil {
			_ = st.Close()
			return nil, err
		}
		if !home {
			g.client = m.internetClient()
		}
		o.gate, o.refused = g, make(chan struct{})
		st.Input = g.args(st.Input)
	}
	if st.Recheck != nil {
		wctx, cancel := context.WithCancel(ctx)
		o.stopWatch = cancel
		log := m.tunerLog().With("topic", "tuner", "tuner", c.TunerID, "channel", c.Name)
		// It ends with the stream, or with the manager.
		m.background("stream re-check", func(stop context.Context) {
			defer context.AfterFunc(stop, cancel)()
			o.watch(wctx, log)
		}, "cat", "tv")
	}
	return o, nil
}

// noteOpenFailure leaves a record of a tuner that wouldn't give a channel.
// Being busy, or the caller leaving, isn't a fault; a refusal is only worth a note.
func (m *Manager) noteOpenFailure(ctx context.Context, c *Channel, err error) {
	log := m.tunerLog().With("topic", "tuner", "tuner", c.TunerID, "channel", c.Name)
	switch {
	case ctx.Err() != nil, errors.Is(err, ErrBusy):
		log.DebugContext(ctx, "channel not opened", "err", err)
	case tuner.IsEncrypted(err):
		log.InfoContext(ctx, "channel refused: encrypted", "err", err)
	default:
		log.WarnContext(ctx, "could not open a tuner channel", "err", err)
	}
}

// noteStream tells WittyTuner what a stream showed, so the guide and settings are right
// from then on: why it is encrypted, or "" for clear.
func (m *Manager) noteStream(id int64, why string) {
	if directory == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := directory.Note(ctx, id, why); err != nil {
		m.Log.Warn("could not save what a stream showed", "channel", id, "err", err)
		return
	}
	m.ForgetChannels()
	if why != "" {
		m.Log.Info("channel marked as encrypted", "module", "wittytuner", "topic", "tuner", "channel", id, "why", why)
	}
}

// Alternative is a clear way to watch a station that is encrypted here.
type Alternative struct {
	Key    string `json:"key"`
	Number string `json:"number"`
	Name   string `json:"name"`
	ATSC3  bool   `json:"atsc3,omitempty"`
	// CanRecord: whether the profile asking may record it. The server fills it in.
	CanRecord bool `json:"canRecord"`
}

var stationSuffix = []string{"atsc30", "atsc3", "nextgen", "nextgentv", "ngtv", "4k", "hd", "dt"}

// stationKey lets WTST, WTST-HD and "WTST ATSC3" count as one station.
func stationKey(name string) string {
	k := normalizeName(name)
	for changed := true; changed; {
		changed = false
		for _, s := range stationSuffix {
			if len(k) > len(s)+1 && strings.HasSuffix(k, s) {
				k, changed = strings.TrimSuffix(k, s), true
			}
		}
	}
	return k
}

// overTheAir: a virtual channel number means the same station on these.
func overTheAir(driver string) bool { return driver == "network" || driver == "dvb" }

// Alternatives finds, for each channel that needs one, the same station from a
// clear source: an ATSC 1.0 version, or another tuner or playlist. It matches
// by call sign and name, and by number over the air, where a number names a
// station. It never offers a channel not known to be watchable. need says which
// channels want one, besides the encrypted: those that would play without sound.
func Alternatives(list []*Channel, need func(*Channel) bool) map[int64]*Alternative {
	var byName, byNumber = map[string][]*Channel{}, map[string][]*Channel{}
	for _, c := range list {
		if c.Security == SecurityEncrypted || !c.Enabled || (need != nil && need(c)) {
			continue
		}
		if k := stationKey(firstNonEmpty(c.CallSign, c.Name)); k != "" {
			byName[k] = append(byName[k], c)
		}
		if overTheAir(c.Driver) && c.Number != "" {
			byNumber[c.Number] = append(byNumber[c.Number], c)
		}
	}
	best := func(cands []*Channel, me *Channel) *Channel {
		var pick *Channel
		for _, d := range cands {
			if d.ID == me.ID {
				continue
			}
			// Sound and picture every client can have comes first.
			if pick == nil || (pick.ATSC3 && !d.ATSC3) {
				pick = d
			}
		}
		return pick
	}
	out := map[int64]*Alternative{}
	for _, c := range list {
		if c.Security != SecurityEncrypted && (need == nil || !need(c)) {
			continue
		}
		d := best(byName[stationKey(firstNonEmpty(c.CallSign, c.Name))], c)
		if d == nil && overTheAir(c.Driver) {
			d = best(byNumber[c.Number], c)
		}
		if d != nil {
			out[c.ID] = &Alternative{Key: "tun-" + strconv.FormatInt(d.ID, 10), Number: d.Number, Name: d.Name, ATSC3: d.ATSC3}
		}
	}
	return out
}

// RegisterViewer records how to stop the live stream on a channel, so that a
// recording can take its tuner.
func (m *Manager) RegisterViewer(channelID int64, stop func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.viewers[channelID] = stop
}

// ForgetViewer undoes RegisterViewer.
func (m *Manager) ForgetViewer(channelID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.viewers, channelID)
}

// freeTuner stops a live viewer on a tuner, because a recording needs it.
func (m *Manager) freeTuner(ctx context.Context, tunerID int64) bool {
	_, by, _ := m.allChannels(ctx, false)
	m.mu.Lock()
	var stop func()
	var who int64
	for cid, f := range m.viewers {
		if c := by[cid]; c != nil && c.TunerID == tunerID {
			stop, who = f, cid
			break
		}
	}
	if stop != nil {
		delete(m.viewers, who)
	}
	m.mu.Unlock()
	if stop == nil {
		return false
	}
	stop()
	return true
}
