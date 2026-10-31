package wittyplay

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Controllers as data. Every client reads these through the server's API and
// draws its own controls from them, so a console's layout is written once, here,
// and works the same in a browser and in a TV app. docs/GAMING-API.md has the
// formats.

// RetroPad buttons. libretro speaks one controller, so a console only needs its own
// names for these, and which of them it has. On the pad itself, bottom is B, right
// is A, left is Y and top is X.
const (
	RetroB = iota
	RetroY
	RetroSelect
	RetroStart
	RetroUp
	RetroDown
	RetroLeft
	RetroRight
	RetroA
	RetroX
	RetroL
	RetroR
	RetroL2
	RetroR2
	RetroL3
	RetroR3
	retroButtons
)

// Analog directions, numbered after the buttons so that a mapping can treat them
// alike: a stick can press a button, and a button can push a stick. Each is one half
// of one axis; up is negative, as libretro has it.
const (
	StickLeftL = iota + retroButtons // left stick, left
	StickRightL
	StickUpL
	StickDownL
	StickLeftR // right stick, left
	StickRightR
	StickUpR
	StickDownR
	targetCount
)

// PadButton is one button of a console's controller.
type PadButton struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// PadLayout says what a console's controller has.
type PadLayout struct {
	Face     []PadButton `json:"face"` // main buttons, left to right
	Shoulder []PadButton `json:"shoulder,omitempty"`
	Meta     []PadButton `json:"meta"` // Select, Start and their relatives
	Sticks   bool        `json:"sticks,omitempty"`
	// Extra buttons a gamepad can reach but an on-screen controller leaves out, such
	// as stick clicks.
	Extra []PadButton `json:"extra,omitempty"`
	// Right is what the right stick's four directions mean, when it has them.
	Right []PadButton `json:"right,omitempty"`
	// Art is the shape of picture to draw: one, two, four, six, handle or dual.
	Art string `json:"art"`
	// StickFirst: the stick, not the D-pad, is how this console's games steer, so an
	// on-screen D-pad and the arrow keys drive the stick.
	StickFirst bool `json:"stickFirst,omitempty"`
	// PreferDevices names the libretro device type to plug in, by words in its name,
	// best first. A core that offers none of them gets its plain pad.
	PreferDevices []string `json:"-"`
}

var (
	selectStart = []PadButton{{RetroSelect, "Select"}, {RetroStart, "Start"}}
	startOnly   = []PadButton{{RetroStart, "Start"}}
	diamond     = []PadButton{{RetroY, "Y"}, {RetroX, "X"}, {RetroB, "B"}, {RetroA, "A"}}
)

var (
	stickClicks = []PadButton{{RetroL3, "L3"}, {RetroR3, "R3"}}
	rightStick  = []PadButton{{StickLeftR, "Right stick left"}, {StickRightR, "Right stick right"}, {StickUpR, "Right stick up"}, {StickDownR, "Right stick down"}}
)

