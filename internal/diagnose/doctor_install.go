package diagnose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/config"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/hls"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

func installChecks() []Check {
	return []Check{
		{ID: "config-file", Area: AreaInstall, Title: "Is wms.json correct?", Run: checkConfigFile},
		{ID: "ffmpeg", Area: AreaInstall, Title: "Is FFmpeg there, and does it run?", Run: checkFFmpeg},
		{ID: "folders", Area: AreaInstall, Title: "Can the server use its folders?", Run: checkFolders, Fix: fixFolders},
		{ID: "disk-space", Area: AreaInstall, Title: "Is there enough disk space?", Run: checkDiskSpace},
		{ID: "port", Area: AreaInstall, Title: "Is the server's port free?", Run: checkPort},
		{ID: "firewall", Area: AreaInstall, Title: "Will other devices be able to connect?", Run: checkFirewall},
		{ID: "leftover-processes", Area: AreaInstall, Title: "Is FFmpeg still running from a server that stopped?", Run: checkLeftoverFFmpeg, Fix: fixProcesses},
		{ID: "layout", Area: AreaInstall, Title: "Is the data folder in a safe place?", Run: checkLayout},
	}
}

// pathInside: p is in dir, or is dir. Case counts only where the system's does.
func pathInside(p, dir string) bool {
	if p == "" || dir == "" {
		return false
	}
	a, b := samePath(p), samePath(dir)
	if rel, err := filepath.Rel(b, a); err == nil {
		return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
	}
	return false
}

// --------------------------------------------------------------- wms.json

func checkConfigFile(_ context.Context, e *DoctorEnv) []Issue {
	var out []Issue
	if e.ConfigError != "" {
		name := e.ConfigFile
		if name == "" {
			name = "wms.json"
		}
		out = append(out, Issue{Key: "config-file:bad", Severity: SevProblem, Title: "wms.json can't be used, so the server won't start", Safety: SafetyManual,
			What: e.ConfigError,
			Fix:  "Correct the file, or rename it to use the defaults.",
			Steps: []string{
				"Open " + name + " in a text editor. It's JSON: names and text in double quotes, with a comma between entries and none after the last.",
				"Only the names in docs/CONFIG.md are allowed, so a misspelled one stops the server. Correct it or remove it.",
				"Or rename the file to wms.json.old, and the server starts with its defaults. wittymediaserver config init writes a new one.",
			}})
		return out
	}
	if e.ConfigFile == "" {
		return nil
	}
	f, err := config.LoadFile(e.ConfigFile)
	if err != nil {
		return out
	}
	base := filepath.Dir(e.ConfigFile)
	for _, c := range []struct{ name, val string }{{"ffmpeg", f.FFmpeg}, {"ffprobe", f.FFprobe}} {
		if c.val == "" {
			continue
		}
		p := c.val
		if !filepath.IsAbs(p) {
			p = filepath.Join(base, p)
		}
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			out = append(out, Issue{Key: "config-file:" + c.name, Severity: SevProblem, Title: "wms.json names a " + c.name + " that isn't there", Safety: SafetyManual,
				What:  c.name + " is set to " + c.val + ", and there's no such file.",
				Fix:   "Correct the path in " + e.ConfigFile + ", or remove the setting.",
				Steps: []string{"Open " + e.ConfigFile + " and correct or remove \"" + c.name + "\". Without it the server looks beside itself and on PATH."}})
		}
	}
	return out
}

// --------------------------------------------------------------- FFmpeg

