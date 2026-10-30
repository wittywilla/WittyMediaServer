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
	if out, err := Command(ctx, t.FFmpeg, "-hide_banner", "-decoders").Output(); err == nil {
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