var pads = map[string]PadLayout{
	"nes":  {Art: "two", Face: []PadButton{{RetroB, "B"}, {RetroA, "A"}}, Meta: selectStart},
	"snes": {Art: "four", Face: diamond, Shoulder: []PadButton{{RetroL, "L"}, {RetroR, "R"}}, Meta: selectStart},
	"gb":   {Art: "two", Face: []PadButton{{RetroB, "B"}, {RetroA, "A"}}, Meta: selectStart},
	"gba":  {Art: "two", Face: []PadButton{{RetroB, "B"}, {RetroA, "A"}}, Shoulder: []PadButton{{RetroL, "L"}, {RetroR, "R"}}, Meta: selectStart},
	"md": {Art: "six", Face: []PadButton{{RetroY, "A"}, {RetroB, "B"}, {RetroA, "C"}}, Shoulder: []PadButton{{RetroL, "X"}, {RetroX, "Y"}, {RetroR, "Z"}},
		Meta: []PadButton{{RetroSelect, "Mode"}, {RetroStart, "Start"}}},
	"sms": {Art: "two", Face: []PadButton{{RetroB, "1"}, {RetroA, "2"}}, Meta: []PadButton{{RetroStart, "Pause"}}},
	"gg":  {Art: "two", Face: []PadButton{{RetroB, "1"}, {RetroA, "2"}}, Meta: startOnly},
	// DualShock when the core offers one: sticks, and rumble with it.
	"ps1": {Art: "dual", Face: []PadButton{{RetroY, "□"}, {RetroX, "△"}, {RetroB, "✕"}, {RetroA, "○"}},
		Shoulder: []PadButton{{RetroL, "L1"}, {RetroR, "R1"}, {RetroL2, "L2"}, {RetroR2, "R2"}}, Meta: selectStart, Sticks: true, Extra: stickClicks, Right: rightStick,
		PreferDevices: []string{"dualshock", "analog"}},
	"pce":   {Art: "two", Face: []PadButton{{RetroB, "II"}, {RetroA, "I"}}, Meta: []PadButton{{RetroSelect, "Select"}, {RetroStart, "Run"}}},
	"a2600": {Art: "one", Face: []PadButton{{RetroB, "Fire"}}, Meta: []PadButton{{RetroSelect, "Select"}, {RetroStart, "Reset"}}},
	"a7800": {Art: "two", Face: []PadButton{{RetroB, "A"}, {RetroA, "B"}}, Meta: []PadButton{{RetroSelect, "Select"}, {RetroStart, "Pause"}}},
	"lynx":  {Art: "two", Face: []PadButton{{RetroB, "B"}, {RetroA, "A"}}, Shoulder: []PadButton{{RetroL, "Opt 1"}, {RetroR, "Opt 2"}}, Meta: []PadButton{{RetroStart, "Pause"}}},
	"ws":    {Art: "two", Face: []PadButton{{RetroB, "B"}, {RetroA, "A"}}, Meta: startOnly},
	"ngp":   {Art: "two", Face: []PadButton{{RetroB, "A"}, {RetroA, "B"}}, Meta: []PadButton{{RetroStart, "Option"}}},
	"vb":    {Art: "two", Face: []PadButton{{RetroB, "B"}, {RetroA, "A"}}, Shoulder: []PadButton{{RetroL, "L"}, {RetroR, "R"}}, Meta: selectStart},
	// Numbered as the cores number them: RetroPad B is the console's A, Y its B, L2 its
	// Z, and the right stick its C buttons.
	"n64": {Art: "handle", Face: []PadButton{{RetroY, "B"}, {RetroB, "A"}}, Shoulder: []PadButton{{RetroL, "L"}, {RetroR, "R"}, {RetroL2, "Z"}}, Meta: startOnly, Sticks: true, StickFirst: true,
		Right: []PadButton{{StickLeftR, "C left"}, {StickRightR, "C right"}, {StickUpR, "C up"}, {StickDownR, "C down"}}},
	"nds": {Art: "four", Face: diamond, Shoulder: []PadButton{{RetroL, "L"}, {RetroR, "R"}}, Meta: selectStart},
	"psp": {Art: "dual", Face: []PadButton{{RetroY, "□"}, {RetroX, "△"}, {RetroB, "✕"}, {RetroA, "○"}}, Shoulder: []PadButton{{RetroL, "L"}, {RetroR, "R"}}, Meta: selectStart, Sticks: true},
	// A is RetroPad B and X is Y, by position, and the triggers are analog on L2 and R2.
	"dc":        {Art: "dual", Face: []PadButton{{RetroY, "X"}, {RetroX, "Y"}, {RetroB, "A"}, {RetroA, "B"}}, Shoulder: []PadButton{{RetroL2, "L"}, {RetroR2, "R"}}, Meta: startOnly, Sticks: true},
	TestConsole: {Art: "four", Face: diamond, Shoulder: []PadButton{{RetroL, "L"}, {RetroR, "R"}}, Meta: selectStart},
}

var fullPad = PadLayout{Art: "dual", Face: diamond, Shoulder: []PadButton{{RetroL, "L"}, {RetroR, "R"}, {RetroL2, "L2"}, {RetroR2, "R2"}}, Meta: selectStart, Sticks: true, Extra: stickClicks, Right: rightStick}

// Pad is a console's controller; anything not listed gets a whole RetroPad.
func Pad(consoleID string) PadLayout {
	if p, ok := pads[consoleID]; ok {
		return p
	}
	return fullPad
}

// Control is something on a controller. Key is what layouts are stored under.
type Control struct {
	Key   string `json:"key"`
	ID    int    `json:"id"` // the RetroPad button; -1 for the D-pad as a whole, -2 for the stick
	Label string `json:"label"`
	Kind  string `json:"kind"` // dpad | stick | face | shoulder | meta
	// Stick: on screen, this D-pad is an analog stick, and drives the left stick.
	Stick bool `json:"stick,omitempty"`
}

// Keys in a touch layout: the D-pad, and an analog stick of its own.
const (
	DPadKey  = "dpad"
	StickKey = "lstick"
)

