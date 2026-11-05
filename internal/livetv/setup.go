package livetv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/tuner"
)

// The guided setup in Settings asks two things the rest of this package doesn't: what a source
// says before it's saved, and whether one of its channels really plays. Both go through
// WittyTuner and Open like everything else, so a source added by hand meets the same checks as
// one a search found, and an encrypted channel is refused here as it is everywhere.

// CheckResult is what a source said when asked for its channels, before it was saved.
type CheckResult struct {
	Detail     string `json:"detail,omitempty"`     // what it says it is: its model and address
	MaxStreams int    `json:"maxStreams,omitempty"` // tuners in the device, or streams the provider allows
	Channels   int    `json:"channels"`
	Encrypted  int    `json:"encrypted"` // marked as protected by the source itself
	// Sample is a few channel names, so that people can see it's the source they meant.
	Sample   []string `json:"sample,omitempty"`
	OwnGuide bool     `json:"ownGuide,omitempty"` // it names guide data of its own
}

// Problem is why a source didn't check out, in words for the page.
type Problem struct {
	// Kind is settings, unreachable, not-a-tuner, unreadable, empty, encrypted, busy or other.
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
	// HelpURL is where encrypted channels are explained.
	HelpURL string `json:"helpUrl,omitempty"`
}

// stageError says which part of a check failed: the settings, or reading the channels.
type stageError struct {
	settings bool
	err      error
}

func (e *stageError) Error() string { return e.err.Error() }
func (e *stageError) Unwrap() error { return e.err }

// Check makes a source from these settings and asks it for its channels, without keeping
// anything. A source that only finds channels by scanning answers with none, which isn't a fault.
func (m *Manager) Check(ctx context.Context, driver, name string, options map[string]string) (*CheckResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "New source"
	}
	s, err := m.newSource(ctx, driver, tuner.Config{Name: name, Options: cloneMap(options)})
	if err != nil {
		return nil, &stageError{settings: true, err: err}
	}
	defer s.Close()
	list, err := s.Channels(ctx)
	if err != nil {
		return nil, &stageError{err: err}
	}
	info := s.Info()
	out := &CheckResult{Detail: info.Detail, MaxStreams: info.MaxStreams, Channels: len(list), OwnGuide: webGuide(info.GuideURL)}
	for _, c := range list {
		if c.Encrypted != "" {
			out.Encrypted++
		} else if len(out.Sample) < 5 && c.Name != "" {
			out.Sample = append(out.Sample, c.Name)
		}
	}
	return out, nil
}

// addressed drivers reach a device or server at an address someone gave.
func addressed(driver string) bool {
	switch driver {
	case "network", "satip", "backend":
		return true
	}
	return false
}

// CheckProblem turns a failed check, or a source that couldn't read its list once saved, into
// words for the page. What a connection said stays out: it names what's on the network.
func CheckProblem(driver string, err error) Problem {
	var st *stageError
	settings := errors.As(err, &st) && st.settings
	text := PublicError(err)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return Problem{Kind: "unreachable", Message: "Nothing answered in time.", Hint: "Check that it's switched on and on the same network as this server, then try again."}
	case tuner.IsEncrypted(err):
		return Problem{Kind: "encrypted", Message: err.Error()}
	case errors.Is(err, tuner.ErrBusy):
		return Problem{Kind: "busy", Message: "Every tuner in this device is in use.", Hint: "Stop watching or recording on it, then try again."}
	case settings:
		return Problem{Kind: "settings", Message: sentence(text)}
	case errors.Is(err, tuner.ErrUnreachable) || text == "the stream could not be reached":
		if addressed(driver) {
			return Problem{Kind: "unreachable", Message: "Nothing answered at that address.", Hint: "Check the address, that the device is switched on, and that it's on the same network as this server."}
		}
		if driver == "iptv" {
			return Problem{Kind: "unreachable", Message: "The playlist's address didn't answer.", Hint: "Check the address, and that this server can reach the internet or the machine it's on."}
		}
		return Problem{Kind: "unreachable", Message: "It couldn't be reached.", Hint: "Check the address and that it's switched on."}
	case addressed(driver):
		return Problem{Kind: "not-a-tuner", Message: "Something answered at that address, but not as this kind of device: " + text + ".",
			Hint: "Check the address is the device's own, and that you chose the right kind of device. A router or a computer's web page answers too, but isn't a tuner."}
	case driver == "iptv" || driver == "stream":
		return Problem{Kind: "unreadable", Message: sentence(text), Hint: "Check the address or the file. A playlist is an M3U or M3U8 file: a web page about one isn't the playlist itself."}
	}
	return Problem{Kind: "other", Message: sentence(text)}
}

