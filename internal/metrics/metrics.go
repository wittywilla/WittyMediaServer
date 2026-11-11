// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package metrics watches the machine for the statistics screen, and costs almost nothing
// when nobody is looking.
//
// There are two speeds. While someone has the screen open, a Watch keeps a sampler going
// every two seconds, and it stops when they leave. All the time, one sample a minute of a
// few cheap numbers goes into a small ring in memory, and one row an hour goes in the
// database, so charts have a history to draw when someone opens them. A minute's sample
// is a few small reads of what the system already counts, and starts no programs.
package metrics

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

const (
	liveEvery = 2 * time.Second
	liveLen   = 300 // ten minutes, while someone watches
	lowEvery  = time.Minute
	lowLen    = 24 * 60 // a day
)

// The series a history can be asked for.
const (
	CPU        = "cpu"        // percent of the whole machine
	Memory     = "memory"     // percent in use
	Swap       = "swap"       // percent in use
	NetRx      = "netRx"      // bytes a second
	NetTx      = "netTx"      //
	DiskRead   = "diskRead"   //
	DiskWrite  = "diskWrite"  //
	ProcessCPU = "processCpu" // percent of the whole machine, this server alone
	RSS        = "rss"        // bytes this server holds
	Heap       = "heap"       // bytes of that in Go's own heap
	Goroutines = "goroutines" //
	Streams    = "streams"    // people watching or listening
	Transcodes = "transcodes" // of those, being converted
	Games      = "games"      // game sessions
)

// SeriesNames is every series, in a steady order.
var SeriesNames = []string{CPU, Memory, Swap, NetRx, NetTx, DiskRead, DiskWrite, ProcessCPU, RSS, Heap, Goroutines, Streams, Transcodes, Games}

// Counts says how many of the server's own things are going on, from the server.
type Counts struct{ Streams, Transcodes, Games int }

// Store keeps an hour's summary for longer than memory does.
type Store interface {
	SaveHour(ctx context.Context, hour int64, name string, avg, peak float64) error
	Hours(ctx context.Context, from, to int64, names []string) ([]HourRow, error)
	Prune(ctx context.Context, before int64) error
}

// HourRow is one series' summary of one hour.
type HourRow struct {
	Hour      int64
	Name      string
	Avg, Peak float64
}

// CPUView is the processor, now.
type CPUView struct {
	Percent float64   `json:"percent"`
	Cores   []float64 `json:"cores"`
	Temps   []Temp    `json:"temps,omitempty"`
}

// Temp is one sensor.
type Temp struct {
	Name    string  `json:"name"`
	Celsius float64 `json:"celsius"`
}

// MemoryView is memory and swap, in bytes.
type MemoryView struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Available uint64 `json:"available"`
	SwapTotal uint64 `json:"swapTotal"`
	SwapUsed  uint64 `json:"swapUsed"`
}

// NetView is one network interface. Speeds are bytes a second.
type NetView struct {
	Name      string  `json:"name"`
	Up        bool    `json:"up"`
	SpeedMbps int64   `json:"speedMbps,omitempty"`
	RxRate    float64 `json:"rxRate"`
	TxRate    float64 `json:"txRate"`
	RxBytes   uint64  `json:"rxBytes"`
	TxBytes   uint64  `json:"txBytes"`
	RxErrs    uint64  `json:"rxErrs"`
	TxErrs    uint64  `json:"txErrs"`
}

// DiskView is one disk's speed, by the system's name for it.
type DiskView struct {
	Name        string  `json:"name"`
	ReadRate    float64 `json:"readRate"`
	WriteRate   float64 `json:"writeRate"`
	BusyPercent float64 `json:"busyPercent,omitempty"`
}

// VolumeView is somewhere files go. Only admins are given Mount, as that's a path.
type VolumeView struct {
	Mount  string `json:"mount"`
	Device string `json:"device,omitempty"`
	Type   string `json:"type,omitempty"`
	Kind   string `json:"kind"`
	Total  uint64 `json:"total"`
	Free   uint64 `json:"free"`
}