func checkFFmpeg(_ context.Context, e *DoctorEnv) []Issue {
	type answer struct {
		t   *ffmpeg.Tools
		err error
	}
	got := make(chan answer, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				got <- answer{nil, fmt.Errorf("%v", p)}
			}
		}()
		t, err := ffmpeg.Locate(e.FFmpeg, e.FFprobe)
		got <- answer{t, err}
	}()
	var a answer
	select {
	case a = <-got:
	case <-time.After(30 * time.Second):
		a.err = errors.New("it started, and gave no answer for 30 seconds")
	}
	if a.err == nil && a.t.Version == "" {
		a.err = errors.New(a.t.FFmpeg + " is there, and doesn't run")
	}
	if a.err == nil {
		if _, err := os.Stat(a.t.FFprobe); err != nil {
			if _, err := exec.LookPath(a.t.FFprobe); err != nil {
				a.err = errors.New("FFmpeg is there, but ffprobe, which comes with it, isn't")
			}
		}
	}
	if a.err == nil {
		return nil
	}
	return []Issue{{Key: "ffmpeg", Severity: SevProblem, Title: "FFmpeg is missing or won't run", Safety: SafetyManual,
		What: "The server needs FFmpeg to start and to convert video. It says: " + a.err.Error() + ".",
		Fix:  "Install FFmpeg, or say where it is.", Steps: ffmpegSteps()}}
}

func ffmpegSteps() []string {
	last := "If it's somewhere else, set WMS_FFMPEG to its full path, or \"ffmpeg\" in wms.json. Then run wittymediaserver doctor again."
	switch runtime.GOOS {
	case "windows":
		return []string{"Reinstall WittyMediaServer, which brings its own copy; or install one: winget install Gyan.FFmpeg", "Open a new terminal and check that this works: ffmpeg -version", last}
	case "darwin":
		return []string{"Install it with Homebrew: brew install ffmpeg", "Check that this works: ffmpeg -version", last}
	case "linux":
		return []string{"Install it with your system's packages, such as: sudo apt install ffmpeg, or: sudo dnf install ffmpeg", "Check that this works: ffmpeg -version", last}
	}
	return []string{"Install FFmpeg with your system's packages, such as: pkg install ffmpeg", "Check that this works: ffmpeg -version", last}
}

// --------------------------------------------------------------- folders

type missingFolders struct{ dirs []string }

func checkFolders(_ context.Context, e *DoctorEnv) []Issue {
	var out []Issue
	var missing []string
	for _, f := range []struct{ name, dir string }{{"data", e.Data}, {"cache", e.Cache}, {"artwork", e.Images}, {"converted video", e.Transcode}, {"log", e.LogDir}} {
		if f.dir == "" {
			continue
		}
		st, err := os.Stat(f.dir)
		if errors.Is(err, fs.ErrNotExist) {
			missing = append(missing, f.dir)
			continue
		}
		if err == nil && !st.IsDir() {
			out = append(out, Issue{Key: "folders:" + f.name, Severity: SevProblem, Title: "The " + f.name + " folder is a file", Safety: SafetyManual,
				What: f.dir + " should be a folder, and it's a file.", Fix: "Move that file away, or choose another folder in wms.json.",
				Steps: []string{"Rename or move " + f.dir + ".", "Run wittymediaserver doctor again."}})
			continue
		}
		if err != nil {
			out = append(out, folderDenied(f.name, f.dir, err))
			continue
		}
		if err := tryWrite(f.dir); err != nil {
			out = append(out, folderDenied(f.name, f.dir, err))
		}
	}
	if len(missing) > 0 {
		out = append(out, Issue{Key: "folders:missing", Severity: SevWarn, Title: "Some of the server's folders are missing", Safety: SafetySafe, Count: len(missing), Details: missing, data: missingFolders{missing},
			What: countOf(len(missing), "folder doesn't", "folders don't") + " exist. If one was on a drive that's away, plug it in first.",
			Fix:  "Makes the folders, empty."})
	}
	return out
}

