// Package wire is what a WittyDVD helper process and a server say to each other
// about one open disc: the names of the calls and the form their answers travel in.
//
// Structured answers go as encoding/gob in the protocol's blob. It needs no
// hand-written copy of the engine's types, carries +Inf still times and fields a JSON
// form leaves out, and a field added by a newer version is simply ignored by an older
// one. Pictures and sub-picture bytes travel raw after a small header: they are most
// of the bytes, and gob would only copy them.
//
// The server decodes what may be a fork's answers, and gob isn't hardened against a
// stranger's bytes: a little input can become a good deal more memory. So every Decode
// here has a low limit on the gob it will read, and pictures and sub-pictures are only
// ever slices of the bytes that really arrived. What the values mean is checked by the server.
package wire

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"slices"

	"github.com/wittywilla/wittydvd"
)

// The calls.
const (
	MethodOpen  = "open"  // OpenParams; answers the disc's structure
	MethodNav   = "nav"   // NavOptions; starts the navigator
	MethodMenu  = "menu"  // MenuCall; answers a menu picture
	MethodRange = "range" // RangeCall; answers file extents
	MethodCell  = "cell"  // CellCall; answers file extents
	MethodSubs  = "subs"  // CellCall; answers a cell's sub-pictures
	MethodRuns  = "runs"  // RunsCall; answers a title's runs

	// Navigator calls take a NavCall and answer a NavAnswer.
	MethodStart     = "start"
	MethodPlayTitle = "playTitle" // N title, M chapter
	MethodOpenMenu  = "openMenu"  // Kind, N title set
	MethodSelect    = "select"    // N button
	MethodPress     = "press"     // N button, Cmd
	MethodMenuKey   = "menuKey"   // Kind
	MethodResume    = "resume"
	MethodNext      = "next"
	MethodPrev      = "prev"
	MethodGoUp      = "goUp"
	MethodCellEnded = "cellEnded"
	MethodTick      = "tick" // Sec
	MethodCurrent   = "current"
	MethodSetAudio  = "setAudio" // N stream
	MethodSetSubs   = "setSubs"  // N stream, On
	MethodSetAngle  = "setAngle" // N angle
	MethodSyncCell  = "syncCell" // N program chain, M cell
	MethodClone     = "clone"    // the what-if copy, replacing any before it
	MethodStepPGC   = "stepPGC"  // Domain, N title set, M program chain; answers PGCAnswer
)

// The codes of refusals the server acts on.
const (
	CodeDenied      = "denied" // the helper's limits may be why it can't read the disc
	CodeNoNav       = "no-nav" // wittydvd.ErrNoNav
	CodeBadCall     = "bad-call"
	CodeUnknownCall = "unknown-method"
)

// What the server reads before it looks at an answer. They are far above what a real
// disc needs; beyond them the answer is a helper's fault, and the built-in copy takes over.
const (
	maxMenuHeader    = 256 << 10
	maxStepAnswer    = 256 << 10
	maxPGCAnswer     = 1 << 20
	maxExtentsAnswer = 8 << 20
	maxDiscAnswer    = 16 << 20
	maxPictureSide   = 4096
)

// OpenParams opens a disc.
type OpenParams struct {
	Path string `json:"path"`
}

// NavOptions are the viewer's settings for a navigator. It is
// wittydvd.RegisterOptions without what can't leave the process.
type NavOptions struct {
	MenuLanguage     string `json:"menuLanguage,omitempty"`
	AudioLanguage    string `json:"audioLanguage,omitempty"`
	SubtitleLanguage string `json:"subtitleLanguage,omitempty"`
	Region           int    `json:"region,omitempty"`
	Parental         int    `json:"parental,omitempty"`
	Country          string `json:"country,omitempty"`
	Angle            int    `json:"angle,omitempty"`
	// Seed makes what a disc picks at random repeatable, for comparing two plays of
	// it. A session never sets it.
	Seed int64 `json:"seed,omitempty"`
}

// MenuCall asks for a menu scan.
type MenuCall struct {
	Space   wittydvd.Space           `json:"space"`
	First   uint32                   `json:"first"`
	Last    uint32                   `json:"last"`
	Options wittydvd.MenuScanOptions `json:"options"`
}

// RangeCall asks where a run of sectors is.
type RangeCall struct {
	Space wittydvd.Space `json:"space"`
	First uint32         `json:"first"`
	Last  uint32         `json:"last"`
}

// CellCall asks about one cell.
type CellCall struct {
	Space wittydvd.Space `json:"space"`
	Cell  wittydvd.Cell  `json:"cell"`
}

// NavCall is one call to a navigator; Clone sends it to the what-if copy.
type NavCall struct {
	Clone  bool              `json:"clone,omitempty"`
	N      int               `json:"n,omitempty"`
	M      int               `json:"m,omitempty"`
	On     bool              `json:"on,omitempty"`
	Kind   string            `json:"kind,omitempty"`
	Sec    float64           `json:"sec,omitempty"`
	Cmd    *wittydvd.Command `json:"cmd,omitempty"`
	Domain wittydvd.Domain   `json:"domain,omitempty"`
}