// ProcessView is this server.
type ProcessView struct {
	CPUPercent float64 `json:"cpuPercent"`
	RSS        uint64  `json:"rss"`
	HeapAlloc  uint64  `json:"heapAlloc"`
	HeapSys    uint64  `json:"heapSys"`
	Goroutines int     `json:"goroutines"`
	Threads    int     `json:"threads,omitempty"`
	Handles    int     `json:"handles,omitempty"`
	GCPauseMs  float64 `json:"gcPauseMs"`
	NumGC      uint32  `json:"numGC"`
	UptimeSec  int64   `json:"uptimeSec"`
}

// Sample is the machine at one moment. A section a system doesn't give is nil or empty.
type Sample struct {
	Time    int64        `json:"time"`
	CPU     *CPUView     `json:"cpu,omitempty"`
	Memory  *MemoryView  `json:"memory,omitempty"`
	Net     []NetView    `json:"net,omitempty"`
	Disks   []DiskView   `json:"disks,omitempty"`
	Volumes []VolumeView `json:"volumes,omitempty"`
	Process *ProcessView `json:"process,omitempty"`
}

// reader remembers the last reading of the counters, so that a speed can be worked out
// from the next. Live and low speeds each have their own.
type reader struct {
	at    time.Time
	all   sysinfo.CPUTicks
	cores []sysinfo.CPUTicks
	net   map[string]sysinfo.NetCounters
	disk  map[string]sysinfo.DiskCounters
	proc  float64
	gc    uint32
}

func pct(busy, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return min(100, float64(busy)*100/float64(total))
}

func delta(now, was uint64) uint64 {
	if now < was { // a counter that went back: a reset, or a device replaced
		return 0
	}
	return now - was
}

