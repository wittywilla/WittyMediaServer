package cdrip

import (
	"errors"

	"github.com/wittywilla/wittycd"
)

// sessionGap is the unreadable stretch between a disc's music and a data session after it.
const sessionGap = 11400

// parseTOC reads the table of contents a drive reports: 4 bytes of header, then 8 for each
// track and the lead-out. The control nibble is the low one.
func parseTOC(b []byte) (*Info, error) {
	if len(b) < 4 {
		return nil, ErrNoRead
	}
	first, last := int(b[2]), int(b[3])
	type entry struct {
		number, lba int
		data, pre   bool
	}
	var es []entry
	for i := 4; i+8 <= len(b) && len(es) <= last-first+1; i += 8 {
		control := b[i+1] & 0x0f
		lba := int(b[i+5])*60*75 + int(b[i+6])*75 + int(b[i+7]) - LeadIn
		es = append(es, entry{number: int(b[i+2]), lba: lba, data: control&4 != 0, pre: control&1 != 0})
	}
	if len(es) < 2 || es[len(es)-1].number != 0xAA {
		return nil, ErrNoRead
	}
	info := &Info{Kind: "audiocd", Drive: true, Tracks: []Track{}}
	for i := 0; i+1 < len(es); i++ {
		e, next := es[i], es[i+1]
		end := next.lba
		if !e.data && next.data && next.number != 0xAA {
			end -= sessionGap // music, then a data session: the gap can't be read
		}
		if end <= e.lba {
			return nil, ErrNoRead
		}
		info.Tracks = append(info.Tracks, Track{Number: e.number, Audio: !e.data, Pregap: e.lba, Start: e.lba, End: end, PreEmphasis: e.pre})
	}
	audio := info.AudioTracks()
	if len(audio) == 0 {
		return nil, errors.New("this disc has no audio tracks")
	}
	toc := wittycd.TOC{First: audio[0].Number, Last: audio[len(audio)-1].Number, LeadOut: audio[len(audio)-1].End + LeadIn}
	for _, a := range audio {
		toc.Offsets = append(toc.Offsets, a.Start+LeadIn)
	}
	infoOfTOC(info, toc)
	return info, nil
}
