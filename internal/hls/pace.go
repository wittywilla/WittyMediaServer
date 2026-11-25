// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package hls

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	machinepace "github.com/wittywilla/wittymediaserver/internal/pace"
)

// A player at 2× uses up what was made ahead twice as fast, so how far ahead FFmpeg keeps
// going, and how near the player may come before a resting FFmpeg starts again, are counted
// in the player's time: both grow with its rate. Players say their rate (SetRate); a stream
// nobody said anything about is at 1×.
//
// The machine may not keep up at all: a small NAS converting a 4K film is slower than real
// time even at 1×. Each job measures its own speed (media seconds made per second), and a
// stream that falls behind the player while the player waits on it is restarted once, with a
// faster encoder setting, from the next segment it needs. If that isn't enough either, Work
// says so (KeepingUp false, Fastest true), and the player steps down to a lower quality or
// tells the viewer, rather than stalling every few seconds.

const (
	minRate = 0.25
	maxRate = 16
	// paceWindow is how long a job's speed is measured over; paceSettle is how long after it
	// starts before it is measured at all (FFmpeg's start, and the seek, are not its pace).
	paceWindow = 20 * time.Second
	paceSettle = 6 * time.Second
	// keepUpMargin: a job is keeping up when it makes at least this much more than the player
	// uses. Exactly the player's rate leaves no room for a moment's hiccup.
	keepUpMargin = 1.1
	// maxPaceStep is the last of the faster settings (fasterPreset).
	maxPaceStep = 1
)

// SetRate records the player's playback rate (1 = normal speed) for this stream and the
// other rungs of its ladder. Rates out of range are clamped; 0 or less is ignored (paused).
func (s *Session) SetRate(r float64) {
	if r <= 0 || math.IsNaN(r) || math.IsInf(r, 0) {
		return
	}
	r = min(max(r, minRate), maxRate)
	s.setRate(r)
	if s.Spec.Group != "" {
		for _, o := range s.m.group(s.Spec.Group) {
			if o != s {
				o.setRate(r)
			}
		}
	}
}

func (s *Session) setRate(r float64) {
	old := s.Rate()
	s.rate.Store(math.Float64bits(r))
	s.mu.Lock()
	started := !s.lastStart.IsZero()
	s.mu.Unlock()
	if r > old*1.2 && started {
		// Faster: the lead that was enough may not be now. A resting stream starts again if what
		// is made falls short of the new lead (wake looks), as it would at the next segment.
		// A stream nothing has asked for yet waits for its first request.
		if n := int(s.lastAsked.Load()); n >= 0 && !s.Spec.Growing && s.SegmentCount() > 0 {
			go s.wake(n)
		}
	}
}

// Rate is the player's playback rate, 1 when no player has said.
func (s *Session) Rate() float64 {
	if b := s.rate.Load(); b != 0 {
		return math.Float64frombits(b)
	}
	return 1
}

// wakeAhead is how many segments ahead of the player a resting FFmpeg starts again: N seconds
// of the player's time, whatever its rate. Slower than 1× still keeps the 1× lead.
func (s *Session) wakeAhead() int {
	return int(math.Ceil(float64(wakeSegs) * max(1, s.Rate())))
}

// restAhead is how far ahead FFmpeg goes before it rests: as much further than wakeAhead as at
// 1×, so a rest always has room before the player comes near.
func (s *Session) restAhead() int {
	return s.wakeAhead() + maxAheadSegs - wakeSegs
}

// pace is one job's measured speed: samples of the newest finished segment over time.
type pace struct {
	mu      sync.Mutex
	started time.Time
	at      []time.Time
	seg     []int64
	speed   float64 // media seconds made per second, over the window; 0 = not measured yet
}

