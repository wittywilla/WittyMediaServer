// Command wms-tray is the desktop launcher: a system-tray / menu-bar icon that
// starts and stops the wms server process, opens the web UI, shows logs and
// can register itself to start at login. It ships as WittyMediaServer(.exe).
package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/trayicon"
)

// version is stamped by the release scripts (-X main.version=...); this is
// the default for source builds.
var version = "0.2.0-alpha"

type launcher struct {
	mu      sync.Mutex
	exe     string // wms binary
	dataDir string
	listen  string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	logFile *os.File
	running bool
	healthy bool
	wantRun bool
	lastErr string
	logPath string
	// Crash-loop protection.
	startedAt time.Time
	fails     int

	mStatus, mOpen, mToggle, mRestart, mData, mLog, mAuto, mAbout, mQuit *systray.MenuItem
}

func main() {
	l := &launcher{}
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	l.exe = findServer(dir)
	l.dataDir = os.Getenv("WMS_DATA_DIR")
	if l.dataDir == "" {
		l.dataDir = config.DefaultDataDir()
	}
	for i, a := range os.Args {
		if (a == "-data" || a == "--data") && i+1 < len(os.Args) {
			l.dataDir = os.Args[i+1]
		}
		if (a == "-listen" || a == "--listen") && i+1 < len(os.Args) {
			l.listen = os.Args[i+1]
		}
	}
	if l.listen == "" {
		l.listen = os.Getenv("WMS_LISTEN")
	}
	if l.listen == "" {
		l.listen = fmt.Sprintf(":%d", config.DefaultPort)
	}
	if abs, err := filepath.Abs(l.dataDir); err == nil {
		l.dataDir = abs
	}
	_ = os.MkdirAll(l.dataDir, 0o755)
	l.logPath = filepath.Join(l.dataDir, "wms.log")
	l.wantRun = !hasArg("-no-autostart")
	systray.Run(l.onReady, l.onExit)
}

func hasArg(s string) bool {
	for _, a := range os.Args[1:] {
		if a == s || a == "-"+s {
			return true
		}
	}
	return false
}

func findServer(dir string) string {
	name := "wms"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, c := range []string{filepath.Join(dir, name), filepath.Join(dir, "..", "Resources", name), filepath.Join(dir, "bin", name)} {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("wms"); err == nil {
		return p
	}
	return filepath.Join(dir, name)
}

func (l *launcher) url() string {
	host, port, err := net.SplitHostPort(l.listen)
	if err != nil {
		return "http://localhost:" + strings.TrimPrefix(l.listen, ":")
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

func (l *launcher) onExit() {
	l.stop()
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
			openURL(l.url())
		case <-l.mToggle.ClickedCh:
			if l.isRunning() {
				l.wantRun = false
				l.stop()
			} else {
				l.wantRun = true
				l.start()
			}
		case <-l.mRestart.ClickedCh:
			l.wantRun = true
			l.stop()
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
			openURL(l.url() + "/settings/about")
		case <-l.mQuit.ClickedCh:
			l.stop()
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

func (l *launcher) start() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		return
	}
	f, err := os.OpenFile(l.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		l.lastErr = err.Error()
		return
	}
	// Keep the log from growing without bound.
	if st, _ := f.Stat(); st != nil && st.Size() > 20<<20 {
		f.Close()
		_ = os.Rename(l.logPath, l.logPath+".1")
		f, _ = os.OpenFile(l.logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	}
	cmd := exec.Command(l.exe, "serve", "-data", l.dataDir, "-listen", l.listen, "-watch-stdin")
	cmd.Dir = filepath.Dir(l.exe)
	cmd.Stdout = f
	cmd.Stderr = f
	hideWindow(cmd)
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
	l.startedAt = time.Now()
	go func(c *exec.Cmd) {
		err := c.Wait()
		l.mu.Lock()
		if l.cmd == c {
			l.running = false
			l.healthy = false
			if err != nil {
				l.lastErr = lastLogLine(l.logPath)
			}
			if l.logFile != nil {
				l.logFile.Close()
				l.logFile = nil
			}
		}
		// Give up after repeated immediate failures (bad config, port in use…).
		if time.Since(l.startedAt) < 10*time.Second {
			l.fails++
		} else {
			l.fails = 0
		}
		if l.fails >= 3 {
			l.wantRun = false
		}
		restart := l.wantRun && l.cmd == c
		l.refreshLocked()
		l.mu.Unlock()
		if restart {
			// Crashed: back off briefly and bring it back.
			time.Sleep(3 * time.Second)
			if l.wantRun && !l.isRunning() {
				l.start()
			}
		}
	}(cmd)
	l.refreshLocked()
}

func (l *launcher) stop() {
	l.mu.Lock()
	cmd, stdin := l.cmd, l.stdin
	l.cmd, l.stdin = nil, nil
	l.mu.Unlock()
	if cmd == nil {
		return
	}
	if stdin != nil {
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
		if l.isRunning() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			req, _ := http.NewRequestWithContext(ctx, "GET", l.url()+"/api/health", nil)
			if res, err := c.Do(req); err == nil {
				res.Body.Close()
				ok = res.StatusCode == 200
			}
			cancel()
		}
		l.mu.Lock()
		changed := ok != l.healthy
		l.healthy = ok
		if ok {
			l.fails = 0
		}
		if changed {
			l.refreshLocked()
		}
		l.mu.Unlock()
	}
}

func (l *launcher) refresh() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refreshLocked()
}

func (l *launcher) refreshLocked() {
	switch {
	case l.running && l.healthy:
		l.mStatus.SetTitle("● Running at " + l.url())
		systray.SetTooltip("WittyMediaServer — running at " + l.url())
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

func openURL(u string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		_ = exec.Command("open", u).Start()
	default:
		_ = exec.Command("xdg-open", u).Start()
	}
}

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
