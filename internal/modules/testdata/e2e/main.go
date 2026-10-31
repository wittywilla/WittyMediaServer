// A run of the whole module system from the outside, against a real server. It builds
// the server with its test-only feed, publishes signed releases on a fake GitHub on this
// machine with a throwaway key, and drives everything through the HTTP API: updating,
// a version that keeps crashing and rolls back by itself, going back to an older version,
// pinning, restarts, withdrawing, working offline, a fork, and taking a module off.
//
//	go run ./internal/modules/testdata/e2e -work <empty folder>
//
// Nothing here touches anything outside that folder.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymodule"
)

const (
	serverVersion = "0.4.0-alpha"
	id            = "wittyphotos"
)

var failures []string

func step(format string, a ...any) { fmt.Printf("\n== "+format+"\n", a...) }

func ok(cond bool, format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	if cond {
		fmt.Println("   ok:", msg)
		return
	}
	fmt.Println("   FAILED:", msg)
	failures = append(failures, msg)
}

func must(err error) {
	if err != nil {
		fmt.Println("FATAL:", err)
		panic(err)
	}
}

// ---------------------------------------------------------------- the fake GitHub

type feed struct {
	mu    sync.Mutex
	base  string
	repos map[string][]update.GHRelease
	files map[string][]byte
	gone  bool
}

func (f *feed) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.gone {
		http.Error(w, "offline", 503)
		return
	}
	if name, found := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/feed/"), ".json"); found && strings.HasPrefix(r.URL.Path, "/feed/") {
		rels, ok := f.repos[name]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, _ := json.Marshal(rels)
		tag := fmt.Sprintf(`"%x"`, sha256.Sum256(b))
		w.Header().Set("ETag", tag)
		if r.Header.Get("If-None-Match") == tag {
			w.WriteHeader(304)
			return
		}
		_, _ = w.Write(b)
		return
	}
	b, ok := f.files[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(b)
}

