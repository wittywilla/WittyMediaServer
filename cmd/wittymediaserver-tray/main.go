// Command wittymediaserver-tray is the app a person opens: a system-tray or menu-bar icon
// that starts and stops the server, opens the web app, shows logs and can start itself at
// login. It ships as WittyMediaServer(.exe).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/desktop"
	"github.com/wittywilla/wittymediaserver/internal/discovery"
	"github.com/wittywilla/wittymediaserver/internal/instance"
	"github.com/wittywilla/wittymediaserver/internal/layout"
	"github.com/wittywilla/wittymediaserver/internal/logging"
	"github.com/wittywilla/wittymediaserver/internal/trayicon"
	"github.com/wittywilla/wittymediaserver/internal/update"
)

// version is stamped by the release scripts (-X main.version=...); this is
// the default for source builds.
var version = "0.5.2-alpha"

// trayExe is this program's path when it started (an update renames the
// running file to *.old and puts the new version at this path).
var trayExe, _ = os.Executable()

type launcher struct {
	mu         sync.Mutex
	exe        string // the server program
	dataDir    string
	listen     string
	configFile string
	args       []string // the app's own arguments, read again before each start
	// listenGiven: -listen came with the app, so it's passed on. Otherwise the server finds
	// its port itself, in settings.ini, and Settings → Network can change it.
	listenGiven bool
	// seen is where the server this app shows answered, when that isn't listen: a background
	// service on another port, found with discovery.AskLocal.
	seen    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	logFile *serverOutput
	running bool
	healthy bool
	// answered: this run of the server has answered at least once, so not answering now
	// means it stopped answering, not that it's still starting.
	answered bool
	wantRun  bool
	lastErr  string
	logPath  string
	// attached: a server was already answering on this port, a background service most
	// likely, or took this app's data folder first. Starting another would only fail, so
	// this app just shows it.
	attached bool
	// Crash-loop protection.
	startedAt time.Time
	fails     int
	// afterCrash: the next start is the tray bringing back a server that stopped by itself.
	afterCrash bool

	// update is a newer version the server found, which the menu offers.
	update string

	mStatus, mUpdate, mOpen, mToggle, mRestart, mData, mLog, mAuto, mAbout, mQuit *systray.MenuItem

	// mRemote says who can connect, in the server's own words (GET /api/remote/status).
	mRemote     *systray.MenuItem
	remoteAsked time.Time
}

func main() {
	l := &launcher{}
	l.exe = findServer()
	// Same settings as the server: config file (portable or per user),
	// environment, then our own -data/-listen/-config arguments.
	var args []string
	for _, a := range os.Args[1:] {
		if a != "-no-autostart" {
			args = append(args, a)
		}
	}
	cfg, err := config.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	cfg, code, ok := claim(cfg, args)
	if !ok {
		os.Exit(code)
	}
	// After claim, a second server on a folder and port of its own was given -listen: it keeps that.
	l.dataDir, l.listen, l.configFile, l.args = cfg.DataDir, cfg.Listen, cfg.ConfigFile, args
	l.listenGiven = cfg.ListenFrom == config.FromFlag
	if abs, err := filepath.Abs(l.dataDir); err == nil {
		l.dataDir = abs
	}
	l.logPath = filepath.Join(cfg.LogDir, "console.log")
	l.wantRun = !hasArg("-no-autostart")
	systray.Run(l.onReady, l.onExit)
}

// claim asks, before a second server starts on a data folder that has one: open the
// running one (the default), start another on a folder and port of its own, or exit. A
// server the service manager runs is shown instead, as before. It returns the config to
// start with, and whether to.
func claim(cfg *config.Config, args []string) (*config.Config, int, bool) {
	for range 5 {
		if !instance.Held(cfg.DataDir) {
			return cfg, 0, true
		}
		out := instance.WhenRunning(cfg.DataDir, cfg.Listen, args, !desktop.Headless(), true)
		if out.Attach {
			return cfg, 0, true // start() finds it answering and shows it
		}
		if out.Again == nil {
			return cfg, out.Code, false
		}
		args = out.Again
		next, err := config.Parse(args)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return cfg, 2, false
		}
		cfg = next
	}
	return cfg, instance.ExitRunning, false
}

func hasArg(s string) bool {
	for _, a := range os.Args[1:] {
		if a == s || a == "-"+s {
			return true
		}
	}
	return false
}

