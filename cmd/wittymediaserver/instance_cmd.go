package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/instance"
)

// claimDataFolder makes this the only server on its data folder, before anything in it
// is opened. When another server has it, a start with a desktop asks what to do and one
// without says where it is. It returns the lock, the config to carry on with (another
// folder and port, if that was chosen), and whether to carry on; if not, code is the exit code.
//
// warn is for a folder that can't be locked: the server starts anyway, as nothing must
// stop a start for want of a lock, and says so once its log is up.
func claimDataFolder(cfg *config.Config, args []string) (lock *instance.Lock, next *config.Config, code int, ok bool, warn string) {
	for range 5 { // each round is a person choosing another folder that turned out busy
		var err error
		if os.Getenv("WMS_RESTARTED") == "1" {
			lock, err = instance.AcquireWait(cfg.DataDir, 30*time.Second) // the last run is still on its way out
		} else {
			lock, err = instance.Acquire(cfg.DataDir)
		}
		if err == nil {
			return lock, cfg, 0, true, ""
		}
		if !errors.Is(err, instance.ErrBusy) {
			// No lock to be had here (some network shares, container file sharing). What's
			// left to go on is instance.json and the health check.
			warn = "this data folder can't be locked, so a second server on it can't be stopped: " + err.Error()
			r := instance.Find(context.Background(), cfg.DataDir, time.Second)
			if !r.Answering {
				return nil, cfg, 0, true, warn
			}
		}
		r := instance.Find(context.Background(), cfg.DataDir, 10*time.Second)
		if !askWhenRunning(cfg) {
			fmt.Fprintln(os.Stderr, instance.Line(r, cfg.DataDir))
			return nil, cfg, instance.ExitRunning, false, ""
		}
		if !r.Answering {
			if _, err := desktop.Ask("WittyMediaServer is already running", instance.NotAnsweringText(r), []string{"OK"}); err != nil {
				fmt.Fprintln(os.Stderr, instance.Line(r, cfg.DataDir))
			}
			return nil, cfg, instance.ExitRunning, false, ""
		}
		d := instance.Decide(instance.DesktopQuestion(r, cfg.DataDir, cfg.Listen))
		switch d.Choice {
		case instance.ChoiceOpen:
			if err := desktop.OpenURL(r.URL); err != nil {
				fmt.Fprintln(os.Stderr, instance.Line(r, cfg.DataDir))
			}
			return nil, cfg, 0, false, ""
		case instance.ChoiceAnother:
			args = instance.WithAnother(args, cfg.Listen, d)
			if cfg, err = config.Parse(args); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return nil, nil, 2, false, ""
			}
			continue
		default:
			return nil, cfg, 0, false, ""
		}
	}
	return nil, cfg, instance.ExitRunning, false, ""
}

// askWhenRunning says this start has a person in front of it, who gets the question
// rather than a line: a double-click on Windows, or a start from a desktop with no
// terminal elsewhere. Services, containers, scripts, terminals and the tray app's own
// server (the tray asks before it starts one) get the line and the exit code.
func askWhenRunning(cfg *config.Config) bool {
	if cfg.WatchStdin || asService || desktop.Headless() || os.Getenv("WMS_NO_DIALOG") == "1" {
		return false
	}
	return startedFromDesktop()
}

// portTaken says, in words, why the server couldn't listen: another WittyMediaServer (with
// its own data folder, or it would have been found already), or some other program.
func portTaken(listen string, err error) string {
	var errno syscall.Errno
	if !errors.As(err, &errno) || (errno != syscall.EADDRINUSE && errno != 10048) { // 10048: WSAEADDRINUSE
		return "The server can't listen on " + listen + ": " + err.Error()
	}
	host, port, _ := net.SplitHostPort(listen)
	n, _ := strconv.Atoi(port)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	other := ""
	if free := instance.FreePort(host, n+1); free != 0 {
		other = fmt.Sprintf(", such as -listen %s", net.JoinHostPort(host, strconv.Itoa(free)))
	}
	if _, ok := instance.HealthID(ctx, listenURL(listen)); ok {
		return fmt.Sprintf("Another WittyMediaServer, with a data folder of its own, is already using port %s at %s. Start this one on another port%s, or stop the other one.", port, listenURL(listen), other)
	}
	return fmt.Sprintf("Port %s is in use by another program. Start the server on another port%s (or set listen in wms.json), or stop that program.", port, other)
}