type asset = struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"browser_download_url"`
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func packed(top string, files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&buf)
		for _, n := range names {
			w, _ := zw.Create(top + "/" + n)
			_, _ = w.Write(files[n])
		}
		_ = zw.Close()
		return buf.Bytes()
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range names {
		_ = tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(files[n]))})
		_, _ = tw.Write(files[n])
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// publish adds a signed release of the fake helper to a repository on the fake GitHub.
func (f *feed) publish(repoName, ver string, key ed25519.PrivateKey, helper []byte, test ...string) {
	man := wittymodule.Manifest{ID: id, Name: "WittyPhotos", Version: ver, Repo: "wittywilla/" + repoName, Delivery: wittymodule.DeliveryHelper,
		Entry: "wittyphotos-module", API: 1, MinServer: serverVersion, MaxServer: "0.5.x"}
	if len(test) > 0 {
		man.Capabilities = map[string][]string{"test": test}
	}
	raw, _ := json.Marshal(man)
	entry := "wittyphotos-module"
	if runtime.GOOS == "windows" {
		entry += ".exe"
	}
	top := wittymodule.TopFolder(id, ver, runtime.GOOS, runtime.GOARCH)
	files := map[string][]byte{wittymodule.ManifestFile: raw, entry: helper}
	archive := packed(top, files)
	archiveName := wittymodule.ArchiveName(id, ver, runtime.GOOS, runtime.GOARCH)
	manName := wittymodule.ManifestName(id, ver)
	var list strings.Builder
	fmt.Fprintf(&list, "%s  %s\n%s  %s\n", hash(archive), archiveName, hash(raw), manName)
	for n, b := range files {
		fmt.Fprintf(&list, "%s  %s/%s\n", hash(b), top, n)
	}
	sums := []byte(list.String())
	sumsName := wittymodule.SumsName(id, ver)
	g := update.GHRelease{TagName: "v" + ver, Name: "WittyPhotos " + ver, Body: "What changed in " + ver, HTMLURL: "https://github.com/wittywilla/" + repoName + "/releases/tag/v" + ver, PublishedAt: time.Now()}
	f.mu.Lock()
	defer f.mu.Unlock()
	for n, b := range map[string][]byte{archiveName: archive, manName: raw, sumsName: sums, sumsName + ".sig": update.Sign(key, sums)} {
		path := "/files/" + repoName + "/" + ver + "/" + n
		f.files[path] = b
		g.Assets = append(g.Assets, asset{n, int64(len(b)), f.base + path})
	}
	f.repos[repoName] = append([]update.GHRelease{g}, f.repos[repoName]...)
}

// withdraw publishes a signed list.
func (f *feed) withdraw(repoName string, serial int, key ed25519.PrivateKey, vs ...wittymodule.Withdrawn) {
	b, _ := json.Marshal(wittymodule.Withdrawals{Module: id, Serial: serial, Versions: vs})
	name := wittymodule.WithdrawalsName(id)
	g := update.GHRelease{TagName: wittymodule.WithdrawalsTag, Name: "Withdrawn versions"}
	f.mu.Lock()
	defer f.mu.Unlock()
	for n, data := range map[string][]byte{name: b, name + ".sig": update.Sign(key, b)} {
		path := "/files/" + repoName + "/withdrawals/" + n
		f.files[path] = data
		g.Assets = append(g.Assets, asset{n, int64(len(data)), f.base + path})
	}
	kept := f.repos[repoName][:0:0]
	for _, r := range f.repos[repoName] {
		if r.TagName != wittymodule.WithdrawalsTag {
			kept = append(kept, r)
		}
	}
	f.repos[repoName] = append(kept, g)
}

// ---------------------------------------------------------------- the server

type server struct {
	bin, data, addr, keyB64, feedURL, token string
	cmd                                     *exec.Cmd
	logf                                    *os.File
}

func (s *server) start() {
	cmd := exec.Command(s.bin)
	cmd.Env = append(os.Environ(), "WMS_DATA_DIR="+s.data, "WMS_LISTEN="+s.addr, "WMS_NO_BROWSER=1", "WMS_UPDATE_KEY="+s.keyB64, "WMS_MODULE_FEED="+s.feedURL, "WMS_LOG_DIR="+filepath.Join(s.data, "logs"))
	var err error
	s.logf, err = os.OpenFile(filepath.Join(filepath.Dir(s.data), "server.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	must(err)
	cmd.Stdout, cmd.Stderr = s.logf, s.logf
	must(cmd.Start())
	s.cmd = cmd
	for i := 0; i < 900; i++ {
		if res, err := http.Get("http://" + s.addr + "/api/health"); err == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	must(fmt.Errorf("the server never answered; see %s", s.logf.Name()))
}

func (s *server) stop() {
	if s.cmd == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	_, _ = s.cmd.Process.Wait()
	s.cmd = nil
	s.logf.Close()
	time.Sleep(300 * time.Millisecond)
}

func (s *server) restart() { s.stop(); s.start() }

func (s *server) call(method, path string, body any) (int, []byte) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://"+s.addr+"/api"+path, r)
	must(err)
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	must(err)
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

type status struct {
	ID, Version, State, Notice, Phase, Error, Pin, LastFailure, Repo string
	Installed                                                        bool
	Latest                                                           *struct{ Version string }
	Source                                                           *struct{ Repo, Fingerprint string }
	Events                                                           []struct{ Kind, Version, Detail string }
}

func (s *server) module() status {
	code, b := s.call("GET", "/modules", nil)
	if code != 200 {
		must(fmt.Errorf("GET /modules: %d %s", code, b))
	}
	var r struct{ Modules []status }
	must(json.Unmarshal(b, &r))
	for _, m := range r.Modules {
		if m.ID == id {
			return m
		}
	}
	must(fmt.Errorf("%s isn't listed", id))
	return status{}
}

func (s *server) check() status {
	code, b := s.call("POST", "/modules/check", nil)
	ok(code == 200, "the check answers (%d)", code)
	_ = b
	time.Sleep(200 * time.Millisecond)
	return s.module()
}

func (s *server) waitInstalled() status {
	for i := 0; i < 600; i++ {
		m := s.module()
		if m.Phase == "idle" || m.Phase == "failed" {
			return m
		}
		time.Sleep(100 * time.Millisecond)
	}
	must(fmt.Errorf("an install never finished"))
	return status{}
}

func (s *server) install(ver string, confirm bool) (int, string) {
	code, b := s.call("POST", "/modules/"+id+"/install", map[string]any{"version": ver, "confirm": confirm})
	return code, string(b)
}

func (s *server) versions() []map[string]any {
	code, b := s.call("GET", "/modules/"+id+"/versions", nil)
	if code != 200 {
		must(fmt.Errorf("versions: %d %s", code, b))
	}
	var l struct{ Versions []map[string]any }
	must(json.Unmarshal(b, &l))
	return l.Versions
}

func findVersion(vs []map[string]any, ver string) map[string]any {
	for _, v := range vs {
		if v["version"] == ver {
			return v
		}
	}
	return nil
}

func writePhotos(dir, prefix string, n int) {
	must(os.MkdirAll(dir, 0o755))
	for i := range n {
		var buf bytes.Buffer
		must(jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8+i, 8)), nil))
		must(os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d.jpg", prefix, i)), buf.Bytes(), 0o644))
	}
}

func randomText() string {
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func main() {
	work := flag.String("work", "", "an empty folder for everything this run makes")
	hold := flag.Bool("hold", false, "stop after the releases are published, with the server running, to look at it in a browser; ends when the file <work>/stop appears")
	flag.Parse()
	if *work == "" {
		fmt.Println("usage: e2e -work <empty folder>")
		os.Exit(2)
	}
	must(os.MkdirAll(*work, 0o755))
	abs, err := filepath.Abs(*work)
	must(err)
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}

	step("build the server (test-only feed) and a helper that can play any version")
	bin := filepath.Join(abs, "wms"+exe)
	build := exec.Command("go", "build", "-tags", "updatetest", "-ldflags", "-X main.version="+serverVersion, "-o", bin, "./cmd/wms")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	must(build.Run())
	helperPath := filepath.Join(abs, "fake"+exe)
	build = exec.Command("go", "build", "-o", helperPath, "./internal/modules/testdata/fakehelper")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	must(build.Run())
	helper, err := os.ReadFile(helperPath)
	must(err)

	pub, key, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	f := &feed{repos: map[string][]update.GHRelease{}, files: map[string][]byte{}}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	f.base = "http://" + l.Addr().String()
	go func() { _ = http.Serve(l, f) }()

	lp, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	addr := lp.Addr().String()
	lp.Close()
	s := &server{bin: bin, data: filepath.Join(abs, "data"), addr: addr, keyB64: base64.StdEncoding.EncodeToString(pub), feedURL: f.base + "/feed"}
	defer s.stop()

	const repo = "WittyPhotos"
	f.publish(repo, "0.1.5", key, helper)
	f.publish(repo, "0.3.0", key, helper)
	f.publish(repo, "0.3.1", key, helper, "crash")
	f.publish(repo, "0.4.0", key, helper)
	f.publish(repo, "9.0.0", key, helper) // in the range too: see below
	f.mu.Lock()
	f.repos[repo] = f.repos[repo][1:] // the newest isn't out yet
	f.mu.Unlock()

	step("first start, and setup through the API")
	s.start()
	password := randomText()
	code, b := s.call("POST", "/setup", map[string]string{"name": "e2e", "password": password, "device": "e2e"})
	ok(code == 200, "setup answers (%d)", code)
	var setup struct{ Token string }
	must(json.Unmarshal(b, &setup))
	s.token = setup.Token
	if *hold {
		s.call("PUT", "/settings", map[string]string{"setupDone": "1"})
		f.mu.Lock()
		f.repos[repo] = append(f.repos[repo], update.GHRelease{TagName: "not-a-version"})
		f.mu.Unlock()
		must(os.WriteFile(filepath.Join(abs, "login.txt"), []byte("http://"+addr+"\ne2e\n"+password+"\n"), 0o600))
		fmt.Println("holding: address and test sign-in are in", filepath.Join(abs, "login.txt"))
		for {
			if _, err := os.Stat(filepath.Join(abs, "stop")); err == nil {
				return
			}
			time.Sleep(time.Second)
		}
	}
	m := s.module()
	ok(m.State == "builtin", "it starts on its built-in copy (%s %s)", m.Version, m.State)
	builtin := m.Version
	photos := filepath.Join(abs, "photos")
	writePhotos(photos, "first", 3)
	code, b = s.call("POST", "/libraries", map[string]any{"name": "Photos", "type": "photos", "paths": []string{photos}})
	ok(code == 201, "a photo library can be made (%d)", code)
	var lib struct{ ID int64 }
	must(json.Unmarshal(b, &lib))

	step("update: every version is listed, the newest offered, and it installs with no restart")
	m = s.check()
	ok(m.Latest != nil && m.Latest.Version == "0.4.0", "0.4.0 is offered (%+v)", m.Latest)
	vs := s.versions()
	ok(len(vs) >= 5, "every version is listed (%d)", len(vs))
	code, _ = s.install("0.3.0", false)
	ok(code == 202, "0.3.0 starts installing (%d)", code)
	m = s.waitInstalled()
	ok(m.Version == "0.3.0" && m.State == "installed" && m.Error == "", "0.3.0 is in use (%s %s %s)", m.Version, m.State, m.Error)

	step("a version that keeps crashing rolls back by itself, with the reason")
	code, _ = s.install("0.3.1", false)
	ok(code == 202, "0.3.1 starts installing (%d)", code)
	m = s.waitInstalled()
	ok(m.Version == "0.3.1", "0.3.1 is in use, since it starts fine (%s)", m.Version)
	writePhotos(photos, "second", 8)
	s.call("POST", fmt.Sprintf("/libraries/%d/scan", lib.ID), nil)
	// Each failure is a start of a new program, which a virus scanner can hold up.
	for i := 0; i < 1800 && s.module().Version == "0.3.1"; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	m = s.module()
	ok(m.Version == "0.3.0", "it went back to 0.3.0 (%s)", m.Version)
	ok(strings.Contains(m.Notice, "kept failing"), "and says why: %s", m.Notice)
	ok(m.LastFailure != "", "with the failure on record: %s", m.LastFailure)
	v := findVersion(s.versions(), "0.3.1")
	ok(v != nil && v["blocked"] != nil, "and 0.3.1 is marked: %v", v["blocked"])

	step("that survives a restart")
	s.restart()
	m = s.module()
	ok(m.Version == "0.3.0", "still 0.3.0 (%s)", m.Version)
	ok(strings.Contains(m.Notice, "kept failing"), "still explained")

	step("a newer version is still offered, and installs")
	m = s.check()
	code, _ = s.install("0.4.0", false)
	ok(code == 202, "0.4.0 starts installing (%d)", code)
	m = s.waitInstalled()
	ok(m.Version == "0.4.0" && m.Notice == "", "0.4.0 is in use (%s, %q)", m.Version, m.Notice)

	step("going back to an older version warns first, and then is pinned")
	code, body := s.install("0.1.5", false)
	ok(code == 409 && strings.Contains(body, `"confirm"`) && strings.Contains(body, "older"), "it asks first (%d)", code)
	ok(s.module().Version == "0.4.0", "and nothing moved")
	code, _ = s.install("0.1.5", true)
	ok(code == 202, "confirmed, it starts (%d)", code)
	m = s.waitInstalled()
	ok(m.Version == "0.1.5" && m.Pin == "0.1.5", "0.1.5 is in use and pinned, older than the built-in %s (%s pin %q, %s)", builtin, m.Version, m.Pin, m.Error)

	step("a pinned version stays through a check and a restart")
	m = s.check()
	ok(m.Version == "0.1.5" && m.Latest != nil, "still 0.1.5, with a newer one offered (%s)", m.Version)
	s.restart()
	m = s.module()
	ok(m.Version == "0.1.5" && m.Pin == "0.1.5", "still 0.1.5 and pinned after a restart (%s %q)", m.Version, m.Pin)

	step("back to the latest ends the pin")
	code, _ = s.call("POST", "/modules/"+id+"/latest", nil)
	ok(code == 202, "it starts (%d)", code)
	m = s.waitInstalled()
	ok(m.Version == "0.4.0" && m.Pin == "", "0.4.0, not pinned (%s %q %s)", m.Version, m.Pin, m.Error)
	code, _ = s.call("POST", "/modules/"+id+"/pin", nil)
	ok(code == 200 && s.module().Pin == "0.4.0", "pinning what's in use works")
	code, _ = s.call("DELETE", "/modules/"+id+"/pin", nil)
	ok(code == 200 && s.module().Pin == "", "and unpinning")

	step("a version withdrawn for security reasons stops, and can't be chosen")
	f.withdraw(repo, 1, key, wittymodule.Withdrawn{Version: "0.4.0", Severity: wittymodule.SeveritySecurity, Reason: "it reads past a buffer"})
	m = s.check()
	ok(m.Version != "0.4.0" && strings.Contains(m.Notice, "reads past a buffer"), "0.4.0 went out of use (%s, %s)", m.Version, m.Notice)
	code, body = s.install("0.4.0", true)
	ok(code == 409 && strings.Contains(body, "security"), "and it is refused (%d)", code)

	step("offline: the list stays, what's installed runs, and switching back needs no network")
	f.mu.Lock()
	f.gone = true
	f.mu.Unlock()
	s.restart()
	v = findVersion(s.versions(), "0.3.0")
	ok(v != nil, "the list of versions is still there")
	s.call("POST", "/modules/check", nil)
	ok(s.module().Phase == "idle", "a failed check breaks nothing")
	f.mu.Lock()
	f.gone = false
	f.mu.Unlock()

	step("a fork: warned first, held to its own key, and treated as the same module")
	forkPub, forkKey, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	const forkRepo = "WittyPhotosFork"
	f.publish(forkRepo, "1.0.0", forkKey, helper)
	f.publish(forkRepo, "1.1.0", key, helper) // signed by the wrong key for this source
	fk := base64.StdEncoding.EncodeToString(forkPub)
	code, body = s.call2("PUT", "/modules/"+id+"/source", map[string]any{"repo": "wittywilla/" + forkRepo, "key": fk})
	ok(code == 409 && strings.Contains(body, "trust"), "it warns first (%d)", code)
	code, _ = s.call2("PUT", "/modules/"+id+"/source", map[string]any{"repo": "wittywilla/" + forkRepo, "key": fk, "confirm": true})
	ok(code == 200, "confirmed, it is the source (%d)", code)
	m = s.check()
	ok(m.Source != nil && m.Repo == "wittywilla/"+forkRepo, "the page shows where it comes from (%+v)", m.Source)
	ok(m.Version == builtin, "it runs the built-in copy until a version is chosen (%s)", m.Version)
	ok(findVersion(s.versions(), "1.1.0") == nil, "a release signed by another key isn't even listed")
	code, _ = s.install("1.0.0", true)
	ok(code == 202, "the fork's version installs (%d)", code)
	m = s.waitInstalled()
	ok(m.Version == "1.0.0" && m.Error == "", "and is in use, or says why not (%s %q)", m.Version, m.Error)
	s.restart()
	ok(s.module().Source != nil, "the choice survives a restart")
	code, _ = s.call("DELETE", "/modules/"+id+"/source", nil)
	m = s.module()
	ok(code == 200 && m.Source == nil && m.Version == builtin && m.Repo == "wittywilla/"+repo, "and back to the official version (%s)", m.Repo)

	step("what is installed limits what libraries can be made")
	code, body = s.call2("POST", "/modules/"+id+"/remove", nil)
	ok(code == 409 && strings.Contains(body, "Photos"), "it can't be removed while a library needs it (%d)", code)
	code, _ = s.call("DELETE", fmt.Sprintf("/libraries/%d", lib.ID), nil)
	ok(code == 200, "the library goes (%d)", code)
	code, _ = s.call("POST", "/modules/"+id+"/remove", nil)
	ok(code == 200 && !s.module().Installed, "then the module can (%d)", code)
	code, body = s.call2("POST", "/libraries", map[string]any{"name": "Photos", "type": "photos", "paths": []string{photos}})
	ok(code == 409 && strings.Contains(body, "module-missing"), "a photo library is refused, naming what it needs (%d)", code)
	code, _ = s.call("POST", "/modules/"+id+"/add", nil)
	ok(code == 200 && s.module().Installed, "adding it back works straight away, offline (%d)", code)
	code, _ = s.call("POST", "/libraries", map[string]any{"name": "Photos", "type": "photos", "paths": []string{photos}})
	ok(code == 201, "and the library can be made again (%d)", code)
	s.restart()
	ok(s.module().Installed, "which survives a restart")

	fmt.Println()
	if len(failures) > 0 {
		fmt.Printf("%d FAILED:\n  %s\n", len(failures), strings.Join(failures, "\n  "))
		s.stop()
		os.Exit(1)
	}
	fmt.Println("everything passed")
}

func (s *server) call2(method, path string, body any) (int, string) {
	code, b := s.call(method, path, body)
	return code, string(b)
}