func findServer() string {
	pkg := layout.Current()
	name := layout.Exe(layout.ServerCommand)
	for _, d := range pkg.ProgramDirs() {
		c := filepath.Join(d, name)
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath(layout.ServerCommand); err == nil {
		return p
	}
	return pkg.ServerPath()
}

// url is the address of the server this app shows. l.mu is held.
func (l *launcher) url() string {
	if l.attached && l.seen != "" {
		return l.seen
	}
	return listenURL(l.listen)
}

// currentURL is url, for callers that don't hold l.mu.
func (l *launcher) currentURL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.url()
}

func listenURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://localhost:" + strings.TrimPrefix(listen, ":")
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func (l *launcher) onReady() {
	l.setIcon(false)
	systray.SetTitle("")
	systray.SetTooltip("WittyMediaServer")
	l.mStatus = systray.AddMenuItem("WittyMediaServer — starting…", "")
	l.mStatus.Disable()
	l.mRemote = systray.AddMenuItem("Remote Access", "Who can connect, and from where. Click to change it.")
	l.mRemote.Hide()
	l.mUpdate = systray.AddMenuItem("Update WittyMediaServer…", "See what's new and how to update")
	l.mUpdate.Hide()
	systray.AddSeparator()
	l.mOpen = systray.AddMenuItem("Open WittyMediaServer", "Open the web app in your browser")
	l.mToggle = systray.AddMenuItem("Stop server", "")
	l.mRestart = systray.AddMenuItem("Restart server", "")
	systray.AddSeparator()
	l.mData = systray.AddMenuItem("Open data folder", l.dataDir)
	l.mLog = systray.AddMenuItem("View log", l.logPath)
	l.mAuto = systray.AddMenuItemCheckbox("Start at login", "Launch this tray app when you sign in", autostartEnabled())
	systray.AddSeparator()
	l.mAbout = systray.AddMenuItem("About WittyMediaServer "+version, "")
	l.mQuit = systray.AddMenuItem("Quit (stops the server)", "")

	go l.loop()
	go l.health()
	if l.wantRun {
		l.start()
	} else {
		l.refresh()
	}
}

// onExit: after Quit the server has already stopped. Otherwise the desktop is closing this
// app (a shutdown, a sign-out, an installer), and the server is told which.
func (l *launcher) onExit() {
	l.stop(endReason())
}

func (l *launcher) setIcon(running bool) {
	if runtime.GOOS == "windows" {
		systray.SetIcon(trayicon.ICO(!running))
	} else {
		systray.SetIcon(trayicon.PNG(64, !running))
	}
}

func (l *launcher) loop() {
	for {
		select {
		case <-l.mOpen.ClickedCh:
			openURL(l.currentURL())
		case <-l.mRemote.ClickedCh:
			openURL(l.currentURL() + "/settings/remote")
		case <-l.mUpdate.ClickedCh:
			openURL(l.currentURL() + "/settings/about#updates")
		case <-l.mToggle.ClickedCh:
			if l.isRunning() {
				l.want(false)
				l.stop(logging.StopTrayStop)
			} else {
				l.want(true)
				l.start()
			}
		case <-l.mRestart.ClickedCh:
			l.want(true)
			l.stop(logging.StopTrayRestart)
			l.start()
		case <-l.mData.ClickedCh:
			openPath(l.dataDir)
		case <-l.mLog.ClickedCh:
			openPath(l.logPath)
		case <-l.mAuto.ClickedCh:
			if l.mAuto.Checked() {
				_ = setAutostart(false)
			} else {
				_ = setAutostart(true)
			}
			if autostartEnabled() {
				l.mAuto.Check()
			} else {
				l.mAuto.Uncheck()
			}
		case <-l.mAbout.ClickedCh:
			openURL(l.currentURL() + "/settings/about")
		case <-l.mQuit.ClickedCh:
			l.stop(logging.StopTrayQuit)
			systray.Quit()
			return
		}
	}
}

func (l *launcher) isRunning() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.running
}

// want says whether the server should be kept running. A choice made in the menu starts
// the count of quick failures again.
func (l *launcher) want(run bool) {
	l.mu.Lock()
	l.wantRun, l.fails = run, 0
	l.mu.Unlock()
}

// wantsRestart: the server should be running and isn't.
func (l *launcher) wantsRestart() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.wantRun && !l.running
}

