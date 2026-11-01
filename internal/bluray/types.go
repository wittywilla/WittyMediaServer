// Package bluray is what the server knows of a Blu-ray: the shapes WittyBluray answers in. WittyBluray
// itself is a module, reached through a helper process (internal/modules/bdremote). These travel as
// gob and JSON, which go by field name, so a field a newer version adds is simply not read here.

package bluray

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"sort"
)

// BDJObject is one .bdjo file.
type BDJObject struct {
	Name    string
	Version string
	// DefaultFont names a font in AUXDATA ("" = none). HAViConfig is the
	// graphics configuration the applications start with.
	DefaultFont     string
	HAViConfig      int
	MenuCallMask    bool
	TitleSearchMask bool
	Cache           []BDJCacheItem
	// Playlists the applications may play. AccessToAll allows every playlist
	// on the disc; AutostartFirst plays the first one as the title starts.
	Playlists      []int
	AccessToAll    bool
	AutostartFirst bool
	Apps           []BDJApp
	KeyInterest    uint32 // the 11 remote keys the title wants, top bit first
	// FileAccess lists the folders the applications may read ("." = all).
	FileAccess []string
}

// BDJCacheItem is a JAR file or folder to load before the title starts.
type BDJCacheItem struct {
	Dir  bool // a folder rather than a JAR
	Name string
	Lang string // "*.*" = always; "%%%" = when no language matches
}

// BDJApp is one application of the title.
type BDJApp struct {
	Control      int // 1 = starts by itself, 2 = another application starts it
	Type         int // 1 = BD-J
	Org          uint32
	ID           uint16
	Priority     int
	Binding      int
	Visibility   int
	Names        map[string]string // by language
	Icon         string
	BaseDir      string // the JAR file's name, without .jar
	ClassPath    string
	InitialClass string
	Params       []string
}

// Autostart lists the applications that start with the title.
func (o *BDJObject) Autostart() []*BDJApp {
	var out []*BDJApp
	for i := range o.Apps {
		if o.Apps[i].Control == 1 {
			out = append(out, &o.Apps[i])
		}
	}
	return out
}

// ticks45 formats a 45 kHz timestamp as seconds.
func ticks45(t uint32) float64 { return float64(t) / 45000 }

// Clip is one .clpi file.
type Clip struct {
	Name    string // five digits
	Version string
	// StreamType: 1 = a normal transport stream. ApplicationType: 1 = main
	// movie, 2 = time-based slideshow, 3 = browsable slideshow pictures, 4 =
	// browsable slideshow audio, 5 = IG menu, 6 = text subtitles, 7 = other
	// sub-path streams (secondary audio or video).
	StreamType, ApplicationType int
	TSRate                      int    // bytes per second of the transport stream
	Packets                     uint32 // source packets (192 bytes each)
	STC                         []STCSequence
	Programs                    []Program
	EPMaps                      []EPMap
}

// STCSequence is a stretch of the clip with one continuous clock.
type STCSequence struct {
	ID         int // the number play items name it by
	PCRPID     uint16
	StartSPN   uint32
	Start, End uint32 // presentation times, 45 kHz
}

// Program is one program of the transport stream and its streams.
type Program struct {
	StartSPN uint32
	PMTPID   uint16
	Streams  []ClipStream
}

// ClipStream is one elementary stream as the clip describes it.
type ClipStream struct {
	PID    uint16
	Coding byte
	Format int
	Rate   int
	Aspect int // video: 2 = 4:3, 3 = 16:9
	Lang   string
	// HEVC video: DynamicRange 0 = SDR, 1 = HDR10, 2 = Dolby Vision; ColorSpace 1 = BT.709, 2 = BT.2020.
	DynamicRange, ColorSpace int
}

// EPMap maps presentation times to source packet numbers for one stream.
type EPMap struct {
	PID     uint16
	Entries []EPEntry // in time order
}

// EPEntry is one entry point: a packet at which decoding can start.
type EPEntry struct {
	PTS uint32 // 45 kHz
	SPN uint32 // source packet number; byte offset = SPN * 192
	// AngleChange marks the packet as a point where a seamless angle change
	// may happen.
	AngleChange bool
}

// Stream finds a stream by PID.
func (c *Clip) Stream(pid uint16) *ClipStream {
	for i := range c.Programs {
		for j := range c.Programs[i].Streams {
			if c.Programs[i].Streams[j].PID == pid {
				return &c.Programs[i].Streams[j]
			}
		}
	}
	return nil
}

// VideoPID is the clip's main video stream, or 0.
func (c *Clip) VideoPID() uint16 {
	for _, p := range c.Programs {
		for _, s := range p.Streams {
			switch s.Coding {
			case 0x01, 0x02, 0x1b, 0x24, 0xea:
				return s.PID
			}
		}
	}
	return 0
}

// SPNAt returns the entry point at or before the time (45 kHz) in the
// stream, so a player can start reading there. ok is false when the clip has
// no entry-point map for that stream.
func (c *Clip) SPNAt(pid uint16, pts uint32) (e EPEntry, ok bool) {
	return c.EntryAt(pid, -1, pts)
}

