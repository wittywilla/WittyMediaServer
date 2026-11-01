package ffmpeg

import (
	"context"
	"strings"
	"sync"
	"time"
)

var decoderSets sync.Map // ffmpeg path -> map[string]bool

// CanDecode says whether this FFmpeg has a decoder by this name. Builds differ:
// AC-4, for one, is in none we ship today. When it can't be found out, the
// answer is yes, and FFmpeg itself has the last word.
func (t *Tools) CanDecode(name string) bool {
	if t == nil || t.FFmpeg == "" {
		return true
	}
	if v, ok := decoderSets.Load(t.FFmpeg); ok {
		set := v.(map[string]bool)
		return len(set) == 0 || set[name]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	set := map[string]bool{}
	if out, _, err := Run(ctx, t.FFmpeg, []string{"-hide_banner", "-decoders"}); err == nil {
		for k := range parseDecoders(string(out)) {
			set[k] = true
		}
	}
	decoderSets.Store(t.FFmpeg, set)
	return len(set) == 0 || set[name]
}

// parseDecoders reads `ffmpeg -decoders`: flags, a name, a description.
func parseDecoders(out string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && len(f[0]) == 6 && strings.ContainsAny(f[0][:1], "VAS") && f[1] != "=" {
			set[f[1]] = true
		}
	}
	return set
}

var (
	demuxers     map[string]bool
	demuxersOnce sync.Once
)

// HasDemuxer says whether this FFmpeg can read an input of this name, found once by
// asking it. An FFmpeg that can't be asked has none.
func (t *Tools) HasDemuxer(name string) bool {
	demuxersOnce.Do(func() {
		demuxers = map[string]bool{}
		if t.FFmpeg == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, _, err := Run(ctx, t.FFmpeg, []string{"-hide_banner", "-demuxers"})
		if err != nil {
			return
		}
		for _, line := range strings.Split(string(out), "\n") {
			if f := strings.Fields(line); len(f) >= 2 && strings.Contains(f[0], "D") {
				for _, n := range strings.Split(f[1], ",") {
					demuxers[n] = true
				}
			}
		}
	})
	return demuxers[name]
}
