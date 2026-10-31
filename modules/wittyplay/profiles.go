package wittyplay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A profile is one person's named mapping for a controller: what each of a console's
// buttons is pressed by, and how sticks and triggers behave. Profiles are written by
// clients and by hand in files people swap, so nothing here trusts them. Which one
// applies is decided in Resolve, and docs/GAMING-API.md explains the rules to clients.

// Kinds of profile.
const (
	ProfileGamepad  = "gamepad"
	ProfileKeyboard = "keyboard"
)

// ControllerKeyboard is the Controller of every keyboard profile.
const ControllerKeyboard = "keyboard"

// Limits on what a person can store and bring in.
const (
	MaxProfiles      = 200
	MaxProfileSize   = 16 << 10
	MaxImportSize    = 256 << 10
	MaxImportEntries = 100
	maxProfileName   = 60
	maxModelName     = 120
)

// ProfileFormat names the files profiles are exported to.
const ProfileFormat = "wittyplay-controller-profiles"

// ProfileOptions is how a controller behaves, apart from which button is which. Zero
// values mean the console's default.
type ProfileOptions struct {
	Deadzone  float64 `json:"deadzone,omitempty"`  // up to 0.6: how far a stick moves before it counts
	Threshold float64 `json:"threshold,omitempty"` // 0.1 to 0.9: how far an axis moves to press a button
	StickDpad *bool   `json:"stickDpad,omitempty"` // the left stick also steers the D-pad
	Turbo     []int   `json:"turbo,omitempty"`     // buttons that repeat while held
	TurboHz   int     `json:"turboHz,omitempty"`   // 2 to 30 presses a second
	NoRumble  bool    `json:"noRumble,omitempty"`
	// Calibration is, per axis, where it reads at its lowest, at rest and at its highest.
	Calibration map[int][3]float64 `json:"calibration,omitempty"`
}

// Profile is one mapping. Its scope is Controller, Console and Game: each may be left
// empty to mean any.
type Profile struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Controller string `json:"controller"` // "054c:0ce6", "id:<model name>", "keyboard", or "" for any
	// ControllerName is for showing, as the browser described the model.
	ControllerName string `json:"controllerName,omitempty"`
	Console        string `json:"console,omitempty"`
	Game           int64  `json:"game,omitempty"`
	GameTitle      string `json:"gameTitle,omitempty"` // filled in by the server
	// Raw: the numbers in Map are what this model reports in this browser, because it
	// isn't a pad whose buttons we can put in standard order. Such a profile applies only
	// to a pad in the same state. Everything else uses W3C standard numbers.
	Raw bool `json:"raw,omitempty"`
	// Active: of the profiles with the same scope, the one in use.
	Active    bool             `json:"active"`
	Map       map[int]*Binding `json:"map,omitempty"`  // what it changes; null means nothing presses it
	Keys      map[int]string   `json:"keys,omitempty"` // keyboard: KeyboardEvent.code names
	Options   ProfileOptions   `json:"options"`
	UpdatedAt int64            `json:"updatedAt"`
}

var modelRe = regexp.MustCompile(`^[0-9a-f]{4}:[0-9a-f]{4}$`)

// tidyText takes out what shouldn't be in a name: control characters, and the invisible
// format characters that can make text read differently from what it says.
func tidyText(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(s, "")))
}

