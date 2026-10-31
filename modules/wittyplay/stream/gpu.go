package stream

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// NoGPUText is in the error a runner gives when a game needs a graphics card and
// this server has none it can use.
const NoGPUText = "no usable graphics card"

// GPUProbe is what a runner found when asked which graphics card it would use.
type GPUProbe struct {
	Name  string `json:"name,omitempty"`
	Error string `json:"error,omitempty"`
}

// ProbeGPU asks the runner which graphics card games would draw with, in the same
// sandbox a game gets, so the answer is the one a game will see. A hidden window
// is the most it opens.
func ProbeGPU(runner, dir string, log *slog.Logger) GPUProbe {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return GPUProbe{Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, runner, "-probe-gpu", "-save", dir)
	cmd.Dir = dir
	cmd.Env = runnerEnv()
	var out bytes.Buffer
	cmd.Stdout = &out
	prepareSandbox(cmd, dir, log)
	if err := cmd.Run(); err != nil {
		return GPUProbe{Error: "the runner could not check: " + err.Error()}
	}
	line := strings.TrimSpace(out.String())
	if name, ok := strings.CutPrefix(line, "gpu: "); ok {
		return GPUProbe{Name: name}
	}
	return GPUProbe{Error: strings.TrimPrefix(line, "none: ")}
}

// How long frames spend in the encoder, and how many come out: what the settings
// and a log reader need in order to tell whether a game is keeping up.

type timeRing struct {
	mu   sync.Mutex
	at   [128]time.Time
	head int
	tail int
}

func (r *timeRing) push(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.at[r.head%len(r.at)] = t
	r.head++
	if r.head-r.tail > len(r.at) {
		r.tail = r.head - len(r.at)
	}
}

func (r *timeRing) pop() (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tail == r.head {
		return time.Time{}, false
	}
	t := r.at[r.tail%len(r.at)]
	r.tail++
	return t, true
}

func (e *videoEncoder) mark() { e.sent.push(time.Now()) }

// EncodeStats counts frames through the encoder.
type EncodeStats struct {
	frames  atomic.Int64
	totalUS atomic.Int64
	maxUS   atomic.Int64
}

func (st *EncodeStats) encoded(e *videoEncoder) {
	t, ok := e.sent.pop()
	if !ok {
		return
	}
	us := time.Since(t).Microseconds()
	st.frames.Add(1)
	st.totalUS.Add(us)
	for {
		m := st.maxUS.Load()
		if us <= m || st.maxUS.CompareAndSwap(m, us) {
			return
		}
	}
}

// Take returns frames encoded, the mean and the worst time from going in to coming
// out, in milliseconds, since it was last asked.
func (st *EncodeStats) Take() (frames int64, meanMS, maxMS float64) {
	frames = st.frames.Swap(0)
	total := st.totalUS.Swap(0)
	maxMS = float64(st.maxUS.Swap(0)) / 1000
	if frames > 0 {
		meanMS = float64(total) / float64(frames) / 1000
	}
	return frames, meanMS, maxMS
}

// EncodeStats is for logs and tests.
func (s *Session) EncodeStats() *EncodeStats { return &s.stats }

// logStats writes a line every half minute while a game is being watched, so a
// slow graphics card or encoder shows up in the server's log.
func (s *Session) logStats() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	last := time.Now()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			frames, mean, worst := s.stats.Take()
			if frames > 0 {
				s.log.Info("streaming", "encoder", s.Info().Encoder, "fps", float64(frames)/time.Since(last).Seconds(), "encodeMs", mean, "worstMs", worst, "renderer", s.info.Renderer)
			}
			last = time.Now()
		}
	}
}