// EntryAt is SPNAt within one time base. A clip with several (the clock
// restarts mid-file, as when studios join separately encoded reels) has the
// same times more than once, and its entries are not in time order overall,
// so a play item's time only means something inside its own time base. stc
// -1 searches the whole map. A stream with no map of its own uses the clip's
// first one (audio-only clips map their audio).
func (c *Clip) EntryAt(pid uint16, stc int, pts uint32) (e EPEntry, ok bool) {
	entries := c.epEntries(pid, stc)
	if len(entries) == 0 {
		return EPEntry{}, false
	}
	k := sort.Search(len(entries), func(j int) bool { return entries[j].PTS > pts })
	if k == 0 {
		return entries[0], true
	}
	return entries[k-1], true
}

// epEntries is a stream's entry points within one time base.
func (c *Clip) epEntries(pid uint16, stc int) []EPEntry {
	var m *EPMap
	for i := range c.EPMaps {
		if c.EPMaps[i].PID == pid && len(c.EPMaps[i].Entries) > 0 {
			m = &c.EPMaps[i]
			break
		}
	}
	if m == nil {
		for i := range c.EPMaps {
			if len(c.EPMaps[i].Entries) > 0 {
				m = &c.EPMaps[i]
				break
			}
		}
	}
	if m == nil {
		return nil
	}
	entries := m.Entries
	if from, to, found := c.STCRange(stc); found {
		lo := sort.Search(len(entries), func(j int) bool { return entries[j].SPN >= from })
		hi := sort.Search(len(entries), func(j int) bool { return entries[j].SPN >= to })
		if lo < hi {
			entries = entries[lo:hi]
		}
	}
	return entries
}

// EntryAfter is the first entry point after a time on a video stream, within
// that time base; ok is false when there is none.
func (c *Clip) EntryAfter(pid uint16, stc int, pts uint32) (e EPEntry, ok bool) {
	entries := c.epEntries(pid, stc)
	k := sort.Search(len(entries), func(j int) bool { return entries[j].PTS > pts })
	if k >= len(entries) {
		return EPEntry{}, false
	}
	return entries[k], true
}

// STCRange is the source packets of a time base; found is false when the
// clip has only one, or no such number.
func (c *Clip) STCRange(stc int) (from, to uint32, found bool) {
	if stc < 0 || len(c.STC) < 2 {
		return 0, 0, false
	}
	for i := range c.STC {
		if c.STC[i].ID == stc {
			to = c.Packets
			if i+1 < len(c.STC) {
				to = c.STC[i+1].StartSPN
			}
			return c.STC[i].StartSPN, to, true
		}
	}
	return 0, 0, false
}

// EndPTS is where a time base ends (45 kHz), or 0 when the clip does not say.
func (c *Clip) EndPTS(stc int) uint32 {
	for i := range c.STC {
		if c.STC[i].ID == stc {
			return c.STC[i].End
		}
	}
	if n := len(c.STC); n > 0 {
		return c.STC[n-1].End
	}
	return 0
}

// Palette is 256 colours. Entries a segment does not set are transparent.
type Palette [256]color.RGBA

// Object is one run-length-coded picture, as indexes into a palette.
type Object struct {
	ID            int
	Width, Height int
	Pixels        []byte // Width*Height palette indexes
}

// draw paints an object with a palette into img at (x, y), cropped to the
// object's crop rectangle when cw > 0.
func draw(img *image.NRGBA, o *Object, p *Palette, x, y, cx, cy, cw, ch int) {
	if cw <= 0 || ch <= 0 {
		cx, cy, cw, ch = 0, 0, o.Width, o.Height
	}
	b := img.Bounds()
	for j := 0; j < ch; j++ {
		sy := cy + j
		dy := y + j
		if sy < 0 || sy >= o.Height || dy < b.Min.Y || dy >= b.Max.Y {
			continue
		}
		for i := 0; i < cw; i++ {
			sx := cx + i
			dx := x + i
			if sx < 0 || sx >= o.Width || dx < b.Min.X || dx >= b.Max.X {
				continue
			}
			c := p[o.Pixels[sy*o.Width+sx]]
			if c.A == 0 {
				continue
			}
			k := img.PixOffset(dx, dy)
			if c.A == 255 {
				img.Pix[k], img.Pix[k+1], img.Pix[k+2], img.Pix[k+3] = c.R, c.G, c.B, 255
				continue
			}
			// Blend over what is there (straight alpha).
			a := uint32(c.A)
			ia := 255 - a
			da := uint32(img.Pix[k+3])
			outA := a + da*ia/255
			if outA == 0 {
				continue
			}
			for n, v := range [3]uint8{c.R, c.G, c.B} {
				img.Pix[k+n] = uint8((uint32(v)*a + uint32(img.Pix[k+n])*da*ia/255) / outA)
			}
			img.Pix[k+3] = uint8(outA)
		}
	}
}

