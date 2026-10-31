package wittyplay

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"

	"github.com/wittywilla/wittyplay/ipc"
)

// Settings are how a person likes a game to look, sound and behave. Each setting is
// described here once, as data: the server checks values against it, and every client
// draws its controls from it. Values live in layers, from the built-in defaults through
// what an administrator set for the server to what a person set for everything, for a
// console and for one game. The most specific layer that has a value wins.
//
// A setting is data and only ever data. None of them can name a file, reach the network
// or change what the sandbox allows.

// Levels a layer can sit at, least specific first.
const (
	LevelBuiltin       = "builtin"
	LevelServer        = "server"
	LevelServerConsole = "serverConsole"
	LevelPerson        = "person"
	LevelConsole       = "console"
	LevelGame          = "game"
)

// Kinds of setting.
const (
	KindToggle = "toggle"
	KindChoice = "choice"
	KindSlider = "slider"
	KindColor  = "color"
	KindBezel  = "bezel"
)

// Groups of settings, in the order menus show them.
const (
	GroupDisplay   = "display"
	GroupOSD       = "osd"
	GroupEmulation = "emulation"
	GroupAudio     = "audio"
	GroupStates    = "states"
	GroupHotkeys   = "hotkeys"
)

// Choice is one value of a choice setting.
type Choice struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Setting describes one setting.
type Setting struct {
	Key     string   `json:"key"`
	Group   string   `json:"group"`
	Section string   `json:"section,omitempty"` // a heading within the group
	Label   string   `json:"label"`
	Help    string   `json:"help,omitempty"`
	Caution string   `json:"caution,omitempty"` // something to know first, such as flashing
	Kind    string   `json:"kind"`
	Default any      `json:"default"`
	Min     float64  `json:"min,omitempty"`
	Max     float64  `json:"max,omitempty"`
	Step    float64  `json:"step,omitempty"`
	Unit    string   `json:"unit,omitempty"`
	Choices []Choice `json:"choices,omitempty"`
	// Shader settings need the drawing path that costs a little; the rest don't.
	Shader bool `json:"shader,omitempty"`
	// Motion: this setting animates, so it stays off for people who ask for less motion.
	Motion bool `json:"motion,omitempty"`
}

func choices(pairs ...string) []Choice {
	out := make([]Choice, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Choice{pairs[i], pairs[i+1]})
	}
	return out
}

var padButtons = choices("none", "Not used", "a", "A", "b", "B", "x", "X", "y", "Y", "l", "L", "r", "R", "l2", "L2", "r2", "R2", "l3", "L3", "r3", "R3", "up", "Up", "down", "Down", "left", "Left", "right", "Right", "start", "Start")