// Controls lists a console's on-screen controls.
func Controls(consoleID string) []Control {
	p := Pad(consoleID)
	out := []Control{{Key: DPadKey, ID: -1, Label: "D-pad", Kind: "dpad", Stick: p.StickFirst}}
	if p.Sticks && !p.StickFirst {
		out = append(out, Control{Key: StickKey, ID: -2, Label: "Stick", Kind: "stick"})
	}
	add := func(list []PadButton, kind string) {
		for _, b := range list {
			out = append(out, Control{Key: fmt.Sprintf("b%d", b.ID), ID: b.ID, Label: b.Label, Kind: kind})
		}
	}
	add(p.Face, "face")
	add(p.Shoulder, "shoulder")
	add(p.Meta, "meta")
	if p.StickFirst { // C buttons: without them a phone can't play these games
		add(p.Right, "face")
	}
	return out
}

// Target is one thing a controller can be mapped to on a console: a button, or one
// direction of a stick.
type Target struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
	Group string `json:"group"` // dpad | face | shoulder | meta | stick
}

// Targets lists all that a console's controller can do, in the order a mapping screen
// should show them.
func Targets(consoleID string) []Target {
	p := Pad(consoleID)
	out := []Target{{RetroUp, "D-pad up", "dpad"}, {RetroDown, "D-pad down", "dpad"}, {RetroLeft, "D-pad left", "dpad"}, {RetroRight, "D-pad right", "dpad"}}
	add := func(list []PadButton, group string) {
		for _, b := range list {
			out = append(out, Target{b.ID, b.Label, group})
		}
	}
	add(p.Face, "face")
	add(p.Shoulder, "shoulder")
	add(p.Meta, "meta")
	add(p.Extra, "meta")
	if p.Sticks {
		out = append(out, Target{StickUpL, "Stick up", "stick"}, Target{StickDownL, "Stick down", "stick"}, Target{StickLeftL, "Stick left", "stick"}, Target{StickRightL, "Stick right", "stick"})
	}
	add(p.Right, "stick")
	return out
}

// Binding is what presses a RetroPad button on a gamepad: a button, one direction of
// an axis, or one direction of a hat (a D-pad reported as a single axis). Numbers are
// those of the W3C standard gamepad, which native clients translate to and from their
// platform's names (docs/GAMING-API.md).
type Binding struct {
	Button *int `json:"b,omitempty"`
	Axis   *int `json:"a,omitempty"`
	Hat    *int `json:"h,omitempty"`
	// Dir: for an axis, 1 or -1. For a hat, 1 up, 2 right, 3 down, 4 left.
	Dir int `json:"d,omitempty"`
	// Rest: where a trigger reported as an axis sits when let go, -1 or 1. Its travel
	// then counts from there rather than from the middle.
	Rest int `json:"r,omitempty"`
}

// Limits on what a browser reports, well above any real controller.
const (
	maxPadButton = 64
	maxPadAxis   = 32
)

func (b Binding) check() error {
	switch {
	case b.Button != nil && b.Axis == nil && b.Hat == nil && *b.Button >= 0 && *b.Button < maxPadButton && b.Dir == 0 && b.Rest == 0:
	case b.Axis != nil && b.Button == nil && b.Hat == nil && *b.Axis >= 0 && *b.Axis < maxPadAxis && (b.Dir == 1 || b.Dir == -1) && b.Rest >= -1 && b.Rest <= 1:
	case b.Hat != nil && b.Button == nil && b.Axis == nil && *b.Hat >= 0 && *b.Hat < maxPadAxis && b.Dir >= 1 && b.Dir <= 4 && b.Rest == 0:
	default:
		return errors.New("a gamepad binding must be one button, or one axis or hat and its direction")
	}
	return nil
}

func button(n int) Binding    { return Binding{Button: &n} }
func axis(n, dir int) Binding { return Binding{Axis: &n, Dir: dir} }

// DefaultGamepad maps by position, which is how libretro expects pads to be laid out.
func DefaultGamepad() map[int]Binding {
	return map[int]Binding{
		RetroB: button(0), RetroA: button(1), RetroY: button(2), RetroX: button(3), RetroL: button(4), RetroR: button(5),
		RetroL2: button(6), RetroR2: button(7), RetroSelect: button(8), RetroStart: button(9), RetroL3: button(10), RetroR3: button(11),
		RetroUp: button(12), RetroDown: button(13), RetroLeft: button(14), RetroRight: button(15),
	}
}

// DefaultGamepadFor is DefaultGamepad for one console: only what its controller has,
// and its sticks where it has them.
func DefaultGamepadFor(consoleID string) map[int]Binding {
	all := DefaultGamepad()
	for id, b := range map[int]Binding{
		StickLeftL: axis(0, -1), StickRightL: axis(0, 1), StickUpL: axis(1, -1), StickDownL: axis(1, 1),
		StickLeftR: axis(2, -1), StickRightR: axis(2, 1), StickUpR: axis(3, -1), StickDownR: axis(3, 1),
	} {
		all[id] = b
	}
	out := map[int]Binding{}
	for _, t := range Targets(consoleID) {
		if b, ok := all[t.ID]; ok {
			out[t.ID] = b
		}
	}
	return out
}

