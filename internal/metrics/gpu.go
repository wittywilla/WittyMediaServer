package metrics

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GPU is a graphics card, as far as its driver tells. Nil numbers aren't known.
type GPU struct {
	Name           string   `json:"name"`
	Percent        *float64 `json:"percent,omitempty"`
	EncoderPercent *float64 `json:"encoderPercent,omitempty"`
	DecoderPercent *float64 `json:"decoderPercent,omitempty"`
	MemoryUsed     *uint64  `json:"memoryUsed,omitempty"`
	MemoryTotal    *uint64  `json:"memoryTotal,omitempty"`
	Celsius        *float64 `json:"celsius,omitempty"`
	EncoderCount   *int     `json:"encoderSessions,omitempty"`
	EncoderFPS     *float64 `json:"encoderFps,omitempty"`
}

// How often a card is asked. nvidia-smi is a program of its own and takes a moment, so
// it's only ever run while someone has this on screen, and no more than this often.
const gpuEvery = 5 * time.Second

// GPUReader asks the graphics cards, at most every few seconds.
type GPUReader struct {
	mu   sync.Mutex
	at   time.Time
	last []GPU
	smi  string
	once sync.Once
}

// Read gives what's known, asking again if it's been a while.
func (g *GPUReader) Read(ctx context.Context) []GPU {
	g.mu.Lock()
	defer g.mu.Unlock()
	if time.Since(g.at) < gpuEvery {
		return g.last
	}
	g.at = time.Now()
	g.once.Do(func() { g.smi, _ = exec.LookPath("nvidia-smi") })
	var out []GPU
	if g.smi != "" {
		c, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		b, err := exec.CommandContext(c, g.smi, "--query-gpu=name,utilization.gpu,utilization.encoder,utilization.decoder,memory.used,memory.total,temperature.gpu,encoder.stats.sessionCount,encoder.stats.averageFps", "--format=csv,noheader,nounits").Output()
		if err == nil {
			out = ParseNvidiaSMI(string(b))
		}
	}
	out = append(out, readSysfsGPUs()...)
	g.last = out
	return out
}

func numberOf(s string) *float64 {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil // "[N/A]", "[Not Supported]"
	}
	return &v
}

// ParseNvidiaSMI reads nvidia-smi's CSV. Memory comes in MiB.
func ParseNvidiaSMI(text string) []GPU {
	var out []GPU
	for _, line := range strings.Split(text, "\n") {
		f := strings.Split(strings.TrimSpace(line), ",")
		if len(f) < 9 {
			continue
		}
		g := GPU{Name: strings.TrimSpace(f[0]), Percent: numberOf(f[1]), EncoderPercent: numberOf(f[2]), DecoderPercent: numberOf(f[3]), Celsius: numberOf(f[6]), EncoderFPS: numberOf(f[8])}
		mib := func(s string) *uint64 {
			if v := numberOf(s); v != nil {
				b := uint64(*v) << 20
				return &b
			}
			return nil
		}
		g.MemoryUsed, g.MemoryTotal = mib(f[4]), mib(f[5])
		if v := numberOf(f[7]); v != nil {
			n := int(*v)
			g.EncoderCount = &n
		}
		out = append(out, g)
	}
	return out
}

// readSysfsGPUs reads what Linux's AMD driver offers in files. Elsewhere, and for other
// cards, there's nothing there and this finds nothing.
func readSysfsGPUs() []GPU {
	busy, _ := filepath.Glob("/sys/class/drm/card*/device/gpu_busy_percent")
	var out []GPU
	for _, p := range busy {
		dir := filepath.Dir(p)
		num := func(name string) *float64 {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				return nil
			}
			return numberOf(string(b))
		}
		bytesOf := func(name string) *uint64 {
			if v := num(name); v != nil {
				n := uint64(*v)
				return &n
			}
			return nil
		}
		g := GPU{Name: "AMD " + filepath.Base(filepath.Dir(dir)), Percent: num("gpu_busy_percent"), MemoryUsed: bytesOf("mem_info_vram_used"), MemoryTotal: bytesOf("mem_info_vram_total")}
		out = append(out, g)
	}
	return out
}
