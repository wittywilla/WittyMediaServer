// Package session is the contract between a player and a disc engine's module: what
// a player should show right now, which bytes to decode, what to draw over them and
// where the buttons are. WittyDVD and WittyBluray each implement it in their own
// player module, so either can be updated without the other or the player.
//
// It follows the same rules as WittyMediaServer's disc sessions, with one difference:
// nothing is transcoded. Where the server starts an HLS stream, a session hands the
// player a list of byte ranges to decode itself.
package session

import (
	"errors"
)

// Presentation is what the player should show right now.
type Presentation struct {
	Kind      string     `json:"kind"` // menu | title | stop
	Serial    int        `json:"serial"`
	Reason    string     `json:"reason,omitempty"`
	Media     *Media     `json:"media,omitempty"`
	Menu      *Menu      `json:"menu,omitempty"`
	Title     *TitleView `json:"title,omitempty"`
	Protected bool       `json:"protected,omitempty"`
	// TimerSec is set while the disc's navigation timer runs; the player sends
	// a "tick" action when it expires.
	TimerSec float64 `json:"timerSec,omitempty"`
	// Pause is set when a disc's button froze or released the picture.
	Pause *bool `json:"pause,omitempty"`
}

// Media is the picture and sound of one presentation.
type Media struct {
	Type string `json:"type"` // clip | none
	// Token names the stream: the same token means the same bytes, so the
	// player keeps what it is playing and only seeks.
	Token     string `json:"token,omitempty"`
	Container string `json:"container,omitempty"` // mpeg | mpegts
	// Files and Extents are the stream as byte ranges: [file index, offset, length].
	Files    []string   `json:"files,omitempty"`
	Extents  [][3]int64 `json:"extents,omitempty"`
	Segments []Segment  `json:"segments,omitempty"`
	// Video and Audio are stream ids (MPEG-PS) or PIDs (transport streams), 0 =
	// none. AudioFirst plays whichever audio stream shows up first (DVD menus).
	Video      int  `json:"video,omitempty"`
	Audio      int  `json:"audio,omitempty"`
	AudioFirst bool `json:"audioFirst,omitempty"`
	// FrontOnly plays only the front channels: a karaoke stream without vocals.
	FrontOnly bool    `json:"frontOnly,omitempty"`
	Loop      bool    `json:"loop"`
	Duration  float64 `json:"durationSec"`
	// StillSec is how long to hold the last picture after the clip; -1 = until
	// the viewer acts.
	StillSec    float64 `json:"stillSec"`
	Deinterlace bool    `json:"deinterlace,omitempty"`
	Aspect      string  `json:"aspect,omitempty"` // 4:3 | 16:9
	// StartSec is where to begin, StartOffset the byte offset of the sync point
	// at or before it and StartAt that point's time. StartPTS is its raw
	// timestamp on a 90 kHz clock, -1 when unknown.
	StartSec    float64 `json:"startSec,omitempty"`
	StartOffset int64   `json:"startOffset,omitempty"`
	StartAt     float64 `json:"startAt,omitempty"`
	StartPTS    int64   `json:"startPts,omitempty"`
}

// Segment is one stretch of the stream with its place on the timeline. Each may
// restart the disc's clock.
type Segment struct {
	Bytes    int64   `json:"bytes"`
	StartSec float64 `json:"startSec"`
	Duration float64 `json:"durationSec"`
	// BasePTS is the segment's first timestamp on a 90 kHz clock, -1 when it is
	// not known until the stream is read.
	BasePTS int64 `json:"basePts"`
}

// Menu carries the interactive layer.
type Menu struct {
	Width          int          `json:"width"`
	Height         int          `json:"height"`
	Aspect         string       `json:"aspect"` // 4:3 | 16:9
	Buttons        []ButtonView `json:"buttons"`
	Selected       int          `json:"selected"`
	Art            int          `json:"art"` // key for "art" calls
	HasOverlay     bool         `json:"hasOverlay,omitempty"`
	HasHighlights  bool         `json:"hasHighlights"`
	ButtonsFromSec float64      `json:"buttonsFromSec,omitempty"`
	// ButtonsUntilSec is when the disc's buttons stop working before the clip
	// does; 0 = while the clip shows.
	ButtonsUntilSec float64 `json:"buttonsUntilSec,omitempty"`
	// ForcedAction is a button the disc presses by itself ActionAtSec into the
	// clip. 0 = none.
	ForcedAction int     `json:"forcedAction,omitempty"`
	ActionAtSec  float64 `json:"actionAtSec,omitempty"`
	InTitle      bool    `json:"inTitle,omitempty"`
	// Popup marks a menu summoned during a title. Closed tells the player to
	// hide it (a button ran POPUP_OFF).
	Popup          bool    `json:"popup,omitempty"`
	Closed         bool    `json:"closed,omitempty"`
	UserTimeoutSec float64 `json:"userTimeoutSec,omitempty"`
	HasSounds      bool    `json:"hasSounds,omitempty"`
	// InEffect is a page's opening transition: seconds per picture.
	InEffect []float64 `json:"inEffect,omitempty"`
}

