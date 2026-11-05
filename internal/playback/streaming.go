package playback

import (
	"fmt"
	"slices"
	"strings"
)

// Streaming modes: how a stream is sent, as opposed to what is in it (Decide).
//
// Standard is how it has always worked: one stream, made as good as the settings allow,
// in four-second segments. Robust favours staying smooth over picture quality on a poor
// connection: a ladder of steady bitrates chosen from what the connection was measured
// to carry, two-second segments that each start with a keyframe (so a player can change
// rung or pick up again after a drop at any of them), and players told to keep a bigger
// buffer and to keep trying, with growing waits, rather than give up.
const (
	StreamStandard = "standard"
	StreamRobust   = "robust"
)

// ValidStreamMode says a mode is one of the two. "" is not: callers decide what it means.
func ValidStreamMode(m string) bool { return m == StreamStandard || m == StreamRobust }

// Where a stream's mode came from.
const (
	StreamFromDefault = "default" // nothing chose robust
	StreamFromServer  = "server"  // the server's setting, for viewers away from home
	StreamFromDevice  = "device"  // this device asked
)

// ChooseStreamMode applies the settings: a device's own choice wins wherever it is (a
// phone on poor Wi-Fi at home can ask for robust, a laptop on a fast connection away can
// ask for standard); otherwise the server's setting applies to viewers away from home.
func ChooseStreamMode(server, device string, away bool) (mode, from string) {
	if ValidStreamMode(device) {
		return device, StreamFromDevice
	}
	if server == StreamRobust && away {
		return StreamRobust, StreamFromServer
	}
	return StreamStandard, StreamFromDefault
}

// Segment lengths, in seconds.
const (
	StandardSegmentSec = 4
	RobustSegmentSec   = 2
)

// Rung is one step of a robust ladder: a picture height and the bitrate of the whole
// stream (picture and sound).
type Rung struct {
	Height int `json:"height"`
	Kbps   int `json:"kbps"`
}

// robustRungs are the steps robust streams are made at. Steady and modest on purpose:
// each is about what H.264 needs for a clean picture at that height, not the most it
// could use.
var robustRungs = []Rung{{1080, 5000}, {720, 2800}, {480, 1400}, {360, 800}, {240, 400}}

const (
	// robustShare is the part of a measured connection a robust stream may plan to use:
	// a connection that carried 5 Mb/s in a test seldom carries it all the time.
	robustSharePct = 60
	// unmeasuredKbps is what a connection is taken to carry when nobody measured it:
	// enough for 480p to start, with the player finding out the rest.
	unmeasuredKbps = 2400
	// maxRungs bounds the streams a ladder can start: each is a transcode of its own
	// when a player asks for it.
	maxRungs = 4
)

// Ladder is the rungs a robust stream offers, best first, and the one to start at.
type Ladder struct {
	Rungs []Rung `json:"rungs"`
	Start int    `json:"start"` // index into Rungs
}

// RobustLadder chooses the rungs for a picture srcHeight lines tall. maxHeight and
// capKbps are the limits that apply (0 = none); measuredKbps is what the connection
// carried in the player's test (0 = not measured).
//
// It starts at the best rung that fits in part of the measured speed, and offers one rung
// above it (a player that finds the connection better may climb once) and every rung
// below it (where it steps down to, instead of stalling).
func RobustLadder(srcHeight, maxHeight, capKbps, measuredKbps int) Ladder {
	ceiling := Smallest(srcHeight, maxHeight)
	var rungs []Rung
	for _, r := range robustRungs {
		// Never taller than the picture, except that a small one still gets the lowest step.
		if ceiling > 0 && r.Height > ceiling && r.Height != robustRungs[len(robustRungs)-1].Height {
			continue
		}
		if capKbps > 0 && r.Kbps > capKbps {
			continue
		}
		rungs = append(rungs, r)
	}
	if len(rungs) == 0 {
		low := robustRungs[len(robustRungs)-1]
		if capKbps > 0 {
			low.Kbps = min(low.Kbps, capKbps)
		}
		rungs = []Rung{low}
	}
	if ceiling > 0 && ceiling < rungs[len(rungs)-1].Height {
		rungs[len(rungs)-1].Height = ceiling // a picture smaller than every step
	}
	budget := unmeasuredKbps
	if measuredKbps > 0 {
		budget = measuredKbps * robustSharePct / 100
	}
	start := len(rungs) - 1
	for i, r := range rungs {
		if r.Kbps <= budget {
			start = i
			break
		}
	}
	from := max(0, start-1)
	rungs = rungs[from:min(len(rungs), from+maxRungs)]
	return Ladder{Rungs: slices.Clone(rungs), Start: start - from}
}