func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "It didn't work, and gave no reason."
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}

// TestChannel is the channel a test tuned.
type TestChannel struct {
	ID     int64  `json:"id"`
	Number string `json:"number"`
	Name   string `json:"name"`
}

// TestResult is what tuning one channel for a few seconds showed.
type TestResult struct {
	OK      bool         `json:"ok"`
	Channel *TestChannel `json:"channel,omitempty"`
	// Problem is why it didn't play, when it didn't.
	Problem *Problem `json:"problem,omitempty"`
	// Seconds is how long it took to get the first of the stream.
	Seconds float64 `json:"seconds"`
	Video   string  `json:"video,omitempty"` // such as "H.264, 1920×1080"
	Audio   string  `json:"audio,omitempty"`
	// Clear: the server's own guard watched enough of it to know it isn't encrypted.
	Clear bool `json:"clear,omitempty"`
}

// How long a test listens, and how much of the stream it keeps to look at.
const (
	testListen   = 8 * time.Second
	testMaxBytes = 4 << 20
)

// pickTestChannel is the first channel in the guide that isn't known to be encrypted, TV before radio.
func pickTestChannel(list []*Channel) (*Channel, int) {
	var radio *Channel
	encrypted := 0
	for _, c := range list {
		switch {
		case c.Security == SecurityEncrypted:
			encrypted++
		case !c.Enabled:
		case c.Radio:
			if radio == nil {
				radio = c
			}
		default:
			return c, encrypted
		}
	}
	return radio, encrypted
}

// TestChannel tunes one of a tuner's channels for a few seconds, as a viewer would, and says
// what arrived. channelID 0 picks one. Nothing is kept but what the stream showed about itself,
// which Open's guard notes as it always does.
func (m *Manager) TestChannel(ctx context.Context, tunerID, channelID int64) (*TestResult, error) {
	list, err := m.Channels(ctx, tunerID)
	if err != nil {
		return nil, err
	}
	var c *Channel
	if channelID != 0 {
		for _, x := range list {
			if x.ID == channelID {
				c = x
			}
		}
		if c == nil {
			return nil, ErrNotFound
		}
	} else {
		var encrypted int
		if c, encrypted = pickTestChannel(list); c == nil {
			p := Problem{Kind: "empty", Message: "There are no channels to try yet.", Hint: "Read the channel list, or scan for channels, first."}
			if encrypted > 0 {
				p = Problem{Kind: "encrypted", Message: "Every channel this source lists is encrypted, so none can be tried here.", Hint: tuner.EncryptedExplain}
			}
			return &TestResult{Problem: &p}, nil
		}
	}
	res := &TestResult{Channel: &TestChannel{ID: c.ID, Number: c.Number, Name: c.Name}}
	t0 := time.Now()
	o, err := m.Open(ctx, c.ID)
	if err != nil {
		p := testProblem(c.driver, err)
		res.Problem = &p
		return res, nil
	}
	defer o.Release()
	if o.Stream.Reader != nil {
		m.listen(ctx, o, res, t0)
	} else {
		m.probeInput(ctx, o, res, t0)
	}
	res.OK = res.Problem == nil
	return res, nil
}

