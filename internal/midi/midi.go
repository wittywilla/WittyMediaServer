// Package midi reads Standard MIDI Files and plays them with a small synthesiser of
// its own: FFmpeg doesn't read MIDI, and this needs no sound font or other download.
// It sounds simple, like a sound card of the 1990s, and it works everywhere.
//
// Files are hostile as ever: tracks and events are bounded, and rendering stops at a
// length no real song reaches.
package midi

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"sort"
	"strings"
)

const (
	maxFile      = 64 << 20
	maxEvents    = 16 << 20
	maxTracks    = 4096
	maxSeconds   = 60 * 60
	maxVoices    = 48
	SampleRate   = 32000
	defaultTempo = 500000 // microseconds per quarter note
)

// Info is what a file says about itself.
type Info struct {
	Title   string
	Seconds float64
}

type event struct {
	at     uint64 // in the file's ticks
	order  uint32
	status byte
	a, b   byte
	tempo  uint32
}

// Song is a parsed file: every event in time order.
type Song struct {
	Info
	events   []event
	division int
	smpte    float64 // ticks per second, for files timed that way
}

var (
	errNotMIDI = errors.New("this isn't a MIDI file")
	errDamaged = errors.New("this MIDI file is damaged")
)

// ReadFile parses a file.
func ReadFile(path string) (*Song, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > maxFile {
		return nil, errNotMIDI
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b)
}

// Parse reads a Standard MIDI File, also when wrapped in RIFF (.rmi).
func Parse(b []byte) (*Song, error) {
	if len(b) >= 20 && string(b[:4]) == "RIFF" && string(b[8:12]) == "RMID" {
		// Chunks after the form type: the song is in "data".
		for off := 12; off+8 <= len(b); {
			size := int(binary.LittleEndian.Uint32(b[off+4:]))
			if size < 0 || off+8+size > len(b) {
				break
			}
			if string(b[off:off+4]) == "data" {
				b = b[off+8 : off+8+size]
				break
			}
			off += 8 + size + size&1
		}
	}
	if len(b) < 14 || string(b[:4]) != "MThd" {
		return nil, errNotMIDI
	}
	hdr := int(binary.BigEndian.Uint32(b[4:]))
	if hdr < 6 || 8+hdr > len(b) {
		return nil, errDamaged
	}
	format := binary.BigEndian.Uint16(b[8:])
	div := binary.BigEndian.Uint16(b[12:])
	s := &Song{}
	if div&0x8000 != 0 {
		fps := float64(256 - int(div>>8))
		res := float64(div & 0xFF)
		if fps <= 0 || res <= 0 {
			return nil, errDamaged
		}
		s.smpte = fps * res
	} else {
		s.division = int(div)
		if s.division == 0 {
			return nil, errDamaged
		}
	}
	off := 8 + hdr
	track := uint64(0)
	var order uint32
	tracks := 0
	for off+8 <= len(b) && tracks < maxTracks {
		size := int(binary.BigEndian.Uint32(b[off+4:]))
		name := string(b[off : off+4])
		off += 8
		if size < 0 || off+size > len(b) {
			size = len(b) - off // the last chunk of a cut-off file
		}
		if name == "MTrk" {
			tracks++
			end, err := s.readTrack(b[off:off+size], &order, track)
			if err != nil {
				return nil, err
			}
			if format == 2 {
				track = end // one song after another
			}
		}
		off += size
		if len(s.events) > maxEvents {
			return nil, errDamaged
		}
	}
	if tracks == 0 {
		return nil, errDamaged
	}
	sort.SliceStable(s.events, func(i, j int) bool {
		if s.events[i].at != s.events[j].at {
			return s.events[i].at < s.events[j].at
		}
		return s.events[i].order < s.events[j].order
	})
	s.Seconds = s.duration()
	return s, nil
}