// ButtonView is one button of a menu, in menu pixels.
type ButtonView struct {
	N     int  `json:"n"`
	X1    int  `json:"x1"`
	Y1    int  `json:"y1"`
	X2    int  `json:"x2"`
	Y2    int  `json:"y2"`
	Up    int  `json:"up"`
	Down  int  `json:"down"`
	Left  int  `json:"left"`
	Right int  `json:"right"`
	Auto  bool `json:"auto"`
	Lit   bool `json:"lit"`
	// Frames and FPS describe an animated selected state.
	Frames int     `json:"frames,omitempty"`
	FPS    float64 `json:"fps,omitempty"`
	// Num is the number key that presses it, when that is not N.
	Num int `json:"num,omitempty"`
	// SoundSelect and SoundPress are sound ids + 1 (0 = silent).
	SoundSelect int `json:"soundSelect,omitempty"`
	SoundPress  int `json:"soundPress,omitempty"`
}

// TitleView describes feature playback.
type TitleView struct {
	TTN       int       `json:"ttn"`
	Chapter   int       `json:"chapter"`
	Duration  float64   `json:"durationSec"`
	StartSec  float64   `json:"startSec"`
	Chapters  []float64 `json:"chapters"`
	Audio     []Track   `json:"audio"`
	Subtitles []Track   `json:"subtitles"`
	AudioN    int       `json:"audioStream"`
	SubN      int       `json:"subtitleStream"` // -1 off
	FrameW    int       `json:"frameW"`
	FrameH    int       `json:"frameH"`
	Aspect    string    `json:"aspect"` // 4:3 | 16:9
	// ForcedStream is the stream whose forced subtitles show even with
	// subtitles off, -1 for none.
	ForcedStream int `json:"forcedStream"`
	Angle        int `json:"angle"`
	Angles       int `json:"angles"`
	// Locks names what the disc forbids here: seek, chapter, skip, pause,
	// audio, subtitle, angle.
	Locks []string `json:"locks,omitempty"`
	// Palette is the DVD colour table for its subtitles, as RRGGBB.
	Palette []string `json:"palette,omitempty"`
}

// Track is one audio or subtitle stream.
type Track struct {
	N        int    `json:"n"`
	Language string `json:"language"`
	Codec    string `json:"codec,omitempty"`
	Channels int    `json:"channels,omitempty"`
	Purpose  string `json:"purpose,omitempty"`
	// ID is the stream id or PID in the stream being played. External means it
	// isn't in that stream: the player asks for it with a "subs" call.
	ID       int  `json:"id,omitempty"`
	External bool `json:"external,omitempty"`
}

// SubEvent is one subtitle kept outside the main stream, on the title timeline.
type SubEvent struct {
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	X      int     `json:"x"`
	Y      int     `json:"y"`
	W      int     `json:"w"`
	H      int     `json:"h"`
	Forced bool    `json:"forced,omitempty"`
	Pic    string  `json:"pic,omitempty"` // key for a "subpic" call
	// Text subtitles carry words instead of a picture.
	Text   string `json:"text,omitempty"`
	Color  string `json:"color,omitempty"`
	Size   int    `json:"size,omitempty"`
	HAlign int    `json:"halign,omitempty"` // 1 left, 2 centre, 3 right
	VAlign int    `json:"valign,omitempty"` // 1 top, 2 middle, 3 bottom
	Bold   bool   `json:"bold,omitempty"`
	Italic bool   `json:"italic,omitempty"`
}

// Location is a place in the stream a player can start decoding from.
type Location struct {
	Offset int64   `json:"offset"`
	Sec    float64 `json:"sec"`
	PTS    int64   `json:"pts"`
}

// Prefs are the viewer's preferences for a session.
type Prefs struct {
	MenuLanguage     string `json:"menuLanguage"`
	AudioLanguage    string `json:"audioLanguage"`
	SubtitleLanguage string `json:"subtitleLanguage"`
	LowPower         bool   `json:"lowPower"`
}

// Action is an input from the player.
type Action struct {
	Type        string  `json:"type"` // press | select | key | ended | title | menu | resume | chapter | audio | subtitle | angle | tick
	Button      int     `json:"button"`
	Key         string  `json:"key"` // up down left right enter menu title resume return next prev
	TTN         int     `json:"ttn"`
	Chapter     int     `json:"chapter"`
	Stream      int     `json:"stream"`
	On          bool    `json:"on"`
	Menu        string  `json:"menu"` // root | title | audio | subpicture | angle | chapter
	PositionSec float64 `json:"positionSec"`
}

