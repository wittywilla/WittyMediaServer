// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
	"github.com/wittywilla/wittymediaserver/web"
)

// wittyinfo edit: the server's own editor, served from this program for one file on this
// computer. The page talks to the same routes as the server's (/api/wittyinfo/file/0/...),
// so one editor serves both. Only 127.0.0.1 listens; a key in the address the program
// prints becomes a cookie, and every request without it is refused, as is any request
// for another host name (a web page can't reach it by renaming itself).

type editor struct {
	mu    sync.Mutex
	file  string // the .wittyinfo
	video string // the video it is for, "" when not found
	key   string
	host  string
	dist  fs.FS
}

var videoExts = []string{".mkv", ".mp4", ".m4v", ".webm", ".mov", ".avi", ".ts", ".m2ts", ".wmv", ".mpg"}

// videoBeside finds the video a .wittyinfo is for.
func videoBeside(file string) string {
	base := strings.TrimSuffix(file, filepath.Ext(file))
	for _, ext := range videoExts {
		for _, p := range []string{base + ext, base + strings.ToUpper(ext)} {
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	if strings.HasSuffix(strings.ToLower(base), ".mkv") || strings.HasSuffix(strings.ToLower(base), ".mp4") {
		if st, err := os.Stat(base); err == nil && !st.IsDir() {
			return base // Film.mkv.wittyinfo
		}
	}
	return ""
}

func cmdEdit(args []string) error {
	fset := flag.NewFlagSet("edit", flag.ExitOnError)
	port := fset.Int("port", 0, "the port to listen on (0: any free one)")
	noOpen := fset.Bool("no-open", false, "don't open a browser; only print the address")
	fset.StringVar(&ffprobeFlag, "ffprobe", "", "the ffprobe program to use")
	fset.Parse(args)
	if fset.NArg() != 1 {
		return errors.New("usage: wittyinfo edit <video or .wittyinfo>")
	}
	target, err := filepath.Abs(fset.Arg(0))
	if err != nil {
		return err
	}
	e := &editor{dist: web.Dist()}
	if isInfo(target) {
		e.file, e.video = target, videoBeside(target)
	} else {
		if _, err := os.Stat(target); err != nil {
			return err
		}
		e.file, e.video = sidecarFor(target), target
	}
	var k [24]byte
	_, _ = rand.Read(k[:])
	e.key = fmt.Sprintf("%x", k)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return err
	}
	e.host = ln.Addr().String()
	url := "http://" + e.host + "/wittyinfo-editor?key=" + e.key
	fmt.Println("Editing", e.file)
	if e.video != "" {
		fmt.Println("For", e.video)
	}
	fmt.Println("Open this address in a browser on this computer:")
	fmt.Println("  ", url)
	fmt.Println("Press Ctrl+C to stop.")
	if !*noOpen {
		openBrowser(url)
	}
	srv := &http.Server{Handler: e, ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(ln)
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	_ = c.Start()
}

const keyCookie = "wittyinfo_key"

func (e *editor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only this computer's own address, by the name it was given.
	if r.Host != e.host {
		http.Error(w, "wrong host", http.StatusMisdirectedRequest)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	if k := r.URL.Query().Get("key"); k != "" {
		if subtle.ConstantTimeCompare([]byte(k), []byte(e.key)) != 1 {
			http.Error(w, "wrong key", http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: keyCookie, Value: e.key, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
	c, err := r.Cookie(keyCookie)
	if err != nil || subtle.ConstantTimeCompare([]byte(c.Value), []byte(e.key)) != 1 {
		http.Error(w, "Open the address wittyinfo printed, key and all.", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+e.host {
			http.Error(w, "wrong origin", http.StatusForbidden)
			return
		}
	}
	p := r.URL.Path
	switch {
	case strings.HasPrefix(p, "/api/wittyinfo/file/0"):
		e.api(w, r, strings.TrimPrefix(p, "/api/wittyinfo/file/0"))
	case strings.HasPrefix(p, "/api/"):
		// The page asks the server it thinks it is on about other things: there is none.
		writeJSON(w, 404, map[string]string{"error": "not in the wittyinfo program"})
	default:
		e.static(w, r)
	}
}

// static serves the web app, unpacking what was packed when the browser can't take it.
func (e *editor) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || !strings.Contains(path.Base(name), ".") {
		name = "index.html"
	}
	b, err := fs.ReadFile(e.dist, name)
	if err != nil {
		gz, gerr := fs.ReadFile(e.dist, name+".gz")
		if gerr != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		b = gz
	}
	ct := map[string]string{".html": "text/html; charset=utf-8", ".js": "text/javascript; charset=utf-8", ".mjs": "text/javascript; charset=utf-8",
		".css": "text/css; charset=utf-8", ".svg": "image/svg+xml", ".png": "image/png", ".json": "application/json", ".woff2": "font/woff2",
		".wasm": "application/wasm", ".ico": "image/x-icon", ".webmanifest": "application/manifest+json"}[path.Ext(name)]
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeInfoErr(w http.ResponseWriter, err error) {
	var e *wittyinfo.Error
	if errors.As(err, &e) {
		writeJSON(w, 400, map[string]any{"error": e.Error(), "problems": e.Problems})
		return
	}
	writeJSON(w, 400, map[string]string{"error": err.Error()})
}

// answer is the same shape as the server's GET /api/wittyinfo/... (docs/CLIENT-API.md).
type answer struct {
	Kind       string              `json:"kind"`
	ID         int                 `json:"id"`
	Info       *wittyinfo.Info     `json:"info"`
	Origin     string              `json:"origin"`
	Path       string              `json:"path"`
	FileFound  bool                `json:"fileFound"`
	FileError  string              `json:"fileError,omitempty"`
	Problems   []wittyinfo.Problem `json:"problems,omitempty"`
	Match      *wittyinfo.Match    `json:"match,omitempty"`
	Force      bool                `json:"force"`
	Files      []map[string]any    `json:"files"`
	CanWrite   bool                `json:"canWrite"`
	Identity   *wittyinfo.Identity `json:"identity,omitempty"`
	Chapters   []probeChapter      `json:"chapters,omitempty"`
	Duration   float64             `json:"durationSec,omitempty"`
	Video      string              `json:"video,omitempty"`
	VideoName  string              `json:"videoName,omitempty"`
	Standalone bool                `json:"standalone"`
	Markers    any                 `json:"markers,omitempty"`
	Defaults   map[string]any      `json:"defaults,omitempty"`
}

// load reads the file as it is on disk now.
func (e *editor) load() (*wittyinfo.Info, error, bool) {
	st, err := os.Stat(e.file)
	if err != nil {
		return nil, nil, false
	}
	if st.Size() > wittyinfo.MaxFile {
		return nil, fmt.Errorf("larger than %d MB", wittyinfo.MaxFile>>20), true
	}
	b, err := os.ReadFile(e.file)
	if err != nil {
		return nil, err, true
	}
	x, err := wittyinfo.Parse(b)
	return x, err, true
}

func (e *editor) answer() answer {
	a := answer{Kind: "file", Path: e.file, CanWrite: true, Files: []map[string]any{}, Standalone: true}
	x, err, found := e.load()
	a.FileFound = found
	if err != nil {
		a.FileError = err.Error()
		var we *wittyinfo.Error
		if errors.As(err, &we) {
			a.Problems = we.Problems
		}
	}
	if x != nil {
		a.Info, a.Origin = x, "file"
		for _, name := range sortedKeys(x.Files) {
			f := map[string]any{"name": name, "size": len(x.Files[name])}
			if strings.HasPrefix(name, "images/") {
				f["url"] = "/api/wittyinfo/file/0/files/" + name
			}
			a.Files = append(a.Files, f)
		}
	}
	if e.video != "" {
		dur, chapters := probe(e.video)
		a.Duration, a.Chapters = dur, chapters
		a.Video, a.VideoName = "/api/wittyinfo/file/0/video", filepath.Base(e.video)
		if x != nil && x.ScopeOr() == wittyinfo.ScopeItem {
			var fp *wittyinfo.Fingerprint
			if f, _, err := wittyinfo.FingerprintFile(e.video); err == nil {
				fp = f
			}
			m := x.MatchVideo(dur, fp)
			a.Match = &m
		}
		if x == nil || x.Identity == nil {
			if id, err := identityOf(e.video); err == nil && id.DurationSec > 0 {
				a.Identity = id
			}
		}
	}
	return a
}

// current is the file to change: the one on disk, or a new one.
func (e *editor) current() *wittyinfo.Info {
	if x, err, _ := e.load(); x != nil && err == nil {
		return x
	}
	scope := wittyinfo.ScopeItem
	switch b := strings.ToLower(filepath.Base(e.file)); {
	case b == "show"+wittyinfo.Ext:
		scope = wittyinfo.ScopeShow
	case strings.HasPrefix(b, "season"):
		scope = wittyinfo.ScopeSeason
	}
	x := wittyinfo.New(scope)
	if e.video != "" {
		if id, err := identityOf(e.video); err == nil && id.DurationSec > 0 {
			x.Identity = id
		}
	}
	return x
}

func (e *editor) save(w http.ResponseWriter, x *wittyinfo.Info) bool {
	if old, err, found := e.load(); found && err != nil && old == nil {
		// Someone's file that can't be read: keep it before replacing it.
		if b, rerr := os.ReadFile(e.file); rerr == nil {
			_ = os.WriteFile(e.file+".bak", b, 0o644)
		}
	}
	if err := writeInfo(e.file, x); err != nil {
		writeInfoErr(w, err)
		return false
	}
	return true
}

func (e *editor) api(w http.ResponseWriter, r *http.Request, rest string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case rest == "" && r.Method == http.MethodGet:
		writeJSON(w, 200, e.answer())
	case rest == "" && r.Method == http.MethodPut:
		var b struct {
			Info json.RawMessage `json:"info"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, wittyinfo.MaxManifest+1024)).Decode(&b); err != nil || len(b.Info) == 0 {
			writeJSON(w, 400, map[string]string{"error": "expected {\"info\": the manifest}"})
			return
		}
		x := &wittyinfo.Info{}
		if err := json.Unmarshal(b.Info, x); err != nil {
			writeInfoErr(w, err)
			return
		}
		cur := e.current()
		x.Files, x.Scope, x.Season = cur.Files, cur.Scope, cur.Season
		if e.save(w, x) {
			writeJSON(w, 200, e.answer())
		}
	case rest == "" && r.Method == http.MethodDelete:
		if _, err := os.Stat(e.file); err == nil {
			b, _ := os.ReadFile(e.file)
			_ = os.WriteFile(e.file+".bak", b, 0o644)
			_ = os.Remove(e.file)
		}
		writeJSON(w, 200, e.answer())
	case rest == "/export" && r.Method == http.MethodGet:
		x := e.current()
		b, err := x.Encode()
		if err != nil {
			writeInfoErr(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.wittyinfo+zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(filepath.Base(e.file), `"`, "")+`"`)
		_, _ = w.Write(b)
	case rest == "/import" && r.Method == http.MethodPost:
		b, err := io.ReadAll(io.LimitReader(r.Body, wittyinfo.MaxFile+1))
		if err != nil || len(b) > wittyinfo.MaxFile {
			writeJSON(w, 413, map[string]string{"error": "A .wittyinfo can be 64 MB at most."})
			return
		}
		x, err := wittyinfo.Parse(b)
		if err != nil {
			writeInfoErr(w, err)
			return
		}
		if e.save(w, x) {
			writeJSON(w, 200, e.answer())
		}
	case rest == "/use-anyway":
		writeJSON(w, 200, e.answer()) // nothing to remember: it is one file, used as it is
	case strings.HasPrefix(rest, "/images/"):
		role := strings.TrimPrefix(rest, "/images/")
		x := e.current()
		switch r.Method {
		case http.MethodPut:
			b, err := io.ReadAll(io.LimitReader(r.Body, wittyinfo.MaxImage+1))
			if err != nil || len(b) > wittyinfo.MaxImage {
				writeJSON(w, 413, map[string]string{"error": "A picture can be 16 MB at most."})
				return
			}
			ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[http.DetectContentType(b)]
			if ext == "" {
				writeJSON(w, 400, map[string]string{"error": "Send a JPEG, PNG or WebP picture."})
				return
			}
			if _, err := x.SetImage(role, ext, b); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
		case http.MethodDelete:
			delete(x.Artwork, role)
		default:
			http.Error(w, "method", 405)
			return
		}
		if e.save(w, x) {
			writeJSON(w, 200, e.answer())
		}
	case strings.HasPrefix(rest, "/files/images/") && r.Method == http.MethodGet:
		name := strings.TrimPrefix(rest, "/files/")
		x, _, _ := e.load()
		ext := strings.ToLower(filepath.Ext(name))
		if x == nil || x.Files[name] == nil || wittyinfo.ImageExts[ext] == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/"+wittyinfo.ImageExts[ext])
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		_, _ = io.Copy(w, bytes.NewReader(x.Files[name]))
	case rest == "/video" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		if e.video == "" {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(e.video)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", videoType(e.video))
		http.ServeContent(w, r, "", st.ModTime(), f)
	default:
		writeJSON(w, 404, map[string]string{"error": "not found"})
	}
}

func videoType(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mkv":
		return "video/x-matroska"
	case ".mov":
		return "video/quicktime"
	}
	return "application/octet-stream"
}
