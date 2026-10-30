// Package bdj runs a disc's BD-J applications. They are Java programs, so they
// need a Java runtime, which this project does not ship: the server's owner
// installs any Java 8 or newer they choose, under that Java's own licence, and
// this package starts it as a separate program. What is ours is the small
// platform (runtime.jar) that disc applications link against, drawing into an
// off-screen picture and talking to the server over a pipe.
//
// A disc is untrusted input, and here it is also code. The runtime loads a
// disc's classes through a default-deny list, with no network, processes, native
// code or reflection and file access only to the disc's own areas; the process
// has a memory limit and dies with the session; and it stays off until an
// administrator turns it on.
package bdj

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittybluray"
)

//go:embed runtime.jar
var runtimeJar []byte

// Frame size of the graphics plane.
const (
	Width  = 1920
	Height = 1080
)

// Java is a Java runtime found on this machine.
type Java struct {
	Path    string
	Version string
}

// Find looks for Java: the path given (the setting), then JAVA_HOME, then the
// PATH, then the usual install folders. The result is checked by Probe.
func Find(explicit string) (*Java, error) {
	exe := "java"
	if runtime.GOOS == "windows" {
		exe = "java.exe"
	}
	var tried []string
	if explicit != "" {
		p := explicit
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			p = filepath.Join(p, "bin", exe)
		}
		tried = append(tried, p)
	} else {
		if h := os.Getenv("JAVA_HOME"); h != "" {
			tried = append(tried, filepath.Join(h, "bin", exe))
		}
		if p, err := exec.LookPath("java"); err == nil {
			tried = append(tried, p)
		}
		for _, pat := range installGlobs() {
			m, _ := filepath.Glob(pat)
			for i := len(m) - 1; i >= 0; i-- {
				tried = append(tried, filepath.Join(m[i], "bin", exe))
			}
		}
	}
	for _, p := range tried {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return &Java{Path: p}, nil
		}
	}
	return nil, errors.New("no Java found: install any Java 8 or newer (an OpenJDK build such as Eclipse Temurin), with its graphics libraries")
}

func installGlobs() []string {
	switch runtime.GOOS {
	case "windows":
		var out []string
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
			if base != "" {
				for _, v := range []string{"Java", "Eclipse Adoptium", "Microsoft", "Amazon Corretto", "Zulu", "BellSoft"} {
					out = append(out, filepath.Join(base, v, "*"))
				}
			}
		}
		return out
	case "darwin":
		return []string{"/Library/Java/JavaVirtualMachines/*/Contents/Home"}
	}
	return []string{"/usr/lib/jvm/*", "/usr/java/*", "/opt/java/*"}
}

// writeRuntime puts runtime.jar where Java can read it.
func writeRuntime(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, "wittybluray-bdj.jar")
	if st, err := os.Stat(p); err == nil && st.Size() == int64(len(runtimeJar)) {
		if b, err := os.ReadFile(p); err == nil && bytes.Equal(b, runtimeJar) {
			return p, nil
		}
	}
	return p, os.WriteFile(p, runtimeJar, 0o644)
}

// jvmArgs keep the runtime small: BD-J was designed for players with very
// little memory, and a media server has other work to do.
func jvmArgs(jar string) []string {
	return []string{"-Xmx192m", "-Xss512k", "-XX:+UseSerialGC", "-XX:TieredStopAtLevel=1", "-XX:+ExitOnOutOfMemoryError", "-Xshare:auto",
		"-Djava.awt.headless=true", "-Dfile.encoding=UTF-8", "-Djavax.xml.accessExternalDTD=", "-Djavax.xml.accessExternalSchema=", "-cp", jar}
}

// Probe checks that a Java can run the platform (a stripped-down runtime
// without graphics cannot), and reports its version.
func (j *Java) Probe(ctx context.Context, cacheDir string) (string, error) {
	jar, err := writeRuntime(cacheDir)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, j.Path, append(jvmArgs(jar), "witty.bdj.Main", ".", ".", ".", "probe")...)
	hide(cmd)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("this Java cannot run BD-J (%v): %s", err, lastLine(errb.String()))
	}
	line := strings.TrimSpace(out.String())
	if !strings.HasPrefix(line, "ok ") {
		return "", fmt.Errorf("this Java cannot run BD-J: %s", lastLine(errb.String()))
	}
	j.Version = strings.TrimPrefix(line, "ok ")
	return j.Version, nil
}

func lastLine(s string) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > 0 {
		return strings.TrimSpace(l[len(l)-1])
	}
	return ""
}

// Event is something the applications asked for.
type Event struct {
	Kind string // play, stop, pause, resume, seek, rate, video, sound, psr, gpr, title, appstarted, appfailed, appdestroyed, exit
	Args []string
}