// NavAnswer is a navigator's state after a call. The registers come along every time
// so the server, which reads them often, can do so without asking.
type NavAnswer struct {
	Step   *wittydvd.Step
	Regs   wittydvd.Registers
	Synced bool
}

// PGCAnswer is the program chain a step plays; nil when there is none.
type PGCAnswer struct{ PGC *wittydvd.PGC }

// Encode is the wire form of a structured answer.
func Encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decode(b []byte, limit int, what string, v any) (err error) {
	if len(b) > limit {
		return fmt.Errorf("the helper's %s is too large: %d bytes", what, len(b))
	}
	// gob is not hardened against a stranger's bytes, and this must not bring the
	// server down whatever arrives.
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("the helper's %s is unreadable: %v", what, p)
		}
	}()
	if err := gob.NewDecoder(bytes.NewReader(b)).Decode(v); err != nil {
		return fmt.Errorf("the helper's %s is unreadable: %w", what, err)
	}
	return nil
}

// DecodeNav reads a NavAnswer.
func DecodeNav(b []byte) (*NavAnswer, error) {
	var a NavAnswer
	if err := decode(b, maxStepAnswer, "step", &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// DecodeExtents reads a list of file extents.
// RunsCall asks how a title's cells are read: see wittydvd.Opened.TitleRuns.
type RunsCall struct {
	Space wittydvd.Space       `json:"space"`
	Cells []wittydvd.TitleCell `json:"cells"`
}

// DecodeRuns reads a title's runs.
func DecodeRuns(b []byte) ([]wittydvd.TitleRun, error) {
	var r []wittydvd.TitleRun
	if err := decode(b, maxExtentsAnswer, "list of runs", &r); err != nil {
		return nil, err
	}
	return r, nil
}

func DecodeExtents(b []byte) ([]wittydvd.FileExtent, error) {
	var e []wittydvd.FileExtent
	if err := decode(b, maxExtentsAnswer, "list of extents", &e); err != nil {
		return nil, err
	}
	return e, nil
}

// A program chain's sub-picture table has a nil for each stream the chain doesn't
// have, and gob refuses a nil inside an array. So for the journey a missing stream is
// this value, which no disc can hold: the parser's stream numbers are 0 to 31.
var absentStream = wittydvd.SubpControl{Narrow: -1, Wide: -1, Letterbox: -1, PanScan: -1}

func markAbsent(p *wittydvd.PGC) {
	for i, c := range p.SubpControl {
		if c == nil {
			p.SubpControl[i] = &absentStream
		}
	}
}

func unmarkAbsent(p *wittydvd.PGC) {
	for i, c := range p.SubpControl {
		if c != nil && *c == absentStream {
			p.SubpControl[i] = nil
		}
	}
}

// eachPGC visits every program chain of a disc.
func eachPGC(d *wittydvd.Disc, f func(*wittydvd.PGC)) {
	do := func(p *wittydvd.PGC) {
		if p != nil {
			f(p)
		}
	}
	entries := func(es []wittydvd.PGCEntry) {
		for _, e := range es {
			do(e.PGC)
		}
	}
	units := func(us []wittydvd.LangUnit) {
		for _, u := range us {
			entries(u.PGCs)
		}
	}
	if d.VMG != nil {
		do(d.VMG.FirstPlay)
		units(d.VMG.Menus)
	}
	for _, v := range d.VTS {
		if v != nil {
			units(v.Menus)
			entries(v.PGCs)
		}
	}
}

// EncodeDisc is the wire form of a disc's structure. The disc is back as it was
// afterwards; a helper answers one call at a time, so nothing sees it marked.
func EncodeDisc(d *wittydvd.Disc) ([]byte, error) {
	eachPGC(d, markAbsent)
	defer eachPGC(d, unmarkAbsent)
	return Encode(d)
}

// DecodeDisc reads a disc's structure.
func DecodeDisc(b []byte) (*wittydvd.Disc, error) {
	var d wittydvd.Disc
	if err := decode(b, maxDiscAnswer, "disc structure", &d); err != nil {
		return nil, err
	}
	eachPGC(&d, unmarkAbsent)
	return &d, nil
}

// EncodePGC is the wire form of the answer to MethodStepPGC.
func EncodePGC(p *wittydvd.PGC) ([]byte, error) {
	if p != nil {
		markAbsent(p)
		defer unmarkAbsent(p)
	}
	return Encode(PGCAnswer{p})
}

// DecodePGC reads the answer to MethodStepPGC.
func DecodePGC(b []byte) (*wittydvd.PGC, error) {
	var a PGCAnswer
	if err := decode(b, maxPGCAnswer, "program chain", &a); err != nil {
		return nil, err
	}
	if a.PGC != nil {
		unmarkAbsent(a.PGC)
	}
	return a.PGC, nil
}

// EncodeMenu is the wire form of a menu picture: the length of a header, the header,
// and then the pixels of each picture in turn.
func EncodeMenu(p *wittydvd.MenuPicture) ([]byte, error) {
	c := *p
	c.Buttons = slices.Clone(p.Buttons)
	var pix [][]byte
	var bad error
	strip := func(r **wittydvd.RGBA) {
		if *r == nil {
			return
		}
		x := **r
		if x.Width < 0 || x.Height < 0 || len(x.Pix) != x.Width*x.Height*4 {
			bad = errors.New("a picture's size doesn't match its pixels")
		}
		pix = append(pix, x.Pix)
		x.Pix = nil
		*r = &x
	}
	strip(&c.Normal)
	for i := range c.Buttons {
		strip(&c.Buttons[i].Select)
		strip(&c.Buttons[i].Action)
	}
	if bad != nil {
		return nil, bad
	}
	head, err := Encode(&c)
	if err != nil {
		return nil, err
	}
	total := 4 + len(head)
	for _, b := range pix {
		total += len(b)
	}
	out := binary.BigEndian.AppendUint32(make([]byte, 0, total), uint32(len(head)))
	out = append(out, head...)
	for _, b := range pix {
		out = append(out, b...)
	}
	return out, nil
}

// DecodeMenu reads a menu picture. Its pixels are slices of b, each exactly the size
// its picture says.
func DecodeMenu(b []byte) (*wittydvd.MenuPicture, error) {
	if len(b) < 4 {
		return nil, errors.New("the helper's menu picture is cut short")
	}
	n := uint64(binary.BigEndian.Uint32(b))
	if n > maxMenuHeader || n > uint64(len(b)-4) {
		return nil, errors.New("the helper's menu picture has an impossible header")
	}
	var p wittydvd.MenuPicture
	if err := decode(b[4:4+n], maxMenuHeader, "menu picture", &p); err != nil {
		return nil, err
	}
	rest := b[4+n:]
	take := func(r *wittydvd.RGBA) error {
		if r == nil {
			return nil
		}
		if r.Width < 0 || r.Height < 0 || r.Width > maxPictureSide || r.Height > maxPictureSide {
			return errors.New("the helper's menu picture has an impossible size")
		}
		size := r.Width * r.Height * 4
		if size > len(rest) {
			return errors.New("the helper's menu picture is cut short")
		}
		r.Pix, rest = rest[:size:size], rest[size:]
		return nil
	}
	if err := take(p.Normal); err != nil {
		return nil, err
	}
	for i := range p.Buttons {
		if err := take(p.Buttons[i].Select); err != nil {
			return nil, err
		}
		if err := take(p.Buttons[i].Action); err != nil {
			return nil, err
		}
	}
	if len(rest) != 0 {
		return nil, errors.New("the helper's menu picture has bytes left over")
	}
	return &p, nil
}

// The fixed part of a cell's sub-pictures: its start time, and for each unit its
// stream, sector, time and size. A long cell has tens of thousands of units, so they
// are fixed records rather than gob, which could turn one byte into a whole unit.
const (
	subsHead = 8
	subsUnit = 20
)

// EncodeSubs is the wire form of a cell's sub-pictures.
func EncodeSubs(c *wittydvd.CellSubs) []byte {
	total := subsHead + len(c.Units)*subsUnit
	for _, u := range c.Units {
		total += len(u.Bytes)
	}
	out := make([]byte, 0, total)
	out = binary.BigEndian.AppendUint32(out, c.BasePTM)
	out = binary.BigEndian.AppendUint32(out, uint32(len(c.Units)))
	for _, u := range c.Units {
		out = binary.BigEndian.AppendUint32(out, uint32(u.Stream))
		out = binary.BigEndian.AppendUint32(out, u.Sector)
		out = binary.BigEndian.AppendUint64(out, uint64(u.PTS))
		out = binary.BigEndian.AppendUint32(out, uint32(len(u.Bytes)))
	}
	for _, u := range c.Units {
		out = append(out, u.Bytes...)
	}
	return out
}

// DecodeSubs reads a cell's sub-pictures. Their bytes are slices of b.
func DecodeSubs(b []byte) (*wittydvd.CellSubs, error) {
	if len(b) < subsHead {
		return nil, errors.New("the helper's sub-pictures are cut short")
	}
	n := uint64(binary.BigEndian.Uint32(b[4:]))
	if n*subsUnit > uint64(len(b)-subsHead) {
		return nil, errors.New("the helper's sub-pictures claim more than arrived")
	}
	c := &wittydvd.CellSubs{BasePTM: binary.BigEndian.Uint32(b), Units: make([]wittydvd.SPUUnit, n)}
	rest := b[subsHead+n*subsUnit:]
	for i := range c.Units {
		r := b[subsHead+uint64(i)*subsUnit:]
		size := uint64(binary.BigEndian.Uint32(r[16:]))
		if size > uint64(len(rest)) {
			return nil, errors.New("the helper's sub-pictures are cut short")
		}
		c.Units[i] = wittydvd.SPUUnit{Stream: int(int32(binary.BigEndian.Uint32(r))), Sector: binary.BigEndian.Uint32(r[4:]),
			PTS: int64(binary.BigEndian.Uint64(r[8:])), Bytes: rest[:size:size]}
		rest = rest[size:]
	}
	if len(rest) != 0 {
		return nil, errors.New("the helper's sub-pictures have bytes left over")
	}
	return c, nil
}