// read takes everything except what's slow to ask, working out speeds since the last time.
func (r *reader) read(started time.Time, full bool) Sample {
	now := time.Now()
	s := Sample{Time: now.Unix()}
	dt := now.Sub(r.at).Seconds()
	first := r.at.IsZero() || dt <= 0
	if all, cores, ok := sysinfo.ReadCPU(); ok {
		v := &CPUView{Cores: make([]float64, len(cores))}
		if !first {
			v.Percent = pct(delta(all.Busy, r.all.Busy), delta(all.Total, r.all.Total))
			for i := range cores {
				if i < len(r.cores) {
					v.Cores[i] = pct(delta(cores[i].Busy, r.cores[i].Busy), delta(cores[i].Total, r.cores[i].Total))
				}
			}
		}
		r.all, r.cores = all, cores
		s.CPU = v
	}
	if m, ok := sysinfo.ReadMemory(); ok {
		s.Memory = &MemoryView{Total: m.Total, Used: m.Total - m.Available, Available: m.Available, SwapTotal: m.SwapTotal, SwapUsed: m.SwapTotal - m.SwapFree}
	}
	nets := sysinfo.ReadNetwork()
	seenNet := make(map[string]sysinfo.NetCounters, len(nets))
	for _, n := range nets {
		v := NetView{Name: n.Name, Up: n.Up, SpeedMbps: n.SpeedMbps, RxBytes: n.RxBytes, TxBytes: n.TxBytes, RxErrs: n.RxErrs, TxErrs: n.TxErrs}
		if was, ok := r.net[n.Name]; ok && !first {
			v.RxRate = float64(delta(n.RxBytes, was.RxBytes)) / dt
			v.TxRate = float64(delta(n.TxBytes, was.TxBytes)) / dt
		}
		seenNet[n.Name] = n
		s.Net = append(s.Net, v)
	}
	r.net = seenNet
	disks := sysinfo.ReadDiskIO()
	seenDisk := make(map[string]sysinfo.DiskCounters, len(disks))
	for _, d := range disks {
		v := DiskView{Name: d.Name}
		if was, ok := r.disk[d.Name]; ok && !first {
			v.ReadRate = float64(delta(d.ReadBytes, was.ReadBytes)) / dt
			v.WriteRate = float64(delta(d.WriteBytes, was.WriteBytes)) / dt
			v.BusyPercent = min(100, float64(delta(d.BusyMs, was.BusyMs))/(dt*10))
		}
		seenDisk[d.Name] = d
		s.Disks = append(s.Disks, v)
	}
	r.disk = seenDisk
	p := &ProcessView{Goroutines: runtime.NumGoroutine(), UptimeSec: int64(now.Sub(started).Seconds())}
	if u, ok := sysinfo.ReadProcess(); ok {
		p.RSS, p.Threads, p.Handles = u.RSS, u.Threads, u.Handles
		if !first {
			p.CPUPercent = min(100, (u.CPUSec-r.proc)*100/dt/float64(max(runtime.NumCPU(), 1)))
		}
		r.proc = u.CPUSec
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	p.HeapAlloc, p.HeapSys, p.NumGC = ms.HeapAlloc, ms.HeapSys, ms.NumGC
	if ms.NumGC > 0 {
		p.GCPauseMs = float64(ms.PauseNs[(ms.NumGC+255)%256]) / 1e6
	}
	r.gc = ms.NumGC
	s.Process = p
	if full {
		for _, t := range sysinfo.ReadTemperatures() {
			if s.CPU != nil {
				s.CPU.Temps = append(s.CPU.Temps, Temp{Name: t.Name, Celsius: t.Celsius})
			}
		}
		for _, v := range sysinfo.ReadVolumes() {
			s.Volumes = append(s.Volumes, VolumeView{Mount: v.Mount, Device: v.Device, Type: v.Type, Kind: v.Kind, Total: v.Total, Free: v.Free})
		}
	}
	r.at = now
	return s
}

// point is one row of a ring: every series at one moment.
type point struct {
	t int64
	v [seriesCount]float32
}

const seriesCount = 14 // len(SeriesNames); a test holds them to it

func pointOf(s Sample, c Counts) point {
	p := point{t: s.Time}
	var i = func(name string) int {
		for n, x := range SeriesNames {
			if x == name {
				return n
			}
		}
		return 0
	}
	set := func(name string, v float64) { p.v[i(name)] = float32(v) }
	if s.CPU != nil {
		set(CPU, s.CPU.Percent)
	}
	if m := s.Memory; m != nil {
		set(Memory, pct(m.Used, m.Total))
		set(Swap, pct(m.SwapUsed, m.SwapTotal))
	}
	var rx, tx, rd, wr float64
	for _, n := range s.Net {
		rx += n.RxRate
		tx += n.TxRate
	}
	for _, d := range s.Disks {
		rd += d.ReadRate
		wr += d.WriteRate
	}
	set(NetRx, rx)
	set(NetTx, tx)
	set(DiskRead, rd)
	set(DiskWrite, wr)
	if pr := s.Process; pr != nil {
		set(ProcessCPU, pr.CPUPercent)
		set(RSS, float64(pr.RSS))
		set(Heap, float64(pr.HeapAlloc))
		set(Goroutines, float64(pr.Goroutines))
	}
	set(Streams, float64(c.Streams))
	set(Transcodes, float64(c.Transcodes))
	set(Games, float64(c.Games))
	return p
}

// ring keeps the last n points.
type ring struct {
	buf  []point
	next int
	full bool
}

func newRing(n int) *ring { return &ring{buf: make([]point, n)} }

func (r *ring) add(p point) {
	r.buf[r.next] = p
	r.next++
	if r.next == len(r.buf) {
		r.next, r.full = 0, true
	}
}

// since lists the points from a time on, oldest first.
func (r *ring) since(t int64) []point {
	var out []point
	add := func(list []point) {
		for _, p := range list {
			if p.t >= t && p.t != 0 {
				out = append(out, p)
			}
		}
	}
	if r.full {
		add(r.buf[r.next:])
	}
	add(r.buf[:r.next])
	return out
}

// Collector is the sampler. New starts the slow part; Watch the fast one.
type Collector struct {
	store   Store
	counts  func() Counts
	started time.Time

	mu       sync.Mutex
	low      *ring
	live     *ring
	latest   Sample
	watchers int
	changed  chan struct{}
	cancel   func()
	hour     hourTally
}

// New makes a collector. store and counts may be nil.
func New(store Store, counts func() Counts) *Collector {
	if counts == nil {
		counts = func() Counts { return Counts{} }
	}
	return &Collector{store: store, counts: counts, started: time.Now(), low: newRing(lowLen), live: newRing(liveLen), changed: make(chan struct{}), hour: newHourTally()}
}

// Run does the once-a-minute sampling until ctx ends.
func (c *Collector) Run(ctx context.Context) {
	var r reader
	r.read(c.started, false) // the first reading has nothing to compare with
	tick := time.NewTicker(lowEvery)
	defer tick.Stop()
	if c.store != nil {
		_ = c.store.Prune(ctx, time.Now().AddDate(0, 0, -KeepHoursDays).Unix())
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			p := pointOf(r.read(c.started, false), c.counts())
			c.mu.Lock()
			c.low.add(p)
			done := c.hour.add(p)
			c.mu.Unlock()
			c.saveHour(ctx, done)
		}
	}
}

