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
// into SDR BT.709 for displays and encoders that cannot show HDR: linear
// light, a Hable curve, then BT.709. Empty when this FFmpeg lacks the
// filters, in which case the picture plays unmapped (flat and dim, but
// watchable).
func (t *Tools) ToneMapFilter() string {
	if !t.HasFilter("zscale") || !t.HasFilter("tonemap") {
		return ""
	}
	return "zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt709,tonemap=tonemap=hable:desat=0,zscale=t=bt709:m=bt709:r=tv,format=yuv420p"
}
