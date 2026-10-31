// Package dvdremote plays a disc through a downloaded version of WittyDVD: a helper
// process for each open disc, behind the interfaces a session already uses. With the
// built-in copy in use it does nothing at all, and when a helper fails, the built-in
// copy takes over and the viewer starts the disc again.
//
// A helper may be a fork, so nothing it says is taken on trust: see check.go for what
// is checked, and scramble.go for how the server refuses encrypted discs for itself.
//
// Every call here is made under the session's lock, one at a time, so none of this
// needs a lock of its own.
package dvdremote

import (
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittydvd/wire"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
)

const moduleID = "wittydvd"

// callTimeout is generous because one call may scan a long menu, or read a whole
// cell for its subtitles, from a slow network share. A helper that goes quiet for
// longer is stopped, and the disc goes on with the built-in copy.
const callTimeout = 3 * time.Minute

// helper is the part of a modules.Attached this package uses.
type helper interface {
	Call(method string, params, out any) ([]byte, error)
	Close()
}

// Opener opens discs for sessions. It is what dvdsession.SetEngine takes.
type Opener struct {
	M   *modules.Manager
	Log *slog.Logger
}

// Open uses the helper of the version in use, or the built-in copy when that is the
// one in use, the disc is refused, or the helper can't open it.
func (o Opener) Open(path string) (*dvdsession.Engine, error) {
	a := o.M.Attach(moduleID, callTimeout)
	if a == nil {
		return dvdsession.OpenBuiltin(path)
	}
	report := func(reason string) { _ = o.M.ReportFailure(moduleID, a.Version(), reason) }
	return open(a, report, o.Log, path)
}

// Called with what the helper did wrong: failures that Attached.Call can't see.
type reporter func(reason string)

func open(h helper, report reporter, log *slog.Logger, path string) (*dvdsession.Engine, error) {
	if log == nil {
		log = slog.Default()
	}
	r := &remote{h: h, report: report, log: log, path: path, checked: map[sample]bool{}, files: map[string]*os.File{}}
	builtin := func(reason error) (*dvdsession.Engine, error) {
		log.Info("dvd: using the built-in copy for this disc", "path", path, "reason", reason)
		r.closeAll()
		return dvdsession.OpenBuiltin(path)
	}
	bounds, err := newBounds(path)
	if err != nil {
		return builtin(err)
	}
	r.bounds = bounds
	// An encrypted disc never reaches a helper: the built-in copy refuses it, as ever.
	if err := r.inspect(); err != nil {
		return builtin(err)
	}
	blob, err := h.Call(wire.MethodOpen, wire.OpenParams{Path: path}, nil)
	if err != nil {
		return builtin(err)
	}
	d, err := wire.DecodeDisc(blob)
	if err == nil {
		err = checkDisc(d)
	}
	if err != nil {
		report(err.Error())
		return builtin(err)
	}
	r.engine = &dvdsession.Engine{DiscEngine: r, Disc: d}
	return r.engine, nil
}

// remote is one open disc in a helper.
type remote struct {
	h      helper
	report reporter
	log    *slog.Logger
	path   string
	bounds *bounds
	engine *dvdsession.Engine

	// guard is the built-in copy's way to read the disc, for checking for encryption.
	// Nil when the built-in copy can't open this disc.
	guard   *wittydvd.Opened
	checked map[sample]bool
	files   map[string]*os.File
	samples int

	subs []cachedSubs // the latest cells, as a session asks for them again and again

	opts wittydvd.NavOptions
	main *nav

	// local is the built-in copy once the helper has gone. restart says the viewer
	// hasn't been taken back to the disc's first-play yet.
	local      *dvdsession.Engine
	restart    bool
	lastSerial int
}

// isRefusal says an answer is a working helper saying no about this disc, as the
// built-in copy would have: a read error, say. It goes to the viewer the same way.
// Anything else says the helper is the trouble.
func isRefusal(err error) bool {
	var ce *wittymodule.CallError
	return errors.As(err, &ce) && ce.Code == ""
}

// refusal is what a viewer sees of one, kept to a sensible length.
func refusal(err error) error {
	if s := err.Error(); len(s) > maxReason {
		return errors.New(s[:maxReason])
	}
	return err
}