// KeepHoursDays is how long the hourly rows are kept.
const KeepHoursDays = 90

func (c *Collector) saveHour(ctx context.Context, rows []HourRow) {
	if c.store == nil || len(rows) == 0 {
		return
	}
	for _, h := range rows {
		_ = c.store.SaveHour(ctx, h.Hour, h.Name, h.Avg, h.Peak)
	}
	_ = c.store.Prune(ctx, time.Now().AddDate(0, 0, -KeepHoursDays).Unix())
}

// hourTally adds up the current hour.
type hourTally struct {
	hour int64
	n    int
	sum  [seriesCount]float64
	peak [seriesCount]float64
}

func newHourTally() hourTally { return hourTally{} }

// add counts a point, and returns the hour just finished when this one starts a new one.
func (h *hourTally) add(p point) []HourRow {
	hour := p.t - p.t%3600
	var done []HourRow
	if h.hour != 0 && hour != h.hour && h.n > 0 {
		for i, name := range SeriesNames {
			done = append(done, HourRow{Hour: h.hour, Name: name, Avg: h.sum[i] / float64(h.n), Peak: h.peak[i]})
		}
		*h = hourTally{}
	}
	h.hour = hour
	h.n++
	for i, v := range p.v {
		h.sum[i] += float64(v)
		h.peak[i] = max(h.peak[i], float64(v))
	}
	return done
}

// Watch keeps the fast sampler going until release is called. The first watcher starts it
// and the last one stops it, so nobody watching costs nothing.
func (c *Collector) Watch() (release func()) {
	c.mu.Lock()
	c.watchers++
	if c.watchers == 1 {
		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		go c.sample(ctx)
	}
	c.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.watchers--
			if c.watchers == 0 {
				c.cancel()
				c.cancel = nil
			}
		})
	}
}

// Watchers says how many are watching.
func (c *Collector) Watchers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.watchers
}

func (c *Collector) sample(ctx context.Context) {
	var r reader
	r.read(c.started, false)
	tick := time.NewTicker(liveEvery)
	defer tick.Stop()
	// Space on a disk changes slowly, and asking a sleeping drive would wake it.
	beats := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		full := beats%15 == 0
		beats++
		s := r.read(c.started, full)
		p := pointOf(s, c.counts())
		c.mu.Lock()
		if !full {
			s.Volumes = c.latest.Volumes
			if s.CPU != nil && c.latest.CPU != nil {
				s.CPU.Temps = c.latest.CPU.Temps
			}
		}
		c.latest = s
		c.live.add(p)
		close(c.changed)
		c.changed = make(chan struct{})
		c.mu.Unlock()
	}
}

