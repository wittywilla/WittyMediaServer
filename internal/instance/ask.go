package instance

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/desktop"
)

// Choice is what the person chose when their server was already running.
type Choice int

const (
	ChoiceOpen    Choice = iota // open the running one in the browser (the default)
	ChoiceAnother               // start a second server, on its own folder and port
	ChoiceExit
)

// Ask shows a question with buttons and returns the index of the one chosen, or -1
// when the question was closed (Escape). The programs pass desktop.Ask; tests pass a fake.
type Ask func(title, text string, buttons []string) int

// Question is everything Decide needs.
type Question struct {
	Running Running
	Dir     string // the data folder that's in use
	Ask     Ask
	// Pick chooses a folder. Nil leaves only the one suggested.
	Pick func() (string, error)
	// FreePort finds a port for the second server; 0 means none.
	FreePort func() int
	// InUse says a folder has a server already. Held, unless a test says otherwise.
	InUse func(dir string) bool
	// Suggest is the folder offered. SuggestFolder, unless a test says otherwise.
	Suggest func(dir string) string
	Now     time.Time
}

// Decision is what to do.
type Decision struct {
	Choice Choice
	Folder string // ChoiceAnother: the second server's data folder
	Port   int    // and its port
}

// The words of the question, kept here so the tray app and the server say the same.
const (
	TitleRunning  = "WittyMediaServer is already running"
	ButtonOpen    = "Open WittyMediaServer"
	ButtonAnother = "Start another server…"
	ButtonExit    = "Exit"

	TitleAnother = "Start another server"
	ButtonStart  = "Start it"
	ButtonChoose = "Choose another folder…"
	ButtonBack   = "Back"
)

// Decide asks what to do about the server already running on q.Dir. Escape or closing
// the window means Exit, as the design says; Back returns to the first question.
func Decide(q Question) Decision {
	inUse, suggest := q.InUse, q.Suggest
	if inUse == nil {
		inUse = Held
	}
	if suggest == nil {
		suggest = SuggestFolder
	}
	// A bounded loop: a person going back and forth gets there, and nothing spins forever.
	for range 20 {
		switch q.Ask(TitleRunning, RunningText(q.Running, q.Now), []string{ButtonOpen, ButtonAnother, ButtonExit}) {
		case 0:
			return Decision{Choice: ChoiceOpen}
		case 1:
			if d, ok := q.another(inUse, suggest(q.Dir)); ok {
				return d
			}
		default:
			return Decision{Choice: ChoiceExit}
		}
	}
	return Decision{Choice: ChoiceExit}
}

// another is the second question; ok false is Back.
func (q Question) another(inUse func(string) bool, folder string) (Decision, bool) {
	port := 0
	if q.FreePort != nil {
		port = q.FreePort()
	}
	if port == 0 {
		q.Ask(TitleAnother, "No free port was found from 9798 up. Close something that uses one, or start the server by hand with -listen.", []string{"OK"})
		return Decision{}, false
	}
	for range 20 {
		buttons := []string{ButtonStart, ButtonChoose, ButtonBack}
		if q.Pick == nil {
			buttons = []string{ButtonStart, ButtonBack}
		}
		c := q.Ask(TitleAnother, AnotherText(folder, port), buttons)
		switch {
		case c == 0:
			return Decision{Choice: ChoiceAnother, Folder: folder, Port: port}, true
		case c == 1 && q.Pick != nil:
			p, err := q.Pick()
			if err != nil || p == "" {
				continue
			}
			if SameFolder(p, q.Dir) || inUse(p) {
				q.Ask(TitleAnother, "A server is already using that folder. Choose another one: each server needs its own.", []string{"OK"})
				continue
			}
			folder = p
		default:
			return Decision{}, false
		}
	}
	return Decision{}, false
}

// RunningText is the first question's text.
func RunningText(r Running, now time.Time) string {
	if r.Started.IsZero() {
		return fmt.Sprintf("It's at %s.", r.URL)
	}
	return fmt.Sprintf("It's at %s and has been running since %s.", r.URL, since(r.Started, now))
}

// AnotherText explains what a second server is before it starts.
func AnotherText(folder string, port int) string {
	return fmt.Sprintf("A second server needs its own data folder: its own libraries, accounts and settings. "+
		"Two servers on one folder would damage its database, so that isn't offered.\n\n"+
		"Folder: %s\nPort: %d\n\n"+
		"It starts as a new server, with its own setup in the browser.", folder, port)
}

// NotAnsweringText is said when the server on the folder doesn't answer.
func NotAnsweringText(r Running) string {
	if r.PID != 0 {
		return fmt.Sprintf("WittyMediaServer is starting, or isn't answering (process %d). Try again in a moment.", r.PID)
	}
	return "Another WittyMediaServer is using this data folder and hasn't said where it is yet. Try again in a moment."
}

func since(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	if y, m, d := t.Date(); now.Year() == y && now.Month() == m && now.Day() == d {
		return t.Format("15:04")
	}
	return t.Format("2 January 15:04")
}

// DesktopQuestion is the question as the programs ask it: in the system's own dialogs,
// with its folder picker, and a port free beside the running one.
func DesktopQuestion(r Running, dir, listen string) Question {
	host, port, _ := net.SplitHostPort(listen)
	running, _ := strconv.Atoi(port)
	return Question{
		Running: r, Dir: dir, Now: time.Now(),
		Ask: func(title, text string, buttons []string) int {
			i, err := desktop.Ask(title, text, buttons)
			if err != nil {
				return -1
			}
			return i
		},
		Pick: func() (string, error) {
			return desktop.PickFolder(context.Background(), "A data folder for the second server")
		},
		FreePort: func() int { return FreePort(host, FirstOtherPort, running) },
	}
}

// WithAnother is args with the second server's folder and port added, which win over
// any given before them.
func WithAnother(args []string, listen string, d Decision) []string {
	host, _, _ := net.SplitHostPort(listen)
	out := append([]string(nil), args...)
	return append(out, "-data", d.Folder, "-listen", net.JoinHostPort(host, strconv.Itoa(d.Port)))
}
