package wittytuner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittytuner/mpegts"
)

// Linux DVB: tuner cards and USB sticks under /dev/dvb. The ioctls are in
// dvb_linux.go; band plans and channel files are here, so they can be tested
// anywhere. Built from the kernel's DVB API documentation and not yet tried
// with real hardware.

func init() {
	register(Driver{
		ID: "dvb", Name: "Linux DVB (DVB-T/T2/C/S/S2, ATSC, ISDB-T)", Status: "untested",
		Platforms:   []string{"linux"},
		Description: "Tuner cards and USB sticks on this Linux machine. Built from the kernel's documentation and not yet tried with real hardware. Satellite dish switches (DiSEqC) are not done yet.",
		Scans:       true,
		Fields: []Field{
			{Key: "adapter", Label: "Adapter", Kind: "number", Default: "0", Help: "0 for /dev/dvb/adapter0."},
			{Key: "system", Label: "Broadcast system", Kind: "select", Required: true, Default: "atsc", Options: []string{"atsc", "qam-us", "dvbt", "dvbt2", "dvbc", "dvbs", "dvbs2", "isdbt"}},
			{Key: "bandwidth", Label: "Bandwidth (MHz)", Kind: "select", Default: "8", Options: []string{"6", "7", "8"}, Help: "DVB-T and DVB-T2."},
			{Key: "channelsConf", Label: "Channels file (dvbv5 or zap format)", Kind: "textarea", Help: "Optional: paste the output of dvbv5-scan instead of scanning here."},
			{Key: "transponders", Label: "Satellite and cable transponders", Kind: "textarea", Help: "DVB-S and DVB-C only, one per line: frequency in kHz, polarisation (h or v, satellite), symbol rate in symbols per second. No lists are included."},
			{Key: "lnbLow", Label: "LNB low frequency (kHz)", Kind: "number", Default: "9750000", Help: "Satellite only."},
			{Key: "lnbHigh", Label: "LNB high frequency (kHz)", Kind: "number", Default: "10600000", Help: "Satellite only."},
			{Key: "lnbSwitch", Label: "LNB switch frequency (kHz)", Kind: "number", Default: "11700000", Help: "Satellite only."},
		},
		build: func(c Config, e Env) (Source, error) {
			a, err := strconv.Atoi(firstNonEmpty(c.Options["adapter"], "0"))
			if err != nil || a < 0 || a > 255 {
				return nil, errors.New("the adapter is a number such as 0")
			}
			if _, ok := bandPlans[c.Options["system"]]; !ok && !needsTransponders(c.Options["system"]) {
				return nil, errors.New("unknown broadcast system")
			}
			return &dvb{cfg: c, env: e, adapter: a}, nil
		},
	})
}

func needsTransponders(system string) bool {
	return system == "dvbs" || system == "dvbs2" || system == "dvbc"
}

// Tuning is what a channel needs to be found again. Frequency is in Hz; for
// satellite, kHz as broadcasters publish it.
type Tuning struct {
	System     string
	Frequency  int64
	Bandwidth  int // Hz
	SymbolRate int
	Pol        string
	Modulation string
}

// Locator is how a tuning is kept with a channel.
func (t Tuning) Locator(program int) string {
	return fmt.Sprintf("%s:%d:%d:%d:%s:%s:%d", t.System, t.Frequency, t.Bandwidth, t.SymbolRate, t.Pol, t.Modulation, program)
}

// ParseLocator reads a kept tuning back.
func ParseLocator(s string) (Tuning, int, error) {
	p := strings.Split(s, ":")
	if len(p) != 7 {
		return Tuning{}, 0, errors.New("bad tuning")
	}
	f, err1 := strconv.ParseInt(p[1], 10, 64)
	bw, err2 := strconv.Atoi(p[2])
	sr, err3 := strconv.Atoi(p[3])
	prog, err4 := strconv.Atoi(p[6])
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return Tuning{}, 0, errors.New("bad tuning")
	}
	return Tuning{System: p[0], Frequency: f, Bandwidth: bw, SymbolRate: sr, Pol: p[4], Modulation: p[5]}, prog, nil
}