// TitleInfo is one title for the Titles menu.
type TitleInfo struct {
	N        int     `json:"n"`
	Duration float64 `json:"durationSec"`
	Chapters int     `json:"chapters"`
	Main     bool    `json:"main,omitempty"`
}

// Info describes an opened disc.
type Info struct {
	Kind   string      `json:"kind"` // dvd | bluray
	Label  string      `json:"label"`
	Main   int         `json:"mainTitle"`
	Titles []TitleInfo `json:"titles"`
	// Notes are plain things the viewer should know, such as a Java menu.
	Notes []string `json:"notes,omitempty"`
}

// Session is one opened disc. Calls are not concurrent: the helper handles one
// at a time.
type Session interface {
	Info() Info
	Start() *Presentation
	PlayTitle(ttn, chapter int) *Presentation
	Do(a Action) *Presentation
	Locate(sec float64) (Location, error)
	// Art is a picture of the menu with key art: overlay, highlight (button n,
	// frame f) or effect (picture n).
	Art(art int, kind string, n, f int) ([]byte, bool)
	Sound(id int) ([]byte, bool)
	Subtitles(ttn, n int, from, to float64) ([]SubEvent, error)
	SubPic(key string) ([]byte, bool)
	Close() error
}

// Opener opens a disc for a session. drive says it is read straight from a disc drive,
// where a read failure means something different than in a file.
type Opener func(path string, drive bool, prefs Prefs) (Session, error)

// Error codes the window turns into plain messages.
const (
	CodeProtected  = "protected"
	CodeNoMedia    = "no-media"
	CodeNotVideo   = "not-video"
	CodeAudioCD    = "audio-cd"
	CodeUnreadable = "unreadable"
	CodeMissing    = "missing"
)

// RefusalError is a disc the player won't or can't play, in words for the viewer.
type RefusalError struct {
	Code    string
	Message string
}

func (e *RefusalError) Error() string { return e.Message }

// Plain messages. The player never unlocks anything, so a protected disc gets a
// straight answer and no way around.
const (
	MsgDVDProtected    = "This DVD is copy-protected (CSS). WittyMediaPlayer doesn't unlock copy protection, so it can't play this disc."
	MsgBlurayProtected = "This Blu-ray is copy-protected (AACS). WittyMediaPlayer doesn't unlock copy protection, so it can't play this disc."
	MsgDriveUnreadable = "The drive couldn't read this disc's video. Copy-protected discs often fail this way, and WittyMediaPlayer doesn't unlock copy protection. If the disc isn't protected, try cleaning it."
	MsgNoMedia         = "There is no disc in this drive."
	MsgAudioCD         = "This is an audio CD. WittyMediaPlayer plays DVDs and Blu-rays from a drive; audio CDs aren't supported yet."
	MsgNotVideo        = "This isn't a DVD or Blu-ray. Its files can be opened one at a time with File, Open."
)

// IsProtected says an error is the refusal of a protected disc.
func IsProtected(err error) bool {
	var r *RefusalError
	return errors.As(err, &r) && r.Code == CodeProtected
}

// PlainReason words a read failure for the viewer; refusals are already worded.
func PlainReason(err error) string {
	var r *RefusalError
	if errors.As(err, &r) {
		return r.Message
	}
	return "This part of the disc could not be read: " + err.Error()
}

// StopPresentation is the end of the disc's program, or a screen that can't be shown.
func StopPresentation(serial int, reason string) *Presentation {
	return &Presentation{Kind: "stop", Serial: serial, Reason: reason, Media: &Media{Type: "none"}}
}

// Extent is a byte range of a file.
type Extent struct {
	File   string
	Offset int64
	Length int64
}

// NewMedia lays stretches of bytes out as a stream, each with its duration.
func NewMedia(container, token string, parts [][]Extent, durations []float64) *Media {
	m := &Media{Type: "clip", Container: container, Token: token, StartPTS: -1}
	files := map[string]int{}
	var at float64
	for i, part := range parts {
		var n int64
		for _, e := range part {
			k, ok := files[e.File]
			if !ok {
				k = len(m.Files)
				files[e.File] = k
				m.Files = append(m.Files, e.File)
			}
			m.Extents = append(m.Extents, [3]int64{int64(k), e.Offset, e.Length})
			n += e.Length
		}
		d := 0.0
		if i < len(durations) {
			d = durations[i]
		}
		m.Segments = append(m.Segments, Segment{Bytes: n, StartSec: at, Duration: d, BasePTS: -1})
		at += d
	}
	return m
}