// giveUp ends the helper and goes on with the built-in copy. If that can't open the
// disc either, the session gets a navigator that stops and an honest error.
func (r *remote) giveUp(why error) {
	if r.local != nil {
		return
	}
	r.log.Warn("dvd: the helper failed; going on with the built-in copy", "path", r.path, "err", why)
	var ce *wittymodule.CallError
	var bad *badAnswer
	if (errors.As(why, &ce) && ce.Code != wire.CodeDenied) || errors.As(why, &bad) {
		// A failure of the process itself was already counted by Attached.Call.
		r.report(why.Error())
	}
	r.closeAll()
	local, err := dvdsession.OpenBuiltin(r.path)
	if err != nil {
		local = &dvdsession.Engine{DiscEngine: deadDisc{err}, Disc: &wittydvd.Disc{VMG: &wittydvd.VMG{}, VTS: map[int]*wittydvd.VTS{}}}
	}
	r.local = local
	r.engine.Disc = local.Disc
	r.restart = true
	if r.main != nil {
		r.main.local = local.NewNav(r.opts)
	}
}

func (r *remote) closeAll() {
	r.h.Close()
	if r.guard != nil {
		_ = r.guard.Close()
		r.guard = nil
	}
	for _, f := range r.files {
		_ = f.Close()
	}
	r.files = map[string]*os.File{}
}

// Close ends the disc.
func (r *remote) Close() error {
	if r.local != nil {
		return r.local.Close()
	}
	r.closeAll()
	return nil
}

// Menu scans a cell for its buttons.
func (r *remote) Menu(sp wittydvd.Space, first, last uint32, opt wittydvd.MenuScanOptions) (*wittydvd.MenuPicture, error) {
	if r.local == nil {
		if r.rangeScrambled(sp, first, last) {
			return &wittydvd.MenuPicture{Scrambled: true}, nil
		}
		pic, err := r.askMenu(sp, first, last, opt)
		if err == nil {
			return pic, nil
		}
		var ce *wittymodule.CallError
		if errors.As(err, &ce) && ce.Code == wire.CodeNoNav {
			return nil, wittydvd.ErrNoNav
		}
		if isRefusal(err) {
			return nil, refusal(err)
		}
		r.giveUp(err)
	}
	return r.local.Menu(sp, first, last, opt)
}