// CleanProfile checks a profile from outside and returns the one form the server keeps:
// fields nobody knows are gone. consoleOK says which consoles exist for this account.
func CleanProfile(p Profile, consoleOK func(string) bool) (Profile, error) {
	name := tidyText(p.Name)
	if name == "" {
		return p, errors.New("a profile needs a name")
	}
	if utf8.RuneCountInString(name) > maxProfileName {
		return p, fmt.Errorf("a profile's name can be up to %d characters", maxProfileName)
	}
	out := Profile{ID: p.ID, Name: name, Kind: p.Kind, Controller: p.Controller, Console: p.Console, Game: p.Game, Raw: p.Raw, Active: p.Active}
	switch p.Kind {
	case ProfileGamepad:
		if len(p.Keys) > 0 {
			return p, errors.New("a gamepad profile can't have keys")
		}
		switch {
		case p.Controller == "":
			if p.Raw {
				return p, errors.New("a profile of a pad's own numbers needs that pad's model")
			}
		case modelRe.MatchString(p.Controller):
		case strings.HasPrefix(p.Controller, "id:"):
			if err := checkModelName(p.Controller[3:]); err != nil {
				return p, err
			}
		default:
			return p, errors.New("this controller model can't be read")
		}
		if len(p.Map) > targetCount {
			return p, errors.New("this profile has too many mappings")
		}
		out.Map = map[int]*Binding{}
		for id, b := range p.Map {
			if id < 0 || id >= targetCount {
				return p, fmt.Errorf("%d is not something a controller can press", id)
			}
			if b == nil {
				out.Map[id] = nil
				continue
			}
			if err := b.check(); err != nil {
				return p, err
			}
			out.Map[id] = &Binding{Button: b.Button, Axis: b.Axis, Hat: b.Hat, Dir: b.Dir, Rest: b.Rest}
		}
	case ProfileKeyboard:
		if len(p.Map) > 0 {
			return p, errors.New("a keyboard profile can't have gamepad buttons")
		}
		if p.Raw || (p.Controller != "" && p.Controller != ControllerKeyboard) {
			return p, errors.New("a keyboard profile is for keyboards")
		}
		out.Controller = ControllerKeyboard
		if len(p.Keys) > targetCount {
			return p, errors.New("this profile has too many mappings")
		}
		out.Keys = map[int]string{}
		for id, k := range p.Keys {
			if id < 0 || id >= targetCount {
				return p, fmt.Errorf("%d is not something a controller can press", id)
			}
			if err := checkKeyName(k); err != nil {
				return p, err
			}
			out.Keys[id] = k
		}
	default:
		return p, errors.New("a profile is for a gamepad or a keyboard")
	}
	if p.ControllerName != "" {
		out.ControllerName = truncateRunes(tidyText(p.ControllerName), maxModelName)
	}
	if p.Console != "" && !consoleOK(p.Console) {
		return p, fmt.Errorf("there is no console called %q", truncateRunes(p.Console, 20))
	}
	if p.Game < 0 {
		return p, errors.New("this game can't be read")
	}
	if p.Game != 0 && p.Console == "" {
		return p, errors.New("a profile for one game has to name its console")
	}
	opt, err := cleanOptions(p.Options)
	if err != nil {
		return p, err
	}
	out.Options = opt
	if b, err := json.Marshal(out); err != nil || len(b) > MaxProfileSize {
		return p, errors.New("this profile is too large")
	}
	return out, nil
}

func checkModelName(s string) error {
	if s == "" || utf8.RuneCountInString(s) > maxModelName || !utf8.ValidString(s) {
		return errors.New("this controller model can't be read")
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return errors.New("this controller model can't be read")
		}
	}
	return nil
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.ToValidUTF8(s, ""))
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

func cleanOptions(o ProfileOptions) (ProfileOptions, error) {
	bad := func(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }
	if bad(o.Deadzone) || o.Deadzone < 0 || o.Deadzone > 0.6 {
		return o, errors.New("a stick's dead zone must be between 0 and 0.6")
	}
	if bad(o.Threshold) || (o.Threshold != 0 && (o.Threshold < 0.1 || o.Threshold > 0.9)) {
		return o, errors.New("a press point must be between 0.1 and 0.9")
	}
	if o.TurboHz != 0 && (o.TurboHz < 2 || o.TurboHz > 30) {
		return o, errors.New("turbo must be between 2 and 30 presses a second")
	}
	out := ProfileOptions{Deadzone: o.Deadzone, Threshold: o.Threshold, StickDpad: o.StickDpad, TurboHz: o.TurboHz, NoRumble: o.NoRumble}
	seen := map[int]bool{}
	for _, id := range o.Turbo {
		if id < 0 || id >= retroButtons {
			return o, errors.New("turbo works on buttons")
		}
		if !seen[id] {
			seen[id] = true
			out.Turbo = append(out.Turbo, id)
		}
	}
	if len(o.Calibration) > maxPadAxis {
		return o, errors.New("this profile calibrates too many axes")
	}
	for axis, c := range o.Calibration {
		if axis < 0 || axis >= maxPadAxis {
			return o, errors.New("this profile calibrates an axis that can't exist")
		}
		for _, v := range c {
			if bad(v) || v < -4 || v > 4 { // a hat at rest reads a little above 3
				return o, errors.New("this profile's calibration is out of range")
			}
		}
		if !(c[0] < c[1] && c[1] < c[2]) {
			return o, errors.New("this profile's calibration is out of order")
		}
		if out.Calibration == nil {
			out.Calibration = map[int][3]float64{}
		}
		out.Calibration[axis] = c
	}
	return out, nil
}

