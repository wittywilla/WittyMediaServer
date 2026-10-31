package wittyplay

import (
	"regexp"
	"strconv"
	"strings"
)

// What a core needs beyond its file. Licences are in the console table; this is
// how a core is run. Every option here is only applied when the core offers that
// exact choice, so a core that changes its options falls back to its own default
// rather than failing.

// CoreTraits describes one core.
type CoreTraits struct {
	// GPU cores draw with a graphics card. They count against the GPU limit and
	// their runner gets what a graphics driver needs. docs/GAMING.md has the list.
	GPU bool
	// Assets is the name of the core's support files on the build server, for
	// cores that won't run without them. They go in the system folder.
	Assets string
	// RequiredBIOS is set when the core can't start without a BIOS file.
	RequiredBIOS *BIOSRule
	// Options are always passed. Each one makes the core use what this runner has.
	Options map[string]string
	// Resolution gives the options for an internal resolution, 1 to 4 times native.
	Resolution func(times int) map[string]string
}

// BIOSRule says where a core looks for its BIOS and what one looks like, so the
// server can say exactly what is missing and find one left beside the games.
type BIOSRule struct {
	Folder  string         // under the system folder
	Example string         // a file name to mention
	For     string         // who it's from, for the message
	Match   *regexp.Regexp // file names, lower case
	MinSize int64
	MaxSize int64
}

// One choice per resolution multiple, from a list of what the core offers.
func pick(key string, choices ...string) func(int) map[string]string {
	return func(times int) map[string]string {
		if times < 1 {
			times = 1
		}
		if times > len(choices) {
			times = len(choices)
		}
		return map[string]string{key: choices[times-1]}
	}
}

// The file name and size of a PlayStation 2 BIOS image, whichever console it came from.
var ps2BIOS = &BIOSRule{
	Folder:  "pcsx2/bios",
	Example: "scph39001.bin",
	For:     "a PlayStation 2 you own",
	Match:   regexp.MustCompile(`^(scph[-_ ]?\d+|ps2[-_ ].*|.*\bbios\b.*)\.(bin|rom0|rom)$`),
	MinSize: 4 << 20,
	MaxSize: (4 << 20) + (1 << 20),
}

var traits = map[string]CoreTraits{
	"mupen64plus_next": {GPU: true, Options: map[string]string{"mupen64plus-rdp-plugin": "gliden64", "mupen64plus-rsp-plugin": "hle"},
		Resolution: pick("mupen64plus-43screensize", "640x480", "1280x960", "1920x1440", "2560x1920")},
	"parallel_n64": {GPU: true, Options: map[string]string{"parallel-n64-gfxplugin": "gliden64"},
		Resolution: pick("parallel-n64-screensize", "640x480", "1280x960", "1920x1440", "2880x2160")},
	"pcsx2": {GPU: true, Assets: "LRPS2", RequiredBIOS: ps2BIOS, Options: map[string]string{"pcsx2_renderer": "OpenGL"},
		Resolution: pick("pcsx2_upscale_multiplier", "1x (Native)", "2x", "4x", "4x")},
	"play": {GPU: true},
	"ppsspp": {GPU: true, Assets: "PPSSPP", Options: map[string]string{"ppsspp_backend": "opengl"},
		Resolution: pick("ppsspp_internal_resolution", "480x272", "960x544", "1440x816", "1920x1088")},
	"flycast": {GPU: true, Options: map[string]string{"reicast_alpha_sorting": "per-triangle (normal)"},
		Resolution: pick("reicast_internal_resolution", "640x480", "1280x960", "1920x1440", "2560x1920")},
	"swanstation": {GPU: true, Options: map[string]string{"swanstation_GPU_Renderer": "OpenGL"},
		Resolution: pick("swanstation_GPU_ResolutionScale", "1", "2", "3", "4")},
	"mednafen_psx_hw": {GPU: true, Options: map[string]string{"beetle_psx_hw_renderer": "hardware_gl"},
		Resolution: pick("beetle_psx_hw_internal_resolution", "1x(native)", "2x", "4x", "4x")},
	// The DS cores read the stylus from the pointer device, and draw in software
	// if told to, but both are faster and sharper on a graphics card.
	"melonds": {GPU: true, Options: map[string]string{"melonds_opengl_renderer": "enabled", "melonds_touch_mode": "Touch", "melonds_screen_layout": "Top/Bottom"},
		Resolution: pick("melonds_opengl_resolution", "1x native (256x192)", "2x native (512x384)", "3x native (768x576)", "4x native (1024x768)")},
	"desmume": {GPU: true, Options: map[string]string{"desmume_opengl_mode": "enabled", "desmume_pointer_type": "touch", "desmume_pointer_mouse": "enabled", "desmume_screens_layout": "top/bottom"},
		Resolution: pick("desmume_internal_resolution", "256x192", "512x384", "768x576", "1024x768")},
	"azahar": {GPU: true, Options: map[string]string{"citra_enable_touch_touchscreen": "enabled", "citra_enable_mouse_touchscreen": "disabled", "citra_enable_touch_pointer_timeout": "disabled", "citra_layout_option": "default"},
		Resolution: pick("citra_resolution_factor", "1", "2", "3", "4")},
	"citra": {GPU: true, Options: map[string]string{"citra_touch_touchscreen": "enabled", "citra_mouse_touchscreen": "disabled", "citra_layout_option": "Default Top-Bottom Screen"},
		Resolution: pick("citra_resolution_factor", "1x (Native)", "2x", "3x", "4x")},
	"panda3ds": {GPU: true},
}

// Traits says how to run a core. A core we don't know gets GPU room, as it may
// draw with a graphics card and nothing says otherwise.
func Traits(id string) CoreTraits {
	if t, ok := traits[id]; ok {
		return t
	}
	if _, listed := CoreLicense(id); listed {
		return CoreTraits{} // a software-rendered core from the console table
	}
	return CoreTraits{GPU: true}
}

// DefaultResolution is how many times native a console renders at with the
// setting on Automatic. Sharp enough to be worth the graphics card, small enough
// to stream well.
func DefaultResolution(consoleID string) int {
	switch consoleID {
	case "n64", "ps1", "ps2", "psp", "dc", "nds":
		return 2
	}
	return 1
}

// CoreOptions are the options to start a core with. resolution is the setting:
// "" for the console's default, or "1" to "4".
func CoreOptions(consoleID, coreID, resolution string) map[string]string {
	t := Traits(coreID)
	out := map[string]string{}
	for k, v := range t.Options {
		out[k] = v
	}
	times, err := strconv.Atoi(resolution)
	if err != nil || times < 1 || times > 4 {
		times = DefaultResolution(consoleID)
	}
	if t.Resolution != nil {
		for k, v := range t.Resolution(times) {
			out[k] = v
		}
	}
	return out
}

// IsBIOSFile says whether a file beside the games is firmware rather than a game,
// going by its name. A .bin of the right name in a game folder shouldn't be listed
// as something to play.
func IsBIOSFile(consoleID, name string) bool {
	lower := strings.ToLower(name)
	if c := ByID(consoleID); c != nil {
		for _, b := range c.BIOS {
			if strings.ToLower(b.File) == lower {
				return true
			}
		}
	}
	if consoleID == "ps2" {
		return ps2BIOS.Match.MatchString(lower)
	}
	return false
}