func (l *launcher) start() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return
	}
	// The port may have been changed in Settings → Network or settings.ini since the last start.
	if cfg, err := config.ParseOnly(l.args); err == nil && !l.listenGiven {
		l.listen = cfg.Listen
	}
	if seen, ok := answering(l.listen, l.seen); ok {
		l.attached, l.seen = true, seen
		l.refreshLocked()
		return
	}
	l.attached, l.seen = false, ""
	f, err := openServerOutput(l.logPath)
	if err != nil {
		l.lastErr = err.Error()
		return
	}
	args := []string{"serve", "-watch-stdin", "-data", l.dataDir}
	if l.listenGiven {
		args = append(args, "-listen", l.listen)
	}
	if l.configFile != "" {
		args = append(args, "-config", l.configFile)
	}
	cmd := exec.Command(l.exe, args...)
	cmd.Dir = filepath.Dir(l.exe)
	cmd.Stdout = f
	cmd.Stderr = f
	cmd.WaitDelay = 5 * time.Second // something it left behind holding stderr mustn't hide that it ended
	hideWindow(cmd)
	if l.afterCrash {
		cmd.Env = append(os.Environ(), "WMS_TRAY_RESTART=1")
		l.afterCrash = false
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		l.lastErr = err.Error()
		f.Close()
		return
	}
	fmt.Fprintf(f, "\n--- tray: starting %s at %s ---\n", l.exe, time.Now().Format(time.RFC3339))
	if err := cmd.Start(); err != nil {
		l.lastErr = err.Error()
		fmt.Fprintf(f, "tray: start failed: %v\n", err)
		f.Close()
		l.refreshLocked()
		return
	}
	l.cmd, l.stdin, l.logFile, l.running, l.lastErr = cmd, stdin, f, true, ""
	l.startedAt, l.answered = time.Now(), false
	go func(c *exec.Cmd) {
		err := c.Wait()
		// The server installed an update and asks to be started again.
		updated := c.ProcessState != nil && c.ProcessState.ExitCode() == update.ExitRestart
		// Another server took the data folder first: show that one, and don't try again.
		taken := c.ProcessState != nil && c.ProcessState.ExitCode() == instance.ExitRunning
		l.mu.Lock()
		if l.cmd == c {
			l.running = false
			l.healthy = false
			if err != nil {
				l.lastErr = lastLogLine(l.logPath)
			}
			if l.logFile != nil {
				if err != nil && !updated {
					fmt.Fprintf(l.logFile, "--- tray: the server stopped at %s: %s ---\n", time.Now().Format(time.RFC3339), logging.ExitReason(err))
				}
				l.logFile.Close()
				l.logFile = nil
			}
		}
		// Give up after repeated immediate failures (bad config, port in use…).
		if updated {
			l.fails = 0
		} else if time.Since(l.startedAt) < 10*time.Second {
			l.fails++
		} else {
			l.fails = 0
		}
		if l.fails >= 3 {
			l.wantRun = false
		}
		if taken && l.cmd == c {
			l.wantRun, l.attached, l.lastErr = false, true, ""
			// Shown at its own address, which needn't be this app's port.
			if info, err := instance.Read(l.dataDir); err == nil && len(info.URLs) > 0 {
				if u, err := url.Parse(info.URLs[0]); err == nil && u.Host != "" {
					l.listen = u.Host
				}
			}
		}
		restart := l.wantRun && l.cmd == c
		l.refreshLocked()
		l.mu.Unlock()
		if updated && restart && relaunchTray() == nil {
			// The new tray app starts the new server.
			systray.Quit()
			return
		}
		if restart {
			// Crashed: back off briefly and bring it back.
			time.Sleep(3 * time.Second)
			if l.wantsRestart() {
				l.mu.Lock()
				l.afterCrash = true // its first log line says the tray started it again
				l.mu.Unlock()
				l.start()
			}
		}
	}(cmd)
	l.refreshLocked()
}

// serverOutput is the file the server's own output goes to while the tray runs it.
// It starts over in a second file at a size, so a server that runs for months
// can't fill the disk with it: at most twice serverOutputMax.
type serverOutput struct {
	path string
	mu   sync.Mutex
	f    *os.File
	size int64
}

const serverOutputMax = 20 << 20

func openServerOutput(path string) (*serverOutput, error) {
	o := &serverOutput{path: path}
	return o, o.open()
}