// Window is a rectangle of the screen that compositions draw into.
type Window struct {
	ID         int
	X, Y, W, H int
}

// compObject places an object in a composition.
type compObject struct {
	object, window int
	forced         bool
	x, y           int
	cx, cy, cw, ch int // crop, when cw > 0
}

// Menu is one IG stream.
type Menu struct {
	Width, Height int
	// Popup menus (UIModel 1) stay hidden until the user's Popup key; always-on
	// menus (0) show at once.
	Popup bool
	// Timeout and SelectionTimeout are the stream times (seconds, 0 = none)
	// when the menu ends and when selection ends and the default activated
	// button runs. UserTimeout is how long a pop-up stays with no key pressed.
	Timeout, SelectionTimeout, UserTimeout float64
	Pages                                  []Page
	palettes                               map[int]Palette
	objects                                map[int]*Object
}

// Page is one menu page.
type Page struct {
	ID               int
	UOMask           uint64
	DefaultSelected  int // button id, or NoButton
	DefaultActivated int // a button that runs when the selection times out, or NoButton
	Palette          int
	// AnimationRate is the video frames per animation frame of the buttons'
	// state animations (0 = no animation).
	AnimationRate int
	BOGs          []BOG
	// In and Out are the page's transition effects, played as it opens and
	// closes.
	In, Out EffectSequence
}

// EffectSequence is a page transition: a list of timed compositions.
type EffectSequence struct {
	Windows []Window
	Effects []Effect
}

// Effect is one step of a transition: pictures placed in windows, for a time.
type Effect struct {
	DurationSec float64
	Palette     int
	Objects     []compObject
}

// NoButton means "none" in button references.
const NoButton = 0xffff

// BOG is a button overlap group: buttons that share a place on screen, of
// which one (the valid one) is shown at a time.
type BOG struct {
	DefaultValid int
	Buttons      []Button
}

// Button is one menu button.
type Button struct {
	ID             int
	NumericSelect  int // the number key that selects it, or NoButton
	AutoAction     bool
	X, Y           int
	Up, Down       int // neighbours by button id, NoButton when none
	Left, Right    int
	Normal         StateImages
	Selected       StateImages
	Activated      StateImages
	SelectedSound  int
	ActivatedSound int
	Commands       []Command
}

// StateImages is the range of pictures of one button state (an animation
// from Start to End; the player shows the first).
type StateImages struct {
	Start, End int // object ids, NoButton when none
	Repeat     bool
}

// ButtonState picks which picture of a button to draw.
type ButtonState int

const (
	ButtonNormal ButtonState = iota
	ButtonSelected
	ButtonActivated
)

// DrawEffect paints one step of a transition on a full frame.
func (m *Menu) DrawEffect(seq *EffectSequence, i int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, m.Width, m.Height))
	if i < 0 || i >= len(seq.Effects) {
		return img
	}
	e := &seq.Effects[i]
	p, ok := m.palettes[e.Palette]
	if !ok {
		return img
	}
	for _, ob := range e.Objects {
		if o := m.objects[ob.object]; o != nil {
			draw(img, o, &p, ob.x, ob.y, ob.cx, ob.cy, ob.cw, ob.ch)
		}
	}
	return img
}

// Page returns a page by id.
func (m *Menu) Page(id int) *Page {
	for i := range m.Pages {
		if m.Pages[i].ID == id {
			return &m.Pages[i]
		}
	}
	return nil
}

// Button finds a button on a page.
func (p *Page) Button(id int) *Button {
	for i := range p.BOGs {
		for j := range p.BOGs[i].Buttons {
			if p.BOGs[i].Buttons[j].ID == id {
				return &p.BOGs[i].Buttons[j]
			}
		}
	}
	return nil
}

// Visible lists the buttons shown on the page: each group's valid button,
// as the group defaults say, overridden by `valid` (button ids enabled by
// ENABLE_BUTTON) and `hidden` (disabled ones).
func (p *Page) Visible(enabled, disabled map[int]bool) []*Button {
	var out []*Button
	for i := range p.BOGs {
		g := &p.BOGs[i]
		pick := -1
		for j := range g.Buttons {
			b := &g.Buttons[j]
			if enabled[b.ID] && !disabled[b.ID] {
				pick = j
				break
			}
		}
		if pick < 0 {
			for j := range g.Buttons {
				if g.Buttons[j].ID == g.DefaultValid && !disabled[g.Buttons[j].ID] {
					pick = j
				}
			}
		}
		if pick >= 0 {
			out = append(out, &g.Buttons[pick])
		}
	}
	return out
}

// Rect is a button's place on screen, from its picture.
func (m *Menu) Rect(b *Button) image.Rectangle {
	for _, st := range []StateImages{b.Normal, b.Selected, b.Activated} {
		if o := m.objects[st.Start]; o != nil && st.Start != NoButton {
			return image.Rect(b.X, b.Y, b.X+o.Width, b.Y+o.Height)
		}
	}
	return image.Rect(b.X, b.Y, b.X, b.Y)
}