// Settings is every setting there is.
var Settings = []Setting{
	// The picture: all of it happens in the browser, on what has already arrived.
	{Key: "display.scaling", Group: GroupDisplay, Section: "Scaling", Label: "Scaling", Help: "Sharp keeps pixels crisp at any size. Pixel is exact squares, which can look uneven unless the size is a whole number. Smooth blurs them.", Kind: KindChoice, Default: "sharp", Choices: choices("sharp", "Sharp", "pixel", "Pixel", "smooth", "Smooth")},
	{Key: "display.integer", Group: GroupDisplay, Section: "Scaling", Label: "Whole-number scale", Help: "Every game pixel is the same size. The picture may not fill the screen.", Kind: KindToggle, Default: false},
	{Key: "display.aspect", Group: GroupDisplay, Section: "Scaling", Label: "Shape", Kind: KindChoice, Default: "original", Choices: choices("original", "As the game says", "4:3", "4:3", "16:9", "16:9", "square", "Square pixels", "stretch", "Fill the screen", "custom", "Custom")},
	{Key: "display.aspectCustom", Group: GroupDisplay, Section: "Scaling", Label: "Custom shape", Help: "Width divided by height, used when Shape is Custom.", Kind: KindSlider, Default: 1.33, Min: 0.5, Max: 3, Step: 0.01},
	{Key: "display.cropX", Group: GroupDisplay, Section: "Scaling", Label: "Crop the sides", Help: "Old consoles drew past the edge of a TV. This hides that part.", Kind: KindSlider, Default: 0.0, Min: 0, Max: 20, Step: 0.5, Unit: "%"},
	{Key: "display.cropY", Group: GroupDisplay, Section: "Scaling", Label: "Crop top and bottom", Kind: KindSlider, Default: 0.0, Min: 0, Max: 20, Step: 0.5, Unit: "%"},
	{Key: "display.rotate", Group: GroupDisplay, Section: "Scaling", Label: "Rotate", Kind: KindChoice, Default: "0", Choices: choices("0", "None", "90", "90° clockwise", "180", "180°", "270", "90° anticlockwise")},

	{Key: "display.scanlines", Group: GroupDisplay, Section: "Screen effects", Label: "Scanlines", Kind: KindSlider, Default: 0.0, Min: 0, Max: 1, Step: 0.05, Shader: true},
	{Key: "display.curve", Group: GroupDisplay, Section: "Screen effects", Label: "CRT curve", Kind: KindSlider, Default: 0.0, Min: 0, Max: 1, Step: 0.05, Shader: true},
	{Key: "display.mask", Group: GroupDisplay, Section: "Screen effects", Label: "CRT phosphor pattern", Kind: KindSlider, Default: 0.0, Min: 0, Max: 1, Step: 0.05, Shader: true},
	{Key: "display.glow", Group: GroupDisplay, Section: "Screen effects", Label: "Glow", Kind: KindSlider, Default: 0.0, Min: 0, Max: 1, Step: 0.05, Shader: true},
	{Key: "display.vignette", Group: GroupDisplay, Section: "Screen effects", Label: "Dark corners", Kind: KindSlider, Default: 0.0, Min: 0, Max: 1, Step: 0.05, Shader: true},
	{Key: "display.lcdGrid", Group: GroupDisplay, Section: "Screen effects", Label: "LCD grid", Help: "The gaps between pixels on a handheld's screen.", Kind: KindSlider, Default: 0.0, Min: 0, Max: 1, Step: 0.05, Shader: true},
	{Key: "display.blend", Group: GroupDisplay, Section: "Screen effects", Label: "Frame blend", Help: "Mixes each picture with the last, as a slow LCD did. Handheld games that flicker sprites on purpose look right with it.", Kind: KindSlider, Default: 0.0, Min: 0, Max: 0.8, Step: 0.05, Shader: true},
	{Key: "display.flicker", Group: GroupDisplay, Section: "Screen effects", Label: "Tube flicker", Caution: "This makes the picture flicker. Leave it off if flashing or flickering light affects you. It stays off when your device asks for reduced motion.", Kind: KindSlider, Default: 0.0, Min: 0, Max: 1, Step: 0.05, Shader: true, Motion: true},

	{Key: "display.brightness", Group: GroupDisplay, Section: "Colour", Label: "Brightness", Kind: KindSlider, Default: 1.0, Min: 0.5, Max: 1.5, Step: 0.01},
	{Key: "display.contrast", Group: GroupDisplay, Section: "Colour", Label: "Contrast", Kind: KindSlider, Default: 1.0, Min: 0.5, Max: 1.5, Step: 0.01},
	{Key: "display.saturation", Group: GroupDisplay, Section: "Colour", Label: "Colour strength", Kind: KindSlider, Default: 1.0, Min: 0, Max: 2, Step: 0.01},
	{Key: "display.gamma", Group: GroupDisplay, Section: "Colour", Label: "Gamma", Kind: KindSlider, Default: 1.0, Min: 0.5, Max: 2, Step: 0.01, Shader: true},

	{Key: "display.bezel", Group: GroupDisplay, Section: "Around the picture", Label: "Around the picture", Kind: KindChoice, Default: "none", Choices: choices("none", "Black", "color", "A colour", "image", "An image of yours")},
	{Key: "display.bezelColor", Group: GroupDisplay, Section: "Around the picture", Label: "Colour", Kind: KindColor, Default: "#14161e"},
	{Key: "display.bezelImage", Group: GroupDisplay, Section: "Around the picture", Label: "Image", Kind: KindBezel, Default: 0.0},
	{Key: "display.bezelInset", Group: GroupDisplay, Section: "Around the picture", Label: "Picture size inside it", Help: "Makes room for what's around the picture.", Kind: KindSlider, Default: 0.0, Min: 0, Max: 45, Step: 1, Unit: "%"},

	// On-screen numbers: off until someone wants them, because they ask the server for a little.
	{Key: "osd.on", Group: GroupOSD, Label: "Show numbers while playing", Kind: KindToggle, Default: false},
	{Key: "osd.corner", Group: GroupOSD, Label: "Corner", Kind: KindChoice, Default: "tl", Choices: choices("tl", "Top left", "tr", "Top right", "bl", "Bottom left", "br", "Bottom right")},
	{Key: "osd.size", Group: GroupOSD, Label: "Text size", Kind: KindChoice, Default: "normal", Choices: choices("small", "Small", "normal", "Normal", "large", "Large")},
	{Key: "osd.fps", Group: GroupOSD, Section: "What to show", Label: "Frames shown here", Kind: KindToggle, Default: true},
	{Key: "osd.gameFps", Group: GroupOSD, Section: "What to show", Label: "Frames the game ran", Kind: KindToggle, Default: true},
	{Key: "osd.latency", Group: GroupOSD, Section: "What to show", Label: "Network delay", Kind: KindToggle, Default: true},
	{Key: "osd.encode", Group: GroupOSD, Section: "What to show", Label: "Encode time", Kind: KindToggle, Default: false},
	{Key: "osd.bitrate", Group: GroupOSD, Section: "What to show", Label: "Bitrate", Kind: KindToggle, Default: false},
	{Key: "osd.dropped", Group: GroupOSD, Section: "What to show", Label: "Dropped frames", Kind: KindToggle, Default: false},
	{Key: "osd.resolution", Group: GroupOSD, Section: "What to show", Label: "Resolution", Kind: KindToggle, Default: false},
	{Key: "osd.core", Group: GroupOSD, Section: "What to show", Label: "Core and encoder", Kind: KindToggle, Default: false},

	// How the game runs: these reach the runner.
	{Key: "emulation.frameSkip", Group: GroupEmulation, Section: "Speed", Label: "Frame skip", Help: "Runs every frame but sends fewer. Helps a slow server. Automatic skips only when it falls behind.", Kind: KindChoice, Default: "off", Choices: choices("off", "Off", "auto", "Automatic", "1", "Skip 1", "2", "Skip 2", "3", "Skip 3", "4", "Skip 4")},
	{Key: "emulation.ffSpeed", Group: GroupEmulation, Section: "Speed", Label: "Fast-forward speed", Help: "This server may hold it lower.", Caution: "Fast-forward can make a game flash quickly.", Kind: KindSlider, Default: 4.0, Min: 1.5, Max: 16, Step: 0.5, Unit: "×"},
	{Key: "emulation.ffMode", Group: GroupEmulation, Section: "Speed", Label: "Fast-forward button", Kind: KindChoice, Default: "hold", Choices: choices("hold", "Hold it", "toggle", "Press on, press off")},
	{Key: "emulation.slowSpeed", Group: GroupEmulation, Section: "Speed", Label: "Slow-motion speed", Kind: KindSlider, Default: 0.5, Min: 0.1, Max: 0.9, Step: 0.05, Unit: "×"},
	{Key: "emulation.rewind", Group: GroupEmulation, Section: "Rewind", Label: "Rewind", Help: "Keeps the last few seconds in memory on the server. Off by default, as it costs a little all the time.", Kind: KindToggle, Default: false},
	{Key: "emulation.rewindSeconds", Group: GroupEmulation, Section: "Rewind", Label: "How far back", Help: "The server's memory limit can make it less.", Kind: KindSlider, Default: 20.0, Min: 5, Max: 120, Step: 5, Unit: "s"},
	{Key: "emulation.runAhead", Group: GroupEmulation, Section: "Latency", Label: "Run-ahead", Help: "Takes frames off the delay by running the game ahead and going back. Costs the server more, and can glitch on some games. It turns itself off when it doesn't fit.", Kind: KindSlider, Default: 0.0, Min: 0, Max: 3, Step: 1, Unit: " frames"},
	{Key: "emulation.pauseHidden", Group: GroupEmulation, Section: "When you step away", Label: "Pause when this tab is hidden", Kind: KindToggle, Default: true},

	{Key: "audio.volume", Group: GroupAudio, Label: "Volume", Kind: KindSlider, Default: 1.0, Min: 0, Max: 1, Step: 0.05},
	{Key: "audio.mute", Group: GroupAudio, Label: "Mute", Kind: KindToggle, Default: false},
	{Key: "audio.latency", Group: GroupAudio, Label: "Sound buffer", Help: "A bigger buffer stops crackling on a poor connection, and puts the sound a little behind the picture.", Kind: KindChoice, Default: "lowest", Choices: choices("lowest", "Lowest delay", "balanced", "Balanced", "smooth", "Smoothest")},

	{Key: "states.autoSave", Group: GroupStates, Label: "Save automatically on exit", Kind: KindToggle, Default: true},
	{Key: "states.resume", Group: GroupStates, Label: "When a game is opened", Kind: KindChoice, Default: "ask", Choices: choices("ask", "Offer to resume", "always", "Always resume", "never", "Start from the beginning")},
	{Key: "states.quickSlot", Group: GroupStates, Label: "Quick-save slot", Kind: KindSlider, Default: 1.0, Min: 1, Max: MaxStateSlot, Step: 1},
	{Key: "states.confirm", Group: GroupStates, Label: "Ask before replacing a saved state", Kind: KindToggle, Default: true},

	{Key: "hotkeys.padCombos", Group: GroupHotkeys, Section: "Controller", Label: "Controller shortcuts", Help: "Hold the shortcut button and press another. Start and Select together, or Guide, always opens the menu.", Kind: KindToggle, Default: true},
	{Key: "hotkeys.padModifier", Group: GroupHotkeys, Section: "Controller", Label: "Shortcut button", Help: "Held for every shortcut. The game also sees it, unless it's a button the console doesn't have.", Kind: KindChoice, Default: "select", Choices: choices("select", "Select", "l3", "Left stick press", "r3", "Right stick press")},
	{Key: "hotkeys.pad.fastForward", Group: GroupHotkeys, Section: "Controller", Label: "Fast-forward", Kind: KindChoice, Default: "r", Choices: padButtons},
	{Key: "hotkeys.pad.rewind", Group: GroupHotkeys, Section: "Controller", Label: "Rewind", Kind: KindChoice, Default: "l", Choices: padButtons},
	{Key: "hotkeys.pad.quickSave", Group: GroupHotkeys, Section: "Controller", Label: "Quick save", Kind: KindChoice, Default: "r2", Choices: padButtons},
	{Key: "hotkeys.pad.quickLoad", Group: GroupHotkeys, Section: "Controller", Label: "Quick load", Kind: KindChoice, Default: "l2", Choices: padButtons},
	{Key: "hotkeys.pad.slow", Group: GroupHotkeys, Section: "Controller", Label: "Slow motion", Kind: KindChoice, Default: "x", Choices: padButtons},
	{Key: "hotkeys.pad.pause", Group: GroupHotkeys, Section: "Controller", Label: "Pause", Kind: KindChoice, Default: "y", Choices: padButtons},
	{Key: "hotkeys.pad.advance", Group: GroupHotkeys, Section: "Controller", Label: "Next frame", Kind: KindChoice, Default: "right", Choices: padButtons},
	{Key: "hotkeys.pad.screenshot", Group: GroupHotkeys, Section: "Controller", Label: "Screenshot", Kind: KindChoice, Default: "a", Choices: padButtons},
	{Key: "hotkeys.pad.menu", Group: GroupHotkeys, Section: "Controller", Label: "Menu", Kind: KindChoice, Default: "b", Choices: padButtons},

	{Key: "hotkeys.key.fastForward", Group: GroupHotkeys, Section: "Keyboard", Label: "Fast-forward", Kind: KindChoice, Default: "Space", Choices: hotkeyKeys},
	{Key: "hotkeys.key.rewind", Group: GroupHotkeys, Section: "Keyboard", Label: "Rewind", Kind: KindChoice, Default: "KeyR", Choices: hotkeyKeys},
	{Key: "hotkeys.key.quickSave", Group: GroupHotkeys, Section: "Keyboard", Label: "Quick save", Kind: KindChoice, Default: "F2", Choices: hotkeyKeys},
	{Key: "hotkeys.key.quickLoad", Group: GroupHotkeys, Section: "Keyboard", Label: "Quick load", Kind: KindChoice, Default: "F4", Choices: hotkeyKeys},
	{Key: "hotkeys.key.slow", Group: GroupHotkeys, Section: "Keyboard", Label: "Slow motion", Kind: KindChoice, Default: "KeyB", Choices: hotkeyKeys},
	{Key: "hotkeys.key.pause", Group: GroupHotkeys, Section: "Keyboard", Label: "Pause", Kind: KindChoice, Default: "KeyP", Choices: hotkeyKeys},
	{Key: "hotkeys.key.advance", Group: GroupHotkeys, Section: "Keyboard", Label: "Next frame", Kind: KindChoice, Default: "KeyN", Choices: hotkeyKeys},
	{Key: "hotkeys.key.screenshot", Group: GroupHotkeys, Section: "Keyboard", Label: "Screenshot", Kind: KindChoice, Default: "F8", Choices: hotkeyKeys},
	{Key: "hotkeys.key.mute", Group: GroupHotkeys, Section: "Keyboard", Label: "Mute", Kind: KindChoice, Default: "KeyM", Choices: hotkeyKeys},
	{Key: "hotkeys.key.osd", Group: GroupHotkeys, Section: "Keyboard", Label: "Show numbers", Kind: KindChoice, Default: "KeyO", Choices: hotkeyKeys},
	{Key: "hotkeys.touchBar", Group: GroupHotkeys, Section: "Touch", Label: "Shortcut buttons on screen", Help: "Fast-forward, rewind, quick save and quick load, beside the menu button.", Kind: KindToggle, Default: false},
}

