package wittybluray

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// BDMV/AUXDATA/sound.bdmv: the short clips a menu plays when a button is
// selected or pressed. They are uncompressed 48 kHz, 16-bit, big-endian PCM.

// Sounds is a disc's sound.bdmv.
type Sounds struct {
	Clips []SoundClip
	data  []byte
}

// SoundClip is one sound, by its place in the file (the id buttons name).
type SoundClip struct {
	Channels int
	Rate     int
	offset   int
	length   int
}

// NoSound is the button sound id meaning "none".
const NoSound = 0xff

// ParseSounds parses sound.bdmv.
func ParseSounds(b []byte) (*Sounds, error) {
	r := &rd{b: b}
	if r.str(0, 4) != "BCLK" {
		return nil, errors.New("not a sound file")
	}
	start := int(r.u32(8))
	n := int(r.u8(45))
	s := &Sounds{data: b}
	for i, p := 0, 46; i < n && r.err == nil; i, p = i+1, p+10 {
		ch := 1
		if r.u8(p)>>4 == 3 {
			ch = 2
		}
		c := SoundClip{Channels: ch, Rate: 48000, offset: start + int(r.u32(p+2)), length: int(r.u32(p + 6))}
		if c.offset < 0 || c.length < 0 || c.offset+c.length > len(b) {
			c.length = 0 // a damaged entry is a silent clip; the others still work
		}
		s.Clips = append(s.Clips, c)
	}
	if r.err != nil {
		return nil, fmt.Errorf("sound.bdmv: %w", r.err)
	}
	return s, nil
}

// WAV returns one clip as a WAV file, which every browser plays.
func (s *Sounds) WAV(id int) ([]byte, bool) {
	if id < 0 || id >= len(s.Clips) || s.Clips[id].length == 0 {
		return nil, false
	}
	c := s.Clips[id]
	pcm := s.data[c.offset : c.offset+c.length&^1]
	out := make([]byte, 44+len(pcm))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(pcm)))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], uint16(c.Channels))
	binary.LittleEndian.PutUint32(out[24:], uint32(c.Rate))
	binary.LittleEndian.PutUint32(out[28:], uint32(c.Rate*c.Channels*2))
	binary.LittleEndian.PutUint16(out[32:], uint16(c.Channels*2))
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(pcm)))
	for i := 0; i+1 < len(pcm); i += 2 {
		out[44+i], out[44+i+1] = pcm[i+1], pcm[i]
	}
	return out, true
}

// Sounds reads the disc's button sounds; nil when it has none.
func (d *Disc) Sounds() *Sounds {
	d.soundsOnce.Do(func() {
		b, err := d.Source.ReadFile(d.layout.root+"/AUXDATA/sound.bdmv", MaxStructureBytes)
		if err != nil {
			return
		}
		d.sounds, _ = ParseSounds(b)
	})
	return d.sounds
}