// Frames is how many pictures a button state animates through (0 = none).
func (st StateImages) Frames() int {
	if st.Start == NoButton || st.End < st.Start {
		return 0
	}
	if n := st.End - st.Start + 1; n <= 256 {
		return n
	}
	return 256
}

// Draw paints one button in a state into img (the menu's full frame): the
// first picture of the state's animation.
func (m *Menu) Draw(img *image.NRGBA, page *Page, b *Button, state ButtonState) bool {
	return m.DrawFrame(img, page, b, state, 0)
}

// DrawFrame paints picture `frame` of a button state's animation (frames
// wrap, so a looping highlight can be drawn for any frame number).
func (m *Menu) DrawFrame(img *image.NRGBA, page *Page, b *Button, state ButtonState, frame int) bool {
	st := b.Normal
	switch state {
	case ButtonSelected:
		if b.Selected.Start != NoButton {
			st = b.Selected
		}
	case ButtonActivated:
		if b.Activated.Start != NoButton {
			st = b.Activated
		} else if b.Selected.Start != NoButton {
			st = b.Selected
		}
	}
	n := st.Frames()
	if n == 0 {
		return false
	}
	o := m.objects[st.Start+frame%n]
	if o == nil {
		return false
	}
	p, ok := m.palettes[page.Palette]
	if !ok {
		return false
	}
	draw(img, o, &p, b.X, b.Y, 0, 0, 0, 0)
	return true
}

// Render draws a whole page with one button selected. Buttons are those
// Visible returns; the result is transparent where there are no buttons.
func (m *Menu) Render(page *Page, buttons []*Button, selected int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, m.Width, m.Height))
	for _, b := range buttons {
		state := ButtonNormal
		if b.ID == selected {
			state = ButtonSelected
		}
		m.Draw(img, page, b, state)
	}
	return img
}

// ObjectKind says what kind of program an index entry runs.
type ObjectKind int

const (
	ObjectNone ObjectKind = iota
	ObjectHDMV            // a movie object in MovieObject.bdmv
	ObjectBDJ             // a BD-J (Java) object
)

// IndexObject is one entry of the index: first play, top menu or a title.
type IndexObject struct {
	Kind        ObjectKind
	Interactive bool   // the object has a menu (an IG stream or BD-J UI) rather than just playing
	MovieObject int    // HDMV: the movie object to run
	BDJName     string // BD-J: the object's name (BDMV/BDJO/<name>.bdjo)
}

// Title is a disc title as the player's title list shows it.
type Title struct {
	Number int // 1-based, as JUMP_TITLE names them
	// Access says what the user may do while this title plays: 0 = anything,
	// 1 = no title search, 2 = no title search and no menu call.
	Access int
	Object IndexObject
}

// Index is BDMV/index.bdmv.
type Index struct {
	Version    string
	Stereo3D   bool // the disc prefers 3D output when the player can
	HasContent bool // the disc says it holds 3D content
	// DynamicRange is the initial dynamic range of an Ultra HD disc.
	DynamicRange int
	// VideoFormat and FrameRate are the disc's main video, as its menus
	// expect: 1 = 480i, 2 = 576i, 3 = 480p, 4 = 1080i, 5 = 720p, 6 = 1080p,
	// 7 = 576p; frame rates 1 = 23.976, 2 = 24, 3 = 25, 4 = 29.97, 6 = 50, 7 = 59.94.
	VideoFormat, FrameRate int
	FirstPlay, TopMenu     IndexObject
	Titles                 []Title
}

// Command is one 12-byte HDMV navigation command, as stored.
type Command struct {
	Op       uint32
	Dst, Src uint32
}

// MovieObject is one program.
type MovieObject struct {
	ResumeIntention bool // playback resumes in this object after a menu call
	MenuCallMask    bool // the user's menu key is ignored while it runs
	TitleSearchMask bool // the user's title search is ignored while it runs
	Commands        []Command
}

// Playlist is one .mpls file.
type Playlist struct {
	Number       int
	Version      string
	PlaybackType int // 1 = sequential, 2 = random, 3 = shuffle
	// PlaybackCount is how many items a random or shuffle playlist plays.
	PlaybackCount int
	UOMask        uint64 // user operations the disc forbids (see UO constants)
	RandomAccess  bool   // the user may not jump around (chapter search, time search) when false
	// AudioMix: the disc mixes button sounds and secondary audio into the
	// primary audio. LosslessBypass: it allows lossless output unmixed.
	// RightEyeBase: on a 3D playlist the base (2D) view is the right eye.
	AudioMix, LosslessBypass, RightEyeBase bool
	Items                                  []PlayItem
	SubPaths                               []SubPath
	Marks                                  []Mark
}

