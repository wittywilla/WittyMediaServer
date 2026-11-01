package bluray

// What a WittyBluray helper process and a server say to each other about one open disc. A copy of
// the module's side of the same contract.

// Params is every call's parameters; each method reads the ones it needs.
type Params struct {
	Path     string     `json:"path,omitempty"`
	Options  NavOptions `json:"options"`
	N        int        `json:"n,omitempty"`
	Name     string     `json:"name,omitempty"`
	Item     int        `json:"item,omitempty"`
	Time     uint32     `json:"time,omitempty"`
	ItemOnly bool       `json:"itemOnly,omitempty"`
	Shown    bool       `json:"shown,omitempty"`
	PID      uint16     `json:"pid,omitempty"`
	Limit    uint32     `json:"limit,omitempty"`
	STC      int        `json:"stc,omitempty"`
	From     uint32     `json:"from,omitempty"`
	To       uint32     `json:"to,omitempty"`
	CharCode int        `json:"charCode,omitempty"`
	Only     []uint16   `json:"only,omitempty"`
	UO       uint64     `json:"uo,omitempty"`
	Page     int        `json:"page,omitempty"`
	Button   int        `json:"button,omitempty"`
	Commands []Command  `json:"commands,omitempty"`
}

// Opened is the answer to "open".
type Opened struct {
	Encrypted bool        `json:"encrypted"`
	Stereo3D  bool        `json:"stereo3d"`
	PSR       [128]uint32 `json:"psr"`
	Label     string      `json:"label,omitempty"`
	Index     *Index      `json:"index,omitempty"`
	BDJ       bool        `json:"bdj,omitempty"`
	Playlists []int       `json:"playlists,omitempty"`
}

// Lead is the answer to "cliplead": see Disc.ClipLead.
type Lead struct {
	Seconds float64
}

// Main is the answer to "main": the number of the feature's playlist.
type Main struct {
	N  int  `json:"n"`
	OK bool `json:"ok"`
}

// Answer is what a call to the navigator comes back with. PSR and the timer are
// what the session reads without asking, so every answer brings them up to date.
// General registers stay here: only the Java side reads those, and it isn't run
// through a helper.
type Answer struct {
	PSR [128]uint32 `json:"psr"`
	// TimerSeq changes when the disc starts a timer, even one the same as before:
	// the session restarts its clock when the timer is a new one.
	Timer    *NavTimer `json:"timer,omitempty"`
	TimerSeq uint64    `json:"timerSeq"`
	// Step is without its playlist, which Playlist names: the server keeps each
	// playlist once, and a session compares them as pointers.
	Step     *Step `json:"step,omitempty"`
	Playlist *int  `json:"playlist,omitempty"`
	OK       bool  `json:"ok,omitempty"`
}

// What a helper says about a disc's Java programs, unasked, as they run.
const (
	NotifyJavaEvent  = "java.event"  // JavaEvent: something the applications asked for
	NotifyJavaChange = "java.change" // JavaChange: a new picture
	NotifyJavaLog    = "java.log"    // a line they printed
	NotifyJavaExit   = "java.exit"   // JavaExit: the process has ended
)

// JavaStart starts Java for this disc; Session names its working folder, and Java is the
// program to use ("" finds it).
type JavaStart struct {
	Java    string `json:"java,omitempty"`
	Session string `json:"session,omitempty"`
}

// JavaLine is one line for the programs.
type JavaLine struct {
	Line string `json:"line"`
}

// JavaChange says which picture is the newest.
type JavaChange struct {
	Version int `json:"version"`
}

// JavaExit is how the process ended.
type JavaExit struct {
	Error   string `json:"error,omitempty"`
	Stopped bool   `json:"stopped,omitempty"`
}

// JavaProbe is whether this machine's Java can run a disc's programs.
type JavaProbe struct {
	CantRun string `json:"cantRun,omitempty"` // Java was found and tried, and failed: why
	Version string `json:"version"`
	Path    string `json:"path"`
}

// PlaylistDuration is how long a playlist plays.
func PlaylistDuration(pl *Playlist) float64 {
	var t float64
	for i := range pl.Items {
		t += pl.Items[i].Duration()
	}
	return t
}
