// Package dvdremote plays a disc through the WittyDVD version in use: a helper process for
// each open disc, behind the interfaces a session uses. When a helper fails the disc stops,
// with the reason, and opening it again starts a new one.
//
// A helper may be a fork, so nothing it says is taken on trust: see check.go for what
// is checked, and scramble.go for how the server refuses encrypted discs for itself.
//
// Every call here is made under the session's lock, one at a time, so none of this
// needs a lock of its own.
package dvdremote

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/dvd"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
	"github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymodule"
)

const moduleID = "wittydvd"

// callTimeout is generous because one call may scan a long menu, or read a whole
// cell for its subtitles, from a slow network share. A helper that goes quiet for
// longer is stopped.
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

// Open uses a helper of the version in use. Without one, it says which module is missing.
func (o Opener) Open(path string) (*dvdsession.Engine, error) {
	a := o.M.Attach(moduleID, callTimeout)
	if a == nil {
		return nil, o.M.Missing(moduleID)
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
	stop := func(err error) (*dvdsession.Engine, error) {
		r.closeAll()
		return nil, err
	}
	bounds, err := newBounds(path)
	if err != nil {
		return stop(err)
	}
	r.bounds = bounds
	// An encrypted disc never reaches a helper.
	if err := r.inspect(); err != nil {
		return stop(err)
	}
	var opened dvd.Opened
	blob, err := h.Call(dvd.MethodOpen, dvd.OpenParams{Path: path}, &opened)
	if err != nil {
		return stop(err)
	}
	d, err := dvd.DecodeDisc(blob)
	if err == nil {
		err = checkDisc(d)
	}
	if err != nil {
		report(err.Error())
		return stop(err)
	}
	r.engine = &dvdsession.Engine{DiscEngine: r, Disc: d, Label: clipText(opened.Label), Format: clipText(opened.Format)}
	return r.engine, nil
}

func clipText(s string) string {
	if len(s) > 256 {
		return s[:256]
	}
	return s
}

// remote is one open disc in a helper.
type remote struct {
	h      helper
	report reporter
	log    *slog.Logger
	path   string
	bounds *bounds
	engine *dvdsession.Engine

	checked map[sample]bool
	files   map[string]*os.File
	samples int

	subs []cachedSubs // the latest cells, as a session asks for them again and again

	opts dvd.NavOptions
	main *nav

	// dead is why the helper has gone. Nothing on this disc works after that.
	dead       error
	lastSerial int
}

// isRefusal says an answer is a working helper saying no about this disc: a read error,
// say. It goes to the viewer the same way.
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

// giveUp ends the helper, and with it this disc.
func (r *remote) giveUp(why error) {
	if r.dead != nil {
		return
	}
	r.log.Warn("dvd: the helper failed", "path", r.path, "err", why)
	var ce *wittymodule.CallError
	var bad *badAnswer
	if (errors.As(why, &ce) && ce.Code != dvd.CodeDenied) || errors.As(why, &bad) {
		// A failure of the process itself was already counted by Attached.Call.
		r.report(why.Error())
	}
	r.closeAll()
	r.dead = fmt.Errorf("WittyDVD stopped working on this disc: %w", why)
}

func (r *remote) closeAll() {
	r.h.Close()
	for _, f := range r.files {
		_ = f.Close()
	}
	r.files = map[string]*os.File{}
}

// Close ends the disc.
func (r *remote) Close() error {
	if r.dead == nil {
		r.closeAll()
	}
	return nil
}

// Menu scans a cell for its buttons.
func (r *remote) Menu(sp dvd.Space, first, last uint32, opt dvd.MenuScanOptions) (*dvd.MenuPicture, error) {
	if r.dead != nil {
		return nil, r.dead
	}
	if r.rangeScrambled(sp, first, last) {
		return &dvd.MenuPicture{Scrambled: true}, nil
	}
	pic, err := r.askMenu(sp, first, last, opt)
	if err == nil {
		return pic, nil
	}
	var ce *wittymodule.CallError
	if errors.As(err, &ce) && ce.Code == dvd.CodeNoNav {
		return nil, dvd.ErrNoNav
	}
	if isRefusal(err) {
		return nil, refusal(err)
	}
	r.giveUp(err)
	return nil, r.dead
}

func (r *remote) askMenu(sp dvd.Space, first, last uint32, opt dvd.MenuScanOptions) (*dvd.MenuPicture, error) {
	blob, err := r.h.Call(dvd.MethodMenu, dvd.MenuCall{Space: sp, First: first, Last: last, Options: opt}, nil)
	if err != nil {
		return nil, err
	}
	pic, err := dvd.DecodeMenu(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	if err := checkMenu(pic); err != nil {
		return nil, err
	}
	return pic, nil
}

// RangeExtents says where sectors of a space are.
func (r *remote) RangeExtents(sp dvd.Space, first, last uint32) ([]dvd.FileExtent, error) {
	return r.extents(dvd.MethodRange, dvd.RangeCall{Space: sp, First: first, Last: last})
}

// CellExtents says where a cell is.
func (r *remote) CellExtents(sp dvd.Space, c dvd.Cell) ([]dvd.FileExtent, error) {
	return r.extents(dvd.MethodCell, dvd.CellCall{Space: sp, Cell: c})
}

// TitleRuns says how a title's cells are read. Its extents are what FFmpeg will read,
// so each is checked like any other, and the rest has to make sense for these cells.
func (r *remote) TitleRuns(sp dvd.Space, cells []dvd.TitleCell) ([]dvd.TitleRun, error) {
	if r.dead != nil {
		return nil, r.dead
	}
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
	return nil, r.dead
}

// RenderSPU has one sub-picture unit drawn. Its pictures only go to the viewer as subtitles,
// and a session checks the sizes of those it draws.
func (r *remote) RenderSPU(b []byte, palette [16]uint32) ([]dvd.SPUFrame, error) {
	if r.dead != nil {
		return nil, r.dead
	}
	blob, err := r.h.Call(dvd.MethodSPU, dvd.SPUCall{Bytes: b, Palette: palette}, nil)
	if err == nil {
		var frames []dvd.SPUFrame
		if frames, err = dvd.DecodeSPU(blob); err == nil {
			return frames, nil
		}
		err = &badAnswer{err}
	}
	if isRefusal(err) {
		return nil, refusal(err)
	}
	r.giveUp(err)
	return nil, r.dead
}

// extents are what FFmpeg will read, so they are checked before anything uses them.
func (r *remote) extents(method string, params any) ([]dvd.FileExtent, error) {
	if r.dead != nil {
		return nil, r.dead
	}
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
	return nil, r.dead
}

func (r *remote) askExtents(method string, params any) ([]dvd.FileExtent, error) {
	blob, err := r.h.Call(method, params, nil)
	if err != nil {
		return nil, err
	}
	ext, err := dvd.DecodeExtents(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	if err := r.bounds.check(ext); err != nil {
		return nil, err
	}
	return ext, nil
}

type cachedSubs struct {
	sp          dvd.Space
	first, last uint32
	subs        *dvd.CellSubs
}

// maxCachedSubs is few because each one is here and in the helper as well.
const maxCachedSubs = 4

// CellSubpictures reads a cell's sub-pictures.
func (r *remote) CellSubpictures(sp dvd.Space, c dvd.Cell) (*dvd.CellSubs, error) {
	if r.dead != nil {
		return nil, r.dead
	}
	for _, k := range r.subs {
		if k.sp == sp && k.first == c.FirstSector && k.last == c.LastSector {
			return k.subs, nil
		}
	}
	// A cell the server finds encrypted shows no subtitles.
	if r.rangeScrambled(sp, c.FirstSector, c.LastSector) {
		return &dvd.CellSubs{}, nil
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
	return nil, r.dead
}

func (r *remote) askSubs(sp dvd.Space, c dvd.Cell) (*dvd.CellSubs, error) {
	blob, err := r.h.Call(dvd.MethodSubs, dvd.CellCall{Space: sp, Cell: c}, nil)
	if err != nil {
		return nil, err
	}
	subs, err := dvd.DecodeSubs(blob)
	if err != nil {
		return nil, &badAnswer{err}
	}
	return subs, nil
}

// NewNav starts the disc's navigator.
func (r *remote) NewNav(o dvd.NavOptions) dvdsession.Navigator {
	r.opts = o
	r.main = &nav{r: r}
	if r.dead != nil {
		return r.main
	}
	ro := o.RegisterOptions
	if _, err := r.main.ask(dvd.MethodNav, dvd.NavParams{MenuLanguage: ro.MenuLanguage, AudioLanguage: ro.AudioLanguage, SubtitleLanguage: ro.SubtitleLanguage,
		Region: ro.Region, Parental: ro.Parental, Country: ro.Country, Angle: ro.Angle}); err != nil {
		r.giveUp(err)
	}
	return r.main
}