// PlayItem is one stretch of a clip.
type PlayItem struct {
	Clip  string // the clip's five-digit name, e.g. "00005"
	Codec string // "M2TS"
	// ConnectionCondition says how this item joins the previous one: 1 =
	// seamless (same clip continues), 5 = seamless across clips, 6 = a break
	// (the player may pause briefly).
	ConnectionCondition int
	STCID               int
	In, Out             uint32 // 45 kHz ticks into the clip
	UOMask              uint64
	RandomAccess        bool
	// StillMode: 0 = none, 1 = a still of StillTime seconds after the item,
	// 2 = a still until the user acts.
	StillMode int
	StillTime int
	// Angles lists the clips of a multi-angle item; the first is the main
	// angle (the same as Clip).
	Angles           []AngleClip
	SeamlessAngles   bool
	DifferentAudios  bool
	Video, Audio, PG []Stream
	IG, SecAudio     []Stream
	SecVideo, PiPPG  []Stream
	DV               []Stream // Dolby Vision enhancement layers
}

// Duration is the item's length in seconds.
func (p *PlayItem) Duration() float64 {
	if p.Out <= p.In {
		return 0
	}
	return ticks45(p.Out - p.In)
}

// AngleClip is one angle's clip.
type AngleClip struct {
	Clip  string
	Codec string
	STCID int
}

// Stream is one selectable stream of a play item.
type Stream struct {
	Kind string // video, audio, pg (subtitles), ig (menu), sec-audio, sec-video, pip-pg
	// EntryType says where the stream is: 1 = in this item's clip, 2 = in a
	// sub-path's clip, 3 = in a sub-path (text subtitles), 4 = in a sub-path
	// clip with its own PID.
	EntryType int
	PID       uint16
	SubPath   int
	SubClip   int
	Coding    byte // MPEG-TS stream type: 0x1b H.264, 0x24 HEVC, 0xea VC-1, 0x02 MPEG-2, 0x20 MVC; 0x80 LPCM, 0x81 AC-3, 0x82 DTS, 0x83 TrueHD, 0x84 E-AC-3, 0x85 DTS-HD, 0x86 DTS-HD MA; 0x90 PG, 0x91 IG, 0x92 text
	Format    int  // video: 1 480i 2 576i 3 480p 4 1080i 5 720p 6 1080p 7 576p 8 2160p; audio: 1 mono 3 stereo 6 multi-channel 12 stereo+multi
	Rate      int  // video frame rate (see Index.FrameRate); audio: 1 48 kHz, 4 96 kHz, 5 192 kHz, 12 48/192, 14 48/96
	Lang      string
	CharCode  int // text subtitles: character coding
	// UHD (HEVC) video: DynamicRange 0 = SDR, 1 = HDR10, 2 = Dolby Vision; ColorSpace 1 = BT.709, 2 = BT.2020; HDR10Plus when the stream carries HDR10+ metadata.
	DynamicRange int
	ColorSpace   int
	HDR10Plus    bool
}

// CodingName is a readable name for the stream's coding.
func (s Stream) CodingName() string {
	switch s.Coding {
	case 0x01:
		return "MPEG-1 video"
	case 0x02:
		return "MPEG-2 video"
	case 0x1b:
		return "H.264"
	case 0x20:
		return "H.264 MVC (3D)"
	case 0x24:
		return "HEVC"
	case 0xea:
		return "VC-1"
	case 0x03, 0x04:
		return "MPEG audio"
	case 0x80:
		return "LPCM"
	case 0x81:
		return "AC-3"
	case 0x82:
		return "DTS"
	case 0x83:
		return "TrueHD"
	case 0x84:
		return "E-AC-3"
	case 0x85:
		return "DTS-HD"
	case 0x86:
		return "DTS-HD Master Audio"
	case 0xa1:
		return "E-AC-3 (secondary)"
	case 0xa2:
		return "DTS-HD (secondary)"
	case 0x90:
		return "subtitles (PG)"
	case 0x91:
		return "menu graphics (IG)"
	case 0x92:
		return "text subtitles"
	}
	return fmt.Sprintf("type 0x%02x", s.Coding)
}

// SubPath is a sequence of sub play items with streams that play alongside
// the main path: out-of-mux menu graphics, secondary audio or video,
// picture-in-picture, text subtitles, or a 3D dependent view.
type SubPath struct {
	// Type: 2 = primary audio of a browsable slideshow, 3 = IG menu, 4 = text
	// subtitles, 5 = out-of-mux synchronous (secondary audio/video), 6 =
	// out-of-mux asynchronous PiP, 7 = in-mux synchronous PiP, 8 = 3D
	// dependent view (MVC), 9 = 3D dependent view with PiP.
	Type   int
	Repeat bool
	Items  []SubPlayItem
}

// SubPlayItem is one stretch of a clip in a sub-path.
type SubPlayItem struct {
	Clip                string
	Codec               string
	ConnectionCondition int
	STCID               int
	In, Out             uint32
	SyncItem            int    // the main path's play item this one is synchronised with
	SyncStart           uint32 // and the time in it (45 kHz)
	MultiClips          []AngleClip
}

// Mark is a chapter (Type 1, "entry mark") or a link point (Type 2).
type Mark struct {
	Type     int
	Item     int    // index into Items
	Time     uint32 // 45 kHz ticks into the clip, in the item's time base
	PID      uint16 // the stream it applies to, 0xffff for all
	Duration uint32
}

