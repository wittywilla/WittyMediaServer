package wittytuner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The Linux DVB API (linux/dvb/frontend.h and dmx.h). Plain ioctls, so no
// C compiler is needed.

const (
	iocWrite = 1
	iocRead  = 2
)

func ioc(dir, typ, nr, size uintptr) uintptr { return dir<<30 | size<<16 | typ<<8 | nr }

// Property commands (DTV_*).
const (
	dtvTune           = 1
	dtvClear          = 2
	dtvFrequency      = 3
	dtvModulation     = 4
	dtvBandwidthHz    = 5
	dtvInversion      = 6
	dtvSymbolRate     = 8
	dtvInnerFEC       = 9
	dtvVoltage        = 10
	dtvTone           = 11
	dtvDeliverySystem = 17
)

// Delivery systems (fe_delivery_system).
const (
	sysDVBCAnnexA = 1
	sysDVBCAnnexB = 2
	sysDVBT       = 3
	sysDVBS       = 5
	sysDVBS2      = 6
	sysISDBT      = 8
	sysATSC       = 11
	sysDVBT2      = 16
)

// Modulations (fe_modulation).
const (
	qam256  = 5
	qamAuto = 6
	vsb8    = 7
	qpsk    = 0
)

const (
	invAuto     = 2
	fecAuto     = 9
	volt13      = 0
	volt18      = 1
	toneOn      = 0
	toneOff     = 1
	hasLock     = 0x10
	dmxOutTS    = 2 // DMX_OUT_TS_TAP: to the dvr device
	dmxInFE     = 0
	allPIDs     = 0x2000
	dmxStart    = 4 // DMX_IMMEDIATE_START
	dmxPESOther = 20
)

// dtvProperty is the kernel's packed struct dtv_property: command, three
// reserved words, a 56-byte union (52 on 32-bit systems), and a result.
type dtvProperty struct {
	cmd      uint32
	reserved [3]uint32
	data     uint32
	rest     [dtvUnionRest]byte
	result   int32
}

const dtvUnionRest = 32 + 4 + 12 + int(unsafe.Sizeof(uintptr(0))) - 4

type dtvProperties struct {
	num   uint32
	props *dtvProperty
}

type dmxPESFilter struct {
	pid     uint16
	input   uint32
	output  uint32
	pesType uint32
	flags   uint32
}

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); e != 0 {
		return e
	}
	return nil
}

// ioctlValue is for calls that take a number rather than a pointer.
func ioctlValue(fd, req, v uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, v); e != 0 {
		return e
	}
	return nil
}

func setProps(fd uintptr, props []dtvProperty) error {
	ps := dtvProperties{num: uint32(len(props)), props: &props[0]}
	return ioctl(fd, ioc(iocWrite, 'o', 82, unsafe.Sizeof(ps)), unsafe.Pointer(&ps))
}

func prop(cmd, v uint32) dtvProperty { return dtvProperty{cmd: cmd, data: v} }

// tuneProps turns a tuning into properties.
func tuneProps(t Tuning, lnbFreq int64, high bool, pol string) ([]dtvProperty, error) {
	p := []dtvProperty{prop(dtvClear, 0)}
	switch t.System {
	case "atsc":
		p = append(p, prop(dtvDeliverySystem, sysATSC), prop(dtvModulation, vsb8), prop(dtvFrequency, uint32(t.Frequency)))
	case "qam-us":
		p = append(p, prop(dtvDeliverySystem, sysDVBCAnnexB), prop(dtvModulation, qamAuto), prop(dtvFrequency, uint32(t.Frequency)))
	case "dvbt", "dvbt2":
		sys := uint32(sysDVBT)
		if t.System == "dvbt2" {
			sys = sysDVBT2
		}
		p = append(p, prop(dtvDeliverySystem, sys), prop(dtvFrequency, uint32(t.Frequency)), prop(dtvBandwidthHz, uint32(t.Bandwidth)), prop(dtvInversion, invAuto))
	case "isdbt":
		p = append(p, prop(dtvDeliverySystem, sysISDBT), prop(dtvFrequency, uint32(t.Frequency)), prop(dtvBandwidthHz, uint32(t.Bandwidth)))
	case "dvbc":
		p = append(p, prop(dtvDeliverySystem, sysDVBCAnnexA), prop(dtvFrequency, uint32(t.Frequency)), prop(dtvSymbolRate, uint32(t.SymbolRate)), prop(dtvModulation, qamAuto), prop(dtvInnerFEC, fecAuto))
	case "dvbs", "dvbs2":
		sys := uint32(sysDVBS)
		if t.System == "dvbs2" {
			sys = sysDVBS2
		}
		v := uint32(volt13)
		if pol == "h" {
			v = volt18
		}
		tone := uint32(toneOff)
		if high {
			tone = toneOn
		}
		p = append(p, prop(dtvDeliverySystem, sys), prop(dtvVoltage, v), prop(dtvTone, tone), prop(dtvFrequency, uint32(lnbFreq)), prop(dtvSymbolRate, uint32(t.SymbolRate)), prop(dtvInnerFEC, fecAuto))
	default:
		return nil, errors.New("unknown broadcast system")
	}
	return append(p, prop(dtvTune, 0)), nil
}