// OneReading is the latest sample if the sampler is going, and otherwise two readings a
// moment apart, as a speed needs two. The disks are read too, as this isn't done often.
func (c *Collector) OneReading(ctx context.Context) Sample {
	c.mu.Lock()
	if c.watchers > 0 && c.latest.Time != 0 && len(c.latest.Volumes) > 0 {
		defer c.mu.Unlock()
		return c.latest
	}
	c.mu.Unlock()
	var r reader
	r.read(c.started, false)
	select {
	case <-time.After(250 * time.Millisecond):
	case <-ctx.Done():
	}
	return r.read(c.started, true)
}

// Latest is the most recent sample, and a channel closed when there's a newer one.
func (c *Collector) Latest() (Sample, <-chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest, c.changed
}

// History is a set of series over a time range. Times are Unix seconds. Peak is present
// when each point stands for several readings.
type History struct {
	From   int64                `json:"from"`
	To     int64                `json:"to"`
	Step   int64                `json:"stepSec"`
	Times  []int64              `json:"times"`
	Series map[string][]float64 `json:"series"`
	Peak   map[string][]float64 `json:"peak,omitempty"`
}

// maxPoints is as many as a chart can use.
const maxPoints = 240

// Read gets a history. Up to ten minutes comes from the fast samples if there are any,
// up to a day from the minute ones, and beyond that from the hourly rows.
func (c *Collector) Read(ctx context.Context, span time.Duration, names []string) (History, error) {
	now := time.Now()
	from := now.Add(-span).Unix()
	h := History{From: from, To: now.Unix(), Times: []int64{}, Series: map[string][]float64{}}
	if len(names) == 0 {
		names = SeriesNames
	}
	if span > 24*time.Hour {
		if c.store == nil {
			return h, nil
		}
		rows, err := c.store.Hours(ctx, from, now.Unix(), names)
		if err != nil {
			return h, err
		}
		h.Step = 3600
		h.Peak = map[string][]float64{}
		at := map[int64]int{}
		for _, r := range rows {
			if _, ok := at[r.Hour]; !ok {
				at[r.Hour] = len(h.Times)
				h.Times = append(h.Times, r.Hour)
			}
		}
		for _, n := range names {
			h.Series[n] = make([]float64, len(h.Times))
			h.Peak[n] = make([]float64, len(h.Times))
		}
		for _, r := range rows {
			if s, ok := h.Series[r.Name]; ok {
				s[at[r.Hour]] = r.Avg
				h.Peak[r.Name][at[r.Hour]] = r.Peak
			}
		}
		return h, nil
	}
	c.mu.Lock()
	var pts []point
	step := int64(60)
	if span <= 10*time.Minute {
		pts = c.live.since(from)
		step = 2
	}
	if len(pts) == 0 {
		pts = c.low.since(from)
		step = 60
	}
	c.mu.Unlock()
	h.Step = step
	stride := max(1, (len(pts)+maxPoints-1)/maxPoints)
	if stride > 1 {
		h.Step = step * int64(stride)
		h.Peak = map[string][]float64{}
	}
	idx := make([]int, len(names))
	for i, n := range names {
		idx[i] = -1
		for j, x := range SeriesNames {
			if x == n {
				idx[i] = j
			}
		}
		h.Series[n] = []float64{}
		if stride > 1 {
			h.Peak[n] = []float64{}
		}
	}
	for start := 0; start < len(pts); start += stride {
		end := min(start+stride, len(pts))
		h.Times = append(h.Times, pts[end-1].t)
		for i, n := range names {
			if idx[i] < 0 {
				continue
			}
			sum, peak := 0.0, 0.0
			for _, p := range pts[start:end] {
				v := float64(p.v[idx[i]])
				sum += v
				peak = max(peak, v)
			}
			h.Series[n] = append(h.Series[n], sum/float64(end-start))
			if stride > 1 {
				h.Peak[n] = append(h.Peak[n], peak)
			}
		}
	}
	return h, nil
}