// FitsRobust says a file can be sent without converting it in robust mode: its bitrate is
// within half of what the connection was measured to carry, which leaves room for the
// connection to get worse. A file nobody measured the connection for is converted, since
// a file sent as it is can't step down.
func FitsRobust(srcKbps, measuredKbps int) bool {
	return srcKbps > 0 && measuredKbps > 0 && srcKbps*2 <= measuredKbps
}

// PlayerHints are what a player should do in a mode, so that every app behaves alike.
type PlayerHints struct {
	// BufferSec is how far ahead the player tries to keep; MaxBufferSec the most it may.
	BufferSec    int `json:"bufferSec"`
	MaxBufferSec int `json:"maxBufferSec"`
	// Retries: how often a failed segment or playlist is asked for again, waiting
	// FirstRetryMs, then twice that each time, up to MaxRetryMs.
	Retries      int `json:"retries"`
	FirstRetryMs int `json:"firstRetryMs"`
	MaxRetryMs   int `json:"maxRetryMs"`
	// SegmentTimeoutMs is how long to wait for a segment to start arriving: the server may
	// be starting a conversion for it.
	SegmentTimeoutMs int `json:"segmentTimeoutMs"`
	// ResumeOnDrop: when the connection is lost, keep the position and pick up there once
	// it's back (asking /playback again with startSec if the stream is gone), rather than
	// show an error.
	ResumeOnDrop bool `json:"resumeOnDrop"`
	// StepDownOnStall: when the picture stalls, move to a lower rung at once rather than
	// wait for the bandwidth estimate to catch up.
	StepDownOnStall bool `json:"stepDownOnStall"`
}

// Hints returns the player hints for a mode.
func Hints(mode string) PlayerHints {
	if mode == StreamRobust {
		return PlayerHints{BufferSec: 60, MaxBufferSec: 120, Retries: 8, FirstRetryMs: 500, MaxRetryMs: 8000, SegmentTimeoutMs: 30000, ResumeOnDrop: true, StepDownOnStall: true}
	}
	return PlayerHints{BufferSec: 30, MaxBufferSec: 90, Retries: 6, FirstRetryMs: 1000, MaxRetryMs: 8000, SegmentTimeoutMs: 60000}
}

// ExplainStreaming words a stream's mode for the player's information: "" for standard,
// which needs no words. ladder is in the order of the stream's master playlist.
func ExplainStreaming(mode, from string, ladder []Rung, measuredKbps int) string {
	if mode != StreamRobust {
		return ""
	}
	who := "Robust streaming, the server's choice for viewers away from home"
	if from == StreamFromDevice {
		who = "Robust streaming, chosen on this device"
	}
	speed := "nobody measured this connection"
	if measuredKbps > 0 {
		speed = "this connection measured " + mbps(measuredKbps)
	}
	if len(ladder) == 0 {
		return fmt.Sprintf("%s: %s, and the file fits it comfortably, so it's sent as it is, with a larger buffer and patient retries.", who, speed)
	}
	low := ladder[0]
	for _, r := range ladder {
		if r.Height < low.Height {
			low = r
		}
	}
	text := fmt.Sprintf("%s: %s, so it starts at %dp (%s) in two-second segments", who, speed, ladder[0].Height, mbps(ladder[0].Kbps))
	if low.Height < ladder[0].Height {
		text += fmt.Sprintf(", and steps down as far as %dp rather than stall", low.Height)
	}
	return text + "."
}

func mbps(kbps int) string {
	if kbps < 1000 {
		return fmt.Sprintf("%d kb/s", kbps)
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(kbps)/1000), ".0") + " Mb/s"
}