// bandPlans are the standard channel-to-frequency formulas, not lists of
// stations: they say where a channel could be, and a scan finds what is there.
var bandPlans = map[string]func(bw int) []Tuning{
	"atsc": func(int) []Tuning {
		// US and Canada RF 2–36 (repacked band).
		return rfPlan("atsc", "vsb8", 6000000, usVHFUHF(36))
	},
	"qam-us": func(int) []Tuning {
		// US cable channels 2–135 as HRC-free standard plan.
		var f []int64
		f = append(f, usVHFUHF(13)...)
		for ch := 14; ch <= 22; ch++ {
			f = append(f, 123000000+int64(ch-14)*6000000)
		}
		for ch := 23; ch <= 94; ch++ {
			f = append(f, 219000000+int64(ch-23)*6000000)
		}
		for ch := 100; ch <= 135; ch++ {
			f = append(f, 651000000+int64(ch-100)*6000000)
		}
		return rfPlan("qam-us", "qam256", 6000000, f)
	},
	"dvbt":  func(bw int) []Tuning { return rfPlan("dvbt", "", bw, europeUHF(bw)) },
	"dvbt2": func(bw int) []Tuning { return rfPlan("dvbt2", "", bw, europeUHF(bw)) },
	"isdbt": func(int) []Tuning {
		// Japan UHF 13–62: centre 473.143 MHz + 6 MHz per channel.
		var f []int64
		for ch := 13; ch <= 62; ch++ {
			f = append(f, 473142857+int64(ch-13)*6000000)
		}
		return rfPlan("isdbt", "", 6000000, f)
	},
}

func usVHFUHF(last int) []int64 {
	var f []int64
	for ch := 2; ch <= last; ch++ {
		switch {
		case ch <= 4:
			f = append(f, 57000000+int64(ch-2)*6000000)
		case ch <= 6:
			f = append(f, 79000000+int64(ch-5)*6000000)
		case ch <= 13:
			f = append(f, 177000000+int64(ch-7)*6000000)
		default:
			f = append(f, 473000000+int64(ch-14)*6000000)
		}
	}
	return f
}

func europeUHF(bw int) []int64 {
	var f []int64
	switch bw {
	case 7000000:
		// VHF band III 7 MHz, as Australia and parts of Europe use.
		for ch := 5; ch <= 12; ch++ {
			f = append(f, 177500000+int64(ch-5)*7000000)
		}
		fallthrough
	default:
		// UHF 21–48 in the post-700 MHz band plan: 474 MHz + 8 MHz per channel.
		for ch := 21; ch <= 48; ch++ {
			f = append(f, 474000000+int64(ch-21)*8000000)
		}
	}
	return f
}

func rfPlan(system, mod string, bw int, freqs []int64) []Tuning {
	out := make([]Tuning, 0, len(freqs))
	for _, f := range freqs {
		out = append(out, Tuning{System: system, Frequency: f, Bandwidth: bw, Modulation: mod})
	}
	return out
}

// plan is what a scan should try.
func (d *dvb) plan() ([]Tuning, error) {
	sys := d.cfg.Options["system"]
	if needsTransponders(sys) {
		return parseTransponders(sys, d.cfg.Options["transponders"])
	}
	bw, _ := strconv.Atoi(firstNonEmpty(d.cfg.Options["bandwidth"], "8"))
	return bandPlans[sys](bw * 1000000), nil
}

// parseTransponders reads the owner's own list: "11362000 h 22000000".
func parseTransponders(system, text string) ([]Tuning, error) {
	var out []Tuning
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		t := Tuning{System: system}
		v, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("could not read the frequency in %q", line)
		}
		t.Frequency = v
		switch system {
		case "dvbc":
			t.Frequency = v * 1000
			if len(f) < 2 {
				return nil, fmt.Errorf("%q needs a symbol rate", line)
			}
			t.SymbolRate, err = strconv.Atoi(f[1])
		default:
			if len(f) < 3 || (f[1] != "h" && f[1] != "v") {
				return nil, fmt.Errorf("%q needs a polarisation (h or v) and a symbol rate", line)
			}
			t.Pol = f[1]
			t.SymbolRate, err = strconv.Atoi(f[2])
		}
		if err != nil {
			return nil, fmt.Errorf("could not read the symbol rate in %q", line)
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, errors.New("add transponders first: this system cannot be scanned from a band plan")
	}
	return out, nil
}

var confSection = regexp.MustCompile(`^\[(.+)\]$`)

