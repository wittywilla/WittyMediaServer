// Package wittycouch gets a person from "Play" to a game running on a PC with Sunshine: it
// works out what each device can really do, and falls back to the separately installed Moonlight
// app where a page can't play. The stream goes through relay and the separate WittyCouch Host.
// It contains no Moonlight code, which is GPL-3.0: see docs/GAME-RIGS.md.
package wittycouch

import (
	"strconv"
	"strings"
)

// Device is what a client says it is.
type Device struct {
	Client string `json:"client"` // browser, desktop (the desktop app), android (the Android app), apple (the Apple app)
	OS     string `json:"os"`     // windows, mac, linux, chromeos, android, androidtv, firetv, ios, tvos, or anything else
}

// Target is the rig and game to reach. The address is only filled in for people allowed
// to play this game, and Moonlight still needs its own pairing before it can do anything.
type Target struct {
	RigName string
	Address string // what this device should use from where it is now; empty if nothing works from here
	Port    int    // the base port, 0 for the default
	UUID    string
	AppID   int
	AppName string
	Away    bool // the person isn't on the home network
}

// Method is one way to play, best first.
type Method struct {
	Kind     string   `json:"kind"`               // client, command, intent, steps
	Label    string   `json:"label"`              // what a button says
	OneClick bool     `json:"oneClick"`           // nothing more to do once it's set up
	Note     string   `json:"note,omitempty"`     // anything worth saying first
	Args     []string `json:"args,omitempty"`     // command: arguments for the moonlight program
	Text     string   `json:"text,omitempty"`     // command: the same, as a line to copy
	Package  string   `json:"package,omitempty"`  // intent
	Activity string   `json:"activity,omitempty"` // intent
	Extras   Extras   `json:"extras,omitempty"`   // intent
	Steps    []string `json:"steps,omitempty"`    // steps
}

// Extras are an Android intent's string extras.
type Extras map[string]string

// Install says where this device gets the Moonlight app.
type Install struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	Note    string `json:"note,omitempty"`
	License string `json:"license"`
	Source  string `json:"source"`
}

// The Moonlight app on Android: an activity it exports for launcher shortcuts takes a
// PC and an app by these names.
const (
	AndroidPackage  = "com.limelight"
	AndroidActivity = "com.limelight.ShortcutTrampoline"
)

// Moonlight's own projects, for the licence and source links shown with any download.
const (
	gplv3     = "GPL-3.0"
	qtSource  = "https://github.com/moonlight-stream/moonlight-qt"
	andSource = "https://github.com/moonlight-stream/moonlight-android"
	iosSource = "https://github.com/moonlight-stream/moonlight-ios"
)

// InstallFor says where to get Moonlight for an OS, from the Moonlight project's own
// download page or store listing. Nothing is bundled or re-hosted here. Nil means no
// client is known.
func InstallFor(os string) *Install {
	switch os {
	case "windows", "mac", "linux":
		return &Install{Name: "Moonlight for " + map[string]string{"windows": "Windows", "mac": "Mac", "linux": "Linux"}[os],
			URL: "https://github.com/moonlight-stream/moonlight-qt/releases/latest", License: gplv3, Source: qtSource}
	case "android", "androidtv":
		return &Install{Name: "Moonlight for Android", URL: "https://play.google.com/store/apps/details?id=com.limelight", License: gplv3, Source: andSource,
			Note: "Android TV and Google TV have it too: search the TV's Play Store for Moonlight."}
	case "firetv":
		return &Install{Name: "Moonlight for Fire TV", URL: "https://www.amazon.com/gp/product/B00JK4MFN2", License: gplv3, Source: andSource}
	case "ios":
		return &Install{Name: "Moonlight for iPhone and iPad", URL: "https://apps.apple.com/us/app/moonlight-game-streaming/id1000551566", License: gplv3, Source: iosSource}
	case "tvos":
		return &Install{Name: "Moonlight for Apple TV", URL: "https://apps.apple.com/us/app/moonlight-game-streaming/id1000551566", License: gplv3, Source: iosSource,
			Note: "On the Apple TV, open the App Store and search for Moonlight."}
	case "chromeos":
		return &Install{Name: "Moonlight for Android", URL: "https://play.google.com/store/apps/details?id=com.limelight", License: gplv3, Source: andSource,
			Note: "Chromebooks that run Android apps use the Android version."}
	}
	return nil
}

// cleanArg: nothing that a program could read as an option, and no line breaks to mislead
// someone copying a command. Sunshine's app names are free text set on the PC, so this is
// the difference between a game called "--quit-after" and a surprise.
func cleanArg(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && !strings.ContainsAny(s, "\r\n\x00")
}