// hotkeyKeys are the keys a shortcut can use: none the browser keeps for itself.
var hotkeyKeys = func() []Choice {
	out := choices("", "Not used", "Space", "Space", "Tab", "Tab", "Backspace", "Backspace", "Backquote", "`", "Minus", "-", "Equal", "=", "BracketLeft", "[", "BracketRight", "]", "Backslash", "\\", "Semicolon", ";", "Quote", "'", "Comma", ",", "Period", ".", "Slash", "/")
	for c := 'A'; c <= 'Z'; c++ {
		out = append(out, Choice{"Key" + string(c), string(c)})
	}
	for c := '0'; c <= '9'; c++ {
		out = append(out, Choice{"Digit" + string(c), string(c)})
	}
	for _, f := range []string{"F2", "F3", "F4", "F6", "F7", "F8", "F9", "F10"} {
		out = append(out, Choice{f, f})
	}
	return out
}()

// MaxStateSlot is the highest numbered save slot a person can use.
const MaxStateSlot = ipc.MaxStateSlot

// Preset is a named set of values a person can apply in one go.
type Preset struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Values      map[string]any    `json:"settings,omitempty"`
	CoreOptions map[string]string `json:"coreOptions,omitempty"`
}

// BuiltinPresets are starting points for the picture.
var BuiltinPresets = []Preset{
	{Name: "Clean", Description: "Sharp pixels and nothing else.", Values: map[string]any{"display.scaling": "sharp", "display.scanlines": 0.0, "display.curve": 0.0, "display.mask": 0.0, "display.glow": 0.0, "display.vignette": 0.0, "display.lcdGrid": 0.0, "display.blend": 0.0, "display.flicker": 0.0}},
	{Name: "Scanlines", Description: "The dark lines of a TV picture, on a flat screen.", Values: map[string]any{"display.scaling": "sharp", "display.scanlines": 0.45, "display.curve": 0.0, "display.mask": 0.0, "display.glow": 0.1, "display.vignette": 0.0, "display.lcdGrid": 0.0, "display.blend": 0.0}},
	{Name: "CRT TV", Description: "Scanlines, a curved glass front and a phosphor pattern.", Values: map[string]any{"display.scaling": "sharp", "display.scanlines": 0.5, "display.curve": 0.35, "display.mask": 0.35, "display.glow": 0.25, "display.vignette": 0.3, "display.lcdGrid": 0.0, "display.blend": 0.0}},
	{Name: "Arcade", Description: "Strong scanlines and glow on a flat tube.", Values: map[string]any{"display.scaling": "sharp", "display.scanlines": 0.7, "display.curve": 0.1, "display.mask": 0.5, "display.glow": 0.4, "display.vignette": 0.1, "display.lcdGrid": 0.0, "display.blend": 0.0}},
	{Name: "Handheld LCD", Description: "A pixel grid and the slow fade of an old handheld's screen.", Values: map[string]any{"display.scaling": "sharp", "display.scanlines": 0.0, "display.curve": 0.0, "display.mask": 0.0, "display.glow": 0.0, "display.vignette": 0.0, "display.lcdGrid": 0.45, "display.blend": 0.4, "display.integer": true}},
}