func tryWrite(dir string) error {
	f, err := os.CreateTemp(dir, ".wms-doctor-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

func folderDenied(name, dir string, err error) Issue {
	var steps []string
	switch runtime.GOOS {
	case "windows":
		steps = []string{"In File Explorer, right-click " + dir + ", then Properties, Security, and give the account that runs the server Modify.", "Or in a terminal: icacls \"" + dir + "\" /grant \"%USERNAME%:(OI)(CI)M\" /T"}
	case "darwin":
		steps = []string{"In a terminal: sudo chown -R \"$(whoami)\" \"" + dir + "\"", "Or in Finder, Get Info on the folder, and give your account Read & Write."}
	default:
		steps = []string{"In a terminal, as the account that runs the server: sudo chown -R \"$(whoami)\" \"" + dir + "\"", "For a service, use its user: sudo chown -R wms:wms \"" + dir + "\". Docker: check the volume's owner and PUID."}
	}
	return Issue{Key: "folders:denied:" + name, Severity: SevProblem, Title: "The server can't write to its " + name + " folder", Safety: SafetyManual,
		What:  "Writing to " + dir + " failed: " + err.Error() + ".",
		Fix:   "Change who may write there. This program won't change permissions by itself.",
		Steps: steps}
}

func fixFolders(_ context.Context, _ *DoctorEnv, i *Issue, _ *FixContext) (string, error) {
	m, ok := i.data.(missingFolders)
	if !ok {
		return "", errors.New("run the check again first")
	}
	for _, d := range m.dirs {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return "", err
		}
	}
	return countOf(len(m.dirs), "folder was", "folders were") + " made.", nil
}

// --------------------------------------------------------------- disk space

const (
	diskLow     = 5 << 30
	diskVeryLow = 1 << 30
	diskUnknown = 0
)

func checkDiskSpace(_ context.Context, e *DoctorEnv) []Issue {
	var out []Issue
	seen := map[string]bool{}
	for _, f := range []struct{ name, dir string }{{"data", e.Data}, {"converted video", e.Transcode}, {"artwork", e.Images}} {
		if f.dir == "" {
			continue
		}
		free := sysinfo.FreeBytes(f.dir)
		key := fmt.Sprint(free >> 20)
		if free <= diskUnknown || seen[key] {
			continue
		}
		seen[key] = true
		if free >= diskLow {
			continue
		}
		sev := SevWarn
		if free < diskVeryLow {
			sev = SevProblem
		}
		out = append(out, Issue{Key: "disk-space:" + f.name, Severity: sev, Title: "The disk holding the " + f.name + " folder is nearly full", Safety: SafetyManual,
			What: fmt.Sprintf("%s free where %s is. The database, artwork and converted video all need room, and a full disk can damage the database.", sizeWords(free), f.dir),
			Fix:  "Make room, or move a folder to a bigger drive.",
			Steps: []string{
				"Delete what you don't need from that drive. The safety copies in " + BackupsDir(e.Data) + " and copies under Settings, Reset can go.",
				"To move folders that fill up to another drive, set transcodeDir and metadataDir in wms.json (see docs/CONFIG.md).",
			}})
	}
	return out
}

// --------------------------------------------------------------- the port

func listenPort(listen string) (host string, port int, ok bool) {
	h, p, err := net.SplitHostPort(listen)
	if err != nil {
		return "", 0, false
	}
	n, err := strconv.Atoi(p)
	if err != nil || n <= 0 {
		return "", 0, false
	}
	return h, n, true
}

// whoAnswers asks whatever is on the port on this machine whether it's a server of ours. It
// only ever looks at this machine, and sends nothing but the request.
func whoAnswers(ctx context.Context, port int) (isOurs bool, version string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(port))+"/api/capabilities", nil)
	if err != nil {
		return false, ""
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return false, ""
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return false, ""
	}
	var c struct {
		Server  string `json:"server"`
		Version string `json:"version"`
	}
	if json.Unmarshal(b, &c) != nil || c.Server != "WittyMediaServer" {
		return false, ""
	}
	return true, c.Version
}

