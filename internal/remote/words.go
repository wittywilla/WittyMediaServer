package remote

import (
	"fmt"
	"strings"
	"time"
)

// How remote access stands, in words. This is the only place they are written:
// the settings page, the setup wizard, the tray icon's menu, the apps
// (GET /api/remote/status) and the help all show what Describe says, so they
// can't drift apart. Never a bare "on" or "off": every line names what it is
// about and what it means for the person.

// Standing is how remote access stands right now.
type Standing struct {
	// Level is the colour: good (working as chosen, green), waiting (asked
	// for, not working yet, amber) or fault (something is broken, red).
	Level   string `json:"level"`
	Subject string `json:"subject"` // what it's about, with the short answer
	Summary string `json:"summary"` // the whole answer, in a sentence or two
	Home    string `json:"home"`    // devices at home
	Away    string `json:"away"`    // devices away from home
	Checked string `json:"checked"` // how that is known
	// Warning stays while the owner lets the internet in unencrypted.
	Warning string `json:"warning,omitempty"`
}

// The subjects. docs/REMOTE.md lists each one, which a test checks.
const (
	SubjectHomeOnly = "Home network only"
	SubjectYes      = "Reachable from away: yes"
	SubjectNotYet   = "Reachable from away: not yet"
	SubjectNo       = "Reachable from away: no"
)

// Subjects are all of them, for the docs test.
var Subjects = []string{SubjectHomeOnly, SubjectYes, SubjectNotYet, SubjectNo}

// PlainAwayWarning is said wherever it matters while unencrypted connections
// from away are allowed: the status card, the dashboard and the apps.
const PlainAwayWarning = "Unencrypted connections from away are allowed. Anyone between a device and this server can read or change passwords, sign-ins and what you watch. Use this only on networks you trust, or behind your own VPN or tunnel."

// Way is one way in from away, as the status found it.
type Way struct {
	Kind     string // vpn | https | proxy | plain
	Label    string // Tailscale, NetBird, VPN, "the internet, over HTTPS", "your reverse proxy"
	URL      string // where to connect, if known
	State    string // ok | waiting | problem
	Detail   string // what is wrong, for a problem
	LastSeen time.Time
}

// Facts are what Describe needs to know.
type Facts struct {
	On        bool // connections from away are allowed
	LocalOnly bool // the server listens on this computer only
	PlainAway bool // the owner lets the internet in without encryption
	Ways      []Way
	Proof     time.Time // when the outside test last passed (zero: never, or too long ago)
	Now       time.Time
}

// Describe says how remote access stands.
func Describe(f Facts) Standing {
	if f.Now.IsZero() {
		f.Now = time.Now()
	}
	s := Standing{Home: "Devices at home: can connect"}
	if f.LocalOnly {
		s.Home = "Devices at home: only this computer, because the server listens on this computer only"
	}
	if f.On && f.PlainAway {
		s.Warning = PlainAwayWarning
	}
	if !f.On {
		s.Level, s.Subject = "good", SubjectHomeOnly
		s.Away = "Devices away from home: not allowed, by your choice"
		s.Summary = "Devices at home can connect. Devices away from home, such as a phone on mobile data, can't: you chose home network only."
		if f.LocalOnly {
			s.Summary = "Only this computer can connect, because the server listens on this computer only. Devices away from home can't: you chose home network only."
		}
		s.Checked = "The server itself refuses every connection from outside your home network, whatever your router lets through."
		return s
	}
	var ok, problems []Way
	waiting := 0
	var last time.Time
	for _, w := range f.Ways {
		switch w.State {
		case "ok":
			ok = append(ok, w)
		case "problem":
			problems = append(problems, w)
		default:
			waiting++
		}
		if w.LastSeen.After(last) {
			last = w.LastSeen
		}
	}
	switch {
	case len(ok) > 0 || !f.Proof.IsZero():
		s.Level, s.Subject = "good", SubjectYes
		s.Away = "Devices away from home: can connect"
		var where []string
		said := map[string]bool{}
		for _, w := range ok {
			if w.URL != "" && said[w.URL] {
				continue // a proxy in front of the HTTPS address is one way in, not two
			}
			said[w.URL] = true
			where = append(where, w.how())
		}
		if len(where) == 0 {
			s.Summary = "Devices away from home can connect."
		} else {
			s.Summary = "Devices away from home can connect " + strings.Join(where, ", or ") + "."
		}
		if len(problems) > 0 {
			s.Level = "waiting"
			s.Summary += " Another way in has a problem: " + problems[0].Detail
		}
	case len(problems) > 0:
		s.Level, s.Subject = "fault", SubjectNo
		s.Away = "Devices away from home: can't connect, something is in the way"
		s.Summary = "A phone on mobile data can't connect. " + problems[0].Detail
	case waiting == 0:
		s.Level, s.Subject = "waiting", SubjectNotYet
		s.Away = "Devices away from home: can't connect yet, there's no way in"
		s.Summary = "You chose to let devices away from home in, but there's no way in yet, so a phone on mobile data can't connect. Finish the steps for your choice below."
	default:
		s.Level, s.Subject = "waiting", SubjectNotYet
		s.Away = "Devices away from home: not confirmed yet"
		s.Summary = "Everything is set up here, but nothing from outside has reached this server yet, so it isn't known whether a phone on mobile data can connect."
		for _, w := range f.Ways {
			if w.Kind == "https" {
				s.Summary += " If you forwarded a port, your router may not be sending connections here yet."
				break
			}
		}
		s.Summary += " Run the check below."
	}
	switch {
	case !f.Proof.IsZero():
		s.Checked = "Checked from outside with your phone, " + Ago(f.Proof, f.Now) + "."
		if last.After(f.Proof) {
			s.Checked += " Last connection from away: " + Ago(last, f.Now) + "."
		}
	case !last.IsZero():
		s.Checked = "Last connection from away: " + Ago(last, f.Now) + ". Not checked with your phone yet."
	default:
		s.Checked = "Not checked from outside yet."
	}
	return s
}

// how says how one way in is used, for a sentence.
func (w Way) how() string {
	at := ""
	if w.URL != "" {
		at = " at " + w.URL
	}
	switch w.Kind {
	case "vpn":
		return "through " + w.Label + at + ", with " + w.Label + " switched on"
	case "proxy":
		return "through your reverse proxy" + at
	case "plain":
		return "over the internet unencrypted" + at
	}
	return "over the internet" + at
}

// Ago says how long ago something was, the way the settings page does.
func Ago(t, now time.Time) string {
	s := int(now.Sub(t).Seconds())
	switch {
	case s < 90:
		return "just now"
	case s < 5400:
		return fmt.Sprintf("%d minutes ago", (s+30)/60)
	case s < 129600:
		return fmt.Sprintf("%d hours ago", (s+1800)/3600)
	}
	return fmt.Sprintf("%d days ago", (s+43200)/86400)
}