// quoted makes one word of a command line to paste. An app's name is free text set on the
// PC, so what ends a quoted word matters: PowerShell also ends one at curly quotes, and
// doubling is its way of keeping any of them. A POSIX shell has no way inside single
// quotes, and needs the quote stepped out of.
func quoted(os, s string) string {
	if os == "windows" {
		return "'" + powershellQuotes.Replace(s) + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var powershellQuotes = strings.NewReplacer("'", "''", "\u2018", "\u2018\u2018", "\u2019", "\u2019\u2019", "\u201a", "\u201a\u201a", "\u201b", "\u201b\u201b")

func withPort(address string, port int) string {
	if strings.Contains(address, ":") { // IPv6
		address = "[" + address + "]"
	}
	if port <= 0 || port == 47989 {
		return address
	}
	return address + ":" + strconv.Itoa(port)
}

// MoonlightArgs are the arguments that start a game with the desktop Moonlight app. Nil
// means this game's name can't be passed safely.
func MoonlightArgs(t Target) []string {
	if !cleanArg(t.Address) || !cleanArg(t.AppName) {
		return nil
	}
	return []string{"stream", withPort(t.Address, t.Port), t.AppName}
}

// PairArgs are the arguments that make the desktop Moonlight app ask a PC to pair, with a
// PIN chosen in advance so it can be shown anywhere.
func PairArgs(address string, port int, pin string) []string {
	if !cleanArg(address) || len(pin) != 4 {
		return nil
	}
	return []string{"pair", withPort(address, port), "--pin", pin}
}

// manualSteps is what's left on devices where nothing can start Moonlight for you.
func manualSteps(t Target) []string {
	return []string{
		"Open the Moonlight app.",
		"Choose " + t.RigName + ".",
		"Choose " + t.AppName + ".",
	}
}

// Plan lists what really works on this device, best first. Playing in the page, through this
// server, comes first wherever a device can run it: clientNote is empty unless there's something
// to say first. Handing off to the Moonlight app is the fallback. What's listed here is checked
// against the table in docs/GAME-RIGS.md.
func Plan(d Device, t Target, inPage bool, clientNote string) []Method {
	var out []Method
	if inPage && d.OS != "tvos" {
		out = append(out, Method{Kind: "client", Label: "Play here", OneClick: true, Note: clientNote})
	}
	if t.Address == "" {
		// Away from home with no VPN address: Moonlight has nothing it could connect to.
		return out
	}
	args := MoonlightArgs(t)
	switch {
	case d.Client == "desktop" && args != nil:
		out = append(out, Method{Kind: "command", Label: "Open in Moonlight", OneClick: true, Args: args})
	case d.Client == "android":
		if t.UUID != "" {
			out = append(out, Method{Kind: "intent", Label: "Open in Moonlight", OneClick: true, Package: AndroidPackage, Activity: AndroidActivity,
				Extras: Extras{"UUID": t.UUID, "AppId": strconv.Itoa(t.AppID), "AppName": t.AppName},
				Note:   "Moonlight has to know this PC already: pair it once inside Moonlight."})
		}
	}
	// A browser can't start Moonlight, as Moonlight has no link scheme on any platform. On
	// a computer it can at least offer the line that does it.
	if d.Client == "browser" && args != nil && (d.OS == "windows" || d.OS == "mac" || d.OS == "linux") {
		out = append(out, Method{Kind: "command", Label: "Copy the Moonlight command", Args: args, Text: commandLine(d.OS, args),
			Note: "Browsers can't open Moonlight by themselves. Paste this in a terminal, or open Moonlight and choose the game."})
	}
	return append(out, Method{Kind: "steps", Label: "Open it in Moonlight yourself", Steps: manualSteps(t)})
}

func commandLine(os string, args []string) string {
	prog := "moonlight"
	switch os {
	case "windows":
		prog = `& 'C:\Program Files\Moonlight Game Streaming\Moonlight.exe'`
	case "mac":
		prog = "/Applications/Moonlight.app/Contents/MacOS/Moonlight"
	}
	parts := []string{prog}
	for _, a := range args {
		parts = append(parts, quoted(os, a))
	}
	return strings.Join(parts, " ")
}

// SetupSteps is how a device pairs with a rig the first time, in words for that device.
func SetupSteps(os, rigName, address string, port int) []string {
	where := withPort(address, port)
	steps := []string{"Install Moonlight on this device if it isn't there yet."}
	steps = append(steps, "Open Moonlight. "+rigName+" should appear by itself on your home network. If it doesn't, add a PC by address and type "+where+".")
	steps = append(steps, "Choose "+rigName+". Moonlight shows a four-digit PIN.")
	steps = append(steps, "Ask whoever runs this server to type that PIN in Settings, Game rigs, or type it yourself in Sunshine on that PC.")
	return steps
}