// lnb works out what a satellite tuner is told: the difference between the
// transponder and the LNB's oscillator.
func (d *dvb) lnb(t Tuning) (int64, bool) {
	num := func(k string, def int64) int64 {
		if v, err := strconv.ParseInt(d.cfg.Options[k], 10, 64); err == nil && v > 0 {
			return v
		}
		return def
	}
	low, high, sw := num("lnbLow", 9750000), num("lnbHigh", 10600000), num("lnbSwitch", 11700000)
	if t.Frequency >= sw {
		return t.Frequency - high, true
	}
	return t.Frequency - low, false
}

// dvbStream holds the frontend and demux open while the dvr is read.
type dvbStream struct {
	dvr, fe, dmx *os.File
}

func (s *dvbStream) Read(p []byte) (int, error) { return s.dvr.Read(p) }

func (s *dvbStream) Close() error {
	err := s.dvr.Close()
	_ = s.dmx.Close()
	_ = s.fe.Close()
	return err
}

func (d *dvb) openTuned(ctx context.Context, t Tuning, wait time.Duration) (io.ReadCloser, error) {
	dir := fmt.Sprintf("/dev/dvb/adapter%d", d.adapter)
	fe, err := os.OpenFile(dir+"/frontend0", os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, syscall.EBUSY) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("the tuner could not be opened: %w", err)
	}
	fail := func(err error) (io.ReadCloser, error) {
		fe.Close()
		return nil, err
	}
	freq, high := d.lnb(t)
	props, err := tuneProps(t, freq, high, t.Pol)
	if err != nil {
		return fail(err)
	}
	if err := setProps(fe.Fd(), props); err != nil {
		return fail(fmt.Errorf("the tuner refused the tuning: %w", err))
	}
	deadline := time.Now().Add(wait)
	locked := false
	for time.Now().Before(deadline) && ctx.Err() == nil {
		var status uint32
		if err := ioctl(fe.Fd(), ioc(iocRead, 'o', 69, 4), unsafe.Pointer(&status)); err == nil && status&hasLock != 0 {
			locked = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !locked {
		return fail(errNoLock)
	}
	dmx, err := os.OpenFile(dir+"/demux0", os.O_RDWR, 0)
	if err != nil {
		return fail(err)
	}
	// A larger buffer: a busy machine should not drop packets.
	_ = ioctlValue(dmx.Fd(), ioc(0, 'o', 45, 0), 8<<20)
	f := dmxPESFilter{pid: allPIDs, input: dmxInFE, output: dmxOutTS, pesType: dmxPESOther, flags: dmxStart}
	if err := ioctl(dmx.Fd(), ioc(iocWrite, 'o', 44, unsafe.Sizeof(f)), unsafe.Pointer(&f)); err != nil {
		dmx.Close()
		return fail(err)
	}
	dvr, err := os.Open(dir + "/dvr0")
	if err != nil {
		dmx.Close()
		return fail(err)
	}
	return &dvbStream{dvr: dvr, fe: fe, dmx: dmx}, nil
}

func discoverDVB(context.Context, Env) []Found {
	dirs, _ := filepath.Glob("/dev/dvb/adapter*")
	sort.Strings(dirs)
	var out []Found
	for _, dir := range dirs {
		if _, err := os.Stat(dir + "/frontend0"); err != nil {
			continue
		}
		n := strings.TrimPrefix(filepath.Base(dir), "adapter")
		out = append(out, Found{Driver: "dvb", Name: "DVB adapter " + n, Detail: dir, Options: map[string]string{"adapter": n}})
	}
	return out
}