// note records that segments up to last are finished now, and works out the speed.
func (p *pace) note(now time.Time, last int64, segSec float64) float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if now.Sub(p.started) < paceSettle {
		return p.speed
	}
	if n := len(p.seg); n == 0 || p.seg[n-1] != last {
		p.at, p.seg = append(p.at, now), append(p.seg, last)
	}
	// Keep the window, plus the sample just before it, so a slow job that made one segment in
	// the whole window still has two points.
	for len(p.at) > 2 && now.Sub(p.at[1]) > paceWindow {
		p.at, p.seg = p.at[1:], p.seg[1:]
	}
	if len(p.at) < 2 {
		// Nothing finished since it settled. Once a window has gone by with nothing, it is slower
		// than one segment per window.
		if len(p.at) == 1 && now.Sub(p.at[0]) > paceWindow {
			p.speed = segSec / now.Sub(p.at[0]).Seconds()
		}
		return p.speed
	}
	// From the first sample to now: a job that has stopped making segments slows down as the
	// wait for the next one grows, not only when that one comes.
	span := now.Sub(p.at[0]).Seconds()
	if span < paceSettle.Seconds() {
		return p.speed
	}
	p.speed = float64(p.seg[len(p.seg)-1]-p.seg[0]) * segSec / span
	return p.speed
}

func (p *pace) get() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.speed
}

// Pace is how a stream's conversion is keeping up, for players and the log.
type Pace struct {
	// Speed is how many seconds of the film the latest job makes per second; 0 = not measured yet.
	Speed float64 `json:"speed"`
	// Rate is the player's playback rate as it last said.
	Rate float64 `json:"rate"`
	// LeadSec is how far past the player's newest request the stream is made, in film seconds.
	LeadSec float64 `json:"leadSec"`
	// KeepingUp: the conversion is at least as fast as the player, with a margin, or rests
	// because it is far enough ahead. False only once measured.
	KeepingUp bool `json:"keepingUp"`
	// Fastest: the faster setting is already in use, so only a lower quality would help.
	Fastest bool `json:"fastest"`
}

// Pace says how the stream's conversion is keeping up with its player.
func (s *Session) Pace() Pace {
	s.mu.Lock()
	j := s.job
	s.mu.Unlock()
	p := Pace{Rate: s.Rate(), KeepingUp: true, Fastest: s.step() >= maxPaceStep}
	if s.Spec.Growing || s.Spec.CopyVideo {
		return p
	}
	asked := s.lastAsked.Load()
	if j != nil {
		p.LeadSec = max(0, float64(j.lastSeg.Load()-asked)*s.Spec.SegmentSec)
		if !j.finished() {
			p.Speed = math.Round(j.pace.get()*100) / 100
			p.KeepingUp = p.Speed == 0 || p.Speed >= p.Rate*keepUpMargin || p.LeadSec >= float64(s.wakeAhead())*s.Spec.SegmentSec
		}
	}
	return p
}

// checkPace is watch's look at a running job's speed: a job that is slower than the player
// while the player is close behind it is restarted with the faster setting, once.
func (s *Session) checkPace(j *job, now time.Time) {
	if s.Spec.HoldLastFrame || s.Spec.CopyVideo || s.Spec.LowLatency {
		return
	}
	speed := j.pace.note(now, j.lastSeg.Load(), s.Spec.SegmentSec)
	if speed == 0 {
		return
	}
	rate := s.Rate()
	lead := j.lastSeg.Load() - s.lastAsked.Load()
	if speed >= rate*keepUpMargin || lead >= int64(s.wakeAhead()) {
		return
	}
	if !j.slowSince.IsZero() && now.Sub(j.slowSince) < paceWindow/2 {
		return
	}
	if j.slowSince.IsZero() {
		// Behind: say so once, and give it half a window more before acting, so a moment's
		// hiccup (another stream starting, a scan) doesn't change anything.
		j.slowSince = now
		s.logger().Info("the conversion is slower than the player", "speed", round2(speed), "rate", rate, "aheadSegments", lead, "step", s.step())
		return
	}
	step := s.step()
	if step >= maxPaceStep || !s.fasterHelps() {
		return
	}
	if !s.paceStep.CompareAndSwap(step, step+1) {
		return
	}
	s.m.neededFaster.Store(true) // the next streams here start with it
	from := int(j.lastSeg.Load()) + 1
	s.logger().Warn("the conversion can't keep up with the player: going on with a faster setting", "speed", round2(speed), "rate", rate, "fromSegment", from, "preset", s.preset())
	go func() {
		if _, err := s.ensureRestart(from); err != nil {
			s.logFailedStart("could not restart a transcode with a faster setting", err)
		}
	}()
}

