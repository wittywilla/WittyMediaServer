package dvdremote

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/wittywilla/wittydvd"
)

// The most a disc can have. A DVD's own tables can't go past most of these, and those
// it could are far beyond any disc.
const (
	maxTitles    = 99
	maxTitleSets = 99
	maxAudio     = 8
	maxSubtitles = 32
	maxPGCs      = 1 << 16
	maxPGCN      = 1 << 16 // a program chain or cell number
	maxUnits     = 256
	maxCells     = 256
	maxPrograms  = 256
	maxButtons   = 256
	maxFrameSide = 1024    // a DVD's picture is at most 720x576
	maxSeconds   = 400000  // what a DVD's time field can hold
	maxSerial    = 1 << 30 // no viewer gets this far, adding to it stays safe, and it fits every platform
	maxReason    = 500     // a reason is shown to the viewer
	maxPosition  = 1 << 16 // of a picture, on or off the frame
	maxExtents   = 100000  // an interleaved feature has a few thousand
	maxOffset    = 1 << 50 // far past any disc
)

// badAnswer is an answer from a helper that no honest helper gives. It counts against
// the version, and the built-in copy takes over.
type badAnswer struct{ err error }

func (b *badAnswer) Error() string { return b.err.Error() }
func (b *badAnswer) Unwrap() error { return b.err }

func bad(format string, a ...any) error {
	return &badAnswer{fmt.Errorf(format, a...)}
}

// seconds is false for NaN too.
func seconds(v float64) bool { return v >= 0 && v <= maxSeconds }

// checkStep checks what a session will use from a step, and fills DomainName, which
// names cache files, from the domain number rather than from what the helper said.
func checkStep(st *wittydvd.Step) error {
	if st == nil {
		return nil
	}
	switch st.Kind {
	case wittydvd.StepMenu, wittydvd.StepTitle, wittydvd.StepStop:
	default:
		return bad("the helper gave a step of an unknown kind")
	}
	if st.Domain < wittydvd.DomainFP || st.Domain > wittydvd.DomainVTS {
		return bad("the helper gave a step in an unknown domain")
	}
	st.DomainName = st.Domain.String()
	if st.VTS < 0 || st.VTS > maxTitleSets || st.PGCN < 0 || st.PGCN > maxPGCN || st.CellN < 0 || st.CellN > maxPGCN {
		return bad("the helper gave a step outside the disc")
	}
	if st.Serial < 0 || st.Serial > maxSerial {
		return bad("the helper gave a step with an impossible serial")
	}
	if !seconds(st.CellDuration) || !(math.IsInf(st.StillSec, 1) || seconds(st.StillSec)) {
		return bad("the helper gave a step with an impossible time")
	}
	if st.DefaultButton < 0 || st.DefaultButton > maxButtons {
		return bad("the helper gave a step with an impossible button")
	}
	if t := st.Title; t != nil && !seconds(t.Duration) {
		return bad("the helper gave a title with an impossible time")
	}
	if len(st.Reason) > maxReason {
		st.Reason = st.Reason[:maxReason]
	}
	return nil
}

// checkPGC checks a program chain.
func checkPGC(p *wittydvd.PGC) error {
	if len(p.Cells) > maxCells || len(p.Programs) > maxPrograms {
		return bad("the helper gave a program chain of an impossible size")
	}
	if !seconds(p.Duration) {
		return bad("the helper gave a program chain with an impossible time")
	}
	for _, c := range p.Cells {
		if !seconds(c.Duration) {
			return bad("the helper gave a cell with an impossible time")
		}
	}
	return nil
}

func checkEntries(es []wittydvd.PGCEntry) error {
	if len(es) > maxPGCs {
		return bad("the helper gave a disc of an impossible size")
	}
	for _, e := range es {
		if e.PGC == nil {
			return bad("the helper gave a disc with a program chain missing")
		}
		if err := checkPGC(e.PGC); err != nil {
			return err
		}
	}
	return nil
}

func checkUnits(us []wittydvd.LangUnit) error {
	if len(us) > maxUnits {
		return bad("the helper gave a disc of an impossible size")
	}
	for _, u := range us {
		if err := checkEntries(u.PGCs); err != nil {
			return err
		}
	}
	return nil
}