// DefaultKeyboardFor is DefaultKeyboard for one console. A stick gets I, J, K and L,
// and where the stick is how games steer it gets the arrows instead.
func DefaultKeyboardFor(consoleID string) map[int]string {
	p := Pad(consoleID)
	all := DefaultKeyboard()
	if p.Sticks {
		all[StickUpL], all[StickDownL], all[StickLeftL], all[StickRightL] = "KeyI", "KeyK", "KeyJ", "KeyL"
	}
	if p.StickFirst {
		all[StickUpL], all[StickDownL], all[StickLeftL], all[StickRightL] = "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"
		all[StickUpR], all[StickDownR], all[StickLeftR], all[StickRightR] = "KeyI", "KeyK", "KeyJ", "KeyL"
		all[RetroUp], all[RetroDown], all[RetroLeft], all[RetroRight] = "KeyT", "KeyG", "KeyF", "KeyH"
	}
	out := map[int]string{}
	for _, t := range Targets(consoleID) {
		if k, ok := all[t.ID]; ok {
			out[t.ID] = k
		}
	}
	return out
}

// DefaultKeyboard uses KeyboardEvent.code names.
func DefaultKeyboard() map[int]string {
	return map[int]string{
		RetroUp: "ArrowUp", RetroDown: "ArrowDown", RetroLeft: "ArrowLeft", RetroRight: "ArrowRight",
		RetroB: "KeyZ", RetroA: "KeyX", RetroY: "KeyA", RetroX: "KeyS", RetroL: "KeyQ", RetroR: "KeyW", RetroL2: "KeyE", RetroR2: "KeyR",
		RetroStart: "Enter", RetroSelect: "ShiftRight",
	}
}

// DefaultRemote is for a TV remote playing a game, using KeyboardEvent.key names. A
// remote has few buttons, so Back opens the game's menu, which can also press Start
// and Select, and a game needing more wants a gamepad.
func DefaultRemote() map[int]string {
	return map[int]string{
		RetroUp: "ArrowUp", RetroDown: "ArrowDown", RetroLeft: "ArrowLeft", RetroRight: "ArrowRight",
		RetroA: "Enter", RetroB: "MediaPlayPause", RetroSelect: "MediaRewind", RetroStart: "MediaFastForward",
	}
}

// Placement is where a touch control sits. Positions are fractions of the play area;
// size is a fraction of its shorter side, so a button stays thumb-sized on any screen.
type Placement struct {
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Size    float64 `json:"size"`
	Opacity float64 `json:"opacity"`
	Hidden  bool    `json:"hidden,omitempty"`
}

// TouchLayout is a whole on-screen controller, by Control.Key.
type TouchLayout map[string]Placement

// Orientations a touch layout can be for.
const (
	Landscape = "landscape"
	Portrait  = "portrait"
)

const defaultOpacity = 0.55