// User operation mask bits (UOMask). A set bit means the operation is
// forbidden while the playlist or item plays.
const (
	UOMenuCall      = 1 << 63
	UOTitleSearch   = 1 << 62
	UOChapterSearch = 1 << 61
	UOTimeSearch    = 1 << 60
	UOSkipToNext    = 1 << 59
	UOSkipBack      = 1 << 58
	UOPlay          = 1 << 57
	UOStop          = 1 << 56
	UOPauseOn       = 1 << 55
	UOPauseOff      = 1 << 54
	UOStillOff      = 1 << 53
	UOForward       = 1 << 52
	UOBackward      = 1 << 51
	UOResume        = 1 << 50
	UOMoveUp        = 1 << 49
	UOMoveDown      = 1 << 48
	UOMoveLeft      = 1 << 47
	UOMoveRight     = 1 << 46
	UOSelect        = 1 << 45
	UOActivate      = 1 << 44
	UOSelectActive  = 1 << 43
	UOPrimaryAudio  = 1 << 42
	UOAngle         = 1 << 40
	UOPopupOn       = 1 << 39
	UOPopupOff      = 1 << 38
	UOPGEnable      = 1 << 37
	UOPGChange      = 1 << 36
	UOSecVideoOn    = 1 << 35
	UOSecVideo      = 1 << 34
	UOSecAudioOn    = 1 << 33
	UOSecAudio      = 1 << 32
	UOPiPPG         = 1 << 30
)

// SubClip finds the clip a stream outside the main path is in: its sub-path
// and the numbered clip of that sub-path's first item.
func (pl *Playlist) SubClip(s Stream) string {
	if s.SubPath >= len(pl.SubPaths) || len(pl.SubPaths[s.SubPath].Items) == 0 {
		return ""
	}
	it := &pl.SubPaths[s.SubPath].Items[0]
	if s.SubClip >= 1 && s.SubClip <= len(it.MultiClips) {
		return it.MultiClips[s.SubClip-1].Clip
	}
	return it.Clip
}

// StepKind is what the player should do.
type StepKind string

const (
	StepPlay        StepKind = "play"
	StepStop        StepKind = "stop"
	StepUnsupported StepKind = "unsupported" // a BD-J object (Step.BDJ names it): run it if the player can, else fall back to the main feature
	StepStay        StepKind = "stay"        // nothing changes (a button only changed menu state)
)

// Step is the navigator's instruction to the player.
type Step struct {
	Kind     StepKind
	Reason   string
	Playlist *Playlist
	Item     int    // where to start: the play item index
	Time     uint32 // and the time in it (45 kHz ticks, in the clip's time base); 0 = the item's IN
	// ItemOnly: play just this item, then report PlaylistEnded; the navigator
	// says which item comes next. Set for random and shuffle playlists, items
	// with a still, and playlists whose items do not share one set of streams.
	ItemOnly bool
	// StillSec is how long the picture holds after what this step plays: 0 =
	// not at all, -1 = until the viewer acts.
	StillSec float64
	// Menu is set when the playlist has an IG (menu graphics) stream: its
	// PID, and the clip it comes from when it is in a sub-path.
	Menu    bool
	IGPID   uint16
	IGClip  string // "" when the IG stream is in the play item's own clip
	Title   int    // PSR4 at this step (0xffff during first play)
	Object  int    // the movie object that asked for this
	Effects *Effects
	// BDJ names the BD-J object of a StepUnsupported step.
	BDJ string
}

// NavOptions configure the player the navigator simulates.
type NavOptions struct {
	Registers RegisterOptions
}

// FileExtent is a byte range of a stream file: inside the image, or a file
// on disk when the disc is a folder.
type FileExtent struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
}

// Subtitle is one subtitle picture and when it shows.
type Subtitle struct {
	Start, End uint32 // 45 kHz, in the clip's time base; End 0 = until the next
	X, Y       int    // on the video frame
	Image      *image.NRGBA
	Forced     bool // the disc wants it shown even with subtitles off (a translation of on-screen text)
}

// SubtitleWindow is what Subtitles returns: the pictures and the frame size
// they are placed on.
type SubtitleWindow struct {
	Width, Height int
	Subtitles     []Subtitle
}

// Sounds is a disc's sound.bdmv.
type Sounds struct {
	Clips []SoundClip
	data  []byte
}

// SoundClip is one sound, by its place in the file (the id buttons name).
type SoundClip struct {
	Channels int
	Rate     int
	offset   int
	length   int
}

// NoSound is the button sound id meaning "none".
const NoSound = 0xff

