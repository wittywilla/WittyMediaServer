package bdsession

import (
	"log/slog"
	"strings"

	"github.com/wittywilla/wittybluray"
)

// A session reads its disc and runs its program through these two interfaces. The
// built-in engine is the default, and is what they usually are. When a downloaded
// WittyBluray is in use they are a helper process for this one disc instead, so a
// version with new fixes reaches a server that hasn't changed.

// DiscEngine is what a session asks of an opened disc.
type DiscEngine interface {
	Clip(name string) *wittybluray.Clip
	Playlist(n int) *wittybluray.Playlist
	MainPlaylist() *wittybluray.Playlist
	PlaylistDuration(pl *wittybluray.Playlist) float64
	ClipExtents(name string) ([]wittybluray.FileExtent, error)
	ClipLead(clip string, video uint16, from uint32, only ...uint16) float64
	Sounds() *wittybluray.Sounds
	LoadMenu(clip string, pid uint16, limit uint32) (*wittybluray.Menu, error)
	BDJObject(name string) (*wittybluray.BDJObject, error)
	SubtitlesIn(clip string, pid uint16, stc int, from, to uint32) (*wittybluray.SubtitleWindow, error)
	TextSubtitles(clip string, pid uint16, charCode int) ([]wittybluray.TextCue, error)
	Close() error
}

// Navigator runs the disc's program. Steps are read and changed by the session as
// its own, and Registers is the same registers from call to call.
type Navigator interface {
	Registers() *wittybluray.Registers
	Timer() *wittybluray.NavTimer
	Start() *wittybluray.Step
	Resume() *wittybluray.Step
	MenuCall() *wittybluray.Step
	PlaylistEnded() *wittybluray.Step
	NavTimerFired() *wittybluray.Step
	PlayTitle(t int) *wittybluray.Step
	PlayPlaylist(pl *wittybluray.Playlist, item int, t uint32) *wittybluray.Step
	PressButton(cmds []wittybluray.Command) *wittybluray.Step
	SkipItem(delta int) (*wittybluray.Step, bool)
	SetSubtitle(s int, shown bool)
	SetAudio(s int)
	SetAngle(a int)
	SetTime(item int, t uint32)
	SetMenuState(page, button int)
	SetChapter(c int)
	LoopsAtEnd(st *wittybluray.Step) bool
	Forbids(uo uint64) bool
}

// engineDisc is a session's disc: the engine it asks, and what the server itself
// found when it opened it.
type engineDisc struct {
	DiscEngine
	// Encrypted comes from the server's own reading of the disc, so it holds whatever
	// a helper says. A helper can add to it, not take away.
	Encrypted bool
	Stereo3D  bool
	Index     *wittybluray.Index
	// builtin is the disc when the built-in engine has it, and nil with a helper. Java
	// programs only run from the built-in engine.
	builtin *wittybluray.Disc
}

// Remote is a disc opened by a helper: both faces of it, and what it says of the disc.
type Remote struct {
	Disc                DiscEngine
	Nav                 Navigator
	Encrypted, Stereo3D bool
}

// RemoteEngine opens a disc through the WittyBluray version in use. It returns nil
// when that is the built-in copy, or when a helper can't open this disc, and the
// session then uses the built-in copy. The server sets it as it starts; nil is no
// helper at all.
var RemoteEngine func(path string, o wittybluray.NavOptions, log *slog.Logger) *Remote

// openEngine opens a disc for a session. The server always reads the disc itself
// first: that is what refuses an encrypted one, whatever a helper makes of it, and
// says whether it has Java programs to run.
func (m *Manager) openEngine(path string, o wittybluray.NavOptions) (*engineDisc, Navigator, error) {
	d, err := wittybluray.Open(path)
	if err != nil {
		return nil, nil, err
	}
	od := &engineDisc{DiscEngine: d, Encrypted: d.Encrypted, Stereo3D: d.Stereo3D, Index: d.Index, builtin: d}
	if RemoteEngine == nil || d.Encrypted || m.runsJava(d) {
		return od, wittybluray.NewNav(d, o), nil
	}
	r := RemoteEngine(path, o, m.Log)
	if r == nil {
		return od, wittybluray.NewNav(d, o), nil
	}
	if r.Encrypted {
		_ = r.Disc.Close()
		od.Encrypted = true
		return od, wittybluray.NewNav(d, o), nil
	}
	_ = d.Close() // the helper has it now, and the server needn't keep a second copy in memory
	return &engineDisc{DiscEngine: r.Disc, Stereo3D: d.Stereo3D || r.Stereo3D, Index: d.Index}, r.Nav, nil
}

// runsJava: a disc with Java programs, on a server that has that turned on. They run
// beside the session and share its navigator, so the built-in copy has this disc.
func (m *Manager) runsJava(d *wittybluray.Disc) bool {
	if !d.BDJ || m.BDJ == nil {
		return false
	}
	enabled, _ := m.BDJ()
	return enabled
}

// navOptions is the player a viewer's choices describe.
func navOptions(prefs Prefs) wittybluray.NavOptions {
	return wittybluray.NavOptions{Registers: wittybluray.RegisterOptions{
		AudioLang: lang3(prefs.AudioLanguage), SubtitleLang: lang3(prefs.SubtitleLanguage), MenuLang: lang3(prefs.MenuLanguage),
		Region: prefs.BDRegion, Parental: prefs.BDAge, Country: strings.ToUpper(prefs.Country),
	}}
}