func (r *remote) askMenu(sp wittydvd.Space, first, last uint32, opt wittydvd.MenuScanOptions) (*wittydvd.MenuPicture, error) {
	blob, err := r.h.Call(wire.MethodMenu, wire.MenuCall{Space: sp, First: first, Last: last, Options: opt}, nil)
	if err != nil {
		return nil, err
	}
	pic, err := wire.DecodeMenu(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	if err := checkMenu(pic); err != nil {
		return nil, err
	}
	return pic, nil
}

// RangeExtents says where sectors of a space are.
func (r *remote) RangeExtents(sp wittydvd.Space, first, last uint32) ([]wittydvd.FileExtent, error) {
	return r.extents(wire.MethodRange, wire.RangeCall{Space: sp, First: first, Last: last}, func(l *dvdsession.Engine) ([]wittydvd.FileExtent, error) {
		return l.RangeExtents(sp, first, last)
	})
}

// CellExtents says where a cell is.
func (r *remote) CellExtents(sp wittydvd.Space, c wittydvd.Cell) ([]wittydvd.FileExtent, error) {
	return r.extents(wire.MethodCell, wire.CellCall{Space: sp, Cell: c}, func(l *dvdsession.Engine) ([]wittydvd.FileExtent, error) {
		return l.CellExtents(sp, c)
	})
}

// TitleRuns says how a title's cells are read. Its extents are what FFmpeg will read,
// so each is checked like any other, and the rest has to make sense for these cells.
func (r *remote) TitleRuns(sp wittydvd.Space, cells []wittydvd.TitleCell) ([]wittydvd.TitleRun, error) {
	if r.local == nil {
		runs, err := r.askRuns(sp, cells)
		if err == nil {
			for _, run := range runs {
				if r.extentsScrambled(run.Extents) {
					return nil, errScrambled
				}
			}
			return runs, nil
		}
		if isRefusal(err) {
			return nil, refusal(err)
		}
		r.giveUp(err)
	}
	return r.local.TitleRuns(sp, cells)
}

func (r *remote) askRuns(sp wittydvd.Space, cells []wittydvd.TitleCell) ([]wittydvd.TitleRun, error) {
	blob, err := r.h.Call(wire.MethodRuns, wire.RunsCall{Space: sp, Cells: cells}, nil)
	if err != nil {
		return nil, err
	}
	runs, err := wire.DecodeRuns(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	next := 0
	for _, run := range runs {
		if err := r.bounds.check(run.Extents); err != nil {
			return nil, err
		}
		fine := func(v float64) bool { return v >= 0 && v <= maxRunSeconds } // false for NaN too
		if !fine(run.Duration) || !fine(run.Lead) || run.FirstCell != next || run.Cells < 1 {
			return nil, &badAnswer{errors.New("a title's runs make no sense")}
		}
		next += run.Cells
	}
	if next != len(cells) {
		return nil, &badAnswer{errors.New("a title's runs don't cover its cells")}
	}
	return runs, nil
}

// maxRunSeconds is longer than a disc can hold.
const maxRunSeconds = 24 * 60 * 60

// extents are what FFmpeg will read, so they are checked before anything uses them.
func (r *remote) extents(method string, params any, local func(*dvdsession.Engine) ([]wittydvd.FileExtent, error)) ([]wittydvd.FileExtent, error) {
	if r.local == nil {
		ext, err := r.askExtents(method, params)
		if err == nil {
			if r.extentsScrambled(ext) {
				return nil, errScrambled
			}
			return ext, nil
		}
		if isRefusal(err) {
			return nil, refusal(err)
		}
		r.giveUp(err)
	}
	return local(r.local)
}

func (r *remote) askExtents(method string, params any) ([]wittydvd.FileExtent, error) {
	blob, err := r.h.Call(method, params, nil)
	if err != nil {
		return nil, err
	}
	ext, err := wire.DecodeExtents(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	if err := r.bounds.check(ext); err != nil {
		return nil, err
	}
	return ext, nil
}

type cachedSubs struct {
	sp          wittydvd.Space
	first, last uint32
	subs        *wittydvd.CellSubs
}

// maxCachedSubs is few because each one is here and in the helper as well.
const maxCachedSubs = 4

// CellSubpictures reads a cell's sub-pictures.
func (r *remote) CellSubpictures(sp wittydvd.Space, c wittydvd.Cell) (*wittydvd.CellSubs, error) {
	if r.local == nil {
		for _, k := range r.subs {
			if k.sp == sp && k.first == c.FirstSector && k.last == c.LastSector {
				return k.subs, nil
			}
		}
		// A cell the server finds encrypted shows no subtitles, as with the built-in copy.
		if r.rangeScrambled(sp, c.FirstSector, c.LastSector) {
			return &wittydvd.CellSubs{}, nil
		}
		subs, err := r.askSubs(sp, c)
		if err == nil {
			if len(r.subs) >= maxCachedSubs {
				r.subs = r.subs[1:]
			}
			r.subs = append(r.subs, cachedSubs{sp, c.FirstSector, c.LastSector, subs})
			return subs, nil
		}
		if isRefusal(err) {
			return nil, refusal(err)
		}
		r.giveUp(err)
		r.subs = nil
	}
	return r.local.CellSubpictures(sp, c)
}

func (r *remote) askSubs(sp wittydvd.Space, c wittydvd.Cell) (*wittydvd.CellSubs, error) {
	blob, err := r.h.Call(wire.MethodSubs, wire.CellCall{Space: sp, Cell: c}, nil)
	if err != nil {
		return nil, err
	}
	subs, err := wire.DecodeSubs(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	return subs, nil
}

// NewNav starts the disc's navigator.
func (r *remote) NewNav(o wittydvd.NavOptions) dvdsession.Navigator {
	r.opts = o
	r.main = &nav{r: r}
	if r.local != nil {
		r.main.local = r.local.NewNav(o)
		return r.main
	}
	ro := o.RegisterOptions
	if _, err := r.main.ask(wire.MethodNav, wire.NavOptions{MenuLanguage: ro.MenuLanguage, AudioLanguage: ro.AudioLanguage, SubtitleLanguage: ro.SubtitleLanguage,
		Region: ro.Region, Parental: ro.Parental, Country: ro.Country, Angle: ro.Angle}); err != nil {
		r.giveUp(err)
	}
	return r.main
}

// deadDisc is a disc nothing can read any more: the helper has gone and the built-in
// copy can't open it.
type deadDisc struct{ err error }

func (d deadDisc) Menu(wittydvd.Space, uint32, uint32, wittydvd.MenuScanOptions) (*wittydvd.MenuPicture, error) {
	return nil, d.err
}

func (d deadDisc) RangeExtents(wittydvd.Space, uint32, uint32) ([]wittydvd.FileExtent, error) {
	return nil, d.err
}

func (d deadDisc) CellExtents(wittydvd.Space, wittydvd.Cell) ([]wittydvd.FileExtent, error) {
	return nil, d.err
}

func (d deadDisc) CellSubpictures(wittydvd.Space, wittydvd.Cell) (*wittydvd.CellSubs, error) {
	return nil, d.err
}

func (d deadDisc) TitleRuns(wittydvd.Space, []wittydvd.TitleCell) ([]wittydvd.TitleRun, error) {
	return nil, d.err
}

func (d deadDisc) NewNav(o wittydvd.NavOptions) dvdsession.Navigator {
	return dvdsession.BuiltinNav(&wittydvd.Disc{VMG: &wittydvd.VMG{}, VTS: map[int]*wittydvd.VTS{}}, o)
}

func (d deadDisc) Close() error { return nil }
