// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package dvdsession

import (
	"errors"
	"sync/atomic"

	"github.com/wittywilla/wittymediaserver/internal/dvd"
)

// DiscEngine is what a session asks of one open disc, apart from its structure. A helper
// process running the WittyDVD version in use answers it (package dvdremote).
type DiscEngine interface {
	Menu(sp dvd.Space, first, last uint32, opt dvd.MenuScanOptions) (*dvd.MenuPicture, error)
	RangeExtents(sp dvd.Space, first, last uint32) ([]dvd.FileExtent, error)
	CellExtents(sp dvd.Space, c dvd.Cell) ([]dvd.FileExtent, error)
	CellSubpictures(sp dvd.Space, c dvd.Cell) (*dvd.CellSubs, error)
	TitleRuns(sp dvd.Space, cells []dvd.TitleCell) ([]dvd.TitleRun, error)
	RenderSPU(b []byte, palette [16]uint32) ([]dvd.SPUFrame, error)
	NewNav(o dvd.NavOptions) Navigator
	Close() error
}

// Engine is one open disc. Disc is its structure, which an engine may replace while
// answering a call: every call of a session is made under the session's lock.
type Engine struct {
	DiscEngine
	Disc   *dvd.Disc
	Label  string
	Format string
}

// Navigator is a disc's virtual machine for one viewer, exactly as a session uses it.
type Navigator interface {
	Start() *dvd.Step
	PlayTitle(ttn, pttn int) *dvd.Step
	OpenMenu(kind string, vts int) *dvd.Step
	SelectButton(btn int) *dvd.Step
	PressButton(btn int, cmd *dvd.Command) *dvd.Step
	MenuKey(kind string) *dvd.Step
	Resume() *dvd.Step
	NextChapter() *dvd.Step
	PrevChapter() *dvd.Step
	GoUp() *dvd.Step
	CellEnded() *dvd.Step
	Tick(seconds float64) *dvd.Step
	Current() *dvd.Step
	SetAudio(stream int)
	SetSubpicture(stream int, on bool)
	SetAngle(a int)
	SyncTitleCell(pgcn, cellN int) bool
	Registers() *dvd.Registers
	StepPGC(st *dvd.Step) *dvd.PGC
	Clone() Navigator
}

var engineOpener atomic.Pointer[func(path string) (*Engine, error)]

// SetEngine says how discs are opened, once at start.
func SetEngine(open func(path string) (*Engine, error)) { engineOpener.Store(&open) }

// ErrNoEngine: nothing has been set to open a disc with.
var ErrNoEngine = errors.New("WittyDVD isn't on this server")

// Open opens a disc through the WittyDVD version in use.
func Open(path string) (*Engine, error) { return openEngine(path) }

func openEngine(path string) (*Engine, error) {
	if open := engineOpener.Load(); open != nil {
		return (*open)(path)
	}
	return nil, ErrNoEngine
}