func (o *serverOutput) open() error {
	f, err := os.OpenFile(o.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	o.f, o.size = f, 0
	if st, err := f.Stat(); err == nil {
		o.size = st.Size()
	}
	return nil
}

func (o *serverOutput) Write(b []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.size+int64(len(b)) > serverOutputMax && o.f != nil {
		o.f.Close()
		o.f = nil
		_ = os.Rename(o.path, o.path+".1")
	}
	if o.f == nil {
		if err := o.open(); err != nil {
			return len(b), nil // what the server says isn't worth stopping it for
		}
	}
	n, err := o.f.Write(b)
	o.size += int64(n)
	if err != nil {
		return len(b), nil
	}
	return n, nil
}

func (o *serverOutput) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.f == nil {
		return nil
	}
	err := o.f.Close()
	o.f = nil
	return err
}

// relaunchTray starts the updated tray app with the same arguments.
func relaunchTray() error {
	var args []string
	for _, a := range os.Args[1:] {
		if a != "-no-autostart" {
			args = append(args, a)
		}
	}
	cmd := exec.Command(trayExe, args...)
	cmd.Dir = filepath.Dir(trayExe)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// answering says whether a WittyMediaServer already answers on this computer, and where: where
// it was last seen, at this app's address, listen, or for a background service moved to another
// port, wherever local discovery says it is. Without a service installed, a server found that way
// would be a second one, on a folder of its own, which this app doesn't show.
func answering(listen, seen string) (string, bool) {
	if seen != "" && healthy(seen) {
		return seen, true
	}
	own := listenURL(listen)
	if own != seen && healthy(own) {
		return own, true
	}
	if !serviceInstalled() {
		return "", false
	}
	if port, tls := discovery.AskLocal(700 * time.Millisecond); port != 0 {
		scheme := "http"
		if tls {
			scheme = "https"
		}
		if u := scheme + "://localhost:" + strconv.Itoa(port); u != own && healthy(u) {
			return u, true
		}
	}
	return "", false
}

func healthy(base string) bool {
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	res, err := c.Get(base + "/api/health")
	if err != nil {
		return false
	}
	res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// stop asks the server to stop, saying why first: the line it logs, and the next start's,
// carry the reason.
func (l *launcher) stop(reason string) {
	l.mu.Lock()
	cmd, stdin := l.cmd, l.stdin
	l.cmd, l.stdin = nil, nil
	l.mu.Unlock()
	if cmd == nil {
		return
	}
	if stdin != nil {
		_, _ = io.WriteString(stdin, "stop "+reason+"\n")
		_ = stdin.Close() // graceful: the server exits when stdin closes
	}
	done := make(chan struct{})
	go func() { _, _ = cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
	}
	l.mu.Lock()
	l.running, l.healthy = false, false
	if l.logFile != nil {
		l.logFile.Close()
		l.logFile = nil
	}
	l.refreshLocked()
	l.mu.Unlock()
}

func (l *launcher) health() {
	c := &http.Client{Timeout: 2 * time.Second}
	for {
		time.Sleep(3 * time.Second)
		ok := false
		l.mu.Lock()
		attached := l.attached
		l.mu.Unlock()
		newer := ""
		if attached || l.isRunning() {
			ok, newer = l.ask(c)
		}
		l.showRemote(c, ok)
		l.mu.Lock()
		if newer != l.update {
			l.update = newer
			l.showUpdate()
		}
		if attached {
			// Not where it was: a background service may have moved to another port.
			if !ok {
				listen := l.listen
				l.mu.Unlock()
				seen, found := answering(listen, "")
				l.mu.Lock()
				if found {
					l.seen, ok = seen, true
				}
			}
			changed := ok != l.healthy
			l.healthy = ok
			if changed {
				l.refreshLocked()
			}
			l.mu.Unlock()
			continue
		}
		changed := ok != l.healthy
		l.healthy = ok
		if ok {
			l.fails = 0
			l.answered = l.running
		}
		if changed {
			l.refreshLocked()
		}
		l.mu.Unlock()
	}
}

// showRemote puts how remote access stands in the menu, at most every half
// minute, and hides the line while the server isn't answering.
func (l *launcher) showRemote(c *http.Client, ok bool) {
	if !ok {
		l.mRemote.Hide()
		l.remoteAsked = time.Time{}
		return
	}
	if time.Since(l.remoteAsked) < 30*time.Second {
		return
	}
	l.remoteAsked = time.Now()
	subject, summary, err := readRemote(c, l.currentURL())
	if err != nil {
		l.mRemote.Hide()
		return
	}
	l.mRemote.SetTitle("Remote Access — " + subject)
	l.mRemote.SetTooltip(summary + " Click to change it.")
	l.mRemote.Show()
}

// readRemote asks the server how remote access stands. The tray needs no
// account for this: the server answers its own computer.
func readRemote(c *http.Client, base string) (subject, summary string, err error) {
	res, err := c.Get(base + "/api/remote/status")
	if err != nil {
		return "", "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d", res.StatusCode)
	}
	var v struct{ Subject, Summary string }
	if err := json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&v); err != nil {
		return "", "", err
	}
	if v.Subject == "" {
		return "", "", fmt.Errorf("no subject")
	}
	return v.Subject, v.Summary, nil
}