// Options start a host.
type Options struct {
	Java *Java
	// CacheDir holds the runtime's jar. WorkDir is this session's: the disc's
	// program files and temporary area, removed on Close. PersistDir is the
	// disc's storage for high scores and bookmarks, kept between sessions.
	CacheDir, WorkDir, PersistDir string
	OnEvent                       func(Event)
	Log                           func(string)
}

// Host is one running Java process with one disc's applications.
type Host struct {
	o    Options
	cmd  *exec.Cmd
	done chan struct{}
	// Lines to the runtime go through a queue and their own goroutine: nothing
	// that holds a lock ever waits on a pipe a disc's code could leave full.
	out  chan string
	stop chan struct{}
	once sync.Once

	fmu     sync.Mutex
	frame   *image.NRGBA
	version int
	changes int
	waiters []chan struct{}
	png     []byte
	pngVer  int
}

// Start exports the disc's program files and starts Java. ready returns when
// the runtime answers.
func Start(ctx context.Context, d *wittybluray.Disc, o Options) (*Host, error) {
	if o.Java == nil {
		return nil, errors.New("no Java")
	}
	jar, err := writeRuntime(o.CacheDir)
	if err != nil {
		return nil, err
	}
	vfs, tmp := filepath.Join(o.WorkDir, "vfs"), filepath.Join(o.WorkDir, "tmp")
	for _, p := range []string{vfs, tmp, o.PersistDir} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return nil, err
		}
	}
	if err := d.ExportProgram(vfs, 1<<30); err != nil {
		return nil, fmt.Errorf("reading the disc's program: %w", err)
	}
	args := append(jvmArgs(jar), "witty.bdj.Main", vfs, o.PersistDir, tmp)
	cmd := exec.Command(o.Java.Path, args...)
	hide(cmd)
	cmd.Dir = tmp
	cmd.Env = minimalEnv()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	h := &Host{o: o, cmd: cmd, done: make(chan struct{}), out: make(chan string, 8192), stop: make(chan struct{}), frame: image.NewNRGBA(image.Rect(0, 0, Width, Height))}
	go h.drainLog(stderr)
	go h.write(stdin)
	ready := make(chan struct{})
	go h.read(stdout, ready)
	select {
	case <-ready:
		return h, nil
	case <-h.done:
		return nil, errors.New("the Java runtime exited at once")
	case <-ctx.Done():
		h.Close()
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		h.Close()
		return nil, errors.New("the Java runtime did not start in time")
	}
}

// minimalEnv is what Java needs and nothing else: a disc's code never sees the
// server's environment anyway, and this keeps secrets out of the process too.
func minimalEnv() []string {
	var out []string
	for _, k := range []string{"PATH", "SystemRoot", "TEMP", "TMP", "HOME", "LANG", "LD_LIBRARY_PATH", "FONTCONFIG_PATH", "FONTCONFIG_FILE"} {
		if v, ok := os.LookupEnv(k); ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func (h *Host) write(w io.WriteCloser) {
	defer w.Close()
	for {
		select {
		case line := <-h.out:
			if _, err := io.WriteString(w, line+"\n"); err != nil {
				return
			}
		case <-h.stop:
			_, _ = io.WriteString(w, "quit\n")
			return
		}
	}
}

func (h *Host) logf(s string) {
	if h.o.Log != nil {
		h.o.Log(s)
	}
}

func (h *Host) drainLog(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4096), 64<<10)
	for sc.Scan() {
		h.logf(sc.Text())
	}
}

// read handles everything the runtime sends. It is the trust boundary on this
// side: a disc controls what comes down this pipe, so sizes are checked before
// anything is read, and nonsense ends the process.
func (h *Host) read(r io.Reader, ready chan struct{}) {
	defer func() {
		h.Close()
		close(h.done)
		h.notify()
		if h.o.OnEvent != nil {
			h.o.OnEvent(Event{Kind: "exit"})
		}
	}()
	br := bufio.NewReaderSize(r, 1<<16)
	var row []byte
	readyDone := false
	for {
		line, err := readLine(br)
		if err != nil {
			return
		}
		w := strings.Split(line, " ")
		switch w[0] {
		case "ready":
			if !readyDone {
				readyDone = true
				close(ready)
			}
		case "log":
			h.logf(strings.TrimPrefix(line, "log "))
		case "frame":
			if len(w) != 5 {
				h.logf("bad frame message")
				return
			}
			x, _ := strconv.Atoi(w[1])
			y, _ := strconv.Atoi(w[2])
			fw, _ := strconv.Atoi(w[3])
			fh, _ := strconv.Atoi(w[4])
			if x < 0 || y < 0 || fw <= 0 || fh <= 0 || x+fw > Width || y+fh > Height {
				h.logf("bad frame size")
				return
			}
			if cap(row) < fw*4 {
				row = make([]byte, fw*4)
			}
			row = row[:fw*4]
			for j := 0; j < fh; j++ {
				if _, err := io.ReadFull(br, row); err != nil {
					return
				}
				h.fmu.Lock()
				o := h.frame.PixOffset(x, y+j)
				px := h.frame.Pix[o : o+fw*4]
				for i := 0; i < fw; i++ { // ARGB to RGBA, not premultiplied in either
					px[i*4], px[i*4+1], px[i*4+2], px[i*4+3] = row[i*4+1], row[i*4+2], row[i*4+3], row[i*4]
				}
				h.fmu.Unlock()
			}
		case "flush":
			h.fmu.Lock()
			h.version++
			h.changes++
			h.fmu.Unlock()
			h.notify()
		default:
			if h.o.OnEvent != nil {
				h.o.OnEvent(Event{Kind: w[0], Args: w[1:]})
			}
		}
	}
}