// Resolve picks the profile for a controller on a console and game. The most specific
// wins, and a game outranks a console, which outranks a controller:
//
//  1. this game and this controller
//  2. this game, any controller
//  3. this console and this controller
//  4. this console, any controller
//  5. this controller, on anything
//  6. any controller, on anything
//
// With none of these the console's defaults apply. raw says the pad is one whose
// numbers can't be put in standard order: only a profile of its own numbers can apply.
func Resolve(list []Profile, kind, controller, console string, game int64, raw bool) *Profile {
	var best *Profile
	bestRank := 0
	for i := range list {
		p := &list[i]
		if !p.Active || p.Kind != kind {
			continue
		}
		if kind == ProfileGamepad && p.Raw != raw {
			continue
		}
		if p.Controller != "" && p.Controller != controller {
			continue
		}
		if p.Console != "" && p.Console != console {
			continue
		}
		if p.Game != 0 && p.Game != game {
			continue
		}
		rank := 1
		named := p.Controller != ""
		switch {
		case p.Game != 0 && named:
			rank = 6
		case p.Game != 0:
			rank = 5
		case p.Console != "" && named:
			rank = 4
		case p.Console != "":
			rank = 3
		case named:
			rank = 2
		}
		if rank > bestRank || (rank == bestRank && p.UpdatedAt > best.UpdatedAt) {
			best, bestRank = p, rank
		}
	}
	return best
}

// EffectiveMap is what each of a console's targets is pressed by under a profile: the
// console's default, changed by what the profile says. p may be nil. A profile of a
// pad's own numbers starts from nothing, as the defaults use standard numbers.
func EffectiveMap(consoleID string, p *Profile) map[int]Binding {
	out := map[int]Binding{}
	if p == nil || !p.Raw {
		out = DefaultGamepadFor(consoleID)
	}
	if p == nil {
		return out
	}
	known := map[int]bool{}
	for _, t := range Targets(consoleID) {
		known[t.ID] = true
	}
	for id, b := range p.Map {
		switch {
		case b == nil:
			delete(out, id)
		case known[id]:
			out[id] = *b
		}
	}
	return out
}

// EffectiveKeys is EffectiveMap for a keyboard. A key does one thing, so a key the
// profile uses leaves the default it would otherwise also press.
func EffectiveKeys(consoleID string, p *Profile) map[int]string {
	out := DefaultKeyboardFor(consoleID)
	if p == nil {
		return out
	}
	known := map[int]bool{}
	for _, t := range Targets(consoleID) {
		known[t.ID] = true
	}
	for id, k := range p.Keys {
		if !known[id] {
			continue
		}
		for other, v := range out {
			if v == k {
				delete(out, other)
			}
		}
		out[id] = k
	}
	return out
}

// EffectiveOptions is how a controller behaves under a profile. p may be nil.
func EffectiveOptions(consoleID string, p *Profile) ProfileOptions {
	out := DefaultOptions(consoleID)
	if p == nil {
		return out
	}
	o := p.Options
	if o.Deadzone != 0 {
		out.Deadzone = o.Deadzone
	}
	if o.Threshold != 0 {
		out.Threshold = o.Threshold
	}
	if o.StickDpad != nil {
		out.StickDpad = o.StickDpad
	}
	if o.TurboHz != 0 {
		out.TurboHz = o.TurboHz
	}
	out.Turbo, out.NoRumble, out.Calibration = o.Turbo, o.NoRumble, o.Calibration
	return out
}

// ShortText is a piece of an untrusted name, safe to say back in a message.
func ShortText(s string) string { return truncateRunes(tidyText(s), maxProfileName) }

// DefaultOptions is how a console's controller behaves until someone changes it. A
// console with no stick lets the left stick steer its D-pad.
func DefaultOptions(consoleID string) ProfileOptions {
	dpad := !Pad(consoleID).Sticks
	return ProfileOptions{Deadzone: 0.15, Threshold: 0.5, StickDpad: &dpad, TurboHz: 12}
}

// ProfileFile is an exported file.
type ProfileFile struct {
	Format   string    `json:"format"`
	Version  int       `json:"version"`
	Profiles []Profile `json:"profiles"`
}

// ParseProfileFile reads a file someone wants to import, entirely untrusted: limited in
// size and count, and with every profile still to go through CleanProfile.
func ParseProfileFile(b []byte) ([]Profile, error) {
	if len(b) > MaxImportSize {
		return nil, errors.New("this file is too large")
	}
	var f ProfileFile
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&f); err != nil || f.Format != ProfileFormat {
		return nil, errors.New("this isn't a controller profile file")
	}
	if f.Version != 1 {
		return nil, errors.New("this file was made by a newer version; update this server first")
	}
	if len(f.Profiles) == 0 {
		return nil, errors.New("this file has no profiles in it")
	}
	if len(f.Profiles) > MaxImportEntries {
		return nil, fmt.Errorf("this file has more than %d profiles", MaxImportEntries)
	}
	return f.Profiles, nil
}