// checkDisc checks a disc's structure, which the session's own code then walks: every
// table it indexes must have the size that code expects, as the parser makes sure for
// the built-in copy.
func checkDisc(d *wittydvd.Disc) error {
	if d.VMG == nil {
		return bad("the helper gave a disc with no structure")
	}
	if len(d.VMG.Titles) > maxTitles || len(d.VTS) > maxTitleSets {
		return bad("the helper gave a disc of an impossible size")
	}
	if p := d.VMG.FirstPlay; p != nil {
		if err := checkPGC(p); err != nil {
			return err
		}
	}
	if err := checkUnits(d.VMG.Menus); err != nil {
		return err
	}
	for n, v := range d.VTS {
		if v == nil || n < 1 || n > maxTitleSets {
			return bad("the helper gave a disc with an impossible title set")
		}
		if len(v.Audio) > maxAudio || len(v.Subp) > maxSubtitles || len(v.MenuAudio) > maxAudio || len(v.MenuSubp) > maxSubtitles {
			return bad("the helper gave a title set with too many streams")
		}
		if err := checkUnits(v.Menus); err != nil {
			return err
		}
		if err := checkEntries(v.PGCs); err != nil {
			return err
		}
	}
	return nil
}

func checkPicture(r *wittydvd.RGBA) error {
	if r != nil && (r.X < -maxPosition || r.X > maxPosition || r.Y < -maxPosition || r.Y > maxPosition) {
		return bad("the helper gave a picture in an impossible place")
	}
	return nil // its size matches its pixels: wire.DecodeMenu sees to that
}

// checkMenu checks a menu picture. The frame's size is what a session allocates when
// it draws one.
func checkMenu(p *wittydvd.MenuPicture) error {
	if p.FrameW < 0 || p.FrameW > maxFrameSide || p.FrameH < 0 || p.FrameH > maxFrameSide {
		return bad("the helper gave a menu of an impossible size")
	}
	if len(p.Buttons) > maxButtons {
		return bad("the helper gave a menu with too many buttons")
	}
	if !seconds(p.Aspect) || !seconds(p.ButtonsFromSec) || !seconds(p.ButtonsUntilSec) || !seconds(p.ActionAtSec) {
		return bad("the helper gave a menu with an impossible time")
	}
	if err := checkPicture(p.Normal); err != nil {
		return err
	}
	for i := range p.Buttons {
		if err := checkPicture(p.Buttons[i].Select); err != nil {
			return err
		}
		if err := checkPicture(p.Buttons[i].Action); err != nil {
			return err
		}
	}
	return nil
}

// bounds says what a disc's video can be: the image itself, or a VOB in its folder.
// FFmpeg reads what the extents name, with the server's rights, so a helper mustn't be
// able to point it at anything else.
type bounds struct {
	image string // the disc image, or
	dir   string // the folder of a disc kept as one
	real  string // that folder, with links followed
	good  map[string]bool
}

func newBounds(path string) (*bounds, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	b := &bounds{good: map[string]bool{}}
	if !st.IsDir() {
		b.image = abs
		return b, nil
	}
	b.dir = abs
	if b.real, err = filepath.EvalSymlinks(abs); err != nil {
		return nil, err
	}
	return b, nil
}

// inFolder is whether rel, from filepath.Rel, stays inside. filepath.IsLocal refuses
// what would leave it, and on Windows the reserved names and alternate streams too.
func inFolder(rel string) bool { return rel != "." && filepath.IsLocal(rel) }

func (b *bounds) checkFile(f string) error {
	if b.good[f] {
		return nil
	}
	if f == "" || strings.ContainsFunc(f, unicode.IsControl) {
		return bad("the helper named an impossible file")
	}
	abs, err := filepath.Abs(f)
	if err != nil {
		return bad("the helper named an impossible file")
	}
	outside := bad("the helper pointed outside the disc, at %s", f)
	if b.image != "" {
		// filepath.Rel compares as the system does: without case on Windows.
		if rel, err := filepath.Rel(b.image, abs); err != nil || rel != "." {
			return outside
		}
	} else {
		rel, err := filepath.Rel(b.dir, abs)
		if err != nil || !inFolder(rel) || !strings.EqualFold(filepath.Ext(abs), ".vob") {
			return outside
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return outside
		}
		if rel, err = filepath.Rel(b.real, real); err != nil || !inFolder(rel) {
			return outside
		}
		if st, err := os.Stat(real); err != nil || !st.Mode().IsRegular() {
			return outside
		}
	}
	b.good[f] = true
	return nil
}

// check checks a list of extents before anything uses it.
func (b *bounds) check(ext []wittydvd.FileExtent) error {
	if len(ext) > maxExtents {
		return bad("the helper gave too many extents")
	}
	for _, e := range ext {
		if err := b.checkFile(e.File); err != nil {
			return err
		}
		if e.Offset < 0 || e.Length < 0 || e.Offset > maxOffset || e.Length > maxOffset {
			return bad("the helper gave an impossible extent")
		}
	}
	return nil
}

var errScrambled = errors.New("this disc is CSS-scrambled; decrypt it before adding it to the library")