// ParseChannelsConf reads a file from dvbv5-scan (the [name] / key = value
// format) or the older colon-separated zap format. Encrypted entries are kept,
// marked, so people can see why they do not play.
func ParseChannelsConf(system, text string) []Channel {
	var out []Channel
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var cur map[string]string
	name := ""
	flush := func() {
		if cur == nil {
			return
		}
		if c, ok := channelFromConf(system, name, cur); ok {
			out = append(out, c)
		}
		cur = nil
	}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if m := confSection.FindStringSubmatch(line); m != nil {
			flush()
			name, cur = m[1], map[string]string{}
			continue
		}
		if cur != nil {
			if k, v, ok := strings.Cut(line, "="); ok {
				cur[strings.ToUpper(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
			continue
		}
		// Zap format: name:frequency:...:video:audio:program
		p := strings.Split(line, ":")
		if len(p) >= 4 {
			if f, err := strconv.ParseInt(p[1], 10, 64); err == nil {
				prog, _ := strconv.Atoi(p[len(p)-1])
				t := Tuning{System: system, Frequency: f}
				out = append(out, Channel{ID: p[1] + "-" + p[len(p)-1], Name: p[0], Number: strconv.Itoa(len(out) + 1), Program: prog, Locator: t.Locator(prog)})
			}
		}
	}
	flush()
	return out
}

func channelFromConf(system, name string, kv map[string]string) (Channel, bool) {
	f, err := strconv.ParseInt(kv["FREQUENCY"], 10, 64)
	if err != nil || f <= 0 {
		return Channel{}, false
	}
	prog, _ := strconv.Atoi(kv["SERVICE_ID"])
	sr, _ := strconv.Atoi(kv["SYMBOL_RATE"])
	bw, _ := strconv.Atoi(kv["BANDWIDTH_HZ"])
	t := Tuning{System: system, Frequency: f, SymbolRate: sr, Bandwidth: bw}
	switch strings.ToUpper(kv["POLARIZATION"]) {
	case "HORIZONTAL":
		t.Pol = "h"
	case "VERTICAL":
		t.Pol = "v"
	}
	c := Channel{ID: kv["FREQUENCY"] + "-" + kv["SERVICE_ID"], Name: name, Number: kv["VCHANNEL"], Program: prog, Locator: t.Locator(prog)}
	if ca, listed := kv["CA_ID"]; listed {
		c.Checked = true
		if ca != "" && ca != "0" {
			c.Encrypted = "the channel file lists a conditional access system for it"
		}
	}
	return c, true
}

type dvb struct {
	cfg     Config
	env     Env
	adapter int
}

func (d *dvb) Info() Info {
	return Info{Driver: "dvb", Name: d.cfg.Name, MaxStreams: 1, Detail: fmt.Sprintf("/dev/dvb/adapter%d, %s", d.adapter, d.cfg.Options["system"])}
}

func (d *dvb) Channels(context.Context) ([]Channel, error) {
	text := d.cfg.Options["channelsConf"]
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	return ParseChannelsConf(d.cfg.Options["system"], text), nil
}

func (d *dvb) Close() error { return nil }

// Scan tries each frequency in the plan: lock, read what is there, move on.
func (d *dvb) Scan(ctx context.Context, _ ScanOptions, progress func(Progress)) ([]Channel, error) {
	plan, err := d.plan()
	if err != nil {
		return nil, err
	}
	var out []Channel
	for i, t := range plan {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if progress != nil {
			progress(Progress{Percent: i * 100 / len(plan), Found: len(out), Message: fmt.Sprintf("%.1f MHz", float64(t.Frequency)/1e6)})
		}
		r, err := d.openTuned(ctx, t, 3*time.Second)
		if errors.Is(err, errNoLock) {
			d.env.logger().DebugContext(ctx, "no signal on this frequency", "topic", "tuner", "mhz", float64(t.Frequency)/1e6)
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, sv := range scanMux(ctx, r, 10*time.Second) {
			if sv.Radio && t.System == "atsc" {
				continue
			}
			out = append(out, Channel{ID: fmt.Sprintf("%d-%d", t.Frequency, sv.Program), Number: sv.number(strconv.Itoa(len(out) + 1)), Name: sv.Name, Group: sv.Provider,
				HD: sv.HD, Radio: sv.Radio, Encrypted: sv.Encrypted, Checked: true, Program: sv.Program, Locator: t.Locator(sv.Program)})
		}
		_ = r.Close()
	}
	if progress != nil {
		progress(Progress{Percent: 100, Found: len(out)})
	}
	return out, nil
}

func (d *dvb) Open(ctx context.Context, ch Channel) (*Stream, error) {
	if ch.Encrypted != "" {
		return nil, &EncryptedError{ch.Name, ch.Encrypted}
	}
	t, prog, err := ParseLocator(ch.Locator)
	if err != nil {
		return nil, err
	}
	r, err := d.openTuned(ctx, t, 5*time.Second)
	if err != nil {
		if errors.Is(err, errNoLock) {
			return nil, fmt.Errorf("no signal on %.1f MHz: %w", float64(t.Frequency)/1e6, err)
		}
		return nil, err
	}
	st := guarded(r)
	if prog == 0 {
		return st, nil
	}
	return &Stream{Reader: mpegts.NewProgramFilter(st.Reader, prog), Guard: st.Guard}, nil
}

var errNoLock = errors.New("the tuner found no signal")

// discoverDVB is in dvb_linux.go and dvb_other.go.