// ensureRestart stops the running job and starts one at segment from, with what the stream's
// settings are now.
func (s *Session) ensureRestart(from int) (*job, error) {
	s.mu.Lock()
	j := s.job
	if j == nil || j.finished() || from >= s.SegmentCount() {
		s.mu.Unlock()
		return nil, nil
	}
	old := s.stopLocked()
	s.mu.Unlock()
	old.waitGone(exitWait)
	return s.startFor(context.Background(), from, from)
}

// fasterHelps: the faster setting changes something for this stream. A hardware encoder has
// presets of its own and the expert options are the owner's; libx264 is where the time goes.
func (s *Session) fasterHelps() bool {
	return len(s.Spec.Quality.VideoArgs) == 0 && s.m.chooseHW(s.Spec.Quality, s.Spec.Preset, s.fallback.Load(), hwSourceOf(s.Spec)).plan.Name == "libx264"
}

// fastStartRate: on a small machine a player this fast starts with the faster setting.
const fastStartRate = 1.5

// step is the faster setting this stream's jobs use: the steps it took itself, or, on a small
// machine, the first one from the start for a player at 1.5× or more, or once any stream on this
// server has needed it. Going faster half-way costs a stall when the lead is short; a NAS that
// needed it once needs it again.
func (s *Session) step() int32 {
	st := s.paceStep.Load()
	if st == 0 && s.m.small() && (s.Rate() >= fastStartRate || s.m.neededFaster.Load()) && s.fasterHelps() {
		return 1
	}
	return st
}

// x264Presets from slowest to fastest.
var x264Presets = []string{"placebo", "veryslow", "slower", "slow", "medium", "fast", "faster", "veryfast", "superfast", "ultrafast"}

// fasterPreset is the libx264 preset steps faster than p ("" is veryfast, the default).
func fasterPreset(p string, steps int) string {
	if p == "" {
		p = "veryfast"
	}
	i := slices.Index(x264Presets, p)
	if i < 0 {
		return p
	}
	return x264Presets[min(i+steps, len(x264Presets)-1)]
}

// preset is the libx264 preset this stream's next job uses: its own, or the profile's, made
// faster for a stream that couldn't keep up. "" leaves it to the profile.
func (s *Session) preset() string {
	if s.step() == 0 {
		return s.Spec.Preset
	}
	p := s.Spec.Preset
	if p == "" {
		p = s.Spec.Quality.Preset
	}
	// Two steps: veryfast to ultrafast, about two and a half times faster again for libx264.
	return fasterPreset(p, 2)
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }

// small says the machine is tiny or small (internal/pace: up to four processors or 4 GB, counting a
// container's limits). Tests set the manager's own answer (smallMachine) rather than a package
// variable, which streams' own goroutines read.
func (m *Manager) small() bool {
	if m.smallMachine != nil {
		return m.smallMachine()
	}
	return machinepace.Machine() != machinepace.Roomy
}

// softwareMaxHeight is the most lines a conversion on the processor makes on a small machine.
const softwareMaxHeight = 1080

// CapForMachine brings a conversion that the processor would do at more than 1080 lines down to
// 1080 on a small machine, and says whether it did. A NAS with two cores converts 4K far slower than
// it plays, and the pictures of a 4K decoder and encoder together outgrow a 1 GB container: FFmpeg was
// killed on the first segment, and the player stopped. A graphics chip that encodes is left to it.
func (m *Manager) CapForMachine(sp *Spec) bool {
	if sp.CopyVideo || sp.NoVideo || sp.Height <= softwareMaxHeight+8 || (sp.MaxHeight > 0 && sp.MaxHeight <= softwareMaxHeight) || !m.small() {
		return false
	}
	if hardware(m.PlanWork(*sp).Encoder) {
		return false
	}
	sp.MaxHeight = softwareMaxHeight
	return true
}
