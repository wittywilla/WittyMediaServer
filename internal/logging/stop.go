// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Why a server stops, from a fixed list, so that a person reading the log, or the page
// that reads it for them, can always tell which it was.
const (
	StopTrayQuit       = "tray-quit"       // Quit in the tray or menu-bar icon
	StopTrayStop       = "tray-stop"       // Stop server in that menu
	StopTrayRestart    = "tray-restart"    // Restart server in that menu
	StopTrayClosed     = "tray-closed"     // the desktop closed the tray app, without a shutdown
	StopTrayGone       = "tray-gone"       // the tray app ended without saying why (it was ended, or crashed)
	StopService        = "service-stop"    // the Windows service, systemd, launchd or rc.d was told to stop
	StopShutdown       = "os-shutdown"     // the computer is shutting down or restarting
	StopLogoff         = "os-logoff"       // the person signed out
	StopInstaller      = "installer"       // an installer asked it to close
	StopUpdate         = "update"          // this server's own updater, restarting into the new version
	StopRestartSetting = "restart-setting" // a reset, restore or repair that needs a restart
	StopSignal         = "signal"          // SIGINT or SIGTERM: Ctrl+C, kill
	StopConsoleClosed  = "console-closed"  // its console window was closed
	StopAPI            = "api"             // an admin pressed Restart in Settings
	StopFatal          = "fatal-error"     // it couldn't carry on
)

// Stop is why this run is stopping.
type Stop struct {
	Reason string    `json:"reason"`
	Detail string    `json:"detail,omitempty"`
	By     string    `json:"by,omitempty"` // the account, for one pressed in Settings
	At     time.Time `json:"at"`
	weak   bool
}

var stopping struct {
	sync.Mutex
	s Stop
}

// NoteStop says why the server is about to stop. The first reason given is the one kept:
// whatever asked first is what started it. It returns false if a reason was already kept.
func NoteStop(reason, detail, by string) bool {
	return noteStop(Stop{Reason: reason, Detail: detail, By: by, At: time.Now()})
}

// NoteStopIfNothingElse is for a reason that's only a guess, such as the tray app's
// pipe closing: anything more exact that follows replaces it.
func NoteStopIfNothingElse(reason, detail string) bool {
	return noteStop(Stop{Reason: reason, Detail: detail, At: time.Now(), weak: true})
}

func noteStop(s Stop) bool {
	stopping.Lock()
	if stopping.s.Reason != "" && !(stopping.s.weak && !s.weak) {
		stopping.Unlock()
		return false
	}
	stopping.s = s
	stopping.Unlock()
	activeGuard.RLock()
	g := activeGuard.g
	activeGuard.RUnlock()
	if g != nil {
		g.noteStopping(s)
	}
	return true
}

// StopNoted is the reason kept, if any.
func StopNoted() Stop {
	stopping.Lock()
	defer stopping.Unlock()
	return stopping.s
}

// resetStop is for tests.
func resetStop() {
	stopping.Lock()
	stopping.s = Stop{}
	stopping.Unlock()
}

// Words for each reason, for the line that starts the next run and for people.
var stopWords = map[string]string{
	StopTrayQuit:       "Quit was chosen in the tray icon",
	StopTrayStop:       "Stop server was chosen in the tray icon",
	StopTrayRestart:    "Restart server was chosen in the tray icon",
	StopTrayClosed:     "the desktop closed the tray app",
	StopTrayGone:       "the tray app ended without saying why",
	StopService:        "the service was stopped",
	StopShutdown:       "the computer shut down or restarted",
	StopLogoff:         "the person signed out",
	StopInstaller:      "an installer closed it",
	StopUpdate:         "it restarted to finish an update",
	StopRestartSetting: "it restarted for a reset, restore or repair",
	StopSignal:         "it was told to stop (a signal)",
	StopConsoleClosed:  "its console window was closed",
	StopAPI:            "an admin restarted it in Settings",
	StopFatal:          "of an error it couldn't carry on from",
}

