package ffmpeg

import (
	"bufio"
	"context"
	"strings"
	"sync"
	"time"
)

// Filter availability: builds of FFmpeg differ (zscale and tonemap need
// libzimg), so features that depend on a filter ask first.

var filterCache struct {
	once sync.Once
	set  map[string]bool
}

// HasFilter says whether this FFmpeg has a video filter, found once by
// asking `ffmpeg -filters`.
func (t *Tools) HasFilter(name string) bool {
	filterCache.once.Do(func() {
		filterCache.set = map[string]bool{}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		out, _, err := Run(ctx, t.FFmpeg, []string{"-hide_banner", "-filters"})
		if err != nil {
			return
		}
		sc := bufio.NewScanner(strings.NewReader(string(out)))
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			// " T.C zscale  V->V  Apply resizing, colorspace and bit depth conversion."
			if len(f) >= 3 && strings.Contains(f[2], "->") {
				filterCache.set[f[1]] = true
			}
		}
	})
	return filterCache.set[name]
}

// ToneMapFilter is the filter chain that turns HDR (BT.2020 PQ or HLG) video
// into SDR BT.709 for displays and encoders that cannot show HDR. rng says which it
// is, as Stream.Range does. Empty when this FFmpeg lacks the filters, in which case
// the picture plays unmapped (flat and dim, but watchable).
func (t *Tools) ToneMapFilter(rng string) string {
	if !t.HasFilter("zscale") || !t.HasFilter("tonemap") {
		return ""
	}
	return ToneMapChain(rng)
}

// ToneMapChain is the chain itself: linear light, a Hable curve, then BT.709. The frames are
// told first what they are, because a file may only say so in its container, and without
// that zscale stops with "no path between colorspaces". They're told afterwards as well, so
// the encoder marks what comes out as an ordinary picture.
func ToneMapChain(rng string) string {
	trc := "smpte2084"
	if rng == "hlg" {
		trc = "arib-std-b67"
	}
	return "setparams=color_primaries=bt2020:color_trc=" + trc + ":colorspace=bt2020nc," +
		"zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p," +
		"setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709"
}