// DefaultTouch is a console's on-screen controller as it starts out.
func DefaultTouch(consoleID, orientation string) TouchLayout {
	pad := Pad(consoleID)
	land := orientation != Portrait
	pick := func(l, p float64) float64 {
		if land {
			return l
		}
		return p
	}
	out := TouchLayout{}
	fy := pick(0.62, 0.72) // the height of the D-pad and main buttons
	out[DPadKey] = Placement{X: pick(0.13, 0.24), Y: fy, Size: pick(0.36, 0.3), Opacity: defaultOpacity}
	if pad.Sticks && !pad.StickFirst {
		// Between the D-pad and the buttons. PlayStation games nearly all steer with the
		// D-pad, so it starts hidden there, and the editor can show it.
		out[StickKey] = Placement{X: pick(0.3, 0.5), Y: pick(0.84, 0.585), Size: pick(0.28, 0.2), Opacity: defaultOpacity, Hidden: consoleID == "ps1"}
	}
	cx := pick(0.87, 0.76)
	s := pick(0.15, 0.125)
	dx, dy := pick(0.085, 0.15), pick(0.17, 0.085)
	for i, b := range pad.Face {
		x, y := cx, fy
		switch {
		case len(pad.Face) == 2:
			side := -1.0
			if i == 1 {
				side = 1
			}
			x, y = cx+side*dx*0.75, fy-side*dy*0.4
		case len(pad.Face) == 3:
			x = cx + float64(i-1)*dx
			y = fy + []float64{dy * 0.5, dy * 0.2, -dy * 0.1}[i]
		case len(pad.Face) >= 4 && i < 4: // a diamond, listed as left, top, bottom, right
			off := [4][2]float64{{-dx, 0}, {0, -dy}, {0, dy}, {dx, 0}}[i]
			x, y = cx+off[0], fy+off[1]
		}
		out[fmt.Sprintf("b%d", b.ID)] = Placement{X: x, Y: y, Size: s, Opacity: defaultOpacity}
	}
	top := pick(0.14, 0.56)
	for i, b := range pad.Shoulder {
		var x, y float64
		if len(pad.Shoulder) == 3 { // three in a row above the right-hand buttons
			x, y = cx+float64(i-1)*dx*0.95, top+pick(0.1, 0)
		} else {
			row := float64(i / 2)
			if i%2 == 0 {
				x = 0.1 + row*pick(0.11, 0.18)
			} else {
				x = 0.9 - row*pick(0.11, 0.18)
			}
			y = top
		}
		out[fmt.Sprintf("b%d", b.ID)] = Placement{X: x, Y: y, Size: s * 0.8, Opacity: defaultOpacity}
	}
	if pad.StickFirst { // the right stick's directions as a small diamond of their own
		rx, ry := pick(0.72, 0.5), pick(0.32, 0.62)
		for i, b := range pad.Right {
			off := [4][2]float64{{-dx * 0.6, 0}, {dx * 0.6, 0}, {0, -dy * 0.6}, {0, dy * 0.6}}[i%4]
			out[fmt.Sprintf("b%d", b.ID)] = Placement{X: rx + off[0], Y: ry + off[1], Size: s * 0.7, Opacity: defaultOpacity}
		}
	}
	for i, b := range pad.Meta {
		offset := 0.0
		if len(pad.Meta) > 1 {
			offset = -1
			if i > 0 {
				offset = 1
			}
		}
		out[fmt.Sprintf("b%d", b.ID)] = Placement{X: 0.5 + offset*pick(0.065, 0.11), Y: pick(0.91, 0.93), Size: s * 0.72, Opacity: defaultOpacity}
	}
	return out
}

// Control kinds a person can save.
const (
	KindGamepad  = "gamepad"
	KindKeyboard = "keyboard"
	KindTouch    = "touch"
)

// MaxControlSize bounds one saved layout; the largest real one is under 2 KB.
const MaxControlSize = 16 << 10

func checkKeyName(key string) error {
	if key == "" || len(key) > 32 {
		return errors.New("a key name must be between 1 and 32 characters")
	}
	for _, r := range key {
		if r < 0x20 || r > 0x7e {
			return errors.New("a key name must be plain text")
		}
	}
	return nil
}

// CleanControl checks a layout someone wants to save, and returns it in the one form
// the server stores. It comes from a client, so nothing about it is taken on trust.
func CleanControl(consoleID, kind string, data json.RawMessage) (json.RawMessage, error) {
	if len(data) > MaxControlSize {
		return nil, errors.New("this layout is too large")
	}
	switch kind {
	case KindGamepad:
		var m map[int]Binding
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, errors.New("this gamepad layout can't be read")
		}
		for id, b := range m {
			if id < 0 || id >= targetCount {
				return nil, fmt.Errorf("%d is not a RetroPad button", id)
			}
			if err := b.check(); err != nil {
				return nil, err
			}
		}
		return json.Marshal(m)
	case KindKeyboard:
		var m map[int]string
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, errors.New("this keyboard layout can't be read")
		}
		for id, key := range m {
			if id < 0 || id >= targetCount {
				return nil, fmt.Errorf("%d is not a RetroPad button", id)
			}
			if err := checkKeyName(key); err != nil {
				return nil, err
			}
		}
		return json.Marshal(m)
	case KindTouch:
		var l TouchLayout
		if err := json.Unmarshal(data, &l); err != nil {
			return nil, errors.New("this touch layout can't be read")
		}
		known := map[string]bool{}
		for _, c := range Controls(consoleID) {
			known[c.Key] = true
		}
		for k, p := range l {
			if !known[k] {
				return nil, fmt.Errorf("this console has no control %q", k)
			}
			if p.X < 0 || p.X > 1 || p.Y < 0 || p.Y > 1 || p.Size < 0.05 || p.Size > 0.6 || p.Opacity < 0.1 || p.Opacity > 1 {
				return nil, fmt.Errorf("%s is out of range", k)
			}
		}
		return json.Marshal(l)
	}
	return nil, errors.New("unknown kind of layout")
}