func (s *Song) readTrack(t []byte, order *uint32, start uint64) (uint64, error) {
	i := 0
	at := start
	var running byte
	readVar := func() (uint64, bool) {
		var v uint64
		for n := 0; n < 4; n++ {
			if i >= len(t) {
				return 0, false
			}
			c := t[i]
			i++
			v = v<<7 | uint64(c&0x7F)
			if c&0x80 == 0 {
				return v, true
			}
		}
		return v, true
	}
	for i < len(t) {
		d, ok := readVar()
		if !ok {
			break
		}
		at += d
		if i >= len(t) {
			break
		}
		st := t[i]
		switch {
		case st == 0xFF:
			i++
			if i >= len(t) {
				return at, nil
			}
			kind := t[i]
			i++
			l, ok := readVar()
			if !ok || uint64(i)+l > uint64(len(t)) {
				return at, nil
			}
			body := t[i : i+int(l)]
			i += int(l)
			switch {
			case kind == 0x51 && len(body) == 3:
				tempo := uint32(body[0])<<16 | uint32(body[1])<<8 | uint32(body[2])
				if tempo > 0 {
					*order++
					s.events = append(s.events, event{at: at, order: *order, status: 0xFF, tempo: tempo})
				}
			case kind == 0x03 && s.Title == "" && len(body) < 256:
				s.Title = cleanText(body)
			case kind == 0x2F:
				return at, nil
			}
		case st == 0xF0 || st == 0xF7:
			i++
			l, ok := readVar()
			if !ok || uint64(i)+l > uint64(len(t)) {
				return at, nil
			}
			i += int(l)
		case st >= 0xF0:
			i++ // system bytes that have no place in a file
		default:
			if st&0x80 != 0 {
				running = st
				i++
			}
			if running == 0 {
				return at, nil // data with no status: the rest can't be read
			}
			kind := running & 0xF0
			need := 2
			if kind == 0xC0 || kind == 0xD0 {
				need = 1
			}
			if i+need > len(t) {
				return at, nil
			}
			e := event{at: at, status: running, a: t[i] & 0x7F}
			if need == 2 {
				e.b = t[i+1] & 0x7F
			}
			i += need
			*order++
			e.order = *order
			s.events = append(s.events, e)
			if len(s.events) > maxEvents {
				return at, errDamaged
			}
		}
	}
	return at, nil
}

func cleanText(b []byte) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, strings.ToValidUTF8(string(b), "")))
}

// duration walks the tempo changes to the last event.
func (s *Song) duration() float64 {
	if len(s.events) == 0 {
		return 0
	}
	sec, tempo, last := 0.0, float64(defaultTempo), uint64(0)
	for _, e := range s.events {
		sec += s.ticks(e.at-last, tempo)
		last = e.at
		if e.status == 0xFF {
			tempo = float64(e.tempo)
		}
		if sec > maxSeconds {
			return maxSeconds
		}
	}
	return sec
}

func (s *Song) ticks(n uint64, tempo float64) float64 {
	if s.smpte > 0 {
		return float64(n) / s.smpte
	}
	return float64(n) * tempo / 1e6 / float64(s.division)
}

// ---------------------------------------------------------------------------
// The synthesiser
// ---------------------------------------------------------------------------

type voice struct {
	on       bool
	key      byte
	channel  byte
	vel      float64
	phase    float64
	freq     float64
	modPhase float64
	age      float64 // seconds since the note began
	released float64 // seconds since it was let go; -1 while held
	wave     waveShape
	drum     bool
	noise    uint32
}

// Instruments are grouped into shapes a few numbers can describe.
type waveShape struct {
	harmonics []float64 // strength of each overtone
	modRatio  float64   // a touch of FM, for bells and electric pianos
	modDepth  float64
	attack    float64
	decay     float64 // time to fall to sustain
	sustain   float64
	release   float64
}

var (
	shapePiano   = waveShape{harmonics: []float64{1, .5, .25, .12, .06}, attack: .002, decay: 1.4, sustain: .12, release: .25}
	shapeEPiano  = waveShape{harmonics: []float64{1, .1}, modRatio: 14, modDepth: .6, attack: .002, decay: 1.1, sustain: .15, release: .3}
	shapeBell    = waveShape{harmonics: []float64{1}, modRatio: 3.5, modDepth: 2.2, attack: .001, decay: 1.6, sustain: 0, release: .5}
	shapeOrgan   = waveShape{harmonics: []float64{1, 1, .6, .4, .1, .2}, attack: .01, decay: .05, sustain: .9, release: .06}
	shapeGuitar  = waveShape{harmonics: []float64{1, .6, .4, .2, .1}, attack: .002, decay: .9, sustain: .05, release: .12}
	shapeBass    = waveShape{harmonics: []float64{1, .7, .2}, attack: .004, decay: .6, sustain: .3, release: .1}
	shapeStrings = waveShape{harmonics: []float64{1, .7, .5, .35, .25, .18, .12}, attack: .18, decay: .4, sustain: .75, release: .35}
	shapeBrass   = waveShape{harmonics: []float64{1, .8, .6, .5, .4, .3}, attack: .05, decay: .2, sustain: .7, release: .15}
	shapeReed    = waveShape{harmonics: []float64{1, .1, .6, .1, .35, .1, .2}, attack: .03, decay: .1, sustain: .75, release: .1}
	shapeFlute   = waveShape{harmonics: []float64{1, .25, .05}, attack: .06, decay: .1, sustain: .8, release: .12}
	shapeLead    = waveShape{harmonics: []float64{1, .5, .33, .25, .2, .17, .14, .12}, attack: .005, decay: .1, sustain: .8, release: .1}
	shapePad     = waveShape{harmonics: []float64{1, .5, .3, .2}, attack: .4, decay: .5, sustain: .8, release: .6}
)