var settingByKey = func() map[string]*Setting {
	m := make(map[string]*Setting, len(Settings))
	for i := range Settings {
		m[Settings[i].Key] = &Settings[i]
	}
	return m
}()

// SettingFor finds a setting's description.
func SettingFor(key string) *Setting { return settingByKey[key] }

var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// CleanValue checks a value against its setting and returns it in its one proper form.
func CleanValue(s *Setting, v any) (any, error) {
	switch s.Kind {
	case KindToggle:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("must be true or false")
		}
		return b, nil
	case KindChoice:
		text, ok := v.(string)
		if !ok {
			if f, isNum := v.(float64); isNum && f == math.Trunc(f) {
				text = strconv.FormatInt(int64(f), 10) // "rotate": 90 is what's meant
			} else {
				return nil, fmt.Errorf("must be one of %s", choiceList(s.Choices))
			}
		}
		for _, c := range s.Choices {
			if c.Value == text {
				return text, nil
			}
		}
		return nil, fmt.Errorf("must be one of %s", choiceList(s.Choices))
	case KindSlider:
		f, ok := v.(float64)
		if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, fmt.Errorf("must be a number from %g to %g", s.Min, s.Max)
		}
		if f < s.Min || f > s.Max {
			return nil, fmt.Errorf("must be from %g to %g", s.Min, s.Max)
		}
		return f, nil
	case KindColor:
		text, ok := v.(string)
		if !ok || !colorRe.MatchString(text) {
			return nil, fmt.Errorf(`must be a colour like "#102030"`)
		}
		return text, nil
	case KindBezel:
		f, ok := v.(float64)
		if !ok || f < 0 || f > 1<<53 || f != math.Trunc(f) {
			return nil, fmt.Errorf("must be the number of an image you uploaded")
		}
		return f, nil
	}
	return nil, fmt.Errorf("can't be set")
}