// listen reads a raw stream through the guard for a few seconds, then looks at what came.
func (m *Manager) listen(ctx context.Context, o *Opened, res *TestResult, t0 time.Time) {
	f, err := os.CreateTemp("", "wms-tune-*.ts")
	if err != nil {
		res.Problem = &Problem{Kind: "other", Message: "There was nowhere to keep a few seconds of the stream."}
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	type got struct {
		n     int64
		first time.Duration
		err   error
	}
	done := make(chan got, 1)
	started := m.background("tuner test", func(stop context.Context) {
		var g got
		defer func() { done <- g }()                                     // after a fault too: nothing is left waiting
		defer context.AfterFunc(stop, func() { _ = o.Stream.Close() })() // ends the read
		buf := make([]byte, 64<<10)
		for g.n < testMaxBytes {
			n, err := o.Stream.Reader.Read(buf)
			if n > 0 {
				if g.n == 0 {
					g.first = time.Since(t0)
				}
				g.n += int64(n)
				if _, werr := f.Write(buf[:n]); werr != nil {
					err = werr
				}
			}
			if err != nil {
				if !errors.Is(err, io.EOF) {
					g.err = err
				}
				break
			}
		}
	}, "cat", "tv")
	if !started {
		res.Problem = &Problem{Kind: "other", Message: "The server is stopping."}
		return
	}
	var g got
	select {
	case g = <-done:
	case <-time.After(testListen):
		_ = o.Stream.Close() // ends the read
		g = <-done
	case <-ctx.Done():
		_ = o.Stream.Close()
		g = <-done
	}
	res.Seconds = g.first.Seconds()
	if gd := o.Stream.Guard; gd != nil {
		if gd.Fatal() != nil {
			e := tuner.RefusalMessage(o.Channel.Name, gd.Fatal())
			res.Problem = &Problem{Kind: "encrypted", Message: e.Error()}
			return
		}
		res.Clear = gd.Verified()
	}
	if g.n == 0 {
		p := Problem{Kind: "no-signal", Message: "The tuner answered, but sent nothing in " + fmt.Sprint(int(testListen.Seconds())) + " seconds.", Hint: "The signal may be too weak. Check the aerial or cable, or try another channel."}
		if g.err != nil && ctx.Err() == nil {
			p.Message = "The stream stopped before anything arrived: " + PublicError(g.err) + "."
		}
		res.Problem = &p
		return
	}
	if m.Tools == nil {
		return
	}
	_ = f.Close()
	pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if pr, err := m.Tools.ProbeFile(pctx, f.Name()); err == nil {
		describe(pr, res)
	}
	if res.Video == "" && res.Audio == "" && !o.Channel.Radio {
		res.Problem = &Problem{Kind: "no-picture", Message: "Something arrived, but no picture or sound could be found in it.", Hint: "The signal may be too weak, or the channel off the air. Try another channel."}
	}
}

// probeInput has FFmpeg open a stream it reaches itself, through the same gate a viewer's would.
func (m *Manager) probeInput(ctx context.Context, o *Opened, res *TestResult, t0 time.Time) {
	if m.Tools == nil {
		res.Problem = &Problem{Kind: "other", Message: "FFmpeg is needed to try this channel."}
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	pr, err := m.Tools.ProbeInput(pctx, o.Stream.Input)
	res.Seconds = time.Since(t0).Seconds()
	// The gate may have found keys in the playlist while FFmpeg read it.
	if rerr := o.CheckNow(ctx); rerr != nil {
		res.Problem = &Problem{Kind: "encrypted", Message: rerr.Error()}
		return
	}
	if err != nil {
		p := Problem{Kind: "no-picture", Message: "The stream couldn't be opened.", Hint: "Check that the address still works: providers change them. If it plays elsewhere, try again in a moment."}
		if errors.Is(pctx.Err(), context.DeadlineExceeded) {
			p = Problem{Kind: "unreachable", Message: "Nothing arrived from the stream in time.", Hint: "It may be off the air, or slow to start. Try again, or another channel."}
		}
		res.Problem = &p
		return
	}
	describe(pr, res)
	if res.Video == "" && res.Audio == "" {
		res.Problem = &Problem{Kind: "no-picture", Message: "The stream opened, but no picture or sound could be found in it."}
	}
}

// describe says what a probe found, in a few words.
func describe(pr *ffmpeg.Probe, res *TestResult) {
	if v := pr.Video(); v != nil {
		res.Video = codecName(v.CodecName)
		if v.Width > 0 && v.Height > 0 {
			res.Video += fmt.Sprintf(", %d×%d", v.Width, v.Height)
		}
	}
	if a := pr.Audio(); len(a) > 0 {
		res.Audio = codecName(a[0].CodecName)
		if a[0].Channels > 0 {
			res.Audio += fmt.Sprintf(", %d channels", a[0].Channels)
		}
	}
}

func codecName(c string) string {
	switch strings.ToLower(c) {
	case "h264":
		return "H.264"
	case "hevc":
		return "HEVC"
	case "mpeg2video":
		return "MPEG-2"
	case "aac":
		return "AAC"
	case "ac3":
		return "Dolby Digital (AC-3)"
	case "eac3":
		return "Dolby Digital Plus"
	case "ac4":
		return "AC-4"
	case "mp2":
		return "MPEG audio"
	case "":
		return "unknown"
	}
	return strings.ToUpper(c)
}

// testProblem is why a channel wouldn't open, in words for the page.
func testProblem(driver string, err error) Problem {
	switch {
	case tuner.IsEncrypted(err):
		return Problem{Kind: "encrypted", Message: err.Error()}
	case errors.Is(err, ErrBusy):
		return Problem{Kind: "busy", Message: "Every tuner in this device is in use.", Hint: "Stop watching or recording on it, then try again."}
	}
	p := CheckProblem(driver, err)
	if p.Kind == "not-a-tuner" || p.Kind == "settings" {
		p = Problem{Kind: "other", Message: sentence(PublicError(err))}
	}
	return p
}