// WAV returns one clip as a WAV file, which every browser plays.
func (s *Sounds) WAV(id int) ([]byte, bool) {
	if id < 0 || id >= len(s.Clips) || s.Clips[id].length == 0 {
		return nil, false
	}
	c := s.Clips[id]
	pcm := s.data[c.offset : c.offset+c.length&^1]
	out := make([]byte, 44+len(pcm))
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(36+len(pcm)))
	copy(out[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(out[16:], 16)
	binary.LittleEndian.PutUint16(out[20:], 1)
	binary.LittleEndian.PutUint16(out[22:], uint16(c.Channels))
	binary.LittleEndian.PutUint32(out[24:], uint32(c.Rate))
	binary.LittleEndian.PutUint32(out[28:], uint32(c.Rate*c.Channels*2))
	binary.LittleEndian.PutUint16(out[32:], uint16(c.Channels*2))
	binary.LittleEndian.PutUint16(out[34:], 16)
	copy(out[36:], "data")
	binary.LittleEndian.PutUint32(out[40:], uint32(len(pcm)))
	for i := 0; i+1 < len(pcm); i += 2 {
		out[44+i], out[44+i+1] = pcm[i+1], pcm[i]
	}
	return out, true
}

// TextStyle is a region style of the dialog style segment.
type TextStyle struct {
	ID            int
	X, Y, W, H    int // the region on the video frame
	Background    color.RGBA
	BoxX, BoxY    int // the text box, relative to the region
	BoxW, BoxH    int
	Flow          int // 1 = left to right, 2 = right to left, 3 = top to bottom
	HAlign        int // 1 = left, 2 = centre, 3 = right
	VAlign        int // 1 = top, 2 = middle, 3 = bottom
	LineSpace     int
	Bold, Italic  bool
	Outline       bool
	FontSize      int
	Color         color.RGBA
	OutlineColor  color.RGBA
	OutlineWeight int
}

// TextCue is one dialog: its words, when, and in what style.
type TextCue struct {
	Start, End uint32 // 45 kHz, in the clip's time base
	Forced     bool
	Style      TextStyle
	Text       string // lines separated by \n
}

// Registers: 4096 general-purpose registers the disc may set freely, and
// 128 player status registers (PSRs) that describe the player and the
// playback state, which the disc reads and the player and SET_SYSTEM
// commands write.
type Registers struct {
	GPR [4096]uint32
	PSR [128]uint32
	// Rand returns a number in [0,1) for RND; tests and diagnostic tools set
	// it to make a run repeatable. nil uses the system generator.
	Rand func() float64
}

// Player status registers the engine uses.
const (
	PSRIGStream       = 0  // selected IG (menu) stream, 1-based
	PSRAudio          = 1  // selected primary audio stream, 1-based (0xff = none chosen yet)
	PSRPG             = 2  // bits 0..11: PG/text subtitle stream, 1-based (0xfff = none); bits 16..27: PiP subtitle stream; bit 31: subtitles shown; bit 30: PiP subtitles shown
	PSRAngle          = 3  // selected angle, 1-based
	PSRTitle          = 4  // current title, 1-based (0xffff while first play runs)
	PSRChapter        = 5  // current chapter, 1-based (0xffff when unknown)
	PSRPlaylist       = 6  // current playlist number
	PSRPlayItem       = 7  // current play item
	PSRTime           = 8  // presentation time, 45 kHz
	PSRNavTimer       = 9  // navigation timer, seconds
	PSRButton         = 10 // selected button id (0xffff = none)
	PSRPage           = 11 // current menu page id
	PSRStyle          = 12 // user style number
	PSRParental       = 13 // parental level (the viewer's age; 255 = no limit)
	PSRSecondary      = 14 // bits 0..7: secondary audio; bits 8..15: secondary video; bits 30, 31: shown
	PSRAudioCap       = 15 // player audio capability
	PSRAudioLang      = 16 // preferred audio language, three letters packed
	PSRPGLang         = 17 // preferred subtitle language
	PSRMenuLang       = 18 // preferred menu language
	PSRCountry        = 19 // country code
	PSRRegion         = 20 // region: 1 = A, 2 = B, 4 = C
	PSROutputPref     = 21 // bit 0: the user prefers 3D output
	PSRStereoStatus   = 22 // bit 0: 3D output is on now
	PSRDisplayCap     = 23 // the display's 3D abilities and width
	PSR3DCap          = 24
	PSRUHDCap         = 25
	PSRUHDDisplayCap  = 26
	PSRHDRPref        = 27
	PSRSDRConv        = 28
	PSRVideoCap       = 29 // bit 0: secondary video in HD; bit 1: 25 and 50 Hz video
	PSRTextCap        = 30 // text subtitle abilities
	PSRProfile        = 31 // player profile (bits 16..23) and version (bits 0..15)
	PSRBackupTitle    = 36
	PSRBackupChapter  = 37
	PSRBackupPlaylist = 38
	PSRBackupItem     = 39
	PSRBackupTime     = 40
	PSRBackupButton   = 42
	PSRBackupPage     = 43
	PSRBackupStyle    = 44
)

// Player profiles as PSR31 carries them. A disc's program reads this to
// decide what to offer: 3D, BD-Live, secondary video.
const (
	Profile1v10 = 0x00000100 // the first players
	Profile1v11 = 0x00010110 // Bonus View: secondary streams and local storage
	Profile2v20 = 0x00030200 // BD-Live: network access
	Profile3v20 = 0x00080200 // audio-only players
	Profile5v24 = 0x00130240 // 3D
	Profile6v30 = 0x00000300 // Ultra HD
)

// RegisterOptions set the player's identity: languages, region, 3D.
type RegisterOptions struct {
	AudioLang, SubtitleLang, MenuLang string // three-letter codes, e.g. "eng"
	Country                           string // two letters, e.g. "US"
	Region                            int    // 1 = A, 2 = B, 4 = C; 0 = A
	Parental                          int    // 0 = no limit
	Stereo3D                          bool   // the display can show 3D
	UHD                               bool   // an Ultra HD player (for Ultra HD discs, which ask)
}

// NewRegisters makes a player's register file for the options. Registers
// nobody has chosen yet start at the "none" values the format defines: a disc
// sees 0xff for the audio stream until a stream is selected, and its program
// can tell a first visit to a menu from a later one that way.
func NewRegisters(o RegisterOptions) *Registers {
	r := &Registers{}
	r.PSR[PSRIGStream] = 1
	r.PSR[PSRAudio] = 0xff
	r.PSR[PSRPG] = 0x0fff0fff
	r.PSR[PSRAngle] = 1
	r.PSR[PSRTitle] = 0xffff
	r.PSR[PSRChapter] = 0xffff
	r.PSR[PSRButton] = 0xffff
	r.PSR[PSRStyle] = 0xff
	r.PSR[PSRSecondary] = 0xffff
	r.PSR[PSRBackupTitle], r.PSR[PSRBackupChapter] = 0xffff, 0xffff
	r.PSR[PSRBackupButton], r.PSR[PSRBackupStyle] = 0xffff, 0xff
	r.PSR[PSRAudioLang] = packLang(orDefault(o.AudioLang, "eng"))
	r.PSR[PSRPGLang] = packLang(orDefault(o.SubtitleLang, "eng"))
	r.PSR[PSRMenuLang] = packLang(orDefault(o.MenuLang, "eng"))
	c := orDefault(o.Country, "US")
	if len(c) < 2 {
		c = "US"
	}
	r.PSR[PSRCountry] = uint32(c[0])<<8 | uint32(c[1])
	r.PSR[PSRRegion] = uint32(orDefaultInt(o.Region, 1))
	r.PSR[PSRParental] = uint32(orDefaultInt(o.Parental, 255))
	// Every disc audio format can be converted, so all count as surround-capable.
	r.PSR[PSRAudioCap] = 0xaaaa
	r.PSR[PSRVideoCap] = 0x03
	r.PSR[PSRTextCap] = 0x1ffff
	r.PSR[PSRProfile] = Profile2v20
	for i := 48; i <= 61; i++ {
		r.PSR[i] = 0xffffffff // characteristic text: everything can be shown
	}
	switch {
	case o.UHD:
		r.PSR[PSRUHDCap], r.PSR[PSRUHDDisplayCap], r.PSR[PSRHDRPref] = 0xffffffff, 0xffffffff, 0xffffffff
		r.PSR[PSRProfile] = Profile6v30
	case o.Stereo3D:
		r.PSR[PSROutputPref] = 1
		r.PSR[PSRDisplayCap] = 0x0f
		r.PSR[PSR3DCap] = 0xffffffff
		r.PSR[PSRProfile] = Profile5v24
	}
	return r
}

func packLang(s string) uint32 {
	if len(s) < 3 {
		return 0
	}
	return uint32(s[0])<<16 | uint32(s[1])<<8 | uint32(s[2])
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func orDefaultInt(v, d int) int {
	if v == 0 {
		return d
	}
	return v
}

// Group is a command's group.
type Group int

const (
	GroupBranch Group = iota
	GroupCompare
	GroupSet
)

// Instr is a decoded command.
type Instr struct {
	Raw      Command
	Group    Group
	Sub      int // branch: 0 goto, 1 jump, 2 play; set: 0 set, 1 set system
	Opt      int // branch option, compare option or set option
	Operands int
	ImmDst   bool
	ImmSrc   bool
	Dst, Src uint32
}

// Effects collects what SET_SYSTEM commands asked for during a run; the
// player applies them.
type Effects struct {
	NavTimer      *NavTimer
	ButtonPage    *ButtonPage
	EnableButton  []uint32
	DisableButton []uint32
	PopupOff      bool
	Still         *bool
	OutputMode3D  *bool
	StreamChange  bool // audio, subtitle, IG or angle registers changed
	// NavTimerOff: SET_NV_TIMER with a time of 0 cancelled the timer.
	NavTimerOff bool
}

// NavTimer is SET_NV_TIMER: run a movie object after some seconds (at most
// 300).
type NavTimer struct {
	Seconds uint32
	Object  int
}

// ButtonPage is SET_BUTTON_PAGE: change the menu's page and/or button.
type ButtonPage struct {
	Button, Page  uint32
	SetButton     bool
	SetPage       bool
	PageEffectOff bool
}