func choiceList(list []Choice) string {
	s := ""
	for i, c := range list {
		if i == 8 {
			return s + ", …"
		}
		if i > 0 {
			s += ", "
		}
		s += strconv.Quote(c.Value)
	}
	return s
}

// Limits on what one layer may hold.
const (
	MaxCoreOptions    = 512
	MaxCustomControls = 64
	MaxPresets        = 32
	MaxControlChoices = 256
	MaxOptionKey      = ipc.MaxOptionKey
	MaxOptionValue    = ipc.MaxOptionValue
	MaxLabel          = 80
)

// Layer is the settings held at one level. Everything in it has been checked.
type Layer struct {
	Values      map[string]any    `json:"settings,omitempty"`
	CoreOptions map[string]string `json:"coreOptions,omitempty"`
	Controls    []CustomControl   `json:"controls,omitempty"`
	Presets     []Preset          `json:"presets,omitempty"`
}

// Empty says the layer holds nothing.
func (l *Layer) Empty() bool {
	return len(l.Values) == 0 && len(l.CoreOptions) == 0 && len(l.Controls) == 0 && len(l.Presets) == 0
}

// CustomControl is a control a person defined for a core option: the label they want
// and how it behaves. It sets one core option and can do nothing else.
type CustomControl struct {
	Label   string   `json:"label"`
	Help    string   `json:"help,omitempty"`
	Type    string   `json:"type"` // toggle | choice | slider
	Option  string   `json:"option"`
	On      string   `json:"on,omitempty"`  // toggle
	Off     string   `json:"off,omitempty"` // toggle
	Choices []Choice `json:"choices,omitempty"`
}