// readLine reads one line of at most 64 KB.
func readLine(br *bufio.Reader) (string, error) {
	b, err := br.ReadSlice('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\r\n"), nil
}

func (h *Host) notify() {
	h.fmu.Lock()
	w := h.waiters
	h.waiters = nil
	h.fmu.Unlock()
	for _, c := range w {
		close(c)
	}
}

// Version counts the frames so far.
func (h *Host) Version() int {
	h.fmu.Lock()
	defer h.fmu.Unlock()
	return h.version
}

// Kick wakes whoever waits in WaitChange: something other than the picture changed.
func (h *Host) Kick() {
	h.fmu.Lock()
	h.changes++
	h.fmu.Unlock()
	h.notify()
}

// WaitChange returns a change count newer than since, or the current one when
// the wait runs out.
func (h *Host) WaitChange(ctx context.Context, since int, max time.Duration) int {
	t := time.NewTimer(max)
	defer t.Stop()
	for {
		h.fmu.Lock()
		v := h.changes
		if v != since {
			h.fmu.Unlock()
			return v
		}
		c := make(chan struct{})
		h.waiters = append(h.waiters, c)
		h.fmu.Unlock()
		select {
		case <-c:
		case <-t.C:
			return since
		case <-ctx.Done():
			return since
		case <-h.done:
			return since
		}
	}
}

var framePNG = png.Encoder{CompressionLevel: png.BestSpeed}

// PNG is the graphics plane as it stands now.
func (h *Host) PNG() ([]byte, int) {
	h.fmu.Lock()
	defer h.fmu.Unlock()
	if h.png != nil && h.pngVer == h.version {
		return h.png, h.version
	}
	var b bytes.Buffer
	_ = framePNG.Encode(&b, h.frame)
	h.png, h.pngVer = b.Bytes(), h.version
	return h.png, h.version
}

// Pixel reads one pixel of the plane (for tests).
func (h *Host) Pixel(x, y int) (r, g, b, a uint8) {
	h.fmu.Lock()
	defer h.fmu.Unlock()
	o := h.frame.PixOffset(x, y)
	p := h.frame.Pix[o : o+4]
	return p[0], p[1], p[2], p[3]
}

// Send queues one line for the runtime.
func (h *Host) Send(format string, args ...any) {
	select {
	case h.out <- fmt.Sprintf(format, args...):
	default:
		h.logf("the link to the BD-J runtime is full; a message was lost")
	}
}

// Sync sends every register, so the applications start from the player's state.
func (h *Host) Sync(r *wittybluray.Registers) {
	for i, v := range r.PSR {
		h.Send("psr %d %d", i, v)
	}
	for i, v := range r.GPR {
		if v != 0 {
			h.Send("gpr %d %d", i, v)
		}
	}
}

// StartTitle ends what is running and starts a title's applications.
func (h *Host) StartTitle(title int, o *wittybluray.BDJObject) {
	h.Send("stopapps")
	h.Send("title %d", title)
	for _, c := range o.Cache {
		if safe(c.Name) {
			h.Send("cache %s", c.Name)
		}
	}
	for _, a := range o.Apps {
		if !safe(a.BaseDir) {
			continue
		}
		line := fmt.Sprintf("app %d %d %d %d %s %s", a.Org, a.ID, a.Control, a.Priority, a.BaseDir, url.QueryEscape(a.InitialClass))
		for _, p := range a.Params {
			line += " " + url.QueryEscape(p)
		}
		h.Send("%s", line)
	}
	h.Send("start")
}

func safe(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// Done is closed when the process has ended.
func (h *Host) Done() <-chan struct{} { return h.done }

// Close ends the process and removes the session's files.
func (h *Host) Close() {
	h.once.Do(func() {
		close(h.stop)
		if h.cmd.Process != nil {
			go func() {
				time.Sleep(500 * time.Millisecond)
				_ = h.cmd.Process.Kill()
				_ = h.cmd.Wait()
				_ = os.RemoveAll(h.o.WorkDir)
			}()
		}
	})
}