func shapeOf(program byte) waveShape {
	switch program / 8 {
	case 0:
		return shapePiano
	case 1:
		if program == 12 || program == 13 {
			return shapeEPiano
		}
		return shapeBell
	case 2:
		return shapeOrgan
	case 3:
		return shapeGuitar
	case 4:
		return shapeBass
	case 5, 6:
		if program == 56 || (program >= 59 && program <= 63) {
			return shapeBrass
		}
		return shapeStrings
	case 7:
		return shapeBrass
	case 8:
		return shapeReed
	case 9:
		return shapeFlute
	case 10:
		return shapeLead
	case 11:
		return shapePad
	case 12:
		return shapePad
	case 13:
		return shapeGuitar
	case 14:
		return shapeBell
	}
	return shapeLead
}

type channelState struct {
	program  byte
	volume   float64
	pan      float64
	express  float64
	bend     float64 // semitones
	sustain  bool
	bendSize float64
}

// sine is a table: a square of the budget for a song is spent here.
var sine [4096]float64

func init() {
	for i := range sine {
		sine[i] = math.Sin(2 * math.Pi * float64(i) / float64(len(sine)))
	}
}

func sin(x float64) float64 { // x in turns
	x -= math.Floor(x)
	return sine[int(x*float64(len(sine)))&(len(sine)-1)]
}

// Render writes the song as 16-bit stereo samples at SampleRate. It stops when w does.
func (s *Song) Render(w io.Writer) error {
	bw := bufio.NewWriterSize(w, 1<<16)
	var channels [16]channelState
	for i := range channels {
		channels[i] = channelState{volume: 100.0 / 127, pan: .5, express: 1, bendSize: 2}
	}
	voices := make([]voice, 0, maxVoices)
	tempo := float64(defaultTempo)
	var sampleClock float64 // seconds rendered
	limit := s.Seconds + 3  // room for the last notes to fade
	if limit > maxSeconds {
		limit = maxSeconds
	}
	var songClock float64
	var lastTick uint64
	buf := make([]byte, 4)
	renderUntil := func(until float64) error {
		for sampleClock < until && sampleClock < limit {
			l, r := mix(voices, channels[:])
			putSample(buf, l, r)
			if _, err := bw.Write(buf); err != nil {
				return err
			}
			sampleClock += 1.0 / SampleRate
		}
		return nil
	}
	for _, e := range s.events {
		songClock += s.ticks(e.at-lastTick, tempo)
		lastTick = e.at
		if err := renderUntil(songClock); err != nil {
			return err
		}
		voices = compact(voices)
		if e.status == 0xFF {
			tempo = float64(e.tempo)
			continue
		}
		ch := e.status & 0x0F
		c := &channels[ch]
		switch e.status & 0xF0 {
		case 0x90:
			if e.b > 0 {
				voices = startNote(voices, c, ch, e.a, float64(e.b)/127)
				break
			}
			noteOff(voices, c, ch, e.a)
		case 0x80:
			noteOff(voices, c, ch, e.a)
		case 0xC0:
			c.program = e.a
		case 0xE0:
			c.bend = (float64(int(e.b)<<7|int(e.a)) - 8192) / 8192 * c.bendSize
			for i := range voices {
				if voices[i].on && voices[i].channel == ch && !voices[i].drum {
					voices[i].freq = noteFreq(voices[i].key, c.bend)
				}
			}
		case 0xB0:
			switch e.a {
			case 7:
				c.volume = float64(e.b) / 127
			case 10:
				c.pan = float64(e.b) / 127
			case 11:
				c.express = float64(e.b) / 127
			case 64:
				c.sustain = e.b >= 64
				if !c.sustain {
					for i := range voices {
						if voices[i].on && voices[i].channel == ch && voices[i].released == -2 {
							voices[i].released = 0
						}
					}
				}
			case 120, 123:
				for i := range voices {
					if voices[i].channel == ch && voices[i].released < 0 {
						voices[i].released = 0
					}
				}
			}
		}
	}
	if err := renderUntil(limit); err != nil {
		return err
	}
	return bw.Flush()
}

func noteFreq(key byte, bend float64) float64 {
	return 440 * math.Pow(2, (float64(key)+bend-69)/12)
}