func checkPort(ctx context.Context, e *DoctorEnv) []Issue {
	if e.Listen == "" {
		return nil
	}
	_, port, ok := listenPort(e.Listen)
	if !ok {
		return []Issue{{Key: "port:address", Severity: SevProblem, Title: "The listen address can't be used", Safety: SafetyManual,
			What:  "The server is told to listen on \"" + e.Listen + "\", which isn't an address and a port.",
			Fix:   "Correct it, such as :9797.",
			Steps: []string{"Correct \"listen\" in wms.json, -listen or WMS_LISTEN. :9797 means every network card on port 9797."}}}
	}
	if e.InServer {
		return nil // that's this server
	}
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		if e.TryPort {
			// Only when asked, only on this machine, and let go of at once.
			l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				return portBusy(ctx, e, port, "Trying it says: "+err.Error()+".")
			}
			_ = l.Close()
		}
		return nil
	}
	_ = conn.Close()
	ours, ver := whoAnswers(ctx, port)
	if ours {
		if _, running := e.serverPID(); running && ver == e.Version {
			return nil
		}
		if ver != e.Version {
			return []Issue{{Key: "port:version", Severity: SevWarn, Title: "A server of a different version is already running", Safety: SafetyManual,
				What:  fmt.Sprintf("WittyMediaServer %s answers on port %d, and this program is %s. Running a command from one install against another's folder can change its files.", ver, port, e.Version),
				Fix:   "Stop the other server, or point this command at its folder.",
				Steps: []string{"If it's an older install, stop it, from its tray icon or service, before starting this one.", "If that's the install you meant, use its own program: run wittymediaserver config path beside it to see where it is."}}}
		}
		return []Issue{{Key: "port:other", Severity: SevWarn, Title: "Another copy of this server is using the port", Safety: SafetyManual,
			What:  fmt.Sprintf("A WittyMediaServer on a different data folder answers on port %d, so a server on this folder can't start there.", port),
			Fix:   "Stop the other copy, or give this one another port.",
			Steps: []string{"Stop the other copy, from its tray icon or service.", "Or start this one with: -listen :9798"}}}
	}
	return portBusy(ctx, e, port, "")
}

func portBusy(ctx context.Context, _ *DoctorEnv, port int, extra string) []Issue {
	who := portOwner(ctx, port)
	what := fmt.Sprintf("Something else on this computer is using port %d, so the server can't start there.", port)
	if who != "" {
		what += " It looks like: " + who + "."
	}
	if extra != "" {
		what += " " + extra
	}
	return []Issue{{Key: "port", Severity: SevProblem, Title: "The server's port is taken", Safety: SafetyManual, What: what,
		Fix:   "Stop that program, or give the server another port.",
		Steps: []string{"Stop the program named above, if it's not needed.", "Or give the server another port: -listen :9798, or \"listen\" in wms.json, and use that port in your browser."}}}
}

// --------------------------------------------------------------- firewall

func checkFirewall(_ context.Context, e *DoctorEnv) []Issue {
	host, port, ok := listenPort(e.Listen)
	if !ok {
		return nil
	}
	if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	var steps []string
	p := strconv.Itoa(port)
	switch runtime.GOOS {
	case "windows":
		steps = []string{
			"Open Windows Security, Firewall & network protection, Allow an app through firewall, and tick WittyMediaServer for Private networks.",
			"Or in a terminal opened as administrator: netsh advfirewall firewall add rule name=\"WittyMediaServer\" dir=in action=allow protocol=TCP localport=" + p + " profile=private",
			"Apps find the server over UDP ports 5353 and 9797, so allow those too if apps don't list it.",
		}
	case "darwin":
		steps = []string{"Open System Settings, Network, Firewall, Options, and allow wms to receive incoming connections.", "Apps find the server over UDP ports 5353 and 9797."}
	case "linux":
		steps = []string{
			"With ufw: sudo ufw allow " + p + "/tcp",
			"With firewalld: sudo firewall-cmd --add-port=" + p + "/tcp --permanent, then sudo firewall-cmd --reload",
			"Apps find the server over UDP ports 5353 and 9797. In Docker, use host networking for that to work.",
		}
	default:
		steps = []string{"Allow TCP port " + p + " in your firewall.", "Apps find the server over UDP ports 5353 and 9797."}
	}
	return []Issue{{Key: "firewall", Severity: SevInfo, Title: "If other devices can't connect, check the firewall", Safety: SafetyManual,
		What:  "This program can't tell what your firewall allows. If the server works on this computer and nowhere else, that's the likely reason.",
		Fix:   "Allow the server through. Nothing here changes firewall settings.",
		Steps: steps}}
}

// --------------------------------------------------------------- FFmpeg left running