// NamedLayer is a layer with the level it came from.
type NamedLayer struct {
	Level string
	Layer *Layer
}

// Resolved is every setting's effective value and where it came from.
type Resolved struct {
	Values      map[string]any    `json:"values"`
	Sources     map[string]string `json:"sources"`
	CoreOptions map[string]string `json:"coreOptions"`
	CoreSources map[string]string `json:"coreSources"`
	Controls    []CustomControl   `json:"controls"`
	Presets     []Preset          `json:"presets"`
}

// Resolve works out the effective settings, with later layers winning. Values are
// checked again here: what's stored may be older than the settings it was checked
// against, and a stale value must never reach a game.
func Resolve(layers []NamedLayer) Resolved {
	r := Resolved{Values: make(map[string]any, len(Settings)), Sources: make(map[string]string, len(Settings)), CoreOptions: map[string]string{}, CoreSources: map[string]string{}, Controls: []CustomControl{}, Presets: []Preset{}}
	for i := range Settings {
		r.Values[Settings[i].Key] = Settings[i].Default
		r.Sources[Settings[i].Key] = LevelBuiltin
	}
	controlAt := map[string]int{}
	presetAt := map[string]int{}
	for _, nl := range layers {
		if nl.Layer == nil {
			continue
		}
		for k, v := range nl.Layer.Values {
			s := settingByKey[k]
			if s == nil {
				continue
			}
			if clean, err := CleanValue(s, v); err == nil {
				r.Values[k] = clean
				r.Sources[k] = nl.Level
			}
		}
		for k, v := range nl.Layer.CoreOptions {
			if goodOptionKey(k) && goodOptionValue(v) {
				r.CoreOptions[k] = v
				r.CoreSources[k] = nl.Level
			}
		}
		for _, c := range nl.Layer.Controls { // a more specific control for an option replaces the other
			if i, ok := controlAt[c.Option]; ok {
				r.Controls[i] = c
			} else {
				controlAt[c.Option] = len(r.Controls)
				r.Controls = append(r.Controls, c)
			}
		}
		for _, p := range nl.Layer.Presets {
			if i, ok := presetAt[p.Name]; ok {
				r.Presets[i] = p
			} else {
				presetAt[p.Name] = len(r.Presets)
				r.Presets = append(r.Presets, p)
			}
		}
	}
	return r
}

var optionKeyRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.:-]*$`)

func goodOptionKey(k string) bool {
	return len(k) <= MaxOptionKey && optionKeyRe.MatchString(k)
}

func goodOptionValue(v string) bool {
	if len(v) > MaxOptionValue {
		return false
	}
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Patch changes a layer: a null value removes a key. It returns what couldn't be used,
// and everything else is applied.
func (l *Layer) Patch(values map[string]any, core map[string]*string) []Issue {
	var ignored []Issue
	if len(values) > 0 && l.Values == nil {
		l.Values = map[string]any{}
	}
	for _, k := range sortedKeys(values) {
		v := values[k]
		s := settingByKey[k]
		switch {
		case s == nil:
			ignored = append(ignored, Issue{Path: k, Message: "there's no setting with this name"})
		case v == nil:
			delete(l.Values, k)
		default:
			clean, err := CleanValue(s, v)
			if err != nil {
				ignored = append(ignored, Issue{Path: k, Message: err.Error()})
				continue
			}
			l.Values[k] = clean
		}
	}
	if len(core) > 0 && l.CoreOptions == nil {
		l.CoreOptions = map[string]string{}
	}
	for _, k := range sortedKeys(core) {
		v := core[k]
		switch {
		case !goodOptionKey(k):
			ignored = append(ignored, Issue{Path: k, Message: "this can't be a core option's name"})
		case v == nil:
			delete(l.CoreOptions, k)
		case !goodOptionValue(*v):
			ignored = append(ignored, Issue{Path: k, Message: "this can't be a core option's value"})
		case len(l.CoreOptions) >= MaxCoreOptions && l.CoreOptions[k] == "":
			ignored = append(ignored, Issue{Path: k, Message: "too many core options"})
		default:
			l.CoreOptions[k] = *v
		}
	}
	return ignored
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// CheckOptions goes through core options against what a core really offers: an unknown
// option or an unlisted value is reported, not silently dropped. The runner would ignore
// them anyway. This is for telling people.
func CheckOptions(core map[string]string, offered []OptionInfo) []Issue {
	byKey := make(map[string]*OptionInfo, len(offered))
	for i := range offered {
		byKey[offered[i].Key] = &offered[i]
	}
	var out []Issue
	for _, k := range sortedKeys(core) {
		o := byKey[k]
		if o == nil {
			out = append(out, Issue{Path: "coreOptions." + k, Message: "this core has no such option, so it will be ignored"})
			continue
		}
		found := false
		for _, c := range o.Choices {
			found = found || c == core[k]
		}
		if !found {
			out = append(out, Issue{Path: "coreOptions." + k, Message: fmt.Sprintf("%q isn't one of this option's values, so it will be ignored", core[k])})
		}
	}
	return out
}

// Limits are what a server allows, whatever anyone's settings say. They exist so that
// one person's fast-forward or rewind can't starve a small server.
type Limits struct {
	MaxSpeed    float64 `json:"maxSpeed"`    // fast-forward, times normal speed
	RewindBytes int64   `json:"rewindBytes"` // memory rewind may hold in a game; 0 means it's off here
	RunAhead    bool    `json:"runAhead"`    // whether run-ahead may be used
}

func asFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

// ControlFor turns settings into what a runner is told, held to the server's limits.
func ControlFor(values map[string]any, lim Limits) ipc.Control {
	skip := 0
	switch v, _ := values["emulation.frameSkip"].(string); v {
	case "off", "":
	case "auto":
		skip = ipc.FrameSkipAuto
	default:
		skip, _ = strconv.Atoi(v)
	}
	ahead := 0
	if lim.RunAhead {
		ahead = int(asFloat(values["emulation.runAhead"]))
	}
	var rewind int64
	if on, _ := values["emulation.rewind"].(bool); on {
		rewind = lim.RewindBytes
	}
	secs := int(asFloat(values["emulation.rewindSeconds"]))
	return ipc.Control{FrameSkip: &skip, RunAhead: &ahead, RewindBytes: &rewind, RewindSeconds: &secs}
}

// OptionInfo is one option a core offers, as told to clients. It's what a runner said,
// so it's cut to size before anyone stores or shows it.
type OptionInfo struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Choices []string `json:"choices"`
	Default string   `json:"default"`
}
