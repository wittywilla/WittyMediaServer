package ffmpeg

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// What a video stream is, read from a probe. Playback and library conversion ask the
// same questions, so the answers live with the probe.

// IsInterlaced: the stream says its fields are stored one after the other.
func (s *Stream) IsInterlaced() bool {
	switch s.FieldOrder {
	case "tt", "bb", "tb", "bt":
		return true
	}
	return false
}

// Depth is the bits per colour sample.
func (s *Stream) Depth() int {
	if n, _ := strconv.Atoi(s.BitsPerSample); n > 0 {
		return n
	}
	switch {
	case strings.Contains(s.PixFmt, "12"):
		return 12
	case strings.Contains(s.PixFmt, "10"):
		return 10
	}
	return 8
}

// DolbyVision finds the stream's Dolby Vision record: its profile and the kind of base
// layer other players can show. ok is false for a stream without one.
func (s *Stream) DolbyVision() (profile, compat int, ok bool) {
	for _, sd := range s.SideData {
		if t, _ := sd["side_data_type"].(string); t == "DOVI configuration record" {
			p, _ := sd["dv_profile"].(float64)
			c, _ := sd["dv_bl_signal_compatibility_id"].(float64)
			return int(p), int(c), true
		}
	}
	return 0, 0, false
}

// Range classes the picture: sdr, hdr10 (PQ), hlg or dv5. Dolby Vision with a base layer
// other players can show counts as that base layer; profile 5 has none (its colours come
// out wrong without a Dolby Vision decoder).
func (s *Stream) Range() string {
	if p, c, ok := s.DolbyVision(); ok {
		if p == 5 {
			return "dv5"
		}
		// The base layer's kind, by the record itself when the stream tags don't say.
		switch c {
		case 1, 6:
			return "hdr10"
		case 2:
			return "sdr"
		case 4:
			return "hlg"
		}
	}
	switch s.ColorTransfer {
	case "smpte2084":
		return "hdr10"
	case "arib-std-b67":
		return "hlg"
	}
	return "sdr"
}

// DynamicRange names what the range alone doesn't say, for the player's information.
func (s *Stream) DynamicRange() string {
	if p, c, ok := s.DolbyVision(); ok {
		return fmt.Sprintf("Dolby Vision profile %d.%d", p, c)
	}
	for _, sd := range s.SideData {
		if t, _ := sd["side_data_type"].(string); t == "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)" {
			return "HDR10+"
		}
	}
	return ""
}

// StereoMode finds a 3D picture stored in one frame, from what the file says or the way
// such files are named: sbs (side by side), tab (one above the other), or "".
func (s *Stream) StereoMode(path string) string {
	mode := strings.ToLower(s.Tags["stereo_mode"])
	for _, sd := range s.SideData {
		if t, _ := sd["side_data_type"].(string); t == "Stereo 3D" {
			if k, ok := sd["type"].(string); ok {
				mode = strings.ToLower(k)
			}
		}
	}
	switch {
	case strings.Contains(mode, "side") && !strings.Contains(mode, "mono"):
		return "sbs"
	case strings.Contains(mode, "top") || strings.Contains(mode, "bottom"):
		return "tab"
	}
	name := strings.ToLower(filepath.Base(path))
	switch {
	case stereoNameSBS.MatchString(name):
		return "sbs"
	case stereoNameTAB.MatchString(name):
		return "tab"
	}
	return ""
}

var (
	stereoNameSBS = regexp.MustCompile(`(?:^|[\s._\-\[(])(?:half-?)?(?:h-?)?sbs(?:$|[\s._\-\])])|3d[\s._-]*(?:h-?)?sbs`)
	stereoNameTAB = regexp.MustCompile(`(?:^|[\s._\-\[(])(?:half-?)?(?:h-?)?(?:tab|ou)(?:$|[\s._\-\])])`)
)

// FlattenFilter keeps the left eye of a 3D picture. A half-width pair is stretched back
// to its proper shape; a full-width one is simply cut in two.
func FlattenFilter(mode string, width, height int) string {
	wide := height > 0 && float64(width)/float64(height) >= 2.8
	tall := height > 0 && float64(width)/float64(height) <= 1.1
	switch {
	case mode == "sbs" && wide:
		return "crop=iw/2:ih:0:0"
	case mode == "sbs":
		return "crop=iw/2:ih:0:0,scale=iw*2:ih,setsar=1"
	case mode == "tab" && tall:
		return "crop=iw:ih/2:0:0"
	case mode == "tab":
		return "crop=iw:ih/2:0:0,scale=iw:ih*2,setsar=1"
	}
	return ""
}

// FrameRates reads the stream's average and nominal rates. Either is 0 when not known.
func (s *Stream) FrameRates() (avg, nominal float64) {
	return ratio(s.AvgFrameRate), ratio(s.RFrameRate)
}

func ratio(s string) float64 {
	a, b, ok := strings.Cut(s, "/")
	if !ok {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	n, _ := strconv.ParseFloat(a, 64)
	d, _ := strconv.ParseFloat(b, 64)
	if d == 0 {
		return 0
	}
	return n / d
}