// Words is the reason in a few plain words.
func (s Stop) Words() string {
	if w, ok := stopWords[s.Reason]; ok {
		if s.Reason == StopFatal && s.Detail != "" {
			return "of an error: " + s.Detail
		}
		return w
	}
	if s.Reason == "" {
		return "no reason was given"
	}
	return s.Reason
}

// ---------------------------------------------------------------- how the last run ended

// LastRun is how the run before this one ended, kept in last-run.json beside the crash
// reports, and the last runs in runs.json.
type LastRun struct {
	PID     int       `json:"pid"`
	Version string    `json:"version"`
	Started time.Time `json:"started"`
	Ended   time.Time `json:"ended"`
	Clean   bool      `json:"clean"`
	Stop    Stop      `json:"stop"`
	// Unclean: the report made for it, and the last line it logged.
	Report  string `json:"report,omitempty"`
	LastLog string `json:"lastLog,omitempty"`
}

const (
	lastRunFile = "last-run.json"
	runsFile    = "runs.json"
	keepRuns    = 20
)

// Summary is the run in one line: "stopped 14:33 (tray-quit), ran 2h38m".
func (r LastRun) Summary() string {
	if r.Started.IsZero() {
		return "none recorded"
	}
	ran := ""
	if !r.Ended.IsZero() {
		ran = ", ran " + Uptime(r.Ended.Sub(r.Started))
	}
	when := r.Ended.Local().Format("2006-01-02 15:04")
	if !r.Clean {
		if r.Stop.Reason != "" {
			return fmt.Sprintf("was stopping at %s (%s) but didn't finish%s", when, r.Stop.Reason, ran)
		}
		return fmt.Sprintf("ended without stopping cleanly, last seen %s%s", when, ran)
	}
	return fmt.Sprintf("stopped %s (%s)%s", when, r.Stop.Reason, ran)
}

// Uptime is a duration as people say it: 2h38m, 4d3h, 12s.
func Uptime(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
}

// Previous is how the run before this one ended, as found at this start.
func (g *CrashGuard) Previous() (LastRun, bool) {
	if g == nil {
		return LastRun{}, false
	}
	return g.previous, !g.previous.Started.IsZero()
}

// Runs is the last runs, newest first, for the support bundle and the log page.
func Runs(crashDir string) []LastRun {
	var runs []LastRun
	b, err := os.ReadFile(filepath.Join(crashDir, runsFile))
	if err != nil || len(b) > 1<<20 {
		return nil
	}
	_ = json.Unmarshal(b, &runs)
	return runs
}

func (g *CrashGuard) record(r LastRun) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err == nil {
		_ = writeAtomic(filepath.Join(g.dir, lastRunFile), b)
	}
	runs := append([]LastRun{r}, Runs(g.dir)...)
	if len(runs) > keepRuns {
		runs = runs[:keepRuns]
	}
	if b, err := json.MarshalIndent(runs, "", "  "); err == nil {
		_ = writeAtomic(filepath.Join(g.dir, runsFile), b)
	}
}

func writeAtomic(p string, b []byte) error {
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func readLastRun(dir string) (LastRun, bool) {
	var r LastRun
	b, err := os.ReadFile(filepath.Join(dir, lastRunFile))
	if err != nil || len(b) > 64<<10 || json.Unmarshal(b, &r) != nil {
		return LastRun{}, false
	}
	return r, true
}

// noteStopping writes the reason into this run's marker: a stop that hangs, and is then
// ended by force, still says what it was doing.
func (g *CrashGuard) noteStopping(s Stop) {
	if g == nil {
		return
	}
	g.markerMu.Lock()
	defer g.markerMu.Unlock()
	if g.marker == "" { // closed: the run has ended cleanly already
		return
	}
	g.run.Stopping = &s
	_ = g.writeMarker()
}

// shortStop is a reason as a log value: "tray-quit" or "signal: SIGTERM".
func shortStop(s Stop) string {
	if s.Detail == "" {
		return s.Reason
	}
	return s.Reason + ": " + strings.TrimSpace(s.Detail)
}