// ask asks the server whether it's answering, and whether it has found a newer version: it
// says so only to a program on its own computer.
func (l *launcher) ask(c *http.Client) (ok bool, newer string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", l.currentURL()+"/api/health", nil)
	res, err := c.Do(req)
	if err != nil {
		return false, ""
	}
	defer res.Body.Close()
	var h struct {
		Update string `json:"update"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&h)
	if !update.Valid(h.Update) {
		h.Update = ""
	}
	return res.StatusCode == http.StatusOK, h.Update
}

// showUpdate puts a newer version in the menu, or takes it out. Call with l.mu held.
func (l *launcher) showUpdate() {
	if l.update == "" {
		l.mUpdate.Hide()
		return
	}
	l.mUpdate.SetTitle("Update to " + l.update + "…")
	l.mUpdate.Show()
}

func (l *launcher) refresh() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refreshLocked()
}

func (l *launcher) refreshLocked() {
	if l.attached {
		if l.healthy {
			l.mStatus.SetTitle("● Running at " + l.url() + ", started outside this app")
			systray.SetTooltip("WittyMediaServer — running at " + l.url())
			l.mOpen.Enable()
		} else {
			l.mStatus.SetTitle("○ The server started outside this app isn't answering")
			systray.SetTooltip("WittyMediaServer — not answering")
			l.mOpen.Disable()
		}
		l.setIcon(l.healthy)
		// It's the service's to start and stop.
		l.mToggle.Disable()
		l.mRestart.Disable()
		return
	}
	switch {
	case l.running && l.healthy:
		l.mStatus.SetTitle("● Running at " + l.url())
		systray.SetTooltip("WittyMediaServer — running at " + l.url())
		l.mToggle.SetTitle("Stop server")
	case l.running && l.answered:
		// Not "starting": a server that stops answering looks crashed from a browser, and
		// someone looking here should see that it's still running, and can be restarted.
		l.mStatus.SetTitle("◌ Running, but not answering")
		systray.SetTooltip("WittyMediaServer — not answering")
		l.mToggle.SetTitle("Stop server")
	case l.running:
		l.mStatus.SetTitle("◐ Starting…")
		systray.SetTooltip("WittyMediaServer — starting")
		l.mToggle.SetTitle("Stop server")
	default:
		msg := "○ Stopped"
		if l.lastErr != "" {
			msg += " — " + truncate(l.lastErr, 60)
		}
		l.mStatus.SetTitle(msg)
		systray.SetTooltip("WittyMediaServer — stopped")
		l.mToggle.SetTitle("Start server")
	}
	l.setIcon(l.running)
	if l.running {
		l.mOpen.Enable()
		l.mRestart.Enable()
	} else {
		l.mOpen.Disable()
		l.mRestart.Disable()
	}
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n-1] + "…"
	}
	return s
}

func lastLogLine(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return "exited"
	}
	defer f.Close()
	st, _ := f.Stat()
	if st != nil && st.Size() > 8192 {
		_, _ = f.Seek(-8192, io.SeekEnd)
	}
	var last string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" && !strings.HasPrefix(t, "---") {
			last = t
		}
	}
	if i := strings.Index(last, "msg="); i >= 0 {
		last = last[i+4:]
	}
	if last == "" {
		return "exited"
	}
	return last
}

func openURL(u string) { _ = desktop.OpenURL(u) }

func openPath(p string) {
	switch runtime.GOOS {
	case "windows":
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			_ = exec.Command("notepad", p).Start()
			return
		}
		_ = exec.Command("explorer", p).Start()
	case "darwin":
		_ = exec.Command("open", p).Start()
	default:
		_ = exec.Command("xdg-open", p).Start()
	}
}
