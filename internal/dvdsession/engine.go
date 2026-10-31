package dvdsession

import (
	"sync/atomic"

	"github.com/wittywilla/wittydvd"
)

// DiscEngine is what a session asks of one open disc, apart from its structure. The
// built-in copy of WittyDVD answers it, or a helper process running a downloaded
// version (package dvdremote).
type DiscEngine interface {
	Menu(sp wittydvd.Space, first, last uint32, opt wittydvd.MenuScanOptions) (*wittydvd.MenuPicture, error)
	RangeExtents(sp wittydvd.Space, first, last uint32) ([]wittydvd.FileExtent, error)
	CellExtents(sp wittydvd.Space, c wittydvd.Cell) ([]wittydvd.FileExtent, error)
	CellSubpictures(sp wittydvd.Space, c wittydvd.Cell) (*wittydvd.CellSubs, error)
	NewNav(o wittydvd.NavOptions) Navigator
	Close() error
}

// Engine is one open disc. Disc is its structure, which an engine may replace while
// answering a call: every call of a session is made under the session's lock.
type Engine struct {
	DiscEngine
	Disc *wittydvd.Disc
}

// Navigator is a disc's virtual machine for one viewer, exactly as a session uses it.
type Navigator interface {
	Start() *wittydvd.Step
	PlayTitle(ttn, pttn int) *wittydvd.Step
	OpenMenu(kind string, vts int) *wittydvd.Step
	SelectButton(btn int) *wittydvd.Step
	PressButton(btn int, cmd *wittydvd.Command) *wittydvd.Step
	MenuKey(kind string) *wittydvd.Step
	Resume() *wittydvd.Step
	NextChapter() *wittydvd.Step
	PrevChapter() *wittydvd.Step
	GoUp() *wittydvd.Step
	CellEnded() *wittydvd.Step
	Tick(seconds float64) *wittydvd.Step
	Current() *wittydvd.Step
	SetAudio(stream int)
	SetSubpicture(stream int, on bool)
	SetAngle(a int)
	SyncTitleCell(pgcn, cellN int) bool
	Registers() *wittydvd.Registers
	StepPGC(st *wittydvd.Step) *wittydvd.PGC
	Clone() Navigator
}

type builtinDisc struct{ *wittydvd.Opened }

func (b builtinDisc) NewNav(o wittydvd.NavOptions) Navigator { return BuiltinNav(b.Disc, o) }

type builtinNav struct{ *wittydvd.Nav }

func (n builtinNav) Clone() Navigator { return builtinNav{n.Nav.Clone()} }

// BuiltinNav is a navigator of the built-in copy over a disc's structure.
func BuiltinNav(d *wittydvd.Disc, o wittydvd.NavOptions) Navigator {
	return builtinNav{wittydvd.NewNav(d, o)}
}

// OpenBuiltin opens a disc with the built-in copy.
func OpenBuiltin(path string) (*Engine, error) {
	o, err := wittydvd.Open(path)
	if err != nil {
		return nil, err
	}
	return &Engine{DiscEngine: builtinDisc{o}, Disc: o.Disc}, nil
}

var engineOpener atomic.Pointer[func(path string) (*Engine, error)]

// SetEngine chooses what opens discs, once at start. Without it, it is the built-in copy.
func SetEngine(open func(path string) (*Engine, error)) { engineOpener.Store(&open) }

func openEngine(path string) (*Engine, error) {
	if open := engineOpener.Load(); open != nil {
		return (*open)(path)
	}
	return OpenBuiltin(path)
}