func startNote(voices []voice, c *channelState, ch, key byte, vel float64) []voice {
	v := voice{on: true, key: key, channel: ch, vel: vel, released: -1, noise: uint32(key)*2654435761 + 1}
	if ch == 9 {
		v.drum = true
		v.freq = 60 + float64(key)*2
	} else {
		v.wave = shapeOf(c.program)
		v.freq = noteFreq(key, c.bend)
	}
	if len(voices) >= maxVoices {
		// Take the oldest released note, or else the oldest.
		pick, age := 0, -1.0
		for i := range voices {
			a := voices[i].age
			if voices[i].released >= 0 {
				a += 1000
			}
			if a > age {
				pick, age = i, a
			}
		}
		voices[pick] = v
		return voices
	}
	return append(voices, v)
}

func noteOff(voices []voice, c *channelState, ch, key byte) {
	for i := range voices {
		if voices[i].on && voices[i].channel == ch && voices[i].key == key && voices[i].released == -1 {
			if c.sustain {
				voices[i].released = -2
			} else {
				voices[i].released = 0
			}
		}
	}
}

func compact(voices []voice) []voice {
	out := voices[:0]
	for _, v := range voices {
		if v.on {
			out = append(out, v)
		}
	}
	return out
}

func envelope(v *voice) float64 {
	w := v.wave
	var level float64
	switch {
	case v.age < w.attack:
		level = v.age / w.attack
	case v.age < w.attack+w.decay:
		f := (v.age - w.attack) / w.decay
		level = 1 - (1-w.sustain)*f
	default:
		level = w.sustain
	}
	if v.released >= 0 {
		if v.released >= w.release || w.release <= 0 {
			v.on = false
			return 0
		}
		level *= 1 - v.released/w.release
	} else if level < 0.002 && v.age > w.attack {
		v.on = false
	}
	return level
}

func mix(voices []voice, channels []channelState) (float64, float64) {
	const dt = 1.0 / SampleRate
	var left, right float64
	for i := range voices {
		v := &voices[i]
		if !v.on {
			continue
		}
		c := &channels[v.channel]
		var out float64
		if v.drum {
			out = drum(v, dt)
		} else {
			env := envelope(v)
			mod := 0.0
			if v.wave.modDepth > 0 {
				v.modPhase += v.freq * v.wave.modRatio * dt
				mod = v.wave.modDepth * sin(v.modPhase) * env
			}
			v.phase += v.freq * dt
			for h, a := range v.wave.harmonics {
				out += a * sin(v.phase*float64(h+1)+mod)
			}
			out *= env * 0.35
		}
		v.age += dt
		if v.released >= 0 {
			v.released += dt
		}
		out *= v.vel * c.volume * c.express * .45
		left += out * math.Sqrt(1-c.pan)
		right += out * math.Sqrt(c.pan)
	}
	return left, right
}

// drum makes a short burst of noise, with a falling tone under it for the low drums.
func drum(v *voice, dt float64) float64 {
	k := v.key
	length, tone, noise := .12, 0.0, .7
	switch {
	case k == 35 || k == 36: // kick
		length, tone, noise = .22, 1, .08
	case k == 38 || k == 40: // snare
		length, tone, noise = .2, .3, .8
	case k >= 41 && k <= 50 && k != 42 && k != 44 && k != 46: // toms
		length, tone, noise = .3, .9, .15
	case k == 42 || k == 44: // closed hat
		length, noise = .05, .55
	case k == 46: // open hat
		length, noise = .3, .5
	case k == 49 || k == 51 || k == 52 || k == 55 || k == 57 || k == 59: // cymbals
		length, noise = .9, .45
	}
	if v.age > length {
		v.on = false
		return 0
	}
	f := 1 - v.age/length
	env := f * f
	v.noise ^= v.noise << 13
	v.noise ^= v.noise >> 17
	v.noise ^= v.noise << 5
	n := float64(int32(v.noise)) / (1 << 31)
	pitch := v.freq * (1 + 2*f)
	v.phase += pitch * dt
	return env * (tone*sin(v.phase) + noise*n)
}

func putSample(b []byte, l, r float64) {
	clip := func(x float64) int16 {
		// A soft knee: many notes at once get louder without breaking up.
		x = x / (1 + math.Abs(x)*0.25)
		if x > 1 {
			x = 1
		} else if x < -1 {
			x = -1
		}
		return int16(x * 32767)
	}
	binary.LittleEndian.PutUint16(b, uint16(clip(l)))
	binary.LittleEndian.PutUint16(b[2:], uint16(clip(r)))
}
