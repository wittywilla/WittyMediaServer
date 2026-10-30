package wittytuner

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/wittywilla/wittytuner/mpegts"
)

// scanMux reads a whole multiplex for a while and works out its channels from
// what the stream says: PAT and PMT always, plus DVB's SDT or ATSC's VCT for
// names. Encrypted services are listed, marked.
func scanMux(ctx context.Context, r io.Reader, wait time.Duration) []muxService {
	type state struct {
		pat  []mpegts.Program
		pmts map[int]mpegts.PMT
		sdt  map[int]mpegts.Service
		vct  map[int]mpegts.VirtualChannel
	}
	st := state{pmts: map[int]mpegts.PMT{}, sdt: map[int]mpegts.Service{}, vct: map[int]mpegts.VirtualChannel{}}
	d := mpegts.NewDemux(mpegts.PIDPAT, mpegts.PIDSDT, mpegts.PIDPSIP)
	d.OnSec = func(pid int, s []byte) {
		switch {
		case pid == mpegts.PIDPAT:
			if p := mpegts.ParsePAT(s); p != nil {
				st.pat = p
				for _, x := range p {
					d.Add(x.PMTPID)
				}
			}
		case pid == mpegts.PIDSDT:
			for _, x := range mpegts.ParseSDT(s) {
				st.sdt[x.ID] = x
			}
		case pid == mpegts.PIDPSIP:
			for _, x := range mpegts.ParseVCT(s) {
				st.vct[x.Program] = x
			}
		case s[0] == 2:
			if m, ok := mpegts.ParsePMT(s); ok {
				st.pmts[m.Program] = m
			}
		}
	}
	deadline := time.Now().Add(wait)
	buf := make([]byte, 188*256)
	var carry []byte
	complete := func() bool {
		if len(st.pat) == 0 {
			return false
		}
		for _, p := range st.pat {
			if _, ok := st.pmts[p.Number]; !ok {
				return false
			}
			if _, ok := st.sdt[p.Number]; !ok {
				if _, ok := st.vct[p.Number]; !ok {
					return false
				}
			}
		}
		return true
	}
	for time.Now().Before(deadline) && ctx.Err() == nil && !complete() {
		n, err := r.Read(buf)
		if n > 0 {
			b := append(carry, buf[:n]...)
			for len(b) >= mpegts.PacketSize {
				if b[0] != 0x47 {
					b = b[1:]
					continue
				}
				if h, ok := mpegts.ParsePacket(b[:mpegts.PacketSize]); ok {
					d.Feed(h)
				}
				b = b[mpegts.PacketSize:]
			}
			carry = append(carry[:0], b...)
		}
		if err != nil {
			break
		}
	}
	var out []muxService
	for _, p := range st.pat {
		m, ok := st.pmts[p.Number]
		sv := muxService{Program: p.Number, PMT: p.PMTPID, Name: fmt.Sprintf("Program %d", p.Number)}
		if ok {
			sv.HD = m.HD()
			sv.Radio = !m.HasVideo()
			if m.CA {
				sv.Encrypted = "the broadcast marks it as conditional access"
			}
			sv.PIDs = append(sv.PIDs, p.PMTPID, m.PCR)
			for _, e := range m.Streams {
				sv.PIDs = append(sv.PIDs, e.PID)
			}
		}
		if x, ok := st.sdt[p.Number]; ok {
			if x.Name != "" {
				sv.Name = x.Name
			}
			if x.FreeCA {
				sv.Encrypted = "the broadcast marks it as conditional access"
			}
			sv.Provider = x.Provider
		}
		if x, ok := st.vct[p.Number]; ok {
			if x.Hidden {
				continue
			}
			if x.Name != "" {
				sv.Name = x.Name
			}
			sv.Major, sv.Minor = x.Major, x.Minor
			sv.Source = x.SourceID
			if x.Encrypted {
				sv.Encrypted = "the broadcast marks it as access controlled"
			}
		}
		out = append(out, sv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Program < out[j].Program })
	return out
}

// muxService is one programme found in a multiplex.
type muxService struct {
	Program      int
	PMT          int
	PIDs         []int
	Name         string
	Provider     string
	HD, Radio    bool
	Encrypted    string
	Major, Minor int
	Source       int
}

// number picks what a viewer would dial.
func (m muxService) number(fallback string) string {
	if m.Major > 0 {
		return fmt.Sprintf("%d.%d", m.Major, m.Minor)
	}
	return fallback
}