// ffmpegsOf finds FFmpeg processes that are provably working for this server: their command
// line names a stream's folder in its conversion folder. That folder is the owner's choice and
// could be anywhere, so an FFmpeg that merely mentions it isn't enough.
func ffmpegsOf(list []procInfo, transcode string) []ownedProcess {
	var out []ownedProcess
	if transcode == "" {
		return out
	}
	prefix := strings.TrimRight(transcode, `/\`)
	for _, p := range list {
		base := strings.ToLower(filepath.Base(p.exe))
		if base != "ffmpeg" && base != "ffmpeg.exe" {
			continue
		}
		rest := p.cmdline
		for {
			i := strings.Index(rest, prefix)
			if i < 0 {
				break
			}
			rest = rest[i+len(prefix):]
			if rest == "" || (rest[0] != '/' && rest[0] != '\\') {
				continue
			}
			seg := rest[1:]
			if j := strings.IndexAny(seg, `/\ "'`); j >= 0 {
				seg = seg[:j]
			}
			if hls.IsSessionDir(seg) {
				out = append(out, ownedProcess{pid: p.pid, exe: p.exe, root: filepath.Join(transcode, seg)})
				break
			}
		}
	}
	return out
}

func checkLeftoverFFmpeg(ctx context.Context, e *DoctorEnv) []Issue {
	if e.ServerUp() {
		return nil // that server's conversions, and it looks after them
	}
	list, err := listProcesses(ctx)
	if err != nil {
		return notChecked("leftover-processes:failed", "FFmpeg left running", "The list of running programs couldn't be read: "+err.Error()+".")
	}
	found := ffmpegsOf(list, e.Transcode)
	if len(found) == 0 {
		return nil
	}
	var names []string
	for _, p := range found {
		names = append(names, fmt.Sprintf("process %d: %s", p.pid, filepath.Base(p.exe)))
	}
	return []Issue{{Key: "leftover-processes", Severity: SevWarn, Title: "FFmpeg is still converting for a server that stopped", Safety: SafetySafe, Count: len(found), Names: names, data: ourProcesses{found},
		What: countOf(len(found), "FFmpeg is", "FFmpegs are") + " running with this server's folders on the command line, and no server is running. They use the processor and hold files open.",
		Fix:  "Ends them. Only an FFmpeg naming this server's own folders is touched, and each is checked again just before."}}
}

// fixProcesses ends what the check proved to be ours, checking again just before, in case
// the process number has since gone to something else.
func fixProcesses(ctx context.Context, e *DoctorEnv, i *Issue, _ *FixContext) (string, error) {
	o, ok := i.data.(ourProcesses)
	if !ok {
		return "", errors.New("run the check again first")
	}
	if e.ServerUp() {
		return "", errors.New("a server has started since the check, so these may be its own. Nothing was ended")
	}
	list, err := listProcesses(ctx)
	if err != nil {
		return "", err
	}
	byPID := map[int]procInfo{}
	for _, p := range list {
		byPID[p.pid] = p
	}
	ended := 0
	for _, p := range o.list {
		now, there := byPID[p.pid]
		if !there || now.exe != p.exe {
			continue
		}
		if !pathInside(now.exe, p.root) && !strings.Contains(now.cmdline, p.root) {
			continue
		}
		proc, err := os.FindProcess(p.pid)
		if err != nil {
			continue
		}
		if err := proc.Kill(); err != nil {
			return "", fmt.Errorf("process %d couldn't be ended: %w", p.pid, err)
		}
		ended++
	}
	return countOf(ended, "process was", "processes were") + " ended.", nil
}

// --------------------------------------------------------------- layout

func checkLayout(_ context.Context, e *DoctorEnv) []Issue {
	if e.Program == "" || e.Portable || !pathInside(e.Data, e.Program) {
		return nil
	}
	return []Issue{{Key: "layout:inside-program", Severity: SevWarn, Title: "The data folder is inside the program's folder", Safety: SafetyManual,
		What: "Data at " + e.Data + " sits in with the program at " + e.Program + ". Reinstalling or uninstalling can remove it.",
		Fix:  "Move the data somewhere of its own, or make this a portable install on purpose.",
		Steps: []string{
			"Stop the server, and move the folder somewhere outside the program's folder.",
			"Point the server at it with dataDir in wms.json, or -data.",
			"Or, if you do want it all in one folder, run wittymediaserver config init --portable.",
		}}}
}
