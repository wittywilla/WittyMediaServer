package bdsession

import (
	"errors"
	"log/slog"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/bluray"
)

// A session reads its disc and runs its program through these two interfaces, which a helper
// process of the WittyBluray version in use answers, one process for one disc.

// DiscEngine is what a session asks of an opened disc.
type DiscEngine interface {
	Clip(name string) *bluray.Clip
	Playlist(n int) *bluray.Playlist
	MainPlaylist() *bluray.Playlist
	PlaylistDuration(pl *bluray.Playlist) float64
	ClipExtents(name string) ([]bluray.FileExtent, error)
	ClipLead(clip string, video uint16, from uint32, only ...uint16) float64
	Sounds() *bluray.Sounds
	LoadMenu(clip string, pid uint16, limit uint32) (*bluray.Menu, error)
	BDJObject(name string) (*bluray.BDJObject, error)
	SubtitlesIn(clip string, pid uint16, stc int, from, to uint32) (*bluray.SubtitleWindow, error)
	TextSubtitles(clip string, pid uint16, charCode int) ([]bluray.TextCue, error)
	Close() error
}

// Navigator runs the disc's program. Steps are read and changed by the session as
// its own, and Registers is the same registers from call to call.
type Navigator interface {
	Registers() *bluray.Registers
	Timer() *bluray.NavTimer
	Start() *bluray.Step
	Resume() *bluray.Step
	MenuCall() *bluray.Step
	PlaylistEnded() *bluray.Step
	NavTimerFired() *bluray.Step
	PlayTitle(t int) *bluray.Step
	PlayPlaylist(pl *bluray.Playlist, item int, t uint32) *bluray.Step
	PressButton(cmds []bluray.Command) *bluray.Step
	SkipItem(delta int) (*bluray.Step, bool)
	SetSubtitle(s int, shown bool)
	SetAudio(s int)
	SetAngle(a int)
	SetTime(item int, t uint32)
	SetMenuState(page, button int)
	SetChapter(c int)
	LoopsAtEnd(st *bluray.Step) bool
	Forbids(uo uint64) bool
}

// engineDisc is a session's disc: the engine it asks, and what was found when it opened.
type engineDisc struct {
	DiscEngine
	// Encrypted comes from the server's own reading of the disc, so it holds whatever
	// a helper says. A helper can add to it, not take away.
	Encrypted bool
	Stereo3D  bool
	BDJ       bool
	Label     string
	Index     *bluray.Index
	Playlists []int
}

// Remote is a disc opened by a helper: both faces of it, and what it says of the disc.
type Remote struct {
	Disc                DiscEngine
	Nav                 Navigator
	Encrypted, Stereo3D bool
	BDJ                 bool
	Label               string
	Index               *bluray.Index
	Playlists           []int
	// Java runs the disc's Java programs, if it can. Nil means it can't.
	Java JavaEngine
}

// ErrNoEngine: nothing has been set to open a disc with.
var ErrNoEngine = errors.New("WittyBluray isn't on this server")

// RemoteEngine opens a disc through the WittyBluray version in use. The server sets it as it
// starts.
var RemoteEngine func(path string, o bluray.NavOptions, log *slog.Logger) (*Remote, error)

// openEngine opens a disc for a session. The server always looks at the disc itself first:
// that is what refuses an encrypted one, whatever a helper makes of it.
func (m *Manager) openEngine(path string, o bluray.NavOptions) (*engineDisc, Navigator, error) {
	encrypted := Scrambled(path)
	if encrypted {
		return &engineDisc{Encrypted: true}, nil, nil
	}
	if RemoteEngine == nil {
		return nil, nil, ErrNoEngine
	}
	r, err := RemoteEngine(path, o, m.Log)
	if err != nil {
		return nil, nil, err
	}
	if r.Encrypted {
		_ = r.Disc.Close()
		return &engineDisc{Encrypted: true}, nil, nil
	}
	return &engineDisc{DiscEngine: r.Disc, Stereo3D: r.Stereo3D, BDJ: r.BDJ, Label: r.Label, Index: r.Index, Playlists: r.Playlists}, r.Nav, nil
}

// Open opens a disc through the WittyBluray version in use, for what isn't a viewer's session.
func Open(path string, log *slog.Logger) (*Remote, error) {
	if Scrambled(path) {
		return &Remote{Encrypted: true}, nil
	}
	if RemoteEngine == nil {
		return nil, ErrNoEngine
	}
	return RemoteEngine(path, bluray.NavOptions{}, log)
}

// runsJava: a disc with Java programs, on a server that has that turned on.
func (m *Manager) runsJava(d *engineDisc) bool {
	if !d.BDJ || m.BDJ == nil {
		return false
	}
	enabled, _ := m.BDJ()
	return enabled
}

// navOptions is the player a viewer's choices describe.
func navOptions(prefs Prefs) bluray.NavOptions {
	return bluray.NavOptions{Registers: bluray.RegisterOptions{
		AudioLang: lang3(prefs.AudioLanguage), SubtitleLang: lang3(prefs.SubtitleLanguage), MenuLang: lang3(prefs.MenuLanguage),
		Region: prefs.BDRegion, Parental: prefs.BDAge, Country: strings.ToUpper(prefs.Country),
	}}
}
