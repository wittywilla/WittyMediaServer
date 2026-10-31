// Package engine is what the host needs from something that runs a game: a
// libretro core in production, a small built-in core in tests.
package engine

// Pixel formats, numbered as libretro numbers them.
const (
	Format0RGB1555 = 0
	FormatXRGB8888 = 1
	FormatRGB565   = 2
)

// BytesPerPixel for a pixel format.
func BytesPerPixel(format int) int {
	if format == FormatXRGB8888 {
		return 4
	}
	return 2
}

// MaxPorts is how many players the protocol carries input for.
const MaxPorts = 8

// RetroPad buttons, as bit numbers in PadState.Buttons.
const (
	ButtonB = iota
	ButtonY
	ButtonSelect
	ButtonStart
	ButtonUp
	ButtonDown
	ButtonLeft
	ButtonRight
	ButtonA
	ButtonX
	ButtonL
	ButtonR
	ButtonL2
	ButtonR2
	ButtonL3
	ButtonR3
)

// PadState is one player's input.
type PadState struct {
	Buttons uint16
	LX, LY  int16
	RX, RY  int16
}

// Handlers receive what an engine produces. All are called on the thread
// running Run, and slices are only valid during the call. Any may be nil.
type Handlers struct {
	Video    func(data []byte, width, height, stride, format int)
	Dupe     func() // the previous frame again
	Audio    func(stereo []int16)
	Input    func() *[MaxPorts]PadState // once a frame
	Rumble   func(port, effect, strength int)
	Message  func(text string)
	Log      func(level int, text string)
	Geometry func(width, height int, aspect float64)
	Rotation func(quarterTurns int)
	AVInfo   func(info AVInfo)
	Shutdown func()
}

// AVInfo is what an engine says about its output.
type AVInfo struct {
	BaseWidth, BaseHeight int
	MaxWidth, MaxHeight   int
	Aspect                float64
	FPS                   float64
	SampleRate            float64
}

// SystemInfo is an engine's description of itself.
type SystemInfo struct {
	Name, Version string
	Extensions    []string
	NeedFullPath  bool
	BlockExtract  bool
}

// Option is one setting an engine offers. The first choice is its default.
type Option struct {
	Key      string
	Label    string
	Info     string // a longer description
	Category string // the heading it belongs under
	Choices  []string
	Labels   []string // the name of each choice, when the core gives them
	Default  string
	Value    string
	Hidden   bool // the core says it doesn't apply just now
}

// Options is an engine with settings of its own. SetOption refuses a key or a value
// the engine didn't offer.
type Options interface {
	Options() []Option
	SetOption(key, value string) bool
}

// OptionsChanged is an engine whose options can change on their own: it says when they have.
type OptionsChanged interface {
	OptionsChanged() bool
}

// Cheats is an engine that takes cheat codes.
type Cheats interface {
	CheatReset()
	CheatSet(index int, enabled bool, code string)
}

// Output tells an engine what's wanted from the next frame, so that one which can
// skip drawing or mixing saves the work.
type Output interface {
	SetOutput(video, audio, fastForward bool)
}

// StateInto is an engine that can save a state into a buffer it's given, for taking
// one every frame without making garbage. It returns nil when it can't.
type StateInto interface {
	SaveStateInto(buf []byte) []byte
}

// Engine runs one game. Every method is called from one locked OS thread.
type Engine interface {
	SystemInfo() SystemInfo
	LoadGame(path string) (AVInfo, error)
	Run()
	Reset()
	SaveState() []byte
	LoadState(b []byte) bool
	SaveRAM() []byte // the engine's own memory; nil when the game has none
	Close()
}
